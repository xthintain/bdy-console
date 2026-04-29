package bdynd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBranchAndSwitchCheckoutDifferentTrees(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "note.txt"), "main\n")
	must(t, Add(r, []string{"note.txt"}))
	mainCommit, err := Commit(r, CommitOptions{Message: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateBranch(r, "feature", mainCommit.OID); err != nil {
		t.Fatal(err)
	}
	if err := Switch(r, "feature"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(r.Root, "note.txt"), "feature\n")
	must(t, Add(r, []string{"note.txt"}))
	featureCommit, err := Commit(r, CommitOptions{Message: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if featureCommit.Parent != mainCommit.OID {
		t.Fatalf("parent=%s want %s", featureCommit.Parent, mainCommit.OID)
	}
	if err := Switch(r, "main"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); got != "main\n" {
		t.Fatalf("main worktree=%q", got)
	}
	if err := Switch(r, "feature"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); got != "feature\n" {
		t.Fatalf("feature worktree=%q", got)
	}
}

func TestTagPointsAtHead(t *testing.T) {
	r := repoWithOneCommit(t, "first")
	head, err := HeadCommit(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateTag(r, "v1", head); err != nil {
		t.Fatal(err)
	}
	oid, err := ResolveRef(r, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if oid != head {
		t.Fatalf("tag=%s head=%s", oid, head)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func repoWithOneCommit(t *testing.T, message string) Repo {
	t.Helper()
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "note.txt"), "base\n")
	must(t, Add(r, []string{"note.txt"}))
	if _, err := Commit(r, CommitOptions{Message: message}); err != nil {
		t.Fatal(err)
	}
	return r
}
