package migrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// T076, FR-063. Against a directory that does not merely lack a database but
// lacks everything, the instance creates what it needs and comes up with the
// full schema.
//
// It deliberately uses core.NewBaseApp rather than tests.NewTestApp: the test
// harness clones a directory and forces settings of its own, and neither of
// those exists on the first boot of a real deployment. The only thing between
// an empty path and a working instance here is Bootstrap and RunAllMigrations.
func TestTheInstanceStartsAgainstACompletelyEmptyDataDirectory(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "pb_data")

	entries, err := os.ReadDir(filepath.Dir(dataDir))
	require.NoError(t, err)
	require.Empty(t, entries, "the directory is not empty, so this proves nothing")

	_, err = os.Stat(dataDir)
	require.ErrorIs(t, err, os.ErrNotExist, "the data directory must not exist yet")

	app := core.NewBaseApp(core.BaseAppConfig{DataDir: dataDir})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())

	for _, name := range []string{"data.db", "auxiliary.db"} {
		_, statErr := os.Stat(filepath.Join(dataDir, name))
		assert.NoErrorf(t, statErr, "%s was not created", name)
	}

	pending, err := Pending(app)
	require.NoError(t, err)
	assert.Empty(t, pending)

	for _, name := range []string{usersCollection, kind.Medication.Collection(), auditEventsCollection} {
		collection, findErr := app.FindCollectionByNameOrId(name)
		assert.NoErrorf(t, findErr, "%s was not created", name)
		assert.NotNil(t, collection)
	}

	// The two assertions that refuse to start. On a fresh instance they pass,
	// which is the whole of FR-063: nothing about the first boot is a special
	// case that has to be waived.
	assert.NoError(t, AssertFatal(app))

	// AssertStrict's settings half is deliberately not asserted here: the two
	// settings it checks are written by the boot sequence, not by a migration,
	// and this instance has no boot sequence. The relation half is schema and
	// is asserted.
	assert.NoError(t, AssertRelations(app))
}
