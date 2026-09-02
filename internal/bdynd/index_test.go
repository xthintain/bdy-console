package bdynd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddStagesFileBlob(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "notes.txt"), "hello\n")
	if err := Add(r, []string{"notes.txt"}); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadIndex(r)
	if err != nil {
		t.Fatal(err)
	}
	e := idx.Entries["notes.txt"]
	if e.Path != "notes.txt" || e.Kind != KindBlob || e.OID == "" || e.Size != 6 {
		t.Fatalf("entry=%+v", e)
	}
}

func TestAddDotSkipsBdyNDIgnorePatterns(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, ".bdyndignore"), "*.log\nsecret/\n/dist/\n")
	writeFile(t, filepath.Join(r.Root, "keep.txt"), "keep\n")
	writeFile(t, filepath.Join(r.Root, "debug.log"), "debug\n")
	writeFile(t, filepath.Join(r.Root, "secret", "token.txt"), "secret\n")
	writeFile(t, filepath.Join(r.Root, "dist", "app.bin"), "bin\n")

	if err := Add(r, []string{"."}); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadIndex(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Entries["keep.txt"]; !ok {
		t.Fatal("keep.txt not indexed")
	}
	for _, path := range []string{"debug.log", "secret/token.txt", "dist/app.bin"} {
		if _, ok := idx.Entries[path]; ok {
			t.Fatalf("ignored path indexed: %s", path)
		}
	}
}

func TestApplyIgnoreRemovesNowIgnoredIndexEntries(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "keep.txt"), "keep\n")
	writeFile(t, filepath.Join(r.Root, "debug.log"), "debug\n")
	if err := Add(r, []string{"."}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(r.Root, ".bdyndignore"), "*.log\n")
	removed, err := ApplyIgnore(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "debug.log" {
		t.Fatalf("removed=%v", removed)
	}
	idx, err := LoadIndex(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Entries["debug.log"]; ok {
		t.Fatal("ignored file still indexed")
	}
	if _, ok := idx.Entries["keep.txt"]; !ok {
		t.Fatal("keep.txt removed unexpectedly")
	}
	if _, err := os.Stat(filepath.Join(r.Root, "debug.log")); err != nil {
		t.Fatalf("worktree file removed: %v", err)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
