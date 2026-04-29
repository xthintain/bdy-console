package lfs

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func InstallGitFilters(root, executable string) error {
	if executable == "" {
		executable = "bdy"
	}
	cmds := [][]string{
		{"git", "config", "filter.bdy-lfs.clean", executable + " lfs clean -- %f"},
		{"git", "config", "filter.bdy-lfs.smudge", executable + " lfs smudge -- %f"},
		{"git", "config", "filter.bdy-lfs.required", "true"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func Track(root string, patterns []string) error {
	if len(patterns) == 0 {
		return fmt.Errorf("no patterns provided")
	}
	path := filepath.Join(root, ".gitattributes")
	existing, _ := os.ReadFile(path)
	text := string(existing)
	for _, pattern := range patterns {
		line := pattern + " filter=bdy-lfs diff=bdy-lfs merge=bdy-lfs -text"
		if !strings.Contains(text, line) {
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			text += line + "\n"
		}
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func Untrack(root string, patterns []string) error {
	path := filepath.Join(root, ".gitattributes")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	remove := map[string]bool{}
	for _, p := range patterns {
		remove[p] = true
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && remove[fields[0]] && strings.Contains(line, "filter=bdy-lfs") {
			continue
		}
		if line != "" {
			kept = append(kept, line)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}

func PointerFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == repoDirName() {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := IsPointerFile(path); ok {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

type TrackedPointer struct {
	Path    string
	Pointer Pointer
}

func TrackedPointers(root string) ([]TrackedPointer, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	raw, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out []TrackedPointer
	for _, part := range bytes.Split(raw, []byte{0}) {
		if len(part) == 0 {
			continue
		}
		path := string(part)
		show := exec.Command("git", "show", ":"+path)
		show.Dir = root
		data, err := show.Output()
		if err != nil {
			continue
		}
		p, err := ParsePointer(bytes.NewReader(data))
		if err == nil {
			out = append(out, TrackedPointer{Path: path, Pointer: p})
		}
	}
	return out, nil
}

func repoDirName() string {
	return ".bdy"
}
