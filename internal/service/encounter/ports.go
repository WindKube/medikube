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
