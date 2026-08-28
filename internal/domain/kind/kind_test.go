package kind

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three spellings, written out. research D-05 and cross-artifact finding H1
// are about a path segment that was singular in one document and plural in
// another, so a structural test alone would not catch the regression: something
// has to hold the literal. This is that something, and it is the only place
// outside kind.go that may spell them.
var wantSpellings = map[Kind]struct {
	enum       string
	segment    string
	collection string
}{
	Medication: {enum: "medication", segment: "medications", collection: "medications"},
}

func TestTheDeclaredSpellingsAreTheDocumentedOnes(t *testing.T) {
	t.Parallel()

	for k, want := range wantSpellings {
		t.Run(want.enum, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want.enum, k.Enum(), "the enum spelling is snake_case singular")
			assert.Equal(t, want.segment, k.Segment(), "the path segment is plural kebab-case (research D-05)")
			assert.Equal(t, want.collection, k.Collection())
			assert.True(t, k.Valid())
		})
	}
}

// The mapping is TOTAL: every Kind that exists has all three spellings, and
// every Kind that exists is in Kinds(). The source file is what "exists" means,
// because a constant declared and left out of the table compiles perfectly and
// then serves 404s.
func TestTheMappingIsTotalOverEveryDeclaredKind(t *testing.T) {
	t.Parallel()

	declared := declaredKindConstants(t, "kind.go")
	require.NotEmpty(t, declared, "parsed no Kind constants out of kind.go — the parser is broken, not the file")

	listed := map[Kind]bool{}
	for _, k := range Kinds() {
		listed[k] = true
	}
	assert.Len(t, Kinds(), len(declared), "Kinds() and the constants in kind.go disagree on how many kinds exist")

	for name, k := range declared {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Truef(t, listed[k], "%s is declared but Kinds() does not return it", name)
			assert.NotEmptyf(t, k.Enum(), "%s has no enum spelling", name)
			assert.NotEmptyf(t, k.Segment(), "%s has no path segment", name)
			assert.NotEmptyf(t, k.Collection(), "%s has no collection", name)
			assert.Truef(t, k.Valid(), "%s is declared but does not report itself valid", name)
			assert.Containsf(t, wantSpellings, k, "%s has no expected spelling in this test's table", name)
		})
	}
}

// INJECTIVE in both directions. Two kinds sharing a path segment means one of
// them is unreachable; two sharing a collection means one of them writes into
// the other's rows.
func TestTheMappingIsInjective(t *testing.T) {
	t.Parallel()

	spellings := map[string]func(Kind) string{
		"enum":       Kind.Enum,
		"segment":    Kind.Segment,
		"collection": Kind.Collection,
	}

	for name, spelling := range spellings {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			seen := map[string]Kind{}
			for _, k := range Kinds() {
				value := spelling(k)
				previous, clash := seen[value]
				assert.Falsef(t, clash, "%s and %s share the %s %q", previous, k, name, value)
				seen[value] = k
			}
			assert.Len(t, seen, len(Kinds()))
		})
	}
}

func TestEverySpellingRoundTripsBackToItsKind(t *testing.T) {
	t.Parallel()

	lookups := map[string]struct {
		spelling func(Kind) string
		parse    func(string) (Kind, bool)
	}{
		"enum":       {spelling: Kind.Enum, parse: FromEnum},
		"segment":    {spelling: Kind.Segment, parse: FromSegment},
		"collection": {spelling: Kind.Collection, parse: FromCollection},
	}

	for name, lookup := range lookups {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, k := range Kinds() {
				spelled := lookup.spelling(k)

				parsed, known := lookup.parse(spelled)
				require.Truef(t, known, "the %s %q does not map back to a kind", name, spelled)
				assert.Equalf(t, k, parsed, "%s round-tripped through its %s into %s", k, name, parsed)

				// And the other way round, so the pair is a bijection rather
				// than two tables that happen to agree on the values tried.
				assert.Equalf(t, spelled, lookup.spelling(parsed), "the %s did not survive the round trip", name)
			}
		})
	}
}

func TestNothingButADeclaredKindResolves(t *testing.T) {
	t.Parallel()

	// A differently-cased or singular segment is a different route, not a
	// forgiving spelling of this one: PocketBase does no normalisation and the
	// generic record handler answers an unknown segment with 404.
	strangers := []string{
		"", "medication", "Medications", "MEDICATIONS", "medications/", "/medications", " medications",
	}

	for _, stranger := range strangers {
		t.Run("segment "+stranger, func(t *testing.T) {
			t.Parallel()

			unknown, known := FromSegment(stranger)
			assert.False(t, known, "%q resolved to a kind", stranger)
			assert.Equal(t, Kind(""), unknown)
		})
	}

	t.Run("an undeclared Kind carries no spellings", func(t *testing.T) {
		t.Parallel()

		invented := Kind("family_member")

		assert.False(t, invented.Valid())
		assert.Empty(t, invented.Segment())
		assert.Empty(t, invented.Collection())

		unknown, known := FromEnum(string(invented))
		assert.False(t, known)
		assert.Equal(t, Kind(""), unknown, "a lookup that failed handed back a usable-looking kind")
	})
}

// Kinds() hands out the registry, and a caller that sorts or truncates the
// result must not be able to change what every other caller sees.
func TestKindsCannotBeMutatedByItsCallers(t *testing.T) {
	t.Parallel()

	first := Kinds()
	require.NotEmpty(t, first)

	first[0] = Kind("mutated")

	assert.NotEqual(t, first, Kinds())
	assert.Equal(t, Medication, Kinds()[0])
}

// By AST rather than by reflection, because a package cannot enumerate its own
// constants at run time and the whole point is to catch the declaration that
// was never wired into the table.
func declaredKindConstants(t *testing.T, file string) map[string]Kind {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)

	found := map[string]Kind{}
	for _, decl := range parsed.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		// A const block carries the type of the last spec that named one.
		var blockType string
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if ident, ok := value.Type.(*ast.Ident); ok {
				blockType = ident.Name
			}
			if blockType != "Kind" {
				continue
			}
			for i, name := range value.Names {
				if !name.IsExported() || i >= len(value.Values) {
					continue
				}
				literal, ok := value.Values[i].(*ast.BasicLit)
				require.Truef(t, ok, "%s is not declared as a string literal", name.Name)
				found[name.Name] = Kind(strings.Trim(literal.Value, `"`))
			}
		}
	}
	return found
}
