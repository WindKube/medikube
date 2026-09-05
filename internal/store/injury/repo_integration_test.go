package injury_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/injury"
	"medikube/internal/service/injury/injurytest"
	"medikube/internal/store"
	pbinjury "medikube/internal/store/injury"

	_ "medikube/internal/store/migrations"
)

const (
	ownerEmail    = "owner@example.test"
	strangerEmail = "stranger@example.test"
)

type harness struct {
	app             *tests.TestApp
	repo            *pbinjury.Repo
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

	repo, err := pbinjury.New(app, codec)
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

func TestThePocketBaseRepositoryPassesTheSameContractTheFakeDoes(t *testing.T) {
	t.Parallel()

	injurytest.RunRepositoryContract(t, func(t *testing.T) (injury.Repository, injurytest.Accounts) {
		t.Helper()

		h := newHarness(t)

		return h.repo, injurytest.Accounts{Patient: h.patient, StrangerPatient: h.strangerPatient}
	})
}

func TestDeletingThePatientCascadesToItsInjuries(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	stored, err := h.repo.Create(ctx, clinical.Injury{
		PatientID: h.patient, Name: "Sprained ankle", BodyPart: "ankle", Status: clinical.ConditionStatusActive,
	})
	require.NoError(t, err)

	patient, err := h.app.FindRecordById("patients", h.patient)
	require.NoError(t, err)
	require.NoError(t, h.app.Delete(patient))

	_, err = h.repo.Get(ctx, stored.ID)
	require.Error(t, err)
}

// TestMedicationLinksSurviveTheMedicationsDeletion is FR-058, SC-006: deleting
// a linked medication removes the injury's own reference to it and the injury
// itself survives.
func TestMedicationLinksSurviveTheMedicationsDeletion(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	medications, err := h.app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	medication := core.NewRecord(medications)
	medication.Set("patient", h.patient)
	medication.Set("name", "Ibuprofen")
	medication.Set("status", "active")
	require.NoError(t, h.app.Save(medication))

	stored, err := h.repo.Create(ctx, clinical.Injury{
		PatientID: h.patient, Name: "Sprained ankle", BodyPart: "ankle",
		Status: clinical.ConditionStatusActive, MedicationIDs: []string{medication.Id},
	})
	require.NoError(t, err)

	require.NoError(t, h.app.Delete(medication))

	read, err := h.repo.Get(ctx, stored.ID)
	require.NoError(t, err)
	require.NotContains(t, read.MedicationIDs, medication.Id)
}
