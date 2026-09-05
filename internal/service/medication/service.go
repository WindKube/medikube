package medication

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
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

	// Practitioner and Pharmacy, phase 002's additions. There is deliberately
	// no Patient here: contracts/medications-rescope.md makes re-attribution
	// impossible by shape, and a field added here would put it back.
	Practitioner *string
	Pharmacy     *string
}

// FieldPatient is the field a create's missing patient is reported against.
const FieldPatient = "patient"

// Service is the medication use cases. Every method authorizes first, and the
// order below is the order every method runs in: the checkpoint, then the
// rules, then the store.
//
// It holds no clock, no logger and no transaction. Every write's audit row is
// the post-commit hooks' to write, and every refusal's audit row is the
// checkpoint's own (research D-13): unlike phase 001, this package writes none
// itself.
type Service struct {
	repository Repository
	authorizer Authorizer
}

// New refuses an incomplete service rather than returning one.
//
// A nil authorizer is a service with no authorization at all — every call would
// panic on its first request, after the process has been accepting traffic for
// however long it took somebody to reach this kind. The composition root gets
// the error instead, at boot.
func New(repository Repository, authorizer Authorizer) (*Service, error) {
	var missing []string

	if repository == nil {
		missing = append(missing, "repository")
	}

	if authorizer == nil {
		missing = append(missing, "authorizer")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("medication: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer}, nil
}

// List is the requested patient's medications, one page at a time.
//
// The patient is authorized before this touches the repository at all
// (contracts/medications-rescope.md's lists section): a list for another
// account's patient is a 404 that never runs a query.
func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Medication], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Medication]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Medication]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

// Get reads one record, authorized from the row itself: the repository is read
// first (which is not a data disclosure — nothing here answers the caller yet)
// and the patient it names is what the checkpoint decides on.
func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Medication, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Medication{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Medication{}, err
	}

	return found, nil
}

// Create stores a new medication for the patient named in the draft.
//
// The patient is required (FR-021, US2-3): draft.PatientID is authorized
// before anything is validated or stored, and an empty one is refused as a
// validation failure rather than an authorization one — there is no patient to
// resolve yet, so "which person is this for" is the honest refusal.
func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Medication) (clinical.Medication, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "a medication is filed against a person")

		return clinical.Medication{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.Medication{}, err
	}

	draft.ID = ""
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
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Medication{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
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
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermOwn); err != nil {
		return err
	}

	return s.repository.Delete(ctx, id, version)
}

// authorizePatient is the one place this package reads the checkpoint's
// answer. access.Authorizer.Patient already audits its own refusal and already
// answers domain.ErrNotFound rather than domain.ErrForbidden (FR-042), so
// there is nothing left for this package to decide except what a failure to
// answer at all means.
func (s *Service) authorizePatient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) error {
	grant, err := s.authorizer.Patient(ctx, actor, patientID, need)
	if err != nil {
		return err
	}

	if !grant.Allows(need) {
		// Unreachable while access.Authorizer.Patient answers only a grant or
		// an error, never a zero grant with a nil error — kept because a
		// future implementation that did would otherwise silently grant here.
		return fmt.Errorf("medication: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
	}

	return nil
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
	assign(&medication.PractitionerID, p.Practitioner)
	assign(&medication.PharmacyID, p.Pharmacy)

	return medication
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
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
