package timeline_test

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
	svctimeline "medikube/internal/service/timeline"
	"medikube/internal/store"
	timelinestore "medikube/internal/store/timeline"

	_ "medikube/internal/store/migrations"
)

const (
	indexFieldPatient    = "patient"
	indexFieldKind       = "kind"
	indexFieldRecordID   = "record_id"
	indexFieldTitle      = "title"
	indexFieldOccurredOn = "occurred_on"
	indexFieldTags       = "tags"
)

type harness struct {
	app     *tests.TestApp
	repo    *timelinestore.Repo
	patient string
	owner   string
}

func newHarness(t *testing.T) harness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := timelinestore.New(app, codec)
	require.NoError(t, err)

	owner := seedAccount(t, app, "owner@example.test")

	return harness{app: app, repo: repo, patient: seedPatient(t, app, owner), owner: owner}
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

func seedTag(t *testing.T, app core.App, ownerID, name string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("tags")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("name", name)
	record.Set("color", "#112233")

	require.NoError(t, app.Save(record))

	return record.Id
}

func (h harness) seedRow(t *testing.T, k kind.Kind, recordID string, occurredOn *domain.Date, tagIDs ...string) {
	t.Helper()

	collection, err := h.app.FindCollectionByNameOrId(timelinestore.Collection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set(indexFieldPatient, h.patient)
	record.Set(indexFieldKind, k.Enum())
	record.Set(indexFieldRecordID, recordID)
	record.Set(indexFieldTitle, "a title")
	record.Set(indexFieldTags, tagIDs)

	if occurredOn != nil {
		record.Set(indexFieldOccurredOn, occurredOn.UTC())
	}

	require.NoError(t, h.app.Save(record))
}

func recID(n int) string { return fmt.Sprintf("rec%012d", n) }

func date(t *testing.T, text string) domain.Date {
	t.Helper()

	parsed, err := domain.ParseDate(text)
	require.NoError(t, err)

	return parsed
}

func refIDs(refs []svctimeline.Ref) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.RecordID)
	}

	return ids
}

// research D-06: primary date descending, absent last.
func TestPageOrdersByOccurredOnDescendingWithNullsLast(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	early := date(t, "2026-01-01")
	late := date(t, "2026-03-01")

	h.seedRow(t, kind.Medication, recID(1), &early)
	h.seedRow(t, kind.Allergy, recID(2), &late)
	h.seedRow(t, kind.Medication, recID(3), nil)

	page, err := h.repo.Page(ctx, h.patient, nil, nil, "", "", 10, "")
	require.NoError(t, err)
	require.Nil(t, page.NextCursor)

	assert.Equal(t, []string{recID(2), recID(1), recID(3)}, refIDs(page.Items))
	assert.True(t, page.Items[2].OccurredOn.IsZero())
}

func TestPageNarrowsByKind(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	on := date(t, "2026-01-01")

	h.seedRow(t, kind.Medication, recID(1), &on)
	h.seedRow(t, kind.Allergy, recID(2), &on)

	page, err := h.repo.Page(ctx, h.patient, []kind.Kind{kind.Medication}, nil, "", "", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recID(1)}, refIDs(page.Items))
}

func TestPageNarrowsByTag(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	on := date(t, "2026-01-01")
	tagged := seedTag(t, h.app, h.owner, "chronic")

	h.seedRow(t, kind.Medication, recID(1), &on, tagged)
	h.seedRow(t, kind.Allergy, recID(2), &on)

	page, err := h.repo.Page(ctx, h.patient, nil, []string{tagged}, "", "", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recID(1)}, refIDs(page.Items))
}

func TestPageNarrowsByDateRange(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	early := date(t, "2026-01-01")
	late := date(t, "2026-06-01")

	h.seedRow(t, kind.Medication, recID(1), &early)
	h.seedRow(t, kind.Medication, recID(2), &late)

	page, err := h.repo.Page(ctx, h.patient, nil, nil, "2026-02-01", "2026-12-01", 10, "")
	require.NoError(t, err)
	assert.Equal(t, []string{recID(2)}, refIDs(page.Items))
}

func TestCountMatchesTheNarrowing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	on := date(t, "2026-01-01")

	h.seedRow(t, kind.Medication, recID(1), &on)
	h.seedRow(t, kind.Allergy, recID(2), &on)

	total, err := h.repo.Count(ctx, h.patient, nil, nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	total, err = h.repo.Count(ctx, h.patient, []kind.Kind{kind.Allergy}, nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}
