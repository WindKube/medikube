package stream_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web/views/ids"
)

// FR-007 against a real instance, a real token and a real socket: "the ended
// session MUST NOT be usable again from anywhere it was still open."
//
// An open stream was the one place it stayed usable. Every ordinary request
// re-derives its actor from the token on every call and a revoked one is
// stopped by PocketBase's loadAuthToken; a stream captured its actor once at
// subscribe and never asked again, so revoking the session — which is what a
// password change does, by re-randomising the collection's token key — answered
// 401 everywhere and delivered rows written afterwards, in full and rendered,
// to the connection that was already open.
//
// Sign-out is US2's. The agent who builds it will watch the 401s and will never
// open internal/web/stream, which is why this is closed here.

// endOfStream is how long a revoked stream is given to close. It is a bound and
// not a threshold: the heartbeat is 50ms in these tests and the check runs on
// every beat and every event, so a connection still open after this is one that
// is never closing.
const endOfStream = 10 * time.Second

// revoke ends every session an account holds, the way a person does it: change
// the password. PocketBase re-randomises the record's token key on a password
// or email change (core/record_model.go:1449), and every token ever signed for
// that account dies with it.
func revoke(t *testing.T, medikube *instance, email string) {
	t.Helper()

	record, err := medikube.App.FindAuthRecordByEmail("users", email)
	require.NoError(t, err)

	before := record.TokenKey()

	record.SetPassword("a-different-" + testsupport.Password)
	require.NoError(t, medikube.App.Save(record))

	require.NotEqual(t, before, record.TokenKey(),
		"the password change did not rotate the token key, so nothing was revoked and this test proves nothing")
}

// write commits one medication without going through the API, which is what a
// revoked caller can no longer do. The post-commit hook publishes it to the hub
// exactly as an API write would, so the stream is offered the same event.
func write(t *testing.T, medikube *instance, ownerID, name string) string {
	t.Helper()

	collection, err := medikube.App.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	record := core.NewRecord(collection)
	require.NoError(t, store.MedicationToRecord(record, clinical.Medication{
		OwnerID: ownerID,
		Name:    name,
		Status:  clinical.TherapyStatusActive,
	}))
	require.NoError(t, medikube.App.Save(record))

	return record.Id
}

// get is one ordinary request, so the test can show the token is dead
// everywhere else before asserting it is dead here too.
func (i *instance) get(t *testing.T, token, path string) int {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, i.base+path, nil)
	require.NoError(t, err)

	request.Header.Set("Authorization", token)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	_, _ = io.Copy(io.Discard, response.Body)

	return response.StatusCode
}

// closed waits for the stream to end and reports everything it delivered on the
// way out, and whether it ended at all.
//
// It returns rather than failing on the timeout, because the two things that go
// wrong here are different failures and the caller has to be able to assert
// them in the right order: a stream that stayed open is a lifetime bug, and a
// row delivered on it is the disclosure. Fail on the timeout and the
// disclosure is never named.
func (s *session) closed(within time.Duration) ([]frame, bool) {
	s.t.Helper()

	var collected []frame

	deadline := time.After(within)

	for {
		select {
		case f, open := <-s.frames:
			if !open {
				return collected, true
			}

			collected = append(collected, f)
		case <-deadline:
			return collected, false
		}
	}
}

func TestARevokedSessionReceivesNothingMoreAndItsOpenStreamCloses(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)

	watching := medikube.open(t, amara, "")
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	// The control. Without it a stream that was broken from the start would
	// pass every assertion below.
	before, _ := medikube.create(t, amara, "Amoxicillin")
	require.Equal(t, "#"+ids.RecordRow(kind.Medication, before), watching.nextPatch(patchDeadline).selector(),
		"the stream was not working before the revocation, so what follows proves nothing")

	revoke(t, medikube, testsupport.AccountAEmail)

	// The token is genuinely dead for every ordinary request, which is the
	// state in which the stream used to keep running.
	require.Equal(t, http.StatusUnauthorized,
		medikube.get(t, amara, "/api/v1/records/"+kind.Medication.Segment()),
		"the session was not actually revoked")

	after := write(t, medikube, testsupport.AccountAID, "Bisoprolol")

	delivered, ended := watching.closed(endOfStream)

	// The disclosure first. It is the reason any of this matters, and a stream
	// that never closed would otherwise fail on the lifetime and never say
	// what it had delivered.
	for _, f := range delivered {
		assert.NotContains(t, f.selector(), after,
			"the revoked session was patched with a row written after it ended")
		assert.NotContains(t, f.elements(), after,
			"the revoked session was sent the content of a row written after it ended")
		assert.NotContains(t, f.elements(), "Bisoprolol",
			"the revoked session was sent a medication written after it ended")
	}

	assert.Truef(t, ended, "the revoked session still held an open stream %s later", endOfStream)
}

// A second stream opened by an account whose session was never touched keeps
// running. Ending every stream on the instance would satisfy the assertion
// above and would be a denial of service wearing its clothes.
func TestRevokingOneAccountsSessionLeavesAnotherAccountsStreamAlone(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)
	boris := medikube.token(t, testsupport.AccountBEmail)

	watchingA := medikube.open(t, amara, "")
	watchingB := medikube.open(t, boris, "")
	require.Equal(t, http.StatusOK, watchingA.Response.StatusCode)
	require.Equal(t, http.StatusOK, watchingB.Response.StatusCode)

	revoke(t, medikube, testsupport.AccountAEmail)

	_, endedA := watchingA.closed(endOfStream)
	assert.True(t, endedA, "the revoked account's stream stayed open")

	// Boris's own write, after Amara's revocation, still reaches Boris.
	created, _ := medikube.create(t, boris, "Ciclosporin")

	assert.Equal(t, "#"+ids.RecordRow(kind.Medication, created), watchingB.nextPatch(patchDeadline).selector(),
		"revoking one account's session closed another account's stream")
}

// A token minted after the revocation works, which is what signing in again
// looks like. Without this the fix would be indistinguishable from an account
// that can never stream again.
func TestASessionOpenedAfterTheRevocationStreamsNormally(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	revoke(t, medikube, testsupport.AccountAEmail)

	fresh := medikube.token(t, testsupport.AccountAEmail)

	watching := medikube.open(t, fresh, "")
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	created, _ := medikube.create(t, fresh, "Dexamethasone")

	assert.Equal(t, "#"+ids.RecordRow(kind.Medication, created), watching.nextPatch(patchDeadline).selector())
}

// The beat is the other half of the re-check, and on its own it is the half
// that matters most: nobody writes to a quiet account, so a stream that only
// re-checked on an event would outlive the revocation until a write that never
// comes. Nothing is published here at all.
//
// The stream is also the resource the revocation was supposed to release. This
// route has no rate limiter, no write deadline and no concurrency cap, so a
// connection that does not end is one that does not end.
func TestARevokedStreamClosesOnAnAccountNothingIsWrittenTo(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)

	watching := medikube.open(t, amara, "")
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	require.True(t, watching.next(patchDeadline).isHeartbeat(), "a stream must open with a heartbeat")

	revoke(t, medikube, testsupport.AccountAEmail)

	_, ended := watching.closed(endOfStream)

	assert.Truef(t, ended,
		"the revoked stream was still beating %s later, with no event ever published to end it", endOfStream)
}

// A stream carrying somebody's records must stop when nothing can say whose
// they still are. The instance is torn down under an open stream, which is the
// bluntest version of "the identity can no longer be re-checked".
func TestAStreamEndsWhenItsIdentityCanNoLongerBeReChecked(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)

	watching := medikube.open(t, amara, "")
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	// Deleting the account is the same question with a different answer: there
	// is no auth record left for the token to resolve to.
	record, err := medikube.App.FindAuthRecordByEmail("users", testsupport.AccountAEmail)
	require.NoError(t, err)
	require.NoError(t, medikube.App.Delete(record))

	delivered, ended := watching.closed(endOfStream)

	for _, f := range delivered {
		assert.NotContains(t, strings.ToLower(f.elements()), "medication",
			"a stream kept rendering rows for an account that no longer exists")
	}

	assert.Truef(t, ended, "the stream outlived the account it was opened by")
}
