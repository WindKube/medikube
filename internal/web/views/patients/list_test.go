package patients_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/person"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/patients"
	"medikube/internal/web/views/viewstest"
)

const patientsRegion = "Patients"

func selfRecord(t *testing.T) patients.PatientView {
	t.Helper()

	birth, err := domain.ParseDate("1988-04-12")
	require.NoError(t, err)

	return patients.NewPatientView(t.Context(), person.Patient{
		ID: "pat0000000001", FirstName: "Amara", LastName: "Okonkwo",
		BirthDate: birth, IsSelfRecord: true,
	}, "", identity.UnitSystemMetric, patients.PatientLinks{Detail: "/patients/pat0000000001"})
}

func childRecord(t *testing.T) patients.PatientView {
	t.Helper()

	birth, err := domain.ParseDate("2015-09-03")
	require.NoError(t, err)

	return patients.NewPatientView(t.Context(), person.Patient{
		ID: "pat0000000002", FirstName: "Chiamaka", LastName: "Okonkwo",
		BirthDate: birth,
	}, "", identity.UnitSystemMetric, patients.PatientLinks{Detail: "/patients/pat0000000002"})
}

// T055, FR-029: the region is present and non-empty whether or not anything is
// recorded — an account with none is unreachable in production (FR-005
// guarantees a self-record) but the shape is the same rule as every list.
func TestTheRegionIsPresentWhetherOrNotAnyoneIsRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		patients []patients.PatientView
	}{
		{name: "an account with people", patients: []patients.PatientView{selfRecord(t), childRecord(t)}},
		{name: "an account with none", patients: nil},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, patients.PatientList(patients.PatientListProps{
				Patients: testCase.patients,
				Total:    len(testCase.patients),
			}), "div")

			region := tree.One(t, viewstest.Region(patientsRegion))
			assert.Equal(t, ids.PatientList(), viewstest.Attr(region, "id"))
			assert.NotEmpty(t, viewstest.Elements(region))
		})
	}
}

// FR-010: the list states how many there are and marks the account holder's
// own row.
func TestTheListStatesTheCountAndMarksTheAccountHolder(t *testing.T) {
	t.Parallel()

	self, child := selfRecord(t), childRecord(t)

	tree := viewstest.Render(t, patients.PatientList(patients.PatientListProps{
		Patients: []patients.PatientView{self, child},
		Total:    2,
	}), "div")

	region := tree.One(t, viewstest.Region(patientsRegion))
	text := viewstest.Text(region)

	assert.Contains(t, text, "2")

	selfRow := tree.One(t, viewstest.WithID(ids.PatientRow(self.ID)))
	assert.Contains(t, viewstest.Text(selfRow), "You")

	childRow := tree.One(t, viewstest.WithID(ids.PatientRow(child.ID)))
	assert.NotContains(t, viewstest.Text(childRow), "You")
}

// Edge case: two people with the same name. Name and date of birth are
// rendered together wherever people are listed.
func TestNameAndDateOfBirthAppearTogether(t *testing.T) {
	t.Parallel()

	self := selfRecord(t)

	tree := viewstest.Render(t, patients.PatientList(patients.PatientListProps{
		Patients: []patients.PatientView{self},
		Total:    1,
	}), "div")

	row := tree.One(t, viewstest.WithID(ids.PatientRow(self.ID)))
	text := viewstest.Text(row)

	assert.Contains(t, text, self.FullName())
	assert.Contains(t, text, self.BirthDate)
}
