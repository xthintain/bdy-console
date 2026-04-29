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
