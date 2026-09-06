package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func TestCourseMedicationValidateRefusesEndBeforeStart(t *testing.T) {
	t.Parallel()

	started := mustDate(t, "2026-03-02")
	ended := mustDate(t, "2026-03-01")

	c := CourseMedication{StartedOn: started, EndedOn: ended}
	err := c.Validate()
	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "ended_on", invalid.Fields[0].Field)
}

func TestCourseMedicationValidateAcceptsEqualDates(t *testing.T) {
	t.Parallel()

	day := mustDate(t, "2026-03-02")
	c := CourseMedication{StartedOn: day, EndedOn: day}
	assert.NoError(t, c.Validate())
}

func TestCourseMedicationResolveFallsBackToTheMedication(t *testing.T) {
	t.Parallel()

	medication := Medication{
		Dosage: "5mg", Frequency: "twice daily",
		PractitionerID: "rec_practitioner", PharmacyID: "rec_pharmacy",
		StartedOn: mustDate(t, "2026-01-01"), EndedOn: mustDate(t, "2026-01-31"),
	}

	t.Run("every course field absent falls back to the medication", func(t *testing.T) {
		t.Parallel()

		fields := CourseMedication{}.Resolve(medication)

		assert.Equal(t, Effective{Value: "5mg", Source: SourceMedication}, fields.Dosage)
		assert.Equal(t, Effective{Value: "twice daily", Source: SourceMedication}, fields.Frequency)
		assert.Equal(t, Effective{Value: nil, Source: SourceNone}, fields.Duration)
		assert.Equal(t, Effective{Value: nil, Source: SourceNone}, fields.Timing)
		assert.Equal(t, Effective{Value: "rec_practitioner", Source: SourceMedication}, fields.Prescriber)
		assert.Equal(t, Effective{Value: "rec_pharmacy", Source: SourceMedication}, fields.Pharmacy)
		assert.Equal(t, Effective{Value: medication.StartedOn, Source: SourceMedication}, fields.StartedOn)
		assert.Equal(t, Effective{Value: medication.EndedOn, Source: SourceMedication}, fields.EndedOn)
	})

	t.Run("a present course field wins over the medication's own", func(t *testing.T) {
		t.Parallel()

		course := CourseMedication{
			Dosage: "3mg", Frequency: "daily", Duration: "6 weeks", Timing: "morning",
			PrescriberID: "rec_other", PharmacyID: "rec_other_pharmacy",
			StartedOn: mustDate(t, "2026-03-02"), EndedOn: mustDate(t, "2026-03-09"),
		}

		fields := course.Resolve(medication)

		assert.Equal(t, Effective{Value: "3mg", Source: SourceCourse}, fields.Dosage)
		assert.Equal(t, Effective{Value: "daily", Source: SourceCourse}, fields.Frequency)
		assert.Equal(t, Effective{Value: "6 weeks", Source: SourceCourse}, fields.Duration)
		assert.Equal(t, Effective{Value: "morning", Source: SourceCourse}, fields.Timing)
		assert.Equal(t, Effective{Value: "rec_other", Source: SourceCourse}, fields.Prescriber)
		assert.Equal(t, Effective{Value: "rec_other_pharmacy", Source: SourceCourse}, fields.Pharmacy)
		assert.Equal(t, Effective{Value: course.StartedOn, Source: SourceCourse}, fields.StartedOn)
		assert.Equal(t, Effective{Value: course.EndedOn, Source: SourceCourse}, fields.EndedOn)
	})

	t.Run("neither present resolves to none", func(t *testing.T) {
		t.Parallel()

		fields := CourseMedication{}.Resolve(Medication{})

		assert.Equal(t, SourceNone, fields.Dosage.Source)
		assert.Nil(t, fields.Dosage.Value)
		assert.Equal(t, SourceNone, fields.StartedOn.Source)
	})
}
