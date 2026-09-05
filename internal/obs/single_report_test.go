package obs_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/logging"
	"medikube/internal/obs"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/phileak"
	"medikube/internal/web"
)

// T284, FR-057: one 500 through the production-shaped chain (RequestLogger,
// Observer, then the handler) produces exactly one log line, one Sentry event
// and one metrics increment.
func TestOneFailureReachesEachSinkExactlyOnce(t *testing.T) {
	t.Parallel()

	var captured strings.Builder
	base := logging.NewTo(&captured, config.LogConfig{Level: "debug"}, "test")

	metrics := obs.NewMetrics("GET /x/boom")

	transport := new(phileak.SentryTransport)
	reporter, err := obs.StartSentryWithTransport(config.SentryConfig{
		DSN:        "http://0123456789abcdef0123456789abcdef@127.0.0.1:1/1",
		SampleRate: 1,
	}, "test", base, transport)
	require.NoError(t, err)
	require.True(t, reporter.Active())

	occurrence := "the store refused the write"

	scenario := tests.ApiScenario{
		Method:          http.MethodGet,
		URL:             "/x/boom",
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{`"code":"internal_error"`},
		TestAppFactory: testsupport.NewAppFactory(binder(func(se *core.ServeEvent) error {
			se.Router.Bind(obs.RequestLogger(base))
			se.Router.Bind(obs.Observer(metrics, reporter))
			se.Router.Bind(web.Errors(nil))
			se.Router.GET("/x/boom", func(e *core.RequestEvent) error {
				return errors.New(occurrence)
			})

			return nil
		})),
	}
	scenario.Test(t)

	assert.Equal(t, 1, strings.Count(captured.String(), "http_request"), "one request, one line")

	events := transport.Events()
	require.Len(t, events, 1, "one occurrence should reach Sentry exactly once")

	rendered, err := json.Marshal(events[0])
	require.NoError(t, err)
	assert.Contains(t, string(rendered), occurrence)

	metricsBody := scrapeRegistry(t, metrics)
	want := "route=" + strconv.Quote("GET /x/boom") + ",status=" + strconv.Quote(strconv.Itoa(http.StatusInternalServerError))
	assert.Equal(t, 1, strings.Count(metricsBody, want))
}

func scrapeRegistry(t testing.TB, metrics *obs.Metrics) string {
	t.Helper()

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{}).ServeHTTP(
		rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	return rec.Body.String()
}
