package bdynd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const IgnoreFileName = ".bdyndignore"

type IgnoreMatcher struct {
	patterns []ignorePattern
}

type ignorePattern struct {
	raw       string
	rooted    bool
	directory bool
}

func LoadIgnore(r Repo) (IgnoreMatcher, error) {
	f, err := os.Open(filepath.Join(r.Root, IgnoreFileName))
	if os.IsNotExist(err) {
		return IgnoreMatcher{}, nil
	}
	if err != nil {
		return IgnoreMatcher{}, err
	}
	defer f.Close()
	var matcher IgnoreMatcher
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := ignorePattern{raw: filepath.ToSlash(line)}
		if strings.HasPrefix(p.raw, "/") {
			p.rooted = true
			p.raw = strings.TrimPrefix(p.raw, "/")
		}
		if strings.HasSuffix(p.raw, "/") {
			p.directory = true
			p.raw = strings.TrimSuffix(p.raw, "/")
		}
		if p.raw != "" {
			matcher.patterns = append(matcher.patterns, p)
		}
	}
	return matcher, scanner.Err()
}

func (m IgnoreMatcher) Ignored(rel string, isDir bool) bool {
	rel = cleanWorktreePath(rel)
	if rel == "" {
		return false
	}
	if rel == IgnoreFileName || strings.HasPrefix(rel, DirName+"/") || rel == DirName {
		return true
	}
	for _, p := range m.patterns {
		if p.matches(rel, isDir) {
			return true
		}
	}
	return false
}

func (p ignorePattern) matches(rel string, isDir bool) bool {
	if p.directory {
		if rel == p.raw || strings.HasPrefix(rel, p.raw+"/") {
			return true
		}
		if !p.rooted && strings.Contains(rel, "/"+p.raw+"/") {
			return true
		}
		return false
	}
	if p.rooted {
		return pathMatch(p.raw, rel)
	}
	if pathMatch(p.raw, rel) || pathMatch(p.raw, filepath.Base(rel)) {
		return true
	}
	return false
}

func pathMatch(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	if err == nil && ok {
		return true
	}
	return pattern == name
}
