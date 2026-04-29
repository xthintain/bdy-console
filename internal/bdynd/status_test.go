package bdynd

import (
	"path/filepath"
	"testing"
)

func TestStatusReportsCleanAfterCommit(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, filepath.Join(r.Root, "notes.txt"), "hello\n")
	if err := Add(r, []string{"notes.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(r, CommitOptions{Message: "first"}); err != nil {
		t.Fatal(err)
	}
	st, err := Status(r)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean() {
		t.Fatalf("status=%+v", st)
	}
}
