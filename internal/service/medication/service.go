package medication

import (
	"context"
	"errors"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// Patch is a change to a medication: every field optional, and a supplied
// field's zero value is a value the person chose rather than a field they left
// alone.
//
// Pointers and not an omnibus entity, because "leave this alone" and "clear
// this" are different instructions and an entity has no room for the
// difference. The two dates need no third state: domain.Date's zero value
// already means "not recorded", so a non-nil pointer to a zero date is the
// explicit clear that contracts/records.md's `null` asks for, and nil is
// absence.
type Patch struct {
	Name            *string
	AlternativeName *string
	Type            *clinical.MedicationType
	Dosage          *string
	Frequency       *string
	Route           *clinical.MedicationRoute
	Indication      *string
	StartedOn       *domain.Date
	EndedOn         *domain.Date
	Status          *clinical.TherapyStatus
	SideEffects     *string
	Notes           *string
}

// Service is the medication use cases. Every method authorizes first, and the
// order below is the order every method runs in: the checkpoint, then the
// rules, then the store.
//
// It holds no clock, no logger and no transaction. The audit rows for the three
// writes are the post-commit hooks' to write (see Auditor), and the one row
// this package writes is the refusal.
type Service struct {
	repository Repository
	authorizer Authorizer
	auditor    Auditor
}

// New refuses an incomplete service rather than returning one.
//
// A nil authorizer is a service with no authorization at all — every call would
// panic on its first request, after the process has been accepting traffic for
// however long it took somebody to reach this kind. The composition root gets
// the error instead, at boot.
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
		return nil, fmt.Errorf("medication: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer, auditor: auditor}, nil
}

// List is the owner's medications, one page at a time.
//
// The owner is read from the authenticated actor and from nowhere else: there
// is no parameter here a caller could name another account in, which is FR-032
// enforced by shape.
func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Medication], error) {
	if err := s.authorizeKind(ctx, actor, access.PermView); err != nil {
		return domain.Page[clinical.Medication]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Medication]{}, err
	}

	page, err := s.repository.List(ctx, actor.UserID, resolved)
	if err != nil {
		return domain.Page[clinical.Medication]{}, err
	}

	return page, nil
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Medication, error) {
	if err := s.authorizeRecord(ctx, actor, id, access.PermView); err != nil {
		return clinical.Medication{}, err
	}

	return s.repository.Get(ctx, actor.UserID, id)
}

// Create stores a new medication for the actor.
//
// The four server-owned fields are taken from the draft and thrown away rather
// than trusted: the request DTO has no member for any of them, so this can only
// be reached by a caller inside the process, and the day one of those exists is
// the day this line is the only thing between it and a record attributed to
// somebody else (FR-032).
func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Medication) (clinical.Medication, error) {
	if err := s.authorizeKind(ctx, actor, access.PermEdit); err != nil {
		return clinical.Medication{}, err
	}

	draft.ID = ""
	draft.OwnerID = actor.UserID
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	// data-model §2's `default active`, applied before validation so that the
	// entity the rules are checked against is the entity that gets stored.
	if draft.Status == "" {
		draft.Status = clinical.TherapyStatusActive
	}

	if err := draft.Validate(); err != nil {
		return clinical.Medication{}, err
	}

	return s.repository.Create(ctx, draft)
}

// Update applies the supplied fields and nothing else.
//
// The rules are checked against the medication as it would be after the patch
// and not against the patch, because "the end date is before the start date" is
// a property of neither half alone: patching only the end date has to be
// refused against the start date already stored.
func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.Medication, error) {
	if err := s.authorizeRecord(ctx, actor, id, access.PermEdit); err != nil {
		return clinical.Medication{}, err
	}

	current, err := s.repository.Get(ctx, actor.UserID, id)
	if err != nil {
		return clinical.Medication{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.Medication{}, err
	}

	return s.repository.Update(ctx, changed, version)
}

// Delete is permanent (FR-028), and it needs the level above editing.
//
// Nothing in this phase can hold PermEdit without holding PermOwn — the owner
// is the only actor there is. Phase 005's shares are what make the difference
// real, and the choice is made here rather than there because a destructive,
// unrecoverable act defaulting to the editing level is the kind of decision
// nobody revisits once a share exists.
func (s *Service) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	if err := s.authorizeRecord(ctx, actor, id, access.PermOwn); err != nil {
		return err
	}

	return s.repository.Delete(ctx, actor.UserID, id, version)
}

func (s *Service) authorizeRecord(ctx context.Context, actor access.Actor, id string, need access.Permission) error {
	grant, err := s.authorizer.Record(ctx, actor, kind.Medication, id, need)

	return s.decide(ctx, actor, id, grant, need, err)
}

func (s *Service) authorizeKind(ctx context.Context, actor access.Actor, need access.Permission) error {
	grant, err := s.authorizer.Kind(ctx, actor, kind.Medication, need)

	return s.decide(ctx, actor, "", grant, need, err)
}

// decide is the one place a checkpoint's answer is read, so that every method
// reads it the same way.
//
// The grant is checked even when the error is nil. An implementation that
// returned a zero Grant and no error would otherwise be granting: the level is
// the answer and the error is only how a refusal travels.
func (s *Service) decide(
	ctx context.Context,
	actor access.Actor,
	id string,
	grant access.Grant,
	need access.Permission,
	err error,
) error {
	switch {
	case err == nil && grant.Allows(need):
		return nil
	case err == nil, errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
		return s.denied(ctx, actor, id)
	default:
		// A checkpoint that could not answer has refused nobody. Answering a
		// miss here would tell a caller the record is not theirs on the
		// strength of a database being down, and would fill the trail with
		// denials nobody attempted.
		return fmt.Errorf("medication: the authorization checkpoint could not answer: %w", err)
	}
}

// denied writes the one audit row this package writes and returns the refusal.
//
// The refusal is domain.ErrNotFound whatever the checkpoint said, because a
// record that is somebody else's is answered exactly as one that does not exist
// (FR-033) — and the service is where that becomes true, not the edge.
func (s *Service) denied(ctx context.Context, actor access.Actor, id string) error {
	refusal := fmt.Errorf("medication: the actor may not reach that record: %w", domain.ErrNotFound)

	if err := s.auditor.Record(ctx, denial(actor, id)); err != nil {
		// Joined, not swallowed and not returned alone. The caller still gets
		// the miss — a 500 where every other refusal is a 404 would be an
		// oracle for "that one exists" — and the unwritten row still reaches
		// the log and Sentry through the same error.
		return errors.Join(refusal, fmt.Errorf("medication: the refusal was not recorded: %w", err))
	}

	return refusal
}

// denial builds the row data-model §3 specifies. An anonymous caller is the
// second row of that table: a system actor, a system target and no id, because
// there is no account to name and the id somebody guessed is not one.
func denial(actor access.Actor, id string) audit.Event {
	event := audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionAccessDenied,
		TargetKind: audit.TargetKindMedication,
		// The id as addressed, bounded: it is a caller's string, and the
		// column that holds it is 64 characters. A row refused for being too
		// long is a refusal nobody recorded.
		TargetID:  truncate(id, audit.MaxTargetID),
		RequestID: actor.RequestID,
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

// resolve applies the published vocabulary. Both refusals are contract: a sort
// outside the allowlist is invalid_value and never silently ignored, and a
// state outside the vocabulary is refused rather than dropped — a dropped term
// narrows to everything and looks like a list that is simply long.
func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	for _, status := range q.Statuses {
		if !status.Valid() {
			invalid.Add(FilterStatus, domain.CodeInvalidValue, "not one of the states MediKube accepts")

			break
		}
	}

	published := Sorts()

	if len(q.Sort) == 0 {
		q.Sort = published[:1]
	}

	for _, term := range q.Sort {
		if !containsSort(published, term) {
			invalid.Add(ParamSort, domain.CodeInvalidValue, "not one of the orderings MediKube publishes")

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

// applyTo returns the medication as it would be with the patch applied. It
// takes a copy: nothing here changes the entity the store handed back, so a
// refused update leaves the caller's copy as it read it.
func (p Patch) applyTo(medication clinical.Medication) clinical.Medication {
	assign(&medication.Name, p.Name)
	assign(&medication.AlternativeName, p.AlternativeName)
	assign(&medication.Type, p.Type)
	assign(&medication.Dosage, p.Dosage)
	assign(&medication.Frequency, p.Frequency)
	assign(&medication.Route, p.Route)
	assign(&medication.Indication, p.Indication)
	assign(&medication.StartedOn, p.StartedOn)
	assign(&medication.EndedOn, p.EndedOn)
	assign(&medication.Status, p.Status)
	assign(&medication.SideEffects, p.SideEffects)
	assign(&medication.Notes, p.Notes)

	return medication
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
