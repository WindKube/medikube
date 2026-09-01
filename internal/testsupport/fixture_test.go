package testsupport

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport/seed"
)

// regenEnv is the switch. The generator lives in the test binary rather than in
// a second command because it is the seeder plus a copy, and a command would be
// a third thing to keep in step with the schema.
//
//	MEDIKUBE_FIXTURE_REGEN=1 go test -run TestRegenerateTheCommittedFixture ./internal/testsupport/
const regenEnv = "MEDIKUBE_FIXTURE_REGEN"

// fixtureSidecars are SQLite's write-ahead log and shared-memory files. They are
// never committed: the databases are checkpointed before they are copied, and a
// committed sidecar would be replayed over the committed database on the next
// clone — silently reinstating whatever state the machine that generated it
// happened to be in.
var fixtureSidecars = []string{".db-wal", ".db-shm", ".db-journal"}

// TestRegenerateTheCommittedFixture rewrites internal/testdata/pb_data from an
// empty directory: PocketBase's system migrations, then MediKube's, then the
// settings a MediKube instance boots with, then the seed. Nothing about the
// result depends on the previous fixture, so a schema change cannot leave a
// stale column behind in it.
func TestRegenerateTheCommittedFixture(t *testing.T) {
	if os.Getenv(regenEnv) == "" {
		t.Skipf("set %s=1 to rewrite %s", regenEnv, FixtureDir())
	}

	build := t.TempDir()

	// EncryptionEnv is deliberately empty. Settings are persisted encrypted
	// when the named variable is set, and tests.NewTestApp opens every clone
	// with its own "pb_test_env" — a fixture generated under a production key
	// would fail to boot in every test with "invalid settings db data".
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: build})

	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())

	settings := app.Settings()
	// The half of the batch lockdown that lives in settings: /api/batch calls
	// the record-CRUD handler bodies directly rather than through the router,
	// so the middleware cannot see those sub-requests.
	settings.Batch.Enabled = false
	// One, and never zero: at zero PocketBase's log batcher drops the record
	// before the interception that turns its own failures into zerolog lines
	// ever fires (research D-29). The value is internal/platform/pb's
	// LogRetentionDays, written out here rather than imported so that the
	// harness does not depend on the package whose HTTP tests will depend on
	// the harness.
	settings.Logs.MaxDays = 1
	// An IP address is personal data about the actor that no requirement asks
	// to keep (FR-038, research D-19).
	settings.Logs.LogIP = false
	settings.Logs.LogAuthId = false
	require.NoError(t, app.Save(settings))

	// The rate limiter is deliberately left at PocketBase's default, which is
	// off. internal/platform/pb.ApplySettings turns it on at every boot, so
	// production has it; a fixture carrying it would put every request of every
	// HTTP test through a token bucket shared with the rest of that test, which
	// buys no fidelity and manufactures flakes.

	require.NoError(t, seed.Apply(app))

	checkpoint(t, app)
	require.NoError(t, app.ResetBootstrapState())

	replaceFixture(t, build, FixtureDir())

	t.Logf("rewrote %s — commit it", FixtureDir())
}

// checkpoint folds the write-ahead log back into the database file, so the
// committed fixture is one self-contained file per database.
func checkpoint(t *testing.T, app core.App) {
	t.Helper()

	_, err := app.DB().NewQuery("PRAGMA wal_checkpoint(TRUNCATE)").Execute()
	require.NoError(t, err, "checkpointing the data database")

	_, err = app.AuxDB().NewQuery("PRAGMA wal_checkpoint(TRUNCATE)").Execute()
	require.NoError(t, err, "checkpointing the auxiliary database")
}

func replaceFixture(t *testing.T, from, to string) {
	t.Helper()

	require.NoError(t, os.RemoveAll(to))
	require.NoError(t, os.MkdirAll(to, 0o755))

	entries, err := os.ReadDir(from)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			// Nothing in this phase writes a file — data-model §0 declares zero
			// file fields — so a directory here is PocketBase's own scratch
			// space and does not belong in a committed fixture.
			continue
		}

		if isSidecar(entry.Name()) {
			continue
		}

		copyFile(t, filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name()))
	}

	// Mirrors what PocketBase commits alongside its own tests/data, so the rule
	// is stated where the files are and not only in the repository root.
	require.NoError(t, os.WriteFile(
		filepath.Join(to, ".gitignore"),
		[]byte("*.db-shm\n*.db-wal\n*.db-journal\n"),
		0o644,
	))
}

func isSidecar(name string) bool {
	for _, suffix := range fixtureSidecars {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return false
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()

	source, err := os.Open(from)
	require.NoError(t, err)
	defer source.Close()

	target, err := os.Create(to)
	require.NoError(t, err)
	defer target.Close()

	_, err = io.Copy(target, source)
	require.NoError(t, err)
	require.NoError(t, target.Sync())
}
