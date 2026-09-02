package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/bdynd"
)

type ndRemoteStore struct {
	client baidu.Client
}

type progressRemoteStore struct {
	base   ndRemoteStore
	out    io.Writer
	prefix string
}

func ndBaiduRemote(ctx context.Context, r bdynd.Repo) (bdynd.RemoteStore, string, error) {
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		return nil, "", err
	}
	remoteRoot := r.Config.Remotes[bdynd.DefaultRemote]
	if remoteRoot == "" {
		remoteRoot = "/apps/baiduyunStorage/nd/repos/" + filepath.Base(r.Root)
		_ = bdynd.SetRemote(r, bdynd.DefaultRemote, remoteRoot)
	}
	return ndRemoteStore{client: baidu.NewClient(cfg)}, remoteRoot, nil
}

func (s ndRemoteStore) UploadFile(ctx context.Context, localPath, remotePath string) error {
	return s.client.UploadFile(ctx, localPath, remotePath)
}

func (s ndRemoteStore) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	name := filepath.Base(remotePath)
	items, err := s.client.List(ctx, parent)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Path != remotePath && item.ServerFilename != name {
			continue
		}
		meta, err := s.client.FileMetas(ctx, []uint64{item.FSID}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return fmt.Errorf("missing dlink for %s", remotePath)
		}
		return s.client.Download(ctx, meta[0].DLink, localPath)
	}
	return os.ErrNotExist
}

func (s ndRemoteStore) Exists(ctx context.Context, remotePath string) (bool, error) {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	name := filepath.Base(remotePath)
	items, err := s.client.List(ctx, parent)
	if err != nil {
		return false, nil
	}
	for _, item := range items {
		if item.Path == remotePath || item.ServerFilename == name {
			return true, nil
		}
	}
	return false, nil
}

func (s ndRemoteStore) ListFiles(ctx context.Context, remoteRoot string) ([]string, error) {
	items, err := s.client.ListAll(ctx, remoteRoot)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item.IsDir == 0 {
			paths = append(paths, item.Path)
		}
	}
	return paths, nil
}

func (s ndRemoteStore) DeleteFiles(ctx context.Context, remotePaths []string) error {
	if len(remotePaths) == 0 {
		return nil
	}
	return s.client.FileManager(ctx, "delete", remotePaths)
}

func (s progressRemoteStore) UploadFile(ctx context.Context, localPath, remotePath string) error {
	return s.base.UploadFile(ctx, localPath, remotePath)
}

func (s progressRemoteStore) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	fmt.Fprintf(s.out, "%s: downloading %s\n", s.prefix, remotePath)
	if err := s.base.DownloadFile(ctx, remotePath, localPath); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "%s: downloaded %s\n", s.prefix, remotePath)
	return nil
}

func (s progressRemoteStore) Exists(ctx context.Context, remotePath string) (bool, error) {
	return s.base.Exists(ctx, remotePath)
}

func (s progressRemoteStore) ListFiles(ctx context.Context, remoteRoot string) ([]string, error) {
	return s.base.ListFiles(ctx, remoteRoot)
}

func (s progressRemoteStore) DeleteFiles(ctx context.Context, remotePaths []string) error {
	return s.base.DeleteFiles(ctx, remotePaths)
}

func printNDHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy nd - Git-like NetDisk version storage

Usage:
  bdy nd init
  bdy nd status
  bdy nd add <path...>
  bdy nd commit -m <message>
  bdy nd log
  bdy nd show <commit>
  bdy nd diff
  bdy nd grep [-i] [-E|--regex] <pattern>
  bdy nd clean [-n]
  bdy nd rm [--cached] <path...>
  bdy nd mv <old> <new>
  bdy nd restore <path...>
  bdy nd reset [--soft|--mixed|--hard] <ref>
  bdy nd pack [--name <name>] [ref]
  bdy nd pack push
  bdy nd pack fetch <id...>
  bdy nd index
  bdy nd search [--type <ext>] [--name <text>] [--since <date>] [--until <date>]
  bdy nd ignore apply
  bdy nd branch [-d name] | [name]
  bdy nd switch <branch>
  bdy nd checkout <ref>
  bdy nd tag [-d name] | <name> [ref]
  bdy nd remote [-v] [set-url <name> <remote-root>]
  bdy nd push [--prune]
  bdy nd fetch
  bdy nd pull [--force]
  bdy nd clone <remote> [dir]
  bdy nd merge <branch>
  bdy nd rebase <upstream>
  bdy nd cherry-pick <ref>
  bdy nd stash push [-m <message>]
  bdy nd stash list
  bdy nd stash pop [id]
  bdy nd lfs track <pattern...>
  bdy nd lfs untrack <pattern...>
  bdy nd lfs status
  bdy nd lfs ls-files
  bdy nd lfs push
  bdy nd lfs fetch
  bdy nd lfs checkout
  bdy nd lfs pull

Repository:
  .bdynd/

Remote object root:
  /apps/baiduyunStorage/nd/repos/<repo-name>

Commands such as blame and bisect are planned in the bdy nd implementation plan.`)
}

func printNDStatus(out io.Writer, st bdynd.StatusResult) {
	for _, p := range st.Added {
		fmt.Fprintf(out, "A  %s\n", p)
	}
	for _, p := range st.Modified {
		fmt.Fprintf(out, "M  %s\n", p)
	}
	for _, p := range st.Deleted {
		fmt.Fprintf(out, "D  %s\n", p)
	}
	if st.Clean() {
		fmt.Fprintln(out, "clean")
	}
}

func shortOID(oid string) string {
	oid = strings.TrimPrefix(oid, "sha256:")
	if len(oid) > 12 {
		return oid[:12]
	}
	return oid
}
