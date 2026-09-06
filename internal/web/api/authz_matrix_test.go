package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/testsupport"
)

// T206, FR-081, FR-082, FR-092, SC-004. One table-driven matrix, run for
// every registered kind, that a non-owning account is refused reaching a
// record — on every one of the six generic record operations
// (listRecords, listRecordsOfKind, createRecord, getRecord, updateRecord,
// deleteRecord) — however it is approached: by its own id, filtered by tag,
// filtered by search term, through the timeline, through a status view
// (where the kind has one), and through another kind's own back-relation
// listing (where one exists). Every refusal is decided from the record's
// stored patient, never from users.active_patient (data-model §6): the
// stranger below never sets an active patient at all.
//
// It reuses testsupport.RunOwnershipMatrix for the six operations (the same
// idiom every per-kind file already runs one of), and plain caller calls for
// the reach-path legs, because those five surfaces are narrowings of an
// already patient-scoped list or of a page route and each needs only a
// status check and a secrecy check, not a byte-identical comparison.
func TestAuthzMatrixAcrossEveryKindAndEveryReachPath(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)
	patientID := testsupport.AccountAPatientChildID

	cases := auditKindCases()
	require.Len(t, cases, len(kind.Kinds()), "every registered kind must have a case in auditKindCases")

	tag := owner.post(tagsURL(), `{"name":"authz-matrix-tag"}`)
	require.Equal(t, http.StatusCreated, tag.Status, tag.Body)
	tagID := tag.tag(t).ID

	for _, k := range kind.Kinds() {
		kase, ok := cases[k]
		require.Truef(t, ok, "%s has no case in auditKindCases", k)

		t.Run(k.Segment(), func(t *testing.T) {
			runAuthzSixOperations(t, owner, stranger, k, kase, patientID)
			runAuthzReachPaths(t, owner, stranger, k, kase, patientID, tagID)
		})
	}

	runAuthzBackrelationLeg(t, owner, stranger)
}

// runAuthzSixOperations is the direct reach path: the six generic record
// operations, run as owner/stranger/guest through testsupport's own matrix
// runner. Order matters — delete last, so an earlier case never addresses a
// record the last one already destroyed.
func runAuthzSixOperations(
	t *testing.T, owner, stranger *caller, k kind.Kind, kase auditKindCase, patientID string,
) {
	t.Helper()

	base := "/api/v1/records/" + k.Segment()
	sentinel := "authz-matrix-sentinel-" + k.Segment()

	created := owner.post(base, kase.create(patientID, sentinel))
	require.Equalf(t, http.StatusCreated, created.Status, "%s: seeding the matrix's own record: %s", k, created.Body)

	id := created.items1(t).ID
	etag := created.etag(t)

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         stranger.handler,
		Owner:           testsupport.BearerToken(owner.token),
		Stranger:        testsupport.BearerToken(stranger.token),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:    "listRecords",
				Method:  http.MethodGet,
				Path:    crossKindURL() + "?patient=" + patientID + "&kind=" + k.Segment(),
				Secrets: []string{id, sentinel},
			},
			{
				Name:    "listRecordsOfKind",
				Method:  http.MethodGet,
				Path:    base + "?patient=" + patientID,
				Secrets: []string{id, sentinel},
			},
			{
				Name:        "createRecord",
				Method:      http.MethodPost,
				Path:        base,
				Body:        kase.create(patientID, sentinel),
				ContentType: "application/json",
				OwnerStatus: http.StatusCreated,
				Secrets:     []string{sentinel},
			},
			{
				Name:        "getRecord",
				Method:      http.MethodGet,
				Path:        base + "/" + id,
				MissingPath: base + "/" + missingID,
				Secrets:     []string{sentinel},
			},
			{
				Name:        "updateRecord",
				Method:      http.MethodPatch,
				Path:        base + "/" + id,
				Body:        fmt.Sprintf(`{%q:%q}`, kase.correctionField, sentinel+"-corrected"),
				ContentType: "application/json",
				Headers:     map[string]string{"If-Match": etag},
				OwnerStatus: http.StatusOK,
				Secrets:     []string{sentinel},
			},
		},
	})

	// deleteRecord runs on its own, after re-reading the version: the
	// updateRecord case above just moved it, as the owner leg of every case
	// before this one might have, and a stale If-Match would refuse the
	// owner for the wrong reason.
	current := owner.get(base + "/" + id)
	require.Equalf(t, http.StatusOK, current.Status, "%s: re-reading before delete: %s", k, current.Body)

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         stranger.handler,
		Owner:           testsupport.BearerToken(owner.token),
		Stranger:        testsupport.BearerToken(stranger.token),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:        "deleteRecord",
				Method:      http.MethodDelete,
				Path:        base + "/" + id,
				Headers:     map[string]string{"If-Match": current.etag(t)},
				OwnerStatus: http.StatusNoContent,
				Secrets:     []string{sentinel},
			},
		},
	})
}

// runAuthzReachPaths is the five indirect reach paths: a fresh record so the
// six-operation matrix's own delete does not interfere, tagged and left in
// place for the tag leg, checked as the stranger only (the owner leg is
// already proven above — this is about whether the narrowing itself is a
// second way in, not a repeat of ownership succeeding).
func runAuthzReachPaths(
	t *testing.T, owner, stranger *caller, k kind.Kind, kase auditKindCase, patientID, tagID string,
) {
	t.Helper()

	base := "/api/v1/records/" + k.Segment()
	sentinel := "authz-matrix-reach-" + k.Segment()

	created := owner.post(base, kase.create(patientID, sentinel))
	require.Equalf(t, http.StatusCreated, created.Status, "%s: seeding the reach-path record: %s", k, created.Body)

	tagged := owner.patch(base+"/"+created.items1(t).ID, fmt.Sprintf(`{"tags":[%q]}`, tagID), created.etag(t))
	require.Equalf(t, http.StatusOK, tagged.Status, "%s: tagging the reach-path record: %s", k, tagged.Body)

	t.Run("through a tag", func(t *testing.T) {
		resp := stranger.get(base + "?patient=" + patientID + "&tags=" + tagID)
		assertRefusedWithNoSecret(t, resp, sentinel)
	})

	t.Run("through search", func(t *testing.T) {
		resp := stranger.get(searchURL("patient=" + patientID + "&q=" + sentinel))
		assertRefusedWithNoSecret(t, resp, sentinel)
	})

	t.Run("through the timeline", func(t *testing.T) {
		resp := stranger.get("/timeline?patient=" + patientID + "&kind=" + k.Segment())
		assertRefusedWithNoSecret(t, resp, sentinel)
	})

	if view, ok := records.StatusViewFor(k); ok {
		t.Run("through a status view", func(t *testing.T) {
			resp := stranger.get(view.SmokeURL(patientID))
			assertRefusedWithNoSecret(t, resp, sentinel)
		})
	}
}

// runAuthzBackrelationLeg is the link reach path: medications and conditions
// are the two kinds link.Backrelations reads (internal/store/link), so a
// stranger reading either directly is where a back-relation count would
// leak, if it were going to. Every other kind's link is a plain relation
// field on the referencing side, already covered by runAuthzSixOperations'
// own getRecord case.
func runAuthzBackrelationLeg(t *testing.T, owner, stranger *caller) {
	t.Helper()

	patientID := testsupport.AccountAPatientChildID

	condition := owner.post("/api/v1/records/"+kind.Condition.Segment(),
		fmt.Sprintf(`{"patient":%q,"diagnosis":"authz-matrix-link-condition","status":"active"}`, patientID))
	require.Equal(t, http.StatusCreated, condition.Status, condition.Body)
	conditionID := condition.items1(t).ID

	encounter := owner.post("/api/v1/records/"+kind.Encounter.Segment(),
		fmt.Sprintf(`{"patient":%q,"reason":"authz-matrix-link-encounter","occurred_on":"2026-01-10","condition":%q}`,
			patientID, conditionID))
	require.Equal(t, http.StatusCreated, encounter.Status, encounter.Body)

	t.Run("a condition's own back-relation count", func(t *testing.T) {
		resp := stranger.get("/api/v1/records/" + kind.Condition.Segment() + "/" + conditionID)
		assertRefusedWithNoSecret(t, resp, conditionID)
	})
}

// assertRefusedWithNoSecret is every reach-path leg's own assertion: a
// stranger is never let in (2xx is a failure), and whatever the response
// carries, it does not carry the sentinel — the one column PHI would have to
// travel through to leak.
func assertRefusedWithNoSecret(t *testing.T, resp response, sentinel string) {
	t.Helper()

	assert.GreaterOrEqualf(t, resp.Status, http.StatusBadRequest,
		"a stranger reached something narrowed by patient, tag, search, timeline or status view: %d %s",
		resp.Status, resp.Body)
	assert.NotContainsf(t, resp.Body, sentinel, "the reach path disclosed %q", sentinel)
}
