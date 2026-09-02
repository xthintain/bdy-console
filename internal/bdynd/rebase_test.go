package bdynd

import (
	"path/filepath"
	"testing"
)

func TestRebaseReplaysCurrentBranchOntoUpstream(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	base, _ := HeadCommit(r)
	must(t, CreateBranch(r, "feature", base))

	writeFile(t, filepath.Join(r.Root, "main.txt"), "main\n")
	must(t, Add(r, []string{"main.txt"}))
	mainCommit, err := Commit(r, CommitOptions{Message: "main"})
	if err != nil {
		t.Fatal(err)
	}

	must(t, Switch(r, "feature"))
	writeFile(t, filepath.Join(r.Root, "feature.txt"), "feature\n")
	must(t, Add(r, []string{"feature.txt"}))
	if _, err := Commit(r, CommitOptions{Message: "feature"}); err != nil {
		t.Fatal(err)
	}

	result, err := Rebase(r, mainCommit.OID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed != 1 {
		t.Fatalf("result=%+v", result)
	}
	if got := readFile(t, filepath.Join(r.Root, "main.txt")); got != "main\n" {
		t.Fatalf("main.txt=%q", got)
	}
	if got := readFile(t, filepath.Join(r.Root, "feature.txt")); got != "feature\n" {
		t.Fatalf("feature.txt=%q", got)
	}
	head, _ := HeadCommit(r)
	if !isAncestor(r, mainCommit.OID, head) {
		t.Fatalf("rebased head %s is not descendant of upstream %s", head, mainCommit.OID)
	}
}

func TestRebaseReportsConflict(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	base, _ := HeadCommit(r)
	must(t, CreateBranch(r, "feature", base))

	writeFile(t, filepath.Join(r.Root, "note.txt"), "main\n")
	must(t, Add(r, []string{"note.txt"}))
	mainCommit, err := Commit(r, CommitOptions{Message: "main"})
	if err != nil {
		t.Fatal(err)
	}

	must(t, Switch(r, "feature"))
	writeFile(t, filepath.Join(r.Root, "note.txt"), "feature\n")
	must(t, Add(r, []string{"note.txt"}))
	if _, err := Commit(r, CommitOptions{Message: "feature"}); err != nil {
		t.Fatal(err)
	}

	result, err := Rebase(r, mainCommit.OID)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0] != "note.txt" {
		t.Fatalf("result=%+v", result)
	}
}
