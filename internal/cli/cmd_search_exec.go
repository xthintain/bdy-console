package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"baiduyunStorage/internal/baidu"
)

func cmdFind(ctx context.Context, client baidu.Client, out io.Writer, opts cmdSearchOptions, resolve func(string) string) error {
	items, err := searchRemoteItems(ctx, client, opts, resolve(opts.Path))
	if err != nil {
		return err
	}
	for _, item := range items {
		if opts.Type == "f" && item.IsDir == 1 {
			continue
		}
		if opts.Type == "d" && item.IsDir == 0 {
			continue
		}
		matched, err := searchMatch(opts, item.Path, item.ServerFilename)
		if err != nil {
			return err
		}
		if opts.Invert {
			matched = !matched
		}
		if matched {
			kind := "file"
			if item.IsDir == 1 {
				kind = "dir "
			}
			fmt.Fprintf(out, "%s %10d %s\n", kind, item.Size, item.Path)
		}
	}
	return nil
}

func searchRemoteItems(ctx context.Context, client baidu.Client, opts cmdSearchOptions, dir string) ([]baidu.FileEntry, error) {
	key := searchRemoteKey(opts.Pattern)
	if key == "" || opts.Invert {
		return client.ListAll(ctx, dir)
	}
	return client.SearchAll(ctx, baidu.SearchOptions{Dir: dir, Key: key, PageSize: 500})
}

func searchRemoteKey(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if !strings.ContainsAny(pattern, "*?[]/\\.(){}^$+|") {
		return pattern
	}
	fields := strings.FieldsFunc(pattern, func(r rune) bool {
		return strings.ContainsRune("*?[]/\\.(){}^$+|-", r)
	})
	best := ""
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len(field) > len(best) {
			best = field
		}
	}
	if best != "" {
		return best
	}
	if strings.ContainsAny(pattern, "*?[") {
		return ""
	}
	return pattern
}

func searchMatch(opts cmdSearchOptions, remotePath, filename string) (bool, error) {
	pattern := opts.Pattern
	ignoreCase := opts.IgnoreCase || opts.Command == "search"
	if opts.Regex {
		if ignoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		return re.MatchString(remotePath) || re.MatchString(filename), nil
	}
	if ignoreCase {
		pattern = strings.ToLower(pattern)
		remotePath = strings.ToLower(remotePath)
		filename = strings.ToLower(filename)
	}
	if strings.ContainsAny(pattern, "*?[") {
		pathMatch, err := filepath.Match(pattern, remotePath)
		if err != nil {
			return false, err
		}
		nameMatch, err := filepath.Match(pattern, filename)
		if err != nil {
			return false, err
		}
		return pathMatch || nameMatch, nil
	}
	return strings.Contains(remotePath, pattern) || strings.Contains(filename, pattern), nil
}
