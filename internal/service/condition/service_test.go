package condition_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/condition"
	"medikube/internal/service/condition/conditiontest"
)

func newService(t *testing.T) (*condition.Service, *conditiontest.Repository, *conditiontest.Authorizer) {
	t.Helper()

	repo := conditiontest.NewRepository()
	auth := conditiontest.NewAuthorizer(conditiontest.OwnerID)

	svc, err := condition.New(repo, auth)
	require.NoError(t, err)

	return svc, repo, auth
}

func owner() access.Actor { return access.Actor{UserID: conditiontest.OwnerID, RequestID: "req-1"} }

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	t.Run("a patient is required", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		_, err := svc.Create(t.Context(), owner(), clinical.Condition{
			Diagnosis: "Type 2 diabetes", Status: clinical.ConditionStatusActive,
		})

		var invalid *domain.ValidationError
		assert.ErrorAs(t, err, &invalid)
	})

	t.Run("an invalid draft is refused before it reaches the repository", func(t *testing.T) {
		t.Parallel()

		svc, repo, _ := newService(t)

		_, err := svc.Create(t.Context(), owner(), clinical.Condition{PatientID: conditiontest.PatientID})

		var invalid *domain.ValidationError
		assert.ErrorAs(t, err, &invalid)
		assert.NotContains(t, repo.Calls(), "create")
	})

	t.Run("resolved without resolved_on is refused", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		_, err := svc.Create(t.Context(), owner(), clinical.Condition{
			PatientID: conditiontest.PatientID, Diagnosis: "Afib", Status: clinical.ConditionStatusResolved,
		})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "resolved_on", invalid.Fields[0].Field)
	})

	t.Run("a non-owner is refused", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		stranger := access.Actor{UserID: conditiontest.StrangerID, RequestID: "req-2"}
		_, err := svc.Create(t.Context(), stranger, clinical.Condition{
			PatientID: conditiontest.PatientID, Diagnosis: "Afib", Status: clinical.ConditionStatusActive,
		})

		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServiceGetUpdateDelete(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), clinical.Condition{
		PatientID: conditiontest.PatientID, Diagnosis: "Afib", Status: clinical.ConditionStatusActive,
	})
	require.NoError(t, err)

	found, err := svc.Get(t.Context(), owner(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Diagnosis, found.Diagnosis)

	stranger := access.Actor{UserID: conditiontest.StrangerID, RequestID: "req-2"}
	_, err = svc.Get(t.Context(), stranger, created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	severity := clinical.SeverityModerate
	updated, err := svc.Update(t.Context(), owner(), created.ID, created.Version, condition.Patch{Severity: &severity})
	require.NoError(t, err)
	assert.Equal(t, clinical.SeverityModerate, updated.Severity)

	require.NoError(t, svc.Delete(t.Context(), owner(), created.ID, updated.Version))

	_, err = svc.Get(t.Context(), owner(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestServiceList(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	_, err := svc.Create(t.Context(), owner(), clinical.Condition{
		PatientID: conditiontest.PatientID, Diagnosis: "Afib", Status: clinical.ConditionStatusActive,
	})
	require.NoError(t, err)

	page, err := svc.List(t.Context(), owner(), condition.Query{PatientID: conditiontest.PatientID})
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)
}
