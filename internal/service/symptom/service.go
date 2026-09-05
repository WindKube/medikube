package symptom

import (
	"context"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/link"
)

// FieldPatient is the field a create's missing patient is reported against.
const FieldPatient = "patient"

// Service is the symptom-episode use cases. Every method authorizes first.
type Service struct {
	repository Repository
	authorizer Authorizer

	linkResolver   link.Resolver
	linkAuthorizer link.Authorizer
}

// Option configures a dependency only some callers need. See allergy's own
// WithLinks for the reasoning: FR-057 validation for the two medication-role
// fields.
type Option func(*Service)

func WithLinks(resolver link.Resolver, authorizer link.Authorizer) Option {
	return func(s *Service) {
		s.linkResolver = resolver
		s.linkAuthorizer = authorizer
	}
}

func New(repository Repository, authorizer Authorizer, options ...Option) (*Service, error) {
	var missing []string

	if repository == nil {
		missing = append(missing, "repository")
	}

	if authorizer == nil {
		missing = append(missing, "authorizer")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("symptom: the service is wired with no %s", joinWords(missing))
	}

	service := &Service{repository: repository, authorizer: authorizer}

	for _, option := range options {
		option(service)
	}

	return service, nil
}

func (s *Service) validateMedications(ctx context.Context, actor access.Actor, patientID string, ids []string) ([]string, error) {
	if s.linkResolver == nil {
		return ids, nil
	}

	return link.ValidateSet(ctx, s.linkResolver, s.linkAuthorizer, actor, patientID, kind.Medication, ids)
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Symptom], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Symptom]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Symptom]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Symptom, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Symptom{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Symptom{}, err
	}

	return found, nil
}

// Create stores a new episode. Recording the same name again is always a new
// row (FR-030): this method never looks for an existing one.
func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Symptom) (clinical.Symptom, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "an episode is filed against a person")

		return clinical.Symptom{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.Symptom{}, err
	}

	draft.ID = ""
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return clinical.Symptom{}, err
	}

	if len(draft.TreatedByMedicationIDs) > 0 {
		validated, err := s.validateMedications(ctx, actor, draft.PatientID, draft.TreatedByMedicationIDs)
		if err != nil {
			return clinical.Symptom{}, err
		}

		draft.TreatedByMedicationIDs = validated
	}

	if len(draft.CausedByMedicationIDs) > 0 {
		validated, err := s.validateMedications(ctx, actor, draft.PatientID, draft.CausedByMedicationIDs)
		if err != nil {
			return clinical.Symptom{}, err
		}

		draft.CausedByMedicationIDs = validated
	}

	return s.repository.Create(ctx, draft)
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.Symptom, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Symptom{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
		return clinical.Symptom{}, err
	}

	if patch.TreatedByMedicationIDs != nil {
		validated, err := s.validateMedications(ctx, actor, current.PatientID, *patch.TreatedByMedicationIDs)
		if err != nil {
			return clinical.Symptom{}, err
		}

		patch.TreatedByMedicationIDs = &validated
	}

	if patch.CausedByMedicationIDs != nil {
		validated, err := s.validateMedications(ctx, actor, current.PatientID, *patch.CausedByMedicationIDs)
		if err != nil {
			return clinical.Symptom{}, err
		}

		patch.CausedByMedicationIDs = &validated
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.Symptom{}, err
	}

	return s.repository.Update(ctx, changed, version)
}

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

func (s *Service) authorizePatient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) error {
	grant, err := s.authorizer.Patient(ctx, actor, patientID, need)
	if err != nil {
		return err
	}

	if !grant.Allows(need) {
		return fmt.Errorf("symptom: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
	}

	return nil
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
