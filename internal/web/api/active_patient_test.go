package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/store"
	"medikube/internal/testsupport"
)

func activePatientURL() string { return "/api/v1/me/active-patient" }

func assertActivePatient(t testing.TB, app *tests.TestApp, userID, want string) {
	t.Helper()

	record, err := app.FindRecordById(store.AccountCollection, userID)
	require.NoError(t, err)
	assert.Equal(t, want, store.UserActivePatientID(record))
}

// pointerAlreadySet is a BeforeTestFunc that writes the pointer directly,
// standing in for an earlier PUT /me/active-patient a prior session already
// made: a second tests.ApiScenario against the same served app double-
// registers its routes and panics (reconciliation C14), so a fixture that
// needs "the pointer was already switched" writes the column itself rather
// than replaying the write through a second live request.
func pointerAlreadySet(userID, patientID string) func(testing.TB, *tests.TestApp, *core.ServeEvent) {
	return func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		t.Helper()

		record, err := app.FindRecordById(store.AccountCollection, userID)
		require.NoError(t, err)

		store.SetUserActivePatientID(record, patientID)
		require.NoError(t, app.Save(record))
	}
}

// T090. contracts/active-patient.md's one write, driven the way
// patients_test.go drives its own family: 200 on an owned target, 404 (with
// the pointer left exactly where it was) for another account's patient, 422
// on a malformed body, 401 anonymous.
func TestSetActivePatientAnswersEveryDocumentedShape(t *testing.T) {
	t.Parallel()

	t.Run("switching to an owned patient answers 200 and writes the pointer", func(t *testing.T) {
		t.Parallel()

		headers, before := patientSignedIn(testsupport.AccountAEmail)

		scenario := tests.ApiScenario{
			Name: "switch to an owned patient", Method: http.MethodPut, URL: activePatientURL(),
			Headers: headers, Body: strings.NewReader(`{"patient":"` + testsupport.AccountAPatientChildID + `"}`),
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{`"active_patient":`, testsupport.AccountAPatientChildID},
			BeforeTestFunc:  before,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				t.Helper()
				assertActivePatient(t, app, testsupport.AccountAID, testsupport.AccountAPatientChildID)
			},
		}

		runPatients(t, scenario)
	})

	t.Run("another account's patient is a 404 and the pointer is left unchanged", func(t *testing.T) {
		t.Parallel()

		headers, before := patientSignedIn(testsupport.AccountAEmail)

		scenario := tests.ApiScenario{
			Name: "another account's patient", Method: http.MethodPut, URL: activePatientURL(),
			Headers: headers, Body: strings.NewReader(`{"patient":"` + testsupport.AccountBPatientSelfID + `"}`),
			ExpectedStatus:  http.StatusNotFound,
			ExpectedContent: []string{`"code":"not_found"`},
			BeforeTestFunc:  before,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				t.Helper()
				// The seed points Account A's pointer at its own self-record
				// (testsupport/seed's applyActivePatients); a refused switch
				// must leave it exactly there rather than clearing it.
				assertActivePatient(t, app, testsupport.AccountAID, testsupport.AccountAPatientSelfID)
			},
		}

		runPatients(t, scenario)
	})

	t.Run("a malformed body is a 422", func(t *testing.T) {
		t.Parallel()

		headers, before := patientSignedIn(testsupport.AccountAEmail)

		scenario := tests.ApiScenario{
			Name: "malformed body", Method: http.MethodPut, URL: activePatientURL(),
			Headers: headers, Body: strings.NewReader(`{"patient":123}`),
			ExpectedStatus:  http.StatusUnprocessableEntity,
			ExpectedContent: []string{`"code":"validation_failed"`},
			BeforeTestFunc:  before,
		}

		runPatients(t, scenario)
	})

	t.Run("anonymous is a 401", func(t *testing.T) {
		t.Parallel()

		scenario := tests.ApiScenario{
			Name: "anonymous", Method: http.MethodPut, URL: activePatientURL(),
			Body:            strings.NewReader(`{"patient":"` + testsupport.AccountAPatientSelfID + `"}`),
			ExpectedStatus:  http.StatusUnauthorized,
			ExpectedContent: []string{`"code":"unauthenticated"`},
		}

		runPatients(t, scenario)
	})
}

// T091/FR-015. The pointer is never an authorization input: an account whose
// active patient is already switched to one it owns still gets a plain 404,
// not the record, when it asks for another account's patient. This is the
// one test in this family that would fail silently if authorization were
// ever reordered to run against the pointer rather than the request's own
// target.
func TestSwitchingPatientsGrantsNoAccessToAnotherAccountsRecords(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)
	pointerSet := pointerAlreadySet(testsupport.AccountAID, testsupport.AccountAPatientSelfID)

	scenario := tests.ApiScenario{
		Name: "the pointer never authorizes", Method: http.MethodGet, URL: patientURL(testsupport.AccountBPatientSelfID),
		Headers:         headers,
		ExpectedStatus:  http.StatusNotFound,
		ExpectedContent: []string{`"code":"not_found"`},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
			t.Helper()
			pointerSet(t, app, se)
			before(t, app, se)
		},
	}

	runPatients(t, scenario)
}

// T092/SC-014. The pointer is a durable fact about the account, not the
// session: GET /api/v1/me answers with it from a session a prior one never
// touched, exactly as it would after a sign-out and a fresh sign-in.
func TestTheActivePatientPointerSurvivesSignOutAndSignIn(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)
	pointerSet := pointerAlreadySet(testsupport.AccountAID, testsupport.AccountAPatientChildID)

	scenario := tests.ApiScenario{
		Name: "the pointer survives a fresh session", Method: http.MethodGet, URL: "/api/v1/me",
		Headers:         headers,
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"active_patient":`, testsupport.AccountAPatientChildID},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
			t.Helper()
			pointerSet(t, app, se)
			// patientSignedIn mints its own fresh token independent of any
			// session that could have set the pointer, exactly what a
			// sign-out followed by a sign-in produces.
			before(t, app, se)
		},
	}

	runPatients(t, scenario)
}
