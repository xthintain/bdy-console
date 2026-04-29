package bdynd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiffReportsWorktreeChangesAgainstHead(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	writeFile(t, filepath.Join(r.Root, "note.txt"), "changed\n")
	writeFile(t, filepath.Join(r.Root, "new.txt"), "new\n")

	diff, err := Diff(r)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPath(diff.Modified, "note.txt") || !hasPath(diff.Added, "new.txt") {
		t.Fatalf("diff=%+v", diff)
	}
}

func TestDiffSkipsBdyNDIgnorePatterns(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	writeFile(t, filepath.Join(r.Root, ".bdyndignore"), "*.tmp\n")
	writeFile(t, filepath.Join(r.Root, "scratch.tmp"), "scratch\n")

	diff, err := Diff(r)
	if err != nil {
		t.Fatal(err)
	}
	if hasPath(diff.Added, "scratch.tmp") {
		t.Fatalf("ignored file reported in diff: %+v", diff)
	}
}

func TestRemoveAndMoveUpdateWorktreeAndIndex(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	if err := Remove(r, []string{"note.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "note.txt")); !os.IsNotExist(err) {
		t.Fatalf("note.txt still exists err=%v", err)
	}
	idx, err := LoadIndex(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Entries["note.txt"]; ok {
		t.Fatalf("removed file still indexed: %+v", idx.Entries["note.txt"])
	}

	writeFile(t, filepath.Join(r.Root, "old.txt"), "old\n")
	if err := Add(r, []string{"old.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := Move(r, "old.txt", "new.txt"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(r.Root, "new.txt")); got != "old\n" {
		t.Fatalf("new.txt=%q", got)
	}
	idx, err = LoadIndex(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Entries["old.txt"]; ok {
		t.Fatal("old path still indexed")
	}
	if _, ok := idx.Entries["new.txt"]; !ok {
		t.Fatal("new path not indexed")
	}
}

func TestRestoreAndResetModes(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	writeFile(t, filepath.Join(r.Root, "note.txt"), "dirty\n")
	if err := Add(r, []string{"note.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := Restore(r, []string{"note.txt"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); got != "base\n" {
		t.Fatalf("restored=%q", got)
	}

	writeFile(t, filepath.Join(r.Root, "note.txt"), "second\n")
	if err := Add(r, []string{"note.txt"}); err != nil {
		t.Fatal(err)
	}
	second, err := Commit(r, CommitOptions{Message: "second"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(r.Root, "note.txt"), "third\n")
	if err := Add(r, []string{"note.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(r, CommitOptions{Message: "third"}); err != nil {
		t.Fatal(err)
	}
	if err := Reset(r, second.Parent, ResetHard); err != nil {
		t.Fatal(err)
	}
	head, err := HeadCommit(r)
	if err != nil {
		t.Fatal(err)
	}
	if head != second.Parent {
		t.Fatalf("head=%s want %s", head, second.Parent)
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); got != "base\n" {
		t.Fatalf("hard reset worktree=%q", got)
	}
}

func hasPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
