package bdynd

import (
	"context"
	"strings"
)

type PruneRemoteStore interface {
	RemoteStore
	ListFiles(ctx context.Context, remoteRoot string) ([]string, error)
	DeleteFiles(ctx context.Context, remotePaths []string) error
}

func PushPrune(ctx context.Context, r Repo, remote PruneRemoteStore, remoteName string) error {
	if err := Push(ctx, r, remote, remoteName); err != nil {
		return err
	}
	remoteRoot, err := GetRemote(r, remoteName)
	if err != nil {
		return err
	}
	keep, err := ReachableRemoteObjectPaths(r, remoteRoot)
	if err != nil {
		return err
	}
	paths, err := remote.ListFiles(ctx, remoteRoot)
	if err != nil {
		return err
	}
	var remove []string
	for _, path := range paths {
		if !isPrunableObjectPath(remoteRoot, path) {
			continue
		}
		if !keep[path] {
			remove = append(remove, path)
		}
	}
	if len(remove) == 0 {
		return nil
	}
	return remote.DeleteFiles(ctx, remove)
}

func ReachableRemoteObjectPaths(r Repo, remoteRoot string) (map[string]bool, error) {
	head, err := HeadCommit(r)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for oid := head; oid != ""; {
		c, err := ReadCommit(r, oid)
		if err != nil {
			return nil, err
		}
		keep[RemoteCommitPath(remoteRoot, oid)] = true
		keep[RemoteTreePath(remoteRoot, c.Tree)] = true
		for _, entry := range c.Entries {
			switch entry.Kind {
			case KindBlob:
				keep[RemoteBlobPath(remoteRoot, entry.OID)] = true
			case KindLFS:
				keep[RemoteLFSObjectPath(remoteRoot, entry.LFSOID)] = true
			}
		}
		oid = c.Parent
	}
	return keep, nil
}

func isPrunableObjectPath(remoteRoot, path string) bool {
	root := strings.TrimRight(remoteRoot, "/") + "/"
	if !strings.HasPrefix(path, root) {
		return false
	}
	rel := strings.TrimPrefix(path, root)
	return strings.HasPrefix(rel, "objects/") || strings.HasPrefix(rel, "lfs/objects/")
}
