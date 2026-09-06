// Package tag is the account's own tag vocabulary (US7, data-model §5.1):
// name and color, applied across any record of any kind. Tags belong to the
// account and never to a patient (FR-062).
package tag

import (
	"context"
	"errors"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/tag"
)

// ErrDuplicateName is Create and Update's refusal of a case-insensitive
// duplicate within the owner's own tags (FR-063, US7-2). It is a plain
// sentinel and not a *web.Coded: this package may not import internal/web,
// and the handler translates it into web.ErrDuplicateName the same way
// patient.ErrSelfRecordProtected is translated into web.ErrSelfRecordProtected.
var ErrDuplicateName = errors.New("tag: this owner already has a tag under that name")

// Patch is a change to a tag: every field optional.
type Patch struct {
	Name  *string
	Color *string
}

// Service is the tag use cases. Every method authorizes first.
type Service struct {
	repository Repository
	ownership  Ownership
	usage      UsageCounter
	authorizer Authorizer
	auditor    Auditor
}

// New refuses an incomplete service rather than returning one.
func New(repository Repository, ownership Ownership, usage UsageCounter, authorizer Authorizer, auditor Auditor) (*Service, error) {
	var missing []string

	if repository == nil {
		missing = append(missing, "repository")
	}

	if ownership == nil {
		missing = append(missing, "ownership check")
	}

	if usage == nil {
		missing = append(missing, "usage counter")
	}

	if authorizer == nil {
		missing = append(missing, "authorizer")
	}

	if auditor == nil {
		missing = append(missing, "auditor")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("tag: the service is wired with no %s", joinWords(missing))
	}

	return &Service{
		repository: repository, ownership: ownership, usage: usage,
		authorizer: authorizer, auditor: auditor,
	}, nil
}

// List is the owner's own tags, one page at a time.
func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[tag.Tag], error) {
	if err := s.authorize(ctx, actor, ""); err != nil {
		return domain.Page[tag.Tag]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[tag.Tag]{}, err
	}

	return s.repository.List(ctx, actor.UserID, resolved)
}

// Usage is FR-068's per-tag counts, answered in one call rather than one per
// tag: every id in ids is assumed to belong to this actor already (List just
// read them), so this does not re-check ownership.
func (s *Service) Usage(ctx context.Context, actor access.Actor, ids []string) (map[string]int, error) {
	if err := s.authorize(ctx, actor, ""); err != nil {
		return nil, err
	}

	return s.usage.Counts(ctx, actor.UserID, ids)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (tag.Tag, error) {
	if err := s.authorize(ctx, actor, id); err != nil {
		return tag.Tag{}, err
	}

	return s.repository.Get(ctx, actor.UserID, id)
}

// Create stores a new tag for the actor.
func (s *Service) Create(ctx context.Context, actor access.Actor, draft tag.Tag) (tag.Tag, error) {
	if err := s.authorize(ctx, actor, ""); err != nil {
		return tag.Tag{}, err
	}

	draft.ID = ""
	draft.OwnerID = actor.UserID
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return tag.Tag{}, err
	}

	return s.repository.Create(ctx, draft)
}

// Update applies the supplied fields and nothing else. There is no If-Match
// here: a tag is not a clinical record and FR-005's concurrency rule is
// scoped to records (contracts/tags.md §3), a deliberate narrowing.
func (s *Service) Update(ctx context.Context, actor access.Actor, id string, patch Patch) (tag.Tag, error) {
	if err := s.authorize(ctx, actor, id); err != nil {
		return tag.Tag{}, err
	}

	current, err := s.repository.Get(ctx, actor.UserID, id)
	if err != nil {
		return tag.Tag{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return tag.Tag{}, err
	}

	return s.repository.Update(ctx, changed)
}

// Delete is permanent (FR-066): every referencing record survives with the
// tag removed, never destroyed.
func (s *Service) Delete(ctx context.Context, actor access.Actor, id string) error {
	if err := s.authorize(ctx, actor, id); err != nil {
		return err
	}

	return s.repository.Delete(ctx, actor.UserID, id)
}

// Owned is internal/records.TagChecker's shape: FR-064's ownership check on
// the ids a record's Patch names. It answers domain.ErrNotFound — never
// domain.ErrForbidden — for a foreign id, identical to a nonexistent one
// (contracts/tags.md §5).
func (s *Service) Owned(ctx context.Context, actor access.Actor, ids []string) error {
	if err := s.authorize(ctx, actor, ""); err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	owned, err := s.ownership.Owned(ctx, actor.UserID, ids)
	if err != nil {
		return fmt.Errorf("tag: checking ownership: %w", err)
	}

	if !owned {
		return fmt.Errorf("tag: one or more ids are not this actor's own tags: %w", domain.ErrNotFound)
	}

	return nil
}

// authorize is the one checkpoint every method runs through.
func (s *Service) authorize(ctx context.Context, actor access.Actor, id string) error {
	grant, err := s.authorizer.Actor(ctx, actor)

	switch {
	case err == nil && grant.Allows(access.PermOwn):
		return nil
	case err == nil, errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return s.denied(ctx, actor, id)
	default:
		return fmt.Errorf("tag: the authorization checkpoint could not answer: %w", err)
	}
}

func (s *Service) denied(ctx context.Context, actor access.Actor, id string) error {
	refusal := fmt.Errorf("tag: the actor may not reach that record: %w", domain.ErrNotFound)

	if err := s.auditor.Record(ctx, denial(actor, id)); err != nil {
		return errors.Join(refusal, fmt.Errorf("tag: the refusal was not recorded: %w", err))
	}

	return refusal
}

func denial(actor access.Actor, id string) audit.Event {
	event := audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionAccessDenied,
		TargetKind: audit.TargetKindTag,
		TargetID:   truncate(id, audit.MaxTargetID),
		RequestID:  actor.RequestID,
	}

	switch {
	case actor.IsSuperuser:
		event.ActorKind = audit.ActorKindSuperuser
	case !actor.Authenticated():
		event.ActorKind = audit.ActorKindSystem
		event.TargetKind = audit.TargetKindSystem
		event.TargetID = ""
	}

	return event
}

// resolve applies the published vocabulary.
func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	published := Sorts()

	if len(q.Sort) == 0 {
		q.Sort = published[:1]
	}

	for _, term := range q.Sort {
		if !containsSort(published, term) {
			invalid.Add("sort", domain.CodeInvalidValue, "not one of the orderings MediKube publishes")

			break
		}
	}

	if err := invalid.OrNil(); err != nil {
		return Query{}, err
	}

	return q, nil
}

func containsSort(published []domain.SortKey, term domain.SortKey) bool {
	for _, key := range published {
		if key == term {
			return true
		}
	}

	return false
}

func (p Patch) applyTo(t tag.Tag) tag.Tag {
	assign(&t.Name, p.Name)
	assign(&t.Color, p.Color)

	return t
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	return string(runes[:limit])
}

func joinWords(words []string) string {
	switch len(words) {
	case 1:
		return words[0]
	case 2:
		return words[0] + " and no " + words[1]
	default:
		return words[0] + ", no " + joinWords(words[1:])
	}
}
