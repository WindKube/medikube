# Implementation Plan: Walking Skeleton

**Branch**: `001-walking-skeleton` | **Date**: 2026-08-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-walking-skeleton/spec.md`

**Constitution**: [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.3.0 (binding)

## Summary

This phase builds MediGo from an empty directory to a deployable container that a person can
sign in to and keep a medication list in, and it builds it in the exact shape phases 002–006
have already been planned against. Nothing here is provisional: the repository interface, the
DTO boundary, the service contract, the authorization checkpoint, the templ + Datastar page
shape, the route registry, the OpenAPI gate, the test pyramid and the CI gates established in
this phase are copied verbatim by every later phase, and those phases' plans and task lists are
already written on the assumption that they exist.

Concretely this is: a Go 1.27 module with PocketBase v0.40.1 embedded; one validated
configuration struct read from the environment; zerolog with the two-mechanism PocketBase log
bridge; Sentry, Prometheus and OpenTelemetry, all off unless configured; liveness and readiness;
the auto-CRUD lockdown with a boot assertion that refuses to start; **three** collections
(`users` amended, `medications`, `audit_events`) as reversible Go migrations; PocketBase-native
authentication behind hand-written `/api/v1/auth/*` DTOs — including password recovery by email
and email confirmation, both wired to PocketBase's own flows rather than rebuilt; the account
surface at `/api/v1/me`;
**one** clinical record kind — medications — complete from migration through repository, service,
the six-operation `/api/v1/records/{kind}` family, templ + Datastar pages and a Datastar SSE
stream, to tests at every layer of the pyramid; the application shell with its landmarks, error
views, empty states and light/dark; the operator command surface; **one** declarative route
table that simultaneously registers routes, emits `medigo routes` and drives the generated
`api/openapi.json`; the Playwright smoke gate proven to go red; CI end to end; and the
repository-level registration in `/.dockerignore` and `/.github/workflows/build-image.yaml`
without which the container build fails with a misleading "file not found".

**22 `/api/v1` operations, one of them the SSE stream. 9 pages plus 3 error views. 3 collections. 3 migrations. 54
acceptance scenarios, every one of them a named test.**

Five commitments carry most of the design weight. Each is verified against real source rather
than documentation, and each is the reason a later phase can be short:

1. **The record route family is one route table, not fifteen.** Six operations serve every
   clinical kind, `{kind}` is an OpenAPI `enum`, and bodies are a discriminated `oneOf` on
   `kind`. VERIFIED-SOURCE-FACTS FACT 9 already built and ran this with `kin-openapi` v0.144.0
   and closed shared-design risk **R1** ahead of time; this phase turns that probe into a
   permanent gate. Phase 003 adds thirteen kinds and **zero** routes because of it.
2. **There is exactly one authorization checkpoint**, `access.Authorizer`, and every service
   method's first act is to call it. The repository never authorizes and the handler never
   authorizes. In this phase the anchor is `medications.owner`; phase 002 moves it one hop to
   `patients.owner` and phase 005 widens it with shares — and neither has to find a second place
   the decision was being made.
3. **The lockdown is real and it fails closed.** All five API rules are `nil` on every
   non-system collection, `Batch.Enabled` is `false`, a `-1019` middleware 404s the record CRUD
   subtree for non-superusers, and a boot assertion refuses to start the process if any rule is
   non-nil. It is scoped precisely to `/records` and `/api/batch`, because PocketBase's native
   auth endpoints share the `/api/collections/` prefix and are MediGo's authentication mechanism
   (VERIFIED-SOURCE-FACTS FACT 2).
4. **Realtime is 100% MediGo's.** A post-commit record hook publishes `{Kind, RecordID, OwnerID}`
   — IDs, never bodies — to an in-process hub; a per-subscriber Datastar SSE handler re-fetches,
   **re-authorises for that subscriber**, renders and patches. Fanning out IDs is what makes
   per-subscriber authorization possible at all, and Principle VII requires it. PocketBase's
   native realtime is unusable here for three independently verified reasons.
5. **PocketBase's hardcoded 5-minute `WriteTimeout` silently kills every SSE stream** and passes
   every test shorter than five minutes. A mandatory `newStream()` helper clears the per-connection
   write deadline, and the `ServeEvent`'s `*http.Server` is adjusted before the listener starts.
   Both fixes ship, and a CI job holds a stream open for longer than five minutes so the fix
   cannot regress invisibly.

## Technical Context

**Language/Version**: Go **1.27**. `go.mod` declares `module medigo`, `go 1.27` and an explicit
`toolchain go1.27.x`. This is **required, not preferred**: PocketBase v0.40.1's `go.mod` line 3
declares `go 1.27` and 67 non-test files import the Go 1.27 stdlib package `encoding/json/v2`,
15 of them under `core/` and `apis/`. `GOTOOLCHAIN=local go build` on 1.26.5 fails outright with
`go.mod requires go >= 1.27`; go1.27.0 builds it clean (VERIFIED-SOURCE-FACTS FACT 0). MediGo is
therefore the first project in this monorepo off the 1.26.5 house standard that `arc-ui`, `gmod`,
`appbase` and `medikeep-mcp` share. **CI MUST NOT set `GOTOOLCHAIN=local`**, or the divergence
surfaces as a misleading toolchain error rather than as the deliberate decision it is.

**Primary Dependencies** — versions pinned to the monorepo house set where they overlap with
`arc-ui`, which HOUSE-PATTERNS identifies as MediGo's template project, and to the verified
module cache otherwise:

| Module | Version | Used in this phase for |
|---|---|---|
| `github.com/pocketbase/pocketbase` | v0.40.1 | HTTP router, embedded SQLite, migrations, auth, files, cron, `RootCmd`, the admin UI, the test harness |
| `github.com/a-h/templ` | v0.3.1020 | 9 pages, 3 error views, the shell, every component. Pinned via the `tool` directive, never `go install` |
| `github.com/starfederation/datastar-go` | v1.2.2 | the SSE bridge and every interaction. **The browser runtime is v1.0.2 — a different version line. Do not "align" them.** |
| `github.com/caarlos0/env/v11` | v11.4.1 | the one configuration mechanism |
| `github.com/rs/zerolog` | v1.35.1 | the only logger; supplies `zerolog.NewSlogHandler` so the PocketBase bridge needs no adapter dependency |
| `github.com/getsentry/sentry-go` | v0.48.0 | errors and panics only, scrubbed, off unless a DSN is configured |
| `github.com/prometheus/client_golang` | v1.24.1 | metrics on a `127.0.0.1`-bound listener |
| `go.opentelemetry.io/otel` (+ sdk, otlptracehttp) | v1.45.0 | tracing, off unless an endpoint is configured |
| `github.com/XSAM/otelsql` | resolved and pinned in this phase's setup | DB instrumentation through `pocketbase.Config.DBConnect` |
| `github.com/getkin/kin-openapi` | v0.144.0 | OpenAPI 3 document construction from the route registry. Pure Go, serves no HTTP, adds no router — it does not collide with the ban on a second OpenAPI framework |
| `github.com/samber/do/v2` | v2.1.0 | dependency injection, composition root only |
| `github.com/samber/lo` | v1.53.0 | generic helpers, stdlib-first rule |
| `github.com/stretchr/testify` | v1.12.0 | the only assertion library |
| `github.com/spf13/cobra` | v1.10.2 — **the one place this is pinned in the suite** | **Transitive, via PocketBase's `RootCmd`. Never a direct `require`.** The version is `pocketbase@v0.40.1`'s own `go.mod` requirement, read from the module cache and verified, **not** a MediGo choice: it moves when PocketBase moves. Phases 002–006 cite this row rather than restating a version (cross-artifact finding **M2**). |
| `modernc.org/sqlite` | v1.57.0 (transitive) | pure-Go SQLite, so `CGO_ENABLED=0` holds |
| Tailwind CSS standalone | v4.3.3, pinned by `ARG` | build-time only |
| Playwright CLI | build/CI only | the browser gate. **No Node in the runtime image.** |

Absent and staying absent: `gin-gonic/gin`, `danielgtaylor/huma`, `spf13/viper`, `samber/mo`,
`samber/ro`, `samber/slog-zerolog`, PocketBase's `jsvm` plugin, any cgo dependency, React/HTMX/
Alpine.

**Storage**: embedded SQLite through PocketBase (`modernc.org/sqlite`, no cgo) under
`MEDIGO_DATA_DIR` (`/data/pb_data` in the image). Schema is code: three reversible Go migrations
registered into `core.AppMigrations`; `migrations.Register(up, down, filename)` **requires** both
directions, so Principle IX's reversibility rule is enforced by the API itself
(VERIFIED-SOURCE-FACTS FACT 8). `Automigrate` is enabled only when `MEDIGO_DEV=true`; in
production it would try to write `.go` files into a directory the distroless image does not have.

**Testing**: `stretchr/testify` — `require` for preconditions, `assert` for independent
assertions — with table-driven `t.Run` subtests as the default shape. All five pyramid layers
are mandatory and every one of them is created by this phase:

- **unit** — services and domain against hand-written fakes in `<pkg>test`, no database, no HTTP,
  no filesystem, `t.Parallel()` on every one;
- **integration** — repository adapters against `tests.NewTestApp`, which clones
  `internal/testdata/pb_data` into a temp dir and is therefore genuinely isolated and
  `t.Parallel()`-safe at ~11 ms per app (VERIFIED-SOURCE-FACTS FACT 7);
- **contract** — one shared `suite.Suite` per interface, run against **every** implementation
  including the fake, which is how Principle II's Liskov clause becomes mechanical;
- **HTTP** — `tests.ApiScenario`, asserting status, DTO shape and authorization boundary, with
  `ExpectedEvents` used to prove a MediGo route fires the audit hooks and **zero** record-CRUD
  request events;
- **UI render** — templ components rendered to a buffer and asserted on, including every empty
  state, because an empty page is the most common way a smoke gate goes falsely red.

Plus the Playwright smoke + console-error gate at 1440×900 and 390×844, whose route list is
derived from `medigo routes` so a page cannot be added without a test.

**Target Platform**: Linux server. One static binary, `CGO_ENABLED=0`, from
`gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, `VOLUME ["/data"]`, built for
linux/amd64 and linux/arm64 through the monorepo's shared `build-image.yaml`. Browser target:
current desktop and mobile browsers **with scripting enabled** — MediGo does not function
without it and says so (FR-049).

**Project Type**: server-rendered Go web application with an embedded framework (PocketBase),
hypermedia interactivity (templ + Datastar), and a hand-written JSON API. One module, one binary,
no separate frontend service, no companion process, no CDN fetch at runtime.

**Performance Goals**:

- SC-002: a 1,000-medication list is narrowable to any single entry within 10 s of interaction,
  and **every page of that list renders within 2 s**. Budget: one indexed keyset query on
  `(owner, started_on DESC, id)`, `LIMIT 26`, no `COUNT(*)` unless `?count=true` is passed.
- SC-007: a change made in one open view appears in another **within 5 s**, and a view left open
  for **60 continuous minutes** is still receiving updates. Budget: hub publish is non-blocking;
  the per-subscriber handler does one `FindRecordById`, one authorization check and one
  `PatchElementTempl`; a 25 s heartbeat keeps intermediaries and the staleness detector honest.
- SC-013: `/api/v1/readyz` answers within 2 s including when storage is unreachable — its DB
  probe carries a 2 s context deadline.
- Edge case "five thousand medications on one account": no page of the list is materially slower
  than the first, which is what a keyset cursor buys and an `OFFSET` would not.

**Constraints**:

- **Single instance by construction.** The realtime hub is in-process and SQLite is single-writer.
  The hub MUST NOT be abstracted behind a broker interface "in case" — Principle I forbids the
  speculative seam and the hub has exactly one consumer.
- `CGO_ENABLED=0` throughout, including every Docker stage before the last, which is what lets
  templ and Tailwind run natively under `--platform=$BUILDPLATFORM` instead of under QEMU.
- `script-src 'unsafe-eval'` is accepted and permanent (Datastar compiles every expression with
  the `Function` constructor); every other CSP directive is strict, and `'unsafe-inline'` never
  enters `script-src`.
- Records are hard deleted. **No `deleted_at` on any collection in this phase.** Soft delete
  exists only for files, which this phase does not have.
- No PHI in logs, metrics, traces or Sentry. An opaque 15-character id is permitted; a name, an
  email address, a medication name, a dose, a reason or any free text is not.
- Go 1.27 `encoding/json/v2` retrofit semantics apply to **MediGo's own DTOs**, not just to
  PocketBase: slices marshal as `[]` and never `null`, unknown fields are rejected, duplicate
  keys are rejected, and `tests.ApiScenario` normalises bodies through `jsontext` before
  substring matching — so `ExpectedContent` compares against *re-encoded* JSON. Every DTO gets a
  round-trip test.
- Only the **free** Datastar attribute set may be used. `data-persist`, `data-query-string`,
  `data-replace-url`, `data-scroll-into-view`, `data-view-transition`, `data-custom-validity`,
  `data-animate`, `data-match-media`, `data-on-raf`, `data-on-resize`, `@clipboard` and `@fit`
  are Pro. A lint rule enforces the allowlist.
- The v1 attribute delimiter is a colon. `data-on-click` **silently does nothing**; it is
  `data-on:click`, and `data-on-load` is now `data-init`.
- The inline-script SDK family — `ExecuteScript`, `ConsoleLog`, `ConsoleError`, `Redirect`,
  `Redirectf`, `DispatchCustomEvent`, `ReplaceURL`, `ReplaceURLQuerystring`, `Prefetch` — is
  banned outright. Every one appends a literal inline `<script>`, every one fails under
  `script-src 'self'`, and each failure logs a CSP violation that fails the console gate.
- `datastar.WithCompression` is not used; PocketBase binds no gzip on application routes and
  double `Content-Encoding` produces an unreadable stream.

**Scale/Scope**: a self-hosted household instance on hardware the operator owns. Design points:
5,000 medications on one account (spec Edge Cases), 1,000 for the SC-002 latency budget, a few
dozen accounts on one instance, a handful of concurrent SSE subscribers. This phase creates
3 collections, 3 migrations, 22 API operations (one of them the SSE stream), 9 pages,
3 error views, 5 Cobra
subcommands, 2 domain packages, 4 service packages, 3 store packages and the whole test harness.

**No NEEDS CLARIFICATION remain.** Every open question the specification left, and every question
the shared design contract left open for this phase, is resolved in [research.md](./research.md)
with a decision, a rationale and the alternatives rejected. Shared-design risks **R1, R6, R11 and
R12 are closed by this phase**, and R2, R7 and R8 have their permanent mechanisms created here.

## Constitution Check

*GATE: evaluated before Phase 0 research and re-evaluated after Phase 1 design. Both passes
recorded. This is a real gate; where the answer is uncomfortable it is written down rather than
smoothed over.*

### I. Simplicity Is A Gate (KISS) — **PASS (with three recorded entries)**

What this phase deliberately does **not** build, each because no requirement in this
specification asks for it:

- **No patients, no practitioners, no pharmacies, no tags, no reminders.** The shared design
  contract puts some of these in phase 001; this phase's charter governs and they land in 002
  and 003. Building a placeholder now is work thrown away and a schema nobody has requirements
  for.
- **No OAuth2/SSO.** It is PocketBase-native, it is a deployment integration rather than a
  day-one auth flow, and it needs provider configuration that phase 006's operator surface
  already exposes. **Phase 006 owns it**; `OAuth2.Enabled` stays `false` here. See the
  Deviations table below — this is a *recorded* hand-off, not a silent drop.
- **Password recovery by email and email confirmation are NOT dropped.** An earlier draft
  deferred both with OAuth2 and no later phase claimed them, which would have shipped a
  medical-records instance whose only recovery path is a superuser editing the database. They
  are in this phase because they are wiring, not construction: `request-password-reset`,
  `confirm-password-reset`, `request-verification` and `confirm-verification` already exist in
  PocketBase, `mails.SendRecordPasswordReset` and `mails.SendRecordVerification` already build
  and send the messages, and `app.FindAuthRecordByToken(token, core.TokenTypePasswordReset)`
  already validates the token — so MediGo writes four thin DTO handlers, three pages and their
  tests, and reimplements nothing. Principle V requires exactly this.
- **No `medigo purge` subcommand.** FR-037 requires audit entries to be purged *automatically*;
  a PocketBase cron does that. A manual trigger is a knob nobody asked for.
- **No broker abstraction over the realtime hub**, no caching layer, no counter table, no
  materialised list, no `?fields=`, no idempotency keys, no soft delete, no `/medications/new`
  or `/medications/{id}/edit` routes (those are UI states, and as routes they would each cost a
  page, a smoke case and an OpenAPI entry for a deep link nobody has asked for).
- **No second interface where one will do.** The six record operations are the whole clinical
  surface. The fifteen "specialised filter" endpoints a records app grows are query parameters.

Every abstraction this phase introduces has **at least two implementations before it lands** —
the real one and the fake that satisfies Principle III — and both run the same contract suite.
Three entries go to Complexity Tracking; see that table.

**Conflict with Principle II, resolved explicitly here** as Principle I requires: Principle II
would have `internal/web/api` declare one `MedicationService` interface covering list, get,
create, update, delete, plus the account surface. That is six-plus methods and Principle II's own
interface-segregation clause caps a comfortable interface at five. Rather than justify an omnibus,
the record surface is consumed through `records.Service` (5 methods, kind-agnostic, the *only*
thing the generic handler needs) and the account surface through three small consumer-declared
ports — `identity.Registrar` (1), `identity.SessionService` (3), `identity.AccountService` (4).
Simplicity is served by the split, not by the omnibus, and nothing switches on `kind.Kind`.

### II. Interfaces At Every Seam (SOLID) — **PASS**

- **Single responsibility.** `internal/web/api` parses and renders. `internal/service/*` decides.
  `internal/store/*` persists. A handler that builds a filter or a service that writes a response
  is a defect and the review catches it because the packages cannot see each other's tools.
- **Open/closed.** `records.Register(kind, svc, views, schema)` is the extension point. One call
  simultaneously registers a kind's CRUD service, its DTO codec, its list/row/detail templ
  components, its audit hooks, its `/api/v1/records/{kind}` enum value, its OpenAPI `oneOf`
  branch and its two pages. **Nothing anywhere switches on `kind.Kind`.** Phase 003 registering
  eleven more kinds changes zero lines in `internal/records`, `internal/web/api/records.go` or
  `internal/web/stream`.
- **Liskov.** `medicationtest.RepositoryContract(t, factory)` and
  `identitytest.RepositoryContract(t, factory)` each run against the PocketBase implementation
  **and** the in-memory fake. An interface for which a contract test cannot be written covering
  all implementations is the wrong interface, and this phase writes those suites before either
  implementation.
- **Interface segregation.** Every interface introduced is 1–5 methods. The largest are
  `records.Service` (5) and `medication.Repository` (4). There is no `Store` and no `Service`
  omnibus.
- **Dependency inversion.** `internal/domain/**` imports nothing but the standard library and
  `zerolog` (for `MarshalZerologObject`). `internal/service/**` imports neither PocketBase, nor
  `net/http`, nor templ, nor any generated `*_templ.go`. `internal/realtime` imports neither
  PocketBase nor `net/http` — the hub trades in `realtime.Event{Kind, RecordID, OwnerID}` values,
  with the publisher in `internal/platform/pb` and the subscriber in `internal/web/stream`.
  **This is enforced by the `depguard` rule wired into CI from this phase's first commit, and a
  task deliberately adds a forbidden import to prove the rule fires.**
- Constructors take interfaces and return concrete types. Wiring happens once, in `internal/di`,
  from `cmd/medigo/main.go`. No package-level globals, no `init()` registration.

### III. Test-First With testify (NON-NEGOTIABLE) — **PASS**

- The specification asks for it three separate ways (FR-068, FR-069, FR-072) and the success
  criteria repeat it (SC-004, SC-006, SC-009, SC-010). **All 54 acceptance scenarios across the
  six user stories map to a named test in [tasks.md](./tasks.md)**, and every test task is
  sequenced before the implementation task it covers.
- All five pyramid layers exist and `tasks.md` names the file for each. This phase *creates* the
  harness the other five phases use: `internal/testsupport/app.go` (the test-app factory),
  `internal/testdata/pb_data` (the committed fixture every `tests.NewTestApp` clones),
  `internal/testsupport/fixtures.go` (seeded ids as exported constants, so no test contains a
  literal id) and `internal/testsupport/authz.go` (`RunOwnershipMatrix`, the table-driven
  owner-succeeds / stranger-refused helper that phases 002–005 extend by adding rows).
- **Authorization is first-class.** Every one of the six record operations and every account
  operation gets an owner-succeeds / stranger-refused pair, and the refusal is asserted
  **byte-identical** to a genuine not-found apart from `request_id`. The suite is table-driven
  precisely so phase 005 can add the "revoked grantee is refused" row for one line.
- **Never share a `tests.TestApp` across `ApiScenario` cases.** `apis.bindUIExtensions` re-enters
  on every `OnServe` trigger and the handler chain grows until the stack overflows. The
  `TestAppFactory` this phase writes constructs a new app on every call and the helper's doc
  comment says why.
- Test code is production code: duplicated setup is extracted into `internal/testsupport`, which
  is the package every later phase reaches for first.

### IV. Idiomatic Go Over Clever Go — **PASS**

- Errors are values, wrapped with `fmt.Errorf("...: %w", err)`, inspected with `errors.Is` /
  `errors.As`, and mapped to HTTP in exactly one function in `internal/web/errors.go`. No handler
  writes a status-code literal. `samber/mo` and `samber/ro` are absent from `go.mod` and are in
  the `depguard` deny list so they cannot arrive by accident.
- `Patch` structs carry absent-vs-explicit-null with plain pointers: `*string` for absent-vs-set,
  `**clinical.Date` for absent-vs-null. That is stdlib, it marshals correctly under
  `encoding/json/v2`, and it needs no custom `MarshalJSON` to round-trip through the OpenAPI
  generator.
- `context.Context` is the first parameter of every function that performs I/O and is honoured —
  the readiness probe, the SSE handler and every repository query respect cancellation.
- `panic` appears in exactly two places, both programmer-error-at-startup: `cmd/medigo/main.go`
  and `httproute.Registry.Handle`, which panics when a page route declares no landmark or no
  `SmokeURL`. **That second panic is load-bearing** — it is why a page cannot escape the browser
  gate. No request handler panics as flow control.
- Goroutines have an owner and a context-bounded lifetime: the hub's fan-out loop, the metrics
  listener and each SSE subscriber, all with a defined shutdown path through `OnTerminate`.
- Generated `*_templ.go` is committed, marked generated, and excluded from lint and coverage.
- `samber/lo` is used only where it removes a loop that carries no meaning. Chained
  `lo.Map(lo.Filter(...))` is banned, and `lo.Must` is banned outside tests.

### V. PocketBase Is The Platform, Not A Detail — **PASS**

- **Nothing PocketBase provides is reimplemented.** Password authentication, token issue and
  refresh, token-key rotation (which is how "sign out everywhere" and "a password change kills
  every other session" are implemented — FR-007, FR-010), the migration runner and its single
  transaction, `RunInTransaction`, relation cascade delete, cron, and the superuser admin UI are
  all used as-is. Login completes through `apis.RecordAuthResponse(e, rec,
  core.RequestInfoContextPasswordAuth, nil)`, which is exported and is the supported way to
  finish an auth flow from a custom route — it mints the token, fires `OnRecordAuthRequest` and
  records the auth origin.
- **Password recovery and email confirmation are wired, not built.** `POST /api/v1/auth/password-reset`
  calls `mails.SendRecordPasswordReset`; `POST /api/v1/auth/password-reset/confirm` resolves the
  token with `app.FindAuthRecordByToken(form.Token, core.TokenTypePasswordReset)` and then
  `SetPassword` + `Save`, which rotates `tokenKey` and therefore ends every prior session by the
  same mechanism FR-010 already relies on; `POST /api/v1/auth/verify-email` and
  `/verify-email/confirm` do the same through `mails.SendRecordVerification` and
  `core.TokenTypeVerification`. MediGo mints no token, writes no email template and stores no
  reset state. The defaults it inherits are PocketBase's: a reset token valid **30 minutes**, a
  verification token valid **24 hours** (`core/collection_model_auth_options.go`).
- **The one thing this depends on is outside MediGo's configuration mechanism, and it fails
  loudly rather than quietly.** SMTP lives in PocketBase's `Settings()` store, which the
  constitution's Technology Constraints carve out of the `caarlos0/env` rule precisely because
  it is platform state, not MediGo configuration. With `SMTP.Enabled` false, `NewMailClient()`
  returns `&mailer.Sendmail{}` — a shell-out to a local `sendmail` binary that the distroless
  image does not contain — so an unconfigured instance does not "silently drop" the message, it
  fails the send. FR-076 turns that into specified behaviour: a boot warning while SMTP is
  unconfigured (alongside the superuser MFA and IP-allowlist warnings this phase already
  emits), a plain refusal to the person rather than a false "check your email", and the failure
  logged once rather than per attempt.
- **The public API is hand-written `/api/v1` routes with explicit DTOs.** A domain record is
  never serialised to a client; a DTO always mediates. `MedicationCreate` and `MedicationPatch`
  omit every server-owned field *by construction*, which is how privilege escalation is prevented
  by shape rather than by a runtime check somebody can forget.
- **The lockdown.** Five nil rules on every non-system collection; `AuthRule` stays `""` on
  `users` (it is *not* one of the five, and setting it `nil` would disable login entirely);
  `Settings().Batch.Enabled = false`; a `-1019` middleware — after `loadAuthToken` at `-1020`, so
  `e.Auth` is populated — that 404s any path containing `/api/collections/` and `/records`, plus
  `/api/batch`; a boot assertion that **refuses to start** on any non-nil rule; and a
  `tests.ApiScenario` per collection proving the auto-CRUD route 404s for an ordinary user.
- **Schema is code.** Three reversible migrations. Nothing is changed by clicking in the admin UI.
- Account deletion and every medication write that must be atomic run inside
  `app.RunInTransaction`.
- **`OnRecord*Request` CRUD hooks are never used** — they are bound inside the built-in CRUD
  handlers the lockdown disables, so logic placed there is silently dead code. A `forbidigo`
  pattern enforces it. **The auth family is explicitly carved out**: `OnRecordAuthRequest` and
  its siblings live in `bindRecordAuthApi`, which is *not* locked down, and this phase depends on
  `OnRecordAuthRequest("users")` firing to write the sign-in audit row for both MediGo's own
  login route and PocketBase's native one (research D-14).
- Cobra subcommands are registered on PocketBase's `RootCmd`, which already is a
  `*cobra.Command`. No second CLI framework. Two traps are handled: the root sets
  `FParseErrWhitelist{UnknownFlags: true}` so flags are validated in `RunE`, and
  `RootCmd.SetErr(&nopWrite{})` discards cobra's error output, so `main` prints it.
- `jsvm` is never registered. One binary, no cgo, static assets embedded via `embed.FS`, no Node
  at runtime, no CDN fetch.
- **Realtime is not PocketBase's** and this phase states why in the plan rather than discovering
  it in the code: its subscription rules are derived from `ViewRule`/`ListRule`, which are `nil`
  under the lockdown, so every broadcast is silently skipped with no error and no 403; its event
  names are not Datastar's two; and it requires a two-step subscribe handshake a Datastar
  attribute cannot perform.

### VI. One Log Stream, One Trace Context — **PASS**

This is the principle the cross-artifact analysis found scheduled nowhere (finding **C4**). It is
scheduled here, explicitly, as four named tasks.

- zerolog is the only logger. `app.Logger()` is banned by `forbidigo` from this phase's first
  commit, with the PocketBase adapter packages excluded.
- **PocketBase's own logs are bridged by BOTH mechanisms, because each covers what the other
  misses**, and `Settings().Logs.MaxDays` is set to **1** and never **0**:
  1. `pb.App = &loggedApp{App: pb.App, logger: slog.New(zerolog.NewSlogHandler(zl))}` —
     `pocketbase.PocketBase` embeds `core.App` as an exported, mutable field, and grepping all
     non-test v0.40.1 source for `.(*BaseApp)` / `.(*core.BaseApp)` returns zero hits, so nothing
     downcasts past the decorator. This covers the whole request path.
  2. `OnModelCreate("_logs")` emits the record into zerolog and returns **without** calling
     `e.Next()`, so the row is never inserted and `_logs` stays permanently empty. This is
     required because `core/db_tx.go`'s `createTxApp` does `clone := *app` on a `*BaseApp`, so
     transaction-scoped logging bypasses mechanism 1 entirely.
  - At `MaxDays = 0` the record never enters the batch at all, so mechanism 2 never fires and
    PocketBase's backup, mailer, cron and OAuth2 failures go to **nowhere** in production. That
    is why the value is 1.
- Both mechanisms, plus the copied `DefaultDBConnect` pragma string used by the otelsql wiring,
  go onto a **PocketBase upgrade checklist** created by this phase in
  `docs/pocketbase-upgrade-checklist.md`, with a test that fails loudly if the observed behaviour
  changes (shared-design risk R8).
- Every request carries a `request_id` — read from a trusted proxy header or minted — which
  appears on every log line produced while handling it and on the error page or error response
  the person is shown (FR-054). When tracing is enabled, `trace_id` and `span_id` join it.
- **Every background run carries a `run_id` from the same helper**, on the context the cron, job,
  migration and backfill code is handed. It appears on that run's log lines and in the
  `request_id` column of any audit row the run writes, so "why did the purge delete 40,000 rows at
  03:20" is one query. This is not a nicety: `audit_events.request_id` is `Required`, the
  retention purge writes a row, and a cron has no HTTP request (ANALYSIS).
- OpenTelemetry traces, Prometheus counts, Sentry receives errors and panics only. **They must
  not double-report one event** (FR-057): only the 500 branch of the error mapper reports to
  Sentry, and the zerolog Sentry hook checks whether the error has already been captured.
- Metric labels are bounded and allowlisted — `route` is the **registered pattern** from
  `http.Request.Pattern`, never the resolved path. No id, name, email or free text is ever a
  label value. `/metrics` binds `127.0.0.1:9090` on its own listener and is not publicly
  reachable.
- Datastar's `ConsoleLog`/`ConsoleError` are not used; the whole inline-script family is banned.
- `/api/v1/healthz` reflects the process only and never touches the database.
  `/api/v1/readyz` reflects storage reachability and migration state. They are distinct routes
  with distinct meanings, and both are excluded from the activity logger and from the metrics and
  tracing middleware so probe traffic does not dominate the dashboards.

### VII. Patient Privacy Is Structural, Not Procedural — **PASS**

This phase writes the mechanisms every later phase inherits, so it is enumerated rather than
asserted.

| Requirement | How it is structural, not procedural |
|---|---|
| FR-032 authorize at the point of access, never from a caller-supplied id | One checkpoint: `access.Authorizer.Record(ctx, actor, kind, id, need)`. Every service method's **first act**. The repository never authorizes; the handler never authorizes; there is nowhere else to look. |
| FR-033 another person's record is answered exactly as a non-existent one | The error mapper sends `ErrNotFound` **and every authorization failure on owner-scoped data** to `404` with the identical envelope. A response-body-equality test asserts the two are byte-identical apart from `request_id`. `403` is reserved for resources whose existence the caller already knows. |
| FR-034 an anonymous request reveals nothing | `RequireAuth` returns `401 unauthenticated` with a body that names no resource, before any handler runs. Public routes are an explicit allowlist of five: `/login`, `/register`, `/api/v1/auth/*`, `/api/v1/healthz`, `/api/v1/readyz`. |
| FR-035 no general-purpose browse or bulk extract | The lockdown. Five nil rules, `Batch.Enabled = false`, the `-1019` 404 middleware, the boot assertion, and a per-collection `ApiScenario` proving the CRUD route 404s. |
| FR-036/FR-038 the audit trail records that something happened and never what | `audit_events` has **no content column at all** — actor, actor kind, action, target kind, opaque target id, request id, timestamp. There is no field a value could be written into. Rows are produced by post-commit hooks registered by `records.Register`, not by handlers. |
| FR-037 audit entries are not editable or deletable through the application | All five rules nil, no MediGo write path, plus bare `OnRecordUpdate("audit_events")` rejecting unconditionally and `OnRecordDelete("audit_events")` rejecting unless the retention cron marked the context. |
| FR-038 no PHI in the operational record | `clinical.Medication` and `identity.User` implement `MarshalZerologObject` emitting **only** ids. `MarshalJSON` is deliberately *not* implemented on a domain type — a domain type has no wire form. Span attributes and metric labels are allowlists, never denylists. A test exercises every endpoint, captures the log stream and the Prometheus registry, and asserts zero occurrences of any seeded name, email address, medication name, dose or note. |
| FR-039 no unconfigured outbound connection | Sentry and OTLP are off unless a DSN/endpoint is configured; the Datastar bundle and every asset are embedded, never fetched; there is no update ping and no telemetry default. A test asserts that with an empty configuration the process opens no outbound socket. |
| FR-040 the break-glass credential | The admin UI ships. The boot check reads the IP allowlist from `Settings().SuperuserIPs` and MFA from the **superusers auth collection's** `MFA.Enabled` — they live in two different places (FACT 10) — and warns loudly and unmistakably on every start until both are configured. It also warns when `MFA.Rule` is non-empty, because a partial rule means some superuser signs in without a second factor, and when the collection has fewer than two auth methods, because PocketBase refuses to enable MFA without them. Every superuser session writes an `admin_session` audit row. |
| FR-041 secrets | `,file` so a secret arrives from a mounted file rather than the process environment, `,unset` so it is removed from `os.Environ()` after parsing, and a `Config.MarshalZerologObject` that redacts every secret field so the struct cannot be logged by accident. |
| FR-042 the CSP | `default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'`. PocketBase's `securityHeaders()` sets **no** CSP for application routes — this one is MediGo's, set by MediGo's middleware. |
| Hard delete | No `deleted_at` anywhere. Deleting an account cascades through `medications.owner` in one transaction; the destructive action is confirmed in the interface, names what is being destroyed, and writes an audit row. |

### VIII. The UI Must Prove It Renders — **PASS**

- **9 pages** (`/login`, `/register`, `/forgot-password`, `/reset-password/{token}`,
  `/verify-email/{token}`, `/`, `/settings`, `/medications`, `/medications/{id}`) plus
  **3 error views** (403, 404, 500), each declaring its landmark in the route registry, each
  therefore automatically in the Playwright gate at 1440×900 and 390×844.
- **The two token pages carry a deterministic, deliberately invalid `SmokeURL`.** A seeded token
  would be expired by the time the gate ran, so `/reset-password/expired-token-for-smoke` is the
  registered smoke target and the page answers it with `200` and the "this link is no longer
  usable, request another" state inside its own landmark. That is not a workaround: FR-074
  requires exactly that state, and registering it as the smoke case is what puts the most likely
  real-world path — a link opened too late — under the browser gate.
- The route list under test is derived from `medigo routes` at Playwright's collection phase, by
  shelling out to the binary under test. **A page registered without a landmark or a `SmokeURL`
  panics at registration and the process cannot boot**, which is the strongest form of the gate
  Principle VIII asks for.
- Every page asserts: HTTP 200; `banner`, `navigation[name="Primary"]`, `main`, `contentinfo`
  visible; its own landmark visible; `body[data-signals]` present, which is what proves Datastar
  actually booted; zero console errors *and warnings*; zero uncaught page errors; zero failed
  network requests and no 4xx/5xx subresource.
- Every page ships an `@EmptyState(title, body, action)` **inside its own landmark**, and the
  seed deliberately leaves one account's medication list empty, because a legitimately empty page
  is the most common way a smoke gate goes falsely red.
- Two Playwright traps are handled explicitly: `waitUntil: 'networkidle'` never resolves on a
  page holding an SSE stream, so the gate uses `domcontentloaded` plus an explicit landmark
  assertion; and aborted SSE streams produce `net::ERR_ABORTED` on teardown, which is filtered
  **exactly** rather than by muting all `requestfailed` events, or half the gate becomes
  decorative.
- **FR-072 and SC-010 are exit criteria, not aspirations**: the gate must be demonstrated to go
  **red** on a deliberately broken page — once by removing a landmark, once by throwing in the
  browser — and the demonstration recorded, before this phase is accepted. The research
  environment never ran a real browser, so an always-green gate is the risk being closed here
  (shared-design risk **R11**).

### IX. Compliance Is A Build Gate, Not A README Paragraph — **PASS**

- **One declarative route table.** `httproute.Registry.Handle(spec, handler)` registers and
  describes in one indivisible call, and `Bind(se)` is the only place a route reaches PocketBase's
  router. `medigo routes` prints the inventory as JSON **with no database, no port and no
  migrations** — it is a pure function of the binary. This exists because PocketBase's route
  table is not introspectable: `RouterGroup.children` is unexported and Go 1.27's
  `http.ServeMux` still has no pattern-enumeration API.
- **Three gates, all `go test`, all in CI**, modelled on `medikeep-mcp`'s coverage test, which is
  the house precedent: (1) registry ↔ OpenAPI agreement in **both** directions;
  (2) `api/openapi.json` is not stale — asserted as a JSON equality so a changed schema is a
  reviewable diff, not a surprise; (3) every page route has a landmark and a concrete `SmokeURL`
  with no unbound `{param}`.
- **The OpenAPI gate must marshal-then-load.** Validating a programmatically built document in
  place fails with "found unresolved ref", because a constructed `SchemaRef` carries only `Ref`
  and no `Value` (FACT 9). The gate round-trips through `openapi3.NewLoader()` the way any
  consumer would.
- All three migrations supply a real `down`; the API signature requires it.
- `depguard` and `forbidigo` are wired into CI in this phase's **setup** tasks, before any
  domain code exists, and a Polish task deliberately introduces a forbidden import and a
  forbidden `app.Logger()` call to prove both rules fire.
- CI runs: format, vet, lint, unit and integration tests with `-race`, the OpenAPI and
  route-inventory gates, the >5-minute SSE liveness job, a container build, and the Playwright
  smoke gate. Any red step blocks merge.

### Post-Design Re-Check (after Phase 1 artefacts)

Re-evaluated against [research.md](./research.md), [data-model.md](./data-model.md),
[contracts/](./contracts/) and [quickstart.md](./quickstart.md): **all nine still pass.** Four
things changed during design and are recorded here rather than buried:

1. **`audit_events` carries no `ip` column.** The shared design contract lists one. No requirement
   in this specification asks for it — FR-036 enumerates actor, action, target, time and
   correlation id, and nothing else — and an IP address is personal data about the actor retained
   for two years by default in a medical-records application. It is dropped. This also closes
   cross-artifact finding **M3** at its source rather than in phase 002. **Phases 002, 005 and 006
   are reconciled to it**: no audit content rule in the suite lists an `ip`, and phase 006's
   reader DTO and CSV do not publish one. See research D-19.
2. **`access_denied` enters the action vocabulary in this phase**, not in phase 003. Phase 002
   was going to record refusals as a "denied variant" of `read_sensitive` — a qualifier with no
   field to carry it — and phase 003 was going to introduce a real `access_denied` action, which
   would leave anyone filtering the trail with refusals under two encodings either side of an
   arbitrary line. Introducing it where `audit_events` is born costs one enum value and closes
   cross-artifact finding **M2**. See research D-20.
3. **The record kind path segment is plural: `/api/v1/records/medications`.** Phase 002's
   contracts once used the singular `/records/medication` while phase 003's kind registry and the
   shared design contract's own rule 2 both mandate plural kebab-case generated from one Go
   constant. The constant is created here, so the spelling is settled here, and phase 002's
   contracts were the ones corrected — they were, on 2026-08-27, and now read
   `/records/medications` throughout. This closes cross-artifact finding **H1** (analysis run
   2026-08-27; it was **H2** in the run before it). See research D-05 and the Deviations table.
4. **The discriminated `oneOf` gate is proven with a synthetic two-kind registry**, not with the
   one kind this phase ships. A `oneOf` with a single branch proves nothing about the mechanism
   phase 003 depends on, so `internal/openapi/gate_test.go` builds a two-kind fixture registry
   and asserts both kinds appear in the discriminator mapping — exactly the shape FACT 9 ran.
   The production document has one branch and that is correct. See research D-08.

## Project Structure

### Documentation (this feature)

```text
specs/001-walking-skeleton/
├── plan.md                       # This file
├── research.md                   # Phase 0: 40 decisions, each with rationale + alternatives
├── data-model.md                 # Phase 1: 3 collections, enums, indexes, validation, migrations
├── quickstart.md                 # Phase 1: bring it up and hand-verify it end to end
├── contracts/
│   ├── README.md                 # conventions, the error envelope, the operation inventory
│   ├── auth.md                   # 5 ops: config, register, login, refresh, logout
│   ├── account.md                # 4 ops: get/patch/delete me, change password
│   ├── records.md                # 6 ops: the record family + the medication DTOs
│   ├── streams.md                # 1 op: the Datastar SSE contract and its frame shapes
│   ├── health.md                 # 2 ops: healthz and readyz
│   ├── pages.md                  # 9 pages + 3 error views, landmarks, smoke assertions
│   └── cli.md                    # the operator command surface and its output contracts
├── checklists/
│   └── requirements.md           # pre-existing, validated
└── tasks.md                      # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root: `medigo/`)

Everything below is **new**; the directory contains only `.specify/` and `specs/` today. Files
marked `[PB]` are the only packages permitted to import
`github.com/pocketbase/pocketbase/...`, and that boundary is a `depguard` rule, not a convention.

```text
go.mod                                    module medigo, go 1.27, toolchain go1.27.x, tool ( templ )
go.sum
Taskfile.yaml                             arc-ui's task names + seed/routes/openapi/test:e2e
Dockerfile                                4 stages, distroless, project-prefixed COPY
.golangci.yml                             v2 schema; depguard + forbidigo carry Principles II and VI
.gitignore                                medigo, .bin/, *_templ.go, app.css, pb_data/, *.db*
CLAUDE.md                                 day-to-day guidance, consistent with the constitution
README.md
api/openapi.json                          generated, committed, diffed, gated
assets/input.css                          Tailwind v4 entrypoint; scans ../internal/web/**/*.templ
docs/pocketbase-upgrade-checklist.md      the three unexported-internals workarounds (risk R8)

cmd/medigo/
  main.go                                 composition root; the ONLY place that panics [PB]
  version.go                              -ldflags -X main.version

internal/config/
  config.go                               the one caarlos0/env struct, MEDIGO_ prefix
  validate.go                             errors.Join — every problem reported at once
  redact.go                               MarshalZerologObject that redacts every secret
  config_test.go

internal/logging/                                                                          [PB]
  logger.go                               zerolog construction, level, pretty, request-scoped
  pbbridge.go                             mechanism 1: the pb.App decorator
  pblogs.go                               mechanism 2: OnModelCreate("_logs"), no e.Next()
  redact.go                               shared redaction helpers
  logger_test.go  pbbridge_test.go  pblogs_test.go

internal/obs/                                                                              [PB]
  sentry.go                               init + BeforeSend scrubber; off without a DSN
  metrics.go                              registry, collectors, the 127.0.0.1 listener
  tracing.go                              OTel tracer + OTLP exporter; off without an endpoint
  db.go                                   otelsql through pocketbase.Config.DBConnect + pragma drift check
  middleware.go                           request id, request logger, metrics, tracing, panic recover

internal/di/
  container.go                            samber/do v2, composition root only
  providers.go

internal/domain/                          imports stdlib + zerolog and nothing else
  errors.go                               sentinels: ErrNotFound, ErrForbidden, ErrUnauthenticated,
                                          ErrVersionMismatch, ErrConflict, ErrRateLimited
  validation.go                           *ValidationError: every offending field, in one error
  date.go                                 clinical.Date — calendar date, no time, no zone
  page.go                                 Page[T] and SortKey
  doc.go
internal/domain/kind/
  kind.go                                 Kind, its values, enum spelling + path segment + collection
  kind_test.go                            total, injective both ways, round-trips
internal/domain/access/
  actor.go                                Actor{UserID, Role, IsSuperuser, RequestID}
  permission.go                           Permission, Grant
internal/domain/clinical/
  medication.go                           the entity + MarshalZerologObject (ids only)
  enums.go                                MedicationType, MedicationRoute, TherapyStatus + Valid()
  validate.go                             Validate() -> *ValidationError, all fields at once
  *_test.go
internal/domain/identity/
  user.go                                 User + redacting marshaller
  enums.go                                Role, UnitSystem, DateFormat, Theme + Valid()
  password.go                             the published password rules (FR-004)
  validate.go
  *_test.go
internal/domain/audit/
  event.go                                Event — a shape with no content field
  enums.go                                ActorKind, Action, TargetKind + Valid()

internal/records/
  registry.go                             Register(kind, Service, Views, Schema); Kinds()
  service.go                              the 5-method kind-agnostic Service interface
  handler.go                              the ONE generic record handler's dispatch table
  registry_test.go                        registering a kind wires all seven consumers

internal/service/access/
  authorizer.go                           THE authorization checkpoint
  authorizer_test.go
internal/service/audit/
  writer.go                               Record(ctx, Event); no content field can be written
  retention.go                            the purge the cron calls (FR-037)
  *_test.go
internal/service/identity/
  service.go                              Register, ChangePassword, UpdateProfile, DeleteAccount
  session.go                              SignIn, SignOut, Refresh
  ports.go                                Repository, Authenticator, Auditor, Clock
  identitytest/{fake.go,contract.go}
  *_test.go
internal/service/medication/
  service.go                              List/Get/Create/Update/Delete; authz first, always
  ports.go                                Repository, Authorizer, Auditor
  adapter.go                              ~40 lines implementing records.Service for this kind
  medicationtest/{fake.go,contract.go}
  *_test.go

internal/store/                                                                            [PB]
  tx.go                                   RunInTransaction helper
  cursor.go                               opaque HMAC-signed keyset cursors
  mapping.go                              *core.Record <-> domain helpers, date handling
  filter.go                               typed query -> PB filter; the DSL never leaves here
internal/store/migrations/                                                                 [PB]
  1756100100_users_profile.go             amend users: fields, nil rules, index, token config
  1756100200_medications.go               create medications
  1756100300_audit_events.go              create audit_events
  assertions.go                           nil rules, no FileField unprotected, cascade matrix
  register.go
  *_test.go
internal/store/medication/{repo.go,repo_integration_test.go}                               [PB]
internal/store/identity/{repo.go,repo_integration_test.go}                                 [PB]
internal/store/audit/{repo.go,repo_integration_test.go}                                    [PB]

internal/platform/pb/                                                                      [PB]
  app.go                                  pocketbase.NewWithConfig, HideStartBanner, DBConnect
  settings.go                             Logs.MaxDays=1, Batch.Enabled=false, rate limits, token TTL
  serve.go                                OnServe: WriteTimeout override, middleware order, Bind
  lockdown.go                             the -1019 record-CRUD 404 middleware
  assert.go                               boot assertions; refuses to start
  adminwarn.go                            the MFA + IP-allowlist boot warning (FACT 10)
  hooks.go                                audit hooks, realtime publisher, audit immutability guards
  cron.go                                 the audit retention job
  *_test.go

internal/realtime/
  hub.go                                  channel + map. IDs, never bodies. No broker interface.
  hub_test.go

internal/httproute/                                                                        [PB]
  registry.go                             Route, Registry, Handle, Bind, Routes, SmokeTargets
  routes.go                               THE declarative table: 22 API (the stream is one) + 9 pages
  registry_test.go  gate_test.go
internal/openapi/
  generate.go                             kin-openapi document built from the registry
  schema.go                               DTO reflection + the discriminated oneOf
  gate_test.go                            both-directions agreement; staleness; marshal-then-load

internal/web/                                                                              [PB]
  errors.go                               the ONE error->status mapper and the envelope
  dto.go                                  decode with unknown-field rejection; encode
  etag.go                                 ETag from updated; If-Match required on PATCH/DELETE
  cursor.go                               cursor encode/decode at the HTTP edge
  actor.go                                e.Auth -> access.Actor into the request context
  session.go                              the HttpOnly session cookie, bound at priority -1021
  security.go                             CSP and the other security headers
  render.go                               templ -> RequestEvent; the non-SSE element-patch path
  *_test.go
internal/web/api/                                                                          [PB]
  auth.go     auth_test.go                config, register, login, refresh, logout, password
                                          reset request/confirm, verification request/confirm
  me.go       me_test.go                  get/patch/delete me, change password
  records.go  records_test.go             the six-operation family
  health.go   health_test.go              healthz, readyz
  dto_medication.go  dto_me.go  dto_auth.go
  *_authz_test.go                         owner succeeds / stranger refused, byte-identical
internal/web/page/                                                                         [PB]
  shell.go                                NavState, the layout wrapper, theme resolution
  login.go  register.go  dashboard.go  settings.go  medications.go
  forgot_password.go  reset_password.go  verify_email.go
  errors.go                               403 / 404 / 500 renderers
  *_test.go
internal/web/stream/                                                                       [PB]
  stream.go                               THE mandatory newStream() helper
  records.go                              per-subscriber re-authorisation, heartbeat
  timeout_test.go                         the >5-minute liveness assertion
  *_test.go
internal/web/views/
  layout.templ                            the shell: skip link, banner, nav, main, alert, live, footer
  shell/{nav.templ,theme.templ,noscript.templ}
  components/{empty_state.templ,field_error.templ,confirm.templ,pagination.templ}
  auth/{login.templ,register.templ,forgot_password.templ,reset_password.templ,verify_email.templ}
  settings/{profile.templ,password.templ,danger_zone.templ}
  records/{medication_row.templ,medication_list.templ,medication_detail.templ,medication_form.templ}
  errors/{forbidden.templ,not_found.templ,server_error.templ}
  ids/ids.go                              deterministic DOM ids, used by BOTH templ and Go
  *_test.go                               render-to-buffer assertions, empty states included
internal/web/static/
  embed.go   app.css   datastar.js (vendored v1.0.2)   .gitkeep

internal/cli/                                                                              [PB]
  root.go                                 registers subcommands on PocketBase's RootCmd
  seed.go                                 deterministic demo data, two accounts
  routes.go                               the inventory as JSON; no DB, no port
  openapi.go                              writes api/openapi.json
  healthcheck.go                          dials /api/v1/readyz from inside the container
  *_test.go

internal/testsupport/
  app.go                                  TestAppFactory — a NEW app per call, never shared
  fixtures.go                             seeded ids as exported constants
  authz.go                                RunOwnershipMatrix — the table phases 002-005 extend
internal/testsupport/phileak/                                                               [PB]
  capture.go                              captures the zerolog stream, the Prometheus gatherer,
                                          the OTel span recorder and a stub Sentry transport
  exercise.go                             drives every operation of the phase against a
                                          sentinel-seeded instance; phases 002-006 extend it
  phileak_test.go                         the assertion: zero sentinels in any sink
internal/testdata/pb_data/                the committed fixture every tests.NewTestApp clones

e2e/
  package.json  playwright.config.ts      desktop 1440x900 + mobile 390x844 projects
  routes.ts                               shells out to `medigo routes` at collection time
  auth.setup.ts                           signs in as the seeded account, stores the state
  smoke.spec.ts                           the gate: 200, landmarks, zero console/page/network errors
  routes.gate.spec.ts                     fails if a registered page has no smoke case
  a11y.spec.ts                            keyboard reachability and visible focus
  README.md                               how the red-gate demonstration was performed

.github/workflows/medigo-ci.yml           kept with the project; placed at the repo root to run
```

**Monorepo integration — two files outside `medigo/`, changed in the same commit** (constitution
Development Workflow; omitting either fails the container build with a misleading "file not
found"):

```text
/.dockerignore                    + !medigo/ in the allowlist block, and the mirror of
                                    arc-ui's build-output exclusions plus medigo/pb_data/
/.github/workflows/build-image.yaml
                                  + medigo in workflow_dispatch.inputs.project-name.options
/.github/workflows/medigo-ci.yml  the copy that actually executes (house convention: the
                                    canonical file lives with the project)
```

**Structure Decision**: this is the package layout mandated by SHARED-DESIGN §4, created for the
first time here and unchanged thereafter. One domain subpackage per bounded concept
(`kind`, `access`, `clinical`, `identity`, `audit`); one service package per aggregate; one store
package per repository; every PocketBase import confined to the `[PB]`-marked packages and
enforced by `depguard`. The `<pkg>test` convention (`medicationtest`, `identitytest`) for fakes
and contract suites is established here and copied by every later phase. The build context is the
**repository root**, because that is what the shared workflow passes unconditionally, so every
`COPY` in the Dockerfile is project-prefixed (`COPY medigo/go.mod medigo/go.sum ./`) — a bare
`COPY go.mod ./` works locally and breaks in CI, and is the likeliest single miss in the phase.

## Deviations from the shared design contract

SHARED-DESIGN.md declares itself binding "until it is amended", and four later phases have
already overridden its phase table on the grounds that the accepted charters govern. This phase
does the same and records every departure here, in the table format phases 003–005 use, so the
next reader does not have to reconstruct it. **No design decision in the contract is changed —
only its allocation, plus three corrections it is wrong about.**

| Contract says | This phase does | Why |
|---|---|---|
| Phase 001 is `foundation` and owns patients, multi-patient switching, and **allergies** + **emergency contacts** as its first two kinds | Phase 001 owns accounts and **medications** as its single kind. Patients arrive in 002; allergies and emergency contacts in 003. | This phase's charter, which the specification's Assumptions state explicitly: *"Where that contract's phase table allocates medications to a later phase, this phase's charter governs."* Phase 002's plan already records the mirror-image decision. |
| Phase 001 is 29 operations *(a pre-amendment figure from SHARED-DESIGN, kept only as provenance — it cannot be reconstructed from the amended §2.3 and nothing reads it)* | 22 operations | The eight not built are the patient and photo surface (→ 002) and OAuth2 (op 4, → 006). Contract operations **7** and **8** (password reset request and confirm) are built here, and two operations the contract does not list are added for email confirmation — see the two rows below. |
| Contract operations **4, 7 and 8** and the three auth pages are "deferred out of the suite" and belong to no phase (§2.3, §3.1) | Ops **7** and **8** are built **here**; op **4** (`POST /api/v1/auth/oauth2`) is claimed by **phase 006**; email confirmation adds ops **94** and **95** | Deferring them left a self-hosted medical instance whose only password recovery is a superuser editing the database, and left three contract operations and three contract pages owned by nobody (cross-artifact finding H7). The decision recorded for the suite: recovery and confirmation are PocketBase-native and belong where authentication is built; external sign-in is a deployment integration and belongs with the operator surface that configures providers. **The contract has been amended to match**: SHARED-DESIGN §2.3 now lists ops 7, 8, 94 and 95 under phase 001 and op 4 under phase 006 (total **94**), and §3.1 returns the three auth pages to phase 001 (total **58** + 3 error views). Resolves this phase's task **T316**. |
| The contract lists no operation for email confirmation | **94** `POST /api/v1/auth/verify-email` (authenticated: send me the message again) and **95** `POST /api/v1/auth/verify-email/confirm` (public: here is the token) | The contract allocated the `/verify-email/{token}` **page** but no operation to serve it, so the page had nowhere to post. Two operations, both thin wrappers over `mails.SendRecordVerification` and `core.TokenTypeVerification`. Numbers 94 and 95 continue the contract's own additive numbering (91–93 are already additions) rather than renumbering anything. |
| Phase 001 is 13 pages *(pre-amendment, same provenance note as the row above)* | 9 pages + 3 error views | `/patients` and `/patients/{id}` → 002; `/allergies`, `/emergency-contacts` and their detail pages → 003. `/forgot-password`, `/reset-password/{token}` and `/verify-email/{token}` **are built here**, with the features they serve. |
| `audit_events` carries an `ip` column | It does not | No requirement asks for it; FR-036 enumerates the fields and an IP is not among them. An IP address is personal data about the actor, retained two years by default, in a medical-records application. Closes cross-artifact finding **C2** (analysis run 2026-08-27; it was **M3** in the run before it). Research D-19. |
| The `audit_events` vocabularies are a phase-001 deliverable, in full (19 actions, the 15 record kinds + 8 platform kinds) | Delivered **in full**, and extended by exactly one action: `access_denied`. Twenty actions, twenty-three target kinds, declared by this phase's migration whether or not this phase writes them — ten actions and twenty target kinds are written by a later phase | An earlier draft of this plan narrowed the migration to the ten actions and three target kinds this phase writes, and left each later phase to add its own delta. That is a runtime failure, not a tidy-up: a `SelectField` write with an undeclared value fails validation, so the trail would have crashed on the first share, the first non-owner photo fetch and the first backup. Declaring the contract's list costs two string slices. Every later phase's vocabulary migration now asserts the **complete** expected set rather than its delta (T070a), so a value written but never declared is a red test. Closes cross-artifact finding **C1**. |
| `access_denied` arrives in phase 003 | It arrives here, with `audit_events` | A trail that encodes refusals two different ways either side of phase 003 cannot be filtered by anyone. One enum value, introduced where the collection is born. Closes cross-artifact finding **M2**. Research D-20. |
| The record path segment (contract rule 2) is plural, generated from one Go constant — but phase 002's contracts use the singular `/records/medication` | Plural: `/api/v1/records/medications`, from `kind.Kind.Segment()` | The constant is created here, so the spelling is settled here, and it is the spelling phase 003's registry already declares. Phase 002's contracts and quickstart **were** corrected to match on 2026-08-27 and now read `/records/medications` throughout. Closes cross-artifact finding **H1** (analysis run 2026-08-27; it was **H2** in the run before it). Research D-05. |
| Health and readiness live at `/api/v1/healthz` and `/api/v1/readyz` (contract §2.3), while the observability dossier proposes `/api/v1/health/live` and `/api/v1/health/ready` | `/api/v1/healthz` and `/api/v1/readyz` | The contract is the binding document; the dossier is a reference. Stated so nobody has to diff them. |
| The `forbidigo` ban is on `OnRecord.*Request` | The ban is on the **CRUD** request family only; the **auth** request family is explicitly permitted | `bindRecordAuthApi` is not disabled by the lockdown, so `OnRecordAuthRequest` *does* fire — and this phase depends on it to audit sign-in for both MediGo's own login route and PocketBase's native one. Banning it would leave the auth trail unwritable. Research D-14. |
| Risk **R1** (discriminated `oneOf`) is open and assigned to this phase | Recorded as **CLOSED** by VERIFIED-SOURCE-FACTS FACT 9, and turned into a permanent regression gate here | It was built and run: document validates after a marshal→load round trip, and every registered kind appears in the discriminator mapping. The 94-operation budget (SHARED-DESIGN §2.3) and phase 003's three-route shape hold. |
| Risk **R6** (reading MFA and the IP allowlist at boot) is open | Recorded as **CLOSED** by FACT 10, and implemented here | Both are readable from Go, but from **different** places: `Settings().SuperuserIPs` and the superusers collection's `MFA.Enabled`. A plan that assumed they sat together would have been wrong. |
| Risk **R3** (FTS5 availability) is open | Recorded as **CLOSED** by FACT 11 | FTS5, `MATCH` and `rank` all work in `modernc.org/sqlite` v1.57.0. Nothing in this phase uses them; phase 004 may stop hedging. |

**The ownership gap this phase used to raise is now closed, and closed here.** An earlier draft
of this plan recorded password recovery, email confirmation and external sign-in as out of scope
with an operator workaround, and flagged that the shared design contract's operations 4, 7 and 8
and its three auth pages belonged to nobody across 001–006 (cross-artifact finding H7, research
D-04). That allocation has been decided for the suite and is applied above: **recovery and
confirmation land in this phase** — they are PocketBase-native, so under Principle V MediGo wires
them rather than builds them, and a medical-records instance whose forgotten password is
unrecoverable without superuser intervention is broken on day one — and **external sign-in lands
in phase 006**, with the operator surface that configures providers. Nothing is left unallocated.
Task **T316** is answered rather than open, and phase 006's plan carries the mirror-image row.

**What this phase must be honest about instead** is the dependency: both flows do nothing useful
on an instance whose operator has not configured outgoing mail in PocketBase's settings store. The
constitution requires that state to be surfaced rather than fail quietly, so it is a boot warning
(FR-076), a plain refusal to the person, and a single logged failure — never a "check your email"
for a message that was never sent.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| **CT-1.** The PocketBase log bridge reaches past the framework's public surface twice: it reassigns the exported embedded `pb.App` field to a decorator, and it binds `OnModelCreate("_logs")` and deliberately returns **without** calling `e.Next()` so a write PocketBase intends never happens. Both rest on observed internal behaviour rather than on a documented extension point, and both must be re-verified on every PocketBase upgrade. | Constitution Principle VI requires exactly one log stream, and PocketBase v0.40.1 hardcodes its slog handler in `core.BaseApp.initLogger` — neither `pocketbase.Config` nor `core.BaseAppConfig` exposes a logger or handler field, and `app.logger` is unexported. There is no injection point. Without both mechanisms, PocketBase's backup, mailer, cron and OAuth2 failures either go nowhere at all (`MaxDays = 0`) or land in a second, invisible log store (`MaxDays > 0` with no interception). In a medical-records application, silently discarding the framework's own failure reports is not acceptable. | *One mechanism instead of two* was rejected on evidence: `core/db_tx.go`'s `createTxApp` does `clone := *app` on a `*BaseApp`, so every transaction-scoped log line bypasses the decorator entirely — the two mechanisms cover disjoint sets of lines. *`MaxDays = 0`* was rejected because `BeforeAddFunc` returns `MaxDays > 0`, so the record never enters the batch and mechanism 2 never fires. *Reading `_logs` on a schedule and forwarding it* was rejected as a third log store with a polling delay and a retention window. *Forking PocketBase* was rejected outright. **Mitigations:** the decorator is safe by verification — grepping all non-test v0.40.1 source for `.(*BaseApp)` / `.(*core.BaseApp)` returns zero hits, so nothing downcasts past it; both mechanisms have tests that fail loudly if the behaviour changes; and both, plus the copied `DefaultDBConnect` pragma string, are the three entries in `docs/pocketbase-upgrade-checklist.md` created by this phase (shared-design risk R8). |
| **CT-2.** `internal/records` is a registry with a dispatch table, introduced with **one** registered kind. Principle I says an abstraction needs at least two real implementations, or a test double, before it is introduced — and on the day this phase ships, medications is the only kind. | The registry is not speculative: it is the phase's *charter*. The specification's Assumptions state that everything this phase establishes "is the template that phases 002 through 006 copy", and phase 003's plan and task list are already written against `records.Register` adding thirteen kinds and **zero** routes. Introducing it in phase 003 instead would mean rewriting phase 001's medication handler, its DTO codec, its templ registration, its audit hooks and its OpenAPI branch — i.e. paying the abstraction cost anyway, later, with a migration. The alternative is not "no abstraction", it is "the same abstraction, retrofitted". | *Defer the registry to phase 003* was rejected on the cost above, and because the shared design contract's entire 94-operation budget (§2.3) depends on the record family existing from the start. *A `switch kind` in the handler* was rejected as the exact violation Principle II's open/closed clause names. **The Principle I clause is satisfied on its own terms, not waived:** the registry ships with **two** implementations of `records.Service` from day one — `medication.Adapter` and `recordstest.FakeKindService`, a second fully registered synthetic kind used only by `internal/records/registry_test.go` and `internal/openapi/gate_test.go`. That second kind is what makes the discriminated-`oneOf` gate meaningful (research D-08) and what proves the registry is genuinely kind-agnostic before phase 003 bets thirteen kinds on it. It is test-only and never registered in a production build, asserted by a test. |
| **CT-3.** `internal/store/cursor.go` derives its HMAC key by HKDF from PocketBase's persisted `users` collection auth-token secret rather than from a MediGo configuration value, which means a MediGo security property depends on a PocketBase field MediGo does not own. | Cursors must be opaque *and* unforgeable — a client that can mint a cursor can choose the keyset boundary and page through rows it was never offered. The key must also survive a process restart, because SC-007 requires a list left open for sixty minutes to still work. A per-process random key breaks every open page on every deploy; a configured key adds a required secret to `MEDIGO_*` that the operator must generate, cannot rotate safely, and will paste into a shell history. | *A configured `MEDIGO_CURSOR_KEY`* was rejected because SC-008 requires an operator to reach a running instance in under ten minutes with the documented settings, and a mandatory generated secret is the single most common reason that fails. *An unsigned opaque cursor (base64 of the keyset tuple)* was rejected: opacity is not integrity, and the forged-cursor path is exactly a Principle VII problem. *A server-side cursor table* was rejected as state with a lifetime nobody wants to own. **Mitigations:** the derivation is one function with a test asserting a cursor minted before a restart still verifies after one; the key never leaves `internal/store`; a forged or tampered cursor is `400 invalid_cursor` and is audited; and the exact PocketBase field name is confirmed against v0.40.1 in the first setup task, with a documented fallback to a configured key if it is not readable (research D-25). |

## Phase Exit Criteria

This phase is done when, and only when:

1. All **54** acceptance scenarios exist as named, passing automated tests (FR-068, SC-004), and
   `specs/001-walking-skeleton/traceability.md` maps every one to its test with no empty row —
   **and, in the same file, every one of this phase's functional requirements to the task ids that
   satisfy it, and every success criterion to its task or to a criterion below.** The join is
   mechanical: an unmapped requirement, or a success criterion neither mapped nor marked
   `[outcome metric]` in `spec.md`, fails the phase (cross-artifact finding M7).
2. Every one of the 22 operations has an owner-succeeds / stranger-refused pair, and the refusal
   is byte-identical to a genuine not-found apart from `request_id` (FR-069, SC-006).
3. `medigo routes`, `api/openapi.json` and the Playwright coverage agree in both directions, and
   the `api/openapi.json` diff in the phase's pull request is reviewed operation by operation
   (FR-064, FR-065, SC-011).
4. The Playwright gate passes on all 9 pages and 3 error views at 1440×900 and 390×844 against a
   freshly seeded instance, **including on the deliberately empty medication list and on the two
   expired-link states** (FR-066, SC-003).
5. **The gate has been demonstrated to go RED** — once with a landmark removed, once with a
   deliberate browser error — and both demonstrations are recorded in `e2e/README.md` (FR-072,
   SC-009, SC-010, shared-design risk R11). *This is the criterion most likely to be skipped and
   the one whose absence makes every other UI claim worthless.*
6. A CI job holds an SSE stream open for **more than five minutes** and it survives, proving the
   `WriteTimeout` override (SC-007, shared-design risk R7).
7. A grep of the captured log stream, Prometheus registry, span attributes and Sentry envelope
   after exercising every endpoint finds **zero** names, email addresses, medication names,
   doses, reasons or notes (FR-038, SC-005).
8. Password recovery has been demonstrated end to end against a mail sink — request, link, new
   password, every prior session dead — **and** on an instance with outgoing mail unconfigured,
   where the person is refused in plain language and the operator was warned at boot (FR-073,
   FR-074, FR-076, SC-016).
9. `task check` is clean, and `depguard` and `forbidigo` have each been proven to fire on a
   deliberate violation that was then reverted (Principle IX).
10. A container image is produced from a clean checkout by the shared pipeline on the first
    attempt, with no manual step (FR-071, SC-015).
11. `quickstart.md` has been run end to end on a clean machine by somebody who did not write it,
    and the ten-minute claim in SC-008 either held or the document was fixed until it did.
