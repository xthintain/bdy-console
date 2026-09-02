package bdynd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPushAndFetchPacksRoundTrip(t *testing.T) {
	source := newTestRepo(t)
	writeFile(t, filepath.Join(source.Root, "data.txt"), "payload\n")
	must(t, Add(source, []string{"data.txt"}))
	if _, err := Commit(source, CommitOptions{Message: "data"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := Pack(source, PackOptions{Name: "batch"})
	if err != nil {
		t.Fatal(err)
	}
	remote := newMemoryRemote()
	remoteRoot := "/apps/baiduyunStorage/nd/repos/demo"

	if err := PushPacks(context.Background(), source, remote, remoteRoot); err != nil {
		t.Fatal(err)
	}
	if ok, _ := remote.Exists(context.Background(), RemotePackPath(remoteRoot, manifest.ID)); !ok {
		t.Fatal("remote pack missing")
	}
	if ok, _ := remote.Exists(context.Background(), RemotePackManifestPath(remoteRoot, manifest.ID)); !ok {
		t.Fatal("remote pack manifest missing")
	}

	target := newTestRepo(t)
	if err := FetchPacks(context.Background(), target, remote, remoteRoot, []string{manifest.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target.Dir, "packs", manifest.ID+".pack")); err != nil {
		t.Fatal(err)
	}
	packs, err := ListPacks(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].ID != manifest.ID || packs[0].Entries[0].Path != "data.txt" {
		t.Fatalf("packs=%+v", packs)
	}
}

func TestFetchPacksRejectsUnsafeID(t *testing.T) {
	target := newTestRepo(t)
	remote := newMemoryRemote()
	err := FetchPacks(context.Background(), target, remote, "/apps/baiduyunStorage/nd/repos/demo", []string{"../../escape"})
	if err == nil {
		t.Fatal("unsafe pack id unexpectedly succeeded")
	}
	if _, statErr := os.Stat(filepath.Join(target.Dir, "escape.json")); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe path was written or stat failed unexpectedly: %v", statErr)
	}
}
