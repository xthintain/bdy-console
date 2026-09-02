package bdynd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanRemovesUntrackedFiles(t *testing.T) {
	r := repoWithOneCommit(t, "base")
	writeFile(t, filepath.Join(r.Root, "scratch.txt"), "scratch\n")
	removed, err := Clean(r, CleanOptions{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "scratch.txt" {
		t.Fatalf("removed=%v", removed)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "scratch.txt")); !os.IsNotExist(err) {
		t.Fatalf("scratch still exists err=%v", err)
	}
	if got := readFile(t, filepath.Join(r.Root, "note.txt")); got != "base\n" {
		t.Fatalf("tracked file changed: %q", got)
	}
}
