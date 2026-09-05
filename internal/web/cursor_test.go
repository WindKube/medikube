package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// The sort allowlist contracts/records.md publishes for the one kind this phase
// registers, in the order OpenAPI publishes it. The first entry is the default.
func allowedSorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: "started_on", Desc: true},
		{Field: "started_on"},
		{Field: "name"},
		{Field: "name", Desc: true},
		{Field: "updated", Desc: true},
		{Field: "updated"},
	}
}

func testCursors(t *testing.T, key string) *Cursors {
	t.Helper()

	codec, err := store.NewCursorCodec(strings.Repeat(key, store.MinCursorSecretLength))
	require.NoError(t, err)

	return NewCursors(codec)
}

func TestACursorRoundTripsThroughTheEdge(t *testing.T) {
	t.Parallel()

	cursors := testCursors(t, "k")
	scope := CursorScope(kind.Medication, testsupport.AccountAID)
	sort := []domain.SortKey{{Field: "started_on", Desc: true}}

	token, err := cursors.Encode(scope, store.Cursor{Sort: sort, Values: []string{"2026-01-01"}, ID: testsupport.NameOnlyMedicationID})
	require.NoError(t, err)
	require.NotEmpty(t, token)

	decoded, err := cursors.Decode(scope, sort, token)
	require.NoError(t, err)
	assert.Equal(t, testsupport.NameOnlyMedicationID, decoded.ID)
	assert.Equal(t, []string{"2026-01-01"}, decoded.Values)
}

// D-29: the boundary value for the by-name ordering IS a drug name, and a
// cursor travels in a query string that reaches the browser history, the
// Referer header and every reverse-proxy access log.
func TestACursorDisclosesNeitherItsBoundaryNorWhoseListItIs(t *testing.T) {
	t.Parallel()

	cursors := testCursors(t, "k")
	sort := []domain.SortKey{{Field: "name"}}

	token, err := cursors.Encode(
		CursorScope(kind.Medication, testsupport.AccountAID),
		store.Cursor{Sort: sort, Values: []string{"Amoxicillin"}, ID: testsupport.NameOnlyMedicationID},
	)
	require.NoError(t, err)

	assert.NotContains(t, token, "Amoxicillin")
	assert.NotContains(t, token, testsupport.AccountAID)
	assert.NotContains(t, token, testsupport.NameOnlyMedicationID)
	assert.NotContains(t, token, kind.Medication.Segment())
}

// A forged, tampered or unparseable cursor is 400 invalid_cursor — one refusal
// for every way of failing, because telling a client which check failed tells
// it how to get closer.
func TestEveryWayOfNotBeingAnIssuedCursorIsTheSameRefusal(t *testing.T) {
	t.Parallel()

	cursors := testCursors(t, "k")
	scope := CursorScope(kind.Medication, testsupport.AccountAID)
	sort := []domain.SortKey{{Field: "started_on", Desc: true}}

	issued, err := cursors.Encode(scope, store.Cursor{Sort: sort, Values: []string{"2026-01-01"}, ID: testsupport.NameOnlyMedicationID})
	require.NoError(t, err)

	other, err := testCursors(t, "j").Encode(scope, store.Cursor{Sort: sort, Values: []string{"2026-01-01"}, ID: testsupport.NameOnlyMedicationID})
	require.NoError(t, err)

	cases := []struct {
		name  string
		scope string
		sort  []domain.SortKey
		token string
	}{
		{"not base64", scope, sort, "not a cursor at all"},
		{"truncated", scope, sort, issued[:len(issued)/2]},
		{"empty", scope, sort, ""},
		{"one byte flipped", scope, sort, flip(issued)},
		{"minted under another key", scope, sort, other},
		{"replayed against another owner", CursorScope(kind.Medication, testsupport.AccountBID), sort, issued},
		{"handed back under another ordering", scope, []domain.SortKey{{Field: "name"}}, issued},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			_, err := cursors.Decode(one.scope, one.sort, one.token)
			require.Error(t, err)
			require.ErrorIs(t, err, store.ErrInvalidCursor)

			status, code := Classify(err)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, CodeInvalidCursor, code)

			assert.NotContains(t, err.Error(), one.name,
				"the refusal says which check failed, which tells a client how to get closer")
		})
	}
}

// A scope has to be one string per (kind, owner) pair and it has to be
// unambiguous: two different pairs that render to the same scope would let one
// person's cursor continue another's list.
func TestACursorScopeIsUnambiguous(t *testing.T) {
	t.Parallel()

	scopes := map[string]string{}

	for _, k := range kind.Kinds() {
		for _, owner := range []string{testsupport.AccountAID, testsupport.AccountBID, ""} {
			scope := CursorScope(k, owner)
			assert.NotEmpty(t, scope)

			key := string(k) + "\x00" + owner
			for existing, from := range scopes {
				assert.NotEqualf(t, existing, scope, "%s and %s share a cursor scope", from, key)
			}
			scopes[scope] = key
		}
	}
}

// The cursor binds the RESOLVED ordering into its associated data, so the edge
// has to resolve `?sort=` before it decodes — the raw parameter is absent on
// every request after the first page.
func TestTheListQueryResolvesTheSortTheCursorWasSealedWith(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodGet, "/x")

	params, err := ListQuery(e, allowedSorts())
	require.NoError(t, err)
	assert.Equal(t, []domain.SortKey{allowedSorts()[0]}, params.Sort,
		"an absent ?sort= did not resolve to the kind's default, so page two seals a different ordering from page one")
	assert.Equal(t, DefaultLimit, params.Limit)
	assert.Empty(t, params.Cursor)
	assert.False(t, params.Count)
}

func TestTheListQueryReadsEveryDocumentedParameter(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodGet, "/x?limit=100&cursor=abc&sort=name,-updated&count=true&q=amox")

	params, err := ListQuery(e, allowedSorts())
	require.NoError(t, err)

	assert.Equal(t, 100, params.Limit)
	assert.Equal(t, "abc", params.Cursor)
	assert.Equal(t, []domain.SortKey{{Field: "name"}, {Field: "updated", Desc: true}}, params.Sort)
	assert.True(t, params.Count)
	assert.Equal(t, "amox", params.Search)
}

// A sort outside the allowlist is 422 invalid_value and never silently ignored,
// because a silently ignored sort produces a list that looks right and is not.
func TestAParameterOutsideItsVocabularyIsRefusedAndNotIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		query string
		field string
	}{
		{"a sort nobody publishes", "?sort=owner", ParamSort},
		{"the filter DSL", `?sort=name~"a"`, ParamSort},
		{"an empty sort term", "?sort=name,", ParamSort},
		{"a limit below the floor", "?limit=0", ParamLimit},
		{"a limit above the ceiling", "?limit=101", ParamLimit},
		{"a limit that is not a number", "?limit=all", ParamLimit},
		{"a negative limit", "?limit=-1", ParamLimit},
		{"a count that is not a boolean", "?count=yes", ParamCount},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			e, _ := event(t, http.MethodGet, "/x"+one.query)

			_, err := ListQuery(e, allowedSorts())
			require.Error(t, err, "the parameter was silently ignored, so the list looks right and is not")

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, one.field, invalid.Fields[0].Field)
			assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)

			status, code := Classify(err)
			assert.Equal(t, http.StatusUnprocessableEntity, status)
			assert.Equal(t, domain.CodeValidationFailed, code)
		})
	}
}

// FR-027: every problem in one response, not the first one found.
func TestTwoBadParametersProduceTwoFieldErrors(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodGet, "/x?limit=0&sort=owner")

	_, err := ListQuery(e, allowedSorts())
	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Len(t, invalid.Fields, 2, "the edge stopped at the first bad parameter")
}

func TestARefusedParameterNeverRepeatsWhatWasSent(t *testing.T) {
	t.Parallel()

	secret := "Amoxicillin"

	e, _ := event(t, http.MethodGet, "/x?sort="+secret)

	_, err := ListQuery(e, allowedSorts())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret,
		"the rejected value is in the error, and the error is logged")
}

// T039, research D-29: every phase-002 list ends its default sort on `id`,
// the mandatory tiebreaker — two rows sharing every other sorted column (twins,
// a father and son with the same name) would otherwise make a cursor ambiguous.
func TestEveryPhase002SortEndsInTheIDTiebreaker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sort []domain.SortKey
	}{
		{name: "patients", sort: PatientsSort()},
		{name: "practitioners", sort: PractitionersSort()},
		{name: "facilities", sort: FacilitiesSort()},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			require.NotEmpty(t, testCase.sort)
			last := testCase.sort[len(testCase.sort)-1]
			assert.Equal(t, "id", last.Field, "the tiebreaker must be the final term, not merely present")
			assert.False(t, last.Desc, "id ascending is what makes the tiebreaker deterministic")
		})
	}
}

// research D-29's table, verbatim.
func TestThePhase002DefaultSortsMatchResearchD29(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []domain.SortKey{{Field: "last_name"}, {Field: "first_name"}, {Field: "id"}}, PatientsSort())
	assert.Equal(t, []domain.SortKey{{Field: "name"}, {Field: "id"}}, PractitionersSort())
	assert.Equal(t, []domain.SortKey{{Field: "kind"}, {Field: "name"}, {Field: "id"}}, FacilitiesSort())
}

// flip changes one byte of a token so the AEAD open fails on authentication
// rather than on decoding.
func flip(token string) string {
	raw := []byte(token)
	if raw[0] == 'A' {
		raw[0] = 'B'
	} else {
		raw[0] = 'A'
	}

	return string(raw)
}
