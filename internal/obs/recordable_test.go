package obs

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/domain"
	"medikube/internal/testsupport/phileak"
)

// answered builds a request event whose response has already been written
// with status, or not written at all when status is zero.
func answered(t *testing.T, status int) *core.RequestEvent {
	t.Helper()

	e := &core.RequestEvent{}
	e.Request = httptest.NewRequest(http.MethodPost, "/api/collections/users/request-password-reset", nil)
	e.Response = &router.ResponseWriter{ResponseWriter: httptest.NewRecorder()}

	if status != 0 {
		e.Response.WriteHeader(status)
	}

	return e
}

// enumerationOracle is the shape PocketBase's password-reset handler produces:
// a 204 already on the wire, then an error carrying the submitted address so
// that its own activity logger can record it (defect D20).
func enumerationOracle() error {
	return fmt.Errorf("failed to fetch users record with email %s: %w", emailSentinel, errors.New("sql: no rows in result set"))
}

func TestRecordableWithholdsTheCauseBelowAServerFailure(t *testing.T) {
	t.Parallel()

	driver := errors.New("database is locked")

	for _, tc := range []struct {
		name      string
		written   int
		err       error
		wantCause bool
		wantOwn   bool
	}{
		{
			name:    "a success PocketBase paired with an error, the enumeration oracle",
			written: http.StatusNoContent,
			err:     enumerationOracle(),
		},
		{
			name: "a refusal whose status comes from the error",
			err:  router.NewNotFoundError("record not found", enumerationOracle()),
		},
		{
			name:    "a refusal MediKube composed itself around a domain sentinel",
			written: http.StatusNotFound,
			err:     fmt.Errorf("owner-scoped refusal answered as a miss: %w", domain.ErrNotFound),
			wantOwn: true,
		},
		{
			name:    "a refusal whose status is on the wire",
			written: http.StatusBadRequest,
			err:     enumerationOracle(),
		},
		{
			name:      "a server failure behind PocketBase's vague envelope",
			err:       router.NewInternalServerError("Something went wrong while processing your request.", driver),
			wantCause: true,
		},
		{
			name:      "a bare error nothing answered, which is a 500 by default",
			err:       driver,
			wantCause: true,
		},
		{
			name:      "a server failure already on the wire",
			written:   http.StatusBadGateway,
			err:       driver,
			wantCause: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Recordable(answered(t, tc.written), tc.err)
			require.Error(t, got)

			if tc.wantCause {
				assert.Equal(t, driver, got, "a server failure must be recorded as its cause, or the line is not actionable")

				return
			}

			if tc.wantOwn {
				assert.Equal(t, tc.err, got, "a refusal MediKube composed around its own sentinel is PHI-free by contract and must survive")

				return
			}

			assert.NotContains(t, got.Error(), emailSentinel, "the withheld cause still quotes the submitted address")
			assert.NotContains(t, got.Error(), "failed to fetch", "the withheld cause still carries the original message")
			assert.Contains(t, got.Error(), "withheld", "the line must say the cause was withheld rather than look like there was none")
			assert.NoError(t, errors.Unwrap(got), "the stand-in has a chain behind it, and a reporter that walks the chain finds the original") //nolint:testifylint // Unwrap's nil is the assertion
		})
	}

	assert.NoError(t, Recordable(answered(t, 0), nil), "nil in, nil out") //nolint:testifylint // nil is the assertion
}

// TestRequestLoggerWithholdsTheEnumerationOracle is the log half of defect
// D20: the line for a 204 that PocketBase paired with an error must not carry
// the address, because whether the line exists at all is then an oracle for
// whether the address has an account (FR-073).
func TestRequestLoggerWithholdsTheEnumerationOracle(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	_ = exercise(t, base, func(e *core.RequestEvent) error {
		e.Response.WriteHeader(http.StatusNoContent)

		return enumerationOracle()
	})

	entry := single(t, buf)
	assert.EqualValues(t, http.StatusNoContent, entry["status"])
	assert.NotContains(t, buf.String(), emailSentinel, "the address reached the log stream")
	assert.Contains(t, buf.String(), "withheld", "the line does not say the cause was withheld")
}

func TestRequestLoggerStillRecordsTheCauseOfAServerFailure(t *testing.T) {
	t.Parallel()

	buf, base := capture(t)
	_ = exercise(t, base, func(_ *core.RequestEvent) error {
		return router.NewInternalServerError("Something went wrong while processing your request.", errors.New("database is locked"))
	})

	entry := single(t, buf)
	assert.EqualValues(t, http.StatusInternalServerError, entry["status"])
	assert.Equal(t, "database is locked", entry["error"], "the driver failure is what makes a 500 actionable and must survive")
}

func reporterInto(t *testing.T, transport *phileak.SentryTransport) *Reporter {
	t.Helper()

	reporter, err := startSentry(config.SentryConfig{
		DSN:         "http://0123456789abcdef0123456789abcdef@127.0.0.1:1/1",
		Environment: "test",
		SampleRate:  1,
	}, "test-release", zerolog.Nop(), transport)
	require.NoError(t, err)
	require.True(t, reporter.Active())

	return reporter
}

// TestSentryReportWithholdsTheEnumerationOracle is the Sentry half of D20.
func TestSentryReportWithholdsTheEnumerationOracle(t *testing.T) {
	t.Parallel()

	transport := new(phileak.SentryTransport)
	reporter := reporterInto(t, transport)

	require.True(t, reporter.Report(answered(t, http.StatusNoContent), enumerationOracle()))

	events := transport.Events()
	require.Len(t, events, 1, "the event never reached the transport, so nothing below is an assertion")
	require.NotEmpty(t, events[0].Exception, "no exception was assembled, so a clean envelope says nothing")

	assertNoSentinelInEnvelope(t, events[0])
}

// TestSentryReportsOneExceptionValueNotTheChain pins the scrubber's
// truncation: sentry-go serialises every error Unwrap reaches, and only the
// outermost message is something the reporter chose to send.
func TestSentryReportsOneExceptionValueNotTheChain(t *testing.T) {
	t.Parallel()

	transport := new(phileak.SentryTransport)
	reporter := reporterInto(t, transport)

	inner := fmt.Errorf("resolving %s: %w", medicationSentinel, errors.New("sql: no rows in result set"))
	outer := router.NewInternalServerError("Something went wrong while processing your request.",
		fmt.Errorf("saving the record: %w", inner))

	require.True(t, reporter.Report(answered(t, 0), outer))

	events := transport.Events()
	require.Len(t, events, 1)

	sent := events[0]
	require.Len(t, sent.Exception, 1, "the whole chain was serialised:\n%s", exceptionValues(sent))
	assert.Equal(t, "saving the record: resolving "+medicationSentinel+": sql: no rows in result set", sent.Exception[0].Value,
		"the cause of a 500 is recorded as the operator needs it, wrapped context included")
	assert.NotNil(t, sent.Exception[0].Stacktrace, "a 500 without a stack trace is a 500 nobody can place")
}

func exceptionValues(event *sentry.Event) string {
	var out string

	for _, exception := range event.Exception {
		out += exception.Type + ": " + exception.Value + "\n"
	}

	return out
}
