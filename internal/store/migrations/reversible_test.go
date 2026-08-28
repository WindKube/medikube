package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T068, FR-059 and constitution Principle IX. Reversibility is by construction
// — register refuses a migration with a nil down — but "there is a down
// function" and "the down undoes the up" are different claims, and only the
// second is worth anything.
//
// The subtests are not parallel and cannot be: they walk one database forward
// through the migration list one step at a time, and step k's assertion is
// meaningless unless step k-1 left the schema where it said it did. A failed
// step therefore stops the walk rather than reporting a cascade of differences
// that all have one cause.
func TestEveryMigrationUpDownUpLeavesAnIdenticalSchema(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	items := core.AppMigrations.Items()
	require.NotEmpty(t, items, "no migration is registered; this test would pass by finding nothing")

	full := runnerFor(items)(app)

	// after[k] is the schema with the first k migrations applied, recorded by
	// walking down from the fully migrated state that newTestApp hands over.
	after := make([]string, len(items)+1)
	after[len(items)] = schemaSnapshot(t, app)

	for k := len(items); k > 0; k-- {
		reverted, err := full.Down(1)
		require.NoErrorf(t, err, "reverting %s", items[k-1].File)
		require.Equal(t, []string{items[k-1].File}, reverted,
			"Down(1) reverted something other than the newest applied migration")

		after[k-1] = schemaSnapshot(t, app)
	}

	require.NotEqual(t, after[0], after[len(items)],
		"reverting every migration changed nothing, so the comparison below proves nothing")

	// Nothing is applied now. Walking back up must reproduce every recorded
	// step exactly — which is the up → down → up identity, asserted at each
	// migration rather than only for the three of them together.
	for k := 1; k <= len(items); k++ {
		migration := items[k-1]

		if !t.Run(migration.File, func(t *testing.T) {
			applied, err := runnerFor(items[:k])(app).Up()
			require.NoError(t, err)
			require.Equal(t, []string{migration.File}, applied)

			assert.Equal(t, after[k], schemaSnapshot(t, app),
				"re-applying %s produced a different schema than it produced the first time, "+
					"so its down left something behind or its up is not deterministic", migration.File)
		}) {
			t.FailNow()
		}
	}
}

// The stronger half of the same claim: reverting every migration does not merely
// return to *a* consistent schema, it returns to the one PocketBase's own system
// migrations create. data-model §5 requires migration 1's down to restore
// PocketBase's stock users rules rather than leave them nil, "because a reversal
// that leaves a collection in a state PocketBase's own migrations did not create
// is not a reversal".
//
// The comparison is against a second instance that has never run an app
// migration at all, so nothing about it is derived from the code under test.
func TestRevertingEveryMigrationRestoresPocketBasesOwnSchema(t *testing.T) {
	t.Parallel()

	migrated := newTestApp(t)

	items := core.AppMigrations.Items()
	require.NotEmpty(t, items)

	reverted, err := runnerFor(items)(migrated).Down(len(items))
	require.NoError(t, err)
	require.Len(t, reverted, len(items))

	// Bootstrap runs the system migrations and stops there; the app migrations
	// are applied later, by RunAllMigrations, which this instance never calls.
	stock := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	require.NoError(t, stock.Bootstrap())
	t.Cleanup(func() { _ = stock.ResetBootstrapState() })

	assert.Equal(t, schemaSnapshot(t, stock), schemaSnapshot(t, migrated))
}
