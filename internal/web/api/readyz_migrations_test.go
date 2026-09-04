package api_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/store/migrations"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
)

func TestReadyzNamesMigrationsWhenOneIsOutstanding(t *testing.T) {
	t.Parallel()

	scenario := tests.ApiScenario{
		Method:          http.MethodGet,
		URL:             "/api/v1/readyz",
		ExpectedStatus:  http.StatusServiceUnavailable,
		ExpectedContent: []string{`"status":"not_ready"`, `"migrations":"error"`},
		TestAppFactory:  testsupport.NewAppFactory(bindHealth(api.HealthDeps{Pending: migrations.Pending})),
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
			files := migrations.Files()
			require.NotEmpty(t, files)

			_, err := app.DB().Delete(core.DefaultMigrationsTable, dbx.HashExp{"file": files[0]}).Execute()
			require.NoError(t, err)
		},
	}
	scenario.Test(t)
}
