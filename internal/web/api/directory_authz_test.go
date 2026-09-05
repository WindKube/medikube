package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport"
)

// T130, FR-037, US5-6, SC-014: all ten directory operations run three ways —
// as the owner, as a signed-in stranger, and as nobody — with the stranger's
// refusal compared with a genuine miss byte for byte, headers included.

const missingPractitionerID = "mkprcnobody0001"

const missingFacilityID = "mkfacnobody0001"

func TestEveryPractitionerOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)

	updateTarget := owner.post(practitionersURL(), `{"name":"Dr. Owner Scoped Update"}`).practitioner(t)
	deleteTarget := owner.post(practitionersURL(), `{"name":"Dr. Owner Scoped Delete"}`).practitioner(t)

	updatePrecondition := map[string]string{"If-Match": owner.get(practitionerURL(updateTarget.ID)).etag(t)}
	deletePrecondition := map[string]string{"If-Match": owner.get(practitionerURL(deleteTarget.ID)).etag(t)}

	readTarget := practitionerURL(testsupport.AccountAPractitionerID)

	secrets := []string{testsupport.AccountAPractitionerID, "Dr. Ngozi Adeyemi", testsupport.AccountAID}
	updateSecrets := []string{updateTarget.ID, "Dr. Owner Scoped Update", testsupport.AccountAID}
	deleteSecrets := []string{deleteTarget.ID, "Dr. Owner Scoped Delete", testsupport.AccountAID}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         owner.handler,
		Owner:           bearer(t, owner.app, testsupport.AccountAEmail),
		Stranger:        bearer(t, owner.app, testsupport.AccountBEmail),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:             "list practitioners",
				Method:           http.MethodGet,
				Path:             practitionersURL() + "?limit=100",
				StrangerIsolated: true,
				Secrets:          secrets,
			},
			{
				Name:             "create a practitioner",
				Method:           http.MethodPost,
				Path:             practitionersURL(),
				Body:             `{"name":"Dr. Isolation Check"}`,
				ContentType:      "application/json",
				OwnerStatus:      http.StatusCreated,
				StrangerStatus:   http.StatusCreated,
				StrangerIsolated: true,
				Secrets:          secrets,
			},
			{
				Name:        "read one practitioner",
				Method:      http.MethodGet,
				Path:        readTarget,
				MissingPath: practitionerURL(missingPractitionerID),
				Secrets:     secrets,
			},
			{
				Name:        "change one practitioner",
				Method:      http.MethodPatch,
				Path:        practitionerURL(updateTarget.ID),
				Body:        `{"phone":"+1 555 0199"}`,
				ContentType: "application/json",
				Headers:     updatePrecondition,
				MissingPath: practitionerURL(missingPractitionerID),
				Secrets:     updateSecrets,
			},
			{
				Name:        "delete one practitioner",
				Method:      http.MethodDelete,
				Path:        practitionerURL(deleteTarget.ID),
				Headers:     deletePrecondition,
				OwnerStatus: http.StatusNoContent,
				MissingPath: practitionerURL(missingPractitionerID),
				Secrets:     deleteSecrets,
			},
		},
	})
}

func TestEveryFacilityOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)

	updateTarget := owner.post(facilitiesURL(), `{"kind":"lab","name":"Owner Scoped Update Lab"}`).facility(t)
	deleteTarget := owner.post(facilitiesURL(), `{"kind":"lab","name":"Owner Scoped Delete Lab"}`).facility(t)

	updatePrecondition := map[string]string{"If-Match": owner.get(facilityURL(updateTarget.ID)).etag(t)}
	deletePrecondition := map[string]string{"If-Match": owner.get(facilityURL(deleteTarget.ID)).etag(t)}

	readTarget := facilityURL(testsupport.AccountAFacilityPracticeID)

	secrets := []string{testsupport.AccountAFacilityPracticeID, "Riverside Family Practice", testsupport.AccountAID}
	updateSecrets := []string{updateTarget.ID, "Owner Scoped Update Lab", testsupport.AccountAID}
	deleteSecrets := []string{deleteTarget.ID, "Owner Scoped Delete Lab", testsupport.AccountAID}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         owner.handler,
		Owner:           bearer(t, owner.app, testsupport.AccountAEmail),
		Stranger:        bearer(t, owner.app, testsupport.AccountBEmail),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:             "list facilities",
				Method:           http.MethodGet,
				Path:             facilitiesURL() + "?limit=100",
				StrangerIsolated: true,
				Secrets:          secrets,
			},
			{
				Name:             "create a facility",
				Method:           http.MethodPost,
				Path:             facilitiesURL(),
				Body:             `{"kind":"lab","name":"Isolation Check Lab"}`,
				ContentType:      "application/json",
				OwnerStatus:      http.StatusCreated,
				StrangerStatus:   http.StatusCreated,
				StrangerIsolated: true,
				Secrets:          secrets,
			},
			{
				Name:        "read one facility",
				Method:      http.MethodGet,
				Path:        readTarget,
				MissingPath: facilityURL(missingFacilityID),
				Secrets:     secrets,
			},
			{
				Name:        "change one facility",
				Method:      http.MethodPatch,
				Path:        facilityURL(updateTarget.ID),
				Body:        `{"city":"Abuja"}`,
				ContentType: "application/json",
				Headers:     updatePrecondition,
				MissingPath: facilityURL(missingFacilityID),
				Secrets:     updateSecrets,
			},
			{
				Name:        "delete one facility",
				Method:      http.MethodDelete,
				Path:        facilityURL(deleteTarget.ID),
				Headers:     deletePrecondition,
				OwnerStatus: http.StatusNoContent,
				MissingPath: facilityURL(missingFacilityID),
				Secrets:     deleteSecrets,
			},
		},
	})
}

// TestSearchNeverLeaksAnotherAccountsDirectoryEntry is FR-037, US5-6 and
// SC-014 as a dedicated test rather than a case buried in a list assertion:
// `?q=` matching another account's practitioner or facility name must answer
// items: [] and never the row itself.
func TestSearchNeverLeaksAnotherAccountsDirectoryEntry(t *testing.T) {
	t.Parallel()

	stranger := newCaller(t).as(testsupport.AccountBEmail)

	practitioners := stranger.get(practitionersURL() + "?q=Ngozi")
	require.Equal(t, http.StatusOK, practitioners.Status, practitioners.Body)
	assert.Contains(t, practitioners.Body, `"items":[]`)
	assert.NotContains(t, practitioners.Body, testsupport.AccountAPractitionerID)

	facilities := stranger.get(facilitiesURL() + "?q=Riverside")
	require.Equal(t, http.StatusOK, facilities.Status, facilities.Body)
	assert.Contains(t, facilities.Body, `"items":[]`)
	assert.NotContains(t, facilities.Body, testsupport.AccountAFacilityPracticeID)
}
