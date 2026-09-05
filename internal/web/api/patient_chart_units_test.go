package api_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// T110, FR-007, contracts/patient-chart.md. Changing the account's
// unit_system changes only the chart's display block; the recorded
// height_cm/weight_kg stay the byte-identical metric value that was stored.
func TestGetPatientChartUnitSystemChangesOnlyTheDisplayBlock(t *testing.T) {
	t.Parallel()

	subject := testsupport.AccountAPatientSelfID
	const storedHeightCM = 180
	const storedWeightKG = 75.5

	seedMeasures := func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
		t.Helper()

		record, err := app.FindRecordById(store.PatientCollection, subject)
		require.NoError(t, err)
		record.Set("height_cm", storedHeightCM)
		record.Set("weight_kg", storedWeightKG)
		require.NoError(t, app.Save(record))
	}

	metricHeaders, metricBefore := patientSignedIn(testsupport.AccountAEmail)

	metric := tests.ApiScenario{
		Name:   "the chart in metric",
		Method: http.MethodGet, URL: patientChartURL(subject), Headers: metricHeaders,
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"height_cm":180`,
			`"weight_kg":75.5`,
			`"unit_system":"metric"`,
			`"height":"180 cm"`,
		},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
			t.Helper()
			metricBefore(t, app, se)
			seedMeasures(t, app, se)
		},
	}
	runPatients(t, metric)

	imperialHeaders, imperialBefore := patientSignedIn(testsupport.AccountAEmail)

	imperial := tests.ApiScenario{
		Name:   "the same patient in imperial",
		Method: http.MethodGet, URL: patientChartURL(subject), Headers: imperialHeaders,
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			// The stored value is untouched: still 180/75.5, never converted
			// in place.
			`"height_cm":180`,
			`"weight_kg":75.5`,
			`"unit_system":"imperial"`,
		},
		NotExpectedContent: []string{`"height":"180 cm"`},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
			t.Helper()
			imperialBefore(t, app, se)
			seedMeasures(t, app, se)

			account, err := app.FindAuthRecordByEmail(store.AccountCollection, testsupport.AccountAEmail)
			require.NoError(t, err)
			account.Set("unit_system", "imperial")
			require.NoError(t, app.Save(account))
		},
	}
	runPatients(t, imperial)
}
