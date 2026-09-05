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

// T106. Mirrors immunization_test.go: Codec.Draft/Patch/Summary/Detail as
// unit tests against api.InjuryCodec, with the laterality, type, severity
// and status vocabularies (clinical.Laterality, clinical.InjuryType,
// clinical.Severity, clinical.ConditionStatus) round-tripped through the
// wire, and MedicationIDs carried under its wire member medication_ids.

func TestInjuryCodecDraft(t *testing.T) {
	t.Parallel()

	t.Run("a minimal create decodes", func(t *testing.T) {
		t.Parallel()

		occurred := "2025-08-20"
		entity, err := api.InjuryCodec{}.Draft(&api.InjuryCreate{
			Patient:    "patient-1",
			Name:       "Sprained ankle",
			OccurredOn: &occurred,
		})
		require.NoError(t, err)

		assert.Equal(t, "patient-1", entity.PatientID)
		assert.Equal(t, "Sprained ankle", entity.Name)
		assert.False(t, entity.OccurredOn.IsZero())
	})

	t.Run("a malformed date is refused and named", func(t *testing.T) {
		t.Parallel()

		occurred := "not-a-date"
		_, err := api.InjuryCodec{}.Draft(&api.InjuryCreate{
			Patient:    "patient-1",
			Name:       "Sprained ankle",
			OccurredOn: &occurred,
		})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, api.InjuryMemberOccurredOn, invalid.Fields[0].Field)
	})

	t.Run("the wrong body type is a wiring failure", func(t *testing.T) {
		t.Parallel()

		_, err := api.InjuryCodec{}.Draft(&api.MedicationCreate{})
		assert.ErrorIs(t, err, api.ErrWrongBodyType)
	})

	t.Run("laterality, type, severity and status carry through", func(t *testing.T) {
		t.Parallel()

		entity, err := api.InjuryCodec{}.Draft(&api.InjuryCreate{
			Patient:     "patient-1",
			Name:        "Sprained ankle",
			Type:        string(clinical.InjuryTypeSprain),
			Laterality:  string(clinical.LateralityRight),
			Severity:    string(clinical.SeverityModerate),
			Status:      string(clinical.ConditionStatusHealing),
			Medications: []string{"med-1", "med-2"},
		})
		require.NoError(t, err)

		assert.Equal(t, clinical.InjuryTypeSprain, entity.Type)
		assert.Equal(t, clinical.LateralityRight, entity.Laterality)
		assert.Equal(t, clinical.SeverityModerate, entity.Severity)
		assert.Equal(t, clinical.ConditionStatusHealing, entity.Status)
		assert.Equal(t, []string{"med-1", "med-2"}, entity.MedicationIDs)
	})
}

func TestInjuryCodecPatch(t *testing.T) {
	t.Parallel()

	t.Run("an absent member changes nothing", func(t *testing.T) {
		t.Parallel()

		patch, err := api.InjuryCodec{}.Patch(&api.InjuryPatch{})
		require.NoError(t, err)

		assert.Nil(t, patch.Name)
		assert.Nil(t, patch.Laterality)
		assert.Nil(t, patch.OccurredOn)
		assert.Nil(t, patch.MedicationIDs)
	})

	t.Run("a supplied laterality sets it", func(t *testing.T) {
		t.Parallel()

		laterality := string(clinical.LateralityBilateral)
		patch, err := api.InjuryCodec{}.Patch(&api.InjuryPatch{Laterality: &laterality})
		require.NoError(t, err)

		require.NotNil(t, patch.Laterality)
		assert.Equal(t, clinical.LateralityBilateral, *patch.Laterality)
	})

	t.Run("an explicit null date clears it", func(t *testing.T) {
		t.Parallel()

		patch, err := api.InjuryCodec{}.Patch(&api.InjuryPatch{
			OccurredOn: web.Cleared[string](),
		})
		require.NoError(t, err)

		require.NotNil(t, patch.OccurredOn)
		assert.True(t, patch.OccurredOn.IsZero())
	})

	t.Run("a non-nil medication list replaces the set", func(t *testing.T) {
		t.Parallel()

		patch, err := api.InjuryCodec{}.Patch(&api.InjuryPatch{Medications: []string{"med-1"}})
		require.NoError(t, err)

		require.NotNil(t, patch.MedicationIDs)
		assert.Equal(t, []string{"med-1"}, *patch.MedicationIDs)
	})

	t.Run("the wrong body type is a wiring failure", func(t *testing.T) {
		t.Parallel()

		_, err := api.InjuryCodec{}.Patch(&api.MedicationPatch{})
		assert.ErrorIs(t, err, api.ErrWrongBodyType)
	})
}

func TestInjuryCodecSummaryAndDetail(t *testing.T) {
	t.Parallel()

	occurred, err := domain.NewDate(2025, 8, 20)
	require.NoError(t, err)

	entity := clinical.Injury{
		ID:             "inj-1",
		PatientID:      "patient-1",
		Name:           "Sprained ankle",
		Type:           clinical.InjuryTypeSprain,
		BodyPart:       "ankle",
		Laterality:     clinical.LateralityRight,
		OccurredOn:     occurred,
		Mechanism:      "fell while running",
		Severity:       clinical.SeverityModerate,
		Status:         clinical.ConditionStatusHealing,
		RecoveryNotes:  "still icing it",
		MedicationIDs:  []string{"med-1"},
		PractitionerID: "prac-1",
		Version:        "v1",
	}

	summary, ok := api.InjuryCodec{}.Summary(entity).(*api.InjurySummary)
	require.True(t, ok)
	assert.Equal(t, "inj-1", summary.ID)
	assert.Equal(t, "Sprained ankle", summary.Name)
	assert.Equal(t, string(clinical.InjuryTypeSprain), summary.Type)
	assert.Equal(t, string(clinical.SeverityModerate), summary.Severity)
	assert.Equal(t, string(clinical.ConditionStatusHealing), summary.Status)
	require.NotNil(t, summary.OccurredOn)
	assert.Equal(t, "2025-08-20", *summary.OccurredOn)

	detail, ok := api.InjuryCodec{}.Detail(entity).(*api.Injury)
	require.True(t, ok)
	assert.Equal(t, "patient-1", detail.Patient)
	assert.Equal(t, "ankle", detail.BodyPart)
	assert.Equal(t, string(clinical.LateralityRight), detail.Laterality)
	assert.Equal(t, "fell while running", detail.Mechanism)
	assert.Equal(t, "still icing it", detail.RecoveryNotes)
	assert.Equal(t, []string{"med-1"}, detail.Medications)
	assert.Equal(t, "prac-1", detail.Practitioner)
}

func TestInjurySearchFields(t *testing.T) {
	t.Parallel()

	title, text := api.InjurySearchFields(&api.Injury{
		InjurySummary: api.InjurySummary{Name: "Sprained ankle"},
		BodyPart:      "ankle",
		Mechanism:     "fell while running",
		RecoveryNotes: "still icing it",
	})

	assert.Equal(t, "Sprained ankle", title)
	assert.Equal(t, "ankle fell while running still icing it", text)

	title, text = api.InjurySearchFields(&api.MedicationSummary{})
	assert.Empty(t, title)
	assert.Empty(t, text)
}

func TestInjuryBasisNarrowsOnlyOnUnresolved(t *testing.T) {
	t.Parallel()

	assert.Nil(t, api.InjuryBasis(&api.InjurySummary{Status: string(clinical.ConditionStatusActive)}, records.Criteria{}))

	narrowed := records.Criteria{Filters: map[string][]string{"unresolved": {"true"}}}

	assert.Nil(t, api.InjuryBasis(&api.MedicationSummary{}, narrowed))
	assert.Equal(t,
		[]string{string(clinical.ConditionStatusActive)},
		api.InjuryBasis(&api.InjurySummary{Status: string(clinical.ConditionStatusActive)}, narrowed),
	)
}
