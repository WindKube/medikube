package logging

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"
)

const (
	settingsHookID = "medikubeLogSettings"
	logsHookID     = "medikubeLogBridge"
)

// reserved are the field names this stream owns. A PocketBase log attribute
// sharing one would emit a duplicate JSON key, which Go 1.27's encoding/json v2
// rejects outright on the way back in, so a collision is renamed rather than
// dropped. "error" is deliberately absent: nothing here writes it, and it is
// the field an operator greps for.
var reserved = map[string]bool{
	"ts": true, "level": true, "msg": true,
	"service": true, "release": true, "src": true, "pb_ts": true,
}

// BridgeLogs is mechanism 2 of the log bridge (CT-1): it intercepts the write
// to PocketBase's own _logs collection, emits the record into zerolog, and
// returns without calling e.Next() so the row is never inserted. The collection
// stays permanently empty and there is still exactly one log store to consult.
//
// This is the half BridgeApp cannot do. core.BaseApp.createTxApp shallow-copies
// a *BaseApp, so a transaction-scoped app carries the hardcoded internal logger
// and every line written inside RunInTransaction — migrations among them —
// bypasses the decorator entirely (research D-29).
//
// It also binds the settings that keep the pipeline open. Without them the
// interception never fires at all.
func BridgeLogs(app core.App, base zerolog.Logger) {
	log := base.With().Str("src", pbSource).Logger()

	app.OnBootstrap().Bind(&hook.Handler[*core.BootstrapEvent]{
		Id: settingsHookID,
		Func: func(e *core.BootstrapEvent) error {
			if err := e.Next(); err != nil {
				return err
			}

			settings := e.App.Settings()
			keepLogPipelineOpen(settings)

			return e.App.Save(settings)
		},
	})

	app.OnModelCreate(core.LogsTableName).Bind(&hook.Handler[*core.ModelEvent]{
		Id:       logsHookID,
		Priority: -99999,
		Func: func(e *core.ModelEvent) error {
			record, ok := e.Model.(*core.Log)
			if !ok {
				return e.Next()
			}

			emit(log, record)

			// Deliberately not calling e.Next(). Trigger stops the chain and
			// reports no error, so the INSERT never happens.
			return nil
		},
	})
}

// keepLogPipelineOpen configures PocketBase's log settings so the bridge can
// see anything at all.
//
// MaxDays must be 1 and must never be 0. The setting reads like a retention
// knob and behaves like an off switch: BeforeAddFunc returns
// `MaxDays > 0`, so at zero the record never enters the batch, this hook never
// fires, and in production printLog does not run either — PocketBase's backup,
// mailer, cron and OAuth2 failures would go nowhere at all. Mechanism 2
// guarantees no row is ever written, so keeping the pipe open costs no storage
// (constitution Principle VI, research D-29, reconciliation C4).
func keepLogPipelineOpen(settings *core.Settings) {
	settings.Logs.MaxDays = 1
	settings.Logs.MinLevel = int(slog.LevelDebug) // zerolog does the real filtering
	settings.Logs.LogIP = false                   // FR-038: an IP address identifies a person
	settings.Logs.LogAuthId = false
}

func emit(log zerolog.Logger, record *core.Log) {
	event := log.WithLevel(zerologLevel(slog.Level(record.Level)))

	for key, value := range record.Data {
		if reserved[key] {
			key = "pb_" + key
		}

		event = event.Interface(key, value)
	}

	// The batch handler flushes on a three-second ticker, so PocketBase's own
	// timestamp is the only honest one on the line.
	event.Time("pb_ts", record.Created.Time()).Msg(record.Message)
}

func zerologLevel(level slog.Level) zerolog.Level {
	switch {
	case level < slog.LevelDebug:
		return zerolog.TraceLevel
	case level < slog.LevelInfo:
		return zerolog.DebugLevel
	case level < slog.LevelWarn:
		return zerolog.InfoLevel
	case level < slog.LevelError:
		return zerolog.WarnLevel
	default:
		return zerolog.ErrorLevel
	}
}
