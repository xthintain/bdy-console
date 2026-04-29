package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/config"
	"baiduyunStorage/internal/repo"
)

type cmdHistoryEntry struct {
	Time    time.Time `json:"time"`
	Command string    `json:"command"`
	Args    []string  `json:"args,omitempty"`
}

const (
	cmdCWDEnv = "BDY_CMD_CWD"
	cmdUsage  = "usage: bdy cmd cd|pwd|ls|ll|la|find|grep|rm|delete|history|cat|mkdir|touch|vim"
)

type cloudFileSpace struct {
	Name      string
	Root      string
	Resolve   func(string) string
	AllowCD   bool
	UseLongLS bool
}

func cmdShell(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(cmdUsage)
	}
	return runCloudFileCommand(ctx, args, out, cloudFileSpace{
		Name:    "cmd",
		Root:    repo.CmdRoot,
		Resolve: cmdPath,
		AllowCD: true,
	})
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
		fmt.Fprintf(out, "export %s=%s\n", cmdCWDEnv, shellQuote(space.Resolve(target)))
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
	case "find", "grep":
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
	case "vim":
		if len(args) != 2 {
			return fmt.Errorf("usage: bdy %s vim <path>", space.Name)
		}
		defer recordCmdHistory("vim", args[1:])
		return cmdVim(ctx, client, out, space.Root, space.Resolve(args[1]))
	default:
		return fmt.Errorf("usage: bdy %s cd|pwd|ls|ll|la|find|grep|rm|delete|history|cat|mkdir|touch|vim", space.Name)
	}
}

type cmdSearchOptions struct {
	Command    string
	Pattern    string
	Path       string
	IgnoreCase bool
	Invert     bool
	Type       string
}

func parseSearchArgs(command string, args []string) (cmdSearchOptions, error) {
	opts := cmdSearchOptions{Command: command, Path: "."}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-i":
			opts.IgnoreCase = true
		case "-v":
			opts.Invert = true
		case "-name":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("usage: bdy cmd %s [-i] [-v] [-name pattern] [-type f|d] [pattern] [path]", command)
			}
			opts.Pattern = args[i]
		case "-type":
			i++
			if i >= len(args) || (args[i] != "f" && args[i] != "d") {
				return opts, fmt.Errorf("usage: bdy cmd %s [-type f|d]", command)
			}
			opts.Type = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				for _, flag := range strings.TrimPrefix(arg, "-") {
					switch flag {
					case 'i':
						opts.IgnoreCase = true
					case 'v':
						opts.Invert = true
					default:
						return opts, fmt.Errorf("unsupported %s flag -%c", command, flag)
					}
				}
				continue
			}
			positional = append(positional, arg)
		}
	}
	if opts.Pattern == "" && len(positional) > 0 {
		opts.Pattern = positional[0]
		positional = positional[1:]
	}
	if len(positional) > 0 {
		opts.Path = positional[0]
	}
	if opts.Pattern == "" {
		return opts, fmt.Errorf("usage: bdy cmd %s <pattern> [path]", command)
	}
	return opts, nil
}

type cmdRMOptions struct {
	Force bool
	Paths []string
}

func parseRMArgs(command string, args []string) (cmdRMOptions, error) {
	opts := cmdRMOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'f':
					opts.Force = true
				case 'r', 'R':
				default:
					return opts, fmt.Errorf("unsupported %s flag -%c", command, flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, fmt.Errorf("usage: bdy cmd %s [-r] [-f] <path...>", command)
	}
	return opts, nil
}

type cmdMkdirOptions struct {
	Paths []string
}

func parseMkdirArgs(args []string) (cmdMkdirOptions, error) {
	opts := cmdMkdirOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'p', 'v':
				default:
					return opts, fmt.Errorf("unsupported mkdir flag -%c", flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, errors.New("usage: bdy cmd mkdir [-p] <path...>")
	}
	return opts, nil
}

type cmdTouchOptions struct {
	NoCreate bool
	Paths    []string
}

func parseTouchArgs(args []string) (cmdTouchOptions, error) {
	opts := cmdTouchOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'c':
					opts.NoCreate = true
				default:
					return opts, fmt.Errorf("unsupported touch flag -%c", flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, errors.New("usage: bdy cmd touch [-c] <path...>")
	}
	return opts, nil
}

type cmdCatOptions struct {
	Number bool
	Paths  []string
}

func parseCatArgs(args []string) (cmdCatOptions, error) {
	opts := cmdCatOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'n':
					opts.Number = true
				default:
					return opts, fmt.Errorf("unsupported cat flag -%c", flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, errors.New("usage: bdy cmd cat [-n] <path...>")
	}
	return opts, nil
}

type cmdLSOptions struct {
	All  bool
	Long bool
}

func parseLSArgs(command string, args []string) (cmdLSOptions, string, error) {
	opts := cmdLSOptions{}
	switch command {
	case "ll":
		opts.Long = true
	case "la":
		opts.All = true
	}
	path := "."
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'a':
					opts.All = true
				case 'l':
					opts.Long = true
				default:
					return opts, "", fmt.Errorf("unsupported ls flag -%c", flag)
				}
			}
			continue
		}
		path = arg
	}
	return opts, path, nil
}

func printCmdEntries(out io.Writer, items []baidu.FileEntry, opts cmdLSOptions) {
	for _, item := range items {
		name := item.ServerFilename
		if name == "" {
			name = filepath.Base(item.Path)
		}
		if !opts.All && strings.HasPrefix(name, ".") {
			continue
		}
		if opts.Long {
			kind := "file"
			if item.IsDir == 1 {
				kind = "dir "
			}
			fmt.Fprintf(out, "%s %10d %s\n", kind, item.Size, item.Path)
			continue
		}
		fmt.Fprintln(out, name)
	}
}

func cmdFind(ctx context.Context, client baidu.Client, out io.Writer, opts cmdSearchOptions, resolve func(string) string) error {
	pattern := opts.Pattern
	if opts.IgnoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	items, err := client.ListAll(ctx, resolve(opts.Path))
	if err != nil {
		return err
	}
	for _, item := range items {
		if opts.Type == "f" && item.IsDir == 1 {
			continue
		}
		if opts.Type == "d" && item.IsDir == 0 {
			continue
		}
		matched := re.MatchString(item.Path) || re.MatchString(item.ServerFilename)
		if opts.Invert {
			matched = !matched
		}
		if matched {
			kind := "file"
			if item.IsDir == 1 {
				kind = "dir "
			}
			fmt.Fprintf(out, "%s %10d %s\n", kind, item.Size, item.Path)
		}
	}
	return nil
}

func cmdCat(ctx context.Context, client baidu.Client, out io.Writer, remotePath string, number bool) error {
	entry, err := findRemoteEntry(ctx, client, remotePath)
	if err != nil {
		return err
	}
	if entry.IsDir == 1 {
		return fmt.Errorf("%s is a directory", remotePath)
	}
	meta, err := client.FileMetas(ctx, []uint64{entry.FSID}, true)
	if err != nil {
		return err
	}
	if len(meta) == 0 || meta[0].DLink == "" {
		return fmt.Errorf("missing dlink for %s", remotePath)
	}
	tmp, err := os.CreateTemp("", "bdy-cmd-cat-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := client.Download(ctx, meta[0].DLink, tmpPath); err != nil {
		return err
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if number {
		return writeNumbered(out, f)
	}
	_, err = io.Copy(out, f)
	return err
}

func writeNumbered(out io.Writer, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		fmt.Fprintf(out, "%6d\t%s", i+1, line)
	}
	return nil
}

func cmdTouch(ctx context.Context, client baidu.Client, root, remotePath string) error {
	if err := ensureRemoteDir(ctx, client, root, filepath.ToSlash(filepath.Dir(remotePath))); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "bdy-cmd-touch-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	return client.UploadFile(ctx, tmpPath, remotePath)
}

func cmdVim(ctx context.Context, client baidu.Client, out io.Writer, root, remotePath string) error {
	if err := ensureRemoteDir(ctx, client, root, filepath.ToSlash(filepath.Dir(remotePath))); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "bdy-cmd-vim-"+filepath.Base(remotePath)+"-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)

	if entry, err := findRemoteEntry(ctx, client, remotePath); err == nil && entry.IsDir == 0 {
		meta, err := client.FileMetas(ctx, []uint64{entry.FSID}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return fmt.Errorf("missing dlink for %s", remotePath)
		}
		if err := client.Download(ctx, meta[0].DLink, tmpPath); err != nil {
			return err
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if err := client.UploadFile(ctx, tmpPath, remotePath); err != nil {
		return err
	}
	fmt.Fprintf(out, "saved %s\n", remotePath)
	return nil
}

func ensureRemoteDir(ctx context.Context, client baidu.Client, root, remotePath string) error {
	remotePath = strings.TrimRight(remotePath, "/")
	if remotePath == "" || remotePath == "." || remotePath == "/" {
		return nil
	}
	root = strings.TrimRight(root, "/")
	if root == "" {
		root = "/"
	}
	if root != "/" && remotePath != root && !strings.HasPrefix(remotePath, root+"/") {
		return fmt.Errorf("path must be under %s", root)
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(remotePath, root), "/")
	current := root
	if current != "/" {
		_ = client.Mkdir(ctx, current)
	}
	if rel == "" {
		return nil
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" {
			continue
		}
		if current == "/" {
			current = "/" + part
		} else {
			current = strings.TrimRight(current, "/") + "/" + part
		}
		_ = client.Mkdir(ctx, current)
	}
	return nil
}

func findRemoteEntry(ctx context.Context, client baidu.Client, remotePath string) (baidu.FileEntry, error) {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	if parent == "." {
		parent = repo.CmdRoot
	}
	name := filepath.Base(remotePath)
	items, err := client.List(ctx, parent)
	if err != nil {
		return baidu.FileEntry{}, err
	}
	for _, item := range items {
		if item.Path == remotePath || item.ServerFilename == name {
			return item, nil
		}
	}
	return baidu.FileEntry{}, fmt.Errorf("not found: %s", remotePath)
}

func cmdPath(p string) string {
	p = strings.TrimSpace(p)
	base := cmdBasePath()
	if p == "" || p == "." || p == "/" {
		if p == "/" {
			return repo.CmdRoot
		}
		return base
	}
	if strings.HasPrefix(p, repo.CmdRoot+"/") || p == repo.CmdRoot {
		return cleanCmdAbsolute(p)
	}
	if strings.HasPrefix(p, "/") {
		return cleanCmdAbsolute(strings.TrimRight(repo.CmdRoot, "/") + "/" + strings.TrimPrefix(repo.CleanPath(p), "/"))
	}
	return cleanCmdAbsolute(strings.TrimRight(base, "/") + "/" + repo.CleanPath(p))
}

func cmdBasePath() string {
	raw := strings.TrimSpace(os.Getenv(cmdCWDEnv))
	if raw == "" {
		return repo.CmdRoot
	}
	return cmdPathFromRoot(raw)
}

func cmdPathFromRoot(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "." || p == "/" {
		return repo.CmdRoot
	}
	if strings.HasPrefix(p, repo.CmdRoot+"/") || p == repo.CmdRoot {
		return cleanCmdAbsolute(p)
	}
	p = strings.TrimPrefix(repo.CleanPath(p), "/")
	return cleanCmdAbsolute(strings.TrimRight(repo.CmdRoot, "/") + "/" + p)
}

func cleanCmdAbsolute(p string) string {
	cleaned := "/" + strings.TrimPrefix(repo.CleanPath(p), "/")
	root := strings.TrimRight(repo.CmdRoot, "/")
	if cleaned != root && !strings.HasPrefix(cleaned, root+"/") {
		return repo.CmdRoot
	}
	return cleaned
}

func shellQuote(s string) string {
	return strconv.Quote(s)
}

func recordCmdHistory(command string, args []string) {
	path, err := cmdHistoryPath()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	entry := cmdHistoryEntry{Time: time.Now(), Command: command, Args: args}
	data, err := json.Marshal(entry)
	if err == nil {
		_, _ = f.Write(append(data, '\n'))
	}
}

func cmdHistory(args []string, out io.Writer) error {
	limit := 0
	clear := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-c", "--clear":
			clear = true
		case "-n":
			i++
			if i >= len(args) {
				return errors.New("usage: bdy cmd history [-c] [-n N]")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return errors.New("history -n requires a non-negative integer")
			}
			limit = n
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unsupported history flag %s", arg)
			}
			n, err := strconv.Atoi(arg)
			if err != nil || n < 0 {
				return errors.New("usage: bdy cmd history [-c] [-n N]")
			}
			limit = n
		}
	}
	path, err := cmdHistoryPath()
	if err != nil {
		return err
	}
	if clear {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry cmdHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		fmt.Fprintf(out, "%s cmd %s %s\n", entry.Time.Format(time.RFC3339), entry.Command, strings.Join(entry.Args, " "))
	}
	return nil
}

func cmdHistoryPath() (string, error) {
	base, err := config.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(base), "cmd_history.jsonl"), nil
}
