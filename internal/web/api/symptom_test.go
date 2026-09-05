package api

import (
	json "encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
)

func TestSymptomTriggersMarshalAsEmptyArrayNeverNull(t *testing.T) {
	t.Parallel()

	detail := SymptomCodec{}.Detail(clinical.Symptom{
		Name:       "Dizziness",
		Severity:   clinical.SeverityModerate,
		OccurredAt: clinical.Now(),
	})

	raw, err := json.Marshal(detail)
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"triggers":[]`)
	assert.Contains(t, string(raw), `"relief_methods":[]`)
}

func TestSymptomDraftRoundTrip(t *testing.T) {
	t.Parallel()

	create := &SymptomCreate{
		Patient:       "pat1",
		Name:          "Dizziness",
		Severity:      string(clinical.SeverityModerate),
		OccurredAt:    "2026-01-02T09:00:00Z",
		Triggers:      []string{"standing up"},
		ReliefMethods: []string{"sitting down"},
	}

	draft, err := SymptomCodec{}.Draft(create)
	require.NoError(t, err)

	assert.Equal(t, "pat1", draft.PatientID)
	assert.Equal(t, "Dizziness", draft.Name)
	assert.Equal(t, clinical.SeverityModerate, draft.Severity)
	assert.Equal(t, "2026-01-02T09:00:00Z", draft.OccurredAt.String())
	assert.Equal(t, []string{"standing up"}, draft.Triggers)
}

func TestSymptomDraftRejectsAMalformedInstant(t *testing.T) {
	t.Parallel()

	_, err := SymptomCodec{}.Draft(&SymptomCreate{
		Patient: "pat1", Name: "Dizziness", Severity: string(clinical.SeverityModerate),
		OccurredAt: "not-a-date",
	})
	require.Error(t, err)
}

func TestSymptomPatchDistinguishesAbsentFromClear(t *testing.T) {
	t.Parallel()

	// Absent: no member sent.
	patch, err := SymptomCodec{}.Patch(&SymptomPatch{})
	require.NoError(t, err)
	assert.Nil(t, patch.DurationMinutes)

	// Present with a value.
	raw := []byte(`{"duration_minutes": 30}`)
	var withValue SymptomPatch
	require.NoError(t, json.Unmarshal(raw, &withValue))

	patch, err = SymptomCodec{}.Patch(&withValue)
	require.NoError(t, err)
	require.NotNil(t, patch.DurationMinutes)
	require.NotNil(t, *patch.DurationMinutes)
	assert.Equal(t, 30, **patch.DurationMinutes)

	// Present and explicit null: the clear.
	raw = []byte(`{"duration_minutes": null}`)
	var cleared SymptomPatch
	require.NoError(t, json.Unmarshal(raw, &cleared))

	patch, err = SymptomCodec{}.Patch(&cleared)
	require.NoError(t, err)
	require.NotNil(t, patch.DurationMinutes)
	assert.Nil(t, *patch.DurationMinutes)
}

func TestSymptomCodecRefusesTheWrongBodyType(t *testing.T) {
	t.Parallel()

	_, err := SymptomCodec{}.Draft(&struct{}{})
	require.ErrorIs(t, err, ErrWrongBodyType)

	_, err = SymptomCodec{}.Patch(&struct{}{})
	require.ErrorIs(t, err, ErrWrongBodyType)
}
