package familymember

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// The columns this kind publishes an ordering over.
const (
	FieldRelationship = "relationship"
	FieldName         = "name"
	FieldUpdated      = "updated"
)

// FilterRelationship is the `?relationship=` narrowing parameter.
const FilterRelationship = "relationship"

// ParamSort is the field a refused ordering is reported against.
const ParamSort = "sort"

// MatchAny and MatchAll are `?tags=&match=` (FR-067, research D-10),
// mirroring internal/records.MatchAny/MatchAll: this package does not import
// internal/records, so it carries its own copy of the two spellings.
const (
	MatchAny = "any"
	MatchAll = "all"
)

// Sorts is the published ordering allowlist, first entry the default:
// data-model §4.13's relationship ASC, LOWER(name) ASC, id DESC.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldRelationship},
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

	Relationships []clinical.FamilyRelationship

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

// Repository is the storage seam.
type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.FamilyMember], error)
	Get(ctx context.Context, id string) (clinical.FamilyMember, error)
	Create(ctx context.Context, entity clinical.FamilyMember) (clinical.FamilyMember, error)
	Update(ctx context.Context, entity clinical.FamilyMember, expectedVersion string) (clinical.FamilyMember, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Authorizer is the patient anchor checkpoint.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
