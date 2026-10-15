// Package codenav helps an agent find its way around Go source: a map of a
// module's packages and their exported API, and symbol lookup through gopls.
package codenav

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Navigator answers questions about the Go code under root. Every dir it is
// given is relative to root and may not leave it.
type Navigator struct {
	root  string
	gopls string
}

// New returns a Navigator for the tree at root. gopls is the gopls binary;
// when it is empty, symbol lookups report that gopls is missing.
func New(root, gopls string) (*Navigator, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Navigator{root: abs, gopls: gopls}, nil
}

// FindGopls returns the gopls on PATH, else the one `go install` puts in
// $GOBIN or $GOPATH/bin, else "".
func FindGopls() string {
	if p, err := exec.LookPath("gopls"); err == nil {
		return p
	}
	var dirs []string
	if d := os.Getenv("GOBIN"); d != "" {
		dirs = append(dirs, d)
	}
	if d := os.Getenv("GOPATH"); d != "" {
		dirs = append(dirs, filepath.Join(d, "bin"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}
	for _, d := range dirs {
		p := filepath.Join(d, "gopls")
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// resolve turns a dir argument into an absolute path inside root.
func (n *Navigator) resolve(dir string) (string, error) {
	abs := filepath.Join(n.root, filepath.FromSlash(dir))
	rel, err := filepath.Rel(n.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("dir %q is outside %s", dir, n.root)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("dir %q is not a directory", dir)
	}
	return abs, nil
}

// rel gives path relative to root with forward slashes, or path unchanged
// when it is outside root.
func (n *Navigator) rel(path string) string {
	r, err := filepath.Rel(n.root, path)
	if err != nil || strings.HasPrefix(r, "..") {
		return path
	}
	return filepath.ToSlash(r)
}

// moduleOf finds the go.mod at or above dir, within root, and returns its
// directory and module path.
func (n *Navigator) moduleOf(dir string) (string, string, error) {
	for d := dir; ; d = filepath.Dir(d) {
		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			sc := bufio.NewScanner(bytes.NewReader(data))
			for sc.Scan() {
				if path, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
					return d, strings.Trim(strings.TrimSpace(path), `"`), nil
				}
			}
			return d, "", nil
		}
		if d == n.root || d == filepath.Dir(d) {
			return "", "", fmt.Errorf("no go.mod at or above %s", n.rel(dir))
		}
	}
}

// Map lists every package under dir with its exported API, one line per
// declaration with the file and line it is at. Test files, testdata, hidden
// directories and nested modules are left out.
func (n *Navigator) Map(dir string) (string, error) {
	start, err := n.resolve(dir)
	if err != nil {
		return "", err
	}
	modDir, modPath, err := n.moduleOf(start)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "module %s (%s)\n", modPath, n.rel(modDir))
	err = filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != start {
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" || name == "node_modules" {
				return fs.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return fs.SkipDir
			}
		}
		relPkg, _ := filepath.Rel(modDir, path)
		importPath := modPath
		if relPkg != "." {
			importPath += "/" + filepath.ToSlash(relPkg)
		}
		return writePackage(&b, path, n.rel(path), importPath)
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// writePackage writes the map of the package in dir, if it has one.
func writePackage(b *strings.Builder, dir, relDir, importPath string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			fmt.Fprintf(b, "\n%s: %v\n", relDir, err)
			return nil
		}
		files = append(files, f)
		names = append(names, name)
	}
	if len(files) == 0 {
		return nil
	}
	pkg, err := doc.NewFromFiles(fset, files, importPath)
	if err != nil {
		fmt.Fprintf(b, "\n%s: %v\n", relDir, err)
		return nil
	}

	fmt.Fprintf(b, "\n%s  package %s", relDir, pkg.Name)
	if s := pkg.Synopsis(pkg.Doc); s != "" {
		fmt.Fprintf(b, ": %s", s)
	}
	fmt.Fprintf(b, "\n  files: %s\n", strings.Join(names, " "))
	if pkg.Name == "main" {
		return nil
	}

	p := mapPrinter{b: b, fset: fset, pkg: pkg, dir: dir}
	for _, v := range pkg.Consts {
		p.values("const", v)
	}
	for _, v := range pkg.Vars {
		p.values("var", v)
	}
	for _, f := range pkg.Funcs {
		p.fn("  ", f)
	}
	for _, t := range pkg.Types {
		p.typ(t)
		for _, v := range t.Consts {
			p.values("const", v)
		}
		for _, v := range t.Vars {
			p.values("var", v)
		}
		for _, f := range t.Funcs {
			p.fn("    ", f)
		}
		for _, f := range t.Methods {
			p.fn("    ", f)
		}
	}
	return nil
}

type mapPrinter struct {
	b    *strings.Builder
	fset *token.FileSet
	pkg  *doc.Package
	dir  string
}

// at gives the file name and line of pos.
func (p mapPrinter) at(pos token.Pos) string {
	position := p.fset.Position(pos)
	return fmt.Sprintf("%s:%d", filepath.Base(position.Filename), position.Line)
}

func (p mapPrinter) line(indent, decl string, pos token.Pos, docText string) {
	fmt.Fprintf(p.b, "%s%s  %s", indent, decl, p.at(pos))
	if s := p.pkg.Synopsis(docText); s != "" {
		fmt.Fprintf(p.b, "  // %s", s)
	}
	p.b.WriteByte('\n')
}

func (p mapPrinter) fn(indent string, f *doc.Func) {
	decl := *f.Decl
	decl.Doc, decl.Body = nil, nil
	p.line(indent, p.print(&decl), f.Decl.Pos(), f.Doc)
}

func (p mapPrinter) values(kind string, v *doc.Value) {
	var names []string
	for _, spec := range v.Decl.Specs {
		for _, name := range spec.(*ast.ValueSpec).Names {
			if name.IsExported() {
				names = append(names, name.Name)
			}
		}
	}
	if len(names) > 0 {
		p.line("  ", kind+" "+strings.Join(names, ", "), v.Decl.Pos(), v.Doc)
	}
}

func (p mapPrinter) typ(t *doc.Type) {
	var spec *ast.TypeSpec
	for _, s := range t.Decl.Specs {
		if ts := s.(*ast.TypeSpec); ts.Name.Name == t.Name {
			spec = ts
		}
	}
	if spec == nil {
		return
	}
	head := "type " + t.Name
	if spec.TypeParams != nil {
		head = "type " + t.Name + p.print(spec.TypeParams)
	}
	switch tt := spec.Type.(type) {
	case *ast.StructType:
		var fields []string
		for _, f := range tt.Fields.List {
			if len(f.Names) == 0 {
				fields = append(fields, p.print(f.Type))
			}
			for _, name := range f.Names {
				if name.IsExported() {
					fields = append(fields, name.Name)
				}
			}
		}
		p.line("  ", head+" struct {"+strings.Join(fields, ", ")+"}", spec.Pos(), t.Doc)
	case *ast.InterfaceType:
		p.line("  ", head+" interface", spec.Pos(), t.Doc)
		for _, m := range tt.Methods.List {
			if len(m.Names) == 0 {
				fmt.Fprintf(p.b, "    %s\n", p.print(m.Type))
				continue
			}
			for _, name := range m.Names {
				if name.IsExported() {
					fmt.Fprintf(p.b, "    %s%s  %s\n", name.Name, strings.TrimPrefix(p.print(m.Type), "func"), p.at(name.Pos()))
				}
			}
		}
	default:
		assign := " "
		if spec.Assign.IsValid() {
			assign = " = "
		}
		p.line("  ", head+assign+p.print(spec.Type), spec.Pos(), t.Doc)
	}
}

// print renders node on one line.
func (p mapPrinter) print(node any) string {
	var buf bytes.Buffer
	if err := (&printer.Config{Mode: printer.RawFormat}).Fprint(&buf, p.fset, node); err != nil {
		return "?"
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}
