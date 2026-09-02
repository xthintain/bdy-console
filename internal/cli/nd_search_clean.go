package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"baiduyunStorage/internal/bdynd"
)

func cmdNDClean(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("nd clean", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dryRun := fs.Bool("n", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: bdy nd clean [-n]")
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	removed, err := bdynd.Clean(r, bdynd.CleanOptions{DryRun: *dryRun})
	if err != nil {
		return err
	}
	for _, path := range removed {
		if *dryRun {
			fmt.Fprintf(out, "would remove %s\n", path)
		} else {
			fmt.Fprintf(out, "removed %s\n", path)
		}
	}
	return nil
}

func cmdNDGrep(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("nd grep", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ignoreCase := fs.Bool("i", false, "")
	regexShort := fs.Bool("E", false, "")
	regexLong := fs.Bool("regex", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: bdy nd grep [-i] [-E|--regex] <pattern>")
	}
	r, err := bdynd.Open(".")
	if err != nil {
		return err
	}
	results, err := bdynd.Grep(r, bdynd.GrepOptions{Pattern: fs.Arg(0), IgnoreCase: *ignoreCase, Regex: *regexShort || *regexLong})
	if err != nil {
		return err
	}
	for _, result := range results {
		fmt.Fprintf(out, "%s:%d:%s\n", result.Path, result.Line, result.Text)
	}
	return nil
}
