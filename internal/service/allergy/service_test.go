package allergy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/allergy"
	"medikube/internal/service/allergy/allergytest"
)

func newService(t *testing.T) (*allergy.Service, *allergytest.Repository, *allergytest.Authorizer) {
	t.Helper()

	repo := allergytest.NewRepository()
	auth := allergytest.NewAuthorizer(allergytest.OwnerID)

	svc, err := allergy.New(repo, auth)
	require.NoError(t, err)

	return svc, repo, auth
}

func owner() access.Actor { return access.Actor{UserID: allergytest.OwnerID, RequestID: "req-1"} }

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	t.Run("a patient is required", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		_, err := svc.Create(t.Context(), owner(), clinical.Allergy{Allergen: "Peanuts", Severity: clinical.SeverityMild})

		var invalid *domain.ValidationError
		assert.ErrorAs(t, err, &invalid)
	})

	t.Run("status defaults to active", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		created, err := svc.Create(t.Context(), owner(), clinical.Allergy{
			PatientID: allergytest.PatientID, Allergen: "Peanuts", Severity: clinical.SeverityMild,
		})
		require.NoError(t, err)
		assert.Equal(t, clinical.ConditionStatusActive, created.Status)
	})

	t.Run("an invalid draft is refused before it reaches the repository", func(t *testing.T) {
		t.Parallel()

		svc, repo, _ := newService(t)

		_, err := svc.Create(t.Context(), owner(), clinical.Allergy{PatientID: allergytest.PatientID})

		var invalid *domain.ValidationError
		assert.ErrorAs(t, err, &invalid)
		assert.NotContains(t, repo.Calls(), "create")
	})

	t.Run("a non-owner is refused", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		stranger := access.Actor{UserID: allergytest.StrangerID, RequestID: "req-2"}
		_, err := svc.Create(t.Context(), stranger, clinical.Allergy{
			PatientID: allergytest.PatientID, Allergen: "Peanuts", Severity: clinical.SeverityMild,
		})

		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServiceGetUpdateDelete(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), clinical.Allergy{
		PatientID: allergytest.PatientID, Allergen: "Peanuts", Severity: clinical.SeverityMild,
	})
	require.NoError(t, err)

	found, err := svc.Get(t.Context(), owner(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Allergen, found.Allergen)

	stranger := access.Actor{UserID: allergytest.StrangerID, RequestID: "req-2"}
	_, err = svc.Get(t.Context(), stranger, created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	reaction := "hives"
	updated, err := svc.Update(t.Context(), owner(), created.ID, created.Version, allergy.Patch{Reaction: &reaction})
	require.NoError(t, err)
	assert.Equal(t, "hives", updated.Reaction)

	require.NoError(t, svc.Delete(t.Context(), owner(), created.ID, updated.Version))

	_, err = svc.Get(t.Context(), owner(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestServiceList(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	_, err := svc.Create(t.Context(), owner(), clinical.Allergy{
		PatientID: allergytest.PatientID, Allergen: "Peanuts", Severity: clinical.SeverityMild,
	})
	require.NoError(t, err)

	page, err := svc.List(t.Context(), owner(), allergy.Query{PatientID: allergytest.PatientID})
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)
}
