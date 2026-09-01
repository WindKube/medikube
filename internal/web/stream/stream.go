package stream

import (
	"errors"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/starfederation/datastar-go/datastar"
)

// HeartbeatInterval is how often the stream patches $stream_beat.
//
// contracts/streams.md fixes it at 25 seconds against a 60-second staleness
// threshold, which is two missed beats plus slack: a single dropped frame or a
// garbage-collection pause must not tell a person their live view has died,
// and three missed beats would leave a genuinely dead stream unreported for a
// minute and a quarter.
const HeartbeatInterval = 25 * time.Second

// StalenessThreshold is the gap the page compares $stream_beat against before
// it says the live view has stopped (FR-031). It is the server's number
// because the server is what decides how often a beat arrives; the page reads
// it out of this constant through the layout rather than spelling its own.
const StalenessThreshold = 60 * time.Second

// SignalStreamBeat is the signal the heartbeat patches, and SignalStreamStale
// is the one the page derives from it. They are constants because a signal
// name is a contract between a Go string and an HTML attribute, and a typo in
// either is a banner that never appears.
const (
	SignalStreamBeat  = "stream_beat"
	SignalStreamStale = "stream_stale"
)

// The response headers newStream sets that datastar.NewSSE does not, or sets
// differently.
const (
	// headerAccelBuffering disables nginx's proxy response buffering. Without
	// it a reverse proxy accumulates the whole stream and the browser receives
	// nothing until the connection closes — which, for a stream held open for
	// an hour, means nothing, ever.
	headerAccelBuffering = "X-Accel-Buffering"

	// no-store rather than the SDK's no-cache. `no-cache` permits an
	// intermediary to store the body and merely requires revalidation; the
	// body here is rendered medication rows, so "may be written to a shared
	// cache's disk" is the wrong answer. PocketBase's own realtime endpoint
	// sets no-store for the same reason.
	streamCacheControl = "no-store"
)

// EventNames publishes the only two event names this package sends.
//
// Datastar v1 recognises exactly two and silently discards everything else, so
// a third name would be a stream that looks healthy and patches nothing. They
// are read off the SDK's own constants rather than spelled, so the assertion
// that only these two are sent cannot pass by agreeing with a typo.
func EventNames() []string {
	return []string{
		string(datastar.EventTypePatchElements),
		string(datastar.EventTypePatchSignals),
	}
}

// newStream opens the SSE stream on a request event.
//
// THE ONLY call to datastar.NewSSE in MediKube. A forbidigo rule in
// .golangci.yml enforces that, because everything below is defence that is
// absent by default and silent when it is missing.
//
// The order is not arrangeable. datastar.NewSSE sets Cache-Control, Content-Type
// and Connection and then flushes the header block; a header set after it is
// never sent, and one set before it is overwritten. Committing the block here,
// with WriteHeader, turns the SDK's Set calls into no-ops and leaves MediKube's
// values on the wire. Measured on a socket: headers-then-NewSSE ships the SDK's
// `no-cache`, NewSSE-then-headers loses X-Accel-Buffering entirely, and only
// this order ships both.
func newStream(e *core.RequestEvent) (*datastar.ServerSentEventGenerator, error) {
	header := e.Response.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", streamCacheControl)
	header.Set("Connection", "keep-alive")
	header.Set(headerAccelBuffering, "no")

	if err := clearWriteDeadline(e.Response); err != nil {
		return nil, err
	}

	// Committing the header block also makes e.Written() true, which is what
	// stops internal/web's error middleware rewriting a response that has
	// already gone if the loop below fails later.
	e.Response.WriteHeader(http.StatusOK)

	return datastar.NewSSE(e.Response, e.Request), nil
}

// clearWriteDeadline removes the per-connection write deadline.
//
// PocketBase builds its http.Server as a struct literal with
// WriteTimeout: 5 * time.Minute and no configuration field (apis/serve.go).
// datastar.NewSSE never touches the deadline, so without this every stream dies
// at exactly five minutes: the flush inside Send fails with an i/o timeout, the
// connection closes and the browser reconnect-loops. Every frame before the
// five minutes arrives, which is why every test shorter than that passes with
// the bug present. internal/web/stream/timeout_test.go is the one that does not.
//
// http.ErrNotSupported is not a failure and must not be treated as one. A
// httptest.ResponseRecorder returns it, so a newStream that refused would break
// every recorder-driven test while working in production — the exact inverse of
// the bug it is fixing.
func clearWriteDeadline(w http.ResponseWriter) error {
	err := http.NewResponseController(w).SetWriteDeadline(time.Time{})
	if err == nil || errors.Is(err, http.ErrNotSupported) {
		return nil
	}

	return err
}
