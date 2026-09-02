package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"baiduyunStorage/internal/config"
	"baiduyunStorage/internal/repo"
)

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

func resolveCloudPath(root, cwd, p string) string {
	p = strings.TrimSpace(p)
	if root == "" {
		root = "/"
	}
	if cwd == "" {
		cwd = root
	}
	if p == "" || p == "." {
		return cleanCloudPath(root, cwd)
	}
	if strings.HasPrefix(p, "/") {
		return cleanCloudPath(root, p)
	}
	return cleanCloudPath(root, strings.TrimRight(cwd, "/")+"/"+repo.CleanPath(p))
}

func cleanCloudPath(root, p string) string {
	cleaned := "/" + strings.TrimPrefix(repo.CleanPath(p), "/")
	if root == "/" {
		return cleaned
	}
	root = strings.TrimRight(root, "/")
	if cleaned == root || strings.HasPrefix(cleaned, root+"/") {
		return cleaned
	}
	return root
}

func cmdBasePath() string {
	raw := strings.TrimSpace(os.Getenv(cmdCWDEnv))
	if raw == "" {
		raw = loadCmdSessionPath()
	}
	if raw == "" {
		return repo.CmdRoot
	}
	return cmdPathFromRoot(raw)
}

func saveCmdSessionPath(path string) error {
	sessionPath, err := cmdSessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(sessionPath, []byte(cmdPathFromRoot(path)+"\n"), 0o600)
}

func loadCmdSessionPath() string {
	sessionPath, err := cmdSessionPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cmdSessionPath() (string, error) {
	dir := os.Getenv(cmdSessionDirEnv)
	if dir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(cacheDir, "bdy", "cmd-cwd")
	}
	return filepath.Join(dir, fmt.Sprintf("%d-%d", os.Getuid(), os.Getppid())), nil
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
