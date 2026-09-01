package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageEnvelopeMatchesTheContract(t *testing.T) {
	t.Parallel()

	cursor := "eyJzIjpbImlkIl19.signature"

	tests := []struct {
		name string
		page Page[string]
		want string
	}{
		{
			// Go 1.27's encoding/json/v2 retrofit is not backward compatible on
			// nil-versus-empty slices, and a client that iterates items without
			// a null check breaks on the first empty page (research D-28).
			name: "an empty page carries an empty array, never null",
			page: NewPage[string](nil, nil),
			want: `{"items":[],"next_cursor":null}`,
		},
		{
			name: "the last page has no cursor, and the member is still present",
			page: NewPage([]string{"a", "b"}, nil),
			want: `{"items":["a","b"],"next_cursor":null}`,
		},
		{
			name: "a page with more behind it carries the opaque cursor",
			page: NewPage([]string{"a"}, &cursor),
			want: `{"items":["a"],"next_cursor":"` + cursor + `"}`,
		},
		{
			name: "total appears only when it was asked for",
			page: NewPage([]string{"a"}, nil).WithTotal(41),
			want: `{"items":["a"],"next_cursor":null,"total":41}`,
		},
		{
			// omitempty on a *int and a total of zero is the trap: ?count=true
			// over an empty result must answer 0, not leave the member out.
			name: "a total of zero is reported, not omitted",
			page: NewPage[string](nil, nil).WithTotal(0),
			want: `{"items":[],"next_cursor":null,"total":0}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(test.page)
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(raw))
		})
	}
}

func TestPageRoundTripsWithAStructItem(t *testing.T) {
	t.Parallel()

	type summary struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	cursor := "opaque"
	original := NewPage([]summary{{ID: "abc123", Name: "one"}}, &cursor).WithTotal(1)

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Page[summary]
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, original.Items, decoded.Items)
	require.NotNil(t, decoded.NextCursor)
	assert.Equal(t, cursor, *decoded.NextCursor)
	require.NotNil(t, decoded.Total)
	assert.Equal(t, 1, *decoded.Total)
}

// NewPage copies nothing it was not given: a caller that keeps its slice must
// not be able to change a page it already handed away.
func TestNewPageDoesNotAliasTheCallersSlice(t *testing.T) {
	t.Parallel()

	items := []string{"a", "b"}
	page := NewPage(items, nil)
	items[0] = "mutated"

	assert.Equal(t, []string{"a", "b"}, page.Items)
}

func TestSortKeySpelling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  SortKey
		valid bool
	}{
		{name: "ascending", input: "started_on", want: SortKey{Field: "started_on"}, valid: true},
		{name: "descending", input: "-started_on", want: SortKey{Field: "started_on", Desc: true}, valid: true},
		{name: "the id tiebreaker", input: "-id", want: SortKey{Field: "id", Desc: true}, valid: true},
		{name: "empty"},
		{name: "a bare minus", input: "-"},
		{name: "two minuses", input: "--started_on"},
		{name: "a trailing space", input: "started_on "},
		{name: "a leading space", input: " started_on"},
		{name: "an inner space", input: "started on"},
		{name: "an ascending marker", input: "+started_on"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			key, err := ParseSortKey(test.input)
			if !test.valid {
				require.Error(t, err)
				assert.Equal(t, SortKey{}, key)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, key)

			// The `-` prefix is the one spelling, so what was parsed renders
			// back to what arrived and a cursor built from it is stable.
			assert.Equal(t, test.input, key.String())

			reparsed, err := ParseSortKey(key.String())
			require.NoError(t, err)
			assert.Equal(t, key, reparsed)
		})
	}
}
