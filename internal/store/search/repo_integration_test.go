package search_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/service/search"
	"medikube/internal/store"
	searchstore "medikube/internal/store/search"

	// The migrations register themselves from their own init and
	// tests.NewTestApp runs core.AppMigrations against the instance. Without
	// this import search_index does not exist against a stock schema.
	_ "medikube/internal/store/migrations"
)

const (
	indexFieldPatient    = "patient"
	indexFieldKind       = "kind"
	indexFieldRecordID   = "record_id"
	indexFieldTitle      = "title"
	indexFieldOccurredOn = "occurred_on"
)

// harness is one instance, one repository and one patient. A new one per test
// rather than one shared, for the same reason internal/store/medication's own
// harness is: the ordering assertions below are written against an index
// holding only what that case put in it.
type harness struct {
	app     *tests.TestApp
	repo    *searchstore.Repo
	owner   string
	patient string
}

func newHarness(t *testing.T) harness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err, "the instance has no cursor key material")

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := searchstore.New(app, codec)
	require.NoError(t, err)

	owner := seedAccount(t, app, "owner@example.test")

	return harness{app: app, repo: repo, owner: owner, patient: seedPatient(t, app, owner)}
}

func seedAccount(t *testing.T, app core.App, email string) string {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword("correct-horse-battery-staple")
	record.Set("name", "Test Person")
	record.Set("role", "user")
	record.Set("unit_system", "metric")
	record.Set("locale", "en")
	record.Set("date_format", "iso")
	record.Set("theme", "system")

	require.NoError(t, app.Save(record))

	return record.Id
}

func seedPatient(t *testing.T, app core.App, ownerID string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Test")
	record.Set("last_name", "Patient")

	require.NoError(t, app.Save(record))

	return record.Id
}

// seedTag writes a real tags row: search_index.tags is a relation, so an id
// that names no such row is a validation error rather than a foreign key
// this test could otherwise get away with faking.
func seedTag(t *testing.T, app core.App, ownerID, name string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("tags")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("name", name)

	require.NoError(t, app.Save(record))

	return record.Id
}

// seedRow writes one search_index row directly, the way the fourteen kinds'
// indexingService does — through record.Set, never through the repository
// under test. occurredOn nil is the row this migration's field leaves unset.
func (h harness) seedRow(t *testing.T, k kind.Kind, recordID string, occurredOn *domain.Date) {
	t.Helper()

	collection, err := h.app.FindCollectionByNameOrId(searchstore.Collection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set(indexFieldPatient, h.patient)
	record.Set(indexFieldKind, k.Enum())
	record.Set(indexFieldRecordID, recordID)
	record.Set(indexFieldTitle, "a title")

	if occurredOn != nil {
		record.Set(indexFieldOccurredOn, occurredOn.UTC())
	}

	require.NoError(t, h.app.Save(record))
}

// recordID mints a fifteen-character id, the length search_index's own
// migration requires (searchRecordIDLen).
func recordID(n int) string {
	return fmt.Sprintf("rec%012d", n)
}

func date(t *testing.T, text string) domain.Date {
	t.Helper()

	parsed, err := domain.ParseDate(text)
	require.NoError(t, err)

	return parsed
}

func refIDs(refs []search.Ref) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.RecordID)
	}

	return ids
}

// The ordering: most recent occurred_on first, and a row with none sorts
// after every row that has one — under a bare DESC column, with no AbsentLast
// flag needed, exactly as internal/store/filter.go documents for a
// descending ordering.
func TestPageOrdersByOccurredOnDescendingWithNullsLast(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	early := date(t, "2026-01-01")
	late := date(t, "2026-03-01")

	h.seedRow(t, kind.Medication, recordID(1), &early)
	h.seedRow(t, kind.Allergy, recordID(2), &late)
	h.seedRow(t, kind.Medication, recordID(3), nil)

	page, err := h.repo.Page(ctx, h.patient, nil, 10, "")
	require.NoError(t, err)
	require.Nil(t, page.NextCursor)

	assert.Equal(t, []string{recordID(2), recordID(1), recordID(3)}, refIDs(page.Items))
}

// The kind narrowing: an empty selection is every kind, a non-empty one is
// exactly that set.
func TestPageNarrowsByTheSelectedKinds(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	on := date(t, "2026-01-01")

	h.seedRow(t, kind.Medication, recordID(1), &on)
	h.seedRow(t, kind.Allergy, recordID(2), &on)

	page, err := h.repo.Page(ctx, h.patient, []kind.Kind{kind.Medication}, 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recordID(1)}, refIDs(page.Items))
}

// FR-023 at the search index: a row inserted into the part of the sequence a
// traversal has already gone past is neither repeated nor skipped, because
// the boundary is the last row read and not an offset.
func TestPagingContinuesWithNoRepeatAndNoSkipUnderAnInsertBetweenPages(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	// Five rows, most recent first once dated descending by id suffix: 5, 4,
	// 3, 2, 1.
	for i := 1; i <= 5; i++ {
		day := date(t, fmt.Sprintf("2026-01-0%d", i))
		h.seedRow(t, kind.Medication, recordID(i), &day)
	}

	first, err := h.repo.Page(ctx, h.patient, nil, 2, "")
	require.NoError(t, err)
	require.NotNil(t, first.NextCursor)
	assert.Equal(t, []string{recordID(5), recordID(4)}, refIDs(first.Items))

	// Inserted between the two calls, dated so it lands ahead of every row
	// already served — the part of the sequence the traversal has passed.
	future := date(t, "2026-06-01")
	h.seedRow(t, kind.Medication, recordID(6), &future)

	second, err := h.repo.Page(ctx, h.patient, nil, 2, *first.NextCursor)
	require.NoError(t, err)
	require.NotNil(t, second.NextCursor)
	assert.Equal(t, []string{recordID(3), recordID(2)}, refIDs(second.Items))

	third, err := h.repo.Page(ctx, h.patient, nil, 2, *second.NextCursor)
	require.NoError(t, err)
	assert.Nil(t, third.NextCursor)
	assert.Equal(t, []string{recordID(1)}, refIDs(third.Items))

	seen := append(append(refIDs(first.Items), refIDs(second.Items)...), refIDs(third.Items)...)
	assert.ElementsMatch(t, []string{recordID(1), recordID(2), recordID(3), recordID(4), recordID(5)}, seen)
}

// Count answers the same narrowing with no keyset boundary.
func TestCountMatchesThePageNarrowing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	on := date(t, "2026-01-01")

	h.seedRow(t, kind.Medication, recordID(1), &on)
	h.seedRow(t, kind.Allergy, recordID(2), &on)

	total, err := h.repo.Count(ctx, h.patient, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	total, err = h.repo.Count(ctx, h.patient, []kind.Kind{kind.Allergy})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}
