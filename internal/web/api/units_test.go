package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/identity"
)

func TestWeightRoundTrip(t *testing.T) {
	t.Parallel()

	kg := 70.0

	displayed := weightToDisplay(&kg, identity.UnitSystemImperial)
	require.NotNil(t, displayed)
	assert.NotEqual(t, kg, *displayed, "an imperial viewer sees pounds, not kilograms")

	backToSI := weightToSI(displayed, identity.UnitSystemImperial)
	require.NotNil(t, backToSI)
	assert.InDelta(t, kg, *backToSI, 0.0001, "the round trip changes nothing recorded (US3-6)")
}

func TestMetricViewerSeesTheStoredValueUnchanged(t *testing.T) {
	t.Parallel()

	kg := 70.0

	displayed := weightToDisplay(&kg, identity.UnitSystemMetric)
	require.NotNil(t, displayed)
	assert.Equal(t, kg, *displayed)
}

func TestNilMeasurementStaysNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, weightToDisplay(nil, identity.UnitSystemImperial))
	assert.Nil(t, heightToSI(nil, identity.UnitSystemImperial))
	assert.Nil(t, temperatureToDisplay(nil, identity.UnitSystemImperial))
	assert.Nil(t, glucoseToSI(nil, identity.UnitSystemImperial))
}

func TestTwoViewersSeeTheSameReadingInTheirOwnUnits(t *testing.T) {
	t.Parallel()

	celsius := 37.0

	metricView := temperatureToDisplay(&celsius, identity.UnitSystemMetric)
	imperialView := temperatureToDisplay(&celsius, identity.UnitSystemImperial)

	require.NotNil(t, metricView)
	require.NotNil(t, imperialView)
	assert.InDelta(t, celsius, *metricView, 0.0001)
	assert.NotEqual(t, *metricView, *imperialView)

	// Neither view mutates the stored reading.
	assert.InDelta(t, 37.0, celsius, 0.0001)
}
