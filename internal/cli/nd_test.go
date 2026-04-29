package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestBdyNDInitAddCommitLog(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"nd", "init"}, &out, &errOut); code != 0 {
		t.Fatalf("init code=%d err=%s", code, errOut.String())
	}
	if err := os.WriteFile("a.txt", []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"nd", "add", "a.txt"}, &out, &errOut); code != 0 {
		t.Fatalf("add code=%d err=%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"nd", "commit", "-m", "first"}, &out, &errOut); code != 0 {
		t.Fatalf("commit code=%d err=%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"nd", "log"}, &out, &errOut); code != 0 {
		t.Fatalf("log code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "first") {
		t.Fatalf("log=%q", out.String())
	}
}

func TestBdyNDBranchSwitchTag(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	mustRunCLI(t, []string{"nd", "init"}, &out, &errOut)
	if err := os.WriteFile("note.txt", []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunCLI(t, []string{"nd", "add", "note.txt"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "commit", "-m", "main"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "branch", "feature"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "switch", "feature"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "tag", "v1"}, &out, &errOut)
	out.Reset()
	mustRunCLI(t, []string{"nd", "branch"}, &out, &errOut)
	if !strings.Contains(out.String(), "* feature") {
		t.Fatalf("branch output=%q", out.String())
	}
}

func TestBdyNDLFSTrackAndStatus(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	mustRunCLI(t, []string{"nd", "init"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "lfs", "track", "*.bin"}, &out, &errOut)
	if err := os.WriteFile("large.bin", []byte(strings.Repeat("x", 2048)), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunCLI(t, []string{"nd", "add", "large.bin"}, &out, &errOut)
	out.Reset()
	mustRunCLI(t, []string{"nd", "lfs", "status"}, &out, &errOut)
	if !strings.Contains(out.String(), "large.bin") {
		t.Fatalf("lfs status=%q", out.String())
	}
	out.Reset()
	mustRunCLI(t, []string{"nd", "lfs", "ls-files"}, &out, &errOut)
	if !strings.Contains(out.String(), "sha256:") {
		t.Fatalf("lfs ls-files=%q", out.String())
	}
	mustRunCLI(t, []string{"nd", "lfs", "untrack", "*.bin"}, &out, &errOut)
}

func TestBdyNDLFSCheckoutRestoresCachedObject(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	mustRunCLI(t, []string{"nd", "init"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "lfs", "track", "*.bin"}, &out, &errOut)
	if err := os.WriteFile("large.bin", []byte("large-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunCLI(t, []string{"nd", "add", "large.bin"}, &out, &errOut)
	if err := os.WriteFile("large.bin", []byte("pointer-placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunCLI(t, []string{"nd", "lfs", "checkout"}, &out, &errOut)
	data, err := os.ReadFile("large.bin")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "large-content" {
		t.Fatalf("large.bin=%q", data)
	}
}

func TestBdyNDRemoteSetURL(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	mustRunCLI(t, []string{"nd", "init"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "remote", "set-url", "origin", "/apps/baiduyunStorage/nd/repos/demo"}, &out, &errOut)
	out.Reset()
	mustRunCLI(t, []string{"nd", "remote"}, &out, &errOut)
	if !strings.Contains(out.String(), "origin /apps/baiduyunStorage/nd/repos/demo") {
		t.Fatalf("remote output=%q", out.String())
	}
}

func TestBdyNDCloneRequiresRemote(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"nd", "clone"}, &out, &errOut)
	if code == 0 {
		t.Fatal("clone without remote unexpectedly succeeded")
	}
	if !strings.Contains(errOut.String(), "usage: bdy nd clone <remote> [dir]") {
		t.Fatalf("err=%q", errOut.String())
	}
}

func TestBdyNDMergeFastForward(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	mustRunCLI(t, []string{"nd", "init"}, &out, &errOut)
	if err := os.WriteFile("note.txt", []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunCLI(t, []string{"nd", "add", "note.txt"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "commit", "-m", "base"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "branch", "feature"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "switch", "feature"}, &out, &errOut)
	if err := os.WriteFile("new.txt", []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunCLI(t, []string{"nd", "add", "new.txt"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "commit", "-m", "feature"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "switch", "main"}, &out, &errOut)
	out.Reset()
	mustRunCLI(t, []string{"nd", "merge", "feature"}, &out, &errOut)
	if !strings.Contains(out.String(), "fast-forward") {
		t.Fatalf("merge output=%q", out.String())
	}
}

func TestBdyNDStashPushListPop(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	mustRunCLI(t, []string{"nd", "init"}, &out, &errOut)
	if err := os.WriteFile("note.txt", []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunCLI(t, []string{"nd", "add", "note.txt"}, &out, &errOut)
	mustRunCLI(t, []string{"nd", "commit", "-m", "base"}, &out, &errOut)
	if err := os.WriteFile("note.txt", []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustRunCLI(t, []string{"nd", "stash", "push", "-m", "wip"}, &out, &errOut)
	if got, err := os.ReadFile("note.txt"); err != nil || string(got) != "base\n" {
		t.Fatalf("after stash note=%q err=%v", got, err)
	}
	out.Reset()
	mustRunCLI(t, []string{"nd", "stash", "list"}, &out, &errOut)
	if !strings.Contains(out.String(), "wip") {
		t.Fatalf("stash list=%q", out.String())
	}
	mustRunCLI(t, []string{"nd", "stash", "pop"}, &out, &errOut)
	if got, err := os.ReadFile("note.txt"); err != nil || string(got) != "dirty\n" {
		t.Fatalf("after pop note=%q err=%v", got, err)
	}
}

func mustRunCLI(t *testing.T, args []string, out, errOut *bytes.Buffer) {
	t.Helper()
	out.Reset()
	errOut.Reset()
	if code := Run(args, out, errOut); code != 0 {
		t.Fatalf("%v code=%d err=%s", args, code, errOut.String())
	}
}
