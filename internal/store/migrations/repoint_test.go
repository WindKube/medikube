package migrations

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// T072: integration test of the re-attribution against a database seeded in
// the phase-001 shape (FR-022, SC-006).
func TestRepointBackfillsEveryMedicationToItsOwnersSelfRecord(t *testing.T) {
	t.Parallel()

	app, items, idx := preRepointApp(t)

	amara := seedLegacyUser(t, app, "amara@example.com", "Amara Okonkwo")
	chidi := seedLegacyUser(t, app, "chidi@example.com", "Chidi Eze")

	seedLegacyMedication(t, app, amara.Id, "Amara's first")
	seedLegacyMedication(t, app, amara.Id, "Amara's second")
	seedLegacyMedication(t, app, chidi.Id, "Chidi's only")

	applied, err := runRepoint(app, items, idx)
	require.NoError(t, err)
	require.Equal(t, []string{items[idx].File}, applied)

	// Zero unattributed: every medication carries a non-empty patient.
	var unattributed int
	require.NoError(t, app.DB().
		Select("COUNT(*) AS c").
		From(kind.Medication.Collection()).
		Where(dbx.NewExp(medicationFieldPatient+" = '' OR "+medicationFieldPatient+" IS NULL")).
		Row(&unattributed))
	assert.Zero(t, unattributed, "every medication must carry a patient after the backfill")

	// Zero medications on a non-self-record patient: the backfill only ever
	// attributes to the account's own self-record.
	var onNonSelf int
	require.NoError(t, app.DB().NewQuery(
		"SELECT COUNT(*) AS c FROM "+kind.Medication.Collection()+" m "+
			"JOIN "+patientsCollection+" p ON p.id = m."+medicationFieldPatient+" "+
			"WHERE p."+patientFieldIsSelfRecord+" = 0",
	).Row(&onNonSelf))
	assert.Zero(t, onNonSelf, "no medication may land on a patient that is not the recording account's self-record")

	// Per-account counts unchanged: Amara's two stay two, Chidi's one stays
	// one, counted through the patient the backfill just attributed them to.
	assertOwnerMedicationCount(t, app, amara.Id, 2)
	assertOwnerMedicationCount(t, app, chidi.Id, 1)
}

func assertOwnerMedicationCount(t *testing.T, app *tests.TestApp, ownerID string, want int) {
	t.Helper()

	var count int
	require.NoError(t, app.DB().NewQuery(
		"SELECT COUNT(*) AS c FROM "+kind.Medication.Collection()+" m "+
			"JOIN "+patientsCollection+" p ON p.id = m."+medicationFieldPatient+" "+
			"WHERE p."+patientFieldOwner+" = {:owner}",
	).Bind(dbx.Params{"owner": ownerID}).Row(&count))
	assert.Equalf(t, want, count, "owner %s's medication count changed across the repoint", ownerID)
}

// T073: a failed post-condition rolls the entire migration batch back,
// leaving the database exactly as it was (research D-13).
func TestRepointRollsBackTheWholeBatchOnAFailedPostCondition(t *testing.T) {
	t.Parallel()

	app, items, idx := preRepointApp(t)

	// A medication whose owner names no account at all. app.Save validates
	// the relation and would refuse this, so it is written underneath
	// validation — exactly the corrupted state the post-condition exists to
	// catch, and the only way to reach it in this database.
	//
	// The failure actually surfaces one step earlier than
	// ErrUnattributedMedication itself: every RelationField compiles to
	// `TEXT DEFAULT '' NOT NULL` regardless of Required
	// (pocketbase/core/field_relation.go ColumnType), so the backfill's own
	// correlated UPDATE — which finds no self-record for this owner and
	// would assign SQL NULL — hits that NOT NULL constraint directly. Either
	// way the batch is one transaction (core/migrations_runner.go's
	// AuxRunInTransaction), so what this test asserts — the whole thing
	// rolls back — holds regardless of which of the two guards catches it.
	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	orphan := core.NewRecord(collection)
	orphan.Set(medicationFieldOwner, "mkNoSuchAccount1")
	orphan.Set(medicationFieldName, "An orphaned medication")
	require.NoError(t, app.SaveNoValidate(orphan))

	before := schemaSnapshot(t, app)

	var beforeOwner string
	require.NoError(t, app.DB().
		Select(medicationFieldOwner).
		From(kind.Medication.Collection()).
		Where(dbx.HashExp{"id": orphan.Id}).
		Row(&beforeOwner))

	_, err = runRepoint(app, items, idx)
	require.Error(t, err, "an owner naming no account must fail the migration rather than silently repoint")

	// The whole batch rolled back: the schema is exactly what it was, and the
	// orphan's row is untouched rather than half-repointed.
	assert.Equal(t, before, schemaSnapshot(t, app), "a failed post-condition must leave the schema exactly as it was")

	var afterOwner string
	require.NoError(t, app.DB().
		Select(medicationFieldOwner).
		From(kind.Medication.Collection()).
		Where(dbx.HashExp{"id": orphan.Id}).
		Row(&afterOwner))
	assert.Equal(t, beforeOwner, afterOwner)

	// And the collection itself never gained the columns step 1 would have
	// added, because step 1 rolled back along with everything after it.
	reread, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)
	assert.Nil(t, reread.Fields.GetByName(medicationFieldPatient), "the patient column must not survive a rolled-back migration")
}
