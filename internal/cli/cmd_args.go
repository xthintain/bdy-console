package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"baiduyunStorage/internal/baidu"
)

type cmdRMOptions struct {
	Force bool
	Paths []string
}

func parseRMArgs(command string, args []string) (cmdRMOptions, error) {
	opts := cmdRMOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'f':
					opts.Force = true
				case 'r', 'R':
				default:
					return opts, fmt.Errorf("unsupported %s flag -%c", command, flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, fmt.Errorf("usage: bdy cmd %s [-r] [-f] <path...>", command)
	}
	return opts, nil
}

type cmdMkdirOptions struct {
	Paths []string
}

func parseMkdirArgs(args []string) (cmdMkdirOptions, error) {
	opts := cmdMkdirOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'p', 'v':
				default:
					return opts, fmt.Errorf("unsupported mkdir flag -%c", flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, errors.New("usage: bdy cmd mkdir [-p] <path...>")
	}
	return opts, nil
}

type cmdTouchOptions struct {
	NoCreate bool
	Paths    []string
}

func parseTouchArgs(args []string) (cmdTouchOptions, error) {
	opts := cmdTouchOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'c':
					opts.NoCreate = true
				default:
					return opts, fmt.Errorf("unsupported touch flag -%c", flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, errors.New("usage: bdy cmd touch [-c] <path...>")
	}
	return opts, nil
}

type cmdCatOptions struct {
	Number bool
	Paths  []string
}

func parseCatArgs(args []string) (cmdCatOptions, error) {
	opts := cmdCatOptions{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'n':
					opts.Number = true
				default:
					return opts, fmt.Errorf("unsupported cat flag -%c", flag)
				}
			}
			continue
		}
		opts.Paths = append(opts.Paths, arg)
	}
	if len(opts.Paths) == 0 {
		return opts, errors.New("usage: bdy cmd cat [-n] <path...>")
	}
	return opts, nil
}

type cmdLSOptions struct {
	All  bool
	Long bool
}

func parseLSArgs(command string, args []string) (cmdLSOptions, string, error) {
	opts := cmdLSOptions{}
	switch command {
	case "ll":
		opts.Long = true
	case "la":
		opts.All = true
	}
	path := "."
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimPrefix(arg, "-") {
				switch flag {
				case 'a':
					opts.All = true
				case 'l':
					opts.Long = true
				default:
					return opts, "", fmt.Errorf("unsupported ls flag -%c", flag)
				}
			}
			continue
		}
		path = arg
	}
	return opts, path, nil
}

func printCmdEntries(out io.Writer, items []baidu.FileEntry, opts cmdLSOptions) {
	for _, item := range items {
		name := item.ServerFilename
		if name == "" {
			name = filepath.Base(item.Path)
		}
		if !opts.All && strings.HasPrefix(name, ".") {
			continue
		}
		if opts.Long {
			kind := "file"
			if item.IsDir == 1 {
				kind = "dir "
			}
			fmt.Fprintf(out, "%s %10d %s\n", kind, item.Size, item.Path)
			continue
		}
		fmt.Fprintln(out, name)
	}
}
