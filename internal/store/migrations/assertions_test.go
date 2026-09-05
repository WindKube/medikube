package migrations

import (
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/person"
)

// T075, T022. data-model §4 and phase 002's §7 relationship map, read off the
// migrated schema field by field. Both booleans are one character from
// silently wrong on each relation, neither flip produces a compile error, and
// the symptoms range from a medication that outlives the account that owned it
// (FR-014, SC-012) to a patient's deletion silently deleting the account that
// holds it (data-model §4).
func TestTheCascadeMatrixIsExactlyWhatDataModelDeclares(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	users, err := app.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)
	facilities, err := app.FindCollectionByNameOrId(facilitiesCollection)
	require.NoError(t, err)
	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	require.NoError(t, err)
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	require.NoError(t, err)
	conditions, err := app.FindCollectionByNameOrId(kind.Condition.Collection())
	require.NoError(t, err)

	cases := []struct {
		relation    relationRule
		target      string
		consequence string
	}{
		{
			relation:    relationRule{collection: kind.Medication.Collection(), field: medicationFieldPatient, required: true, cascadeDelete: true},
			target:      patients.Id,
			consequence: "deleting a patient must delete every medication row filed against them, and a row without a patient is unreachable (research D-13)",
		},
		{
			relation:    relationRule{collection: kind.Medication.Collection(), field: medicationFieldPractitioner, required: false, cascadeDelete: false},
			target:      practitioners.Id,
			consequence: "deleting a practitioner must unset a medication's prescriber reference, not delete the medication",
		},
		{
			relation:    relationRule{collection: kind.Medication.Collection(), field: medicationFieldPharmacy, required: false, cascadeDelete: false},
			target:      facilities.Id,
			consequence: "deleting a facility must unset a medication's pharmacy reference, not delete the medication",
		},
		{
			relation:    relationRule{collection: auditEventsCollection, field: auditFieldActor, required: false, cascadeDelete: false},
			target:      users.Id,
			consequence: "deleting an account must unset the reference and keep the row, so the account_delete entry survives",
		},
		{
			relation:    relationRule{collection: facilitiesCollection, field: facilityFieldOwner, required: true, cascadeDelete: true},
			target:      users.Id,
			consequence: "FR-037: closing the account destroys its directory",
		},
		{
			relation:    relationRule{collection: practitionersCollection, field: practitionerFieldOwner, required: true, cascadeDelete: true},
			target:      users.Id,
			consequence: "FR-037: closing the account destroys its directory",
		},
		{
			relation:    relationRule{collection: practitionersCollection, field: practitionerFieldFacility, required: false, cascadeDelete: false},
			target:      facilities.Id,
			consequence: "deleting a facility must unset a practitioner's reference to it, not delete the practitioner",
		},
		{
			relation:    relationRule{collection: patientsCollection, field: patientFieldOwner, required: true, cascadeDelete: true},
			target:      users.Id,
			consequence: "FR-002: closing the account destroys the people it kept records for",
		},
		{
			relation:    relationRule{collection: patientsCollection, field: patientFieldPrimaryPractitioner, required: false, cascadeDelete: false},
			target:      practitioners.Id,
			consequence: "deleting a practitioner must unset a patient's primary-practitioner reference, not delete the patient",
		},
		{
			relation:    relationRule{collection: usersCollection, field: usersFieldActivePatient, required: false, cascadeDelete: false},
			target:      patients.Id,
			consequence: "a patient's deletion must not delete the account whose active_patient pointed at it",
		},
		{
			relation:    relationRule{collection: auditEventsCollection, field: auditFieldPatient, required: false, cascadeDelete: false},
			target:      patients.Id,
			consequence: "a patient's deletion must unset the reference and keep the historical row",
		},
		{
			relation:    relationRule{collection: kind.Immunization.Collection(), field: immunizationFieldPatient, required: true, cascadeDelete: true},
			target:      patients.Id,
			consequence: "deleting a patient must delete every immunization row filed against them (FR-087)",
		},
		{
			relation:    relationRule{collection: kind.Immunization.Collection(), field: immunizationFieldPractitioner, required: false, cascadeDelete: false},
			target:      practitioners.Id,
			consequence: "deleting a practitioner must unset an immunization's own reference, not delete the immunization",
		},
		{
			relation:    relationRule{collection: kind.Immunization.Collection(), field: immunizationFieldFacility, required: false, cascadeDelete: false},
			target:      facilities.Id,
			consequence: "deleting a facility must unset an immunization's own reference, not delete the immunization",
		},
		{
			relation:    relationRule{collection: kind.Injury.Collection(), field: injuryFieldPatient, required: true, cascadeDelete: true},
			target:      patients.Id,
			consequence: "deleting a patient must delete every injury row filed against them (FR-087)",
		},
		{
			relation:    relationRule{collection: kind.Injury.Collection(), field: injuryFieldPractitioner, required: false, cascadeDelete: false},
			target:      practitioners.Id,
			consequence: "deleting a practitioner must unset an injury's own reference, not delete the injury",
		},
		{
			relation:    relationRule{collection: kind.Equipment.Collection(), field: equipmentFieldPatient, required: true, cascadeDelete: true},
			target:      patients.Id,
			consequence: "deleting a patient must delete every equipment row filed against them",
		},
		{
			relation:    relationRule{collection: kind.Equipment.Collection(), field: equipmentFieldSupplier, required: false, cascadeDelete: false},
			target:      facilities.Id,
			consequence: "deleting a facility must unset equipment's supplier reference, not delete the row",
		},
		{
			relation:    relationRule{collection: kind.Equipment.Collection(), field: equipmentFieldPractitioner, required: false, cascadeDelete: false},
			target:      practitioners.Id,
			consequence: "deleting a practitioner must unset equipment's prescriber reference, not delete the row",
		},
		{
			relation:    relationRule{collection: kind.Insurance.Collection(), field: insuranceFieldPatient, required: true, cascadeDelete: true},
			target:      patients.Id,
			consequence: "deleting a patient must delete every insurance policy filed against them",
		},
		{
			relation:    relationRule{collection: kind.Symptom.Collection(), field: symptomFieldPatient, required: true, cascadeDelete: true},
			target:      patients.Id,
			consequence: "deleting a patient must delete every symptom episode filed against them, and a row without a patient is unreachable",
		},
		{
			relation:    relationRule{collection: kind.Vitals.Collection(), field: vitalsFieldPatient, required: true, cascadeDelete: true},
			target:      patients.Id,
			consequence: "deleting a patient must delete every measurement set filed against them, and a row without a patient is unreachable",
		},
		{
			relation:    relationRule{collection: kind.Vitals.Collection(), field: vitalsFieldPractitioner, required: false, cascadeDelete: false},
			target:      practitioners.Id,
			consequence: "deleting a practitioner must unset a measurement set's reference to it, not delete the measurement set",
		},
		{
			relation:    relationRule{collection: kind.Encounter.Collection(), field: careFieldCondition, required: false, cascadeDelete: false},
			target:      conditions.Id,
			consequence: "deleting a condition must unset an encounter's reference to it, not delete the encounter",
		},
		{
			relation:    relationRule{collection: kind.Procedure.Collection(), field: careFieldCondition, required: false, cascadeDelete: false},
			target:      conditions.Id,
			consequence: "deleting a condition must unset a procedure's reference to it, not delete the procedure",
		},
		{
			relation:    relationRule{collection: kind.Treatment.Collection(), field: careFieldCondition, required: false, cascadeDelete: false},
			target:      conditions.Id,
			consequence: "deleting a condition must unset a treatment's reference to it, not delete the treatment",
		},
	}

	declared := make([]relationRule, 0, len(cases))
	for _, testCase := range cases {
		declared = append(declared, testCase.relation)
	}
	require.ElementsMatch(t, declared, Relations(),
		"the matrix the boot assertion checks is not the matrix this test declares")

	for _, testCase := range cases {
		t.Run(testCase.relation.collection+"."+testCase.relation.field, func(t *testing.T) {
			t.Parallel()

			collection, findErr := app.FindCollectionByNameOrId(testCase.relation.collection)
			require.NoError(t, findErr)

			relation, relErr := relationField(collection, testCase.relation.field)
			require.NoError(t, relErr)

			assert.Equal(t, testCase.relation.required, relation.Required, testCase.consequence)
			assert.Equal(t, testCase.relation.cascadeDelete, relation.CascadeDelete, testCase.consequence)
			assert.Equal(t, 1, relation.MaxSelect)
			assert.Equal(t, testCase.target, relation.CollectionId)
		})
	}

	assert.NoError(t, AssertRelations(app))
}

// The behaviour the two booleans buy, asserted rather than assumed: FR-014 and
// SC-012 are satisfied by PocketBase's deleteRefRecords rather than by MediKube
// code, which is exactly why they need a test that deletes a real account.
func TestDeletingAnAccountDeletesItsMedicationsAndOutlivesItsAuditTrail(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	user := newUser(t, app, "amara@example.test")

	patient := newRecord(t, app, patientsCollection, map[string]any{
		patientFieldOwner:               user.Id,
		patientFieldIsSelfRecord:        true,
		patientFieldRelationshipToOwner: string(person.RelationshipSelf),
	})

	medication := newRecord(t, app, kind.Medication.Collection(), map[string]any{
		medicationFieldPatient: patient.Id,
		medicationFieldName:    "Amoxicillin",
		medicationFieldStatus:  string(clinical.TherapyStatusActive),
	})

	event := newRecord(t, app, auditEventsCollection, map[string]any{
		auditFieldOccurredAt: types.NowDateTime(),
		auditFieldActor:      user.Id,
		auditFieldActorKind:  string(audit.ActorKindUser),
		auditFieldAction:     string(audit.ActionAccountDelete),
		auditFieldTargetKind: string(audit.TargetKindUser),
		auditFieldTargetID:   user.Id,
		auditFieldRequestID:  "req_0123456789",
	})

	require.NoError(t, app.Delete(user))

	orphans, err := app.CountRecords(kind.Medication.Collection(), dbx.HashExp{medicationFieldPatient: patient.Id})
	require.NoError(t, err)
	assert.EqualValues(t, 0, orphans, "SC-012: a deleted account leaves no medication behind")

	_, err = app.FindRecordById(kind.Medication.Collection(), medication.Id)
	assert.Error(t, err, "the medication should be gone, not merely unreachable")

	survivor, err := app.FindRecordById(auditEventsCollection, event.Id)
	require.NoError(t, err, "FR-037: the row recording the deletion must outlive the account")

	// A date column is TEXT DEFAULT '' NOT NULL, and so is a single relation:
	// "unset" is the empty string throughout, never SQL NULL.
	assert.Empty(t, survivor.GetString(auditFieldActor))
	assert.Equal(t, string(audit.ActorKindUser), survivor.GetString(auditFieldActorKind),
		"actor_kind is what still says a person did it once the reference is gone")
	assert.Equal(t, user.Id, survivor.GetString(auditFieldTargetID))
}

// Assertion 1 of data-model §5. nil is superuser-only; types.Pointer("") is no
// constraint at all. Nothing at save time tells them apart, so this is the only
// control there is.
func TestAssertAPIRulesRefusesANonNilRuleOnANonSystemCollection(t *testing.T) {
	t.Parallel()

	rules := []string{"listRule", "viewRule", "createRule", "updateRule", "deleteRule"}

	for _, rule := range rules {
		t.Run(rule, func(t *testing.T) {
			t.Parallel()

			app := newTestApp(t)
			require.NoError(t, AssertAPIRules(app), "the migrated schema must start clean")

			collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
			require.NoError(t, err)

			open := types.Pointer("")
			switch rule {
			case "listRule":
				collection.ListRule = open
			case "viewRule":
				collection.ViewRule = open
			case "createRule":
				collection.CreateRule = open
			case "updateRule":
				collection.UpdateRule = open
			case "deleteRule":
				collection.DeleteRule = open
			}
			require.NoError(t, app.Save(collection))

			err = AssertAPIRules(app)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrAPIRuleNotNil)
			assert.Contains(t, err.Error(), rule)
			assert.Contains(t, err.Error(), kind.Medication.Collection())

			assert.ErrorIs(t, AssertFatal(app), ErrAPIRuleNotNil)
		})
	}
}

// PocketBase's own _mfas, _otps, _externalAuths and _authOrigins ship non-nil
// list and view rules. They are System and rewriting them would be rewriting
// PocketBase, which is why the assertion's qualifier is load-bearing rather
// than a convenience.
func TestAssertAPIRulesIgnoresPocketBasesOwnSystemCollections(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collections, err := app.FindAllCollections()
	require.NoError(t, err)

	var withRules int
	for _, collection := range collections {
		if collection.System && collection.ListRule != nil {
			withRules++
		}
	}

	require.Positive(t, withRules,
		"no system collection carries a rule, so the exemption is untested and could be removed unnoticed")
	assert.NoError(t, AssertAPIRules(app))
}

// Assertion 2. No file field ships in this phase, so the gate is exercised on a
// synthetic one — an assertion that has never fired is an assertion nobody has
// tested, and phase 002's patients.photo has to land into a gate that works.
func TestAssertProtectedFilesRefusesAnUnprotectedFileField(t *testing.T) {
	t.Parallel()

	t.Run("synthetic collection", func(t *testing.T) {
		t.Parallel()

		collection := core.NewBaseCollection("patients")
		collection.Fields.Add(&core.FileField{Name: "photo", MaxSelect: 1})

		offences := unprotectedFileFields(collection)
		require.Len(t, offences, 1)
		assert.ErrorIs(t, offences[0], ErrFileFieldUnprotected)
		assert.Contains(t, offences[0].Error(), "patients.photo")

		collection.Fields.Add(&core.FileField{Name: "photo", MaxSelect: 1, Protected: true})
		assert.Empty(t, unprotectedFileFields(collection))
	})

	t.Run("saved into the schema", func(t *testing.T) {
		t.Parallel()

		app := newTestApp(t)
		require.NoError(t, AssertProtectedFiles(app), "this phase ships zero file fields")

		collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
		require.NoError(t, err)
		collection.Fields.Add(&core.FileField{Name: "leaflet", MaxSelect: 1})
		require.NoError(t, app.Save(collection))

		err = AssertProtectedFiles(app)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileFieldUnprotected)
		assert.ErrorIs(t, AssertFatal(app), ErrFileFieldUnprotected)
	})
}

// Assertion 3, from the other side: the matrix has to reject a flip, not merely
// agree with the schema it was written against.
func TestAssertRelationsRefusesAFlippedBoolean(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		flip func(relation *core.RelationField)
	}{
		{name: "cascade off", flip: func(r *core.RelationField) { r.CascadeDelete = false }},
		{name: "required off", flip: func(r *core.RelationField) { r.Required = false }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp(t)
			require.NoError(t, AssertRelations(app))

			collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
			require.NoError(t, err)

			relation, err := relationField(collection, medicationFieldPatient)
			require.NoError(t, err)
			testCase.flip(relation)
			require.NoError(t, app.Save(collection))

			err = AssertRelations(app)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrRelationMismatch)
			assert.Contains(t, err.Error(), medicationFieldPatient)
		})
	}
}

// Assertion 4. Batch is a second door into the record CRUD handlers the
// lockdown closes, and PocketBase reads Logs.MaxDays zero as "keep forever" —
// so the value that looks like "keep nothing" is the one that keeps everything
// (research D-29).
func TestAssertSettingsRefusesBatchAndAZeroLogRetention(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		apply func(settings *core.Settings)
	}{
		{name: "batch enabled", apply: func(s *core.Settings) { s.Batch.Enabled = true }},
		{name: "logs kept forever", apply: func(s *core.Settings) { s.Logs.MaxDays = 0 }},
		{name: "logs kept too long", apply: func(s *core.Settings) { s.Logs.MaxDays = 7 }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp(t)

			// The harness forces Logs.MaxDays to zero for its own reasons, which
			// is precisely the value this refuses — so the baseline has to be
			// written before it can be broken.
			app.Settings().Batch.Enabled = false
			app.Settings().Logs.MaxDays = LogsMaxDays
			require.NoError(t, AssertSettings(app))

			testCase.apply(app.Settings())

			err := AssertSettings(app)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrSettingsMismatch)
			assert.ErrorIs(t, AssertStrict(app), ErrSettingsMismatch)
		})
	}
}

// The columns and the entity agree on every published limit. The migration
// writes the numbers of data-model §2 and clinical.Medication.Validate enforces
// them independently, so nothing but a test connects the two — and a column
// narrower than the entity refuses, at the storage layer, a value the form
// already told the person was acceptable.
func TestEveryMedicationTextColumnIsSizedAsTheDomainValidates(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	cases := []struct {
		column string
		set    func(medication *clinical.Medication, value string)
	}{
		{medicationFieldName, func(m *clinical.Medication, v string) { m.Name = v }},
		{medicationFieldAlternativeName, func(m *clinical.Medication, v string) { m.AlternativeName = v }},
		{medicationFieldDosage, func(m *clinical.Medication, v string) { m.Dosage = v }},
		{medicationFieldFrequency, func(m *clinical.Medication, v string) { m.Frequency = v }},
		{medicationFieldIndication, func(m *clinical.Medication, v string) { m.Indication = v }},
		{medicationFieldSideEffects, func(m *clinical.Medication, v string) { m.SideEffects = v }},
		{medicationFieldNotes, func(m *clinical.Medication, v string) { m.Notes = v }},
	}

	for _, testCase := range cases {
		t.Run(testCase.column, func(t *testing.T) {
			t.Parallel()

			field := collection.Fields.GetByName(testCase.column)
			require.NotNil(t, field)

			text, isText := field.(*core.TextField)
			require.Truef(t, isText, "%s is a %s field, not text", testCase.column, field.Type())
			require.Positive(t, text.Max, "an unbounded text column silently becomes PocketBase's own 5000")

			atLimit := clinical.Medication{Name: "a", Status: clinical.TherapyStatusActive}
			testCase.set(&atLimit, strings.Repeat("a", text.Max))
			assert.NoError(t, atLimit.Validate(),
				"the column accepts %d characters but the entity refuses them", text.Max)

			over := clinical.Medication{Name: "a", Status: clinical.TherapyStatusActive}
			testCase.set(&over, strings.Repeat("a", text.Max+1))

			var invalid *domain.ValidationError
			require.ErrorAs(t, over.Validate(), &invalid,
				"the entity accepts %d characters but the column refuses them", text.Max+1)

			var named bool
			for _, offence := range invalid.Fields {
				if offence.Field == testCase.column && offence.Code == domain.CodeTooLong {
					named = true
				}
			}
			assert.True(t, named, "the refusal must name the column and its limit")
		})
	}
}

func newUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.SetEmail(email)
	record.SetPassword("a-long-enough-password")
	record.Set(usersFieldName, "Amara Okafor")
	record.Set(usersFieldRole, string(identity.DefaultRole))
	record.Set(usersFieldUnitSystem, string(identity.DefaultUnitSystem))
	record.Set(usersFieldLocale, identity.DefaultLocale)
	record.Set(usersFieldDateFormat, string(identity.DefaultDateFormat))
	record.Set(usersFieldTheme, string(identity.DefaultTheme))

	require.NoError(t, app.Save(record))

	return record
}

// T021. Every one of the five API rules is nil on all three collections this
// phase adds — the lockdown at the schema layer, and the only thing a
// PocketBase upgrade that added a default rule to NewBaseCollection could not
// silently undo.
func TestTheThreeNewCollectionsHaveEveryAPIRuleNil(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	for _, name := range []string{facilitiesCollection, practitionersCollection, patientsCollection} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			collection, err := app.FindCollectionByNameOrId(name)
			require.NoError(t, err)

			assert.Nil(t, collection.ListRule)
			assert.Nil(t, collection.ViewRule)
			assert.Nil(t, collection.CreateRule)
			assert.Nil(t, collection.UpdateRule)
			assert.Nil(t, collection.DeleteRule)
		})
	}

	assert.NoError(t, AssertAPIRules(app))
}

// T023. patients.photo (FR-008, FR-009, FR-044): Protected so no PocketBase
// file token and no link carrying its own credential can reach it, sized to
// the configured 15 MiB and offering exactly the two thumbnails data-model §3
// names.
func TestPatientsPhotoIsProtectedAndSizedToTheConfiguredLimit(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(patientsCollection)
	require.NoError(t, err)

	field := collection.Fields.GetByName(patientFieldPhoto)
	require.NotNil(t, field)

	file, isFile := field.(*core.FileField)
	require.Truef(t, isFile, "%s is a %s field, not a file", patientFieldPhoto, field.Type())

	assert.True(t, file.Protected, "FR-044: no file token and no link may reach a patient's photograph")
	assert.EqualValues(t, PhotoMaxBytes, file.MaxSize)
	assert.ElementsMatch(t, []string{"image/jpeg", "image/png", "image/webp"}, file.MimeTypes)
	assert.ElementsMatch(t, []string{"100x100t", "400x400f"}, file.Thumbs)
	assert.Equal(t, 1, file.MaxSelect)

	assert.NoError(t, AssertProtectedFiles(app))
}

// T023's other half: the boot assertion refuses to start when Protected is
// flipped false, exercised on the real migrated schema rather than on the
// synthetic collection TestAssertProtectedFilesRefusesAnUnprotectedFileField
// already covers.
func TestBootRefusesAnUnprotectedPatientPhoto(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(patientsCollection)
	require.NoError(t, err)

	file, isFile := collection.Fields.GetByName(patientFieldPhoto).(*core.FileField)
	require.True(t, isFile)
	file.Protected = false
	require.NoError(t, app.Save(collection))

	err = AssertProtectedFiles(app)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFileFieldUnprotected)
	assert.Contains(t, err.Error(), patientsCollection+"."+patientFieldPhoto)
	assert.ErrorIs(t, AssertFatal(app), ErrFileFieldUnprotected)
}

func newRecord(t *testing.T, app core.App, collectionName string, values map[string]any) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(collectionName)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	for field, value := range values {
		record.Set(field, value)
	}

	require.NoError(t, app.Save(record))

	return record
}
