package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"medikube/internal/obs"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
)

// binder adapts a func to testsupport.ServeBinder.
type binder func(se *core.ServeEvent) error

func (b binder) Bind(se *core.ServeEvent) error { return b(se) }

// bindHealth registers healthz and readyz directly on the router, without the
// rest of the composition root.
func bindHealth(deps api.HealthDeps) testsupport.ServeBinder {
	return bindHealthWithLogger(deps, zerolog.Nop())
}

// bindHealthWithLogger is bindHealth plus obs.RequestLogger, for the tests
// that assert on what readyz logs.
func bindHealthWithLogger(deps api.HealthDeps, log zerolog.Logger) testsupport.ServeBinder {
	return binder(func(se *core.ServeEvent) error {
		se.Router.Bind(obs.RequestLogger(log))

		handlers := api.HealthHandlers(deps)
		se.Router.GET("/api/v1/healthz", handlers[api.OpHealthz])
		se.Router.GET("/api/v1/readyz", handlers[api.OpReadyz])

		return nil
	})
}

// closeDB breaks the app's database so a handler that touches it fails.
func closeDB(t testing.TB, app *tests.TestApp) {
	t.Helper()

	db, ok := app.ConcurrentDB().(*dbx.DB)
	require.True(t, ok)
	require.NoError(t, db.DB().Close())
}

func TestHealthzAnswersOkAndTouchesNoDatabase(t *testing.T) {
	t.Parallel()

	deps := api.HealthDeps{
		Version:   "1.2.3-test",
		StartedAt: time.Date(2026, 8, 27, 9, 14, 2, 0, time.UTC),
	}

	scenario := tests.ApiScenario{
		Method:         http.MethodGet,
		URL:            "/api/v1/healthz",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"status":"ok"`, `"version":"1.2.3-test"`, `"started_at":"2026-08-27T09:14:02Z"`,
		},
		TestAppFactory: testsupport.NewAppFactory(bindHealth(deps)),
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
			closeDB(t, app)
		},
	}
	scenario.Test(t)
}
