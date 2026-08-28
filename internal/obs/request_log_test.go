package obs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"medikube/internal/config"
	"medikube/internal/logging"
)

// The strings a request can carry that must never reach the operational record
// (FR-038). Each is planted somewhere the naive implementation would pick it
// up: the query string, a cookie, the request body, the response body.
const (
	querySecret    = "insulin"
	cookieSecret   = "eyJhbGciOiJIUzI1NiJ9.super-secret-session"
	requestSecret  = "Amoxicillin 500mg twice daily"
	responseSecret = "Ibuprofen"
)

type requestOpt func(*http.Request)

// exercise drives one request through the middleware and returns the decoded
// log entry plus the response recorder.
func exercise(t *testing.T, base zerolog.Logger, next func(*core.RequestEvent) error, opts ...requestOpt) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/things?q="+querySecret+"&note=chronic",
		strings.NewReader(`{"name":"`+requestSecret+`"}`),
	)
	req.AddCookie(&http.Cookie{Name: "pb_auth", Value: cookieSecret})
	req.Header.Set("Authorization", "Bearer "+cookieSecret)

	for _, opt := range opts {
		opt(req)
	}

	rec := httptest.NewRecorder()

	e := &core.RequestEvent{}
	e.Request = req
	e.Response = &router.ResponseWriter{ResponseWriter: rec}

	var h hook.Hook[*core.RequestEvent]
	h.Bind(RequestLogger(base))

	_ = h.Trigger(e, next)

	return rec
}

func capture(t *testing.T) (*bytes.Buffer, zerolog.Logger) {
	t.Helper()

	buf := new(bytes.Buffer)

	return buf, logging.NewTo(buf, config.LogConfig{Level: "debug"}, "test")
}

func single(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	raw := strings.TrimSpace(buf.String())
	require.NotEmpty(t, raw, "the request logger wrote nothing at all")
	require.NotContains(t, raw, "\n", "one handled request is exactly one line")

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &entry))

	return entry
}

func created(e *core.RequestEvent) error {
	e.Response.WriteHeader(http.StatusCreated)
	_, err := e.Response.Write([]byte(`{"drug":"` + responseSecret + `"}`))

	return err
}

func TestRequestLoggerRecordsTheTransportFacts(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	rec := exercise(t, base, created)

	entry := single(t, buf)

	assert.Equal(t, http.MethodPost, entry["method"])
	assert.Equal(t, "/api/things", entry["path"])
	assert.EqualValues(t, http.StatusCreated, entry["status"])
	assert.Contains(t, entry, "duration_ms", "how long it took is the point of the line")
	assert.NotEmpty(t, entry[logging.CorrelationField])
	assert.Equal(t, entry[logging.CorrelationField], rec.Header().Get(CorrelationHeader),
		"FR-054: the id in the record is the id the person can quote back")
}

func TestRequestLoggerNeverRecordsBodiesQueryStringsOrCookies(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	_ = exercise(t, base, created)

	line := buf.String()

	for _, forbidden := range []string{
		querySecret,
		requestSecret,
		responseSecret,
		cookieSecret,
		"q=insulin",
		"note=chronic",
		"pb_auth",
		"Authorization",
		"Bearer",
	} {
		assert.NotContainsf(t, line, forbidden,
			"FR-038: %q reached the operational record; the line was %s", forbidden, line)
	}

	entry := single(t, buf)
	for _, key := range []string{"query", "body", "cookies", "headers", "request_uri", "url"} {
		assert.NotContainsf(t, entry, key, "the line must have no %q field at all", key)
	}
}

func TestRequestLoggerHonoursAWellFormedInboundCorrelationId(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	rec := exercise(t, base, created, func(r *http.Request) {
		r.Header.Set(CorrelationHeader, "01HQ8Z6M2X-edge-42")
	})

	assert.Equal(t, "01HQ8Z6M2X-edge-42", single(t, buf)[logging.CorrelationField],
		"a correlation id minted at the edge must survive, or the two records cannot be joined")
	assert.Equal(t, "01HQ8Z6M2X-edge-42", rec.Header().Get(CorrelationHeader))
}

func TestRequestLoggerRefusesAHostileInboundCorrelationId(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		inbound  string
		unwanted string
	}{
		{name: "free text", inbound: "patient Jane Doe takes " + querySecret, unwanted: querySecret},
		{name: "over length", inbound: strings.Repeat("a", 65), unwanted: strings.Repeat("a", 65)},
		{name: "control characters", inbound: "abc\ndef", unwanted: "abc\ndef"},
		{name: "empty", inbound: "", unwanted: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buf, base := capture(t)
			_ = exercise(t, base, created, func(r *http.Request) {
				r.Header.Set(CorrelationHeader, tt.inbound)
			})

			entry := single(t, buf)
			got, ok := entry[logging.CorrelationField].(string)

			require.True(t, ok)
			assert.NotEmpty(t, got, "a rejected id is replaced, never dropped")
			assert.NotEqual(t, tt.inbound, got)

			if tt.unwanted != "" {
				assert.NotContains(t, buf.String(), tt.unwanted,
					"FR-038: a header is attacker-controlled free text and must not become a log field")
			}
		})
	}
}

func TestRequestLoggerAttachesTheRequestScopedLoggerToTheContext(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)

	var fromCtx string

	_ = exercise(t, base, func(e *core.RequestEvent) error {
		zerolog.Ctx(e.Request.Context()).Info().Msg("handler line")
		fromCtx = CorrelationID(e.Request.Context())

		return created(e)
	})

	entries := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, entries, 2, "the handler's own line and the request line")

	var handlerLine, requestLine map[string]any
	require.NoError(t, json.Unmarshal([]byte(entries[0]), &handlerLine))
	require.NoError(t, json.Unmarshal([]byte(entries[1]), &requestLine))

	assert.Equal(t, "handler line", handlerLine["msg"])
	assert.Equal(t, requestLine[logging.CorrelationField], handlerLine[logging.CorrelationField],
		"FR-054: every line produced while handling the request carries the same id")
	assert.Equal(t, requestLine[logging.CorrelationField], fromCtx,
		"FR-054 also puts the id on the error page, so the handler has to be able to read it back")
}

func TestRequestLoggerCarriesTheTraceContextWhenASpanIsActive(t *testing.T) {
	t.Parallel()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	buf, base := capture(t)
	_ = exercise(t, base, created, func(r *http.Request) {
		*r = *r.WithContext(trace.ContextWithSpanContext(r.Context(), sc))
	})

	entry := single(t, buf)
	assert.Equal(t, traceID.String(), entry["trace_id"])
	assert.Equal(t, spanID.String(), entry["span_id"])
}

func TestRequestLoggerOmitsTheTraceContextWhenThereIsNoSpan(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	_ = exercise(t, base, created)

	entry := single(t, buf)
	assert.NotContains(t, entry, "trace_id")
	assert.NotContains(t, entry, "span_id")
}

func TestRequestLoggerReportsAFailureAndTheStatusItImplies(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	_ = exercise(t, base, func(_ *core.RequestEvent) error {
		return router.NewNotFoundError("record not found", nil)
	})

	entry := single(t, buf)
	assert.Equal(t, "error", entry["level"])
	assert.EqualValues(t, http.StatusNotFound, entry["status"],
		"nothing was written to the wire, so the status comes from the error")
	assert.Contains(t, entry, "error")
}

func TestRequestLoggerHasNoUserIdWithoutAnAuthenticatedActor(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	_ = exercise(t, base, created)

	assert.NotContains(t, single(t, buf), "user_id")
}
