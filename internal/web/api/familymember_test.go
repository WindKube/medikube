package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// TestFamilyMemberCreateRejectsAnUnknownMember is T193's unknown-member half.
func TestFamilyMemberCreateRejectsAnUnknownMember(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(&api.FamilyMemberCreate{
		Patient: "pat0000000000001", Name: "Nadia Okonkwo", Relationship: "mother",
	})
	require.NoError(t, err)

	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(valid, &members))

	members["zzz_a_field_no_dto_declares"] = json.RawMessage(`"anything"`)

	tainted, err := json.Marshal(members)
	require.NoError(t, err)

	target := new(api.FamilyMemberCreate)
	decodeErr := web.DecodeBytes(tainted, target)
	require.Error(t, decodeErr)

	var invalid *domain.ValidationError
	require.ErrorAs(t, decodeErr, &invalid)

	var sawUnknownField bool
	for _, field := range invalid.Fields {
		if field.Code == domain.CodeUnknownField {
			sawUnknownField = true
		}
	}
	assert.True(t, sawUnknownField)
}

// TestFamilyMemberConditionsMarshalsAsAnEmptyArrayNeverNull is FR-053's wire
// contract: a relative with no conditions still returns `"conditions":[]`.
func TestFamilyMemberConditionsMarshalsAsAnEmptyArrayNeverNull(t *testing.T) {
	t.Parallel()

	detail, ok := api.FamilyMemberCodec{}.Detail(clinical.FamilyMember{ID: "fam1", Name: "Nadia"}).(*api.FamilyMember)
	require.True(t, ok)

	encoded, err := json.Marshal(detail)
	require.NoError(t, err)

	assert.Contains(t, string(encoded), `"conditions":[]`)
	assert.NotContains(t, string(encoded), `"conditions":null`)
}

// TestFamilyMemberConditionsRejectsAnUnknownMember proves a condition entry
// with an unrecognised member is refused the same way the outer body is.
func TestFamilyMemberConditionsRejectsAnUnknownMember(t *testing.T) {
	t.Parallel()

	tainted := []byte(`{"patient":"pat0000000000001","name":"Nadia","relationship":"mother",` +
		`"conditions":[{"name":"Breast cancer","zzz_unknown":"anything"}]}`)

	target := new(api.FamilyMemberCreate)
	decodeErr := web.DecodeBytes(tainted, target)
	require.Error(t, decodeErr)

	var invalid *domain.ValidationError
	require.ErrorAs(t, decodeErr, &invalid)
}

// TestFamilyMemberDraftAndDetailRoundTripConditions proves conditions survive
// Draft then Detail unchanged.
func TestFamilyMemberDraftAndDetailRoundTripConditions(t *testing.T) {
	t.Parallel()

	age := 62

	entity, err := api.FamilyMemberCodec{}.Draft(&api.FamilyMemberCreate{
		Patient: "pat0000000000001", Name: "Nadia Okonkwo", Relationship: "grandmother",
		Conditions: []api.FamilyCondition{
			{Name: "Breast cancer", DiagnosedAge: &age, Severity: "severe", Status: "resolved"},
		},
	})
	require.NoError(t, err)

	detail, ok := api.FamilyMemberCodec{}.Detail(entity).(*api.FamilyMember)
	require.True(t, ok)
	require.Len(t, detail.Conditions, 1)
	assert.Equal(t, "Breast cancer", detail.Conditions[0].Name)
	require.NotNil(t, detail.Conditions[0].DiagnosedAge)
	assert.Equal(t, 62, *detail.Conditions[0].DiagnosedAge)
}
