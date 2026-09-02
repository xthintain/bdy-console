package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/bdynd"
)

func cmdND(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy nd init|status|add|commit|log|show|branch|switch|checkout|tag|lfs|remote|push|fetch|pull|clone|merge|rebase|cherry-pick|stash|diff|rm|mv|restore|reset|clean|grep|pack|index|search|ignore")
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
		if len(args) == 3 && args[1] == "-d" {
			if err := bdynd.DeleteBranch(r, args[2]); err != nil {
				return err
			}
			fmt.Fprintf(out, "deleted branch %s\n", args[2])
			return nil
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
		if len(args) == 3 && args[1] == "-d" {
			r, err := bdynd.Open(".")
			if err != nil {
				return err
			}
			if err := bdynd.DeleteTag(r, args[2]); err != nil {
				return err
			}
			fmt.Fprintf(out, "deleted tag %s\n", args[2])
			return nil
		}
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: bdy nd tag [-d <name>] | <name> [ref]")
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
		prunePush := false
		if args[0] == "push" {
			fs := flag.NewFlagSet("nd push", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			prune := fs.Bool("prune", false, "")
			if err := fs.Parse(args[1:]); err != nil {
				return err
			}
			if fs.NArg() != 0 {
				return errors.New("usage: bdy nd push [--prune]")
			}
			prunePush = *prune
		}
		forcePull := false
		if args[0] == "pull" {
			fs := flag.NewFlagSet("nd pull", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			force := fs.Bool("force", false, "")
			if err := fs.Parse(args[1:]); err != nil {
				return err
			}
			if fs.NArg() != 0 {
				return errors.New("usage: bdy nd pull [--force]")
			}
			forcePull = *force
		} else if args[0] != "push" && len(args) != 1 {
			return fmt.Errorf("usage: bdy nd %s", args[0])
		}
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
			if prunePush {
				pruneStore, ok := store.(bdynd.PruneRemoteStore)
				if !ok {
					return errors.New("remote does not support prune")
				}
				if err := bdynd.PushPrune(ctx, r, pruneStore, bdynd.DefaultRemote); err != nil {
					return err
				}
				fmt.Fprintln(out, "push prune complete")
				return nil
			}
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
			if forcePull {
				if err := bdynd.ForcePull(ctx, r, store, bdynd.DefaultRemote); err != nil {
					return err
				}
				fmt.Fprintln(out, "force pull complete")
				return nil
			}
			if err := bdynd.Pull(ctx, r, store, bdynd.DefaultRemote); err != nil {
				return err
			}
			fmt.Fprintln(out, "pull complete")
		}
		return nil
	case "clone":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: bdy nd clone <remote> [dir]")
		}
		cfg, err := auth.EnsureToken(ctx)
		if err != nil {
			return err
		}
		dest := ""
		if len(args) == 3 {
			dest = args[2]
		}
		store := progressRemoteStore{
			base:   ndRemoteStore{client: baidu.NewClient(cfg)},
			out:    out,
			prefix: "clone",
		}
		fmt.Fprintf(out, "clone: fetching %s\n", args[1])
		r, err := bdynd.Clone(ctx, store, args[1], dest)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "cloned %s into %s\n", args[1], r.Root)
		return nil
	case "merge":
		if len(args) != 2 {
			return errors.New("usage: bdy nd merge <branch>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		result, err := bdynd.Merge(r, args[1])
		if len(result.Conflicts) > 0 {
			fmt.Fprintf(out, "conflicts: %s\n", strings.Join(result.Conflicts, ", "))
		} else if result.Kind == bdynd.MergeFastForward {
			fmt.Fprintln(out, "fast-forward")
		} else if result.Kind == bdynd.MergeMerged {
			fmt.Fprintf(out, "merged %s\n", args[1])
		}
		return err
	case "rebase":
		if len(args) != 2 {
			return errors.New("usage: bdy nd rebase <upstream>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		result, err := bdynd.Rebase(r, args[1])
		if len(result.Conflicts) > 0 {
			fmt.Fprintf(out, "conflicts: %s\n", strings.Join(result.Conflicts, ", "))
		} else {
			fmt.Fprintf(out, "rebased %d commit(s) onto %s\n", result.Replayed, args[1])
		}
		return err
	case "cherry-pick":
		if len(args) != 2 {
			return errors.New("usage: bdy nd cherry-pick <ref>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		result, err := bdynd.CherryPick(r, args[1])
		if len(result.Conflicts) > 0 {
			fmt.Fprintf(out, "conflicts: %s\n", strings.Join(result.Conflicts, ", "))
		} else {
			fmt.Fprintf(out, "cherry-picked %s\n", shortOID(result.Commit))
		}
		return err
	case "stash":
		return cmdNDStash(args[1:], out)
	case "clean":
		return cmdNDClean(args[1:], out)
	case "grep":
		return cmdNDGrep(args[1:], out)
	case "diff":
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		diff, err := bdynd.Diff(r)
		if err != nil {
			return err
		}
		printNDStatus(out, diff)
		return nil
	case "rm":
		fs := flag.NewFlagSet("nd rm", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		cached := fs.Bool("cached", false, "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return errors.New("usage: bdy nd rm [--cached] <path...>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		if *cached {
			if err := bdynd.RemoveCached(r, fs.Args()); err != nil {
				return err
			}
			fmt.Fprintf(out, "unstaged %d path(s)\n", fs.NArg())
			return nil
		}
		if err := bdynd.Remove(r, fs.Args()); err != nil {
			return err
		}
		fmt.Fprintf(out, "removed %d path(s)\n", fs.NArg())
		return nil
	case "mv":
		if len(args) != 3 {
			return errors.New("usage: bdy nd mv <old> <new>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		if err := bdynd.Move(r, args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintf(out, "moved %s -> %s\n", args[1], args[2])
		return nil
	case "restore":
		if len(args) < 2 {
			return errors.New("usage: bdy nd restore <path...>")
		}
		r, err := bdynd.Open(".")
		if err != nil {
			return err
		}
		if err := bdynd.Restore(r, args[1:]); err != nil {
			return err
		}
		fmt.Fprintf(out, "restored %d path(s)\n", len(args)-1)
		return nil
	case "reset":
		return cmdNDReset(args[1:], out)
	case "pack":
		return cmdNDPack(ctx, args[1:], out)
	case "index":
		return cmdNDIndex(args[1:], out)
	case "search":
		return cmdNDSearch(args[1:], out)
	case "ignore":
		return cmdNDIgnore(args[1:], out)
	default:
		return errors.New("usage: bdy nd init|status|add|commit|log|show|branch|switch|checkout|tag|lfs|remote|push|fetch|pull|clone|merge|rebase|cherry-pick|stash|diff|rm|mv|restore|reset|clean|grep|pack|index|search|ignore")
	}
}
