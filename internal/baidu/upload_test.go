package baidu

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileBlockMD5sSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks, whole, err := FileBlockMD5s(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "900150983cd24fb0d6963f7d28e17f72"
	if len(blocks) != 1 || blocks[0] != want || whole != want {
		t.Fatalf("blocks=%v whole=%s", blocks, whole)
	}
}
