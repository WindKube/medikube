package obs

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SpanTracer adapts an OTel tracer to the tiny
// `Start(ctx, name, attrs) (context.Context, func(error))` seam
// internal/service/patient and internal/store/patient each declare for
// themselves (FR-038): neither package imports OTel to get a span, and this
// is the one place that satisfies both with the same method set.
//
// Nil-safe throughout: a *SpanTracer nobody built (the zero value a struct
// literal would carry, or a nil pointer) behaves exactly like an instance
// built over a noop TracerProvider, so a package that forgets to call
// SetTracer observes nothing rather than panicking.
type SpanTracer struct {
	tracer trace.Tracer
}

// NewSpanTracer names the span source — "service.patient", "store.patients"
// — the same way NewMetrics is handed the route patterns: once, at wiring
// time, never inferred from a call site.
func NewSpanTracer(provider trace.TracerProvider, name string) *SpanTracer {
	if provider == nil {
		return &SpanTracer{}
	}

	return &SpanTracer{tracer: provider.Tracer(name)}
}

// Start begins spanName and returns the span's own context plus a function
// that ends it. end's argument records only WHETHER the call failed
// (codes.Error) — never err.Error(), which is exactly the free-text
// destination FR-046 forbids a span from becoming.
func (t *SpanTracer) Start(ctx context.Context, spanName string, attrs map[string]string) (context.Context, func(err error)) {
	if t == nil || t.tracer == nil {
		return ctx, func(error) {}
	}

	var options []trace.SpanStartOption
	if len(attrs) > 0 {
		kv := make([]attribute.KeyValue, 0, len(attrs))
		for key, value := range attrs {
			kv = append(kv, attribute.String(key, value))
		}

		options = append(options, trace.WithAttributes(kv...))
	}

	spanCtx, span := t.tracer.Start(ctx, spanName, options...)

	return spanCtx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, "")
		}

		span.End()
	}
}
