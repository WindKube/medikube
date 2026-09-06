//go:build scale

package timeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	timelinestore "medikube/internal/store/timeline"
)

// T184, SC-011: one patient's timeline stays fast when it has a lot to
// order — 50,000 rows in search_index, paged and counted in under 2s each.
// The rows are written directly, as apitest.Populate writes its own bulk
// medications: a hook-driven index write per row is what the real service
// does at request time, one row at a time, and is not what this test is
// timing (research D-06 is the query's own ordering, not the write path).
func TestPagingAndCountingFiftyThousandRowsStaysUnderTwoSeconds(t *testing.T) {
	const rows = 50_000

	h := newHarness(t)

	collection, err := h.app.FindCollectionByNameOrId(timelinestore.Collection)
	require.NoError(t, err)

	epoch := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, store.RunInTransaction(h.app, func(txApp core.App) error {
		for index := range rows {
			record := core.NewRecord(collection)

			day := epoch.AddDate(0, 0, index)

			started, dateErr := domain.NewDate(day.Year(), day.Month(), day.Day())
			if dateErr != nil {
				return dateErr
			}

			record.Set(indexFieldPatient, h.patient)
			record.Set(indexFieldKind, kind.Medication.Enum())
			record.Set(indexFieldRecordID, recID(index))
			record.Set(indexFieldTitle, "a title")
			record.Set(indexFieldOccurredOn, started.UTC())

			if saveErr := txApp.SaveNoValidate(record); saveErr != nil {
				return saveErr
			}
		}

		return nil
	}))

	ctx := context.Background()

	start := time.Now()
	page, err := h.repo.Page(ctx, h.patient, nil, nil, "", "", 50, "")
	pageElapsed := time.Since(start)

	require.NoError(t, err)
	assert.Len(t, page.Items, 50)
	t.Logf("paging %d rows: %s", rows, pageElapsed)
	assert.Lessf(t, pageElapsed, 2*time.Second, "paging %d rows took %s", rows, pageElapsed)

	start = time.Now()
	total, err := h.repo.Count(ctx, h.patient, nil, nil, "", "")
	countElapsed := time.Since(start)

	require.NoError(t, err)
	t.Logf("counting %d rows: %s", rows, countElapsed)
	assert.Equal(t, rows, total)
	assert.Lessf(t, countElapsed, 2*time.Second, "counting %d rows took %s", rows, countElapsed)
}
