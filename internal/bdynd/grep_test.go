package bdynd

import (
	"path/filepath"
	"testing"
)

func TestGrepTrackedFiles(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "a.txt"), "hello\nworld\n")
	writeFile(t, filepath.Join(r.Root, "b.txt"), "skip\n")
	must(t, Add(r, []string{"."}))
	results, err := Grep(r, GrepOptions{Pattern: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "a.txt" || results[0].Line != 1 {
		t.Fatalf("results=%+v", results)
	}
}
