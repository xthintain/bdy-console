package bdynd

import "testing"

func TestWriteBlobIsContentAddressed(t *testing.T) {
	r := newTestRepo(t)
	oid, err := WriteBlob(r, []byte("hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ReadBlob(r, oid)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("blob=%q", data)
	}
	oid2, err := WriteBlob(r, []byte("hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	if oid != oid2 {
		t.Fatalf("oid not stable: %s != %s", oid, oid2)
	}
}

func newTestRepo(t *testing.T) Repo {
	t.Helper()
	r, err := Init(t.TempDir(), InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
