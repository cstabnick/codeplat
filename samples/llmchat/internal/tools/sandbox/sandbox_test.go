package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llmchat/internal/chat"
)

// fixture holds a workspace inside a temporary base directory, plus
// neighbours that the tests use as paths the workspace must not reach.
type fixture struct {
	base    string
	ws      string
	ws2     string
	outside string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{
		base:    base,
		ws:      filepath.Join(base, "ws"),
		ws2:     filepath.Join(base, "ws2"),
		outside: filepath.Join(base, "outside"),
	}
	for _, d := range []string{f.ws, f.ws2, f.outside} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.outside, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestResolveInside(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		rel  string
		want string
	}{
		{"empty rel is the workspace", "", f.ws},
		{"nonexistent relative path is joined", "a/b.txt", filepath.Join(f.ws, "a", "b.txt")},
		{"dot and dotdot are cleaned", "./a/../b", filepath.Join(f.ws, "b")},
		{"absolute path inside", filepath.Join(f.ws, "x", "y"), filepath.Join(f.ws, "x", "y")},
		{"absolute path to the workspace itself", f.ws, f.ws},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(f.ws, tt.rel)
			if err != nil {
				t.Fatalf("Resolve(%q) error: %v", tt.rel, err)
			}
			if got != tt.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.rel, got, tt.want)
			}
		})
	}
}

func TestResolveRefusesEscape(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		rel  string
	}{
		{"absolute path outside", filepath.Join(f.outside, "file")},
		{"dotdot", ".."},
		{"dotdot and name", "../x"},
		{"dotdot after descent", "a/../../x"},
		{"sibling sharing the name prefix", "../ws2/file"},
		{"absolute sibling sharing the name prefix", filepath.Join(f.ws2, "file")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(f.ws, tt.rel)
			if !errors.Is(err, chat.ErrInvalid) {
				t.Fatalf("Resolve(%q) error = %v, want chat.ErrInvalid", tt.rel, err)
			}
			if !strings.Contains(err.Error(), "outside the workspace") {
				t.Fatalf("Resolve(%q) error %q does not say outside the workspace", tt.rel, err)
			}
		})
	}
}

func TestResolveSymlinks(t *testing.T) {
	t.Run("link to outside is refused for existing target", func(t *testing.T) {
		f := newFixture(t)
		symlink(t, f.outside, filepath.Join(f.ws, "link"))
		_, err := Resolve(f.ws, "link/file")
		if !errors.Is(err, chat.ErrInvalid) {
			t.Fatalf("error = %v, want chat.ErrInvalid", err)
		}
	})

	t.Run("link to outside is refused for nonexistent target", func(t *testing.T) {
		f := newFixture(t)
		symlink(t, f.outside, filepath.Join(f.ws, "link"))
		_, err := Resolve(f.ws, "link/new.txt")
		if !errors.Is(err, chat.ErrInvalid) {
			t.Fatalf("error = %v, want chat.ErrInvalid", err)
		}
	})

	t.Run("link to a place inside is allowed", func(t *testing.T) {
		f := newFixture(t)
		target := filepath.Join(f.ws, "real")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		symlink(t, target, filepath.Join(f.ws, "alias"))
		got, err := Resolve(f.ws, "alias/f.txt")
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if want := filepath.Join(target, "f.txt"); got != want {
			t.Fatalf("Resolve = %q, want %q", got, want)
		}
	})

	t.Run("dangling link is refused", func(t *testing.T) {
		f := newFixture(t)
		symlink(t, filepath.Join(f.outside, "nowhere"), filepath.Join(f.ws, "dangle"))
		_, err := Resolve(f.ws, "dangle")
		if !errors.Is(err, chat.ErrInvalid) {
			t.Fatalf("error = %v, want chat.ErrInvalid", err)
		}
	})

	t.Run("workspace reached through a symlink works", func(t *testing.T) {
		f := newFixture(t)
		wsLink := filepath.Join(f.base, "wslink")
		symlink(t, f.ws, wsLink)
		got, err := Resolve(wsLink, "a/b.txt")
		if err != nil {
			t.Fatalf("Resolve error: %v", err)
		}
		if want := filepath.Join(f.ws, "a", "b.txt"); got != want {
			t.Fatalf("Resolve = %q, want %q", got, want)
		}
	})
}

func TestResolveBadWorkspace(t *testing.T) {
	f := newFixture(t)
	file := filepath.Join(f.ws, "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		workspace string
	}{
		{"relative workspace", "rel/dir"},
		{"missing workspace", filepath.Join(f.base, "missing")},
		{"workspace is a file", file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.workspace, "a")
			if !errors.Is(err, chat.ErrInvalid) {
				t.Fatalf("Resolve error = %v, want chat.ErrInvalid", err)
			}
		})
	}
}

func TestRel(t *testing.T) {
	f := newFixture(t)
	if got := Rel(f.ws, f.ws); got != "." {
		t.Errorf("Rel(workspace, workspace) = %q, want \".\"", got)
	}
	nested := filepath.Join(f.ws, "a", "b.txt")
	if got := Rel(f.ws, nested); got != "a/b.txt" {
		t.Errorf("Rel(workspace, nested) = %q, want \"a/b.txt\"", got)
	}
}
