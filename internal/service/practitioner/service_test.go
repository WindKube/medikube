package practitioner_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/directory"
	"medikube/internal/service/practitioner"
	"medikube/internal/service/practitioner/practitionertest"
)

const requestID = "req-practitioner-service"

func actor() access.Actor {
	return access.Actor{UserID: practitionertest.OwnerID, RequestID: requestID}
}

type harness struct {
	service    *practitioner.Service
	repository *practitionertest.Repository
	authorizer *practitionertest.Authorizer
	auditor    *practitionertest.Auditor
}

func newHarness(t *testing.T) harness {
	t.Helper()

	repository := practitionertest.NewRepository()
	authorizer := practitionertest.NewAuthorizer(practitionertest.OwnerID)
	auditor := practitionertest.NewAuditor()

	service, err := practitioner.New(repository, authorizer, auditor)
	require.NoError(t, err)

	return harness{service: service, repository: repository, authorizer: authorizer, auditor: auditor}
}

func (h harness) store(t *testing.T, name string) directory.Practitioner {
	t.Helper()

	stored, err := h.repository.Create(t.Context(), directory.Practitioner{
		OwnerID: practitionertest.OwnerID,
		Name:    name,
	})
	require.NoError(t, err)

	h.repository.Forget()

	return stored
}

// TestEveryMethodAuthorizesBeforeItReachesTheStore walks the method set with
// reflection, mirroring medication's own T137: the checkpoint is consulted and
// a refusal stops the call before the store is touched, for every method that
// exists today and any added later.
func TestEveryMethodAuthorizesBeforeItReachesTheStore(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(&practitioner.Service{})

	require.GreaterOrEqual(t, serviceType.NumMethod(), 5,
		"the walk found almost no methods; it is not looking at the service it thinks it is")

	for i := range serviceType.NumMethod() {
		method := serviceType.Method(i)

		t.Run(method.Name, func(t *testing.T) {
			t.Parallel()

			refused := newHarness(t)
			refused.authorizer.Refuse(domain.ErrNotFound)

			results := call(t, refused.service, method)

			assert.Positive(t, refused.authorizer.Calls(),
				"%s never consulted the authorization checkpoint", method.Name)
			assert.Empty(t, refused.repository.Writes(),
				"%s wrote to the store after the checkpoint refused", method.Name)
			assert.ErrorIs(t, errorOf(t, results), domain.ErrNotFound,
				"%s answered a refusal as something other than a miss", method.Name)

			denials := refused.auditor.Events()
			require.Len(t, denials, 1, "%s wrote %d audit rows for one refusal", method.Name, len(denials))
			assert.Equal(t, audit.ActionAccessDenied, denials[0].Action)
		})
	}
}

func call(t *testing.T, service *practitioner.Service, method reflect.Method) []reflect.Value {
	t.Helper()

	args := make([]reflect.Value, 0, method.Type.NumIn())
	args = append(args, reflect.ValueOf(service))

	for i := 1; i < method.Type.NumIn(); i++ {
		switch in := method.Type.In(i); {
		case in == reflect.TypeOf((*context.Context)(nil)).Elem():
			args = append(args, reflect.ValueOf(t.Context()))
		case in == reflect.TypeOf(access.Actor{}):
			args = append(args, reflect.ValueOf(actor()))
		default:
			args = append(args, reflect.New(in).Elem())
		}
	}

	return method.Func.Call(args)
}

func errorOf(t *testing.T, results []reflect.Value) error {
	t.Helper()

	require.NotEmpty(t, results, "the method returned nothing at all")

	last := results[len(results)-1]
	require.True(t, last.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()),
		"the method's last result is not an error")

	if last.IsNil() {
		return nil
	}

	err, ok := last.Interface().(error)
	require.True(t, ok)

	return err
}

func TestARefusalWritesOneAccessDeniedRowNamingTheIdentityAsAddressed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authorizer.Refuse(domain.ErrNotFound)

	_, err := h.service.Get(t.Context(), actor(), "somebodyelses1")
	require.ErrorIs(t, err, domain.ErrNotFound)

	events := h.auditor.Events()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, audit.ActionAccessDenied, event.Action)
	assert.Equal(t, audit.ActorKindUser, event.ActorKind)
	assert.Equal(t, audit.TargetKindPractitioner, event.TargetKind)
	assert.Equal(t, "somebodyelses1", event.TargetID)
	assert.Equal(t, practitionertest.OwnerID, event.ActorID)
	assert.Equal(t, requestID, event.RequestID)
	assert.WithinDuration(t, time.Now(), event.OccurredAt, time.Minute)
	assert.NoError(t, event.Validate())
}

// TestAnIdentityThatNeverExistedIsAuditedTheSameAsOneThatIsSomebodyElses.
// Unlike medication's two-checkpoint design, this package has no kind-level
// Record lookup that can tell a genuine miss apart from an ownership
// violation before reaching the store — Repository.Owner is both checks at
// once, and its ErrNotFound is treated as the refusal either way (FR-037: the
// two are indistinguishable on the wire, and they are indistinguishable here
// too).
func TestAnIdentityThatNeverExistedIsAuditedTheSameAsOneThatIsSomebodyElses(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Get(t.Context(), actor(), "nosuchrecord01")

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Len(t, h.auditor.Events(), 1)
}

// TestOwnershipRefusalIs404Shaped is the ownership-refusal path Repository.Owner
// exists for: a stranger's practitioner is answered exactly as one that does
// not exist, and the attempt is audited.
func TestOwnershipRefusalIs404Shaped(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stranger, err := h.repository.Create(t.Context(), directory.Practitioner{
		OwnerID: practitionertest.StrangerID,
		Name:    "Dr. Somebody Else",
	})
	require.NoError(t, err)

	h.repository.Forget()

	_, err = h.service.Get(t.Context(), actor(), stranger.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	events := h.auditor.Events()
	require.Len(t, events, 1)
	assert.Equal(t, audit.TargetKindPractitioner, events[0].TargetKind)
}

func TestAFailingCheckpointIsNotADenial(t *testing.T) {
	t.Parallel()

	broken := errors.New("the checkpoint could not be reached")

	h := newHarness(t)
	h.authorizer.Refuse(broken)

	_, err := h.service.Get(t.Context(), actor(), "somerecord0001")

	require.ErrorIs(t, err, broken)
	assert.NotErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, h.auditor.Events())
}

func TestCreateAttributesTheRecordToTheActor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), directory.Practitioner{
		ID:        "chosenbycaller",
		OwnerID:   practitionertest.StrangerID,
		Name:      "Dr. Amara Okonkwo",
		CreatedAt: time.Unix(0, 0),
		UpdatedAt: time.Unix(0, 0),
		Version:   "chosen-by-the-caller",
	})
	require.NoError(t, err)

	assert.Equal(t, practitionertest.OwnerID, created.OwnerID, "the body chose the owner")
	assert.NotEqual(t, "chosenbycaller", created.ID, "the body chose the identity")
	assert.NotEqual(t, "chosen-by-the-caller", created.Version, "the body chose the version")
	assert.False(t, created.CreatedAt.Equal(time.Unix(0, 0)), "the body chose the creation time")
}

// TestCreateRefusesADuplicateNameAndSpecialty is FR-038 as the service sees
// it: the store's ErrConflict passes straight through.
func TestCreateRefusesADuplicateNameAndSpecialty(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	h.store(t, "Dr. Duplicate")

	_, err := h.service.Create(t.Context(), actor(), directory.Practitioner{Name: "dr. duplicate"})

	assert.ErrorIs(t, err, domain.ErrConflict)
}

// TestCreateRefusesASpecialtyOutsideTheVocabulary is FR-033: 422 invalid_value.
func TestCreateRefusesASpecialtyOutsideTheVocabulary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Create(t.Context(), actor(), directory.Practitioner{
		Name:      "Dr. Unknown Specialty",
		Specialty: "not-a-real-specialty",
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "specialty", invalid.Fields[0].Field)
	assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
	assert.Empty(t, h.repository.Writes())
}

// TestCreateAcceptsNoSpecialty. Empty is valid: unset, not "outside the
// vocabulary".
func TestCreateAcceptsNoSpecialty(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), directory.Practitioner{Name: "Dr. No Specialty"})

	require.NoError(t, err)
	assert.Empty(t, created.Specialty)
}

// TestCreateAcceptsAnEmptyFacility. FacilityID is optional and "" means none.
func TestCreateAcceptsAnEmptyFacility(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), directory.Practitioner{Name: "Dr. No Facility"})

	require.NoError(t, err)
	assert.Empty(t, created.FacilityID)
}

// TestCreateRefusesACrossOwnerFacilityReference is FR-042, the store's
// refusal passing through the service unchanged.
func TestCreateRefusesACrossOwnerFacilityReference(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.repository.AllowFacility(practitionertest.StrangerID, "somebodyelsfac")

	_, err := h.service.Create(t.Context(), actor(), directory.Practitioner{
		Name:       "Dr. Wrong Facility",
		FacilityID: "somebodyelsfac",
	})

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateReportsAMissingNameAndStoresNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Create(t.Context(), actor(), directory.Practitioner{})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "name", invalid.Fields[0].Field)
	assert.Empty(t, h.repository.Writes())
}

func TestUpdateChangesOnlyWhatWasSupplied(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), directory.Practitioner{
		Name:  "Dr. Amara Okonkwo",
		Phone: "+1 555 0100",
	})
	require.NoError(t, err)

	email := "amara@example.test"

	updated, err := h.service.Update(t.Context(), actor(), created.ID, created.Version,
		practitioner.Patch{Email: &email})
	require.NoError(t, err)

	assert.Equal(t, email, updated.Email)
	assert.Equal(t, "Dr. Amara Okonkwo", updated.Name)
	assert.Equal(t, "+1 555 0100", updated.Phone)

	cleared := ""

	afterClear, err := h.service.Update(t.Context(), actor(), created.ID, updated.Version,
		practitioner.Patch{FacilityID: &cleared})
	require.NoError(t, err)

	assert.Empty(t, afterClear.FacilityID)
	assert.Equal(t, email, afterClear.Email, "the clear took an unrelated field with it")
}

func TestUpdateValidatesTheResultAndStoresNothingWhenItIsInvalid(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), directory.Practitioner{Name: "Dr. Amara Okonkwo"})
	require.NoError(t, err)

	h.repository.Forget()

	outside := directory.Specialty("not-a-real-specialty")

	_, err = h.service.Update(t.Context(), actor(), created.ID, created.Version,
		practitioner.Patch{Specialty: &outside})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "specialty", invalid.Fields[0].Field)
	assert.Empty(t, h.repository.Writes())
}

func TestUpdateCarriesTheVersionToTheStore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	created := h.store(t, "Dr. Amara Okonkwo")

	_, err := h.service.Update(t.Context(), actor(), created.ID, "not-the-current-version", practitioner.Patch{})

	assert.ErrorIs(t, err, domain.ErrVersionMismatch)
}

func TestDeleteCarriesTheVersionToTheStore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	created := h.store(t, "Dr. Amara Okonkwo")

	err := h.service.Delete(t.Context(), actor(), created.ID, "not-the-current-version")

	assert.ErrorIs(t, err, domain.ErrVersionMismatch)
}

func TestListIsScopedToTheActor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.store(t, "Dr. Amara Okonkwo")

	_, err := h.repository.Create(t.Context(), directory.Practitioner{
		OwnerID: practitionertest.StrangerID,
		Name:    "Dr. Boris Novak",
	})
	require.NoError(t, err)

	page, err := h.service.List(t.Context(), actor(), practitioner.Query{})
	require.NoError(t, err)

	require.Len(t, page.Items, 1)
	assert.Equal(t, practitionertest.OwnerID, page.Items[0].OwnerID)
	assert.Equal(t, practitionertest.OwnerID, h.repository.LastOwner())
}

func TestListRefusesASpecialtyOutsideTheVocabulary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.List(t.Context(), actor(), practitioner.Query{Specialty: "not-a-real-specialty"})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, practitioner.FilterSpecialty, invalid.Fields[0].Field)
	assert.Empty(t, h.repository.Calls())
}

func TestListRefusesAnOrderingNobodyPublishes(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.List(t.Context(), actor(), practitioner.Query{
		Sort: []domain.SortKey{{Field: "notes"}},
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Empty(t, h.repository.Calls())
}

func TestNewRefusesAnIncompleteService(t *testing.T) {
	t.Parallel()

	repository := practitionertest.NewRepository()
	authorizer := practitionertest.NewAuthorizer(practitionertest.OwnerID)
	auditor := practitionertest.NewAuditor()

	for _, testCase := range []struct {
		name       string
		repository practitioner.Repository
		authorizer practitioner.Authorizer
		auditor    practitioner.Auditor
	}{
		{name: "no repository", authorizer: authorizer, auditor: auditor},
		{name: "no authorizer", repository: repository, auditor: auditor},
		{name: "no auditor", repository: repository, authorizer: authorizer},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			service, err := practitioner.New(testCase.repository, testCase.authorizer, testCase.auditor)

			require.Error(t, err)
			assert.Nil(t, service)
		})
	}
}

func TestAnUnrecordedRefusalIsStillARefusal(t *testing.T) {
	t.Parallel()

	unwritable := errors.New("the audit row could not be written")

	h := newHarness(t)
	h.authorizer.Refuse(domain.ErrNotFound)
	h.auditor.Fail(unwritable)

	_, err := h.service.Get(t.Context(), actor(), "somebodyelses1")

	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.ErrorIs(t, err, unwritable)
}

// TestDefaultAuthorizerGrantsOnlyAuthenticatedNonSuperusers is the wired-in
// default's own contract.
func TestDefaultAuthorizerGrantsOnlyAuthenticatedNonSuperusers(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		actor   access.Actor
		granted bool
	}{
		{"authenticated", access.Actor{UserID: "someone0000001"}, true},
		{"unauthenticated", access.Anonymous("req"), false},
		{"superuser", access.Actor{UserID: "someone0000001", IsSuperuser: true}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			grant, err := practitioner.DefaultAuthorizer.Actor(t.Context(), testCase.actor)
			require.NoError(t, err)

			assert.Equal(t, testCase.granted, grant.Allows(access.PermOwn))
		})
	}
}
