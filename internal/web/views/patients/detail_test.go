package patients_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/person"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/patients"
	"medikube/internal/web/views/viewstest"
)

const patientChartArticle = "Patient chart"

// T055, FR-030. The landmark is present and non-empty, and it is the id the
// route table's own SmokeURL addresses.
func TestTheDetailRendersItsLandmark(t *testing.T) {
	t.Parallel()

	patient := selfRecord(t)

	tree := viewstest.Render(t, patients.PatientDetail(patients.PatientDetailProps{Patient: patient}), "div")

	region := tree.One(t, viewstest.Region(patientChartArticle))
	assert.Equal(t, ids.PatientDetail(patient.ID), viewstest.Attr(region, "id"))
	assert.NotEmpty(t, viewstest.Elements(region))
}

// FR-030, US4-2, SC-013: the landmark is present in both the populated and
// the entirely empty case, and @EmptyState renders inside it rather than
// instead of it — the assertion T097's smoke gate depends on.
func TestTheDetailLandmarkHoldsBothThePopulatedAndTheEmptyChart(t *testing.T) {
	t.Parallel()

	patient := selfRecord(t)

	t.Run("populated", func(t *testing.T) {
		t.Parallel()

		props := patients.PatientDetailProps{
			Patient:      patient,
			Tiles:        []patients.ChartTile{{Label: "Medications", Href: "/" + kind.Medication.Segment() + "?patient=" + patient.ID, Count: 3}},
			TotalRecords: 3,
		}

		tree := viewstest.Render(t, patients.PatientDetail(props), "div")

		region := tree.One(t, viewstest.Region(patientChartArticle))
		assert.Contains(t, viewstest.Text(region), "Medications")
		assert.Contains(t, viewstest.Text(region), "3")
	})

	t.Run("entirely empty", func(t *testing.T) {
		t.Parallel()

		props := patients.PatientDetailProps{Patient: patient}
		require.True(t, props.Empty())

		tree := viewstest.Render(t, patients.PatientDetail(props), "div")

		region := tree.One(t, viewstest.Region(patientChartArticle))
		assert.NotEmpty(t, viewstest.Elements(region))
		assert.Contains(t, viewstest.Text(region), "Nothing recorded yet")
	})
}

// FR-030, US1-6: an absent detail renders as absent, never as "0" or a blank
// box that reads as a recorded value.
func TestAbsentDetailsDoNotRenderAsZeroOrBlank(t *testing.T) {
	t.Parallel()

	birth, err := domain.ParseDate("2015-09-03")
	require.NoError(t, err)

	sparse := person.Patient{
		ID: "pat0000000003", FirstName: "Chiamaka", LastName: "Okonkwo", BirthDate: birth,
	}
	view := patients.NewPatientView(t.Context(), sparse, "", identity.UnitSystemMetric, patients.PatientLinks{})

	require.Empty(t, view.BloodType)
	require.Empty(t, view.HeightCM)
	require.Empty(t, view.WeightKG)

	tree := viewstest.Render(t, patients.PatientDetail(patients.PatientDetailProps{Patient: view}), "div")
	text := viewstest.Text(tree.One(t, viewstest.Region(patientChartArticle)))

	for _, absent := range []string{"0 cm", "0 kg", "blood_type"} {
		assert.NotContains(t, text, absent)
	}
}

// FR-030: every value actually recorded is shown.
func TestTheDetailShowsEveryRecordedValue(t *testing.T) {
	t.Parallel()

	birth, err := domain.ParseDate("1955-01-20")
	require.NoError(t, err)

	full := person.Patient{
		ID: "pat0000000004", FirstName: "Emeka", LastName: "Okonkwo",
		BirthDate: birth, Sex: person.SexMale, BloodType: person.BloodTypeAPos,
		HeightCM: 175, WeightKG: 70.5, Address: "12 Rowan Street",
		RelationshipToOwner: person.RelationshipParent,
	}
	view := patients.NewPatientView(t.Context(), full, "", identity.UnitSystemMetric, patients.PatientLinks{})

	tree := viewstest.Render(t, patients.PatientDetail(patients.PatientDetailProps{Patient: view}), "div")
	text := viewstest.Text(tree.One(t, viewstest.Region(patientChartArticle)))

	for _, value := range []string{
		view.BirthDate, view.Sex, view.BloodType, view.HeightCM, view.WeightKG,
		view.Address, view.Relationship,
	} {
		require.NotEmpty(t, value, "the fixture left a field empty, so this assertion proves nothing")
		assert.Contains(t, text, value)
	}
}
