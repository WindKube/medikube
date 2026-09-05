package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

func decodedPatient(t *testing.T, r response) api.Patient {
	t.Helper()

	var p api.Patient
	require.NoError(t, json.Unmarshal(r.rawBody, &p))

	return p
}

// T147, FR-049, US6-2, US6-3, SC-010. The 204 leg, and permanence: gone means
// gone, never a soft flag a later read could see past.
func TestDeletePatientAnswers204AndTheRecordIsReallyGone(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	subject := decodedPatient(t, caller.get(patientURL(testsupport.AccountAPatientChildID)))
	version := caller.get(patientURL(subject.ID)).etag(t)

	answer := caller.delete(patientURL(subject.ID), version)
	require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)
	assert.Empty(t, answer.Body)

	missing := caller.get(patientURL(subject.ID))
	assert.Equal(t, http.StatusNotFound, missing.Status)
}

// FR-051, US6-4. The self-record is refused with 409 and an explanation that
// closing the account, not a delete, is what removes it — and nothing about
// the record is touched.
func TestDeletePatientRefusesTheSelfRecordWith409(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	subject := testsupport.AccountAPatientSelfID
	version := caller.get(patientURL(subject)).etag(t)

	answer := caller.delete(patientURL(subject), version)
	require.Equal(t, http.StatusConflict, answer.Status, answer.Body)
	assert.Contains(t, answer.Body, web.CodeSelfRecordProtected)

	stillThere := caller.get(patientURL(subject))
	assert.Equal(t, http.StatusOK, stillThere.Status)
}

// FR-049. A stale If-Match is 412, and the record survives it.
func TestDeletePatientRefusesAStaleIfMatchWith412(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	subject := testsupport.AccountAPatientChildID

	answer := caller.delete(patientURL(subject), `"not-the-real-etag"`)
	require.Equal(t, http.StatusPreconditionFailed, answer.Status, answer.Body)
	assert.Contains(t, answer.Body, web.CodeVersionMismatch)

	stillThere := caller.get(patientURL(subject))
	assert.Equal(t, http.StatusOK, stillThere.Status)
}

// T144, FR-049. A missing If-Match is 422, not a silent overwrite refusal
// dressed as something else.
func TestDeletePatientRefusesAMissingIfMatchWith422(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	subject := testsupport.AccountAPatientChildID

	answer := caller.delete(patientURL(subject), "")
	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	stillThere := caller.get(patientURL(subject))
	assert.Equal(t, http.StatusOK, stillThere.Status)
}

// T148, FR-042, FR-045. Account B deleting Account A's patient is 404,
// indistinguishable from a genuine miss; nothing is deleted, and the attempt
// itself is audited.
func TestDeletePatientRefusesAStrangerAsNotFoundAndAudits(t *testing.T) {
	t.Parallel()

	callerA := newCaller(t)
	callerB := callerA.as(testsupport.AccountBEmail)

	subject := testsupport.AccountAPatientChildID
	version := callerA.get(patientURL(subject)).etag(t)

	require.Empty(t, apitest.Events(t, callerB.app),
		"the fixture already holds audit rows, so nothing below is attributable to this request")

	answer := callerB.delete(patientURL(subject), version)
	require.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	assert.NotContains(t, answer.Body, subject)

	stillThere := callerA.get(patientURL(subject))
	assert.Equal(t, http.StatusOK, stillThere.Status, "the stranger's refused attempt deleted the record anyway")

	events := apitest.Events(t, callerB.app)
	require.Len(t, events, 1, "the refusal wrote no row, or more than one")
	assert.Equal(t, audit.ActionAccessDenied, events[0].Action)
	assert.Equal(t, audit.TargetKindPatient, events[0].TargetKind)
	assert.Equal(t, subject, events[0].TargetID)
	assert.Equal(t, testsupport.AccountBID, events[0].ActorID)
}

// FR-038, FR-045, US6-5, SC-009. Exactly one delete/patient audit row, and it
// carries no name — the deleted person's own identity is nowhere on it but
// the id.
func TestDeletePatientWritesExactlyOneAuditRowCarryingNoName(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	subject := testsupport.AccountAPatientChildID
	version := caller.get(patientURL(subject)).etag(t)

	require.Empty(t, apitest.Events(t, caller.app),
		"the fixture already holds audit rows, so nothing below is attributable to this request")

	answer := caller.delete(patientURL(subject), version)
	require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)

	events := apitest.Events(t, caller.app)
	require.Len(t, events, 1, "the delete wrote no row, or more than one")

	event := events[0]
	assert.Equal(t, audit.ActionDelete, event.Action)
	assert.Equal(t, audit.TargetKindPatient, event.TargetKind)
	assert.Equal(t, subject, event.TargetID)
	assert.NotContains(t, event.Action, "Chiamaka")
}

// 401 anonymous.
func TestDeletePatientIsUnauthenticatedForAnonymous(t *testing.T) {
	t.Parallel()

	caller := newCaller(t).anonymous()

	answer := caller.delete(patientURL(testsupport.AccountAPatientChildID), `"anything"`)
	assert.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
}
