package encounter

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const (
	FieldReason     = "reason"
	FieldOccurredOn = "occurred_on"
	FieldUpdated    = "updated"
)

const (
	FilterVisitType = "visit_type"
	FilterPriority  = "priority"
)

const ParamSort = "sort"

// MatchAny and MatchAll are `?tags=&match=` (FR-067, research D-10),
// mirroring internal/records.MatchAny/MatchAll: this package does not import
// internal/records, so it carries its own copy of the two spellings.
const (
	MatchAny = "any"
	MatchAll = "all"
)

func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldOccurredOn, Desc: true},
		{Field: FieldOccurredOn},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

type Query struct {
	PatientID string
	Search    string

	VisitTypes []clinical.VisitType
	Priorities []clinical.VisitPriority

	// Tags and Match are `?tags=a,b&match=any|all` (FR-067). Tags empty
	// means the narrowing is not applied; Match is MatchAny unless the
	// caller asked for MatchAll.
	Tags  []string
	Match string

	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
}

type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Encounter], error)
	Get(ctx context.Context, id string) (clinical.Encounter, error)
	Create(ctx context.Context, entity clinical.Encounter) (clinical.Encounter, error)
	Update(ctx context.Context, entity clinical.Encounter, expectedVersion string) (clinical.Encounter, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
