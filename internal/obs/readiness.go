package obs

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// idlePollInterval is how often Idle rechecks the in-flight count.
const idlePollInterval = 2 * time.Millisecond

// InFlightMiddlewareID names the in-flight counter, so binding it twice
// replaces rather than appends.
const InFlightMiddlewareID = "medikubeInFlight"

// inFlightPriority is outside both RequestLogger (-10) and Observer (-11), so
// a request is counted in flight for the whole time either of them could still
// be measuring it.
const inFlightPriority = apis.DefaultActivityLoggerMiddlewarePriority - 12

// Readiness is the drain flag and in-flight counter readyz and the terminate
// handler share (FR-062).
type Readiness struct {
	draining atomic.Bool
	inflight atomic.Int64
}

// NewReadiness returns a Readiness that is neither draining nor carrying any
// in-flight work, which is every instance's state before its first request.
func NewReadiness() *Readiness {
	return &Readiness{}
}

// Begin records one more request in flight. Nil-safe, so a build that wires no
// Readiness still binds the counting middleware without a nil check.
func (r *Readiness) Begin() {
	if r == nil {
		return
	}

	r.inflight.Add(1)
}

// End records that a request in flight has finished.
func (r *Readiness) End() {
	if r == nil {
		return
	}

	r.inflight.Add(-1)
}

// InFlight is the current count.
func (r *Readiness) InFlight() int64 {
	if r == nil {
		return 0
	}

	return r.inflight.Load()
}

// Drain flips the flag readyz reads. It never unflips.
func (r *Readiness) Drain() {
	if r == nil {
		return
	}

	r.draining.Store(true)
}

// Draining reports whether the instance has begun shutting down.
func (r *Readiness) Draining() bool {
	return r != nil && r.draining.Load()
}

// Idle blocks until no request is in flight or ctx is done, whichever comes
// first.
func (r *Readiness) Idle(ctx context.Context) error {
	if r.InFlight() == 0 {
		return nil
	}

	ticker := time.NewTicker(idlePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if r.InFlight() == 0 {
				return nil
			}
		}
	}
}

// TrackInFlight keeps Readiness's counter honest.
//
// Bound on the router rather than in web.Outermost: a CORS preflight or a
// ServeMux redirect never reaches the router at all, so nothing worth waiting
// for on shutdown is missed by counting only what the router sees. Probes
// (healthz, readyz) are counted too — excluding them would save nothing and
// need its own exclusion list.
func TrackInFlight(readiness *Readiness) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id: InFlightMiddlewareID,
		// Outside RequestLogger and Observer, so the count covers everything
		// either of them measures and nothing narrower.
		Priority: inFlightPriority,
		Func: func(e *core.RequestEvent) error {
			readiness.Begin()
			defer readiness.End()

			return e.Next()
		},
	}
}
