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

	"medikube/internal/httproute"
)

// T302, FR-069. Every route that is both user-scoped and reaches clinical
// data must have a testsupport.RunOwnershipMatrix proving a stranger is
// refused and the owner succeeds (internal/testsupport/authz.go). The
// predicate below is read off Route's own fields — Auth and Path — rather
// than a hand-picked list of OpIDs, so a fifteenth clinical route added later
// is covered by construction instead of by somebody remembering to add it
// here too.
//
// Streams are deliberately outside this gate's reach: contracts/streams.md's
// authorization model re-checks the caller on every event rather than once
// per request (internal/web/stream/session_test.go proves that), which
// RunOwnershipMatrix's single-request-per-leg shape cannot exercise. A stream
// route matching the predicate below would need its own justification to stay
// out, which streamRecords does not need because Kind excludes it outright.

// clinicalUserRoute is the predicate: a JSON operation, requiring a session,
// whose path names the one collection this phase's clinical data lives
// under.
func clinicalUserRoute(route httproute.Route) bool {
	return route.Kind == httproute.KindAPI &&
		route.Auth == httproute.AuthUser &&
		strings.Contains(route.Path, "/records")
}

// routeShape classifies a records route path by its trailing path
// parameters, which is also how the ownership-matrix test's own URL helpers
// (collectionURL, crossKindURL, recordURL) are told apart below.
func routeShape(path string) string {
	switch {
	case strings.HasSuffix(path, "/{id}/medications/{medicationId}"):
		return "courseItem"
	case strings.HasSuffix(path, "/{id}/medications"):
		return "courseList"
	case strings.HasSuffix(path, "/{kind}/{id}"):
		return "one"
	case strings.HasSuffix(path, "/{kind}"):
		return "ofKind"
	case !strings.Contains(path, "{"):
		return "collection"
	default:
		return ""
	}
}

func TestEveryClinicalUserRouteIsInTheOwnershipMatrix(t *testing.T) {
	t.Parallel()

	required := map[string]httproute.Route{}
	for _, route := range httproute.Inventory().Routes() {
		if !clinicalUserRoute(route) {
			continue
		}

		shape := routeShape(route.Path)
		require.NotEmptyf(t, shape, "%s matches the clinical-user-route predicate but routeShape does not recognise its path %s — teach it the new shape", route.OpID, route.Path)

		required[route.Method+" "+shape] = route
	}
	require.NotEmpty(t, required, "the predicate matched no route at all — it is broken, not the inventory")

	covered := ownershipMatrixCoverage(t)

	for key, route := range required {
		assert.Containsf(t, covered, key,
			"%s (%s) touches clinical data as a user-scoped route, but no testsupport.RunOwnershipMatrix case covers it", route.OpID, route.Pattern())
	}

	for key := range covered {
		_, isRequired := required[key]
		assert.Truef(t, isRequired,
			"an ownership-matrix case covers %q, which is not a route the predicate considers clinical and user-scoped any more — the matrix or the predicate has drifted", key)
	}
}

// ownershipMatrixCoverage walks internal/web's own *_test.go sources (the
// application's tests, not testsupport's self-test of the matrix mechanism
// in internal/testsupport/authz_test.go, which runs the matrix against a fake
// handler and proves nothing about a real route) for calls to
// testsupport.RunOwnershipMatrix, and resolves each case's Method and Path to
// the "METHOD shape" key routeShape/clinicalUserRoute produce.
func ownershipMatrixCoverage(t *testing.T) map[string]bool {
	t.Helper()

	dir := filepath.Join(repoRoot(t), "internal/web")

	covered := map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		require.NoErrorf(t, parseErr, "parsing %s", path)

		// Cases often address a variable set up above them — `target :=
		// recordURL(id)`, then `Path: target` — rather than the call
		// expression inline, so idents are resolved against every assignment
		// in the file before the Cases themselves are read.
		idents := pathShapedIdents(file)

		ast.Inspect(file, func(n ast.Node) bool {
			for _, key := range ownershipCaseKeys(n, idents) {
				covered[key] = true
			}

			return true
		})

		return nil
	})
	require.NoError(t, err)

	return covered
}

// ownershipCaseKeys matches `testsupport.RunOwnershipMatrix(t,
// testsupport.OwnershipMatrix{ ... Cases: []testsupport.OwnershipCase{ ...
// } })` and returns one "METHOD shape" key per case found.
func ownershipCaseKeys(n ast.Node, idents map[string]string) []string {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "RunOwnershipMatrix" || len(call.Args) != 2 {
		return nil
	}

	matrix, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		return nil
	}

	var keys []string

	for _, elt := range matrix.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Cases" {
			continue
		}

		cases, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}

		for _, caseElt := range cases.Elts {
			if key := ownershipCaseKey(caseElt, idents); key != "" {
				keys = append(keys, key)
			}
		}
	}

	return keys
}

func ownershipCaseKey(elt ast.Expr, idents map[string]string) string {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok {
		return ""
	}

	var method, shape string

	for _, field := range lit.Elts {
		kv, ok := field.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "Method":
			method = httpMethod(kv.Value)
		case "Path":
			shape = pathShape(kv.Value, idents)
		}
	}

	if method == "" || shape == "" {
		return ""
	}

	return method + " " + shape
}

// pathShapedIdents finds every `ident := recordURL(...)` (or
// collectionURL/crossKindURL) short variable declaration in a file and
// records the shape its right-hand side resolves to, so a Case that
// addresses the variable rather than the call inline still resolves.
func pathShapedIdents(file *ast.File) map[string]string {
	idents := map[string]string{}

	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}

		name, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}

		if shape := pathShape(assign.Rhs[0], nil); shape != "" {
			idents[name.Name] = shape
		}

		return true
	})

	return idents
}

// httpMethod resolves `http.MethodGet` and friends to the verb the route
// table itself is keyed by.
func httpMethod(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "http" {
		return ""
	}

	return strings.ToUpper(strings.TrimPrefix(sel.Sel.Name, "Method"))
}

// pathShape resolves a Path expression to the same "one" / "ofKind" /
// "collection" vocabulary routeShape uses, by recognising the harness's own
// URL helpers (harness_test.go: collectionURL, recordURL, crossKindURL) at
// the head of the expression — a concatenation like `collectionURL() +
// "?limit=100"` names the same route the bare call does.
func pathShape(expr ast.Expr, idents map[string]string) string {
	for {
		binary, ok := expr.(*ast.BinaryExpr)
		if !ok || binary.Op != token.ADD {
			break
		}

		expr = binary.X
	}

	if ident, ok := expr.(*ast.Ident); ok {
		return idents[ident.Name]
	}

	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return ""
	}

	switch ident.Name {
	case "crossKindURL":
		return "collection"
	case "collectionURL":
		return "ofKind"
	case "recordURL":
		return "one"
	case "courseMedicationListURL":
		return "courseList"
	case "courseMedicationItemURL":
		return "courseItem"
	default:
		return ""
	}
}
