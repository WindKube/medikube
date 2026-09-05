package patient_test

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"medikube/internal/domain"
	"medikube/internal/domain/person"
	"medikube/internal/obs"
	"medikube/internal/service/patient/patienttest"
	"medikube/internal/store"
	pbpatient "medikube/internal/store/patient"

	// The migrations register themselves from their own init, mirroring
	// repo_integration_test.go's own comment.
	_ "medikube/internal/store/migrations"
)

// endedSpanRecorder is a hand-written sdktrace.SpanProcessor, not
// go.opentelemetry.io/otel/sdk/trace/tracetest's own: internal/testsupport/
// phileak's sole_test.go (cross-artifact finding M6) refuses that import
// anywhere outside itself, so a legitimate unit test for SpanTracer's
// mechanism writes the four-method interface by hand instead.
type endedSpanRecorder struct {
	ended []sdktrace.ReadOnlySpan
}

func (r *endedSpanRecorder) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (r *endedSpanRecorder) OnEnd(s sdktrace.ReadOnlySpan)                   { r.ended = append(r.ended, s) }
func (r *endedSpanRecorder) Shutdown(context.Context) error                  { return nil }
func (r *endedSpanRecorder) ForceFlush(context.Context) error                { return nil }

var _ sdktrace.SpanProcessor = (*endedSpanRecorder)(nil)

// T160. store.patients.* spans, proven against a real Repo rather than a
// fake: SetTracer is the one seam a composition root uses, and this is what
// proves it actually reaches OTel and not merely that the interface compiles.
func TestRepoCreateUpdateDeleteOpenTheirOwnSpans(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbpatient.New(app, codec)
	require.NoError(t, err)

	recorder := &endedSpanRecorder{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	repo.SetTracer(obs.NewSpanTracer(provider, "store.patients"))

	ownerID := "mktelemetryownr"
	seedAccountWithID(t, app, ownerID, "telemetry-owner@example.test")

	created, err := repo.Create(t.Context(), person.Patient{
		OwnerID: ownerID, FirstName: "Amara", LastName: "Okonkwo",
		BirthDate: mustParseDate(t, "1988-04-12"),
	})
	require.NoError(t, err)

	created.Address = "221B Baker Street"
	updated, err := repo.Update(t.Context(), created, created.Version)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(t.Context(), ownerID, updated.ID, updated.Version))

	var names []string
	for _, span := range recorder.ended {
		names = append(names, span.Name())
	}

	assert.Equal(t, []string{"store.patients.Create", "store.patients.Update", "store.patients.Delete"}, names)
}

func mustParseDate(t *testing.T, text string) domain.Date {
	t.Helper()

	date, err := domain.ParseDate(text)
	require.NoError(t, err)

	return date
}

// TestPhotoStoreReportsBytesAndThumbDuration is T160's other half:
// medikube_files_photo_bytes and medikube_files_thumb_duration_seconds{size},
// proven against a real upload rather than a fake.
func TestPhotoStoreReportsBytesAndThumbDuration(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	remapAccounts(t, app)
	patientID := seedOnePatient(t, app, "Amara", "Okonkwo")

	photos, err := pbpatient.NewPhotoStore(app, thumbSizes)
	require.NoError(t, err)

	metrics := obs.NewMetrics()
	photos.SetMetrics(metrics)

	upload := onePixelUpload(t)
	_, err = photos.Put(t.Context(), patienttest.OwnerID, patientID, upload)
	require.NoError(t, err)

	families, err := metrics.Registry().Gather()
	require.NoError(t, err)

	var (
		sawPhotoBytes bool
		thumbSeries   []string
	)

	for _, family := range families {
		switch family.GetName() {
		case "medikube_files_photo_bytes":
			require.NotEmpty(t, family.GetMetric())
			sawPhotoBytes = true
			assert.Positive(t, family.GetMetric()[0].GetHistogram().GetSampleCount())
		case "medikube_files_thumb_duration_seconds":
			for _, metric := range family.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "size" {
						thumbSeries = append(thumbSeries, label.GetValue())
					}
				}
			}
		}
	}

	assert.True(t, sawPhotoBytes, "medikube_files_photo_bytes recorded nothing")
	assert.ElementsMatch(t, thumbSizes, thumbSeries)
}
