package clinical_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

func f(v float64) *float64 { return &v }

func validVitals() clinical.Vitals {
	return clinical.Vitals{
		RecordedAt: clinical.NewInstant(time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)),
		WeightKg:   f(70),
	}
}

func TestVitalsValidate(t *testing.T) {
	t.Parallel()

	t.Run("a set with one measurement is accepted", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validVitals().Validate())
	})

	t.Run("recorded_at is required", func(t *testing.T) {
		t.Parallel()

		err := clinical.Vitals{WeightKg: f(70)}.Validate()
		require.Error(t, err)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "recorded_at", invalid.Fields[0].Field)
	})

	t.Run("recorded_at cannot be in the future", func(t *testing.T) {
		t.Parallel()

		v := validVitals()
		v.RecordedAt = clinical.NewInstant(time.Now().Add(24 * time.Hour))
		require.Error(t, v.Validate())
	})

	t.Run("a set with no measurement at all is refused", func(t *testing.T) {
		t.Parallel()

		v := clinical.Vitals{RecordedAt: clinical.NewInstant(time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC))}
		err := v.Validate()
		require.Error(t, err)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)

		found := false
		for _, field := range invalid.Fields {
			if field.Code == clinical.CodeAtLeastOneMeasurement {
				found = true
			}
		}
		assert.True(t, found)
	})

	t.Run("systolic without diastolic names the missing field", func(t *testing.T) {
		t.Parallel()

		v := clinical.Vitals{RecordedAt: validVitals().RecordedAt, SystolicMmHg: f(120)}
		err := v.Validate()
		require.Error(t, err)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)

		reported := make([]string, 0, len(invalid.Fields))
		for _, field := range invalid.Fields {
			reported = append(reported, field.Field)
		}
		assert.Contains(t, reported, "diastolic_mmhg")
	})

	t.Run("diastolic must be less than systolic", func(t *testing.T) {
		t.Parallel()

		v := clinical.Vitals{RecordedAt: validVitals().RecordedAt, SystolicMmHg: f(100), DiastolicMmHg: f(100)}
		require.Error(t, v.Validate())
	})

	t.Run("every bounded field names its accepted range when out of range", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name  string
			apply func(*clinical.Vitals)
		}{
			{"systolic_mmhg", func(v *clinical.Vitals) { v.SystolicMmHg, v.DiastolicMmHg = f(301), f(60) }},
			{"heart_rate_bpm", func(v *clinical.Vitals) { v.HeartRateBpm = f(301) }},
			{"respiratory_rate_bpm", func(v *clinical.Vitals) { v.RespiratoryRateBpm = f(81) }},
			{"temperature_c", func(v *clinical.Vitals) { v.TemperatureC = f(46) }},
			{"spo2_pct", func(v *clinical.Vitals) { v.SpO2Pct = f(49) }},
			{"weight_kg", func(v *clinical.Vitals) { v.WeightKg = f(0.1) }},
			{"height_cm", func(v *clinical.Vitals) { v.HeightCm = f(29) }},
			{"glucose_mmol_l", func(v *clinical.Vitals) { v.GlucoseMmolL = f(61) }},
			{"hba1c_pct", func(v *clinical.Vitals) { v.Hba1cPct = f(21) }},
			{"pain_scale", func(v *clinical.Vitals) { v.PainScale = f(11) }},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				v := clinical.Vitals{RecordedAt: validVitals().RecordedAt}
				tc.apply(&v)

				err := v.Validate()
				require.Error(t, err)

				var invalid *domain.ValidationError
				require.ErrorAs(t, err, &invalid)

				var found *domain.FieldError
				for i, field := range invalid.Fields {
					if field.Field == tc.name {
						found = &invalid.Fields[i]
					}
				}
				require.NotNil(t, found, "no refusal named %s", tc.name)
				assert.Equal(t, domain.CodeOutOfRange, found.Code)
			})
		}
	})

	t.Run("an unpublished glucose_context is refused", func(t *testing.T) {
		t.Parallel()

		v := validVitals()
		v.GlucoseContext = "not-a-real-context"
		require.Error(t, v.Validate())
	})
}

func TestVitalsBMI(t *testing.T) {
	t.Parallel()

	t.Run("absent when either input is missing", func(t *testing.T) {
		t.Parallel()

		_, ok := clinical.Vitals{WeightKg: f(70)}.BMI()
		assert.False(t, ok)
	})

	t.Run("derived when both are present", func(t *testing.T) {
		t.Parallel()

		bmi, ok := clinical.Vitals{WeightKg: f(70), HeightCm: f(175)}.BMI()
		require.True(t, ok)
		assert.InDelta(t, clinical.BMI(70, 175), bmi, 0.0001)
	})
}

func TestVitalsMarshalZerologObjectEmitsOnlyIdentifiers(t *testing.T) {
	t.Parallel()

	v := validVitals()
	v.ID = "vit1"
	v.PatientID = "pat1"
	v.Device = "a-device-name-that-must-never-reach-a-log-line"

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(v).Send()
	line := buf.String()

	assert.Contains(t, line, "vit1")
	assert.Contains(t, line, "pat1")
	assert.NotContains(t, line, v.Device)
}
