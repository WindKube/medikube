package tag

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/tag"
)

// FieldName is the field a refused sort or a duplicate name is reported
// against.
const FieldName = "name"

// SortName and SortUsageDesc are contracts/tags.md §1's `sort` values: `name`
// (default) and `-usage`, the only two the list publishes.
func Sorts() []domain.SortKey {
	return []domain.SortKey{{Field: FieldName}, {Field: "usage", Desc: true}}
}

// Query is one list request, resolved.
type Query struct {
	// Search is `?q=`: a prefix/substring match over name, case-insensitive —
	// the autocomplete (FR-068).
	Search string

	Sort []domain.SortKey

	Limit  int
	Cursor string
	Count  bool
}

// Repository is the storage seam, declared by the consumer (Principle II).
//
// Every method is owner-scoped: List narrows in the predicate, Get/Update/
// Delete scope in the query itself rather than checking afterward, the same
// reasoning practitioner.Repository documents.
type Repository interface {
	List(ctx context.Context, ownerID string, query Query) (domain.Page[tag.Tag], error)

	// Get answers domain.ErrNotFound for a row that does not exist and for
	// one that is not this owner's, with the same error either way
	// (FR-062, US7-5).
	Get(ctx context.Context, ownerID, id string) (tag.Tag, error)

	// Create refuses a case-insensitive duplicate name for this owner as
	// tag.ErrDuplicateName (FR-063, US7-2): the unique index is the storage
	// layer's own enforcement and this is its translation.
	Create(ctx context.Context, t tag.Tag) (tag.Tag, error)

	// Update is one row update: the tag is a relation, not a copied string,
	// so every record carrying it follows with no second write (FR-065,
	// SC-007).
	Update(ctx context.Context, t tag.Tag) (tag.Tag, error)

	// Delete is permanent. PocketBase's own relation cleanup removes the tag
	// from every referencing record; this deletes only the tag's own row
	// (FR-066, US7-4).
	Delete(ctx context.Context, ownerID, id string) error
}

// Ownership is FR-064's check: every tag id a record patch names must belong
// to the actor. It is a separate interface from Repository (plan.md's
// interface-segregation cap) because internal/records.TagChecker consumes
// only this shape, through tag.Service.Owned.
type Ownership interface {
	// Owned answers whether every id in ids is a tag this owner holds.
	Owned(ctx context.Context, ownerID string, ids []string) (bool, error)
}

// UsageCounter is FR-068's derived count: how many records, across every
// registered kind, carry each tag (FR-090 — never stored).
type UsageCounter interface {
	Counts(ctx context.Context, ownerID string, ids []string) (map[string]int, error)
}

// Authorizer is the checkpoint this package consumes: may this actor reach
// their own tag vocabulary at all. Tags have no share and no level beyond
// ownership, the same as practitioner.Authorizer.
type Authorizer interface {
	Actor(ctx context.Context, actor access.Actor) (access.Grant, error)
}

// actorAuthorizer is the trivial default: an authenticated, non-superuser
// actor holds access.PermOwn over their own tags.
type actorAuthorizer struct{}

// DefaultAuthorizer is the default Authorizer.
var DefaultAuthorizer Authorizer = actorAuthorizer{}

func (actorAuthorizer) Actor(_ context.Context, actor access.Actor) (access.Grant, error) {
	if actor.Authenticated() && !actor.IsSuperuser {
		return access.Grant{Level: access.PermOwn}, nil
	}

	return access.Grant{}, nil
}

// Auditor writes the trail. This package reaches it for exactly one thing:
// the access_denied row a refusal produces.
type Auditor interface {
	Record(ctx context.Context, event audit.Event) error
}
