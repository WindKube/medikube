package pb

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"

	"medikube/internal/obs"
)

// DrainHookID names MediKube's own OnTerminate handler, so binding it twice
// replaces rather than appends.
const DrainHookID = "medikubeDrain"

// DrainHookPriority runs before PocketBase's own HTTP shutdown, hardcoded at
// -9999 with a 1-second timeout (apis/serve.go:171). By the time that runs,
// this has already flipped readiness, waited the operator's grace period and
// waited out the in-flight counter.
const DrainHookPriority = -10000

// DrainOptions is what the composition root decides: the shared flag, and how
// long each half of the sequence gets.
type DrainOptions struct {
	// Readiness is flipped to draining first, so `readyz` starts answering 503
	// before anything downstream is touched.
	Readiness *obs.Readiness

	// Delay is MEDIKUBE_DRAIN_DELAY: how long to wait, after flipping
	// readiness, for a load balancer or an orchestrator to notice and stop
	// routing new work. config.Validate requires it to be shorter than Max.
	Delay time.Duration

	// Max is MEDIKUBE_DRAIN_MAX: the outside bound on waiting for the
	// in-flight counter to reach zero once the delay has passed.
	Max time.Duration

	// Log records whether the wait ran out before every request finished. It
	// is not a reason to refuse to stop — PocketBase's own shutdown runs
	// either way — but it is worth an operator's attention when it happens.
	Log zerolog.Logger
}

// BindDrain installs the drain sequence.
func BindDrain(app core.App, opts DrainOptions) {
	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id:       DrainHookID,
		Priority: DrainHookPriority,
		Func: func(e *core.TerminateEvent) error {
			// A restart (TerminateEvent.IsRestart) still stops and restarts
			// the HTTP server, so the sequence does not branch on it.
			opts.Readiness.Drain()

			time.Sleep(opts.Delay)

			ctx, cancel := context.WithTimeout(context.Background(), opts.Max)
			defer cancel()

			if err := opts.Readiness.Idle(ctx); err != nil {
				opts.Log.Error().
					Int64("in_flight", opts.Readiness.InFlight()).
					Msg("the drain window closed with a request still in flight")
			}

			return e.Next()
		},
	})
}
