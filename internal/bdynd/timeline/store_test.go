package timeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// memoryTransfer is an in-memory RemoteTransfer for tests.
type memoryTransfer struct {
	files map[string][]byte
}

func newMemoryTransfer() *memoryTransfer {
	return &memoryTransfer{files: map[string][]byte{}}
}

func (m *memoryTransfer) UploadFile(_ context.Context, localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	m.files[remotePath] = data
	return nil
}

func (m *memoryTransfer) DownloadFile(_ context.Context, remotePath, localPath string) error {
	data, ok := m.files[remotePath]
	if !ok {
		return os.ErrNotExist
	}
	return os.WriteFile(localPath, data, 0o644)
}

func (m *memoryTransfer) Exists(_ context.Context, remotePath string) (bool, error) {
	_, ok := m.files[remotePath]
	return ok, nil
}

func (m *memoryTransfer) ListFiles(_ context.Context, _ string) ([]string, error) {
	var out []string
	for p := range m.files {
		out = append(out, p)
	}
	return out, nil
}

func (m *memoryTransfer) DeleteFiles(_ context.Context, paths []string) error {
	for _, p := range paths {
		delete(m.files, p)
	}
	return nil
}

func newTestStore(t *testing.T) (*Store, *memoryTransfer) {
	t.Helper()
	root := t.TempDir()
	layout := NewLayout(filepath.Join(root, ".bdynd", "timeline"))
	remote := NewRemoteLayout("/apps/baiduyunStorage/nd/repos/test")
	transfer := newMemoryTransfer()
	store, err := NewStore(filepath.Join(root, "index.sqlite"), layout, remote, transfer, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	return store, transfer
}

func TestStoreAppendFlushAndList(t *testing.T) {
	store, _ := newTestStore(t)
	n := NodeMeta{CommitID: "sha256:c1", Branch: "main", Seq: 1, Message: "first", TimestampMs: 1000}
	if err := store.AppendNode(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	blockID, err := store.FlushNode(context.Background(), n, []DeltaOp{{Op: OpUpsert, Path: "a.txt", ObjectID: "sha256:o1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if blockID == "" {
		t.Fatal("empty block id")
	}
	nodes, err := store.ListNodesByBranch("main")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].CommitID != "sha256:c1" {
		t.Fatalf("nodes=%+v", nodes)
	}
}

func TestStoreBuildSegmentAndUpload(t *testing.T) {
	store, transfer := newTestStore(t)
	n1 := NodeMeta{CommitID: "sha256:c1", Branch: "main", Seq: 1, TimestampMs: 1000}
	n2 := NodeMeta{CommitID: "sha256:c2", ParentID: "sha256:c1", Branch: "main", Seq: 2, TimestampMs: 2000}
	nb1, _ := encodeNodeBlockForTest(NodeBlockHeader{NodeID: "sha256:c1", Seq: 1}, []DeltaOp{{Op: OpUpsert, Path: "a.txt", ObjectID: "sha256:o1"}}, nil)
	nb2, _ := encodeNodeBlockForTest(NodeBlockHeader{NodeID: "sha256:c2", Seq: 2, ParentNodeID: "sha256:c1"}, []DeltaOp{{Op: OpDelete, Path: "a.txt"}}, nil)

	segID, err := store.BuildSegment(context.Background(), "main", []NodeMeta{n1, n2}, [][]byte{nb1, nb2})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UploadBlock(context.Background(), segID); err != nil {
		t.Fatal(err)
	}
	remotePath := store.Remote.ArchivePackPath(segID)
	ok, err := transfer.Exists(context.Background(), remotePath)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("remote file not uploaded: %s", remotePath)
	}
	state, _ := store.DB.BlockState(segID)
	if state != StateActive {
		t.Fatalf("state after upload=%q", state)
	}
}

func TestStoreUploadRejectsLocalCorruption(t *testing.T) {
	store, _ := newTestStore(t)
	n := NodeMeta{CommitID: "sha256:c1", Branch: "main", Seq: 1}
	blockID, err := store.FlushNode(context.Background(), n, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	localPath := store.findLocalBlock(blockID)
	if localPath == "" {
		t.Fatal("local block not found")
	}
	if err := os.WriteFile(localPath, []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.UploadBlock(context.Background(), blockID); err == nil {
		t.Fatal("upload should reject corrupted local file")
	}
}

func TestStoreUpdateRef(t *testing.T) {
	store, transfer := newTestStore(t)
	if err := store.UpdateRef(context.Background(), "main", "sha256:c1"); err != nil {
		t.Fatal(err)
	}
	ok, err := transfer.Exists(context.Background(), store.Remote.RefPath("main"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("remote ref not uploaded")
	}
}
