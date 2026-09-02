package bdynd

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

type GrepOptions struct {
	Pattern    string
	IgnoreCase bool
	Regex      bool
}

type GrepResult struct {
	Path string
	Line int
	Text string
}

func Grep(r Repo, opts GrepOptions) ([]GrepResult, error) {
	idx, err := LoadIndex(r)
	if err != nil {
		return nil, err
	}
	var re *regexp.Regexp
	pattern := opts.Pattern
	if opts.Regex {
		if opts.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
	}
	if opts.IgnoreCase && !opts.Regex {
		pattern = strings.ToLower(pattern)
	}
	var results []GrepResult
	for _, entry := range sortedIndexEntries(idx) {
		if entry.Kind != KindBlob {
			continue
		}
		data, err := ReadBlob(r, entry.OID)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			matched := false
			if opts.Regex {
				matched = re.MatchString(line)
			} else if opts.IgnoreCase {
				matched = strings.Contains(strings.ToLower(line), pattern)
			} else {
				matched = strings.Contains(line, pattern)
			}
			if matched {
				results = append(results, GrepResult{Path: entry.Path, Line: lineNo, Text: line})
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	return results, nil
}
