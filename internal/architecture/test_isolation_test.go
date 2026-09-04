package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T322, reconciliation C14. internal/testsupport/app.go's NewApp says it
// outright: "never share the result between two tests.ApiScenario runs" —
// apis.NewRouter binds a fresh, unnamed OnServe handler on every run, and a
// shared *tests.TestApp accumulates one more of them per scenario until the
// goroutine stack limit ends the process. The symptom is a stack overflow,
// not a readable test failure, which is exactly the shape of bug a source
// walk should catch before anyone hits it by running the suite.
//
// The heuristic: a package-level var (or a var inside an unexported init-like
// helper is not the target — only file scope, which is what every test in a
// package would see) that is either typed *tests.TestApp / tests.TestApp, or
// initialised from a call to NewApp / NewAppWith / tests.NewTestApp, is the
// anti-pattern this test exists to catch.

func TestNoTestFileSharesATestAppAcrossTests(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var offenders []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == "pb_data" ||
				(path != root && strings.HasPrefix(entry.Name(), ".")) {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)

		if sharesTestApp(t, path) {
			offenders = append(offenders, filepath.ToSlash(rel))
		}

		return nil
	})
	require.NoError(t, err)

	assert.Empty(t, offenders,
		"these test files declare a package-level *tests.TestApp — build a fresh one per test with testsupport.NewApp instead")
}

func sharesTestApp(t *testing.T, path string) bool {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	require.NoErrorf(t, err, "parsing %s", path)

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			if value.Type != nil && namesTestApp(value.Type) {
				return true
			}

			for _, rhs := range value.Values {
				if callsNewTestApp(rhs) {
					return true
				}
			}
		}
	}

	return false
}

// namesTestApp matches *tests.TestApp and tests.TestApp, however the tests
// package happens to be imported under an alias — the Sel name is what
// PocketBase actually calls the type, and that does not change with the
// import name.
func namesTestApp(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	sel, ok := expr.(*ast.SelectorExpr)

	return ok && sel.Sel.Name == "TestApp"
}

// callsNewTestApp matches an initialiser built from a call whose name ends in
// NewTestApp, NewApp or NewAppWith — testsupport's own constructors and
// PocketBase's tests.NewTestApp, whichever package the file imports it from.
func callsNewTestApp(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}

	var name string

	switch fn := call.Fun.(type) {
	case *ast.Ident:
		name = fn.Name
	case *ast.SelectorExpr:
		name = fn.Sel.Name
	default:
		return false
	}

	return name == "NewTestApp" || name == "NewApp" || name == "NewAppWith"
}
