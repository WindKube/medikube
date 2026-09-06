package stream_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/testsupport"
	"medikube/internal/web/stream"
)

// T207, FR-010, SC-017, risk R7. stream_test.go proves newStream's own
// contract once, and TestAStreamSurvivesTheFiveMinuteWriteTimeout
// (timeout_test.go, build-tagged sselive) proves it survives PocketBase's
// hardcoded five-minute WriteTimeout by actually waiting past it — which is
// the one thing this file cannot do and must not try to: PocketBase's own
// WriteTimeout passes every test shorter than five minutes whether or not the
// clear is wired in, so what is worth asserting here is the construction, not
// elapsed time.
//
// This is that construction, driven per registered kind: every one of them is
// a live `?kind=` filter value that reaches the one stream handler, and every
// one of them gets the same contract headers newStream sets (which is what
// proves the WriteTimeout-clearing newStream ran, since it is the only place
// that clears it) and the same activity-log skip the route table wires.
func TestEveryRegisteredKindsStreamIsBuiltThroughNewStream(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())
	token := medikube.token(t, testsupport.AccountAEmail)

	for _, k := range kind.Kinds() {
		t.Run(k.Segment(), func(t *testing.T) {
			t.Parallel()

			watching := medikube.open(t, token,
				"?patient="+testsupport.AccountAPatientSelfID+"&"+stream.ParamKind+"="+k.Segment())
			require.Equalf(t, http.StatusOK, watching.Response.StatusCode, "%s: opening the stream", k)

			for header, want := range map[string]string{
				"Content-Type":      "text/event-stream",
				"Cache-Control":     "no-store",
				"X-Accel-Buffering": "no",
			} {
				assert.Equalf(t, want, watching.Response.Header.Get(header), "%s: %s", k, header)
			}
		})
	}
}

// TestTheStreamRouteSkipsTheSuccessActivityLog is T207's other half: the
// route table registers OpStreamRecords with apis.SkipSuccessActivityLog(),
// which every registered kind shares because there is one stream route and
// not one per kind (internal/web/stream/records.go).
func TestTheStreamRouteSkipsTheSuccessActivityLog(t *testing.T) {
	t.Parallel()

	var found *httproute.Route

	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == stream.OpStreamRecords {
			route := route
			found = &route

			break
		}
	}

	require.NotNil(t, found, "%s is not in the route table", stream.OpStreamRecords)
	require.NotEmpty(t, found.Middlewares, "%s carries no per-route middleware", stream.OpStreamRecords)

	var hasSkip bool
	for _, middleware := range found.Middlewares {
		if middleware.Id == apis.DefaultSkipSuccessActivityLogMiddlewareId {
			hasSkip = true
		}
	}

	assert.True(t, hasSkip, "%s does not skip the success activity log", stream.OpStreamRecords)
}
