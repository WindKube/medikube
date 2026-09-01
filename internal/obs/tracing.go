package obs

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"medikube/internal/config"
)

// tracerServiceName is what MediKube calls itself on a span. It is a constant
// and not a hostname, a pod name or anything else derived from where the
// process happens to be running.
const tracerServiceName = "medikube"

// Tracing is the tracer provider and the exporter behind it.
//
// Inactive is the zero value and inactive is a noop provider, not a configured
// provider with no exporter: a provider that samples and records spans nobody
// exports still allocates them, still runs every attribute the instrumentation
// sets, and still gives a later change somewhere to send them.
type Tracing struct {
	provider *sdktrace.TracerProvider
}

// StartTracing builds the tracer provider for cfg.
//
// Without an endpoint — or with tracing switched off, whatever the endpoint
// says — no exporter is constructed, so there is nothing that could open a
// connection (FR-039, FR-056).
//
// It does not call otel.SetTracerProvider. The global provider is process
// state every package can reach and it decides what the whole binary traces;
// the composition root installs this one, and a test that builds a provider
// does not silently become the process's tracer.
func StartTracing(ctx context.Context, cfg config.OTelConfig, release string, log zerolog.Logger) (*Tracing, error) {
	// Unconditional, and not only when tracing is on: otel's default handler
	// writes export and instrumentation failures to stderr with the standard
	// log package, which is one more stream than MediKube has (Principle VI),
	// and otelsql raises them whether or not a tracer is installed.
	otel.SetErrorHandler(traceErrorHandler{log: log})

	if !cfg.Enabled || strings.TrimSpace(cfg.Endpoint) == "" {
		return &Tracing{}, nil
	}

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithHeaders(cfg.Headers),
	}

	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("build the OTLP trace exporter for %s: %w", cfg.Endpoint, err)
	}

	return &Tracing{provider: sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(cfg, release)),
		// ParentBased so that a sampling decision a caller already made is
		// honoured rather than re-rolled halfway through a trace.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)}, nil
}

// Active reports whether an operator has configured a destination.
func (t *Tracing) Active() bool { return t != nil && t.provider != nil }

// TracerProvider is always usable and never nil, so a caller that wants a span
// does not have to ask whether tracing is on.
func (t *Tracing) TracerProvider() trace.TracerProvider {
	if !t.Active() {
		return noop.NewTracerProvider()
	}

	return t.provider
}

// Shutdown flushes whatever is batched and stops the exporter. It is nil-safe
// and inactive-safe.
func (t *Tracing) Shutdown(ctx context.Context) error {
	if !t.Active() {
		return nil
	}

	if err := t.provider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shut the tracer provider down: %w", err)
	}

	return nil
}

// newResource is the process-wide attribute set every span carries.
//
// It is built by hand rather than from resource.Default() or the environment
// detectors, and that is the point: the detectors add the host name, the
// process command line and the process owner, each of which names the machine
// or the operator rather than the service. FR-038's rule for spans is an
// allowlist, and this is it.
func newResource(cfg config.OTelConfig, release string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(tracerServiceName),
		semconv.ServiceVersion(release),
		semconv.DeploymentEnvironmentNameKey.String(cfg.Environment),
	)
}

// traceErrorHandler puts OpenTelemetry's own failures on the one stream.
type traceErrorHandler struct {
	log zerolog.Logger
}

func (h traceErrorHandler) Handle(err error) {
	h.log.Error().Err(err).Str("component", "otel").Msg("otel_error")
}
