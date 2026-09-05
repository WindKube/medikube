package emergencycontact_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/emergencycontact"
	"medikube/internal/service/emergencycontact/emergencycontacttest"
)

func newService(t *testing.T) (*emergencycontact.Service, *emergencycontacttest.Repository, *emergencycontacttest.Authorizer) {
	t.Helper()

	repo := emergencycontacttest.NewRepository()
	auth := emergencycontacttest.NewAuthorizer(emergencycontacttest.OwnerID)

	svc, err := emergencycontact.New(repo, auth)
	require.NoError(t, err)

	return svc, repo, auth
}

func owner() access.Actor {
	return access.Actor{UserID: emergencycontacttest.OwnerID, RequestID: "req-1"}
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	t.Run("a patient is required", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		_, err := svc.Create(t.Context(), owner(), clinical.EmergencyContact{
			Name: "Chidi", Relationship: clinical.ContactRelationshipSibling, Phone: "+1",
		})

		var invalid *domain.ValidationError
		assert.ErrorAs(t, err, &invalid)
	})

	t.Run("an invalid draft is refused before it reaches the repository", func(t *testing.T) {
		t.Parallel()

		svc, repo, _ := newService(t)

		_, err := svc.Create(t.Context(), owner(), clinical.EmergencyContact{PatientID: emergencycontacttest.PatientID})

		var invalid *domain.ValidationError
		assert.ErrorAs(t, err, &invalid)
		assert.NotContains(t, repo.Calls(), "create")
	})

	t.Run("a non-owner is refused", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		stranger := access.Actor{UserID: emergencycontacttest.StrangerID, RequestID: "req-2"}
		_, err := svc.Create(t.Context(), stranger, clinical.EmergencyContact{
			PatientID: emergencycontacttest.PatientID, Name: "Chidi", Relationship: clinical.ContactRelationshipSibling, Phone: "+1",
		})

		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServiceDisplacesThePreviousPrimary(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	first, err := svc.Create(t.Context(), owner(), clinical.EmergencyContact{
		PatientID: emergencycontacttest.PatientID, Name: "Chidi", Relationship: clinical.ContactRelationshipSibling,
		Phone: "+1", IsPrimary: true,
	})
	require.NoError(t, err)
	assert.True(t, first.IsPrimary)
	assert.Empty(t, first.DisplacedID)

	second, err := svc.Create(t.Context(), owner(), clinical.EmergencyContact{
		PatientID: emergencycontacttest.PatientID, Name: "Boris", Relationship: clinical.ContactRelationshipFriend,
		Phone: "+2", IsPrimary: true,
	})
	require.NoError(t, err)
	assert.True(t, second.IsPrimary)
	assert.Equal(t, first.ID, second.DisplacedID)

	reread, err := svc.Get(t.Context(), owner(), first.ID)
	require.NoError(t, err)
	assert.False(t, reread.IsPrimary)
}

func TestServiceGetUpdateDelete(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), clinical.EmergencyContact{
		PatientID: emergencycontacttest.PatientID, Name: "Chidi", Relationship: clinical.ContactRelationshipSibling, Phone: "+1",
	})
	require.NoError(t, err)

	found, err := svc.Get(t.Context(), owner(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Name, found.Name)

	stranger := access.Actor{UserID: emergencycontacttest.StrangerID, RequestID: "req-2"}
	_, err = svc.Get(t.Context(), stranger, created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	phone := "+44 20 7946 0000"
	updated, err := svc.Update(t.Context(), owner(), created.ID, created.Version, emergencycontact.Patch{Phone: &phone})
	require.NoError(t, err)
	assert.Equal(t, phone, updated.Phone)

	require.NoError(t, svc.Delete(t.Context(), owner(), created.ID, updated.Version))

	_, err = svc.Get(t.Context(), owner(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
