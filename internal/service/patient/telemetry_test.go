package patient_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/person"
)

// fakeMetrics and fakeTracer are hand-written fakes (constitution Principle
// III): no DB, no HTTP, no filesystem, and no otel import either — proving
// that any type with the right two methods satisfies patient.Metrics /
// patient.Tracer, exactly like *obs.Metrics / *obs.SpanTracer do in
// production.
type fakeMetrics struct {
	created  []string
	switches []string
}

func (f *fakeMetrics) RecordCreated(kind string)    { f.created = append(f.created, kind) }
func (f *fakeMetrics) PatientSwitch(outcome string) { f.switches = append(f.switches, outcome) }

type fakeTracer struct {
	started []string
	ended   []error
}

func (f *fakeTracer) Start(ctx context.Context, spanName string, _ map[string]string) (context.Context, func(error)) {
	f.started = append(f.started, spanName)

	return ctx, func(err error) { f.ended = append(f.ended, err) }
}

func TestCreateReportsTheRecordCreatedMetricAndOpensASpan(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	metrics := &fakeMetrics{}
	tracer := &fakeTracer{}
	svc.SetMetrics(metrics)
	svc.SetTracer(tracer)

	_, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12"),
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"patient"}, metrics.created)
	assert.Equal(t, []string{"service.patient.Create"}, tracer.started)
	require.Len(t, tracer.ended, 1)
	assert.NoError(t, tracer.ended[0])
}

func TestCreateReportsNothingWhenValidationRefuses(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	metrics := &fakeMetrics{}
	svc.SetMetrics(metrics)

	_, err := svc.Create(t.Context(), owner(), person.Patient{})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Empty(t, metrics.created, "an invalid draft must never be counted as a record created")
}

func TestSetActivePatientReportsEveryOutcome(t *testing.T) {
	t.Parallel()

	t.Run("clearing the pointer", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)
		metrics := &fakeMetrics{}
		svc.SetMetrics(metrics)

		_, err := svc.SetActivePatient(t.Context(), owner(), nil)
		require.NoError(t, err)

		assert.Equal(t, []string{"cleared"}, metrics.switches)
	})

	t.Run("an unauthenticated actor", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)
		metrics := &fakeMetrics{}
		svc.SetMetrics(metrics)

		id := "mkfakepatient01"
		_, err := svc.SetActivePatient(t.Context(), access.Actor{}, &id)
		require.ErrorIs(t, err, domain.ErrUnauthenticated)

		assert.Equal(t, []string{"unauthenticated"}, metrics.switches)
	})

	t.Run("choosing a patient that does not exist", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)
		metrics := &fakeMetrics{}
		svc.SetMetrics(metrics)

		id := "mkfakepatient01"
		_, err := svc.SetActivePatient(t.Context(), owner(), &id)
		require.Error(t, err)

		assert.Equal(t, []string{"not_found"}, metrics.switches)
	})
}
