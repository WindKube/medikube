package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// T082, FR-023. A cursor is a boundary *in an ordering*. Handed back under a
// different ordering it names a row that is somewhere else entirely in the new
// sequence, so continuing from it silently skips everything between here and
// there and repeats everything before it. Nothing about the resulting page
// looks wrong: the rows are real, the count is right, and the entries the
// person never saw are simply gone.
//
// So the ordering is authenticated rather than compared. It goes into the AEAD
// as associated data, which means a cursor opened under the wrong sort fails to
// authenticate at all — there is no branch that a later refactor can forget to
// take.
func TestACursorIsRejectedUnderAnyOrderingButTheOneItWasIssuedFor(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	issued := []domain.SortKey{{Field: "started_on", Desc: true}}

	token, err := codec.Encode(testScope, Cursor{
		Sort:   issued,
		Values: []string{"2026-03-01"},
		ID:     "abcdefghij12345",
	})
	require.NoError(t, err)

	// The positive control first: the ordering it was issued for still works,
	// so a test that rejected everything would not pass this file.
	decoded, err := codec.Decode(testScope, issued, token)
	require.NoError(t, err)
	require.Equal(t, issued, decoded.Sort)

	cases := []struct {
		name string
		sort []domain.SortKey
	}{
		{
			name: "another field, which is FR-022's second ordering",
			sort: []domain.SortKey{{Field: "name"}},
		},
		{
			name: "the same field reversed, where the boundary is at the wrong end of the list",
			sort: []domain.SortKey{{Field: "started_on", Desc: false}},
		},
		{
			name: "no ordering at all",
			sort: nil,
		},
		{
			name: "the ordering with a term appended",
			sort: []domain.SortKey{{Field: "started_on", Desc: true}, {Field: "name"}},
		},
		{
			name: "the ordering with a term prepended",
			sort: []domain.SortKey{{Field: "name"}, {Field: "started_on", Desc: true}},
		},
		{
			name: "a field whose name is a prefix of the issued one",
			sort: []domain.SortKey{{Field: "started", Desc: true}},
		},
		{
			name: "a field whose name is the issued one with a suffix",
			sort: []domain.SortKey{{Field: "started_on_utc", Desc: true}},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rejected, decodeErr := codec.Decode(testScope, testCase.sort, token)
			require.ErrorIs(t, decodeErr, ErrInvalidCursor,
				"the cursor paged through a different sequence instead of being refused")
			assert.Equal(t, Cursor{}, rejected)
		})
	}
}

// The three orderings FR-022 publishes, each proved to be a separate keyspace.
// Every cursor validates under its own ordering and under no other, which is
// the whole matrix rather than one representative pair.
func TestEachPublishedOrderingIsItsOwnKeyspace(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	orderings := map[string][]domain.SortKey{
		"most recently started": {{Field: "started_on", Desc: true}},
		"by name":               {{Field: "name"}},
		"most recently changed": {{Field: "updated", Desc: true}},
	}

	tokens := make(map[string]string, len(orderings))
	for label, sort := range orderings {
		token, err := codec.Encode(testScope, Cursor{
			Sort:   sort,
			Values: []string{"boundary"},
			ID:     "abcdefghij12345",
		})
		require.NoError(t, err)
		tokens[label] = token
	}

	for issuedUnder, token := range tokens {
		for readUnder, sort := range orderings {
			t.Run(issuedUnder+" read under "+readUnder, func(t *testing.T) {
				t.Parallel()

				_, err := codec.Decode(testScope, sort, token)
				if issuedUnder == readUnder {
					assert.NoError(t, err)
					return
				}
				assert.ErrorIs(t, err, ErrInvalidCursor)
			})
		}
	}
}

// The sort spelling that is authenticated has to be unambiguous. Two different
// orderings whose terms concatenate to the same text would share a keyspace,
// and a cursor would cross between them without being refused.
func TestTwoOrderingsCannotShareASpelling(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	// `-a` then `b` versus `-ab`: the naive spelling is "-a" + "b" == "-ab".
	twoTerms := []domain.SortKey{{Field: "a", Desc: true}, {Field: "b"}}
	oneTerm := []domain.SortKey{{Field: "ab", Desc: true}}

	token, err := codec.Encode(testScope, Cursor{
		Sort:   twoTerms,
		Values: []string{"1", "2"},
		ID:     "abcdefghij12345",
	})
	require.NoError(t, err)

	_, err = codec.Decode(testScope, oneTerm, token)
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

// The same guard on the scope. A cursor is scoped to the query it continues,
// and for an owner-scoped list that includes whose list it was — so a cursor
// lifted from one person's page cannot be replayed against another's, even
// before the repository's own owner filter runs.
func TestTwoScopesCannotShareASpelling(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	list := kind.Medication.Collection()

	token, err := codec.Encode(list+":aa", Cursor{ID: "abcdefghij12345"})
	require.NoError(t, err)

	for _, scope := range []string{list + ":ab", list + ":a", list + ":aaa", list, ""} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()

			_, decodeErr := codec.Decode(scope, nil, token)
			assert.ErrorIs(t, decodeErr, ErrInvalidCursor)
		})
	}
}
