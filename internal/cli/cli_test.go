package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndInitStatusSmoke(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"help"}, &out, &errOut); code != 0 {
		t.Fatalf("help code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "bdy - Git-like") {
		t.Fatalf("help output = %q", out.String())
	}
	if !strings.Contains(out.String(), "mkdir|touch|vim") {
		t.Fatalf("help missing new cmd commands: %q", out.String())
	}

	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"init"}, &out, &errOut); code != 0 {
		t.Fatalf("init code=%d err=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".bdy", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"status"}, &out, &errOut); code != 0 {
		t.Fatalf("status code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "clean") {
		t.Fatalf("status output=%q", out.String())
	}
}

func TestInitRejectsWholeNetdiskRoot(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{"init", "--remote-root", "/"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("init / unexpectedly succeeded")
	}
	if !strings.Contains(errOut.String(), "use bdy home") {
		t.Fatalf("unexpected error: %q", errOut.String())
	}
}

func TestLFSRequiresGitRepo(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{"lfs", "install"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("lfs install unexpectedly succeeded")
	}
	if !strings.Contains(errOut.String(), "not a git repository") {
		t.Fatalf("unexpected error: %q", errOut.String())
	}
}

func TestCmdPathUsesAppRootAndTemporaryCWD(t *testing.T) {
	tests := map[string]string{
		"":                                "/apps/baiduyunStorage",
		".":                               "/apps/baiduyunStorage",
		"/":                               "/apps/baiduyunStorage",
		"notes/a.txt":                     "/apps/baiduyunStorage/notes/a.txt",
		"/notes/a.txt":                    "/apps/baiduyunStorage/notes/a.txt",
		"/apps/baiduyunStorage/notes.txt": "/apps/baiduyunStorage/notes.txt",
	}
	for input, want := range tests {
		if got := cmdPath(input); got != want {
			t.Fatalf("cmdPath(%q)=%q want %q", input, got, want)
		}
	}
	t.Setenv(cmdCWDEnv, "/apps/baiduyunStorage/git")
	if got, want := cmdPath("repo.txt"), "/apps/baiduyunStorage/git/repo.txt"; got != want {
		t.Fatalf("cmdPath with cwd=%q want %q", got, want)
	}
	if got, want := cmdPath("/root.txt"), "/apps/baiduyunStorage/root.txt"; got != want {
		t.Fatalf("absolute cmdPath with cwd=%q want %q", got, want)
	}
}

func TestParseLSArgs(t *testing.T) {
	opts, path, err := parseLSArgs("ls", []string{"-al", "git"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.All || !opts.Long || path != "git" {
		t.Fatalf("opts=%+v path=%q", opts, path)
	}
	opts, path, err = parseLSArgs("ll", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Long || opts.All || path != "." {
		t.Fatalf("ll opts=%+v path=%q", opts, path)
	}
	opts, path, err = parseLSArgs("la", []string{"docs"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.All || opts.Long || path != "docs" {
		t.Fatalf("la opts=%+v path=%q", opts, path)
	}
	if _, _, err := parseLSArgs("ls", []string{"-z"}); err == nil {
		t.Fatalf("unsupported flag did not fail")
	}
}

func TestParseCommonCmdArgs(t *testing.T) {
	rmOpts, err := parseRMArgs("rm", []string{"-rf", "a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if !rmOpts.Force || len(rmOpts.Paths) != 2 {
		t.Fatalf("rm opts=%+v", rmOpts)
	}

	mkdirOpts, err := parseMkdirArgs([]string{"-p", "a/b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mkdirOpts.Paths) != 1 || mkdirOpts.Paths[0] != "a/b" {
		t.Fatalf("mkdir opts=%+v", mkdirOpts)
	}

	touchOpts, err := parseTouchArgs([]string{"-c", "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !touchOpts.NoCreate || len(touchOpts.Paths) != 1 {
		t.Fatalf("touch opts=%+v", touchOpts)
	}

	catOpts, err := parseCatArgs([]string{"-n", "a.txt", "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !catOpts.Number || len(catOpts.Paths) != 2 {
		t.Fatalf("cat opts=%+v", catOpts)
	}

	searchOpts, err := parseSearchArgs("grep", []string{"-iv", "-type", "f", "token", "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if !searchOpts.IgnoreCase || !searchOpts.Invert || searchOpts.Type != "f" || searchOpts.Pattern != "token" || searchOpts.Path != "docs" {
		t.Fatalf("search opts=%+v", searchOpts)
	}
}
