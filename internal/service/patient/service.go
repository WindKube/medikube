package patient

import (
	"context"
	"errors"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/person"
)

// ErrSelfRecordProtected is Delete's refusal of a self-record (FR-051,
// US6-4). It is a package sentinel rather than a domain one: internal/web
// translates it to its own 409 (web.ErrSelfRecordProtected) with the
// account-closure explanation, the same way it translates a stale If-Match
// into a version-mismatch response, rather than through the generic
// domain-sentinel mapper.
var ErrSelfRecordProtected = errors.New("patient: a self-record cannot be deleted; closing the account is what removes it")

// Patch is a change to a patient: every field optional, and a supplied
// field's zero value is a value the person chose rather than a field they
// left alone. Mirrors internal/service/medication.Patch.
type Patch struct {
	FirstName           *string
	LastName            *string
	BirthDate           *domain.Date
	Sex                 *person.Sex
	BloodType           *person.BloodType
	HeightCM            **float64
	WeightKG            **float64
	Address             *string
	Relationship        *person.RelationshipToOwner
	PrimaryPractitioner **string
}

// Service is the patient use cases. Every method that names a record
// authorizes first, against Authorizer.Patient — which is what makes FR-042
// structural: a stranger's request never reaches the repository at all.
//
// List and Create authorize only the session: contracts/patients.md answers
// 200/201 for any signed-in account and 401 for none, and the owner comes from
// the actor, never from a request (FR-002).
type Service struct {
	repository Repository
	photos     PhotoStore
	authorizer Authorizer
	pointer    ActivePatientStore
	auditor    Auditor
	counter    RecordCounter
	activity   RecentActivityReader
}

// New refuses an incomplete service rather than returning one (mirrors
// internal/service/medication.New).
func New(
	repository Repository,
	photos PhotoStore,
	authorizer Authorizer,
	pointer ActivePatientStore,
	auditor Auditor,
	counter RecordCounter,
	activity RecentActivityReader,
) (*Service, error) {
	var missing []string

	if repository == nil {
		missing = append(missing, "repository")
	}

	if photos == nil {
		missing = append(missing, "photo store")
	}

	if authorizer == nil {
		missing = append(missing, "authorizer")
	}

	if pointer == nil {
		missing = append(missing, "active-patient pointer")
	}

	if auditor == nil {
		missing = append(missing, "auditor")
	}

	if counter == nil {
		missing = append(missing, "record counter")
	}

	if activity == nil {
		missing = append(missing, "recent activity reader")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("patient: the service is wired with no %s", joinWords(missing))
	}

	return &Service{
		repository: repository,
		photos:     photos,
		authorizer: authorizer,
		pointer:    pointer,
		auditor:    auditor,
		counter:    counter,
		activity:   activity,
	}, nil
}

// List is the actor's own patients, one page at a time (FR-010).
func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[person.Patient], error) {
	if !actor.Authenticated() {
		return domain.Page[person.Patient]{}, domain.ErrUnauthenticated
	}

	return s.repository.List(ctx, actor.UserID, query)
}

// Get answers one patient, authorized against the person rather than a kind
// (research D-05).
func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (person.Patient, error) {
	if _, err := s.authorizer.Patient(ctx, actor, id, access.PermView); err != nil {
		return person.Patient{}, err
	}

	return s.repository.Get(ctx, actor.UserID, id)
}

// Create stores a new patient for the actor.
//
// owner and is_self_record are never read from draft's caller-facing
// counterparts: the DTO layer has no member for either (FR-002, FR-004), and
// this is where that absence becomes the stored fact regardless of what an
// internal caller passed in.
func (s *Service) Create(ctx context.Context, actor access.Actor, draft person.Patient) (person.Patient, error) {
	if !actor.Authenticated() {
		return person.Patient{}, domain.ErrUnauthenticated
	}

	draft.ID = ""
	draft.OwnerID = actor.UserID
	draft.IsSelfRecord = false
	draft.Version = ""

	if err := draft.Validate(); err != nil {
		return person.Patient{}, err
	}

	return s.repository.Create(ctx, draft)
}

// Update applies the supplied fields and nothing else, over the patient as it
// would be after the patch — never over the patch alone, so a rule that spans
// two fields is checked against the record the write would leave behind.
func (s *Service) Update(ctx context.Context, actor access.Actor, id, version string, patch Patch) (person.Patient, error) {
	if _, err := s.authorizer.Patient(ctx, actor, id, access.PermOwn); err != nil {
		return person.Patient{}, err
	}

	current, err := s.repository.Get(ctx, actor.UserID, id)
	if err != nil {
		return person.Patient{}, err
	}

	changed := patch.applyTo(current)

	if err := changed.Validate(); err != nil {
		return person.Patient{}, err
	}

	return s.repository.Update(ctx, changed, version)
}

// Delete is permanent (FR-049, US6-2, US6-3): the cascade over the patient's
// medications, its photo and its thumbnails, and the auto-unset of every
// pointer at the row, are the repository's job, relying on PocketBase's own
// cascade (research D-06).
//
// A self-record is refused with domain.ErrSelfRecordProtected (FR-051,
// US6-4): closing the account is what removes it, and this is checked ahead
// of the version comparison so the refusal is about what the record is, not
// about who is asking or when they last read it.
func (s *Service) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	if _, err := s.authorizer.Patient(ctx, actor, id, access.PermOwn); err != nil {
		return err
	}

	current, err := s.repository.Get(ctx, actor.UserID, id)
	if err != nil {
		return err
	}

	if current.IsSelfRecord {
		return ErrSelfRecordProtected
	}

	return s.repository.Delete(ctx, actor.UserID, id, version)
}

func (p Patch) applyTo(patient person.Patient) person.Patient {
	assign(&patient.FirstName, p.FirstName)
	assign(&patient.LastName, p.LastName)
	assign(&patient.BirthDate, p.BirthDate)
	assign(&patient.Sex, p.Sex)
	assign(&patient.BloodType, p.BloodType)
	assign(&patient.Address, p.Address)
	assign(&patient.RelationshipToOwner, p.Relationship)

	if p.HeightCM != nil {
		patient.HeightCM = derefOrZero(*p.HeightCM)
	}

	if p.WeightKG != nil {
		patient.WeightKG = derefOrZero(*p.WeightKG)
	}

	if p.PrimaryPractitioner != nil {
		patient.PrimaryPractitionerID = derefOrZeroString(*p.PrimaryPractitioner)
	}

	return patient
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}

func derefOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}

	return *value
}

func derefOrZeroString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
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
