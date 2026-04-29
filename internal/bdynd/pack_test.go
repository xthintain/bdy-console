package bdynd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackHeadWritesPackAndManifest(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "a.txt"), "alpha\n")
	writeFile(t, filepath.Join(r.Root, "b.txt"), "beta\n")
	must(t, Add(r, []string{"a.txt", "b.txt"}))
	commit, err := Commit(r, CommitOptions{Message: "dataset"})
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := Pack(r, PackOptions{Ref: "HEAD", Name: "dataset"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID == "" || manifest.Ref != commit.OID || len(manifest.Entries) != 2 {
		t.Fatalf("manifest=%+v", manifest)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "packs", manifest.ID+".pack")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "packs", manifest.ID+".json")); err != nil {
		t.Fatal(err)
	}

	packs, err := ListPacks(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].ID != manifest.ID {
		t.Fatalf("packs=%+v", packs)
	}
	if packs[0].Entries[0].Offset != 0 || packs[0].Entries[0].Length == 0 {
		t.Fatalf("entry=%+v", packs[0].Entries[0])
	}
}
