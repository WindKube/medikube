---
description: "Task list for phase 001 — Walking Skeleton"
---

# Tasks: Walking Skeleton

**Input**: Design documents in `/specs/001-walking-skeleton/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: **MANDATORY.** Constitution Principle III makes test-first non-negotiable and the
specification requests tests explicitly (FR-068, FR-069, SC-004). Every test task precedes the
implementation it covers. A task that says TEST is not done until the test **fails for the right
reason** — a test that passes before the implementation exists is proving nothing.

**Organization**: by user story, in priority order, so each story is independently completable and
independently testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: different file, no dependency on another unstarted task — safe to run in parallel
- **[Story]**: US1…US6, or blank for Setup/Foundational/Polish
- **TEST**: a `go test` / Playwright assertion that **blocks merge**. Must fail for the right reason first.
- **BENCH**: a `Benchmark*` function that `go test ./...` does not run and that **does not block
  merge**. Used where the honest measurement is a trend a human reads, not a threshold — see T202a
  and Constitution VIII's ban on flaky gate assertions (ANALYSIS N13).
- Every task names the **exact** file path
- Paths are relative to `medikube/` unless they start with `/`, which means the repository root

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: the module, the toolchain, the lint gates and the monorepo wiring. Nothing here
imports anything; this is the ground the rest stands on.

- [ ] T001 Create `go.mod` — `module medikube`, `go 1.27`, `toolchain go1.27.x`, and the
  `tool ( github.com/a-h/templ/cmd/templ )` directive. **Not 1.26.5**: PocketBase v0.40.1 imports
  `encoding/json/v2` in 67 non-test files and will not build (research D-02, VERIFIED FACT 0).
- [ ] T002 Add every pinned dependency at the exact version in `plan.md`'s Technical Context and
  commit `go.sum` — pocketbase v0.40.1, templ v0.3.977, datastar-go v1.2.2, zerolog v1.34.0,
  caarlos0/env v11.3.1, samber/do/v2 v2.1.1, kin-openapi v0.144.0, testify v1.11.1. **No Gin, no
  Huma, no Viper, no samber/mo, no samber/ro, no samber/slog-zerolog** (constitution Forbidden).
- [ ] T003 [P] Create the package skeleton with `doc.go` in each package per `plan.md` Project
  Structure, so the `[PB]` boundary exists before any code can cross it.
- [ ] T004 [P] Create `.golangci.yml` on the **v2** schema with the arc-ui baseline plus the two new
  blocks: `depguard` denying `github.com/pocketbase/...` everywhere except the `[PB]` packages
  (Principle II) and `forbidigo` banning `app.Logger()`, `slog.`, `log.Print` and the
  `OnRecord(Create|Update|Delete|View|List)Request` family outside `internal/platform/pb/hooks.go`
  (Principles VI, and research D-14 for the auth-family carve-out).
- [ ] T005 [P] Create `.gitignore` — `medikube`, `.bin/`, `*_templ.go`, `assets/app.css`, `pb_data/`,
  `*.db*`, `e2e/node_modules`, `e2e/test-results`.
- [ ] T006 [P] Create `Taskfile.yaml` with arc-ui's task names (`gen`, `vet`, `lint`, `test`,
  `build`, `run`, `docker:build`) plus `migrate`, `seed`, `routes`, `openapi`, `smoke`,
  `test:e2e` and **`test:phileak`** (the build-tagged PHI-leak suite that phases 002–006 extend
  rather than re-declare — cross-artifact finding M6). `gen` runs `templ generate` then Tailwind
  and everything else depends on it.
- [ ] T007 [P] Create `assets/input.css` — Tailwind v4 entrypoint with `@source` pointing at
  `../internal/web/**/*.templ`. Pointing it at `.go` files finds nothing and produces an empty
  stylesheet with no error (house-pattern trap).
- [ ] T008 [P] Vendor Datastar browser runtime **v1.0.2** to
  `internal/web/static/datastar.js` and embed it via `internal/web/static/embed.go`. The Go SDK is
  v1.2.2 — **different version lines, both correct** (research D-33).
- [ ] T009 [P] Create `cmd/medikube/version.go` with the `version` var populated by
  `-ldflags -X main.version`, and wire the flag into `Taskfile.yaml` and the `Dockerfile`.
- [ ] T010 [P] Create the 4-stage `Dockerfile` — templ+tailwind, Go build with `CGO_ENABLED=0`,
  distroless runtime, `USER 65532:65532`. **Every `COPY` is project-prefixed**
  (`COPY medikube/go.mod ./`) because the shared workflow passes the repository root as the build
  context. Pin `TAILWIND_VERSION`, use the `x64` asset name (**not** `amd64`), and create every
  directory in an earlier stage — distroless has no shell and no `mkdir`. No `HEALTHCHECK`.
  The image is the single self-contained artefact, with no companion service (FR-061).
- [ ] T011 [P] Add `!medikube/` to the allowlist block of `/.dockerignore`, mirroring arc-ui's
  build-output exclusions plus `medikube/pb_data/`. **Omitting this fails the image build with a
  misleading "file not found"** (constitution Development Workflow; recorded in the user's memory).
- [ ] T012 TEST `internal/platform/pb/build_smoke_test.go` — a test that merely imports
  `github.com/pocketbase/pocketbase` and constructs an app, run **first**, to prove the toolchain
  actually builds PocketBase v0.40.1 before anything is written on top of it. On Go 1.26.5 it
  fails at compile time on `encoding/json/v2` (VERIFIED FACT 0).
- [ ] T013 [P] TEST `internal/architecture/forbidden_deps_test.go` — a source walk asserting none of
  the forbidden dependencies is in `go.mod` or imported anywhere: Gin, Huma, Viper, samber/mo,
  samber/ro, samber/slog-zerolog, any React or HTMX asset, jsvm, or any cgo-requiring package
  (constitution Forbidden).
- [ ] T014 [P] Add `medikube` to `workflow_dispatch.inputs.project-name.options` in
  `/.github/workflows/build-image.yaml`.
- [ ] T015 [P] Create `.github/workflows/medikube-ci.yml` and copy it to
  `/.github/workflows/medikube-ci.yml` (house convention: the canonical file lives with the
  project, the root copy executes). Jobs: `gen` → `vet` → `lint` → `test` → `openapi-diff` →
  `e2e` → `stream-liveness`. **Do not set `GOTOOLCHAIN=local`.**
- [ ] T016 [P] Create `README.md` and `CLAUDE.md` (day-to-day guidance consistent with the
  constitution; no restatement of it).
- [ ] T017 [P] Create `docs/pocketbase-upgrade-checklist.md` listing the three places this phase
  reaches past a public API — the `pb.App` decorator, the `_logs` hook, and the `WriteTimeout`
  override — with the symptom each produces if a PocketBase upgrade breaks it (risk R8, CT-1).

**Checkpoint**: `task lint` and `task build` succeed on an empty program.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: everything phases 002–006 already assume exists. **No user story can start until this
phase is done**, and no later phase gets to re-litigate any of it.

**This is the largest phase in the suite and that is correct** — it is the entire reason phase 001
exists.

### Configuration and the log stream

- [x] T018 [P] TEST `internal/config/config_test.go` — every `MEDIKUBE_` variable parses, defaults
  apply, an absent `MEDIKUBE_DATA_DIR` fails, `MEDIKUBE_DRAIN_MAX <= MEDIKUBE_DRAIN_DELAY` fails, and
  **every** validation problem is reported in one error rather than the first (FR-051). `MEDIKUBE_DATA_DIR`
  is the one location everything the instance holds lives under (FR-061).
- [x] T019 [P] TEST `internal/config/redact_test.go` — marshalling the config to zerolog emits no
  secret value for any secret-bearing field, asserted field by field via reflection so a newly
  added secret fails the test by default (FR-041).
- [x] T020 Implement `internal/config/config.go` — the ONE `caarlos0/env` struct, `MEDIKUBE_` prefix,
  per the observability dossier's field list. **No Viper.**
- [x] T021 Implement `internal/config/validate.go` using `errors.Join`.
- [x] T022 Implement `internal/config/redact.go` — `MarshalZerologObject` redacting every secret.
- [x] T023 [P] TEST `internal/logging/logger_test.go` — level, pretty mode, request-scoped child
  loggers, and that the correlation id appears on every line (FR-053, FR-054).
- [x] T024 [P] TEST `internal/obs/request_log_test.go` — the request logger records method, path,
  status, duration and correlation id and **never** a request or response body, a query string or
  a cookie (FR-038).
- [x] T025 [P] TEST `internal/logging/pbbridge_test.go` — a log written through PocketBase's own
  framework logger arrives in the captured zerolog stream. **Mechanism 1**: the exported embedded
  `pb.App` field is reassigned to a decorator (CT-1).
- [x] T026 [P] TEST `internal/logging/pblogs_test.go` — a record written to the `_logs` collection is
  diverted to zerolog and **not** persisted. **Mechanism 2**: `OnModelCreate("_logs")` returning
  **without** calling `e.Next()`. The same test asserts `Logs.MaxDays == 1`: at `0` PocketBase's
  internal `BeforeAddFunc` short-circuits on `MaxDays > 0` and this hook never fires (research
  D-29, reconciliation C4).
- [x] T027 Implement `internal/logging/logger.go`.
- [x] T028 Implement `internal/logging/pbbridge.go` (mechanism 1).
- [x] T029 Implement `internal/logging/pblogs.go` (mechanism 2).
- [x] T030 Implement `internal/logging/redact.go` — the shared redaction helpers the domain
  marshallers use.
- [x] T031 TEST `internal/logging/singlestream_test.go` — a source-walking test asserting **zero**
  uses of `slog`, `log.Print*` or `fmt.Print*` outside `internal/logging`, and **zero** downcasts
  of `core.App` to `*pocketbase.PocketBase`. This is the test that keeps CT-1's workaround
  contained (Principle VI).
- [x] T032 [P] TEST `internal/logging/coverage_test.go` — the bridge captures PocketBase's cron,
  mailer and migration logs too, not only request logs. Mechanism 1 alone misses the transactional
  path because `createTxApp` does `clone := *app` on a `*BaseApp`; mechanism 2 alone misses
  everything that never reaches `_logs`. Both, or there is a hole (CT-1).
- [x] T033 [P] TEST `internal/config/documented_test.go` — every field of the config struct appears
  in `README.md`'s documented environment and in `quickstart.md`, asserted by reflection. A new
  setting nobody documented fails the build (FR-051).

### Domain primitives — stdlib and zerolog only

- [x] T034 [P] TEST `internal/domain/errors_test.go` — every sentinel is distinct and
  `errors.Is`-matchable through wrapping.
- [x] T035 [P] TEST `internal/domain/validation_test.go` — a `*ValidationError` carries **every**
  offending field at once, each with a machine code and a message (FR-027).
- [x] T036 [P] TEST `internal/domain/validation_envelope_test.go` — the JSON shape of a
  `*ValidationError` matches contracts/README.md's envelope exactly, field for field, so the
  client contract cannot drift from the domain type.
- [x] T037 [P] TEST `internal/domain/date_test.go` — `clinical.Date` shows the identical calendar day
  under `UTC`, `Pacific/Auckland` and `America/Los_Angeles`, and round-trips through JSON and
  through the store mapping unchanged (FR-019, research D-27).
- [x] T038 [P] TEST `internal/domain/page_test.go` — `Page[T]` and `SortKey`.
- [x] T039 [P] Implement `internal/domain/errors.go` — `ErrNotFound`, `ErrForbidden`,
  `ErrUnauthenticated`, `ErrVersionMismatch`, `ErrConflict`, `ErrRateLimited`.
- [x] T040 [P] Implement `internal/domain/validation.go`.
- [x] T041 [P] Implement `internal/domain/date.go`.
- [x] T042 [P] Implement `internal/domain/page.go`.
- [x] T043 TEST `internal/architecture/domain_imports_test.go` — no package under
  `internal/domain` imports `net/http`, a database driver, or anything beyond the standard library
  and zerolog. depguard covers PocketBase; this covers the rest of the seam (Principle II).
- [x] T044 [P] TEST `internal/domain/kind/kind_test.go` — the mapping from `Kind` to enum spelling,
  **plural** path segment and collection name is total and injective in both directions, and
  round-trips. This test is what stops the singular/plural drift that cross-artifact finding H1
  records (research D-05).
- [x] T045 Implement `internal/domain/kind/kind.go` — `Kind`, `Segment()`, `Collection()`,
  `Enum()`, `Kinds()`. **This constant is the single source of the path spelling for every phase
  after this one.**
- [x] T046 TEST `internal/architecture/kind_literals_test.go` — a source walk asserting **no string
  literal** spelling a kind's path segment or collection name exists outside
  `internal/domain/kind/kind.go`. This is the mechanical form of research D-05 and the reason
  cross-artifact finding H1 cannot recur.
- [x] T047 [P] TEST `internal/domain/access/actor_test.go` — `Actor` construction and
  `Permission`/`Grant` semantics.
- [x] T048 [P] Implement `internal/domain/access/actor.go` and `permission.go`.
- [x] T049 [P] TEST `internal/domain/audit/enums_test.go` — `ActorKind`, `Action` (all ten values
  including `access_denied`), `TargetKind`, each with `Valid()` (data-model §3, research D-20).
- [x] T050 [P] Implement `internal/domain/audit/event.go` and `enums.go`. **`Event` has no content
  field and no free-text field** — the shape itself is what makes FR-038 structural rather than
  a review item.

### The platform: PocketBase, locked down

- [x] T051 [P] TEST `internal/platform/pb/app_test.go` — `pocketbase.NewWithConfig` with
  `HideStartBanner`, an explicit `DefaultDataDir` and the `DBConnect` hook.
- [x] T052 Implement `internal/platform/pb/app.go`.
- [x] T053 [P] TEST `internal/platform/pb/settings_test.go` — after boot: `Batch.Enabled == false`,
  `Logs.MaxDays == 1`, rate limits and token TTLs match the MediKube config. Settings are written
  from validated env at boot and never hand-edited in the admin UI (ANALYSIS M4).
- [x] T054 Implement `internal/platform/pb/settings.go`.
- [x] T055 [P] TEST `internal/platform/pb/lockdown_test.go` — **the most important test in the
  phase.** For anonymous and for an ordinary signed-in user: every method on
  `/api/collections/{c}/records[/{id}]` returns **404** for every collection, and `POST /api/batch`
  returns 404. For a superuser the subtree still works.
- [x] T056 [P] TEST `internal/platform/pb/lockdown_priority_test.go` — the middleware is bound at
  **-1019**, after `loadAuthToken` at -1020, asserted against the bound middleware list. Bound
  earlier it cannot see the authenticated actor; bound later the built-in handler has already run.
- [x] T057 TEST `internal/platform/pb/hooks_never_fire_test.go` — binding an `OnRecordCreateRequest`
  hook and issuing every request that could plausibly trigger it produces **zero** invocations
  under the lockdown, because those hooks live inside the CRUD handlers the lockdown disables.
  This test is the evidence behind the forbidigo rule, and without it the ban reads as dogma
  (reconciliation C13).
- [x] T058 TEST `internal/platform/pb/lockdown_auth_test.go` — **all fourteen** PocketBase auth
  routes under `/api/collections/` remain reachable and behave normally: `auth-methods`,
  `auth-with-password`, `auth-with-oauth2`, `auth-refresh`, `request-verification`,
  `confirm-verification`, `request-password-reset`, `confirm-password-reset`,
  `request-email-change`, `confirm-email-change`, `impersonate`, and the superusers equivalents
  (VERIFIED FACT 2). A prefix match one segment too greedy breaks every phase after this one.
- [x] T059 Implement `internal/platform/pb/lockdown.go` — the middleware at priority **-1019**,
  immediately after `loadAuthToken` at -1020, scoped to the `/records` subtree.
- [x] T060 [P] TEST `internal/platform/pb/assert_test.go` — the boot assertions **refuse to start**
  when any non-system collection has a non-nil API rule, when `Batch` is enabled, and when any
  `FileField` has `Protected: false`. Each is asserted as a distinct refusal with its own message.
- [x] T061 Implement `internal/platform/pb/assert.go`.
- [x] T062 [P] TEST `internal/platform/pb/protected_files_test.go` — a synthetic collection with a
  `FileField` whose `Protected` is false makes the boot assertion refuse. No file field ships in
  this phase; the assertion does, because phase 002 adds one and a file served to a stranger is
  exactly the failure Principle VII exists to prevent.
- [x] T063 [P] TEST `internal/platform/pb/no_file_routes_test.go` — `GET /api/files/...` is
  unreachable for every collection and no MediKube route issues a PocketBase file token. Files are
  served only from MediKube's own `/api/v1` routes, with authorization applied.
- [x] T064 [P] TEST `internal/platform/pb/adminwarn_test.go` — the warning fires on each of the four
  conditions independently: empty `Settings().SuperuserIPs`, superusers-collection `MFA.Enabled`
  false, a **non-empty `MFA.Rule`** (a partial rollout that reads as "on"), and fewer than two
  enabled auth methods (VERIFIED FACT 10, research D-32). This is FR-040's start-up warning.
- [x] T065 Implement `internal/platform/pb/adminwarn.go` (FR-040).
- [x] T066 [P] TEST `internal/platform/pb/serve_test.go` — middleware bind order is exactly as
  designed, and `se.Server.WriteTimeout` has been overridden away from PocketBase's hardcoded
  five minutes before the listener starts (research D-34, reconciliation C9).
- [x] T067 Implement `internal/platform/pb/serve.go` — the `OnServe` binding: WriteTimeout override,
  middleware order, and `httproute.Registry.Bind(se)`.

### Migrations and the schema

- [x] T068 [P] TEST `internal/store/migrations/reversible_test.go` — for **every** registered
  migration, up → down → up leaves an identical schema. Reversibility is by construction
  (`Register(up, down, filename)`) but only a test proves it (VERIFIED FACT 8, FR-059).
- [x] T069 Implement `internal/store/migrations/1756100100_users_profile.go` — amend `users` with
  the seven MediKube fields, set all five API rules to nil, leave `AuthRule` as
  `types.Pointer("")`, add `idx_users_email_lower`, set token config (data-model §1).
- [x] T070 Implement `internal/store/migrations/1756100200_medications.go` — create `medications`
  with thirteen fields, three enums, `owner → users` **`Required: true, CascadeDelete: true`**,
  five indexes each ending in `id` for cursor stability (data-model §2).
- [x] T070a [P] TEST `internal/store/migrations/audit_vocab_test.go` — the **complete** declared
  vocabulary, not a delta: `audit_events.action` has exactly the twenty values of data-model §3
  and `target_kind` exactly the twenty-three, asserted set-equal (extra values fail too, so a
  later phase cannot quietly widen the trail). This is the test every later phase's vocabulary
  migration extends and re-asserts; a value a phase writes but no migration declared is a red test
  here rather than a failed `SelectField` validation in production (ANALYSIS C1). **Also asserts
  the two field sizes the later phases depend on**: `target_id` is `Max 64` (phase 006 writes
  20–31-character job names and ~40-character archive names into it) and `request_id` is `Max 64`
  **and `Required`** — a row that correlates to nothing must not be writable (ANALYSIS).
- [x] T071 Implement `internal/store/migrations/1756100300_audit_events.go` — create `audit_events`
  with seven fields, **no `ip` column, no `reason` column, no `affected` column, no content
  column**, three enums declaring the **complete** vocabulary of data-model §3 (four actor kinds,
  **twenty** actions, **twenty-three** target kinds), `target_id` at **`Max 64`** and `request_id`
  at **`Max 64`, `Required`**, and the **three indexes carrying phase 006's tiebreaker columns**
  so 006 creates no index of its own and none collides by name (data-model §3, research D-19,
  ANALYSIS).
- [x] T072 Implement `internal/store/migrations/register.go` and `assertions.go`.
- [x] T073 [P] TEST `internal/store/migrations/applied_set_test.go` — the applied migration set
  equals the registered set, which is exactly what `readyz`'s `migrations` check reports.
- [x] T074 [P] TEST `internal/store/migrations/settings_persist_test.go` — the settings written at
  boot survive a restart and are re-asserted rather than assumed (ANALYSIS M4).
- [x] T075 TEST `internal/store/migrations/assertions_test.go` — the cascade matrix is exactly as
  data-model §4 states. **`medications.owner` must be `Required: true` AND `CascadeDelete: true`**;
  a one-character flip leaves orphaned medications after an account delete and silently breaks
  FR-014 and SC-012 with no other symptom.
- [x] T076 [P] TEST `internal/store/migrations/empty_start_test.go` — the instance starts against a
  completely empty data directory and creates everything it needs (FR-063).

### The store layer

- [x] T077 [P] TEST `internal/store/cursor_test.go` — a cursor round-trips; a **tampered** cursor is
  rejected rather than trusted; the encoding is keyset, never an offset; and a cursor issued
  before a restart still validates afterwards (research D-25, CT-3).
- [x] T078 Implement `internal/store/cursor.go` — opaque HMAC-signed keyset cursors, key derived by
  HKDF from PocketBase's persisted auth-token secret with `MEDIKUBE_CURSOR_KEY` as an override
  (**CT-3** — confirm the exact secret field name against v0.40.1 at implementation time).
- [x] T079 [P] TEST `internal/store/mapping_test.go` — `*core.Record` ↔ domain in both directions,
  including the calendar-date handling and `encoding/json/v2`'s stricter semantics (research D-28).
- [x] T080 Implement `internal/store/mapping.go` and `internal/store/tx.go`.
- [x] T081 [P] TEST `internal/store/tx_test.go` — a failing operation inside `RunInTransaction`
  rolls back completely and publishes no realtime event.
- [x] T082 [P] TEST `internal/store/cursor_sort_test.go` — a cursor issued under one sort order is
  rejected under another rather than silently paging through a different sequence (FR-023).
- [x] T083 [P] TEST `internal/store/filter_test.go` — the typed query builder produces the expected
  PocketBase filter string and **no filter DSL string appears outside this package**, asserted by
  a source walk.
- [x] T084 Implement `internal/store/filter.go`.

### Test support — the harness every later phase inherits

- [x] T085 Create `internal/testdata/pb_data/` — the committed fixture directory every
  `tests.NewTestApp` clones (VERIFIED FACT 7; ~11 ms per clone).
- [x] T086 Implement `internal/testsupport/app.go` — `TestAppFactory` returning a **new** app per
  call.
- [x] T087 TEST `internal/testsupport/app_test.go` — two factory calls produce independent apps, and
  a guard test documents why an `ApiScenario` must never share a `tests.TestApp`: the shared case
  recurses infinitely and blows the stack (reconciliation C14).
- [x] T088 [P] Implement `internal/testsupport/fixtures.go` — the seeded ids as exported constants,
  so no later phase hardcodes an id string.
- [x] T089 [P] Implement `internal/testsupport/phileak/capture.go` — captures **all four**
  diagnostic sinks for the PHI-leak suite: the zerolog stream, the Prometheus gatherer output
  (names and label values), an OTel `tracetest.SpanRecorder` and a stub Sentry transport. This is
  the package phases 002–006 extend; there is deliberately **no** second `logcapture.go` and no
  `internal/obs/phi_leak_test.go` (cross-artifact finding M6).
- [x] T090 Implement `internal/testsupport/authz.go` — `RunOwnershipMatrix(t, cases)`, the
  owner-succeeds / stranger-refused table **phases 002–005 extend rather than reinvent**.
- [x] T091 [P] TEST `internal/testsupport/authz_test.go` — a self-test: a deliberately permissive
  fake handler makes `RunOwnershipMatrix` **fail**. A matrix that cannot fail proves nothing, and
  five later phases depend on this one helper.
- [x] T092 [P] TEST `internal/testsupport/fixtures_test.go` — the exported fixture constants match
  what `medikube seed` actually writes, so a drifting seed breaks the harness loudly.

### The route registry, OpenAPI and the record family

- [x] T093 [P] TEST `internal/httproute/registry_test.go` — `Handle(spec, handler)` registers and
  describes in one call; `Routes()` and `SmokeTargets()` agree; and registering a **page** with no
  `Landmark` or no `SmokeURL` **panics at registration time**, so the failure is a boot failure and
  not a silent hole in the render gate (FR-067).
- [x] T094 Implement `internal/httproute/registry.go`.
- [x] T095 [P] TEST `internal/httproute/duplicate_test.go` — registering the same method and path
  twice panics at registration rather than letting one handler silently win.
- [x] T096 [P] TEST `internal/httproute/smoketargets_test.go` — `SmokeTargets()` returns exactly
  the registered pages and error views and nothing else, so the Playwright list cannot quietly
  shrink.
- [x] T097 Implement `internal/httproute/routes.go` — THE declarative table: 22 API operations, 1
  stream, 9 pages. **PocketBase's router is a private field with no introspection API**, so this
  table is the only inventory that exists and the OpenAPI document, the `routes` command and the
  Playwright list all read it (reconciliation C15).
- [x] T098 [P] TEST `internal/openapi/gate_test.go` — every registered route appears in the
  generated document and every documented operation is registered. Both directions; either
  asymmetry fails (FR-065, SC-011).
- [x] T099 [P] TEST `internal/openapi/oneof_test.go` — with **two** kinds registered (the real one
  plus the synthetic fixture), the generated document validates after a **marshal-then-load**
  round trip through `openapi3.NewLoader()`, and every kind appears in `Discriminator.Mapping`.
  `Mapping` is `map[string]openapi3.MappingRef` — a struct, not a string — and an in-memory
  `Validate()` alone accepts documents that do not round-trip (VERIFIED FACT 9, risk R1).
- [x] T100 Implement `internal/openapi/schema.go` — DTO reflection and the discriminated `oneOf`.
- [x] T101 Implement `internal/openapi/generate.go`.
- [x] T102 [P] TEST `internal/openapi/document_test.go` — the document declares OpenAPI **3.1**,
  every `operationId` is unique and matches the inventory in contracts/README.md, and every
  operation carries its authorization rule in the description.
- [x] T103 [P] TEST `internal/openapi/envelope_test.go` — every documented error response
  references the one shared error-envelope schema rather than an inline copy, so the published
  contract cannot disagree with `internal/web/errors.go`.
- [x] T104 [P] TEST `internal/records/registry_test.go` — registering a kind wires all seven
  consumers (routes, OpenAPI schema, page views, stream filter, audit target, authorizer, CLI
  inventory) and a kind missing any one of them fails registration.
- [x] T105 [P] TEST `internal/records/duplicate_kind_test.go` — registering the same kind twice
  fails loudly at registration rather than leaving whichever service won.
- [x] T106 Implement `internal/records/registry.go` and `service.go` — the five-method
  kind-agnostic `Service` interface.
- [x] T107 Implement `internal/records/handler.go` — the ONE generic record handler and its dispatch
  table. Every later phase adds a kind, **never a route**.
- [x] T108 [P] TEST `internal/records/handler_test.go` — an unregistered `{kind}` segment yields
  **404** and not 400, a differently-cased segment does not match, and the dispatch table covers
  every registered kind.
- [x] T109 Implement `internal/records/recordstest/fake.go` — `FakeKindService`, a second registered
  synthetic kind. It is what lets the `oneOf` gate be meaningful with one real kind, and it is
  Principle I's second implementation for **CT-2**.

### The HTTP edge

- [x] T110 [P] TEST `internal/web/errors_test.go` — the error taxonomy: every domain sentinel maps
  to exactly one status and one machine code, and **`ErrForbidden` on owner-scoped data maps to
  404**, not 403 (contracts/README.md, FR-033).
- [x] T111 Implement `internal/web/errors.go` — the ONE error→status mapper and the response
  envelope.
- [x] T112 [P] TEST `internal/web/dto_test.go` — decoding rejects unknown fields; absent versus
  explicit `null` are distinguishable (`**string`) so a PATCH can clear a field without clearing
  every omitted one.
- [x] T113 Implement `internal/web/dto.go`.
- [x] T114 [P] TEST `internal/web/json_semantics_test.go` — `encoding/json/v2`'s stricter
  behaviour is what the DTO layer relies on: duplicate keys are rejected and a case-insensitive
  field match no longer silently succeeds (research D-28).
- [x] T115 [P] TEST `internal/web/etag_test.go` — the ETag derives from `updated`; PATCH and DELETE
  **require** `If-Match`; a stale `If-Match` yields **412 carrying the current representation**;
  a missing one yields 428 (FR-026, risk R12).
- [x] T116 Implement `internal/web/etag.go`.
- [x] T117 [P] TEST `internal/web/cursor_test.go` — cursor encode/decode at the HTTP edge and the
  refusal of a malformed one.
- [x] T118 Implement `internal/web/cursor.go`.
- [x] T119 [P] TEST `internal/web/actor_test.go` — `e.Auth` becomes an `access.Actor` in the request
  context, and a handler that never sees an actor cannot be reached.
- [x] T120 Implement `internal/web/actor.go`.
- [x] T121 [P] TEST `internal/web/security_test.go` — the CSP is exactly as designed:
  `default-src 'none'`, `script-src 'self' 'unsafe-eval'` (the **only** relaxed directive, accepted
  permanently for Datastar's expression evaluator), `object-src 'none'`,
  `frame-ancestors 'none'`, `base-uri 'self'`, `form-action 'self'`, plus HSTS, nosniff and
  referrer policy (FR-042, reconciliation C10).
- [x] T122 Implement `internal/web/security.go`.
- [x] T123 [P] TEST `internal/web/security_errors_test.go` — the security headers and the CSP are
  present on **error** responses and on the SSE stream too, not only on 200s.
- [x] T124 Implement `internal/web/render.go` — templ → `RequestEvent`, the **non-SSE** element-patch
  path Datastar honours for a plain `text/html` response.
- [x] T125 [P] TEST `internal/obs/middleware_test.go` — request id generated or honoured, the
  request logger emits one line per request with that id, the panic recovery returns the 500 view
  and logs once, and **a single occurrence is never reported twice** across log, Sentry and trace
  (FR-057).
- [x] T126 Implement `internal/obs/middleware.go`.
- [x] T127 [P] Implement `internal/realtime/hub.go` — a channel and a map. `Event{Kind, RecordID,
  OwnerID}`. **IDs, never bodies.** No broker interface, no abstraction: the instance is single by
  construction and pretending otherwise is the complexity Principle I forbids.
- [x] T128 [P] TEST `internal/realtime/hub_test.go` — publish/subscribe/unsubscribe, a slow
  subscriber does not block the publisher, and the hub carries no record body.
- [x] T129 [P] TEST `internal/realtime/hub_shutdown_test.go` — cancelling a subscriber's context
  unsubscribes it and leaks no goroutine, asserted with a goroutine count.
- [x] T130 [P] TEST `internal/di/container_test.go` — every provider resolves and the container
  builds with no cycle, so a wiring mistake fails a test rather than the first request.
- [x] T131 Implement `internal/di/container.go` and `providers.go` — samber/do v2, wired **only** in
  the composition root.
- [x] T132 Implement `cmd/medikube/main.go` — the composition root and the **only** place in the
  program permitted to panic.
- [x] T133 TEST `cmd/medikube/main_test.go` — the composition root builds end to end against a test
  app and reaches a serving state against an empty data directory (FR-063).

**Checkpoint**: `task test` passes, the instance boots against an empty data directory, the
lockdown holds, and the log stream is pure zerolog. User stories can now start in parallel.

---

## Phase 3: User Story 1 — Keep an accurate medication list (Priority: P1) 🎯 MVP

**Goal**: one clinical record kind proving every layer — domain, store, service, the generic
record route family, the DTOs, the templ views, the live stream — end to end, so that phases
002–006 add kinds and never architecture.

**Independent Test**: sign in as the seeded account, record a medication, see it in the list, edit
it, watch a second open view update within 5 seconds, delete it with an explicit confirmation, and
confirm the row is gone from stored data. All nine of US1's acceptance scenarios pass.

### Tests for User Story 1 ⚠️ write first, watch them fail

- [x] T134 [P] [US1] TEST `internal/domain/clinical/enums_test.go` — `MedicationType`,
  `MedicationRoute` and `TherapyStatus` accept **exactly** the published values and reject
  everything else, including near misses and different casing (FR-016, data-model §2).
- [x] T135 [P] [US1] TEST `internal/domain/clinical/validate_test.go` — every rule in data-model §2's
  validation table, plus: an end date before the start date is refused (FR-018), a future start
  date beyond the allowed window is refused, every free-text maximum is enforced (FR-017), and
  **all** violations come back in one `*ValidationError` (FR-027).
- [x] T136 [P] [US1] TEST `internal/domain/clinical/medication_test.go` — `MarshalZerologObject`
  emits the id and **never** the name, dose, reason or notes (FR-038, SC-005).
- [x] T137 [P] [US1] TEST `internal/service/medication/service_test.go` against the fake repository —
  **authorization is called before every read and every write**, and a service method that skips
  it fails the test.
- [x] T138 [P] [US1] TEST `internal/service/medication/medicationtest/contract.go` — the
  `testify/suite` contract every `Repository` implementation must pass (Principle II): create,
  get, list with cursor, update with version check, delete, and owner scoping.
- [x] T139 [P] [US1] TEST `internal/service/medication/fake_contract_test.go` — the in-memory fake
  passes the contract suite.
- [x] T140 [P] [US1] TEST `internal/store/medication/repo_integration_test.go` — the PocketBase
  repository passes the **same** contract suite against a real `tests.NewTestApp`, plus: keyset
  paging over 1,000 rows shows no entry twice and skips none **while a row is inserted mid-page**
  (FR-023), and every sort order is stable because every index ends in `id`.
- [x] T141 [P] [US1] TEST `internal/web/api/records_test.go` — `ApiScenario` cases for all six
  operations: status codes, response shapes, the error envelope, unknown-field rejection, and the
  query-parameter table in contracts/records.md — `listRecords`, `listRecordsOfKind`, `createRecord`,
  `getRecord`, `updateRecord`, `deleteRecord` (FR-025). **A fresh `tests.TestApp` per case** — a shared
  one recurses infinitely (reconciliation C14).
- [x] T142 [P] [US1] TEST `internal/web/api/records_list_test.go` — all three sort orders return
  the documented sequence, the status filter narrows correctly, and paging forwards then backwards
  returns the same set (FR-021, FR-022).
- [x] T143 [P] [US1] TEST `internal/web/api/records_create_test.go` — a create returns **201** with
  the `Location` header and an `ETag`, and the created representation matches the stored one, name
  required and every other field optional (FR-015).
- [x] T144 [P] [US1] TEST `internal/web/api/records_validation_test.go` — a request violating four
  rules at once comes back with **all four** field errors in one envelope, each with its field
  path and machine code (FR-027).
- [x] T145 [P] [US1] TEST `internal/web/api/records_delete_test.go` — a delete removes the row
  from stored data outright: no `deleted_at`, no tombstone, no filtered-out survivor. Soft delete
  in MediKube is **files only**, and a record collection carrying a deletion column would be a
  schema-level contradiction of that (data-model §2).
- [x] T146 [P] [US1] TEST `internal/web/api/records_etag_test.go` — PATCH and DELETE without
  `If-Match` are refused; with a stale one the response is **412 carrying the current
  representation** so the client can show the person what changed (FR-026).
- [x] T147 [P] [US1] TEST `internal/web/api/records_authz_test.go` — `RunOwnershipMatrix` over all
  six operations: the owner succeeds, a stranger is refused, and **the refusal body is
  byte-identical to a genuine not-found apart from `request_id`** (FR-032, FR-033, FR-069).
- [x] T148 [P] [US1] TEST `internal/web/views/records/medication_row_test.go` — render to buffer:
  the row carries the deterministic id from `ids.RecordRow`, shows name, dose and status, and
  omits absent optional fields rather than rendering empty labels (FR-024).
- [x] T149 [P] [US1] TEST `internal/web/views/records/medication_list_test.go` — render to buffer:
  the list is inside `region[name="Medications"]`, and **an account with no medications still
  renders that region**, containing the empty state and a create action (FR-029).
- [x] T150 [P] [US1] TEST `internal/web/views/records/medication_form_test.go` — render to buffer:
  every field error is adjacent to its field and linked by `aria-describedby`, and the enum fields
  offer exactly the published values.
- [x] T151 [P] [US1] TEST `internal/web/views/records/medication_detail_test.go` — render to
  buffer: every recorded value present, absent optional values omitted entirely rather than shown
  as blank labels, the last-changed time shown (FR-020), and the landmark `article[name="Medication"]` (FR-024).
- [x] T152 [P] [US1] TEST `internal/web/api/records_audit_test.go` — a create, an update and a
  delete each write exactly one audit row with the right action, target kind and target id, and
  **no** clinical content (FR-036).
- [x] T153 [P] [US1] TEST `internal/web/views/components/confirm_test.go` — the delete confirmation
  is a **rendered element** carrying `region[name="Confirm delete"]` and the medication's name.
  Not a `window.confirm`, which the render gate cannot see (FR-028).
- [x] T154 [P] [US1] TEST `internal/web/stream/records_test.go` — for **every** event the handler
  re-runs `access.Authorizer.Record(...)` for that subscriber; two sessions on two accounts stream
  simultaneously and a write on one produces **zero** frames on the other (FR-030, FR-032).
- [x] T155 [P] [US1] TEST `internal/web/stream/kindfilter_test.go` — the `kind` query parameter
  filters the stream, an unregistered kind is refused, and an absent parameter subscribes to every
  registered kind (contracts/streams.md).
- [x] T156 [US1] TEST `internal/web/stream/endtoend_test.go` — a write through the API reaches a
  second open subscriber's stream **within five seconds**, measured, which is SC-007's actual
  claim and the only test that exercises hook → hub → re-authorise → render → patch as one path.
- [x] T157 [P] [US1] TEST `internal/web/stream/stream_test.go` — `newStream()` sets the write
  deadline to zero via `http.NewResponseController`, sets `X-Accel-Buffering: no` and
  `Cache-Control: no-store`, binds `apis.SkipSuccessActivityLog()`, and emits **only** the two
  Datastar v1 event names.
- [x] T158 [US1] TEST `internal/web/stream/timeout_test.go` — a stream held open for **longer than
  five minutes** still receives heartbeats. Tagged so it runs in the dedicated CI job and not in
  `task test`. **Every test shorter than five minutes passes with the bug present**, which is why
  this one exists (SC-007, risk R7).
- [x] T159 [P] [US1] TEST `internal/web/stream/heartbeat_test.go` — a `datastar-patch-signals` frame
  carrying `stream_beat` arrives every 25 s, and the staleness threshold the page compares against
  is 60 s (FR-031).
- [x] T160 [P] [US1] TEST `internal/platform/pb/hooks_records_test.go` — the realtime publisher is
  bound to the **After…Success** hooks, so a rolled-back transaction publishes nothing. A
  pre-commit binding is a live view showing a change that did not happen.

### Implementation for User Story 1

- [x] T161 [P] [US1] Implement `internal/domain/clinical/enums.go` — the three enumerations with
  `Valid()`, exact values per data-model §2.
- [x] T162 [P] [US1] Implement `internal/domain/clinical/medication.go` — the entity and its
  redacting `MarshalZerologObject`.
- [x] T163 [US1] Implement `internal/domain/clinical/validate.go` — every field checked, every
  violation collected, one error returned.
- [x] T164 [US1] Implement `internal/service/medication/ports.go` — `Repository`, `Authorizer`,
  `Auditor`. Interfaces at the seam, defined by the consumer (Principle II).
- [x] T165 [US1] Implement `internal/service/medication/service.go` — List, Get, Create, Update,
  Delete. **Authorization first, always**, then validation, then the store, then the audit write.
- [x] T166 [P] [US1] Implement `internal/service/medication/medicationtest/fake.go`.
- [x] T167 [US1] Implement `internal/store/medication/repo.go` — the PocketBase-backed repository,
  keyset paging, the three sort orders (FR-022), and the version check.
- [x] T168 [US1] Implement `internal/service/medication/adapter.go` — roughly forty lines
  implementing `records.Service` for this kind, and register it. **This file is the template every
  later kind copies.**
- [x] T169 [P] [US1] Implement `internal/web/api/dto_medication.go` — `MedicationSummary`,
  `Medication`, `MedicationCreate`, `MedicationPatch` — the full recorded field set of FR-015 plus the
  created and last-changed times of FR-020 — with `**string` where absent must be
  distinguishable from explicit null.
- [x] T170 [US1] Implement `internal/web/api/records.go` — the six operations `listRecords`,
  `listRecordsOfKind`, `createRecord`, `getRecord`, `updateRecord`, `deleteRecord` bound through the ONE
  generic handler, registered in `internal/httproute/routes.go` under those `operationId`s so the
  Principle IX gate finds the same six names in the registry and in `api/openapi.json` (FR-015, FR-025).
- [x] T171 [P] [US1] Implement `internal/web/views/ids/ids.go` — deterministic DOM ids used by both
  templ and Go, so a patch selector can never drift from the element it targets.
- [x] T172 [P] [US1] Implement `internal/web/views/records/medication_row.templ`.
- [x] T173 [P] [US1] Implement `internal/web/views/records/medication_list.templ` — including the
  empty state **inside** the region.
- [x] T174 [P] [US1] Implement `internal/web/views/records/medication_detail.templ` — every recorded
  value plus the last-changed time (FR-020), omitting what was not recorded (FR-024).
- [x] T175 [P] [US1] Implement `internal/web/views/records/medication_form.templ` — create and edit,
  every recorded field editable (FR-015, FR-025), field-adjacent errors.
- [x] T176 [P] [US1] Implement `internal/web/views/components/{empty_state,field_error,confirm,pagination}.templ`.
- [x] T177 [US1] Implement `internal/web/page/medications.go` — the list page and the detail page,
  registered with their `Landmark` and `SmokeURL` (contracts/pages.md P4, P5).
- [x] T178 [US1] Implement `internal/web/stream/stream.go` — **the mandatory `newStream()` helper**:
  the write-deadline clear, the two headers, the activity-log skip. Add a lint rule forbidding
  `datastar.NewSSE` anywhere else. There is no second path.
- [x] T179 [US1] Implement `internal/web/stream/records.go` — `streamRecords`, the per-subscriber handler:
  filter by kind, **re-authorise per event**, re-fetch, render, patch by id, plus the 25-second
  heartbeat and the `e.Request.Context().Done()` shutdown path.
- [x] T180 [US1] Implement the realtime publisher in `internal/platform/pb/hooks.go` — bound to
  `OnRecordAfterCreateSuccess`, `OnRecordAfterUpdateSuccess`, `OnRecordAfterDeleteSuccess`.
  Publishes `{Kind, RecordID, OwnerID}` only.
- [x] T181 [P] [US1] Add the staleness detector to `internal/web/views/layout.templ` —
  `data-on-interval__duration.10s` comparing `$stream_beat`, revealing a `role="alert"` banner.
  **Free Datastar attributes only**: `data-persist`, `data-match-media` and `data-on-raf` are Pro.
- [x] T182 [US1] Add the medication cases to `internal/cli/seed.go` — account A spanning all three
  statuses including one row with every optional field empty, account B with exactly one, account
  C with none (data-model §6, research D-39).
- [ ] T183 [P] [US1] TEST `e2e/smoke.spec.ts` cases for `/medications` and `/medications/{id}` at
  both viewports, and a keyboard-only pass covering record → edit → delete (SC-014).
- [x] T184 [P] [US1] TEST `internal/web/api/records_bench_test.go` — a 1,000-medication list renders
  within the SC-002 budget, with the 5,000-row case from the spec's Edge Cases exercised for
  correctness rather than latency.
- [x] T185 [P] [US1] TEST `internal/cli/seed_medications_test.go` — the seeded medication set is
  exactly the mixed set data-model §6 describes: all three statuses, one row with every optional
  field empty, one account with a single row and one with none.

**Checkpoint**: US1 is independently demonstrable. All nine of its acceptance scenarios pass.

---

## Phase 4: User Story 2 — Own and control my account (Priority: P2)

**Goal**: registration, sign-in, sessions, profile, password change and account deletion, built on
PocketBase's native auth but exposed only through MediKube's own DTOs.

**Independent Test**: register, sign in, change the display name and theme, change the password
and watch every other session stop working, then delete the account and confirm every medication
under it is gone from stored data.

### Tests for User Story 2 ⚠️ write first, watch them fail

- [x] T186 [P] [US2] TEST `internal/domain/identity/password_test.go` — the published rules:
  minimum eight characters, the refusal message states the rule, and the rules the API publishes
  are **the same values** the validator enforces, asserted from one source (FR-004).
- [x] T187 [P] [US2] TEST `internal/domain/identity/enums_test.go` — `Role`, `UnitSystem`,
  `DateFormat`, `Theme` with `Valid()` (data-model §1).
- [x] T188 [P] [US2] TEST `internal/domain/identity/validate_test.go` — display-name limits, email
  shape, and that `role` and `status` are **not settable** from any user-supplied structure
  (FR-012).
- [x] T189 [P] [US2] TEST `internal/service/identity/identitytest/contract.go` — the contract suite
  for `Repository` and `Authenticator`.
- [x] T190 [P] [US2] TEST `internal/service/identity/service_test.go` against the fakes —
  Register, ChangePassword, UpdateProfile, DeleteAccount, each with its audit write asserted.
- [x] T191 [P] [US2] TEST `internal/store/identity/repo_integration_test.go` — the PocketBase
  implementation passes the same contract suite, plus `idx_users_email_lower` makes
  `Amara@…` and `amara@…` the same account (FR-003).
- [x] T192 [P] [US2] TEST `internal/web/api/auth_test.go` — `ApiScenario` cases for
  `getAuthConfig`, `register`, `login`, `refreshSession`, `logout` per contracts/auth.md —
  an account created from email, display name and password on an open instance (FR-001), and a sign-in
  answering an unknown address and a wrong password identically (FR-005) —
  including: registration refused with **403** `registration_closed` when closed (FR-002, defect D15), a
  duplicate email answered **409** `conflict` with a message that does not confirm registration
  (FR-003, defect D16), and the session
  response carrying **no token in the body** — the token is an `HttpOnly` cookie.
- [x] T193 [P] [US2] TEST `internal/web/api/me_test.go` — `getMe`, `updateMe`, `changePassword`,
  `deleteMe` per contracts/account.md, including display name and the four preferences read back and
  changed (FR-011), the re-entered current password on deletion, and the exact confirmation phrase
  `DELETE MY ACCOUNT` and the refusal of anything else (FR-013).
- [x] T194 [P] [US2] TEST `internal/web/api/me_privilege_test.go` — `updateMe` cannot set `role`
  or `status` by any spelling, including an unknown field, a null and a nested object (FR-012).
- [x] T195 [P] [US2] TEST `internal/web/api/me_counts_test.go` — `MeCounts` reports the account's
  own medication count and nobody else's.
- [x] T196 [P] [US2] TEST `internal/web/api/session_expiry_test.go` — a session older than the
  configured TTL is refused and the person is asked to sign in again (FR-008), and `logout`
  invalidates the session immediately (FR-007).
- [x] T197 [P] [US2] TEST `internal/web/api/change_password_test.go` — a password change without
  the current password is refused, and the refusal does not confirm whether the supplied current
  password was the wrong one or the new one invalid (FR-009).
- [x] T198 [US2] TEST `internal/web/api/me_delete_integration_test.go` — after `deleteMe`,
  `SELECT COUNT(*) FROM medications WHERE owner = '<id>'` is **0**,
  `SELECT COUNT(*) FROM audit_events WHERE target_id = '<id>' AND action = 'account_delete'` is
  **greater than 0**, and that surviving row's `actor` is **`''`**. The medications cascade; the
  audit rows deliberately do not, and because `actor` is `CascadeDelete: false` *and*
  `Required: false`, PocketBase **unsets** it rather than leaving a dangling id — so a query on
  `actor` finds nothing and `actor_kind` is the only surviving evidence a person did it. Keying on
  the row count alone would pass on a row about somebody else (FR-014, SC-012, research D-22, defect D17).
- [x] T199 [P] [US2] TEST `internal/web/session_test.go` — the cookie is `HttpOnly`, `SameSite=Lax`,
  `Secure` outside dev, and the cookie→`Authorization` middleware is bound at priority **-1021**,
  before `loadAuthToken` at -1020. Bound after it, every authenticated request is anonymous.
- [x] T200 [US2] TEST `internal/service/identity/revocation_test.go` — after a password change,
  **every** session issued before it stops working, driven by rotating `RefreshTokenKey`
  (FR-010, research D-16).
- [x] T201 [P] [US2] TEST `internal/web/api/auth_ratelimit_test.go` — repeated failed sign-ins are
  slowed or blocked, and the response to a rate-limited attempt is the same shape as a wrong
  password (FR-006).
- [x] T202 [P] [US2] TEST `internal/web/api/auth_timing_test.go` — a sign-in attempt for an unknown
  email and one for a known email with a wrong password produce the **same body**, and the
  unknown-email path **performs the bcrypt comparison against the fixed dummy hash** (asserted
  through a counting seam on the hash comparer, not through a clock). A faster "no such account" is
  an account-existence oracle; the dummy comparison is what removes it, so this test asserts the
  mechanism rather than the latency. Deterministic — it blocks merge (FR-005, research D-17).
- [x] T202a [P] [US2] BENCH `internal/web/api/timing_bench_test.go` — benchmarks the two sign-in
  refusal paths and the two not-found paths (T226) side by side and reports the ratio of medians
  over `-benchtime` samples. **Not on the merge gate**: it is a `Benchmark*` function, so
  `go test ./...` does not run it, and CI runs it on merge to `main` for the trend only.
  A regression shows as a ratio drifting away from 1, investigated by a human against T202's
  mechanism assertion — never auto-failed on a threshold nobody can defend. Constitution VIII
  (no flaky gate assertion); replaces the undefined "agreed tolerance" (ANALYSIS N13).
- [x] T203 [P] [US2] TEST `internal/web/views/auth/{login,register}_test.go` — render to buffer,
  landmarks `form[name="Sign in"]` and `form[name="Create account"]`, errors adjacent to fields.
- [x] T204 [P] [US2] TEST `internal/web/views/settings/*_test.go` — profile, password and the danger
  zone, each rendering inside `region[name="Settings"]`.
- [x] T205 [P] [US2] TEST `internal/platform/pb/hooks_auth_test.go` — `OnRecordAuthRequest` writes
  the `login` audit row for **both** MediKube's own login route and PocketBase's native one. This
  is the one `OnRecord*Request` binding the phase permits and the reason for the forbidigo
  carve-out (research D-14).
- [x] T206 [P] [US2] TEST `internal/web/api/register_closed_test.go` — with registration closed,
  `POST /api/v1/auth/register` returns **403** `registration_closed` and the `/register` page
  **renders an explanation inside the normal application frame**, and the route is still present
  in the inventory. FR-002 is normative and says render an explanation; a 404 is what this
  codebase answers for owner-scoped data, and whether registration is open is instance-wide
  configuration, identical for every caller (contracts/auth.md, defect D15).
- [x] T207 [P] [US2] TEST `internal/service/identity/audit_coverage_test.go` — each of the six
  identity actions writes its audit row, driven from the enum so a new action cannot be added
  without a test.

### Implementation for User Story 2

- [x] T208 [P] [US2] Implement `internal/domain/identity/{user.go,enums.go,password.go,validate.go}`.
- [x] T209 [US2] Implement `internal/service/identity/ports.go` — `Repository`, `Authenticator`,
  `Auditor`, `Clock`.
- [x] T210 [US2] Implement `internal/service/identity/service.go` and `session.go`.
- [x] T211 [P] [US2] Implement `internal/service/identity/identitytest/fake.go`.
- [x] T212 [US2] Implement `internal/store/identity/repo.go`.
- [x] T213 [P] [US2] Implement `internal/web/api/dto_auth.go` and `dto_me.go` — `AuthConfig`,
  `PwRules`, `RegisterRequest`, `LoginRequest`, `Session`, `Me`, `MeCounts`, `MePatch`,
  `ChangePasswordRequest`, `DeleteAccountRequest`.
- [x] T214 [US2] Implement `internal/web/api/auth.go` — the five auth operations `getAuthConfig`,
  `register`, `login`, `refreshSession`, `logout`, registered under those `operationId`s
  (FR-001, FR-005).
- [x] T215 [US2] Implement `internal/web/api/me.go` — the four account operations `getMe`, `updateMe`,
  `changePassword`, `deleteMe`, registered under those `operationId`s (FR-011, FR-013).
- [x] T216 [US2] Implement `internal/web/session.go` — the `HttpOnly` cookie and the cookie→header
  middleware at priority -1021.
- [x] T217 [US2] Implement the `RefreshTokenKey` rotation on password change and account changes.
- [x] T218 [P] [US2] Implement `internal/web/views/auth/{login.templ,register.templ}`.
- [x] T219 [P] [US2] Implement
  `internal/web/views/settings/{profile.templ,password.templ,danger_zone.templ}` — the display name and
  the four preference controls (FR-011), and a danger zone that states plainly beforehand that deletion
  cannot be undone and asks for the password and the typed confirmation (FR-013).
- [x] T220 [US2] Implement `internal/web/page/{login.go,register.go,settings.go}`, registered with
  their landmarks and smoke URLs. **`/register` is registered unconditionally and renders an
  explanation inside the normal application frame when registration is closed** (FR-002, defect D15) — a
  route that disappears under configuration is a route the inventory gate cannot check.
- [x] T221 [US2] Implement the auth audit hooks in `internal/platform/pb/hooks.go`, writing the
  declared vocabulary of data-model §3 and nothing else — `create`, `login`, `login_failed`,
  `logout`, `password_change`, `account_delete`, each with `target_kind = user` (FR-036). A
  password replaced through a recovery link writes the same `password_change` row, and a
  confirmed address writes `update` / `user`: **no new action value is introduced**, so the
  vocabulary counts every later phase asserts are unchanged (data-model §3).
- [x] T221a [P] [US2] TEST `internal/platform/pb/hooks_admin_session_test.go` — a superuser
  authenticating against the `_superusers` collection writes exactly one `admin_session` audit row
  with `actor_kind = superuser` and `target_kind = user`, and an ordinary sign-in writes none. This
  is the tenth of the ten action values data-model §3 declares this phase writes, and the third
  clause of FR-040 — the credential separation and the boot warning are T055–T059 and T064/T065
  (FR-040, data-model §3).
- [x] T221b [US2] Implement the `admin_session` audit hook in `internal/platform/pb/hooks.go` —
  `OnRecordAuthRequest` bound on `_superusers` alongside T221's binding on `users`. It writes an
  already-declared action value, so the vocabulary is still unchanged (FR-040, data-model §3).
- [x] T222 [US2] Add the seeded accounts A, B and C to `internal/cli/seed.go` with deterministic ids
  (data-model §6, FR-060). **Account C is seeded with an unconfirmed address**, so the settings
  page's "not confirmed, send it again" state is a seeded smoke case rather than an untested
  branch (FR-075).
- [x] T223 [P] [US2] TEST `e2e/smoke.spec.ts` cases for `/login`, `/register` and `/settings`, and
  `e2e/auth.setup.ts` signing in as the seeded account and storing the state.

### Recovery and confirmation for User Story 2 (FR-073 … FR-077) ⚠️ tests first

These sixteen tasks are the flows cross-artifact finding **H7** allocated to this phase. Every one
of them wires a PocketBase mechanism rather than building one: `mails.SendRecordPasswordReset`,
`mails.SendRecordVerification`, `app.FindAuthRecordByToken(token, core.TokenTypePasswordReset |
core.TokenTypeVerification)`, and `SetPassword` + `Save` — which rotates `tokenKey` and therefore
ends every prior session through the same mechanism T200 already asserts.

- [x] T223a [P] [US2] TEST `internal/domain/identity/recovery_test.go` — the rules that hold with
  no token in sight: a password chosen through recovery is validated by **the same** published
  rules as one chosen at registration (FR-004, FR-074), and the response to a recovery request is
  a value that cannot vary with whether an account exists (FR-073).
- [x] T223b [P] [US2] TEST `internal/service/identity/recovery_test.go` against the fakes —
  `RequestPasswordReset`, `ConfirmPasswordReset`, `RequestVerification`, `ConfirmVerification`,
  each asserting its audit write and its call to the consumer-declared `Mailer` port. The service
  never touches `mails.*` directly; that is the adapter's job (Principle II).
- [x] T223c [P] [US2] TEST `internal/web/api/password_reset_test.go` — `ApiScenario` cases per
  contracts/auth.md: a known address and an unknown address produce **byte-identical** `202`
  bodies; a valid token sets the password and returns `204`; an expired, already-used or tampered
  token is `400 invalid_token` with one message for all three; repeated requests are `429`
  (FR-073, FR-074, FR-077).
- [x] T223d [P] [US2] TEST `internal/web/api/verify_email_test.go` — `requestEmailVerification`
  requires a session and is refused `401` without one; `confirmEmailVerification` is public,
  accepts the token once, sets the address confirmed, and answers a second use exactly as it
  answers an expired one (FR-075).
- [x] T223e [P] [US2] TEST `internal/web/api/mail_unconfigured_test.go` — with
  `Settings().SMTP.Enabled` false, **neither** request pretends to have sent anything: the answer
  is `503 mail_unconfigured` with a message naming no address, the failure is logged **once per
  burst rather than once per attempt**, and no audit row claims a message was sent (FR-076).
- [x] T223f [P] [US2] TEST `internal/platform/pb/adminwarn_test.go` — the boot warning covers
  **three** conditions, not two: superuser MFA unconfigured, the superuser IP allowlist
  unconfigured (FACT 10) and outgoing mail unconfigured (FR-076). Table-driven, each condition
  asserted independently and in combination.
- [x] T223g [P] [US2] TEST `internal/service/identity/reset_revocation_test.go` — a password set
  through a recovery link ends **every** session issued before it, asserted the same way T200
  asserts it for a deliberate password change (FR-074).
- [x] T223h [P] [US2] TEST
  `internal/web/views/auth/{forgot_password_test.go,reset_password_test.go,verify_email_test.go}` —
  render to buffer: landmarks `form[name="Reset password"]`, `form[name="Choose a new password"]`
  and `region[name="Email confirmation"]`; the **expired-link state renders inside the landmark**
  with the offer to request another (FR-074); errors adjacent to their field.
- [x] T223i [P] [US2] TEST `internal/web/api/auth_enumeration_test.go` — the recovery response for
  an address with an account and for one without are identical apart from `request_id`, **and the
  handler performs the same work on both branches** — same response constructor, no early return
  on the no-account branch (`contracts/auth.md`, `requestPasswordReset`). Asserted structurally,
  not with a clock: no wall-clock assertion here either, and the latency is reported by the
  non-gating benchmark T202a (FR-073, research D-17; ANALYSIS N13).
- [x] T223j [US2] Implement the `Mailer` port in `internal/service/identity/ports.go` — two
  methods, `SendPasswordReset(ctx, userID)` and `SendVerification(ctx, userID)`, declared by the
  consumer — and its adapter `internal/platform/pb/mail.go` over `mails.SendRecordPasswordReset`
  and `mails.SendRecordVerification`, plus a fake in `identitytest`.
- [x] T223k [US2] Implement `internal/service/identity/recovery.go` — the four service methods,
  each writing its audit row and each returning the same result shape whether or not the account
  exists.
- [x] T223l [US2] Implement the four operations in `internal/web/api/auth.go` and their DTOs in
  `dto_auth.go`, and register them in `internal/httproute/routes.go` as `requestPasswordReset`,
  `confirmPasswordReset`, `requestEmailVerification` and `confirmEmailVerification`
  (contracts/auth.md, contracts/README.md operations 19–22).
- [x] T223m [P] [US2] Implement
  `internal/web/views/auth/{forgot_password.templ,reset_password.templ,verify_email.templ}`,
  including the expired/used-link state and the "this instance cannot send mail" state.
- [x] T223n [US2] Implement `internal/web/page/{forgot_password.go,reset_password.go,verify_email.go}`
  registered with their landmarks and with the **deterministic invalid-token** smoke URLs
  `/reset-password/expired-token-for-smoke` and `/verify-email/expired-token-for-smoke`, both of
  which answer `200` with the expired-link state (contracts/pages.md).
- [x] T223o [US2] Implement the mail-unconfigured refusal and the third boot warning in
  `internal/platform/pb/adminwarn.go` and the auth handlers, and confirm the two token durations
  MediKube inherits are the documented ones — reset **30 minutes**, confirmation **24 hours** —
  writing them into `contracts/auth.md` if PocketBase's defaults differ from what is documented
  there.
- [x] T223p [P] [US2] `e2e/smoke.spec.ts` cases for `/forgot-password`, `/reset-password/{token}`
  and `/verify-email/{token}` at both viewports, plus `e2e/recovery.spec.ts` driving the whole
  recovery flow against a mail sink: request → read the link out of the sink → set a new password
  → the old session is dead → sign in with the new password (SC-016, Phase Exit Criterion 8).

**Checkpoint**: US1 and US2 both work independently. All twelve of US2's acceptance scenarios pass.

---

## Phase 5: User Story 3 — My records are mine alone (Priority: P3)

**Goal**: make the privacy guarantees **structural** — one authorization checkpoint, refusals
indistinguishable from not-founds, an audit trail that cannot be edited, and an operational record
with no patient content in it.

**Independent Test**: as account B, attempt every operation on account A's medication and receive
responses byte-identical to a not-found; confirm each attempt produced an `access_denied` audit
row; grep the whole captured log stream and find no medication name.

### Tests for User Story 3 ⚠️ write first, watch them fail

- [x] T224 [P] [US3] TEST `internal/service/access/authorizer_test.go` — the single checkpoint:
  owner allowed, stranger refused, superuser handled explicitly rather than by accident, and an
  unauthenticated actor refused. Table-driven over every `Permission`.
- [x] T225 [US3] TEST `internal/service/access/exhaustive_test.go` — a source-walking test asserting
  **every** service method that touches a record calls the authorizer. A new method that forgets
  fails the build rather than the review (FR-032).
- [x] T226 [P] [US3] TEST `internal/web/api/notfound_parity_test.go` — for every record operation,
  the stranger response and the genuine-not-found response are compared **byte for byte** after
  masking `request_id`, and both are asserted to be emitted by the **same response constructor**
  — one `notFound` call site, reached by both the refusal and the genuine miss, so the two cannot
  drift apart under a later edit. No wall-clock assertion here: latency is reported by the
  non-gating benchmark T202a, because a tolerance nothing defines is the flaky gate assertion
  Constitution VIII forbids (FR-033, SC-006; ANALYSIS N13).
- [x] T227 [P] [US3] TEST `internal/web/api/anonymous_test.go` — every route except the public five
  refuses an anonymous caller, and the refusal never reveals whether the target exists (FR-034).
- [x] T228 [P] [US3] TEST `internal/service/audit/denied_test.go` — a refused access writes an
  `access_denied` row and a **genuine** not-found writes nothing. That distinction is the whole
  value of introducing the enum value here rather than in phase 003 (research D-20).
- [x] T229 [P] [US3] TEST `internal/obs/tracing_test.go` — with no endpoint configured, tracing is
  entirely inactive: no exporter, no spans, no outbound connection (FR-039, FR-056).
- [x] T230 [P] [US3] TEST `internal/web/page/errors_test.go` — the three error views render inside
  the full shell, carry their landmarks, and contain no stack trace, driver message or query
  (FR-046).
- [x] T231 [P] [US3] TEST `internal/service/audit/writer_test.go` — `Record(ctx, Event)` writes each
  of the ten actions; the `Event` type **has no field capable of carrying clinical content**, so
  a leak would have to be a schema change (FR-038). **Plus the no-request case**: called on a
  context with no HTTP request — a cron, a job, a migration, a backfill — it fills `request_id`
  from the context's **run id** and never writes the empty string, asserted by a case that calls
  it on a bare `context.Background()` derived run context and reads the row back. Without this the
  retention purge fails `Required` validation on its first nightly tick, in production, not in
  test (ANALYSIS).
- [x] T232 [US3] TEST `internal/platform/pb/hooks_audit_immutability_test.go` — update and delete on
  `audit_events` are refused **through every path**: the API, the record hooks, and a direct
  service call. Only the retention job may remove a row, and only past the retention horizon
  (FR-037).
- [x] T233 [P] [US3] TEST `internal/service/audit/retention_test.go` — rows older than the configured
  retention (default two years) are removed and nothing younger is.
- [x] T234 [P] [US3] TEST `internal/config/retention_default_test.go` — the audit retention default
  is two years and is a documented setting rather than a constant buried in the purge job
  (FR-037).
- [x] T235 [US3] TEST `internal/testsupport/phileak/{exercise.go,phileak_test.go}` — **the phase's
  most valuable single test**: `exercise.go` drives every endpoint this phase defines against a
  sentinel-seeded instance with distinctive medication names, doses, reasons and notes;
  `phileak_test.go` asserts **zero** occurrences of any of those strings in the four sinks
  `capture.go` (T089) records, naming the sink on failure (FR-038, SC-005). Phases 002–006 extend
  `exercise.go` with their own sentinels and operations, never the assertion, and run it through
  `task test:phileak`.
- [x] T236 [P] [US3] TEST `internal/obs/sentry_test.go` — with no DSN configured, nothing is sent
  anywhere; with one configured, the `BeforeSend` scrubber strips request bodies, cookies and
  query strings (FR-039, FR-056).
- [x] T237 [P] [US3] TEST `internal/obs/metrics_test.go` — the metrics listener binds to
  **127.0.0.1 only** and is unreachable from the application's own port (FR-055).
- [x] T238 [P] [US3] TEST `internal/web/api/bulk_absence_test.go` — no general-purpose browsing or
  bulk-extraction facility is reachable by an ordinary account: the collections subtree is 404,
  `/api/batch` is 404, and no MediKube route accepts an arbitrary filter expression (FR-035).

### Implementation for User Story 3

- [x] T239 [US3] Implement `internal/service/access/authorizer.go` — **THE** authorization
  checkpoint. Every service consults it; nothing else decides.
- [x] T240 [US3] Implement `internal/service/audit/writer.go` — `Record(ctx, Event)`, called
  post-commit, resolving `request_id` from the request id when the context carries one and from
  the **run id** otherwise. Both are minted by the same helper as `internal/obs`'s request id, so
  a background run's audit row and its zerolog lines carry the same correlation handle and FR-054
  holds for system rows too (depends on T231).
- [x] T241 [P] [US3] Implement `internal/service/audit/retention.go` and the cron binding in
  `internal/platform/pb/cron.go`.
- [x] T242 [US3] Implement the audit-immutability guards in `internal/platform/pb/hooks_audit_immutability.go` — a separate file from `hooks.go`, which US2's auth audit hooks own, matching the existing `hooks_records.go` split.
- [x] T243 [US3] Implement `internal/store/audit/repo.go`.
- [x] T244 [P] [US3] Implement `internal/obs/sentry.go` — off entirely without a DSN, with the
  `BeforeSend` scrubber.
- [x] T245 [P] [US3] Implement `internal/obs/metrics.go` — the registry, the collectors and the
  loopback-only listener.
- [x] T246 [P] [US3] Implement `internal/obs/tracing.go` — off entirely without an endpoint.
- [x] T247 [US3] Implement `internal/obs/db.go` — `otelsql` through `pocketbase.Config.DBConnect`,
  **copying PocketBase's own pragmas exactly**, plus a drift check that fails the build when
  upstream changes them (risk R8).
- [x] T248 [P] [US3] Implement `internal/web/page/errors.go` and
  `internal/web/views/errors/{not_found,forbidden,server_error}.templ` — the three error views,
  inside the full shell, carrying only the request id (FR-046).

**Checkpoint**: the privacy guarantees are enforced by tests that fail when the code forgets. All
eight of US3's acceptance scenarios pass.

---

## Phase 6: User Story 4 — Find my way around without getting lost (Priority: P4)

**Goal**: one shell, four landmarks, on every page, at both viewports, proven by a render gate
rather than by looking at it.

**Independent Test**: visit all nine pages and all three error views at 1440×900 and 390×844 with
the console open; every page has the shell, nothing overflows, and the console is silent.

### Tests for User Story 4 ⚠️ write first, watch them fail

- [x] T249 [P] [US4] TEST `internal/web/views/layout_test.go` — render to buffer: the skip link is
  the first focusable element, and `banner`, `navigation[name="Primary"]`, `main` and `contentinfo`
  are all present, in order, on every page (FR-043).
- [x] T250 [P] [US4] TEST `internal/web/page/shell_test.go` — `#error-banner` and `#toast` are
  rendered on **every** page even when empty. Datastar patches by id and an element that does not
  exist cannot be patched.
- [x] T251 [P] [US4] TEST `internal/web/views/shell/theme_test.go` — the theme class is on `<html>`,
  **server-rendered** from the stored preference, and the follow-the-device setting emits no class
  and relies on the Tailwind `dark` variant responding to `prefers-color-scheme`. No inline script:
  the CSP bans it and `data-persist` is Datastar Pro (FR-045, research D-36).
- [x] T252 [P] [US4] TEST `internal/web/views/shell/noscript_test.go` — every page carries a
  `<noscript>` block **inside `main`** stating plainly that MediKube requires scripting (FR-049).
- [x] T253 [P] [US4] TEST `internal/web/page/nav_test.go` — the current location is marked
  `aria-current`, and every page offers a route back to the medication list (FR-050).
- [x] T254 [P] [US4] TEST `internal/web/render_test.go` — after a full-region patch, focus is moved
  to the patched region's heading (FR-048).
- [x] T255 [P] [US4] TEST `internal/web/api/feedback_test.go` — every data-changing operation
  produces an explicit success or failure message announced through the `role="status"` or
  `role="alert"` region (FR-047).
- [ ] T256 [P] [US4] TEST `e2e/a11y.spec.ts` — keyboard reachability and a visible focus indicator on
  every interactive element of every page, at both viewports (FR-048, SC-014).
- [ ] T257 [P] [US4] TEST `e2e/responsive.spec.ts` — at 390 px no page scrolls horizontally and
  every navigation target remains reachable (FR-044).
- [x] T258 [P] [US4] TEST `internal/web/views/shell/live_regions_test.go` — `#error-banner` is
  `role="alert" aria-live="assertive"` and `#toast` is `role="status" aria-live="polite"`, on
  every page (FR-047).
- [x] T259 [P] [US4] TEST `internal/architecture/templ_coverage_test.go` — every `.templ`
  component has a render-to-buffer test. A component nobody renders in a test is a component the
  gate only catches once it is already on a page (Principle VIII, SC-014).
- [ ] T260 [P] [US4] TEST `e2e/smoke.spec.ts` — the seven assertions from contracts/pages.md for
  every page **and every error view**, at both viewports: status, shell landmarks, the page's own
  landmark **non-empty**, title, zero console errors or warnings, zero CSP violations, zero failed
  requests (FR-066, SC-003).

### Implementation for User Story 4

- [x] T261 [US4] Implement `internal/web/views/layout.templ` — the shell: skip link, banner, nav,
  main, `#error-banner`, `#toast`, footer.
- [x] T262 [P] [US4] Implement `internal/web/views/shell/{nav.templ,theme.templ,noscript.templ}`.
- [x] T263 [US4] Implement `internal/web/page/shell.go` — `NavState`, the layout wrapper and theme
  resolution.
- [x] T264 [P] [US4] Implement `internal/web/page/dashboard.go` and the Overview page, registered
  with `region[name="Overview"]`.
- [x] T265 [P] [US4] Implement the responsive rules in `assets/input.css` and the templ markup so
  nothing overflows horizontally at 390 px (FR-044).
- [x] T266 [P] [US4] Wire the appearance preference through `updateMe` to the rendered class
  (FR-045).
- [ ] T267 [US4] Configure `e2e/playwright.config.ts` with the two projects — desktop 1440×900 and
  mobile 390×844 — and the two flakiness mitigations from reconciliation C16.
- [ ] T268 [US4] Implement `e2e/routes.ts` — the smoke list produced by shelling out to
  `medikube routes --json` at collection time, so the gate's page list **is** the application's own
  inventory (FR-067).
- [ ] T269 [P] [US4] Implement `e2e/package.json` and pin the Playwright version.

**Checkpoint**: every page renders, at both viewports, with a silent console. All eight of US4's
acceptance scenarios pass.

---

## Phase 7: User Story 5 — Run the instance and know it is healthy (Priority: P5)

**Goal**: an operator can start it, migrate it, seed it, observe it, and stop it cleanly, learning
nothing about the data from any of those signals.

**Independent Test**: from a clean machine, reach a ready seeded instance in under ten minutes
using only `quickstart.md`; make storage unreachable and watch liveness stay up while readiness
goes down with a reason that reveals nothing; then shut down while a request is in flight and see
it complete.

### Tests for User Story 5 ⚠️ write first, watch them fail

- [x] T270 [P] [US5] TEST `internal/web/api/health_test.go` — `healthz` returns 200 with status,
  version and start time and **performs no database access**, asserted by running it against an
  app whose database has been made unreadable (FR-052).
- [x] T271 [P] [US5] TEST `internal/web/api/readyz_test.go` — the three checks, the 2-second probe
  deadline, 503 when the database is unreachable, and a body containing **only** the closed check
  vocabulary — no path, no DSN, no driver message (FR-052, SC-013).
- [x] T272 [P] [US5] TEST `internal/web/api/readyz_migrations_test.go` — with a migration
  outstanding, readiness reports not-ready and names `migrations` as the failing check.
- [x] T273 [US5] TEST `internal/platform/pb/drain_test.go` — the `OnTerminate` handler at priority
  **-10000** runs before PocketBase's shutdown at -9999: readiness flips to `draining`, the delay
  elapses, in-flight requests complete, and only then does the process exit. PocketBase's
  one-second window is hardcoded and unreachable from config, so without this a request is cut
  mid-response (FR-062, observability §8).
- [x] T274 [P] [US5] TEST `internal/web/api/readyz_draining_test.go` — while draining, `readyz`
  returns **503 draining** with an empty check set, and an in-flight request still completes
  successfully.
- [ ] T275 [P] [US5] TEST `internal/cli/seed_test.go` — the seed is deterministic (same ids twice),
  **idempotent** (running it twice changes nothing), and **refuses to run** when
  `MEDIKUBE_ENV=production` with no override (FR-060).
- [x] T276 [P] [US5] TEST `internal/cli/routes_test.go` — `medikube routes --json` lists exactly the
  registry's routes, needs no database and binds no port.
- [x] T277 [P] [US5] TEST `internal/web/api/health_exclusions_test.go` — probe traffic appears in
  neither the activity log, nor the metrics, nor the traces.
- [x] T278 [P] [US5] TEST `internal/cli/migrate_test.go` — `migrate down` is refused when
  `MEDIKUBE_ENV=production` without `--force`, because the down of migration 3 drops `audit_events`.
- [x] T279 [P] [US5] TEST `internal/cli/openapi_cmd_test.go` — the command writes a document
  byte-identical to the committed `api/openapi.json`.
- [ ] T280 [P] [US5] TEST `internal/config/failure_output_test.go` — a configuration failure names
  the offending variable and **never** its value (FR-041).
- [x] T281 [P] [US5] TEST `internal/cli/healthcheck_test.go` — exits 0 against a ready instance and
  non-zero otherwise, printing nothing on success.
- [x] T282 [P] [US5] TEST `internal/cli/root_test.go` — MediKube's subcommands are registered on
  PocketBase's `RootCmd`, and **every MediKube flag is defined on its own subcommand**: PocketBase
  pre-parses `--dir`, `--encryptionEnv` and `--dev` from `os.Args` inside `NewWithConfig`, before
  Cobra runs, and swallows an unrecognised global flag silently.
- [ ] T283 [P] [US5] TEST `internal/logging/correlation_test.go` — one request produces log lines
  that all carry the same correlation id, and that id is the one returned to the client (FR-054).
- [x] T284 [US5] TEST `internal/obs/single_report_test.go` — one failure appears **once** across the
  log stream, the error reporter and the trace — not three times (FR-057).

### Implementation for User Story 5

- [x] T285 [US5] Implement `internal/web/api/health.go` — `healthz` and `readyz`, excluded from the
  activity logger, the metrics middleware and the tracing middleware.
- [x] T286 [US5] Implement the drain handler in `internal/platform/pb/serve.go` at priority -10000,
  with `MEDIKUBE_DRAIN_DELAY` and `MEDIKUBE_DRAIN_MAX`, and handle `TerminateEvent.IsRestart`.
- [x] T287 [P] [US5] Implement `internal/cli/root.go` — subcommand registration and the removal of
  the PocketBase commands that would bypass the lockdown. T287–T291 and `serve` are together the six
  operator commands of FR-058.
- [x] T288 [P] [US5] Implement `internal/cli/seed.go` (FR-058, FR-060).
- [x] T289 [P] [US5] Implement `internal/cli/routes.go` (FR-058).
- [x] T290 [P] [US5] Implement `internal/cli/openapi.go` (FR-058, FR-064).
- [x] T291 [P] [US5] Implement `internal/cli/healthcheck.go` (FR-058).
- [ ] T292 [US5] Implement the boot sequence in `cmd/medikube/main.go` — config, migrations, the two
  boot assertions, settings, the admin warning, and **one** startup line at info (FR-053).

**Checkpoint**: the instance is operable and observable. All nine of US5's acceptance scenarios
pass.

---

## Phase 8: User Story 6 — Every change proves itself before it ships (Priority: P6)

**Goal**: the gates. Every one of them must be **demonstrated to fail** before this phase is
accepted; a gate nobody has watched go red is decoration.

**Independent Test**: break one thing per gate — a page's landmark, a console error, an
undocumented route, a PocketBase import in a domain package, an `app.Logger()` call — and watch
each break the build for the right reason.

### Tests for User Story 6 ⚠️ these ARE the gates

- [x] T293 [P] [US6] TEST `internal/openapi/staleness_test.go` — regenerating the document produces
  **no diff** against the committed `api/openapi.json`; CI runs `task openapi` and
  `git diff --exit-code` (FR-064).
- [ ] T294 [P] [US6] TEST `internal/httproute/gate_test.go` — the registry, the OpenAPI document and
  the Playwright route list agree in every direction, with no route documented but unserved and
  none served but undocumented (FR-065, SC-011).
- [ ] T295 [US6] TEST `e2e/routes.gate.spec.ts` — a registered page with no smoke case **fails**.
  Then demonstrate it: add a page without a case, watch it go red, and record the demonstration in
  `e2e/README.md` (FR-067, SC-009).
- [ ] T296 [US6] TEST — **the red-gate demonstration for a removed landmark.** Build with one
  landmark deleted, run `task smoke`, confirm it **fails**, record the output in `e2e/README.md`,
  and revert. This is the single check most likely to be skipped and the one whose absence makes
  every UI claim in this phase worthless (FR-072, SC-010, risk R11).
- [ ] T297 [US6] TEST — **the red-gate demonstration for a console error.** Add a deliberate
  `console.error` to one page, run `task smoke`, confirm it **fails**, record it, and revert
  (FR-072, SC-010).
- [ ] T298 [P] [US6] TEST `internal/architecture/depguard_test.go` — a fixture package importing
  `github.com/pocketbase/pocketbase/core` from `internal/domain` makes `task lint` fail. Demonstrate
  it once and record the output (Principle II).
- [ ] T299 [P] [US6] TEST `internal/architecture/forbidigo_test.go` — an `app.Logger()` call outside
  `internal/logging` makes `task lint` fail, and an `OnRecordCreateRequest` binding outside
  `internal/platform/pb/hooks.go` does too (Principles VI, reconciliation C13).
- [ ] T300 [US6] TEST `internal/architecture/scenarios_test.go` — a coverage test asserting that
  **all 50** acceptance scenarios in `spec.md` have a named automated test, matched by a scenario
  identifier in the test name. A scenario without a test fails this test (FR-068, SC-004).
- [ ] T301 [P] [US6] TEST `internal/architecture/scenario_ids_test.go` — every acceptance-scenario
  identifier is unique and every one is claimed by exactly one test, so two tests cannot both
  claim a scenario while another has none.
- [ ] T302 [P] [US6] TEST `internal/architecture/authz_coverage_test.go` — every route in the
  registry that touches clinical data appears in `RunOwnershipMatrix` (FR-069).
- [ ] T303 [P] [US6] TEST `internal/architecture/ci_workflow_test.go` — the CI workflow contains
  the `stream-liveness` job, does **not** set `GOTOOLCHAIN=local`, and runs every gate. A gate
  that exists but is not wired into CI is not a gate (FR-070).

### Implementation for User Story 6

- [x] T304 [US6] Complete `.github/workflows/medikube-ci.yml` — `gen`, `vet`, `lint`, `test`,
  `openapi-diff`, `e2e` and the separate **`stream-liveness`** job that runs longer than five
  minutes. Every failure blocks the merge (FR-070).
- [ ] T305 [US6] Verify the image builds through the shared pipeline from a clean checkout on the
  first attempt — `/.dockerignore` and `/.github/workflows/build-image.yaml` both correct
  (FR-071, SC-015).
- [ ] T306 [P] [US6] Write `e2e/README.md` — how each red-gate demonstration was performed, with the
  captured failure output. **This file is the evidence for exit criterion 5.**
- [x] T307 [P] [US6] Commit the generated `api/openapi.json`.

**Checkpoint**: every gate exists **and has been seen to fail**. All eight of US6's acceptance
scenarios pass.

---

## Phase 9: Polish & Cross-Cutting

- [ ] T308 [P] Run `quickstart.md` start to finish on a clean machine and **time it**; fix the
  document until the ten-minute claim in SC-008 is true rather than aspirational.
- [ ] T309 [P] Verify every one of the sixteen success criteria has a named test or a recorded
  measurement, and list where each is proven.
- [ ] T310 [P] TEST `internal/web/api/errors_taxonomy_test.go` — every error code in
  contracts/README.md's table is producible and no handler invents one outside it.
- [ ] T311 [P] TEST `internal/web/api/unknown_field_test.go` — every write DTO rejects unknown
  fields, asserted by reflection over the DTO set so a new DTO is covered by default.
- [ ] T312 [P] Add the `medications` empty-state, single-row and full-list cases to the smoke run so
  the widest and the narrowest row are both exercised (research D-39).
- [ ] T313 [P] Review every log call site for content: ids and codes only, never a name, dose,
  reason or note. `internal/testsupport/phileak` enforces it; this is the human pass that finds
  what the fixture strings missed.
- [ ] T314 [P] Update `docs/pocketbase-upgrade-checklist.md` with the final list of unexported
  internals depended upon and the exact symptom each produces on breakage (risk R8, CT-1).
- [ ] T315 [P] **Resolved, not open** (cross-artifact finding H1): phase 002's `contracts/` and
  `quickstart.md` were corrected to the **plural** `/api/v1/records/medications` on 2026-08-27,
  matching the constant created here. Confirm only — assert `kind.Kind.Segment()` emits the
  plural and grep the suite for a surviving singular `records/medication`.
- [ ] T316 [P] **Resolved, not open** (cross-artifact finding H7): password recovery and email
  confirmation are built in this phase (T223a–T223p) and external sign-in is owned by phase 006.
  [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) §2.3 and §3.1 have been amended to match (ops 7, 8, 94 and 95 under phase
  001, op 4 under phase 006, the three pages back in phase 001's inventory; totals **94**
  operations and **58** pages). What remains here is bookkeeping: confirm no document in the suite
  still describes any of the three as belonging to nobody, and that every phase's running totals
  cite the contract's amended figures rather than a re-derived one.
- [ ] T317 [P] Confirm the exact PocketBase v0.40.1 field name backing the cursor HMAC key
  derivation and update `internal/store/cursor.go` and research D-25 if it differs (**CT-3**).
- [ ] T318 [P] Publish the complete documented environment in `README.md` — every `MEDIKUBE_`
  variable, its default and whether it is required — and check `quickstart.md`'s minimum set
  matches it exactly (FR-051, SC-008).
- [ ] T319 [P] Run `task lint` with `--max-issues-per-linter=0 --max-same-issues=0` and clear
  everything.
- [ ] T320 [P] Delete every scaffold `doc.go` that never acquired a package.
- [ ] T321 [P] Re-run the full suite three times to catch order-dependence, and once with
  `-race`.
- [ ] T322 [P] TEST `internal/architecture/test_isolation_test.go` — no test shares a
  `tests.TestApp` with another, asserted by a source walk. The symptom of getting this wrong is a
  stack overflow, not a readable failure (reconciliation C14).
- [ ] T323 [P] Verify `internal/testsupport/authz.go` and `internal/records/registry.go` read
  cleanly as the API phases 002–006 will extend — this phase's real deliverable is what comes
  next, and a bad seam here is paid for five times.
- [ ] T324 Write `specs/001-walking-skeleton/traceability.md` — the mechanical join, generated from
  `spec.md` and `tasks.md` rather than written by hand: one row per functional requirement naming
  the task ids that satisfy it and the named test that proves it, and one row per success criterion
  naming its task or its phase-exit criterion. **A functional requirement with no task, or a success
  criterion that is neither mapped nor marked `[outcome metric]`, fails the phase.** This is the
  same file exit criterion 1 already requires for the 54 acceptance scenarios; the requirement and
  success-criterion joins live beside them (cross-artifact finding M7).
- [ ] T325 Walk `plan.md`'s eleven Phase Exit Criteria and confirm each, in writing, before declaring
  the phase complete.

---

## Dependencies & Execution Order

### Phase dependencies

```
Setup  T001-T017
  │
  ▼
Foundational  T018-T133   ← BLOCKS EVERYTHING. Nothing below can start.
  │
  ├────────────┬────────────┬────────────┬────────────┬────────────┐
  ▼            ▼            ▼            ▼            ▼            ▼
US1 P1      US2 P2       US3 P3       US4 P4       US5 P5       US6 P6
T134-T185   T186-T223p   T224-T248    T249-T269    T270-T292    T293-T307
  │            │            │            │            │            │
  └────────────┴────────────┴────────────┴────────────┴────────────┘
                                   │
                                   ▼
                        Polish  T308-T325
```

### Real cross-story dependencies

The stories are independently testable, but four honest edges exist and pretending otherwise
would produce a schedule that does not work:

- **US3's `RunOwnershipMatrix` cases for medications need US1's routes.** The authorizer itself
  (T239) does not; the matrix run over the record family does.
- **US4's smoke run needs at least one story's pages to exist.** Run it against US1's pages first
  and add the rest as they land; the gate is per-page, not all-or-nothing.
- **US6's OpenAPI and route-inventory gates need the routes registered**, so they are meaningful
  only once US1 and US2 are in. Write the gate tests early anyway — they should fail loudly while
  the routes are missing, which is exactly what a gate is for.
- **US2's account-deletion cascade test needs US1's medications.** That is the point of the test:
  it proves the foreign key, and there is nothing to cascade without the kind.

### Within each story

1. TEST tasks first, and **watch them fail for the right reason** before writing anything
2. Domain → ports → service → store → HTTP → templ → page → stream
3. Contract suites (`*test/contract.go`) before either implementation of the interface they
   describe, so both are held to one standard
4. The story is done when its acceptance scenarios pass, not when its code compiles

### Critical path

`T001 → T012 (build smoke) → T018 (config) → T023 (logging) → T055-T059 (the lockdown) →
T068-T076 (migrations) → T093-T109 (registry + records) → T134 (US1)`. Everything else has slack. The lockdown and the log
bridge are the two places where getting it wrong is discovered late and expensively.

---

## Parallel Execution Examples

**Setup** — everything except `go.mod` and the dependency pin runs at once:

```
T003 skeleton  │ T004 golangci │ T005 gitignore    │ T006 Taskfile │ T007 input.css
T008 datastar  │ T009 version  │ T010 Dockerfile   │ T011 dockerignore
T013 forbidden-deps test │ T014 build-image │ T015 CI │ T016 README │ T017 upgrade-checklist
```

T012 (the PocketBase build smoke test) runs **first and alone** — if the toolchain cannot build
PocketBase, nothing else in this phase is worth starting.

**Foundational, the four independent tracks** — four people, no collisions:

```
A: config + logging                      T018-T033
B: domain primitives                     T034-T050
C: platform/pb + migrations + store      T051-T084
D: test support + registry + openapi     T085-T109
E: the HTTP edge                         T110-T133
```

Track D can be written against the registry interface before track C's app exists, because the
registry does not import PocketBase until `Bind(se)`. Track E depends on nothing but the domain
primitives from track B.

**US1's tests** — all 30 are `[P]` except the two integration ones that share a fixture:

```
T134 enums   │ T135 validate │ T136 marshal   │ T137 service  │ T138 contract suite
T139 fake    │ T141 api      │ T142 list      │ T143 create   │ T144 validation
T145 delete  │ T146 etag     │ T147 authz     │ T148 row      │ T149 list view  …
```

The two that are **not** `[P]`: `T140` (the repository integration test) and `T160` (the
>5-minute stream liveness test), because both own a fixture app for the duration.

**US1's implementation tasks T161-T185** are largely sequential — domain, then ports, then
service, then store, then the adapter — with the templ components (T170-T175) parallel among
themselves.

**Across stories, once Foundational is green** — six people, one story each. US1 is the MVP and
should be staffed first regardless.

---

## Implementation Strategy

### MVP

1. Setup (T001-T017)
2. **Foundational (T018-T133)** — the long pole, and the reason this phase exists
3. US1 (T134-T185)
4. **Stop. Demonstrate.** Record, edit, delete a medication; two views updating live; a stranger
   refused indistinguishably. That is a walking skeleton and it is worth showing to somebody.

### Then, in priority order

US2 makes it multi-account and real. US3 makes the privacy claims **testable** rather than
asserted. US4 makes it navigable and proves it renders. US5 makes it operable. US6 closes the
gates and demonstrates each one going red.

### What not to do

- **Do not defer US6.** The gates are cheap to write alongside and expensive to retrofit, and a
  gate written after the code it guards is written to pass.
- **Do not skip the red-gate demonstrations** (T296, T297). They are two afternoons and they are
  the difference between a render gate and a decoration.
- **Do not start a story before Foundational is green.** Every one of them imports it.

---

## Notes

- `[P]` = different file, no unstarted dependency
- Commit after each task or logical group; the checkpoints are the natural review points
- **Every test task is done only when the test has failed for the right reason first**
- Task counts: **345 total** (`grep -cE '^- \[ \] T[0-9]+' tasks.md`), across nine phases. The
  earlier "341 total, 187 of them test tasks (54.8%)" is withdrawn: the total was stale, and the
  test-task figure was produced by a regex heuristic no stable rule reproduces, so it is dropped
  rather than re-guessed (corrected 2026-08-27, ANALYSIS N13's edit and N8's rule). Suffixed ids
  are used rather than renumbering, so that every task id cited elsewhere in the suite still points
  at the same task: the sixteen `T223a`–`T223p` tasks are password recovery and email confirmation,
  which cross-artifact finding **H7** allocated to this phase, and `T202a` is the non-gating timing
  benchmark
- The three Complexity Tracking entries in `plan.md` (CT-1 the log bridge, CT-2 the record
  registry with one kind, CT-3 the cursor key derivation) each have their mitigation tasks in this
  list: CT-1 → T031, T032 and T017; CT-2 → T109 and T099; CT-3 → T077 and T317
