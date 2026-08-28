package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
)

// lines decodes the captured stream. Every entry MediKube emits is one JSON
// object on one line; a test that cannot decode the buffer has already found
// the bug it was looking for.
func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any

	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}

		var entry map[string]any
		require.NoErrorf(t, json.Unmarshal([]byte(raw), &entry), "not a JSON log line: %s", raw)
		out = append(out, entry)
	}

	return out
}

func TestNewEmitsOneJSONObjectPerEvent(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := NewTo(&buf, config.LogConfig{Level: "info"}, "v1.2.3")

	log.Info().Str("extra", "value").Msg("started")

	entries := lines(t, &buf)
	require.Len(t, entries, 1)

	assert.Equal(t, "info", entries[0]["level"])
	assert.Equal(t, "started", entries[0]["msg"])
	assert.Equal(t, "medikube", entries[0]["service"])
	assert.Equal(t, "v1.2.3", entries[0]["release"])
	assert.Equal(t, "value", entries[0]["extra"])
	assert.NotEmpty(t, entries[0]["ts"], "every line is timestamped")
}

func TestNewHonoursTheConfiguredLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level string
		want  []string
	}{
		{name: "debug shows everything from debug up", level: "debug", want: []string{"debug", "info", "warn", "error"}},
		{name: "info drops debug", level: "info", want: []string{"info", "warn", "error"}},
		{name: "warn drops info", level: "warn", want: []string{"warn", "error"}},
		{name: "error keeps only errors", level: "error", want: []string{"error"}},
		{name: "an unparseable level falls back to info rather than going silent", level: "not-a-level", want: []string{"info", "warn", "error"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			log := NewTo(&buf, config.LogConfig{Level: tt.level}, "test")

			log.Debug().Msg("d")
			log.Info().Msg("i")
			log.Warn().Msg("w")
			log.Error().Msg("e")

			entries := lines(t, &buf)

			got := make([]string, 0, len(entries))
			for _, entry := range entries {
				got = append(got, entry["level"].(string))
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrettyModeIsConsoleOutputRatherThanJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := NewTo(&buf, config.LogConfig{Level: "info", Pretty: true}, "test")

	log.Info().Str("request_id", "abc").Msg("hello")

	out := buf.String()
	require.NotEmpty(t, out)

	var entry map[string]any
	assert.Error(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &entry),
		"pretty mode is for a human terminal; it must not claim to be the machine-readable stream")
	assert.Contains(t, out, "hello")
	assert.Contains(t, out, "abc")
}

func TestForRequestPutsTheCorrelationIdOnEveryLine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := NewTo(&buf, config.LogConfig{Level: "debug"}, "test")

	log := ForRequest(base, "corr-1234")
	log.Debug().Msg("one")
	log.Info().Msg("two")
	log.Error().Msg("three")

	entries := lines(t, &buf)
	require.Len(t, entries, 3)

	for _, entry := range entries {
		assert.Equal(t, "corr-1234", entry[CorrelationField],
			"FR-054: a report is tied to the record by this id alone, so a line without it is unusable")
		assert.Equal(t, "medikube", entry["service"], "the child keeps the process fields")
	}
}

func TestForRequestDoesNotLeakBackIntoTheBaseLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := NewTo(&buf, config.LogConfig{Level: "info"}, "test")

	requestLog := ForRequest(base, "corr-1")
	requestLog.Info().Msg("in request")
	base.Info().Msg("outside request")

	entries := lines(t, &buf)
	require.Len(t, entries, 2)

	assert.Equal(t, "corr-1", entries[0][CorrelationField])
	assert.NotContains(t, entries[1], CorrelationField,
		"a background line must not inherit the previous request's correlation id")
}

func TestForRequestNestsSoLaterFieldsKeepTheCorrelationId(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := NewTo(&buf, config.LogConfig{Level: "info"}, "test")

	child := ForRequest(base, "corr-9").With().Str("component", "medication").Logger()
	child.Info().Msg("saved")

	entries := lines(t, &buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "corr-9", entries[0][CorrelationField])
	assert.Equal(t, "medication", entries[0]["component"])
}

func TestNewSetsAContextFallbackSoACtxLessPathIsNotSilent(t *testing.T) {
	t.Parallel()

	_ = New(config.LogConfig{Level: "info"}, "test")

	assert.NotNil(t, zerolog.DefaultContextLogger,
		"zerolog.Ctx on a context with no logger returns a disabled one, which would drop the line silently")
}
