package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/logging"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
)

func TestReadyzAnswersReadyWithTheThreeChecks(t *testing.T) {
	t.Parallel()

	scenario := tests.ApiScenario{
		Method:          http.MethodGet,
		URL:             "/api/v1/readyz",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"status":"ready"`, `"database":"ok"`, `"migrations":"ok"`, `"storage":"ok"`},
		TestAppFactory:  testsupport.NewAppFactory(bindHealth(api.HealthDeps{})),
	}
	scenario.Test(t)
}

func TestReadyzReportsAFailingDatabaseAndLeaksNothingOfIt(t *testing.T) {
	t.Parallel()

	var logs strings.Builder
	log := logging.NewTo(&logs, config.LogConfig{Level: "debug"}, "test")

	var body string

	scenario := tests.ApiScenario{
		Method:         http.MethodGet,
		URL:            "/api/v1/readyz",
		ExpectedStatus: http.StatusServiceUnavailable,
		ExpectedContent: []string{
			`"status":"not_ready"`, `"database":"error"`, `"migrations":"ok"`, `"storage":"ok"`,
		},
		TestAppFactory: testsupport.NewAppFactory(bindHealthWithLogger(api.HealthDeps{}, log)),
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
			closeDB(t, app)
		},
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			t.Helper()

			raw, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			body = string(raw)
		},
	}
	scenario.Test(t)

	for _, forbidden := range []string{"sql:", "database is closed", "pb_data", ".db"} {
		assert.NotContains(t, body, forbidden, "the response body leaked a driver detail")
	}

	assert.Contains(t, logs.String(), "closed", "the underlying database error was not logged")
}
