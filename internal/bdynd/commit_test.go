package bdynd

import (
	"path/filepath"
	"testing"
)

func TestCommitUpdatesHeadAndLog(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "notes.txt"), "hello\n")
	if err := Add(r, []string{"notes.txt"}); err != nil {
		t.Fatal(err)
	}
	c, err := Commit(r, CommitOptions{Message: "first"})
	if err != nil {
		t.Fatal(err)
	}
	head, err := HeadCommit(r)
	if err != nil {
		t.Fatal(err)
	}
	if head != c.OID {
		t.Fatalf("head=%s commit=%s", head, c.OID)
	}
	log, err := Log(r, LogOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0].Message != "first" {
		t.Fatalf("log=%+v", log)
	}
	read, err := ReadCommit(r, c.OID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Message != "first" || read.Tree == "" {
		t.Fatalf("read commit=%+v", read)
	}
}
