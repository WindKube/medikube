package condition

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const FieldPatient = "patient"

type Service struct {
	repository Repository
	authorizer Authorizer
}

func New(repository Repository, authorizer Authorizer) (*Service, error) {
	var missing []string

	if repository == nil {
		missing = append(missing, "repository")
	}

	if authorizer == nil {
		missing = append(missing, "authorizer")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("condition: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer}, nil
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Condition], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Condition]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Condition]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Condition, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Condition{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Condition{}, err
	}

	return found, nil
}

func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Condition) (clinical.Condition, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "a condition is filed against a person")

		return clinical.Condition{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.Condition{}, err
	}

	draft.ID = ""
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return clinical.Condition{}, err
	}

	return s.repository.Create(ctx, draft)
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.Condition, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Condition{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
		return clinical.Condition{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.Condition{}, err
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
		return fmt.Errorf("condition: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
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
