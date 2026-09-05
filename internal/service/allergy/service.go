package allergy

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/link"
)

// FieldPatient is the field a create's missing patient is reported against.
const FieldPatient = "patient"

// Service is the allergy use cases. Every method authorizes first.
type Service struct {
	repository Repository
	authorizer Authorizer

	linkResolver   link.Resolver
	linkAuthorizer link.Authorizer
}

// Option configures a dependency only some callers need. WithLinks is the
// FR-057 validation for the `medications` field; a service built without it
// writes MedicationIDs unvalidated, which is only acceptable where a test
// does not exercise that field.
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
		return nil, fmt.Errorf("allergy: the service is wired with no %s", joinWords(missing))
	}

	service := &Service{repository: repository, authorizer: authorizer}

	for _, option := range options {
		option(service)
	}

	return service, nil
}

// validateMedications is FR-057 applied to the one multi-relation field this
// kind carries: every id must belong to the same patient and be editable by
// the actor. It is a no-op when the service was built with no link resolver
// (WithLinks not supplied) or the field was not part of this write.
func (s *Service) validateMedications(ctx context.Context, actor access.Actor, patientID string, ids []string) ([]string, error) {
	if s.linkResolver == nil {
		return ids, nil
	}

	return link.ValidateSet(ctx, s.linkResolver, s.linkAuthorizer, actor, patientID, kind.Medication, ids)
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Allergy], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Allergy]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Allergy]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Allergy, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Allergy{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Allergy{}, err
	}

	return found, nil
}

func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Allergy) (clinical.Allergy, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "an allergy is filed against a person")

		return clinical.Allergy{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.Allergy{}, err
	}

	draft.ID = ""
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if draft.Status == "" {
		draft.Status = clinical.ConditionStatusActive
	}

	if err := draft.Validate(); err != nil {
		return clinical.Allergy{}, err
	}

	if len(draft.MedicationIDs) > 0 {
		validated, err := s.validateMedications(ctx, actor, draft.PatientID, draft.MedicationIDs)
		if err != nil {
			return clinical.Allergy{}, err
		}

		draft.MedicationIDs = validated
	}

	return s.repository.Create(ctx, draft)
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.Allergy, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Allergy{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
		return clinical.Allergy{}, err
	}

	if patch.MedicationIDs != nil {
		validated, err := s.validateMedications(ctx, actor, current.PatientID, *patch.MedicationIDs)
		if err != nil {
			return clinical.Allergy{}, err
		}

		patch.MedicationIDs = &validated
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.Allergy{}, err
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
		return fmt.Errorf("allergy: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
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
