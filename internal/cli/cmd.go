package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/repo"
)

type cmdHistoryEntry struct {
	Time    time.Time `json:"time"`
	Command string    `json:"command"`
	Args    []string  `json:"args,omitempty"`
}

const (
	cmdCWDEnv        = "BDY_CMD_CWD"
	cmdSessionDirEnv = "BDY_CMD_SESSION_DIR"
	cmdUsage         = "usage: bdy cmd cd|pwd|ls|ll|la|find|grep|search|rm|delete|history|cat|download|get|mkdir|touch|vim"
)

type cloudFileSpace struct {
	Name      string
	Root      string
	Resolve   func(string) string
	AllowCD   bool
	UseLongLS bool
}

var cmdInput io.Reader = os.Stdin

func cmdShell(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return runCloudFileREPL(ctx, cmdInput, out, cloudFileSpace{
			Name:    "cmd",
			Root:    repo.CmdRoot,
			Resolve: cmdPath,
			AllowCD: true,
		})
	}
	return runCloudFileCommand(ctx, args, out, cloudFileSpace{
		Name:    "cmd",
		Root:    repo.CmdRoot,
		Resolve: cmdPath,
		AllowCD: true,
	})
}

func runCloudFileREPL(ctx context.Context, in io.Reader, out io.Writer, space cloudFileSpace) error {
	cwd := space.Root
	if space.Name == "cmd" {
		cwd = cmdBasePath()
	}
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprintf(out, "bdy:%s$ ", cwd)
		if !scanner.Scan() {
			fmt.Fprintln(out)
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		args, err := parseInteractiveLine(line)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}
		if len(args) == 0 {
			continue
		}
		switch args[0] {
		case "exit", "quit":
			return nil
		case "help":
			fmt.Fprintln(out, cmdUsage)
			continue
		case "pwd":
			fmt.Fprintln(out, cwd)
			continue
		case "cd":
			target := "."
			if len(args) > 1 {
				target = args[1]
			}
			cwd = resolveCloudPath(space.Root, cwd, target)
			continue
		}
		replSpace := space
		replSpace.AllowCD = false
		replSpace.Resolve = func(p string) string {
			return resolveCloudPath(space.Root, cwd, p)
		}
		if err := runCloudFileCommand(ctx, args, out, replSpace); err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
		}
	}
}

func parseInteractiveLine(line string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args, nil
}

func runCloudFileCommand(ctx context.Context, args []string, out io.Writer, space cloudFileSpace) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bdy %s cd|pwd|ls|ll|la|find|grep|rm|delete|history|cat|mkdir|touch|vim", space.Name)
	}
	switch args[0] {
	case "cd":
		if !space.AllowCD {
			return fmt.Errorf("cd is only supported for bdy cmd; use absolute paths with bdy %s", space.Name)
		}
		target := "."
		if len(args) > 1 {
			target = args[1]
		}
		resolved := space.Resolve(target)
		_ = saveCmdSessionPath(resolved)
		fmt.Fprintf(out, "export %s=%s\n", cmdCWDEnv, shellQuote(resolved))
		return nil
	case "pwd":
		if space.AllowCD {
			fmt.Fprintln(out, cmdBasePath())
			return nil
		}
		fmt.Fprintln(out, space.Root)
		return nil
	}
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return err
	}
	client := baidu.NewClient(cfg)
	switch args[0] {
	case "ls", "ll", "la":
		defer recordCmdHistory(args[0], args[1:])
		opts, path, err := parseLSArgs(args[0], args[1:])
		if err != nil {
			return err
		}
		items, err := client.List(ctx, space.Resolve(path))
		if err != nil {
			return err
		}
		if space.UseLongLS && !opts.All && !opts.Long {
			printRemoteEntries(out, items)
			return nil
		}
		printCmdEntries(out, items, opts)
		return nil
	case "find", "grep", "search":
		defer recordCmdHistory(args[0], args[1:])
		opts, err := parseSearchArgs(args[0], args[1:])
		if err != nil {
			return err
		}
		return cmdFind(ctx, client, out, opts, space.Resolve)
	case "rm", "delete":
		defer recordCmdHistory(args[0], args[1:])
		rmOpts, err := parseRMArgs(args[0], args[1:])
		if err != nil {
			return err
		}
		var paths []string
		for _, p := range rmOpts.Paths {
			paths = append(paths, space.Resolve(p))
		}
		if err := client.FileManager(ctx, "delete", paths); err != nil {
			if !rmOpts.Force {
				return err
			}
		}
		for _, p := range paths {
			fmt.Fprintf(out, "deleted %s\n", p)
		}
		return nil
	case "mkdir":
		defer recordCmdHistory("mkdir", args[1:])
		mkdirOpts, err := parseMkdirArgs(args[1:])
		if err != nil {
			return err
		}
		for _, p := range mkdirOpts.Paths {
			remotePath := space.Resolve(p)
			if err := ensureRemoteDir(ctx, client, space.Root, remotePath); err != nil {
				return err
			}
			fmt.Fprintf(out, "created %s\n", remotePath)
		}
		return nil
	case "touch":
		defer recordCmdHistory("touch", args[1:])
		touchOpts, err := parseTouchArgs(args[1:])
		if err != nil {
			return err
		}
		for _, p := range touchOpts.Paths {
			remotePath := space.Resolve(p)
			if touchOpts.NoCreate {
				if _, err := findRemoteEntry(ctx, client, remotePath); err != nil {
					continue
				}
			}
			if err := cmdTouch(ctx, client, space.Root, remotePath); err != nil {
				return err
			}
			fmt.Fprintf(out, "touched %s\n", remotePath)
		}
		return nil
	case "history":
		return cmdHistory(args[1:], out)
	case "cat":
		defer recordCmdHistory("cat", args[1:])
		catOpts, err := parseCatArgs(args[1:])
		if err != nil {
			return err
		}
		for i, path := range catOpts.Paths {
			if i > 0 {
				fmt.Fprintln(out)
			}
			if err := cmdCat(ctx, client, out, space.Resolve(path), catOpts.Number); err != nil {
				return err
			}
		}
		return nil
	case "download", "get":
		defer recordCmdHistory(args[0], args[1:])
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: bdy %s %s <remote-path> [local-path]", space.Name, args[0])
		}
		dest := "."
		if len(args) == 3 {
			dest = args[2]
		}
		localPath, err := downloadDestination(space.Resolve(args[1]), dest)
		if err != nil {
			return err
		}
		if err := cmdDownload(ctx, client, out, space.Resolve(args[1]), localPath); err != nil {
			return err
		}
		fmt.Fprintf(out, "downloaded %s -> %s\n", space.Resolve(args[1]), localPath)
		return nil
	case "vim":
		if len(args) != 2 {
			return fmt.Errorf("usage: bdy %s vim <path>", space.Name)
		}
		defer recordCmdHistory("vim", args[1:])
		return cmdVim(ctx, client, out, space.Root, space.Resolve(args[1]))
	default:
		return fmt.Errorf("usage: bdy %s cd|pwd|ls|ll|la|find|grep|search|rm|delete|history|cat|download|get|mkdir|touch|vim", space.Name)
	}
}
