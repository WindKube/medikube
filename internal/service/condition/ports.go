package condition

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const (
	FieldDiagnosis = "diagnosis"
	FieldOnsetOn   = "onset_on"
	FieldUpdated   = "updated"
)

// FilterStatus, FilterSeverity and FilterActive are the `?status=`,
// `?severity=` and `?active=true` narrowings (contracts/pages.md §3.5,
// FR-078).
const (
	FilterStatus   = "status"
	FilterSeverity = "severity"
	FilterActive   = "active"
)

const ParamSort = "sort"

// MatchAny and MatchAll are `?tags=&match=` (FR-067, research D-10),
// mirroring internal/records.MatchAny/MatchAll: this package does not import
// internal/records, so it carries its own copy of the two spellings.
const (
	MatchAny = "any"
	MatchAll = "all"
)

// Sorts is the published ordering allowlist, most recent onset first.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldOnsetOn, Desc: true},
		{Field: FieldOnsetOn},
		{Field: FieldDiagnosis},
		{Field: FieldUpdated, Desc: true},
	}
}

type Query struct {
	PatientID string
	Search    string

	Statuses   []clinical.ConditionStatus
	Severities []clinical.Severity
	// Active narrows to status active/chronic when true (FR-078). nil means
	// unnarrowed.
	Active *bool

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
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Condition], error)
	Get(ctx context.Context, id string) (clinical.Condition, error)
	Create(ctx context.Context, entity clinical.Condition) (clinical.Condition, error)
	Update(ctx context.Context, entity clinical.Condition, expectedVersion string) (clinical.Condition, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
