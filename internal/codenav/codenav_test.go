package codenav_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeplat/internal/codenav"
)

// writeTree creates a small repository: module example.com/shop with a
// package that declares Cart twice over, and a nested module that must be
// mapped on its own.
func writeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/shop\n\ngo 1.22\n",
		"cart/cart.go": `// Package cart keeps what a shopper is buying.
package cart

// Limit is the most items a cart holds.
const Limit = 50

// Cart is one shopper's items.
type Cart struct {
	Owner string
	items []string
}

// New returns an empty cart for owner.
func New(owner string) *Cart { return &Cart{Owner: owner} }

// Add puts an item in the cart.
func (c *Cart) Add(item string) { c.items = append(c.items, item) }

func helper() {}
`,
		"cart/cart_test.go": "package cart\n\nfunc TestHidden() {}\n",
		"store/store.go": `package store

import "example.com/shop/cart"

// Saver persists carts.
type Saver interface {
	Save(c *cart.Cart) error
}

type Cart struct{}

func Fill() { cart.New("x").Add("y") }
`,
		"cmd/shop/main.go":  "// Command shop runs the shop.\npackage main\n\nfunc main() {}\n",
		"testdata/skip.go":  "package skip\n\nfunc Skipped() {}\n",
		"plugin/go.mod":     "module example.com/plugin\n\ngo 1.22\n",
		"plugin/plugin.go":  "// Package plugin is a separate module.\npackage plugin\n\nfunc Hook() {}\n",
		".hidden/hidden.go": "package hidden\n\nfunc Hidden() {}\n",
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestMap(t *testing.T) {
	nav, err := codenav.New(writeTree(t), "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := nav.Map("")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"module example.com/shop (.)",
		"cart  package cart: Package cart keeps what a shopper is buying.",
		"files: cart.go\n",
		"const Limit  cart.go:5  // Limit is the most items a cart holds.",
		"type Cart struct {Owner}  cart.go:8  // Cart is one shopper's items.",
		"    func New(owner string) *Cart  cart.go:14",
		"    func (c *Cart) Add(item string)  cart.go:17  // Add puts an item in the cart.",
		"type Saver interface  store.go:6",
		"    Save(c *cart.Cart) error  store.go:7",
		"cmd/shop  package main: Command shop runs the shop.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("map lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"helper", "TestHidden", "Skipped", "Hook", "Hidden", "func main"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("map has %q:\n%s", unwanted, got)
		}
	}

	nested, err := nav.Map("plugin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(nested, "module example.com/plugin (plugin)") || !strings.Contains(nested, "func Hook()") {
		t.Errorf("nested module map:\n%s", nested)
	}

	for _, dir := range []string{"..", "../x", "nope", "go.mod"} {
		if _, err := nav.Map(dir); err == nil {
			t.Errorf("Map(%q) succeeded", dir)
		}
	}
}

func TestSymbolsWithoutGopls(t *testing.T) {
	nav, err := codenav.New(writeTree(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nav.Find(context.Background(), "", "Cart"); !errors.Is(err, codenav.ErrNoGopls) {
		t.Errorf("Find err = %v, want ErrNoGopls", err)
	}
	if _, err := nav.References(context.Background(), "", "Cart", ""); !errors.Is(err, codenav.ErrNoGopls) {
		t.Errorf("References err = %v, want ErrNoGopls", err)
	}
}

func TestSymbols(t *testing.T) {
	gopls := codenav.FindGopls()
	if gopls == "" {
		t.Skip("gopls is not installed")
	}
	nav, err := codenav.New(writeTree(t), gopls)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	syms, err := nav.Find(ctx, "", "Add")
	if err != nil {
		t.Fatal(err)
	}
	if got := codenav.FormatSymbols(syms); !strings.HasPrefix(got, "cart/cart.go:17  Method Cart.Add  func (c *Cart) Add(item string) {") {
		t.Errorf("Find Add:\n%s", got)
	}

	if _, err := nav.References(ctx, "", "Cart", ""); err == nil || !strings.Contains(err.Error(), "2 declarations are named \"Cart\"") {
		t.Errorf("ambiguous References err = %v", err)
	}
	if _, err := nav.References(ctx, "", "Missing", ""); err == nil {
		t.Error("References to a missing symbol succeeded")
	}

	locs, err := nav.References(ctx, "", "Cart.Add", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "cart/cart.go:17  func (c *Cart) Add(item string) { c.items = append(c.items, item) }\n" +
		"store/store.go:12  func Fill() { cart.New(\"x\").Add(\"y\") }\n"
	if got := codenav.FormatLocations(locs); got != want {
		t.Errorf("References Cart.Add:\n%s\nwant:\n%s", got, want)
	}

	locs, err = nav.References(ctx, "", "Cart", "store/")
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 || locs[0].File != "store/store.go" || locs[0].Line != 10 {
		t.Errorf("References Cart in store/: %+v", locs)
	}

	syms, err = nav.Find(ctx, "plugin", "Hook")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 || syms[0].File != "plugin/plugin.go" {
		t.Errorf("Find Hook in nested module: %+v", syms)
	}
}
