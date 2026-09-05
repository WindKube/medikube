package symptom

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// The columns this kind publishes an ordering over, and the named parameters
// it publishes a narrowing over (contracts/records-clinical.md §1).
const (
	FieldOccurredAt = "occurred_at"
	FieldName       = "name"
	FieldUpdated    = "updated"
)

const (
	FilterName      = "name"
	FilterSeverity  = "severity"
	FilterStatus    = "status"
	FilterIsChronic = "is_chronic"
)

const ParamSort = "sort"

// Sorts is the published ordering allowlist. The first entry is the default:
// most recently occurred first, matching idx_symptoms_patient_at.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldOccurredAt, Desc: true},
		{Field: FieldOccurredAt},
		{Field: FieldName},
		{Field: FieldName, Desc: true},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

// Query is one list request as the service resolved it.
type Query struct {
	PatientID  string
	Search     string
	Name       string
	Severities []clinical.Severity
	Statuses   []clinical.ConditionStatus
	IsChronic  *bool
	Sort       []domain.SortKey
	Limit      int
	Cursor     string
	Count      bool
}

// Repository is the storage seam. List and Get both answer episodes with
// FR-031's aggregate already attached: the store computes it on read, this
// package never maintains it (research: aggregate.go's correlated query).
type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Symptom], error)
	Get(ctx context.Context, id string) (clinical.Symptom, error)
	Create(ctx context.Context, symptom clinical.Symptom) (clinical.Symptom, error)
	Update(ctx context.Context, symptom clinical.Symptom, expectedVersion string) (clinical.Symptom, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Authorizer is the patient checkpoint every kind registers the same
// implementation against.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
