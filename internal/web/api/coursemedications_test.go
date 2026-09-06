package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/apitest"
)

// courseMedicationListURL and courseMedicationItemURL build the two shapes
// contracts/treatment-medications.md publishes: nested one level under the
// treatment a course medication attaches to, rather than under the generic
// /api/v1/records/{kind}/{id} family every other kind shares (internal/httproute
// dispatches treatments there too, but the medications join is its own three
// routes, not one of the six recordRoutes publishes per kind).
func courseMedicationListURL(treatmentID string) string {
	return "/api/v1/records/" + kind.Treatment.Collection() + "/" + treatmentID + "/medications"
}

func courseMedicationItemURL(treatmentID, medicationID string) string {
	return courseMedicationListURL(treatmentID) + "/" + medicationID
}

func treatmentRecordURL(id string) string {
	return "/api/v1/records/" + kind.Treatment.Segment() + "/" + id
}

// TestEveryCourseMedicationOperationIsOwnerScoped is T147's FR-032/FR-069
// matrix applied to the one payload-carrying join US6 adds: a stranger to the
// treatment's patient is refused on listing, attaching and detaching a
// medication, and the refusal is byte-identical to a genuine miss.
func TestEveryCourseMedicationOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	owner := &caller{t: t, app: instance.App, handler: handler, token: testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)}

	treatmentID := testsupport.TreatmentNameOnlyID
	medicationID := testsupport.NameOnlyMedicationID

	current := owner.get(treatmentRecordURL(treatmentID))
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	precondition := map[string]string{"If-Match": current.etag(t)}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         handler,
		Owner:           bearer(t, instance.App, testsupport.AccountAEmail),
		Stranger:        bearer(t, instance.App, testsupport.AccountBEmail),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:    "list the course's medications",
				Method:  http.MethodGet,
				Path:    courseMedicationListURL(treatmentID),
				Secrets: []string{treatmentID, medicationID},
			},
			{
				Name:        "attach a medication to the course",
				Method:      http.MethodPut,
				Path:        courseMedicationItemURL(treatmentID, medicationID),
				Body:        `{"dosage":"3mg"}`,
				ContentType: "application/json",
				Headers:     precondition,
				OwnerStatus: http.StatusCreated,
				Secrets:     []string{treatmentID, medicationID},
			},
			{
				Name:        "detach the medication",
				Method:      http.MethodDelete,
				Path:        courseMedicationItemURL(treatmentID, medicationID),
				Headers:     precondition,
				OwnerStatus: http.StatusNoContent,
				Secrets:     []string{treatmentID, medicationID},
			},
		},
	})
}

// TestAttachingACourseMedicationIsVisibleFromTheMedicationSideAndDetachClearsBoth
// is T150/FR-055/FR-058's join-relation proof: PUT attaches a medication from
// the treatment's own address, the attachment is immediately visible reading
// the medication's own back-relation count (the other end medication's page
// resolves into a removable link), and DELETE — the op medication's own Remove
// button drives — clears it from both, leaving the join row gone rather than
// merely hidden.
func TestAttachingACourseMedicationIsVisibleFromTheMedicationSideAndDetachClearsBoth(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)

	treatmentID := seed.CourseMedicationTreatmentID
	medicationID := testsupport.NameOnlyMedicationID
	medicationTarget := recordURL(medicationID)

	before := owner.get(medicationTarget)
	require.Equal(t, http.StatusOK, before.Status, before.Body)

	var beforeBody referencesEnvelope
	before.decode(t, &beforeBody)
	require.Equal(t, 0, beforeBody.References.Total, "this medication starts with no course attachments")

	current := owner.get(treatmentRecordURL(treatmentID))
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	attached := owner.do(http.MethodPut, courseMedicationItemURL(treatmentID, medicationID),
		`{"dosage":"3mg"}`, map[string]string{"If-Match": current.etag(t)})
	require.Equal(t, http.StatusCreated, attached.Status, attached.Body)

	afterAttach := owner.get(medicationTarget)
	require.Equal(t, http.StatusOK, afterAttach.Status, afterAttach.Body)

	var afterAttachBody referencesEnvelope
	afterAttach.decode(t, &afterAttachBody)
	assert.Equal(t, 1, afterAttachBody.References.Total, "attaching from the treatment side is visible from the medication side")

	detached := owner.delete(courseMedicationItemURL(treatmentID, medicationID), current.etag(t))
	require.Equal(t, http.StatusNoContent, detached.Status, detached.Body)

	afterDetach := owner.get(medicationTarget)
	require.Equal(t, http.StatusOK, afterDetach.Status, afterDetach.Body)

	var afterDetachBody referencesEnvelope
	afterDetach.decode(t, &afterDetachBody)
	assert.Equal(t, 0, afterDetachBody.References.Total, "detaching clears the medication's back-relation too")
}
