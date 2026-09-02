package cli

import (
	"fmt"
	"io"

	"baiduyunStorage/internal/repo"
)

func printRootHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy - Baidu Netdisk command line storage

Usage:
  bdy [global flags] <space> <command> [command flags] [args]
  bdy help [command]
  bdy <command> --help

Global flags:
  -h, --help       Show help
  -v, --version    Show version
  -q, --quiet      Reduce optional output
  -y, --yes        Assume yes for commands that ask for confirmation
  -C, --cwd PATH   Temporary cloud working directory for this command
  --json           Reserve machine-readable output mode for supported commands

Spaces:
  cmd      Manage files under /apps/baiduyunStorage
  lfs      Store Git-LFS-style large objects under /apps/baiduyunStorage/lfs
  nd       Git-like NetDisk version storage under .bdynd
  home     Inspect the whole Baidu Netdisk only when explicitly requested
  sync     Snapshot sync commands under an isolated remote workspace

Core commands:
  config   Manage app credentials or rewrite auth config
  auth     OAuth device-code login, SDK token import, and token status
  cmd      Bash-style cloud file commands: ls, find, grep, rm, cat, mkdir, touch, vim
  lfs      Git-LFS-style large file commands: track, push, fetch, checkout, pull
  nd       NetDisk version commands: init, add, commit, log, status, show
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
  bdy config set-app --app-key AK --secret-key SK
  bdy auth login
  bdy auth status
  bdy cmd ls -al
  bdy -C git cmd ls
  eval "$(bdy cmd cd git)"
  bdy home cmd mkdir /tmp/demo
  bdy nd init
  bdy lfs track '*.zip'
  bdy sync init
  bdy sync add README.md && bdy sync commit -m 'snapshot' && bdy sync push`)
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
	case "nd":
		printNDHelp(out)
	case "init", "status", "add", "commit", "push", "pull", "ls", "rm", "mv", "remote", "sync":
		printSyncHelp(out)
	default:
		return fmt.Errorf("unknown help topic %q", topic)
	}
	return nil
}

func printNestedHelp(out io.Writer, args []string) error {
	if len(args) == 0 {
		printRootHelp(out)
		return nil
	}
	if len(args) == 1 || isHelpArg(args[1]) {
		return printHelpTopic(out, args[0])
	}
	switch args[0] {
	case "cmd":
		return printFileCommandHelp(out, "cmd", args[1], false)
	case "home":
		sub := args[1]
		viaCmd := false
		if sub == "cmd" {
			viaCmd = true
			if len(args) < 3 {
				printHomeHelp(out)
				return nil
			}
			sub = args[2]
		}
		return printFileCommandHelp(out, "home", sub, viaCmd)
	case "sync":
		printSyncHelp(out)
		return nil
	default:
		return printHelpTopic(out, args[0])
	}
}

func printFileCommandHelp(out io.Writer, space, command string, viaCmd bool) error {
	prefix := "bdy " + space
	root := repo.CmdRoot
	if space == "home" {
		root = "/"
	}
	if command == "cmd" && space == "home" {
		printHomeHelp(out)
		return nil
	}
	if viaCmd {
		fmt.Fprintf(out, "Equivalent:\n  bdy home %s", command)
		switch command {
		case "mkdir", "touch", "cat", "download", "get", "rm", "delete", "find", "grep", "search", "ls", "history", "vim", "pwd", "cd":
			fmt.Fprintln(out, " ...")
		default:
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "\nAlso accepted:\n  bdy home cmd %s ...\n\n", command)
	}
	switch command {
	case "pwd":
		fmt.Fprintf(out, "%s pwd\n\nRoot:\n  %s\n", prefix, root)
	case "cd":
		fmt.Fprintf(out, "%s cd [path]\n\nFor cmd space, use:\n  eval \"$(bdy cmd cd git)\"\n", prefix)
	case "ls", "ll", "la":
		fmt.Fprintf(out, "%s ls [-a] [-l] [path]\n%s ls [-al] [path]\n\nRoot:\n  %s\n", prefix, prefix, root)
	case "find":
		fmt.Fprintf(out, "%s find [-i] [-E|--regex] [-name pattern] [-type f|d] [pattern] [path]\n\nUses Baidu remote search to prefilter candidates, then matches path and filename metadata locally.\n", prefix)
	case "grep":
		fmt.Fprintf(out, "%s grep [-i] [-v] [-E|--regex] [-type f|d] <pattern> [path]\n\nUses Baidu remote search to prefilter path and filename metadata, not file contents.\n", prefix)
	case "search":
		fmt.Fprintf(out, "%s search [-i] [-E|--regex] [-type f|d] <pattern> [path]\n\nFast path and filename metadata search using Baidu remote search plus local glob or regex filtering. Quote shell globs, for example: %s search '*.mp4'\n\nRegex examples:\n  %s search --regex '.*\\.mp4$'\n  %s search -E '^demo-[0-9]+\\.mp4$'\n", prefix, prefix, prefix, prefix)
	case "rm", "delete":
		fmt.Fprintf(out, "%s %s [-r] [-f] <path...>\n\nDeletes remote cloud paths.\n", prefix, command)
	case "history":
		fmt.Fprintf(out, "%s history [-c] [-n N]\n", prefix)
	case "cat":
		fmt.Fprintf(out, "%s cat [-n] <path...>\n", prefix)
	case "download", "get":
		fmt.Fprintf(out, "%s %s <remote-path> [local-path]\n\nIf local-path is omitted or is a directory, the remote filename is used. Downloads show byte progress.\n", prefix, command)
	case "mkdir":
		fmt.Fprintf(out, "%s mkdir [-p] <path...>\n\nRoot:\n  %s\n", prefix, root)
	case "touch":
		fmt.Fprintf(out, "%s touch [-c] <path...>\n\nRoot:\n  %s\n", prefix, root)
	case "vim":
		fmt.Fprintf(out, "%s vim <path>\n\nDownloads a temporary copy, opens $EDITOR or vim, then uploads it back.\n", prefix)
	default:
		return fmt.Errorf("unknown %s command %q", space, command)
	}
	return nil
}

func printConfigHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy config - manage local auth configuration

Usage:
  bdy config set-app --app-key KEY --secret-key SECRET [--app-id ID] [--sign-key SIGN]
  bdy config clear-app

set-app:
  Save Baidu Open Platform application credentials (AppKey + SecretKey) locally.
  These are required for the OAuth device-code login flow:
    bdy auth login

Token-only mode:
  Prefer external SDK token injection when you do not want bdy to store app credentials:
    export BDY_ACCESS_TOKEN='...'
    export BDY_TOKEN_EXPIRES_IN=2592000
    bdy auth status

Config file:
  ~/.config/bdy/config.json

The config file may contain app credentials and access tokens.
Keep it private and never commit it.`)
}

func printAuthHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy auth - Baidu OAuth login and token status

Usage:
  bdy auth login
  bdy auth login --temporary 1d
  bdy auth import-token
  bdy auth import-token --temporary 1d
  bdy auth status

Commands:
  login         Start the OAuth device-code flow, print the verification URL, user code, and QR URL
  import-token  Save a token produced by your own SDK layer
  status        Check whether a valid token is saved locally

OAuth flow:
  1. Run 'bdy config set-app --app-key KEY --secret-key SECRET' once with your app credentials.
  2. Run 'bdy auth login'.
  3. Open the printed URL or QR code and approve the basic,netdisk scope.
  4. Use 'bdy auth status' to verify the saved token.

SDK token / Token-only flow:
  export BDY_ACCESS_TOKEN='...'
  export BDY_TOKEN_EXPIRES_IN=2592000
  bdy auth status

Temporary read-only:
  bdy auth login --temporary 1d stores ~/.config/bdy/temporary.json and blocks write commands locally.`)
}

func printHomeHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy home - explicit whole-netdisk inspection

Usage:
  bdy home ls [path]
  bdy home search <pattern> [path]

Examples:
  bdy home ls /
  bdy home search mp4
  bdy home search '*.mp4'
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
  bdy cmd download <remote-path> [local-path]
  bdy cmd get <remote-path> [local-path]
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
  bdy cmd download notes/today.txt .
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
  bdy sync init [--remote-root /apps/baiduyunStorage/workspace]
  bdy sync status
  bdy sync add <path...>
  bdy sync commit -m <message>
  bdy sync push
  bdy sync pull
  bdy sync ls [remote-path]
  bdy sync rm <path...>
  bdy sync mv <old> <new>
  bdy sync remote

Legacy aliases:
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
  bdy sync init
  bdy sync status
  bdy sync add notes.txt docs
  bdy sync commit -m 'snapshot'
  bdy sync push
  bdy sync pull

This is not a full Git database. It stores snapshots and manifests under .bdy/
locally and syncs them to an isolated remote root. Use 'bdy lfs' for true large
file object storage.`)
}
