package pb_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/platform/pb"
)

// The data directory and the dev flag are the two options that must be settled
// before Bootstrap, which is the whole reason MediKube constructs PocketBase
// with NewWithConfig instead of New (research D-06).
//
// HideStartBanner is deliberately not asserted here: PocketBase keeps it in an
// unexported field and consumes it only when Start builds the serve command
// (pocketbase.go:172). There is no getter, so an assertion would have to test a
// private field or a running process. The banner is covered instead by
// internal/logging's single-stream test, which asserts the process writes
// nothing but zerolog.
func TestNewReadsTheDataDirectoryAndDevFlagFromTheMediKubeConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dev  bool
		env  string
	}{
		{name: "production", dev: false, env: "production"},
		{name: "development", dev: true, env: "development"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "pb_data")

			cfg := testConfig(t, dir)
			cfg.Env = tc.env
			cfg.Dev = tc.dev

			app := pb.New(cfg, pb.Options{})
			require.NotNil(t, app, "pocketbase.NewWithConfig returned nil")

			assert.Equal(t, dir, app.DataDir(), "MEDIKUBE_DATA_DIR is the only place data may land (FR-061)")
			assert.Equal(t, tc.dev, app.IsDev())
		})
	}
}

// DBConnect is the injection point otelsql attaches to (research D-30), so what
// matters is that MediKube's function is the one PocketBase actually calls —
// for every one of the four connections it opens.
func TestNewRoutesEveryDatabaseConnectionThroughTheSuppliedDBConnect(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	var paths []string

	app := pb.New(testConfig(t, filepath.Join(t.TempDir(), "pb_data")), pb.Options{
		DBConnect: func(dbPath string) (*dbx.DB, error) {
			calls.Add(1)
			paths = append(paths, filepath.Base(dbPath))

			return core.DefaultDBConnect(dbPath)
		},
	})
	require.NotNil(t, app)

	require.NoError(t, app.Bootstrap())
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	// Four, not one: core/base.go:1240 and :1248 open the data database
	// concurrent and non-concurrent, :1302 and :1310 do the same for the
	// auxiliary one. Anything with a per-connection side effect pays for it
	// four times.
	assert.EqualValues(t, 4, calls.Load(), "PocketBase opens four connections and every one must go through the hook")
	assert.ElementsMatch(t, []string{"data.db", "data.db", "auxiliary.db", "auxiliary.db"}, paths)
}

// An omitted DBConnect must leave PocketBase's own connector in place rather
// than installing a nil function, because a nil there is a panic at bootstrap
// and the composition root is allowed not to instrument.
func TestNewWithoutADBConnectStillBoots(t *testing.T) {
	t.Parallel()

	app := pb.New(testConfig(t, filepath.Join(t.TempDir(), "pb_data")), pb.Options{})
	require.NotNil(t, app)

	require.NoError(t, app.Bootstrap())
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	assert.NotNil(t, app.DB(), "a bootstrapped app has a data database")
}
