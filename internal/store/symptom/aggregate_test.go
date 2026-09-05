package symptom_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	service "medikube/internal/service/symptom"
	"medikube/internal/store"
	pbsymptom "medikube/internal/store/symptom"

	_ "medikube/internal/store/migrations"
)

// T084, FR-031, FR-090, SC-016. Episode counts and the most recent occurrence
// are derived on every read and never maintained by hand: four episodes of one
// name aggregate to episode_count 4, and after the newest is deleted, the very
// next read reflects it with nothing recomputed by a job. Names differing only
// in case group together.
func TestAggregateCountsEpisodesAndTracksTheMostRecentOccurrence(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbsymptom.New(app, codec)
	require.NoError(t, err)

	owner := symptomSeedAccount(t, app, "aggregate-episodes@example.test")
	patientID := symptomSeedPatient(t, app, owner)

	occurredAt := []time.Time{
		time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, time.February, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
	}

	var newestID string

	for i, when := range occurredAt {
		// Deliberately mixed case: "Headache", "headache", "HEADACHE",
		// "HeadAche" — names differing only in case must group together.
		name := []string{"Headache", "headache", "HEADACHE", "HeadAche"}[i]

		created, err := repo.Create(t.Context(), clinical.Symptom{
			PatientID: patientID, Name: name, Severity: clinical.SeverityModerate, OccurredAt: clinical.NewInstant(when),
		})
		require.NoError(t, err)

		if i == len(occurredAt)-1 {
			newestID = created.ID
		}
	}

	page, err := repo.List(t.Context(), patientID, service.Query{Limit: 10})
	require.NoError(t, err)
	require.Len(t, page.Items, 4)

	for _, item := range page.Items {
		assert.Equal(t, 4, item.EpisodeCount, "four episodes of the same name (case-insensitive) must aggregate to 4")
		assert.True(t, item.LastOccurredAt.Time().Equal(occurredAt[len(occurredAt)-1]),
			"last_occurred_at must be the newest occurrence")
	}

	found, err := repo.Get(t.Context(), newestID)
	require.NoError(t, err)
	require.NoError(t, repo.Delete(t.Context(), found.ID, found.Version))

	afterDelete, err := repo.List(t.Context(), patientID, service.Query{Limit: 10})
	require.NoError(t, err)
	require.Len(t, afterDelete.Items, 3)

	for _, item := range afterDelete.Items {
		assert.Equal(t, 3, item.EpisodeCount,
			"the very next read must reflect the deletion with nothing recomputed by a job")
		assert.True(t, item.LastOccurredAt.Time().Equal(occurredAt[len(occurredAt)-2]),
			"last_occurred_at must now be the newest surviving occurrence")
	}
}
