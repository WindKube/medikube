package audit

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/audit"
	"medikube/internal/store"
)

// Repo appends to the audit trail.
//
// Append and nothing else: data-model §3 makes the trail immutable, and a
// repository with an Update or a Delete on it is the edit somebody reaches for
// when a row is inconvenient. The immutability guards in internal/platform/pb
// are the enforcement; this is the shape that does not invite the attempt.
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
