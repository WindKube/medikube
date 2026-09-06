package logging

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This package is the bridge. It is the one place allowed to hold a
// slog.Handler and to name PocketBase's logger, and the exemption stops here:
// everything the workaround needs lives behind this directory, which is what
// keeps CT-1 contained rather than spreading (constitution Principle VI).
const bridgePackage = "internal/logging"

// operatorSurface is the exemption from the stdout rule below, and it is a
// list of directories rather than a list of function names on purpose.
//
// Principle VI governs the log stream, not the file descriptor: `medikube
// routes`, `medikube openapi` and the serve banner PocketBase's own cobra
// command prints are operator output, not log lines, and a JSON-only stdout
// would be unimplementable on this stack. What must never happen is MediKube's
// own request, service or domain code writing anywhere but the injected
// logger, because that is the write no redaction, no level and no correlation
// id applies to.
//
// internal/logging is here because it owns the stream: logger.go is the one
// file that opens os.Stdout at all.
var operatorSurface = map[string]bool{
	"cmd/medikube":     true,
	"internal/cli":     true,
	"internal/logging": true,
	// scripts holds standalone `go run` build-time tools (e.g. traceability.go)
	// — never linked into the medikube binary, never touching a request,
	// service or domain path — so their stderr is operator output too.
	"scripts": true,
}

// bannedCalls are the writers that put a line somewhere other than the one
// stream. fmt.Print* is also caught inside the bridge itself by forbidigo, so
// between the two gates there is no gap.
var bannedCalls = map[string][]string{
	"fmt": {"Print", "Printf", "Println"},
	"log": {"Print", "Printf", "Println", "Fatal", "Fatalf", "Fatalln", "Panic", "Panicf", "Panicln"},
}

// bannedImports would give a package a second logger outright.
var bannedImports = map[string]string{
	"log/slog": "log/slog belongs to the PocketBase bridge and nowhere else",
}

// downcasts undo the decorator. pocketbase.PocketBase's embedded core.App is
// reassigned to a wrapper, so any code that asserts its way back to the
// concrete type gets an app whose Logger() is PocketBase's own again — and the
// lines it writes leave the stream silently (research D-29).
var downcasts = map[string]string{
	"github.com/pocketbase/pocketbase.PocketBase":   "asserting back to *pocketbase.PocketBase steps around the log decorator",
	"github.com/pocketbase/pocketbase/core.BaseApp": "asserting back to *core.BaseApp steps around the log decorator",
}

type finding struct {
	position string
	detail   string
}

func TestNothingOutsideTheBridgeWritesToASecondStream(t *testing.T) {
	t.Parallel()

	var (
		imports []finding
		calls   []finding
		scanned int
	)

	walkModule(t, func(path string, file *ast.File, fset *token.FileSet) {
		scanned++

		if withinBridge(t, path) {
			return
		}

		aliases := importAliases(file)

		for _, spec := range file.Imports {
			imported := unquote(t, spec.Path.Value)
			if reason, banned := bannedImports[imported]; banned {
				imports = append(imports, finding{fset.Position(spec.Pos()).String(), reason})
			}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			pkg, name, ok := qualifiedName(call.Fun, aliases)
			if !ok {
				return true
			}

			for _, banned := range bannedCalls[pkg] {
				if banned == name {
					calls = append(calls, finding{
						fset.Position(call.Pos()).String(),
						pkg + "." + name + " writes outside the one stream",
					})
				}
			}

			return true
		})
	})

	require.Greater(t, scanned, 20, "the walk found almost nothing; it is not looking where it thinks it is")
	assert.Empty(t, render(imports), "FR-053: one machine-readable stream, one logger")
	assert.Empty(t, render(calls), "FR-053: use the injected zerolog logger")
}

// The half fmt.Print* alone does not cover. `fmt.Fprintln(os.Stdout, name)` is
// the same defect wearing a different function name, and the ban has to be
// drawn on the destination rather than on the spelling or it catches only the
// careless version of the mistake (FR-038, FR-053).
func TestNothingOutsideTheOperatorSurfaceWritesToStdout(t *testing.T) {
	t.Parallel()

	var found []finding

	walkModule(t, func(path string, file *ast.File, fset *token.FileSet) {
		if operatorSurface[packageDir(t, path)] {
			return
		}

		aliases := importAliases(file)

		ast.Inspect(file, func(node ast.Node) bool {
			selector, isSelector := node.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}

			pkg, name, ok := qualifiedName(selector, aliases)
			if !ok || pkg != "os" || (name != "Stdout" && name != "Stderr") {
				return true
			}

			found = append(found, finding{
				fset.Position(node.Pos()).String(),
				"os." + name + " is named outside the operator surface; use the injected logger",
			})

			return true
		})
	})

	assert.Empty(t, render(found),
		"FR-053: MediKube's own code reaches the operator through the logger, never through the descriptor")
}

func TestNothingDowncastsPastTheLogDecorator(t *testing.T) {
	t.Parallel()

	var found []finding

	walkModule(t, func(path string, file *ast.File, fset *token.FileSet) {
		aliases := importAliases(file)

		report := func(expr ast.Expr) {
			star, ok := expr.(*ast.StarExpr)
			if !ok {
				return
			}

			pkg, name, ok := qualifiedName(star.X, aliases)
			if !ok {
				return
			}

			if reason, banned := downcasts[pkg+"."+name]; banned {
				found = append(found, finding{fset.Position(expr.Pos()).String(), reason})
			}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.TypeAssertExpr:
				if n.Type != nil {
					report(n.Type)
				}
			case *ast.CaseClause:
				for _, expr := range n.List {
					report(expr)
				}
			}

			return true
		})
	})

	assert.Empty(t, render(found),
		"CT-1: the decorator is the whole bridge on the request path; a downcast is how it stops working in silence")
}

func walkModule(t *testing.T, visit func(path string, file *ast.File, fset *token.FileSet)) {
	t.Helper()

	root := moduleRoot(t)
	fset := token.NewFileSet()

	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			name := entry.Name()
			if name != "." && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}

		visit(path, parsed, fset)

		return nil
	}))
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above %s", dir)
		dir = parent
	}
}

// packageDir is the file's directory relative to the module root, in slash
// form, which is how both exemption lists above are written.
func packageDir(t *testing.T, path string) string {
	t.Helper()

	rel, err := filepath.Rel(moduleRoot(t), path)
	require.NoError(t, err)

	return filepath.ToSlash(filepath.Dir(rel))
}

func withinBridge(t *testing.T, path string) bool {
	t.Helper()

	rel, err := filepath.Rel(moduleRoot(t), path)
	require.NoError(t, err)

	return filepath.Dir(rel) == filepath.FromSlash(bridgePackage)
}

// importAliases maps the identifier a file actually uses to the package it
// names, so a renamed import cannot walk past any of the rules above.
func importAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string, len(file.Imports))

	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		name := path
		if slash := strings.LastIndex(name, "/"); slash >= 0 {
			name = name[slash+1:]
		}

		if spec.Name != nil {
			name = spec.Name.Name
		}

		if name == "_" || name == "." {
			continue
		}

		aliases[name] = path
	}

	return aliases
}

func qualifiedName(expr ast.Expr, aliases map[string]string) (pkg, name string, ok bool) {
	selector, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return "", "", false
	}

	ident, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return "", "", false
	}

	path, known := aliases[ident.Name]
	if !known {
		return "", "", false
	}

	return path, selector.Sel.Name, true
}

func unquote(t *testing.T, quoted string) string {
	t.Helper()

	value, err := strconv.Unquote(quoted)
	require.NoError(t, err)

	return value
}

func render(found []finding) []string {
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.position+": "+f.detail)
	}

	return out
}
