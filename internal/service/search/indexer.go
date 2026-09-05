package search

import (
	"context"
	"errors"

	"medikube/internal/domain/kind"
)

// ErrNoRepository guards against a zero-value Indexer.
var ErrNoRepository = errors.New("search: no repository")

// Indexer is the write side of the unified search index: every registered
// kind's create/update/delete keeps search_index in step through this.
type Indexer struct {
	repo Repository
}

// NewIndexer wires an Indexer to its Repository.
func NewIndexer(repo Repository) (*Indexer, error) {
	if repo == nil {
		return nil, ErrNoRepository
	}
	return &Indexer{repo: repo}, nil
}

// Create indexes a newly created record. One row per record: a second Create
// for the same (Kind, RecordID) — which should not happen — still leaves
// exactly one row, matching Upsert's contract.
func (ix *Indexer) Create(ctx context.Context, row Row) error {
	return ix.repo.Upsert(ctx, row)
}

// Update replaces the indexed row for a record that changed.
func (ix *Indexer) Update(ctx context.Context, row Row) error {
	return ix.repo.Upsert(ctx, row)
}

// Delete removes the indexed row for one deleted record.
func (ix *Indexer) Delete(ctx context.Context, k kind.Kind, recordID string) error {
	return ix.repo.Remove(ctx, k, recordID)
}

// DeletePatient removes every indexed row for a deleted patient (FR-087,
// SC-005): the cascade is total, not per-kind.
func (ix *Indexer) DeletePatient(ctx context.Context, patientID string) error {
	return ix.repo.RemoveByPatient(ctx, patientID)
}
