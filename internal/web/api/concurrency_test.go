package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
)

// T211, FR-005, SC-009, spec Edge Cases. For every registered kind, a PATCH
// or DELETE built on a stale version answers 412 carrying the current
// representation and applies nothing — and the same rule holds for
// attaching or detaching a link (TestStaleIfMatchOnALinkChangeAnswers412)
// and for the course-medication upsert/delete
// (TestStaleIfMatchOnACourseMedicationAnswers412), both below.
func TestStaleIfMatchAcrossEveryKindAnswers412AndOverwritesNothing(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	patientID := testsupport.AccountAPatientChildID

	cases := auditKindCases()
	require.Len(t, cases, len(kind.Kinds()), "every registered kind must have a case in auditKindCases")

	for _, k := range kind.Kinds() {
		kase, ok := cases[k]
		require.Truef(t, ok, "%s has no case in auditKindCases", k)

		t.Run(k.Segment(), func(t *testing.T) {
			base := "/api/v1/records/" + k.Segment()
			sentinel := "concurrency-sentinel-" + k.Segment()

			created := owner.post(base, kase.create(patientID, sentinel))
			require.Equalf(t, http.StatusCreated, created.Status, "%s: create: %s", k, created.Body)
			id := created.items1(t).ID

			current := owner.get(base + "/" + id)
			require.Equalf(t, http.StatusOK, current.Status, "%s: read: %s", k, current.Body)

			t.Run("PATCH", func(t *testing.T) {
				answer := owner.patch(base+"/"+id,
					fmt.Sprintf(`{%q:%q}`, kase.correctionField, sentinel+"-mutated-by-a-stale-patch"), staleVersion)
				assertStalePrecondition412(t, k, "PATCH", current, answer)

				after := owner.get(base + "/" + id)
				require.Equalf(t, http.StatusOK, after.Status, "%s: re-read after the stale PATCH: %s", k, after.Body)
				assert.JSONEqf(t, current.Body, after.Body, "%s: the stale PATCH changed something", k)
			})

			t.Run("DELETE", func(t *testing.T) {
				answer := owner.delete(base+"/"+id, staleVersion)
				assertStalePrecondition412(t, k, "DELETE", current, answer)

				after := owner.get(base + "/" + id)
				require.Equalf(t, http.StatusOK, after.Status, "%s: the stale DELETE removed the row", k)
			})
		})
	}
}

// assertStalePrecondition412 is every kind's own assertion: 412, and the
// body's "current" member is byte-for-byte the representation the owner's
// own GET just returned — not a second request the caller has to make to
// find out what changed underneath them.
func assertStalePrecondition412(t *testing.T, k kind.Kind, op string, current, answer response) {
	t.Helper()

	require.Equalf(t, http.StatusPreconditionFailed, answer.Status, "%s: %s: %s", k, op, answer.Body)

	var currentBody map[string]any
	require.NoError(t, json.Unmarshal(current.rawBody, &currentBody))

	var answerBody struct {
		Current map[string]any `json:"current"`
	}
	require.NoError(t, json.Unmarshal(answer.rawBody, &answerBody))

	assert.Equalf(t, currentBody, answerBody.Current,
		"%s: %s: the 412 body did not carry the current representation", k, op)
}

// TestStaleIfMatchOnALinkChangeAnswers412 is T211's link leg: a stale
// version refuses attaching a medication to (or detaching one from) an
// allergy's own relation field exactly as it refuses any other PATCH — the
// precondition check runs before the patch is interpreted, so it does not
// matter which field the patch would have changed.
func TestStaleIfMatchOnALinkChangeAnswers412(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	patientID := testsupport.AccountAPatientChildID

	medication := owner.post("/api/v1/records/"+kind.Medication.Segment(),
		fmt.Sprintf(`{"patient":%q,"name":"concurrency-link-medication"}`, patientID))
	require.Equal(t, http.StatusCreated, medication.Status, medication.Body)
	medicationID := medication.items1(t).ID

	allergy := owner.post("/api/v1/records/"+kind.Allergy.Segment(),
		fmt.Sprintf(`{"patient":%q,"allergen":"concurrency-link-allergy","severity":"mild"}`, patientID))
	require.Equal(t, http.StatusCreated, allergy.Status, allergy.Body)
	allergyID := allergy.items1(t).ID
	allergyURL := "/api/v1/records/" + kind.Allergy.Segment() + "/" + allergyID

	current := owner.get(allergyURL)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	medicationsField := kind.Medication.Collection()

	t.Run("attach", func(t *testing.T) {
		answer := owner.patch(allergyURL, fmt.Sprintf(`{%q:[%q]}`, medicationsField, medicationID), staleVersion)
		assertStalePrecondition412(t, kind.Allergy, "attach", current, answer)
	})

	attached := owner.patch(allergyURL, fmt.Sprintf(`{%q:[%q]}`, medicationsField, medicationID), current.etag(t))
	require.Equal(t, http.StatusOK, attached.Status, attached.Body)

	t.Run("detach", func(t *testing.T) {
		answer := owner.patch(allergyURL, fmt.Sprintf(`{%q:[]}`, medicationsField), staleVersion)
		assertStalePrecondition412(t, kind.Allergy, "detach", attached, answer)
	})
}

// TestStaleIfMatchOnACourseMedicationAnswers412 is T211's other named leg:
// treatment_medications is not a kind.Kind, served by its own three routes
// rather than the generic six-operation family, so its own precondition
// handling (internal/web/api/coursemedications.go's failure method) needs
// its own proof rather than inheriting the one above.
func TestStaleIfMatchOnACourseMedicationAnswers412(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	patientID := testsupport.AccountAPatientChildID

	treatment := owner.post("/api/v1/records/"+kind.Treatment.Segment(),
		fmt.Sprintf(`{"patient":%q,"name":"concurrency-course-medication","started_on":"2026-01-10"}`, patientID))
	require.Equal(t, http.StatusCreated, treatment.Status, treatment.Body)
	treatmentID := treatment.items1(t).ID

	medication := owner.post("/api/v1/records/"+kind.Medication.Segment(),
		fmt.Sprintf(`{"patient":%q,"name":"concurrency-course-medication"}`, patientID))
	require.Equal(t, http.StatusCreated, medication.Status, medication.Body)
	medicationID := medication.items1(t).ID

	current := owner.get(treatmentRecordURL(treatmentID))
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	t.Run("upsert", func(t *testing.T) {
		answer := owner.do(http.MethodPut, courseMedicationItemURL(treatmentID, medicationID), `{"dosage":"3mg"}`,
			map[string]string{"If-Match": staleVersion})
		assertStalePrecondition412(t, kind.Treatment, "upsert", current, answer)
	})

	attached := owner.do(http.MethodPut, courseMedicationItemURL(treatmentID, medicationID), `{"dosage":"3mg"}`,
		map[string]string{"If-Match": current.etag(t)})
	require.Equal(t, http.StatusCreated, attached.Status, attached.Body)

	// The attach above just changed the treatment's own reference count, so
	// the baseline for the delete leg is a fresh read, not the one taken
	// before it.
	afterAttach := owner.get(treatmentRecordURL(treatmentID))
	require.Equal(t, http.StatusOK, afterAttach.Status, afterAttach.Body)

	t.Run("delete", func(t *testing.T) {
		answer := owner.delete(courseMedicationItemURL(treatmentID, medicationID), staleVersion)
		assertStalePrecondition412(t, kind.Treatment, "delete", afterAttach, answer)
	})
}
