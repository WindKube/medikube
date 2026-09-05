package migrations

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
)

// repointFile is the migration under test in every file in this group,
// composed rather than spelled outright: kind_literals_test.go refuses any
// file outside the kind table that spells a kind's collection literally, and
// the migration's own file name has medications' collection in it.
var repointFile = "1756200600_" + kind.Medication.Collection() + "_repoint.go"

// repointIndex finds the repoint migration's position in the registered
// list, so a test can hand runnerFor exactly the prefix that ends with it.
func repointIndex(t *testing.T, items []*core.Migration) int {
	t.Helper()

	for i, item := range items {
		if strings.Contains(item.File, repointFile) {
			return i
		}
	}

	t.Fatalf("%s is not registered", repointFile)

	return -1
}

// preRepointApp hands back a fully migrated instance reverted to exactly the
// schema the repoint migration expects to find: medications anchored on
// `owner`, patients existing as a collection but with no self-records
// provisioned yet — the phase-001 shape research D-13 backfills from.
//
// It reverts by walking DOWN from newTestApp's fully migrated state rather
// than by stopping a fresh boot partway through, because PocketBase's own
// migration runner has no "migrate to" entry point short of running every
// migration up to a file name — and walking down is the mechanism
// reversible_test.go already established for this package.
func preRepointApp(t *testing.T) (app *tests.TestApp, items []*core.Migration, idx int) {
	t.Helper()

	app = newTestApp(t)
	items = core.AppMigrations.Items()
	idx = repointIndex(t, items)

	full := runnerFor(items)(app)
	toRevert := len(items) - idx

	reverted, err := full.Down(toRevert)
	require.NoError(t, err)
	require.Len(t, reverted, toRevert)

	return app, items, idx
}

// runRepoint applies exactly the repoint migration (and nothing after it,
// which is not yet applied anyway) over an app preRepointApp handed back.
func runRepoint(app *tests.TestApp, items []*core.Migration, idx int) ([]string, error) {
	return runnerFor(items[:idx+1])(app).Up()
}

// seedLegacyUser is one phase-001 account: nothing but the seven profile
// columns every account has always had, no active_patient (this repoints
// entirely without one).
func seedLegacyUser(t *testing.T, app core.App, email, name string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.SetEmail(email)
	record.SetPassword("correct horse battery staple")
	record.Set(usersFieldName, name)
	record.Set(usersFieldRole, string(identity.DefaultRole))
	record.Set(usersFieldUnitSystem, string(identity.DefaultUnitSystem))
	record.Set(usersFieldLocale, identity.DefaultLocale)
	record.Set(usersFieldDateFormat, string(identity.DefaultDateFormat))
	record.Set(usersFieldTheme, string(identity.DefaultTheme))

	require.NoError(t, app.Save(record))

	return record
}

// seedLegacyMedication is one phase-001 medication: owned directly, the
// shape the repoint migration's step 3 reads from and step 6 retires.
func seedLegacyMedication(t *testing.T, app core.App, ownerID, name string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set(medicationFieldOwner, ownerID)
	record.Set(medicationFieldName, name)
	record.Set(medicationFieldStatus, string(clinical.TherapyStatusActive))

	require.NoError(t, app.Save(record))

	return record
}
