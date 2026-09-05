package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/httproute"
)

// T255, FR-047. Every data-changing operation should announce success or
// failure through #toast or #error-banner. Today every mutating route in the
// table is KindAPI and answers the JSON envelope contracts/records.md and
// contracts/account.md require — there is no KindPage mutation to patch a
// toast into. dataChangingJSONOnly is the reason map: a mutation that starts
// rendering HTML (a KindPage route, or a Datastar element patch) and is not
// listed here fails this test, which is the point.
var dataChangingJSONOnly = map[string]string{
	"register":                 "contracts/auth.md: answers the JSON envelope",
	"login":                    "contracts/auth.md: answers the JSON envelope",
	"refreshSession":           "contracts/auth.md: answers the JSON envelope",
	"logout":                   "contracts/auth.md: answers the JSON envelope",
	"requestPasswordReset":     "contracts/auth.md: answers the JSON envelope",
	"confirmPasswordReset":     "contracts/auth.md: answers the JSON envelope",
	"requestEmailVerification": "contracts/auth.md: answers the JSON envelope",
	"confirmEmailVerification": "contracts/auth.md: answers the JSON envelope",
	"updateMe":                 "contracts/account.md: answers the JSON envelope",
	"deleteMe":                 "contracts/account.md: answers the JSON envelope",
	"changePassword":           "contracts/account.md: answers the JSON envelope",
	"setActivePatient":         "contracts/active-patient.md: answers the JSON envelope; a Datastar @put's own feedback is the re-rendered switcher, not a toast",
	"createRecord":             "contracts/records.md: answers the JSON envelope; live feedback is the SSE stream",
	"updateRecord":             "contracts/records.md: answers the JSON envelope; live feedback is the SSE stream",
	"deleteRecord":             "contracts/records.md: answers the JSON envelope; live feedback is the SSE stream",
	"createPatient":            "contracts/patients.md: answers the JSON envelope",
	"updatePatient":            "contracts/patients.md: answers the JSON envelope",
	"putPatientPhoto":          "contracts/patient-photo.md: answers the JSON envelope",
	"deletePatientPhoto":       "contracts/patient-photo.md: answers the JSON envelope",
	"createPractitioner":       "contracts/practitioners.md: answers the JSON envelope",
	"updatePractitioner":       "contracts/practitioners.md: answers the JSON envelope",
	"deletePractitioner":       "contracts/practitioners.md: answers the JSON envelope",
	"createFacility":           "contracts/facilities.md: answers the JSON envelope",
	"updateFacility":           "contracts/facilities.md: answers the JSON envelope",
	"deleteFacility":           "contracts/facilities.md: answers the JSON envelope",
}

func TestEveryDataChangingRouteIsAccountedForByFeedback(t *testing.T) {
	t.Parallel()

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal || route.Method == http.MethodGet {
			continue
		}

		reason, exempt := dataChangingJSONOnly[route.OpID]

		t.Run(route.OpID, func(t *testing.T) {
			assert.Truef(t, exempt, "%s is a data-changing route with no #toast/#error-banner feedback and no exemption reason", route.OpID)
			assert.NotEmpty(t, reason)
			assert.NotEqualf(t, httproute.KindPage, route.Kind,
				"%s is a page route: FR-047's feedback applies and it cannot be exempted as JSON-only", route.OpID)
		})
	}
}
