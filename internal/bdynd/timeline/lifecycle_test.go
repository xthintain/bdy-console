package timeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTestTimeline populates a test store with two nodes, flushes them, and
// builds a segment. It returns the store, transfer, and the segment id.
func buildTestTimeline(t *testing.T) (*Store, *memoryTransfer, string) {
	t.Helper()
	store, transfer := newTestStore(t)
	ctx := context.Background()
	n1 := NodeMeta{CommitID: "sha256:c1", Branch: "main", Seq: 1, Message: "one", TimestampMs: 1000}
	n2 := NodeMeta{CommitID: "sha256:c2", ParentID: "sha256:c1", Branch: "main", Seq: 2, Message: "two", TimestampMs: 2000}
	if err := store.AppendNode(ctx, n1); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendNode(ctx, n2); err != nil {
		t.Fatal(err)
	}
	// Flush node 1 (upsert a.txt -> o1) and node 2 (upsert b.txt -> o2).
	nb1, err := store.FlushNode(ctx, n1, []DeltaOp{{Op: OpUpsert, Path: "a.txt", ObjectID: "sha256:o1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	nb2, err := store.FlushNode(ctx, n2, []DeltaOp{{Op: OpUpsert, Path: "b.txt", ObjectID: "sha256:o2"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = nb1
	_ = nb2
	// Build a segment from both node blocks.
	segID, err := store.BuildSegment(ctx, "main",
		[]NodeMeta{n1, n2},
		[][]byte{mustNodeBytes(t, n1, "a.txt", "sha256:o1"), mustNodeBytes(t, n2, "b.txt", "sha256:o2")},
	)
	if err != nil {
		t.Fatal(err)
	}
	return store, transfer, segID
}

func mustNodeBytes(t *testing.T, n NodeMeta, path, oid string) []byte {
	t.Helper()
	b, err := encodeNodeBlockForTest(NodeBlockHeader{NodeID: n.CommitID, ParentNodeID: n.ParentID, ProjectID: n.Branch, Seq: n.Seq, TimestampMs: n.TimestampMs},
		[]DeltaOp{{Op: OpUpsert, Path: path, ObjectID: oid}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVerifyBlockAcceptsValidAndRejectsCorrupt(t *testing.T) {
	store, _, _ := buildTestTimeline(t)
	ctx := context.Background()
	// FlushNode wrote a node block; verify it.
	n := NodeMeta{CommitID: "sha256:v1", Branch: "main", Seq: 3}
	blockID, err := store.FlushNode(ctx, n, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyBlock(ctx, blockID); err != nil {
		t.Fatalf("verify valid block: %v", err)
	}
	// Corrupt the local file and verify it now fails.
	p := store.findLocalBlock(blockID)
	data, _ := os.ReadFile(p)
	data[len(data)/2] ^= 0xFF
	_ = os.WriteFile(p, data, 0o644)
	if err := store.VerifyBlock(ctx, blockID); err == nil {
		t.Fatal("verify corrupt block should fail")
	}
}

func TestRestoreNodeBuildsTreeFromCheckpointAndDelta(t *testing.T) {
	store, _, _ := buildTestTimeline(t)
	ctx := context.Background()
	// Build a checkpoint over both nodes.
	cpID, err := store.BuildCheckpoint(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if cpID == "" {
		t.Fatal("empty checkpoint id")
	}
	// Restore node 2: checkpoint base tree has a.txt+b.txt, node2 delta b.txt.
	tree, err := store.RestoreNode(ctx, "sha256:c2")
	if err != nil {
		t.Fatal(err)
	}
	if tree["a.txt"] != "sha256:o1" || tree["b.txt"] != "sha256:o2" {
		t.Fatalf("restored tree mismatch: %v", tree)
	}
}

func TestBuildCheckpointRoundTrip(t *testing.T) {
	store, _, _ := buildTestTimeline(t)
	ctx := context.Background()
	cpID, err := store.BuildCheckpoint(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	p := store.findLocalBlock(cpID)
	if p == "" {
		t.Fatalf("checkpoint local file missing")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	h, tree, objs, err := decodeCheckpointBlock(data)
	if err != nil {
		t.Fatal(err)
	}
	if h.BaseSeq != 2 {
		t.Fatalf("base seq=%d, want 2", h.BaseSeq)
	}
	if !strings.Contains(string(tree), "a.txt") || !strings.Contains(string(tree), "b.txt") {
		t.Fatalf("tree content missing entries: %s", tree)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
}

func TestRepackSegmentsBuildsArchive(t *testing.T) {
	store, transfer, segID := buildTestTimeline(t)
	ctx := context.Background()
	// Upload segment first so it's active and remotely present.
	if err := store.UploadBlock(ctx, segID); err != nil {
		t.Fatal(err)
	}
	archID, err := store.RepackSegments(ctx, "main", []string{segID})
	if err != nil {
		t.Fatal(err)
	}
	if archID == "" {
		t.Fatal("empty archive id")
	}
	// Source segment should now be superseded.
	st, _ := store.DB.BlockState(segID)
	if st != StateSuperseded {
		t.Fatalf("segment state after repack=%q, want superseded", st)
	}
	// Archive should exist locally and parse.
	p := store.findLocalBlock(archID)
	if p == "" {
		t.Fatal("archive local file missing")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := decodeArchiveBlock(data); err != nil {
		t.Fatalf("archive decode: %v", err)
	}
	_ = transfer
}

func TestPruneCollectsSupersededAfterGrace(t *testing.T) {
	store, _, segID := buildTestTimeline(t)
	ctx := context.Background()
	// Mark segment superseded and backdate its created_at past grace.
	if err := store.DB.SetBlockState(segID, StateSuperseded); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.sql.Exec(`UPDATE blocks SET created_at=? WHERE id=?`, time.Now().Add(-10*24*time.Hour).UnixMilli(), segID); err != nil {
		t.Fatal(err)
	}
	pruned, err := store.Prune(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range pruned {
		if id == segID {
			found = true
		}
	}
	if !found {
		t.Fatalf("segment not pruned: %v", pruned)
	}
}

func TestStatusSummary(t *testing.T) {
	store, _, _ := buildTestTimeline(t)
	ctx := context.Background()
	status, err := store.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status["nodes"] != 2 {
		t.Fatalf("status nodes=%v, want 2", status["nodes"])
	}
	if status["objects"] != 0 {
		t.Fatalf("status objects=%v, want 0", status["objects"])
	}
	branches, ok := status["branches"].([]string)
	if !ok || len(branches) != 1 || branches[0] != "main" {
		t.Fatalf("status branches=%v", status["branches"])
	}
	_ = filepath.Join
}
