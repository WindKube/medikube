package medication_test

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
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/medication"
	"medikube/internal/service/medication/medicationtest"
)

const requestID = "req-medication-service"

func actor() access.Actor {
	return access.Actor{UserID: medicationtest.OwnerID, RequestID: requestID}
}

type harness struct {
	service    *medication.Service
	repository *medicationtest.Repository
	authorizer *medicationtest.Authorizer
	auditor    *medicationtest.Auditor
}

func newHarness(t *testing.T) harness {
	t.Helper()

	repository := medicationtest.NewRepository()
	authorizer := medicationtest.NewAuthorizer(medicationtest.OwnerID)
	auditor := medicationtest.NewAuditor()

	service, err := medication.New(repository, authorizer, auditor)
	require.NoError(t, err)

	return harness{service: service, repository: repository, authorizer: authorizer, auditor: auditor}
}

func (h harness) store(t *testing.T, name string) clinical.Medication {
	t.Helper()

	stored, err := h.repository.Create(t.Context(), clinical.Medication{
		OwnerID: medicationtest.OwnerID,
		Name:    name,
		Status:  clinical.TherapyStatusActive,
	})
	require.NoError(t, err)

	h.repository.Forget()

	return stored
}

// TestEveryMethodAuthorizesBeforeItReachesTheStore is T137, and it is
// deliberately structural rather than a check of the five methods that exist
// today.
//
// It walks the method set with reflection, so a sixth method added next year is
// covered on the day it is added — which is the only version of this test worth
// having. The two halves are what "authorization first, always" means
// operationally: the checkpoint is consulted, and a refusal stops the call
// before the store is touched.
func TestEveryMethodAuthorizesBeforeItReachesTheStore(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(&medication.Service{})

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
			assert.Empty(t, refused.repository.Calls(),
				"%s reached the store after the checkpoint refused: %v", method.Name, refused.repository.Calls())
			assert.ErrorIs(t, errorOf(t, results), domain.ErrNotFound,
				"%s answered a refusal as something other than a miss", method.Name)

			denials := refused.auditor.Events()
			require.Len(t, denials, 1, "%s wrote %d audit rows for one refusal", method.Name, len(denials))
			assert.Equal(t, audit.ActionAccessDenied, denials[0].Action)

			granted := newHarness(t)
			_ = call(t, granted.service, method)

			assert.Positive(t, granted.authorizer.Calls(),
				"%s never consulted the authorization checkpoint", method.Name)
			assert.Equal(t, kind.Medication, granted.authorizer.LastKind(),
				"%s authorized against another kind's records", method.Name)
		})
	}
}

// call invokes a method with a context, an actor and the zero value of
// everything else. The zero values are the point: a zero id and an empty patch
// are refused by validation, and validation runs after the checkpoint — so a
// method that reached them at all has already passed the assertion above.
func call(t *testing.T, service *medication.Service, method reflect.Method) []reflect.Value {
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
	assert.Equal(t, audit.TargetKindMedication, event.TargetKind)
	assert.Equal(t, "somebodyelses1", event.TargetID, "the row records the id as it was addressed")
	assert.Equal(t, medicationtest.OwnerID, event.ActorID)
	assert.Equal(t, requestID, event.RequestID)
	assert.WithinDuration(t, time.Now(), event.OccurredAt, time.Minute)
	assert.NoError(t, event.Validate(), "the row the service builds is one the store would refuse")
}

// TestAGenuineMissWritesNoAuditRow is the distinction research D-20 introduces
// the action for: a refusal is somebody reaching for a record that is not
// theirs, and a miss is a record that is not there. The checkpoint is what
// tells them apart, so the audit row is written where the checkpoint refuses
// and nowhere else.
func TestAGenuineMissWritesNoAuditRow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Get(t.Context(), actor(), "nosuchrecord01")

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, h.auditor.Events(), "a record that never existed is not an access denial")
}

// TestAGrantThatDoesNotCoverTheNeedIsARefusal. An authorizer that answers
// without an error has still only granted what it granted; a service that read
// the error and not the level would let a view-only caller delete.
func TestAGrantThatDoesNotCoverTheNeedIsARefusal(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		granted access.Permission
		call    func(*testing.T, harness, clinical.Medication) error
	}{
		{
			name:    "delete with edit",
			granted: access.PermEdit,
			call: func(t *testing.T, h harness, stored clinical.Medication) error {
				return h.service.Delete(t.Context(), actor(), stored.ID, stored.Version)
			},
		},
		{
			name:    "update with view",
			granted: access.PermView,
			call: func(t *testing.T, h harness, stored clinical.Medication) error {
				_, err := h.service.Update(t.Context(), actor(), stored.ID, stored.Version, medication.Patch{})

				return err
			},
		},
		{
			name:    "create with view",
			granted: access.PermView,
			call: func(t *testing.T, h harness, _ clinical.Medication) error {
				_, err := h.service.Create(t.Context(), actor(), clinical.Medication{Name: "Amoxicillin"})

				return err
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			stored := h.store(t, "Amoxicillin")
			h.authorizer.Grant(testCase.granted)

			err := testCase.call(t, h, stored)

			assert.ErrorIs(t, err, domain.ErrNotFound)
			assert.Empty(t, h.repository.Writes(), "the store was written to on a grant that did not cover the need")
			assert.Len(t, h.auditor.Events(), 1)
		})
	}
}

// TestAFailingCheckpointIsNotADenial. A checkpoint that could not answer has
// not refused anybody: answering 404 would tell a caller the record is not
// theirs on the strength of a database being down, and the audit trail would
// fill with denials nobody attempted.
func TestAFailingCheckpointIsNotADenial(t *testing.T) {
	t.Parallel()

	broken := errors.New("the checkpoint could not be reached")

	h := newHarness(t)
	h.authorizer.Refuse(broken)

	_, err := h.service.Get(t.Context(), actor(), "somerecord0001")

	require.ErrorIs(t, err, broken)
	assert.NotErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, h.auditor.Events())
	assert.Empty(t, h.repository.Calls())
}

// TestCreateAttributesTheRecordToTheActor is FR-032 at the last place it can
// be enforced. The write DTO has no owner member, so this cannot arrive over
// HTTP — and this is the assertion that keeps it true for the next caller.
func TestCreateAttributesTheRecordToTheActor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), clinical.Medication{
		ID:        "chosenbycaller",
		OwnerID:   medicationtest.StrangerID,
		Name:      "Amoxicillin",
		Status:    clinical.TherapyStatusActive,
		CreatedAt: time.Unix(0, 0),
		UpdatedAt: time.Unix(0, 0),
		Version:   "chosen-by-the-caller",
	})
	require.NoError(t, err)

	assert.Equal(t, medicationtest.OwnerID, created.OwnerID, "the body chose the owner")
	assert.NotEqual(t, "chosenbycaller", created.ID, "the body chose the identity")
	assert.NotEqual(t, "chosen-by-the-caller", created.Version, "the body chose the version")
	assert.False(t, created.CreatedAt.Equal(time.Unix(0, 0)), "the body chose the creation time")
}

// TestCreateDefaultsTheState is data-model §2's `default active`, applied here
// rather than by the column so that the entity the service validates is the
// entity that gets stored.
func TestCreateDefaultsTheState(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), clinical.Medication{Name: "Amoxicillin"})
	require.NoError(t, err)

	assert.Equal(t, clinical.TherapyStatusActive, created.Status)
}

// TestCreateReportsEveryViolationAtOnceAndStoresNothing is FR-027 and US1-4.
func TestCreateReportsEveryViolationAtOnceAndStoresNothing(t *testing.T) {
	t.Parallel()

	started, err := domain.NewDate(2026, time.March, 10)
	require.NoError(t, err)

	ended, err := domain.NewDate(2026, time.March, 1)
	require.NoError(t, err)

	h := newHarness(t)

	_, err = h.service.Create(t.Context(), actor(), clinical.Medication{
		StartedOn: started,
		EndedOn:   ended,
		Type:      clinical.MedicationType("homeopathic"),
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)

	fields := make([]string, 0, len(invalid.Fields))
	for _, field := range invalid.Fields {
		fields = append(fields, field.Field)
	}

	assert.ElementsMatch(t, []string{"name", "type", "ended_on"}, fields,
		"every problem is reported in one response, not the first one found")
	assert.Empty(t, h.repository.Writes(), "an invalid medication reached the store")
}

// TestUpdateChangesOnlyWhatWasSupplied is contracts/records.md's "a PATCH with
// one field leaves the other twelve untouched", and the second half is the
// explicit clear: absent and null are different instructions.
func TestUpdateChangesOnlyWhatWasSupplied(t *testing.T) {
	t.Parallel()

	started, err := domain.NewDate(2026, time.February, 2)
	require.NoError(t, err)

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), clinical.Medication{
		Name:       "Amoxicillin",
		Dosage:     "500 mg",
		Indication: "a chest infection",
		StartedOn:  started,
		Status:     clinical.TherapyStatusActive,
	})
	require.NoError(t, err)

	dosage := "250 mg"

	updated, err := h.service.Update(t.Context(), actor(), created.ID, created.Version, medication.Patch{Dosage: &dosage})
	require.NoError(t, err)

	assert.Equal(t, "250 mg", updated.Dosage)
	assert.Equal(t, "Amoxicillin", updated.Name)
	assert.Equal(t, "a chest infection", updated.Indication)
	assert.Equal(t, started, updated.StartedOn)

	cleared, err := h.service.Update(t.Context(), actor(), created.ID, updated.Version,
		medication.Patch{StartedOn: &domain.Date{}})
	require.NoError(t, err)

	assert.True(t, cleared.StartedOn.IsZero(), "an explicit null did not clear the date")
	assert.Equal(t, "250 mg", cleared.Dosage, "the clear took an unrelated field with it")
}

// TestUpdateValidatesTheResultAndStoresNothingWhenItIsInvalid. The rules are
// checked against the medication as it would be after the patch, not against
// the patch, because "the end date is before the start date" is a property of
// neither half on its own.
func TestUpdateValidatesTheResultAndStoresNothingWhenItIsInvalid(t *testing.T) {
	t.Parallel()

	started, err := domain.NewDate(2026, time.May, 20)
	require.NoError(t, err)

	earlier, err := domain.NewDate(2026, time.May, 1)
	require.NoError(t, err)

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), clinical.Medication{
		Name:      "Amoxicillin",
		StartedOn: started,
		Status:    clinical.TherapyStatusActive,
	})
	require.NoError(t, err)

	h.repository.Forget()

	_, err = h.service.Update(t.Context(), actor(), created.ID, created.Version, medication.Patch{EndedOn: &earlier})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "ended_on", invalid.Fields[0].Field)
	assert.Equal(t, clinical.CodeEndBeforeStart, invalid.Fields[0].Code)
	assert.Empty(t, h.repository.Writes(), "the store was written to with a medication that does not validate")
}

// TestUpdateCarriesTheVersionToTheStore. The service does not compare versions
// itself: the comparison and the write have to be one act, or two callers can
// both read the same version and both be told they are current.
func TestUpdateCarriesTheVersionToTheStore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	created := h.store(t, "Amoxicillin")

	_, err := h.service.Update(t.Context(), actor(), created.ID, "a version that is not the current one",
		medication.Patch{})

	assert.ErrorIs(t, err, domain.ErrVersionMismatch)
}

func TestDeleteCarriesTheVersionToTheStore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	created := h.store(t, "Amoxicillin")

	err := h.service.Delete(t.Context(), actor(), created.ID, "a version that is not the current one")

	assert.ErrorIs(t, err, domain.ErrVersionMismatch)
}

// TestListIsScopedToTheActor. The owner is read from the authenticated actor
// and from nothing else — there is no parameter here a caller could name
// somebody else's account in (FR-032).
func TestListIsScopedToTheActor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.store(t, "Amoxicillin")

	_, err := h.repository.Create(t.Context(), clinical.Medication{
		OwnerID: medicationtest.StrangerID,
		Name:    "Bisoprolol",
		Status:  clinical.TherapyStatusActive,
	})
	require.NoError(t, err)

	page, err := h.service.List(t.Context(), actor(), medication.Query{})
	require.NoError(t, err)

	require.Len(t, page.Items, 1)
	assert.Equal(t, medicationtest.OwnerID, page.Items[0].OwnerID)
	assert.Equal(t, medicationtest.OwnerID, h.repository.LastOwner())
}

// TestListRefusesAVocabularyItDoesNotPublish. A silently ignored sort produces
// a list that looks right and is not; a silently ignored state narrows to
// everything.
func TestListRefusesAVocabularyItDoesNotPublish(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		query medication.Query
		field string
	}{
		{
			name:  "a state nobody publishes",
			query: medication.Query{Statuses: []clinical.TherapyStatus{"lapsed"}},
			field: medication.FilterStatus,
		},
		{
			name:  "an ordering nobody publishes",
			query: medication.Query{Sort: []domain.SortKey{{Field: "notes"}}},
			field: medication.ParamSort,
		},
		{
			name:  "a published column in an unpublished direction",
			query: medication.Query{Sort: []domain.SortKey{{Field: medication.FieldStartedOn, Desc: true}, {Field: "id"}}},
			field: medication.ParamSort,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			_, err := h.service.List(t.Context(), actor(), testCase.query)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, testCase.field, invalid.Fields[0].Field)
			assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
			assert.Empty(t, h.repository.Calls(), "the store was asked a question the service does not publish")
		})
	}
}

// TestListDefaultsToThePublishedOrdering. The default is the first published
// ordering and is applied here rather than left to the store, so that the
// cursor a page mints is bound to an ordering the next request will resolve to
// the same way.
func TestListDefaultsToThePublishedOrdering(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.store(t, "Amoxicillin")

	_, err := h.service.List(t.Context(), actor(), medication.Query{})
	require.NoError(t, err)

	assert.Equal(t, medication.Sorts()[:1], h.repository.LastQuery().Sort)
}

// TestNewRefusesAnIncompleteService. A nil authorizer is a service with no
// authorization at all, and it fails at boot rather than on the first request
// that would have been refused.
func TestNewRefusesAnIncompleteService(t *testing.T) {
	t.Parallel()

	repository := medicationtest.NewRepository()
	authorizer := medicationtest.NewAuthorizer(medicationtest.OwnerID)
	auditor := medicationtest.NewAuditor()

	for _, testCase := range []struct {
		name       string
		repository medication.Repository
		authorizer medication.Authorizer
		auditor    medication.Auditor
	}{
		{name: "no repository", authorizer: authorizer, auditor: auditor},
		{name: "no authorizer", repository: repository, auditor: auditor},
		{name: "no auditor", repository: repository, authorizer: authorizer},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			service, err := medication.New(testCase.repository, testCase.authorizer, testCase.auditor)

			require.Error(t, err)
			assert.Nil(t, service)
		})
	}
}

// TestAnUnrecordedRefusalIsStillARefusal. The audit write failing must not
// change what the caller is told: a 500 where every other refusal is a 404 is
// an oracle for "this record exists and is somebody else's".
func TestAnUnrecordedRefusalIsStillARefusal(t *testing.T) {
	t.Parallel()

	unwritable := errors.New("the audit row could not be written")

	h := newHarness(t)
	h.authorizer.Refuse(domain.ErrNotFound)
	h.auditor.Fail(unwritable)

	_, err := h.service.Get(t.Context(), actor(), "somebodyelses1")

	assert.ErrorIs(t, err, domain.ErrNotFound, "the caller was told something other than a miss")
	assert.ErrorIs(t, err, unwritable, "the unwritten audit row was swallowed")
}
