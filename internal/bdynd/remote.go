package bdynd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RemoteStore interface {
	UploadFile(ctx context.Context, localPath, remotePath string) error
	DownloadFile(ctx context.Context, remotePath, localPath string) error
	Exists(ctx context.Context, remotePath string) (bool, error)
}

func RemoteLFSObjectPath(remoteRoot, oid string) string {
	hash := strings.TrimPrefix(oid, oidPrefixSHA256)
	prefix := hash
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return strings.TrimRight(remoteRoot, "/") + "/lfs/objects/sha256/" + prefix + "/" + hash
}

func SetRemote(r Repo, name, remoteRoot string) error {
	name = strings.TrimSpace(name)
	remoteRoot = strings.TrimRight(strings.TrimSpace(remoteRoot), "/")
	if name == "" || remoteRoot == "" {
		return fmt.Errorf("remote name and root are required")
	}
	cfg := r.Config
	if cfg.DefaultBranch == "" {
		cfg.DefaultBranch = DefaultBranch
	}
	if cfg.Remotes == nil {
		cfg.Remotes = map[string]string{}
	}
	cfg.Remotes[name] = remoteRoot
	return writeJSON(r.ConfigPath(), cfg)
}

func GetRemote(r Repo, name string) (string, error) {
	if name == "" {
		name = DefaultRemote
	}
	remoteRoot := r.Config.Remotes[name]
	if remoteRoot == "" {
		return "", fmt.Errorf("remote %q not configured", name)
	}
	return strings.TrimRight(remoteRoot, "/"), nil
}

func Push(ctx context.Context, r Repo, remote RemoteStore, remoteName string) error {
	remoteRoot, err := GetRemote(r, remoteName)
	if err != nil {
		return err
	}
	branch, err := CurrentBranch(r)
	if err != nil {
		return err
	}
	head, err := HeadCommit(r)
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) == "" {
		return fmt.Errorf("nothing to push; create a commit first")
	}
	if _, err := ReadCommit(r, head); err != nil {
		return fmt.Errorf("nothing to push; HEAD commit is missing: %w", err)
	}
	if err := pushReachable(ctx, r, remote, remoteRoot, head); err != nil {
		return err
	}
	if err := PushLFS(ctx, r, remote, remoteRoot); err != nil {
		return err
	}
	return uploadBytes(ctx, remote, []byte(head+"\n"), RemoteRefPath(remoteRoot, "refs/heads/"+branch))
}

func Fetch(ctx context.Context, r Repo, remote RemoteStore, remoteName string) error {
	remoteRoot, err := GetRemote(r, remoteName)
	if err != nil {
		return err
	}
	branch := r.Config.DefaultBranch
	if branch == "" {
		branch = DefaultBranch
	}
	remoteRef := RemoteRefPath(remoteRoot, "refs/heads/"+branch)
	tmp, err := os.CreateTemp(r.Dir, "fetch-ref-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := remote.DownloadFile(ctx, remoteRef, tmpPath); err != nil {
		return err
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return err
	}
	oid := strings.TrimSpace(string(data))
	if err := fetchReachable(ctx, r, remote, remoteRoot, oid); err != nil {
		return err
	}
	return UpdateRef(r, "refs/remotes/"+remoteName+"/"+branch, oid)
}

func Pull(ctx context.Context, r Repo, remote RemoteStore, remoteName string) error {
	if remoteName == "" {
		remoteName = DefaultRemote
	}
	if err := Fetch(ctx, r, remote, remoteName); err != nil {
		return err
	}
	branch := r.Config.DefaultBranch
	if branch == "" {
		branch = DefaultBranch
	}
	remoteOID, err := ResolveRef(r, "refs/remotes/"+remoteName+"/"+branch)
	if err != nil {
		return err
	}
	localOID, _ := HeadCommit(r)
	if localOID != "" && !isAncestor(r, localOID, remoteOID) {
		return fmt.Errorf("non-fast-forward pull; fetch and merge first")
	}
	c, err := ReadCommit(r, remoteOID)
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
	return UpdateHead(r, remoteOID)
}

func Clone(ctx context.Context, remote RemoteStore, remoteRoot, dest string) (Repo, error) {
	remoteRoot = strings.TrimRight(strings.TrimSpace(remoteRoot), "/")
	if remoteRoot == "" {
		return Repo{}, fmt.Errorf("remote root is required")
	}
	if dest == "" {
		dest = filepath.Base(remoteRoot)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return Repo{}, err
	}
	r, err := Init(dest, InitOptions{RemoteName: DefaultRemote, RemoteRoot: remoteRoot})
	if err != nil {
		return Repo{}, err
	}
	if err := Pull(ctx, r, remote, DefaultRemote); err != nil {
		return Repo{}, err
	}
	return r, nil
}

func RemoteRefPath(remoteRoot, ref string) string {
	return strings.TrimRight(remoteRoot, "/") + "/" + strings.TrimPrefix(ref, "/")
}

func RemoteCommitPath(remoteRoot, oid string) string {
	return remoteObjectPath(remoteRoot, "commits", oid)
}

func RemoteTreePath(remoteRoot, oid string) string {
	return remoteObjectPath(remoteRoot, "trees", oid)
}

func RemoteBlobPath(remoteRoot, oid string) string {
	return remoteObjectPath(remoteRoot, "blobs", oid)
}

func remoteObjectPath(remoteRoot, kind, oid string) string {
	hash := strings.TrimPrefix(oid, oidPrefixSHA256)
	prefix := hash
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return strings.TrimRight(remoteRoot, "/") + "/objects/" + kind + "/sha256/" + prefix + "/" + hash
}

func pushReachable(ctx context.Context, r Repo, remote RemoteStore, remoteRoot, oid string) error {
	for oid != "" {
		c, err := ReadCommit(r, oid)
		if err != nil {
			return err
		}
		if err := uploadLocalFile(ctx, remote, localObjectPath(r, "commits", oid), RemoteCommitPath(remoteRoot, oid)); err != nil {
			return err
		}
		if err := uploadLocalFile(ctx, remote, localObjectPath(r, "trees", c.Tree), RemoteTreePath(remoteRoot, c.Tree)); err != nil {
			return err
		}
		for _, entry := range c.Entries {
			if entry.Kind == KindBlob {
				if err := uploadLocalFile(ctx, remote, localObjectPath(r, "blobs", entry.OID), RemoteBlobPath(remoteRoot, entry.OID)); err != nil {
					return err
				}
			}
		}
		oid = c.Parent
	}
	return nil
}

func fetchReachable(ctx context.Context, r Repo, remote RemoteStore, remoteRoot, oid string) error {
	for oid != "" {
		if err := downloadRemoteFile(ctx, remote, RemoteCommitPath(remoteRoot, oid), localObjectPath(r, "commits", oid)); err != nil {
			return err
		}
		c, err := ReadCommit(r, oid)
		if err != nil {
			return err
		}
		if err := downloadRemoteFile(ctx, remote, RemoteTreePath(remoteRoot, c.Tree), localObjectPath(r, "trees", c.Tree)); err != nil {
			return err
		}
		for _, entry := range c.Entries {
			if entry.Kind == KindBlob {
				if err := downloadRemoteFile(ctx, remote, RemoteBlobPath(remoteRoot, entry.OID), localObjectPath(r, "blobs", entry.OID)); err != nil {
					return err
				}
			}
		}
		oid = c.Parent
	}
	return nil
}

func localObjectPath(r Repo, kind, oid string) string {
	path, _ := objectPath(r, kind, oid)
	return path
}

func uploadLocalFile(ctx context.Context, remote RemoteStore, localPath, remotePath string) error {
	exists, err := remote.Exists(ctx, remotePath)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return remote.UploadFile(ctx, localPath, remotePath)
}

func downloadRemoteFile(ctx context.Context, remote RemoteStore, remotePath, localPath string) error {
	if _, err := os.Stat(localPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	return remote.DownloadFile(ctx, remotePath, localPath)
}

func uploadBytes(ctx context.Context, remote RemoteStore, data []byte, remotePath string) error {
	tmp, err := os.CreateTemp("", "bdynd-remote-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	err = remote.UploadFile(ctx, tmpPath, remotePath)
	_ = os.Remove(tmpPath)
	return err
}

func isAncestor(r Repo, ancestor, oid string) bool {
	for oid != "" {
		if oid == ancestor {
			return true
		}
		c, err := ReadCommit(r, oid)
		if err != nil {
			return false
		}
		oid = c.Parent
	}
	return false
}

func PushLFS(ctx context.Context, r Repo, remote RemoteStore, remoteRoot string) error {
	files, err := LFSFiles(r)
	if err != nil {
		return err
	}
	for _, file := range files {
		localPath := LFSObjectPath(r, file.LFSOID)
		if _, err := os.Stat(localPath); err != nil {
			return fmt.Errorf("missing lfs object for %s: %w", file.Path, err)
		}
		remotePath := RemoteLFSObjectPath(remoteRoot, file.LFSOID)
		exists, err := remote.Exists(ctx, remotePath)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := remote.UploadFile(ctx, localPath, remotePath); err != nil {
			return err
		}
	}
	return nil
}

func FetchLFS(ctx context.Context, r Repo, remote RemoteStore, remoteRoot string) error {
	files, err := LFSFiles(r)
	if err != nil {
		return err
	}
	for _, file := range files {
		localPath := LFSObjectPath(r, file.LFSOID)
		if _, err := os.Stat(localPath); err == nil {
			continue
		}
		remotePath := RemoteLFSObjectPath(remoteRoot, file.LFSOID)
		tmp, err := os.CreateTemp(filepath.Join(r.Dir, "lfs"), "fetch-*")
		if err != nil {
			return err
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		if err := remote.DownloadFile(ctx, remotePath, tmpPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		p, err := StoreLFSFile(r, tmpPath)
		_ = os.Remove(tmpPath)
		if err != nil {
			return err
		}
		if p.OID != file.LFSOID || p.Size != file.Size {
			return fmt.Errorf("lfs object verification failed for %s", file.Path)
		}
	}
	return nil
}

func CheckoutLFS(r Repo) error {
	files, err := LFSFiles(r)
	if err != nil {
		return err
	}
	for _, file := range files {
		localPath := LFSObjectPath(r, file.LFSOID)
		data, err := os.ReadFile(localPath)
		if err != nil {
			return fmt.Errorf("missing cached lfs object for %s: %w", file.Path, err)
		}
		dest := filepath.Join(r.Root, file.Path)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func PullLFS(ctx context.Context, r Repo, remote RemoteStore, remoteRoot string) error {
	if err := FetchLFS(ctx, r, remote, remoteRoot); err != nil {
		return err
	}
	return CheckoutLFS(r)
}
