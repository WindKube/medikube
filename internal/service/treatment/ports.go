package treatment

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const (
	FieldName      = "name"
	FieldStartedOn = "started_on"
	FieldUpdated   = "updated"
)

const (
	FilterStatus  = "status"
	FilterOngoing = "ongoing"
)

const ParamSort = "sort"

func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldStartedOn, Desc: true},
		{Field: FieldStartedOn},
		{Field: FieldName},
		{Field: FieldUpdated, Desc: true},
	}
}

// ongoingStatuses is what "still running" means (data-model §1's TherapyStatus
// ladder): active and on_hold are the two states a course of treatment is
// still, in some sense, underway.
var ongoingStatuses = []clinical.TherapyStatus{clinical.TherapyStatusActive, clinical.TherapyStatusOnHold}

type Query struct {
	PatientID string
	Search    string

	Statuses []clinical.TherapyStatus
	Ongoing  *bool

	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
}

// LinkTarget names a multi-relation's collection and the ids it was asked to
// carry, for the same-patient invariant (FR-028, FR-057).
type LinkTarget struct {
	Collection string
	IDs        []string
}

type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Treatment], error)
	Get(ctx context.Context, id string) (clinical.Treatment, error)
	Create(ctx context.Context, entity clinical.Treatment) (clinical.Treatment, error)
	Update(ctx context.Context, entity clinical.Treatment, expectedVersion string) (clinical.Treatment, error)
	Delete(ctx context.Context, id, expectedVersion string) error

	// SamePatient answers whether every id in target belongs to the given
	// patient (FR-057): a differing patient, a missing row and an
	// unreachable one are all "no", and the caller reports the identical
	// refusal for each.
	SamePatient(ctx context.Context, patientID string, target LinkTarget) (bool, error)
}

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
