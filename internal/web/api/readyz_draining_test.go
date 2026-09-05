package api_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"medikube/internal/obs"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
)

func TestReadyzAnswersDrainingWithAnEmptyCheckSet(t *testing.T) {
	t.Parallel()

	readiness := obs.NewReadiness()
	readiness.Drain()

	scenario := tests.ApiScenario{
		Method:          http.MethodGet,
		URL:             "/api/v1/readyz",
		ExpectedStatus:  http.StatusServiceUnavailable,
		ExpectedContent: []string{`"status":"draining"`, `"checks":{}`},
		TestAppFactory:  testsupport.NewAppFactory(bindHealth(api.HealthDeps{Readiness: readiness})),
	}
	scenario.Test(t)
}

// An in-flight request still completes successfully while draining — readyz
// answering draining is a signal to stop routing NEW work, not a reason to
// fail work already accepted.
func TestAnInFlightRequestStillCompletesWhileDraining(t *testing.T) {
	t.Parallel()

	readiness := obs.NewReadiness()
	readiness.Drain()

	scenario := tests.ApiScenario{
		Method:          http.MethodGet,
		URL:             "/api/v1/healthz",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"status":"ok"`},
		TestAppFactory:  testsupport.NewAppFactory(bindHealth(api.HealthDeps{Readiness: readiness})),
	}
	scenario.Test(t)
}
