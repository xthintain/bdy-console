package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"baiduyunStorage/internal/config"
	"baiduyunStorage/internal/repo"
)

func TestHelpAndInitStatusSmoke(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"help"}, &out, &errOut); code != 0 {
		t.Fatalf("help code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "bdy - Baidu Netdisk command line storage") {
		t.Fatalf("help output = %q", out.String())
	}
	if !strings.Contains(out.String(), "Spaces:") || !strings.Contains(out.String(), "Run 'bdy <command> --help'") {
		t.Fatalf("help missing sections: %q", out.String())
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

func TestDetailedHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "cmd help",
			args: []string{"cmd", "--help"},
			want: []string{"Usage:", "bdy cmd ls [-al] [path]", "eval \"$(bdy cmd cd git)\"", "download <remote-path>", "mkdir [-p]"},
		},
		{
			name: "lfs help",
			args: []string{"lfs", "--help"},
			want: []string{"Git-LFS-style large file storage", "bdy lfs track '<pattern>'", "bdy lfs push", "Remote object root"},
		},
		{
			name: "auth help",
			args: []string{"auth", "--help"},
			want: []string{"bdy auth import-token", "bdy auth status", "SDK token"},
		},
		{
			name: "config help",
			args: []string{"config", "--help"},
			want: []string{"bdy config clear-app", "BDY_ACCESS_TOKEN", "Token-only mode"},
		},
		{
			name: "help command help",
			args: []string{"help", "cmd"},
			want: []string{"bdy cmd", "Cloud working directory", "history [-n N]"},
		},
		{
			name: "cmd command help",
			args: []string{"cmd", "mkdir", "--help"},
			want: []string{"bdy cmd mkdir [-p] <path...>", "Root:", "/apps/baiduyunStorage"},
		},
		{
			name: "home command help",
			args: []string{"home", "cmd", "mkdir", "--help"},
			want: []string{"bdy home mkdir [-p] <path...>", "Equivalent:", "bdy home cmd mkdir"},
		},
		{
			name: "home search help",
			args: []string{"home", "search", "--help"},
			want: []string{"bdy home search", "Baidu remote search", "'*.mp4'"},
		},
		{
			name: "sync help",
			args: []string{"sync", "--help"},
			want: []string{"bdy sync init", "bdy sync commit -m <message>", "Legacy aliases"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := Run(tt.args, &out, &errOut); code != 0 {
				t.Fatalf("code=%d err=%s", code, errOut.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("help output missing %q:\n%s", want, out.String())
				}
			}
		})
	}
}

func TestGlobalFlags(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"-v"}, &out, &errOut); code != 0 {
		t.Fatalf("version code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "bdy ") {
		t.Fatalf("version output=%q", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"-C", "git", "cmd", "pwd"}, &out, &errOut); code != 0 {
		t.Fatalf("cwd code=%d err=%s", code, errOut.String())
	}
	if got, want := strings.TrimSpace(out.String()), "/apps/baiduyunStorage/git"; got != want {
		t.Fatalf("pwd=%q want %q", got, want)
	}
}

func TestTemporaryReadOnlyAuthBlocksWriteCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BDY_CONFIG_HOME", dir)
	cfg := config.Config{
		AccessToken:        "temporary-token",
		RefreshToken:       "temporary-refresh",
		ExpiresAt:          time.Now().Add(time.Hour),
		Temporary:          true,
		ReadOnly:           true,
		TemporaryExpiresAt: time.Now().Add(time.Hour),
	}
	if err := config.SaveTemporary(cfg); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"nd", "commit", "-m", "blocked"}, &out, &errOut); code == 0 {
		t.Fatal("write command unexpectedly succeeded")
	}
	if !strings.Contains(errOut.String(), "temporary read-only auth forbids write operation") {
		t.Fatalf("err=%q", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"auth", "status"}, &out, &errOut); code != 0 {
		t.Fatalf("auth status code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "temporary") || !strings.Contains(out.String(), "read-only") {
		t.Fatalf("status=%q", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"auth", "import-token"}, &out, &errOut); code == 0 {
		t.Fatal("persistent auth import unexpectedly allowed during temporary read-only auth")
	}
	if !strings.Contains(errOut.String(), "temporary read-only auth forbids write operation") {
		t.Fatalf("err=%q", errOut.String())
	}
}

func TestTemporaryDurationParsesDays(t *testing.T) {
	d, err := parseTemporaryDuration("1d")
	if err != nil {
		t.Fatal(err)
	}
	if d != 24*time.Hour {
		t.Fatalf("duration=%s", d)
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

func TestCmdCDPersistsForCurrentShellSession(t *testing.T) {
	t.Setenv(cmdCWDEnv, "")
	t.Setenv(cmdSessionDirEnv, t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"cmd", "cd", "git"}, &out, &errOut); code != 0 {
		t.Fatalf("cd code=%d err=%s", code, errOut.String())
	}
	if got, want := cmdPath("repo.txt"), "/apps/baiduyunStorage/git/repo.txt"; got != want {
		t.Fatalf("session cmdPath=%q want %q", got, want)
	}
}

func TestCloudFileREPLHandlesLocalCWD(t *testing.T) {
	var out bytes.Buffer
	input := strings.NewReader("pwd\ncd git\npwd\ncd ..\npwd\nexit\n")
	err := runCloudFileREPL(context.Background(), input, &out, cloudFileSpace{
		Name:    "cmd",
		Root:    repo.CmdRoot,
		Resolve: cmdPath,
		AllowCD: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"/apps/baiduyunStorage", "/apps/baiduyunStorage/git"} {
		if !strings.Contains(got, want) {
			t.Fatalf("repl output missing %q:\n%s", want, got)
		}
	}
}

func TestParseInteractiveLineSupportsQuotes(t *testing.T) {
	args, err := parseInteractiveLine(`search -E "^贤者.*$" "/娱乐/动漫"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"search", "-E", "^贤者.*$", "/娱乐/动漫"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args=%q want %q", args, want)
	}
}

func TestDownloadDestinationUsesRemoteFilenameForDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := downloadDestination("/apps/baiduyunStorage/config.json", dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "config.json"); got != want {
		t.Fatalf("dest=%q want %q", got, want)
	}

	got, err = downloadDestination("/config.json", ".")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(".", "config.json"); got != want {
		t.Fatalf("dest=%q want %q", got, want)
	}

	got, err = downloadDestination("/config.json", "local.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != "local.json" {
		t.Fatalf("dest=%q want local.json", got)
	}
}

func TestHomePathUsesWholeNetdiskRoot(t *testing.T) {
	tests := map[string]string{
		"":                    "/",
		".":                   "/",
		"/":                   "/",
		"docs/a.txt":          "/docs/a.txt",
		"/docs/a.txt":         "/docs/a.txt",
		"../escape/docs.txt":  "/escape/docs.txt",
		"/apps/demo/file.txt": "/apps/demo/file.txt",
	}
	for input, want := range tests {
		if got := homePath(input); got != want {
			t.Fatalf("homePath(%q)=%q want %q", input, got, want)
		}
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

	searchOpts, err = parseSearchArgs("search", []string{"mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if searchOpts.Pattern != "mp4" || searchOpts.Path != "." {
		t.Fatalf("search opts=%+v", searchOpts)
	}

	searchOpts, err = parseSearchArgs("search", []string{"-E", `.*\.mp4$`})
	if err != nil {
		t.Fatal(err)
	}
	if !searchOpts.Regex || searchOpts.Pattern != `.*\.mp4$` {
		t.Fatalf("regex search opts=%+v", searchOpts)
	}
}

func TestCmdSearchMatchSupportsGlobAndPlainText(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		file    string
		want    bool
	}{
		{name: "glob extension", pattern: "*.mp4", path: "/movies/demo.mp4", file: "demo.mp4", want: true},
		{name: "plain extension", pattern: "mp4", path: "/movies/demo.mp4", file: "demo.mp4", want: true},
		{name: "case insensitive", pattern: "*.mp4", path: "/movies/DEMO.MP4", file: "DEMO.MP4", want: true},
		{name: "no match", pattern: "*.mp4", path: "/movies/demo.txt", file: "demo.txt", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := searchMatch(cmdSearchOptions{Command: "search", Pattern: tt.pattern}, tt.path, tt.file)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("match=%v want %v", got, tt.want)
			}
		})
	}
}

func TestCmdSearchMatchSupportsRegex(t *testing.T) {
	opts := cmdSearchOptions{Pattern: `^demo-[0-9]+\.mp4$`, Regex: true, IgnoreCase: true}
	got, err := searchMatch(opts, "/movies/DEMO-42.MP4", "DEMO-42.MP4")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("regex did not match")
	}
	got, err = searchMatch(opts, "/movies/demo-final.mp4", "demo-final.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("regex unexpectedly matched")
	}
}

func TestSearchRemoteKeyExtractsUsefulToken(t *testing.T) {
	tests := map[string]string{
		"*.mp4":              "mp4",
		"movie*.mp4":         "movie",
		"report-final":       "report-final",
		"*":                  "",
		`^demo-[0-9]+\.mp4$`: "demo",
	}
	for input, want := range tests {
		if got := searchRemoteKey(input); got != want {
			t.Fatalf("searchRemoteKey(%q)=%q want %q", input, got, want)
		}
	}
}
