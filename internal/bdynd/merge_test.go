package bdynd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeFastForward(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	base, _ := HeadCommit(r)
	must(t, CreateBranch(r, "feature", base))
	must(t, Switch(r, "feature"))
	writeFile(t, filepath.Join(r.Root, "new.txt"), "new\n")
	must(t, Add(r, []string{"new.txt"}))
	feature, err := Commit(r, CommitOptions{Message: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	must(t, Switch(r, "main"))
	result, err := Merge(r, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != MergeFastForward {
		t.Fatalf("merge=%+v", result)
	}
	head, _ := HeadCommit(r)
	if head != feature.OID {
		t.Fatalf("head=%s feature=%s", head, feature.OID)
	}
}

func TestMergeWritesConflictMarkers(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	base, _ := HeadCommit(r)
	must(t, CreateBranch(r, "feature", base))
	writeFile(t, filepath.Join(r.Root, "note.txt"), "main\n")
	must(t, Add(r, []string{"note.txt"}))
	if _, err := Commit(r, CommitOptions{Message: "main"}); err != nil {
		t.Fatal(err)
	}
	must(t, Switch(r, "feature"))
	writeFile(t, filepath.Join(r.Root, "note.txt"), "feature\n")
	must(t, Add(r, []string{"note.txt"}))
	if _, err := Commit(r, CommitOptions{Message: "feature"}); err != nil {
		t.Fatal(err)
	}
	must(t, Switch(r, "main"))
	result, err := Merge(r, "feature")
	if err == nil {
		t.Fatal("expected conflict")
	}
	if result.Kind != MergeConflict {
		t.Fatalf("merge=%+v", result)
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); !strings.Contains(got, "<<<<<<< HEAD") {
		t.Fatalf("conflict file=%q", got)
	}
}
