package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/bdynd"
)

func cmdND(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy nd init|status|add|commit|log|show|branch|switch|checkout|tag|lfs|remote|push|fetch|pull|diff|rm|mv|restore|reset")
	}
	switch args[0] {
	case "init":
		r, err := bdynd.Init(".", bdynd.InitOptions{})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "initialized bdy nd repo in %s\n", r.Dir)
		return nil
	case "add":
		if len(args) < 2 {
			return errors.New("usage: bdy nd add <path...>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		if err := bdynd.Add(r, args[1:]); err != nil {
			return err
		}
		fmt.Fprintf(out, "staged %d path(s)\n", len(args)-1)
		return nil
	case "commit":
		fs := flag.NewFlagSet("nd commit", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		msg := fs.String("m", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		c, err := bdynd.Commit(r, bdynd.CommitOptions{Message: *msg})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "[%s] %s\n", shortOID(c.OID), c.Message)
		return nil
	case "log":
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		commits, err := bdynd.Log(r, bdynd.LogOptions{Limit: 50})
		if err != nil {
			return err
		}
		for _, c := range commits {
			fmt.Fprintf(out, "commit %s\nDate: %s\n\n    %s\n\n", c.OID, c.Time.Format("2006-01-02 15:04:05 -0700"), c.Message)
		}
		return nil
	case "status":
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		st, err := bdynd.Status(r)
		if err != nil {
			return err
		}
		printNDStatus(out, st)
		return nil
	case "show":
		if len(args) != 2 {
			return errors.New("usage: bdy nd show <commit>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		c, err := bdynd.ReadCommit(r, args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "commit %s\nTree: %s\nParent: %s\nDate: %s\n\n    %s\n", c.OID, c.Tree, c.Parent, c.Time.Format("2006-01-02 15:04:05 -0700"), c.Message)
		return nil
	case "branch":
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		if len(args) == 1 {
			branches, err := bdynd.ListBranches(r)
			if err != nil {
				return err
			}
			for _, branch := range branches {
				mark := " "
				if branch.Current {
					mark = "*"
				}
				fmt.Fprintf(out, "%s %s\n", mark, branch.Name)
			}
			return nil
		}
		head, err := bdynd.HeadCommit(r)
		if err != nil {
			return err
		}
		if err := bdynd.CreateBranch(r, args[1], head); err != nil {
			return err
		}
		fmt.Fprintf(out, "created branch %s\n", args[1])
		return nil
	case "switch":
		if len(args) != 2 {
			return errors.New("usage: bdy nd switch <branch>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		if err := bdynd.Switch(r, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(out, "switched to %s\n", args[1])
		return nil
	case "checkout":
		if len(args) != 2 {
			return errors.New("usage: bdy nd checkout <ref>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		if err := bdynd.Checkout(r, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(out, "checked out %s\n", args[1])
		return nil
	case "tag":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: bdy nd tag <name> [ref]")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		oid := ""
		if len(args) == 3 {
			oid, err = bdynd.ResolveRef(r, args[2])
			if err != nil {
				return err
			}
		}
		if err := bdynd.CreateTag(r, args[1], oid); err != nil {
			return err
		}
		fmt.Fprintf(out, "created tag %s\n", args[1])
		return nil
	case "lfs":
		return cmdNDLFS(ctx, args[1:], out)
	case "remote":
		return cmdNDRemote(args[1:], out)
	case "push", "fetch", "pull":
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		store, _, err := ndBaiduRemote(ctx, r)
		if err != nil {
			return err
		}
		switch args[0] {
		case "push":
			if err := bdynd.Push(ctx, r, store, bdynd.DefaultRemote); err != nil {
				return err
			}
			fmt.Fprintln(out, "push complete")
		case "fetch":
			if err := bdynd.Fetch(ctx, r, store, bdynd.DefaultRemote); err != nil {
				return err
			}
			fmt.Fprintln(out, "fetch complete")
		case "pull":
			if err := bdynd.Pull(ctx, r, store, bdynd.DefaultRemote); err != nil {
				return err
			}
			fmt.Fprintln(out, "pull complete")
		}
		return nil
	case "diff", "rm", "mv", "restore", "reset":
		return fmt.Errorf("bdy nd %s is planned but not implemented yet", args[0])
	default:
		return errors.New("usage: bdy nd init|status|add|commit|log|show|branch|switch|checkout|tag|lfs|remote|push|fetch|pull|diff|rm|mv|restore|reset")
	}
}

func cmdNDRemote(args []string, out io.Writer) error {
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	if len(args) == 0 {
		for name, root := range r.Config.Remotes {
			fmt.Fprintf(out, "%s %s\n", name, root)
		}
		return nil
	}
	switch args[0] {
	case "set-url":
		if len(args) != 3 {
			return errors.New("usage: bdy nd remote set-url <name> <remote-root>")
		}
		if err := bdynd.SetRemote(r, args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s %s\n", args[1], strings.TrimRight(args[2], "/"))
		return nil
	default:
		return errors.New("usage: bdy nd remote [set-url <name> <remote-root>]")
	}
}

func cmdNDLFS(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy nd lfs track|untrack|status|ls-files|push|fetch|checkout|pull")
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	switch args[0] {
	case "track":
		if len(args) < 2 {
			return errors.New("usage: bdy nd lfs track <pattern...>")
		}
		for _, pattern := range args[1:] {
			if err := bdynd.TrackPattern(r, pattern); err != nil {
				return err
			}
			fmt.Fprintf(out, "tracking %s\n", pattern)
		}
		return nil
	case "untrack":
		if len(args) < 2 {
			return errors.New("usage: bdy nd lfs untrack <pattern...>")
		}
		for _, pattern := range args[1:] {
			if err := bdynd.UntrackPattern(r, pattern); err != nil {
				return err
			}
			fmt.Fprintf(out, "untracked %s\n", pattern)
		}
		return nil
	case "status", "ls-files":
		files, err := bdynd.LFSFiles(r)
		if err != nil {
			return err
		}
		for _, file := range files {
			fmt.Fprintf(out, "%s %s %d\n", file.LFSOID, file.Path, file.Size)
		}
		return nil
	case "checkout":
		if err := bdynd.CheckoutLFS(r); err != nil {
			return err
		}
		fmt.Fprintln(out, "lfs checkout complete")
		return nil
	case "push", "fetch", "pull":
		store, remoteRoot, err := ndBaiduRemote(ctx, r)
		if err != nil {
			return err
		}
		switch args[0] {
		case "push":
			if err := bdynd.PushLFS(ctx, r, store, remoteRoot); err != nil {
				return err
			}
			fmt.Fprintln(out, "lfs push complete")
		case "fetch":
			if err := bdynd.FetchLFS(ctx, r, store, remoteRoot); err != nil {
				return err
			}
			fmt.Fprintln(out, "lfs fetch complete")
		case "pull":
			if err := bdynd.PullLFS(ctx, r, store, remoteRoot); err != nil {
				return err
			}
			fmt.Fprintln(out, "lfs pull complete")
		}
		return nil
	default:
		return errors.New("usage: bdy nd lfs track|untrack|status|ls-files|push|fetch|checkout|pull")
	}
}

type ndRemoteStore struct {
	client baidu.Client
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

func printNDHelp(out io.Writer) {
	fmt.Fprintln(out, `bdy nd - Git-like NetDisk version storage

Usage:
  bdy nd init
  bdy nd status
  bdy nd add <path...>
  bdy nd commit -m <message>
  bdy nd log
  bdy nd show <commit>
  bdy nd branch [name]
  bdy nd switch <branch>
  bdy nd checkout <ref>
  bdy nd tag <name> [ref]
  bdy nd remote [set-url <name> <remote-root>]
  bdy nd push
  bdy nd fetch
  bdy nd pull
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

Commands such as lfs push/pull, clone, merge, and stash are planned in the bdy
nd implementation plan.`)
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
