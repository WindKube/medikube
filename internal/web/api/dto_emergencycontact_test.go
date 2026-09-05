package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/web/api"
)

func TestEmergencyContactCodecRoundTrip(t *testing.T) {
	t.Parallel()

	entity := clinical.EmergencyContact{
		ID: "cnt1", PatientID: "pat1", Name: "Chidi Eze",
		Relationship: clinical.ContactRelationshipSibling, Phone: "+1 555 0100",
		IsActive: true,
	}

	codec := api.EmergencyContactCodec{}

	detail, ok := codec.Detail(entity).(*api.EmergencyContact)
	require.True(t, ok)
	assert.Nil(t, detail.Displaced)
	assert.NotNil(t, detail.Tags)
	assert.Empty(t, detail.Tags)
}

func TestEmergencyContactCodecSurfacesDisplacement(t *testing.T) {
	t.Parallel()

	entity := clinical.EmergencyContact{
		ID: "cnt2", PatientID: "pat1", Name: "Boris", Phone: "+2",
		Relationship: clinical.ContactRelationshipFriend, IsPrimary: true, IsActive: true,
		DisplacedID: "cnt1",
	}

	codec := api.EmergencyContactCodec{}

	detail, ok := codec.Detail(entity).(*api.EmergencyContact)
	require.True(t, ok)
	require.NotNil(t, detail.Displaced)
	assert.Equal(t, "cnt1", detail.Displaced.ID)
	assert.Equal(t, "emergency_contact", detail.Displaced.Kind)
}

func TestEmergencyContactCreateDefaultsIsActiveToTrue(t *testing.T) {
	t.Parallel()

	codec := api.EmergencyContactCodec{}

	draft, err := codec.Draft(&api.EmergencyContactCreate{
		Patient: "pat1", Name: "Chidi", Relationship: "sibling", Phone: "+1",
	})
	require.NoError(t, err)
	assert.True(t, draft.IsActive)
}

func TestEmergencyContactCreateAndPatchCarryNoServerOwnedMembers(t *testing.T) {
	t.Parallel()

	create, err := json.Marshal(api.EmergencyContactCreate{})
	require.NoError(t, err)

	var createFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(create, &createFields))

	for _, forbidden := range []string{"id", "created_at", "updated_at", "version"} {
		_, present := createFields[forbidden]
		assert.False(t, present, "EmergencyContactCreate carries %s", forbidden)
	}

	patch, err := json.Marshal(api.EmergencyContactPatch{})
	require.NoError(t, err)

	var patchFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(patch, &patchFields))

	for _, forbidden := range []string{"id", "patient", "created_at", "updated_at", "version"} {
		_, present := patchFields[forbidden]
		assert.False(t, present, "EmergencyContactPatch carries %s", forbidden)
	}
}
