package audit

import (
	"context"
	"slices"
	"sync"
	"time"

	domainaudit "medikube/internal/domain/audit"
)

// trail is the in-memory audit store the tests in this package run against.
//
// It is both the Repository the writer appends through and the Purger the
// retention job deletes through, so a test can write rows the way production
// writes them and then purge them the way production purges them, against one
// store that actually holds them. A purger that only recorded the cutoff it was
// handed would make the retention test an assertion about arithmetic; this one
// makes it an assertion about which rows are left.
type trail struct {
	mu sync.Mutex

	rows []domainaudit.Event

	// cutoffs is every horizon DeleteBefore was asked for, in order.
	cutoffs []time.Time

	appendErr error
	purgeErr  error
}

func newTrail() *trail { return &trail{} }

func (t *trail) Append(_ context.Context, event domainaudit.Event) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.appendErr != nil {
		return t.appendErr
	}

	t.rows = append(t.rows, event)

	return nil
}

// DeleteBefore removes strictly older rows, which is the boundary the retention
// horizon has: a row that occurred exactly at the cutoff is exactly the
// configured age and not older than it.
func (t *trail) DeleteBefore(_ context.Context, cutoff time.Time) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cutoffs = append(t.cutoffs, cutoff)

	if t.purgeErr != nil {
		return 0, t.purgeErr
	}

	kept := make([]domainaudit.Event, 0, len(t.rows))

	for _, row := range t.rows {
		if !row.OccurredAt.Before(cutoff) {
			kept = append(kept, row)
		}
	}

	removed := len(t.rows) - len(kept)
	t.rows = kept

	return removed, nil
}

func (t *trail) Rows() []domainaudit.Event {
	t.mu.Lock()
	defer t.mu.Unlock()

	return slices.Clone(t.rows)
}

func (t *trail) Cutoffs() []time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()

	return slices.Clone(t.cutoffs)
}

func (t *trail) FailAppends(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.appendErr = err
}

func (t *trail) FailPurges(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.purgeErr = err
}

// fixedClock is the retention job's clock. The horizon is arithmetic on a
// wall-clock reading, and a test that read the real clock would be asserting
// against a moving cutoff.
type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
