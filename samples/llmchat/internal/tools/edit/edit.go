// Package edit holds the tools that change files in a conversation's
// workspace. Both always need the person's approval before they run.
package edit

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"llmchat/internal/chat"
	"llmchat/internal/tools/sandbox"
)

// maxWriteBytes is the largest content write_file accepts.
const maxWriteBytes = 1 << 20

// WriteFile returns the write_file tool, which creates or replaces a file.
func WriteFile() chat.Tool { return writeFile{} }

// EditFile returns the edit_file tool, which replaces exact text in a file.
func EditFile() chat.Tool { return editFile{} }

type writeFile struct{}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

var writeFileParams = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)

func (writeFile) Spec() chat.ToolSpec {
	return chat.ToolSpec{
		Name:        "write_file",
		Description: "Create a file, or replace it completely, in the workspace. Missing directories are created.",
		Parameters:  writeFileParams,
	}
}

func (writeFile) NeedsApproval(json.RawMessage) bool { return true }

func (writeFile) Run(ctx context.Context, workspace string, args json.RawMessage) (string, error) {
	var a writeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("%w: write_file arguments: %v", chat.ErrInvalid, err)
	}
	if len(a.Content) > maxWriteBytes {
		return "", fmt.Errorf("%w: content is %d bytes; the limit is %d", chat.ErrInvalid, len(a.Content), maxWriteBytes)
	}
	path, err := sandbox.Resolve(workspace, a.Path)
	if err != nil {
		return "", err
	}

	mode := fs.FileMode(0o644)
	existed := false
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return "", fmt.Errorf("%w: %s is a directory", chat.ErrInvalid, sandbox.Rel(workspace, path))
		}
		mode = info.Mode().Perm()
		existed = true
	case !os.IsNotExist(err):
		return "", fmt.Errorf("%w: %v", chat.ErrInvalid, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(a.Content), mode); err != nil {
		return "", err
	}
	rel := sandbox.Rel(workspace, path)
	if !existed {
		return fmt.Sprintf("created %s (%d bytes)", rel, len(a.Content)), nil
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), rel), nil
}

type editFile struct{}

type editArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

var editFileParams = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_string","new_string"]}`)

func (editFile) Spec() chat.ToolSpec {
	return chat.ToolSpec{
		Name:        "edit_file",
		Description: "Replace exact text in an existing file. old_string must match exactly once unless replace_all is true.",
		Parameters:  editFileParams,
	}
}

func (editFile) NeedsApproval(json.RawMessage) bool { return true }

func (editFile) Run(ctx context.Context, workspace string, args json.RawMessage) (string, error) {
	var a editArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("%w: edit_file arguments: %v", chat.ErrInvalid, err)
	}
	if a.OldString == "" {
		return "", fmt.Errorf("%w: old_string must not be empty", chat.ErrInvalid)
	}
	if a.OldString == a.NewString {
		return "", fmt.Errorf("%w: old_string and new_string are identical", chat.ErrInvalid)
	}
	path, err := sandbox.Resolve(workspace, a.Path)
	if err != nil {
		return "", err
	}
	rel := sandbox.Rel(workspace, path)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s does not exist", chat.ErrNotFound, rel)
		}
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%w: %s is a directory", chat.ErrInvalid, rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := string(data)
	count := strings.Count(content, a.OldString)
	if count == 0 {
		return "", fmt.Errorf("%w: old_string not found in %s", chat.ErrInvalid, rel)
	}
	if count > 1 && !a.ReplaceAll {
		return "", fmt.Errorf("%w: old_string appears %d times in %s; add surrounding context so it matches once", chat.ErrInvalid, count, rel)
	}

	updated := strings.Replace(content, a.OldString, a.NewString, -1)
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return "", err
	}
	return fmt.Sprintf("replaced %d occurrence(s) in %s", count, rel), nil
}
