package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"

	// MediKube's migrations register themselves from their package init, and
	// core.AppMigrations is what tests.NewTestApp runs. Without this import the
	// whole suite would run against a schema with no medications collection and
	// every mapping assertion would be reading a column that does not exist.
	_ "medikube/internal/store/migrations"
)

// newTestApp builds one instance per test against an empty directory rather
// than PocketBase's own tests/data fixture, whose demo collections this package
// would then have to filter out of every query assertion.
func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	return app
}

// newBaseApp is the restart seam: a bare BaseApp over a directory the caller
// keeps, so the same instance can be bootstrapped twice and a second one can be
// opened over what the first left behind. tests.TestApp clones its data
// directory, so it cannot answer a question about persistence.
func newBaseApp(t *testing.T, dataDir string) *core.BaseApp {
	t.Helper()

	app := core.NewBaseApp(core.BaseAppConfig{DataDir: dataDir})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())

	return app
}

func tempDataDir(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "pb_data")
}

// seedUser creates one account with every column the profile migration made
// required. It exists because a medication without an owner cannot be saved and
// every query in this package is owner-scoped.
func seedUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()

	users, err := app.FindCollectionByNameOrId(authCollection)
	require.NoError(t, err)

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword("correct-horse-battery-staple")
	record.Set(userFieldName, "Test Person")
	record.Set(userFieldRole, "user")
	record.Set(userFieldUnitSystem, "metric")
	record.Set(userFieldLocale, "en")
	record.Set(userFieldDateFormat, "iso")
	record.Set(userFieldTheme, "system")

	require.NoError(t, app.Save(record))

	return record
}

// seedPatient creates one minimal patient owned by the given account. It
// exists because a medication is patient-owned (research D-13) and the
// `patient` relation refuses a row that does not exist, so every medication
// this package seeds needs one of these under it first.
func seedPatient(t *testing.T, app core.App, ownerID string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Test")
	record.Set("last_name", "Patient")

	require.NoError(t, app.Save(record))

	return record
}

// seedMedication writes one row through the mapper under test, so every query
// test is reading rows the mapper produced rather than rows hand-assembled to
// suit the assertion.
func seedMedication(t *testing.T, app core.App, med clinical.Medication) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	record := core.NewRecord(collection)
	require.NoError(t, MedicationToRecord(record, med))
	require.NoError(t, app.Save(record))

	return record
}

func mustDate(t *testing.T, text string) domain.Date {
	t.Helper()

	parsed, err := domain.ParseDate(text)
	require.NoError(t, err)

	return parsed
}

// sampleMedication is a complete entity: every column populated, so a mapper
// that dropped one is caught by the round trip rather than by a later phase.
func sampleMedication(t *testing.T, patientID string) clinical.Medication {
	t.Helper()

	return clinical.Medication{
		PatientID:       patientID,
		Name:            "Amoxicillin",
		AlternativeName: "Amoxil",
		Type:            clinical.MedicationTypePrescription,
		Dosage:          "500 mg",
		Frequency:       "three times a day",
		Route:           clinical.MedicationRouteOral,
		Indication:      "chest infection",
		StartedOn:       mustDate(t, "2026-03-01"),
		EndedOn:         mustDate(t, "2026-03-10"),
		Status:          clinical.TherapyStatusActive,
		SideEffects:     "mild nausea",
		Notes:           "take with food",
	}
}

// withinRecently is how an autodate column is asserted without pinning a clock:
// the value has to be an instant this test could plausibly have produced.
func withinRecently(t *testing.T, instant time.Time) {
	t.Helper()

	require.False(t, instant.IsZero(), "the instant was not read at all")
	require.WithinDuration(t, time.Now().UTC(), instant, time.Minute)
	require.Equal(t, time.UTC, instant.Location(), "an instant that reaches the domain is UTC or it is a bug (D-27)")
}

// dbxID is the one-row predicate the raw-SQL assertions use. It is spelled once
// here so a test that reaches past the mapper still reaches past it the same
// way each time.
func dbxID(id string) dbx.Expression {
	return dbx.HashExp{"id": id}
}
