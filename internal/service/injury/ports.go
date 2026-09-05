package injury

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const (
	FieldOccurredOn = "occurred_on"
	FieldName       = "name"
	FieldUpdated    = "updated"
)

const (
	FilterStatus     = "status"
	FilterSeverity   = "severity"
	FilterType       = "type"
	FilterLaterality = "laterality"
	// FilterUnresolved is `?unresolved=true`: status in {active, healing}
	// (contracts/records-clinical.md §1, US4-5).
	FilterUnresolved = "unresolved"
)

const ParamSort = "sort"

func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldOccurredOn, Desc: true},
		{Field: FieldOccurredOn},
		{Field: FieldName},
		{Field: FieldName, Desc: true},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

// unresolvedStatuses is what `?unresolved=true` narrows to.
func unresolvedStatuses() []clinical.ConditionStatus {
	return []clinical.ConditionStatus{clinical.ConditionStatusActive, clinical.ConditionStatusHealing}
}

// Query is one list request as the service resolved it.
type Query struct {
	PatientID string
	Search    string

	Statuses     []clinical.ConditionStatus
	Severities   []clinical.Severity
	Types        []clinical.InjuryType
	Lateralities []clinical.Laterality
	Unresolved   bool

	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
}

type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Injury], error)
	Get(ctx context.Context, id string) (clinical.Injury, error)
	Create(ctx context.Context, injury clinical.Injury) (clinical.Injury, error)
	Update(ctx context.Context, injury clinical.Injury, expectedVersion string) (clinical.Injury, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
