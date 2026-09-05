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

func familyMemberCollectionURL() string      { return "/api/v1/records/" + kind.FamilyMember.Segment() }
func familyMemberRecordURL(id string) string { return familyMemberCollectionURL() + "/" + id }

// TestEveryFamilyMemberOperationIsOwnerScoped is US10's FR-092 matrix for
// family_member: an account that does not own the patient is refused on
// listing, creating, reading, correcting and deleting a relative.
func TestEveryFamilyMemberOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	owner := &caller{t: t, app: instance.App, handler: handler, token: testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)}

	target := familyMemberRecordURL(seed.FamilyMemberGrandmotherID)

	current := owner.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	precondition := map[string]string{"If-Match": current.etag(t)}

	// The deletion needs its own subject: the change leg above succeeds for
	// the owner and moves the version, so a deletion aimed at the same
	// record would be refused on a stale precondition rather than on
	// ownership.
	removable := familyMemberRecordURL(seed.FamilyMemberUncleID)

	removableNow := owner.get(removable)
	require.Equal(t, http.StatusOK, removableNow.Status, removableNow.Body)

	removablePrecondition := map[string]string{"If-Match": removableNow.etag(t)}

	secrets := []string{seed.FamilyMemberGrandmotherID, "Adaeze Okonkwo"}
	removableSecrets := []string{seed.FamilyMemberUncleID, "Emeka Okonkwo"}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         handler,
		Owner:           bearer(t, instance.App, testsupport.AccountAEmail),
		Stranger:        bearer(t, instance.App, testsupport.AccountBEmail),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:    "list family history",
				Method:  http.MethodGet,
				Path:    familyMemberCollectionURL() + "?patient=" + seed.AccountAPatientParentID + "&limit=100",
				Secrets: secrets,
			},
			{
				Name:        "record a relative",
				Method:      http.MethodPost,
				Path:        familyMemberCollectionURL(),
				Body:        `{"patient":"` + seed.AccountAPatientParentID + `","name":"Nkechi Okonkwo","relationship":"aunt"}`,
				OwnerStatus: http.StatusCreated,
				Secrets:     secrets,
			},
			{
				Name:        "read one",
				Method:      http.MethodGet,
				Path:        target,
				MissingPath: familyMemberRecordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "change one",
				Method:      http.MethodPatch,
				Path:        target,
				Body:        `{"name":"Adaeze Okonkwo-Van"}`,
				ContentType: "application/json",
				Headers:     precondition,
				MissingPath: familyMemberRecordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "delete one",
				Method:      http.MethodDelete,
				Path:        removable,
				Headers:     removablePrecondition,
				OwnerStatus: http.StatusNoContent,
				MissingPath: familyMemberRecordURL(missingID),
				Secrets:     removableSecrets,
			},
		},
	})
}
