package procedure

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const (
	FieldName       = "name"
	FieldOccurredOn = "occurred_on"
	FieldUpdated    = "updated"
)

const (
	FilterStatus    = "status"
	FilterScheduled = "scheduled"
)

const ParamSort = "sort"

func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldOccurredOn, Desc: true},
		{Field: FieldOccurredOn},
		{Field: FieldName},
		{Field: FieldUpdated, Desc: true},
	}
}

// Query is one list request. Scheduled implements FR-026's `?scheduled=true`:
// a nil pointer means unfiltered, and a set value narrows to
// status ∈ {ordered, scheduled} when true, or its complement when false.
type Query struct {
	PatientID string
	Search    string

	Statuses  []clinical.OrderStatus
	Scheduled *bool

	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
}

type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Procedure], error)
	Get(ctx context.Context, id string) (clinical.Procedure, error)
	Create(ctx context.Context, entity clinical.Procedure) (clinical.Procedure, error)
	Update(ctx context.Context, entity clinical.Procedure, expectedVersion string) (clinical.Procedure, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
