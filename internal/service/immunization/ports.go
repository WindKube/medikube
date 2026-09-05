// Package immunization is the immunization kind's use cases: the ports it
// declares, and the service that authorizes and validates against them.
package immunization

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// The columns this kind publishes an ordering over.
const (
	FieldAdministeredOn = "administered_on"
	FieldVaccineName    = "vaccine_name"
	FieldUpdated        = "updated"
)

// ParamSort is the field a refused ordering is reported against.
const ParamSort = "sort"

// MatchAny and MatchAll are `?tags=&match=` (FR-067, research D-10),
// mirroring internal/records.MatchAny/MatchAll: this package does not import
// internal/records, so it carries its own copy of the two spellings.
const (
	MatchAny = "any"
	MatchAll = "all"
)

// Sorts is the published ordering allowlist, most recently administered
// first by default (data-model §3).
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldAdministeredOn, Desc: true},
		{Field: FieldAdministeredOn},
		{Field: FieldVaccineName},
		{Field: FieldVaccineName, Desc: true},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

// Query is one list request as the service resolved it.
type Query struct {
	PatientID string
	Search    string

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
// Five methods, following medication's Repository shape exactly.
type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Immunization], error)
	Get(ctx context.Context, id string) (clinical.Immunization, error)
	Create(ctx context.Context, immunization clinical.Immunization) (clinical.Immunization, error)
	Update(ctx context.Context, immunization clinical.Immunization, expectedVersion string) (clinical.Immunization, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Authorizer is the patient checkpoint, as this package consumes it.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
