# MediKube — Dossier Reconciliation

**Date:** 2026-08-26
**Inputs:** `domain-clinical.md`, `domain-platform.md`, `pocketbase.md`, `frontend.md`,
`observability.md`, `conventions.md`, `testing.md`, plus the two index files
`VERIFIED-SOURCE-FACTS.md` and `HOUSE-PATTERNS.md`.
**Also read:** `/Users/krzysztof.wiatrzyk/private/monorepo/medikube/.specify/memory/constitution.md`
(the only file that currently exists under `medikube/` — the directory is otherwise empty).

**Method.** Every load-bearing claim below was re-verified against the actual v0.40.1 source in
the module cache (`/home/agent/go/pkg/mod/github.com/pocketbase/pocketbase@v0.40.1`), the
`datastar-go@v1.2.2` module, `arc-ui/go.mod`, and the monorepo's `.dockerignore` /
`build-image.yaml`. Where a dossier and the source disagreed, the source won and it is called
out. No Go toolchain is installed in this sandbox, so nothing was compile-verified here;
`testing.md` did run `go mod tidy` in an environment that had one, and that result is treated as
primary evidence.

**Posture.** This document is adversarial. Its job is to find where the plan is wrong. Section 1
is the contradiction register; §2 drops libraries; §3 is the confirmed stack; §4 is what the user
must decide; §5 is the phase plan. Non-contradictions that were checked and cleared are recorded
in §6 so nobody re-litigates them.

---

## 1. Contradiction register

Ordered most severe first. Severity means: **blocker** = a locked decision cannot be implemented
as written and code must not be started until it is amended; **friction** = implementable but the
plan is wrong about the cost or the mechanism; **minor** = a trap that will cost a day if
undocumented.

---

### C1 — Go 1.26.5 cannot build PocketBase v0.40.1 — **BLOCKER**

Two locked decisions ("Go 1.26.5" and "PocketBase v0.40.1 embedded") are mutually exclusive.

**Evidence.**

- PB v0.40.1 `go.mod` line 3: `go 1.27`. Verified by direct read.
- `grep -rl '"encoding/json/v2"' --include=*.go | grep -v _test.go | wc -l` → **67 non-test
  files**, of which **15 are in `core/` and `apis/`** — i.e. unavoidable, on the main import
  path, with no build tags and no fallback. A further 11 files import
  `encoding/json/jsontext`.
- `testing.md` §0 actually executed it: `go: github.com/pocketbase/pocketbase@v0.40.1 requires
  go >= 1.27 (running go 1.26.5)`. Under Go 1.27.0 the identical module tidied, built and tested
  clean.
- `pocketbase.md` §0 quotes the v0.40.0 CHANGELOG: *"Bumped the min Go version to 1.27.0 and
  migrated to the new `encoding/json/v2` package."*
- `conventions.md` (lines 179–196) locks `go 1.26.5` because every recent sibling
  (`appbase`, `arc-ui`, `gmod`, `go-modules`, `technologia`, `claudy`, `sre-agent`) is on it —
  confirmed by reading `arc-ui/go.mod`.
- The constitution's Technology Constraints say "Go 1.26.5".

**`VERIFIED-SOURCE-FACTS.md` is wrong here.** Its table asserts PB v0.40.1 "resolves on go
1.26.5". That conflates *module resolution* (downloading a zip into the module cache, which
indeed works) with *building*. It does not build. Two other dossiers independently caught this;
the index file should be corrected so it stops contradicting them.

**Resolution.** Move MediKube to **Go 1.27.x**. Concretely:

- `go.mod`: `module medikube` / `go 1.27` / `toolchain go1.27.x`.
- Dockerfile `ARG GO_VERSION=1.27` (`conventions.md` line 512 currently says 1.26).
- golangci-lint **v2** at a release that understands Go 1.27 (`arc-ui`'s Taskfile already notes
  "v1 does not understand Go 1.26" — the same reasoning applies one version up).
- MediKube will be the **only** project in the monorepo not on 1.26.5. That is a deliberate,
  documented divergence, not drift. Note `medi-keep-go` documented a `GOTOOLCHAIN=local` trap
  (`conventions.md` line 390) — with a `toolchain` directive present, CI must **not** set
  `GOTOOLCHAIN=local` or the build fails with a confusing "built with go1.26 < targeted go1.27".
- Budget real time for the knock-on: Go 1.27 retrofits v1 `encoding/json` onto v2 and the
  CHANGELOG explicitly warns it is *not fully backward compatible*. This affects **MediKube's own
  DTO marshalling**, not just PocketBase — nil-vs-empty slices, `json.RawMessage`, duplicate
  keys, case-insensitive field matching. `tests.ApiScenario` normalises bodies through
  `jsontext` before substring matching, so `ExpectedContent` compares against *re-encoded* JSON.

**Fallback if 1.27 is immovable:** pin PocketBase to v0.39.x. This is strictly worse — it
forfeits v0.40's `filesystem.NewWriter`/`OnNewWriter`/`OnDelete` hooks, `Record.GetInt64`, the
log data-size cap and the backup improvements, and it puts MediKube on a branch upstream will stop
patching. It also **invalidates every file:line citation in `pocketbase.md` and
`observability.md`**, which would need a full re-verification pass. Escalate before any code is
written.

---

### C2 — The auto-API lockdown and PocketBase native realtime are mutually exclusive — **BLOCKER**

Locked decision: *"use PocketBase's native realtime where PocketBase natively supports it."*
Under the equally-locked auto-API lockdown, **PocketBase natively supports it nowhere** for
MediKube's users.

**Evidence.**

- `apis/realtime.go:605-613` builds its authorization map directly from the CRUD rules:

  ```go
  subscriptionRuleMap := map[string]*string{
      (collection.Name + "/" + record.Id + "?"): collection.ViewRule,
      (collection.Name + "/*?"):                 collection.ListRule,
      ...
  }
  ```

- `core/record_query.go:599-612` — `CanAccessRecord`: superuser → true; **`accessRule == nil` →
  false**; `*accessRule == ""` → true.
- Therefore `ListRule = ViewRule = nil` (the lockdown) means every broadcast is **skipped** in
  the fan-out loop for non-superusers. Not an error. Not a 403 on the stream. **Silent
  nothing** — the worst possible failure mode, and hours of debugging.
- `frontend.md` §6 independently shows that even *if* the rules were open, Datastar cannot
  consume PB's realtime: PB emits `event:medications/abc123` with a
  `{"action","record"}` JSON body, and Datastar only registers watchers for
  `datastar-patch-elements` / `datastar-patch-signals` (verified in
  `datastar-go@v1.2.2/datastar/consts.go:66-68` — exactly two event types) and silently drops
  everything else. Independently, PB requires a **two-step handshake**: `GET /api/realtime`
  yields a `clientId` in a `PB_CONNECT` event, and you must then `POST /api/realtime` echoing it.
  Datastar's `@get` has no mechanism to parse a custom event, extract an id and fire a follow-up
  POST.

So there are *three* independent reasons this locked decision cannot be implemented: rules,
event names, handshake.

**Resolution.** **Strike native realtime from the locked decisions. Datastar SSE is 100% of
MediKube's realtime.** This is a simplification, not a loss — native realtime broadcasts *raw
collection records*, which is precisely the abstraction the hand-written-DTO decision is paying
to avoid; shipping it would leak internal schema to the browser through a second transport with
a second authorization model.

The bridge (from `frontend.md` §6, sound as designed): `OnRecordAfterCreateSuccess` /
`...UpdateSuccess` / `...DeleteSuccess` (which fire *after commit*, so you never render a
rolled-back row) → a non-blocking in-process hub → a per-subscriber SSE handler that
**re-fetches by ID and re-authorises per event** before rendering templ and emitting
`PatchElementTempl`. **Fan out IDs, never record bodies** — that is exactly what makes
per-subscriber authorization possible, and it is the single most important design rule in the
realtime layer.

Amend the constitution: it currently says PocketBase "owns HTTP routing, persistence, auth, file
storage, **realtime**, and the superuser admin UI". Delete `realtime` from that list.

**Carry-forward constraint:** the hub is in-process, so MediKube is **single-instance only**. See
open question Q4.

---

### C3 — Non-protected file fields are served to unauthenticated strangers — **BLOCKER**

This is a Principle VII (Patient Privacy) violation of the data-breach class, and the lockdown
does nothing about it.

**Evidence.** `apis/file.go:109` — verified by direct read:

```go
// check whether the request is authorized to view the protected file
if fileField.Protected {
    ...
    if ok, _ := e.App.CanAccessRecord(record, &requestInfo, record.Collection().ViewRule); !ok {
        return e.NotFoundError("", errors.New("insufficient permissions to access the file resource"))
    }
}
```

**There is no `else`.** For a `FileField` with `Protected: false`,
`GET /api/files/{collection}/{recordId}/{filename}` serves the blob to anyone — no token, no
auth, no rule evaluation. The only protection is a random 10-character filename suffix, i.e.
security through obscurity. For lab PDFs, scans and insurance documents, a leaked or
proxy-logged URL becomes a permanent public link.

**And the mirror-image problem:** with `ViewRule = nil` (the lockdown), the *protected* branch
returns **404 for every non-superuser**. So PB's built-in file route is simultaneously too open
(unprotected fields) and completely closed (protected fields). It is unusable either way.

**`domain-platform.md` is internally contradictory here and its top-5 claim is wrong.** §A2 /
§D1 recommend leaning on "PB file fields + thumbnails + **protected file tokens**" to delete 34
operations — while the *same document* (§10.7) lists `?token=<jwt>` in a URL as a PHI leak to be
fixed. PB's protected-file token mechanism **is** `?token=<jwt>` in the URL. You cannot adopt it
and condemn it.

**Resolution.**

1. **`Protected: true` on every single `FileField`. No exceptions.** Assert it at boot alongside
   the API-rule assertion; refuse to start otherwise.
2. **Serve every file from a hand-written `/api/v1` route**, authorised by the Go service layer,
   using `app.NewFilesystem()` + `fsys.Serve(...)` (which handles Range, ETag, Content-Type and
   Content-Disposition; v0.40 added quoting so patient-supplied filenames are safe to pass).
   `defer fsys.Close()` always.
3. **Never use `POST /api/files/token`.** Your session token is already on the request.
4. **Thumbnails do not generate themselves once you bypass `/api/files/`.** PB generates thumbs
   lazily inside that built-in handler. Call `fsys.CreateThumb(originalKey, thumbKey, size)`
   yourself, eagerly on upload, in a `TxInfo().OnComplete` callback so it does not extend the
   write transaction.
5. Optionally 404 `/api/files/` for non-superusers in the path-prefix middleware.

**Budget correction.** `domain-platform.md` §C puts Files at "0–2" endpoints. With PB's file
route unusable it is realistically **4–5** (upload, download, thumbnail, delete, and a list).
Still a 34→5 collapse — the win is real, the number was optimistic.

**Note also** `filesystem.NewFileFromURL` is a textbook SSRF sink (`pocketbase.md` §9). The
monorepo has already been bitten by exactly this pattern (`fix(technologia): block SSRF in logo
uploads`). If MediKube ever ingests by URL, validate the resolved IP against private/link-local
ranges *after* DNS resolution and never follow redirects blindly.

---

### C4 — `Logs.MaxDays = 0` silently destroys every PocketBase internal log — **BLOCKER**

The locked decision says both *"a slog.Handler bridges PocketBase's internal logs into zerolog"*
**and** *"PB's `_logs` collection is disabled"*. At `MaxDays = 0` these two clauses are in direct
conflict, and the constitution has already picked the losing side.

**Evidence.** `core/base.go:1472-1536` `initLogger()`, verified by direct read:

```go
BeforeAddFunc: func(ctx context.Context, log *logger.Log) bool {
    if app.IsDev() { printLog(log); ... }
    ticker.Reset(duration)
    return app.Settings().Logs.MaxDays > 0        // <- the kill switch
},
WriteFunc: func(ctx context.Context, logs []*logger.Log) error {
    if !app.IsBootstrapped() || app.Settings().Logs.MaxDays == 0 { return nil }
    ...
},
```

With `MaxDays = 0` the log never even enters the batch. In production (`IsDev() == false`)
`printLog` does not run either. So PocketBase's internal logs go **nowhere**: backup failures,
mailer failures, cron errors, OAuth2 failures, WAL checkpoint warnings — all silently dropped.
`observability.md` §2.2 calls this "unacceptable for a medical records app" and is right.

Constitution Principle VI currently mandates exactly this:
> PocketBase's log persistence MUST be disabled (`Settings().Logs.MaxDays = 0`).

**The two dossiers disagree on the fix, and both are half right.**
`pocketbase.md` §13 proposes decorating `pb.App`; `observability.md` §2.3 proposes intercepting
the `_logs` model write; `VERIFIED-SOURCE-FACTS.md` FACT 1 declares the bridge "not achievable"
and says *"pick one, do not claim both"*. **All three are wrong to frame it as either/or.** The
two mechanisms cover *disjoint* sets of log lines and compose cleanly.

**Resolution — apply BOTH.**

**(a) Decorate the embedded `core.App` interface field.** Verified: `pocketbase.go:33-44`
declares `type PocketBase struct { core.App; ...; RootCmd *cobra.Command }` — an **exported
embedded interface field named `App`**. So:

```go
pb.App = &LoggedApp{App: pb.App, logger: slog.New(zerolog.NewSlogHandler(zl))}
```

redirects `Logger()` for the whole request path. Verified end to end:
`Start()` (`pocketbase.go:170`) does `cmd.NewServeCommand(pb, ...)` passing **`pb` itself**;
`apis/base.go:24` does `event.App = app`. Because `pb` is a pointer and `App` is a mutable
field, `pb.Logger()` resolves through the decorator dynamically at call time. **Verified safe:**
`grep` for `.(*BaseApp)` / `.(*core.BaseApp)` / `.(*pocketbase.PocketBase)` across all non-test
v0.40.1 code returns **zero hits**, so no PB internal type-asserts past the wrapper.
Covers: `activityLogger`, `panicRecover`, and every `e.App.Logger()` call in `apis/*` — 95%+ of
volume, synchronously, with request correlation.

**(b) Set `Logs.MaxDays = 1` (NOT 0) and intercept the `_logs` write.** Verified:
`core/log_model.go:31` `func (l *Log) TableName() string { return LogsTableName }` where
`LogsTableName = "_logs"`; `WriteFunc` persists via `txApp.AuxSave(model)` →
`(*BaseApp).save` → `(*BaseApp).create`, which triggers the **tagged** `OnModelCreate()` hook
(`core/db.go`). So:

```go
app.OnModelCreate("_logs").BindFunc(func(e *core.ModelEvent) error {
    emitToZerolog(e.Model)
    return nil          // deliberately NOT e.Next() -> the INSERT never happens
})
```

The `_logs` table stays permanently empty — the locked "disable `_logs`" intent is satisfied —
while the lines still reach zerolog. Covers the tail (b) exists for: `BaseApp`-internal warnings,
and anything inside `RunInTransaction`, because `core/db_tx.go:53` does `clone := *app` on the
**`*BaseApp`**, not on your wrapper, so `txApp.Logger()` bypasses (a).

**Costs, stated honestly:** the (b) path batches on a 3-second ticker, loses source PC and
request context, and is a workaround over an unexported field that must be **re-verified on
every PocketBase upgrade**. Write that into the spec as an upgrade checklist item.

**Constitution amendment required:** Principle VI's `MaxDays = 0` must become
`MaxDays = 1` + the interceptor, with the rationale recorded. Everything else in Principle VI
(zerolog only, `forbidigo` on `.Logger()`, bounded metric labels, no `ConsoleLog`/`ConsoleError`)
stands and is well-founded.

`conventions.md` (line 1011-1017) flagged a contradiction between the brief and the constitution
and concluded "the constitution is the one with the verified evidence". **That conclusion is
wrong** — the constitution's `MaxDays = 0` is the part that does not survive contact with
`initLogger`. The brief's "bridge" is achievable; it just takes two mechanisms rather than the
one non-existent injection point everybody looked for.

---

### C5 — `domain-platform`'s "API rules as defence-in-depth" reopens the auto-CRUD API — **FRICTION**

**The conflict.** `domain-platform.md` §A5 proposes:

```
// patients.viewRule
owner = @request.auth.id ||
@collection.shares.patient ?= id && @collection.shares.grantee ?= @request.auth.id && ...
```

and §D4 repeats it ("mirrored into PB API rules as defence-in-depth"). `pocketbase.md` §7
requires **all five rules `nil`** plus a boot assertion that *fails startup* on any non-nil rule.
These cannot both be true.

**Resolution: `nil` rules win. Fail closed.** A `viewRule` that expresses sharing does not add
defence in depth — it **re-opens `GET /api/collections/patients/records`** to every
authenticated user, returning raw collection records with no DTO boundary. That is precisely
what the locked API design exists to prevent. A non-nil rule is not a safety net; it is a second,
undocumented, un-versioned public API.

- Authoritative authorization lives in a Go `Authorizer` interface consulted by every service —
  which the locked decisions already require ("interfaces at every seam", "backed by
  interface-defined services").
- Defence in depth comes from three cheap mechanisms instead: the **boot assertion** (refuse to
  start with a non-nil rule on any non-system collection), the **path-prefix 404 middleware** at
  priority `-1019` (after `loadAuthToken` at `-1020`, so `e.Auth` is populated), and a
  **per-collection `tests.ApiScenario`** asserting the auto CRUD route 404s for a normal user.
  Write that scenario for *every* collection; it is the regression test for the whole design.
- **The read-time-expiry benefit is preserved**, just not by rules. Put
  `(expires_at = '' || expires_at > {:now}) && revoked_at = ''` in the repository's
  share-resolution query. `domain-platform.md`'s real insight — that expiry should be a filter
  in the read path, not a cron sweep — survives intact and still deletes
  `patient-sharing/cleanup-expired` and `invitations/cleanup` from the correctness path.

**Also confirmed while checking this:** `/api/batch` is a transactional multiplexer that re-uses
the same CRUD handler bodies, so nil rules *do* cover it (the boolean those constructors take is
`responseWriteAfterTx`, not a rules bypass). Keep `Settings().Batch.Enabled = false` anyway — it
is a transaction-holding surface with a ~128MB default body limit and no benefit here — and
assert it at boot.

---

### C6 — "Every public endpoint is a hand-written `/api/v1` route" collides with PB native auth's URLs — **FRICTION**

Two locked decisions rub: *"Every public endpoint is a hand-written Go route under `/api/v1`"*
and *"Auth: PocketBase NATIVE auth ... PB's built-in OAuth2 providers for SSO"*.

**Evidence.** PB's auth endpoints live under the **same `/api/collections/` prefix** as the CRUD
routes being locked down (`apis/record_auth.go`): `auth-with-password`, `auth-with-oauth2`,
`auth-refresh`, `request-otp`, `auth-with-otp`, `request-password-reset`,
`confirm-password-reset`, `request-verification`, `confirm-verification`,
`request-email-change`, `confirm-email-change`, plus the global `GET|POST /api/oauth2-redirect`.
`pocketbase.md` §8 notes MFA/OTP are woven through `apis/record_auth_*.go` with their own events
and **cannot be cheaply re-implemented**.

**This is also why the naive lockdown is dangerous.** A blanket 403/404 on `/api/collections/*`
would kill PB native auth outright. `VERIFIED-SOURCE-FACTS.md` FACT 2 gets this right and it is
the single most important implementation detail of the lockdown middleware.

**Resolution.**

- Hand-write `/api/v1/auth/login`, `/refresh`, `/logout` with MediKube DTOs, completing the flow
  through `apis.RecordAuthResponse(e, rec, core.RequestInfoContextPasswordAuth, nil)` — exported,
  and the supported way to finish an auth flow from a custom route (it mints the token, fires
  `OnRecordAuthRequest`, and records the auth origin).
- **Explicitly exempt and enumerate** the PB-native paths that remain public: `auth-with-oauth2`,
  `/api/oauth2-redirect`, and the OTP/MFA endpoints if MFA is enabled. These go into the route
  registry and the OpenAPI document as a **named, documented exception**, not as an accident.
- The lockdown middleware must match only paths containing `/records` — verified this leaves
  every auth route untouched.
- `AuthRule` is **not** one of the five CRUD rules. Setting `ListRule = nil` etc. does **not**
  disable login. Keep `AuthRule = types.Pointer("")`, or `types.Pointer("verified = true")` if
  MediKube requires verification before use.

**Answering the brief's question (f) directly: yes, PB native auth survives the multi-patient
sharing model cleanly.** Authentication and authorization are orthogonal here. PB issues the
token and owns identity; the `shares` table plus the Go `Authorizer` own access. Collection API
rules never enter the picture because they are all `nil` and the public API is hand-written. The
sharing model does **not** make API rules unworkable — it makes them *unnecessary*.

---

### C7 — `samber/mo` and `samber/ro` contradict the mandated KISS + Go-best-practices principles — **FRICTION**

**Evidence** (`observability.md` §7.4–7.5, verified against what the modules actually are):

- `samber/mo` v1.17.0 is a **monad library** — `Option`, `Result`, `Either`, `Future`, `IO`,
  `Task`, `State`.
- `samber/ro` v0.4.1 is **RxGo** — a ReactiveX implementation with `Observable`, `Observer`,
  `Subject`, `Pipe`, `CombineLatest`. It is **pre-1.0 with documented breaking changes**. If it
  was added on the assumption that "ro" meant read-only helpers or iterators, that assumption is
  wrong.

**Why `mo` fails the brief's own principles.** The brief mandates KISS, Go best practices, and
testify. `mo.Result` (1) severs `errors.Is`/`errors.As`/`%w`, which `router.ToApiError`, the
Sentry integration and zerolog's `Err()` field all key off — it is a wall between the domain and
every piece of error plumbing the observability design just built; (2) makes an ignored error
**lint-invisible** (an ignored `error` return is an `errcheck` failure; an ignored `mo.Result` is
a legal expression statement) — in a medical records app "we forgot to check whether that write
succeeded" is not a style question; (3) drives everyone to `.MustGet()`, converting
compile-time-visible error handling into runtime panics; (4) **Go has no `?` operator**, so
propagating a `Result` through five layers is *more* code than `if err != nil`, not less — you
pay the full syntactic cost of monads for none of the benefit. Even the one defensible niche
(`mo.Option` for PATCH absent-vs-zero) loses to plain `*T`, which is stdlib, marshals correctly,
and needs no custom `MarshalJSON` to round-trip through the OpenAPI generator.

**Why `ro` fails.** MediKube is: HTTP in → validate → SQLite → SSE push → HTTP out. There is no
stream algebra to express. Channels, `context` and `errgroup` cover every case. PB's own
`apis/realtime.go` solves the identical problem with a channel per client and a `select` — that
is the right amount of machinery. Adding Observables is a **second concurrency model** in one
codebase, with failure modes (leaked unsubscribed observables, hot vs cold semantics,
error-channel termination) that are unfamiliar to Go developers. And it is a pre-1.0 dependency
sitting in the path of the realtime layer of an app whose users cannot tolerate a broken upgrade.

**The constitution's current position is the worst of both worlds.** It permits them "only inside
package internals, never on an exported signature". Carrying a dependency you have banned in the
rules is an invitation, not a control — the first `MustGet()` in an internal helper is how it
starts.

**Resolution.** Drop both from `go.mod` entirely. Move them in the constitution from
"permitted internally" to the **Forbidden dependencies** list, next to gin/huma/viper, and add
them to the `depguard` `no-forbidden-frameworks` deny rule so it is a build gate rather than a
review convention. Errors are `error`, wrapped with `%w`. Optionality is `*T` or a `bool`
companion return.

---

### C8 — "13 clinical record types" under-scopes the clinical domain — **FRICTION**

The locked scope names "13 clinical record types". `domain-clinical.md` §2 inventories **25
entities / ~499 fields** (378 core + 121 across 17 link tables).

**The gap.** The 13 patient-scoped types cannot function standalone. Medication FKs to
Practitioner *and* Pharmacy; Encounter to Practitioner and Practice; Lab Test Component to the
Standardized Test catalog; Injury to Injury Type; Practitioner to Medical Specialty. The
reference entities (Practitioner, Practice, Pharmacy, Medical Specialty, Injury Type) and the two
read-only standardized catalogs (Test, Vaccine) are **not** among the 13 but are prerequisites
for most of them.

**Resolution.**

- Add an explicit **reference-data + catalogs phase** before the bulk clinical phases (Phase 2
  in §5).
- **The walking skeleton's single record type must have no reference FKs**, or Phase 1 drags the
  whole reference graph in and stops being a skeleton. **Allergy** (10 fields, one optional
  `medication_id`) or **Condition** (14 fields) are the right choices; **Medication is not**
  (21 fields, two reference FKs). Recommend **Allergy**.

**Two more clinical facts the spec must absorb** (both from `domain-clinical.md`, both sound):

- **Only six enums are DECLARED** in the upstream OpenAPI document (`EntityType`,
  `VaccineCategory`, `ExportFormat`, `ExportScope`, `ChannelType`, `EventType`) plus one
  `pattern`. Every clinical enum — allergy/condition severity, all status ladders, medication
  route, insurance type — is a bare `{"type":"string"}` because Pydantic validators never
  reached the schema. Since MediKube has **no wire-compat obligation**, this is a licence: §30 of
  that dossier proposes the canonical set. The spec must say plainly that MediKube is *choosing*
  these values, not *inheriting* them. Encode them as PB `SelectField.Values`.
- **Collapsing 11 of 17 link tables into PB multi-relation fields is technically sound** — I
  verified PB supports back-relation traversal (`core/record_field_resolver_runner.go:496`,
  "check for back relation (eg. yourCollection_via_yourRelField)", usable in both filters and
  `expand`). Twelve of the seventeen carry only `relevance_note`, which the bulk-create schemas
  prove is per-*operation*, not per-*pair*. Keep the 5 real joins (four of them Treatment's,
  which carry genuine payload — `treatment_medications` alone has 8 payload fields). This kills
  ~44 endpoints and ~35 schemas. Note `age`, `days_until_expiry`, `is_active` and `linked_at`
  **do not exist upstream** — do not model them; the first two are computed and the last is
  `created_at`.

---

### C9 — PocketBase's 5-minute `WriteTimeout` silently kills every SSE stream — **FRICTION**

**Evidence.** `apis/serve.go:145-160`, verified by direct read:

```go
server := &http.Server{
    // higher defaults to accommodate large file uploads/downloads
    WriteTimeout:      5 * time.Minute,
    ReadTimeout:       5 * time.Minute,
    ...
}
```

It is a struct literal with no config field. `datastar.NewSSE` sets `Cache-Control`,
`Content-Type` and `Connection` and flushes — it **never touches the write deadline**. So every
long-lived Datastar stream dies at exactly five minutes with a write error and the client
reconnect-loops. **It passes every test shorter than five minutes**, which is why it is
dangerous.

**Resolution — two verified fixes; use both.**

1. **Per-connection (primary).** In a mandatory `newStream(e)` helper:
   `http.NewResponseController(e.Response).SetWriteDeadline(time.Time{})`. Verified this reaches
   the real `net/http` writer: `tools/router/router.go:312` implements
   `func (rw *ResponseWriter) Unwrap() http.ResponseWriter`.
2. **Global belt-and-braces (this one the dossiers missed).** `core.ServeEvent` exposes
   **`Server *http.Server`** as an exported, mutable field (`core/events.go:110`), and
   `apis/serve.go` assigns it at `:212`, fires `OnServe().Trigger` at `:217`, and then calls
   `serveEvent.Server.Serve(listener)` at `:304/307`. So an `OnServe` hook can adjust the
   timeouts **before the listener starts**. `frontend.md`'s summary presented this as an
   unresolved danger; fix (1) is in its body and fix (2) is additionally available.

The same helper must also set `X-Accel-Buffering: no` (or an upstream nginx buffers the entire
stream) and `Cache-Control: no-store` (PB's own realtime uses `no-store`; correct for a medical
app). Register stream routes with `Bind(apis.SkipSuccessActivityLog())`.

**Add a >5-minute SSE liveness test to CI**, or this regresses invisibly the first time someone
refactors the helper.

**Related, and real:** PB's global `rateLimit` middleware plus Datastar's reconnect loop
(`retryMaxCount: 10`, exponential backoff to 30s) can lock a user out after a server restart.
Exempt the stream routes or set their limit generously. Note also that
`RateLimits.Enabled` **defaults to `false`** — a self-hosted medical app on the internet wants it
explicitly on, especially for auth routes.

---

### C10 — Datastar requires `script-src 'unsafe-eval'`, permanently — **MINOR (but must be an explicit decision)**

**Evidence.** `frontend.md` §9 verified this in the shipped `datastar.js` v1.0.2 bundle rather
than guessing: the expression compiler is literally the `Function` constructor
(`let l = Function("el","$","__action","evt",...n,s)`), and the signal parser falls back to it
too so that non-strict-JSON `data-signals="{foo: 1}"` works. Without `'unsafe-eval'` **every
`data-*` expression on every page throws** — the app does not partially degrade, it is entirely
non-functional, and it fills the console with CSP violations, **failing the zero-console-error
Playwright gate on every route**.

**Resolution.** Record it in the constitution as an **accepted risk**, not a config footnote:

```
default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self';
img-src 'self' data: blob:; connect-src 'self'; form-action 'self';
frame-ancestors 'none'; base-uri 'none'; object-src 'none'
```

What bounds the risk: `'unsafe-eval'` does not itself create an injection vector, and **every
Datastar expression is server-authored templ output** — expression text never comes from user
input. The compensating controls that matter are that `'unsafe-inline'` is **not** granted (so
injected `<script>` tags still don't run), plus `connect-src`/`form-action 'self'` and
`object-src`/`base-uri 'none'`.

**Two lint-enforced rules keep it safe, and they are load-bearing:**

1. **Never interpolate user data into a `data-*` expression** — not `data-text`, not `data-on:*`,
   not `data-signals`. User data reaches the client as a *signal value* or as escaped text
   content, never as expression source. A CI grep for templ interpolation `{ ... }` inside a
   `data-on:` / `data-computed:` attribute value is cheap and worth having.
2. **Keep `'unsafe-inline'` out of `script-src` permanently.** This is what banning the
   inline-script SDK family buys you — verified in
   `datastar-go@v1.2.2/datastar/execute-script.go:83-97`, `ExecuteScript` appends a literal
   inline `<script>`. Under `script-src 'self'` these all silently fail *and* log a CSP
   violation: `ExecuteScript`, `ConsoleLog`, `ConsoleError`, `Redirect`, `Redirectf`,
   `DispatchCustomEvent`, `ReplaceURL`, `ReplaceURLQuerystring`, `Prefetch`. **Ban the whole
   family.** All are avoidable (`Redirect` → a plain 303 before opening the stream;
   `ConsoleError` → zerolog server-side + a `role="alert"` banner). Note this is already
   half-encoded in constitution Principle VI, which forbids `ConsoleLog`/`ConsoleError`; widen it
   to the full list.

Note PB's `securityHeaders()` middleware does **not** set a CSP — PB's `defaultCSP` applies only
to the `/_` admin UI. MediKube must set its own.

**If `'unsafe-eval'` is unacceptable to a stakeholder, the honest answer is that Datastar is the
wrong choice, not that it can be configured around.** See open question Q2.

---

### C11 — `data-persist` and eight other attributes are Datastar **Pro (paid)** — **MINOR**

`frontend.md` §0 Trap 5 enumerated the plugins actually registered in the MIT bundle. The free
set is: `attr bind class computed effect indicator init json-signals on on-intersect on-interval
on-signal-patch ref show signals style text`, plus actions `peek setAll toggleAll get post put
patch delete`, plus the bare markers `data-ignore`, `data-ignore-morph`, `data-preserve-attr`,
`data-on-signal-patch-filter`.

**Pro (not usable):** `data-persist`, `data-query-string`, `data-replace-url`,
`data-scroll-into-view`, `data-view-transition`, `data-custom-validity`, `data-animate`,
`data-match-media`, `data-on-raf`, `data-on-resize`, and actions `@clipboard`, `@fit`.

**Resolution.** Treat the free bundle as a hard boundary; make it a lint rule (no `data-`
attribute outside the free list). Every Pro attribute has a trivial free replacement, and for
`data-persist` the replacement is **better anyway**: persist UI preferences in the PB `users`
record and hydrate via `data-signals` on page render — a medical app should not be scattering
PHI-adjacent state into `localStorage`.

---

### C12 — v0.x Datastar material is actively wrong, and one rename fails silently — **MINOR**

`data-on-click` **silently does nothing** in v1 — the delimiter changed from `-` to `:`, so it is
`data-on:click`, and `data-on-load` became `data-init` outright. SSE event names are exactly two
(verified in `consts.go:66-68`); `remove-fragments` / `remove-signals` / `execute-script` are
gone; the data line is `elements` not `fragments`, and `mode` not `mergeMode`. The **official
templ Datastar documentation page is stale** and still shows the v0.x `MergeFragmentTempl` API.
The npm package is abandoned at `1.0.0-beta.11` — vendor the bundle from the GitHub tag.

**Critically: the Go SDK is v1.2.2 but the JS runtime is v1.0.2.** Different repos, independent
version numbers. Do not "align" them; a `datastar.js v1.2.2` does not exist.

**Resolution.** The constitution already encodes the two-event-type rule — good. Add the
`data-on-click` → `data-on:click` trap and the SDK-vs-runtime version split to the spec, since
they are silent failures rather than errors.

**Also worth acting on:** Datastar honours a plain `text/html` response as an element patch, so
**most MediKube interactions need no SSE at all**. Use the non-SSE fast path by default and reserve
streams for genuinely live views. This materially reduces the surface exposed to C9.

---

### C13 — `OnRecord*Request` hooks will never fire — **MINOR**

The `*Request` hook family is bound **inside the built-in CRUD handlers**, which the lockdown
disables. Business logic placed there is dead code that fails silently.

**Resolution.** All domain logic goes in the **bare** hooks (`OnRecordValidate`,
`OnRecordCreate`, `OnRecordAfterCreateSuccess`). Add a `forbidigo` pattern for
`OnRecord.*Request` outside the auth package. This will bite someone otherwise.

---

### C14 — Sharing a `tests.TestApp` across scenarios causes a stack overflow — **MINOR**

Found by `testing.md` while prototyping: returning the *same* app from `TestAppFactory` for
several scenarios makes `apis.bindUIExtensions` re-enter on every `OnServe` trigger and the
handler chain grows without bound (`runtime: goroutine stack exceeds 1000000000-byte limit`).

**Resolution.** `TestAppFactory` **must construct a new app on every call**. It costs 10.9 ms;
there is no reason to share. `NewTestAppWithConfig` already calls `TempDirClone` internally, so
you do **not** write `t.TempDir()` + copy yourself. `t.Parallel()` on every integration test is
safe and measured (200 isolated apps: 1.96 s sequential, 0.75 s parallel, 6.5 s under `-race`).
Always `defer app.Cleanup()` immediately after the `require.NoError`, or a panicking test leaks
~700 KB in `/tmp` for the life of the runner.

---

### C15 — OpenAPI cannot be introspected out of PB's router — **MINOR**

`testing.md` §7 checked both ends: PB's `RouterGroup.children` is **unexported**, and Go 1.27's
`http.ServeMux` still exposes **no** pattern-enumeration API. So "OpenAPI is generated from those
Go types" cannot mean "walk the router after the fact".

**Resolution.** An `httproute.Registry` where **describing and registering a route are the same
call**, with `Bind()` separated so `medikube routes --list` needs no DB, no port and no
migrations. Three `go test` gates in CI, modelled on `medikeep-mcp`'s `task api:coverage`:
registry↔OpenAPI agreement in **both** directions; committed `api/openapi.json` is not stale;
every page route is smokeable. A page registered without a landmark element panics at
registration time. Proven in the dossier: adding one Go route took `--list` from 9 to 11 tests
with zero TypeScript edits.

---

### C16 — Two Playwright flakiness traps that will look like real failures — **MINOR**

1. **`waitUntil: 'networkidle'` hangs on every live page** — an SSE stream never goes idle. Use
   `domcontentloaded` plus an explicit landmark assertion.
2. **`net::ERR_ABORTED` on teardown** from aborted SSE streams. Filter it **exactly**; do not mute
   all `requestfailed` or the "zero failed network requests" half of the gate becomes decorative.

**Caveat to carry:** browser assertions were **never actually executed** — the research sandbox
had no root to install Chromium's libraries. Config and specs parse and collect correctly, but
**verify the gate goes red on a deliberately broken page before trusting it.** An
always-green gate is worse than no gate. Make that the Phase 1 exit criterion.

---

## 2. Libraries to drop (beyond the already-dropped Gin / Huma / Viper)

| Library | Verdict | Reason |
|---|---|---|
| `github.com/samber/mo` | **DROP** | Severs `errors.Is`/`As`/`%w`, which `router.ToApiError`, Sentry and zerolog all depend on. Makes ignored errors lint-invisible. No `?` operator ⇒ strictly more code than `if err != nil`. Drives `.MustGet()` panics. Directly contradicts the mandated KISS + Go-best-practices principles. (`observability.md` §7.4) |
| `github.com/samber/ro` | **DROP** | It is RxGo, not read-only helpers. **v0.4.1, pre-1.0, documented breaking changes**, in the path of the realtime layer of a medical app. A second concurrency model for a problem MediKube does not have — realtime is already solved by channels + Datastar SSE. (`observability.md` §7.5) |
| `github.com/samber/slog-zerolog` | **DO NOT ADD** | Not in the locked list, but the obvious thing someone will reach for. Unnecessary: **zerolog v1.35.1 ships `zerolog.NewSlogHandler` natively** (`rs/zerolog/slog.go`), with correct level mapping, `LogValuer` resolution, typed-field dispatch without hot-path reflection, and `event.Ctx(ctx)` propagation that the Sentry hook depends on. |
| PB `plugins/jsvm` | **DO NOT REGISTER** | Ships a full JS runtime (goja) into a Go-only medical binary. Already in the constitution's forbidden list; keep it there and in `depguard`. |
| `datastar.WithCompression` | **DO NOT USE** | Redundant under PB (which does not gzip your routes) and risks double `Content-Encoding` ⇒ an unreadable stream. |

**Kept, with a rule:** `samber/lo` — stdlib `slices`/`maps` first; reach for `lo` only for
`GroupBy`, `KeyBy`, `SliceToMap`, `Chunk`, `UniqBy`, `PartitionBy`, `CountBy`, `ToPtr`. Ban
chained `lo.Map(lo.Filter(lo.Map(...)))` and ban `lo.Must` outside tests/`init` (it panics).
**Kept:** `samber/do` v2 — composition root only (`internal/di/` + `New*` providers), no runtime
`MustInvoke`.

---

## 3. Confirmed stack

Versions are pinned to the monorepo house set where they overlap (verified by reading
`arc-ui/go.mod` directly), and to the module cache otherwise.

| Module | Version | Role |
|---|---|---|
| *(toolchain)* | **go 1.27.x** | **Changed from the locked 1.26.5 — see C1.** `module medikube`, bare name, `toolchain go1.27.x`, `tool ( github.com/a-h/templ/cmd/templ )` |
| `github.com/pocketbase/pocketbase` | v0.40.1 | Persistence, HTTP router, auth, files, migrations, admin UI, cron, mailer |
| `github.com/a-h/templ` | v0.3.1020 | Server-rendered typed components (matches arc-ui) |
| `github.com/starfederation/datastar-go` | v1.2.2 | SSE + hypermedia Go SDK. **Browser runtime is v1.0.2, vendored+embedded** |
| `github.com/caarlos0/env/v11` | v11.4.1 | The only config mechanism (matches arc-ui) |
| `github.com/rs/zerolog` | v1.35.1 | The only app logger; supplies `NewSlogHandler` (matches arc-ui) |
| `github.com/getsentry/sentry-go` | v0.48.0 | Errors and panics only, scrubbed (matches arc-ui) |
| `github.com/prometheus/client_golang` | v1.24.1 | Metrics on a 127.0.0.1-bound `/metrics` |
| `go.opentelemetry.io/otel` | v1.45.0 | Tracing only (+ SDK, OTLP exporter, `exporters/prometheus` for the otelsql bridge) |
| `github.com/XSAM/otelsql` | *pin at first build* | DB instrumentation via `pocketbase.Config.DBConnect`. **Not present in the module cache — resolve and pin during Phase 1.** |
| `github.com/samber/do/v2` | v2.1.0 | DI, composition root only |
| `github.com/samber/lo` | v1.53.0 | Generic helpers, stdlib-first rule (matches arc-ui) |
| `github.com/stretchr/testify` | v1.12.0 | The only assertion library (matches arc-ui; PB carries it indirectly at v1.8.0 — MVS resolves, no conflict) |
| `github.com/spf13/cobra` | v1.10.2 | **Transitively via PocketBase's `RootCmd`. Never a direct require.** Note this is newer than arc-ui's v1.7.0 |
| `modernc.org/sqlite` | v1.57.0 | Transitively via PocketBase. **Pure Go ⇒ `CGO_ENABLED=0` holds** |
| Tailwind CSS standalone | v4.3.3, pinned by `ARG` | Build-time only, no Node in runtime |
| Playwright CLI | build/CI only | The UI gate |

**Dropped:** `samber/mo`, `samber/ro` (§2), plus the already-dropped Gin, Huma, Viper.

**Docker / monorepo integration** (verified against the live files):

- `/.dockerignore` is a **deny-everything-then-readmit allowlist** (`*` on line 12). Add
  `!medikube/` to the allowlist block after `!arc-ui/`, then mirror arc-ui's build-output
  exclusions **plus** `medikube/pb_data/` — that directory holds the live database and uploaded
  medical records and must never enter a build context. **Miss the allowlist entry and the build
  context is empty**, failing with a misleading "file not found".
- `/.github/workflows/build-image.yaml` — add `medikube` to
  `workflow_dispatch.inputs.project-name.options` (currently technologia, appbase, gmod,
  arc-ui). Verified: the matrix is **arch-only**, the workflow is `workflow_dispatch`-only, and
  it is fully generic on `inputs.project-name`. **No matrix entry, no path filter.**
- **The hard constraint:** CI passes `context: .` (repo root) unconditionally, so MediKube's
  Dockerfile **must** use project-prefixed COPY (`COPY medikube/go.mod medikube/go.sum ./`). A bare
  `COPY go.mod ./` works locally and breaks in CI — the likeliest single miss.
- Copy **arc-ui's** layout, Taskfile and 4-stage Dockerfile, not appbase's. Do **not** copy
  arc-ui's HTTP layer (it is Gin; PocketBase owns MediKube's router). MediKube needs no
  `replace ../go-modules`.
- Ship a `medikube healthcheck` Cobra subcommand — distroless has no curl/wget for `HEALTHCHECK`.
- Commit `internal/web/static/.gitkeep` or `go:embed` fails on the empty directory.
- Land the deletion of the already-staged `medi-keep-go/` in the same commit.

---

## 4. Open questions for the user

Research settled the facts; these five are genuine decisions that materially change the
architecture and cannot be made from the dossiers.

1. **Go 1.27.x, or drop PocketBase to v0.39.x?** (C1) Everything is downstream of this. 1.27 is
   strongly recommended, but it makes MediKube the only project in the monorepo off the 1.26.5
   house standard, and the fallback invalidates every file:line citation in two dossiers.
2. **Is `script-src 'unsafe-eval'` acceptable for an app holding diagnoses and lab results?**
   (C10) It is mandatory and permanent for Datastar. If a stakeholder says no, the honest
   consequence is that **Datastar is the wrong frontend choice** and that locked decision
   reopens — which is far cheaper to discover now than in Phase 4.
3. **Does the PocketBase superuser admin UI ship in production?** The constitution locks "keep
   it", and the lockdown depends on superusers bypassing all rules — which means a superuser can
   read every patient's complete chart through `/_`. That is a large PHI surface reachable with
   one credential. `-tags no_ui` strips it entirely (`ui/embed_no_ui.go`). If it ships, the
   answer must also cover superuser IP allowlisting and MFA.
4. **Is MediKube ever horizontally scaled?** The Datastar SSE hub is **in-process** (C2), so the
   design is single-instance by construction. If multi-instance is ever required, the hub needs
   an external broker and that is an architectural change, not a config flag. Decide now.
5. **Record-level soft delete, or file-trash only?** The locked scope lists "soft-delete trash",
   but `domain-platform.md` §8.5/§B5 found that **upstream's "trash" is only a path-keyed file
   directory — there is no record-level undo anywhere in MediKeep**. Real record-level soft
   delete touches every collection and every query in the app. Inheriting it by accident is the
   failure mode the dossier warns about.

---

## 5. Recommended phase decomposition

Seven phases. Each is independently shippable and independently verifiable.

### 1 — `walking-skeleton`

**Goal.** One clinical record type works end to end on embedded PocketBase, with every
cross-cutting decision proven and gated, so later phases only add domain.

**Deliverables.** Go 1.27 module (`module medikube`, `tool` directive for templ) and the
`.dockerignore` + `build-image.yaml` entries landed in the same commit; arc-ui-shaped layout,
Taskfile and 4-stage distroless Dockerfile with the Tailwind `x64`-not-`amd64` and glibc traps
handled; `pocketbase.NewWithConfig` with `HideStartBanner` and `DefaultEncryptionEnv`;
`caarlos0/env/v11` config validated at boot; zerolog + the **two-part** PB bridge (C4: `pb.App`
decorator **and** `OnModelCreate("_logs")` interception at `MaxDays = 1`); Sentry + Prometheus on
a 127.0.0.1 `/metrics` + OTel with `otelsql` wired through `DBConnect` (including the
copied-pragma drift check); PB native auth with the hand-written `/api/v1/auth/*` trio and the
enumerated PB-path exceptions (C6); **the lockdown**: nil rules in migrations, the boot assertion
that refuses to start on any non-nil rule, `Batch.Enabled = false`, and the `-1019` prefix
middleware; **Allergy** end to end (migration, typed record proxy, repository interface, service,
DTOs, `/api/v1` CRUD) plus a templ page with Datastar and one SSE stream through the mandatory
`newStream` helper (C9); the `httproute.Registry` with all three OpenAPI gates (C15); `medikube
seed` / `routes` / `openapi` / `healthcheck` Cobra subcommands; `.golangci.yml` v2 with
`depguard` + `forbidigo`; Playwright smoke gate **proven red on a deliberately broken page**
(C16), desktop + mobile.
**Depends on:** nothing.

### 2 — `reference-and-catalogs`

**Goal.** Every lookup entity the clinical records depend on exists, with MediKube's canonical
enums defined rather than inherited.

**Deliverables.** Practitioner, Practice, Pharmacy, Medical Specialty, Injury Type; the two
read-only standardized catalogs (Test, Vaccine) with seed data; the canonical enum set from
`domain-clinical.md` §30 as PB `SelectField.Values` (one severity ladder, one status ladder per
shape); normalized `tags` collection replacing the denormalized string array (9 endpoints → 4);
uniqueness as collection indexes (`AddIndex`, since PB has no per-field `Unique`); the committed
`internal/testdata/pb_data` fixture that every `TestApp` clones.
**Depends on:** `walking-skeleton`.

### 3 — `clinical-records`

**Goal.** All 13 clinical record types and the multi-patient model, on one canonical route shape.

**Deliverables.** Patient CRUD + `is_self_record` + the `active_patient` pointer + patient
switching; the remaining 12 record types; **one** canonical patient-scoped route shape with
server-side authorization — `?required_permission=` is deleted from the wire entirely (it was
client-supplied authorization on 41 upstream operations, defaulting to `view` **on writes**); 11
of 17 link tables collapsed into PB multi-relation fields with back-relations, keeping the 5 real
joins; the 9 confirmed redundant field pairs deduplicated (`condition_name`/`diagnosis`,
`treatment_type`/`treatment_category`, `specialty`/`specialty_name`, stored `bmi`, `time_of_day`)
and the type holes fixed (vitals has *zero* numeric bounds upstream and `recorded_date`
round-trips lossily).
**Depends on:** `walking-skeleton`, `reference-and-catalogs`.

### 4 — `labs-and-attachments`

**Goal.** Labs and files, with file serving that does not leak PHI.

**Deliverables.** Lab results + lab test components + standardized test catalog wiring; **all
file fields `Protected: true` with a boot assertion**, served exclusively from hand-written
`/api/v1` routes via `app.NewFilesystem()` (C3); eager thumbnail generation in a
`TxInfo().OnComplete` callback; the two parallel upstream file systems (`entity_files` +
`lab_result_files`, 34 ops) collapsed onto PB file fields; unified search — and be honest in the
spec about SQLite FTS limits.
**Depends on:** `clinical-records`.

### 5 — `sharing-and-invitations`

**Goal.** The one thing that must be built well: row-level access, unified.

**Deliverables.** A single resource-generic `shares` collection (`ResourceKind` = patient |
family_member) with **two** levels `view|edit` — `full` is dropped as undefined — and
`ExpiresAt`/`RevokedAt` timestamps instead of `is_active`; `custom_permissions` dropped entirely;
the Go `Authorizer` interface as the **sole** authoritative access check, with read-time expiry
in the repository query rather than in an API rule (C5) and no cron in the correctness path; one
invitations table with a **typed discriminated payload** replacing untyped `context_data`;
family-history sharing preserved as a product distinction but as a `ResourceKind`, not 20
duplicate endpoints (26 upstream ops → ~9); `entity-files/.../cleanup`, which upstream documents
as performing **no authorization at all**, is not reproduced.
**Depends on:** `clinical-records`.

### 6 — `reporting-and-ops`

**Goal.** Export, audit and the operational surface — mostly by *not* building it.

**Deliverables.** Custom reports + async export in the documented portable format the
constitution requires ("no user is ever trapped"); the bespoke domain activity log (PB's `_logs`
is a *request* log and is disabled, so the audit trail is MediKube's); thin wrappers over PB's
native backup/restore + cron retention (~20 ops → ~4), keeping only safety-backup-before-restore
and restore preview; the trash/soft-delete decision from Q5 implemented whichever way it lands;
the superuser admin UI decision from Q3 implemented; `frontend-logs/*` deliberately **not** built
(two of the four were unauthenticated write endpoints, and a templ+Datastar frontend has no React
error boundaries to ship home).
**Depends on:** `sharing-and-invitations`, `labs-and-attachments`.

### 7 — `hardening-and-release`

**Goal.** Prove the whole thing holds under the gates before it touches real medical data.

**Deliverables.** The realtime SSE hub generalized across every subsystem with per-event
re-authorization; a **>5-minute SSE liveness test** in CI (C9); the final CSP with the
`'unsafe-eval'` accepted-risk record and the two lint rules that bound it (C10); the free-Datastar-
attribute lint rule (C11); rate limits enabled and tuned, with stream routes exempted; the full
Playwright matrix across every registered route, desktop + mobile, zero console errors, zero
failed network requests; coverage and OpenAPI-completeness gates enforced; a PocketBase-upgrade
re-verification checklist covering the three unexported-internals workarounds (logger decorator,
`_logs` interception, copied pragma string); security review with emphasis on Principle VII.
**Depends on:** `reporting-and-ops`.

---

## 6. Checked and cleared — do not re-litigate

- **Locking down `/api/collections/*` is genuinely clean.** Five `nil` rules is a first-class,
  source-verified superuser-only mode (`apis/record_crud.go` + `core.CanAccessRecord`).
  **Internal Go access is completely unaffected** — rules are evaluated only in the `apis` HTTP
  layer, never by `app.FindRecordById` / `app.Save` / `app.RecordQuery`. This is the single
  property that makes the whole MediKube design viable. **The admin UI survives** because
  superusers bypass rules. The caveats are C2/C3/C6, not the mechanism itself.
- **gzip does not break SSE, because PB never gzips your routes.** Verified: `apis.Gzip()` is
  bound at exactly two sites (`apis/serve.go:99`, `apis/extensions.go:39`), **both scoped to the
  `/_` admin UI static tree**. The global chain is `activityLogger, panicRecover, rateLimit,
  loadAuthToken, superuserIPsWhitelist, securityHeaders, BodyLimit` — no gzip. Rule: never bind
  `apis.Gzip()` at the router root; bind per-group on static assets only, and add a `forbidigo`
  rule against a root-level bind.
- **otelsql attaches cleanly.** `pocketbase.Config.DBConnect` is the injection point; use
  `otelsql.Open("sqlite", dsn, ...)` + `dbx.NewFromDB(db, "sqlite")`. **Not `otelsql.Register`** —
  it opens with an empty DSN and `DBConnect` is called **four times** (data + aux, concurrent +
  nonconcurrent), which would burn four global driver slots. **Critical:** pass the logical name
  `"sqlite"` to `NewFromDB` or `dbx` falls back to `NewStandardBuilder` (`dbx/db.go:321`) and
  breaks PB's SQL quoting. One real liability: PB's pragma string is a **local variable** inside
  `DefaultDBConnect` (`core/db_connect.go`), not an exported constant, so it must be copied and
  CI-checked for drift — get it wrong and you silently lose WAL or foreign keys.
- **Cobra needs no extra dependency.** `app.RootCmd` is a real `*cobra.Command` (v1.10.2, PB's
  own dep), and custom subcommands receive a **fully bootstrapped app** — DB open, settings
  loaded, migrations applied — because `skipBootstrap()` only skips for help/version/unknown
  commands. Two traps: the root sets `FParseErrWhitelist{UnknownFlags: true}` so typo'd flags are
  silently tolerated (validate in `RunE`), and `RootCmd.SetErr(&nopWrite{})` discards cobra's
  error output (print it yourself in `main`; set `SilenceUsage: true`).
- **templ has zero conflict with PB's `tools/template`** — the latter is an unregistered helper
  nothing in the router references. Ignore the package. `datastar.NewSSE` takes a plain
  `http.ResponseWriter` + `*http.Request`, and `core.RequestEvent` exposes `.Response`/`.Request`,
  so Datastar drops into a PB handler with **zero glue**. `PatchElementTempl` accepts a
  `TemplComponent` *interface*, so datastar-go does not hard-depend on templ.
- **`tests.NewTestAppWithConfig` gives genuinely isolated, `t.Parallel()`-safe databases** out of
  the box via an internal `TempDirClone`. Measured 10.9 ms/app. `ExpectedEvents map[string]int`
  (with `"*": 0`) is the clean way to assert a request had no side effects — write that scenario
  against the locked-down CRUD route for every collection.
- **Migrations are reversible by construction** — `migrations.Register(up, down, ...)` requires
  both functions. Set `Automigrate: cfg.Dev` **only**; in production it tries to write `.go`
  files into a directory that does not exist in the container.
- **`CGO_ENABLED=0` holds**, because PB's SQLite is `modernc.org/sqlite` (pure Go). So every
  Docker stage before the last can be `--platform=$BUILDPLATFORM` and nothing runs under QEMU.
- **Hand-written fakes over a mock framework.** `testify/mock` is stringly-typed and survives
  renames; mockery/moq is codegen overhead for 4-method ports you designed yourself. Fakes in
  `<pkg>test` with `var _ Iface = (*Fake)(nil)`, kept honest by a shared `suite.Suite` contract
  suite run against **both** the real implementation and the fake — that solves drift, which
  generators do not.
- **No per-field `Unique` in PocketBase.** Uniqueness is `collection.AddIndex(name, unique,
  cols, where)`. This is an upgrade, not a limitation — partial and expression indexes come free
  (case-insensitive unique email is `AddIndex("idx", true, "LOWER(email)", "")`).
- **`number` fields are float64** (~2^53-1 safe). Never store identifiers in a `number` field;
  use `text`. `GetInt64` exists but does not widen the underlying type.
- **No trailing-slash normalisation** since v0.23 — `/api/v1/patients/` ≠ `/api/v1/patients`.
  Pick "no trailing slash", enforce it in the OpenAPI generator, cover it in the smoke gate.
- **MediKube's UI will not work with JavaScript disabled, and that is fine — but state it
  plainly.** Datastar does not degrade: with JS off nothing is bound, `data-bind` populates no
  signals, so a form submit sends nothing at all. This is structural, not incidental. Do not let
  it be discovered as a bug in Phase 7.
