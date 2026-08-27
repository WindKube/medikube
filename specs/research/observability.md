# MediGo — Observability & Cross-Cutting Concerns

Research basis: **PocketBase v0.40.1 source read directly** from the module cache
(`~/go/pkg/mod/github.com/pocketbase/pocketbase@v0.40.1`), plus `pocketbase/dbx@v1.12.0`,
`rs/zerolog@v1.35.1`, `getsentry/sentry-go@v0.48.0`, `prometheus/client_golang@v1.24.1`,
`caarlos0/env/v11@v11.4.1`, `modernc.org/sqlite@v1.57.0`. Third-party facts (samber/*, XSAM/otelsql,
sentry-go/otel) verified against pkg.go.dev / upstream source, August 2026.

Every claim about PocketBase internals below cites a file:line you can re-verify.

---

## 0. TL;DR verdicts

| Question | Verdict |
|---|---|
| slog → zerolog adapter | **Clean.** zerolog ships `zerolog.NewSlogHandler` natively since v1.35 (`rs/zerolog/slog.go`). No `samber/slog-zerolog` needed. |
| Injecting that handler into PocketBase | **Hacky — PB hardcodes it.** `BaseAppConfig` has no logger field; `(*BaseApp).initLogger()` does `app.logger = slog.New(logger.NewBatchHandler(...))` (core/base.go:1536). No setter exists. Workaround = intercept the `_logs` model write. |
| otelsql against PB | **Feasible and clean.** `pocketbase.Config.DBConnect` + `otelsql.Open` + `dbx.NewFromDB(db, "sqlite")`. One 20-line function. One caveat: you must copy PB's pragma DSN string. |
| Prometheus vs OTel metrics | **Prometheus native for app metrics; OTel for traces only; bridge otelsql's OTel metrics into the same Prometheus registry.** One `/metrics`, two producers. |
| samber/mo | **Ban it.** Breaks `errors.Is/As`, breaks Sentry/zerolog error plumbing, fights every reviewer's Go instincts. |
| samber/ro | **Ban it.** It is RxGo (Observables), v0.4.1 pre-1.0. Zero fit for a CRUD records app. |

---

## 1. zerolog as the single app logger

### 1.1 Container setup

```go
// internal/obs/log.go
package obs

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// NewLogger builds the process-wide base logger.
// JSON to stdout, RFC3339Nano timestamps, level from config, caller on warn+.
func NewLogger(cfg LogConfig, release string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "msg"
	zerolog.ErrorFieldName = "error"
	zerolog.CallerFieldName = "caller"
	zerolog.TimestampFieldName = "ts"

	// Trim the caller to pkg/file.go:line — full build paths are noise in a container.
	zerolog.CallerMarshalFunc = func(_ uintptr, file string, line int) string {
		if i := strings.LastIndexByte(file, '/'); i >= 0 {
			if j := strings.LastIndexByte(file[:i], '/'); j >= 0 {
				file = file[j+1:]
			}
		}
		return file + ":" + strconv.Itoa(line)
	}

	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	var w io.Writer = os.Stdout
	if cfg.Pretty { // dev only; never in the container
		w = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen}
	}

	return zerolog.New(w).
		Level(level).
		With().
		Timestamp().
		Str("service", "medigo").
		Str("release", release).
		Logger()
}
```

**Do not enable `.Caller()` on the base logger.** It costs a `runtime.Caller` on every event.
Attach it only where it pays: `log.Warn()`/`log.Error()` call sites via a derived logger, or
globally in dev. If you want it everywhere, measure first.

### 1.2 Stack traces — the honest recommendation

zerolog's stack support (`zerolog.ErrorStackMarshaler` + `zerolog/pkgerrors.MarshalStack`)
requires `github.com/pkg/errors` (archived) and only produces frames for errors *created* by
that package. Wrapping with `fmt.Errorf("%w")` yields nothing.

**Recommendation: do not add `pkg/errors`.** Capture stacks at the Sentry boundary instead —
`sentry.CaptureException` walks the goroutine stack itself and Sentry renders it properly.
zerolog logs `error` (the message chain) + `caller`; Sentry owns the stack. One less dependency,
no half-working stacks in JSON logs.

If you later decide you want stacks in logs, add a minimal marshaler over your own
`medigo/internal/apperr` type rather than pulling in `pkg/errors`.

### 1.3 Request-scoped logger through PocketBase

PocketBase's request event is `core.RequestEvent` (core/event_request.go:19). It embeds
`router.Event`, which carries a `store.Store[string, any]` (`e.Get`/`e.Set`), and it exposes
`e.Request *http.Request` — so you have both a per-event bag and a `context.Context`.

```go
// internal/obs/httpmw.go
package obs

import (
	"context"

	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const RequestIDHeader = "X-Request-Id"

// RequestLogger is the FIRST middleware in the chain (lowest priority number).
// It mints a request id, derives a request-scoped zerolog.Logger, and puts it on
// the request context so every downstream layer can reach it without an interface change.
func RequestLogger(base zerolog.Logger) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       "medigoRequestLogger",
		Priority: apis.DefaultActivityLoggerMiddlewarePriority - 10, // run before PB's own
		Func: func(e *core.RequestEvent) error {
			start := time.Now()

			reqID := e.Request.Header.Get(RequestIDHeader)
			if reqID == "" {
				reqID = uuid.NewString()
			}
			e.Response.Header().Set(RequestIDHeader, reqID)

			lc := base.With().
				Str("request_id", reqID).
				Str("method", e.Request.Method).
				Str("route", routePattern(e.Request)) // bounded — see §4.2
			
			// OTel ids, if a span is already active (tracing middleware runs before this).
			if sc := trace.SpanContextFromContext(e.Request.Context()); sc.IsValid() {
				lc = lc.Str("trace_id", sc.TraceID().String()).
					Str("span_id", sc.SpanID().String())
			}

			logger := lc.Logger()
			ctx := logger.WithContext(e.Request.Context())
			ctx = context.WithValue(ctx, ctxKeyRequestID{}, reqID)
			e.Request = e.Request.WithContext(ctx)

			err := e.Next()

			// auth id is only known AFTER PB's loadAuthToken middleware ran
			ev := logger.Info()
			if err != nil {
				ev = logger.Error().Err(err)
			}
			if e.Auth != nil {
				ev = ev.Str("user_id", e.Auth.Id)
			}
			ev.Int("status", e.Status()).
				Dur("duration", time.Since(start)).
				Msg("http_request")

			return err
		},
	}
}
```

Downstream, anything with a `ctx` gets the logger for free:

```go
func (s *recordService) Create(ctx context.Context, in CreateRecordInput) (*Record, error) {
	log := zerolog.Ctx(ctx) // never nil; returns a disabled logger if unset
	log.Debug().Str("patient_id", in.PatientID).Msg("creating record")
	...
}
```

Two things make this safe:

- `zerolog.Ctx(ctx)` returns a disabled logger, never nil, when nothing is attached
  (`rs/zerolog/ctx.go`) — so tests and background jobs don't panic.
- Set `zerolog.DefaultContextLogger = &base` at boot so a ctx-less path still logs somewhere
  instead of silently dropping.

### 1.4 Context-carried vs injected logger — recommendation

**Recommendation: BOTH, with a hard rule on which is which.**

- **Injected (constructor / samber/do):** the *base* logger. A service that logs gets
  `zerolog.Logger` in its constructor and stores it as `s.log`. This is what runs in
  background jobs, cron, startup, and shutdown, where there is no request.
- **Context-carried:** the *per-request correlation fields only* (request_id, trace_id,
  span_id, user_id, patient_id). Retrieved with `zerolog.Ctx(ctx)` inside methods that already
  take a `ctx`.

Rule for the spec: **every service method takes `ctx context.Context` as its first parameter**
(this is Go idiom regardless of logging), and **logging inside a request path uses
`zerolog.Ctx(ctx)`; logging outside one uses the injected `s.log`.** Never pass a
`*zerolog.Logger` as a method parameter.

**SOLID justification:**

- **ISP** — the alternative (a `SetLogger(l)` method or a `logger` parameter on every method)
  fattens every service interface with a concern no caller cares about. `ctx` is already on the
  signature for cancellation and deadlines; correlation rides free.
- **DIP** — services depend on `context.Context` (stdlib) plus a concrete `zerolog.Logger`
  value injected at the composition root. They never reach for a package-level singleton
  (`log.Logger`), so tests substitute a logger writing to a buffer with zero ceremony.
- **OCP** — adding `patient_id` to every log line is a change in one middleware, not in
  N service interfaces. That is the whole point.
- **SRP** — the middleware owns correlation; the service owns domain logic. Neither knows the
  other's fields.

The counter-argument to context-carried loggers ("context is for cancellation, not
dependencies") is real but does not apply here: what's in the context is *request-scoped data*
(ids), which is exactly what `context.Context` is for. The logger is merely the carrier. The
*dependency* — the writer, the level, the encoder — is injected.

**Anti-pattern to ban explicitly:** a `Logger` field on a request DTO, or a service constructor
that takes `*core.RequestEvent`. Both leak transport into the domain.

---

## 2. The slog → zerolog bridge (the ugly part)

### 2.1 The adapter itself: solved, first-party, zero extra deps

**zerolog v1.35.1 ships a native `slog.Handler`.** File `rs/zerolog/slog.go`:

```go
func NewSlogHandler(logger Logger) *SlogHandler
var _ slog.Handler = (*SlogHandler)(nil)
```

It maps levels correctly (`slogToZerologLevel`), resolves `slog.LogValuer`, flattens groups with
dotted prefixes, type-switches `slog.KindString/Int64/Duration/Time/...` to the matching zerolog
typed field (no reflection on the hot path), special-cases `error` → `event.AnErr`, and
propagates the slog `ctx` onto the zerolog event via `event.Ctx(ctx)` so zerolog hooks can read
it with `Event.GetCtx()` — which matters for the Sentry hook in §3.

**So do NOT use `samber/slog-zerolog`.** It exists and works, but it is a third-party module
duplicating something now in zerolog proper, and it would be a second place to keep in sync.
Use `zerolog.NewSlogHandler`.

```go
func NewSlogBridge(l zerolog.Logger) *slog.Logger {
	return slog.New(zerolog.NewSlogHandler(l.With().Str("src", "pocketbase").Logger()))
}
```

### 2.2 PocketBase does NOT let you inject it. Here is the proof.

I read the source. All three plausible injection points are closed:

**(a) The config struct has no logger field.** `core/base.go:61`:

```go
type BaseAppConfig struct {
	DBConnect        DBConnectFunc
	DataDir          string
	EncryptionEnv    string
	QueryTimeout     time.Duration
	DataMaxOpenConns int
	DataMaxIdleConns int
	AuxMaxOpenConns  int
	AuxMaxIdleConns  int
	IsDev            bool
}
```

`pocketbase.Config` (pocketbase.go:46) is a strict subset plus banner/flag defaults. No `Logger`,
no `LogHandler`, no `LogWriter`.

**(b) `Logger()` is a plain getter over a private field with no setter.** `core/base.go:378`:

```go
func (app *BaseApp) Logger() *slog.Logger {
	if app.logger == nil {
		return slog.Default()
	}
	return app.logger
}
```

`grep -rn "SetLogger" core/` → nothing. `app.logger` is written in exactly one place.

**(c) The handler is constructed unconditionally inside an unexported bootstrap step.**
`core/base.go:1472-1536`, `(*BaseApp).initLogger()`:

```go
handler := logger.NewBatchHandler(logger.BatchOptions{
	Level:     getLoggerMinLevel(app),
	BatchSize: 200,
	BeforeAddFunc: func(ctx context.Context, log *logger.Log) bool {
		if app.IsDev() { printLog(log); ... }
		ticker.Reset(duration)
		return app.Settings().Logs.MaxDays > 0     // <-- the gate
	},
	WriteFunc: func(ctx context.Context, logs []*Log) error {
		if !app.IsBootstrapped() || app.Settings().Logs.MaxDays == 0 { return nil }
		app.AuxRunInTransaction(func(txApp App) error {
			model := &Log{}
			for _, l := range logs { ...; txApp.AuxSave(model) }
			return nil
		})
		return nil
	},
})
...
app.logger = slog.New(handler)   // core/base.go:1536
```

`printLog` (core/log_printer.go:27) is an unexported package var — not overridable from outside
`core`, and it only fires when `IsDev()`.

**Brutally honest verdict: PocketBase hardcodes its slog handler. There is no supported
injection point in v0.40.1.** The adapter is clean and first-party; getting PB's logs *into* it
is a workaround. Anyone who tells you otherwise hasn't read `initLogger`.

**The consequence, stated plainly:** in production (`IsDev()==false`), PocketBase's internal
logs go to exactly one destination — the `_logs` table in `auxiliary.db`. They never touch
stdout. If you set `Logs.MaxDays = 0` to disable the `_logs` collection (the locked decision),
`BeforeAddFunc` returns `false` and **every PocketBase internal log is silently dropped on the
floor.** Backup failures, mailer failures, cron errors, OAuth2 failures: gone. That is
unacceptable for a medical records app.

### 2.3 The workaround: intercept the `_logs` model write

`WriteFunc` persists each log via `txApp.AuxSave(model)` where `model` is a `*core.Log`.
`AuxSave` → `(*BaseApp).save` → `(*BaseApp).create` (core/db.go:270), which triggers
`app.OnModelCreate().Trigger(event, ...)` — a **tagged** hook. Tags for a model event come from
`baseModelEventData.Tags()` (core/events.go:30), which returns `[]string{e.Model.TableName()}`,
and `(*Log).TableName()` returns `"_logs"` (core/log_model.go:31).

And `hook.Hook.Trigger` (tools/hook/hook.go:~155) builds a chain where **a handler that returns
without calling `e.Next()` stops the chain and `Trigger` returns nil** — no DB write, no error.

So this is a supported-API interception, not a hack into private state:

```go
// internal/obs/pblog.go
package obs

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"
)

// BridgePBLogs diverts every PocketBase internal log record into zerolog and
// cancels the _logs table insert, leaving the collection permanently empty.
func BridgePBLogs(app core.App, base zerolog.Logger) {
	l := base.With().Str("src", "pocketbase").Logger()

	app.OnModelCreate("_logs").Bind(&hook.Handler[*core.ModelEvent]{
		Id:       "medigoLogBridge",
		Priority: -99999, // before anything else that might want the record
		Func: func(e *core.ModelEvent) error {
			pbLog, ok := e.Model.(*core.Log)
			if !ok {
				return e.Next() // not ours — let PB do its thing
			}

			ev := l.WithLevel(slogToZerolog(slog.Level(pbLog.Level)))
			for k, v := range pbLog.Data {
				ev = ev.Interface(k, v)
			}
			ev.Time("pb_ts", pbLog.Created.Time()).Msg(pbLog.Message)

			// Deliberately NOT calling e.Next(): the INSERT never happens.
			return nil
		},
	})
}

func slogToZerolog(l slog.Level) zerolog.Level {
	switch {
	case l < slog.LevelDebug:
		return zerolog.TraceLevel
	case l < slog.LevelInfo:
		return zerolog.DebugLevel
	case l < slog.LevelWarn:
		return zerolog.InfoLevel
	case l < slog.LevelError:
		return zerolog.WarnLevel
	default:
		return zerolog.ErrorLevel
	}
}
```

**This requires `Settings().Logs.MaxDays > 0`** — otherwise `BeforeAddFunc` drops the record
before `WriteFunc` ever runs and your hook never fires. So the configuration is:

```go
app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
	if err := e.Next(); err != nil { return err }

	s := e.App.Settings()
	s.Logs.MaxDays = 1      // keep the pipe OPEN so the bridge sees records
	s.Logs.MinLevel = -4    // slog.LevelDebug; let zerolog do the real filtering
	s.Logs.LogIP = false    // PII — never
	s.Logs.LogAuthId = false// PII — we attach user_id ourselves, scrubbed
	return e.App.Save(s)    // core/settings_query.go:41 uses exactly this
})
```

The `_logs` table stays empty because nothing is ever inserted. From the operator's point of
view the collection is disabled; from PB's point of view the pipe is open.

**Costs of this workaround — state them in the spec, don't hide them:**

1. **Latency.** Logs are batched: flushed at 200 records or on a 3s ticker
   (`core/base.go:1473`, `BatchSize: 200`). A PocketBase internal warning can appear in stdout
   up to ~3 seconds late. Your own application logs (via zerolog directly) are synchronous and
   unaffected. Only PB's internals are delayed.
2. **Lost source location.** `logger.BatchHandler.Handle` drops `slog.Record.PC`
   (tools/logger/batch_handler.go:~127 — it builds `Log{Time, Level, Message, Data}` only). You
   cannot recover the PB file:line. Acceptable — you get the message and attrs.
3. **No request correlation.** The flush runs on a background goroutine
   (`routine.FireAndForget`, core/base.go:1522), so there is no `ctx` and no request_id. Use
   workaround (b) below if you want that for the request path.
4. **Attribute fidelity.** Values arrive pre-flattened as `types.JSONMap[any]`; errors have
   already been stringified by `serializeLogError`. You lose typed fields; everything becomes
   `.Interface()`. Fine for PB internals.
5. **Upgrade risk.** If a future PB changes `Log`'s table name or moves the write off
   `AuxSave`, the bridge silently stops. **Add a boot-time self-test** (below).

**Self-test — make the silence loud:**

```go
// after bootstrap, prove the bridge is live
app.OnServe().BindFunc(func(e *core.ServeEvent) error {
	probe := make(chan struct{}, 1)
	app.OnModelCreate("_logs").Bind(&hook.Handler[*core.ModelEvent]{
		Id: "medigoLogBridgeProbe",
		Func: func(me *core.ModelEvent) error { select { case probe <- struct{}{}: default: }; return nil },
	})
	app.Logger().Info("medigo log bridge probe")
	go func() {
		select {
		case <-probe:
			app.OnModelCreate("_logs").Unbind("medigoLogBridgeProbe")
		case <-time.After(10 * time.Second):
			base.Error().Msg("PB->zerolog log bridge is NOT working; PocketBase internal logs are being lost")
		}
	}()
	return e.Next()
})
```

### 2.4 Second workaround: swap `e.App` per request (synchronous, correlated)

`core.RequestEvent.App` is an **exported field of interface type `core.App`**
(core/event_request.go:20), and PB's own request-path code calls `e.App.Logger()` —
`apis/middlewares.go:201` (`loadAuthToken failure`), `apis/middlewares.go:460-462`
(`logRequest`), `apis/file.go:182`, `apis/record_auth_otp_request.go:77`, `apis/realtime.go`
per-connection debug. So a top-priority middleware can substitute a wrapper:

```go
type appWithLogger struct {
	core.App              // interface embedding promotes the unexported methods too
	logger   *slog.Logger
}

func (a *appWithLogger) Logger() *slog.Logger { return a.logger }

// inside the request-logger middleware, before e.Next():
e.App = &appWithLogger{App: e.App, logger: slog.New(zerolog.NewSlogHandler(logger))}
```

This compiles from outside `core` even though `core.App` has unexported methods
(`onFilesystemNewWriter()`, `onFilesystemDelete()` at core/app.go:1273/1277), because embedding
an *interface value* promotes every method, exported or not.

Result: PB's request-scoped internal logs land in zerolog **synchronously, with request_id and
trace_id attached**. It does not cover background work (cron, backups, realtime broadcast
goroutines that capture `app` in a closure — `apis/realtime.go:347-737`), which is what §2.3 is
for.

**Recommendation: ship §2.3 always; add §2.4 only if request-correlated PB internals turn out to
matter in practice.** §2.3 alone is the KISS answer.

### 2.5 Also disable PocketBase's own request activity logging

PB registers `activityLogger()` on every route (`apis/base.go:30`) which, when
`Logs.MaxDays != 0`, emits an Info/Error per request through `app.Logger()`
(`apis/middlewares.go:460-462`). Since we keep `MaxDays = 1` for the bridge, that would
double-log every request (once from PB, ~3s late, with PB's field names; once from our own
middleware, immediately, with trace ids).

Kill PB's version — `RouterGroup.Unbind` (tools/router/group.go:78) both removes it and adds it
to an exclusion set so sub-groups don't re-add it:

```go
app.OnServe().BindFunc(func(e *core.ServeEvent) error {
	e.Router.Unbind(apis.DefaultActivityLoggerMiddlewareId) // "pbActivityLogger"
	e.Router.Bind(obs.RequestLogger(base))                  // ours
	return e.Next()
})
```

### 2.6 Lock down the `_logs` HTTP API

`apis.NewRouter` calls `bindLogsApi(app, apiGroup)` (apis/base.go:44), exposing
`GET /api/logs` and `GET /api/logs/stats` to superusers. The table is empty, so these are
harmless but confusing. Either leave them (they return zero rows) or shadow them with a 410:

```go
e.Router.GET("/api/logs", func(e *core.RequestEvent) error {
	return e.JSON(http.StatusGone, map[string]string{"message": "logs are shipped to stdout"})
})
```

Not load-bearing. Do it for operator sanity, not security.

---

## 3. Sentry

Verified against `getsentry/sentry-go@v0.48.0` (v0.49.0 is current; the `otel` submodule is a
separate module).

### 3.1 Init — medical-records posture

The v0.48+ SDK replaced the blunt `SendDefaultPII` flag with a structured `DataCollection`
(`data_collection.go`). Use it — it is the difference between "we asked the SDK not to send PII"
and "the SDK is structurally incapable of sending it".

```go
// internal/obs/sentry.go
func InitSentry(cfg SentryConfig, release string) (func(), error) {
	if cfg.DSN == "" {
		return func() {}, nil // disabled: no DSN, no client
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          release,
		AttachStacktrace: true,
		SampleRate:       1.0,

		// TRACING IS OWNED BY OTEL. See §5.5.
		EnableTracing:    false,
		TracesSampleRate: 0.0,

		// Structural PII lockdown.
		DataCollection: &sentry.DataCollection{
			UserInfo: sentry.Set(false),                                    // no auto user.*
			Cookies:  &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			HTTPBodies: []sentry.BodyType{},                                // NEVER any body
			HTTPHeaders: &sentry.HeaderCollectionConfig{
				Request: &sentry.KeyValueCollectionBehavior{
					Mode:  sentry.CollectionAllowList,
					Terms: []string{"content-type", "user-agent", "x-request-id", "accept"},
				},
				Response: &sentry.KeyValueCollectionBehavior{
					Mode:  sentry.CollectionAllowList,
					Terms: []string{"content-type", "x-request-id"},
				},
			},
			QueryParams: &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
		},

		BeforeSend:            scrubEvent,
		BeforeSendTransaction: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event { return nil },
		BeforeBreadcrumb:      scrubBreadcrumb,

		Integrations: func(in []sentry.Integration) []sentry.Integration {
			// sentryotel links Sentry errors to the active OTel span. See §5.5.
			return append(in, sentryotel.NewOtelIntegration())
		},
	})
	if err != nil {
		return nil, err
	}
	return func() { sentry.Flush(5 * time.Second) }, nil
}
```

Note `HTTPBodies: []sentry.BodyType{}` — an **empty non-nil slice**. `resolveDataCollection`
only substitutes `allBodyTypes()` when the field is `nil` (data_collection.go:~172). An empty
slice means "collect nothing". Getting this wrong ships patient JSON to Sentry.

### 3.2 What must NEVER reach Sentry — the MediGo denylist

This is a medical records app. Treat the Sentry payload as a hostile egress channel.

**Category A — categorically forbidden, no exceptions:**

| Data | Where it leaks from |
|---|---|
| Patient names, DOB, addresses, phone, email, national/insurance IDs | request bodies, DTO structs in `Extra`, error messages, `User.Email/Name` |
| Any clinical content: conditions, medications, allergies, procedures, immunisations, vitals, lab values, practitioner notes, encounter text | request/response bodies, validation error messages that echo field values |
| File contents and original filenames of attachments | multipart bodies, `filesystem.File` in error wrapping |
| Auth tokens, PB record tokens, OAuth2 codes/state, password reset & OTP tokens, session cookies | `Authorization` header, cookies, query params, URLs |
| Raw SQL with bound values, and any `dbx` error carrying a query string | driver errors bubbled to `CaptureException` |
| Backup archives / paths inside `pb_data` beyond the base dir name | backup failures |
| Full request URLs containing record ids that are themselves identifiers | `Request.URL`, `Transaction` |

**Category B — allowed, because it is how you debug:**

opaque PocketBase record ids (`user_id`, `patient_id`, `record_id` — 15-char random, not
derived from PII), the route *pattern* (not the concrete path), HTTP method and status,
`request_id`, `trace_id`/`span_id`, release, environment, Go stack frames, error *types*.

### 3.3 Enforcing it — `BeforeSend`

Belt and braces. `DataCollection` handles the SDK's own auto-population; `BeforeSend` handles
everything *you* might accidentally attach, and is the last gate before the wire.

```go
var (
	// Anything whose KEY smells like PII gets dropped wholesale.
	deniedKeyRe = regexp.MustCompile(`(?i)(name|dob|birth|ssn|nhs|insur|address|phone|email|` +
		`note|comment|desc|diagnos|condition|medicat|allerg|procedur|immunis|immuniz|vital|` +
		`result|value|token|secret|password|passwd|authoriz|cookie|otp|code|file|attachment|content)`)

	// Anything that LOOKS like a bearer/JWT/long hex blob gets masked wherever it appears.
	tokenRe = regexp.MustCompile(`(?i)(bearer\s+)?[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)
	longHex = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)

	// PB path segments that are ids -> collapse to a placeholder.
	pbIDInPath = regexp.MustCompile(`/[a-z0-9]{15}(/|$)`)
)

func scrubString(s string) string {
	s = tokenRe.ReplaceAllString(s, "[redacted-token]")
	s = longHex.ReplaceAllString(s, "[redacted-hex]")
	return s
}

func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	// 1. User: opaque id ONLY. No email, no username, no name, no IP.
	event.User = sentry.User{ID: event.User.ID}

	// 2. Request: keep method + normalised URL; nuke body, cookies, query, env.
	if r := event.Request; r != nil {
		r.Data = ""
		r.Cookies = ""
		r.QueryString = ""
		r.Env = nil
		r.URL = pbIDInPath.ReplaceAllString(r.URL, "/{id}$1")
		for k := range r.Headers {
			if deniedKeyRe.MatchString(k) || strings.EqualFold(k, "authorization") {
				delete(r.Headers, k)
			}
		}
	}

	// 3. Messages and exception values: mask token-shaped substrings.
	event.Message = scrubString(event.Message)
	for i := range event.Exception {
		event.Exception[i].Value = scrubString(event.Exception[i].Value)
		// Local variables in stack frames can contain a whole patient struct.
		if st := event.Exception[i].Stacktrace; st != nil {
			for j := range st.Frames {
				st.Frames[j].Vars = nil
			}
		}
	}

	// 4. Tags and contexts: drop denied keys, scrub the rest.
	for k, v := range event.Tags {
		if deniedKeyRe.MatchString(k) {
			delete(event.Tags, k)
			continue
		}
		event.Tags[k] = scrubString(v)
	}
	for ctxName, ctxVal := range event.Contexts {
		if ctxName == "trace" || ctxName == "runtime" || ctxName == "os" || ctxName == "device" {
			continue
		}
		for k := range ctxVal {
			if deniedKeyRe.MatchString(k) {
				delete(ctxVal, k)
			}
		}
	}

	// 5. Breadcrumbs get the same treatment.
	for _, b := range event.Breadcrumbs {
		b.Message = scrubString(b.Message)
		for k := range b.Data {
			if deniedKeyRe.MatchString(k) {
				delete(b.Data, k)
			}
		}
	}

	// 6. Transaction name must be the ROUTE PATTERN, never a concrete path.
	event.Transaction = pbIDInPath.ReplaceAllString(event.Transaction, "/{id}$1")

	return event
}
```

**Two rules the spec must encode, because `BeforeSend` is a net, not a wall:**

1. **Never put a domain struct in `Extra`, a tag, or an error message.** Log ids, not values.
   `fmt.Errorf("invalid record %q for patient %s", rec.Description, p.Name)` is a HIPAA
   incident waiting to happen. Enforce with a lint rule and a code-review checklist item.
2. **Test the scrubber.** A `testify` table test that feeds a fabricated event containing a
   fake patient name, a JWT, a DOB, and a base64 file blob, and asserts none of them survive
   `scrubEvent`. This test is a compliance artefact — make it a required gate.

Also worth a `sentry.Init` guard: refuse to boot with `SENTRY_DSN` set and
`MEDIGO_ENV=production` unless `SENTRY_SCRUB_VERIFIED=true` (a build-time constant set by CI
after the scrubber tests pass). Cheap, and it makes the invariant structural.

### 3.4 Panic recovery middleware on PB's router

PB already has `panicRecover()` bound by default (apis/base.go:31, priority
`DefaultPanicRecoverMiddlewarePriority`), which converts a panic into a 500. Bind **before** it
(lower priority) so you see the panic first, report it, then re-panic and let PB produce the
response:

```go
func SentryRecover() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       "medigoSentryRecover",
		Priority: apis.DefaultPanicRecoverMiddlewarePriority - 1,
		Func: func(e *core.RequestEvent) (err error) {
			hub := sentry.CurrentHub().Clone()
			hub.Scope().SetRequest(e.Request) // DataCollection already gates what's kept
			hub.Scope().SetTag("route", routePattern(e.Request))
			hub.Scope().SetTag("request_id", RequestIDFrom(e.Request.Context()))
			e.Request = e.Request.WithContext(sentry.SetHubOnContext(e.Request.Context(), hub))

			defer func() {
				if r := recover(); r != nil {
					hub.RecoverWithContext(e.Request.Context(), r)
					hub.Flush(2 * time.Second)
					panic(r) // let PB's panicRecover write the 500
				}
			}()

			err = e.Next()

			// Report 5xx application errors that were handled, not panicked.
			if err != nil && router.ToApiError(err).Status >= 500 {
				hub.CaptureException(err)
			}
			return err
		},
	}
}
```

Attach auth *after* `loadAuthToken` has run — inside the same middleware, post-`e.Next()`, do
`hub.Scope().SetUser(sentry.User{ID: e.Auth.Id})` before capturing. `scrubEvent` will strip
anything else regardless.

### 3.5 The zerolog → Sentry hook, without double-reporting

This is the classic footgun: a zerolog hook on Error + an explicit `CaptureException` in the
middleware = two Sentry issues for one failure.

**Rule: exactly one reporter per failure, chosen by a marker on the event.**

```go
// sentryHook forwards Error/Fatal/Panic zerolog events to Sentry, unless the
// call site opted out with .Bool("sentry_skip", true) — used by code paths that
// already reported (the recover middleware, the service that returned the error).
type sentryHook struct{ minLevel zerolog.Level }

func (h sentryHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level < h.minLevel || level == zerolog.NoLevel {
		return
	}
	if e.GetCtx() != nil && skipSentry(e.GetCtx()) {
		return
	}

	// Prefer the hub already on the event's context (carries route/request_id/user).
	hub := sentry.CurrentHub()
	if ctx := e.GetCtx(); ctx != nil {
		if h := sentry.GetHubFromContext(ctx); h != nil {
			hub = h
		}
	}

	ev := sentry.NewEvent()
	ev.Level = sentry.LevelError
	if level >= zerolog.FatalLevel {
		ev.Level = sentry.LevelFatal
	}
	ev.Message = msg
	hub.CaptureEvent(ev)
}
```

**Design decision that actually prevents duplicates — pick ONE of these and write it into the
spec. Do not do both.**

- **Option A (recommended): the zerolog hook is the ONLY reporter for handled errors.** The
  Sentry middleware reports *panics only*. Every service that fails logs
  `zerolog.Ctx(ctx).Error().Err(err).Msg("...")` exactly once at the boundary that decides the
  request has failed (the HTTP handler), and the hook ships it. Services below that log at
  `Warn` or `Debug`, never `Error`. This gives you one Sentry event per failed request, already
  carrying request_id/trace_id from the ctx-attached hub.
- **Option B: the middleware is the only reporter, and the zerolog hook is not installed at
  all.** Simpler, but you lose Sentry visibility for errors in cron jobs and background
  goroutines that never touch the router.

**Recommendation: Option A, plus the middleware reporting panics only** (a panic never becomes
a `zerolog.Error()`, so there is no overlap by construction). The single rule for developers is:

> **`log.Error()` means "this became a Sentry issue". If you are not the boundary that owns the
> failure, use `Warn` and return the error.**

That rule is enforceable in review and eliminates duplicate reporting without any deduplication
machinery. Add `sentry_skip` as the escape hatch for the rare loud-but-not-alertable error.

Install it as:

```go
base = base.Hook(sentryHook{minLevel: zerolog.ErrorLevel})
```

Because `zerolog.SlogHandler.Handle` calls `event.Ctx(ctx)`, PocketBase internals bridged
through §2.4 also carry a hub. The §2.3 batch bridge has no ctx, so its Errors fall back to
`sentry.CurrentHub()` — still reported, just without request correlation. Correct behaviour.

### 3.6 Sentry tracing vs OTel — how they coexist

**Current upstream reality (verified Aug 2026):** the old `sentry-go/otel` span-processor bridge
(`sentryotel.NewSentrySpanProcessor` / `NewSentryPropagator`) is **gone**. The `otel` submodule
now contains `linking_integration.go` and an `otlp/` subpackage:

- `sentryotel.NewOtelIntegration()` — a Sentry *integration* that "links captured Sentry errors,
  logs, and metrics to the active OpenTelemetry trace when a context carrying an active OTel span
  is used." It resolves trace/span ids from the OTel context and stamps them on Sentry events.
  It creates **no spans**.
- `sentryotlp.NewTraceExporter(ctx, dsn string, opts ...Option) (sdktrace.SpanExporter, error)` —
  an OTLP/HTTP span exporter that sends OTel spans **to Sentry**, deriving endpoint/headers from
  the DSN.

**The architecture that cannot double-report, and the one to adopt:**

1. **OpenTelemetry is the only thing that creates spans.** `EnableTracing: false` and
   `TracesSampleRate: 0` in `ClientOptions`. Sentry's own `StartTransaction`/`StartSpan` API is
   never called in MediGo code.
2. `BeforeSendTransaction` returns `nil` — a hard stop. If a dependency ever starts a Sentry
   transaction behind your back, it dies at the gate. (This is the belt to the braces.)
3. Add `sentryotel.NewOtelIntegration()` so every Sentry error carries the `trace_id`/`span_id`
   of the OTel span it happened in. Clicking from a Sentry issue to the trace works.
4. **Where spans go is a config choice, not an architecture choice.** Either
   `otlptracehttp.New(...)` to your collector (recommended for self-hosted MediGo), *or*
   `sentryotlp.NewTraceExporter(ctx, dsn)` to send them to Sentry, *or* both as two batch
   processors on the same `TracerProvider`. Either way the spans are produced exactly once, by
   OTel.

There is no duplicate-span scenario left, because there is only one span producer. The failure
mode to avoid is the legacy one: enabling `EnableTracing: true` *and* an OTel SDK, which in the
old bridge world produced two parallel span trees. Don't.

---

## 4. Prometheus

`prometheus/client_golang@v1.24.1`.

### 4.1 The metric set a records app actually needs

Keep it small enough that every metric has an owner and a dashboard panel. Anything else is
cardinality debt.

```go
// internal/obs/metrics.go
package obs

type Metrics struct {
	reg *prometheus.Registry

	// --- HTTP (RED) ---
	HTTPDuration *prometheus.HistogramVec // route, method, status
	HTTPInFlight prometheus.Gauge
	HTTPReqBytes  *prometheus.HistogramVec // route, method
	HTTPRespBytes *prometheus.HistogramVec // route, method

	// --- Persistence ---
	DBQueryDuration *prometheus.HistogramVec // op(select|insert|update|delete|tx), table
	DBTxDuration    prometheus.Histogram
	DBBusyRetries   prometheus.Counter // SQLITE_BUSY — the canary for a WAL app

	// --- Realtime ---
	SSEStreams   *prometheus.GaugeVec   // kind(datastar|pb_realtime)
	SSEMessages  *prometheus.CounterVec // kind
	SSEDropped   *prometheus.CounterVec // kind, reason(slow_client|closed)

	// --- Business ---
	RecordsCreated *prometheus.CounterVec // record_type (13 values, bounded)
	RecordsDeleted *prometheus.CounterVec // record_type, mode(soft|hard)
	LabResults     *prometheus.CounterVec // outcome(normal|abnormal|critical|unknown)
	AuthAttempts   *prometheus.CounterVec // method(password|oauth2|otp), outcome(success|failure|locked)
	AuthFailures   *prometheus.CounterVec // reason(bad_password|unknown_user|mfa|rate_limited)
	SharesGranted  *prometheus.CounterVec // scope(patient|family_history), via(invite|direct)
	SharesRevoked  *prometheus.CounterVec // scope
	FilesStored    prometheus.Counter
	FileBytesTotal prometheus.Counter     // cumulative bytes written
	FileBytesGauge prometheus.Gauge       // current storage footprint, refreshed by a cron
	ExportsRun     *prometheus.CounterVec // format(pdf|csv|json), outcome
	BackupsRun     *prometheus.CounterVec // outcome(success|failure)
	BackupAgeSecs  prometheus.Gauge       // seconds since last successful backup

	// --- Build info ---
	BuildInfo *prometheus.GaugeVec // version, commit, go_version
}

func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	f := promauto.With(reg)

	m := &Metrics{
		reg: reg,
		HTTPDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "medigo", Subsystem: "http", Name: "request_duration_seconds",
			Help:    "HTTP request latency by route pattern.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			// Native histograms: cheap high-resolution latency without bucket tuning.
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"route", "method", "status"}),
		HTTPInFlight: f.NewGauge(prometheus.GaugeOpts{
			Namespace: "medigo", Subsystem: "http", Name: "requests_in_flight",
			Help: "In-flight HTTP requests.",
		}),
		RecordsCreated: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: "medigo", Subsystem: "records", Name: "created_total",
			Help: "Clinical records created, by type.",
		}, []string{"record_type"}),
		AuthFailures: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: "medigo", Subsystem: "auth", Name: "failures_total",
			Help: "Failed authentication attempts, by reason.",
		}, []string{"reason"}),
		// ... rest elided, same shape
	}
	return m
}

func (m *Metrics) Registry() *prometheus.Registry { return m.reg }
```

**Deliberately NOT metrics:**

- Anything labelled by `patient_id`, `user_id`, `record_id`, filename, or tag name. That is
  unbounded and it is also PII in your monitoring system, which is a much worse problem than a
  cardinality bill.
- `status` as the raw code is fine (bounded set); do **not** add a `status_class` label — derive
  it in PromQL.
- `LabResults` by test name would be unbounded (the standardized test catalog grows). Label by
  outcome only; if you need per-test insight, that's an analytics query against SQLite, not a
  time series.

### 4.2 Route-label cardinality — solved by `http.Request.Pattern`

This is the question everyone gets wrong, and PocketBase happens to make it trivial.

`tools/router/router.go:loadMux` builds patterns and registers them on a **standard Go
`http.ServeMux`**:

```go
var pattern string
if v.Method != "" { pattern = v.Method + " " }
for _, p := range parents { pattern += p.Prefix; ... }
pattern += group.Prefix
pattern += v.Path
mux.HandleFunc(pattern, func(resp http.ResponseWriter, req *http.Request) { ... })
```

Since **Go 1.23**, `ServeMux.ServeHTTP` sets `Request.Pattern` to the matched pattern before
dispatching (go1.23 release notes: *"For inbound requests, the new `Request.Pattern` field
contains the `ServeMux` pattern (if any) that matched the request."*). MediGo targets Go 1.26,
so this is available unconditionally.

```go
// routePattern returns a BOUNDED label: the ServeMux pattern with the method prefix stripped.
// "GET /api/v1/patients/{patientId}/records" -> "/api/v1/patients/{patientId}/records"
func routePattern(r *http.Request) string {
	p := r.Pattern
	if p == "" {
		return "unmatched"
	}
	if i := strings.IndexByte(p, ' '); i >= 0 {
		p = p[i+1:]
	}
	if p == "" {
		return "unmatched"
	}
	return p
}
```

Why this is bounded: the label space is exactly the set of registered routes (~80-120 for
MediGo + PB's ~40 built-ins), which is a compile-time constant. Path params never appear —
`{patientId}` stays literal. And unmatched requests can't explode it either, because
`Router.BuildMux` (tools/router/router.go:65) installs a catch-all:

```go
if !r.HasRoute("", "/") {
	r.Route("", "/", func(e T) error { return NewNotFoundError("", nil) })
}
```

so every 404 matches `/` and yields the single label `"/"`. Scanning for `/wp-admin.php` does
not create a new time series.

**Belt-and-braces guard** (because a future refactor could register a route with a `{path...}`
wildcard that someone then "improves"):

```go
type routeLabeller struct {
	mu    sync.RWMutex
	known map[string]struct{} // seeded from the router at OnServe
}

func (rl *routeLabeller) label(r *http.Request) string {
	p := routePattern(r)
	rl.mu.RLock()
	_, ok := rl.known[p]
	rl.mu.RUnlock()
	if !ok {
		return "other" // never mint a new series at request time
	}
	return p
}
```

Seed `known` in `OnServe` by walking the router's registered routes, or simply by hardcoding the
allowed set in a generated file. The `"other"` bucket makes a misconfiguration visible instead
of expensive.

### 4.3 The middleware

```go
func MetricsMiddleware(m *Metrics, rl *routeLabeller) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       "medigoMetrics",
		Priority: apis.DefaultActivityLoggerMiddlewarePriority - 20,
		Func: func(e *core.RequestEvent) error {
			route := rl.label(e.Request)
			start := time.Now()

			m.HTTPInFlight.Inc()
			defer m.HTTPInFlight.Dec()

			err := e.Next()

			status := e.Status() // router.Event.Status() unwraps via RWUnwrapper
			if status == 0 {
				status = router.ToApiError(err).Status
			}
			m.HTTPDuration.WithLabelValues(
				route, e.Request.Method, strconv.Itoa(status),
			).Observe(time.Since(start).Seconds())

			return err
		},
	}
}
```

`e.Status()` is safe here: `router.Event.Status()` → `getStatus(rw)` walks `RWUnwrapper`
(tools/router/router.go:329-340), so it survives any well-behaved ResponseWriter wrapping.

### 4.4 DB query metrics

Two sources, use both:

1. **`sql.DBStats`** — pool saturation. Register per PB database (see §5.3 for why there are
   four `*sql.DB` handles). `collectors.NewDBStatsCollector(db, "data")` from
   `prometheus/client_golang/prometheus/collectors`. You need a handle on the `*sql.DB`, which
   you have because *you* wrote `DBConnect`.
2. **otelsql's query metrics**, bridged into the same registry (§5.6). This gives you
   `db.client.operation.duration` without hand-instrumenting dbx.

Do **not** try to hook `dbx.QueryLogFunc`/`ExecLogFunc` for metrics: PB overwrites both in dev
mode (core/base.go:1260-1267) and they carry the raw SQL string, which is a labelling trap.

### 4.5 Exposing and protecting `/metrics`

**Recommendation: a separate HTTP server on its own port, bound to `127.0.0.1` by default.**

```go
func StartMetricsServer(cfg MetricsConfig, m *Metrics, log zerolog.Logger) (*http.Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{
		ErrorHandling:     promhttp.HTTPErrorOnError,
		EnableOpenMetrics: true, // required for native histograms + exemplars
		MaxRequestsInFlight: 3,
		Timeout:             10 * time.Second,
	}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	srv := &http.Server{
		Addr:              cfg.Addr, // default "127.0.0.1:9090"
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("metrics server failed")
		}
	}()
	return srv, nil
}
```

**Why a separate port and not a PB route:**

- **It is not part of the API surface.** `/metrics` on the main router means it inherits CORS,
  rate limiting, the activity logger, security headers, and the auth middleware chain — all
  irrelevant, all things that can accidentally expose or accidentally break it.
- **It survives the app being unhealthy.** If PB's bootstrap fails or the main router is
  wedged, you still want to scrape. A separate `http.Server` with its own mux gives you that.
- **Binding to localhost is a real control, not a config toggle.** In the container, Prometheus
  scrapes over the pod network only if you deliberately set `MEDIGO_METRICS_ADDR=0.0.0.0:9090`.
  Default-deny.
- **Superuser auth on `/metrics` is the wrong shape.** Prometheus scrape configs with a PB
  superuser token is operationally awful (token rotation, a superuser credential sitting in
  your monitoring config, and PB tokens expire). Don't.

If you are forced onto a single port (some PaaS), the fallback is a PB route with
`apis.RequireSuperuserAuth()` **plus** a static bearer token from
`MEDIGO_METRICS_TOKEN` compared with `subtle.ConstantTimeCompare`. Second-best, and say so.

**Metrics carry no PHI**, so the exposure risk is enumeration of usage patterns, not patient
data — but a self-hosted medical app's operator will still be asked about it. Localhost default
is the answer that ends the conversation.

---

## 5. OpenTelemetry

### 5.1 SDK bootstrap

```go
// internal/obs/otel.go
func InitTracing(ctx context.Context, cfg OTelConfig, release string) (func(context.Context) error, error) {
	if !cfg.Enabled {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName("medigo"),
			semconv.ServiceVersion(release),
			semconv.DeploymentEnvironmentName(cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithHeaders(cfg.Headers),
		func() otlptracehttp.Option {
			if cfg.Insecure { return otlptracehttp.WithInsecure() }
			return otlptracehttp.WithTLSClientConfig(nil)
		}(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp, sdktrace.WithMaxQueueSize(4096)),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.SampleRatio), // 1.0 for a self-hosted single-tenant app
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Warn().Err(err).Msg("otel internal error")
	}))

	return tp.Shutdown, nil
}
```

### 5.2 Tracing middleware for PB's router

**Do not use `otelhttp.NewMiddleware` via `apis.WrapStdMiddleware`.** It *would* work —
`WrapStdMiddleware` (apis/base.go:69) reassigns `e.Request`/`e.Response` inside the std handler,
and PB's `getStatus`/`getWritten` unwrap through `RWUnwrapper` — but otelhttp wraps the
ResponseWriter with its own type, and you are now betting the correctness of PB's
"was a response already written?" logic (`router.ErrorHandler`, tools/router/router.go:164) on a
third-party wrapper's `Unwrap()` behaviour. For 40 lines of your own middleware, don't take
that bet. Also, `r.Pattern` already gives you the span name that `otelhttp.WithRouteTag` exists
to provide.

```go
func TracingMiddleware(rl *routeLabeller) *hook.Handler[*core.RequestEvent] {
	tracer := otel.Tracer("medigo/http")
	prop := otel.GetTextMapPropagator()

	return &hook.Handler[*core.RequestEvent]{
		Id:       "medigoTracing",
		Priority: apis.DefaultActivityLoggerMiddlewarePriority - 30, // outermost of ours
		Func: func(e *core.RequestEvent) error {
			route := rl.label(e.Request)

			ctx := prop.Extract(e.Request.Context(),
				propagation.HeaderCarrier(e.Request.Header))

			ctx, span := tracer.Start(ctx,
				e.Request.Method+" "+route,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(e.Request.Method),
					semconv.HTTPRoute(route),
					semconv.URLScheme(scheme(e.Request)),
					semconv.UserAgentOriginal(e.Request.UserAgent()),
					// NOTE: no URLFull / URLQuery — the path can carry ids and
					// the query can carry filter values. Route only.
				),
			)
			defer span.End()

			e.Request = e.Request.WithContext(ctx)

			err := e.Next()

			status := e.Status()
			if status == 0 {
				status = router.ToApiError(err).Status
			}
			span.SetAttributes(semconv.HTTPResponseStatusCode(status))
			if e.Auth != nil {
				span.SetAttributes(attribute.String("medigo.user_id", e.Auth.Id))
			}
			if err != nil {
				span.RecordError(err)
				if status >= 500 {
					span.SetStatus(codes.Error, http.StatusText(status))
				}
			}
			return err
		},
	}
}
```

Middleware ordering (PB sorts by `Priority` ascending, `hook.Hook.Bind`):

```
medigoTracing        (activityLogger - 30)  <- creates the span, must be outermost
medigoMetrics        (activityLogger - 20)
medigoRequestLogger  (activityLogger - 10)  <- reads trace_id from ctx
medigoSentryRecover  (panicRecover - 1)
pbPanicRecover / pbRateLimit / pbLoadAuthToken / ...
```

### 5.3 otelsql against PocketBase — YES, and it is clean

**Injection point confirmed.** `pocketbase.Config.DBConnect` (pocketbase.go:62) is
`core.DBConnectFunc = func(dbPath string) (*dbx.DB, error)` (core/base.go:59), passed straight
into `core.NewBaseApp` and used by `initDataDB` and `initAuxDB` (core/base.go:1240, 1248, 1302,
1310).

**The default it replaces** (core/db_connect.go:10):

```go
func DefaultDBConnect(dbPath string) (*dbx.DB, error) {
	pragmas := "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=journal_size_limit(200000000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=temp_store(MEMORY)&_pragma=cache_size(-32000)&_defensive=1"
	db, err := dbx.Open("sqlite", dbPath+pragmas)
	...
}
```

**The mechanism.** `dbx.Open` is just `sql.Open` + `dbx.NewFromDB(sqlDB, driverName)`
(dbx/db.go:100). And `dbx.NewFromDB` is exported. So you swap `sql.Open` for `otelsql.Open`
(which returns a `*sql.DB` wrapping the driver) and hand the result to `dbx.NewFromDB`:

```go
// internal/obs/db.go
package obs

import (
	"database/sql"
	"path/filepath"

	"github.com/XSAM/otelsql"
	"github.com/pocketbase/dbx"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

// KEEP IN SYNC with core.DefaultDBConnect (pocketbase v0.40.1, core/db_connect.go).
// Verified against v0.40.1 — re-check on every PocketBase upgrade.
const pbPragmas = "?_pragma=busy_timeout(10000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=journal_size_limit(200000000)" +
	"&_pragma=synchronous(NORMAL)" +
	"&_pragma=foreign_keys(ON)" +
	"&_pragma=temp_store(MEMORY)" +
	"&_pragma=cache_size(-32000)" +
	"&_defensive=1"

// NewInstrumentedDBConnect returns a core.DBConnectFunc that traces every query.
func NewInstrumentedDBConnect(reg func(*sql.DB, string)) func(dbPath string) (*dbx.DB, error) {
	return func(dbPath string) (*dbx.DB, error) {
		dbName := filepath.Base(dbPath) // "data.db" | "auxiliary.db"

		sqlDB, err := otelsql.Open("sqlite", dbPath+pbPragmas,
			otelsql.WithAttributes(
				semconv.DBSystemNameSqlite,
				semconv.DBNamespace(dbName),
			),
			otelsql.WithSpanOptions(otelsql.SpanOptions{
				// SQL text is a PHI risk in a medical app: WHERE clauses can
				// contain values. Statements are parameterised by dbx, but do
				// not take the chance.
				DisableQuery:         true,
				OmitConnResetSession: true,
				OmitConnPrepare:      true,
				OmitRows:             true,
				OmitConnectorConnect: true,
			}),
			otelsql.WithSQLCommenter(false),
		)
		if err != nil {
			return nil, err
		}

		if reg != nil {
			reg(sqlDB, dbName) // hand the *sql.DB to Prometheus for DBStats
		}

		// CRITICAL: pass the LOGICAL driver name "sqlite", not otelsql's
		// generated wrapper name. dbx picks its query builder from this string
		// (dbx.BuilderFuncMap, dbx/db.go:75); an unknown name silently falls
		// back to NewStandardBuilder and breaks PocketBase's SQL quoting.
		return dbx.NewFromDB(sqlDB, "sqlite"), nil
	}
}
```

Wire it up:

```go
app := pocketbase.NewWithConfig(pocketbase.Config{
	DefaultDataDir: cfg.DataDir,
	DBConnect:      obs.NewInstrumentedDBConnect(metrics.RegisterDBStats),
})
```

**Why `otelsql.Open` and not `otelsql.Register`.** `Register(driverName)` (otelsql/sql.go) does:

```go
db, err := sql.Open(driverName, "")   // <-- EMPTY DSN
dri := db.Driver()
...
sql.Register(driverName+"-otelsql-"+n, newDriver(dri, newConfig(options...)))
```

Three problems for us: it opens with an empty DSN (works with `modernc.org/sqlite` today, but
it's an unnecessary bet), it registers a global driver name that you must not register twice —
and **`DBConnect` is called four times** (data concurrent + nonconcurrent, aux concurrent +
nonconcurrent), so a naive `Register` inside it would burn four driver slots or panic. It also
caps at `maxDriverSlot`. `otelsql.Open` has none of these problems: it wraps per-call, returns a
`*sql.DB` directly, and is idempotent by construction.

**Caveats to write into the spec:**

1. **The pragma string is copy-pasted from PB internals.** It is a local variable inside
   `DefaultDBConnect`, not an exported constant — there is no way to reference it. Add a CI
   check that greps `core/db_connect.go` in the module cache and fails if the string drifts,
   or at minimum a `// KEEP IN SYNC` comment plus a checklist item on PB upgrades. Getting this
   wrong silently disables WAL or foreign keys.
2. **Four `*sql.DB` handles, not one.** Attribute them (`db.namespace = data.db|auxiliary.db`)
   or your DB span/metric attributes merge nonsensically. You cannot distinguish
   concurrent from nonconcurrent from inside `DBConnect` — the function only receives a path.
   If you need that split, keep a per-path call counter; honestly, `db.namespace` is enough.
3. **Span volume.** SQLite queries are microseconds and PocketBase issues a *lot* of them
   (every record read hits `_collections` lookups, hooks, etc.). At `SampleRatio: 1.0` you will
   generate large traces. That is fine for a single-tenant self-hosted app and genuinely useful
   for debugging N+1 patterns in the record services — but set
   `OmitRows: true` (done above) or you get a span per `Rows.Next()`.
4. **Context propagation is automatic *only if* the query carries a ctx.** See §5.4.

### 5.4 Propagating context through PocketBase

This is where the trace breaks if you're not careful. Three rules:

**(a) Use the `*WithContext` variants everywhere in your own code.** `core.App` exposes
`SaveWithContext`, `SaveNoValidateWithContext`, `DeleteWithContext`, `ValidateWithContext`,
`AuxSaveWithContext`, `RunInTransaction` etc. (core/db.go:178-230). The non-ctx forms call
`context.Background()` — which **silently detaches the span**. Ban the non-ctx variants in
MediGo code with a lint rule.

**(b) For dbx queries, use `db.WithContext(ctx)`.** `dbx.DB.WithContext` (dbx/db.go:~144)
clones the DB with the ctx attached, and every subsequent `Query`/`Execute` uses it. In PB terms:

```go
func (r *recordRepo) ListByPatient(ctx context.Context, patientID string) ([]*core.Record, error) {
	var out []*core.Record
	err := r.app.RecordQuery("medical_records").
		AndWhere(dbx.HashExp{"patient": patientID}).
		WithContext(ctx).           // <- otelsql sees the span; without it, orphan
		All(&out)
	return out, err
}
```

**(c) PocketBase hooks carry a ctx — use it, don't invent one.** `core.ModelEvent` and
`core.RecordEvent` have a `Context context.Context` field (core/events.go:225). When a hook is
triggered from inside a request, that ctx descends from your traced request ctx *provided* you
used the `*WithContext` save variants in (a). `core.RequestEvent` has no `Context` field of its
own — use `e.Request.Context()`, which your tracing middleware has already replaced.

```go
app.OnRecordAfterCreateSuccess("medical_records").BindFunc(func(e *core.RecordEvent) error {
	ctx := e.Context // already carries the span if the caller used SaveWithContext
	span := trace.SpanFromContext(ctx)
	span.AddEvent("record.indexed")
	zerolog.Ctx(ctx).Info().Str("record_id", e.Record.Id).Msg("record created")
	return e.Next()
})
```

**Known gap, be honest about it:** PocketBase's *own* internal writes (auth token cleanup, MFA/OTP
expiry, cron, backups, the log batch flush) run under `context.Background()` via
`routine.FireAndForget`. Those DB spans will be root spans with no parent. That is correct — they
genuinely have no request parent — but expect them in your trace backend. Filter them by
`span.kind=internal` + absent `http.route` if they're noisy.

### 5.5 Sentry ↔ OTel (repeat of §3.6 for the OTel reader)

- OTel is the sole span producer. `EnableTracing: false` in Sentry.
- `sentryotel.NewOtelIntegration()` stamps `trace_id`/`span_id` onto Sentry events.
- `BeforeSendTransaction` returns `nil` as a hard backstop.
- Optionally add `sentryotlp.NewTraceExporter(ctx, dsn)` as a *second* batch processor if you
  want the traces in Sentry too. Same spans, two destinations, still produced once.

### 5.6 OTel metrics vs raw Prometheus — RECOMMENDATION

**Recommendation: Prometheus `client_golang` is the metrics API MediGo code writes against.
OpenTelemetry is for traces. Bridge otelsql's OTel metrics into the Prometheus registry with
`go.opentelemetry.io/otel/exporters/prometheus`. One `/metrics`, two producers, one scrape.**

```go
func InitMetrics(reg *prometheus.Registry) error {
	exp, err := otelprom.New(
		otelprom.WithRegisterer(reg),   // <- the SAME registry as promauto
		otelprom.WithoutScopeInfo(),
		otelprom.WithoutTargetInfo(),
		otelprom.WithNamespace("medigo"),
	)
	if err != nil {
		return err
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exp))
	otel.SetMeterProvider(mp)   // otelsql picks this up automatically
	return nil
}
```

Now `otelsql`'s `db.client.*` instruments and any future OTel-instrumented library land on the
same `/metrics` endpoint as `medigo_http_request_duration_seconds`.

**Justification — why not go all-in on OTel metrics:**

- **KISS, which the spec demands.** `m.RecordsCreated.WithLabelValues("lab_result").Inc()` is
  one line and every Go developer reads it instantly. The OTel equivalent needs a `Meter`, a
  `Int64Counter` constructed with an `error` return, a `ctx` at every call site, and
  `metric.WithAttributes(attribute.String(...))`. For ~20 hand-written business counters that
  is pure ceremony tax with no payoff.
- **The scrape target is Prometheus.** MediGo is a self-hosted single-binary app. The realistic
  deployment is Docker Compose with a Prometheus sidecar, not an OTel Collector pipeline.
  Optimise for the actual deployment.
- **`promauto` + a registry is testable.** `prometheus/client_golang/prometheus/testutil`
  (`ToFloat64`, `CollectAndCompare`) makes asserting "creating a record bumps the counter" a
  three-line testify test. OTel's metric testing story (`sdkmetric/metricdata` + a manual
  reader) is materially more code for the same assertion.
- **Native histograms** give you the high-resolution latency that OTel's exponential histograms
  offer, in the Prometheus API, today (`NativeHistogramBucketFactor` in `HistogramOpts`).
- **But you should not hand-instrument the DB layer**, and otelsql already emits OTel metrics
  for free. Hence the bridge. This is the one place where OTel metrics earn their keep, and the
  exporter makes it a three-line integration.

**Do NOT run both stacks side by side with two endpoints.** Two `/metrics` URLs, two naming
conventions, two sets of dashboards. If you take nothing else from this section: one registry,
one endpoint.

**When you would flip this recommendation:** if MediGo ever ships to an environment where an
OTel Collector is mandatory and Prometheus scraping is unavailable (e.g. a managed vendor that
only accepts OTLP), swap the reader for `otlpmetricgrpc` and rewrite the ~20 business counters.
That is a day of work, and it's the right trade to defer.

---

## 6. samber/do v2 — the service container

`samber/do/v2` v2.1.0. Generics-based, no codegen, no external deps.

### 6.1 The boundary with `core.App` — the important decision

PocketBase's `core.App` is already a de-facto container: it hands out the DB (`app.DB()`,
`app.ConcurrentDB()`), the filesystem (`app.NewFilesystem()`), settings, the mailer, the cron
scheduler, the subscription broker, and every hook. It is a 1500-line interface (core/app.go).

**Recommendation — draw the line at "infrastructure vs application":**

| Layer | Owner | Rule |
|---|---|---|
| DB handles, file storage, settings, mailer, cron, hooks, HTTP router, auth | `core.App` | Never re-wrap in `do`. Just `do.ProvideValue[core.App]`. |
| Repositories (interface-typed, thin, `core.App`-backed) | `do` | One provider each, returns an interface. |
| Domain services (records, patients, labs, sharing, search, export) | `do` | Interface-typed, depend only on other interfaces. |
| Cross-cutting singletons (zerolog.Logger, *Metrics, Config, Tracer) | `do` | Provided eagerly at boot. |
| HTTP handlers | `do` | Invoked once at `OnServe` to bind routes. |

**Explicitly: do NOT put `core.App` behind your own facade interface.** It is tempting
(`type Database interface { ... }`) and it is a mistake — you would be re-declaring a fraction
of a 1500-method interface, and every PB feature you later want forces an interface change. Put
the seam at the **repository**, one level up, where the interface is small and domain-shaped:

```go
type RecordRepository interface {
	Get(ctx context.Context, id string) (*domain.Record, error)
	ListByPatient(ctx context.Context, patientID string, f domain.RecordFilter) ([]*domain.Record, error)
	Create(ctx context.Context, r *domain.Record) error
	Update(ctx context.Context, r *domain.Record) error
	SoftDelete(ctx context.Context, id string) error
}
```

That interface is mockable, has five methods, and `core.App` never appears in a service
signature. **This is the DIP boundary. `core.App` lives below it, `do` lives above it.**

### 6.2 The container

```go
// internal/di/container.go
package di

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"
	"github.com/samber/do/v2"
)

func New(app core.App, cfg config.Config, log zerolog.Logger, m *obs.Metrics) do.Injector {
	i := do.New()

	// --- eager singletons: things that must exist and must not fail lazily ---
	do.ProvideValue[core.App](i, app)
	do.ProvideValue[config.Config](i, cfg)
	do.ProvideValue[zerolog.Logger](i, log)
	do.ProvideValue[*obs.Metrics](i, m)

	// --- repositories: concrete type provided, interface aliased ---
	do.Provide(i, repo.NewRecordRepository)   // func(do.Injector) (*repo.RecordRepo, error)
	do.MustInvokeAs[domain.RecordRepository](i) // compile-time proof it satisfies the interface
	do.As[*repo.RecordRepo, domain.RecordRepository](i)

	do.Provide(i, repo.NewPatientRepository)
	do.As[*repo.PatientRepo, domain.PatientRepository](i)

	// --- services ---
	do.Provide(i, service.NewRecordService)
	do.As[*service.RecordService, domain.RecordService](i)

	do.Provide(i, service.NewSharingService)
	do.As[*service.SharingService, domain.SharingService](i)

	// --- handlers (bound to routes at OnServe) ---
	do.Provide(i, httpapi.NewRecordHandler)

	return i
}
```

### 6.3 A provider that depends on `core.App` + logger + a repository interface

This is the shape the spec should mandate for every service:

```go
// internal/service/record.go
package service

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"
	"github.com/samber/do/v2"

	"medigo/internal/domain"
	"medigo/internal/obs"
)

// Compile-time interface satisfaction — one line, catches drift at build time.
var _ domain.RecordService = (*RecordService)(nil)

type RecordService struct {
	app     core.App                 // infrastructure: tx, hooks, files
	repo    domain.RecordRepository  // interface — swapped in tests
	patients domain.PatientRepository
	log     zerolog.Logger           // base logger; request scope comes from ctx
	metrics *obs.Metrics
}

// NewRecordService is the do provider. Signature is fixed by samber/do:
// func(do.Injector) (T, error).
func NewRecordService(i do.Injector) (*RecordService, error) {
	return &RecordService{
		app:      do.MustInvoke[core.App](i),
		repo:     do.MustInvoke[domain.RecordRepository](i),
		patients: do.MustInvoke[domain.PatientRepository](i),
		log:      do.MustInvoke[zerolog.Logger](i).With().Str("component", "record_service").Logger(),
		metrics:  do.MustInvoke[*obs.Metrics](i),
	}, nil
}

func (s *RecordService) Create(ctx context.Context, in domain.CreateRecordInput) (*domain.Record, error) {
	log := zerolog.Ctx(ctx) // request-scoped; falls back to disabled outside a request

	p, err := s.patients.Get(ctx, in.PatientID)
	if err != nil {
		return nil, fmt.Errorf("load patient: %w", err) // NOTE: id only, never the name
	}

	rec := domain.NewRecord(p.ID, in)

	// RunInTransaction gives the hook chain a ctx-carrying txApp.
	err = s.app.RunInTransaction(func(txApp core.App) error {
		return s.repo.WithApp(txApp).Create(ctx, rec)
	})
	if err != nil {
		return nil, fmt.Errorf("create record: %w", err)
	}

	s.metrics.RecordsCreated.WithLabelValues(string(in.Type)).Inc()
	log.Info().Str("record_id", rec.ID).Str("record_type", string(in.Type)).Msg("record created")
	return rec, nil
}
```

Test with testify + a mock repo, zero PocketBase:

```go
func TestRecordService_Create(t *testing.T) {
	repo := mocks.NewRecordRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
	// ... construct RecordService directly; do.Injector is not needed in tests
}
```

**Rule for the spec: `do` is used at the composition root ONLY.** No service ever holds a
`do.Injector` or calls `do.MustInvoke` outside its provider function. The moment a service
reaches into the container at runtime, you have a service locator, not dependency injection,
and testability dies. Enforce with a lint rule: `do.` imports allowed only under `internal/di/`
and in `New*` provider functions.

### 6.4 Lazy vs eager

- **`do.Provide` (lazy) is the default.** Services are constructed on first `Invoke`. Fine for
  everything that is only reachable through an HTTP route.
- **`do.ProvideValue` (eager) for values you already have**: `core.App`, config, logger,
  metrics, tracer.
- **Force-instantiate at boot anything whose constructor can fail**, so a misconfiguration is a
  crash at startup rather than a 500 on the first request:

```go
func (c *Container) Warmup() error {
	for _, invoke := range []func() error{
		func() error { _, err := do.Invoke[domain.RecordService](c.i); return err },
		func() error { _, err := do.Invoke[domain.SharingService](c.i); return err },
		func() error { _, err := do.Invoke[domain.ExportService](c.i); return err },
	} {
		if err := invoke(); err != nil {
			return err
		}
	}
	return nil
}
```

Call `Warmup()` in `OnServe` before binding routes. Fail-fast beats lazy surprises in a medical
app.

### 6.5 Shutdown ordering and health checks

`do` shuts services down in **reverse dependency order** automatically — a service is torn down
before the things it depends on. Implement the interfaces:

```go
type ShutdownerWithContext interface{ Shutdown(ctx context.Context) error }
type HealthcheckerWithContext interface{ HealthCheck(ctx context.Context) error }
```

```go
func (s *ExportService) Shutdown(ctx context.Context) error {
	return s.workers.StopAndWait(ctx)
}

func (s *RecordService) HealthCheck(ctx context.Context) error {
	return s.repo.Ping(ctx)
}
```

**Do NOT use `injector.ShutdownOnSignals(...)`.** PocketBase already owns the signal handler
(`pocketbase.go:Execute()` calls `signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)`). Two
handlers racing over SIGTERM is exactly the kind of shutdown bug that eats an in-flight upload.
Hook `do` into PB's terminate instead:

```go
app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
	Id:       "medigoContainerShutdown",
	Priority: -100, // after PB's own graceful HTTP shutdown (priority -9999)
	Func: func(e *core.TerminateEvent) error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if report := injector.ShutdownWithContext(ctx); report != nil && !report.Succeed {
			log.Error().Interface("report", report.Services).Msg("container shutdown had failures")
		}
		return e.Next()
	},
})
```

Health checks feed the readiness endpoint (§8):

```go
func (c *Container) Ready(ctx context.Context) error {
	if errs := c.i.HealthCheckWithContext(ctx); len(errs) > 0 {
		for name, err := range errs {
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return nil
}
```

Note `HealthCheck` only probes services that have **already been invoked** — another reason to
`Warmup()`.

---

## 7. The samber family — what each actually is, and where it earns its place

### 7.1 What they are (verified)

| Module | What it actually is | Version (Aug 2026) |
|---|---|---|
| `samber/lo` | Lodash-style generic helpers over slices/maps/channels. Pure functions, no types of its own. | v1.53.0 |
| `samber/do` | Generics-based DI container. Injector, scopes, providers, `Shutdowner`/`Healthchecker`. | v2.1.0 |
| `samber/mo` | **Monads.** `Option[T]`, `Result[T]`, `Either[L,R]`, `Either3..5`, `Future[T]`, `IO[T]`, `IOEither`, `Task`, `TaskEither`, `State[S,A]`. `.Map()`, `.FlatMap()`, `.Match()`, `.OrElse()`, `.MustGet()`. | v1.17.0 |
| `samber/ro` | **A Go implementation of the ReactiveX spec.** `Observable[T]`, `Observer[T]`, `Subject`, `Subscription`, `Notification[T]`; `Just`, `Of`, `Range`, `FromSlice`, `Interval`, `Map`, `FlatMap`, `Scan`, `Filter`, `Distinct`, `Take`, `Skip`, `Merge`, `CombineLatest`, `Zip`, `Catch`, `Retry`, `Pipe`, `ToSlice`. | **v0.4.1 — pre-1.0** |

If you assumed `ro` was "read-only helpers" or an iterator package: it is not. It is RxGo.

### 7.2 `samber/lo` — ADOPT, with a stdlib-first rule

`lo` is genuinely useful and MediGo will use it. But Go 1.26's `slices` and `maps` already cover
a large slice of what people reach for `lo` for, and a stdlib call is always the better choice
when it exists.

**Rule for the spec:**

> Prefer `slices`/`maps` from the standard library. Reach for `lo` only for operations the
> stdlib does not have.

**Use `lo` for** (no stdlib equivalent, and the loop version is genuinely worse):
`lo.GroupBy`, `lo.PartitionBy`, `lo.KeyBy`, `lo.SliceToMap`, `lo.Chunk`, `lo.UniqBy`,
`lo.Associate`, `lo.FlatMap`, `lo.CountBy`, `lo.Ternary` (sparingly), `lo.ToPtr`/`lo.FromPtr`,
`lo.Must` (in tests and `init` only).

Real MediGo use: grouping 13 record types for the dashboard, keying the standardized lab test
catalog by code, chunking bulk export batches.

**Do NOT use `lo` for** — use stdlib or a plain loop:
`lo.Contains` → `slices.Contains`. `lo.IndexOf` → `slices.Index`. `lo.Uniq` →
`slices.Sort` + `slices.Compact`. `lo.Reverse` → `slices.Reverse`. `lo.Keys`/`lo.Values` →
`maps.Keys`/`maps.Values` (+ `slices.Collect`). `lo.Min`/`lo.Max` → builtin `min`/`max`.
`lo.Filter`+`lo.Map` chains that would be one readable `for` loop with an `if`.

**The readability trap to ban outright:** chained `lo.Map(lo.Filter(lo.Map(...)))`. Three
allocations, three closures, and a reader who has to unwind it inside-out. A `for` loop with
`continue` is faster and clearer. `lo` is for *operations*, not for turning Go into a pipeline
language.

Also ban `lo.Must` outside tests/`init` — it panics, and a medical app should not panic on a
recoverable error.

### 7.3 `samber/do` — ADOPT, at the composition root only

Covered in §6. One rule: `do` imports are confined to `internal/di/` and to `New*` provider
functions. No runtime `MustInvoke`.

### 7.4 `samber/mo` — REJECT for error handling. Here is why, bluntly.

The spec asks whether `mo.Result`/`mo.Option` fight Go idiom and KISS. **They do, and it is not
close.** This is the "clever library that makes the codebase worse" trap.

**1. It breaks the error ecosystem you just spent sections 1-5 building.**

`errors.Is`, `errors.As`, and `%w` wrapping are how Go errors work. A `mo.Result[T]` boxes the
error inside a struct; every layer that wants to inspect it has to `.Error()` it back out first,
and the moment someone forgets, the wrap chain is severed. Your Sentry integration, your zerolog
`Err()` field, and your `router.ToApiError(err)` status mapping all key off a real `error`.
`mo.Result` is a wall between your domain and every one of them.

**2. `errcheck`/`golangci-lint` stop protecting you.**

An ignored `error` return is a compile-adjacent lint failure. An ignored `mo.Result` is a
perfectly legal expression statement. You are trading a tool-enforced invariant for a
convention. In a **medical records app**, "we forgot to check whether that write succeeded" is
not a style issue.

**3. `.MustGet()` becomes the path of least resistance.**

Every codebase that adopts `Result` in Go ends up littered with `.MustGet()` because the
alternative — `.Match(func(T) X, func(error) X)` at every call site — is more verbose than
`if err != nil`. `MustGet` panics. You have now converted compile-time-visible error handling
into runtime panics. This is strictly worse than what you started with.

**4. Go has no `?` operator, so monads never pay off.**

`Result` earns its keep in Rust because `?` makes propagation a single character. In Go,
propagating a `mo.Result` through five layers is *more* code than `if err != nil { return err }`,
not less. You pay the full syntactic cost of monads and receive none of the ergonomic benefit.

**5. It fails the "new contributor" test, which is what KISS actually means.**

`if err != nil` is readable by every Go developer alive. `res.FlatMap(...).Match(...)` requires
the reader to hold a type-class model in their head. For a self-hosted personal medical records
app that wants contributors, this is an own goal.

**Narrow, honest exception — `mo.Option[T]`:** there *is* one place it is defensible, which is
distinguishing "field absent" from "field explicitly set to zero" in PATCH request DTOs — the
classic `*string` vs `sql.NullString` problem. Medical records have real nullable fields
(end_date on a medication, a lab reference range).

**But even there, reject it**, because:

- `*T` already expresses it, is stdlib, and marshals correctly with `encoding/json`.
- `mo.Option[T]` needs custom `MarshalJSON`/`UnmarshalJSON` handling to round-trip through your
  DTOs and OpenAPI generation, and PocketBase's field types (`types.JSONMap`, `types.DateTime`)
  don't know about it.
- Introducing one monad type opens the door to the rest.

**RULE FOR THE SPEC (encode verbatim):**

> **`samber/mo` MUST NOT be used in MediGo.** Errors are `error`, returned as the last value and
> handled with `if err != nil`, wrapped with `%w`. Optionality is `*T` or a `bool` companion
> return. `mo` is not in `go.mod`. If a PR adds it, the PR is rejected.

If it is already in the locked stack list, the honest recommendation is: **drop it from the
stack**. Carrying a dependency you have banned in the rules is worse than not having it — it is
an invitation.

### 7.5 `samber/ro` — REJECT. It is RxGo, and MediGo is a CRUD app.

`ro` is a ReactiveX implementation: `Observable`, `Observer`, `Subject`, `Subscription`,
`Pipe`, `Merge`, `CombineLatest`, `Retry`, `Catch`. At **v0.4.1**, pre-1.0, published August
2026, explicitly warning of breaking changes.

**Why it does not belong in MediGo:**

1. **The problem it solves does not exist here.** Rx pays off for complex event-stream
   composition — debouncing user input against three merged async sources with backpressure and
   retry. MediGo is: HTTP request in → validate → SQLite read/write → SSE push → HTTP response
   out. There is no stream algebra to express.
2. **Go already has the primitives.** Channels, `context`, `errgroup`, and Go 1.23+ `iter.Seq`
   cover every case MediGo will hit. Adding an Observable layer on top of channels is a second
   concurrency model in one codebase — now every contributor must know both, and the failure
   modes (unsubscribed observables leaking goroutines, hot vs cold semantics, error-channel
   termination) are subtle and unfamiliar to Go developers.
3. **"But we have SSE" is not an argument.** Datastar SSE is: hold the connection, write events
   as they occur, flush. PocketBase's realtime is already an implemented broker
   (`tools/subscriptions.Broker`). Wrapping either in `Observable[T]` adds an abstraction with
   nothing underneath it. Look at `apis/realtime.go` — PB solves this with a channel per client
   and a `select`. That is the right amount of machinery.
4. **Pre-1.0 dependency in a medical records app.** A `v0.x` module with documented breaking
   changes, in the path of your realtime layer, in an app whose users cannot tolerate a broken
   upgrade. The risk/reward is indefensible.
5. **It fights KISS harder than `mo` does.** `mo` makes one thing (errors) weird. `ro` makes
   your entire concurrency model unfamiliar.

**RULE FOR THE SPEC:**

> **`samber/ro` MUST NOT be used in MediGo.** Realtime is PocketBase's native realtime where PB
> supports it, and Datastar SSE elsewhere, implemented with channels, `context`, and
> `golang.org/x/sync/errgroup`. Reactive-stream abstractions are out of scope.

Same recommendation as `mo`: **drop it from the stack list.** It was almost certainly added on
the assumption that `ro` meant something else.

### 7.6 Summary rule the spec can encode

```
ALLOWED:  samber/lo  — stdlib slices/maps first; lo only for GroupBy/KeyBy/Chunk/UniqBy/
                       SliceToMap/PartitionBy/CountBy/ToPtr. No chained Map(Filter(Map())).
                       No lo.Must outside tests.
ALLOWED:  samber/do  — composition root only (internal/di + New* providers). No runtime Invoke.
BANNED:   samber/mo  — errors are `error`; optionality is *T. Not in go.mod.
BANNED:   samber/ro  — realtime is channels + context + PB realtime + Datastar SSE. Not in go.mod.
```

---

## 8. Graceful shutdown, health and readiness

### 8.1 What PocketBase actually does on SIGTERM

`(*PocketBase).Execute()` (pocketbase.go:~180):

```go
signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
go func() { execCh <- routine.SafeWrap(pb.RootCmd.Execute)() }()
select {
case <-sigCh:
case execErr = <-execCh:
}
signal.Stop(sigCh)
event := new(core.TerminateEvent)
event.App = pb
return pb.OnTerminate().Trigger(event, func(e *core.TerminateEvent) error {
	return errors.Join(e.App.ResetBootstrapState(), execErr)
})
```

And `apis.Serve` binds the HTTP shutdown at priority `-9999` (apis/serve.go:171):

```go
cancelBaseCtx()                                              // kills SSE/long-poll contexts
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
wg.Add(1)
_ = server.Shutdown(ctx)
...
```

### 8.2 The landmine: the drain window is hardcoded to 1 second

`context.WithTimeout(context.Background(), 1*time.Second)` — **one second**, not configurable,
not exposed on `ServeConfig`. Any request still running after 1s has its connection cut
mid-response.

For MediGo that is a real problem:

- attachment uploads/downloads of a scanned lab PDF,
- `REPORTING/export` generating a multi-patient PDF,
- `backup/restore` operations,
- any Datastar SSE stream (though `cancelBaseCtx()` handles those deliberately).

**Mitigations, in order of preference:**

1. **Make long operations resumable or asynchronous.** Exports and backups become jobs with a
   status endpoint rather than a long-held HTTP response. This is the right fix and it's a
   design decision, not a workaround — put it in the spec: *no MediGo endpoint holds a request
   open for more than a few seconds except SSE.*
2. **Bind your own pre-drain at a priority lower than -9999** to stop accepting new work and
   give in-flight requests a chance before PB's 1s window opens:

```go
app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
	Id:       "medigoPreDrain",
	Priority: -10000, // BEFORE pbGracefulShutdown (-9999)
	Func: func(e *core.TerminateEvent) error {
		readiness.Store(false)        // /readyz starts failing -> LB stops routing
		log.Info().Msg("draining")
		time.Sleep(cfg.DrainDelay)    // default 5s: let the LB notice
		inflight.Wait(cfg.DrainMax)   // bounded wait on our own in-flight counter
		return e.Next()               // now PB's 1s Shutdown runs against an idle server
	},
})
```

This is the standard "fail readiness, then drain" pattern and it makes PB's 1s window
irrelevant, because by the time it opens there is nothing in flight.

3. **SSE streams must handle `cancelBaseCtx()`.** `server.BaseContext` returns `baseCtx`, so
   every request context is cancelled the moment terminate starts. Your Datastar SSE handler
   must select on `e.Request.Context().Done()` and return cleanly — otherwise the goroutine
   leaks until process exit and the client sees a hard reset instead of a stream close.

Also note `TerminateEvent.IsRestart`: on a restart PB waits an extra 3s (`time.AfterFunc`) for
`execve`. Don't assume terminate always means exit.

### 8.3 Health and readiness endpoints

PocketBase already has `GET /api/health` (`apis/health.go`) — it returns 200 unconditionally and
leaks `canBackup`/`realIP`/`possibleProxyHeader` to superusers. It is a liveness probe and
nothing more. **Add your own, hand-written under `/api/v1`, per the locked API decision:**

```go
// GET /api/v1/health/live  — is the process alive? Cheap. No dependencies.
func (h *HealthHandler) Live(e *core.RequestEvent) error {
	return e.JSON(http.StatusOK, LivenessResponse{Status: "ok"})
}

// GET /api/v1/health/ready — can we serve traffic? Probes dependencies.
func (h *HealthHandler) Ready(e *core.RequestEvent) error {
	if !h.readiness.Load() {
		return e.JSON(http.StatusServiceUnavailable, ReadinessResponse{Status: "draining"})
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{}
	status := http.StatusOK

	// DB: a real query, not a Ping (Ping on SQLite proves nothing).
	var one int
	if err := h.app.ConcurrentDB().NewQuery("SELECT 1").WithContext(ctx).Row(&one); err != nil {
		checks["database"] = "error"
		status = http.StatusServiceUnavailable
	} else {
		checks["database"] = "ok"
	}

	// Container services that implement do.Healthchecker.
	if err := h.container.Ready(ctx); err != nil {
		checks["services"] = "error"
		status = http.StatusServiceUnavailable
	} else {
		checks["services"] = "ok"
	}

	// Filesystem for attachments.
	if fs, err := h.app.NewFilesystem(); err != nil {
		checks["storage"] = "error"
		status = http.StatusServiceUnavailable
	} else {
		_ = fs.Close()
		checks["storage"] = "ok"
	}

	return e.JSON(status, ReadinessResponse{Status: statusWord(status), Checks: checks})
}

// GET /api/v1/health/startup — bootstrap + migrations complete?
```

**Rules:**

- **Liveness must not check dependencies.** A DB blip should not get your container killed and
  restarted into the same blip. Liveness = "the process is not deadlocked".
- **Readiness must check dependencies and must respect the drain flag.** This is the only
  endpoint the load balancer reads.
- **Both are unauthenticated** (they leak nothing) but **exclude them from the activity log and
  metrics** so probe traffic doesn't dominate your dashboards:

```go
e.Router.GET("/api/v1/health/live", h.Live).
	Unbind(apis.DefaultActivityLoggerMiddlewareId, "medigoMetrics", "medigoTracing")
```

`Route.Unbind` (tools/router/route.go:52) removes middleware for a single route.

### 8.4 Ordering everything at boot

```go
func main() {
	cfg := config.MustLoad()                            // caarlos0/env, §9
	log := obs.NewLogger(cfg.Log, buildRelease)
	zerolog.DefaultContextLogger = &log

	flushSentry, err := obs.InitSentry(cfg.Sentry, buildRelease)
	fatalOn(log, err)
	defer flushSentry()

	metrics := obs.NewMetrics()
	fatalOn(log, obs.InitMetrics(metrics.Registry()))    // OTel->prom bridge
	shutdownTracing, err := obs.InitTracing(ctx, cfg.OTel, buildRelease)
	fatalOn(log, err)

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  cfg.DataDir,
		DefaultDev:      cfg.Dev,
		HideStartBanner: true,
		DBConnect:       obs.NewInstrumentedDBConnect(metrics.RegisterDBStats),
	})

	obs.BridgePBLogs(app, log)                           // §2.3 — BEFORE bootstrap

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil { return err }
		return obs.ConfigurePBSettings(e.App)             // MaxDays=1, LogIP=false, ...
	})

	container := di.New(app, cfg, log, metrics)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := container.Warmup(); err != nil { return err }

		e.Router.Unbind(apis.DefaultActivityLoggerMiddlewareId)
		e.Router.Bind(obs.TracingMiddleware(rl))
		e.Router.Bind(obs.MetricsMiddleware(metrics, rl))
		e.Router.Bind(obs.RequestLogger(log))
		e.Router.Bind(obs.SentryRecover())

		httpapi.Bind(e.Router, container)                 // /api/v1/*
		lockdown.BindCollectionGuards(e.Router)           // block public /api/collections/*
		return e.Next()
	})

	metricsSrv, err := obs.StartMetricsServer(cfg.Metrics, metrics, log)
	fatalOn(log, err)

	app.OnTerminate().Bind(preDrainHandler(cfg, log))     // priority -10000
	app.OnTerminate().Bind(containerShutdownHandler(container, log))  // -100
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = metricsSrv.Shutdown(ctx)
		_ = shutdownTracing(ctx)
		flushSentry()
		return e.Next()
	})

	if err := app.Start(); err != nil {                   // registers serve/superuser cmds
		log.Fatal().Err(err).Msg("app exited")
	}
}
```

Note `app.Start()` registers PB's Cobra subcommands and calls `Execute()`. Since
`pb.RootCmd` **is** a `*cobra.Command`, MediGo's own commands (`medigo migrate`,
`medigo seed-catalog`) are added with `app.RootCmd.AddCommand(...)` before `Start()` — no second
CLI framework, per the locked decision.

**One caveat on the observability init order:** everything above (`InitSentry`, `InitTracing`,
`StartMetricsServer`) runs for *every* subcommand, including `medigo --help` and
`medigo superuser create`. Gate them on `os.Args` naming `serve`, or on a `cobra` `PersistentPreRunE`,
so a CLI invocation doesn't open an OTLP connection and a metrics port.

---

## 9. Configuration — `caarlos0/env/v11`

`caarlos0/env/v11@v11.4.1`. Verified tag vocabulary from `env.go:550-600`:

- `env:"NAME"` plus comma options: `required`, `notEmpty`, `unset`, `expand`, `file`
- `envDefault:"..."` — default value
- `envSeparator:","` — slice element separator (also used for map entries)
- `envKeyValSeparator:":"` — map key/value separator (`env.go:769`)
- `envPrefix:"..."` — prefix applied to a nested struct's fields
- `Options.Prefix` — global prefix applied to every key

### 9.1 The struct

```go
// internal/config/config.go
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// --- app ---
	Env       string        `env:"ENV"        envDefault:"production"`
	Dev       bool          `env:"DEV"        envDefault:"false"`
	DataDir   string        `env:"DATA_DIR"   envDefault:"/var/lib/medigo/pb_data"`
	HTTPAddr  string        `env:"HTTP_ADDR"  envDefault:"0.0.0.0:8090"`
	PublicURL string        `env:"PUBLIC_URL,required,notEmpty"`

	// --- shutdown ---
	DrainDelay time.Duration `env:"DRAIN_DELAY" envDefault:"5s"`
	DrainMax   time.Duration `env:"DRAIN_MAX"   envDefault:"25s"`

	// --- CORS / trusted proxies ---
	AllowedOrigins []string `env:"ALLOWED_ORIGINS" envSeparator:"," envDefault:""`
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`

	// --- nested, prefixed ---
	Log     LogConfig     `envPrefix:"LOG_"`
	Sentry  SentryConfig  `envPrefix:"SENTRY_"`
	Metrics MetricsConfig `envPrefix:"METRICS_"`
	OTel    OTelConfig    `envPrefix:"OTEL_"`
	Files   FilesConfig   `envPrefix:"FILES_"`
}

type LogConfig struct {
	Level  string `env:"LEVEL"  envDefault:"info"`
	Pretty bool   `env:"PRETTY" envDefault:"false"`
}

type SentryConfig struct {
	// `file` lets the DSN come from a mounted secret file (Docker/K8s) instead
	// of the process environment, where it would show up in `docker inspect`.
	// `unset` removes it from os.Environ() after parsing so it cannot leak
	// into a subprocess or a crash dump.
	DSN         string  `env:"DSN,file,unset"`
	Environment string  `env:"ENVIRONMENT" envDefault:"production"`
	SampleRate  float64 `env:"SAMPLE_RATE" envDefault:"1.0"`
	Debug       bool    `env:"DEBUG"       envDefault:"false"`
}

type MetricsConfig struct {
	Enabled bool   `env:"ENABLED" envDefault:"true"`
	// localhost by default: exposing metrics is an explicit act.
	Addr    string `env:"ADDR"    envDefault:"127.0.0.1:9090"`
	Token   string `env:"TOKEN,file,unset"` // only used in single-port fallback mode
}

type OTelConfig struct {
	Enabled     bool              `env:"ENABLED"      envDefault:"false"`
	Endpoint    string            `env:"ENDPOINT"     envDefault:"localhost:4318"`
	Insecure    bool              `env:"INSECURE"     envDefault:"true"`
	SampleRatio float64           `env:"SAMPLE_RATIO" envDefault:"1.0"`
	// OTEL_HEADERS="authorization:Bearer xyz,x-tenant:acme"
	Headers     map[string]string `env:"HEADERS,file" envSeparator:"," envKeyValSeparator:":"`
	Environment string            `env:"ENVIRONMENT"  envDefault:"production"`
}

type FilesConfig struct {
	MaxUploadBytes int64    `env:"MAX_UPLOAD_BYTES" envDefault:"33554432"` // 32 MiB
	AllowedMIME    []string `env:"ALLOWED_MIME" envSeparator:"," envDefault:"application/pdf,image/png,image/jpeg,image/heic,text/plain"`
}
```

Env vars land as `MEDIGO_SENTRY_DSN`, `MEDIGO_OTEL_ENDPOINT`, `MEDIGO_LOG_LEVEL`, etc., because
of the global prefix below.

### 9.2 Loading and validating

`caarlos0/env` parses; it does **not** validate semantics. Validate explicitly and fail at boot
— the locked decision says config is "validated at boot", so make that real:

```go
func Load() (Config, error) {
	cfg, err := env.ParseAsWithOptions[Config](env.Options{
		Prefix:          "MEDIGO_",
		RequiredIfNoDef: false, // be explicit with `required` instead of blanket-requiring
	})
	if err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func MustLoad() Config {
	cfg, err := Load()
	if err != nil {
		// stderr, not zerolog: the logger config may be what's broken.
		fmt.Fprintln(os.Stderr, "FATAL:", err)
		os.Exit(2)
	}
	return cfg
}

func (c Config) Validate() error {
	var errs []error

	if _, err := zerolog.ParseLevel(c.Log.Level); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL %q is not a valid level", c.Log.Level))
	}
	if c.Env != "production" && c.Env != "staging" && c.Env != "development" {
		errs = append(errs, fmt.Errorf("ENV %q must be production|staging|development", c.Env))
	}
	if u, err := url.Parse(c.PublicURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("PUBLIC_URL %q is not an absolute URL", c.PublicURL))
	}
	if c.Sentry.SampleRate < 0 || c.Sentry.SampleRate > 1 {
		errs = append(errs, errors.New("SENTRY_SAMPLE_RATE must be within [0,1]"))
	}
	if c.OTel.SampleRatio < 0 || c.OTel.SampleRatio > 1 {
		errs = append(errs, errors.New("OTEL_SAMPLE_RATIO must be within [0,1]"))
	}
	if c.OTel.Enabled && c.OTel.Endpoint == "" {
		errs = append(errs, errors.New("OTEL_ENDPOINT is required when OTEL_ENABLED=true"))
	}
	if c.Metrics.Enabled {
		if _, _, err := net.SplitHostPort(c.Metrics.Addr); err != nil {
			errs = append(errs, fmt.Errorf("METRICS_ADDR %q is not host:port", c.Metrics.Addr))
		}
	}
	// Production safety rails.
	if c.Env == "production" {
		if c.Dev {
			errs = append(errs, errors.New("DEV must be false in production"))
		}
		if c.Log.Pretty {
			errs = append(errs, errors.New("LOG_PRETTY must be false in production"))
		}
		if strings.HasPrefix(c.PublicURL, "http://") && !isLoopback(c.PublicURL) {
			errs = append(errs, errors.New("PUBLIC_URL must be https in production"))
		}
		if c.Metrics.Enabled && strings.HasPrefix(c.Metrics.Addr, "0.0.0.0") && c.Metrics.Token == "" {
			errs = append(errs, errors.New("METRICS_TOKEN is required when METRICS_ADDR binds 0.0.0.0"))
		}
	}
	if c.DrainMax <= c.DrainDelay {
		errs = append(errs, errors.New("DRAIN_MAX must exceed DRAIN_DELAY"))
	}
	return errors.Join(errs...)
}
```

`errors.Join` reports **every** problem at once. Nothing is worse than fixing one env var per
container restart.

### 9.3 Secrets handling

Three mechanisms, use all three:

1. **`,file`** — the value of the env var is a *path*; `env` reads the file
   (`env.go:643 getFromFile`). This is how Docker secrets and Kubernetes projected volumes
   work. `MEDIGO_SENTRY_DSN=/run/secrets/sentry_dsn`.
2. **`,unset`** — after parsing, the variable is removed from the process environment
   (`env.go:575`). A subsequent `os.Environ()` — including anything a subprocess or a crash
   handler dumps — no longer contains it.
3. **Never log the config struct.** Give `Config` a `String()`/`MarshalZerologObject` that
   redacts, or simply never pass it to a logger:

```go
func (c Config) MarshalZerologObject(e *zerolog.Event) {
	e.Str("env", c.Env).
		Str("http_addr", c.HTTPAddr).
		Str("data_dir", c.DataDir).
		Str("log_level", c.Log.Level).
		Bool("sentry", c.Sentry.DSN != "").      // presence, not value
		Bool("otel", c.OTel.Enabled).
		Bool("metrics", c.Metrics.Enabled)
}
```

Note that PB has its own secret channel: the `--encryptionEnv` flag names an env var holding a
32-char key used to encrypt settings at rest. Wire it from
`MEDIGO_PB_ENCRYPTION_ENV` and keep it out of the `Config` struct — PB reads the env var itself.

### 9.4 Testing config

`env.ParseAsWithOptions` accepts an explicit `Environment map[string]string`, so config tests
need no `t.Setenv` and can run in parallel:

```go
func TestConfigValidate(t *testing.T) {
	cfg, err := env.ParseAsWithOptions[Config](env.Options{
		Prefix: "MEDIGO_",
		Environment: map[string]string{
			"MEDIGO_PUBLIC_URL":  "https://medigo.example",
			"MEDIGO_ENV":         "production",
			"MEDIGO_LOG_PRETTY":  "true",
		},
	})
	require.NoError(t, err)
	require.ErrorContains(t, cfg.Validate(), "LOG_PRETTY must be false in production")
}
```

---

## 10. Consolidated rules for the spec

1. zerolog is the only app logger. JSON to stdout. Base logger injected via `do`; request
   correlation via `zerolog.Ctx(ctx)`. Never pass a logger as a method parameter.
2. Every service method takes `ctx context.Context` first. Every PB write uses the
   `*WithContext` variant. Every dbx query uses `.WithContext(ctx)`.
3. PB internal logs reach zerolog via the `OnModelCreate("_logs")` bridge, with
   `Logs.MaxDays = 1` and a boot-time self-test. `_logs` never receives a row.
   PB's `pbActivityLogger` middleware is unbound and replaced.
4. `log.Error()` means "this becomes a Sentry issue" — only the boundary that owns the failure
   calls it. Everything below uses `Warn`/`Debug` and returns the error. This is the
   anti-double-report rule.
5. Sentry: `EnableTracing: false`, `BeforeSendTransaction` returns nil, `DataCollection` locks
   out bodies/cookies/query params, `BeforeSend` scrubs. A testify test proves the scrubber
   drops names/DOB/tokens/medical text — this test is a required CI gate.
6. Never put a domain struct, a field value, or a filename in an error message, a Sentry tag,
   `Extra`, a metric label, or a span attribute. Ids only.
7. Metric route labels come from `http.Request.Pattern`, filtered through an allowlist that
   collapses unknowns to `"other"`.
8. `/metrics` is a separate `http.Server` on `127.0.0.1:9090` by default.
9. OTel owns traces. Prometheus owns metrics. otelsql's OTel metrics are bridged into the
   Prometheus registry. One `/metrics`.
10. `samber/do` at the composition root only. `core.App` stays below the repository interface
    and never appears in a service signature.
11. `samber/lo` after stdlib `slices`/`maps`, for grouping/keying operations only.
12. `samber/mo` and `samber/ro` are banned and should be removed from the stack list.
13. `/api/v1/health/live` never touches a dependency. `/api/v1/health/ready` does, and respects
    the drain flag. Both are excluded from tracing/metrics/activity logging.
14. Drain readiness before PocketBase's hardcoded 1-second HTTP shutdown window opens. No
    endpoint holds a request open for more than a few seconds except SSE.
15. Config is `caarlos0/env/v11` with prefix `MEDIGO_`, secrets via `,file,unset`, and an
    explicit `Validate()` returning `errors.Join` of every problem.

## 11. Open risks to track

| Risk | Impact | Mitigation |
|---|---|---|
| PB changes `_logs` write path on upgrade | All PB internal logs silently lost | Boot-time bridge self-test (§2.3) |
| PB's DSN pragma string drifts from our copy | WAL/foreign keys silently off | CI grep against the module cache (§5.3) |
| PB's 1s shutdown window | Cut connections on deploy | Pre-drain at priority -10000 (§8.2) |
| `sentry-go/otel` submodule is young; the span-processor bridge was removed | Integration churn | Pin the version; the linking integration is a small surface |
| otelsql span volume on SQLite | Large traces | `OmitRows`, `OmitConnPrepare`, sampling if needed |
| Scrubber gaps | PHI in Sentry | Denylist by key + value regex + a required test + the "ids only" review rule |
