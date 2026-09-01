//go:build sselive

package stream_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
	"medikube/internal/web/stream"
)

// T158, SC-007 and shared design risk R7. A stream held open for LONGER THAN
// FIVE MINUTES still receives heartbeats.
//
// EVERY TEST SHORTER THAN FIVE MINUTES PASSES WITH THE BUG PRESENT. That is the
// entire reason this file exists and the reason it is build-tagged rather than
// deleted for being slow: PocketBase constructs its http.Server as a struct
// literal with WriteTimeout: 5 * time.Minute and no configuration field
// (apis/serve.go), datastar.NewSSE never touches the write deadline, and so
// without internal/web/stream's clear the flush inside Send fails with an i/o
// timeout at exactly 5:00, the connection closes and the browser
// reconnect-loops. Every frame before that arrives perfectly.
//
// The server below is given PocketBase's own literal deliberately. Production
// sets pb.ServeOptions.WriteTimeout to zero, so testing against a server with
// no timeout at all would assert that a fix nobody applied is unnecessary — the
// per-request clear is the half that survives somebody restoring a
// server-level default, and this is what holds it in place.
//
// Run it with: task test:sselive

// pocketbaseWriteTimeout is apis/serve.go's hardcoded literal.
const pocketbaseWriteTimeout = 5 * time.Minute

// hold is how long the stream is kept open. It has to be past the timeout by
// more than one heartbeat interval, or a passing run would not have proved a
// beat arrived on the far side of it.
const hold = pocketbaseWriteTimeout + 2*stream.HeartbeatInterval

func TestAStreamSurvivesTheFiveMinuteWriteTimeout(t *testing.T) {
	wired := apitest.New(t)

	server := httptest.NewUnstartedServer(testsupport.NewEdgeHandler(t, wired.App))
	server.Config.WriteTimeout = pocketbaseWriteTimeout
	server.Config.ReadTimeout = pocketbaseWriteTimeout
	server.Start()

	t.Cleanup(server.Close)

	medikube := &instance{Instance: wired, base: server.URL}

	watching := medikube.open(t, medikube.token(t, testsupport.AccountAEmail), "")
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	started := time.Now()
	deadline := started.Add(hold)

	var (
		beats      int
		lastBeatAt time.Time
		afterCut   int
	)

	// The bound on any single wait is one interval plus generous slack: a beat
	// that has not arrived by then is a dead stream and not a slow one, and
	// waiting the whole remaining hold for it would report the failure five
	// minutes late.
	//
	// It is deliberately NOT clamped to the time left before the deadline. A
	// clamp makes the final wait shorter than the interval it is waiting for —
	// a coin flip against the last beat, which failed a run at 5m25s with the
	// stream perfectly alive. Overshooting the hold by up to one interval costs
	// 25 seconds; a flaky liveness gate costs the gate.
	wait := stream.HeartbeatInterval + 30*time.Second

	for time.Now().Before(deadline) {
		frame := watching.next(wait)
		require.Truef(t, frame.isHeartbeat(),
			"an unexpected frame arrived on an idle stream after %s: %+v", time.Since(started), frame)

		beats++
		lastBeatAt = time.Now()

		if elapsed := lastBeatAt.Sub(started); elapsed > pocketbaseWriteTimeout {
			afterCut++
		}

		t.Logf("beat %d at %s", beats, lastBeatAt.Sub(started).Round(time.Second))
	}

	assert.Positivef(t, afterCut,
		"no heartbeat arrived after %s: the stream died at the write timeout", pocketbaseWriteTimeout)

	assert.Greaterf(t, lastBeatAt.Sub(started), pocketbaseWriteTimeout,
		"the last beat was at %s, which is inside the five-minute window this test exists to get past",
		lastBeatAt.Sub(started).Round(time.Second))

	// The count is a sanity check on the interval rather than a threshold: a
	// stream that survived by sending nothing would satisfy everything above.
	expected := int(hold / stream.HeartbeatInterval)
	assert.GreaterOrEqualf(t, beats, expected-2,
		"only %d beats in %s at an interval of %s", beats, hold, stream.HeartbeatInterval)

	t.Logf("held open for %s, %d heartbeats, %d of them after the %s write timeout",
		time.Since(started).Round(time.Second), beats, afterCut, pocketbaseWriteTimeout)
}
