package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T223i, FR-073's structural half at the edge.
//
// The behavioural half — three addresses, three answers compared byte for byte
// after masking request_id — is TestEveryRecoveryRequestIsAnsweredIdentically in
// password_reset_test.go, and it is not repeated here. What that test cannot say
// is WHY the three answers are the same: two branches producing identical bytes
// today drift apart in one edit tomorrow, and the edit that does it is somebody
// tidying the handler by returning early for an address with no account. So the
// shape of the handler is the assertion, read out of the source.
//
// Asserted structurally and never with a clock. A wall-clock tolerance nothing
// defines is the flaky gate Constitution VIII forbids; the latency is reported
// by the non-gating benchmark in timing_bench_test.go (T202a, ANALYSIS N13).

// The file and the method this reads. The handler is unexported, so it is
// reached through the source rather than through the package.
const (
	authSource      = "auth.go"
	recoveryHandler = "requestPasswordReset"

	// The Deps member the handler consults. A call whose receiver chain ends
	// in this field is the account being looked at, whatever the method is
	// called; anything else in the handler is decoding, configuration or the
	// answer.
	accountsMember = "Accounts"

	// The one response constructor. It is web's and not e.JSON's, which is
	// what makes "the same constructor on both branches" a countable thing.
	responseConstructor = "WriteJSON"

	// The value that constructor writes. FR-073's acknowledgement, built once.
	acknowledgementType = "Acknowledgement"
)

// TestTheRecoveryHandlerCannotTellTheTwoBranchesApart is the assertion T223i
// asks for: after the account is consulted the handler is straight-line to a
// single exit, so there is no branch for a "no such account" answer to live on
// and no second constructor for one to be built by.
//
// Every clause below fails on a real edit:
//
//   - a second call into the identity service is the lookup somebody adds in
//     order to branch on it;
//   - an if, a switch or a return between the consultation and the exit is the
//     early return itself;
//   - a second WriteJSON or a second Acknowledgement is the two branches each
//     building their own answer.
func TestTheRecoveryHandlerCannotTellTheTwoBranchesApart(t *testing.T) {
	t.Parallel()

	body := handlerBody(t, authSource, recoveryHandler)
	require.NotEmpty(t, body.List, "%s has an empty body, so everything below would pass having read nothing", recoveryHandler)

	consulted := -1
	consultations := 0

	for index, statement := range body.List {
		calls := serviceCalls(statement)
		if len(calls) == 0 {
			continue
		}

		if consulted < 0 {
			consulted = index
		}

		consultations += len(calls)
	}

	require.GreaterOrEqual(t, consulted, 0,
		"%s never calls h.deps.%s, so it is not the handler this test believes it is reading",
		recoveryHandler, accountsMember)
	require.Equal(t, 1, consultations,
		"%s consults the identity service %d times: a second lookup is how a handler learns which branch it is on",
		recoveryHandler, consultations)

	tail := body.List[consulted+1:]
	require.NotEmpty(t, tail,
		"the consultation is the last statement of %s, so the straight-line assertion below reads nothing", recoveryHandler)

	assert.Empty(t, branchesIn(tail),
		"%s branches after the account is consulted: %v. One of those arms is reached only by an address with no account, "+
			"which is the oracle FR-073 closes", recoveryHandler, branchesIn(tail))

	exits := 0

	for _, statement := range tail[:len(tail)-1] {
		ast.Inspect(statement, func(node ast.Node) bool {
			if _, isReturn := node.(*ast.ReturnStmt); isReturn {
				exits++
			}

			return true
		})
	}

	assert.Zero(t, exits,
		"%s returns %d times before its last statement: an early return on the no-account branch is exactly that edit",
		recoveryHandler, exits)

	final, isReturn := tail[len(tail)-1].(*ast.ReturnStmt)
	require.True(t, isReturn, "%s does not end in a return, so its answer is written somewhere this test cannot see", recoveryHandler)
	require.Len(t, final.Results, 1)

	assert.Equal(t, 1, callsNamed(body, responseConstructor),
		"%s writes its answer through %d calls to web.%s, so the branches can drift apart",
		recoveryHandler, callsNamed(body, responseConstructor), responseConstructor)
	assert.Equal(t, 1, literalsOf(body, acknowledgementType),
		"%s builds %d %s values: one address gets one of them and one gets the other",
		recoveryHandler, literalsOf(body, acknowledgementType), acknowledgementType)

	// The single exit IS the single construction, rather than merely being
	// accompanied by one somewhere else in the body.
	assert.Equal(t, 1, callsNamed(final, responseConstructor))
	assert.Equal(t, 1, literalsOf(final, acknowledgementType))
}

// serviceCalls reports the identity-service methods one statement calls, by the
// receiver chain rather than by the method name: what matters is that the
// account was looked at, not which question was asked about it.
func serviceCalls(node ast.Node) []string {
	var called []string

	ast.Inspect(node, func(found ast.Node) bool {
		call, isCall := found.(*ast.CallExpr)
		if !isCall {
			return true
		}

		method, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}

		if receiver, ok := method.X.(*ast.SelectorExpr); ok && receiver.Sel.Name == accountsMember {
			called = append(called, method.Sel.Name)
		}

		return true
	})

	return called
}

// branchesIn names every place control could take one path rather than another.
// A loop counts: `for _, address := range known` with a return inside it is an
// early exit wearing a different keyword.
func branchesIn(statements []ast.Stmt) []string {
	var found []string

	for _, statement := range statements {
		ast.Inspect(statement, func(node ast.Node) bool {
			switch node.(type) {
			case *ast.IfStmt:
				found = append(found, "if")
			case *ast.SwitchStmt:
				found = append(found, "switch")
			case *ast.TypeSwitchStmt:
				found = append(found, "type switch")
			case *ast.SelectStmt:
				found = append(found, "select")
			case *ast.ForStmt, *ast.RangeStmt:
				found = append(found, "for")
			}

			return true
		})
	}

	return found
}

func callsNamed(node ast.Node, name string) int {
	count := 0

	ast.Inspect(node, func(found ast.Node) bool {
		if call, isCall := found.(*ast.CallExpr); isCall {
			if selector, isSelector := call.Fun.(*ast.SelectorExpr); isSelector && selector.Sel.Name == name {
				count++
			}
		}

		return true
	})

	return count
}

func literalsOf(node ast.Node, name string) int {
	count := 0

	ast.Inspect(node, func(found ast.Node) bool {
		literal, isLiteral := found.(*ast.CompositeLit)
		if !isLiteral {
			return true
		}

		switch typed := literal.Type.(type) {
		case *ast.Ident:
			if typed.Name == name {
				count++
			}
		case *ast.SelectorExpr:
			if typed.Sel.Name == name {
				count++
			}
		}

		return true
	})

	return count
}

// handlerBody finds one method of this package in one of its own source files.
//
// It reads the source and not the compiled form because the thing being
// asserted is the shape of the code, which is what the next author edits. It
// fails rather than returning nothing when the method has moved: a walk that
// found nothing would otherwise make every assertion above pass vacuously.
func handlerBody(t *testing.T, file, method string) *ast.BlockStmt {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	for _, declaration := range parsed.Decls {
		function, isFunction := declaration.(*ast.FuncDecl)
		if !isFunction || function.Recv == nil || function.Name.Name != method {
			continue
		}

		require.NotNil(t, function.Body)

		return function.Body
	}

	t.Fatalf("%s declares no method %s; every assertion below would pass having read nothing", file, method)

	return nil
}
