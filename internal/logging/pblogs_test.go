package logging

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
)

func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	return app
}

// pbLog builds the record PocketBase's own batch handler builds in
// core.BaseApp.initLogger's WriteFunc, field for field.
func pbLog(message string, level slog.Level, data types.JSONMap[any]) *core.Log {
	record := &core.Log{
		Message: message,
		Level:   int(level),
		Data:    data,
		Created: types.NowDateTime(),
	}
	record.MarkAsNew()
	record.Id = core.GenerateDefaultRandomId()

	return record
}

func countLogRows(t *testing.T, app core.App) int {
	t.Helper()

	var total int
	require.NoError(t, app.AuxDB().Select("count(*)").From(core.LogsTableName).Row(&total))

	return total
}

func TestBridgeLogsDivertsTheRecordIntoTheStream(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	app := newTestApp(t)

	BridgeLogs(app, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

	record := pbLog("Failed to send email", slog.LevelError, types.JSONMap[any]{
		"error": "dial tcp 127.0.0.1:587: connection refused",
	})
	require.NoError(t, app.AuxSave(record))

	entries := lines(t, &buf)
	require.Len(t, entries, 1)

	assert.Equal(t, "Failed to send email", entries[0]["msg"])
	assert.Equal(t, "error", entries[0]["level"])
	assert.Equal(t, pbSource, entries[0]["src"])
	assert.Equal(t, "dial tcp 127.0.0.1:587: connection refused", entries[0]["error"])
	assert.NotEmpty(t, entries[0]["pb_ts"],
		"the batch handler flushes up to three seconds late, so PocketBase's own timestamp has to travel with the line")
}

func TestBridgeLogsNeverPersistsTheRecord(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	app := newTestApp(t)

	before := countLogRows(t, app)

	BridgeLogs(app, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

	record := pbLog("Failed to delete old logs", slog.LevelWarn, nil)
	require.NoError(t, app.AuxSave(record), "cancelling the chain is not an error")

	assert.Equal(t, before, countLogRows(t, app),
		"the hook returns without calling e.Next(), so the INSERT never happens and there is still exactly one log store")

	found, err := app.FindLogById(record.Id)
	assert.Error(t, err)
	assert.Nil(t, found)
}

func TestBridgeLogsMapsEveryPocketBaseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level slog.Level
		want  string
	}{
		{name: "below debug", level: slog.LevelDebug - 4, want: "trace"},
		{name: "debug", level: slog.LevelDebug, want: "debug"},
		{name: "info", level: slog.LevelInfo, want: "info"},
		{name: "warn", level: slog.LevelWarn, want: "warn"},
		{name: "error", level: slog.LevelError, want: "error"},
		{name: "above error", level: slog.LevelError + 4, want: "error"},
	}

	app := newTestApp(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			BridgeLogs(app, NewTo(&buf, config.LogConfig{Level: "trace"}, "test"))

			require.NoError(t, app.AuxSave(pbLog("line", tt.level, nil)))

			entries := lines(t, &buf)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.want, entries[0]["level"])
		})
	}
}

func TestLogRetentionIsOneDayAndNeverZero(t *testing.T) {
	t.Parallel()

	settings := &core.Settings{}
	settings.Logs.MaxDays = 0
	settings.Logs.LogIP = true
	settings.Logs.LogAuthId = true

	keepLogPipelineOpen(settings)

	assert.Equal(t, 1, settings.Logs.MaxDays,
		"research D-29 / reconciliation C4: at 0 the batch handler's BeforeAddFunc returns false, "+
			"the record never enters the batch and the _logs hook never fires, so PocketBase's backup, "+
			"mailer, cron and OAuth2 failures go nowhere at all")
	assert.NotZero(t, settings.Logs.MaxDays, "MaxDays reads like a retention knob and behaves like an off switch")
	assert.Equal(t, int(slog.LevelDebug), settings.Logs.MinLevel,
		"zerolog does the real filtering; PocketBase must not drop lines before the bridge sees them")
	assert.False(t, settings.Logs.LogIP, "FR-038: an IP address identifies a person")
	assert.False(t, settings.Logs.LogAuthId)
}

func TestBridgeLogsBindsTheSettingsHookSoTheRetentionIsAppliedAtBoot(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	app := newTestApp(t)

	before := app.OnBootstrap().Length()
	BridgeLogs(app, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

	assert.Equal(t, before+1, app.OnBootstrap().Length(),
		"nothing else applies the retention setting the interception depends on")
}

func TestBridgeLogsRenamesAnAttributeThatWouldCollideWithTheStreamsOwnFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	app := newTestApp(t)

	BridgeLogs(app, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

	require.NoError(t, app.AuxSave(pbLog("collision", slog.LevelInfo, types.JSONMap[any]{
		"msg":   "not the message",
		"level": "not the level",
		"error": "kept as is",
	})))

	entries := lines(t, &buf)
	require.Len(t, entries, 1, "a duplicate JSON key would make the line undecodable under encoding/json v2")

	assert.Equal(t, "collision", entries[0]["msg"])
	assert.Equal(t, "info", entries[0]["level"])
	assert.Equal(t, "not the message", entries[0]["pb_msg"])
	assert.Equal(t, "not the level", entries[0]["pb_level"])
	assert.Equal(t, "kept as is", entries[0]["error"], "the field operators grep is left alone")
}
