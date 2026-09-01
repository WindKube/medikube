package config

import "github.com/rs/zerolog"

// MarshalZerologObject writes the configuration to the log stream. It is an
// allowlist and not a denylist: a field reaches the stream only by being named
// here, so a secret added to Config later is redacted before anybody notices it
// exists (FR-041).
//
// Secrets are reported as presence, never as value — the operator needs to know
// whether Sentry is wired up, not what the DSN is.
func (c Config) MarshalZerologObject(e *zerolog.Event) {
	e.Str("env", c.Env).
		Str("data_dir", c.DataDir).
		Str("http_addr", c.HTTPAddr).
		Str("log_level", c.Log.Level).
		Bool("sentry", c.Sentry.DSN != "").
		Bool("otel", c.OTel.Enabled).
		Bool("metrics", c.Metrics.Enabled).
		Bool("cursor_key", c.CursorKey != "")
}
