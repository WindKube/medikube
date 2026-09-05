package equipment

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// Patch is a change to a piece of equipment: every field optional.
type Patch struct {
	Name         *string
	Type         *clinical.EquipmentType
	Manufacturer *string
	Model        *string
	Serial       *string
	PrescribedOn *domain.Date
	ServicedOn   *domain.Date
	ServiceDueOn *domain.Date
	Instructions *string
	Status       *clinical.TherapyStatus
	Notes        *string

	Supplier     *string
	Practitioner *string

	// Tags is data-model §0.8's universal field, replace-set (FR-064,
	// FR-065): nil leaves the applied tags alone, non-nil (including empty)
	// replaces the whole set.
	Tags *[]string
}

// FieldPatient is the field a create's missing patient is reported against.
const FieldPatient = "patient"

// Service is the equipment use cases.
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
		return nil, fmt.Errorf("equipment: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer}, nil
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Equipment], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Equipment]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Equipment]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Equipment, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Equipment{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Equipment{}, err
	}

	return found, nil
}

func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Equipment) (clinical.Equipment, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "equipment is filed against a person")

		return clinical.Equipment{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.Equipment{}, err
	}

	draft.ID = ""
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return clinical.Equipment{}, err
	}

	return s.repository.Create(ctx, draft)
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.Equipment, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Equipment{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
		return clinical.Equipment{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.Equipment{}, err
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
		return fmt.Errorf("equipment: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
	}

	return nil
}

func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	for _, t := range q.Types {
		if !t.Valid() {
			invalid.Add(FilterType, domain.CodeInvalidValue, "not one of the kinds MediKube accepts")

			break
		}
	}

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

func (p Patch) applyTo(entity clinical.Equipment) clinical.Equipment {
	assign(&entity.Name, p.Name)
	assign(&entity.Type, p.Type)
	assign(&entity.Manufacturer, p.Manufacturer)
	assign(&entity.Model, p.Model)
	assign(&entity.Serial, p.Serial)
	assign(&entity.PrescribedOn, p.PrescribedOn)
	assign(&entity.ServicedOn, p.ServicedOn)
	assign(&entity.ServiceDueOn, p.ServiceDueOn)
	assign(&entity.Instructions, p.Instructions)
	assign(&entity.Status, p.Status)
	assign(&entity.Notes, p.Notes)
	assign(&entity.SupplierID, p.Supplier)
	assign(&entity.PractitionerID, p.Practitioner)

	if p.Tags != nil {
		entity.Tags = *p.Tags
	}

	return entity
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
