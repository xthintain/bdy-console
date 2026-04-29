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

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
