package page_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web/page"
)

// T095/US3-3. The three page-layer redirect rules contracts/active-patient.md
// spells out for a bare, unscoped record page.

// addPatient writes one more patient owned by ownerID directly, so a test can
// give an account more reachable people than the seed does without touching
// anybody else's fixture.
func addPatient(t *testing.T, app core.App, ownerID, id, firstName string) {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(store.PatientCollection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Id = id
	record.Set("owner", ownerID)
	record.Set("first_name", firstName)
	record.Set("last_name", "Eze")
	record.Set("relationship_to_owner", "child")
	require.NoError(t, app.Save(record))
}

func setActivePatientPointer(t *testing.T, app core.App, userID, patientID string) {
	t.Helper()

	record, err := app.FindRecordById(store.AccountCollection, userID)
	require.NoError(t, err)

	store.SetUserActivePatientID(record, patientID)
	require.NoError(t, app.Save(record))
}

// TestABareMedicationsPageRedirectsToTheActivePatient is the medications half:
// /medications carries no ?patient= of its own, so the page redirects to the
// one the pointer already names (Account A's seeded self-record).
func TestABareMedicationsPageRedirectsToTheActivePatient(t *testing.T) {
	t.Parallel()

	browser := newBrowser(t)

	status, headers, body := browser.get(pageRoutes(t)[page.OpMedicationListPage].Path)

	require.Equal(t, http.StatusSeeOther, status, body)
	assert.Contains(t, headers.Get("Location"),
		pageRoutes(t)[page.OpMedicationListPage].Path+"?patient="+testsupport.AccountAPatientSelfID)
}

// TestANullPointerWithSeveralPatientsRedirectsToPatients: an account with no
// active patient and more than one reachable person cannot be auto-selected
// (FR-018 only fires on exactly one), so the page sends it to /patients
// rather than guessing.
func TestANullPointerWithSeveralPatientsRedirectsToPatients(t *testing.T) {
	t.Parallel()

	browser := newBrowser(t).as(testsupport.AccountCEmail)

	addPatient(t, browser.app, testsupport.AccountCID, "mkpatchidi00001", "Chidi")
	addPatient(t, browser.app, testsupport.AccountCID, "mkpatchidi00002", "Ngozi")

	status, headers, body := browser.get(pageRoutes(t)[page.OpMedicationListPage].Path)

	require.Equal(t, http.StatusSeeOther, status, body)
	assert.Equal(t, patientsPageURL(t), headers.Get("Location"))
}

// TestAPointerAtAnUnreachablePatientNeverRendersAnotherPersonsData: the
// pointer names a real patient row, but not one this account can reach (it
// belongs to Account B). ResolveActivePatient treats that exactly like null
// (FR-017) rather than rendering — or even naming — somebody else's record.
func TestAPointerAtAnUnreachablePatientNeverRendersAnotherPersonsData(t *testing.T) {
	t.Parallel()

	browser := newBrowser(t).as(testsupport.AccountCEmail)

	addPatient(t, browser.app, testsupport.AccountCID, "mkpatchidi00003", "Chidi")
	addPatient(t, browser.app, testsupport.AccountCID, "mkpatchidi00004", "Ngozi")
	setActivePatientPointer(t, browser.app, testsupport.AccountCID, testsupport.AccountAPatientSelfID)

	status, headers, body := browser.get(pageRoutes(t)[page.OpMedicationListPage].Path)

	require.Equal(t, http.StatusSeeOther, status, body)
	assert.Equal(t, patientsPageURL(t), headers.Get("Location"))
	assert.NotContains(t, body, "Amara", "another account's patient must never be named on a redirect")
}

func patientsPageURL(t *testing.T) string {
	t.Helper()

	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == page.OpPatientListPage {
			return route.Path
		}
	}

	t.Fatal("patientListPage is not registered")

	return ""
}
