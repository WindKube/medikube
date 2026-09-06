package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// T142, FR-060. Every effective_* value renders its provenance: whether it is
// the course's own or fell back to the medication's default.
func TestCourseMedicationsRendersEveryEffectiveValueWithItsProvenance(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.CourseMedications("course-medications", "Course medications", []records.CourseMedicationRow{
		{
			MedicationName: "Amoxicillin",
			MedicationHref: "/" + kind.Medication.Collection() + "/med1",
			Dosage:         records.CourseMedicationEffectiveView{Value: "500mg", Source: "course"},
			Frequency:      records.CourseMedicationEffectiveView{Value: "twice daily", Source: "medication"},
		},
	}, records.CourseMedicationFormProps{}), "div")

	section := tree.One(t, viewstest.Region("Course medications"))
	text := viewstest.Text(section)

	assert.Contains(t, text, "500mg")
	assert.Contains(t, text, "this course")
	assert.Contains(t, text, "twice daily")
	assert.Contains(t, text, "the medication")

	anchor := tree.One(t, viewstest.Tag("a"))
	assert.Equal(t, "/"+kind.Medication.Collection()+"/med1", viewstest.Attr(anchor, "href"))
	assert.Equal(t, "Amoxicillin", viewstest.Text(anchor))
}

// A field the entity never set renders nothing at all (FR-024's convention),
// rather than an empty value with a provenance for something not recorded.
func TestCourseMedicationsOmitsAFieldWithNoValue(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.CourseMedications("course-medications", "Course medications", []records.CourseMedicationRow{
		{MedicationName: "Amoxicillin", MedicationHref: "/" + kind.Medication.Collection() + "/med1"},
	}, records.CourseMedicationFormProps{}), "div")

	section := tree.One(t, viewstest.Region("Course medications"))
	require.Empty(t, tree.All(viewstest.Tag("dt")))
	assert.Contains(t, viewstest.Text(section), "Amoxicillin")
}

func TestCourseMedicationsRendersAnEmptyStateWithNoRows(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.CourseMedications("course-medications", "Course medications", nil, records.CourseMedicationFormProps{}), "div")

	section := tree.One(t, viewstest.Region("Course medications"))
	require.Empty(t, tree.All(viewstest.Tag("a")))
	assert.NotEmpty(t, viewstest.Text(section))
}

// A row gets its own Remove button (FR-058), and the section always ends with
// the attach form's picker regardless of how many rows there are.
func TestCourseMedicationsRendersRemoveAndTheAttachForm(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.CourseMedications("course-medications", "Course medications", []records.CourseMedicationRow{
		{MedicationName: "Amoxicillin", MedicationHref: "/medications/med1", RemoveOn: "@delete('/x')"},
	}, records.CourseMedicationFormProps{
		Options: []records.MedicationLinkOption{{ID: "med2", Name: "Ibuprofen"}},
	}), "div")

	tree.One(t, viewstest.Region("Course medications"))
	assert.NotEmpty(t, tree.All(viewstest.Tag("button")))

	picker := tree.One(t, viewstest.Tag("select"))
	assert.Contains(t, viewstest.Text(picker), "Ibuprofen")
}
