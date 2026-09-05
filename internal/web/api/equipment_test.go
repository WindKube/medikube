package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/records"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// TestEquipmentCreateRejectsAnUnknownMember is T123's equipment half.
func TestEquipmentCreateRejectsAnUnknownMember(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(&api.EquipmentCreate{
		Patient: "pat0000000000001", Name: "CPAP machine", Type: "cpap",
	})
	require.NoError(t, err)

	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(valid, &members))

	members["zzz_a_field_no_dto_declares"] = json.RawMessage(`"anything"`)

	tainted, err := json.Marshal(members)
	require.NoError(t, err)

	target := new(api.EquipmentCreate)
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

// TestEquipmentDraftAndDetailRoundTripTheServiceDates proves the three
// optional dates (prescribed, serviced, service-due) survive Draft then
// Detail unchanged — FR-048/FR-049's own fields, not a nested object.
func TestEquipmentDraftAndDetailRoundTripTheServiceDates(t *testing.T) {
	t.Parallel()

	prescribedOn := "2024-01-01"
	servicedOn := "2024-06-01"
	serviceDueOn := "2025-06-01"

	entity, err := api.EquipmentCodec{}.Draft(&api.EquipmentCreate{
		Patient:      "pat0000000000001",
		Name:         "CPAP machine",
		Type:         "cpap",
		PrescribedOn: &prescribedOn,
		ServicedOn:   &servicedOn,
		ServiceDueOn: &serviceDueOn,
	})
	require.NoError(t, err)

	detail, ok := api.EquipmentCodec{}.Detail(entity).(*api.Equipment)
	require.True(t, ok)
	require.NotNil(t, detail.PrescribedOn)
	assert.Equal(t, prescribedOn, *detail.PrescribedOn)
	require.NotNil(t, detail.ServicedOn)
	assert.Equal(t, servicedOn, *detail.ServicedOn)
	require.NotNil(t, detail.ServiceDueOn)
	assert.Equal(t, serviceDueOn, *detail.ServiceDueOn)
}

// TestEquipmentBasisReadsBackTheSummarysField is FR-049's basis mechanism at
// the DTO boundary.
func TestEquipmentBasisReadsBackTheSummarysField(t *testing.T) {
	t.Parallel()

	summary, ok := api.EquipmentCodec{}.Summary(clinical.Equipment{ID: "eqp1"}, []string{"overdue"}).(*api.EquipmentSummary)
	require.True(t, ok)
	assert.Equal(t, []string{"overdue"}, summary.Basis)
	assert.Equal(t, []string{"overdue"}, api.EquipmentBasis(summary, records.Criteria{}))
}
