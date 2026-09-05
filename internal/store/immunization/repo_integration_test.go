package immunization_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/immunization"
	"medikube/internal/service/immunization/immunizationtest"
	"medikube/internal/store"
	pbimmunization "medikube/internal/store/immunization"

	_ "medikube/internal/store/migrations"
)

const (
	ownerEmail    = "owner@example.test"
	strangerEmail = "stranger@example.test"
)

type harness struct {
	app             *tests.TestApp
	repo            *pbimmunization.Repo
	patient         string
	strangerPatient string
}

func newHarness(t *testing.T) harness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbimmunization.New(app, codec)
	require.NoError(t, err)

	owner := seedAccount(t, app, ownerEmail)
	stranger := seedAccount(t, app, strangerEmail)

	return harness{
		app:             app,
		repo:            repo,
		patient:         seedPatient(t, app, owner),
		strangerPatient: seedPatient(t, app, stranger),
	}
}

func seedAccount(t *testing.T, app core.App, email string) string {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword("correct-horse-battery-staple")
	record.Set("name", "Test Person")
	record.Set("role", "user")
	record.Set("unit_system", "metric")
	record.Set("locale", "en")
	record.Set("date_format", "iso")
	record.Set("theme", "system")

	require.NoError(t, app.Save(record))

	return record.Id
}

func seedPatient(t *testing.T, app core.App, ownerID string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Test")
	record.Set("last_name", "Patient")

	require.NoError(t, app.Save(record))

	return record.Id
}

// TestThePocketBaseRepositoryPassesTheSameContractTheFakeDoes is T105/T140's
// counterpart for immunizations: the same suite the in-memory fake passes
// (Principle II).
func TestThePocketBaseRepositoryPassesTheSameContractTheFakeDoes(t *testing.T) {
	t.Parallel()

	immunizationtest.RunRepositoryContract(t, func(t *testing.T) (immunization.Repository, immunizationtest.Accounts) {
		t.Helper()

		h := newHarness(t)

		return h.repo, immunizationtest.Accounts{Patient: h.patient, StrangerPatient: h.strangerPatient}
	})
}

// TestDeletingThePatientCascadesToItsImmunizations is FR-087/SC-005: the
// patient relation is CascadeDelete, so deleting the patient must leave no
// orphan row behind.
func TestDeletingThePatientCascadesToItsImmunizations(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	administeredOn, err := domain.NewDate(2026, 1, 1)
	require.NoError(t, err)

	stored, err := h.repo.Create(ctx, clinical.Immunization{
		PatientID: h.patient, VaccineName: "Influenza", AdministeredOn: administeredOn,
	})
	require.NoError(t, err)

	patient, err := h.app.FindRecordById("patients", h.patient)
	require.NoError(t, err)
	require.NoError(t, h.app.Delete(patient))

	_, err = h.repo.Get(ctx, stored.ID)
	require.Error(t, err)
}
