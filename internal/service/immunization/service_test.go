package immunization_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/immunization"
	"medikube/internal/service/immunization/immunizationtest"
)

const requestID = "req-immunization-service"

func actor() access.Actor {
	return access.Actor{UserID: immunizationtest.OwnerID, RequestID: requestID}
}

type harness struct {
	service    *immunization.Service
	repository *immunizationtest.Repository
	authorizer *immunizationtest.Authorizer
}

func newHarness(t *testing.T) harness {
	t.Helper()

	repository := immunizationtest.NewRepository()
	authorizer := immunizationtest.NewAuthorizer(immunizationtest.OwnerID)

	service, err := immunization.New(repository, authorizer)
	require.NoError(t, err)

	return harness{service: service, repository: repository, authorizer: authorizer}
}

func TestNewRefusesAnIncompleteService(t *testing.T) {
	t.Parallel()

	_, err := immunization.New(nil, immunizationtest.NewAuthorizer(immunizationtest.OwnerID))
	assert.Error(t, err)

	_, err = immunization.New(immunizationtest.NewRepository(), nil)
	assert.Error(t, err)
}

func TestCreateRequiresAPatient(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Create(t.Context(), actor(), clinical.Immunization{VaccineName: "Influenza"})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

func TestCreateRefusesWhenTheAuthorizerRefuses(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authorizer.Refuse(domain.ErrNotFound)

	_, err := h.service.Create(t.Context(), actor(), clinical.Immunization{
		PatientID: immunizationtest.PatientID, VaccineName: "Influenza",
	})

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateAppliesTheDomainsValidation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	dose := 0

	_, err := h.service.Create(t.Context(), actor(), clinical.Immunization{
		PatientID: immunizationtest.PatientID, VaccineName: "Influenza", DoseNumber: &dose,
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

func TestUpdateAuthorizesAtTheEditLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored, err := h.repository.Create(t.Context(), clinical.Immunization{
		PatientID: immunizationtest.PatientID, VaccineName: "Influenza",
	})
	require.NoError(t, err)

	h.authorizer.Refuse(domain.ErrNotFound)

	_, err = h.service.Update(t.Context(), actor(), stored.ID, stored.Version, immunization.Patch{})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteAuthorizesAtTheOwnLevel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored, err := h.repository.Create(t.Context(), clinical.Immunization{
		PatientID: immunizationtest.PatientID, VaccineName: "Influenza",
	})
	require.NoError(t, err)

	h.authorizer.Grant(access.PermEdit)

	err = h.service.Delete(t.Context(), actor(), stored.ID, stored.Version)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListRefusesAnUnpublishedSort(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.List(t.Context(), actor(), immunization.Query{
		PatientID: immunizationtest.PatientID,
		Sort:      []domain.SortKey{{Field: "no_such_field"}},
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}
