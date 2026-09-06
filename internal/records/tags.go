package records

import (
	"context"
	"fmt"

	"medikube/internal/domain/access"
	tagsvc "medikube/internal/service/tag"
)

// tagCatalogLimit is contracts/tags.md §1's own page cap: the account's whole
// tag catalog, not a search result, so a picker has the same "whole of what
// this account has" list the tag manager's own GET already returns.
const tagCatalogLimit = 200

// TagOption is one tag as a kind's Form can offer it in its picker: enough to
// render a suggestion and its usage count, with none of the tag service
// itself in reach of Views (Views.Form has no ctx or actor to reach it with).
type TagOption struct {
	ID         string
	Name       string
	Color      string
	UsageCount int
}

// TagCatalogResolve resolves the tag service lazily, the same shape
// search.Reader and patient.Service already reach this package through.
type TagCatalogResolve func() (*tagsvc.Service, error)

// AttachTagOptions populates record.Tags with the actor's whole tag catalog,
// so that whichever kind's Views.Form renders next can offer every tag as a
// suggestion without reaching the tag service itself. It is the one place
// this fetch happens: every page handler and the API's generic form
// re-render call it, once, before calling Views.Form — not once per kind.
//
// A nil resolve is a build that never wired tags in (most test harnesses)
// and is a deliberate no-op rather than an error: a form with no tag picker
// is a smaller build, not a broken one.
func AttachTagOptions(ctx context.Context, actor access.Actor, resolve TagCatalogResolve, record *Record) error {
	if resolve == nil || record == nil {
		return nil
	}

	service, err := resolve()
	if err != nil {
		return err
	}

	page, err := service.List(ctx, actor, tagsvc.Query{Limit: tagCatalogLimit})
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(page.Items))
	for _, t := range page.Items {
		ids = append(ids, t.ID)
	}

	usage, err := service.Usage(ctx, actor, ids)
	if err != nil {
		return err
	}

	options := make([]TagOption, 0, len(page.Items))
	for _, t := range page.Items {
		options = append(options, TagOption{ID: t.ID, Name: t.Name, Color: t.Color, UsageCount: usage[t.ID]})
	}

	record.Tags = options

	return nil
}

// SelectedTagIDs reads whichever tag ids a record's own Body already carries
// — Tagged for a detail read after a create or an update, Taggable for a
// submission still being validated or re-rendered — so a kind's Form can seed
// its picker's chips without any kind-specific mapping of its own.
func SelectedTagIDs(body any) []string {
	if tagged, ok := body.(Tagged); ok {
		return tagged.GetTags()
	}

	if taggable, ok := body.(Taggable); ok {
		if ids, supplied := taggable.TagIDs(); supplied {
			return ids
		}
	}

	return nil
}

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

// Tagged is implemented by a kind's detail DTO — what Record.Body carries
// after a create or an update (research D-11) — to expose the tags stored on
// it. indexingService reads this to keep search_index.tags in step with the
// record it was derived from, the same way SearchFields reads title and body
// off the same value.
type Tagged interface {
	GetTags() []string
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
