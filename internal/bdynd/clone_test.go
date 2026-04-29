package bdynd

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCloneFetchesAndChecksOutDefaultBranch(t *testing.T) {
	source := repoWithOneCommit(t, "first")
	writeFile(t, filepath.Join(source.Root, "readme.txt"), "hello\n")
	must(t, Add(source, []string{"readme.txt"}))
	if _, err := Commit(source, CommitOptions{Message: "readme"}); err != nil {
		t.Fatal(err)
	}
	remote := newMemoryRemote()
	must(t, SetRemote(source, "origin", "/apps/baiduyunStorage/nd/repos/demo"))
	must(t, Push(context.Background(), source, remote, "origin"))

	dest := filepath.Join(t.TempDir(), "clone")
	r, err := Clone(context.Background(), remote, "/apps/baiduyunStorage/nd/repos/demo", dest)
	if err != nil {
		t.Fatal(err)
	}
	if r.Root != dest {
		t.Fatalf("root=%q", r.Root)
	}
	if got := readFile(t, filepath.Join(dest, "readme.txt")); got != "hello\n" {
		t.Fatalf("readme=%q", got)
	}
}
