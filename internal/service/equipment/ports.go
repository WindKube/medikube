package equipment

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// The columns this kind publishes an ordering over.
const (
	FieldName         = "name"
	FieldPrescribedOn = "prescribed_on"
	FieldUpdated      = "updated"
)

// FilterType, FilterStatus are the `?type=`/`?status=` parameters.
const (
	FilterType   = "type"
	FilterStatus = "status"
)

// ParamServiceDueWithin is `?service_due_within_days=` (default 30, FR-049).
const ParamServiceDueWithin = "service_due_within_days"

// DefaultServiceDueWithinDays is contracts/records-clinical.md §1's default.
const DefaultServiceDueWithinDays = 30

// ParamSort is the field a refused ordering is reported against.
const ParamSort = "sort"

// Sorts is the published ordering allowlist, first entry the default.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldPrescribedOn, Desc: true},
		{Field: FieldPrescribedOn},
		{Field: FieldName},
		{Field: FieldName, Desc: true},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

// Query is one list request as the service resolved it.
type Query struct {
	PatientID string
	Search    string

	Types    []clinical.EquipmentType
	Statuses []clinical.TherapyStatus

	// ServiceDueWithinDays selects rows whose service is overdue or falls due
	// within this many days (FR-049). Nil means the narrowing is not applied.
	ServiceDueWithinDays *int

	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
}

// Repository is the storage seam.
type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Equipment], error)
	Get(ctx context.Context, id string) (clinical.Equipment, error)
	Create(ctx context.Context, entity clinical.Equipment) (clinical.Equipment, error)
	Update(ctx context.Context, entity clinical.Equipment, expectedVersion string) (clinical.Equipment, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Authorizer is the patient anchor checkpoint.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
