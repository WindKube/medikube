package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"medikube/internal/logging"
)

// CorrelationHeader carries the correlation id on the way in, so a reverse
// proxy's id survives, and on the way out, so the person looking at a failure
// can quote an id back without disclosing anything about themselves (FR-054).
const CorrelationHeader = "X-Request-Id"

// RequestLoggerID is the hook id, so a later middleware can be ordered against
// this one by name rather than by guessing at priorities.
const RequestLoggerID = "medikubeRequestLogger"

// maxCorrelationIDLen bounds what MediKube will echo back from a header.
const maxCorrelationIDLen = 64

type correlationKey struct{}

// RequestLogger mints the correlation id, derives the request-scoped logger,
// puts both on the request context, and writes the one line that records the
// handled request.
//
// It replaces PocketBase's own activity logger, which must be unbound: that one
// records the full request URI, and a query string is where a search for a
// medication name would end up (FR-038).
func RequestLogger(base zerolog.Logger) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id: RequestLoggerID,
		// Ahead of everything PocketBase binds, so a line rejected by the rate
		// limiter is still correlated.
		Priority: apis.DefaultActivityLoggerMiddlewarePriority - 10,
		Func: func(e *core.RequestEvent) error {
			start := time.Now()

			// The outermost wrapper has already minted an id for requests that
			// reach the process through the served handler, and taking it back
			// rather than minting a second is what makes one request one id.
			// There is no ledger under tests.ApiScenario, which drives the mux
			// directly, so the empty answer is the ordinary one in tests.
			edge := EdgeFrom(e.Request.Context())

			id := edge.CorrelationID()
			if id == "" {
				id = correlationID(e.Request.Header.Get(CorrelationHeader))
			}

			e.Response.Header().Set(CorrelationHeader, id)

			log := logging.ForRequest(base, id)

			if sc := trace.SpanContextFromContext(e.Request.Context()); sc.IsValid() {
				log = log.With().
					Str("trace_id", sc.TraceID().String()).
					Str("span_id", sc.SpanID().String()).
					Logger()
			}

			ctx := log.WithContext(e.Request.Context())
			ctx = context.WithValue(ctx, correlationKey{}, id)
			e.Request = e.Request.WithContext(ctx)

			err := e.Next()

			// The error middleware answers the client and returns nil, so the
			// occurrence it recorded is what this line reports. One occurrence,
			// one report (FR-057).
			reported := err
			if reported == nil {
				reported = Fault(e)
			}

			record(log, e, reported, time.Since(start))

			// Claimed only once the line is out. The outermost wrapper writes
			// the line for requests that never got here — a preflight answered
			// ahead of this handler cannot happen (this is bound outside CORS)
			// but a ServeMux redirect never enters the router at all — and an
			// unclaimed ledger is what tells it to.
			edge.MarkLogged()

			return err
		},
	}
}

// CorrelationID returns the correlation id carried by ctx, or the empty string
// outside a request. It is how an error page shows the person the same id the
// operational record holds.
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)

	return id
}

// record writes the single line for one handled request.
//
// Every field is either a bounded value or an opaque id. The URL is reduced to
// its path: nothing here carries a query string, a header, a cookie or a byte
// of either body, because all four are places a medication name, an email
// address or a note reaches the process (FR-038).
func record(log zerolog.Logger, e *core.RequestEvent, err error, took time.Duration) {
	event := log.Info()
	if err != nil {
		event = log.Error().Err(Recordable(e, err))
	}

	if e.Auth != nil {
		event = event.Str("user_id", e.Auth.Id)
	}

	event.
		Str("method", e.Request.Method).
		Str("path", e.Request.URL.Path).
		Int("status", status(e, err)).
		Dur("duration_ms", took).
		Msg("http_request")
}

// status reports what the client actually got. A handler that failed before
// writing leaves the response untouched, so the status has to come from the
// error instead.
func status(e *core.RequestEvent, err error) int {
	if written := e.Status(); written != 0 {
		return written
	}

	var apiErr *router.ApiError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}

	if err != nil {
		return http.StatusInternalServerError
	}

	return http.StatusOK
}

// correlationID takes the id the edge minted, but only when it is safe to put
// in a log field. An inbound header is attacker-controlled free text, and free
// text is exactly what must never become one (FR-038).
func correlationID(inbound string) string {
	if len(inbound) > 0 && len(inbound) <= maxCorrelationIDLen && isOpaque(inbound) {
		return inbound
	}

	var raw [16]byte
	// Documented never to fail: crypto/rand panics internally instead.
	_, _ = rand.Read(raw[:])

	return hex.EncodeToString(raw[:])
}

func isOpaque(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}

	return true
}
