package stream

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/realtime"
	"medikube/internal/records/recordstest"
	"medikube/internal/web/views/ids"
)

// T157. newStream is the whole of MediKube's SSE prologue and every one of its
// four jobs fails silently when it is missing: a stream with the SDK's own
// headers works for five minutes and then dies, a stream behind nginx without
// X-Accel-Buffering delivers nothing at all and looks idle, and a `no-cache`
// body is one an intermediary may write to disk.

// deadlineWriter records what SetWriteDeadline was asked for. A
// httptest.ResponseRecorder cannot: http.NewResponseController answers
// ErrNotSupported for it, so a test driven through one would assert the
// deadline was cleared while never observing a deadline at all.
type deadlineWriter struct {
	http.ResponseWriter

	deadlines []time.Time
	err       error
}

func (w *deadlineWriter) SetWriteDeadline(at time.Time) error {
	w.deadlines = append(w.deadlines, at)

	return w.err
}

// Unwrap is how http.ResponseController reaches the writer underneath for
// everything this one does not answer itself — the flush in particular, which
// datastar.NewSSE panics on rather than returning. Both writers MediKube's
// stream actually sits behind (PocketBase's router.ResponseWriter and
// internal/web's edgeWriter) implement it, so a fake that did not would be
// testing a chain production does not have.
func (w *deadlineWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func requestEvent(response http.ResponseWriter) *core.RequestEvent {
	e := new(core.RequestEvent)
	e.Response = response
	e.Request = httptest.NewRequest(http.MethodGet, "/api/v1/streams/records", nil)

	return e
}

// research D-34 / risk R7. PocketBase builds its server with a hardcoded
// WriteTimeout of five minutes, so without this clear every stream dies at
// exactly 5:00 — and every test shorter than five minutes passes with the bug
// present. timeout_test.go is the one that does not; this is the structural
// half, which is what stops the clear being deleted as dead code by somebody
// who checks only production behaviour.
func TestTheStreamClearsThePerRequestWriteDeadline(t *testing.T) {
	t.Parallel()

	writer := &deadlineWriter{ResponseWriter: httptest.NewRecorder()}

	sse, err := newStream(requestEvent(writer))
	require.NoError(t, err)
	require.NotNil(t, sse)

	require.Len(t, writer.deadlines, 1, "the write deadline was never touched")
	assert.Truef(t, writer.deadlines[0].IsZero(),
		"the write deadline was set to %s rather than cleared; anything but the zero time is a cap on the stream",
		writer.deadlines[0])
}

// A httptest.ResponseRecorder answers ErrNotSupported, and tests.ApiScenario
// drives the mux with one. A newStream that treated that as a failure would
// break every scenario-based test while working in production — the exact
// inverse of the bug it is fixing.
func TestAWriterThatCannotTakeADeadlineIsNotAFailure(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	err := clearWriteDeadline(recorder)
	require.NoError(t, err, "a writer that does not support deadlines refused the stream")

	direct := http.NewResponseController(recorder).SetWriteDeadline(time.Time{})
	require.ErrorIs(t, direct, http.ErrNotSupported,
		"the recorder no longer answers ErrNotSupported, so this test is asserting nothing")
}

// Any other failure is real: the deadline was not cleared, so the stream would
// die at the server's timeout with no sign that it was ever going to.
func TestAWriterThatRefusesTheDeadlineRefusesTheStream(t *testing.T) {
	t.Parallel()

	refused := errors.New("the connection is gone")
	writer := &deadlineWriter{ResponseWriter: httptest.NewRecorder(), err: refused}

	sse, err := newStream(requestEvent(writer))

	require.ErrorIs(t, err, refused)
	assert.Nil(t, sse)
}

// contracts/streams.md's four response headers, all of them.
//
// The order inside newStream is what makes this pass and it is not
// arrangeable: datastar.NewSSE sets Cache-Control, Content-Type and Connection
// and then flushes, so headers set after it never ship and headers set before
// it are overwritten unless the block has already been committed.
func TestTheStreamShipsMediKubesHeadersAndNotTheSDKsDefaults(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	_, err := newStream(requestEvent(recorder))
	require.NoError(t, err)

	result := recorder.Result()
	defer func() { _ = result.Body.Close() }()

	assert.Equal(t, http.StatusOK, result.StatusCode)

	for header, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-store",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	} {
		assert.Equalf(t, want, result.Header.Get(header), "%s", header)
	}

	assert.NotEqual(t, "no-cache", result.Header.Get("Cache-Control"),
		"the SDK's own Cache-Control won: no-cache permits an intermediary to store a body of medication rows and merely revalidate it")
}

// contracts/streams.md: Datastar v1 recognises exactly two event names and
// silently discards everything else, so a third would be a stream that looks
// healthy and patches nothing.
func TestTheStreamEmitsOnlyTheTwoDatastarEventNames(t *testing.T) {
	t.Parallel()

	rig := newRig(t, withHeartbeat(20*time.Millisecond))

	created, err := rig.services[kind.Medication].Create(t.Context(), actorOf(recordstest.OwnerID),
		&recordstest.Create{Name: "Amoxicillin"})
	require.NoError(t, err)

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: created.ID, OwnerID: recordstest.OwnerID})
	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: "mkgone0000000001", OwnerID: recordstest.OwnerID})

	seen := make(map[string]int)

	for range 6 {
		seen[rig.next().Event]++
	}

	for name := range seen {
		assert.Containsf(t, EventNames(), name,
			"%q is not one of the two names the Datastar v1 runtime handles, so the browser discards it in silence", name)
	}

	assert.Positive(t, seen["datastar-patch-elements"], "no element patch was ever sent")
	assert.Positive(t, seen["datastar-patch-signals"], "no heartbeat was ever sent")
}

// The two names are read off the SDK's own constants rather than spelled, so
// the assertion above cannot pass by agreeing with a typo.
func TestThePublishedEventNamesAreTheDatastarV1Ones(t *testing.T) {
	t.Parallel()

	assert.ElementsMatch(t, []string{"datastar-patch-elements", "datastar-patch-signals"}, EventNames())
	assert.NotContains(t, EventNames(), "datastar-merge-fragments", "that is a v0.x name and the runtime discards it")
}

// A created or changed record is patched by the id its row carries, which is
// the same call internal/web/views/ids renders into the element.
func TestAChangeIsPatchedByTheRowsOwnID(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	created, err := rig.services[kind.Medication].Create(t.Context(), actorOf(recordstest.OwnerID),
		&recordstest.Create{Name: "Amoxicillin"})
	require.NoError(t, err)

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: created.ID, OwnerID: recordstest.OwnerID})

	// The first frame is the immediate heartbeat: a page opened at the wrong
	// moment must not sit for a whole interval with no $stream_beat at all.
	require.Equal(t, "datastar-patch-signals", rig.next().Event)

	patch := rig.next()
	assert.Equal(t, "datastar-patch-elements", patch.Event)
	assert.Equal(t, "#"+ids.RecordRow(kind.Medication, created.ID), patch.selector())
	assert.Equal(t, recordstest.RenderedRow, patch.elements())
	assert.Empty(t, patch.mode(), "outer is the default and is omitted from the wire")
}

// A record the subscriber can no longer fetch is a row removal, and the
// removal names the same id the patch would have.
func TestARecordThatIsGoneIsRemovedFromTheRowItOccupied(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	require.Equal(t, "datastar-patch-signals", rig.next().Event)

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: "mkgone0000000001", OwnerID: recordstest.OwnerID})

	removal := rig.next()
	assert.Equal(t, "datastar-patch-elements", removal.Event)
	assert.Equal(t, "#"+ids.RecordRow(kind.Medication, "mkgone0000000001"), removal.selector())
	assert.Equal(t, "remove", removal.mode())
	assert.Empty(t, removal.elements(), "a removal carries no markup")
}

// T157's last clause and contracts/streams.md's registration line. Neither the
// activity-log skip nor the rate-limiter exemption is observable from a
// response, and neither can be bound by the handler: by the time a handler
// runs, the chain that would have held them is already assembled.
func TestTheStreamRouteIsRegisteredWithItsTwoMiddlewareDecisions(t *testing.T) {
	t.Parallel()

	var route httproute.Route

	for _, candidate := range httproute.Inventory().Routes() {
		if candidate.OpID == OpStreamRecords {
			route = candidate
		}
	}

	require.Equal(t, OpStreamRecords, route.OpID, "the route table has no %s", OpStreamRecords)
	require.Equal(t, httproute.KindStream, route.Kind)
	require.Equal(t, httproute.AuthUser, route.Auth,
		"an anonymous stream is a 401 delivered as a frame, which is indistinguishable from a working stream that never sends anything")

	assert.True(t, slices.Contains(route.MiddlewareIDs(), apis.DefaultSkipSuccessActivityLogMiddlewareId),
		"the stream does not skip the success activity log")
	assert.True(t, slices.Contains(route.Unbind, apis.DefaultRateLimitMiddlewareId),
		"the stream is inside the rate limiter: one stream is one request that lasts an hour")
}
