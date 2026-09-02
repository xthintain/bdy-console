package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"baiduyunStorage/internal/config"
)

var version = "dev"

const quietEnv = "BDY_QUIET"

type globalOptions struct {
	CWD   string
	Yes   bool
	Quiet bool
	JSON  bool
}

func Run(args []string, stdout, stderr io.Writer) int {
	opts, args, showHelp, showVersion, err := parseGlobalArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if showVersion {
		fmt.Fprintf(stdout, "bdy %s\n", version)
		return 0
	}
	if len(args) == 0 || showHelp {
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
	if containsHelpArg(args[1:]) {
		if err := printNestedHelp(stdout, args); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	}
	restore := applyGlobalOptions(opts)
	defer restore()
	ctx := context.Background()
	if err := enforceTemporaryReadOnly(args); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if err := run(ctx, args, stdout); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func parseGlobalArgs(args []string) (globalOptions, []string, bool, bool, error) {
	opts := globalOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			return opts, args[i+1:], true, false, nil
		case "-v", "--version":
			return opts, args[i+1:], false, true, nil
		case "-q", "--quiet":
			opts.Quiet = true
		case "-y", "--yes":
			opts.Yes = true
		case "--json":
			opts.JSON = true
		case "-C", "--cwd":
			i++
			if i >= len(args) {
				return opts, nil, false, false, errors.New("-C requires a path")
			}
			opts.CWD = args[i]
		case "--":
			return opts, args[i+1:], false, false, nil
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, nil, false, false, fmt.Errorf("unknown global flag %s", arg)
			}
			return opts, args[i:], false, false, nil
		}
	}
	return opts, nil, false, false, nil
}

func applyGlobalOptions(opts globalOptions) func() {
	oldCWD, hadCWD := os.LookupEnv(cmdCWDEnv)
	oldQuiet, hadQuiet := os.LookupEnv(quietEnv)
	if opts.CWD != "" {
		_ = os.Setenv(cmdCWDEnv, cmdPathFromRoot(opts.CWD))
	}
	if opts.Quiet {
		_ = os.Setenv(quietEnv, "1")
	}
	return func() {
		if opts.CWD != "" {
			if hadCWD {
				_ = os.Setenv(cmdCWDEnv, oldCWD)
			} else {
				_ = os.Unsetenv(cmdCWDEnv)
			}
		}
		if opts.Quiet {
			if hadQuiet {
				_ = os.Setenv(quietEnv, oldQuiet)
			} else {
				_ = os.Unsetenv(quietEnv)
			}
		}
	}
}

func isHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func containsHelpArg(args []string) bool {
	for _, arg := range args {
		if isHelpArg(arg) {
			return true
		}
	}
	return false
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
	case "nd":
		return cmdND(ctx, args[1:], out)
	case "sync":
		return cmdSync(ctx, args[1:], out)
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

var errTemporaryReadOnlyWrite = errors.New("temporary read-only auth forbids write operation")

func enforceTemporaryReadOnly(args []string) error {
	cfg, err := config.LoadActive()
	if err != nil {
		return err
	}
	if !cfg.IsTemporaryReadOnly() {
		return nil
	}
	if temporaryReadOnlyAllows(args) {
		return nil
	}
	return errTemporaryReadOnlyWrite
}

func temporaryReadOnlyAllows(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "help":
		return true
	case "auth":
		return temporaryReadOnlyAllowsAuth(args[1:])
	case "cmd":
		return len(args) > 1 && oneOf(args[1], "cd", "pwd", "ls", "ll", "la", "find", "grep", "search", "cat", "download", "get", "history")
	case "home":
		if len(args) < 2 {
			return false
		}
		if args[1] == "cmd" {
			return len(args) > 2 && oneOf(args[2], "ls", "ll", "la", "find", "grep", "search", "cat", "download", "get")
		}
		return oneOf(args[1], "ls", "ll", "la", "find", "grep", "search", "cat", "download", "get")
	case "nd":
		return temporaryReadOnlyAllowsND(args[1:])
	case "lfs":
		return len(args) > 1 && oneOf(args[1], "fetch", "checkout", "status", "ls-files")
	case "sync":
		return len(args) > 1 && oneOf(args[1], "status", "ls", "pull", "remote")
	case "status", "ls", "pull", "remote":
		return true
	default:
		return false
	}
}

func temporaryReadOnlyAllowsAuth(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "status" {
		return true
	}
	if args[0] != "import-token" {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "--temporary" || strings.HasPrefix(arg, "--temporary=") {
			return true
		}
	}
	return false
}

func temporaryReadOnlyAllowsND(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "clone", "fetch", "pull", "log", "status", "show", "diff", "index", "search":
		return true
	case "lfs":
		return len(args) > 1 && oneOf(args[1], "fetch", "checkout", "status", "ls-files")
	case "pack":
		return len(args) > 1 && args[1] == "fetch"
	default:
		return false
	}
}

func oneOf(s string, allowed ...string) bool {
	for _, item := range allowed {
		if s == item {
			return true
		}
	}
	return false
}
