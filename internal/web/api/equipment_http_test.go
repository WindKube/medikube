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

func equipmentCollectionURL() string      { return "/api/v1/records/" + kind.Equipment.Segment() }
func equipmentRecordURL(id string) string { return equipmentCollectionURL() + "/" + id }

// TestEveryEquipmentOperationIsOwnerScoped is T125's FR-092 matrix for
// equipment: an account that does not own the patient is refused on
// listing, creating, reading, correcting and deleting an item.
func TestEveryEquipmentOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	owner := &caller{t: t, app: instance.App, handler: handler, token: testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)}

	target := equipmentRecordURL(seed.EquipmentOverdueID)

	current := owner.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	precondition := map[string]string{"If-Match": current.etag(t)}

	// The deletion needs its own subject: the change leg above succeeds for
	// the owner and moves the version, so a deletion aimed at the same
	// record would be refused on a stale precondition rather than on
	// ownership.
	removable := equipmentRecordURL(seed.EquipmentDueSoonID)

	removableNow := owner.get(removable)
	require.Equal(t, http.StatusOK, removableNow.Status, removableNow.Body)

	removablePrecondition := map[string]string{"If-Match": removableNow.etag(t)}

	secrets := []string{seed.EquipmentOverdueID, "CPAP machine"}
	removableSecrets := []string{seed.EquipmentDueSoonID, "Nebulizer"}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         handler,
		Owner:           bearer(t, instance.App, testsupport.AccountAEmail),
		Stranger:        bearer(t, instance.App, testsupport.AccountBEmail),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:    "list equipment",
				Method:  http.MethodGet,
				Path:    equipmentCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100",
				Secrets: secrets,
			},
			{
				Name:        "record an item",
				Method:      http.MethodPost,
				Path:        equipmentCollectionURL(),
				Body:        `{"patient":"` + testsupport.AccountAPatientSelfID + `","name":"Wheelchair","type":"wheelchair"}`,
				OwnerStatus: http.StatusCreated,
				Secrets:     secrets,
			},
			{
				Name:        "read one",
				Method:      http.MethodGet,
				Path:        target,
				MissingPath: equipmentRecordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "change one",
				Method:      http.MethodPatch,
				Path:        target,
				Body:        `{"model":"AirSense 12"}`,
				ContentType: "application/json",
				Headers:     precondition,
				MissingPath: equipmentRecordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "delete one",
				Method:      http.MethodDelete,
				Path:        removable,
				Headers:     removablePrecondition,
				OwnerStatus: http.StatusNoContent,
				MissingPath: equipmentRecordURL(missingID),
				Secrets:     removableSecrets,
			},
		},
	})
}
