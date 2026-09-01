package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A textual field of the configuration, found by reflection rather than by a
// list somebody maintains. `log:"emit"` is the only way a value reaches the log
// stream, so a field added without thinking about it is redacted by default and
// this test is what makes that true rather than merely intended.
type textField struct {
	path  string
	emit  bool
	value reflect.Value
}

func textFields(v reflect.Value, prefix string, out *[]textField) {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)
		path := prefix + f.Name

		switch {
		case f.Type.Kind() == reflect.Struct:
			textFields(fv, path+".", out)
		case f.Type.Kind() == reflect.String,
			f.Type == reflect.TypeOf([]string(nil)),
			f.Type == reflect.TypeOf(map[string]string(nil)):
			*out = append(*out, textField{path: path, emit: f.Tag.Get("log") == "emit", value: fv})
		}
	}
}

func fillWithSentinels(t *testing.T, cfg *Config) []textField {
	t.Helper()

	var fields []textField
	textFields(reflect.ValueOf(cfg).Elem(), "", &fields)
	require.NotEmpty(t, fields)

	seen := map[string]string{}
	for i := range fields {
		// The trailing Z keeps one sentinel from being a substring of another.
		sentinel := fmt.Sprintf("SENTINEL%dZ", i)
		require.NotContains(t, seen, sentinel)
		seen[sentinel] = fields[i].path

		switch v := fields[i].value; v.Kind() {
		case reflect.String:
			v.SetString(sentinel)
		case reflect.Slice:
			v.Set(reflect.ValueOf([]string{sentinel}))
		case reflect.Map:
			v.Set(reflect.ValueOf(map[string]string{"header": sentinel}))
		default:
			t.Fatalf("%s: unhandled kind %s", fields[i].path, v.Kind())
		}
	}

	return fields
}

func marshal(t *testing.T, cfg Config) string {
	t.Helper()

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Info().EmbedObject(cfg).Msg("configuration")

	require.True(t, json.Valid(buf.Bytes()), "the config marshaller must emit valid JSON")

	return buf.String()
}

// FR-041. Not "the secrets we remembered": every textual field, checked one by
// one, against a value that could only have come from that field.
func TestNoConfigValueReachesTheLogStreamUnlessItIsMarkedEmit(t *testing.T) {
	t.Parallel()

	var cfg Config
	fields := fillWithSentinels(t, &cfg)
	line := marshal(t, cfg)

	emitted := 0
	for i, f := range fields {
		sentinel := fmt.Sprintf("SENTINEL%dZ", i)
		if f.emit {
			emitted++
			assert.Contains(t, line, sentinel,
				"%s is marked log:\"emit\" but the marshaller does not emit it", f.path)

			continue
		}

		assert.NotContains(t, line, sentinel,
			"%s reached the log stream; add log:\"emit\" deliberately or stop logging it (FR-041)", f.path)
	}

	assert.Positive(t, emitted, "no field is emitted at all — the assertion above passes vacuously")
}

// The operator still has to be able to tell whether Sentry is wired up. The
// answer is a boolean, never the credential that answers it.
func TestSecretsAreReportedAsPresenceOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		set    func(*Config)
		key    string
		secret string
	}{
		{
			name:   "sentry dsn",
			set:    func(c *Config) { c.Sentry.DSN = "https://key@sentry.example/1" },
			key:    `"sentry":true`,
			secret: "https://key@sentry.example/1",
		},
		{
			name:   "metrics token",
			set:    func(c *Config) { c.Metrics.Enabled = true; c.Metrics.Token = "s3cr3t-token" },
			key:    `"metrics":true`,
			secret: "s3cr3t-token",
		},
		{
			name:   "cursor key",
			set:    func(c *Config) { c.CursorKey = "cursor-signing-key" },
			key:    `"cursor_key":true`,
			secret: "cursor-signing-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg Config
			tt.set(&cfg)

			line := marshal(t, cfg)
			assert.Contains(t, line, tt.key)
			assert.NotContains(t, line, tt.secret)
		})
	}
}

func TestAbsentSecretsAreReportedAsAbsent(t *testing.T) {
	t.Parallel()

	line := marshal(t, Config{})

	assert.Contains(t, line, `"sentry":false`)
	assert.Contains(t, line, `"otel":false`)
	assert.Contains(t, line, `"metrics":false`)
	assert.Contains(t, line, `"cursor_key":false`)
}
