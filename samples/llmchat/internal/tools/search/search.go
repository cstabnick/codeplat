// Package search holds the read-only grep tool, which searches the
// workspace's files for a regular expression.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"llmchat/internal/chat"
	"llmchat/internal/tools/sandbox"
)

const (
	maxMatches   = 200
	maxLineRunes = 300
	maxFileSize  = 2 << 20 // 2 MiB
	binarySniff  = 8000
)

// skipDirs are directory names the walk never enters.
var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

type grepTool struct{}

// Grep returns the read-only tool that searches workspace files for a regular expression.
func Grep() chat.Tool { return grepTool{} }

type grepArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	IgnoreCase bool   `json:"ignore_case"`
}

func (grepTool) Spec() chat.ToolSpec {
	return chat.ToolSpec{
		Name:        "grep",
		Description: "Search files in the workspace for a regular expression (Go RE2 syntax). Returns matching lines as path:line:text.",
		Parameters: json.RawMessage(`{"type":"object","properties":{` +
			`"pattern":{"type":"string","description":"Regular expression"},` +
			`"path":{"type":"string","description":"File or directory to search, relative to the workspace (default: the workspace)"},` +
			`"glob":{"type":"string","description":"Only search files whose base name matches this glob, e.g. *.go"},` +
			`"ignore_case":{"type":"boolean"}},` +
			`"required":["pattern"]}`),
	}
}

func (grepTool) NeedsApproval(json.RawMessage) bool { return false }

func (grepTool) Run(ctx context.Context, workspace string, raw json.RawMessage) (string, error) {
	var a grepArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("%w: arguments: %v", chat.ErrInvalid, err)
	}
	if a.Pattern == "" {
		return "", fmt.Errorf("%w: pattern is empty", chat.ErrInvalid)
	}
	expr := a.Pattern
	if a.IgnoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return "", fmt.Errorf("%w: pattern: %v", chat.ErrInvalid, err)
	}
	if a.Glob != "" {
		if _, err := filepath.Match(a.Glob, ""); err != nil {
			return "", fmt.Errorf("%w: glob %q: %v", chat.ErrInvalid, a.Glob, err)
		}
	}

	root, err := sandbox.Resolve(workspace, a.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chat.ErrInvalid, err)
	}

	s := &searcher{ctx: ctx, workspace: workspace, re: re}
	if info.IsDir() {
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() && p != root {
					return filepath.SkipDir
				}
				return nil
			}
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if p != root && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			if a.Glob != "" {
				if ok, _ := filepath.Match(a.Glob, d.Name()); !ok {
					return nil
				}
			}
			return s.file(p)
		})
	} else {
		err = s.file(root)
	}
	if err != nil && !s.stopped {
		return "", err
	}

	if len(s.lines) == 0 {
		return "no matches", nil
	}
	out := strings.Join(s.lines, "\n")
	if s.stopped {
		out += fmt.Sprintf("\n[stopped after %d matches]", maxMatches)
	}
	return out, nil
}

// searcher collects matching lines across files.
type searcher struct {
	ctx       context.Context
	workspace string
	re        *regexp.Regexp
	lines     []string
	stopped   bool
}

// errStop ends the walk once the match limit is passed.
var errStop = errors.New("stop")

// file searches one file and appends its matches. It skips files that are
// too large, binary, or unreadable. It returns errStop when the limit is passed.
func (s *searcher) file(path string) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileSize {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	sniff := data
	if len(sniff) > binarySniff {
		sniff = sniff[:binarySniff]
	}
	if strings.IndexByte(string(sniff), 0) >= 0 {
		return nil
	}

	rel := sandbox.Rel(s.workspace, path)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !s.re.MatchString(line) {
			continue
		}
		if len(s.lines) == maxMatches {
			s.stopped = true
			return errStop
		}
		s.lines = append(s.lines, fmt.Sprintf("%s:%d:%s", rel, i+1, cut(line, maxLineRunes)))
	}
	return nil
}

// cut returns the first n runes of s.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
