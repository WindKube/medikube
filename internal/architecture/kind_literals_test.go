package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// T046, the mechanical form of research D-05. A kind has three spellings —
// the enum value, the plural path segment and the collection — declared side by
// side in one table precisely because they will not always agree: `insurance`
// and `family-history` are not mechanical plurals of anything. Cross-artifact finding H1 is what happens when a second file
// spells one of them by hand — a route says one thing, a page says another, and
// nothing fails until somebody clicks.
//
// So the spelling exists once. Everywhere else calls Segment() or Collection().
const kindDeclaration = "internal/domain/kind/kind.go"

// Files that may hold the literal anyway, each because pinning the spelling is
// the whole point of the file. Anything not listed here calls the accessor.
var kindLiteralExempt = map[string]string{
	"internal/domain/kind/kind_test.go": "the golden table: the one place that pins each kind's plural spelling, which is finding H1's fix",
	"internal/domain/audit/enums_test.go": "the near-miss list: it asserts the plural is refused as an audit target kind, " +
		"which is the same drift caught from the other side",
}

func TestNoFileOutsideTheKindTableSpellsAKindSegmentOrCollection(t *testing.T) {
	t.Parallel()

	// One entry per spelling, not per role: segment and collection are the same
	// string today, and a message naming only one of them would send the reader
	// to the wrong accessor.
	spellings := map[string][]string{}
	for _, k := range kind.Kinds() {
		require.NotEmpty(t, k.Segment(), "%s has no segment", k)
		require.NotEmpty(t, k.Collection(), "%s has no collection", k)

		spellings[k.Segment()] = append(spellings[k.Segment()], string(k)+"'s path segment")
		spellings[k.Collection()] = append(spellings[k.Collection()], string(k)+"'s collection")
	}
	require.NotEmpty(t, spellings)

	var (
		offences []string
		scanned  int
	)

	root := repoRoot(t)
	fset := token.NewFileSet()

	walkRepo(t, root, func(rel string, _ fs.DirEntry) {
		if filepath.Ext(rel) != ".go" || rel == kindDeclaration {
			return
		}

		if _, exempt := kindLiteralExempt[rel]; exempt {
			return
		}

		scanned++

		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "parsing %s", rel)

		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}

			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil {
				return true
			}

			// Contained, not equal: `/api/v1/records/medications` is the
			// hardcoded route this exists to stop, and it is never the whole
			// literal.
			for spelling, roles := range spellings {
				if strings.Contains(value, spelling) {
					offences = append(offences, fset.Position(literal.Pos()).String()+
						": the literal spells "+strings.Join(roles, " and ")+
						" — call Segment() or Collection() on the kind (research D-05)")
				}
			}

			return true
		})
	})

	require.Greater(t, scanned, 20, "the walk found almost nothing; it is not looking where it thinks it is")

	sort.Strings(offences)
	assert.Empty(t, offences)
}
