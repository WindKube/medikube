package api_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/records"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

func TestImmunizationCodecDraft(t *testing.T) {
	t.Parallel()

	t.Run("a minimal create decodes", func(t *testing.T) {
		t.Parallel()

		administered := "2024-01-01"
		entity, err := api.ImmunizationCodec{}.Draft(&api.ImmunizationCreate{
			Patient:        "patient-1",
			VaccineName:    "Influenza",
			AdministeredOn: &administered,
		})
		require.NoError(t, err)

		assert.Equal(t, "patient-1", entity.PatientID)
		assert.Equal(t, "Influenza", entity.VaccineName)
		assert.False(t, entity.AdministeredOn.IsZero())
	})

	t.Run("a malformed date is refused and named", func(t *testing.T) {
		t.Parallel()

		administered := "not-a-date"
		_, err := api.ImmunizationCodec{}.Draft(&api.ImmunizationCreate{
			Patient:        "patient-1",
			VaccineName:    "Influenza",
			AdministeredOn: &administered,
		})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, api.ImmunizationMemberAdministeredOn, invalid.Fields[0].Field)
	})

	t.Run("the wrong body type is a wiring failure", func(t *testing.T) {
		t.Parallel()

		_, err := api.ImmunizationCodec{}.Draft(&api.MedicationCreate{})
		assert.ErrorIs(t, err, api.ErrWrongBodyType)
	})

	t.Run("a dose number carries through", func(t *testing.T) {
		t.Parallel()

		administered := "2024-01-01"
		dose := 2
		entity, err := api.ImmunizationCodec{}.Draft(&api.ImmunizationCreate{
			Patient:        "patient-1",
			VaccineName:    "Influenza",
			AdministeredOn: &administered,
			DoseNumber:     &dose,
		})
		require.NoError(t, err)
		require.NotNil(t, entity.DoseNumber)
		assert.Equal(t, 2, *entity.DoseNumber)
	})
}

func TestImmunizationCodecPatch(t *testing.T) {
	t.Parallel()

	t.Run("an absent member changes nothing", func(t *testing.T) {
		t.Parallel()

		patch, err := api.ImmunizationCodec{}.Patch(&api.ImmunizationPatch{})
		require.NoError(t, err)

		assert.Nil(t, patch.VaccineName)
		assert.Nil(t, patch.AdministeredOn)
		assert.Nil(t, patch.DoseNumber)
	})

	t.Run("a supplied dose number sets it", func(t *testing.T) {
		t.Parallel()

		dose := 3
		patch, err := api.ImmunizationCodec{}.Patch(&api.ImmunizationPatch{DoseNumber: &dose})
		require.NoError(t, err)

		require.NotNil(t, patch.DoseNumber)
		require.NotNil(t, *patch.DoseNumber)
		assert.Equal(t, 3, **patch.DoseNumber)
	})

	t.Run("an explicit null date clears it", func(t *testing.T) {
		t.Parallel()

		patch, err := api.ImmunizationCodec{}.Patch(&api.ImmunizationPatch{
			ExpiresOn: web.Cleared[string](),
		})
		require.NoError(t, err)

		require.NotNil(t, patch.ExpiresOn)
		assert.True(t, patch.ExpiresOn.IsZero())
	})

	t.Run("the wrong body type is a wiring failure", func(t *testing.T) {
		t.Parallel()

		_, err := api.ImmunizationCodec{}.Patch(&api.MedicationPatch{})
		assert.ErrorIs(t, err, api.ErrWrongBodyType)
	})

}

func TestImmunizationCodecSummaryAndDetail(t *testing.T) {
	t.Parallel()

	administered, err := domain.NewDate(2024, 1, 1)
	require.NoError(t, err)

	dose := 1

	entity := clinical.Immunization{
		ID:             "imm-1",
		PatientID:      "patient-1",
		VaccineName:    "Influenza",
		TradeName:      "Fluzone",
		AdministeredOn: administered,
		DoseNumber:     &dose,
		LotNumber:      "AB123",
		Manufacturer:   "Sanofi",
		Site:           clinical.ImmunizationSiteLeftArm,
		Route:          clinical.ImmunizationRouteIntramuscular,
		Version:        "v1",
	}

	summary, ok := api.ImmunizationCodec{}.Summary(entity).(*api.ImmunizationSummary)
	require.True(t, ok)
	assert.Equal(t, "imm-1", summary.ID)
	assert.Equal(t, "Influenza", summary.VaccineName)
	require.NotNil(t, summary.AdministeredOn)
	assert.Equal(t, "2024-01-01", *summary.AdministeredOn)
	require.NotNil(t, summary.DoseNumber)
	assert.Equal(t, 1, *summary.DoseNumber)

	detail, ok := api.ImmunizationCodec{}.Detail(entity).(*api.Immunization)
	require.True(t, ok)
	assert.Equal(t, "patient-1", detail.Patient)
	assert.Equal(t, "Fluzone", detail.TradeName)
	assert.Equal(t, "AB123", detail.LotNumber)
	assert.Equal(t, string(clinical.ImmunizationSiteLeftArm), detail.Site)
}

func TestImmunizationSearchFields(t *testing.T) {
	t.Parallel()

	title, text := api.ImmunizationSearchFields(&api.Immunization{
		ImmunizationSummary: api.ImmunizationSummary{VaccineName: "Influenza"},
		TradeName:           "Fluzone",
		Manufacturer:        "Sanofi",
		LotNumber:           "AB123",
	})

	assert.Equal(t, "Influenza", title)
	assert.Equal(t, "Fluzone Sanofi AB123", text)

	title, text = api.ImmunizationSearchFields(&api.MedicationSummary{})
	assert.Empty(t, title)
	assert.Empty(t, text)
}

func TestImmunizationBasisNarrowsNothing(t *testing.T) {
	t.Parallel()

	assert.Nil(t, api.ImmunizationBasis(nil, records.Criteria{}))
}
