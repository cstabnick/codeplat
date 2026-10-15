// Package fs holds the read-only tools that let the model look at files in
// a conversation's workspace: read_file and list_dir. Every path goes
// through sandbox.Resolve, so neither tool can read outside the workspace.
package fs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"llmchat/internal/chat"
	"llmchat/internal/tools/sandbox"
)

const (
	defaultLimit   = 2000
	maxFileSize    = 2 << 20 // 2 MiB: larger files need offset or limit
	binarySniff    = 8000    // bytes checked for a NUL to call a file binary
	maxListEntries = 500
)

const readFileParams = `{"type":"object","properties":{"path":{"type":"string","description":"File path, relative to the workspace"},"offset":{"type":"integer","description":"First line to read, from 1 (default 1)"},"limit":{"type":"integer","description":"Number of lines to read (default 2000)"}},"required":["path"]}`

const listDirParams = `{"type":"object","properties":{"path":{"type":"string","description":"Directory path, relative to the workspace (default: the workspace)"}}}`

// ReadFile returns the read_file tool.
func ReadFile() chat.Tool { return readFile{} }

// ListDir returns the list_dir tool.
func ListDir() chat.Tool { return listDir{} }

// decode reads tool arguments into v. Empty arguments count as {}.
func decode(args json.RawMessage, v any) error {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, v); err != nil {
		return fmt.Errorf("%w: %v", chat.ErrInvalid, err)
	}
	return nil
}

type readFile struct{}

type readFileArgs struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset"`
	Limit  *int   `json:"limit"`
}

func (readFile) Spec() chat.ToolSpec {
	return chat.ToolSpec{
		Name:        "read_file",
		Description: "Read a text file in the workspace. Returns the lines with line numbers.",
		Parameters:  json.RawMessage(readFileParams),
	}
}

func (readFile) NeedsApproval(json.RawMessage) bool { return false }

func (readFile) Run(_ context.Context, workspace string, args json.RawMessage) (string, error) {
	var a readFileArgs
	if err := decode(args, &a); err != nil {
		return "", err
	}
	offset, limit := 1, defaultLimit
	if a.Offset != nil {
		if *a.Offset < 1 {
			return "", fmt.Errorf("%w: offset must be at least 1", chat.ErrInvalid)
		}
		offset = *a.Offset
	}
	if a.Limit != nil {
		if *a.Limit < 1 {
			return "", fmt.Errorf("%w: limit must be at least 1", chat.ErrInvalid)
		}
		limit = *a.Limit
	}

	p, err := sandbox.Resolve(workspace, a.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("cannot read %q: %w", a.Path, err)
	}
	if info.IsDir() {
		return "", errors.New("path is a directory")
	}
	if info.Size() > maxFileSize && a.Offset == nil && a.Limit == nil {
		return "", fmt.Errorf("file too large (%d bytes); use offset and limit", info.Size())
	}

	f, err := os.Open(p)
	if err != nil {
		return "", fmt.Errorf("cannot read %q: %w", a.Path, err)
	}
	defer f.Close()

	sniff := make([]byte, binarySniff)
	n, err := io.ReadFull(f, sniff)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", fmt.Errorf("cannot read %q: %w", a.Path, err)
	}
	if bytes.IndexByte(sniff[:n], 0) >= 0 {
		return "", errors.New("binary file")
	}
	if info.Size() == 0 {
		return "(empty file)", nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("cannot read %q: %w", a.Path, err)
	}
	return readLines(f, offset, limit)
}

// readLines writes up to limit lines starting at line offset, each as
// "%6d\t%s\n". It reads the file as a stream, so no size cap applies here.
func readLines(r io.Reader, offset, limit int) (string, error) {
	br := bufio.NewReader(r)
	var b strings.Builder
	lineNo, emitted := 0, 0
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			lineNo++
			if lineNo >= offset {
				if emitted == limit {
					fmt.Fprintf(&b, "[more lines follow; use offset %d]\n", lineNo)
					return b.String(), nil
				}
				text := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
				fmt.Fprintf(&b, "%6d\t%s\n", lineNo, text)
				emitted++
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if lineNo < offset {
		return "", fmt.Errorf("offset %d is past the end of the file", offset)
	}
	return b.String(), nil
}

type listDir struct{}

type listDirArgs struct {
	Path string `json:"path"`
}

func (listDir) Spec() chat.ToolSpec {
	return chat.ToolSpec{
		Name:        "list_dir",
		Description: "List the entries of a directory in the workspace.",
		Parameters:  json.RawMessage(listDirParams),
	}
}

func (listDir) NeedsApproval(json.RawMessage) bool { return false }

func (listDir) Run(_ context.Context, workspace string, args json.RawMessage) (string, error) {
	var a listDirArgs
	if err := decode(args, &a); err != nil {
		return "", err
	}
	p, err := sandbox.Resolve(workspace, a.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("cannot list %q: %w", a.Path, err)
	}
	if !info.IsDir() {
		return "", errors.New("not a directory")
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return "", fmt.Errorf("cannot list %q: %w", a.Path, err)
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var b strings.Builder
	for i, e := range entries {
		if i == maxListEntries {
			fmt.Fprintf(&b, "[%d more entries not shown]\n", len(entries)-maxListEntries)
			break
		}
		name := e.Name()
		if isDir(filepath.Join(p, name), e) {
			name += "/"
		}
		b.WriteString(name + "\n")
	}
	return b.String(), nil
}

// isDir reports whether the entry at path is a directory. A symbolic link
// counts as a directory when it points to one; a dangling link does not.
func isDir(path string, e os.DirEntry) bool {
	if e.Type()&os.ModeSymlink == 0 {
		return e.IsDir()
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
