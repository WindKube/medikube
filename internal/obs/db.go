package obs

import (
	"fmt"

	"github.com/XSAM/otelsql"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// sqliteDriver is the driver name modernc.org/sqlite registers itself under and
// the logical name dbx has to be told.
//
// The second use is the one that is easy to get wrong: dbx.NewFromDB falls back
// to NewStandardBuilder when it does not recognise the name (dbx/db.go:321),
// and the standard builder quotes identifiers the way ANSI SQL does rather than
// the way SQLite does. Nothing fails at boot; queries fail later (research
// D-30).
const sqliteDriver = "sqlite"

// pocketbasePragmas is copied VERBATIM from core.DefaultDBConnect
// (core/db_connect.go) in PocketBase v0.40.1.
//
// It has to be copied because it is a local variable inside that function
// rather than an exported constant, and overriding Config.DBConnect replaces
// the function wholesale — there is no way to wrap it and keep the pragmas.
//
// Get it wrong and nothing fails: the database opens, the application runs, and
// WAL or foreign keys are quietly not set. It surfaces weeks later as lock
// contention or as a constraint that never fired. db_test.go is the drift
// check, and entry 4 of docs/pocketbase-upgrade-checklist.md is the procedure
// (risk R8).
//
// Note from upstream, preserved because it is load-bearing: busy_timeout must
// be first, because the connection has to be set to block on busy before WAL
// mode is set.
const pocketbasePragmas = "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=journal_size_limit(200000000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=temp_store(MEMORY)&_pragma=cache_size(-32000)&_defensive=1"

// InstrumentedDBConnect is what pocketbase.Config.DBConnect is set to.
//
// It returns nil when tracing is inactive, and that is the important half. Nil
// means PocketBase calls core.DefaultDBConnect itself, so an untraced
// deployment — every deployment that has not configured an OTLP endpoint —
// opens its database through PocketBase's own function with PocketBase's own
// pragma string and never touches the copy above. The drift risk is confined to
// the configuration that asked for instrumentation.
//
// PocketBase calls the result FOUR times per boot: the data and auxiliary
// databases, each concurrent and non-concurrent (core/base.go:1240, :1248,
// :1302, :1310). That is why this uses otelsql.Open and not otelsql.Register —
// Register mints a global database/sql driver slot per call and opens with an
// empty DSN, so four boots' worth of slots would leak for one process
// (research D-30).
func InstrumentedDBConnect(tracing *Tracing) core.DBConnectFunc {
	if !tracing.Active() {
		return nil
	}

	return instrumentedDBConnect(tracing.TracerProvider())
}

// instrumentedDBConnect is the half that does not decide whether to
// instrument, so a test can hand it a provider that records rather than one
// that exports.
func instrumentedDBConnect(provider trace.TracerProvider) core.DBConnectFunc {
	return func(dbPath string) (*dbx.DB, error) {
		db, err := otelsql.Open(sqliteDriver, dbPath+pocketbasePragmas, dbTraceOptions(provider)...)
		if err != nil {
			return nil, fmt.Errorf("open the instrumented %s database at %s: %w", sqliteDriver, dbPath, err)
		}

		return dbx.NewFromDB(db, sqliteDriver), nil
	}
}

// dbTraceOptions is what a span is allowed to say about a query.
//
// DisableQuery is the FR-038 decision and it is not a default: otelsql puts the
// statement text on every span as db.query.text unless told not to. PocketBase
// binds its parameters, so the text is ordinarily structure rather than
// content — but "ordinarily" is the wrong strength for a span destination
// MediKube does not own, and a filter assembled with an inlined literal would
// carry a medication name to a third party for as long as the trace is
// retained. The method name is enough to find a slow query; the query itself
// is in the code.
//
// Ping and RowsNext stay off because they are per-row and per-poll events on a
// connection pool PocketBase opens four times, and DisableErrSkip is on because
// driver.ErrSkip is how database/sql negotiates optional interfaces, not a
// failure.
func dbTraceOptions(provider trace.TracerProvider) []otelsql.Option {
	return []otelsql.Option{
		otelsql.WithTracerProvider(provider),
		otelsql.WithAttributes(attribute.String("db.system.name", sqliteDriver)),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			DisableQuery:         true,
			Ping:                 false,
			RowsNext:             false,
			DisableErrSkip:       true,
			OmitConnResetSession: true,
		}),
	}
}
