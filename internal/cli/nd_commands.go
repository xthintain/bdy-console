package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"baiduyunStorage/internal/bdynd"
)

func cmdNDIgnore(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "apply" {
		return errors.New("usage: bdy nd ignore apply")
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	removed, err := bdynd.ApplyIgnore(r)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %d ignored path(s)\n", len(removed))
	for _, path := range removed {
		fmt.Fprintf(out, "%s\n", path)
	}
	return nil
}

func cmdNDSearch(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("nd search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	typ := fs.String("type", "", "")
	name := fs.String("name", "", "")
	sinceRaw := fs.String("since", "", "")
	untilRaw := fs.String("until", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: bdy nd search [--type <ext>] [--name <text>] [--since <date>] [--until <date>]")
	}
	since, err := parseSearchTime(*sinceRaw)
	if err != nil {
		return err
	}
	until, err := parseSearchTime(*untilRaw)
	if err != nil {
		return err
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	results, err := bdynd.SearchPacks(r, bdynd.SearchOptions{Type: *typ, Name: *name, Since: since, Until: until})
	if err != nil {
		return err
	}
	for _, result := range results {
		packName := result.PackName
		if packName == "" {
			packName = "-"
		}
		fmt.Fprintf(out, "%s %s %s %s %d %s\n", result.CreatedAt.Format(time.RFC3339), result.PackID, packName, result.Kind, result.Size, result.Path)
	}
	return nil
}

func parseSearchTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q; use YYYY-MM-DD or RFC3339", raw)
}

func cmdNDPack(ctx context.Context, args []string, out io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "push":
			if len(args) != 1 {
				return errors.New("usage: bdy nd pack push")
			}
			r, err := bdynd.Open(".")
			if err != nil {
				return err
			}
			store, remoteRoot, err := ndBaiduRemote(ctx, r)
			if err != nil {
				return err
			}
			if err := bdynd.PushPacks(ctx, r, store, remoteRoot); err != nil {
				return err
			}
			fmt.Fprintln(out, "pack push complete")
			return nil
		case "fetch":
			if len(args) < 2 {
				return errors.New("usage: bdy nd pack fetch <id...>")
			}
			r, err := bdynd.Open(".")
			if err != nil {
				return err
			}
			store, remoteRoot, err := ndBaiduRemote(ctx, r)
			if err != nil {
				return err
			}
			if err := bdynd.FetchPacks(ctx, r, store, remoteRoot, args[1:]); err != nil {
				return err
			}
			fmt.Fprintln(out, "pack fetch complete")
			return nil
		}
	}
	fs := flag.NewFlagSet("nd pack", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("name", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("usage: bdy nd pack [--name <name>] [ref]")
	}
	ref := "HEAD"
	if fs.NArg() == 1 {
		ref = fs.Arg(0)
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	manifest, err := bdynd.Pack(r, bdynd.PackOptions{Ref: ref, Name: *name})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "packed %s %d object(s)\n", manifest.ID, len(manifest.Entries))
	return nil
}

func cmdNDIndex(args []string, out io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: bdy nd index")
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	packs, err := bdynd.ListPacks(r)
	if err != nil {
		return err
	}
	for _, pack := range packs {
		name := pack.Name
		if name == "" {
			name = "-"
		}
		for _, entry := range pack.Entries {
			fmt.Fprintf(out, "%s %s %s %s %d %d %d\n", pack.ID, name, entry.Kind, entry.Path, entry.Size, entry.Offset, entry.Length)
		}
	}
	return nil
}

func cmdNDReset(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("nd reset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	soft := fs.Bool("soft", false, "")
	mixed := fs.Bool("mixed", false, "")
	hard := fs.Bool("hard", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: bdy nd reset [--soft|--mixed|--hard] <ref>")
	}
	mode := bdynd.ResetMixed
	if *soft {
		mode = bdynd.ResetSoft
	}
	if *mixed {
		mode = bdynd.ResetMixed
	}
	if *hard {
		mode = bdynd.ResetHard
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	if err := bdynd.Reset(r, fs.Arg(0), mode); err != nil {
		return err
	}
	fmt.Fprintf(out, "reset %s to %s\n", mode, fs.Arg(0))
	return nil
}

func cmdNDStash(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy nd stash push|list|pop")
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	switch args[0] {
	case "push":
		fs := flag.NewFlagSet("nd stash push", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		msg := fs.String("m", "WIP", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		stash, err := bdynd.StashPush(r, *msg)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "stashed %s\n", stash.ID)
		return nil
	case "list":
		stashes, err := bdynd.StashList(r)
		if err != nil {
			return err
		}
		for _, stash := range stashes {
			fmt.Fprintf(out, "%s %s\n", stash.ID, stash.Message)
		}
		return nil
	case "pop":
		if len(args) > 2 {
			return errors.New("usage: bdy nd stash pop [id]")
		}
		id := ""
		if len(args) == 2 {
			id = args[1]
		}
		if err := bdynd.StashPop(r, id); err != nil {
			return err
		}
		fmt.Fprintln(out, "applied stash")
		return nil
	default:
		return errors.New("usage: bdy nd stash push|list|pop")
	}
}

func cmdNDRemote(args []string, out io.Writer) error {
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	if len(args) == 0 || (len(args) == 1 && args[0] == "-v") {
		for name, root := range r.Config.Remotes {
			if len(args) == 1 {
				fmt.Fprintf(out, "%s %s (fetch)\n%s %s (push)\n", name, root, name, root)
			} else {
				fmt.Fprintf(out, "%s %s\n", name, root)
			}
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
		return errors.New("usage: bdy nd remote [-v] [set-url <name> <remote-root>]")
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
