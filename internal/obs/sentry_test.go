package obs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/phileak"
)

// The values a scrubber has to remove. They are spelled once so that the
// sweep over the whole serialised envelope and the field-by-field assertions
// are looking for the same thing.
const (
	medicationSentinel = "Amoxicillin"
	emailSentinel      = "helena@example.test"
	cookieSentinel     = "pb_auth=eyJhbGciOiJIUzI1NiJ9.leaked"
	hostnameSentinel   = "clinic-db-01"
)

// recordsURL is spelled through the kind table rather than by hand: the segment
// is declared once (research D-05) and internal/architecture fails a second
// spelling.
var recordsURL = "https://medikube.example/api/v1/records/" + kind.Medication.Segment()

// outboundTrap is a destination that records every TCP connection opened to it
// and answers 200 to whatever arrives.
//
// It is what turns "inactive when unconfigured" from a claim about a nil
// pointer into a claim about a socket. An assertion that a nil client sent
// nothing passes exactly as well when the subsystem does not exist at all —
// which is what made the repository's earlier inactivity claims true by
// absence rather than by design. Pointing the exporter at a listener and
// asserting the listener was never dialled fails the moment something is
// constructed anyway.
//
// tracing_test.go drives the same trap through the OTLP exporter.
type outboundTrap struct {
	server *httptest.Server

	mu          sync.Mutex
	connections int
	payloads    []string
}

func newOutboundTrap(t *testing.T) *outboundTrap {
	t.Helper()

	trap := &outboundTrap{}

	trap.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		trap.mu.Lock()
		trap.payloads = append(trap.payloads, string(body))
		trap.mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))

	// The connection and not the request: a subsystem that dialled and then
	// failed to speak the protocol has still made the outbound connection
	// FR-039 forbids.
	trap.server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state != http.StateNew {
			return
		}

		trap.mu.Lock()
		trap.connections++
		trap.mu.Unlock()
	}

	trap.server.Start()
	t.Cleanup(trap.server.Close)

	return trap
}

func (o *outboundTrap) connectionCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.connections
}

// address is the host:port form the OTLP exporter's endpoint option wants.
func (o *outboundTrap) address() string {
	return strings.TrimPrefix(o.server.URL, "http://")
}

func (o *outboundTrap) sentryDSN() string {
	return "http://0123456789abcdef0123456789abcdef@" + o.address() + "/1"
}

// Off until an operator configures a destination (FR-039, FR-056).
//
// The configured case is not decoration: it is what proves the unconfigured
// assertion is capable of failing. Both rows make the same two assertions and
// expect opposite answers, so a Reporter that stopped reporting at all, or one
// that reported without a DSN, fails one of them.
func TestSentryIsEntirelyOffUntilADSNIsConfigured(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		dsn        func(*outboundTrap) string
		wantActive bool
	}{
		{
			name: "no DSN at all",
			dsn:  func(*outboundTrap) string { return "" },
		},
		{
			name: "a DSN of whitespace, which is what an empty mounted secret file looks like",
			dsn:  func(*outboundTrap) string { return "  \n\t " },
		},
		{
			name:       "a DSN the operator configured",
			dsn:        func(trap *outboundTrap) string { return trap.sentryDSN() },
			wantActive: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			trap := newOutboundTrap(t)

			reporter, err := StartSentry(config.SentryConfig{
				DSN:         tc.dsn(trap),
				Environment: "test",
				SampleRate:  1,
			}, "test-release", zerolog.Nop())
			require.NoError(t, err)

			assert.Equal(t, tc.wantActive, reporter.Active())
			assert.Equal(t, tc.wantActive, reporter.Report(t.Context(), errors.New("a failure worth reporting")),
				"Report disagrees with Active about whether there is a destination")

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			require.NoError(t, reporter.Shutdown(ctx))

			if tc.wantActive {
				assert.Positive(t, trap.connectionCount(),
					"a configured DSN sent nothing, so the unconfigured rows above prove nothing")

				return
			}

			assert.Zero(t, trap.connectionCount(),
				"an unconfigured Sentry opened an outbound connection (FR-039)")
		})
	}
}

// The scrubber, on an event that already carries the contents.
//
// sentry.ClientOptions.DataCollection stops the SDK collecting them in the
// first place, which is the other half of this and is asserted separately.
// This half exists because an event assembled anywhere else — by hand, by a
// future integration, by a library that reads an *http.Request — reaches
// BeforeSend fully populated, and prepareEvent leaves an already-set Request
// alone. So the scrubber is the only thing standing between it and the wire.
func TestSentryScrubsAnAlreadyAssembledEventBeforeItLeaves(t *testing.T) {
	t.Parallel()

	transport := new(phileak.SentryTransport)

	reporter, err := startSentry(config.SentryConfig{
		DSN:         "http://0123456789abcdef0123456789abcdef@127.0.0.1:1/1",
		Environment: "test",
		SampleRate:  1,
	}, "test-release", zerolog.Nop(), transport)
	require.NoError(t, err)
	require.True(t, reporter.Active())

	event := sentry.NewEvent()
	event.Message = "the store refused the write"
	event.ServerName = hostnameSentinel
	event.Request = &sentry.Request{
		URL:         recordsURL + "?q=" + medicationSentinel,
		Method:      http.MethodPost,
		Data:        `{"name":"` + medicationSentinel + `"}`,
		QueryString: "q=" + medicationSentinel,
		Cookies:     cookieSentinel,
		Headers:     map[string]string{"Cookie": cookieSentinel, "Referer": "https://medikube.example/?q=" + medicationSentinel},
		Env:         map[string]string{"REMOTE_ADDR": "203.0.113.7"},
	}
	event.User = sentry.User{
		ID:        "pbc0123456789abc",
		Email:     emailSentinel,
		Username:  emailSentinel,
		Name:      "Helena Nowak",
		IPAddress: "203.0.113.7",
		Data:      map[string]string{"medication": medicationSentinel},
	}
	event.Breadcrumbs = []*sentry.Breadcrumb{{
		Message: "searched for " + medicationSentinel,
		Data:    map[string]any{"q": medicationSentinel},
	}}
	event.Attachments = []*sentry.Attachment{{
		Filename: "request.json",
		Payload:  []byte(`{"name":"` + medicationSentinel + `"}`),
	}}

	require.NotNil(t, reporter.client)
	reporter.client.CaptureEvent(event, nil, sentry.NewScope())

	events := transport.Events()
	require.Len(t, events, 1, "the event never reached the transport, so nothing below is an assertion")

	sent := events[0]

	require.NotNil(t, sent.Request, "the whole request was dropped, so the field assertions below cannot fail")
	assert.Equal(t, http.MethodPost, sent.Request.Method,
		"the method is bounded and useful and must survive, or this test would pass on a scrubber that nils everything")
	assert.Equal(t, recordsURL, sent.Request.URL)
	assert.Empty(t, sent.Request.Data, "the request body reached Sentry (FR-056)")
	assert.Empty(t, sent.Request.QueryString, "the query string reached Sentry (FR-056)")
	assert.Empty(t, sent.Request.Cookies, "the cookies reached Sentry (FR-056)")
	assert.Empty(t, sent.Request.Headers, "the request headers reached Sentry (FR-056)")
	assert.Empty(t, sent.Request.Env, "the request environment reached Sentry (FR-056)")

	assert.Equal(t, "pbc0123456789abc", sent.User.ID, "the opaque id is permitted and must survive")
	assert.Empty(t, sent.User.Email)
	assert.Empty(t, sent.User.Username)
	assert.Empty(t, sent.User.Name)
	assert.Empty(t, sent.User.IPAddress)
	assert.Empty(t, sent.User.Data)

	assert.Empty(t, sent.Breadcrumbs, "breadcrumbs are assembled out of whatever passed through")
	assert.Empty(t, sent.Attachments, "an attachment is a file's worth of whatever the reporter attached")
	assert.Empty(t, sent.ServerName, "the hostname names the machine somebody's records sit on")

	assertNoSentinelInEnvelope(t, sent)
}

// The other half: the SDK is configured not to collect request contents at
// all, so they never reach an event for the scrubber to remove.
//
// The event is observed BETWEEN assembly and the scrubber, because observing
// it afterwards observes the scrubber: scrub replaces the whole Request, so a
// client with no DataCollection settings at all would pass an
// after-the-scrubber assertion while collecting every header and query
// parameter on the way. sentry-go's own default with SendDefaultPII false is
// not enough — it leaves headers and query parameters on a *deny list*, which
// keeps `Referer: /medications?q=<name>` because "referer" is not one of its
// sensitive terms.
//
// It drives sentry's own scope.SetRequest / SetRequestBody path — the one a
// request-aware reporter uses — rather than a hand-built Request, because
// DataCollection is consulted while the event is being assembled and a
// hand-built Request walks straight past it.
func TestSentryDoesNotCollectRequestContentsInTheFirstPlace(t *testing.T) {
	t.Parallel()

	transport := new(phileak.SentryTransport)

	options := sentryOptions(config.SentryConfig{
		DSN:         "http://0123456789abcdef0123456789abcdef@127.0.0.1:1/1",
		Environment: "test",
		SampleRate:  1,
	}, "test-release", zerolog.Nop(), transport)

	var collected sentry.Request

	scrubber := options.BeforeSend
	require.NotNil(t, scrubber, "sentryOptions installs no BeforeSend, so this test would observe an unscrubbed pipeline")

	options.BeforeSend = func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
		if event.Request != nil {
			collected = *event.Request
		}

		return scrubber(event, hint)
	}

	client, err := sentry.NewClient(options)
	require.NoError(t, err)

	body := `{"name":"` + medicationSentinel + `","note":"twice daily"}`

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		recordsURL+"?q="+medicationSentinel,
		strings.NewReader(body))
	request.Header.Set("Cookie", cookieSentinel)
	request.Header.Set("Authorization", "Bearer "+cookieSentinel)
	request.Header.Set("Referer", "https://medikube.example/"+kind.Medication.Segment()+"?q="+medicationSentinel)

	scope := sentry.NewScope()
	scope.SetRequest(request)
	scope.SetRequestBody([]byte(body))
	scope.SetUser(sentry.User{ID: "pbc0123456789abc", Email: emailSentinel})

	client.CaptureException(errors.New("the store refused the write"), nil, scope)

	events := transport.Events()
	require.Len(t, events, 1, "the event never reached the transport, so nothing below is an assertion")

	require.Equal(t, http.MethodPost, collected.Method,
		"no request was assembled at all, so the emptiness below is emptiness by absence")

	assert.Empty(t, collected.Data, "the SDK buffered the request body into the event")
	assert.Empty(t, collected.QueryString, "the SDK collected the query string into the event")
	assert.Empty(t, collected.Cookies, "the SDK collected the cookies into the event")
	assert.Empty(t, collected.Headers, "the SDK collected the request headers into the event")

	assertNoSentinelInEnvelope(t, events[0])
}

// The sweep that catches the field nobody enumerated. It runs over the
// serialised envelope rather than over named fields, because the field a
// future sentry-go adds is the one no field-by-field assertion mentions.
func assertNoSentinelInEnvelope(t *testing.T, event *sentry.Event) {
	t.Helper()

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	for _, attachment := range event.Attachments {
		payload = append(payload, attachment.Payload...)
	}

	sentinels := []string{medicationSentinel, emailSentinel, cookieSentinel, hostnameSentinel}
	require.NotEmpty(t, sentinels)

	envelope := strings.ToLower(string(payload))
	require.NotEmpty(t, envelope, "the envelope is empty, so this sweep looked at nothing")

	for _, sentinel := range sentinels {
		assert.NotContainsf(t, envelope, strings.ToLower(sentinel),
			"the Sentry envelope carries %q:\n%s", sentinel, payload)
	}
}
