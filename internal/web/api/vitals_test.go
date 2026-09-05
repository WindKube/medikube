package api

import (
	json "encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
)

func TestVitalsDraftConvertsImperialInputToSI(t *testing.T) {
	t.Parallel()

	weightLb := 154.0 // ~70kg

	draft, err := VitalsCodec{}.Draft(&VitalsCreate{
		Patient:    "pat1",
		RecordedAt: "2026-01-02T09:00:00Z",
		WeightKg:   &weightLb,
	}, identity.UnitSystemImperial)
	require.NoError(t, err)

	require.NotNil(t, draft.WeightKg)
	assert.InDelta(t, 69.85, *draft.WeightKg, 0.5)
}

func TestVitalsSummaryRendersInTheActorsOwnUnitSystem(t *testing.T) {
	t.Parallel()

	weightKg := 70.0

	v := clinical.Vitals{WeightKg: &weightKg}

	metric, ok := VitalsCodec{}.Summary(v, identity.UnitSystemMetric).(*VitalsSummary)
	require.True(t, ok)
	require.NotNil(t, metric.WeightKg)
	assert.InDelta(t, 70.0, *metric.WeightKg, 0.0001)

	imperial, ok := VitalsCodec{}.Summary(v, identity.UnitSystemImperial).(*VitalsSummary)
	require.True(t, ok)
	require.NotNil(t, imperial.WeightKg)
	assert.NotEqual(t, *metric.WeightKg, *imperial.WeightKg)
}

func TestVitalsBMIRendersOnlyWhenBothHeightAndWeightArePresent(t *testing.T) {
	t.Parallel()

	weight := 70.0
	height := 175.0

	withBoth, ok := VitalsCodec{}.Summary(clinical.Vitals{WeightKg: &weight, HeightCm: &height}, identity.UnitSystemMetric).(*VitalsSummary)
	require.True(t, ok)
	require.NotNil(t, withBoth.Bmi)

	withOne, ok := VitalsCodec{}.Summary(clinical.Vitals{WeightKg: &weight}, identity.UnitSystemMetric).(*VitalsSummary)
	require.True(t, ok)
	assert.Nil(t, withOne.Bmi)
}

func TestVitalsPatchRoundTripsImperialThroughSI(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"weight_kg": 154}`)

	var incoming VitalsPatch
	require.NoError(t, json.Unmarshal(raw, &incoming))

	patch, err := VitalsCodec{}.Patch(&incoming, identity.UnitSystemImperial)
	require.NoError(t, err)
	require.NotNil(t, patch.WeightKg)
	require.NotNil(t, *patch.WeightKg)
	assert.InDelta(t, 69.85, **patch.WeightKg, 0.5)
}

func TestVitalsCodecRefusesTheWrongBodyType(t *testing.T) {
	t.Parallel()

	_, err := VitalsCodec{}.Draft(&struct{}{}, identity.UnitSystemMetric)
	require.ErrorIs(t, err, ErrWrongBodyType)

	_, err = VitalsCodec{}.Patch(&struct{}{}, identity.UnitSystemMetric)
	require.ErrorIs(t, err, ErrWrongBodyType)
}
