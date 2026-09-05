package familymember_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/familymember"
	"medikube/internal/service/familymember/familymembertest"
)

func actor() access.Actor {
	return access.Actor{UserID: familymembertest.OwnerID, RequestID: "req-familymember"}
}

func stranger() access.Actor {
	return access.Actor{UserID: familymembertest.StrangerID, RequestID: "req-familymember-stranger"}
}

func newService(t *testing.T) *familymember.Service {
	t.Helper()

	service, err := familymember.New(familymembertest.NewRepository(), familymembertest.NewAuthorizer(familymembertest.OwnerID))
	require.NoError(t, err)

	return service
}

func minimalRelative() clinical.FamilyMember {
	return clinical.FamilyMember{
		PatientID:    familymembertest.PatientID,
		Name:         "Nadia Okonkwo",
		Relationship: clinical.FamilyRelationshipGrandmother,
	}
}

func TestServiceNew(t *testing.T) {
	t.Parallel()

	_, err := familymember.New(nil, familymembertest.NewAuthorizer(familymembertest.OwnerID))
	assert.Error(t, err)

	_, err = familymember.New(familymembertest.NewRepository(), nil)
	assert.Error(t, err)
}

func TestCreateRequiresAPatient(t *testing.T) {
	t.Parallel()

	service := newService(t)

	_, err := service.Create(t.Context(), actor(), clinical.FamilyMember{})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, familymember.FieldPatient, invalid.Fields[0].Field)
}

func TestCreateGet(t *testing.T) {
	t.Parallel()

	service := newService(t)

	created, err := service.Create(t.Context(), actor(), minimalRelative())
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	found, err := service.Get(t.Context(), actor(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)

	_, err = service.Get(t.Context(), stranger(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUpdateAndDeleteCheckVersion(t *testing.T) {
	t.Parallel()

	service := newService(t)

	created, err := service.Create(t.Context(), actor(), minimalRelative())
	require.NoError(t, err)

	_, err = service.Update(t.Context(), actor(), created.ID, "stale", familymember.Patch{})
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)

	newName := "Amara Okonkwo Sr."
	updated, err := service.Update(t.Context(), actor(), created.ID, created.Version, familymember.Patch{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, newName, updated.Name)

	err = service.Delete(t.Context(), actor(), created.ID, "stale")
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)

	require.NoError(t, service.Delete(t.Context(), actor(), created.ID, updated.Version))

	_, err = service.Get(t.Context(), actor(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestNonOwnerIsRefused(t *testing.T) {
	t.Parallel()

	service := newService(t)

	_, err := service.Create(t.Context(), stranger(), minimalRelative())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
