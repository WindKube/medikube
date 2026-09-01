package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/audit"
	"medikube/internal/store"
)

// Repo appends to the audit trail, and past the retention horizon removes from
// it.
//
// Append and DeleteBefore, and nothing between them: data-model §3 makes the
// trail immutable, and a repository with an Update or a per-row Delete on it is
// the edit somebody reaches for when a row is inconvenient. DeleteBefore takes
// no id and cannot be pointed at one — the only thing it can be told is an age.
type Repo struct {
	app core.App
}

func New(app core.App) (*Repo, error) {
	if app == nil {
		return nil, fmt.Errorf("audit: the repository is wired with no application")
	}

	return &Repo{app: app}, nil
}

// Append writes one row. It takes no transaction: an audit row written inside
// the transaction it describes disappears with a rollback, and a trail that
// loses the record of what failed is a trail that only records success.
func (r *Repo) Append(ctx context.Context, event audit.Event) error {
	collection, err := r.app.FindCachedCollectionByNameOrId(store.AuditCollection)
	if err != nil {
		return fmt.Errorf("audit: reading %s: %w", store.AuditCollection, err)
	}

	record := core.NewRecord(collection)
	if err := store.AuditEventToRecord(record, event); err != nil {
		return err
	}

	if err := r.app.SaveWithContext(ctx, record); err != nil {
		return fmt.Errorf("audit: appending a %s row: %w", event.Action, err)
	}

	return nil
}

// DeleteBefore removes every row that occurred strictly before cutoff.
//
// Strictly: a row that occurred exactly at the cutoff is exactly the configured
// age and not older than it, which is a row the operator's retention says to
// keep.
//
// It is one bulk statement rather than a walk that deletes record by record,
// and that is a design decision rather than an optimisation. A record-level
// delete fires the model hooks, which is where T242's immutability guards live;
// a purge that went through them would need the guards to carry an exception,
// and an exception is a door. Going around the record layer entirely lets the
// guard refuse EVERY record-level delete with no escape hatch at all, which is
// the stronger shape. The trade is that this statement is not itself guarded,
// so the thing that keeps it honest is that it takes an age and never an id.
func (r *Repo) DeleteBefore(ctx context.Context, cutoff time.Time) (int, error) {
	older, err := store.AuditOlderThan(cutoff)
	if err != nil {
		return 0, fmt.Errorf("audit: purging the trail: %w", err)
	}

	result, err := r.app.NonconcurrentDB().
		Delete(store.AuditCollection, older).
		WithContext(ctx).
		Execute()
	if err != nil {
		return 0, fmt.Errorf("audit: removing %s rows older than %s: %w", store.AuditCollection, cutoff, err)
	}

	removed, err := result.RowsAffected()
	if err != nil {
		// The rows are gone either way; what is missing is the count. Saying
		// so is better than reporting zero, which reads as a purge that found
		// nothing to do.
		return 0, fmt.Errorf("audit: the trail was purged past %s but the driver did not report how many rows left: %w", cutoff, err)
	}

	return int(removed), nil
}
