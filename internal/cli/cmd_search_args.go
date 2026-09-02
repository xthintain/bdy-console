package cli

import (
	"fmt"
	"strings"
)

type cmdSearchOptions struct {
	Command    string
	Pattern    string
	Path       string
	IgnoreCase bool
	Invert     bool
	Regex      bool
	Type       string
}

func parseSearchArgs(command string, args []string) (cmdSearchOptions, error) {
	opts := cmdSearchOptions{Command: command, Path: "."}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-i":
			opts.IgnoreCase = true
		case "-v":
			opts.Invert = true
		case "-E", "--regex":
			opts.Regex = true
		case "-name":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("usage: bdy cmd %s [-i] [-v] [-name pattern] [-type f|d] [pattern] [path]", command)
			}
			opts.Pattern = args[i]
		case "-type":
			i++
			if i >= len(args) || (args[i] != "f" && args[i] != "d") {
				return opts, fmt.Errorf("usage: bdy cmd %s [-type f|d]", command)
			}
			opts.Type = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				for _, flag := range strings.TrimPrefix(arg, "-") {
					switch flag {
					case 'E':
						opts.Regex = true
					case 'i':
						opts.IgnoreCase = true
					case 'v':
						opts.Invert = true
					default:
						return opts, fmt.Errorf("unsupported %s flag -%c", command, flag)
					}
				}
				continue
			}
			positional = append(positional, arg)
		}
	}
	if opts.Pattern == "" && len(positional) > 0 {
		opts.Pattern = positional[0]
		positional = positional[1:]
	}
	if len(positional) > 0 {
		opts.Path = positional[0]
	}
	if opts.Pattern == "" {
		return opts, fmt.Errorf("usage: bdy cmd %s <pattern> [path]", command)
	}
	return opts, nil
}
