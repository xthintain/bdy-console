package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanIgnoresBDYDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DirName, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 || m.Entries[0].Path != "tracked.txt" {
		t.Fatalf("entries = %+v", m.Entries)
	}
}

func TestApplyIndexOnlyStagesSelectedPaths(t *testing.T) {
	base := Manifest{Entries: []Entry{{Path: "old.txt", Size: 1, MD5: "old"}, {Path: "keep.txt", Size: 1, MD5: "same"}}}
	current := Manifest{Entries: []Entry{{Path: "old.txt", Size: 2, MD5: "new"}, {Path: "keep.txt", Size: 9, MD5: "changed-but-unstaged"}}}
	next := ApplyIndex(base, current, Index{Added: []string{"old.txt"}})
	got := next.Map()
	if got["old.txt"].MD5 != "new" {
		t.Fatalf("old.txt not updated: %+v", got["old.txt"])
	}
	if got["keep.txt"].MD5 != "same" {
		t.Fatalf("unstaged file changed: %+v", got["keep.txt"])
	}
}
