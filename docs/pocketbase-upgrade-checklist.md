# PocketBase upgrade checklist

MediKube embeds PocketBase rather than wrapping it, and in four places it reaches past a
public API because no public API exists. Each one is a place a PocketBase upgrade can break
MediKube **silently** — which is why every entry below records the symptom rather than only the
mechanism. Work this list before bumping the pinned version, and read the symptom column first:
it is what you will actually see.

Pinned at **PocketBase v0.40.1** (`go.mod`). Risk R8, cross-artifact CT-1.

## 1. The `pb.App` decorator — the log bridge, request path

**What.** `internal/logging/pbbridge.go` decorates the exported embedded `core.App` field on
`pocketbase.PocketBase` so `Logger()` resolves through MediKube's zerolog handler.

**Why there is no public API.** `core.BaseApp.initLogger` hardcodes its slog handler, so the
handler cannot be injected. Decorating the embedded field is the only seam. It works because
PocketBase does `event.App = app`, so `Logger()` resolves through the decorator dynamically at
call time (research D-29).

**Symptom if an upgrade breaks it.** Application logs keep flowing and look correct, but
PocketBase's own lines stop appearing in the zerolog stream — or start appearing twice, in
PocketBase's own format. Nothing errors. The bridge test in `internal/logging` is what fails.

**Check on upgrade.** That `core.App` is still an exported embedded field on
`pocketbase.PocketBase`, and that `event.App` is still assigned per event rather than captured
once at construction.

## 2. The `_logs` model hook — the log bridge, transaction path

**What.** `internal/logging/pblogs.go` binds `OnModelCreate("_logs")` and emits the record into
zerolog. It deliberately does **not** call `e.Next()`.

**Why there is no public API.** The decorator in (1) cannot see transaction-scoped logging:
`createTxApp` shallow-copies a `*BaseApp`, keeping the hardcoded internal logger, so every line
logged inside `RunInTransaction` bypasses the decorator entirely (research D-29). Intercepting
the model write is the second half of the bridge, and both halves are required.

`Logs.MaxDays` must be **1**, never 0. At 0 PocketBase disables its log writes altogether and
its own failures go nowhere at all — the setting reads like a retention knob and behaves like an
off switch (constitution Principle VI).

**Symptom if an upgrade breaks it.** Log lines written inside a transaction vanish. Everything
outside a transaction still logs, so the gap is invisible until you go looking for the record of
a write that failed. If `_logs` is renamed, the hook silently never fires.

**Check on upgrade.** That the internal collection is still called `_logs`, that
`OnModelCreate` still fires for it, and that `Logs.MaxDays = 1` still means retain-one-day.

## 3. The `WriteTimeout` override — SSE survival

**What.** An `OnServe` hook adjusts `se.Server.WriteTimeout` before the listener starts, and
every SSE handler goes through the mandatory `internal/web/stream.newStream()` helper, which
clears the per-connection deadline with
`http.NewResponseController(e.Response).SetWriteDeadline(time.Time{})`.

**Why there is no public API.** `apis/serve.go` constructs the server as a struct literal with
`WriteTimeout: 5 * time.Minute` and no configuration field. `datastar.NewSSE` sets
`Cache-Control`, `Content-Type` and `Connection` and flushes — it never touches the write
deadline (research D-34).

**Symptom if an upgrade breaks it.** Every long-lived stream dies at **exactly five minutes**
with a write error, and the browser reconnect-loops. It passes every test shorter than five
minutes, which is what makes it dangerous. SC-007 requires a view left open for sixty continuous
minutes to still be receiving updates, so the CI job that holds a stream open for more than five
minutes is the only thing standing between this and production.

**Check on upgrade.** That `tools/router.ResponseWriter` still implements
`Unwrap() http.ResponseWriter` — `SetWriteDeadline`, `Flush` and `Hijack` all pass through that
one method — and that the server is still built somewhere a hook can reach before `ListenAndServe`.

## 4. The copied `DefaultDBConnect` pragma string — otelsql

**What.** otelsql attaches through `pocketbase.Config.DBConnect`, which means MediKube supplies
the connection function and therefore carries its own copy of PocketBase's pragma string
(research D-30).

**Why there is no public API.** Overriding `DBConnect` replaces PocketBase's default wholesale;
there is no way to wrap it and keep the pragmas.

**Symptom if an upgrade breaks it.** Nothing fails. The database opens, the application runs,
and a pragma PocketBase started relying on — journal mode, busy timeout, foreign keys — is
quietly not set. It surfaces much later as lock contention or as a constraint that does not fire.

**Check on upgrade.** Diff PocketBase's current `DefaultDBConnect` pragma string against the
copy in `internal/platform/pb/app.go`. The drift-check test is what should catch this; make sure
it is still comparing against the real thing.
