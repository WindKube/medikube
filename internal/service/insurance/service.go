package insurance

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// Patch is a change to a policy: every field optional.
type Patch struct {
	Type          *clinical.InsuranceType
	Company       *string
	PlanName      *string
	EmployerGroup *string
	MemberName    *string
	MemberID      *string
	GroupNumber   *string
	HolderName    *string
	Relationship  *clinical.HolderRelationship

	EffectiveOn *domain.Date
	ExpiresOn   *domain.Date
	Status      *clinical.InsuranceStatus
	IsPrimary   *bool

	Coverage *clinical.Coverage
	Contact  *clinical.Contact

	Notes *string
}

// FieldPatient is the field a create's missing patient is reported against.
const FieldPatient = "patient"

// Service is the insurance use cases.
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
		return nil, fmt.Errorf("insurance: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer}, nil
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Insurance], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Insurance]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Insurance]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Insurance, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Insurance{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Insurance{}, err
	}

	return found, nil
}

// Create stores a new policy. When the draft is primary, the repository
// displaces the previous primary policy in the same transaction as the write
// and Result.Displaced names it (FR-045).
func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Insurance) (Result, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "an insurance policy is filed against a person")

		return Result{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return Result{}, err
	}

	draft.ID = ""
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return Result{}, err
	}

	created, displaced, err := s.repository.Create(ctx, draft)
	if err != nil {
		return Result{}, err
	}

	return Result{Insurance: created, Displaced: displaced}, nil
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (Result, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}

	err = s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit)
	if err != nil {
		return Result{}, err
	}

	changed := patch.applyTo(current)

	err = changed.Validate()
	if err != nil {
		return Result{}, err
	}

	updated, displaced, err := s.repository.Update(ctx, changed, version)
	if err != nil {
		return Result{}, err
	}

	return Result{Insurance: updated, Displaced: displaced}, nil
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

// Result is a write's outcome: the policy as stored, and the policy it
// displaced from primary, if any.
type Result struct {
	Insurance clinical.Insurance
	Displaced *Displaced
}

func (s *Service) authorizePatient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) error {
	grant, err := s.authorizer.Patient(ctx, actor, patientID, need)
	if err != nil {
		return err
	}

	if !grant.Allows(need) {
		return fmt.Errorf("insurance: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
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

func (p Patch) applyTo(entity clinical.Insurance) clinical.Insurance {
	assign(&entity.Type, p.Type)
	assign(&entity.Company, p.Company)
	assign(&entity.PlanName, p.PlanName)
	assign(&entity.EmployerGroup, p.EmployerGroup)
	assign(&entity.MemberName, p.MemberName)
	assign(&entity.MemberID, p.MemberID)
	assign(&entity.GroupNumber, p.GroupNumber)
	assign(&entity.HolderName, p.HolderName)
	assign(&entity.Relationship, p.Relationship)
	assign(&entity.EffectiveOn, p.EffectiveOn)
	assign(&entity.ExpiresOn, p.ExpiresOn)
	assign(&entity.Status, p.Status)
	assign(&entity.IsPrimary, p.IsPrimary)
	assign(&entity.Coverage, p.Coverage)
	assign(&entity.Contact, p.Contact)
	assign(&entity.Notes, p.Notes)

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
