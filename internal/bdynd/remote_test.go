package bdynd

import (
	"context"
	"testing"
)

func TestPushUploadsReachableObjectsAndRefs(t *testing.T) {
	r := repoWithOneCommit(t, "first")
	remote := newMemoryRemote()
	if err := SetRemote(r, "origin", "/apps/baiduyunStorage/nd/repos/demo"); err != nil {
		t.Fatal(err)
	}
	if err := Push(context.Background(), r, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := remote.Exists(context.Background(), "/apps/baiduyunStorage/nd/repos/demo/refs/heads/main"); !ok {
		t.Fatal("main ref not uploaded")
	}
	head, _ := HeadCommit(r)
	if ok, _ := remote.Exists(context.Background(), RemoteCommitPath("/apps/baiduyunStorage/nd/repos/demo", head)); !ok {
		t.Fatal("head commit not uploaded")
	}
}

func TestFetchUpdatesRemoteTrackingRef(t *testing.T) {
	source := repoWithOneCommit(t, "first")
	remote := newMemoryRemote()
	must(t, SetRemote(source, "origin", "/apps/baiduyunStorage/nd/repos/demo"))
	must(t, Push(context.Background(), source, remote, "origin"))
	target := newTestRepo(t)
	must(t, SetRemote(target, "origin", "/apps/baiduyunStorage/nd/repos/demo"))
	if err := Fetch(context.Background(), target, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	ref, err := ResolveRef(target, "refs/remotes/origin/main")
	if err != nil || ref == "" {
		t.Fatalf("remote ref=%q err=%v", ref, err)
	}
}

func TestPullFastForwardsCurrentBranch(t *testing.T) {
	source := repoWithOneCommit(t, "first")
	remote := newMemoryRemote()
	must(t, SetRemote(source, "origin", "/apps/baiduyunStorage/nd/repos/demo"))
	must(t, Push(context.Background(), source, remote, "origin"))
	target := newTestRepo(t)
	must(t, SetRemote(target, "origin", "/apps/baiduyunStorage/nd/repos/demo"))
	if err := Pull(context.Background(), target, remote, "origin"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target.Root+"/note.txt"); got != "base\n" {
		t.Fatalf("note.txt=%q", got)
	}
}
