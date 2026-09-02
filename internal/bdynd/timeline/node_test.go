package timeline

import "testing"

func TestNodeBlockRoundTrip(t *testing.T) {
	h := NodeBlockHeader{
		NodeID:       "sha256:abc",
		ParentNodeID: "sha256:prev",
		ProjectID:    "main",
		Author:       "tester",
		TimestampMs:  1756800000000,
		Seq:          7,
		Compression:  0,
	}
	ops := []DeltaOp{
		{Op: OpUpsert, Path: "src/main.go", ObjectID: "sha256:obj1"},
		{Op: OpDelete, Path: "old.txt", ObjectID: ""},
	}
	refs := []ObjectRef{
		{ObjectID: "sha256:obj1", Size: 128, SHA256: "hash1"},
	}
	encoded, err := encodeNodeBlockForTest(h, ops, refs)
	if err != nil {
		t.Fatal(err)
	}
	gotH, gotOps, gotRefs, err := decodeNodeBlock(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if gotH.NodeID != h.NodeID || gotH.Seq != h.Seq || gotH.ParentNodeID != h.ParentNodeID {
		t.Fatalf("header mismatch: %+v", gotH)
	}
	if len(gotOps) != 2 || gotOps[0].Path != "src/main.go" || gotOps[1].Op != OpDelete {
		t.Fatalf("ops mismatch: %+v", gotOps)
	}
	if len(gotRefs) != 1 || gotRefs[0].ObjectID != "sha256:obj1" {
		t.Fatalf("refs mismatch: %+v", gotRefs)
	}
}

func TestNodeBlockCorruptIsRejected(t *testing.T) {
	h := NodeBlockHeader{NodeID: "n1", Seq: 1}
	encoded, err := encodeNodeBlockForTest(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt a byte inside the inner frame payload (not the wrapper tags).
	innerStart := 0
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '\n' {
			innerStart = i + 1
			break
		}
	}
	// Flip a byte in the middle of the inner framed payload.
	encoded[innerStart+len(encoded)/2-5] ^= 0xFF
	if _, _, _, err := decodeNodeBlock(encoded); err == nil {
		t.Fatal("corrupt node block should fail")
	}
}

func TestBlockIDAndWrap(t *testing.T) {
	id := BlockID(KindArchive, "main", 1, 100)
	if id != "archive-main-000001-000100" {
		t.Fatalf("id=%q", id)
	}
	inner := []byte("hello")
	wrapped := wrapBlock(KindNode, "n1", inner)
	if _, err := unwrapBlock(wrapped); err != nil {
		t.Fatalf("unwrap failed: %v", err)
	}
}
