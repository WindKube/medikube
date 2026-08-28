package logging

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
)

// wrap presents a bootstrapped app the way the composition root does: as the
// exported embedded core.App of a pocketbase.PocketBase, which is the field
// mechanism 1 reassigns.
func wrap(app core.App) *pocketbase.PocketBase {
	return &pocketbase.PocketBase{App: app}
}

// flush forces PocketBase's batch handler to write. In production it fires on a
// three-second ticker; a test that waited for it would be slow and flaky, and
// this is the same call PocketBase itself makes on terminate.
func flush(t *testing.T, app core.App) {
	t.Helper()

	handler, ok := app.Logger().Handler().(*logger.BatchHandler)
	require.True(t, ok, "PocketBase no longer batches its own logs; the whole of mechanism 2 rests on this")
	require.NoError(t, handler.WriteAll(t.Context()))
}

func TestTheDecoratorCarriesTheFrameworkLinesThatReachTheAppInterface(t *testing.T) {
	t.Parallel()

	// The real call shapes from PocketBase v0.40.1, each from a subsystem that
	// has nothing to do with serving a request.
	tests := []struct {
		name    string
		message string
		attrs   []any
	}{
		{name: "cron", message: "Failed to delete old logs", attrs: []any{slog.String("error", "database is locked")}},
		{name: "mailer", message: "Failed to send verification email", attrs: []any{slog.String("error", "dial tcp: connection refused")}},
		{name: "backup", message: "Failed to create backup", attrs: []any{slog.String("name", "pb_backup.zip")}},
		{name: "migration", message: "Applied new migration", attrs: []any{slog.String("file", "1717233556_v0.23_migrate.go")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			pb := newPB(t)
			BridgeApp(pb, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

			pb.Logger().Error(tt.message, tt.attrs...)

			entries := lines(t, &buf)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.message, entries[0]["msg"])
			assert.Equal(t, pbSource, entries[0]["src"])
		})
	}
}

func TestTheDecoratorAloneMissesTheTransactionPath(t *testing.T) {
	t.Parallel()

	var decorated bytes.Buffer

	app := newTestApp(t)
	pb := wrap(app)
	BridgeApp(pb, NewTo(&decorated, config.LogConfig{Level: "debug"}, "test"))

	pb.Logger().Warn("outside a transaction")
	require.NotEmpty(t, decorated.String(), "the decorator is live")

	decorated.Reset()

	require.NoError(t, pb.RunInTransaction(func(txApp core.App) error {
		txApp.Logger().Warn("Failed to delete old logs.db file")

		return nil
	}))

	assert.Empty(t, decorated.String(),
		"createTxApp does `clone := *app` on a *BaseApp, so the transaction-scoped app keeps PocketBase's "+
			"hardcoded logger and mechanism 1 never sees the line (research D-29)")
}

func TestTheLogsInterceptionCoversWhatTheDecoratorMisses(t *testing.T) {
	t.Parallel()

	var bridged bytes.Buffer

	app := newTestApp(t)
	BridgeLogs(app, NewTo(&bridged, config.LogConfig{Level: "debug"}, "test"))
	keepLogPipelineOpen(app.Settings())

	before := countLogRows(t, app)

	require.NoError(t, app.RunInTransaction(func(txApp core.App) error {
		txApp.Logger().Warn("Failed to delete old logs.db file")

		return nil
	}))
	flush(t, app)

	assert.Contains(t, bridged.String(), "Failed to delete old logs.db file",
		"mechanism 2 is the half that covers transaction-scoped logging")
	assert.Equal(t, before, countLogRows(t, app), "and it still never writes a row")
}

func TestTheLogsInterceptionNeverFiresWhenRetentionIsZero(t *testing.T) {
	t.Parallel()

	var bridged bytes.Buffer

	app := newTestApp(t)
	BridgeLogs(app, NewTo(&bridged, config.LogConfig{Level: "debug"}, "test"))

	app.Settings().Logs.MaxDays = 0
	app.Logger().Error("Failed to create backup")
	flush(t, app)

	require.Empty(t, bridged.String(),
		"BeforeAddFunc returns MaxDays > 0, so at zero the record never enters the batch, the hook never "+
			"fires, and PocketBase's own failures go nowhere at all (research D-29, reconciliation C4)")

	keepLogPipelineOpen(app.Settings())
	app.Logger().Error("Failed to create backup")
	flush(t, app)

	assert.Contains(t, bridged.String(), "Failed to create backup",
		"MaxDays = 1 keeps the pipe open; the interception is what keeps the table empty")
}

func TestTheLogsInterceptionAloneMissesEverythingThatNeverReachesTheCollection(t *testing.T) {
	t.Parallel()

	var bridged bytes.Buffer

	// Nothing is bootstrapped yet, so there is no batch handler and no _logs
	// pipeline for mechanism 2 to intercept — and initLogger's WriteFunc drops
	// every record until IsBootstrapped() is true regardless.
	pb := newPB(t)
	BridgeLogs(pb, NewTo(&bridged, config.LogConfig{Level: "debug"}, "test"))

	pb.Logger().Error("Failed to initialize the app")
	require.Empty(t, bridged.String(), "mechanism 2 sees only what becomes a _logs record")

	BridgeApp(pb, NewTo(&bridged, config.LogConfig{Level: "debug"}, "test"))
	pb.Logger().Error("Failed to initialize the app")

	assert.Contains(t, bridged.String(), "Failed to initialize the app",
		"only the decorator covers this path; both mechanisms, or there is a hole (CT-1)")
}
