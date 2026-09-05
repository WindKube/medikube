// Package search is the write side of the unified search index (data-model
// §5.3, US8). The read side (the query, grouped results, per-group cursors)
// is a later story; this phase's job is keeping search_index in step with
// every registered kind's rows as they are created, updated and deleted.
package search

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// Row is one search_index row, exactly the columns data-model §5.3 declares.
// It stores no content the source row does not already store.
type Row struct {
	PatientID  string
	Kind       kind.Kind
	RecordID   string
	Title      string
	Body       string
	OccurredOn domain.Date
	TagIDs     []string
}

// Repository is the PocketBase-side seam this package writes through. It is
// declared here, by the consumer, and implemented in internal/store/search.
type Repository interface {
	// Upsert creates the row if none exists for (Kind, RecordID) and replaces
	// it otherwise — one row per record, always (uniq_search_record).
	Upsert(ctx context.Context, row Row) error
	// Remove deletes the row for one record, if any.
	Remove(ctx context.Context, k kind.Kind, recordID string) error
	// RemoveByPatient deletes every row for a patient, in the same commit as
	// the patient's own delete (FR-087, SC-005).
	RemoveByPatient(ctx context.Context, patientID string) error
}
