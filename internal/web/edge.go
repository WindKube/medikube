package web

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/logging"
	"medikube/internal/obs"
)

// Outermost returns the net/http middleware that wraps the handler PocketBase
// builds, from outside PocketBase's router entirely.
//
// Everything else in this package is a *router* middleware, and a router
// middleware only ever sees a request the router routed. Two classes of
// response are answered before that and were therefore answered with nothing:
//
//   - A CORS preflight. apis.CORS is bound at
//     apis.DefaultCorsMiddlewarePriority — DefaultActivityLoggerMiddlewarePriority-1,
//     which is -1041 — and it answers OPTIONS itself without calling e.Next()
//     (apis/middlewares_cors.go:186-189). [SecurityHeaders] sits at -1010,
//     thirty-one steps later, so every OPTIONS response left this process with
//     Access-Control-* and an X-Request-Id and nothing else: no policy, no
//     nosniff, no X-Frame-Options, no COOP, no Referrer-Policy, no HSTS.
//
//   - A path-normalising redirect. net/http's ServeMux answers `//`, `/./` and
//     `/../` with a redirect of its own, decided in ServeMux.findHandler before
//     it looks for a registered handler, so no route and no hook chain is ever
//     entered. `GET /api/v1/records//medications` was a 307 with no security
//     headers, no correlation id and — the part that matters most — no log
//     line: an anonymous caller had an unbounded class of request that left no
//     operational record at all, against FR-053, FR-054 and Principle VI's one
//     request, one line.
//
// The handler this wraps is the built mux, so it is the one seam from which
// both are visible. What it guarantees, for every response the process emits:
// the five unconditional security headers, a content security policy unless
// some handler wrote its own, an X-Request-Id, and exactly one log line.
func Outermost(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// The ledger is opened here and nowhere else, so the id the client
			// is handed, the id obs.RequestLogger writes and the id this
			// handler would write are one id (FR-054).
			ctx, edge := obs.NewEdge(r.Context(), r.Header.Get(obs.CorrelationHeader))
			r = r.WithContext(ctx)

			w.Header().Set(obs.CorrelationHeader, edge.CorrelationID())
			writeUnconditionalHeaders(w.Header())

			tracked := &edgeWriter{ResponseWriter: w}

			// Deferred rather than sequential: a panic that escapes
			// PocketBase's recovery middleware — which is inside this handler,
			// so anything outside it is unrecovered — still leaves the
			// operational record behind before net/http drops the connection.
			defer func() {
				if edge.Logged() {
					return
				}

				log := logging.ForRequest(base, edge.CorrelationID())

				log.Info().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Int("status", tracked.statusCode()).
					Dur("duration_ms", time.Since(start)).
					Msg("http_request")
			}()

			next.ServeHTTP(tracked, r)
		})
	}
}

// edgeWriter is the outermost response writer. It remembers the status for the
// fallback log line, and it fills in the content security policy at the moment
// the header block is committed.
//
// Set-if-empty at commit, rather than Set before delegating, and the difference
// is load-bearing. The one handler in this process that writes its own policy
// is PocketBase's admin UI at /_/ (apis/serve.go:83-99) — and it writes it
// *only when the header is empty*. A policy set eagerly out here would
// therefore not be overridden by it; it would replace it. MediKube's policy
// forbids the inline styles and the map tiles that UI loads, so constitution
// VII's break-glass interface would come up blank. Filling the gap at commit
// leaves whatever the chain decided and covers only the responses that decided
// nothing.
//
// Path-prefix matching on /_/ out here would be the other way to do it, and it
// is worse: the discriminator inside the router is the router's own matched
// pattern (see AdminUIPattern), so a `POST /_/x` — which ServeMux answers 405
// for, under no pattern at all — would lose the policy it must keep.
type edgeWriter struct {
	http.ResponseWriter

	status    int
	committed bool
}

var (
	_ http.Flusher  = (*edgeWriter)(nil)
	_ http.Hijacker = (*edgeWriter)(nil)
)

func (w *edgeWriter) WriteHeader(status int) {
	// 1xx is informational: net/http sends it and the header block stays open,
	// so it is not the commit and the status it carries is not the answer.
	if status >= http.StatusOK {
		w.commit(status)
	}

	w.ResponseWriter.WriteHeader(status)
}

func (w *edgeWriter) Write(b []byte) (int, error) {
	w.commit(http.StatusOK)

	return w.ResponseWriter.Write(b)
}

func (w *edgeWriter) Flush() { _ = w.FlushError() }

// FlushError is what http.ResponseController reaches for before Flush, and
// PocketBase's own router.ResponseWriter flushes through a controller
// (tools/router/router.go:264) — so an SSE stream's first flush arrives here,
// and it arrives before any status has been written. Committing first is what
// keeps the policy on a response whose handler only ever flushed.
func (w *edgeWriter) FlushError() error {
	w.commit(http.StatusOK)

	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *edgeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// A hijacked connection has no header block left to write to, so the
	// commit is recorded without touching one.
	w.committed = true

	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// Unwrap is how http.ResponseController reaches the writer net/http handed us,
// so everything this type does not implement — SetWriteDeadline, EnableFullDuplex
// — still finds the real one rather than ErrNotSupported.
func (w *edgeWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *edgeWriter) commit(status int) {
	if w.committed {
		return
	}

	w.committed = true
	w.status = status

	if w.Header().Get(headerContentSecurityPolicy) == "" {
		w.Header().Set(headerContentSecurityPolicy, ContentSecurityPolicy)
	}
}

// statusCode is what the client got. Nothing written is net/http's implicit
// 200, and a hijacked connection has no status at all — 200 is the honest
// answer for both, and the one net/http itself records.
func (w *edgeWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}

	return w.status
}
