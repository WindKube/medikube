package coursemedication_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/store/coursemedication"

	_ "medikube/internal/store/migrations"
)

func seedRecord(t *testing.T, app core.App, collection string, fields map[string]any) string {
	t.Helper()

	c, err := app.FindCollectionByNameOrId(collection)
	require.NoError(t, err)

	record := core.NewRecord(c)
	for field, value := range fields {
		record.Set(field, value)
	}

	require.NoError(t, app.Save(record))

	return record.Id
}

type harness struct {
	app        *tests.TestApp
	repo       *coursemedication.Repo
	patient    string
	treatment  string
	treatmentV string
	medication string
}

func newHarness(t *testing.T) harness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	repo, err := coursemedication.New(app)
	require.NoError(t, err)

	owner := seedRecord(t, app, "users", map[string]any{
		"email": "coursemed@example.test", "password": "correct-horse-battery-staple",
		"name": "Test", "role": "user", "unit_system": "metric", "locale": "en",
		"date_format": "iso", "theme": "system",
	})
	patient := seedRecord(t, app, "patients", map[string]any{"owner": owner, "first_name": "T", "last_name": "P"})
	treatment := seedRecord(t, app, kind.Treatment.Collection(), map[string]any{
		"patient": patient, "name": "Anticoagulation",
	})
	medication := seedRecord(t, app, kind.Medication.Collection(), map[string]any{
		"patient": patient, "name": "Warfarin", "status": "active",
	})

	treatmentRecord, err := app.FindRecordById(kind.Treatment.Collection(), treatment)
	require.NoError(t, err)

	return harness{
		app: app, repo: repo, patient: patient,
		treatment: treatment, treatmentV: store.Version(treatmentRecord), medication: medication,
	}
}

func TestUpsertTwiceUpdatesTheSameRow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	entity := clinical.CourseMedication{TreatmentID: h.treatment, MedicationID: h.medication, Dosage: "3mg"}

	stored, created, err := h.repo.Upsert(ctx, entity, h.treatmentV)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "3mg", stored.Dosage)

	entity.Dosage = "5mg"
	stored2, created2, err := h.repo.Upsert(ctx, entity, h.treatmentV)
	require.NoError(t, err)
	assert.False(t, created2)
	assert.Equal(t, stored.ID, stored2.ID)
	assert.Equal(t, "5mg", stored2.Dosage)

	rows, err := h.repo.List(ctx, h.treatment)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}

func TestUpsertRefusesAStaleTreatmentVersion(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	entity := clinical.CourseMedication{TreatmentID: h.treatment, MedicationID: h.medication}

	_, _, err := h.repo.Upsert(ctx, entity, "stale-version")
	require.ErrorIs(t, err, domain.ErrVersionMismatch)
}

func TestDeleteRemovesOnlyTheLinkRow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	entity := clinical.CourseMedication{TreatmentID: h.treatment, MedicationID: h.medication}
	_, _, err := h.repo.Upsert(ctx, entity, h.treatmentV)
	require.NoError(t, err)

	require.NoError(t, h.repo.Delete(ctx, h.treatment, h.medication, h.treatmentV))

	rows, err := h.repo.List(ctx, h.treatment)
	require.NoError(t, err)
	assert.Empty(t, rows)

	_, err = h.app.FindRecordById(kind.Treatment.Collection(), h.treatment)
	assert.NoError(t, err, "the treatment survives")

	_, err = h.app.FindRecordById(kind.Medication.Collection(), h.medication)
	assert.NoError(t, err, "the medication survives")
}
