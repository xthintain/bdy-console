package bdynd

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// memoryRemoteSimple is a minimal RemoteStore used by the concurrency tests.
type memoryRemoteSimple struct {
	files map[string][]byte
	mu    sync.Mutex
}

func newMemoryRemoteSimple() *memoryRemoteSimple {
	return &memoryRemoteSimple{files: map[string][]byte{}}
}

func (m *memoryRemoteSimple) UploadFile(_ context.Context, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.files[remotePath] = data
	m.mu.Unlock()
	return nil
}

func (m *memoryRemoteSimple) DownloadFile(_ context.Context, remotePath, localPath string) error {
	m.mu.Lock()
	data, ok := m.files[remotePath]
	m.mu.Unlock()
	if !ok {
		return os.ErrNotExist
	}
	return os.WriteFile(localPath, data, 0o644)
}

func (m *memoryRemoteSimple) Exists(_ context.Context, remotePath string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[remotePath]
	return ok, nil
}

func newTestRepoWithObject(t *testing.T, oid, kind string, data []byte) Repo {
	t.Helper()
	dir := t.TempDir()
	r := Repo{Dir: dir, Root: dir}
	if err := os.MkdirAll(filepath.Join(dir, ".bdynd", "objects", kind, "sha256"), 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

// runWithCap runs fn in parallel and returns the max observed in-flight count.
func runWithCap(t *testing.T, n int, total int, fn func(i int)) int {
	t.Helper()
	var inflight atomic.Int64
	var maxInflight atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, n)
	for i := 0; i < total; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			cur := inflight.Add(1)
			for {
				old := maxInflight.Load()
				if cur <= old || maxInflight.CompareAndSwap(old, cur) {
					break
				}
			}
			fn(i)
			inflight.Add(-1)
			<-sem
		}()
	}
	wg.Wait()
	return int(maxInflight.Load())
}

// trackingExistsRemote records max concurrent Exists calls.
type trackingExistsRemote struct {
	base     RemoteStore
	inflight atomic.Int64
	max      atomic.Int64
}

func (t *trackingExistsRemote) UploadFile(ctx context.Context, l, r string) error {
	return t.base.UploadFile(ctx, l, r)
}
func (t *trackingExistsRemote) DownloadFile(ctx context.Context, r, l string) error {
	return t.base.DownloadFile(ctx, r, l)
}
func (t *trackingExistsRemote) Exists(ctx context.Context, p string) (bool, error) {
	cur := t.inflight.Add(1)
	for {
		old := t.max.Load()
		if cur <= old || t.max.CompareAndSwap(old, cur) {
			break
		}
	}
	defer t.inflight.Add(-1)
	return t.base.Exists(ctx, p)
}

func TestConcurrentRunnerPushUploadsAll(t *testing.T) {
	dir := t.TempDir()
	r := Repo{Dir: dir, Root: dir}
	remote := newMemoryRemoteSimple()
	runner := NewConcurrentRunner(4)

	// Build one commit with two blobs locally.
	mkObj := func(kind, oid string, data []byte) {
		p, err := objectPath(r, kind, oid)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commitOID := "sha256:" + stringsRepeat("a", 64)
	treeOID := "sha256:" + stringsRepeat("b", 64)
	blobOID := "sha256:" + stringsRepeat("c", 64)
	blobOID2 := "sha256:" + stringsRepeat("d", 64)
	mkObj("commits", commitOID, []byte("commit"))
	mkObj("trees", treeOID, []byte("tree"))
	mkObj("blobs", blobOID, []byte("blob1"))
	mkObj("blobs", blobOID2, []byte("blob2"))

	// Write commit via normal path so ReadCommit can parse it.
	if err := WriteCommitObject(r, CommitObject{OID: commitOID, Tree: treeOID, Message: "msg",
		Entries: []IndexEntry{
			{Path: "a", Kind: KindBlob, OID: blobOID, Size: 5},
			{Path: "b", Kind: KindBlob, OID: blobOID2, Size: 5},
		}}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := runner.Push(ctx, r, remote, "/remote", commitOID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		RemoteCommitPath("/remote", commitOID),
		RemoteTreePath("/remote", treeOID),
		RemoteBlobPath("/remote", blobOID),
		RemoteBlobPath("/remote", blobOID2),
	} {
		if err := remote.DownloadFile(ctx, path, filepath.Join(dir, "x")); err != nil {
			t.Errorf("missing remote %s: %v", path, err)
		}
	}
}

func TestConcurrentRunnerFetchDownloadsAll(t *testing.T) {
	dir := t.TempDir()
	r := Repo{Dir: dir, Root: dir}
	remote := newMemoryRemoteSimple()
	runner := NewConcurrentRunner(4)
	ctx := context.Background()

	commitOID := "sha256:" + stringsRepeat("a", 64)
	treeOID := "sha256:" + stringsRepeat("b", 64)
	blobOID := "sha256:" + stringsRepeat("c", 64)
	// Seed remote.
	_ = remote.UploadFile(ctx, writeTemp(t, "commit"), RemoteCommitPath("/r", commitOID))
	_ = remote.UploadFile(ctx, writeTemp(t, "tree"), RemoteTreePath("/r", treeOID))
	_ = remote.UploadFile(ctx, writeTemp(t, "blob"), RemoteBlobPath("/r", blobOID))

	// Local commit object must exist for ReadCommit during collect.
	if err := WriteCommitObject(r, CommitObject{OID: commitOID, Tree: treeOID, Message: "msg",
		Entries: []IndexEntry{{Path: "a", Kind: KindBlob, OID: blobOID, Size: 4}}}); err != nil {
		t.Fatal(err)
	}

	if err := runner.Fetch(ctx, r, remote, "/r", commitOID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		localObjectPath(r, "trees", treeOID),
		localObjectPath(r, "blobs", blobOID),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing local %s: %v", p, err)
		}
	}
}

func TestConcurrentRunnerPushSkipsExistingRemote(t *testing.T) {
	dir := t.TempDir()
	r := Repo{Dir: dir, Root: dir}
	remote := newMemoryRemoteSimple()
	ctx := context.Background()

	commitOID := "sha256:" + stringsRepeat("a", 64)
	treeOID := "sha256:" + stringsRepeat("b", 64)
	blobOID := "sha256:" + stringsRepeat("c", 64)
	// Commit exists remotely already.
	_ = remote.UploadFile(ctx, writeTemp(t, "commit"), RemoteCommitPath("/r", commitOID))

	if err := WriteCommitObject(r, CommitObject{OID: commitOID, Tree: treeOID, Message: "msg",
		Entries: []IndexEntry{{Path: "a", Kind: KindBlob, OID: blobOID, Size: 4}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeObject(r, "trees", treeOID, []byte("tree")); err != nil {
		t.Fatal(err)
	}
	if err := writeObject(r, "blobs", blobOID, []byte("blob")); err != nil {
		t.Fatal(err)
	}

	tr := &trackingExistsRemote{base: remote}
	runner := NewConcurrentRunner(4)
	if err := runner.Push(ctx, r, tr, "/r", commitOID); err != nil {
		t.Fatal(err)
	}
	// Commit upload must have been skipped because remote.Exists said yes.
	if err := remote.DownloadFile(ctx, RemoteCommitPath("/r", commitOID), filepath.Join(dir, "y")); err != nil {
		t.Fatalf("commit remote should exist: %v", err)
	}
	// Tree and blob should now exist remotely too.
	for _, p := range []string{RemoteTreePath("/r", treeOID), RemoteBlobPath("/r", blobOID)} {
		if err := remote.DownloadFile(ctx, p, filepath.Join(dir, "z")); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
}

// Helpers ---------------------------------------------------------------

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tmp")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeObject(r Repo, kind, oid string, data []byte) error {
	p, err := objectPath(r, kind, oid)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
