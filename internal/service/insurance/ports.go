package insurance

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// The columns this kind publishes an ordering over.
const (
	FieldCompany     = "company"
	FieldEffectiveOn = "effective_on"
	FieldUpdated     = "updated"
)

// FilterType, FilterStatus, FilterIsPrimary are the narrowing parameters.
const (
	FilterType      = "type"
	FilterStatus    = "status"
	FilterIsPrimary = "is_primary"
)

// ParamExpiringWithin is `?expiring_within_days=` (default 60, FR-046).
const ParamExpiringWithin = "expiring_within_days"

// DefaultExpiringWithinDays is contracts/records-clinical.md §1's default.
const DefaultExpiringWithinDays = 60

// ParamSort is the field a refused ordering is reported against.
const ParamSort = "sort"

// Sorts is the published ordering allowlist, first entry the default.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldEffectiveOn, Desc: true},
		{Field: FieldEffectiveOn},
		{Field: FieldCompany},
		{Field: FieldCompany, Desc: true},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

// Query is one list request as the service resolved it.
type Query struct {
	PatientID string
	Search    string

	Types     []clinical.InsuranceType
	Statuses  []clinical.InsuranceStatus
	IsPrimary *bool

	// ExpiringWithinDays selects policies whose cover ends within this many
	// days (FR-046). Nil means the narrowing is not applied.
	ExpiringWithinDays *int

	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
}

// Displaced names the policy a primary-flag change unset (FR-045).
type Displaced struct {
	ID string
}

// Repository is the storage seam. Create and Update return the policy this
// write displaced from primary, if any: the displacement and the write are one
// transaction (FR-045), so the repository — the layer that owns transactions
// — is what reports it.
type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Insurance], error)
	Get(ctx context.Context, id string) (clinical.Insurance, error)
	Create(ctx context.Context, entity clinical.Insurance) (clinical.Insurance, *Displaced, error)
	Update(ctx context.Context, entity clinical.Insurance, expectedVersion string) (clinical.Insurance, *Displaced, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Authorizer is the patient anchor checkpoint.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
