package facility

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/directory"
)

// The columns a facility list is filtered and ordered by. Constants rather
// than string literals because the wire spelling, the query parameter and the
// storage column are one word, and internal/store maps it straight through
// (contracts/facilities.md: "Sort: kind, name, id").
const (
	FieldKind = "kind"
	FieldName = "name"
)

// FilterKind is the `?kind=` parameter, and the field a refused kind is
// attached to.
const FilterKind = "kind"

// Sorts is the one ordering contracts/facilities.md publishes: kind, then
// name, with the identity as the repository's own tiebreaker. There is no
// `?sort=` for this resource — unlike medication, the directory has exactly
// one published ordering and nothing negotiates it.
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldKind},
		{Field: FieldName},
	}
}

// Query is one list request: a substring narrowing over name and brand, a
// kind narrowing, and an opaque paging boundary. There is no expression here a
// caller could inject — the same discipline every list in this codebase
// holds to.
type Query struct {
	// Search is the case-insensitive substring match over the name and the
	// brand (FR-036).
	Search string

	// Kind narrows to one member of the published vocabulary. Empty is every
	// kind, and a value outside the vocabulary is refused rather than
	// dropped: a dropped term narrows to everything and looks like a list
	// that is simply long.
	Kind directory.FacilityKind

	Limit  int
	Cursor string
	Count  bool
}

// Usage is how much of the directory a facility is load-bearing for: the
// practitioners that point at it and the medications whose pharmacy it is.
// It is what a delete would silently orphan if the reference cascaded, and it
// is why it does not (research D-06): both counts stay answerable, and the
// caller decides, forever, whether to delete anyway.
type Usage struct {
	Practitioners int
	Records       int
}

// Repository is the storage seam, declared by the consumer (Principle II).
//
// Every CRUD method is owner-scoped, and that is deliberate duplication: the
// service authorizes above this via Owner and the repository refuses a row
// that is not the owner's anyway. Two independent refusals, because either
// one of them is one edit away from not being there.
//
// Implementations: internal/store/facility against PocketBase, and
// facilitytest's in-memory fake. Both pass facilitytest.RunRepositoryContract.
type Repository interface {
	// List returns one page of the owner's facilities, ordered kind, name,
	// with the identity as the tiebreaker, and mints the boundary for the
	// next page.
	List(ctx context.Context, ownerID string, query Query) (domain.Page[directory.Facility], error)

	// Get answers domain.ErrNotFound for a row that does not exist and for
	// one that is not this owner's, with the same error either way (FR-037).
	Get(ctx context.Context, ownerID, id string) (directory.Facility, error)

	// Create mints the identity, the timestamps and the version, and returns
	// the row as stored.
	Create(ctx context.Context, facility directory.Facility) (directory.Facility, error)

	// Update writes the entity over the row it identifies — facility.ID
	// addressed within facility.OwnerID — but only while the stored version
	// is still expectedVersion.
	Update(ctx context.Context, facility directory.Facility, expectedVersion string) (directory.Facility, error)

	// Delete is permanent. References from practitioners.facility and
	// medications.pharmacy are unset, not cascaded (research D-06): the
	// practitioner and the medication both survive.
	Delete(ctx context.Context, ownerID, id, expectedVersion string) error

	// Owner answers who owns id, or domain.ErrNotFound if it does not exist.
	// The service reaches this before Get/Update/Delete to tell "does not
	// exist" apart from "exists and is somebody else's" — the second is what
	// gets audited, and this is the only method that can answer which one it
	// is without also performing the write or the read.
	Owner(ctx context.Context, id string) (string, error)

	// Usage counts what points at id: practitioners whose facility this is,
	// and medications whose pharmacy this is.
	Usage(ctx context.Context, ownerID, id string) (Usage, error)
}

// Authorizer is the one checkpoint this package consumes.
//
// It has a single method because a facility has no anchor beyond the account
// that owns it (FR-037): there is no patient, no share and no kind-specific
// rule to resolve — only "is this actor a real account, acting for itself".
// Which record and which owner is a separate question, answered by
// Repository.Owner rather than by this interface, because that answer needs
// the row to exist and this one does not.
type Authorizer interface {
	Actor(ctx context.Context, actor access.Actor) (access.Grant, error)
}

// Auditor writes the trail. This package reaches it for exactly one thing:
// the access_denied row a refusal produces.
type Auditor interface {
	Record(ctx context.Context, event audit.Event) error
}
