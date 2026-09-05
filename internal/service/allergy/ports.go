package allergy

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const (
	FieldAllergen = "allergen"
	FieldOnsetOn  = "onset_on"
	FieldUpdated  = "updated"
)

// FilterStatus, FilterSeverity and FilterCritical are the `?status=`,
// `?severity=` and `?critical=true` narrowings (data-model §4.1, contracts/
// pages.md §3.5).
const (
	FilterStatus   = "status"
	FilterSeverity = "severity"
	FilterCritical = "critical"
)

const ParamSort = "sort"

// MatchAny and MatchAll are `?tags=&match=` (FR-067, research D-10),
// mirroring internal/records.MatchAny/MatchAll: this package does not import
// internal/records, so it carries its own copy of the two spellings.
const (
	MatchAny = "any"
	MatchAll = "all"
)

// Sorts is the published ordering allowlist, most recent onset first
// (data-model §4.1's index).
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldOnsetOn, Desc: true},
		{Field: FieldOnsetOn},
		{Field: FieldAllergen},
		{Field: FieldUpdated, Desc: true},
	}
}

// Query is one list request as the service resolved it.
type Query struct {
	PatientID string
	Search    string

	Statuses   []clinical.ConditionStatus
	Severities []clinical.Severity
	// Critical narrows to Allergy.Critical() when true. nil means unnarrowed.
	Critical *bool

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

// Repository is the storage seam, declared by the consumer (Principle II).
type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Allergy], error)
	Get(ctx context.Context, id string) (clinical.Allergy, error)
	Create(ctx context.Context, entity clinical.Allergy) (clinical.Allergy, error)
	Update(ctx context.Context, entity clinical.Allergy, expectedVersion string) (clinical.Allergy, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Authorizer is THE authorization checkpoint, anchored on the patient
// (research D-13).
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
