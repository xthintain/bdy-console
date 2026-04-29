package bdynd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Branch struct {
	Name    string
	OID     string
	Current bool
}

func HeadRef(r Repo) (string, error) {
	return headRef(r)
}

func CurrentBranch(r Repo) (string, error) {
	ref, err := HeadRef(r)
	if err != nil {
		return "", err
	}
	const prefix = "refs/heads/"
	if !strings.HasPrefix(ref, prefix) {
		return "", errors.New("HEAD is not on a branch")
	}
	return strings.TrimPrefix(ref, prefix), nil
}

func ResolveRef(r Repo, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "HEAD" {
		return HeadCommit(r)
	}
	if strings.HasPrefix(name, "HEAD~") {
		return resolveHeadAncestor(r, strings.TrimPrefix(name, "HEAD~"))
	}
	candidates := []string{
		name,
		"refs/heads/" + name,
		"refs/tags/" + name,
		"refs/remotes/" + name,
	}
	for _, candidate := range candidates {
		path := filepath.Join(r.Dir, candidate)
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}
	if strings.HasPrefix(name, oidPrefixSHA256) {
		if _, err := ReadCommit(r, name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("unknown ref %q", name)
}

func resolveHeadAncestor(r Repo, rawDepth string) (string, error) {
	var depth int
	if _, err := fmt.Sscanf(rawDepth, "%d", &depth); err != nil || depth < 0 {
		return "", fmt.Errorf("unknown ref %q", "HEAD~"+rawDepth)
	}
	oid, err := HeadCommit(r)
	if err != nil {
		return "", err
	}
	for i := 0; i < depth; i++ {
		c, err := ReadCommit(r, oid)
		if err != nil {
			return "", err
		}
		if c.Parent == "" {
			return "", fmt.Errorf("unknown ref %q", "HEAD~"+rawDepth)
		}
		oid = c.Parent
	}
	return oid, nil
}

func UpdateRef(r Repo, ref, oid string) error {
	path := filepath.Join(r.Dir, ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(oid)+"\n"), 0o644)
}

func CreateBranch(r Repo, name, oid string) error {
	if err := validateRefName(name); err != nil {
		return err
	}
	if oid == "" {
		var err error
		oid, err = HeadCommit(r)
		if err != nil {
			return err
		}
	}
	return UpdateRef(r, "refs/heads/"+name, oid)
}

func ListBranches(r Repo) ([]Branch, error) {
	current, _ := CurrentBranch(r)
	dir := filepath.Join(r.Dir, "refs", "heads")
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var branches []Branch
	for _, item := range items {
		if item.IsDir() {
			continue
		}
		name := item.Name()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		branches = append(branches, Branch{Name: name, OID: strings.TrimSpace(string(data)), Current: name == current})
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].Name < branches[j].Name })
	return branches, nil
}

func Switch(r Repo, branch string) error {
	if err := validateRefName(branch); err != nil {
		return err
	}
	oid, err := ResolveRef(r, "refs/heads/"+branch)
	if err != nil {
		return err
	}
	if oid != "" {
		c, err := ReadCommit(r, oid)
		if err != nil {
			return err
		}
		if err := CheckoutTree(r, c.Tree); err != nil {
			return err
		}
		idx := Index{Entries: map[string]IndexEntry{}}
		for _, entry := range c.Entries {
			idx.Entries[entry.Path] = entry
		}
		if err := SaveIndex(r, idx); err != nil {
			return err
		}
	}
	return os.WriteFile(r.HeadPath(), []byte("ref: refs/heads/"+branch+"\n"), 0o644)
}

func Checkout(r Repo, ref string) error {
	oid, err := ResolveRef(r, ref)
	if err != nil {
		return err
	}
	c, err := ReadCommit(r, oid)
	if err != nil {
		return err
	}
	return CheckoutTree(r, c.Tree)
}

func CreateTag(r Repo, name, oid string) error {
	if err := validateRefName(name); err != nil {
		return err
	}
	if oid == "" {
		var err error
		oid, err = HeadCommit(r)
		if err != nil {
			return err
		}
	}
	return UpdateRef(r, "refs/tags/"+name, oid)
}

func validateRefName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, " \t\n\r\\") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return fmt.Errorf("invalid ref name %q", name)
	}
	return nil
}
