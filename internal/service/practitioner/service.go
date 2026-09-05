package practitioner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/directory"
)

// Patch is a change to a practitioner: every field optional, and a supplied
// field's zero value is a value the person chose rather than a field they left
// alone.
//
// FacilityID follows directory.Practitioner's own convention for "none": nil
// means leave the facility alone, and a non-nil pointer to the empty string is
// the explicit clear — the same "leave alone" vs. "clear" distinction
// medication.Patch's dates draw with domain.Date's zero value.
type Patch struct {
	Name       *string
	Specialty  *directory.Specialty
	FacilityID *string
	Phone      *string
	Email      *string
	Website    *string
	Notes      *string
}

// Service is the practitioner directory use cases. Every method authorizes
// first: the checkpoint, then the rules, then the store.
type Service struct {
	repository Repository
	authorizer Authorizer
	auditor    Auditor
}

// New refuses an incomplete service rather than returning one.
func New(repository Repository, authorizer Authorizer, auditor Auditor) (*Service, error) {
	var missing []string

	if repository == nil {
		missing = append(missing, "repository")
	}

	if authorizer == nil {
		missing = append(missing, "authorizer")
	}

	if auditor == nil {
		missing = append(missing, "auditor")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("practitioner: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer, auditor: auditor}, nil
}

// List is the owner's directory, one page at a time.
func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[directory.Practitioner], error) {
	if err := s.authorize(ctx, actor, ""); err != nil {
		return domain.Page[directory.Practitioner]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[directory.Practitioner]{}, err
	}

	return s.repository.List(ctx, actor.UserID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (directory.Practitioner, error) {
	if err := s.authorizeRecord(ctx, actor, id); err != nil {
		return directory.Practitioner{}, err
	}

	return s.repository.Get(ctx, actor.UserID, id)
}

// Create stores a new practitioner for the actor.
//
// The four server-owned fields are taken from the draft and thrown away
// rather than trusted, mirroring medication.Service.Create's reasoning
// (FR-032).
func (s *Service) Create(ctx context.Context, actor access.Actor, draft directory.Practitioner) (directory.Practitioner, error) {
	if err := s.authorize(ctx, actor, ""); err != nil {
		return directory.Practitioner{}, err
	}

	draft.ID = ""
	draft.OwnerID = actor.UserID
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return directory.Practitioner{}, err
	}

	return s.repository.Create(ctx, draft)
}

// Update applies the supplied fields and nothing else.
func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (directory.Practitioner, error) {
	if err := s.authorizeRecord(ctx, actor, id); err != nil {
		return directory.Practitioner{}, err
	}

	current, err := s.repository.Get(ctx, actor.UserID, id)
	if err != nil {
		return directory.Practitioner{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return directory.Practitioner{}, err
	}

	return s.repository.Update(ctx, changed, version)
}

// Delete is permanent. Every referencing record survives with the reference
// cleared (contracts/practitioners.md).
func (s *Service) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	if err := s.authorizeRecord(ctx, actor, id); err != nil {
		return err
	}

	return s.repository.Delete(ctx, actor.UserID, id, version)
}

// Usage answers FR-040 without a second round trip: how many patients name
// this practitioner as their primary one, and how many clinical records
// reference them. Mirrors facility.Service.Usage.
func (s *Service) Usage(ctx context.Context, actor access.Actor, id string) (Usage, error) {
	if err := s.authorizeRecord(ctx, actor, id); err != nil {
		return Usage{}, err
	}

	return s.repository.Usage(ctx, actor.UserID, id)
}

// authorize is the kind-level checkpoint List and Create authorize against:
// may this actor reach the directory at all.
func (s *Service) authorize(ctx context.Context, actor access.Actor, id string) error {
	grant, err := s.authorizer.Actor(ctx, actor)

	return s.decide(ctx, actor, id, grant, err)
}

// authorizeRecord is Get, Update and Delete's checkpoint: the actor must hold
// the directory at all, and the row addressed must be theirs. Repository.Owner
// exists purely to detect the second half — the repository's own CRUD methods
// independently refuse a row that is not the owner's, which is the same
// belt-and-suspenders reasoning medication.go documents.
func (s *Service) authorizeRecord(ctx context.Context, actor access.Actor, id string) error {
	grant, err := s.authorizer.Actor(ctx, actor)
	if decideErr := s.decide(ctx, actor, id, grant, err); decideErr != nil {
		return decideErr
	}

	owner, err := s.repository.Owner(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return s.denied(ctx, actor, id)
		}

		return fmt.Errorf("practitioner: reading the owner of %s: %w", id, err)
	}

	if owner != actor.UserID {
		return s.denied(ctx, actor, id)
	}

	return nil
}

// decide is the one place a checkpoint's answer is read.
func (s *Service) decide(ctx context.Context, actor access.Actor, id string, grant access.Grant, err error) error {
	switch {
	case err == nil && grant.Allows(access.PermOwn):
		return nil
	case err == nil, errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return s.denied(ctx, actor, id)
	default:
		return fmt.Errorf("practitioner: the authorization checkpoint could not answer: %w", err)
	}
}

// denied writes the one audit row this package writes and returns the
// refusal, exactly as medication.Service.denied does.
func (s *Service) denied(ctx context.Context, actor access.Actor, id string) error {
	refusal := fmt.Errorf("practitioner: the actor may not reach that record: %w", domain.ErrNotFound)

	if err := s.auditor.Record(ctx, denial(actor, id)); err != nil {
		return errors.Join(refusal, fmt.Errorf("practitioner: the refusal was not recorded: %w", err))
	}

	return refusal
}

func denial(actor access.Actor, id string) audit.Event {
	event := audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionAccessDenied,
		TargetKind: audit.TargetKindPractitioner,
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

// resolve applies the published vocabulary: a sort outside Sorts() or a
// specialty outside directory.Specialties() is refused rather than silently
// dropped or ignored.
func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	if q.Specialty != "" && !q.Specialty.Valid() {
		invalid.Add(FilterSpecialty, domain.CodeInvalidValue, "not one of the specialties MediKube accepts")
	}

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

// applyTo returns the practitioner as it would be with the patch applied. It
// takes a copy: nothing here changes the entity the store handed back.
func (p Patch) applyTo(practitioner directory.Practitioner) directory.Practitioner {
	assign(&practitioner.Name, p.Name)
	assign(&practitioner.Specialty, p.Specialty)
	assign(&practitioner.FacilityID, p.FacilityID)
	assign(&practitioner.Phone, p.Phone)
	assign(&practitioner.Email, p.Email)
	assign(&practitioner.Website, p.Website)
	assign(&practitioner.Notes, p.Notes)

	return practitioner
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
