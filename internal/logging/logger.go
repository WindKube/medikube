package logging

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/config"
)

// CorrelationField names the log field that carries a request's correlation id.
// It is exported because more than one package writes it and a second spelling
// would quietly break the join between a person's report and the record
// (FR-054).
const CorrelationField = "request_id"

const serviceName = "medikube"

// The wire contract of the one stream, pinned rather than inherited. `ts` and
// `msg` differ from zerolog's defaults; the rest are named so that a change in
// the library cannot silently reshape a format operators grep.
//
// This is package state because zerolog's field names are package state; there
// is no per-logger equivalent. Doing it here rather than in a constructor keeps
// it off the path of any test that builds a logger concurrently.
func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "msg"
	zerolog.ErrorFieldName = "error"
	zerolog.DurationFieldUnit = time.Millisecond
}

// New builds the process-wide base logger: machine-readable JSON on stdout, or
// a console rendering on stderr when an operator asks for it. config.Validate
// refuses Pretty in production, so the second branch cannot be reached by an
// instance holding somebody's records.
func New(cfg config.LogConfig, release string) zerolog.Logger {
	out := io.Writer(os.Stdout)
	if cfg.Pretty {
		out = os.Stderr
	}

	logger := NewTo(out, cfg, release)

	// zerolog.Ctx on a context with no logger attached returns a *disabled*
	// logger, so a background path that forgot to carry one drops its lines in
	// silence. The process has exactly one base logger; point the fallback at it.
	zerolog.DefaultContextLogger = &logger

	return logger
}

// NewTo is New with the destination supplied by the caller and no process-wide
// side effect. It is what a test captures and what the composition root uses
// when stdout is not the answer.
func NewTo(out io.Writer, cfg config.LogConfig, release string) zerolog.Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		// Only reachable if something bypassed config.Validate. A logger that
		// refused to exist would take the report of its own misconfiguration
		// down with it.
		level = zerolog.InfoLevel
	}

	w := out
	if cfg.Pretty {
		w = zerolog.ConsoleWriter{Out: out, TimeFormat: time.Kitchen}
	}

	return zerolog.New(w).
		Level(level).
		With().
		Timestamp().
		Str("service", serviceName).
		Str("release", release).
		Logger()
}

// ForRequest derives the logger every line produced while handling one request
// is written through (FR-053, FR-054).
func ForRequest(base zerolog.Logger, correlationID string) zerolog.Logger {
	return base.With().Str(CorrelationField, correlationID).Logger()
}
