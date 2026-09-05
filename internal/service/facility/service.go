package facility

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

// Patch is a change to a facility: every field optional, and a supplied
// field's zero value is a value the person chose rather than a field they left
// alone.
type Patch struct {
	Kind         *directory.FacilityKind
	Name         *string
	Brand        *string
	Street       *string
	City         *string
	Region       *string
	PostalCode   *string
	Country      *string
	Phone        *string
	Fax          *string
	Email        *string
	Website      *string
	PortalURL    *string
	Hours        *string
	Open24h      *bool
	DriveThrough *bool
	Services     *string
	Notes        *string
}

// Service is the facility use cases. Every method authorizes first, and the
// order below is the order every method runs in: the checkpoint, then the
// rules, then the store.
type Service struct {
	repository Repository
	authorizer Authorizer
	auditor    Auditor
	metrics    Metrics
}

// Metrics is FR-055's observability seam, mirroring
// internal/service/patient's own: counts in bounded vocabulary, no import of
// how they are exported. *obs.Metrics satisfies this with no import here.
type Metrics interface {
	RecordCreated(kind string)
}

type noopMetrics struct{}

func (noopMetrics) RecordCreated(string) {}

// SetMetrics wires the counter, optionally: a service nobody calls this on
// observes nothing rather than panicking on a nil interface.
func (s *Service) SetMetrics(metrics Metrics) {
	if metrics == nil {
		metrics = noopMetrics{}
	}

	s.metrics = metrics
}

func (s *Service) metricsOrNoop() Metrics {
	if s.metrics == nil {
		return noopMetrics{}
	}

	return s.metrics
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
		return nil, fmt.Errorf("facility: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer, auditor: auditor}, nil
}

// List is the owner's directory, one page at a time. The owner is read from
// the authenticated actor and from nowhere else, which is FR-037 enforced by
// shape.
func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[directory.Facility], error) {
	if err := s.authorize(ctx, actor, access.PermView); err != nil {
		return domain.Page[directory.Facility]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[directory.Facility]{}, err
	}

	return s.repository.List(ctx, actor.UserID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (directory.Facility, error) {
	if err := s.authorizeRecord(ctx, actor, id, access.PermView); err != nil {
		return directory.Facility{}, err
	}

	return s.repository.Get(ctx, actor.UserID, id)
}

// Create stores a new facility for the actor.
//
// The four server-owned fields are taken from the draft and thrown away
// rather than trusted, mirroring medication.Service.Create: the request DTO
// has no member for any of them, and this line is what keeps a record
// attributed to somebody else out of reach (FR-037).
func (s *Service) Create(ctx context.Context, actor access.Actor, draft directory.Facility) (directory.Facility, error) {
	if err := s.authorize(ctx, actor, access.PermEdit); err != nil {
		return directory.Facility{}, err
	}

	draft.ID = ""
	draft.OwnerID = actor.UserID
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return directory.Facility{}, err
	}

	created, err := s.repository.Create(ctx, draft)
	if err == nil {
		s.metricsOrNoop().RecordCreated("facility")
	}

	return created, err
}

// Update applies the supplied fields and nothing else, and checks the rules
// against the facility as it would be after the patch — the same reasoning as
// medication.Service.Update.
func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (directory.Facility, error) {
	if err := s.authorizeRecord(ctx, actor, id, access.PermEdit); err != nil {
		return directory.Facility{}, err
	}

	current, err := s.repository.Get(ctx, actor.UserID, id)
	if err != nil {
		return directory.Facility{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return directory.Facility{}, err
	}

	return s.repository.Update(ctx, changed, version)
}

// Delete is permanent. It needs the owning level, the same as
// medication.Service.Delete: nothing in this phase can hold edit without
// holding own.
func (s *Service) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	if err := s.authorizeRecord(ctx, actor, id, access.PermOwn); err != nil {
		return err
	}

	return s.repository.Delete(ctx, actor.UserID, id, version)
}

// Usage answers how much of the directory a facility is load-bearing for, so
// a caller can warn before a delete that leaves practitioners and medications
// with their reference cleared.
func (s *Service) Usage(ctx context.Context, actor access.Actor, id string) (Usage, error) {
	if err := s.authorizeRecord(ctx, actor, id, access.PermView); err != nil {
		return Usage{}, err
	}

	return s.repository.Usage(ctx, actor.UserID, id)
}

// authorize is the checkpoint for an operation with no record to name: a list
// or a create.
func (s *Service) authorize(ctx context.Context, actor access.Actor, need access.Permission) error {
	grant, err := s.authorizer.Actor(ctx, actor)

	switch {
	case err == nil && grant.Allows(need):
		return nil
	case err == nil, errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return s.denied(ctx, actor, "")
	default:
		return fmt.Errorf("facility: the authorization checkpoint could not answer: %w", err)
	}
}

// authorizeRecord is the checkpoint for one addressed facility: the actor
// first, and then — only once the actor is real — whether the id is theirs.
//
// The two questions are answered by two different things on purpose. The
// Authorizer never sees an id (there is nothing kind-specific or
// patient-anchored to resolve for FR-037), so Repository.Owner is what tells a
// row that does not exist apart from one that exists and belongs to somebody
// else: the first is never audited and the second always is, both being
// domain.ErrNotFound to the caller either way.
func (s *Service) authorizeRecord(ctx context.Context, actor access.Actor, id string, need access.Permission) error {
	if err := s.authorize(ctx, actor, need); err != nil {
		return err
	}

	owner, err := s.repository.Owner(ctx, id)

	switch {
	case err == nil && owner == actor.UserID:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return err
	case err == nil:
		return s.denied(ctx, actor, id)
	default:
		return fmt.Errorf("facility: could not resolve ownership: %w", err)
	}
}

// denied writes the one audit row this package writes and returns the
// refusal, exactly as medication.Service.denied does.
func (s *Service) denied(ctx context.Context, actor access.Actor, id string) error {
	refusal := fmt.Errorf("facility: the actor may not reach that record: %w", domain.ErrNotFound)

	if err := s.auditor.Record(ctx, denial(actor, id)); err != nil {
		return errors.Join(refusal, fmt.Errorf("facility: the refusal was not recorded: %w", err))
	}

	return refusal
}

func denial(actor access.Actor, id string) audit.Event {
	event := audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionAccessDenied,
		TargetKind: audit.TargetKindFacility,
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

// resolve applies the published vocabulary: a kind outside it is refused
// rather than silently dropped.
func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	if q.Kind != "" && !q.Kind.Valid() {
		invalid.Add(FilterKind, domain.CodeInvalidValue, "not one of the kinds MediKube accepts")
	}

	if err := invalid.OrNil(); err != nil {
		return Query{}, err
	}

	return q, nil
}

// applyTo returns the facility as it would be with the patch applied. It
// takes a copy: nothing here changes the entity the store handed back, so a
// refused update leaves the caller's copy as it read it.
func (p Patch) applyTo(facility directory.Facility) directory.Facility {
	assign(&facility.Kind, p.Kind)
	assign(&facility.Name, p.Name)
	assign(&facility.Brand, p.Brand)
	assign(&facility.Street, p.Street)
	assign(&facility.City, p.City)
	assign(&facility.Region, p.Region)
	assign(&facility.PostalCode, p.PostalCode)
	assign(&facility.Country, p.Country)
	assign(&facility.Phone, p.Phone)
	assign(&facility.Fax, p.Fax)
	assign(&facility.Email, p.Email)
	assign(&facility.Website, p.Website)
	assign(&facility.PortalURL, p.PortalURL)
	assign(&facility.Hours, p.Hours)
	assign(&facility.Open24h, p.Open24h)
	assign(&facility.DriveThrough, p.DriveThrough)
	assign(&facility.Services, p.Services)
	assign(&facility.Notes, p.Notes)

	return facility
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
