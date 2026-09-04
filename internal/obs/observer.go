package obs

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// ObserverID names the metrics-and-Sentry middleware, so binding it twice
// replaces rather than appends.
const ObserverID = "medikubeObserver"

// observerPriority is outside RequestLogger, mirroring phileak's own private
// observer(): both need the final status and the recorded fault.
const observerPriority = apis.DefaultActivityLoggerMiddlewarePriority - 11

// Observer records a request into the two destinations RequestLogger does
// not: the Prometheus registry and the Sentry reporter. excluded is
// contracts/health.md's probe traffic, as registered patterns.
func Observer(metrics *Metrics, reporter *Reporter, excluded ...string) *hook.Handler[*core.RequestEvent] {
	skip := newPatternSet(excluded)

	return &hook.Handler[*core.RequestEvent]{
		Id:       ObserverID,
		Priority: observerPriority,
		Func: func(e *core.RequestEvent) error {
			start := time.Now()

			err := e.Next()

			pattern := e.Request.Pattern
			if skip.has(pattern) {
				return err
			}

			// FR-057: one occurrence, and this is the only call site that
			// reports one to Sentry.
			fault := err
			if fault == nil {
				fault = Fault(e)
			}

			answered := status(e, err)

			if metrics != nil {
				metrics.ObserveRequest(pattern, e.Request.Method, answered, time.Since(start))
			}

			// Below 500 is an anticipated refusal, not a failure worth Sentry.
			if answered >= http.StatusInternalServerError && fault != nil && reporter != nil {
				reporter.Report(e, fault)
			}

			return err
		},
	}
}
