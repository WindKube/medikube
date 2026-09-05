package injury

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// Patch is a change to an injury: every field optional.
type Patch struct {
	Name          *string
	Type          *clinical.InjuryType
	BodyPart      *string
	Laterality    *clinical.Laterality
	OccurredOn    *domain.Date
	Mechanism     *string
	Severity      *clinical.Severity
	Status        *clinical.ConditionStatus
	RecoveryNotes *string

	Practitioner *string

	// MedicationIDs is nil when not sent (leave alone) and non-nil to
	// replace the whole set (FR-056's replace-set semantics), including an
	// empty, non-nil slice to clear it.
	MedicationIDs *[]string
}

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
		return nil, fmt.Errorf("injury: the service is wired with no %s", joinWords(missing))
	}

	return &Service{repository: repository, authorizer: authorizer}, nil
}

func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[clinical.Injury], error) {
	if err := s.authorizePatient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[clinical.Injury]{}, err
	}

	resolved, err := query.resolve()
	if err != nil {
		return domain.Page[clinical.Injury]{}, err
	}

	return s.repository.List(ctx, query.PatientID, resolved)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Injury, error) {
	found, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Injury{}, err
	}

	if err := s.authorizePatient(ctx, actor, found.PatientID, access.PermView); err != nil {
		return clinical.Injury{}, err
	}

	return found, nil
}

func (s *Service) Create(ctx context.Context, actor access.Actor, draft clinical.Injury) (clinical.Injury, error) {
	if draft.PatientID == "" {
		var invalid domain.ValidationError
		invalid.Add(FieldPatient, domain.CodeRequired, "an injury is filed against a person")

		return clinical.Injury{}, invalid.OrNil()
	}

	if err := s.authorizePatient(ctx, actor, draft.PatientID, access.PermEdit); err != nil {
		return clinical.Injury{}, err
	}

	draft.ID = ""
	draft.CreatedAt = time.Time{}
	draft.UpdatedAt = time.Time{}
	draft.Version = ""

	if draft.Status == "" {
		draft.Status = clinical.ConditionStatusActive
	}

	if err := draft.Validate(); err != nil {
		return clinical.Injury{}, err
	}

	return s.repository.Create(ctx, draft)
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (clinical.Injury, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return clinical.Injury{}, err
	}

	if err := s.authorizePatient(ctx, actor, current.PatientID, access.PermEdit); err != nil {
		return clinical.Injury{}, err
	}

	changed := patch.applyTo(current)
	if err := changed.Validate(); err != nil {
		return clinical.Injury{}, err
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
		return fmt.Errorf("injury: the authorization checkpoint granted nothing and refused nothing: %w", domain.ErrNotFound)
	}

	return nil
}

func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	for _, status := range q.Statuses {
		if !status.Valid() {
			invalid.Add(FilterStatus, domain.CodeInvalidValue, "not one of the states MediKube accepts")
			break
		}
	}

	for _, severity := range q.Severities {
		if !severity.Valid() {
			invalid.Add(FilterSeverity, domain.CodeInvalidValue, "not one of the severities MediKube accepts")
			break
		}
	}

	for _, t := range q.Types {
		if !t.Valid() {
			invalid.Add(FilterType, domain.CodeInvalidValue, "not one of the types MediKube accepts")
			break
		}
	}

	for _, l := range q.Lateralities {
		if !l.Valid() {
			invalid.Add(FilterLaterality, domain.CodeInvalidValue, "not one of the sides MediKube accepts")
			break
		}
	}

	if q.Unresolved {
		q.Statuses = append(q.Statuses, unresolvedStatuses()...)
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

func (p Patch) applyTo(injury clinical.Injury) clinical.Injury {
	assign(&injury.Name, p.Name)
	assign(&injury.Type, p.Type)
	assign(&injury.BodyPart, p.BodyPart)
	assign(&injury.Laterality, p.Laterality)
	assign(&injury.OccurredOn, p.OccurredOn)
	assign(&injury.Mechanism, p.Mechanism)
	assign(&injury.Severity, p.Severity)
	assign(&injury.Status, p.Status)
	assign(&injury.RecoveryNotes, p.RecoveryNotes)
	assign(&injury.PractitionerID, p.Practitioner)

	if p.MedicationIDs != nil {
		injury.MedicationIDs = *p.MedicationIDs
	}

	return injury
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
