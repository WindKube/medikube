package immunization

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// Patch is a change to an immunization: every field optional.
type Patch struct {
	VaccineName    *string
	TradeName      *string
	AdministeredOn *domain.Date
	// DoseNumber carries the three PATCH states through **int: absent (nil),
	// clear (non-nil pointing at nil) and a value (non-nil pointing at a
	// pointer to it).
	DoseNumber   **int
	LotNumber    *string
	Manufacturer *string
	Site         *clinical.ImmunizationSite
	Route        *clinical.ImmunizationRoute
	ExpiresOn    *domain.Date

	Practitioner *string
	Facility     *string
}

// FieldPatient is the field a create's missing patient is reported against.
const FieldPatient = "patient"

// Service is the immunization use cases, following medication.Service's shape.
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
		return nil, fmt.Errorf("immunization: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer}, nil
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Immunization], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Immunization]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Immunization]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Immunization, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Immunization{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Immunization{}, err
	}

	return found, nil
}

func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Immunization) (clinical.Immunization, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "a vaccination is filed against a person")

		return clinical.Immunization{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.Immunization{}, err
	}

	draft.ID = ""
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return clinical.Immunization{}, err
	}

	return s.repository.Create(ctx, draft)
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.Immunization, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Immunization{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
		return clinical.Immunization{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.Immunization{}, err
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
		return fmt.Errorf("immunization: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
	}

	return nil
}

func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

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

func (p Patch) applyTo(immunization clinical.Immunization) clinical.Immunization {
	assign(&immunization.VaccineName, p.VaccineName)
	assign(&immunization.TradeName, p.TradeName)
	assign(&immunization.AdministeredOn, p.AdministeredOn)

	if p.DoseNumber != nil {
		immunization.DoseNumber = *p.DoseNumber
	}

	assign(&immunization.LotNumber, p.LotNumber)
	assign(&immunization.Manufacturer, p.Manufacturer)
	assign(&immunization.Site, p.Site)
	assign(&immunization.Route, p.Route)
	assign(&immunization.ExpiresOn, p.ExpiresOn)
	assign(&immunization.PractitionerID, p.Practitioner)
	assign(&immunization.FacilityID, p.Facility)

	return immunization
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
