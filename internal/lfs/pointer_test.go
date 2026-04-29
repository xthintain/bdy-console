package lfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPointerRoundTrip(t *testing.T) {
	p := Pointer{OID: "sha256:abc123", Size: 42}
	got, err := ParsePointer(strings.NewReader(FormatPointer(p)))
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("got %+v want %+v", got, p)
	}
}

func TestHashAndStoreObject(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "big.bin")
	if err := os.WriteFile(src, []byte("large content"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := HashFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := StoreObject(root, src, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ObjectPath(root, p)); err != nil {
		t.Fatal(err)
	}
	if p.OID != "sha256:b40f7e910f04aa4f15e98e9a9250aba22a43fe4744f7398f24bf5acf9352d8ba" {
		t.Fatalf("unexpected oid: %s", p.OID)
	}
}

func TestTrackWritesAttributes(t *testing.T) {
	root := t.TempDir()
	if err := Track(root, []string{"*.zip"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	want := "*.zip filter=bdy-lfs diff=bdy-lfs merge=bdy-lfs -text"
	if !strings.Contains(string(data), want) {
		t.Fatalf("missing attributes line: %s", data)
	}
}
