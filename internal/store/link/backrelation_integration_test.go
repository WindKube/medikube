package link_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/store/link"

	_ "medikube/internal/store/migrations"
)

func newApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	return app
}

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

func seedOwner(t *testing.T, app core.App) string {
	t.Helper()

	return seedRecord(t, app, "users", map[string]any{
		"email": "linktest@example.test", "password": "correct-horse-battery-staple",
		"name": "Test", "role": "user", "unit_system": "metric", "locale": "en",
		"date_format": "iso", "theme": "system",
	})
}

func TestBackrelationsReadsConditionsBackwards(t *testing.T) {
	t.Parallel()

	app := newApp(t)
	owner := seedOwner(t, app)
	patient := seedRecord(t, app, "patients", map[string]any{"owner": owner, "first_name": "T", "last_name": "P"})
	condition := seedRecord(t, app, kind.Condition.Collection(), map[string]any{
		"patient": patient, "diagnosis": "Hypertension", "status": "active",
	})

	encounter := seedRecord(t, app, kind.Encounter.Collection(), map[string]any{
		"patient": patient, "reason": "check-up", "occurred_on": "2026-01-01", "condition": condition,
	})
	procedure := seedRecord(t, app, kind.Procedure.Collection(), map[string]any{
		"patient": patient, "name": "biopsy", "occurred_on": "2026-01-02", "status": "completed", "condition": condition,
	})
	treatment := seedRecord(t, app, kind.Treatment.Collection(), map[string]any{
		"patient": patient, "name": "ACE inhibitor course", "condition": condition,
	})

	backrelations, err := link.NewBackrelations(app)
	require.NoError(t, err)

	refs, err := backrelations.Conditions(t.Context(), condition)
	require.NoError(t, err)

	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, string(ref.Kind)+":"+ref.ID)
	}

	assert.ElementsMatch(t, []string{
		string(kind.Encounter) + ":" + encounter,
		string(kind.Procedure) + ":" + procedure,
		string(kind.Treatment) + ":" + treatment,
	}, ids)
}

func TestBackrelationsReadsMedicationsBackwards(t *testing.T) {
	t.Parallel()

	app := newApp(t)
	owner := seedOwner(t, app)
	patient := seedRecord(t, app, "patients", map[string]any{"owner": owner, "first_name": "T", "last_name": "P"})
	medication := seedRecord(t, app, kind.Medication.Collection(), map[string]any{
		"patient": patient, "name": "Warfarin", "status": "active",
	})

	allergy := seedRecord(t, app, kind.Allergy.Collection(), map[string]any{
		"patient": patient, "allergen": "Penicillin", "severity": "mild", "status": "active",
		kind.Medication.Collection(): []string{medication},
	})
	condition := seedRecord(t, app, kind.Condition.Collection(), map[string]any{
		"patient": patient, "diagnosis": "AFib", "status": "active",
		kind.Medication.Collection(): []string{medication},
	})
	symptomTreated := seedRecord(t, app, kind.Symptom.Collection(), map[string]any{
		"patient": patient, "name": "Bruising", "severity": "mild", "occurred_at": "2026-01-01 00:00:00.000Z",
		"treated_by_" + kind.Medication.Collection(): []string{medication},
	})
	symptomCaused := seedRecord(t, app, kind.Symptom.Collection(), map[string]any{
		"patient": patient, "name": "Nausea", "severity": "mild", "occurred_at": "2026-01-02 00:00:00.000Z",
		"caused_by_" + kind.Medication.Collection(): []string{medication},
	})

	backrelations, err := link.NewBackrelations(app)
	require.NoError(t, err)

	refs, err := backrelations.Medications(t.Context(), medication)
	require.NoError(t, err)

	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, string(ref.Kind)+":"+ref.ID)
	}

	assert.ElementsMatch(t, []string{
		string(kind.Allergy) + ":" + allergy,
		string(kind.Condition) + ":" + condition,
		string(kind.Symptom) + ":" + symptomTreated,
		string(kind.Symptom) + ":" + symptomCaused,
	}, ids)
}

func TestBackrelationsReadsTreatmentMedicationsBothWays(t *testing.T) {
	t.Parallel()

	app := newApp(t)
	owner := seedOwner(t, app)
	patient := seedRecord(t, app, "patients", map[string]any{"owner": owner, "first_name": "T", "last_name": "P"})
	medication := seedRecord(t, app, kind.Medication.Collection(), map[string]any{
		"patient": patient, "name": "Warfarin", "status": "active",
	})
	treatment := seedRecord(t, app, kind.Treatment.Collection(), map[string]any{
		"patient": patient, "name": "Anticoagulation",
	})
	seedRecord(t, app, "treatment_"+kind.Medication.Collection(), map[string]any{
		"treatment": treatment, "medication": medication, "dosage": "3mg",
	})

	backrelations, err := link.NewBackrelations(app)
	require.NoError(t, err)

	medications, err := backrelations.TreatmentMedicationMedications(t.Context(), treatment)
	require.NoError(t, err)
	require.Len(t, medications, 1)
	assert.Equal(t, kind.Medication, medications[0].Kind)
	assert.Equal(t, medication, medications[0].ID)

	treatments, err := backrelations.TreatmentMedicationTreatments(t.Context(), medication)
	require.NoError(t, err)
	require.Len(t, treatments, 1)
	assert.Equal(t, kind.Treatment, treatments[0].Kind)
	assert.Equal(t, treatment, treatments[0].ID)
}

func TestResolverFindsExistenceAndPatient(t *testing.T) {
	t.Parallel()

	app := newApp(t)
	owner := seedOwner(t, app)
	patient := seedRecord(t, app, "patients", map[string]any{"owner": owner, "first_name": "T", "last_name": "P"})
	medication := seedRecord(t, app, kind.Medication.Collection(), map[string]any{
		"patient": patient, "name": "Warfarin", "status": "active",
	})

	resolver, err := link.NewResolver(app)
	require.NoError(t, err)

	refs, err := resolver.Resolve(t.Context(), kind.Medication, []string{medication, "missing"})
	require.NoError(t, err)
	require.Len(t, refs, 2)

	assert.True(t, refs[0].Found)
	assert.Equal(t, patient, refs[0].PatientID)
	assert.False(t, refs[1].Found)
}
