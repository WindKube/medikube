package obs_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/domain"
	"medikube/internal/logging"
	"medikube/internal/obs"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/phileak"
	"medikube/internal/web"
)

// binder adapts a function to the interface testsupport.NewAppFactory takes. It
// deliberately does not call se.Next(): the harness owns the chain.
type binder func(se *core.ServeEvent) error

func (b binder) Bind(se *core.ServeEvent) error { return b(se) }

// chain is the middleware order plan.md sets, as far as this phase has built
// it: the request logger outermost, then the error envelope, and PocketBase's
// panic recovery between the envelope and the handler.
func chain(base zerolog.Logger, view web.ErrorView, build func(se *core.ServeEvent)) testsupport.ServeBinder {
	return binder(func(se *core.ServeEvent) error {
		se.Router.Bind(obs.RequestLogger(base))
		se.Router.Bind(web.Errors(view))
		build(se)

		return nil
	})
}

// lines splits a captured zerolog stream into decoded records, so an assertion
// can be made about how many there are rather than about a substring appearing
// somewhere in the text.
func lines(t *testing.T, captured string) []map[string]any {
	t.Helper()

	var decoded []map[string]any

	for _, raw := range strings.Split(strings.TrimSpace(captured), "\n") {
		if raw == "" {
			continue
		}

		var one map[string]any
		require.NoErrorf(t, json.Unmarshal([]byte(raw), &one), "the log stream is not JSON: %s", raw)
		decoded = append(decoded, one)
	}

	return decoded
}

func requestLines(t *testing.T, captured string) []map[string]any {
	t.Helper()

	var requests []map[string]any

	for _, line := range lines(t, captured) {
		if line[zerolog.MessageFieldName] == "http_request" {
			requests = append(requests, line)
		}
	}

	return requests
}

// event builds a request event around a recorder, for the helpers that need no
// router and no database.
func newEvent(t *testing.T) *core.RequestEvent {
	t.Helper()

	e := &core.RequestEvent{}
	e.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	e.Response = &router.ResponseWriter{ResponseWriter: httptest.NewRecorder()}

	return e
}

// FR-057. A second reporter of the same occurrence is refused, which is the
// mechanism that keeps one failure out of the log twice, out of Sentry twice
// and out of a span twice once those are wired in US3.
func TestOneOccurrenceIsRecordedOnceAndTheSecondReportIsRefused(t *testing.T) {
	t.Parallel()

	e := newEvent(t)

	first := errors.New("the store could not open the database")
	second := errors.New("and the handler noticed separately")

	assert.Nil(t, obs.Fault(e), "a request with nothing wrong carries an occurrence")

	assert.True(t, obs.Report(e, first), "the first report was refused")
	assert.False(t, obs.Report(e, second), "the second report was accepted, so one failure is reported twice")

	assert.Equal(t, first, obs.Fault(e), "the second report replaced the first, which loses the cause")

	assert.False(t, obs.Report(e, nil), "a nil was recorded as an occurrence")
	assert.Equal(t, first, obs.Fault(e))
}

func TestPanickedRecognisesPocketBasesRecoveredPanicAndNothingElse(t *testing.T) {
	t.Parallel()

	recovered := router.NewInternalServerError("", errors.New("[PANIC RECOVER] boom goroutine 1 [running]:"))

	assert.True(t, obs.Panicked(recovered))
	assert.False(t, obs.Panicked(domain.ErrNotFound))
	assert.False(t, obs.Panicked(nil))
	assert.False(t, obs.Panicked(router.NewInternalServerError("", errors.New("an ordinary failure"))))
}

// One line per request, and it carries the id the response carries.
func TestOneRequestProducesExactlyOneLineCarryingTheCorrelationId(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		inbound  string
		expected string
	}{
		{"no inbound id: one is minted", "", ""},
		{"a safe inbound id is honoured", "01JQ8ZTESTID", "01JQ8ZTESTID"},
		{"an unsafe one is replaced", "a medication name", ""},
		{"an over-long one is replaced", strings.Repeat("a", 65), ""},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			var captured strings.Builder
			base := logging.NewTo(&captured, config.LogConfig{Level: zerolog.LevelDebugValue}, "test")

			var header string

			headers := map[string]string{}
			if one.inbound != "" {
				headers[obs.CorrelationHeader] = one.inbound
			}

			scenario := tests.ApiScenario{
				Name:           one.name,
				Method:         http.MethodGet,
				URL:            "/x/ok",
				Headers:        headers,
				ExpectedStatus: http.StatusNoContent,
				TestAppFactory: testsupport.NewAppFactory(chain(base, nil, func(se *core.ServeEvent) {
					se.Router.Route(http.MethodGet, "/x/ok", func(e *core.RequestEvent) error {
						return e.NoContent(http.StatusNoContent)
					})
				})),
				AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
					header = res.Header.Get(obs.CorrelationHeader)
				},
			}
			scenario.Test(t)

			requests := requestLines(t, captured.String())
			require.Len(t, requests, 1, "one request produced %d lines", len(requests))

			assert.NotEmpty(t, header, "the response carries no correlation id to quote")
			assert.Equal(t, header, requests[0][logging.CorrelationField],
				"the id on the wire and the id in the log are different, so quoting one finds nothing")

			if one.expected != "" {
				assert.Equal(t, one.expected, header, "a safe inbound id was not honoured")
			} else if one.inbound != "" {
				assert.NotEqual(t, one.inbound, header, "attacker-controlled free text became a log field")
			}
		})
	}
}

// The panic path end to end: PocketBase recovers it, the envelope answers 500,
// the log carries exactly one line for the request, and the stack reaches
// neither the client nor a second line.
func TestAPanicIsAnsweredWithTheFiveHundredViewAndLoggedExactlyOnce(t *testing.T) {
	t.Parallel()

	const secret = "Amoxicillin 500mg"

	var captured strings.Builder
	base := logging.NewTo(&captured, config.LogConfig{Level: zerolog.LevelDebugValue}, "test")

	var viewCalls int

	view := func(e *core.RequestEvent, status int, failure web.Failure) (bool, error) {
		viewCalls++

		return true, e.HTML(status, `<section aria-label="Something went wrong">`+failure.RequestID+`</section>`)
	}

	scenario := tests.ApiScenario{
		Name:           "a panic in a page handler",
		Method:         http.MethodGet,
		URL:            "/x/panic",
		ExpectedStatus: http.StatusInternalServerError,
		// contracts/pages.md E3: the request id and nothing else.
		ExpectedContent:    []string{`aria-label="Something went wrong"`},
		NotExpectedContent: []string{"PANIC RECOVER", "goroutine", secret},
		TestAppFactory: testsupport.NewAppFactory(chain(base, view, func(se *core.ServeEvent) {
			se.Router.Route(http.MethodGet, "/x/panic", func(e *core.RequestEvent) error {
				panic("a nil map write while reading " + secret)
			})
		})),
	}
	scenario.Test(t)

	assert.Equal(t, 1, viewCalls, "the error view rendered %d times", viewCalls)

	requests := requestLines(t, captured.String())
	require.Len(t, requests, 1, "one panic produced %d request lines", len(requests))
	assert.Equal(t, "error", requests[0]["level"], "a 500 was logged at info level")
	assert.EqualValues(t, http.StatusInternalServerError, requests[0]["status"])
	assert.NotEmpty(t, requests[0]["error"], "the line records the request but not what went wrong")

	// The one place the stack is allowed to be. It is not PHI — file paths and
	// function names — and without it a 500 is unactionable.
	assert.Contains(t, captured.String(), "PANIC RECOVER")
}

// A handler that returns an error rather than panicking is the ordinary case,
// and it must also produce exactly one line.
func TestAHandledFailureProducesOneLineAndNoSecondReport(t *testing.T) {
	t.Parallel()

	var captured strings.Builder
	base := logging.NewTo(&captured, config.LogConfig{Level: zerolog.LevelDebugValue}, "test")

	scenario := tests.ApiScenario{
		Name:            "a domain refusal",
		Method:          http.MethodGet,
		URL:             "/x/missing",
		ExpectedStatus:  http.StatusNotFound,
		ExpectedContent: []string{`"code":"not_found"`},
		TestAppFactory: testsupport.NewAppFactory(chain(base, nil, func(se *core.ServeEvent) {
			se.Router.Route(http.MethodGet, "/x/missing", func(e *core.RequestEvent) error {
				return domain.ErrNotFound
			})
		})),
	}
	scenario.Test(t)

	requests := requestLines(t, captured.String())
	require.Len(t, requests, 1)
	assert.EqualValues(t, http.StatusNotFound, requests[0]["status"])
	assert.Equal(t, "not found", requests[0]["error"],
		"the line does not carry the cause, so the 404 is unattributable")
}

// FR-057 across all four sinks phileak watches. Sentry and the tracer are not
// wired until US3, so the honest assertion today is that the log holds the one
// report and the other three hold nothing at all — which is also what makes
// the second report visible the moment somebody adds one.
func TestASingleOccurrenceReachesExactlyOneSink(t *testing.T) {
	t.Parallel()

	// The sentinel travels in the query string, which is where a search for a
	// medication name ends up and the reason PocketBase's own activity logger
	// is unbound (FR-038). The occurrence itself is PHI-free, as every domain
	// error message is by contract.
	const sentinel = "Amoxicillin"
	const occurrence = "the store refused the write"

	capture := phileak.New(t)
	capture.WatchMetrics(prometheus.NewRegistry())

	scenario := tests.ApiScenario{
		Name:            "one failure, four sinks",
		Method:          http.MethodGet,
		URL:             "/x/boom?q=" + sentinel,
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{`"code":"internal_error"`},
		TestAppFactory: testsupport.NewAppFactory(chain(capture.Logger(), nil, func(se *core.ServeEvent) {
			se.Router.Route(http.MethodGet, "/x/boom", func(e *core.RequestEvent) error {
				return errors.New(occurrence)
			})
		})),
	}
	scenario.Test(t)

	capture.AssertNoSentinels(t, sentinel)

	occurrences := 0

	for _, sink := range capture.Sinks() {
		occurrences += strings.Count(sink.Text, occurrence)
	}

	assert.Equal(t, 1, occurrences,
		"one occurrence reached the sinks %d times, and FR-057 allows one", occurrences)
}

// The stack from a recovered panic never reaches a client, whatever answers.
func TestNoInternalDetailReachesAClient(t *testing.T) {
	t.Parallel()

	var body string

	scenario := tests.ApiScenario{
		Name:            "the envelope, with no view",
		Method:          http.MethodGet,
		URL:             "/x/panic",
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{`"code":"internal_error"`},
		TestAppFactory: testsupport.NewAppFactory(chain(
			logging.NewTo(io.Discard, config.LogConfig{Level: zerolog.LevelInfoValue}, "test"),
			nil,
			func(se *core.ServeEvent) {
				se.Router.Route(http.MethodGet, "/x/panic", func(e *core.RequestEvent) error {
					panic(errors.New("open /var/lib/medikube/pb_data/data.db: permission denied"))
				})
			},
		)),
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			raw, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			body = string(raw)
		},
	}
	scenario.Test(t)

	assert.Contains(t, body, `"code":"internal_error"`)
	assert.Contains(t, body, `"message":"`+web.InternalMessage+`"`)
	assert.NotContains(t, body, "pb_data")
	assert.NotContains(t, body, "permission denied")
	assert.NotContains(t, body, "PANIC RECOVER")
}

// A second recover would report the same panic twice. PocketBase already
// recovers at -1030 and MediKube's chain is built around that rather than on
// top of it; this is the assertion that keeps it so.
func TestMediKubeAddsNoSecondPanicRecovery(t *testing.T) {
	t.Parallel()

	bound := []*hook.Handler[*core.RequestEvent]{
		obs.RequestLogger(logging.NewTo(io.Discard, config.LogConfig{Level: zerolog.LevelInfoValue}, "test")),
		web.Errors(nil),
	}

	handlers := make([]string, 0, len(bound))
	for _, h := range bound {
		handlers = append(handlers, h.Id)
	}

	assert.NotContains(t, handlers, "pbPanicRecover",
		"MediKube bound a handler under PocketBase's panic-recovery id")

	assert.Less(t, web.ErrorsMiddlewarePriority, -1030,
		"the envelope is inside PocketBase's panic recovery, so a recovered panic answers in PocketBase's shape")
}
