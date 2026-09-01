package obs

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
)

// FR-039 and FR-056: no exporter, no spans and no outbound connection until an
// operator names a destination.
//
// The endpoint is a real listener in every row, including the rows that expect
// silence, and it is the same trap sentry_test.go uses. That is the whole
// design of this test: an inactivity assertion that only reads a struct field
// passes just as well when there is no tracing subsystem at all, which is the
// state this repository was in until this task. Pointing the exporter at a
// socket and counting what arrives cannot pass by absence, and the last row
// proves it by making the same assertions come out the other way.
func TestTracingIsEntirelyOffUntilAnEndpointIsConfigured(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		enabled    bool
		endpoint   func(*outboundTrap) string
		wantActive bool
	}{
		{
			name:     "off, with an endpoint configured anyway",
			endpoint: func(trap *outboundTrap) string { return trap.address() },
		},
		{
			name:     "on, with no endpoint to send to",
			enabled:  true,
			endpoint: func(*outboundTrap) string { return "" },
		},
		{
			name:     "on, with an endpoint of whitespace",
			enabled:  true,
			endpoint: func(*outboundTrap) string { return "   " },
		},
		{
			name:       "on, with the endpoint the operator configured",
			enabled:    true,
			endpoint:   func(trap *outboundTrap) string { return trap.address() },
			wantActive: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			trap := newOutboundTrap(t)

			tracing, err := StartTracing(t.Context(), config.OTelConfig{
				Enabled:     tc.enabled,
				Endpoint:    tc.endpoint(trap),
				Insecure:    true,
				SampleRatio: 1,
				Environment: "test",
			}, "test-release", zerolog.Nop())
			require.NoError(t, err)

			assert.Equal(t, tc.wantActive, tracing.Active())

			_, span := tracing.TracerProvider().Tracer("medikube/internal/obs").Start(t.Context(), "a-unit-of-work")
			span.End()

			assert.Equal(t, tc.wantActive, span.SpanContext().IsValid(),
				"the span context disagrees with whether tracing is configured")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			require.NoError(t, tracing.Shutdown(ctx))

			if tc.wantActive {
				assert.Positive(t, trap.connectionCount(),
					"a configured exporter sent nothing, so the silent rows above prove nothing")

				return
			}

			assert.Zero(t, trap.connectionCount(),
				"unconfigured tracing opened an outbound connection (FR-039)")
		})
	}
}

// A span that is not recording costs nothing and carries nothing. It is
// asserted separately from the span context because the two are independent:
// a provider can hand back a valid, sampled-out context, and IsRecording is
// what decides whether an attribute set on it is retained anywhere.
func TestAnInactiveProviderHandsBackASpanThatRecordsNothing(t *testing.T) {
	t.Parallel()

	tracing, err := StartTracing(t.Context(), config.OTelConfig{}, "test-release", zerolog.Nop())
	require.NoError(t, err)
	require.False(t, tracing.Active())

	_, span := tracing.TracerProvider().Tracer("medikube/internal/obs").Start(t.Context(), "a-unit-of-work")
	defer span.End()

	assert.False(t, span.IsRecording())
}

// The resource is process-wide and travels on every span, so it is the one
// attribute set nobody reviews per call site.
//
// It is an allowlist, and the assertion is an exact match rather than an
// absence check: resource.Default() would add the telemetry SDK's own
// attributes and resource.WithHost() would add the hostname of the machine
// somebody's records sit on, and both would pass a test that only looked for
// the three that should be there.
func TestTheTracerResourceCarriesOnlyPublishedAttributes(t *testing.T) {
	t.Parallel()

	res := newResource(config.OTelConfig{Environment: "staging"}, "v1.2.3")
	require.NotNil(t, res)

	got := make(map[string]string, res.Len())
	for _, attribute := range res.Attributes() {
		got[string(attribute.Key)] = attribute.Value.String()
	}

	require.NotEmpty(t, got, "the resource carries no attributes at all, so this comparison compared nothing")

	assert.Equal(t, map[string]string{
		"service.name":                "medikube",
		"service.version":             "v1.2.3",
		"deployment.environment.name": "staging",
	}, got)
}

// The composition root's shutdown path must not have to ask whether tracing
// was ever configured, and neither must a caller that wants a tracer.
func TestAnInactiveTracingIsSafeToUseAndToShutDown(t *testing.T) {
	t.Parallel()

	var tracing *Tracing

	assert.False(t, tracing.Active())
	assert.NotNil(t, tracing.TracerProvider())
	assert.NoError(t, tracing.Shutdown(t.Context()))
}
