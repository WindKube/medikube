package audit

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Purger removes rows past the retention horizon.
//
// It is a second seam rather than a method on Repository, and deliberately so:
// the trail is append-only (data-model §3), and a repository interface carrying
// a delete is the shape somebody reaches for when a row is inconvenient. This
// interface exists for exactly one caller — Retention — and the immutability
// guards refuse the operation through every other path.
type Purger interface {
	// DeleteBefore removes every row that occurred strictly before cutoff and
	// reports how many left. Strictly: a row that occurred exactly at the
	// cutoff is exactly the configured age and not older than it, so the
	// operator's configuration says to keep it.
	DeleteBefore(ctx context.Context, cutoff time.Time) (int, error)
}

// Clock reads the wall clock the horizon is measured back from.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// Retention is the audit trail's only eraser.
type Retention struct {
	purger Purger
	days   int
	clock  Clock
}

// NewRetention refuses a job that would purge the wrong thing.
//
// The horizon in particular: zero days is not "keep everything", it is a cutoff
// at this instant, and a job built with it empties the trail on its first tick.
// A negative one puts the cutoff in the future and does the same. Neither is a
// value an operator meant, and both are cheaper to refuse at assembly than to
// discover from a trail that is gone.
func NewRetention(purger Purger, days int, clock Clock) (*Retention, error) {
	switch {
	case purger == nil:
		return nil, errors.New("audit: the retention job is wired with no purger, so nothing would ever leave the trail")
	case clock == nil:
		return nil, errors.New("audit: the retention job is wired with no clock, so it has no horizon to measure back from")
	case days <= 0:
		return nil, fmt.Errorf("audit: a retention horizon of %d days puts the cutoff at or after now, which empties the trail on the first tick", days)
	}

	return &Retention{purger: purger, days: days, clock: clock}, nil
}

// Horizon is the cutoff this job would ask for now: the configured number of
// days back from the clock, in UTC.
//
// UTC and not the clock's own zone: the horizon is a duration in days and a
// day is not a fixed number of hours in a zone that observes a transition, so
// arithmetic in local time moves the cutoff by an hour twice a year.
func (r *Retention) Horizon() time.Time {
	return r.clock.Now().UTC().AddDate(0, 0, -r.days)
}

// Purge removes everything older than the horizon and reports how many rows
// left.
//
// A failure is returned rather than absorbed. A purge that failed silently is a
// trail that never shrinks and an operator who is never told, and the disk
// fills instead.
func (r *Retention) Purge(ctx context.Context) (int, error) {
	removed, err := r.purger.DeleteBefore(ctx, r.Horizon())
	if err != nil {
		return 0, fmt.Errorf("audit: purging the trail past its retention horizon: %w", err)
	}

	return removed, nil
}
