package clinical

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func validCondition() Condition {
	return Condition{Diagnosis: "Type 2 diabetes", Status: ConditionStatusActive}
}

func TestConditionValidate(t *testing.T) {
	t.Parallel()

	t.Run("a minimal valid condition passes", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validCondition().Validate())
	})

	t.Run("diagnosis is required", func(t *testing.T) {
		t.Parallel()
		c := validCondition()
		c.Diagnosis = ""
		err := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "diagnosis", invalid.Fields[0].Field)
	})

	t.Run("status is required", func(t *testing.T) {
		t.Parallel()
		c := validCondition()
		c.Status = ""
		err := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "status", invalid.Fields[0].Field)
	})

	t.Run("resolved requires resolved_on (FR-020)", func(t *testing.T) {
		t.Parallel()
		c := validCondition()
		c.Status = ConditionStatusResolved
		err := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "resolved_on", invalid.Fields[0].Field)
		assert.Equal(t, domain.CodeRequired, invalid.Fields[0].Code)
	})

	t.Run("resolved_on before onset_on is refused", func(t *testing.T) {
		t.Parallel()
		c := validCondition()
		c.Status = ConditionStatusResolved
		onset, err := domain.NewDate(2024, 6, 1)
		require.NoError(t, err)
		resolved, err := domain.NewDate(2024, 1, 1)
		require.NoError(t, err)
		c.OnsetOn = onset
		c.ResolvedOn = resolved

		verr := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, verr, &invalid)

		fields := make([]string, 0, len(invalid.Fields))
		for _, f := range invalid.Fields {
			fields = append(fields, f.Field)
		}
		assert.Contains(t, fields, "resolved_on")
	})

	t.Run("resolved_on in the future is refused", func(t *testing.T) {
		t.Parallel()
		c := validCondition()
		c.Status = ConditionStatusResolved
		future, err := domain.NewDate(2999, 1, 1)
		require.NoError(t, err)
		c.ResolvedOn = future

		verr := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, verr, &invalid)

		codes := make([]string, 0, len(invalid.Fields))
		for _, f := range invalid.Fields {
			codes = append(codes, f.Code)
		}
		assert.Contains(t, codes, CodeNotFuture)
	})

	t.Run("both an ordering and a future violation are reported together", func(t *testing.T) {
		t.Parallel()
		c := validCondition()
		c.Status = ConditionStatusResolved
		onset, err := domain.NewDate(2999, 6, 1)
		require.NoError(t, err)
		resolved, err := domain.NewDate(2999, 1, 1)
		require.NoError(t, err)
		c.OnsetOn = onset
		c.ResolvedOn = resolved

		verr := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, verr, &invalid)
		assert.GreaterOrEqual(t, len(invalid.Fields), 2)
	})
}

func TestConditionMarshalZerologObjectRedactsPatientData(t *testing.T) {
	t.Parallel()

	c := Condition{ID: "condition-1", PatientID: "patient-1", Diagnosis: "SECRET-DIAGNOSIS", Notes: "SECRET-NOTES"}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(c).Msg("")

	line := buf.String()
	assert.Contains(t, line, "condition-1")
	assert.NotContains(t, line, "SECRET-DIAGNOSIS")
	assert.NotContains(t, line, "SECRET-NOTES")
}
