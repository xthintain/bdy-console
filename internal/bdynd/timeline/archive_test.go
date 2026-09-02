package timeline

import (
	"fmt"
	"testing"
)

func TestArchiveBlockRoundTrip(t *testing.T) {
	nb1, _ := encodeNodeBlockForTest(NodeBlockHeader{NodeID: "n1", Seq: 1, ProjectID: "main"}, []DeltaOp{{Op: OpUpsert, Path: "a.txt", ObjectID: "sha256:o1"}}, nil)
	nb2, _ := encodeNodeBlockForTest(NodeBlockHeader{NodeID: "n2", Seq: 2, ProjectID: "main"}, []DeltaOp{{Op: OpDelete, Path: "a.txt"}}, nil)
	sb1, err := encodeSegmentBlock(SegmentHeader{SegmentID: "segment-1-2", ProjectID: "main", BeginSeq: 1, EndSeq: 2, NodeCount: 2}, [][]byte{nb1, nb2})
	if err != nil {
		t.Fatal(err)
	}
	ah := ArchiveHeader{
		ArchiveID:    "archive-main-000001-000002",
		ProjectID:    "main",
		BeginSeq:     1,
		EndSeq:       2,
		SegmentCount: 1,
		NodeCount:    2,
		Compression:  0,
	}
	objects := []ObjectRef{
		{ObjectID: "sha256:o1", Size: 6, SHA256: "hello\n"},
	}
	encoded, err := encodeArchiveBlock(ah, [][]byte{sb1}, objects)
	if err != nil {
		t.Fatal(err)
	}
	gotH, gotSegs, gotObjects, err := decodeArchiveBlock(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if gotH.ArchiveID != ah.ArchiveID || gotH.BeginSeq != 1 || gotH.EndSeq != 2 {
		t.Fatalf("header mismatch: %+v", gotH)
	}
	if len(gotSegs) != 1 {
		t.Fatalf("segments=%d", len(gotSegs))
	}
	// Inner segment must decode back.
	_, gotNodes, _, err := decodeSegmentBlock(gotSegs[0])
	if err != nil {
		t.Fatalf("inner segment decode: %v", err)
	}
	if len(gotNodes) != 2 {
		t.Fatalf("inner nodes=%d", len(gotNodes))
	}
	if len(gotObjects) != 1 || gotObjects[0].ObjectID != "sha256:o1" {
		t.Fatalf("objects mismatch: %+v", gotObjects)
	}
}

func TestArchiveBlockObjectCorruptRejected(t *testing.T) {
	nb1, _ := encodeNodeBlockForTest(NodeBlockHeader{NodeID: "n1", Seq: 1}, nil, nil)
	sb1, _ := encodeSegmentBlock(SegmentHeader{SegmentID: "s1", ProjectID: "main", BeginSeq: 1, EndSeq: 1, NodeCount: 1}, [][]byte{nb1})
	ah := ArchiveHeader{ArchiveID: "a1", ProjectID: "main", BeginSeq: 1, EndSeq: 1, SegmentCount: 1, NodeCount: 1}
	objects := []ObjectRef{{ObjectID: "sha256:o1", Size: 4, SHA256: "data"}}
	encoded, err := encodeArchiveBlock(ah, [][]byte{sb1}, objects)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt inner bytes (not the outer tags).
	beginEnd := -1
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '\n' {
			beginEnd = i + 1
			break
		}
	}
	encoded[beginEnd+len(encoded)/2-5] ^= 0xFF
	if _, _, _, err := decodeArchiveBlock(encoded); err == nil {
		t.Fatal("corrupt archive should fail")
	}
}

var _ = fmt.Sprint
