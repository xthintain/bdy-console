package bdynd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLFSTrackStoresPointerEntry(t *testing.T) {
	r := newTestRepo(t)
	if err := TrackPattern(r, "*.bin"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(r.Root, "large.bin"), strings.Repeat("x", 1024))
	if err := Add(r, []string{"large.bin"}); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadIndex(r)
	if err != nil {
		t.Fatal(err)
	}
	e := idx.Entries["large.bin"]
	if e.Kind != KindLFS || !strings.HasPrefix(e.LFSOID, "sha256:") || e.Size != 1024 {
		t.Fatalf("entry=%+v", e)
	}
	if _, err := os.Stat(LFSObjectPath(r, e.LFSOID)); err != nil {
		t.Fatal(err)
	}
	ptr := FormatLFSPointer(e.LFSOID, e.Size)
	if !strings.Contains(ptr, "version https://bdy-lfs/spec/v1") {
		t.Fatalf("pointer=%q", ptr)
	}
}

func TestLFSUntrackRestoresNormalBlobAdd(t *testing.T) {
	r := newTestRepo(t)
	must(t, TrackPattern(r, "*.bin"))
	must(t, UntrackPattern(r, "*.bin"))
	writeFile(t, filepath.Join(r.Root, "large.bin"), "content\n")
	must(t, Add(r, []string{"large.bin"}))
	idx, _ := LoadIndex(r)
	if idx.Entries["large.bin"].Kind != KindBlob {
		t.Fatalf("entry=%+v", idx.Entries["large.bin"])
	}
}
