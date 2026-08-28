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
)

// T075. data-model §4's matrix, read off the migrated schema field by field.
// Both booleans are one character from silently wrong on each relation, neither
// flip produces a compile error, and the symptoms are a medication that
// outlives the account that owned it (FR-014, SC-012) and an audit trail that
// deletes the record of the deletion (FR-036, FR-037).
func TestTheCascadeMatrixIsExactlyWhatDataModelDeclares(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	users, err := app.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)

	cases := []struct {
		relation    relationRule
		maxSelect   int
		consequence string
	}{
		{
			relation:    relationRule{collection: kind.Medication.Collection(), field: medicationFieldOwner, required: true, cascadeDelete: true},
			maxSelect:   1,
			consequence: "deleting an account must delete every medication row it owns, and a row without an owner is unreachable",
		},
		{
			relation:    relationRule{collection: auditEventsCollection, field: auditFieldActor, required: false, cascadeDelete: false},
			maxSelect:   1,
			consequence: "deleting an account must unset the reference and keep the row, so the account_delete entry survives",
		},
	}

	require.ElementsMatch(t, []relationRule{cases[0].relation, cases[1].relation}, Relations(),
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
			assert.Equal(t, testCase.maxSelect, relation.MaxSelect)
			assert.Equal(t, users.Id, relation.CollectionId)
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

	medication := newRecord(t, app, kind.Medication.Collection(), map[string]any{
		medicationFieldOwner:  user.Id,
		medicationFieldName:   "Amoxicillin",
		medicationFieldStatus: string(clinical.TherapyStatusActive),
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

	orphans, err := app.CountRecords(kind.Medication.Collection(), dbx.HashExp{medicationFieldOwner: user.Id})
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

			relation, err := relationField(collection, medicationFieldOwner)
			require.NoError(t, err)
			testCase.flip(relation)
			require.NoError(t, app.Save(collection))

			err = AssertRelations(app)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrRelationMismatch)
			assert.Contains(t, err.Error(), medicationFieldOwner)
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
