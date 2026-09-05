package clinical

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func validAllergy() Allergy {
	return Allergy{Allergen: "Penicillin", Severity: SeverityLifeThreatening, Status: ConditionStatusActive}
}

func TestAllergyValidate(t *testing.T) {
	t.Parallel()

	t.Run("a minimal valid allergy passes", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validAllergy().Validate())
	})

	t.Run("allergen is required", func(t *testing.T) {
		t.Parallel()
		a := validAllergy()
		a.Allergen = "  "
		err := a.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "allergen", invalid.Fields[0].Field)
	})

	t.Run("severity is required", func(t *testing.T) {
		t.Parallel()
		a := validAllergy()
		a.Severity = ""
		err := a.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "severity", invalid.Fields[0].Field)
	})

	t.Run("severity must be published", func(t *testing.T) {
		t.Parallel()
		a := validAllergy()
		a.Severity = "extreme"
		err := a.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
	})

	t.Run("status defaults are the service's, not Validate's, but an invalid one is refused", func(t *testing.T) {
		t.Parallel()
		a := validAllergy()
		a.Status = "gone"
		err := a.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "status", invalid.Fields[0].Field)
	})

	t.Run("reaction, onset and notes are optional", func(t *testing.T) {
		t.Parallel()
		a := validAllergy()
		a.Reaction = "anaphylaxis"
		onset, err := domain.NewDate(2020, 1, 1)
		require.NoError(t, err)
		a.OnsetOn = onset
		assert.NoError(t, a.Validate())
	})
}

func TestAllergyCritical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		severity Severity
		status   ConditionStatus
		want     bool
	}{
		{"severe and active is critical", SeveritySevere, ConditionStatusActive, true},
		{"life-threatening and chronic is critical", SeverityLifeThreatening, ConditionStatusChronic, true},
		{"mild and active is not critical", SeverityMild, ConditionStatusActive, false},
		{"severe and resolved is not critical", SeveritySevere, ConditionStatusResolved, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := Allergy{Severity: tt.severity, Status: tt.status}
			assert.Equal(t, tt.want, a.Critical())
		})
	}
}

func TestAllergyMarshalZerologObjectRedactsPatientData(t *testing.T) {
	t.Parallel()

	a := Allergy{
		ID: "allergy-1", PatientID: "patient-1",
		Allergen: "SECRET-ALLERGEN", Reaction: "SECRET-REACTION", Notes: "SECRET-NOTES",
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(a).Msg("")

	line := buf.String()
	assert.Contains(t, line, "allergy-1")
	assert.Contains(t, line, "patient-1")
	assert.NotContains(t, line, "SECRET-ALLERGEN")
	assert.NotContains(t, line, "SECRET-REACTION")
	assert.NotContains(t, line, "SECRET-NOTES")
}
