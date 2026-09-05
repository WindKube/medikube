package injury_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/injury"
	"medikube/internal/service/injury/injurytest"
)

const requestID = "req-injury-service"

func actor() access.Actor {
	return access.Actor{UserID: injurytest.OwnerID, RequestID: requestID}
}

type harness struct {
	service    *injury.Service
	repository *injurytest.Repository
	authorizer *injurytest.Authorizer
}

func newHarness(t *testing.T) harness {
	t.Helper()

	repository := injurytest.NewRepository()
	authorizer := injurytest.NewAuthorizer(injurytest.OwnerID)

	service, err := injury.New(repository, authorizer)
	require.NoError(t, err)

	return harness{service: service, repository: repository, authorizer: authorizer}
}

func TestNewRefusesAnIncompleteService(t *testing.T) {
	t.Parallel()

	_, err := injury.New(nil, injurytest.NewAuthorizer(injurytest.OwnerID))
	assert.Error(t, err)

	_, err = injury.New(injurytest.NewRepository(), nil)
	assert.Error(t, err)
}

func TestCreateRequiresAPatient(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Create(t.Context(), actor(), clinical.Injury{Name: "Sprain", BodyPart: "ankle"})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

func TestCreateAppliesTheDefaultStatus(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created, err := h.service.Create(t.Context(), actor(), clinical.Injury{
		PatientID: injurytest.PatientID, Name: "Sprain", BodyPart: "ankle",
	})
	require.NoError(t, err)

	assert.Equal(t, clinical.ConditionStatusActive, created.Status)
}

func TestCreateRefusesWhenTheAuthorizerRefuses(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authorizer.Refuse(domain.ErrNotFound)

	_, err := h.service.Create(t.Context(), actor(), clinical.Injury{
		PatientID: injurytest.PatientID, Name: "Sprain", BodyPart: "ankle",
	})

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUpdateAuthorizesAtTheEditLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored, err := h.repository.Create(t.Context(), clinical.Injury{
		PatientID: injurytest.PatientID, Name: "Sprain", BodyPart: "ankle", Status: clinical.ConditionStatusActive,
	})
	require.NoError(t, err)

	h.authorizer.Refuse(domain.ErrNotFound)

	_, err = h.service.Update(t.Context(), actor(), stored.ID, stored.Version, injury.Patch{})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteAuthorizesAtTheOwnLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored, err := h.repository.Create(t.Context(), clinical.Injury{
		PatientID: injurytest.PatientID, Name: "Sprain", BodyPart: "ankle", Status: clinical.ConditionStatusActive,
	})
	require.NoError(t, err)

	h.authorizer.Grant(access.PermEdit)

	err = h.service.Delete(t.Context(), actor(), stored.ID, stored.Version)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListNarrowsByUnresolved(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	active, err := h.repository.Create(t.Context(), clinical.Injury{
		PatientID: injurytest.PatientID, Name: "Fresh burn", BodyPart: "hand", Status: clinical.ConditionStatusActive,
	})
	require.NoError(t, err)

	resolved, err := h.repository.Create(t.Context(), clinical.Injury{
		PatientID: injurytest.PatientID, Name: "Old sprain", BodyPart: "ankle", Status: clinical.ConditionStatusResolved,
	})
	require.NoError(t, err)

	page, err := h.service.List(t.Context(), actor(), injury.Query{PatientID: injurytest.PatientID, Unresolved: true})
	require.NoError(t, err)

	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	assert.Contains(t, found, active.ID)
	assert.NotContains(t, found, resolved.ID)
}

func TestListRefusesAnUnpublishedFilterValue(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.List(t.Context(), actor(), injury.Query{
		PatientID: injurytest.PatientID,
		Types:     []clinical.InjuryType{"not-a-real-type"},
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}
