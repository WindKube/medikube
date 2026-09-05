package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/apitest"
)

func insuranceCollectionURL() string      { return "/api/v1/records/" + kind.Insurance.Segment() }
func insuranceRecordURL(id string) string { return insuranceCollectionURL() + "/" + id }

// TestEveryInsuranceOperationIsOwnerScoped is T125's FR-092 matrix for
// insurance: an account that does not own the patient is refused on
// listing, creating, reading, correcting and deleting a policy.
func TestEveryInsuranceOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	owner := &caller{t: t, app: instance.App, handler: handler, token: testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)}

	target := insuranceRecordURL(seed.InsurancePrimaryID)

	current := owner.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	precondition := map[string]string{"If-Match": current.etag(t)}

	// The deletion needs its own subject: the change leg above succeeds for
	// the owner and moves the version, so a deletion aimed at the same
	// record would be refused on a stale precondition rather than on
	// ownership.
	removable := insuranceRecordURL(seed.InsuranceExpiringID)

	removableNow := owner.get(removable)
	require.Equal(t, http.StatusOK, removableNow.Status, removableNow.Body)

	removablePrecondition := map[string]string{"If-Match": removableNow.etag(t)}

	secrets := []string{seed.InsurancePrimaryID, "Meridian Health Assurance"}
	removableSecrets := []string{seed.InsuranceExpiringID, "Riverbend Dental Trust"}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         handler,
		Owner:           bearer(t, instance.App, testsupport.AccountAEmail),
		Stranger:        bearer(t, instance.App, testsupport.AccountBEmail),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:    "list insurance",
				Method:  http.MethodGet,
				Path:    insuranceCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100",
				Secrets: secrets,
			},
			{
				Name:   "record a policy",
				Method: http.MethodPost,
				Path:   insuranceCollectionURL(),
				Body: `{"patient":"` + testsupport.AccountAPatientSelfID + `","type":"medical","company":"Acme Health Insurance",` +
					`"member_name":"A Person","member_id":"MEM-1","effective_on":"2024-01-01"}`,
				OwnerStatus: http.StatusCreated,
				Secrets:     secrets,
			},
			{
				Name:        "read one",
				Method:      http.MethodGet,
				Path:        target,
				MissingPath: insuranceRecordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "change one",
				Method:      http.MethodPatch,
				Path:        target,
				Body:        `{"company":"Renamed Insurer"}`,
				ContentType: "application/json",
				Headers:     precondition,
				MissingPath: insuranceRecordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "delete one",
				Method:      http.MethodDelete,
				Path:        removable,
				Headers:     removablePrecondition,
				OwnerStatus: http.StatusNoContent,
				MissingPath: insuranceRecordURL(missingID),
				Secrets:     removableSecrets,
			},
		},
	})
}
