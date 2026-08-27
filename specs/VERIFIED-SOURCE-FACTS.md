# Verified source facts — read from actual module source, not documentation

Probed against real downloaded sources on 2026-08-26. These OVERRIDE any
doc-derived claim in the other dossiers. Module cache: /home/agent/go/pkg/mod

> **CORRECTION (applied after a build test).** An earlier revision of this file
> said PocketBase v0.40.1 "resolves on go 1.26.5". That was WRONG: it conflated
> `go get` (module download) with compilation. See FACT 0.

| module | version | verified |
| --- | --- | --- |
| github.com/pocketbase/pocketbase | v0.40.1 | **requires go >= 1.27**; builds clean on go1.27.0 |
| github.com/starfederation/datastar-go | v1.2.2 | builds; go.mod declares go 1.24 |
| github.com/a-h/templ | v0.3.1020 | builds |

All three resolve together in one module graph. PocketBase pulls
`modernc.org/sqlite v1.57.0` (pure-Go SQLite — **no cgo required**, so
`CGO_ENABLED=0` cross-compilation holds).

---

## FACT 0 — PocketBase v0.40.1 REQUIRES Go 1.27. Go 1.26.5 cannot build it.

Verified by actually building, not by downloading:

```
$ GOTOOLCHAIN=local go build ./...
go: go.mod requires go >= 1.27 (running go 1.26.5; GOTOOLCHAIN=local)

$ go build ./...        # auto toolchain fetches go1.27.0
$ go version
go version go1.27.0 linux/arm64                 # 33MB binary, builds clean
```

Evidence:

- `github.com/pocketbase/pocketbase@v0.40.1/go.mod` line 3 declares `go 1.27`.
- **67 non-test `.go` files** import the Go 1.27 stdlib package
  `encoding/json/v2`; **15 of those are in `core/` and `apis/`**, so it is
  unavoidable, not confined to an optional subpackage.
- A further **10 non-test files** import `encoding/json/jsontext`.
- No build tags, no fallback path.

### Consequence

MediGo MUST be `go 1.27`, not the 1.26.5 house standard that `arc-ui`, `gmod`,
`appbase` and `medikeep-mcp` share. MediGo is the first project in this monorepo
off that standard, and that is a deliberate, documented divergence forced by the
PocketBase decision.

Required changes:

- `go.mod`: `go 1.27` plus a `toolchain go1.27.x` line.
- Dockerfile: `ARG GO_VERSION=1.27`.
- CI: do **NOT** set `GOTOOLCHAIN=local`, or the build fails with the
  misleading "built with go1.26 < targeted go1.27".
- golangci-lint: v2 at a release that understands Go 1.27.
- Budget real time for Go 1.27's `encoding/json` v2 retrofit semantics in
  MediGo's OWN DTOs — nil-versus-empty slices, `json.RawMessage` handling, and
  duplicate-key rejection are not fully backward compatible.

Rejected alternative: pinning PocketBase to v0.39.x to stay on Go 1.26.5. It
forfeits v0.40's filesystem hooks and backup fixes, and would invalidate every
file:line citation in this document.

---

## FACT 1 — PocketBase's logger CANNOT be injected. Confirmed.

`pocketbase.Config` (pocketbase.go) fields are ONLY:
HideStartBanner, DefaultDev, DefaultDataDir, DefaultEncryptionEnv,
DefaultQueryTimeout, DataMaxOpenConns, DataMaxIdleConns, AuxMaxOpenConns,
AuxMaxIdleConns, DBConnect.

`core.BaseAppConfig` (core/base.go) fields are ONLY:
DBConnect, DataDir, EncryptionEnv, QueryTimeout, DataMaxOpenConns,
DataMaxIdleConns, AuxMaxOpenConns, AuxMaxIdleConns, IsDev.

**Neither exposes a Logger or slog.Handler field.**

`core/base.go:1472 initLogger()` hardcodes:

```go
handler := logger.NewBatchHandler(logger.BatchOptions{
    Level: getLoggerMinLevel(app),
    BatchSize: 200,
    BeforeAddFunc: ...,   // prints to console when app.IsDev()
    WriteFunc: ...,       // batch-INSERTs into the aux _logs DB
})
app.logger = slog.New(handler)     // core/base.go:1536 — private field
```

`app.logger` is unexported and `Logger()` (core/base.go:378) just returns it.

### Consequences for the locked "zerolog primary, bridged into PB" decision

- Injecting a zerolog-backed slog.Handler into PocketBase is **impossible** in
  v0.40.1 without forking. Decision as literally stated is not achievable.
- What IS achievable, and what the plan must say instead:
  1. MediGo's own code uses zerolog exclusively and NEVER calls `app.Logger()`.
     This is enforceable by a lint rule / forbidigo entry.
  2. PocketBase's own framework logs are suppressed in production by setting
     `Settings().Logs.MaxDays = 0` — verified: `WriteFunc` returns early when
     `app.Settings().Logs.MaxDays == 0`, and `BeforeAddFunc` returns
     `app.Settings().Logs.MaxDays > 0`, so nothing is retained or written.
  3. To actually CAPTURE PB's internal logs in zerolog, the only non-fork route
     is to keep `_logs` writes enabled and mirror the `Log` aux model into
     zerolog from a model hook. This is roundabout and contradicts (2) — pick
     one, do not claim both.
- `getLoggerMinLevel` returns -99999 in dev mode, so dev builds print
  everything to the console regardless of settings.

### CORRECTION — the bridge IS achievable. Two mechanisms, and both are needed.

An earlier revision of this file concluded the bridge was "unachievable without
a fork" and that MaxDays=0 was the answer. Both halves were wrong.

**(a) Decorate the exported embedded interface.** `pocketbase.go` declares:

```go
type PocketBase struct {
	core.App              // <- EXPORTED embedded interface field
	...
	RootCmd *cobra.Command
}
```

so `pb.App = &loggedApp{App: pb.App, logger: zerologBackedSlog}` is legal and
redirects the entire request path — `Start()` passes `pb` itself into the serve
command, and `apis.NewRouter` does `event.App = app`. Verified SAFE: grepping
all non-test v0.40.1 source for `.(*BaseApp)` / `.(*core.BaseApp)` type
assertions returns **ZERO hits**, so nothing downcasts past the decorator.

**(b) Intercept the `_logs` write.** The decorator does NOT cover everything:
`core/db_tx.go` `createTxApp` does `clone := *app` on a `*BaseApp`, so a
transaction-scoped app is a bare `*BaseApp` whose `Logger()` returns the
hardcoded internal logger. Those lines bypass (a). Bind
`OnModelCreate("_logs")` to emit into zerolog and return WITHOUT calling
`e.Next()`, so the INSERT never executes and `_logs` stays permanently empty.
(`core/log_model.go` `TableName()` returns `"_logs"`.)

**Set `Logs.MaxDays = 1`, NOT 0.** At 0, `BeforeAddFunc` returns
`MaxDays > 0` == false and the record never enters the batch at all — so
mechanism (b) never fires and PocketBase's backup failures, mailer failures,
cron errors and OAuth2 failures go to **nowhere** in production (`printLog`
only runs when `IsDev()`). MaxDays=1 keeps the pipeline alive so (b) can
intercept it; the interception prevents any row ever being written.

Cost: roughly 3s of batch latency on path (b), and a PocketBase-upgrade
checklist item to re-verify both mechanisms.

---

## FACT 2 — Locking down the auto-collections API is clean, but the naive prefix block breaks auth.

`apis.NewRouter` (apis/base.go:19) hardcodes every built-in binding with
**no opt-out flag**:

```go
apiGroup := pbRouter.Group("/api")
bindSettingsApi(app, apiGroup); bindCollectionApi(app, apiGroup)
bindRecordCrudApi(app, apiGroup); bindRecordAuthApi(app, apiGroup)
bindLogsApi(app, apiGroup); bindBackupApi(app, apiGroup)
bindCronApi(app, apiGroup); bindFileApi(app, apiGroup)
bindBatchApi(app, apiGroup); bindRealtimeApi(app, apiGroup)
bindHealthApi(app, apiGroup); bindSQLApi(app, apiGroup)
```

Record **CRUD** routes (apis/record_crud.go) — the ones to lock down:

```
GET    /api/collections/{collection}/records
GET    /api/collections/{collection}/records/{id}
POST   /api/collections/{collection}/records
PATCH  /api/collections/{collection}/records/{id}
DELETE /api/collections/{collection}/records/{id}
```

Record **AUTH** routes (apis/record_auth.go) — these MUST stay reachable and
live under the SAME `/api/collections/` prefix:

```
GET  /api/collections/{collection}/auth-methods
POST /api/collections/{collection}/auth-refresh
POST /api/collections/{collection}/auth-with-password
POST /api/collections/{collection}/auth-with-oauth2
POST /api/collections/{collection}/request-otp
POST /api/collections/{collection}/auth-with-otp
POST /api/collections/{collection}/request-password-reset
POST /api/collections/{collection}/confirm-password-reset
POST /api/collections/{collection}/request-verification
POST /api/collections/{collection}/confirm-verification
POST /api/collections/{collection}/request-email-change
POST /api/collections/{collection}/confirm-email-change
POST /api/collections/{collection}/impersonate/{id}   (superuser only)
GET|POST /api/oauth2-redirect
```

**A blanket 403/404 on `/api/collections/*` would kill PocketBase native auth,
which is the locked auth decision.** The lockdown must therefore be either:

- (preferred, idiomatic) set all five API rules (list/view/create/update/delete)
  to `nil` on every collection => superuser-only. Auth routes consult
  `authRule`/`manageRule`, NOT the CRUD rules, so login keeps working and the
  superuser admin UI keeps working; or
- (belt and braces) additionally bind a middleware that rejects the exact
  `/api/collections/{collection}/records` subtree plus `/api/batch` for
  non-superusers.

Note `bindRecordCrudApi` calls `.Unbind(DefaultRateLimitMiddlewareId)` on its
group, so the CRUD subtree is NOT rate-limited by default.

---

## FACT 3 — gzip is NOT bound by default, so the SSE/gzip fear is moot.

Default middlewares bound by `apis.NewRouter`, in order:
`activityLogger, panicRecover, rateLimit, loadAuthToken,
superuserIPsWhitelist, securityHeaders, BodyLimit(DefaultMaxBodySize)`.

**Gzip is absent.** It is opt-in via `apis.Gzip()` / `apis.GzipWithConfig()`.
Even if enabled, `gzipResponseWriter.Flush()` (apis/middlewares_gzip.go:189)
flushes the gzip.Writer and then calls
`http.NewResponseController(w.ResponseWriter).Flush()`, so streaming still
flushes correctly. Additionally datastar-go ships its own
`sse-compression.go`. Recommendation: do NOT bind PB's gzip globally; if
compression is wanted, bind it per-group and exclude SSE routes.

---

## FACT 4 — Datastar v1 API surface (v0.x names are DEAD).

`datastar/consts.go` defines exactly TWO event types:

```go
EventTypePatchElements EventType = "datastar-patch-elements"
EventTypePatchSignals  EventType = "datastar-patch-signals"
```

The v0.x names (`datastar-merge-fragments`, `datastar-merge-signals`,
`datastar-remove-fragments`, `datastar-remove-signals`,
`datastar-execute-script`) **no longer exist**. Any tutorial using them is
stale. `ExecuteScript` and `RemoveElement` are now implemented ON TOP of
`PatchElements`.

`ElementPatchMode` values: `outer` (default), `inner`, `remove`, `replace`,
`prepend`, `append`, `before`, `after`.

Verified public API:

```go
func NewSSE(w http.ResponseWriter, r *http.Request, opts ...SSEOption) *ServerSentEventGenerator
func ReadSignals(r *http.Request, signals any) error
func (sse *ServerSentEventGenerator) PatchElements(elements string, opts ...PatchElementOption) error
func (sse *ServerSentEventGenerator) PatchElementTempl(c TemplComponent, opts ...PatchElementOption) error
func (sse *ServerSentEventGenerator) PatchElementf(format string, args ...any) error
func (sse *ServerSentEventGenerator) RemoveElement(selector string, opts ...PatchElementOption) error
func (sse *ServerSentEventGenerator) RemoveElementByID(id string) error
func (sse *ServerSentEventGenerator) PatchSignals(signalsContents []byte, opts ...PatchSignalsOption) error
func (sse *ServerSentEventGenerator) MarshalAndPatchSignals(signals any, opts ...PatchSignalsOption) error
func (sse *ServerSentEventGenerator) MarshalAndPatchSignalsIfMissing(signals any, opts ...PatchSignalsOption) error
func (sse *ServerSentEventGenerator) ExecuteScript(scriptContents string, opts ...ExecuteScriptOption) error
func (sse *ServerSentEventGenerator) ConsoleLog / ConsoleLogf / ConsoleError
func (sse *ServerSentEventGenerator) Redirect / Redirectf
func (sse *ServerSentEventGenerator) DispatchCustomEvent(eventName string, detail any, ...) error
func (sse *ServerSentEventGenerator) ReplaceURL(u url.URL, ...) / ReplaceURLQuerystring
func (sse *ServerSentEventGenerator) Prefetch(urls ...string) error
func (sse *ServerSentEventGenerator) Context() context.Context
func (sse *ServerSentEventGenerator) IsClosed() bool
func (sse *ServerSentEventGenerator) Send(eventType EventType, dataLines []string, ...) error
```

**templ integration is first-class**: `PatchElementTempl` accepts a
`TemplComponent` *interface* (elements-sugar.go:150), so datastar-go does not
hard-depend on templ. `NewSSE` takes plain `http.ResponseWriter` +
`*http.Request`, and PocketBase's `core.RequestEvent` exposes `.Response` and
`.Request` — so Datastar drops into a PB handler with **zero glue**.

`ConsoleError`/`ConsoleLog` deliberately write to the browser console. Given
the Playwright gate asserts ZERO console errors, the spec must forbid
`ConsoleError` in production code paths or the gate will fight itself.

---

## FACT 5 — Custom realtime publishing is available.

`core/base.go:632` exposes `func (app *BaseApp) SubscriptionsBroker() *subscriptions.Broker`,
so MediGo can publish custom messages to PB realtime subscribers, not just
record-change events. However PB's realtime wire format is its own JSON
envelope on `/api/realtime` — it is **NOT** the `datastar-patch-*` SSE format,
so a browser cannot point Datastar at PB's realtime stream directly. Any
"live" UI needs a MediGo-owned SSE endpoint that translates PB record hooks
into Datastar patch events.

## FACT 6 — Plugins and Cobra.

`plugins/` contains `migratecmd`, `jsvm`, `ghupdate`.
`migratecmd.MustRegister(app core.App, rootCmd *cobra.Command, config Config)`
confirms PocketBase's RootCmd is a real `*cobra.Command`, so registering custom
Cobra subcommands (`medigo seed`, `medigo routes`, `medigo openapi`) is native
and needs no extra dependency. **jsvm must be left unregistered** — MediGo is
Go-only and must not ship a JS runtime.

`apis.WrapStdHandler(h http.Handler)` and
`apis.WrapStdMiddleware(m func(http.Handler) http.Handler)` exist, which is how
any stdlib-shaped middleware (OTel, Prometheus promhttp, Sentry) mounts onto
PB's router.

`core.ServeEvent` exposes: App, Router, Server, CertManager, Listener,
InstallerFunc, UIExtensions. Setting `InstallerFunc = nil` skips PB's
first-run installer page — relevant for deterministic Playwright fixtures.

---

## FACT 7 — Testing harness, verified.

`tests.NewTestApp(optTestDataDir ...string)` -> `NewTestAppWithConfig`, which
calls `TempDirClone(config.DataDir)` and then `app.Bootstrap()`. So **every
test app is a filesystem clone of a fixture data dir into a temp dir**. That
means:

- Tests ARE isolated from each other and from the fixture, so `t.Parallel()` is
  safe.
- The cost per test app is a directory copy plus a SQLite bootstrap — cheap but
  not free. Unit tests of service logic MUST therefore use fakes, not a test
  app; a test app is for adapter/integration and HTTP tests only. This is what
  Principle III's pyramid encodes.
- MediGo needs its own committed fixture data dir (migrated schema + seed
  superuser) for `NewTestApp` to clone.

`tests.ApiScenario` fields (tests/api.go):
`Name, Method, URL, Body, Headers, Delay, Timeout, DisableTestAppCleanup,
ExpectedStatus, ExpectedContent, NotExpectedContent, ExpectedEvents,
TestAppFactory, BeforeTestFunc, AfterTestFunc`.

`ExpectedEvents map[string]int` asserts which hooks fired and how many times —
useful for proving that a custom /api/v1 route did NOT trigger record CRUD
hooks it should not, and for proving audit-trail hooks DID fire.

## FACT 8 — Migrations are reversible by construction.

`migrations.Register(up func(core.App) error, down func(core.App) error, optFilename ...string)`
(migrations/1640988000_init.go:14) — the signature REQUIRES both an up and a
down function. Principle IX's reversibility rule is therefore enforced by the
API itself; the only escape is passing a `down` that returns nil, which is what
the "document the irreversibility in the migration file" clause covers.

`core.AppMigrations` is the user-defined migration list;
`core.SystemMigrations` is PocketBase's own. MediGo registers into
`AppMigrations` only.

---

## FACT 9 — RISK R1 IS CLOSED. The discriminated record family is generatable and gateable.

SHARED-DESIGN.md §8 names R1 as "the one risk that could invalidate a phase":
if a discriminated `oneOf` cannot be emitted and gated, the record route family
collapses into per-kind routes, the operation count goes from 90 to ~150, and
phase 003 grows from 3 routes to 62. It assigned the proof to phase 001.

**It has been proven ahead of phase 001, by building and running it.**

Generator: `github.com/getkin/kin-openapi/openapi3` **v0.144.0**, which is
pure Go, cgo-free, and adds no HTTP framework — so it does not collide with the
constitution's ban on a second router or OpenAPI-serving framework. It models
the OpenAPI 3 object graph directly, which suits MediGo exactly, because
PocketBase's route table is not introspectable (FACT: `apis.NewRouter` exposes
no route list) and MediGo must therefore keep a single declarative route
registry anyway. The document is BUILT from that registry rather than reflected
out of the router.

Verified end to end:

```
OpenAPI document VALIDATES after marshal -> load round trip
document bytes: 1212
GATE PASSES: all 2 registered kinds present in discriminator mapping
discriminator: kind map[allergies:... emergency-contacts:...]
```

What the probe actually did, which is the shape phase 001 should implement:

1. Declared a registry of record kinds, each carrying its own fully typed
   object schema — `allergies` (allergen, severity enum) and
   `emergency-contacts` (full_name, relationship).
2. Emitted one component schema per kind, then a single `Record` schema whose
   `oneOf` lists them and whose `discriminator` is `{propertyName: "kind",
   mapping: {...}}`.
3. Registered ONE path, `POST /api/v1/records/{kind}`, whose `kind` path
   parameter is an `enum` of the registered kind names and whose request and
   response bodies both `$ref` the `Record` union.
4. Marshalled the document, RELOADED it through `openapi3.NewLoader()` the way
   any consumer would, and validated the loaded form. This round trip matters:
   validating the in-memory document fails with "found unresolved ref", because
   a programmatically built `SchemaRef` carries only `Ref` and no `Value`.
   **The gate must marshal-then-load, not validate in place.**
5. Asserted that every registered kind appears in the discriminator mapping —
   the Principle IX gate. A kind added to the registry without a schema fails
   the build.

API notes for whoever writes this, both of which cost a compile error first:

- `Discriminator.Mapping` is `map[string]openapi3.MappingRef`, NOT
  `map[string]string`.
- `MappingRef` is a struct (an alias of `SchemaRef`), so the value is
  `openapi3.MappingRef{Ref: "#/components/schemas/Record_allergies"}`, not a
  bare string. It marshals to a plain string via `MarshalText`.

**Consequence: the 90-operation budget and phase 003's 3-route shape hold.**
Phase 001 should still carry the task — with the two kinds above — as a
regression gate rather than as an open question, and R1 should be recorded as
CLOSED with this evidence rather than left open in the risk register.

---

## FACT 10 — RISK R6 IS CLOSED, and the two settings live in DIFFERENT places.

The constitution requires MediGo to warn loudly at boot when superuser MFA or
the superuser IP allowlist is unconfigured (Principle VII, following the
"admin UI ships in production, hardened" decision). SHARED-DESIGN.md R6 left it
unverified. Both are readable from Go in v0.40.1 — but a plan that assumes they
sit together will be wrong.

**IP allowlist — on GLOBAL settings.**
`core/settings_model.go:125` — `` SuperuserIPs []string `json:"superuserIPs"` ``.
Boot check: `len(app.Settings().SuperuserIPs) == 0` -> warn.
It is validated as `IPOrSubnet`, so CIDR ranges are accepted.
It is also enforced in two places already seen in this codebase: the router's
`superuserIPsWhitelist()` middleware, and `apis/file.go`, which drops superuser
auth on a file request from a non-allowlisted IP.

**MFA — on the SUPERUSERS AUTH COLLECTION, not on settings.**
`core/collection_model_auth_options.go:348`:

```go
type MFAConfig struct {
	Enabled  bool   `json:"enabled"`
	Duration int64  `json:"duration"` // seconds
	Rule     string `json:"rule"`     // optional filter; empty = everyone
}
```

reached via the auth collection's `MFA MFAConfig` field (line 132). Boot check:
find the superusers collection by `core.CollectionNameSuperusers` and read
`collection.MFA.Enabled`.

Two consequences worth putting in the plan:

- PocketBase refuses to enable MFA unless the collection has **at least two auth
  methods** enabled (`validation_mfa_not_enough_auths`, line ~201). So "turn on
  superuser MFA" is not a single toggle — the instance must also have a second
  method configured, and MediGo's boot warning should say so rather than just
  reporting MFA off.
- `MFAConfig.Rule` being empty means MFA applies to everyone, which is what
  MediGo wants for superusers. A non-empty rule is a partial rollout and should
  ALSO trigger the warning, since it means some superuser can sign in without
  a second factor.

---

## FACT 11 — RISK R3 IS CLOSED. FTS5 IS available, and so are the JSON functions.

SHARED-DESIGN.md R3 recorded FTS5 availability in the vendored SQLite as
"Unconfirmed", and hedged that if it were absent, search would degrade to `LIKE`
over `search_index` with date ordering and "MediGo must not claim relevance
ranking". That hedge is unnecessary.

Tested against `modernc.org/sqlite v1.57.0` — the exact version PocketBase
v0.40.1 pulls transitively — on Go 1.27:

```
compile_options contains FTS5  : true
compile_options contains FTS4  : false
compile_options contains FTS3  : false

CREATE VIRTUAL TABLE search_index USING fts5(kind, title, body)   -- OK
SELECT kind, title, rank FROM search_index
  WHERE search_index MATCH 'antibiotic' ORDER BY rank             -- OK, 2 rows ranked
```

**FTS5 works, including `MATCH` and the `rank` column**, so MediGo's unified
search CAN legitimately claim relevance ranking and phase 004's spec does not
need the `LIKE` fallback hedge. FTS3 and FTS4 are not compiled in, which is
irrelevant — FTS5 supersedes both and is the one to use.

The JSON functions are also present, despite `JSON` not appearing in
`compile_options` (in modern SQLite the JSON1 functions are built in rather than
being a compile-time option, so their absence from that list is expected and is
NOT evidence they are missing):

```
SELECT json_extract('{"a":{"b":"ok"}}','$.a.b')   -> "ok"
SELECT count(*) FROM json_each('[1,2,3]')         -> 3
```

This matters beyond search: PocketBase's `json` field type and its filter
resolver rely on these, and MediGo's own report-template storage and any
multi-relation filtering will too.

Caveat worth carrying into the plan: an FTS5 virtual table is a SEPARATE table
that must be kept in step with the record collections. PocketBase will not do
that for you, and its migrations do not model virtual tables — so the
`search_index` table is created by a raw SQL migration and maintained by
post-commit record hooks, with a `medigo reindex` subcommand for rebuilds.
Deleting a record MUST delete its index row, or search leaks the titles of
deleted records, which is a Principle VII problem.
