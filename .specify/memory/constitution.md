<!--
SYNC IMPACT REPORT
==================
Version change: 1.2.0 -> 1.3.0
Rationale: MINOR. The phase plans, correctly, could not be written without four
modules the 1.2.0 stack did not list, and they surfaced a second configuration
store that 1.2.0's wording forbade. Technology Constraints is amended to admit
both, with the plans' own rationales, rather than leaving six plans in standing
violation of a clause that says the stack may only change by amendment. Nothing
is removed and no gate is weakened.

 10. Technology Constraints: four modules ADMITTED — kin-openapi (OpenAPI
     document construction; PocketBase's router is not introspectable so the
     document must be built, not reflected), XSAM/otelsql (Principle VI's DB
     tracing), go-pdf/fpdf (a phase-006 export format), and golang.org/x/text
     as a direct dependency. All four are pure Go and cgo-free.
 11. Technology Constraints: PocketBase's own `Settings()` store is CARVED OUT
     of the "caarlos0/env is the only configuration mechanism" rule. It is not a
     second config system MediKube chose; it is part of the platform Principle V
     says MediKube must not reimplement.

Prior: 1.1.0 -> 1.2.0
Rationale: MINOR. Four operator-facing decisions were settled and are now
recorded as binding rather than left implicit: the Datastar CSP trade-off, the
admin UI's production posture, single-instance-by-construction, and the scope of
soft delete. New guidance, nothing removed or weakened.

  5. Principle VII: `script-src 'unsafe-eval'` is ACCEPTED as a permanent,
     documented trade-off (Datastar compiles expressions with the Function
     constructor), compensated by a strict CSP in every other directive.
  6. Principle VII: the PocketBase admin UI SHIPS in production, and is
     therefore subject to mandatory superuser MFA, IP allowlisting, and session
     auditing — because a superuser bypasses every API rule by design.
  7. Technology Constraints: MediKube is SINGLE-INSTANCE BY CONSTRUCTION.
  8. Principle VII + scope: soft delete is FILES ONLY. Records are hard
     deleted.
  9. Principle IV + Technology Constraints: `samber/mo` and `samber/ro` move
     from "permitted inside package internals" to FORBIDDEN outright. mo.Result
     severs the errors.Is/errors.As/%w chain three other subsystems depend on,
     and samber/ro is not a read-only helper at all — it is a pre-1.0 RxGo-style
     reactive library that would sit in the path of the realtime layer.
     `samber/slog-zerolog` is forbidden as redundant: zerolog v1.35.1 ships
     NewSlogHandler natively.

Prior: 1.0.0 -> 1.1.0
Rationale: MINOR amendment. Four blocking contradictions were found between the
1.0.0 text and PocketBase v0.40.1's actual behaviour, each verified by reading
or building the real source. No principle was removed and no gate was weakened;
three were tightened and one language-version constraint was corrected, so this
is MINOR rather than MAJOR.

Amendments in 1.1.0:
  1. Technology Constraints: Go 1.26.5 -> Go 1.27. PocketBase v0.40.1's go.mod
     declares `go 1.27` and 67 non-test files import the 1.27 stdlib package
     `encoding/json/v2`. Verified: `GOTOOLCHAIN=local go build` on 1.26.5 fails
     with "go.mod requires go >= 1.27"; go1.27.0 builds it clean. MediKube is
     therefore the first project in this monorepo off the 1.26.5 house standard,
     deliberately and for a recorded reason.
  2. Principle V: PocketBase no longer "owns realtime". Its native realtime is
     unusable here for three independent reasons, so Datastar SSE is 100% of
     MediKube's realtime and a Go-side bridge translates record hooks into it.
  3. Principle VI: the "Logs.MaxDays = 0" instruction was wrong and is replaced.
     At 0, PocketBase's own failures go nowhere at all. The rule is now the
     two-mechanism bridge that actually works.
  4. Principle VII: mandatory `Protected: true` on every file field plus
     MediKube-owned file routes, closing an unauthenticated-file-access hole that
     the Principle V lockdown does not cover.

Prior: TEMPLATE (unversioned) -> 1.0.0. Initial ratification.

Principles defined (9, all new):
  I.    Simplicity Is A Gate (KISS)
  II.   Interfaces At Every Seam (SOLID)
  III.  Test-First With testify (NON-NEGOTIABLE)
  IV.   Idiomatic Go Over Clever Go
  V.    PocketBase Is The Platform, Not A Detail
  VI.   One Log Stream, One Trace Context
  VII.  Patient Privacy Is Structural, Not Procedural
  VIII. The UI Must Prove It Renders
  IX.   Compliance Is A Build Gate, Not A README Paragraph

Sections added:
  - Technology Constraints (locked stack + explicitly forbidden dependencies)
  - Development Workflow & Quality Gates
  - Governance

Removed sections: none (scaffold placeholders replaced wholesale).

Follow-up TODOs: none. All placeholder tokens resolved.

Notes on decisions that were reconciled against verified library source
(see specs/*/research.md for evidence):
  - Principle VI states zerolog as the ONLY app logger and forbids
    app.Logger(). PocketBase v0.40.1 hardcodes its slog handler in
    core.BaseApp.initLogger, so the handler itself cannot be injected — but a
    bridge IS achievable and IS required, by two complementary mechanisms:
    decorating the exported embedded core.App field on pocketbase.PocketBase
    (which covers the request path), and intercepting the _logs model write
    (which covers transaction-scoped logging that the decorator misses because
    createTxApp shallow-copies a *BaseApp). See Principle VI for the binding
    rule, including why Logs.MaxDays must be 1 and never 0.
  - Principle V's lockdown of the auto-CRUD API is scoped to the record CRUD
    subtree only, because PocketBase's native auth endpoints share the
    /api/collections/ prefix and must stay reachable.
-->

# MediKube Constitution

MediKube is a self-hosted personal medical records application written in Go. It
takes the problem domain of MediKeep (a React + FastAPI + PostgreSQL
application) and rebuilds it as a single, self-contained Go binary on top of an
embedded PocketBase. It is a reimagining, not a port: MediKube owes MediKeep no
API compatibility and is free to design a smaller, cleaner surface.

The data MediKube holds is among the most sensitive a person owns. Every
principle below is written with that in mind.

## Core Principles

### I. Simplicity Is A Gate (KISS)

Simplicity is not an aesthetic preference; it is an enforced gate. Every plan
MUST pass it before implementation begins.

- The simplest design that satisfies the specification WINS, even when a more
  general one is obvious and tempting.
- YAGNI is binding. Abstractions MUST NOT be introduced for requirements that
  are not in the current phase's specification. Speculative extension points,
  unused configuration knobs, and "we might need this later" parameters are
  defects.
- An abstraction MUST have at least two real, present-day implementations, or a
  test double that exists to satisfy Principle III, before it is introduced. A
  one-implementation interface that exists only to look decoupled is a defect.
- Every layer of indirection MUST be justified in writing in `plan.md` under
  Complexity Tracking. Unjustified layers are removed in review, not debated.
- Prefer deleting code to adding configuration. Prefer a function to a struct
  with one method. Prefer standard library to a dependency. Prefer a table-
  driven test to a framework.
- When Principle I conflicts with any other principle except VII, the conflict
  MUST be resolved explicitly in `plan.md`, not silently.

Rationale: this codebase's stated stack contains twelve libraries and a
framework that wants to own the whole application. Without an enforced
simplicity gate, MediKube becomes a demonstration of its dependency list rather
than a medical records application.

### II. Interfaces At Every Seam (SOLID)

MediKube is structured so that its domain logic never depends on PocketBase, on
HTTP, or on the browser.

- **Single responsibility.** A type has one reason to change. Handlers parse
  and render. Services decide. Repositories persist. A handler that builds a
  database filter, or a service that writes a response, is a defect.
- **Open/closed.** New clinical record types are added by satisfying existing
  interfaces, not by extending switch statements on entity kinds.
- **Liskov.** Every implementation of an interface MUST satisfy the same
  contract tests. Where a contract test cannot be written to cover all
  implementations, the interface is wrong.
- **Interface segregation.** Interfaces are declared by the CONSUMER package,
  named for what the consumer needs, and kept small. One to three methods is
  normal; more than five requires justification in `plan.md`. MediKube MUST NOT
  define a single omnibus `Store` or `Service` interface.
- **Dependency inversion.** Domain and service packages MUST NOT import
  `github.com/pocketbase/pocketbase/...`, `net/http`, or any templ-generated
  package. PocketBase is reached exclusively through repository interfaces
  implemented in an adapter package. This is verified by an import-boundary
  lint rule, not by convention.
- Constructors accept interfaces and return concrete types. Dependencies are
  injected, never reached for via package-level globals or `init()`.

Rationale: PocketBase is a framework with strong opinions and a fast release
cadence. Confining it to an adapter layer is what makes MediKube testable
without a database and survivable across PocketBase upgrades.

### III. Test-First With testify (NON-NEGOTIABLE)

Tests are written before the implementation they describe, and they are written
with `stretchr/testify`.

- Every specification's acceptance scenarios MUST exist as failing tests before
  the corresponding implementation task is started. Red, then green, then
  refactor.
- `testify/require` for preconditions and anything whose failure makes the rest
  of the test meaningless; `testify/assert` for independent assertions that
  should all be reported in one run. Bare `if got != want { t.Errorf }` is not
  used.
- Table-driven subtests with `t.Run` are the default shape. Test names describe
  behaviour, not method names.
- The test pyramid is explicit and each layer is mandatory:
  - **Unit** — service and domain logic against hand-written fakes or
    `testify/mock`. No database, no HTTP, no filesystem. These MUST run with
    `t.Parallel()`.
  - **Integration** — repository adapters against a real throwaway PocketBase
    test app. These prove the adapter honours the interface contract.
  - **Contract** — one shared suite per interface, run against every
    implementation including fakes, enforcing Principle II's Liskov clause.
  - **HTTP** — handlers through PocketBase's test harness, asserting status
    codes, DTO shapes, and authorization boundaries.
  - **UI render** — templ components rendered to a buffer and asserted on.
- Authorization is tested as a first-class concern. For every endpoint that
  touches patient data there MUST be a test proving an unauthorized user is
  refused, and — once sharing exists — a test proving a user who was granted
  access succeeds and a user whose access was revoked is refused.
- A bug fix begins with a test that reproduces the bug.
- Test code is production code. It is reviewed to the same standard, and
  duplicated setup is extracted into helpers.

Rationale: the specification is only real if it is executable. In an
application where an authorization mistake exposes somebody's medical history,
tests are the deliverable.

### IV. Idiomatic Go Over Clever Go

MediKube reads like Go written by someone who likes Go.

- Errors are values. Functions return `error` as the last value, wrap with
  `fmt.Errorf("...: %w", err)`, and are inspected with `errors.Is` /
  `errors.As`. Sentinel errors and typed errors live in the domain package.
- Monadic error handling is FORBIDDEN outright, and `samber/mo` MUST NOT be a
  dependency of this project. `mo.Result` severs the `errors.Is` / `errors.As`
  / `%w` chain that PocketBase's error-to-status mapping, the Sentry
  integration and zerolog's `Err()` field all depend on, and an ignored
  `mo.Result` is silent where an ignored `error` is a lint failure. A library
  that makes errors quieter has no place in an application that holds medical
  records.
- `samber/ro` MUST NOT be a dependency either. Despite the name it is not a
  read-only-collections helper — it is an RxGo-style reactive library
  (Observable, Observer, Subject, Pipe), it is pre-1.0 at v0.4.1 with
  upstream-documented breaking changes, and it would sit directly in the path
  of the realtime layer of an application whose users cannot tolerate a dropped
  update. Principle I settles this: MediKube's realtime hub is a channel and a
  map, and that is the correct amount of machinery.
- `context.Context` is the first parameter of every function that performs I/O,
  and it is honoured — cancellation and deadlines are respected, not accepted
  and ignored.
- `panic` is reserved for programmer error at startup. A request handler MUST
  NOT panic as flow control.
- No naked returns, no single-letter receivers beyond established idiom, no
  stuttering names (`medication.MedicationService` is wrong;
  `medication.Service` is right). Exported identifiers are documented.
- Goroutines have an owner, a lifetime bounded by a context, and a defined
  shutdown path. Unbounded `go func()` is a defect.
- `samber/lo` is used where it removes a loop that carries no meaning. It MUST
  NOT be used to build chains that a plain `for` would express more clearly.
- Generated code (`*_templ.go`, mocks, generated OpenAPI types) is committed,
  marked as generated, and excluded from coverage and lint.
- `gofmt`, `go vet`, and `golangci-lint` pass with zero findings. Suppressions
  carry a comment explaining why.

Rationale: the requested stack includes functional-programming libraries whose
unchecked use produces Go that Go programmers cannot read. This principle draws
that line where it belongs — at the package boundary.

### V. PocketBase Is The Platform, Not A Detail

PocketBase v0.40.1 is embedded as a library and owns the HTTP server, the
database, authentication, file storage, and the superuser admin UI. MediKube does
not fight it and does not reimplement it. PocketBase does NOT own realtime —
see the realtime clause below.

- MediKube MUST NOT rebuild what PocketBase provides. Authentication,
  OAuth2/SSO, password reset, email verification, file storage with
  thumbnails, database backup and restore, and the admin UI are PocketBase's
  responsibilities. A task that reimplements one of these MUST be rejected
  unless `plan.md` records why PocketBase's version is unusable.
- The public API surface is hand-written Go routes under `/api/v1` with
  explicit request and response DTOs. Domain records are NEVER serialised
  directly to a client; a DTO always mediates.
- PocketBase's automatic record CRUD API — the
  `/api/collections/{collection}/records` subtree — MUST NOT be publicly
  reachable. Every collection's list/view/create/update/delete rules are
  superuser-only. This lockdown is scoped precisely to the record CRUD subtree
  and `/api/batch`: PocketBase's native auth endpoints share the
  `/api/collections/` prefix and MUST remain reachable, because they are
  MediKube's authentication mechanism.
- Schema is code. Collections are defined in Go migrations under version
  control. Schema is never changed by clicking in the admin UI.
- Writes that must be atomic run inside `app.RunInTransaction`.
- **Realtime is Datastar's, not PocketBase's.** PocketBase's native realtime
  MUST NOT be used, for three independently fatal reasons verified in v0.40.1's
  source: (a) `apis/realtime.go` builds its subscription rule map from each
  collection's `ViewRule`/`ListRule`, and `CanAccessRecord` returns false when a
  rule is `nil` — so under this principle's own lockdown every broadcast is
  silently skipped, with no error and no 403; (b) PocketBase emits its own JSON
  envelope on events named `<collection>/<id>`, while datastar-go v1.2.2
  recognises exactly two event names and discards everything else; (c)
  PocketBase requires a two-step subscribe handshake that a Datastar attribute
  cannot perform. Live updates are therefore delivered by a MediKube-owned bridge:
  a post-commit record hook publishes record IDs — never record bodies — to an
  in-process hub, and a per-subscriber Datastar SSE handler re-fetches each ID,
  RE-AUTHORISES IT FOR THAT SUBSCRIBER, and only then renders and patches.
  Fanning out IDs rather than bodies is what makes per-subscriber authorization
  possible, and Principle VII requires it.
- PocketBase's JavaScript VM plugin (`jsvm`) MUST NOT be registered. MediKube
  ships no scripting runtime.
- Cobra subcommands are registered on PocketBase's `RootCmd`, which already is
  a `*cobra.Command`. No second CLI framework is introduced.
- MediKube ships as ONE binary with no cgo, with static assets embedded via
  `embed.FS`. There is no Node.js, no separate frontend server, and no CDN
  fetch at runtime.

Rationale: choosing an embedded framework and then routing around it produces
the costs of both approaches and the benefits of neither. The lockdown clause
exists because PocketBase's default posture is to expose the database over
HTTP, which is incompatible with a designed API.

### VI. One Log Stream, One Trace Context

Diagnosing MediKube means reading one stream.

- `zerolog` is the ONLY logger for MediKube's own code. Structured JSON to
  stdout, level from configuration, one event per meaningful occurrence.
- Application code MUST NOT call PocketBase's `app.Logger()`. This is enforced
  by lint.
- PocketBase's own framework logs MUST be redirected into zerolog by BOTH of the
  following mechanisms, because each covers what the other misses:
  1. Decorate the exported embedded interface — `pocketbase.PocketBase` embeds
     `core.App` as an exported field, so reassigning `pb.App` to a wrapper whose
     `Logger()` returns a zerolog-backed `slog.Logger` redirects the entire
     request path. This is safe: v0.40.1's non-test source contains zero type
     assertions back to `*core.BaseApp`.
  2. Bind `OnModelCreate("_logs")` to emit the record into zerolog and return
     WITHOUT calling `e.Next()`, so the row is never inserted. This is required
     because `core.BaseApp.createTxApp` shallow-copies a `*BaseApp`, so
     transaction-scoped logging bypasses mechanism 1 entirely.
- `Settings().Logs.MaxDays` MUST be set to `1`, and MUST NOT be set to `0`. At
  zero, PocketBase's batch handler drops every record before it is ever offered,
  so mechanism 2 never fires and PocketBase's backup, mailer, cron and OAuth2
  failures are discarded silently in production. MaxDays=1 keeps the pipeline
  alive; mechanism 2 guarantees no row is ever actually written, so there is
  still exactly one log store to consult.
- Both mechanisms MUST be re-verified on every PocketBase upgrade, as a recorded
  checklist item.
- Every request carries a request ID and, when tracing is enabled, a trace ID
  and span ID. Both appear on every log line emitted while handling it.
- OpenTelemetry provides tracing. Prometheus provides metrics. Sentry receives
  errors and panics only. These MUST NOT double-report the same event.
- Metric labels are bounded. Route labels use the registered route PATTERN,
  never the resolved path — no identifier ever becomes a label value.
- `/metrics` is not publicly reachable.
- Datastar's `ConsoleLog` and `ConsoleError` MUST NOT be used on production
  paths; they write to the browser console and would fail the gate in
  Principle VIII.
- Health and readiness endpoints are distinct: readiness reflects the database
  and migration state, health reflects the process.

Rationale: an application assembled from a framework plus five observability
libraries defaults to four uncorrelated log streams. This principle spends the
effort once, up front, to prevent that.

### VII. Patient Privacy Is Structural, Not Procedural

MediKube stores diagnoses, medications, lab results, and identities. Privacy is
enforced by construction, not by remembering.

- Patient-identifying and clinical data MUST NEVER be written to logs, traces,
  metrics, or Sentry. This includes names, dates of birth, addresses, phone
  numbers, email addresses, note and description fields, file names, file
  contents, lab values, diagnoses, medication names, and free-text of any kind.
  Records are referenced by opaque ID only.
- Domain types carrying sensitive fields MUST implement redacting marshalling
  so that logging one by accident cannot leak it. "Remember not to log this" is
  not a control.
- Sentry runs with a `BeforeSend` scrubber, request bodies disabled, and PII
  collection off. Metric and span attributes are allowlisted, never
  denylisted.
- Every read and write of patient data is authorized against the authenticated
  user at the point of access. Authorization is never inferred from a
  client-supplied patient identifier.
- MediKube makes no outbound network request that the operator has not
  explicitly configured. There are no telemetry defaults, no CDN assets, no
  update pings. Sentry and OTLP exporters are off unless configured.
- **Every file field MUST be declared `Protected: true`, without exception, and
  the application MUST refuse to start if any field is not.** PocketBase's file
  handler performs its authorization check only inside `if fileField.Protected`
  and has no else branch, so an unprotected field is served to any anonymous
  caller who knows the URL. For a medical record this is a data breach, and the
  Principle V lockdown does not close it.
- Files MUST be served exclusively from MediKube's own `/api/v1` routes, through
  the filesystem abstraction, authorized by the service layer. PocketBase's
  file-token mechanism MUST NOT be used: it places a credential in a URL, where
  it lands in logs, proxies and referrer headers. Because MediKube bypasses
  PocketBase's file route, thumbnails MUST be generated eagerly on upload.
- Any function that fetches a remote URL into storage is an SSRF sink and MUST
  validate its target against an allowlist before connecting.
- The Content Security Policy MUST include `script-src 'unsafe-eval'`, and this
  is an accepted, permanent trade-off rather than an oversight: Datastar
  compiles every expression with the JavaScript `Function` constructor and is
  entirely non-functional without it. It is compensated, not excused — every
  other directive MUST be strict: `default-src 'self'`, no `unsafe-inline`, no
  external origins whatsoever (the Datastar bundle and all assets are embedded,
  never fetched from a CDN), `frame-ancestors 'none'`, `object-src 'none'`,
  `base-uri 'self'`. The residual risk is that an XSS defect becomes more
  exploitable, so the escaping rules templ enforces are load-bearing and MUST
  NOT be bypassed with raw HTML injection.
- The PocketBase superuser admin UI SHIPS in production. Because a superuser
  bypasses every collection API rule by design — which is precisely what makes
  the Principle V lockdown safe — a superuser credential can read every
  patient's complete record. That account is therefore treated as a break-glass
  credential: superuser multi-factor authentication MUST be enabled, the
  superuser IP allowlist MUST be configured, and every admin-UI session MUST
  produce an audit-trail entry. An instance running without MFA and an IP
  allowlist configured is a misconfiguration, and MediKube MUST warn loudly at
  boot when it detects one.
- Soft delete applies to FILES ONLY. A deleted attachment is recoverable for a
  documented retention window and then purged by a scheduled job. Records are
  HARD deleted, and destructive record actions MUST therefore be confirmed in
  the interface and recorded in the audit trail. MediKube MUST NOT grow a
  `deleted_at` column on record collections: that would put a filter on every
  query in the application, and Principle I forbids paying that cost for a
  capability the specification does not ask for.
- Secrets arrive from the environment, are never logged, and are never written
  to the database in plaintext.
- Actions that mutate or export patient data are recorded in an audit trail
  that stores actor, action, target ID, and timestamp — never the content.
- Deletion means deletion. Where soft-delete exists for recovery, its
  retention window is documented and enforced by a scheduled purge.

Rationale: this is the principle whose violation causes real harm to a real
person. It outranks every other principle including Principle I, and it is the
one exception to Principle I's precedence rule.

### VIII. The UI Must Prove It Renders

A phase is not complete because its handlers return 200 in a Go test. It is
complete when a real browser renders it without complaint.

- Every user-facing route MUST be covered by a Playwright smoke test asserting:
  HTTP 200; the expected ARIA landmark or role-based selector present; ZERO
  browser console errors; ZERO uncaught page errors; ZERO failed network
  requests. At desktop (1440x900) and mobile (390x844) viewports.
- The route list under test MUST be derived from the application itself — a
  `medikube routes` subcommand emitting the registered route inventory as JSON —
  so that adding a page without adding a test FAILS the build. A hand-
  maintained route list is forbidden; it rots silently.
- The gate runs against a deterministically seeded instance brought up by a
  `medikube seed` subcommand. Authenticated routes are covered, using a seeded
  test user.
- Playwright is a build-time and test-time dependency only. No Node.js is
  present in the runtime image.
- The gate blocks merge. A flaky assertion is fixed or removed, never retried
  into passing.

Rationale: templ compiles, Datastar patches the DOM at runtime, and Tailwind
purges classes at build time. Nothing in the Go type system catches a UI that
compiles and then throws in the browser. Only a browser does.

### IX. Compliance Is A Build Gate, Not A README Paragraph

Every claim MediKube makes about itself is machine-checked, or it is not made.

- Route inventory, generated OpenAPI document, and Playwright route coverage
  MUST agree. A registered route absent from the OpenAPI document, or a
  user-facing route absent from the smoke gate, FAILS the build.
- The generated OpenAPI document is committed and diffed. An unintended API
  change shows up as a reviewable diff, not as a surprise for a client.
- Import boundaries from Principle II, the logging rule from Principle VI, and
  the forbidden dependencies listed under Technology Constraints are enforced
  by linters wired into CI, not by review vigilance.
- A migration MUST be reversible, or its irreversibility MUST be documented in
  the migration file itself.
- CI runs: format, vet, lint, unit and integration tests with race detection,
  the OpenAPI and route-inventory gates, a container build, and the Playwright
  smoke gate. Any red step blocks merge.

Rationale: this repository already contains a sibling project that fails its
build when a single API operation is unaccounted for. That standard is the
house standard, and MediKube inherits it.

## Technology Constraints

The stack is settled. Deviation requires a constitution amendment, not a
plan-level decision.

**Runtime and platform**
- **MediKube is single-instance by construction and MUST NOT be horizontally
  scaled.** PocketBase's embedded SQLite is single-writer and cannot be shared
  between processes, and the realtime hub required by Principle V is in-process.
  Scale vertically. High availability means restarting quickly from a backup,
  not running replicas. The hub MUST NOT be abstracted behind a broker
  interface "in case" — Principle I forbids the speculative seam, and adding one
  later is a contained change because the hub has exactly one consumer.
- **Go 1.27.** Single static binary, no cgo — pure-Go SQLite arrives via
  PocketBase's `modernc.org/sqlite` dependency, so `CGO_ENABLED=0`
  cross-compilation holds. Go 1.27 is REQUIRED, not preferred: PocketBase
  v0.40.1 declares `go 1.27` and imports the 1.27 stdlib package
  `encoding/json/v2` across 67 non-test files, 15 of them in `core/` and
  `apis/`. Building on 1.26.5 fails outright. This makes MediKube the only project
  in this monorepo off the 1.26.5 house standard; the divergence is deliberate
  and forced by the PocketBase decision. CI MUST NOT set `GOTOOLCHAIN=local`,
  which turns this into a misleading toolchain error. Plans MUST budget for Go
  1.27's `encoding/json` v2 semantics in MediKube's own DTOs, which are not fully
  backward compatible around nil-versus-empty slices, `json.RawMessage`, and
  duplicate keys.
- `github.com/pocketbase/pocketbase` v0.40.1 — embedded; owns HTTP routing,
  persistence, auth, file storage, realtime, and the superuser admin UI.
- `spf13/cobra` — reached through PocketBase's `RootCmd`, not added separately.

**Presentation**
- `github.com/a-h/templ` v0.3.1020 — server-rendered, type-safe components.
- `github.com/starfederation/datastar-go` v1.2.2 — hypermedia interactivity.
  The v1 SSE contract is exactly two event types, `datastar-patch-elements`
  and `datastar-patch-signals`. Any material referring to
  `datastar-merge-fragments` or `datastar-merge-signals` describes v0.x and
  MUST NOT be followed.
- Tailwind CSS via the standalone CLI at build time; output embedded with
  `embed.FS`. The Datastar browser bundle is vendored and embedded, never
  loaded from a CDN.

**Cross-cutting**
- `rs/zerolog` — the only application logger.
- `getsentry/sentry-go` — errors and panics, scrubbed, opt-in.
- `prometheus/client_golang` — metrics on a non-public endpoint.
- `go.opentelemetry.io/otel` — tracing, opt-in via OTLP.
- `caarlos0/env/v11` — the only configuration mechanism MediKube itself defines.
  Environment variables into one validated struct at boot. No configuration
  files. **Carve-out:** PocketBase's own `Settings()` store — SMTP, S3, backup
  schedule, OAuth2 providers, superuser IP allowlist, MFA — is database-backed
  and edited through the admin UI. That is not a second configuration system
  MediKube chose; it is part of the platform Principle V forbids reimplementing.
  MediKube MUST NOT mirror those settings into environment variables, and MUST NOT
  invent its own UI for them. Where an unconfigured PocketBase setting would
  silently break a MediKube feature — SMTP for invitations being the live example
  — MediKube MUST surface that state and warn at boot rather than fail quietly.
- `github.com/getkin/kin-openapi` — constructs the OpenAPI document. It is a
  document model, not an HTTP framework, so it does not offend the ban on a
  second router. It is required because PocketBase's route table is NOT
  introspectable: the document is BUILT from MediKube's single declarative route
  registry rather than reflected out of the router, and that same registry
  feeds the route inventory and the Playwright route list, so all three cannot
  drift. Verified: it emits a discriminated `oneOf` for the record family and
  the registry-to-discriminator gate is enforceable.
- `github.com/XSAM/otelsql` — wraps the database driver for Principle VI's
  tracing. Note it requires copying PocketBase's connection pragma string; that
  copy is one of the upgrade-fragility items below.
- `github.com/go-pdf/fpdf` — PDF rendering for one phase-006 export format,
  behind a renderer interface and imported from exactly one package. Pure Go,
  no cgo. Headless-browser PDF generation is FORBIDDEN: it is Node-adjacent and
  cannot live in a distroless image.
- `golang.org/x/text` — permitted as a direct dependency.
- `samber/do` — dependency injection for MediKube's own services. PocketBase's
  `core.App` remains the platform handle; it is not wrapped by the container.
- `samber/lo` — generic helpers, used sparingly per Principle IV.
- `stretchr/testify` — the only assertion library.
- Playwright CLI — the UI gate, build-time only.

**Forbidden dependencies.** These MUST NOT appear in `go.mod`, in the runtime
image, or in a plan:
- `gin-gonic/gin`, `danielgtaylor/huma`, or any second HTTP router or
  OpenAPI-serving framework. PocketBase's router is the router.
- `samber/mo` and `samber/ro`, for the reasons given in Principle IV.
- `samber/slog-zerolog` or any similar adapter: zerolog v1.35.1 already ships
  `zerolog.NewSlogHandler`, so the Principle VI bridge needs no new dependency.
- `spf13/viper` or any second configuration system.
- Any second logging library, any second assertion library, any second DI
  container.
- React, HTMX, Alpine, or any SPA framework. Node.js in the runtime image.
- PocketBase's `jsvm` plugin.
- Any dependency requiring cgo.

**Deliberately out of scope.** MediKube does not implement Paperless-ngx sync,
Papra sync, OCR of lab documents, or migration of existing MediKeep data.
MediKube is greenfield. It MUST, however, be able to export a user's complete
data in a documented portable format, so that no user is ever trapped.

## Development Workflow & Quality Gates

- Work proceeds through Spec Kit: `constitution` -> `specify` -> `clarify`
  (when a specification carries open questions) -> `plan` -> `tasks` ->
  `analyze` -> `implement`. A phase's `plan.md` MUST pass the Constitution
  Check gate before any implementation task is started.
- Phases ship in order and each is independently shippable and verifiable. A
  phase is done when its acceptance scenarios pass as tests, the Playwright
  gate covers its new routes, and CI is green.
- `Taskfile.yaml` is the entry point for every routine action, matching this
  monorepo's convention. A contributor runs tasks, not remembered command
  lines.
- Commits follow Conventional Commits, scoped `medikube`.
- A pull request states which principles its changes are governed by and
  records any Complexity Tracking entries its plan added.
- MediKube is a project inside a shared monorepo. Its integration points —
  including the root `.dockerignore` allowlist and the image build workflow —
  MUST be updated in the same change that introduces the project, because
  omitting them fails the container build with a misleading error.

## Governance

This constitution supersedes team habit, personal preference, and prior
practice. Where a specification, plan, task list, or review comment conflicts
with it, this document wins.

**Precedence.** Principle VII (Patient Privacy) outranks all others. Principle
I (Simplicity) outranks the remainder. Any other conflict is resolved
explicitly in the affected `plan.md`, never silently in code.

**Amendment.** An amendment requires: a written rationale, the version bump
below, and — where it invalidates existing code — a migration note describing
what must change. Adding a forbidden dependency, removing a principle, or
weakening a gate is a MAJOR amendment.

**Versioning.** Semantic. MAJOR for a backward-incompatible governance change,
principle removal, or redefinition. MINOR for a new principle or materially
expanded guidance. PATCH for clarification and wording.

**Compliance.** Every pull request is reviewed against these principles, and
the gates in Principle IX enforce the mechanically checkable subset. Complexity
that is not justified in writing is removed. `/speckit-analyze` is run before
`/speckit-implement` on every phase, and its findings are resolved or recorded.

**Runtime guidance.** Day-to-day development guidance lives in `CLAUDE.md` at
the project root and MUST stay consistent with this constitution; where they
disagree, this constitution governs and `CLAUDE.md` is corrected.

**Version**: 1.3.0 | **Ratified**: 2026-08-26 | **Last Amended**: 2026-08-27
