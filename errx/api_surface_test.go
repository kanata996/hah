package errx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 这些编译期断言用于锁住 errx 的稳定公开 API 表面。
type httpErrorPublicSurface interface {
	Error() string
	Unwrap() error
	Status() int
	Code() string
	Title() string
	Detail() string
	Errors() []Violation
	WithViolations([]Violation) *HTTPError
}

var _ httpErrorPublicSurface = (*HTTPError)(nil)

var (
	_ = Violation{}

	_ ViolationCode = CodeInvalid
	_ ViolationCode = CodeRequired
	_ ViolationCode = CodeUnknown
	_ ViolationCode = CodeType
	_ ViolationCode = CodeMultiple

	_ ViolationIn = InBody
	_ ViolationIn = InQuery
	_ ViolationIn = InPath
	_ ViolationIn = InHeader

	_ func(int, string, string) *HTTPError        = NewHTTPError
	_ func(int, string, string, error) *HTTPError = NewHTTPErrorWithCause
)

// 设计文档明确要求 errx 不暴露包级 WithViolations；它只能是 HTTPError 实例方法。
func TestErrxPackageDoesNotExposePackageLevelWithViolations(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	dir := filepath.Dir(filename)
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parser.ParseFile(%q) error = %v", name, err)
		}
		if file.Name.Name != "errx" {
			t.Fatalf("file %q parsed package = %q, want errx", name, file.Name.Name)
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil && decl.Name.Name == "WithViolations" {
					t.Fatalf("unexpected package-level WithViolations in %s", name)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if spec.Name.Name == "WithViolations" {
							t.Fatalf("unexpected package-level WithViolations in %s", name)
						}
					case *ast.ValueSpec:
						for _, ident := range spec.Names {
							if ident.Name == "WithViolations" {
								t.Fatalf("unexpected package-level WithViolations in %s", name)
							}
						}
					}
				}
			}
		}
	}
}
