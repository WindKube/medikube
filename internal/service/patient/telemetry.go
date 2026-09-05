package patient

import "context"

// Metrics is FR-055's observability seam: the service reports what happened
// in bounded, pre-agreed vocabulary and never how it is exported or stored.
// *obs.Metrics satisfies this with no import here, the same way
// *store/patient.PhotoStore satisfies PhotoStore without this package
// knowing PocketBase exists.
type Metrics interface {
	// RecordCreated counts one record of kind coming into existence
	// (medikube_records_total{kind}).
	RecordCreated(kind string)
	// PatientSwitch counts one active-patient change attempt by its outcome
	// (medikube_patients_switch_total{outcome}).
	PatientSwitch(outcome string)
}

// Tracer is FR-038's span seam: one span per public method, ended with
// whether it failed and never with the error's own text — a driver message
// or a validation detail is exactly the free-text destination FR-046
// forbids. attrs is the allowlist itself: nothing this package does not
// explicitly pass ever reaches a span.
type Tracer interface {
	Start(ctx context.Context, spanName string, attrs map[string]string) (context.Context, func(err error))
}

// noopMetrics and noopTracer are the zero-value behaviour: a service nobody
// called SetMetrics/SetTracer on observes nothing rather than panicking on a
// nil interface, so wiring them stays optional at every call site.
type noopMetrics struct{}

func (noopMetrics) RecordCreated(string) {}
func (noopMetrics) PatientSwitch(string) {}

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, _ string, _ map[string]string) (context.Context, func(error)) {
	return ctx, func(error) {}
}

// SetMetrics wires the counters. Composition-root-only: nothing in this
// package calls it, so a test that never does gets noopMetrics and observes
// nothing, which is the correct behaviour for a build with no destination
// configured.
func (s *Service) SetMetrics(metrics Metrics) {
	if metrics == nil {
		metrics = noopMetrics{}
	}

	s.metrics = metrics
}

// SetTracer wires the tracer, the same optional way.
func (s *Service) SetTracer(tracer Tracer) {
	if tracer == nil {
		tracer = noopTracer{}
	}

	s.tracer = tracer
}

// span starts a span for one method, defaulting to the noop tracer when
// nobody has called SetTracer.
func (s *Service) span(ctx context.Context, name string, attrs map[string]string) (context.Context, func(error)) {
	if s.tracer == nil {
		return ctx, func(error) {}
	}

	return s.tracer.Start(ctx, name, attrs)
}

func (s *Service) metricsOrNoop() Metrics {
	if s.metrics == nil {
		return noopMetrics{}
	}

	return s.metrics
}
