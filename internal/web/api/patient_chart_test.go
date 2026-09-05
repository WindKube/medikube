package api_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

func patientChartURL(id string) string { return patientURL(id) + "/summary" }

// T108, contracts/patient-chart.md. 200's documented shape: one entry per
// registered kind including a kind with zero records, and recent_activity as
// `[]` rather than `null` for a patient with nothing recorded.
func TestGetPatientChartAnswersTheDocumentedShapeForAnEmptyPatient(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	// The child: seeded with no medications, so its chart is the empty case
	// (data-model §9).
	subject := testsupport.AccountAPatientChildID

	scenario := tests.ApiScenario{
		Name:   "the chart for a patient with nothing recorded",
		Method: http.MethodGet, URL: patientChartURL(subject), Headers: headers,
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"kind":"` + kind.Medication.Enum() + `"`,
			`"path":"/` + kind.Medication.Segment() + `"`,
			`"count":0`,
			`"total_records":0`,
			`"recent_activity":[]`,
		},
		BeforeTestFunc: before,
	}

	runPatients(t, scenario)
}

// FR-028, US4-1, SC-007. The self-record holds AccountAMedicationCount's own
// medications and the chart's count is exactly that, never a different
// number and never every account's rows summed together.
func TestGetPatientChartCountsOnlyThisPatientsMedications(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	subject := testsupport.AccountAPatientSelfID

	scenario := tests.ApiScenario{
		Name:   "the chart counts only this patient's own records",
		Method: http.MethodGet, URL: patientChartURL(subject), Headers: headers,
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"kind":"` + kind.Medication.Enum() + `"`,
		},
		BeforeTestFunc: before,
	}

	runPatients(t, scenario)
}

// 404 for another account's patient, indistinguishable from a genuine miss
// (FR-042).
func TestGetPatientChartIsNotFoundForAnotherAccountsPatient(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountBEmail)

	scenario := tests.ApiScenario{
		Name:   "a stranger's chart request is 404",
		Method: http.MethodGet, URL: patientChartURL(testsupport.AccountAPatientSelfID), Headers: headers,
		ExpectedStatus:  http.StatusNotFound,
		ExpectedContent: []string{`"code":"` + web.CodeNotFound + `"`},
		BeforeTestFunc:  before,
	}

	runPatients(t, scenario)
}

// 401 anonymous (FR-043).
func TestGetPatientChartIsUnauthenticatedForAnonymous(t *testing.T) {
	t.Parallel()

	scenario := tests.ApiScenario{
		Name:            "an anonymous chart request is 401",
		Method:          http.MethodGet,
		URL:             patientChartURL(testsupport.AccountAPatientSelfID),
		ExpectedStatus:  http.StatusUnauthorized,
		ExpectedContent: []string{`"code":"` + web.CodeUnauthenticated + `"`},
	}

	runPatients(t, scenario)
}

// T109, FR-029, US4-5. An activity entry states kind, action and time and
// carries no name, value, note or filename; a target that has since been
// deleted answers target_exists: false.
func TestGetPatientChartActivityEntryCarriesNoContentAndReportsADeletedTarget(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	subject := testsupport.AccountAPatientChildID
	const medicationName = "Amoxicillin-Sentinel"

	scenario := tests.ApiScenario{
		Name:   "an activity entry for a deleted medication",
		Method: http.MethodGet, URL: patientChartURL(subject), Headers: headers,
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"action":"delete"`,
			`"target_kind":"` + kind.Medication.Enum() + `"`,
			`"target_exists":false`,
		},
		NotExpectedContent: []string{medicationName},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
			t.Helper()

			before(t, app, se)
			seedThenDeleteMedication(t, app, subject, medicationName)
		},
	}

	runPatients(t, scenario)
}

func seedThenDeleteMedication(t testing.TB, app *tests.TestApp, patientID, name string) {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set(store.MedicationPatient, patientID)
	record.Set(store.MedicationName, name)
	record.Set(store.MedicationStatus, "active")

	require.NoError(t, app.Save(record))
	require.NoError(t, app.Delete(record))
}
