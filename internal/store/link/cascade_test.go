package link_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/store/link"
)

// findLinkRows is this file's own store.Query lookup, standing in for the
// FindRecordsByFilter call PocketBase's filter DSL would use — this package
// builds a bound query instead (research D-26, plan.md internal/store).
func findLinkRows(t *testing.T, app *tests.TestApp, field, value string) []*core.Record {
	t.Helper()

	collection := "treatment_" + kind.Medication.Collection()
	schema := store.NewSchema(collection, store.Column{Name: field, FilterOnly: true})

	built, err := schema.Build(store.Query{Conditions: []store.Condition{store.Equal(field, value)}})
	require.NoError(t, err)

	var rows []*core.Record
	require.NoError(t, built.Apply(app.RecordQuery(collection)).All(&rows))

	return rows
}

// T137, FR-058, US6-4, SC-006: deleting a linked record leaves the other
// intact, removes the link from both sides, and leaves no dangling reference.
func TestDeletingAMedicationRemovesTheLinkButNotTheReferencingRecord(t *testing.T) {
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

	medicationRecord, err := app.FindRecordById(kind.Medication.Collection(), medication)
	require.NoError(t, err)
	require.NoError(t, app.Delete(medicationRecord))

	allergyRecord, err := app.FindRecordById(kind.Allergy.Collection(), allergy)
	require.NoError(t, err, "the allergy must survive the medication's deletion")
	assert.Empty(t, allergyRecord.GetStringSlice(kind.Medication.Collection()), "the dangling reference must be removed")

	backrelations, err := link.NewBackrelations(app)
	require.NoError(t, err)

	refs, err := backrelations.Medications(t.Context(), medication)
	require.NoError(t, err)
	assert.Empty(t, refs, "a deleted medication has no back-relations left")
}

func TestDeletingATreatmentRemovesTheCourseMedicationLinkButNotTheMedication(t *testing.T) {
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

	treatmentRecord, err := app.FindRecordById(kind.Treatment.Collection(), treatment)
	require.NoError(t, err)
	require.NoError(t, app.Delete(treatmentRecord))

	medicationRecord, err := app.FindRecordById(kind.Medication.Collection(), medication)
	require.NoError(t, err, "the medication must survive the treatment's deletion")
	_ = medicationRecord

	rows := findLinkRows(t, app, "treatment", treatment)
	assert.Empty(t, rows, "the join row is cascade-deleted with the treatment")
}

func TestDeletingAMedicationRemovesItsCourseMedicationLinkButNotTheTreatment(t *testing.T) {
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

	medicationRecord, err := app.FindRecordById(kind.Medication.Collection(), medication)
	require.NoError(t, err)
	require.NoError(t, app.Delete(medicationRecord))

	treatmentRecord, err := app.FindRecordById(kind.Treatment.Collection(), treatment)
	require.NoError(t, err, "the treatment must survive the medication's deletion")
	_ = treatmentRecord

	rows := findLinkRows(t, app, "medication", medication)
	assert.Empty(t, rows, "the join row is cascade-deleted with the medication")
}
