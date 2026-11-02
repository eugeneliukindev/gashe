package gashe_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReferenceListsEveryExport fails when an exported name of a package is missing from
// REFERENCE.md or REFERENCE.ru.md, so the reference does not fall behind the code.
func TestReferenceListsEveryExport(t *testing.T) {
	dirs := []string{".", "backend/memory", "backend/nearcache", "backend/redis", "codec/msgpack", "backendtest"}
	names := make([]string, 0, 200) // about the size of the public API
	for _, dir := range dirs {
		names = append(names, exports(t, dir)...)
	}
	if len(names) < 50 {
		t.Fatalf("found only %d exports; the parser missed packages", len(names))
	}
	for _, doc := range []string{"REFERENCE.md", "REFERENCE.ru.md"} {
		raw, err := os.ReadFile(doc) //nolint:gosec // one of two fixed file names
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if !regexp.MustCompile(`\b` + name + `\b`).Match(raw) {
				t.Errorf("%s does not mention %s", doc, name)
			}
		}
	}
}

// exports returns the exported functions, types, constants and variables of the non-test files in
// dir, with the exported methods and fields of exported types.
func exports(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	add := func(id *ast.Ident) {
		if id.IsExported() {
			names = append(names, id.Name)
		}
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil || receiverExported(d.Recv.List[0].Type) {
					add(d.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						add(s.Name)
						if st, ok := s.Type.(*ast.StructType); ok && s.Name.IsExported() {
							for _, field := range st.Fields.List {
								for _, n := range field.Names {
									add(n)
								}
							}
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							add(n)
						}
					}
				}
			}
		}
	}
	return names
}

// receiverExported reports whether a method receiver such as *Cache or Policy[V] is an exported type.
func receiverExported(e ast.Expr) bool {
	for {
		switch x := e.(type) {
		case *ast.StarExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.Ident:
			return x.IsExported()
		default:
			return false
		}
	}
}
