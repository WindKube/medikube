package records

import (
	"context"
	"fmt"

	"medikube/internal/domain/access"
)

// The `?tags=a,b&match=any|all` narrowing every registered kind publishes
// (FR-067, research D-10). Declared once here so a kind cannot spell either
// parameter differently from another.
const (
	FilterTags  = "tags"
	FilterMatch = "match"

	MatchAny = "any"
	MatchAll = "all"
)

// TagFilters is the FilterSpec pair a kind's Register merges into its own
// Schema.Filters. `match` defaults to MatchAny so `?tags=a,b` alone is a
// disjunction, and `tags` has no declared vocabulary of its own: the ids it
// names are checked against the caller's own tags by TagChecker, not against
// a fixed list.
func TagFilters() map[string]FilterSpec {
	return map[string]FilterSpec{
		FilterTags: {Name: FilterTags, Kind: FilterFreeform},
		FilterMatch: {
			Name: FilterMatch, Kind: FilterEnum,
			Allowed: []string{MatchAny, MatchAll}, Default: MatchAny,
		},
	}
}

// Taggable is implemented by a kind's create and patch DTOs when they carry
// the universal `tags` field (data-model §0.8). Supplied is false for a patch
// that omitted the field — "leave the tags alone" — and true for a create or
// a patch that named the field at all, including with an empty list, which
// clears it.
type Taggable interface {
	TagIDs() (ids []string, supplied bool)
}

// TagChecker is the account's tag ownership check (contracts/tags.md §5):
// every tag id a record patch names must belong to the actor, or the write is
// refused as domain.ErrNotFound — identical to naming a tag that does not
// exist at all, so a foreign tag id discloses nothing about the tag it names.
//
// One implementation, internal/service/tag, is wired into every registry by
// SetTagChecker so this check runs for every kind without any kind's own
// service knowing tags exist.
type TagChecker interface {
	Owned(ctx context.Context, actor access.Actor, ids []string) error
}

// SetTagChecker wires FR-064's ownership check into every kind registered
// from this point on. Like SetIndexer it is a setter and not a NewRegistry
// parameter: the registry is built once, empty, from internal/di, before the
// tag service it would need exists.
func (r *Registry) SetTagChecker(checker TagChecker) { r.tagChecker = checker }

// tagCheckingService decorates a kind's Service with FR-064's ownership
// check, the same way indexingService decorates it with the search index's
// write side: neither the kind's own service nor its store knows the check
// runs, and a kind added later gets it for free by registering.
type tagCheckingService struct {
	Service

	checker TagChecker
}

func (s *tagCheckingService) Create(ctx context.Context, actor access.Actor, body any) (Record, error) {
	if err := s.check(ctx, actor, body); err != nil {
		return Record{}, err
	}

	return s.Service.Create(ctx, actor, body)
}

func (s *tagCheckingService) Update(ctx context.Context, actor access.Actor, id, version string, body any) (Record, error) {
	if err := s.check(ctx, actor, body); err != nil {
		return Record{}, err
	}

	return s.Service.Update(ctx, actor, id, version, body)
}

func (s *tagCheckingService) check(ctx context.Context, actor access.Actor, body any) error {
	tagged, ok := body.(Taggable)
	if !ok {
		return nil
	}

	ids, supplied := tagged.TagIDs()
	if !supplied || len(ids) == 0 {
		return nil
	}

	if err := s.checker.Owned(ctx, actor, ids); err != nil {
		return fmt.Errorf("records: checking tag ownership: %w", err)
	}

	return nil
}
