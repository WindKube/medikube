package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// envPrefix is applied to every key by env.Options, so each `env` tag below
// names its variable without repeating it.
const envPrefix = "MEDIKUBE_"

// Config is the whole of MediKube's configuration. There is no second
// mechanism: no file, no flag that outranks it, no settings collection
// (FR-051).
//
// `log:"emit"` marks the few values the boot line may carry. Everything else is
// redacted by redact.go, and redact_test.go enforces that by reflection rather
// than by a list of known secrets.
type Config struct {
	Env       string `env:"ENV" envDefault:"production" log:"emit"`
	Dev       bool   `env:"DEV" envDefault:"false"`
	DataDir   string `env:"DATA_DIR" log:"emit"`
	HTTPAddr  string `env:"HTTP_ADDR" envDefault:"0.0.0.0:8090" log:"emit"`
	PublicURL string `env:"PUBLIC_URL"`

	DrainDelay time.Duration `env:"DRAIN_DELAY" envDefault:"5s"`
	DrainMax   time.Duration `env:"DRAIN_MAX" envDefault:"25s"`

	AllowedOrigins []string `env:"ALLOWED_ORIGINS" envSeparator:","`
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`

	// The override of last resort for the list cursor's signing key, which is
	// otherwise derived from PocketBase's persisted auth-token secret (CT-3).
	CursorKey string `env:"CURSOR_KEY,file,unset"`

	// Off only for a test instance whose browser suite registers accounts faster than a stranger may.
	RateLimits bool `env:"RATE_LIMITS" envDefault:"true"`

	Log       LogConfig       `envPrefix:"LOG_"`
	Auth      AuthConfig      `envPrefix:"AUTH_"`
	Retention RetentionConfig `envPrefix:"RETENTION_"`
	Sentry    SentryConfig    `envPrefix:"SENTRY_"`
	Metrics   MetricsConfig   `envPrefix:"METRICS_"`
	OTel      OTelConfig      `envPrefix:"OTEL_"`
	Files     FilesConfig     `envPrefix:"FILES_"`
}

type LogConfig struct {
	Level  string `env:"LEVEL" envDefault:"info" log:"emit"`
	Pretty bool   `env:"PRETTY" envDefault:"false"`
}

type AuthConfig struct {
	// Closed by default: an instance reachable from the internet must not accept
	// accounts from strangers (research D-18).
	RegistrationOpen bool          `env:"REGISTRATION_OPEN" envDefault:"false"`
	SessionTTL       time.Duration `env:"SESSION_TTL" envDefault:"168h"`
}

type RetentionConfig struct {
	AuditDays int `env:"AUDIT_DAYS" envDefault:"730"`
}

type SentryConfig struct {
	// `file` lets the DSN arrive as a mounted secret rather than through the
	// process environment, where `docker inspect` would show it; `unset` drops
	// it from os.Environ() after parsing so no subprocess or crash dump repeats
	// it.
	DSN         string  `env:"DSN,file,unset"`
	Environment string  `env:"ENVIRONMENT" envDefault:"production"`
	SampleRate  float64 `env:"SAMPLE_RATE" envDefault:"1.0"`
	Debug       bool    `env:"DEBUG" envDefault:"false"`
}

type MetricsConfig struct {
	Enabled bool `env:"ENABLED" envDefault:"true"`
	// Loopback by default: exposing metrics is an explicit act.
	Addr  string `env:"ADDR" envDefault:"127.0.0.1:9090"`
	Token string `env:"TOKEN,file,unset"`
}

type OTelConfig struct {
	Enabled     bool              `env:"ENABLED" envDefault:"false"`
	Endpoint    string            `env:"ENDPOINT" envDefault:"localhost:4318"`
	Insecure    bool              `env:"INSECURE" envDefault:"true"`
	SampleRatio float64           `env:"SAMPLE_RATIO" envDefault:"1.0"`
	Headers     map[string]string `env:"HEADERS,file" envSeparator:"," envKeyValSeparator:":"`
	Environment string            `env:"ENVIRONMENT" envDefault:"production"`
}

type FilesConfig struct {
	MaxUploadBytes int64    `env:"MAX_UPLOAD_BYTES" envDefault:"33554432"`
	AllowedMIME    []string `env:"ALLOWED_MIME" envSeparator:"," envDefault:"application/pdf,image/png,image/jpeg,image/heic,text/plain"`

	// The three limits patients.photo is migrated with (research D-16, D-18).
	// PhotoMaxBytes is 15 MiB — PocketBase's own DefaultFileFieldMaxSize is 5
	// MiB and rejects an ordinary phone photograph for the wrong reason.
	PhotoMaxBytes  int64    `env:"PHOTO_MAX_BYTES" envDefault:"15728640"`
	PhotoMimeTypes []string `env:"PHOTO_MIME_TYPES" envSeparator:"," envDefault:"image/jpeg,image/png,image/webp"`
	PhotoThumbs    []string `env:"PHOTO_THUMBS" envSeparator:"," envDefault:"100x100t,400x400f"`
}

// Load reads and validates the process environment. It is the only way a
// Config comes into existence, so an unvalidated one cannot reach the rest of
// the program.
func Load() (Config, error) {
	return loadFrom(nil)
}

// loadFrom is the seam the tests use: env.Options.Environment replaces the
// process environment outright, which is what lets the config suite run in
// parallel without t.Setenv.
func loadFrom(environ map[string]string) (Config, error) {
	cfg, err := env.ParseAsWithOptions[Config](env.Options{
		Prefix:      envPrefix,
		Environment: environ,
	})
	if err != nil {
		return Config{}, fmt.Errorf("read environment: %w", err)
	}

	// A Docker or Kubernetes secret is a file, and a file ends with a newline.
	// Carried into a DSN or a bearer token it becomes a third-party rejection
	// with no local symptom.
	cfg.Sentry.DSN = strings.TrimSpace(cfg.Sentry.DSN)
	cfg.Metrics.Token = strings.TrimSpace(cfg.Metrics.Token)
	cfg.CursorKey = strings.TrimSpace(cfg.CursorKey)

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}
