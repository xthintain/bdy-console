package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/config"
	"baiduyunStorage/internal/lfs"
	"baiduyunStorage/internal/repo"
)

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printRootHelp(stdout)
		return 0
	}
	if args[0] == "help" {
		topic := ""
		if len(args) > 1 {
			topic = args[1]
		}
		if err := printHelpTopic(stdout, topic); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	}
	if len(args) > 1 && isHelpArg(args[1]) {
		if err := printHelpTopic(stdout, args[0]); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	}
	ctx := context.Background()
	if err := run(ctx, args, stdout); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func isHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func run(ctx context.Context, args []string, out io.Writer) error {
	switch args[0] {
	case "config":
		return cmdConfig(args[1:], out)
	case "auth":
		return cmdAuth(ctx, args[1:], out)
	case "home":
		return cmdHome(ctx, args[1:], out)
	case "cmd":
		return cmdShell(ctx, args[1:], out)
	case "lfs":
		return cmdLFS(ctx, args[1:], out)
	case "init":
		return cmdInit(args[1:], out)
	case "status":
		return cmdStatus(out)
	case "add":
		return cmdAdd(args[1:], out)
	case "rm":
		return cmdRemove(args[1:], out)
	case "mv":
		return cmdMove(args[1:], out)
	case "commit":
		return cmdCommit(args[1:], out)
	case "ls":
		return cmdLS(ctx, args[1:], out)
	case "push":
		return cmdPush(ctx, out)
	case "pull":
		return cmdPull(ctx, out)
	case "remote":
		return cmdRemote(ctx, out)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printRootHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy - Baidu Netdisk command line storage

Usage:
  bdy <command> [args]
  bdy help [command]
  bdy <command> --help

Spaces:
  cmd      Manage files under /apps/baiduyunStorage
  lfs      Store Git-LFS-style large objects under /apps/baiduyunStorage/lfs
  home     Inspect the whole Baidu Netdisk only when explicitly requested
  sync     Snapshot sync commands under an isolated remote workspace

Core commands:
  config   Configure Baidu Open Platform application credentials
  auth     Login with Baidu OAuth device-code flow and check token status
  cmd      Bash-style cloud file commands: ls, find, grep, rm, cat, mkdir, touch, vim
  lfs      Git-LFS-style large file commands: track, push, fetch, checkout, pull
  home     Whole-netdisk inspection commands
  init     Initialize snapshot sync metadata in the current directory
  status   Show local snapshot sync status
  add      Stage paths for snapshot sync
  commit   Commit a snapshot
  push     Upload committed snapshot changes
  pull     Download remote snapshot changes
  ls       List snapshot sync remote files
  rm       Remove local paths and stage removals
  mv       Move local paths and stage moves
  remote   Show snapshot sync remote root

Run 'bdy <command> --help' for detailed command help.
Examples:
  bdy config set-app --app-id ID --app-key KEY --secret-key SECRET --sign-key SIGN
  bdy auth login
  bdy cmd ls -al
  eval "$(bdy cmd cd git)"
  bdy lfs track '*.zip'
  bdy init
  bdy add README.md && bdy commit -m 'snapshot' && bdy push`)
}

func printHelpTopic(out io.Writer, topic string) error {
	switch topic {
	case "", "help":
		printRootHelp(out)
	case "config":
		printConfigHelp(out)
	case "auth":
		printAuthHelp(out)
	case "home":
		printHomeHelp(out)
	case "cmd":
		printCmdHelp(out)
	case "lfs":
		printLFSHelp(out)
	case "init", "status", "add", "commit", "push", "pull", "ls", "rm", "mv", "remote", "sync":
		printSyncHelp(out)
	default:
		return fmt.Errorf("unknown help topic %q", topic)
	}
	return nil
}

func printConfigHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy config - configure local Baidu Open Platform credentials

Usage:
  bdy config set-app --app-id ID --app-key KEY --secret-key SECRET [--sign-key SIGN]

Options:
  --app-id       Baidu Netdisk Open Platform AppID
  --app-key      OAuth client ID / AppKey
  --secret-key   OAuth client secret / SecretKey
  --sign-key     Application SignKey when your API flow needs it

Config file:
  ~/.config/bdy/config.json

The config file may contain app credentials, access tokens, and refresh tokens.
Keep it private and never commit it.`)
}

func printAuthHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy auth - Baidu OAuth login and token status

Usage:
  bdy auth login
  bdy auth status

Commands:
  login    Start the OAuth device-code flow, print the verification URL, user code, and QR URL
  status   Check whether a valid token is saved locally

Flow:
  1. Run 'bdy config set-app ...' once with your app credentials.
  2. Run 'bdy auth login'.
  3. Open the printed URL or QR code and approve the basic,netdisk scope.
  4. Use 'bdy auth status' to verify the saved token.`)
}

func printHomeHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy home - explicit whole-netdisk inspection

Usage:
  bdy home ls [path]

Examples:
  bdy home ls /
  bdy home ls /apps
  bdy home ls /Document

Normal 'cmd', 'lfs', and snapshot sync commands stay under /apps/baiduyunStorage.
Use 'home' only when you intentionally want to inspect the whole Baidu Netdisk.`)
}

func printCmdHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy cmd - bash-style cloud file commands

Root:
  /apps/baiduyunStorage

Usage:
  bdy cmd pwd
  bdy cmd cd [path]
  bdy cmd ls [-a] [-l] [path]
  bdy cmd ls [-al] [path]
  bdy cmd ls [-la] [path]
  bdy cmd ll [path]
  bdy cmd la [path]
  bdy cmd find [-name pattern] [-type f|d] [pattern] [path]
  bdy cmd grep [-i] [-v] [-type f|d] <pattern> [path]
  bdy cmd rm [-r] [-f] <path...>
  bdy cmd delete [-r] [-f] <path...>
  bdy cmd history [-n N]
  bdy cmd history -c
  bdy cmd cat [-n] <path...>
  bdy cmd mkdir [-p] <path...>
  bdy cmd touch [-c] <path...>
  bdy cmd vim <path>

Cloud working directory:
  eval "$(bdy cmd cd git)"
  bdy cmd pwd

'cd' exports BDY_CMD_CWD for the current shell only. A new terminal without that
environment variable returns to /apps/baiduyunStorage.

Examples:
  bdy cmd ls -al
  bdy cmd mkdir -p notes/archive
  bdy cmd touch notes/today.txt
  bdy cmd cat -n notes/today.txt
  bdy cmd find -name '.*\.txt$' -type f
  bdy cmd grep -i report notes
  bdy cmd rm -rf notes/archive
  bdy cmd history -n 20

Notes:
  find and grep search remote path and filename metadata, not file contents.
  vim downloads a temporary copy, opens $EDITOR or vim, and uploads it back.`)
}

func printLFSHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy lfs - Git-LFS-style large file storage

Usage:
  bdy lfs install
  bdy lfs track '<pattern>'
  bdy lfs untrack '<pattern>'
  bdy lfs status
  bdy lfs ls-files
  bdy lfs push
  bdy lfs fetch
  bdy lfs checkout
  bdy lfs pull

Git filter commands:
  bdy lfs clean -- <path>
  bdy lfs smudge -- <path>

Remote object root:
  /apps/baiduyunStorage/lfs

Local cache:
  .bdy/lfs/objects

Examples:
  bdy lfs install
  bdy lfs track '*.zip'
  git add .gitattributes large.zip
  git commit -m 'track large file'
  bdy lfs push
  bdy lfs pull

Git stores pointer files. Real contents are cached locally and uploaded to Baidu
Netdisk by SHA-256 object ID.`)
}

func printSyncHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy snapshot sync - lightweight manifest-based folder sync

Usage:
  bdy init [--remote-root /apps/baiduyunStorage/workspace]
  bdy status
  bdy add <path...>
  bdy commit -m <message>
  bdy push
  bdy pull
  bdy ls [remote-path]
  bdy rm <path...>
  bdy mv <old> <new>
  bdy remote

Examples:
  bdy init
  bdy status
  bdy add notes.txt docs
  bdy commit -m 'snapshot'
  bdy push
  bdy pull

This is not a full Git database. It stores snapshots and manifests under .bdy/
locally and syncs them to an isolated remote root. Use 'bdy lfs' for true large
file object storage.`)
}

func cmdLFS(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy lfs install|track|untrack|status|ls-files|push|fetch|pull|checkout|clean|smudge")
	}
	switch args[0] {
	case "install":
		root, err := lfs.GitRoot(".")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(root, repo.DirName, "lfs", "objects"), 0o755); err != nil {
			return err
		}
		exe, _ := os.Executable()
		if err := lfs.InstallGitFilters(root, exe); err != nil {
			return err
		}
		fmt.Fprintln(out, "bdy lfs filters installed")
		return nil
	case "track":
		root, err := lfs.GitRoot(".")
		if err != nil {
			return err
		}
		if err := lfs.Track(root, args[1:]); err != nil {
			return err
		}
		fmt.Fprintf(out, "tracking %d pattern(s)\n", len(args)-1)
		return nil
	case "untrack":
		root, err := lfs.GitRoot(".")
		if err != nil {
			return err
		}
		if err := lfs.Untrack(root, args[1:]); err != nil {
			return err
		}
		fmt.Fprintf(out, "untracked %d pattern(s)\n", len(args)-1)
		return nil
	case "clean":
		return lfsClean(args[1:], out)
	case "smudge":
		return lfsSmudge(args[1:], out)
	case "ls-files":
		return lfsLsFiles(out)
	case "status":
		return lfsStatus(out)
	case "push":
		return lfsPush(ctx, out)
	case "fetch":
		return lfsFetch(ctx, out)
	case "checkout":
		return lfsCheckout(out)
	case "pull":
		if err := lfsFetch(ctx, out); err != nil {
			return err
		}
		return lfsCheckout(out)
	default:
		return errors.New("usage: bdy lfs install|track|untrack|status|ls-files|push|fetch|pull|checkout|clean|smudge")
	}
}

func cmdConfig(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "set-app" {
		return errors.New("usage: bdy config set-app --app-id ID --app-key KEY --secret-key SECRET [--sign-key SIGN]")
	}
	fs := flag.NewFlagSet("set-app", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	appID := fs.String("app-id", "", "")
	appKey := fs.String("app-key", "", "")
	secretKey := fs.String("secret-key", "", "")
	signKey := fs.String("sign-key", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *appKey == "" || *secretKey == "" {
		return errors.New("--app-key and --secret-key are required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AppID = *appID
	cfg.AppKey = *appKey
	cfg.SecretKey = *secretKey
	cfg.SignKey = *signKey
	if err := config.Save(cfg); err != nil {
		return err
	}
	path, _ := config.Path()
	fmt.Fprintf(out, "saved app config to %s\n", path)
	return nil
}

func cmdAuth(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy auth login|status")
	}
	switch args[0] {
	case "status":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasToken() {
			return errors.New("not logged in or token expired")
		}
		fmt.Fprintf(out, "logged in; token expires at %s\n", cfg.ExpiresAt.Format(time.RFC3339))
		return nil
	case "login":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasApp() {
			return errors.New("app credentials missing; run bdy config set-app first")
		}
		oa := auth.New()
		dc, err := oa.RequestDeviceCode(ctx, cfg.AppKey)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Open: %s\nCode: %s\nQR: %s\n", dc.VerificationURL, dc.UserCode, dc.QRCodeURL)
		deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(time.Duration(dc.Interval) * time.Second)
			tok, err := oa.PollToken(ctx, cfg.AppKey, cfg.SecretKey, dc.DeviceCode)
			if err != nil {
				if strings.Contains(err.Error(), "authorization_pending") || strings.Contains(err.Error(), "slow_down") {
					continue
				}
				return err
			}
			cfg.AccessToken = tok.AccessToken
			cfg.RefreshToken = tok.RefreshToken
			cfg.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintln(out, "login complete")
			return nil
		}
		return errors.New("device code expired")
	default:
		return errors.New("usage: bdy auth login|status")
	}
}

func cmdInit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	remoteRoot := fs.String("remote-root", repo.DefaultRemoteRoot, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(*remoteRoot); err != nil {
		return err
	}
	r, err := repo.Init(".", *remoteRoot)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "initialized bdy repo in %s\n", r.Dir)
	return nil
}

func cmdHome(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy home ls [path]")
	}
	switch args[0] {
	case "ls":
		cfg, err := auth.EnsureToken(ctx)
		if err != nil {
			return err
		}
		dir := "/"
		if len(args) > 1 {
			dir = homePath(args[1])
		}
		items, err := baidu.NewClient(cfg).List(ctx, dir)
		if err != nil {
			return err
		}
		printRemoteEntries(out, items)
		return nil
	default:
		return errors.New("usage: bdy home ls [path]")
	}
}

func cmdStatus(out io.Writer) error {
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	base, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	current, err := repo.Scan(r.Root)
	if err != nil {
		return err
	}
	st := repo.Diff(base, current)
	printStatus(out, st)
	return nil
}

func cmdAdd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy add <path...>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	for _, p := range args {
		idx.Add(p)
	}
	if err := repo.SaveIndex(r.IndexPath(), idx); err != nil {
		return err
	}
	fmt.Fprintf(out, "staged %d path(s)\n", len(args))
	return nil
}

func cmdRemove(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy rm <path...>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	for _, p := range args {
		idx.Remove(p)
		_ = os.RemoveAll(filepath.Join(r.Root, repo.CleanPath(p)))
	}
	if err := repo.SaveIndex(r.IndexPath(), idx); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %d path(s)\n", len(args))
	return nil
}

func cmdMove(args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: bdy mv <old> <new>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	oldPath := filepath.Join(r.Root, repo.CleanPath(args[0]))
	newPath := filepath.Join(r.Root, repo.CleanPath(args[1]))
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	idx.Move(args[0], args[1])
	if err := repo.SaveIndex(r.IndexPath(), idx); err != nil {
		return err
	}
	fmt.Fprintf(out, "moved %s -> %s\n", args[0], args[1])
	return nil
}

func cmdCommit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	msg := fs.String("m", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *msg == "" {
		return errors.New("usage: bdy commit -m <message>")
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	idx, err := repo.LoadIndex(r.IndexPath())
	if err != nil {
		return err
	}
	if len(idx.Added) == 0 && len(idx.Removed) == 0 && len(idx.Moved) == 0 {
		return errors.New("nothing staged")
	}
	current, err := repo.Scan(r.Root)
	if err != nil {
		return err
	}
	base, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	next := repo.ApplyIndex(base, current, idx)
	c, err := repo.CreateCommit(r, *msg, next)
	if err != nil {
		return err
	}
	if err := repo.SaveManifest(r.ManifestPath(), next); err != nil {
		return err
	}
	if err := repo.SaveIndex(r.IndexPath(), repo.Index{}); err != nil {
		return err
	}
	fmt.Fprintf(out, "[%s] %s\n", c.ID, c.Message)
	return nil
}

func cmdLS(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(r.Config.RemoteRoot); err != nil {
		return err
	}
	dir := r.Config.RemoteRoot
	if len(args) > 0 {
		dir = remoteJoin(r.Config.RemoteRoot, args[0])
	}
	items, err := baidu.NewClient(cfg).List(ctx, dir)
	if err != nil {
		return err
	}
	printRemoteEntries(out, items)
	return nil
}

func cmdPush(ctx context.Context, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(r.Config.RemoteRoot); err != nil {
		return err
	}
	local, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)
	_ = client.Mkdir(ctx, r.Config.RemoteRoot)
	_ = client.Mkdir(ctx, remoteJoin(r.Config.RemoteRoot, ".bdy"))
	_ = client.Mkdir(ctx, remoteJoin(r.Config.RemoteRoot, ".bdy/commits"))
	remote, _ := loadRemoteManifest(ctx, client, r)
	diff := repo.Diff(remote, local)
	for _, p := range append(diff.Added, diff.Modified...) {
		entry := local.Map()[p]
		if entry.IsDir {
			_ = client.Mkdir(ctx, remoteJoin(r.Config.RemoteRoot, p))
			continue
		}
		if err := client.UploadFile(ctx, filepath.Join(r.Root, p), remoteJoin(r.Config.RemoteRoot, p)); err != nil {
			return fmt.Errorf("upload %s: %w", p, err)
		}
		fmt.Fprintf(out, "uploaded %s\n", p)
	}
	for _, p := range diff.Deleted {
		_ = client.FileManager(ctx, "delete", []string{remoteJoin(r.Config.RemoteRoot, p)})
		fmt.Fprintf(out, "deleted remote %s\n", p)
	}
	if err := client.UploadFile(ctx, r.ManifestPath(), remoteJoin(r.Config.RemoteRoot, ".bdy/manifest.json")); err != nil {
		return err
	}
	for _, id := range commitFiles(r) {
		localPath := filepath.Join(r.CommitsDir(), id)
		if err := client.UploadFile(ctx, localPath, remoteJoin(r.Config.RemoteRoot, ".bdy/commits/"+id)); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "push complete")
	return nil
}

func cmdPull(ctx context.Context, out io.Writer) error {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	if err := validateIsolatedRemoteRoot(r.Config.RemoteRoot); err != nil {
		return err
	}
	base, err := repo.LoadManifest(r.ManifestPath())
	if err != nil {
		return err
	}
	local, err := repo.Scan(r.Root)
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)
	remote, err := loadRemoteManifest(ctx, client, r)
	if err != nil {
		return err
	}
	conflicts := repo.Conflicts(base, local, remote)
	if len(conflicts) > 0 {
		return fmt.Errorf("conflicts: %s", strings.Join(conflicts, ", "))
	}
	allRemote, err := client.ListAll(ctx, r.Config.RemoteRoot)
	if err != nil {
		return err
	}
	fsids := map[string]uint64{}
	for _, item := range allRemote {
		rel := strings.TrimPrefix(item.Path, strings.TrimRight(r.Config.RemoteRoot, "/")+"/")
		if strings.HasPrefix(rel, ".bdy/") {
			continue
		}
		fsids[rel] = item.FSID
	}
	diff := repo.Diff(base, remote)
	remoteMap := remote.Map()
	for _, p := range append(diff.Added, diff.Modified...) {
		entry := remoteMap[p]
		dest := filepath.Join(r.Root, p)
		if entry.IsDir {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		meta, err := client.FileMetas(ctx, []uint64{fsids[p]}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return fmt.Errorf("missing dlink for %s", p)
		}
		if err := client.Download(ctx, meta[0].DLink, dest); err != nil {
			return err
		}
		fmt.Fprintf(out, "downloaded %s\n", p)
	}
	for _, p := range diff.Deleted {
		_ = os.RemoveAll(filepath.Join(r.Root, p))
		fmt.Fprintf(out, "deleted local %s\n", p)
	}
	if err := repo.SaveManifest(r.ManifestPath(), remote); err != nil {
		return err
	}
	fmt.Fprintln(out, "pull complete")
	return nil
}

func cmdRemote(ctx context.Context, out io.Writer) error {
	r, err := repo.Open(".")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "remote root: %s\n", r.Config.RemoteRoot)
	cfg, _ := config.Load()
	if cfg.AppID != "" {
		fmt.Fprintf(out, "app id: %s\n", cfg.AppID)
	}
	return nil
}

func printStatus(out io.Writer, st repo.Status) {
	for _, p := range st.Added {
		fmt.Fprintf(out, "A  %s\n", p)
	}
	for _, p := range st.Modified {
		fmt.Fprintf(out, "M  %s\n", p)
	}
	for _, p := range st.Deleted {
		fmt.Fprintf(out, "D  %s\n", p)
	}
	if len(st.Added)+len(st.Modified)+len(st.Deleted) == 0 {
		fmt.Fprintln(out, "clean")
	}
}

func printRemoteEntries(out io.Writer, items []baidu.FileEntry) {
	for _, item := range items {
		kind := "file"
		if item.IsDir == 1 {
			kind = "dir "
		}
		fmt.Fprintf(out, "%s %10d %s\n", kind, item.Size, item.Path)
	}
}

func validateIsolatedRemoteRoot(root string) error {
	root = strings.TrimRight(root, "/")
	if root == "" || root == "/" {
		return errors.New("ordinary bdy commands cannot target /; use bdy home ls [path] for whole-netdisk access")
	}
	appRoot := strings.TrimRight(repo.AppRoot, "/")
	if root == appRoot || strings.HasPrefix(root, appRoot+"/") {
		return nil
	}
	return fmt.Errorf("ordinary bdy commands are isolated under %s; use bdy home ls [path] for whole-netdisk access", repo.AppRoot)
}

func homePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	p = strings.TrimPrefix(repo.CleanPath(p), "/")
	return "/" + p
}

func loadRemoteManifest(ctx context.Context, client baidu.Client, r repo.Repo) (repo.Manifest, error) {
	items, err := client.List(ctx, remoteJoin(r.Config.RemoteRoot, ".bdy"))
	if err != nil {
		return repo.Manifest{}, err
	}
	for _, item := range items {
		if filepath.Base(item.Path) != "manifest.json" {
			continue
		}
		meta, err := client.FileMetas(ctx, []uint64{item.FSID}, true)
		if err != nil {
			return repo.Manifest{}, err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return repo.Manifest{}, errors.New("remote manifest has no dlink")
		}
		tmp, err := os.CreateTemp("", "bdy-remote-manifest-*")
		if err != nil {
			return repo.Manifest{}, err
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		defer os.Remove(tmpPath)
		if err := client.Download(ctx, meta[0].DLink, tmpPath); err != nil {
			return repo.Manifest{}, err
		}
		return repo.LoadManifest(tmpPath)
	}
	return repo.Manifest{}, errors.New("remote manifest not found")
}

func remoteJoin(root, p string) string {
	p = repo.CleanPath(p)
	if p == "" {
		return root
	}
	return strings.TrimRight(root, "/") + "/" + p
}

func commitFiles(r repo.Repo) []string {
	items, err := os.ReadDir(r.CommitsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		if !item.IsDir() && strings.HasSuffix(item.Name(), ".json") {
			out = append(out, item.Name())
		}
	}
	return out
}
