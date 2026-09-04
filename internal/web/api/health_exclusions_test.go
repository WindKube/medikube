package api_test

import (
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
	"medikube/internal/web/api"
)

// TestProbeTrafficAppearsInNeitherTheLogNorTheMetrics is T277: healthz and
// readyz, excluded by pattern, produce no "http_request" line and no metrics
// observation, while an ordinary route still produces both.
func TestProbeTrafficAppearsInNeitherTheLogNorTheMetrics(t *testing.T) {
	t.Parallel()

	var logs strings.Builder
	log := logging.NewTo(&logs, config.LogConfig{Level: "debug"}, "test")
	metrics := obs.NewMetrics("GET /x/ok")
	reporter, err := obs.StartSentry(config.SentryConfig{}, "test", log)
	require.NoError(t, err)

	probes := []string{"GET /api/v1/healthz", "GET /api/v1/readyz"}

	scenario := tests.ApiScenario{
		Method:         http.MethodGet,
		URL:            "/api/v1/healthz",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"status":"ok"`,
		},
		TestAppFactory: testsupport.NewAppFactory(binder(func(se *core.ServeEvent) error {
			se.Router.Bind(obs.RequestLogger(log, probes...))
			se.Router.Bind(obs.Observer(metrics, reporter, probes...))

			handlers := api.HealthHandlers(api.HealthDeps{})
			se.Router.GET("/api/v1/healthz", handlers[api.OpHealthz])
			se.Router.GET("/x/ok", func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) })

			return nil
		})),
	}
	scenario.Test(t)

	assert.NotContains(t, logs.String(), "http_request", "the probe was logged")
	assert.NotContains(t, scrapeMetrics(t, metrics), "route="+strconv.Quote("GET /api/v1/healthz"), "the probe was metered")

	// Control: an ordinary route through the same chain IS logged and metered.
	logs.Reset()

	controlScenario := tests.ApiScenario{
		Method:         http.MethodGet,
		URL:            "/x/ok",
		ExpectedStatus: http.StatusNoContent,
		TestAppFactory: testsupport.NewAppFactory(binder(func(se *core.ServeEvent) error {
			se.Router.Bind(obs.RequestLogger(log, probes...))
			se.Router.Bind(obs.Observer(metrics, reporter, probes...))
			se.Router.GET("/x/ok", func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) })

			return nil
		})),
	}
	controlScenario.Test(t)

	assert.Contains(t, logs.String(), "http_request", "an ordinary route was not logged")
	assert.Contains(t, scrapeMetrics(t, metrics), "route="+strconv.Quote("GET /x/ok"), "an ordinary route was not metered")
}

func scrapeMetrics(t testing.TB, metrics *obs.Metrics) string {
	t.Helper()

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{}).ServeHTTP(
		rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	return rec.Body.String()
}
