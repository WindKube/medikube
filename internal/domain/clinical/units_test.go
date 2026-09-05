package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const tolerance = 0.01

func within(t *testing.T, want, got float64) {
	t.Helper()
	assert.InDeltaf(t, want, got, tolerance, "want %v got %v", want, got)
}

func TestWeightRoundTrips(t *testing.T) {
	t.Parallel()

	kg := 70.0
	within(t, kg, LbToKg(KgToLb(kg)))
	assert.InDelta(t, 154.32, KgToLb(kg), 0.1)
}

func TestHeightRoundTrips(t *testing.T) {
	t.Parallel()

	cm := 180.0
	within(t, cm, InToCm(CmToIn(cm)))
	assert.InDelta(t, 70.87, CmToIn(cm), 0.1)
}

func TestTemperatureRoundTrips(t *testing.T) {
	t.Parallel()

	c := 37.0
	within(t, c, FahrenheitToCelsius(CelsiusToFahrenheit(c)))
	assert.InDelta(t, 98.6, CelsiusToFahrenheit(c), 0.1)
}

func TestGlucoseRoundTrips(t *testing.T) {
	t.Parallel()

	mmol := 5.5
	within(t, mmol, MgDlToMmolL(MmolLToMgDl(mmol)))
}

func TestBMIDerivation(t *testing.T) {
	t.Parallel()

	got := BMI(70, 180)
	want := 70 / (1.8 * 1.8)
	assert.InDelta(t, want, got, 0.001)

	assert.Zero(t, BMI(70, 0), "no height on file, no BMI to derive")
}
