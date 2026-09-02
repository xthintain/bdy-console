package bdynd

import (
	"path/filepath"
	"testing"
)

func TestCherryPickAppliesCommitChanges(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	base, _ := HeadCommit(r)
	must(t, CreateBranch(r, "feature", base))
	must(t, Switch(r, "feature"))
	writeFile(t, filepath.Join(r.Root, "feature.txt"), "feature\n")
	must(t, Add(r, []string{"feature.txt"}))
	featureCommit, err := Commit(r, CommitOptions{Message: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	must(t, Switch(r, "main"))
	result, err := CherryPick(r, featureCommit.OID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Commit == "" {
		t.Fatalf("result=%+v", result)
	}
	if got := readFile(t, filepath.Join(r.Root, "feature.txt")); got != "feature\n" {
		t.Fatalf("feature.txt=%q", got)
	}
}
