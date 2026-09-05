// Package search is the PocketBase side of the unified search index's write
// path: internal/service/search declares the Repository port, this package
// implements it against the search_index collection (data-model §5.3).
package search

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/service/search"
)

// Collection is search_index's name, declared here rather than imported from
// internal/store/migrations: every store package that owns a collection
// declares its own copy of the name and its field spellings (see
// internal/store/owner.go), so that no store package depends on the
// migrations that merely created what it now reads and writes.
const Collection = "search_index"

const (
	fieldPatient    = "patient"
	fieldKind       = "kind"
	fieldRecordID   = "record_id"
	fieldTitle      = "title"
	fieldBody       = "body"
	fieldOccurredOn = "occurred_on"
	fieldTags       = "tags"
)

// Repo is search.Repository against a real instance.
type Repo struct {
	app core.App
}

var _ search.Repository = (*Repo)(nil)

func New(app core.App) (*Repo, error) {
	if app == nil {
		return nil, fmt.Errorf("search: the repository is wired with no application")
	}

	return &Repo{app: app}, nil
}

// Upsert creates the row if none exists for (Kind, RecordID) and replaces it
// otherwise — one row per record, always (uniq_search_record).
func (r *Repo) Upsert(ctx context.Context, row search.Row) error {
	record, err := r.find(ctx, row.Kind, row.RecordID)
	if err != nil {
		return err
	}

	if record == nil {
		collection, collErr := r.app.FindCachedCollectionByNameOrId(Collection)
		if collErr != nil {
			return fmt.Errorf("search: reading %s: %w", Collection, collErr)
		}

		record = core.NewRecord(collection)
	}

	record.Set(fieldPatient, row.PatientID)
	record.Set(fieldKind, row.Kind.Enum())
	record.Set(fieldRecordID, row.RecordID)
	record.Set(fieldTitle, row.Title)
	record.Set(fieldBody, row.Body)
	record.Set(fieldOccurredOn, row.OccurredOn.UTC())
	record.Set(fieldTags, row.TagIDs)

	if err := r.app.SaveWithContext(ctx, record); err != nil {
		return fmt.Errorf("search: indexing a %s row: %w", row.Kind, err)
	}

	return nil
}

// Find reads the indexed row for one record, if any. It exists for a caller
// that reads the index back to prove the write side actually wrote what it
// claims to (internal/web/api's own record-contract proof), not for the
// unified search that is US8's own.
func (r *Repo) Find(ctx context.Context, k kind.Kind, recordID string) (search.Row, bool, error) {
	record, err := r.find(ctx, k, recordID)
	if err != nil {
		return search.Row{}, false, err
	}

	if record == nil {
		return search.Row{}, false, nil
	}

	return search.Row{
		PatientID: record.GetString(fieldPatient),
		Kind:      k,
		RecordID:  record.GetString(fieldRecordID),
		Title:     record.GetString(fieldTitle),
		Body:      record.GetString(fieldBody),
	}, true, nil
}

// Remove deletes the row for one record, if any.
func (r *Repo) Remove(ctx context.Context, k kind.Kind, recordID string) error {
	record, err := r.find(ctx, k, recordID)
	if err != nil {
		return err
	}

	if record == nil {
		return nil
	}

	if err := r.app.DeleteWithContext(ctx, record); err != nil {
		return fmt.Errorf("search: removing a %s row: %w", k, err)
	}

	return nil
}

// RemoveByPatient deletes every row for a patient, in the same commit as the
// patient's own delete (FR-087, SC-005). The relation also cascades this on
// its own; this exists for the caller that removes a patient without going
// through PocketBase's own delete, and so that the cascade is asserted rather
// than assumed.
func (r *Repo) RemoveByPatient(ctx context.Context, patientID string) error {
	var records []*core.Record

	if err := r.app.RecordQuery(Collection).
		AndWhere(dbx.HashExp{fieldPatient: patientID}).
		WithContext(ctx).
		All(&records); err != nil {
		return fmt.Errorf("search: reading a patient's index rows: %w", err)
	}

	for _, record := range records {
		if err := r.app.DeleteWithContext(ctx, record); err != nil {
			return fmt.Errorf("search: removing a patient's index row: %w", err)
		}
	}

	return nil
}

func (r *Repo) find(ctx context.Context, k kind.Kind, recordID string) (*core.Record, error) {
	var records []*core.Record

	err := r.app.RecordQuery(Collection).
		AndWhere(dbx.HashExp{fieldKind: k.Enum(), fieldRecordID: recordID}).
		Limit(1).
		WithContext(ctx).
		All(&records)
	if err != nil {
		return nil, fmt.Errorf("search: reading a %s row: %w", k, err)
	}

	if len(records) == 0 {
		return nil, nil
	}

	return records[0], nil
}
