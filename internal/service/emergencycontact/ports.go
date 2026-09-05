package emergencycontact

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// FR-051's default sort has no primary date: active first, then primary,
// then the name folded, then the identity.
const (
	FieldIsActive  = "is_active"
	FieldIsPrimary = "is_primary"
	FieldName      = "name"
)

const (
	FilterIsActive = "is_active"
)

const ParamSort = "sort"

// MatchAny and MatchAll are `?tags=&match=` (FR-067, research D-10),
// mirroring internal/records.MatchAny/MatchAll: this package does not import
// internal/records, so it carries its own copy of the two spellings.
const (
	MatchAny = "any"
	MatchAll = "all"
)

// Sorts is the one published ordering FR-051 fixes: is_active DESC,
// is_primary DESC, LOWER(name) ASC — id DESC is the repository's own
// tiebreaker and is not part of the published vocabulary.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldIsActive, Desc: true},
		{Field: FieldIsPrimary, Desc: true},
		{Field: FieldName},
	}
}

type Query struct {
	PatientID string
	Search    string
	IsActive  *bool

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

// Repository is the storage seam. Create and Update apply FR-045/FR-051's
// primary displacement transactionally and report it on the returned
// entity's DisplacedID.
type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.EmergencyContact], error)
	Get(ctx context.Context, id string) (clinical.EmergencyContact, error)
	Create(ctx context.Context, entity clinical.EmergencyContact) (clinical.EmergencyContact, error)
	Update(ctx context.Context, entity clinical.EmergencyContact, expectedVersion string) (clinical.EmergencyContact, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
