package logging

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
)

// newPB builds an un-bootstrapped PocketBase instance. Nothing here touches the
// database: mechanism 1 is a field reassignment and is complete before
// bootstrap, which is the whole reason it can catch the lines PocketBase writes
// on its way up.
func newPB(t *testing.T) *pocketbase.PocketBase {
	t.Helper()

	return pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})
}

func TestBridgeAppReassignsTheExportedEmbeddedField(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	pb := newPB(t)

	before := pb.App
	require.NotNil(t, before)

	BridgeApp(pb, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

	assert.NotSame(t, before, pb.App,
		"CT-1 mechanism 1 is the reassignment of pocketbase.PocketBase's exported embedded core.App")
	assert.Equal(t, before.DataDir(), pb.DataDir(),
		"the decorator embeds the original app, so everything except Logger() still resolves to it")
}

func TestBridgeAppSendsPocketBaseFrameworkLogsToTheStream(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	pb := newPB(t)

	pb.Logger().Info("before the bridge")
	require.Empty(t, buf.String(), "without the bridge PocketBase's logger goes anywhere but here")

	BridgeApp(pb, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

	pb.Logger().Info("cron finished", slog.String("job", "backup"))

	entries := lines(t, &buf)
	require.Len(t, entries, 1)

	assert.Equal(t, "cron finished", entries[0]["msg"])
	assert.Equal(t, "backup", entries[0]["job"], "slog attributes survive as zerolog fields")
	assert.Equal(t, pbSource, entries[0]["src"],
		"one stream, but a reader must still be able to tell whose line it is")
	assert.Equal(t, "medikube", entries[0]["service"], "the process fields come along")
}

func TestBridgeAppResolvesThroughTheInterfaceValuePocketBaseHandsAround(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	pb := newPB(t)
	BridgeApp(pb, NewTo(&buf, config.LogConfig{Level: "debug"}, "test"))

	// This is what apis/base.go does per event: `event.App = app`. The decorator
	// has to be reachable through that interface value or the request path is
	// not covered at all (research D-29).
	var app core.App = pb
	app.Logger().Warn("failed to load auth token")

	entries := lines(t, &buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "warn", entries[0]["level"])
	assert.Equal(t, "failed to load auth token", entries[0]["msg"])
}

func TestBridgeAppMapsEverySlogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level slog.Level
		want  string
	}{
		{name: "debug", level: slog.LevelDebug, want: "debug"},
		{name: "info", level: slog.LevelInfo, want: "info"},
		{name: "warn", level: slog.LevelWarn, want: "warn"},
		{name: "error", level: slog.LevelError, want: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			pb := newPB(t)
			BridgeApp(pb, NewTo(&buf, config.LogConfig{Level: "trace"}, "test"))

			pb.Logger().Log(t.Context(), tt.level, "line")

			entries := lines(t, &buf)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.want, entries[0]["level"])
		})
	}
}
