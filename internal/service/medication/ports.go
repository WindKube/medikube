package medication

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// The columns this kind publishes an ordering over, and the one named parameter
// it publishes a narrowing over. They are the storage column names because the
// wire spelling and the column are the same word for all three (FR-022,
// contracts/records.md) — internal/store maps them straight through, and a
// column renamed on one side without the other is what T167 asserts against.
//
// They are constants here rather than in internal/store because the service
// publishes the vocabulary and the store answers it: a repository that could
// name its own sort fields would be a second allowlist.
const (
	FieldName      = "name"
	FieldStartedOn = "started_on"
	FieldUpdated   = "updated"
)

// FilterStatus is the `?status=` parameter, and the field a refusal of one of
// its values is attached to.
const FilterStatus = "status"

// ParamSort is the field a refused ordering is reported against. It is the
// query parameter's own name, because that is what the caller has to change.
const ParamSort = "sort"

// MatchAny and MatchAll are `?tags=&match=` (FR-067, research D-10),
// mirroring internal/records.MatchAny/MatchAll: this package does not import
// internal/records, so it carries its own copy of the two spellings.
const (
	MatchAny = "any"
	MatchAll = "all"
)

// Sorts is the published ordering allowlist, in the order OpenAPI publishes it.
// The first entry is the default, and contracts/records.md fixes it as most
// recently started.
//
// Cloned on every call, as every published vocabulary in this codebase is: a
// caller that sorted the result for one display would otherwise reorder the
// default for everybody.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldStartedOn, Desc: true},
		{Field: FieldStartedOn},
		{Field: FieldName},
		{Field: FieldName, Desc: true},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

// Query is one list request as the service resolved it: named parameters, a
// resolved ordering and an opaque boundary. There is nothing here a caller
// could put an expression in — PocketBase's filter DSL never reaches the wire
// and it does not reach this package either.
type Query struct {
	// PatientID is the person the list is scoped to (FR-023, research D-13).
	// It is required: a list request without one is refused before this
	// package is ever reached (contracts/medications-rescope.md).
	PatientID string

	// Search is the case-insensitive substring match over the name and the
	// alternative name (FR-022).
	Search string

	// Statuses narrows by state. Empty is every state, and a value outside the
	// published vocabulary is refused rather than dropped: a dropped term
	// narrows to everything and looks like a list that is simply long.
	Statuses []clinical.TherapyStatus

	// Tags and Match are `?tags=a,b&match=any|all` (FR-067).
	Tags  []string
	Match string

	// Sort is the resolved ordering, checked against Sorts() before the
	// repository sees it. The identity tiebreaker is the repository's and is
	// never named here.
	Sort []domain.SortKey

	// Limit is the page size the caller asked for. Zero means the
	// repository's published default; the bounds are the edge's to enforce,
	// and the repository refuses what gets past it.
	Limit int

	// Cursor is the opaque boundary the previous page returned. It is minted
	// and read by the repository — the only layer that can bind it to the
	// ordering and to the owner — and travels through this package unread.
	Cursor string

	Count bool
}

// Repository is the storage seam, declared by the consumer (Principle II).
//
// List is patient-scoped in SQL (FR-023). Get, Update and Delete are not
// scoped by patient at all: the service resolves and authorizes the patient
// from the row itself before touching it, the same shape
// access.Authorizer.Patient uses to resolve a patient's owner (research D-13).
// A row that is somebody else's patient is refused by the checkpoint before
// this package is ever reached for it, so a second, redundant scope here would
// only hide which layer actually decided.
//
// Five methods, which is plan.md's interface-segregation cap. There is no Save
// that means create-or-update: the two have different preconditions — one mints
// an identity, the other requires the version the caller last read — and a
// method that branched on an empty id would hide that.
//
// Implementations: internal/store/medication against PocketBase, and
// medicationtest's in-memory fake. Both pass medicationtest.RunRepositoryContract.
type Repository interface {
	// List returns one page of the patient's medications, ordered by
	// query.Sort with the identity as the tiebreaker, and mints the boundary
	// for the next page. A cursor this repository did not issue, or one issued
	// under another ordering, is an error and never an ignored parameter.
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Medication], error)

	// Get answers domain.ErrNotFound for a row that does not exist. It is not
	// scoped by patient: the service reads medication.PatientID off the result
	// and authorizes it there (FR-033).
	Get(ctx context.Context, id string) (clinical.Medication, error)

	// Create mints the identity, the timestamps and the version, and returns
	// the row as stored. Whatever the entity carried in those four fields is
	// not read.
	Create(ctx context.Context, medication clinical.Medication) (clinical.Medication, error)

	// Update writes the entity over the row it identifies — medication.ID —
	// but only while the stored version is still expectedVersion. A mismatch
	// is domain.ErrVersionMismatch and writes nothing.
	//
	// The comparison and the write are one act. Read-then-write in the service
	// would let two callers both read the same version and both be told they
	// were current.
	Update(ctx context.Context, medication clinical.Medication, expectedVersion string) (clinical.Medication, error)

	// Delete is permanent (FR-028) and takes the version for the same reason
	// Update does: deleting the row you last saw is a different act from
	// deleting whatever is there now.
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Authorizer is THE authorization checkpoint, as this package consumes it: the
// patient anchor (contracts/medications-rescope.md, research D-13). One
// implementation, internal/service/access, satisfies this.
//
// It answers with an error on every refusal rather than a Grant of zero value
// and it audits its own denial (FR-045): unlike phase 001's Record/Kind pair,
// this package writes no audit row of its own. A refusal is domain.ErrNotFound
// whatever the reason, never domain.ErrForbidden — a patient's existence is
// itself PHI (FR-042).
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
