package migrations

import (
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T074, ANALYSIS M4. MediKube writes PocketBase's own settings from its
// validated environment at boot and nobody edits them in the admin UI, which is
// what keeps the environment the single source (research D-18). This test is
// about the half of that claim nothing else covers: the write reaches the
// database rather than only the in-memory Settings object, and it is still
// there after a restart.
//
// It is re-asserted rather than assumed on purpose. PocketBase's own test
// harness sets Logs.MaxDays to zero in memory and never saves it, so a settings
// assertion that trusted the last thing written would be reading a value no
// restart would reproduce.
func TestTheSettingsWrittenAtBootSurviveARestart(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "pb_data")

	app := core.NewBaseApp(core.BaseAppConfig{DataDir: dataDir})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())

	// A fresh instance already has batch off; MaxDays is PocketBase's five, so
	// one of the two is a real write and the other is a regression guard.
	require.False(t, app.Settings().Batch.Enabled)
	require.NotEqual(t, LogsMaxDays, app.Settings().Logs.MaxDays,
		"the value under test is already correct on a fresh instance, so persisting it proves nothing")
	require.Error(t, AssertSettings(app))

	app.Settings().Batch.Enabled = false
	app.Settings().Logs.MaxDays = LogsMaxDays
	require.NoError(t, app.Save(app.Settings()))
	require.NoError(t, AssertSettings(app))

	// The restart. Bootstrap resets the previous state, reopens the same
	// directory and reloads settings from the database — so what comes back is
	// what was stored, not what was last assigned.
	require.NoError(t, app.Bootstrap())

	assert.False(t, app.Settings().Batch.Enabled)
	assert.Equal(t, LogsMaxDays, app.Settings().Logs.MaxDays)
	assert.NoError(t, AssertSettings(app))

	// The schema is still there too: a restart is not a re-migration, and
	// nothing this phase writes at boot is applied twice.
	pending, err := Pending(app)
	require.NoError(t, err)
	assert.Empty(t, pending)
	assert.NoError(t, AssertFatal(app))
}

// A second process reading the same directory sees the same settings. The
// restart above reuses one BaseApp, which would still pass if the values were
// cached on the object rather than stored.
func TestASecondInstanceOverTheSameDirectoryReadsTheStoredSettings(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "pb_data")

	first := core.NewBaseApp(core.BaseAppConfig{DataDir: dataDir})
	require.NoError(t, first.Bootstrap())
	require.NoError(t, first.RunAllMigrations())

	first.Settings().Batch.Enabled = false
	first.Settings().Logs.MaxDays = LogsMaxDays
	require.NoError(t, first.Save(first.Settings()))
	require.NoError(t, first.ResetBootstrapState())

	second := core.NewBaseApp(core.BaseAppConfig{DataDir: dataDir})
	t.Cleanup(func() { _ = second.ResetBootstrapState() })
	require.NoError(t, second.Bootstrap())

	assert.NoError(t, AssertSettings(second))
	assert.NoError(t, AssertFatal(second))
}
