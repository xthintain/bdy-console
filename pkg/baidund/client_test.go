package baidund

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitAddCommitStatusLog(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := New(Credentials{AccessToken: "test-token"})
	repo, err := client.Init(root, DefaultRemoteRoot("demo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Add("note.txt"); err != nil {
		t.Fatal(err)
	}
	st, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Added) != 1 || st.Added[0] != "note.txt" {
		t.Fatalf("status=%+v", st)
	}
	commit, err := repo.Commit("first")
	if err != nil {
		t.Fatal(err)
	}
	if commit.OID == "" || commit.Message != "first" {
		t.Fatalf("commit=%+v", commit)
	}
	log, err := repo.Log(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0].OID != commit.OID {
		t.Fatalf("log=%+v", log)
	}
}

func TestDefaultRemoteRoot(t *testing.T) {
	if got, want := DefaultRemoteRoot("demo"), "/apps/baiduyunStorage/nd/repos/demo"; got != want {
		t.Fatalf("root=%q want %q", got, want)
	}
}
