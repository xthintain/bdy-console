package bdynd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesBdyNDLayout(t *testing.T) {
	root := t.TempDir()
	r, err := Init(root, InitOptions{RemoteName: "origin", RemoteRoot: "/apps/baiduyunStorage/nd/repos/demo"})
	if err != nil {
		t.Fatal(err)
	}
	mustExist(t, filepath.Join(root, ".bdynd", "config.json"))
	mustExist(t, filepath.Join(root, ".bdynd", "HEAD"))
	mustExist(t, filepath.Join(root, ".bdynd", "refs", "heads", "main"))
	mustExist(t, filepath.Join(root, ".bdynd", "objects", "blobs"))
	mustExist(t, filepath.Join(root, ".bdynd", "objects", "trees"))
	mustExist(t, filepath.Join(root, ".bdynd", "objects", "commits"))
	mustExist(t, filepath.Join(root, ".bdynd", "lfs", "objects"))
	if r.Config.DefaultBranch != "main" {
		t.Fatalf("default branch=%q", r.Config.DefaultBranch)
	}
	if got := r.Config.Remotes["origin"]; got != "/apps/baiduyunStorage/nd/repos/demo" {
		t.Fatalf("origin=%q", got)
	}
}

func TestOpenFindsParentRepository(t *testing.T) {
	root := t.TempDir()
	_, err := Init(root, InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := Open(nested)
	if err != nil {
		t.Fatal(err)
	}
	if r.Root != root {
		t.Fatalf("root=%q want %q", r.Root, root)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
