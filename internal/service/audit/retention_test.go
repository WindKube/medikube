package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainaudit "medikube/internal/domain/audit"
)

// The default, two years, and a short one. Two horizons and not one: a purge
// that read a constant instead of its configuration passes every assertion a
// single-horizon test can make.
const (
	twoYears = 730
	oneMonth = 30
)

func at(now time.Time, days int, offset time.Duration) domainaudit.Event {
	event := minimal(domainaudit.ActionCreate)
	event.OccurredAt = now.AddDate(0, 0, -days).Add(offset)

	return event
}

func occurrences(rows []domainaudit.Event) []time.Time {
	times := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		times = append(times, row.OccurredAt)
	}

	return times
}

// The horizon's boundary, both sides of it and exactly on it. "Older than the
// retention" is strict: a row that occurred exactly `days` ago is exactly that
// old and not older, and a purge that removed it would remove a row the
// operator's configuration says to keep.
func TestPurgeRemovesWhatIsOlderThanTheHorizonAndNothingYounger(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		days int
	}{
		{name: "the two-year default", days: twoYears},
		{name: "a shorter configured horizon", days: oneMonth},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			now := time.Date(2026, time.March, 14, 3, 0, 0, 0, time.UTC)

			doomed := []domainaudit.Event{
				at(now, testCase.days*3, 0),
				at(now, testCase.days+1, 0),
				at(now, testCase.days, -time.Second),
			}
			kept := []domainaudit.Event{
				at(now, testCase.days, 0),
				at(now, testCase.days, time.Second),
				at(now, testCase.days-1, 0),
				at(now, 0, 0),
			}

			store := newTrail()
			for _, event := range append(append([]domainaudit.Event{}, doomed...), kept...) {
				require.NoError(t, store.Append(t.Context(), event))
			}

			require.Len(t, store.Rows(), len(doomed)+len(kept),
				"the store never took the rows, so a purge that removed nothing would pass")

			retention, err := NewRetention(store, testCase.days, fixedClock{now: now})
			require.NoError(t, err)

			removed, err := retention.Purge(t.Context())
			require.NoError(t, err)

			assert.Equal(t, len(doomed), removed, "Purge reported a count that is not what left the trail")
			assert.ElementsMatch(t, occurrences(kept), occurrences(store.Rows()),
				"the horizon took rows the configuration says to keep, or left rows past it")
		})
	}
}

// The horizon is arithmetic on the configured number of days, in UTC, and the
// purger is asked for exactly it. A purge asked for `now` empties the trail.
func TestPurgeAsksForTheHorizonTheConfigurationDescribes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 14, 3, 0, 0, 0, time.UTC)

	store := newTrail()

	retention, err := NewRetention(store, twoYears, fixedClock{now: now})
	require.NoError(t, err)

	_, err = retention.Purge(t.Context())
	require.NoError(t, err)

	cutoffs := store.Cutoffs()
	require.Len(t, cutoffs, 1, "the purger was not asked for a horizon at all")
	assert.Equal(t, now.AddDate(0, 0, -twoYears).UTC(), cutoffs[0].UTC())
	assert.True(t, cutoffs[0].Before(now), "the horizon is not in the past, so the purge would take everything")
}

func TestPurgeReportsAFailureRatherThanClaimingAnEmptyTrail(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("the trail is unreachable")

	store := newTrail()
	store.FailPurges(sentinel)

	retention, err := NewRetention(store, twoYears, fixedClock{now: time.Now()})
	require.NoError(t, err)

	removed, err := retention.Purge(context.Background())

	assert.ErrorIs(t, err, sentinel, "a purge that failed silently is a trail that never shrinks and nobody is told")
	assert.Zero(t, removed)
}

func TestNewRetentionRefusesAJobThatWouldPurgeTheWrongThing(t *testing.T) {
	t.Parallel()

	now := fixedClock{now: time.Now()}

	for _, testCase := range []struct {
		name   string
		purger Purger
		days   int
		clock  Clock
	}{
		{name: "no purger", purger: nil, days: twoYears, clock: now},
		{name: "no clock", purger: newTrail(), days: twoYears, clock: nil},
		{name: "no horizon", purger: newTrail(), days: 0, clock: now},
		{name: "a horizon in the future", purger: newTrail(), days: -1, clock: now},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			retention, err := NewRetention(testCase.purger, testCase.days, testCase.clock)

			require.Error(t, err)
			assert.Nil(t, retention)
		})
	}
}
