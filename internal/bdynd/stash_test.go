package bdynd

import (
	"path/filepath"
	"testing"
)

func TestStashPushAndPopRestoresWorktree(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	writeFile(t, filepath.Join(r.Root, "note.txt"), "dirty\n")

	stash, err := StashPush(r, "wip")
	if err != nil {
		t.Fatal(err)
	}
	if stash.ID == "" {
		t.Fatal("stash id empty")
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); got != "base\n" {
		t.Fatalf("after stash=%q", got)
	}

	if err := StashPop(r, stash.ID); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); got != "dirty\n" {
		t.Fatalf("after pop=%q", got)
	}
}
