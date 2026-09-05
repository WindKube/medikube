package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// secretFile writes value to a file and returns its path, because the
// secret-bearing variables carry `,file`: their env value is a path, not the
// secret itself.
func secretFile(t *testing.T, name, value string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(value), 0o600))

	return path
}

// minimalEnv is what an operator must supply and nothing more: everything else
// in the struct carries a default.
func minimalEnv() map[string]string {
	return map[string]string{
		"MEDIKUBE_DATA_DIR":   "/var/lib/medikube/pb_data",
		"MEDIKUBE_PUBLIC_URL": "https://medikube.example",
	}
}

func TestLoadAppliesDocumentedDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := loadFrom(minimalEnv())
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.Env)
	assert.False(t, cfg.Dev)
	assert.Equal(t, "0.0.0.0:8090", cfg.HTTPAddr)
	assert.Equal(t, 5*time.Second, cfg.DrainDelay)
	assert.Equal(t, 25*time.Second, cfg.DrainMax)
	assert.Empty(t, cfg.CursorKey)

	assert.Equal(t, "info", cfg.Log.Level)
	assert.False(t, cfg.Log.Pretty)

	assert.False(t, cfg.Auth.RegistrationOpen, "registration is closed by default (research D-18)")
	assert.Equal(t, 168*time.Hour, cfg.Auth.SessionTTL, "seven days (FR-008)")

	assert.Equal(t, 730, cfg.Retention.AuditDays, "two years (data-model, audit_events)")

	assert.Empty(t, cfg.Sentry.DSN)
	assert.Equal(t, "production", cfg.Sentry.Environment)
	assert.InDelta(t, 1.0, cfg.Sentry.SampleRate, 0)
	assert.False(t, cfg.Sentry.Debug)

	assert.True(t, cfg.Metrics.Enabled)
	assert.Equal(t, "127.0.0.1:9090", cfg.Metrics.Addr, "exposing metrics is an explicit act")
	assert.Empty(t, cfg.Metrics.Token)

	assert.False(t, cfg.OTel.Enabled)
	assert.Equal(t, "localhost:4318", cfg.OTel.Endpoint)
	assert.True(t, cfg.OTel.Insecure)
	assert.InDelta(t, 1.0, cfg.OTel.SampleRatio, 0)
	assert.Empty(t, cfg.OTel.Headers)
	assert.Equal(t, "production", cfg.OTel.Environment)

	assert.Equal(t, int64(33_554_432), cfg.Files.MaxUploadBytes)
	assert.Equal(t,
		[]string{"application/pdf", "image/png", "image/jpeg", "image/heic", "text/plain"},
		cfg.Files.AllowedMIME)

	assert.Equal(t, int64(15_728_640), cfg.Files.PhotoMaxBytes, "15 MiB (research D-18)")
	assert.Equal(t, []string{"image/jpeg", "image/png", "image/webp"}, cfg.Files.PhotoMimeTypes)
	assert.Equal(t, []string{"100x100t", "400x400f"}, cfg.Files.PhotoThumbs)
}

func TestEveryVariableParses(t *testing.T) {
	t.Parallel()

	environ := map[string]string{
		"MEDIKUBE_ENV":             "staging",
		"MEDIKUBE_DEV":             "true",
		"MEDIKUBE_DATA_DIR":        "/srv/medikube",
		"MEDIKUBE_HTTP_ADDR":       "127.0.0.1:9000",
		"MEDIKUBE_PUBLIC_URL":      "https://records.example",
		"MEDIKUBE_DRAIN_DELAY":     "2s",
		"MEDIKUBE_DRAIN_MAX":       "45s",
		"MEDIKUBE_ALLOWED_ORIGINS": "https://a.example,https://b.example",
		"MEDIKUBE_TRUSTED_PROXIES": "10.0.0.1,10.0.0.2",
		"MEDIKUBE_CURSOR_KEY":      secretFile(t, "cursor.key", "cursor-key-value"),

		"MEDIKUBE_LOG_LEVEL":  "debug",
		"MEDIKUBE_LOG_PRETTY": "true",

		"MEDIKUBE_AUTH_REGISTRATION_OPEN": "true",
		"MEDIKUBE_AUTH_SESSION_TTL":       "24h",

		"MEDIKUBE_RETENTION_AUDIT_DAYS": "365",

		"MEDIKUBE_SENTRY_DSN":         secretFile(t, "sentry.dsn", "https://key@sentry.example/1"),
		"MEDIKUBE_SENTRY_ENVIRONMENT": "staging",
		"MEDIKUBE_SENTRY_SAMPLE_RATE": "0.25",
		"MEDIKUBE_SENTRY_DEBUG":       "true",

		"MEDIKUBE_METRICS_ENABLED": "false",
		"MEDIKUBE_METRICS_ADDR":    "0.0.0.0:9091",
		"MEDIKUBE_METRICS_TOKEN":   secretFile(t, "metrics.token", "metrics-token-value"),

		"MEDIKUBE_OTEL_ENABLED":      "true",
		"MEDIKUBE_OTEL_ENDPOINT":     "collector:4318",
		"MEDIKUBE_OTEL_INSECURE":     "false",
		"MEDIKUBE_OTEL_SAMPLE_RATIO": "0.1",
		"MEDIKUBE_OTEL_HEADERS":      secretFile(t, "otel.headers", "authorization:Bearer xyz,x-tenant:acme"),
		"MEDIKUBE_OTEL_ENVIRONMENT":  "staging",

		"MEDIKUBE_FILES_MAX_UPLOAD_BYTES": "1024",
		"MEDIKUBE_FILES_ALLOWED_MIME":     "application/pdf,image/png",
		"MEDIKUBE_FILES_PHOTO_MAX_BYTES":  "2048",
		"MEDIKUBE_FILES_PHOTO_MIME_TYPES": "image/png",
		"MEDIKUBE_FILES_PHOTO_THUMBS":     "50x50t",
	}

	// A field nobody added an entry for would otherwise be silently untested.
	require.ElementsMatch(t, envKeys(), keysOf(environ),
		"every MEDIKUBE_ variable the struct declares must be exercised here")

	cfg, err := loadFrom(environ)
	require.NoError(t, err)

	assert.Equal(t, "staging", cfg.Env)
	assert.True(t, cfg.Dev)
	assert.Equal(t, "/srv/medikube", cfg.DataDir)
	assert.Equal(t, "127.0.0.1:9000", cfg.HTTPAddr)
	assert.Equal(t, "https://records.example", cfg.PublicURL)
	assert.Equal(t, 2*time.Second, cfg.DrainDelay)
	assert.Equal(t, 45*time.Second, cfg.DrainMax)
	assert.Equal(t, []string{"https://a.example", "https://b.example"}, cfg.AllowedOrigins)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, cfg.TrustedProxies)
	assert.Equal(t, "cursor-key-value", cfg.CursorKey)

	assert.Equal(t, "debug", cfg.Log.Level)
	assert.True(t, cfg.Log.Pretty)

	assert.True(t, cfg.Auth.RegistrationOpen)
	assert.Equal(t, 24*time.Hour, cfg.Auth.SessionTTL)

	assert.Equal(t, 365, cfg.Retention.AuditDays)

	assert.Equal(t, "https://key@sentry.example/1", cfg.Sentry.DSN)
	assert.Equal(t, "staging", cfg.Sentry.Environment)
	assert.InDelta(t, 0.25, cfg.Sentry.SampleRate, 0)
	assert.True(t, cfg.Sentry.Debug)

	assert.False(t, cfg.Metrics.Enabled)
	assert.Equal(t, "0.0.0.0:9091", cfg.Metrics.Addr)
	assert.Equal(t, "metrics-token-value", cfg.Metrics.Token)

	assert.True(t, cfg.OTel.Enabled)
	assert.Equal(t, "collector:4318", cfg.OTel.Endpoint)
	assert.False(t, cfg.OTel.Insecure)
	assert.InDelta(t, 0.1, cfg.OTel.SampleRatio, 0)
	assert.Equal(t, map[string]string{"authorization": "Bearer xyz", "x-tenant": "acme"}, cfg.OTel.Headers)
	assert.Equal(t, "staging", cfg.OTel.Environment)

	assert.Equal(t, int64(1024), cfg.Files.MaxUploadBytes)
	assert.Equal(t, []string{"application/pdf", "image/png"}, cfg.Files.AllowedMIME)

	assert.Equal(t, int64(2048), cfg.Files.PhotoMaxBytes)
	assert.Equal(t, []string{"image/png"}, cfg.Files.PhotoMimeTypes)
	assert.Equal(t, []string{"50x50t"}, cfg.Files.PhotoThumbs)
}

// A mounted Docker or Kubernetes secret is a file, and files end with a
// newline. Carrying it into a DSN or a bearer token breaks the credential in a
// way that only shows up as a silent 401 from a third party.
func TestFileBackedSecretsLoseTheirTrailingNewline(t *testing.T) {
	t.Parallel()

	environ := minimalEnv()
	environ["MEDIKUBE_SENTRY_DSN"] = secretFile(t, "sentry.dsn", "https://key@sentry.example/1\n")
	environ["MEDIKUBE_METRICS_TOKEN"] = secretFile(t, "metrics.token", "metrics-token-value\n")
	environ["MEDIKUBE_CURSOR_KEY"] = secretFile(t, "cursor.key", "cursor-key-value\n")

	cfg, err := loadFrom(environ)
	require.NoError(t, err)

	assert.Equal(t, "https://key@sentry.example/1", cfg.Sentry.DSN)
	assert.Equal(t, "metrics-token-value", cfg.Metrics.Token)
	assert.Equal(t, "cursor-key-value", cfg.CursorKey)
}

// FR-061: one location holds everything the instance keeps. There is no
// default, because PocketBase's own fallback puts pb_data next to the binary,
// which in the distroless image is a read-only layer.
func TestDataDirIsRequired(t *testing.T) {
	t.Parallel()

	environ := minimalEnv()
	delete(environ, "MEDIKUBE_DATA_DIR")

	_, err := loadFrom(environ)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MEDIKUBE_DATA_DIR")
}

func TestDrainMaxMustExceedDrainDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		delay   string
		max     string
		wantErr bool
	}{
		{name: "max above delay", delay: "5s", max: "25s", wantErr: false},
		{name: "max equals delay", delay: "5s", max: "5s", wantErr: true},
		{name: "max below delay", delay: "5s", max: "1s", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			environ := minimalEnv()
			environ["MEDIKUBE_DRAIN_DELAY"] = tt.delay
			environ["MEDIKUBE_DRAIN_MAX"] = tt.max

			_, err := loadFrom(environ)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), "MEDIKUBE_DRAIN_MAX")
		})
	}
}

func TestValidateRejectsEachSetting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "log level is not a level",
			env:  map[string]string{"MEDIKUBE_LOG_LEVEL": "chatty"},
			want: "MEDIKUBE_LOG_LEVEL",
		},
		{
			name: "env is not one of the documented environments",
			env:  map[string]string{"MEDIKUBE_ENV": "prd"},
			want: "MEDIKUBE_ENV",
		},
		{
			name: "public url is not absolute",
			env:  map[string]string{"MEDIKUBE_PUBLIC_URL": "/records"},
			want: "MEDIKUBE_PUBLIC_URL",
		},
		{
			name: "sentry sample rate is out of range",
			env:  map[string]string{"MEDIKUBE_SENTRY_SAMPLE_RATE": "1.5"},
			want: "MEDIKUBE_SENTRY_SAMPLE_RATE",
		},
		{
			name: "otel sample ratio is out of range",
			env:  map[string]string{"MEDIKUBE_OTEL_SAMPLE_RATIO": "-0.1"},
			want: "MEDIKUBE_OTEL_SAMPLE_RATIO",
		},
		{
			name: "otel is on with no endpoint",
			env:  map[string]string{"MEDIKUBE_OTEL_ENABLED": "true", "MEDIKUBE_OTEL_ENDPOINT": " "},
			want: "MEDIKUBE_OTEL_ENDPOINT",
		},
		{
			name: "metrics address is not host:port",
			env:  map[string]string{"MEDIKUBE_METRICS_ADDR": "9090"},
			want: "MEDIKUBE_METRICS_ADDR",
		},
		{
			name: "session ttl is not positive",
			env:  map[string]string{"MEDIKUBE_AUTH_SESSION_TTL": "0s"},
			want: "MEDIKUBE_AUTH_SESSION_TTL",
		},
		{
			name: "audit retention is not positive",
			env:  map[string]string{"MEDIKUBE_RETENTION_AUDIT_DAYS": "0"},
			want: "MEDIKUBE_RETENTION_AUDIT_DAYS",
		},
		{
			name: "upload ceiling is not positive",
			env:  map[string]string{"MEDIKUBE_FILES_MAX_UPLOAD_BYTES": "0"},
			want: "MEDIKUBE_FILES_MAX_UPLOAD_BYTES",
		},
		{
			// An empty value cannot be tested through the environment: env
			// falls back to envDefault for it, so the reachable failure is a
			// malformed entry. The empty list is still rejected, for a Config
			// built in code.
			name: "upload type is not a media type",
			env:  map[string]string{"MEDIKUBE_FILES_ALLOWED_MIME": "pdf"},
			want: "MEDIKUBE_FILES_ALLOWED_MIME",
		},
		{
			name: "photo size ceiling is not positive",
			env:  map[string]string{"MEDIKUBE_FILES_PHOTO_MAX_BYTES": "0"},
			want: "MEDIKUBE_FILES_PHOTO_MAX_BYTES",
		},
		{
			name: "photo mime type is not a media type",
			env:  map[string]string{"MEDIKUBE_FILES_PHOTO_MIME_TYPES": "jpeg"},
			want: "MEDIKUBE_FILES_PHOTO_MIME_TYPES",
		},
		{
			name: "dev mode in production",
			env:  map[string]string{"MEDIKUBE_DEV": "true"},
			want: "MEDIKUBE_DEV",
		},
		{
			name: "pretty logs in production",
			env:  map[string]string{"MEDIKUBE_LOG_PRETTY": "true"},
			want: "MEDIKUBE_LOG_PRETTY",
		},
		{
			name: "plaintext public url in production",
			env:  map[string]string{"MEDIKUBE_PUBLIC_URL": "http://records.example"},
			want: "MEDIKUBE_PUBLIC_URL",
		},
		{
			name: "metrics exposed to the network with no token in production",
			env:  map[string]string{"MEDIKUBE_METRICS_ADDR": "0.0.0.0:9090"},
			want: "MEDIKUBE_METRICS_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			environ := minimalEnv()
			for k, v := range tt.env {
				environ[k] = v
			}

			_, err := loadFrom(environ)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// FR-051. A validator that stops at the first problem costs one container
// restart per misspelled variable.
func TestEveryProblemIsReportedInOneError(t *testing.T) {
	t.Parallel()

	environ := map[string]string{
		"MEDIKUBE_PUBLIC_URL":         "not-a-url",
		"MEDIKUBE_LOG_LEVEL":          "chatty",
		"MEDIKUBE_ENV":                "prd",
		"MEDIKUBE_SENTRY_SAMPLE_RATE": "9",
		"MEDIKUBE_DRAIN_DELAY":        "30s",
		"MEDIKUBE_DRAIN_MAX":          "10s",
	}

	_, err := loadFrom(environ)
	require.Error(t, err)

	for _, want := range []string{
		"MEDIKUBE_DATA_DIR",
		"MEDIKUBE_PUBLIC_URL",
		"MEDIKUBE_LOG_LEVEL",
		"MEDIKUBE_ENV",
		"MEDIKUBE_SENTRY_SAMPLE_RATE",
		"MEDIKUBE_DRAIN_MAX",
	} {
		assert.Contains(t, err.Error(), want, "the one error must name every offending setting")
	}

	var joined interface{ Unwrap() []error }
	require.ErrorAs(t, err, &joined, "the error must be an errors.Join tree, not a formatted string")
	assert.Len(t, joined.Unwrap(), 6)
}

func TestValidateIsReachableOnItsOwn(t *testing.T) {
	t.Parallel()

	cfg, err := loadFrom(minimalEnv())
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	cfg.DrainMax = cfg.DrainDelay
	cfg.Files.AllowedMIME = nil
	cfg.Files.PhotoThumbs = nil
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MEDIKUBE_DRAIN_MAX")
	assert.Contains(t, err.Error(), "MEDIKUBE_FILES_ALLOWED_MIME")
	assert.Contains(t, err.Error(), "MEDIKUBE_FILES_PHOTO_THUMBS")
}

func TestLoadReadsTheProcessEnvironment(t *testing.T) {
	// Not parallel: t.Setenv forbids it.
	t.Setenv("MEDIKUBE_DATA_DIR", "/srv/medikube")
	t.Setenv("MEDIKUBE_PUBLIC_URL", "https://medikube.example")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/srv/medikube", cfg.DataDir)
}

// envKeys is the reflection walk the documented-environment test also needs:
// every MEDIKUBE_ variable the struct declares, prefixes resolved.
func envKeys() []string {
	var keys []string
	walkEnvKeys(reflect.TypeOf(Config{}), envPrefix, &keys)
	sort.Strings(keys)

	return keys
}

func walkEnvKeys(t reflect.Type, prefix string, out *[]string) {
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Type.Kind() == reflect.Struct {
			walkEnvKeys(f.Type, prefix+f.Tag.Get("envPrefix"), out)
			continue
		}

		name, _, _ := strings.Cut(f.Tag.Get("env"), ",")
		if name != "" {
			*out = append(*out, prefix+name)
		}
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
