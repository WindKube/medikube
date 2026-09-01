package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/store"
	auditstore "medikube/internal/store/audit"
	"medikube/internal/testsupport"
)

// The retention purge is the only operation in MediKube that removes a medical
// record's history, it runs unattended at three in the morning, and the thing
// it acts on is an age rather than a row somebody chose. That combination is
// why it is exercised against a real instance rather than against the fake the
// service tier uses: the horizon is compared lexicographically against
// PocketBase's own date layout, and a cutoff bound in Go's rendering instead
// would match nothing at all while every assertion about arithmetic passed.

func newRepo(t *testing.T) (*auditstore.Repo, func() []time.Time) {
	t.Helper()

	app := testsupport.NewApp(t)

	repo, err := auditstore.New(app)
	require.NoError(t, err)

	occurrences := func() []time.Time {
		records, err := app.FindAllRecords(store.AuditCollection)
		require.NoError(t, err)

		times := make([]time.Time, 0, len(records))
		for _, record := range records {
			times = append(times, record.GetDateTime(store.AuditFieldOccurredAt).Time().UTC())
		}

		return times
	}

	return repo, occurrences
}

func rowAt(occurredAt time.Time) domainaudit.Event {
	return domainaudit.Event{
		OccurredAt: occurredAt,
		ActorID:    testsupport.AccountAID,
		ActorKind:  domainaudit.ActorKindUser,
		Action:     domainaudit.ActionCreate,
		TargetKind: domainaudit.TargetKindMedication,
		TargetID:   testsupport.NameOnlyMedicationID,
		RequestID:  "0123456789abcdef0123456789abcdef",
	}
}

// The horizon's boundary against the real column, both sides of it and exactly
// on it. The seeded fixture is left in place deliberately: it is the population
// a purge with a badly bound parameter would take away wholesale, and asserting
// on what remains is what catches that.
func TestDeleteBeforeRemovesWhatIsPastTheHorizonAndNothingYounger(t *testing.T) {
	t.Parallel()

	repo, occurrences := newRepo(t)

	now := time.Date(2026, time.March, 14, 3, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -730)

	require.Empty(t, occurrences(),
		"the committed fixture now seeds audit rows; this case assumes the trail starts empty and would be asserting against them too")

	doomed := []time.Time{
		cutoff.AddDate(0, 0, -400),
		cutoff.AddDate(0, 0, -1),
		cutoff.Add(-time.Second),
	}
	kept := []time.Time{
		cutoff,
		cutoff.Add(time.Second),
		cutoff.AddDate(0, 0, 1),
		now,
	}

	for _, occurredAt := range append(append([]time.Time{}, doomed...), kept...) {
		require.NoError(t, repo.Append(t.Context(), rowAt(occurredAt)))
	}

	require.Len(t, occurrences(), len(doomed)+len(kept),
		"the rows never reached the trail, so a purge that removed nothing would pass")

	removed, err := repo.DeleteBefore(t.Context(), cutoff)

	require.NoError(t, err)
	assert.Equal(t, len(doomed), removed, "DeleteBefore reported a count that is not what left the trail")
	assert.ElementsMatch(t, kept, occurrences(),
		"the horizon took rows the retention says to keep, or left rows past it")
}

// A cutoff in the far past is the shape an operator's mistake takes, and the
// answer has to be that nothing happens rather than that everything does.
func TestDeleteBeforeAHorizonNothingReachesRemovesNothing(t *testing.T) {
	t.Parallel()

	repo, occurrences := newRepo(t)

	for _, day := range []int{0, 1, 4000} {
		require.NoError(t, repo.Append(t.Context(), rowAt(time.Now().UTC().AddDate(0, 0, -day))))
	}

	before := occurrences()
	require.Len(t, before, 3, "the rows never reached the trail, so removing none of them proves nothing")

	removed, err := repo.DeleteBefore(t.Context(), time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Zero(t, removed)
	assert.ElementsMatch(t, before, occurrences())
}

// The context is honoured, so a purge cannot outlive the shutdown that
// cancelled it and hold the write lock while the process is trying to leave.
func TestDeleteBeforeRefusesOnACancelledContext(t *testing.T) {
	t.Parallel()

	repo, occurrences := newRepo(t)

	require.NoError(t, repo.Append(t.Context(), rowAt(time.Now().UTC().AddDate(0, 0, -4000))))

	before := occurrences()
	require.Len(t, before, 1, "there is nothing here for a cancelled purge to have spared")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	removed, err := repo.DeleteBefore(ctx, time.Now().UTC())

	require.Error(t, err)
	assert.Zero(t, removed)
	assert.ElementsMatch(t, before, occurrences(), "a cancelled purge still emptied the trail")
}
