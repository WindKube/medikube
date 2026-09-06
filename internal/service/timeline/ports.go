// Package timeline is US9's one chronological view across every registered
// kind (FR-076, FR-077, research D-06). It reads the same unified index
// internal/service/search's write side keeps in step, through its own
// Reader port: the timeline narrows by kind, by tag and by date range, none
// of which search.Reader's cross-kind list needs (contracts/records.md).
package timeline

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
)

// Ref is one indexed row's identity and primary date — enough to hydrate
// through the kind's own Service.Get, which is where authorization and the
// current record body both live.
type Ref struct {
	Kind       kind.Kind
	RecordID   string
	OccurredOn domain.Date
}

// Reader is the timeline's read side: one patient's indexed rows, across the
// selected kinds, narrowed by tag and by date range, most recent primary date
// first and a null one sorted last (research D-06). id descending is the
// tie-break, and it is the boundary a cursor is minted from, not a value the
// caller ever sees.
type Reader interface {
	Page(
		ctx context.Context, patientID string, kinds []kind.Kind, tags []string, from, to string,
		limit int, cursor string,
	) (domain.Page[Ref], error)

	Count(ctx context.Context, patientID string, kinds []kind.Kind, tags []string, from, to string) (int, error)
}

// recordService is the seam List's per-row hydration hands the actor to:
// records.Service.Get, where the row's own kind authorizes it (FR-081). It is
// declared locally, matching records.Service's shape exactly, so this
// package's own authorization-reaching gate can see the delegation
// (internal/service/access/exhaustive_test.go).
type recordService interface {
	Get(ctx context.Context, actor access.Actor, id string) (records.Record, error)
}

var _ recordService = records.Service(nil)
