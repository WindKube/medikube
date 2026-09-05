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
