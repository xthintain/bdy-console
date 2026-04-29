package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"baiduyunStorage/internal/bdynd"
)

func cmdND(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy nd init|status|add|commit|log|show|diff|rm|mv|restore|reset")
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
	case "diff", "rm", "mv", "restore", "reset":
		return fmt.Errorf("bdy nd %s is planned but not implemented yet", args[0])
	default:
		return errors.New("usage: bdy nd init|status|add|commit|log|show|diff|rm|mv|restore|reset")
	}
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

Repository:
  .bdynd/

Remote object root:
  /apps/baiduyunStorage/nd/repos/<repo-name>

Commands such as branch, switch, lfs, push, pull, clone, merge, and stash are
planned in the bdy nd implementation plan.`)
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
