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

// Ref is one indexed row's identity — enough for a caller that already knows
// how to hydrate a record of that kind (kind.Kind's own Service.Get) to fetch
// the rest. It carries no title and no body: this is what a cross-kind page
// reads to decide *which* records to hydrate, never what it answers with.
type Ref struct {
	Kind       kind.Kind
	RecordID   string
	OccurredOn domain.Date
}

// Hit is one matched row of a grouped search result (US8): everything
// contracts/search.md's item needs, read straight off the index. Unlike Ref,
// which exists only so a caller that already knows how can hydrate the rest,
// a Hit carries its own title and tags — a search result names what it
// found, rather than making the caller re-fetch the record to render one
// line. It is still never logged: the title is PHI the same way the term is.
type Hit struct {
	Kind       kind.Kind
	RecordID   string
	Title      string
	TagIDs     []string
	OccurredOn domain.Date
}

// Searcher is US8's read side: one kind, one term, one page of it, ordered
// occurred_on DESC, id DESC, nulls last (FR-073). Every group in a grouped
// search result is one call.
type Searcher interface {
	SearchKind(
		ctx context.Context, patientID string, k kind.Kind, term string, limit int, cursor string,
	) (domain.Page[Hit], error)
}

// Counter answers whether a patient has any indexed rows at all, ignoring
// the term — what tells a grouped search apart "no_matches" (rows exist,
// none matched) from "no_records" (there is nothing to match against yet),
// US8 scenario 2. Reader already declares this shape; Counter is named
// separately because a caller wiring only the grouped search needs nothing
// else Reader promises.
type Counter interface {
	Count(ctx context.Context, patientID string, kinds []kind.Kind) (int, error)
}

// Reader is the read side of the unified search index: phase 003's cross-kind
// list pages search_index directly, ordered by occurred_on (most recent
// first, absent last) with id as the keyset tiebreaker, because no per-kind
// keyset cursor can continue a page merged from more than one kind's table.
type Reader interface {
	// Page returns one page of a patient's indexed records across the given
	// kinds (every registered kind when empty), oldest boundary first —
	// meaning most recent occurred_on first. cursor is the opaque token a
	// previous page minted; the empty string starts the first page.
	Page(ctx context.Context, patientID string, kinds []kind.Kind, limit int, cursor string) (domain.Page[Ref], error)

	// Count answers how many rows the same narrowing matches, with no keyset
	// boundary — the same number on every page of a traversal.
	Count(ctx context.Context, patientID string, kinds []kind.Kind) (int, error)
}
