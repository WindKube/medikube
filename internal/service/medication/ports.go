package medication

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
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
	// Search is the case-insensitive substring match over the name and the
	// alternative name (FR-022).
	Search string

	// Statuses narrows by state. Empty is every state, and a value outside the
	// published vocabulary is refused rather than dropped: a dropped term
	// narrows to everything and looks like a list that is simply long.
	Statuses []clinical.TherapyStatus

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
// Every method is owner-scoped, and that is deliberate duplication: the service
// authorizes above this and the repository refuses a row that is not the
// owner's anyway. Two independent refusals, because either one of them is one
// edit away from not being there, and the failure they guard is the one where
// somebody reads another person's medication.
//
// Five methods, which is plan.md's interface-segregation cap. There is no Save
// that means create-or-update: the two have different preconditions — one mints
// an identity, the other requires the version the caller last read — and a
// method that branched on an empty id would hide that.
//
// Implementations: internal/store/medication against PocketBase, and
// medicationtest's in-memory fake. Both pass medicationtest.RunRepositoryContract.
type Repository interface {
	// List returns one page of the owner's medications, ordered by query.Sort
	// with the identity as the tiebreaker, and mints the boundary for the next
	// page. A cursor this repository did not issue, or one issued under
	// another ordering, is an error and never an ignored parameter.
	List(ctx context.Context, ownerID string, query Query) (domain.Page[clinical.Medication], error)

	// Get answers domain.ErrNotFound for a row that does not exist and for one
	// that is not this owner's, with the same error either way (FR-033).
	Get(ctx context.Context, ownerID, id string) (clinical.Medication, error)

	// Create mints the identity, the timestamps and the version, and returns
	// the row as stored. Whatever the entity carried in those four fields is
	// not read.
	Create(ctx context.Context, medication clinical.Medication) (clinical.Medication, error)

	// Update writes the entity over the row it identifies — medication.ID
	// addressed within medication.OwnerID — but only while the stored version
	// is still expectedVersion. A mismatch is domain.ErrVersionMismatch and
	// writes nothing; a row that is not this owner's is domain.ErrNotFound.
	//
	// The comparison and the write are one act. Read-then-write in the service
	// would let two callers both read the same version and both be told they
	// were current.
	Update(ctx context.Context, medication clinical.Medication, expectedVersion string) (clinical.Medication, error)

	// Delete is permanent (FR-028) and takes the version for the same reason
	// Update does: deleting the row you last saw is a different act from
	// deleting whatever is there now.
	Delete(ctx context.Context, ownerID, id, expectedVersion string) error
}

// Authorizer is THE authorization checkpoint, as this package consumes it. One
// implementation, internal/service/access, satisfies this and the identically
// shaped records.Authorizer the registry and the stream consume.
//
// Two methods because a list has no record to name. Overloading Record with an
// empty id would mean an implementation that answered it as a record lookup
// would refuse every list — and would look, from here, exactly like an
// authorization decision.
type Authorizer interface {
	// Record is the checkpoint for one addressed record. The refusal for a
	// record that exists and is not the actor's is domain.ErrNotFound, never
	// domain.ErrForbidden (FR-033).
	//
	// A record that does not exist is not this method's refusal to make: it
	// grants, and the repository reports the miss. That is what keeps a
	// genuine not-found out of the audit trail while every real refusal is in
	// it (research D-20).
	Record(ctx context.Context, actor access.Actor, k kind.Kind, recordID string, need access.Permission) (access.Grant, error)

	// Kind is the checkpoint for the kind itself: may this actor reach these
	// records at all, and at what level. It is what a list and a create are
	// authorized against, neither of which names an existing record.
	Kind(ctx context.Context, actor access.Actor, k kind.Kind, need access.Permission) (access.Grant, error)
}

// Auditor writes the trail. This package reaches it for exactly one thing: the
// access_denied row a refusal produces (data-model §3, research D-20).
//
// The create, update and delete rows are NOT written here. They are written by
// the post-commit hooks in internal/platform/pb, because a row written beside
// the service call is a row that survives a rolled-back transaction — an audit
// trail claiming a change that did not happen (contracts/records.md, T160).
type Auditor interface {
	Record(ctx context.Context, event audit.Event) error
}
