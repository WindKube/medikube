package encounter_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/encounter"
	"medikube/internal/service/encounter/encountertest"
)

const requestID = "req-encounter-service"

func actor() access.Actor {
	return access.Actor{UserID: encountertest.OwnerID, RequestID: requestID}
}

type harness struct {
	service    *encounter.Service
	repository *encountertest.Repository
	authorizer *encountertest.Authorizer
}

func newHarness(t *testing.T) harness {
	t.Helper()

	repository := encountertest.NewRepository()
	authorizer := encountertest.NewAuthorizer(encountertest.OwnerID)

	service, err := encounter.New(repository, authorizer)
	require.NoError(t, err)

	return harness{service: service, repository: repository, authorizer: authorizer}
}

func (h harness) store(t *testing.T, reason string) clinical.Encounter {
	t.Helper()

	stored, err := h.repository.Create(t.Context(), clinical.Encounter{
		PatientID: encountertest.PatientID,
		Reason:    reason, OccurredOn: dateOrPanic("2026-03-01"),
	})
	require.NoError(t, err)

	h.repository.Forget()

	return stored
}

func dateOrPanic(text string) domain.Date {
	d, err := domain.ParseDate(text)
	if err != nil {
		panic(err)
	}

	return d
}

// TestEveryMethodAuthorizesBeforeItReachesTheStore mirrors medication's own
// structural proof (T137): every method of the service reaches the
// authorization checkpoint, and a refusal stops it before any write.
func TestEveryMethodAuthorizesBeforeItReachesTheStore(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(&encounter.Service{})
	require.GreaterOrEqual(t, serviceType.NumMethod(), 5,
		"the walk found almost no methods; it is not looking at the service it thinks it is")

	for i := range serviceType.NumMethod() {
		method := serviceType.Method(i)

		t.Run(method.Name, func(t *testing.T) {
			t.Parallel()

			refused := newHarness(t)
			stored := refused.store(t, "Annual check-up")
			refused.authorizer.Refuse(domain.ErrNotFound)

			results := call(t, refused.service, method, stored)

			assert.Positive(t, refused.authorizer.Calls(),
				"%s never consulted the authorization checkpoint", method.Name)
			assert.Empty(t, refused.repository.Writes(),
				"%s wrote to the store after the checkpoint refused", method.Name)

			last := results[len(results)-1]
			require.True(t, last.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()))
			assert.True(t, !last.IsNil(), "%s returned no error for a refused actor", method.Name)
		})
	}
}

func call(t *testing.T, service *encounter.Service, method reflect.Method, stored clinical.Encounter) []reflect.Value {
	t.Helper()

	args := make([]reflect.Value, 0, method.Type.NumIn())
	args = append(args, reflect.ValueOf(service))

	strings := 0

	for i := 1; i < method.Type.NumIn(); i++ {
		switch in := method.Type.In(i); {
		case in == reflect.TypeOf((*context.Context)(nil)).Elem():
			args = append(args, reflect.ValueOf(t.Context()))
		case in == reflect.TypeOf(access.Actor{}):
			args = append(args, reflect.ValueOf(actor()))
		case in == reflect.TypeOf(""):
			strings++
			if strings == 1 {
				args = append(args, reflect.ValueOf(stored.ID))
			} else {
				args = append(args, reflect.ValueOf(stored.Version))
			}
		case in == reflect.TypeOf(encounter.Query{}):
			args = append(args, reflect.ValueOf(encounter.Query{PatientID: stored.PatientID}))
		case in == reflect.TypeOf(clinical.Encounter{}):
			args = append(args, reflect.ValueOf(clinical.Encounter{
				PatientID: stored.PatientID, Reason: "Follow-up", OccurredOn: dateOrPanic("2026-03-02"),
			}))
		default:
			args = append(args, reflect.New(in).Elem())
		}
	}

	return method.Func.Call(args)
}

func TestCreateStoresTheDraftAgainstTheAuthorizedPatient(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), clinical.Encounter{
		PatientID: encountertest.PatientID, Reason: "Sprained ankle", OccurredOn: dateOrPanic("2026-04-01"),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "Sprained ankle", created.Reason)
	assert.Equal(t, encountertest.PatientID, h.authorizer.LastPatient())
}

func TestCreateRefusesAnEncounterWithNoPatient(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Create(t.Context(), actor(), clinical.Encounter{Reason: "Sprained ankle"})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "patient", invalid.Fields[0].Field)
	assert.Zero(t, h.authorizer.Calls(), "a draft with no patient never reaches the checkpoint")
}

func TestUpdateAppliesOnlyTheSuppliedFields(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.store(t, "Annual check-up")

	newReason := "Annual check-up, revised"
	updated, err := h.service.Update(t.Context(), actor(), stored.ID, stored.Version,
		encounter.Patch{Reason: &newReason})

	require.NoError(t, err)
	assert.Equal(t, newReason, updated.Reason)
	assert.Equal(t, stored.OccurredOn, updated.OccurredOn, "a field the patch did not name is untouched")
}

func TestDeleteRemovesTheRecord(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.store(t, "Annual check-up")

	require.NoError(t, h.service.Delete(t.Context(), actor(), stored.ID, stored.Version))

	_, err := h.repository.Get(t.Context(), stored.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
