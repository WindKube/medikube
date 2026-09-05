package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/web/api"
)

func TestConditionCodecRoundTrip(t *testing.T) {
	t.Parallel()

	onset, err := domain.NewDate(2020, 3, 1)
	require.NoError(t, err)

	entity := clinical.Condition{
		ID: "cnd1", PatientID: "pat1", Diagnosis: "Type 2 diabetes",
		Status: clinical.ConditionStatusActive, OnsetOn: onset,
	}

	codec := api.ConditionCodec{}

	detail, ok := codec.Detail(entity).(*api.Condition)
	require.True(t, ok)
	require.NotNil(t, detail.OnsetOn)
	assert.Equal(t, "2020-03-01", *detail.OnsetOn)
	assert.Nil(t, detail.ResolvedOn)
	assert.NotNil(t, detail.Tags)
	assert.Empty(t, detail.Tags)
}

func TestConditionCreateAndPatchCarryNoServerOwnedMembers(t *testing.T) {
	t.Parallel()

	create, err := json.Marshal(api.ConditionCreate{})
	require.NoError(t, err)

	var createFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(create, &createFields))

	for _, forbidden := range []string{"id", "created_at", "updated_at", "version"} {
		_, present := createFields[forbidden]
		assert.False(t, present, "ConditionCreate carries %s", forbidden)
	}

	patch, err := json.Marshal(api.ConditionPatch{})
	require.NoError(t, err)

	var patchFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(patch, &patchFields))

	for _, forbidden := range []string{"id", "patient", "created_at", "updated_at", "version"} {
		_, present := patchFields[forbidden]
		assert.False(t, present, "ConditionPatch carries %s", forbidden)
	}
}
