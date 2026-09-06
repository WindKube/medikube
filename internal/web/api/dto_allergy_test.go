package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/records"
	"medikube/internal/web/api"
)

func TestAllergyCodecRoundTrip(t *testing.T) {
	t.Parallel()

	onset, err := domain.NewDate(2024, 1, 15)
	require.NoError(t, err)

	entity := clinical.Allergy{
		ID: "alg1", PatientID: "pat1", Allergen: "Penicillin", Reaction: "hives",
		Severity: clinical.SeverityModerate, Status: clinical.ConditionStatusActive,
		OnsetOn: onset,
	}

	codec := api.AllergyCodec{}

	detail, ok := codec.Detail(entity).(*api.Allergy)
	require.True(t, ok)
	require.NotNil(t, detail.OnsetOn)
	assert.Equal(t, "2024-01-15", *detail.OnsetOn)
	assert.NotNil(t, detail.Tags)
	assert.Empty(t, detail.Tags)

	payload, err := json.Marshal(detail)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), `"tags":null`)
}

func TestAllergyCreateAndPatchCarryNoServerOwnedMembers(t *testing.T) {
	t.Parallel()

	create, err := json.Marshal(api.AllergyCreate{})
	require.NoError(t, err)

	var createFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(create, &createFields))

	for _, forbidden := range []string{"id", "created_at", "updated_at", "version"} {
		_, present := createFields[forbidden]
		assert.False(t, present, "AllergyCreate carries %s", forbidden)
	}

	patch, err := json.Marshal(api.AllergyPatch{})
	require.NoError(t, err)

	var patchFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(patch, &patchFields))

	for _, forbidden := range []string{"id", "patient", "created_at", "updated_at", "version"} {
		_, present := patchFields[forbidden]
		assert.False(t, present, "AllergyPatch carries %s", forbidden)
	}
}

func TestAllergyBasisNarrowsToCritical(t *testing.T) {
	t.Parallel()

	critical := &api.AllergySummary{Critical: true}
	assert.Equal(t, []string{"critical"}, api.AllergyBasis(critical, records.Criteria{}))

	notCritical := &api.AllergySummary{Critical: false}
	assert.Nil(t, api.AllergyBasis(notCritical, records.Criteria{}))
}
