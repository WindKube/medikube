package migrations

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// data-model §2's thirteen columns, in the order that document lists them,
// plus the three §1.0 puts on every collection. Asserted as a whole list: the
// failure worth catching is a column added or dropped by a later phase without
// the data model saying so.
func TestMedicationsCarriesExactlyTheColumnsDataModelDeclares(t *testing.T) {
	t.Parallel()

	collection := medicationsSchema(t, newTestApp(t))

	// owner sat second (research D-13's medications-repoint migration drops
	// it); patient, practitioner and pharmacy are appended at the end, in the
	// order that migration adds them.
	expected := []string{
		core.FieldNameId,
		medicationFieldName,
		medicationFieldAlternativeName,
		medicationFieldType,
		medicationFieldDosage,
		medicationFieldFrequency,
		medicationFieldRoute,
		medicationFieldIndication,
		medicationFieldStartedOn,
		medicationFieldEndedOn,
		medicationFieldStatus,
		medicationFieldSideEffects,
		medicationFieldNotes,
		fieldCreated,
		fieldUpdated,
		medicationFieldPatient,
		medicationFieldPractitioner,
		medicationFieldPharmacy,
	}

	assert.Equal(t, expected, collection.Fields.FieldNames())

	// NewBaseCollection supplies id and nothing else — created and updated are
	// not automatic, and idx_medications_owner_upd indexes a column that would
	// not otherwise exist.
	for _, column := range []string{fieldCreated, fieldUpdated} {
		autodate, isAutodate := collection.Fields.GetByName(column).(*core.AutodateField)
		require.Truef(t, isAutodate, "%s is not an autodate column", column)
		assert.True(t, autodate.OnCreate, column)
	}

	updated, isAutodate := collection.Fields.GetByName(fieldUpdated).(*core.AutodateField)
	require.True(t, isAutodate)
	assert.True(t, updated.OnUpdate, "the ETag is derived from updated (research D-24)")

	// Deliberately absent, each with the phase that adds it. A name appearing
	// here early is a phase reaching backwards rather than a typo. owner is
	// deliberately absent too, in the other direction: research D-13's
	// medications-repoint migration removes it.
	for _, absent := range []string{medicationFieldOwner, "tags", "deleted_at", "reminder_enabled"} {
		assert.Nilf(t, collection.Fields.GetByName(absent), "%s belongs to a different phase", absent)
	}
}

// FR-016: a value outside a published set is refused rather than stored as free
// text, at the storage layer as well as in the domain. A select field carrying
// the wrong vocabulary and one carrying none look the same from a distance.
func TestMedicationEnumColumnsCarryTheDomainVocabularies(t *testing.T) {
	t.Parallel()

	collection := medicationsSchema(t, newTestApp(t))

	cases := []struct {
		column   string
		values   []string
		required bool
		count    int
	}{
		{column: medicationFieldType, values: enumValues(clinical.MedicationTypes()), count: 4},
		{column: medicationFieldRoute, values: enumValues(clinical.MedicationRoutes()), count: 14},
		{column: medicationFieldStatus, values: enumValues(clinical.TherapyStatuses()), required: true, count: 5},
	}

	for _, testCase := range cases {
		t.Run(testCase.column, func(t *testing.T) {
			t.Parallel()

			field := collection.Fields.GetByName(testCase.column)
			require.NotNil(t, field)

			selectField, isSelect := field.(*core.SelectField)
			require.Truef(t, isSelect, "%s is a %s field, not a select", testCase.column, field.Type())

			require.Len(t, testCase.values, testCase.count, "the domain vocabulary changed size")
			assert.ElementsMatch(t, testCase.values, selectField.Values)
			assert.Equal(t, 1, selectField.MaxSelect)
			assert.Equal(t, testCase.required, selectField.Required)
		})
	}
}

// The four indexes of data-model §8, as research D-13's medications-repoint
// migration leaves them. The one that backs FR-022's default ordering ends in
// id, because the keyset cursor's tiebreaker is always the id (research
// D-25) — without it, two medications started on the same day page unstably,
// which is what FR-023 forbids.
func TestMedicationIndexesEndInTheCursorTiebreaker(t *testing.T) {
	t.Parallel()

	collection := medicationsSchema(t, newTestApp(t))

	name := kind.Medication.Collection()

	cases := []struct {
		index     string
		columns   string
		tiebroken bool
	}{
		{index: "idx_" + name + "_patient", columns: "(patient)"},
		{index: "idx_" + name + "_patient_start", columns: "(patient, started_on DESC, id DESC)", tiebroken: true},
		{index: "idx_" + name + "_practitioner", columns: "(practitioner)"},
		{index: "idx_" + name + "_pharmacy", columns: "(pharmacy)"},
	}

	require.Len(t, collection.Indexes, len(cases))

	for _, testCase := range cases {
		t.Run(testCase.index, func(t *testing.T) {
			t.Parallel()

			declared := indexNamed(t, collection, testCase.index)

			assert.Contains(t, declared, testCase.columns)

			if testCase.tiebroken {
				assert.True(t, strings.HasSuffix(declared, "id DESC)"),
					"an ordering index that does not end in id pages unstably (FR-023): %s", declared)
			}
		})
	}
}

// indexNamed returns the collection's own CREATE INDEX statement, so an
// assertion is made against what the schema declares rather than against the
// string the test was about to compare it with.
func indexNamed(t *testing.T, collection *core.Collection, name string) string {
	t.Helper()

	for _, index := range collection.Indexes {
		if strings.Contains(index, "`"+name+"`") {
			return index
		}
	}

	require.Failf(t, "missing index", "%s declares no index called %s", collection.Name, name)

	return ""
}

func medicationsSchema(t *testing.T, app core.App) *core.Collection {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	return collection
}
