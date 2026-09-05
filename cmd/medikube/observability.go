package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"

	"medikube/internal/config"
	"medikube/internal/httproute"
	"medikube/internal/obs"
)

// sinksShutdownHookID names the terminate handler that flushes the three
// operational destinations, so binding it twice replaces rather than appends.
const sinksShutdownHookID = "medikubeObservabilityShutdown"

// sinksShutdownPriority runs the flush late: a span or an event raised while
// something else is shutting down is exactly the one worth having, and an
// exporter closed first would drop it.
const sinksShutdownPriority = 10000

// sinksShutdownTimeout bounds the flush. An exporter whose destination has gone
// away blocks until its own deadline, and a process that will not exit is worse
// than a trace nobody receives.
const sinksShutdownTimeout = 10 * time.Second

// sinks are the three operational destinations, held together because they
// share a lifetime and nothing else.
//
// None of them is on by default and none of them is reachable from a request
// path here: FR-039 and FR-056 make an unconfigured destination structurally
// inert rather than quietly enabled, and internal/obs is where that is proven.
// This file only decides when they start and when they stop.
type sinks struct {
	tracing *obs.Tracing
	sentry  *obs.Reporter
	metrics *obs.MetricsServer

	// measurements is the registry the listener serves and the thing a request
	// middleware records into. It is held rather than discarded because the
	// listener is one half of FR-055 and the recording is the other, and a
	// registry nothing writes to is an endpoint that publishes the Go runtime
	// and nothing about MediKube.
	measurements *obs.Metrics
}

// startSinks builds the two destinations that do not need the route table.
//
// Tracing first, and before pb.New: the database connection function is built
// out of the tracer provider (T247), and PocketBase reads Config.DBConnect
// during bootstrap, so a provider installed afterwards would leave every query
// untraced with nothing to show for the configuration.
func startSinks(ctx context.Context, cfg config.Config, log zerolog.Logger) (*sinks, error) {
	tracing, err := obs.StartTracing(ctx, cfg.OTel, version, log)
	if err != nil {
		return nil, fmt.Errorf("start tracing: %w", err)
	}

	// The global provider, set here and nowhere else. internal/obs deliberately
	// does not set it — a package that installed the process's tracer as a side
	// effect of being constructed would make every test that built one the
	// process's tracer too.
	otel.SetTracerProvider(tracing.TracerProvider())

	reporter, err := obs.StartSentry(cfg.Sentry, version, log)
	if err != nil {
		return nil, fmt.Errorf("start the error reporter: %w", err)
	}

	// Built here, before the route table exists, and not in startMetrics:
	// this phase's counters (RecordCreated, PatientSwitch, ...) are wired
	// into the service and store layers while the route table is still being
	// assembled, and PublishRoutes adds the route-pattern allowlist once it
	// is. The registry answers scrapes with an empty route allowlist for the
	// brief window before that — never with no registry at all.
	measurements := obs.NewMetrics()

	return &sinks{tracing: tracing, sentry: reporter, measurements: measurements}, nil
}

// startMetrics binds the measurement listener, which can only happen once the
// route table exists.
//
// The label allowlist is the registry's own patterns rather than a list kept
// beside it: a route added to internal/httproute and not to this call would
// have every one of its requests recorded as `other`, which is a metric that
// silently stops measuring the thing somebody just added (FR-055).
func (s *sinks) startMetrics(ctx context.Context, cfg config.MetricsConfig, registry *httproute.Registry, log zerolog.Logger) error {
	routes := registry.Routes()

	patterns := make([]string, 0, len(routes))
	for _, route := range routes {
		patterns = append(patterns, route.Pattern())
	}

	s.measurements.PublishRoutes(patterns...)

	server, err := obs.StartMetrics(ctx, cfg, s.measurements, log)
	if err != nil {
		return fmt.Errorf("start the measurements listener: %w", err)
	}

	s.metrics = server

	return nil
}

// bindShutdown flushes the three on the way out.
//
// On OnTerminate rather than on a defer in run(): PocketBase owns the signal
// handling and the shutdown sequence, and a defer would run after apis.Serve
// had already returned — which is late enough that the spans describing the
// shutdown have nowhere to go.
func (s *sinks) bindShutdown(app core.App, log zerolog.Logger) {
	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id:       sinksShutdownHookID,
		Priority: sinksShutdownPriority,
		Func: func(e *core.TerminateEvent) error {
			if err := s.shutdown(); err != nil {
				// Reported, not returned. A destination that would not flush
				// is worth saying out loud and is not a reason to refuse to
				// stop: returning it here aborts PocketBase's own termination
				// sequence, and the process would stay up because a trace
				// exporter could not be reached.
				log.Error().Err(err).Msg("flush the operational destinations")
			}

			return e.Next()
		},
	})
}

// shutdown flushes everything and reports every failure rather than the first.
//
// Ordered by what still needs the others: the tracer flushes the spans, the
// reporter flushes the events those spans may have raised, and the measurement
// listener stops answering last, so an operator watching a drain sees it to the
// end.
func (s *sinks) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), sinksShutdownTimeout)
	defer cancel()

	return errors.Join(
		s.tracing.Shutdown(ctx),
		s.sentry.Shutdown(ctx),
		s.metrics.Shutdown(ctx),
	)
}
