package codenav

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// maxSymbols caps how many matches Find returns.
const maxSymbols = 40

// Symbol is a declaration gopls found.
type Symbol struct {
	Name string `json:"name"` // e.g. Service.ClaimIssue
	Kind string `json:"kind"` // e.g. Method
	Location
	pos string // file:line:col, absolute, for passing back to gopls
}

// Location is a line of source.
type Location struct {
	File string `json:"file"` // relative to the Navigator's root
	Line int    `json:"line"`
	Text string `json:"text"` // the source line, trimmed
}

// ErrNoGopls is returned by symbol lookups when gopls is not installed.
var ErrNoGopls = errors.New("gopls is not installed; run: go install golang.org/x/tools/gopls@latest")

// gopls prints "path:line:col-endcol" and, for symbols, " Name Kind".
var goplsLine = regexp.MustCompile(`^(.+):(\d+):(\d+)(?:-\d+)?(?:\s+(\S+)\s+(\S+))?$`)

// Find returns the declarations in dir's module whose name contains query,
// case-sensitively, exact matches first. A query may be qualified by its
// receiver or type, e.g. Service.ClaimIssue.
func (n *Navigator) Find(ctx context.Context, dir, query string) ([]Symbol, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	out, err := n.run(ctx, dir, "workspace_symbol", "-matcher=casesensitive", query)
	if err != nil {
		return nil, err
	}
	var syms []Symbol
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		m := goplsLine.FindStringSubmatch(line)
		if m == nil || m[4] == "" {
			continue
		}
		file, lineNo, col := m[1], atoi(m[2]), m[3]
		if !n.inside(file) {
			continue
		}
		syms = append(syms, Symbol{
			Name:     m[4],
			Kind:     m[5],
			Location: n.location(file, lineNo),
			pos:      file + ":" + m[2] + ":" + col,
		})
	}
	slices.SortStableFunc(syms, func(a, b Symbol) int {
		return exactness(b.Name, query) - exactness(a.Name, query)
	})
	if len(syms) > maxSymbols {
		syms = syms[:maxSymbols]
	}
	return syms, nil
}

// References returns every use of the declaration named symbol, its
// declaration first. symbol is a name such as ClaimIssue or
// Service.ClaimIssue. When several declarations have that name, in picks one
// by a part of its file path, such as "store/sqlite".
func (n *Navigator) References(ctx context.Context, dir, symbol, in string) ([]Location, error) {
	syms, err := n.Find(ctx, dir, symbol)
	if err != nil {
		return nil, err
	}
	var matches []Symbol
	for _, s := range syms {
		if exactness(s.Name, symbol) > 0 && (in == "" || strings.Contains(s.File, in)) {
			matches = append(matches, s)
		}
	}
	switch {
	case len(matches) == 0:
		return nil, fmt.Errorf("no declaration named %q%s; symbol_find %q lists close matches", symbol, inClause(in), symbol)
	case len(matches) > 1:
		var b strings.Builder
		fmt.Fprintf(&b, "%d declarations are named %q%s; pass `in` with part of one's path:\n", len(matches), symbol, inClause(in))
		b.WriteString(FormatSymbols(matches))
		return nil, errors.New(strings.TrimRight(b.String(), "\n"))
	}

	out, err := n.run(ctx, dir, "references", "-d", matches[0].pos)
	if err != nil {
		return nil, err
	}
	var locs []Location
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		m := goplsLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		locs = append(locs, n.location(m[1], atoi(m[2])))
	}
	decl := matches[0].Location
	slices.SortStableFunc(locs, func(a, b Location) int {
		if (a == decl) != (b == decl) {
			if a == decl {
				return -1
			}
			return 1
		}
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return a.Line - b.Line
	})
	return locs, nil
}

// FormatSymbols renders symbols one per line: file:line  Kind Name  text.
func FormatSymbols(syms []Symbol) string {
	if len(syms) == 0 {
		return "no matches\n"
	}
	var b strings.Builder
	for _, s := range syms {
		fmt.Fprintf(&b, "%s:%d  %s %s  %s\n", s.File, s.Line, s.Kind, s.Name, s.Text)
	}
	return b.String()
}

// FormatLocations renders locations one per line: file:line  text.
func FormatLocations(locs []Location) string {
	if len(locs) == 0 {
		return "no references\n"
	}
	var b strings.Builder
	for _, l := range locs {
		fmt.Fprintf(&b, "%s:%d  %s\n", l.File, l.Line, l.Text)
	}
	return b.String()
}

// run runs gopls in the module that contains dir, so that a nested module
// is its own workspace.
func (n *Navigator) run(ctx context.Context, dir string, args ...string) (string, error) {
	if n.gopls == "" {
		return "", ErrNoGopls
	}
	abs, err := n.resolve(dir)
	if err != nil {
		return "", err
	}
	modDir, _, err := n.moduleOf(abs)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, n.gopls, args...)
	cmd.Dir = modDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gopls %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

// inside reports whether path is within root, so dependencies and the
// standard library are left out.
func (n *Navigator) inside(path string) bool {
	r, err := filepath.Rel(n.root, path)
	return err == nil && !strings.HasPrefix(r, "..")
}

func (n *Navigator) location(file string, line int) Location {
	return Location{File: n.rel(file), Line: line, Text: sourceLine(file, line)}
}

// exactness ranks how well name matches query: 2 for the same name, 1 for a
// name whose last part is query (Service.ClaimIssue for ClaimIssue), else 0.
func exactness(name, query string) int {
	switch {
	case name == query:
		return 2
	case strings.HasSuffix(name, "."+query):
		return 1
	}
	return 0
}

func inClause(in string) string {
	if in == "" {
		return ""
	}
	return fmt.Sprintf(" in a path containing %q", in)
}

func sourceLine(file string, line int) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for i := 1; sc.Scan(); i++ {
		if i == line {
			return strings.TrimSpace(sc.Text())
		}
	}
	return ""
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
