package timeline

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDBRecordBlockAndTransition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b := NewBlockMeta("archive-main-000001-000100", KindArchive)
	b.State = StateCreating
	b.Size = 128
	b.SHA256 = "abc"
	b.RemotePath = "/apps/nd/archives/x.pack"
	if err := db.RecordBlock(b); err != nil {
		t.Fatal(err)
	}
	if got, err := db.BlockState("archive-main-000001-000100"); err != nil || got != StateCreating {
		t.Fatalf("state=%q err=%v", got, err)
	}
	if err := db.SetBlockState(b.ID, StateActive); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.BlockState(b.ID); got != StateActive {
		t.Fatalf("after transition state=%q", got)
	}
}

func TestDBRecordNodeRefObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	n := NodeMeta{CommitID: "sha256:c1", ParentID: "", Branch: "main", Seq: 1, Message: "first", TimestampMs: 1000}
	if err := db.RecordNode(n, "segment-s1"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateRef("main", "sha256:c1", time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	o := ObjectRef{ObjectID: "sha256:o1", Size: 4, SHA256: "data"}
	if err := db.RecordObject(o, "segment-s1"); err != nil {
		t.Fatal(err)
	}

	var seq uint64
	if err := db.sql.QueryRow(`SELECT seq FROM nodes WHERE node_id=?`, "sha256:c1").Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if seq != 1 {
		t.Fatalf("seq=%d", seq)
	}
	var oid string
	if err := db.sql.QueryRow(`SELECT oid FROM objects WHERE oid=?`, "sha256:o1").Scan(&oid); err != nil {
		t.Fatal(err)
	}
}
