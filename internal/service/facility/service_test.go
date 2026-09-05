package facility_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/directory"
	"medikube/internal/service/facility"
	"medikube/internal/service/facility/facilitytest"
)

const requestID = "req-facility-service"

func actor() access.Actor {
	return access.Actor{UserID: facilitytest.OwnerID, RequestID: requestID}
}

type harness struct {
	service    *facility.Service
	repository *facilitytest.Repository
	authorizer *facilitytest.Authorizer
	auditor    *facilitytest.Auditor
}

func newHarness(t *testing.T) harness {
	t.Helper()

	repository := facilitytest.NewRepository()
	authorizer := facilitytest.NewAuthorizer()
	auditor := facilitytest.NewAuditor()

	service, err := facility.New(repository, authorizer, auditor)
	require.NoError(t, err)

	return harness{service: service, repository: repository, authorizer: authorizer, auditor: auditor}
}

func (h harness) store(t *testing.T, name string) directory.Facility {
	t.Helper()

	stored, err := h.repository.Create(t.Context(), directory.Facility{
		OwnerID: facilitytest.OwnerID,
		Kind:    directory.FacilityKindPractice,
		Name:    name,
	})
	require.NoError(t, err)

	h.repository.Forget()

	return stored
}

func draft(name string) directory.Facility {
	return directory.Facility{Kind: directory.FacilityKindPractice, Name: name}
}

// fakeMetrics is a hand-written fake (Principle III): anything with a
// RecordCreated(string) method satisfies facility.Metrics, the same way
// *obs.Metrics does in production with no import of it here.
type fakeMetrics struct{ created []string }

func (f *fakeMetrics) RecordCreated(kind string) { f.created = append(f.created, kind) }

// T160: a successful create reports medikube_records_total{kind="facility"}.
func TestCreateReportsTheRecordCreatedMetric(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	metrics := &fakeMetrics{}
	h.service.SetMetrics(metrics)

	_, err := h.service.Create(t.Context(), actor(), draft("Riverside Practice"))
	require.NoError(t, err)

	assert.Equal(t, []string{"facility"}, metrics.created)
}

// TestCreateRequiresAKind is FR-034: kind is required, and its absence is a
// 422 shaped as domain.ValidationError with the required code.
func TestCreateRequiresAKind(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Create(t.Context(), actor(), directory.Facility{Name: "Riverside Practice"})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.True(t, hasField(invalid, "kind", domain.CodeRequired), "missing kind: %v", invalid)
}

// TestCreateRefusesAKindOutsideTheVocabulary is the other half: a kind that
// is not empty and not one of the six published values is invalid_value, not
// silently accepted.
func TestCreateRefusesAKindOutsideTheVocabulary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Create(t.Context(), actor(), directory.Facility{
		Kind: directory.FacilityKind("clinic"),
		Name: "Riverside Practice",
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.True(t, hasField(invalid, "kind", domain.CodeInvalidValue), "kind outside the vocabulary: %v", invalid)
}

func hasField(invalid *domain.ValidationError, field, code string) bool {
	for _, entry := range invalid.Fields {
		if entry.Field == field && entry.Code == code {
			return true
		}
	}

	return false
}

// TestCreateAcceptsTwoFacilitiesSharingAName is FR-035 and US5-3: a second
// branch of the same chain is a second row, and both are stored and both
// appear in a subsequent list — the mirror image of the practitioner
// uniqueness test, deliberately as mandatory as it.
func TestCreateAcceptsTwoFacilitiesSharingAName(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	first := draft("Boots")
	first.Street = "1 High Street"

	second := draft("Boots")
	second.Street = "2 Station Road"

	firstStored, err := h.service.Create(t.Context(), actor(), first)
	require.NoError(t, err)

	secondStored, err := h.service.Create(t.Context(), actor(), second)
	require.NoError(t, err)

	assert.NotEqual(t, firstStored.ID, secondStored.ID)

	page, err := h.service.List(t.Context(), actor(), facility.Query{})
	require.NoError(t, err)

	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	assert.ElementsMatch(t, []string{firstStored.ID, secondStored.ID}, found,
		"both branches of the same chain must appear in the list")
}

// TestARecordThatBelongsToAnotherOwnerIsRefusedAsNotFound is FR-037: a
// stranger addressing somebody else's facility gets the same shape as one
// that never existed, and the attempt is audited.
func TestARecordThatBelongsToAnotherOwnerIsRefusedAsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.store(t, "Somebody Else's Practice")

	stranger := access.Actor{UserID: facilitytest.StrangerID, RequestID: requestID}

	_, err := h.service.Get(t.Context(), stranger, stored.ID)

	require.ErrorIs(t, err, domain.ErrNotFound)

	denials := h.auditor.Events()
	require.Len(t, denials, 1, "a cross-owner attempt must be audited exactly once")
	assert.Equal(t, audit.ActionAccessDenied, denials[0].Action)
	assert.Equal(t, audit.TargetKindFacility, denials[0].TargetKind)
	assert.Equal(t, stored.ID, denials[0].TargetID)
}

// TestARecordThatNeverExistedIsRefusedWithNoAudit is the control for the case
// above: a genuine miss is not a cross-owner attempt and is never audited
// (research D-20).
func TestARecordThatNeverExistedIsRefusedWithNoAudit(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Get(t.Context(), actor(), "nosuchrecord01")

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, h.auditor.Events(), "a record that never existed is not a refusal to audit")
}

// TestEveryMethodAuthorizesBeforeItReachesTheStore is structural, mirroring
// medication's T137: it walks the method set so a method added later is
// covered on the day it is added.
func TestEveryMethodAuthorizesBeforeItReachesTheStore(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(&facility.Service{})

	require.GreaterOrEqual(t, serviceType.NumMethod(), 5,
		"the walk found almost no methods; it is not looking at the service it thinks it is")

	for i := range serviceType.NumMethod() {
		method := serviceType.Method(i)

		// SetMetrics is a wiring setter (T160), not a use case: it carries no
		// context and no actor, so there is nothing for it to authorize.
		if method.Name == "SetMetrics" {
			continue
		}

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
		})
	}
}

func call(t *testing.T, service *facility.Service, method reflect.Method) []reflect.Value {
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
