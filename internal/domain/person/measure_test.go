package person

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain/identity"
)

func TestFormatHeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cm     float64
		system identity.UnitSystem
		want   string
	}{
		{"metric whole", 175, identity.UnitSystemMetric, "175 cm"},
		{"metric fractional", 175.5, identity.UnitSystemMetric, "175.5 cm"},
		{"imperial example from research D-21", 175, identity.UnitSystemImperial, "5 ft 9 in"},
		{"unset is empty", 0, identity.UnitSystemMetric, ""},
		{"unset is empty imperial", 0, identity.UnitSystemImperial, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, FormatHeight(tt.cm, tt.system))
		})
	}
}

func TestFormatWeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		kg     float64
		system identity.UnitSystem
		want   string
	}{
		{"metric whole", 70, identity.UnitSystemMetric, "70 kg"},
		{"metric fractional", 70.5, identity.UnitSystemMetric, "70.5 kg"},
		{"imperial example from research D-21", 70.5, identity.UnitSystemImperial, "155 lb 7 oz"},
		{"unset is empty", 0, identity.UnitSystemMetric, ""},
		{"unset is empty imperial", 0, identity.UnitSystemImperial, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, FormatWeight(tt.kg, tt.system))
		})
	}
}

// Conversion must never mutate the canonical SI value it was given — changing
// the display preference must never alter what was recorded (FR-007).
func TestFormatHeightAndWeightDoNotMutateSI(t *testing.T) {
	t.Parallel()

	heightCM := 175.0
	weightKG := 70.5

	_ = FormatHeight(heightCM, identity.UnitSystemImperial)
	_ = FormatWeight(weightKG, identity.UnitSystemImperial)

	assert.Equal(t, 175.0, heightCM)
	assert.Equal(t, 70.5, weightKG)

	// Formatting twice in different systems from the same canonical value
	// gives consistent, independent results.
	assert.Equal(t, "175 cm", FormatHeight(heightCM, identity.UnitSystemMetric))
	assert.Equal(t, "5 ft 9 in", FormatHeight(heightCM, identity.UnitSystemImperial))
}
