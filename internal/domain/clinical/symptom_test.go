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

func validSymptom() clinical.Symptom {
	return clinical.Symptom{
		Name:       "Dizziness",
		Severity:   clinical.SeverityModerate,
		OccurredAt: clinical.NewInstant(time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)),
	}
}

func TestSymptomValidate(t *testing.T) {
	t.Parallel()

	t.Run("a minimal episode is accepted", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validSymptom().Validate())
	})

	t.Run("name, severity and occurred_at are required", func(t *testing.T) {
		t.Parallel()

		err := clinical.Symptom{}.Validate()
		require.Error(t, err)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)

		fields := make(map[string]bool, len(invalid.Fields))
		for _, f := range invalid.Fields {
			fields[f.Field] = true
		}

		assert.True(t, fields["name"])
		assert.True(t, fields["severity"])
		assert.True(t, fields["occurred_at"])
	})

	t.Run("resolved_at before occurred_at is refused", func(t *testing.T) {
		t.Parallel()

		s := validSymptom()
		s.ResolvedAt = clinical.NewInstant(s.OccurredAt.Time().Add(-time.Hour))

		err := s.Validate()
		require.Error(t, err)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		require.Len(t, invalid.Fields, 1)
		assert.Equal(t, "resolved_at", invalid.Fields[0].Field)
	})

	t.Run("resolved_at equal to occurred_at is accepted", func(t *testing.T) {
		t.Parallel()

		s := validSymptom()
		s.ResolvedAt = s.OccurredAt
		require.NoError(t, s.Validate())
	})

	t.Run("pain_scale is bounded 0..10", func(t *testing.T) {
		t.Parallel()

		for _, bad := range []int{-1, 11} {
			s := validSymptom()
			s.PainScale = &bad
			require.Error(t, s.Validate())
		}

		for _, ok := range []int{0, 10} {
			s := validSymptom()
			s.PainScale = &ok
			require.NoError(t, s.Validate())
		}
	})

	t.Run("duration_minutes cannot be negative", func(t *testing.T) {
		t.Parallel()

		s := validSymptom()
		negative := -1
		s.DurationMinutes = &negative
		require.Error(t, s.Validate())
	})

	t.Run("triggers and relief_methods are bounded", func(t *testing.T) {
		t.Parallel()

		s := validSymptom()
		s.Triggers = make([]string, 21)
		require.Error(t, s.Validate())

		s = validSymptom()
		s.Triggers = []string{string(make([]byte, 81))}
		require.Error(t, s.Validate())

		s = validSymptom()
		s.ReliefMethods = make([]string, 21)
		require.Error(t, s.Validate())
	})

	t.Run("an unpublished category, impact or status is refused", func(t *testing.T) {
		t.Parallel()

		s := validSymptom()
		s.Category = "not-a-real-category"
		require.Error(t, s.Validate())

		s = validSymptom()
		s.Impact = "not-a-real-impact"
		require.Error(t, s.Validate())

		s = validSymptom()
		s.Status = "not-a-real-status"
		require.Error(t, s.Validate())
	})
}

func TestSymptomMarshalZerologObjectEmitsOnlyIdentifiers(t *testing.T) {
	t.Parallel()

	s := validSymptom()
	s.ID = "sym1"
	s.PatientID = "pat1"
	s.Name = "a-name-that-must-never-reach-a-log-line"

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(s).Send()
	line := buf.String()

	assert.Contains(t, line, "sym1")
	assert.Contains(t, line, "pat1")
	assert.NotContains(t, line, s.Name)
}
