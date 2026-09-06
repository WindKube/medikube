package familymember

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/person"
)

// Patch is a change to a relative: every field optional.
type Patch struct {
	Name         *string
	Relationship *clinical.FamilyRelationship
	Sex          *person.Sex
	BirthYear    **int
	DeathYear    **int
	IsDeceased   *bool
	Conditions   *[]clinical.FamilyCondition

	// Tags is data-model §0.8's universal field, replace-set (FR-064,
	// FR-065): nil leaves the applied tags alone, non-nil (including empty)
	// replaces the whole set.
	Tags *[]string
}

// FieldPatient is the field a create's missing patient is reported against.
const FieldPatient = "patient"

// Service is the family-history use cases. Every method authorizes first.
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
		return nil, fmt.Errorf("familymember: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer}, nil
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.FamilyMember], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.FamilyMember]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.FamilyMember]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.FamilyMember, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.FamilyMember{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.FamilyMember{}, err
	}

	return found, nil
}

func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.FamilyMember) (clinical.FamilyMember, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "a relative is filed against the person whose family it is")

		return clinical.FamilyMember{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.FamilyMember{}, err
	}

	draft.ID = ""
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return clinical.FamilyMember{}, err
	}

	return s.repository.Create(ctx, draft)
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.FamilyMember, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.FamilyMember{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
		return clinical.FamilyMember{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.FamilyMember{}, err
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
		return fmt.Errorf("familymember: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
	}

	return nil
}

func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	for _, relationship := range q.Relationships {
		if !relationship.Valid() {
			invalid.Add(FilterRelationship, domain.CodeInvalidValue, "not one of the relationships MediKube accepts")

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

func (p Patch) applyTo(entity clinical.FamilyMember) clinical.FamilyMember {
	assign(&entity.Name, p.Name)
	assign(&entity.Relationship, p.Relationship)
	assign(&entity.Sex, p.Sex)
	assign(&entity.BirthYear, p.BirthYear)
	assign(&entity.DeathYear, p.DeathYear)
	assign(&entity.IsDeceased, p.IsDeceased)
	assign(&entity.Conditions, p.Conditions)

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
