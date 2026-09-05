package practitioner

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/directory"
)

// FieldName is the one column the directory is ordered by (contracts/
// practitioners.md: "Sort: name, id"). There is no second ordering to publish:
// unlike medications' therapy timeline, a directory entry has no started_on
// and no updated-recency the contract asks for.
const FieldName = "name"

// FilterSpecialty and FilterFacility are the `?specialty=` and `?facility=`
// query parameters, and the fields a refused value is reported against.
const (
	FilterSpecialty = "specialty"
	FilterFacility  = "facility"
)

// Sorts is the published ordering allowlist. One entry, ascending by name,
// because that is the only ordering the contract offers — this list exists so
// Query.resolve refuses anything else rather than silently ignoring it.
func Sorts() []domain.SortKey {
	return []domain.SortKey{{Field: FieldName}}
}

// Query is one list request as the service resolved it.
type Query struct {
	// Search is the case-insensitive substring match over the name (FR-039's
	// type-ahead and the directory page share this one operation).
	Search string

	// Specialty narrows to one specialty. Empty is every specialty.
	Specialty directory.Specialty

	// FacilityID narrows to practitioners at one facility. Empty is every
	// facility.
	FacilityID string

	Sort []domain.SortKey

	Limit int

	Cursor string

	Count bool
}

// Usage answers FR-040 without a second round trip: how many patients name
// this practitioner as their primary one, and how many clinical records
// reference them.
type Usage struct {
	Patients int
	Records  int
}

// Repository is the storage seam, declared by the consumer (Principle II).
//
// Every method is owner-scoped, and that is deliberate duplication: the
// service authorizes above this and the repository refuses a row that is not
// the owner's anyway — the same reasoning medication.Repository documents.
//
// Implementations: internal/store/practitioner against PocketBase, and
// practitionertest's in-memory fake. Both pass
// practitionertest.RunRepositoryContract.
type Repository interface {
	// List returns one page of the owner's directory, ordered by query.Sort
	// with the identity as the tiebreaker.
	List(ctx context.Context, ownerID string, query Query) (domain.Page[directory.Practitioner], error)

	// Get answers domain.ErrNotFound for a row that does not exist and for one
	// that is not this owner's, with the same error either way (FR-037).
	Get(ctx context.Context, ownerID, id string) (directory.Practitioner, error)

	// Create mints the identity, the timestamps and the version. A facility
	// named on the draft that does not belong to ownerID is refused as
	// domain.ErrNotFound (FR-042). A duplicate (owner, LOWER(name), specialty)
	// is refused as domain.ErrConflict (FR-038).
	Create(ctx context.Context, practitioner directory.Practitioner) (directory.Practitioner, error)

	// Update writes the entity over the row it identifies, only while the
	// stored version is still expectedVersion. The same facility-ownership and
	// uniqueness refusals as Create apply.
	Update(ctx context.Context, practitioner directory.Practitioner, expectedVersion string) (directory.Practitioner, error)

	// Delete is permanent. Every referencing record — a patient's
	// primary_practitioner, a medication's practitioner — survives with the
	// reference cleared (contracts/practitioners.md).
	Delete(ctx context.Context, ownerID, id, expectedVersion string) error

	// Owner answers the account a row belongs to, or domain.ErrNotFound for a
	// row that does not exist. It exists purely to let the service detect a
	// cross-owner access attempt worth auditing: the CRUD methods above already
	// scope by owner and answer ErrNotFound for a row that is not the caller's
	// on their own.
	Owner(ctx context.Context, id string) (string, error)

	// Usage counts what references this practitioner: patients naming it as
	// their primary practitioner, and clinical records (medications, this
	// phase) naming it as the prescriber.
	Usage(ctx context.Context, ownerID, id string) (Usage, error)
}

// Authorizer is the checkpoint this package consumes. It answers one question
// — may this actor reach the directory at all — because the directory has no
// share and no level beyond ownership in this phase: every authenticated,
// non-superuser actor who owns the row may do anything to it.
type Authorizer interface {
	Actor(ctx context.Context, actor access.Actor) (access.Grant, error)
}

// actorAuthorizer is the trivial default: an authenticated, non-superuser
// actor holds access.PermOwn over their own directory, and nobody else holds
// anything. It carries no configuration because there is nothing this phase
// lets an operator tune about it.
type actorAuthorizer struct{}

// DefaultAuthorizer is the default Authorizer, for wiring that has nothing
// more specific to construct.
var DefaultAuthorizer Authorizer = actorAuthorizer{}

func (actorAuthorizer) Actor(_ context.Context, actor access.Actor) (access.Grant, error) {
	if actor.Authenticated() && !actor.IsSuperuser {
		return access.Grant{Level: access.PermOwn}, nil
	}

	return access.Grant{}, nil
}

// Auditor writes the trail. This package reaches it for exactly one thing: the
// access_denied row a refusal produces.
type Auditor interface {
	Record(ctx context.Context, event audit.Event) error
}
