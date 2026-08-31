package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T205's second layer, and the one that is easy to leave out.
//
// Every hook MediKube binds into PocketBase — the record audit, the sign-in
// audit, the realtime publisher — is bound in TWO places: the composition root
// the binary runs, and the one internal/web/apitest builds for the HTTP suite.
// Every behavioural test in that suite drives the second. So a binding deleted
// from the binary alone changes nothing any test can see: the audit trail of a
// real deployment stops recording sign-ins, and `go test ./...` stays green.
// That is not a hypothetical — it is what deleting either audit binding from
// cmd/medikube/handlers.go did before this file existed.
//
// The two are held together here instead. Between them the layers cover both
// directions: delete a binding from one root and this test fails; delete it
// from both and the HTTP suite fails, because the harness no longer binds what
// its assertions are about. Neither on its own is enough, which is the whole
// reason this is a separate file rather than one more assertion somewhere.
const (
	binaryRoot  = "../../cmd/medikube"
	harnessRoot = "../../internal/web/apitest"
)

func TestTheBinaryAndTheHTTPHarnessBindTheSamePlatformHooks(t *testing.T) {
	t.Parallel()

	binary := platformBindings(t, binaryRoot)
	harness := platformBindings(t, harnessRoot)

	// Vacuously equal is the failure this guards against second: two roots that
	// bind nothing agree perfectly. The HTTP suite is what fails in that case,
	// and this keeps the pair from looking healthy while it does.
	require.NotEmpty(t, binary, "%s binds no platform hooks at all", binaryRoot)

	assert.Equal(t, binary, harness,
		"the binary and the HTTP harness no longer bind the same hooks.\n"+
			"A hook only the harness binds is a hook the suite proves and the deployment does not have;\n"+
			"one only the binary binds is a hook nothing tests.")
}

// platformBindings is every pb.Bind… called in one composition root, sorted.
//
// The whole package is read rather than one file, because which file of a root
// binds what is an arrangement rather than a contract: the binary binds the
// server in main.go and the hooks in handlers.go, and moving one between them
// is not a change this test has an opinion about.
//
// It reads the call and not the import, because what matters is which hooks are
// actually bound: a root that imports internal/platform/pb and binds nothing is
// exactly the failure above.
func platformBindings(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "cannot read the composition root at %s", dir)

	var bound []string

	inspect := func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}

		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}

		pkg, isIdent := selector.X.(*ast.Ident)
		if !isIdent || pkg.Name != "pb" || !strings.HasPrefix(selector.Sel.Name, "Bind") {
			return true
		}

		bound = append(bound, selector.Sel.Name)

		return true
	}

	var read int

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)

		ast.Inspect(file, inspect)

		read++
	}

	require.Positive(t, read, "no Go source at %s", dir)

	sort.Strings(bound)

	return bound
}
