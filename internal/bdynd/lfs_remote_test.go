package bdynd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLFSPushUploadsMissingObjects(t *testing.T) {
	r := newTestRepo(t)
	must(t, TrackPattern(r, "*.bin"))
	writeFile(t, filepath.Join(r.Root, "large.bin"), strings.Repeat("x", 4096))
	must(t, Add(r, []string{"large.bin"}))
	idx, _ := LoadIndex(r)
	remote := newMemoryRemote()
	if err := PushLFS(context.Background(), r, remote, "/apps/baiduyunStorage/nd/repos/demo"); err != nil {
		t.Fatal(err)
	}
	remotePath := RemoteLFSObjectPath("/apps/baiduyunStorage/nd/repos/demo", idx.Entries["large.bin"].LFSOID)
	if ok, _ := remote.Exists(context.Background(), remotePath); !ok {
		t.Fatalf("missing remote object %s", remotePath)
	}
}

func TestLFSFetchAndCheckoutRestoresLargeFile(t *testing.T) {
	source := newTestRepo(t)
	must(t, TrackPattern(source, "*.bin"))
	writeFile(t, filepath.Join(source.Root, "large.bin"), "large-content")
	must(t, Add(source, []string{"large.bin"}))
	if _, err := Commit(source, CommitOptions{Message: "large"}); err != nil {
		t.Fatal(err)
	}
	remote := newMemoryRemote()
	must(t, PushLFS(context.Background(), source, remote, "/apps/baiduyunStorage/nd/repos/demo"))

	target := cloneRepoMetadataOnly(t, source)
	must(t, FetchLFS(context.Background(), target, remote, "/apps/baiduyunStorage/nd/repos/demo"))
	must(t, CheckoutLFS(target))
	if got := readFile(t, filepath.Join(target.Root, "large.bin")); got != "large-content" {
		t.Fatalf("large.bin=%q", got)
	}
}

type memoryRemote struct {
	files map[string][]byte
}

func newMemoryRemote() *memoryRemote {
	return &memoryRemote{files: map[string][]byte{}}
}

func (m *memoryRemote) UploadFile(ctx context.Context, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	m.files[remotePath] = append([]byte(nil), data...)
	return nil
}

func (m *memoryRemote) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	data, ok := m.files[remotePath]
	if !ok {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(localPath, data, 0o644)
}

func (m *memoryRemote) Exists(ctx context.Context, remotePath string) (bool, error) {
	_, ok := m.files[remotePath]
	return ok, nil
}

func cloneRepoMetadataOnly(t *testing.T, source Repo) Repo {
	t.Helper()
	target := newTestRepo(t)
	for _, name := range []string{"index.json", "attributes.json"} {
		data, err := os.ReadFile(filepath.Join(source.Dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target.Dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return target
}
