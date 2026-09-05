package patient_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	auditstore "medikube/internal/store/audit"

	_ "medikube/internal/store/migrations"
)

// BenchmarkPatientChartSummaryWith50kMedications is SC-004's own measurement:
// a patient holding 50,000 medications, and the chart's per-kind count plus
// its recent-activity read both answering within 2 seconds at the 95th
// percentile. It is a Go benchmark rather than a test, so `go test` (and
// `task test`) never runs it: `go test -bench . ./internal/store/patient/...`
// does.
//
// The query shape measured is exactly contracts/patient-chart.md's: one
// indexed COUNT(*) per registered kind (one kind, this phase) plus one
// LIMIT-10 scan of the audit trail's own (patient, occurred_at DESC) index —
// never a full record read of the 50,000 rows themselves.
func BenchmarkPatientChartSummaryWith50kMedications(b *testing.B) {
	app, err := tests.NewTestApp(b.TempDir())
	require.NoError(b, err)
	b.Cleanup(app.Cleanup)

	owner := "mkbenchowner001"
	seedBenchAccount(b, app, owner)
	patientID := seedBenchPatient(b, app, owner)

	seedBenchMedications(b, app, patientID, 50_000)

	trail, err := auditstore.New(app)
	require.NoError(b, err)

	ctx := context.Background()

	durations := make([]time.Duration, 0, 20)

	for range 20 {
		started := time.Now()

		count, countErr := store.CountByPatient(ctx, app, kind.Medication.Collection(), patientID)
		require.NoError(b, countErr)
		require.Equal(b, 50_000, count)

		_, recentErr := trail.RecentForPatient(ctx, patientID, 10)
		require.NoError(b, recentErr)

		durations = append(durations, time.Since(started))
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	p95 := durations[int(float64(len(durations))*0.95)-1]

	b.Logf("p95 over %d runs: %s", len(durations), p95)
	require.Lessf(b, p95, 2*time.Second, "SC-004 requires the chart summary's own queries to answer within 2s at p95")
}

func seedBenchAccount(b *testing.B, app core.App, id string) {
	b.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(b, err)

	record := core.NewRecord(users)
	record.Id = id
	record.SetEmail(id + "@example.test")
	record.SetPassword("correct-horse-battery-staple")
	record.Set("name", "Bench Owner")
	record.Set("role", "user")
	record.Set("unit_system", "metric")
	record.Set("locale", "en")
	record.Set("date_format", "iso")
	record.Set("theme", "system")

	require.NoError(b, app.Save(record))
}

func seedBenchPatient(b *testing.B, app core.App, ownerID string) string {
	b.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(b, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Bench")
	record.Set("last_name", "Patient")

	require.NoError(b, app.Save(record))

	return record.Id
}

func seedBenchMedications(b *testing.B, app core.App, patientID string, count int) {
	b.Helper()

	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(b, err)

	const batchSize = 1000

	for start := 0; start < count; start += batchSize {
		end := min(start+batchSize, count)

		require.NoError(b, app.RunInTransaction(func(txApp core.App) error {
			for range end - start {
				record := core.NewRecord(collection)
				if err := store.MedicationToRecord(record, clinical.Medication{
					PatientID: patientID, Name: "Bench Medication", Status: clinical.TherapyStatusActive,
				}); err != nil {
					return err
				}

				if err := txApp.Save(record); err != nil {
					return err
				}
			}

			return nil
		}))
	}
}
