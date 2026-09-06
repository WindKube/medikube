package stream_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
	"medikube/internal/web/views/ids"
)

// T154, and the test contracts/streams.md calls mandatory: two sessions on two
// accounts stream simultaneously and a write on one produces ZERO frames on the
// other (FR-030, FR-032).
//
// The hub fans every event out to every open stream — that is what it is for —
// so the only thing standing between one account's write and another account's
// browser is the per-subscriber authorization inside the loop. A check hoisted
// out of the loop, an event carrying a record body, or a removal sent on a
// not-found would each break this and none of them would break anything else.
//
// "Zero frames" is asserted with a barrier rather than with a sleep. After the
// write under test, a second write is made that the other subscriber MUST
// receive; once that one has arrived, the first has provably been through the
// same loop, so silence about it is a fact. A sleep long enough never to miss a
// frame is long enough to be worth skipping, and one short enough to be quick
// misses the frame it was looking for.

const patchDeadline = 5 * time.Second

func TestAWriteOnOneAccountReachesNoOtherAccountsStream(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)
	boris := medikube.token(t, testsupport.AccountBEmail)

	watchingA := medikube.open(t, amara, "?patient="+testsupport.AccountAPatientSelfID)
	watchingB := medikube.open(t, boris, "?patient="+testsupport.AccountBPatientSelfID)

	require.Equal(t, 200, watchingA.Response.StatusCode)
	require.Equal(t, 200, watchingB.Response.StatusCode)

	// The write under test, on Amara's account.
	_, _ = medikube.create(t, amara, testsupport.AccountAPatientSelfID, "Amoxicillin")

	// The barrier, on Boris's own account. Boris must receive this one.
	_, _ = medikube.create(t, boris, testsupport.AccountBPatientSelfID, "Ibuprofen")

	seenByB := watchingB.nextPatch(patchDeadline)
	require.Equal(t, "#"+ids.RecordRows(kind.Medication), seenByB.selector(),
		"the first patch Boris received is not his own record — Amara's write reached his stream")

	assert.Empty(t, watchingB.elementPatches(),
		"Boris received a further element patch after his own barrier: Amara's write is on his wire")

	// Amara must have received his own write, or the assertion above would be
	// satisfied by a stream that never delivers anything at all.
	seenByA := watchingA.nextPatch(patchDeadline)
	assert.Equal(t, "#"+ids.RecordRows(kind.Medication), seenByA.selector(),
		"Amara did not receive his own write, so the isolation assertion proves nothing")
}

// The same guarantee on the path that has to reason about a record that is no
// longer there. internal/service/access grants for an id it cannot resolve —
// deliberately, so that a genuine miss never lands in the audit trail — so
// "granted, then not found" is reached by every deletion on every account, and
// a removal sent on it would put one account's record id on another's wire.
func TestADeletionOnOneAccountReachesNoOtherAccountsStream(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)
	boris := medikube.token(t, testsupport.AccountBEmail)

	doomed, etag := medikube.create(t, amara, testsupport.AccountAPatientSelfID, "Amoxicillin")

	watchingA := medikube.open(t, amara, "?patient="+testsupport.AccountAPatientSelfID)
	watchingB := medikube.open(t, boris, "?patient="+testsupport.AccountBPatientSelfID)

	medikube.remove(t, amara, doomed, etag)

	_, _ = medikube.create(t, boris, testsupport.AccountBPatientSelfID, "Ibuprofen")

	seenByB := watchingB.nextPatch(patchDeadline)
	require.Equal(t, "#"+ids.RecordRows(kind.Medication), seenByB.selector(),
		"Boris was told about a record that was deleted on somebody else's account")
	assert.Empty(t, watchingB.elementPatches(), "a further patch reached Boris after his barrier")

	removal := watchingA.nextPatch(patchDeadline)
	assert.Equal(t, "#"+ids.RecordRow(kind.Medication, doomed), removal.selector())
	assert.Equal(t, "remove", removal.mode(),
		"Amara's own deletion did not remove the row, so the isolation assertion above proves nothing")
}

// The checkpoint inside the loop is what refuses, and it refuses BEFORE the
// kind's service is reached. That ordering is not decoration.
//
// The two tests above pass with the re-authorisation deleted outright — measured,
// not assumed — because medication.Service authorizes its own Get and refuses
// Boris anyway. What that costs is this: every write on every other account
// would reach every subscriber's service call, and medication.Service records an
// access_denied row on every refusal (service.go's denied). Boris, who did
// nothing but leave a tab open, would accumulate one access_denied per write per
// account in a compliance trail, and the trail would grow with writes times
// subscribers rather than with attempts.
//
// So the assertion is the trail: a passive subscriber attempts nothing, and the
// audit log must say nothing about him. This is the test that goes red when the
// checkpoint is hoisted, deleted or reordered after the fetch.
func TestAPassiveSubscriberIsRefusedBeforeTheServiceAndAccusedOfNothing(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)
	boris := medikube.token(t, testsupport.AccountBEmail)

	watchingB := medikube.open(t, boris, "?patient="+testsupport.AccountBPatientSelfID)
	require.Equal(t, 200, watchingB.Response.StatusCode)

	before := len(denials(t, medikube, testsupport.AccountBID))

	// Three writes on Amara's account, each of which the hub fans out to Boris.
	for _, name := range []string{"Amoxicillin", "Bisoprolol", "Ciclosporin"} {
		_, _ = medikube.create(t, amara, testsupport.AccountAPatientSelfID, name)
	}

	// The barrier: once Boris has received his own record, all three of Amara's
	// have been through his loop.
	_, _ = medikube.create(t, boris, testsupport.AccountBPatientSelfID, "Ibuprofen")

	seen := watchingB.nextPatch(patchDeadline)
	require.Equal(t, "#"+ids.RecordRows(kind.Medication), seen.selector(),
		"Amara's write reached Boris's stream")

	after := denials(t, medikube, testsupport.AccountBID)

	assert.Len(t, after, before,
		"a passive subscriber was recorded as having been denied access %d times: "+
			"the stream reached the service before its own checkpoint refused", len(after)-before)
}

// denials is every access_denied row naming one actor.
func denials(t *testing.T, medikube *instance, actorID string) []audit.Event {
	t.Helper()

	var found []audit.Event

	for _, event := range apitest.Events(t, medikube.App) {
		if event.Action == audit.ActionAccessDenied && event.ActorID == actorID {
			found = append(found, event)
		}
	}

	return found
}

// FR-038 and the rule that makes per-subscriber authorization possible at all:
// the hub carries identifiers, so nothing a person recorded can be on the wire
// before the subscriber's own checkpoint has run. The rendered row that follows
// is the kind's own view, produced after that check.
func TestTheStreamCarriesNoRecordContentItDidNotRenderForThisSubscriber(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)
	boris := medikube.token(t, testsupport.AccountBEmail)

	watching := medikube.open(t, boris, "?patient="+testsupport.AccountBPatientSelfID)

	const secret = "Amoxicillin"

	_, _ = medikube.create(t, amara, testsupport.AccountAPatientSelfID, secret)

	_, _ = medikube.create(t, boris, testsupport.AccountBPatientSelfID, "Ibuprofen")

	require.Equal(t, "#"+ids.RecordRows(kind.Medication), watching.nextPatch(patchDeadline).selector())

	for _, f := range append(watching.drained(), frame{}) {
		for _, line := range f.Data {
			assert.NotContainsf(t, line, secret,
				"another account's medication name is on this subscriber's wire: %q", line)
		}
	}
}

// Anonymous is refused before the stream opens. A 401 delivered as a frame is
// indistinguishable from a working stream that never sends anything, so the
// browser would sit on a dead connection believing it live.
func TestAnAnonymousSubscriberIsRefusedBeforeTheStreamOpens(t *testing.T) {
	t.Parallel()

	medikube := serve(t)

	refused := medikube.open(t, "", "")

	assert.Equal(t, 401, refused.Response.StatusCode)
	assert.NotEqual(t, "text/event-stream", refused.Response.Header.Get("Content-Type"))
}

// contracts/streams.md's response headers, on the wire, through the whole edge
// — the security binder, the lockdown, the router and the SDK. The unit test in
// stream_test.go asserts newStream sets them; this asserts nothing between
// newStream and the socket takes them away again.
func TestTheOpenStreamCarriesTheContractsHeaders(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	watching := medikube.open(t, medikube.token(t, testsupport.AccountAEmail), "?patient="+testsupport.AccountAPatientSelfID)

	require.Equal(t, 200, watching.Response.StatusCode)

	for header, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-store",
		"X-Accel-Buffering": "no",
	} {
		assert.Equalf(t, want, watching.Response.Header.Get(header), "%s", header)
	}
}
