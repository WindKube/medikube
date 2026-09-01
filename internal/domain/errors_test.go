package domain

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sentinels are the whole vocabulary the error mapper switches on, so the
// table is written out by hand: a sentinel that reaches production without a
// name here is a sentinel nothing above the domain knows how to answer.
func sentinelsByName(t *testing.T) map[string]error {
	t.Helper()

	return map[string]error{
		"ErrNotFound":        ErrNotFound,
		"ErrForbidden":       ErrForbidden,
		"ErrUnauthenticated": ErrUnauthenticated,
		"ErrVersionMismatch": ErrVersionMismatch,
		"ErrConflict":        ErrConflict,
		"ErrRateLimited":     ErrRateLimited,
	}
}

func TestEverySentinelIsDistinct(t *testing.T) {
	t.Parallel()

	named := sentinelsByName(t)

	for name, err := range named {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Error(t, err, "%s is nil", name)
			assert.NotEmpty(t, err.Error(), "%s carries no message", name)

			for otherName, other := range named {
				if otherName == name {
					continue
				}
				// errors.Is both ways: two distinct sentinels must never answer
				// for one another, or every 404 is also a 409.
				assert.Falsef(t, errors.Is(err, other), "%s matches %s", name, otherName)
				assert.Falsef(t, errors.Is(other, err), "%s matches %s", otherName, name)
				assert.NotEqualf(t, err.Error(), other.Error(), "%s and %s share a message", name, otherName)
			}
		})
	}
}

func TestSentinelsSurviveWrapping(t *testing.T) {
	t.Parallel()

	for name, sentinel := range sentinelsByName(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Every layer above the domain wraps with context before returning,
			// and the HTTP edge is several frames up. One %w that does not chain
			// turns a 404 into a 500.
			once := fmt.Errorf("store: reading record: %w", sentinel)
			twice := fmt.Errorf("service: %w", once)
			thrice := fmt.Errorf("handler: %w", twice)

			for depth, wrapped := range []error{once, twice, thrice} {
				assert.ErrorIsf(t, wrapped, sentinel, "%s lost at wrap depth %d", name, depth+1)
			}

			assert.Equal(t, sentinel, errors.Unwrap(once), "unwrapping one level did not reach the sentinel")
			assert.Contains(t, thrice.Error(), sentinel.Error(), "the wrapped message dropped the sentinel's own text")
		})
	}
}

// joined errors reach the mapper from the accumulating paths (a batch, a
// multi-step delete), and errors.Is has to see through the join too.
func TestSentinelsSurviveJoining(t *testing.T) {
	t.Parallel()

	joined := errors.Join(ErrNotFound, ErrRateLimited)

	assert.ErrorIs(t, joined, ErrNotFound)
	assert.ErrorIs(t, joined, ErrRateLimited)
	assert.NotErrorIs(t, joined, ErrConflict)
}

// A sentinel declared but left out of Sentinels() is invisible to the error
// mapper's completeness test, which is the only thing that proves every
// sentinel has a status and a machine code. So the source file is the source of
// truth for what exists, and this asserts the list agrees with it.
func TestSentinelsIsTotalOverTheSourceFile(t *testing.T) {
	t.Parallel()

	declared := declaredErrVars(t, "errors.go")
	require.NotEmpty(t, declared, "parsed no Err* variables out of errors.go — the parser is broken, not the file")

	listed := make(map[string]bool, len(Sentinels()))
	for _, err := range Sentinels() {
		listed[err.Error()] = true
	}
	assert.Len(t, Sentinels(), len(declared), "Sentinels() and errors.go disagree on how many sentinels exist")

	byName := sentinelsByName(t)
	for _, name := range declared {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			sentinel, known := byName[name]
			require.Truef(t, known, "%s is declared in errors.go but this test does not know it", name)
			assert.Truef(t, listed[sentinel.Error()], "%s is declared but missing from Sentinels()", name)
		})
	}
}

// Deliberately textual-by-AST rather than by reflection: a package cannot
// enumerate its own package-level variables at run time, and the point is to
// catch the declaration nobody wired up.
func declaredErrVars(t *testing.T, file string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)

	var names []string
	for _, decl := range parsed.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.VAR {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range value.Names {
				if strings.HasPrefix(name.Name, "Err") {
					names = append(names, name.Name)
				}
			}
		}
	}
	return names
}
