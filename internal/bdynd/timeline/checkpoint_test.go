package timeline

import (
	"strings"
	"testing"
)

func TestCheckpointBlockRoundTrip(t *testing.T) {
	tree := "main.go sha256:o1\nreadme.md sha256:o2\n"
	ch := CheckpointHeader{
		CheckpointID: "checkpoint-main-000100",
		ProjectID:    "main",
		BaseSeq:      100,
		TreeRoot:     HashBytes([]byte(tree)),
		FileCount:    2,
		TotalBytes:   uint64(len(tree)),
		Compression:  0,
	}
	objects := []ObjectRef{
		{ObjectID: "sha256:o1", Size: 8, SHA256: "func main"},
		{ObjectID: "sha256:o2", Size: 5, SHA256: "readme"},
	}
	encoded, err := encodeCheckpointBlock(ch, []byte(tree), objects)
	if err != nil {
		t.Fatal(err)
	}
	gotH, gotTree, gotObjects, err := decodeCheckpointBlock(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if gotH.CheckpointID != ch.CheckpointID || gotH.BaseSeq != 100 {
		t.Fatalf("header mismatch: %+v", gotH)
	}
	if string(gotTree) != tree {
		t.Fatalf("tree mismatch: %q", gotTree)
	}
	if len(gotObjects) != 2 || gotObjects[0].ObjectID != "sha256:o1" {
		t.Fatalf("objects mismatch: %+v", gotObjects)
	}
	if gotH.TreeRoot != ch.TreeRoot {
		t.Fatalf("tree root mismatch")
	}
}

func TestCheckpointBlockCorruptRejected(t *testing.T) {
	ch := CheckpointHeader{CheckpointID: "c1", ProjectID: "main", BaseSeq: 1}
	encoded, err := encodeCheckpointBlock(ch, []byte("tree"), nil)
	if err != nil {
		t.Fatal(err)
	}
	beginEnd := strings.IndexByte(string(encoded), '\n') + 1
	encoded[beginEnd+len(encoded)/2-5] ^= 0xFF
	if _, _, _, err := decodeCheckpointBlock(encoded); err == nil {
		t.Fatal("corrupt checkpoint should fail")
	}
}
