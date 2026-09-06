package search_test

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/service/search"
)

// seedTerm writes one row with a chosen title and body, for the term-matching
// tests below — seedRow's fixed "a title" says nothing about what is being
// searched for.
func (h harness) seedTerm(t *testing.T, k kind.Kind, recordID, title, body string, occurredOn *domain.Date) {
	t.Helper()

	collection, err := h.app.FindCachedCollectionByNameOrId("search_index")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set(indexFieldPatient, h.patient)
	record.Set(indexFieldKind, k.Enum())
	record.Set(indexFieldRecordID, recordID)
	record.Set("title", title)
	record.Set("body", body)

	if occurredOn != nil {
		record.Set(indexFieldOccurredOn, occurredOn.UTC())
	}

	require.NoError(t, h.app.Save(record))
}

func hitIDs(hits []search.Hit) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.RecordID)
	}

	return ids
}

func TestSearchKindMatchesTitleAndBodyCaseInsensitively(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	h.seedTerm(t, kind.Medication, recordID(1), "Warfarin", "", &on)
	h.seedTerm(t, kind.Medication, recordID(2), "Ibuprofen", "taken for WARFARIN interaction", &on)
	h.seedTerm(t, kind.Medication, recordID(3), "Paracetamol", "", &on)

	page, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "warfarin", nil, "", 10, "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{recordID(1), recordID(2)}, hitIDs(page.Items))
}

func TestSearchKindNarrowsByPatientAndKind(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	h.seedTerm(t, kind.Medication, recordID(1), "Warfarin", "", &on)
	h.seedTerm(t, kind.Allergy, recordID(2), "Warfarin allergy", "", &on)

	page, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "warfarin", nil, "", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recordID(1)}, hitIDs(page.Items))
}

// The term is escaped for LIKE before it is bound: a literal `%`, `_` or `\`
// in what somebody typed must match itself and not act as a wildcard or
// break the escape sequence (contracts/search.md §5).
func TestSearchKindEscapesLikeWildcardsAndTheEscapeCharacter(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	h.seedTerm(t, kind.Medication, recordID(1), "50% dose reduction", "", &on)
	h.seedTerm(t, kind.Medication, recordID(2), "50X dose reduction", "", &on)
	h.seedTerm(t, kind.Medication, recordID(3), "a_b marker", "", &on)
	h.seedTerm(t, kind.Medication, recordID(4), "aXb marker", "", &on)
	h.seedTerm(t, kind.Medication, recordID(5), `a\b path`, "", &on)

	percent, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "50%", nil, "", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recordID(1)}, hitIDs(percent.Items), "a literal %% must not act as a wildcard")

	underscore, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "a_b", nil, "", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recordID(3)}, hitIDs(underscore.Items), "a literal _ must not act as a single-char wildcard")

	backslash, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, `a\b`, nil, "", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recordID(5)}, hitIDs(backslash.Items), "a literal backslash must not break the escape sequence")
}

// A term that looks like PocketBase's own filter DSL, or like SQL, is matched
// as inert text — proof that it is bound as a parameter and never
// concatenated into a filter expression (contracts/search.md §5).
func TestSearchKindTreatsFilterAndSQLLookingTermsAsLiteralText(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	const poison = `' OR 1=1; owner != ""`

	h.seedTerm(t, kind.Medication, recordID(1), poison, "", &on)
	h.seedTerm(t, kind.Medication, recordID(2), "an unrelated title", "", &on)

	page, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, poison, nil, "", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recordID(1)}, hitIDs(page.Items))
}

func TestSearchKindOrdersAndPagesTheSameWayPageDoes(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	early := date(t, "2026-01-01")
	late := date(t, "2026-03-01")

	h.seedTerm(t, kind.Medication, recordID(1), "warfarin one", "", &early)
	h.seedTerm(t, kind.Medication, recordID(2), "warfarin two", "", &late)
	h.seedTerm(t, kind.Medication, recordID(3), "warfarin three", "", nil)

	first, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "warfarin", nil, "", 2, "")
	require.NoError(t, err)
	require.NotNil(t, first.NextCursor)
	assert.Equal(t, []string{recordID(2), recordID(1)}, hitIDs(first.Items))

	second, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "warfarin", nil, "", 2, *first.NextCursor)
	require.NoError(t, err)
	assert.Nil(t, second.NextCursor)
	assert.Equal(t, []string{recordID(3)}, hitIDs(second.Items))
}

// Each kind's cursor is its own: continuing group A's page with no cursor for
// group B, and vice versa, never cross-contaminates — the shape a grouped
// search's per-group pagination depends on (contracts/search.md §2).
func TestSearchKindCursorsAreScopedPerKind(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	h.seedTerm(t, kind.Medication, recordID(1), "shared term", "", &on)
	h.seedTerm(t, kind.Allergy, recordID(2), "shared term", "", &on)

	medPage, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "shared", nil, "", 10, "")
	require.NoError(t, err)
	assert.Nil(t, medPage.NextCursor)

	allergyPage, err := h.repo.SearchKind(ctx, h.patient, kind.Allergy, "shared", nil, "", 10, "")
	require.NoError(t, err)
	assert.Nil(t, allergyPage.NextCursor)

	assert.Equal(t, []string{recordID(1)}, hitIDs(medPage.Items))
	assert.Equal(t, []string{recordID(2)}, hitIDs(allergyPage.Items))
}

func TestSearchKindReadsTitleAndTagsForTheMatchedHit(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	h.seedTerm(t, kind.Medication, recordID(1), "Warfarin", "", &on)

	page, err := h.repo.SearchKind(ctx, h.patient, kind.Medication, "warfarin", nil, "", 10, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Warfarin", page.Items[0].Title)
	assert.Equal(t, kind.Medication, page.Items[0].Kind)
	assert.Equal(t, on, page.Items[0].OccurredOn)
}
