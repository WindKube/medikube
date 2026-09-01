package openapi_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/openapi"
)

// T098, FR-065, SC-011. The gate runs in BOTH directions and either asymmetry
// fails: a route the document forgot is an undocumented operation, and an
// operation the registry does not serve is a promise nothing keeps. Neither is
// visible from the other side, which is why one assertion cannot stand in for
// the other.

// documentedOperation is one (method, path, operation) triple recovered from a
// generated document — the same triple httproute.Route carries.
type documentedOperation struct {
	method string
	path   string
	op     *openapi3.Operation
}

// operationsByID enumerates a document the way a client would: every path, every
// method on it, keyed by operationId. Paths.Keys() is used rather than
// InMatchingOrder() because Keys() is sorted and a gate that reorders itself
// between runs is one nobody can diff.
func operationsByID(t *testing.T, doc *openapi3.T) map[string]documentedOperation {
	t.Helper()

	require.NotNil(t, doc.Paths)

	found := make(map[string]documentedOperation)

	for _, path := range doc.Paths.Keys() {
		item := doc.Paths.Value(path)
		require.NotNilf(t, item, "path %s has no item", path)

		for method, op := range item.Operations() {
			require.NotEmptyf(t, op.OperationID, "%s %s carries no operationId", method, path)

			previous, duplicate := found[op.OperationID]
			require.Falsef(t, duplicate,
				"operationId %q is on both %s %s and %s %s",
				op.OperationID, previous.method, previous.path, method, path)

			found[op.OperationID] = documentedOperation{method: method, path: path, op: op}
		}
	}

	return found
}

// documentPath is the one translation between the registry's paths and the
// document's, transcribed here by hand rather than borrowed from the generator
// so the gate has its own opinion. Go's ServeMux spells a trailing wildcard
// `{path...}`; OpenAPI has no such form and would read the parameter's name as
// `path...`.
func documentPath(routePath string) string {
	return strings.ReplaceAll(routePath, "...}", "}")
}

func TestEveryRegisteredRouteAppearsInTheDocument(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))
	documented := operationsByID(t, loaded)

	for _, route := range documentedRoutes(t) {
		t.Run(route.OpID, func(t *testing.T) {
			t.Parallel()

			op, present := documented[route.OpID]
			require.Truef(t, present,
				"%s (%s) is registered but does not appear in the document", route.OpID, route.Pattern())

			assert.Equal(t, route.Method, op.method, "%s is documented under the wrong method", route.OpID)
			assert.Equal(t, documentPath(route.Path), op.path, "%s is documented under the wrong path", route.OpID)
		})
	}
}

func TestEveryDocumentedOperationIsRegistered(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	registered := make(map[string]httproute.Route)
	for _, route := range documentedRoutes(t) {
		registered[route.OpID] = route
	}

	for opID, op := range operationsByID(t, loaded) {
		route, present := registered[opID]
		require.Truef(t, present,
			"%s (%s %s) is documented but no route is registered under that operationId", opID, op.method, op.path)

		assert.Equal(t, route.Method, op.method)
		assert.Equal(t, documentPath(route.Path), op.path)
	}
}

// The two directions above are equivalent only when the sets are also the same
// size, which neither loop asserts on its own: a document holding every route
// plus one extra passes the first, and a registry holding every operation plus
// one extra passes the second.
func TestTheDocumentAndTheRegistryHoldTheSameOperations(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	routes := documentedRoutes(t)
	registered := make([]string, 0, len(routes))

	for _, route := range routes {
		registered = append(registered, route.OpID)
	}

	operations := operationsByID(t, loaded)
	documented := make([]string, 0, len(operations))

	for opID := range operations {
		documented = append(documented, opID)
	}

	assert.ElementsMatch(t, registered, documented)
}

// Pages are the browser gate's inventory, not the API's. They carry no DTO and
// no operationId a client could call, so a page in api/openapi.json is a
// promise of an interface that does not exist.
func TestNoPageOrAssetRouteIsDocumented(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))
	documented := operationsByID(t, loaded)

	var pages int

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage && route.Kind != httproute.KindAsset {
			continue
		}

		pages++

		_, present := documented[route.OpID]
		assert.Falsef(t, present, "%s is a %s and must not appear in api/openapi.json", route.OpID, route.Kind)
	}

	require.Positive(t, pages, "no page was registered; this test asserted nothing")
}

// The other half of the same rule, from the document's side: nothing under a
// path outside /api/v1 may be documented except the PocketBase-native paths
// contracts/README.md deliberately leaves reachable.
func TestOnlyTheDocumentedExternalsLiveOutsideTheAPIBase(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	externals := make(map[string]struct{})

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			externals[route.OpID] = struct{}{}
		}
	}

	require.NotEmpty(t, externals)

	for opID, op := range operationsByID(t, loaded) {
		if strings.HasPrefix(op.path, "/api/v1/") {
			continue
		}

		_, external := externals[opID]
		assert.Truef(t, external, "%s is documented at %s, which is outside /api/v1 and is not a documented external", opID, op.path)
	}
}

// A route nobody documented is the first half of the gate made unavoidable at
// generation time: the document cannot be produced at all, so the failure is a
// boot failure and never a quietly thinner api/openapi.json.
func TestGenerateRefusesARouteItHasNoDocumentationFor(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	in.Routes = append(slices.Clone(in.Routes), httproute.Route{
		OpID:    "operationNobodyDocumented",
		Method:  http.MethodGet,
		Path:    "/api/v1/undocumented",
		Kind:    httproute.KindAPI,
		Auth:    httproute.AuthUser,
		Summary: "A route added without a documentation entry.",
	})

	_, err := openapi.Generate(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "operationNobodyDocumented")
}

// And the second half: a documentation entry no route serves.
func TestGenerateRefusesDocumentationForARouteNobodyServes(t *testing.T) {
	t.Parallel()

	in := twoKindInput()

	dropped := "getRecord"
	in.Routes = slices.DeleteFunc(slices.Clone(in.Routes), func(route httproute.Route) bool {
		return route.OpID == dropped
	})
	require.Len(t, in.Routes, len(twoKindInput().Routes)-1, "the route to drop was not in the inventory")

	_, err := openapi.Generate(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), dropped)
}
