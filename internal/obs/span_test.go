package obs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// endedSpanRecorder is a hand-written sdktrace.SpanProcessor rather than
// go.opentelemetry.io/otel/sdk/trace/tracetest's own: internal/testsupport/
// phileak's sole_test.go (cross-artifact finding M6) refuses that import
// anywhere outside itself, on the theory that a second span capture is how a
// second, partial PHI-leak harness gets built by accident. This one exists to
// prove SpanTracer's own mechanism, never to scan for a sentinel.
type endedSpanRecorder struct {
	ended []sdktrace.ReadOnlySpan
}

func newEndedSpanRecorder() *endedSpanRecorder { return &endedSpanRecorder{} }

func (r *endedSpanRecorder) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (r *endedSpanRecorder) OnEnd(s sdktrace.ReadOnlySpan)                   { r.ended = append(r.ended, s) }
func (r *endedSpanRecorder) Shutdown(context.Context) error                  { return nil }
func (r *endedSpanRecorder) ForceFlush(context.Context) error                { return nil }

var _ sdktrace.SpanProcessor = (*endedSpanRecorder)(nil)

// T160. internal/service/patient and internal/store/patient each declare
// their own tiny Start(ctx, name, attrs) seam so neither imports OTel; this is
// the one adapter that satisfies both, so the test lives here rather than
// twice.
func TestSpanTracerRecordsTheNameTheAttributesAndTheOutcome(t *testing.T) {
	t.Parallel()

	recorder := newEndedSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	tracer := NewSpanTracer(provider, "service.patient")

	ctx, end := tracer.Start(t.Context(), "service.patient.Create", map[string]string{"medikube.kind": "patient"})
	require.NotNil(t, ctx)
	end(nil)

	ctx, end = tracer.Start(t.Context(), "service.patient.SetActivePatient", nil)
	require.NotNil(t, ctx)
	end(errors.New("a driver failure whose text must never reach the span"))

	spans := recorder.ended
	require.Len(t, spans, 2)

	assert.Equal(t, "service.patient.Create", spans[0].Name())
	require.Len(t, spans[0].Attributes(), 1)
	assert.Equal(t, "medikube.kind", string(spans[0].Attributes()[0].Key))
	assert.Equal(t, "patient", spans[0].Attributes()[0].Value.AsString())
	assert.Equal(t, codes.Unset, spans[0].Status().Code)

	assert.Equal(t, "service.patient.SetActivePatient", spans[1].Name())
	assert.Equal(t, codes.Error, spans[1].Status().Code)
	assert.Empty(t, spans[1].Status().Description,
		"the span status carries a driver's error text, which FR-046 forbids")
}

// A *SpanTracer nobody built — the zero value, or one handed a nil provider —
// must not panic a caller that has no reason to nil-check it, the same
// invariant NewMetrics's own instruments carry.
func TestSpanTracerIsNilSafe(t *testing.T) {
	t.Parallel()

	var nilTracer *SpanTracer

	assert.NotPanics(t, func() {
		ctx, end := nilTracer.Start(t.Context(), "op", map[string]string{"k": "v"})
		require.NotNil(t, ctx)
		end(errors.New("boom"))
	})

	unwired := NewSpanTracer(nil, "service.patient")
	assert.NotPanics(t, func() {
		ctx, end := unwired.Start(t.Context(), "op", nil)
		require.NotNil(t, ctx)
		end(nil)
	})
}
