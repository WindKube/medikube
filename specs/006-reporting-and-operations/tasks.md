---
description: "Task list for phase 006 — Reporting and Operations"
---

# Tasks: Reporting and Operations

**Input**: Design documents from `/specs/006-reporting-and-operations/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: **MANDATORY.** Constitution Principle III makes test-first non-negotiable, and this
specification demands it by name (FR-124, FR-125, FR-126, FR-128, FR-129, FR-130, FR-131, SC-025).
Every test task below precedes the implementation task it covers. A red-to-green transition that was
never red is a defect.

**Organization**: by user story, in the spec's priority order. Each story is independently
implementable, testable and demonstrable once the Foundational phase is complete.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel with its siblings (different files, no incomplete dependency)
- **[Story]**: `[US1]`…`[US9]`; Setup, Foundational and Polish carry no story label
- Every task names the exact file path it touches

## Path conventions

All paths are relative to `/Users/krzysztof.wiatrzyk/private/monorepo/medigo`.

## Read before writing a single line of this phase

1. **[research D-23](./research.md#d-23) — a restore destroys the audit rows that describe it.**
   `core/backup_restore.go:30-46` replaces the whole of `pb_data`. Anything written before a restore
   is gone after it. The journal is the answer and it is not optional.
2. **[research D-24](./research.md#d-24) — `,unset` × `execve`.** The environment snapshot in
   `main()` must be the **first statement**, before `config.Load()`. Get the order wrong and the
   instance comes back from disaster recovery with no secrets.
3. **[phase 005 D-01](../005-sharing-and-collaboration/research.md#d-01) — PocketBase has no NULL.**
   Every predicate is `= ''`. `task lint:isnull` is extended to this phase's store packages by T005.
4. **[research D-44](./research.md#d-44) — one resolver.** The count the builder shows and the
   records the document contains come from the same object, or SC-002 is false.

---

## Phase 1: Setup

**Purpose**: settle the one open technical choice, wire the linters and the four new CI suites, and
put the fixtures in place. No domain logic here.

- [ ] T001 Verify the toolchain precondition and record it in `specs/006-reporting-and-operations/quickstart.md` §0: `go.mod` declares `go 1.27` with a `toolchain go1.27.x` line, and `GOTOOLCHAIN` is unset in `Taskfile.yaml` and every file under `.github/workflows/`
- [ ] T002 **Gating spike** — build a throwaway `internal/render/pdf/spike/main.go` that produces a two-page PDF with `AddUTF8FontFromBytes`, `SetHeaderFunc`, `SetFooterFunc`, `AliasNbPages`, `Line`, `Rect`, `SetDashPattern` and `RegisterImageOptionsReader` under `CGO_ENABLED=0`, assert the output opens, then record the result in `specs/006-reporting-and-operations/research.md` D-01. If any symbol is missing, switch the pin to `github.com/signintech/gopdf` and change **only** `internal/render/pdf` (blocks T112)
- [ ] T003 Pin `github.com/go-pdf/fpdf` v0.9.0 and promote `golang.org/x/text` v0.30.0 to a direct dependency in `go.mod`, then `task tidy` and confirm the module graph still builds `CGO_ENABLED=0` for `linux/amd64` and `linux/arm64` (depends on T002)
- [ ] T004 [P] Add `internal/render/pdf`, `internal/render/archive` and `internal/platform/backup` to the `depguard` allowlists in `.golangci.yml` — the two render packages may import `internal/web/api` for the DTOs and must **not** import PocketBase; the backup package is `[PB]` — and confirm the existing `**/internal/service/**` and `**/internal/domain/**` deny globs already cover this phase's new service and domain packages
- [ ] T005 [P] Extend `task lint:isnull` in `Taskfile.yaml` to scan `internal/store/reporttemplate/`, `internal/store/exportjob/`, `internal/store/audit/` and `internal/store/stats/` for the literal `IS NULL`, failing with a message pointing at phase 005 research D-01
- [ ] T006 [P] Add `task lint:noconvert` to `Taskfile.yaml` — greps the whole tree for a unit-conversion function (`func .*Convert.*Unit`, `mgdlToMmol`, `mmolToMgdl` and the like) and fails on sight, with the message pointing at FR-018 and research D-39
- [ ] T007 [P] Write failing tests in `internal/config/config_test.go` for the eleven new `MEDIGO_*` values of `data-model.md` §7: every default, every bound, and a **boot failure naming the variable and the bound** for each out-of-range value, plus the rule that `MEDIGO_STATE_DIR` must not be inside `MEDIGO_DATA_DIR` (FR-113)
- [ ] T008 Add `ReportConfig`, `ExportConfig`, `BackupConfig` and `StateDir` to `internal/config/config.go` with the defaults and bounds of `data-model.md` §7, and a `Validate()` that reports **every** violation at once (depends on T007)
- [ ] T009 [P] Add `task test:scale`, `task test:slowsse`, `task spike:pdf` and `task purge` wrappers to `Taskfile.yaml`, and extend `task test:phileak` and `task test:netgate` — both already created by earlier phases (001 T006, 002 T159a) — to the whole-application exercise
- [ ] T010 [P] Add four CI jobs in `.github/workflows/` — `phileak` and `netgate` on every pull request, `scale` on merge to `main`, `slowsse` on merge to `main` and nightly — none of them setting `GOTOOLCHAIN`
- [ ] T011 [P] Vendor the Noto Sans, Noto Sans Arabic and Noto Sans Hebrew faces into `internal/render/pdf/fonts/` with `embed.go`, record their licences in `internal/render/pdf/fonts/LICENSE`, and assert the added binary size is under 2 MB in `internal/render/pdf/fonts/embed_test.go` (research D-04)
- [ ] T012 Extend `internal/cli/seed.go` with the full cast of `data-model.md` §9 — six accounts including `empty@`, `disabled@` and `mustchange@`; three saved reports including one over a person with no records and one whose patient is emptied; five jobs in five states; the two-unit lab series; the non-Latin person; two archives, one without `medigo.json`; a trail carrying every action including `system` and `superuser` rows — then run `task fixture:regen` to rewrite `internal/testdata/pb_data`
- [ ] T013 [P] Extend `internal/testsupport/scale/generate.go` with the documented volumes of `data-model.md` §10 (10,000 records, 2,000 documents, 1,000,000 activity entries, 500 readings, 200 jobs, 500 accounts, 60 archives), behind the `scale` build tag

**Checkpoint**: the PDF decision is settled, the linters and CI suites exist, configuration refuses a
value it cannot honour, and the fixtures are in place. No production behaviour has changed.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the two collections, the domain, the ports and their contract suites, the job envelope
and the route registry. **No user story may start until this phase is complete** — every story reads
a job row, a figure or a trail entry.

### The domain (no PocketBase, no HTTP, no templ, no fpdf)

- [ ] T014 [P] Write failing table-driven tests in `internal/domain/retention/window_test.go` for `DueAt`, `Expired` and `DaysRemaining`: whole days from the recorded moment; a clock moved **backwards** removes nothing early; a clock jumped **forwards** removes nothing that has not elapsed; a DST transition and a leap day change no outcome; `DaysRemaining` floors at 0 (FR-053, FR-061, US8 AS-9)
- [ ] T015 Implement `internal/domain/retention/window.go` — the one helper every purge, label and window statement in the application uses (depends on T014, research D-50)
- [ ] T016 [P] Write failing tests in `internal/domain/report/criteria_test.go` for `Criteria.Validate()`: at least one kind, every kind registered, no duplicates, `from <= to`, statuses legal for at least one selected kind, and **every** violation reported together in one `*domain.ValidationError` (FR-002, data-model §1.2)
- [ ] T017 [P] Write failing tests in `internal/domain/report/chart_test.go` for `ChartSelection.Validate()`: `source ∈ {vitals, lab}`; `vitals` requires a known metric and no canonical name; `lab` requires a canonical name and no metric; **`unit` is always required**; the chart count ceiling; and a case asserting **no conversion path exists** by construction (FR-016, FR-018, FR-023, data-model §1.3)
- [ ] T018 [P] Write failing tests in `internal/domain/report/settings_test.go` for `Settings.Validate()` and its defaults, including that `include_header: false` still leaves the person identifiable by an opaque reference (FR-002, US1 AS-8)
- [ ] T019 Implement `internal/domain/report/{definition.go,criteria.go,chart.go,settings.go,errors.go}` — the entities, `ErrNothingMatched`, `ErrTooManyRecords`, `ErrTooManyCharts`, `ErrPatientUnreachable` (depends on T016, T017, T018)
- [ ] T020 [P] Write failing **exhaustive** tests in `internal/domain/exportjob/transition_test.go` enumerating every `(Status, Event)` pair of `data-model.md` §2.2: the seven legal edges succeed, every other pair returns `*exportjob.TransitionError` carrying the state the row is actually in, **no edge returns to `queued`**, and `succeeded → expired` is the only edge out of a terminal state
- [ ] T021 [P] Write failing tests in `internal/domain/exportjob/job_test.go` for the invariants of `data-model.md` §2.3: `kind=report` implies a patient; `format` matches `kind`; `progress ∈ [0,100]`; `error_code` is drawn from the bounded set; `artifact` is empty unless `status = succeeded`
- [ ] T022 Implement `internal/domain/exportjob/{job.go,status.go,stage.go,scope.go,options.go,transition.go,errors.go}` (depends on T020, T021)
- [ ] T023 [P] Write failing tests in `internal/domain/adminuser/rules_test.go` — exhaustive over the actor/target/tier matrix: an actor may not change **their own** role or disable themselves; the **last enabled `admin`** may not be demoted or disabled **by anybody**, including a second administrator; a demotion that would leave one other enabled administrator is allowed; each refusal carries the reason (FR-095, FR-096, US5 AS-12, AS-13)
- [ ] T024 Implement `internal/domain/adminuser/rules.go` as pure functions, so `medigo seed`, a future CLI subcommand and a test fixture are all bound by them (depends on T023, research D-19)
- [ ] T025 [P] Write failing tests in `internal/domain/audit/query_test.go` for the reader's typed narrowing: unknown action, unknown target kind and `from > to` are rejected before any query is built, and the type **has no free-text field**, asserted by reflection (FR-065, FR-068)
- [ ] T026 [P] Implement `internal/domain/audit/{query.go,vocab.go}` — the typed narrowing plus the ten new actions and the `report_template` target kind of `data-model.md` §4 (depends on T025)
- [ ] T027 [P] Write failing tests in `internal/domain/report/redaction_test.go` and `internal/domain/exportjob/redaction_test.go`: `MarshalZerologObject` on every new domain type emits **only** ids, enum values and counts, and a rendered log line contains none of a seeded template name, criteria, tag name, patient name, file name or archive path (FR-117)

### The collections

- [ ] T028 [P] Write failing migration tests in `internal/store/migrations/report_templates_test.go`: all five API rules `nil`; the three indexes of `data-model.md` §1.1 exist; **`idx_report_templates_name` is unique and case-insensitive** — a second row differing only in capitalisation is rejected; deleting the owning `users` row cascades; **deleting the referenced `patients` row empties `patient` and leaves the template**; the `down` drops the collection
- [ ] T029 Implement `internal/store/migrations/1757xxx100_report_templates.go` per `data-model.md` §1 and §6, with a real `down` (depends on T028)
- [ ] T030 [P] Write failing migration tests in `internal/store/migrations/export_jobs_test.go`: all five rules `nil`; the three indexes of §2.1; `artifact` is a `FileField` with **`Protected: true`**; deleting the owning `users` row deletes the job **and its stored blob**, asserted against `app.NewFilesystem()`; deleting a `report_templates` row **empties** `template` rather than deleting the job; the `down` drops the collection
- [ ] T031 Implement `internal/store/migrations/1757xxx200_export_jobs.go` per `data-model.md` §2 and §6, documenting in the file that the `down` destroys any artifact still stored (depends on T029, T030)
- [ ] T032 [P] Implement `internal/store/migrations/1757xxx300_users_must_change_password.go` (bool, default false) with a reversing `down`, and a test asserting an existing account is unaffected by the migration
- [ ] T032a [P] Write failing tests in `internal/store/migrations/audit_vocab_ops_test.go`: `audit_events` has an `affected` column (number, optional, integer ≥ 0) after `up` and does not after `down`; and the **complete** expected vocabulary after this phase — **thirty-six** actions and **twenty-eight** target kinds, set-equal, not a delta — extending the shared vocabulary test from phase 001 (T070a). Also assert `audit.Reason.Valid()` accepts every bounded token this phase writes, including every `error_code` the job envelope and the export worker produce, so a `job_failed` row cannot be refused by its own writer (ANALYSIS C1, C2)
- [ ] T033 [P] Implement `internal/store/migrations/1757xxx400_audit_vocab_ops.go` adding the `affected` column (`data-model.md` §4.0), extending `audit_events.action` by the ten values and `target_kind` by `report_template`, with a `down` that documents the caveat in `data-model.md` §6 (depends on T032a)
- [ ] T034 [P] Write tests in `internal/store/audit/reader_plan_test.go` asserting the reader's four narrowings each use one of the four audit indexes, by `EXPLAIN QUERY PLAN` rather than by the index merely existing. **There is no migration in this task**: `data-model.md` §4.3 and `001/data-model.md` §Indexes both state that this phase creates no audit index — the four are created wide enough by phases 001 and 002, and a fifth under a name 001 already holds fails `CREATE INDEX` at first boot (ANALYSIS N1)
- [ ] T035 Extend `internal/store/migrations/assertions.go` and `assertions_test.go` so boot refuses to start when either new collection has a non-nil API rule, and so the file-field assertion names **exactly three** fields — `patients.photo`, `attachments.file`, `export_jobs.artifact` — all `Protected: true`, failing on a fourth of any kind (depends on T031)
- [ ] T036 [P] Write failing `tests.ApiScenario` cases in `internal/store/migrations/lockdown_test.go` proving `/api/collections/report_templates/records` and `/api/collections/export_jobs/records` return `404` to a normal authenticated user, for all five verbs

### The ports, the fakes and the contract suites

- [ ] T037 Declare the consumer-side ports: `report.Renderer` and `report.Repository` in `internal/service/report/ports.go`; `exportjob.Archiver`, `exportjob.Repository`, `exportjob.Clock` in `internal/service/exportjob/ports.go`; `admin.Counter`, `admin.Storage`, `admin.Posture`, `admin.AccountAdmin`, `admin.Archives` in `internal/service/admin/ports.go`; `audit.Reader`, `audit.Retention` in `internal/service/audit/ports.go` — every one consumer-declared, none an omnibus interface, and `admin.Archives`' seven methods justified in a doc comment citing research D-21
- [ ] T038 [P] Implement in-memory fakes for every port above in `internal/service/report/reporttest/fake.go`, `internal/service/exportjob/exporttest/fake.go`, `internal/service/admin/admintest/fake.go` and `internal/service/audit/audittest/fake.go`, each with an injectable clock so retention and expiry are tested by moving time, never by sleeping
- [ ] T039 [P] Implement `internal/service/report/reporttest/contract.go` — `RendererContract` as a `testify/suite` parameterised by a factory, run against the real PDF renderer and the fake: a document with zero sections still renders; every selected kind produces a section; page count grows with content; the renderer **never** performs I/O beyond its `io.Writer`
- [ ] T040 [P] Implement `internal/service/exportjob/exporttest/contract.go` — `ArchiverContract` run against the real archiver and the fake: an empty account still yields a manifest and empty arrays; every declared file appears in the manifest and every manifest entry appears in the archive; a cancelled write leaves nothing readable
- [ ] T041 [P] Implement `internal/service/report/reporttest/repository_contract.go` — `RepositoryContract` for `report_templates` run against the real repository and the fake: not-found; case-insensitive name conflict; cursor stability under a concurrent insert; cascade on owner delete; **emptied** patient on patient delete
- [ ] T042 [P] Implement `internal/service/audit/audittest/contract.go` — `ReaderContract` run against the real reader and the fake: newest-first ordering; every narrowing singly and combined; the scoped reader's constraint; a cursor walk that repeats 0 and skips 0 under a concurrent writer; and **no query against any record collection**
- [ ] T043 [P] Implement `internal/service/admin/admintest/contract.go` — `ArchivesContract` run against the PocketBase-backed adapter and the fake: list, create, upload, preview, download, delete, and the `StoreKeyActiveBackup` refusal — so the restore preconditions are testable without taking a real backup in a unit test

### The stores

- [ ] T044 [P] Write failing integration tests in `internal/store/reporttemplate/repo_test.go` running `RepositoryContract` against a real `tests.NewTestApp`
- [ ] T045 Implement `internal/store/reporttemplate/{repo.go,mapper.go}` — the `= ''` predicates written once, the JSON columns validated on read as well as write (depends on T044)
- [ ] T046 [P] Write failing integration tests in `internal/store/exportjob/repo_test.go`: the queue query returns the **oldest** `queued` row by `(created asc, id asc)`; the position `COUNT` matches the row order exactly; the conditional cancel affects one row and then zero; a `succeeded` row's artifact is readable through `app.NewFilesystem()`
- [ ] T047 Implement `internal/store/exportjob/{repo.go,mapper.go,queue.go}` (depends on T046)
- [ ] T048 [P] Write failing integration tests in `internal/store/audit/reader_test.go` running `ReaderContract` against a real test app, plus an `EXPLAIN QUERY PLAN` assertion that each narrowing uses one of the four indexes
- [ ] T049 Implement `internal/store/audit/{reader.go,cursor.go}` — keyset over `(occurred_at DESC, id DESC)`, HMAC-signed, never an offset (depends on T048, research D-52)
- [ ] T050 [P] Write failing integration tests in `internal/store/stats/counter_test.go`: every count of `contracts/admin-instance.md` op 80 — accounts, people, records of each kind, stored documents, live sharing arrangements and outstanding invitations (FR-079) — is correct on a populated fixture **and zero on an empty instance**, and each is a single indexed query
- [ ] T051 Implement `internal/store/stats/counter.go` behind `admin.Counter` — the six count families of FR-079 (depends on T050)

### The job envelope, the boot additions and the registry

- [ ] T052 [P] Write failing tests in `internal/platform/pb/jobs_test.go` for the envelope of research D-43: a successful job writes **exactly one** `job_succeeded` row carrying its `affected` count; a failing job writes exactly one `job_failed` with a bounded `error_code`; a **panicking** job is recovered and reported as `job_failed`; the envelope **never retries**; `target_id` is the bounded job name — up to 29 characters, which the `≤64` column of 001 [data-model](../001-walking-skeleton/data-model.md) §3 accepts — and no row carries content. **Plus the run-id case**: the envelope mints one `run_id` per run onto the `ctx` it hands the job body, its own row's `request_id` is that value and never empty, a row the **body** writes on that ctx carries the same value, two runs of the same job differ, and the value equals the one on the run's log lines — so a `Required` column has a source in a context with no HTTP request (001 [data-model](../001-walking-skeleton/data-model.md) §3, 001 T240)
- [ ] T053 Implement `internal/platform/pb/jobs.go` — `Run(app, name, fn)`, minting the run's `run_id` from the same helper `internal/obs` uses for request ids and carrying it on the `ctx` passed to `fn` so the body's rows correlate too — and wire it into `internal/platform/pb/cron.go` for the three new jobs **and** for phase 004's trash purge and orphan sweep and phase 005's tidy, without editing any job body (depends on T052)
- [ ] T054 [P] Write failing tests in `internal/platform/pb/posture_test.go`: `superuser_mfa` reads the superusers collection's `MFA.Enabled` and reports `partial` when `MFA.Rule` is non-empty; `superuser_ip_allowlist` reads `len(Settings().SuperuserIPs)`; `smtp` reads `Settings().SMTP.Enabled`; `oauth2` reads whether any provider in `Settings().OAuth2.Providers` is enabled and **never reads a client id or a secret**; migration state compares `_migrations` to the registered list
- [ ] T055 Implement `internal/platform/pb/posture.go` behind `admin.Posture` (depends on T054, research D-17)
- [ ] T056 [P] Write failing tests in `internal/platform/pb/storage_test.go`: the database figure sums `data.db`, `-wal` and `auxiliary.db`; the document figure walks `<DataDir>/storage`; both are served from the gauge with their `computed_at`; and **neither runs on a request path**, asserted by timing the handler with a stubbed gauge
- [ ] T057 Implement `internal/platform/pb/storage.go` and register `medigo_storage_refresh` (every 15 minutes plus once at boot) through the envelope (depends on T053, T056, research D-16)
- [ ] T058 [P] Write failing tests in `internal/platform/pb/boot_test.go` for `medigo.json`: it is written at every boot into `<DataDir>`, carries `app`, `app_version`, `schema_version` and `written_at`, and rides inside an archive taken immediately afterwards (research D-25)
- [ ] T059 Implement the `medigo.json` writer in `internal/platform/pb/boot.go` (depends on T058)
- [ ] T060 Add the 25 API routes — `listAuditEvents`, `getReportSummary`, `getReportTrends`, `listReportTemplates`, `createReportTemplate`, `getReportTemplate`, `updateReportTemplate`, `deleteReportTemplate`, `createExport`, `listExports`, `getExport`, `downloadExport`, `cancelExport`, `getAdminStats`, `getAdminSystem`, `listAdminUsers`, `updateAdminUser`, `listBackups`, `createBackup`, `uploadBackup`, `getBackupPreview`, `downloadBackup`, `restoreBackup`, `deleteBackup` and op 4, `signInWithOAuth2` — the one **public** route this phase adds — plus 7 page routes and 3 page-action routes to `internal/httproute/routes.go`, each under exactly that `operationId` so the Principle IX gate finds the same twenty-five names in the registry and in `api/openapi.json` (page actions excluded from OpenAPI), its landmark, its `SmokeURL` with seeded ids substituted, `PatientScoped: true` where it applies, and `AdminOnly: true` on the eleven operator routes
- [ ] T061 [P] Write failing `encoding/json/v2` round-trip tests in `internal/web/api/dto_reporting_test.go` for every DTO in `contracts/`: slices marshal as `[]` never `null`, unknown fields are rejected, duplicate keys are rejected — **plus two reflection tests that fail the build**: `AuditEntry` has no field capable of carrying free text outside the enumerated vocabularies, so an entry cannot carry content (FR-068, FR-114), and neither admin DTO has a free-text field outside `State`/`Version` (FR-086)
- [ ] T062 Implement `internal/web/api/dto_reporting.go` (depends on T061)
- [ ] T063 Add `artifact_expired`, `not_downloadable`, `not_cancellable`, `job_in_progress`, `archive_operation_in_progress`, `duplicate_name`, `patient_unreachable`, `archive_unreadable`, `archive_version_unsupported`, `safety_backup_failed`, `password_change_required`, `account_disabled` and `restore_in_progress` to the error-mapping table in `internal/web/errors.go`, with a test asserting **no message produced by this phase names a storage location, a file name, a record value or a person's name** (FR-118)

**Checkpoint**: both collections exist, the domain is exhaustively tested, every port has a fake and a
contract suite, every scheduled job reports itself, and the registry knows about every new route.
**User stories may now start, and may run in parallel.**

---

## Phase 3: User Story 1 — Walk into a visit with the right paper (Priority: P1) 🎯 MVP

**Goal**: a person turns a maintained history into one document they can hand to a consultant.

**Independent Test**: sign in as the seeded account, open the builder for the seeded person, confirm
the per-kind counts match what the application holds, select four kinds and a twelve-month range,
confirm the count shown before asking matches what comes out, produce the report, download it, and
confirm it contains exactly those records, identifies the person and the criteria on its first page,
and contains nothing belonging to any other person.

### Tests for User Story 1 ⚠️ write first, confirm red

- [ ] T064 [P] [US1] Failing unit tests in `internal/service/report/selection_test.go` for the **one resolver** (research D-44): `Counts()` and `Each()` agree per kind and in total over a table of selections including empty ones, a selection with every kind, and one narrowed to a single tag (FR-003, SC-002)
- [ ] T065 [P] [US1] Failing HTTP tests in `internal/web/api/reports_http_test.go` for `GET /api/v1/reports/summary` (op 69): every registered kind is present **including those at zero**; an owner's own read writes **no** audit row and a grantee's or a break-glass reader's writes exactly one, per `contracts/reports.md` and phase 005's single statement of the rule (FR-115); a missing `patient` is `400 patient_required`; a person the caller cannot reach is `404` byte-identical to a non-existent id **and writes one `access_denied` row**; an unknown tag id is `422 unknown_tag` identical for "not yours" and "does not exist" (US1 AS-1, AS-2, AS-9, FR-001, FR-003)
- [ ] T066 [P] [US1] Failing HTTP tests in `internal/web/api/exports_http_test.go` for `POST /api/v1/exports` (op 76) — **discharges SC-003**: `202` in under 2 s with a `Location` and a queued job, for **both** `kind=report` and `kind=data_export`, so the acknowledgement budget is proven for every request the operation accepts; the requester is never held waiting; op 78 reports `stage` and `progress` while it runs; and the job stays readable and downloadable by that account after the session has navigated away and come back (FR-044, SC-003); a selection matching nothing is `422 nothing_matched` **with the selection left intact**; a selection over `MEDIGO_REPORT_MAX_RECORDS` is `422 too_many_records` **stating the limit, before anything is produced** (US1 AS-4, AS-5, AS-14, FR-004, FR-005, FR-010)
- [ ] T067 [P] [US1] Failing HTTP tests in `internal/web/api/exports_http_test.go` for `GET /api/v1/exports/{id}/download` (op 79) — the full actor matrix of `contracts/README.md`, re-authorized on **every** request so the address alone gives nothing (FR-048), including **an administrator downloading another account's document is `404`**, an unauthenticated request is `401`, and every refusal writes one `access_denied` row (US1 AS-12, FR-013, FR-048, SC-009)
- [ ] T068 [P] [US1] Failing tests in `internal/service/exportjob/runner_test.go` for the worker's authorization: authorization is resolved **at dequeue**, not at request; a person whose access was withdrawn in between is dropped and named as withdrawn; a disabled owner fails the job with `owner_unavailable` (US1 AS-9, FR-011, research D-09)
- [ ] T069 [P] [US1] Failing artifact tests in `internal/render/pdf/renderer_test.go` — **re-open the produced PDF and assert on it** — **discharges SC-004**: the first page identifies the person, states the production moment and states the criteria **in prose**; every page carries the identity or the opaque reference **and `Page N of M`**; the section order is the documented one; a selected kind that matched nothing carries the sentence "No records of this kind matched the selection" rather than being omitted (US1 AS-6, AS-7, FR-006, FR-007, FR-008, research D-37)
- [ ] T070 [P] [US1] Failing tests in `internal/render/pdf/renderer_test.go` for the presentation settings: `sort ∈ {date_desc, date_asc, name_asc}` and `group ∈ {none, kind, year}` each change the document as specified, and both round-trip through a saved report (FR-009)
- [ ] T071 [P] [US1] Failing tests in `internal/render/pdf/unrenderable_test.go`: a person whose name, notes or tags carry Arabic, Hebrew, CJK and `<script>` text produces a document whose **first page counts and states** the characters that could not be rendered faithfully, nothing is silently dropped or substituted, and `<script>` is drawn as literal characters (US1 AS-15, FR-006, research D-04)
- [ ] T072 [P] [US1] Failing tests in `internal/render/pdf/renderer_test.go` for the identity options: with the header and photograph excluded, neither appears **and the person is still identifiable by the opaque reference the report carries** (US1 AS-8, FR-002)
- [ ] T073 [P] [US1] Failing tests in `internal/service/exportjob/runner_test.go` for isolation: with two people under one account, a produced document concerns exactly one and contains **no** record of the other, asserted by scanning the produced bytes for the other person's seeded sentinel (US1 AS-10, FR-011, SC-002)
- [ ] T074 [P] [US1] Failing tests in `internal/service/exportjob/purge_test.go`: past its window an artifact's content is **no longer stored** (asserted against the filesystem, not the row), the request reads `expired` with the window that applied, download is `410 artifact_expired` with a plain statement, and re-production is offered (US1 AS-11, FR-012, FR-047, research D-42)
- [ ] T075 [P] [US1] Failing tests in `internal/service/audit/writer_test.go` and `internal/web/api/exports_http_test.go`: producing writes one `export` row and downloading writes one `export_download` row (FR-048), each naming who, what, which person and when — and **neither carries a record value, a name of anything recorded, or a file name**, asserted against seeded sentinels (US1 AS-13, FR-014, SC-014)
- [ ] T076 [P] [US1] Failing tests in `internal/service/exportjob/runner_test.go` for FR-015: producing the same definition twice yields **two independent rows**, each downloadable, each with its own production moment
- [ ] T077 [P] [US1] Failing templ render tests in `internal/web/views/reports/builder_templ_test.go`: the builder, the per-kind counts and the **empty state** each render inside `region[name="Reports"]`, and a person with nothing recorded shows every kind at zero with an explanation rather than an empty list (US1 AS-1, FR-125)

### Implementation for User Story 1

- [ ] T078 [US1] Implement `internal/service/report/selection.go` — `Counts()` and `Each()` over one query builder and one authorization sequence (depends on T064)
- [ ] T079 [US1] Implement `internal/service/report/service.go` and `internal/web/api/reports.go` — op 69 per `contracts/reports.md` (depends on T065, T078)
- [ ] T080 [US1] Implement `internal/service/report/document.go` — building the `report.Document` the renderer draws: already resolved, already authorized, already ordered, carrying no repository and no context (depends on T078)
- [ ] T081 [US1] Implement `internal/render/pdf/{renderer.go,layout.go,cover.go,section.go}` — the header/footer callbacks, `AliasNbPages`, the cover, the contents and the sections (depends on T003, T069, T070, T072)
- [ ] T082 [US1] Implement `internal/render/pdf/{fontrun.go,bidi.go,unrenderable.go}` — the coverage chain, the `golang.org/x/text/unicode/bidi` reordering, the U+FFFD substitution and the counted first-page statement, plus `MEDIGO_REPORT_EXTRA_FONT_DIR` loading at boot (depends on T011, T071)
- [ ] T083 [US1] Implement `internal/service/exportjob/{runner.go,queue.go}` — one worker started from `OnServe` and stopped from `OnTerminate`, taking the oldest queued row, re-authorizing at dequeue (depends on T047, T068, T073)
- [ ] T084 [US1] Implement `internal/web/api/exports.go` ops 76, 77, 78 and 79 per `contracts/exports.md`, streaming the artifact through `app.NewFilesystem()` + `fsys.Serve` with **no file token** (depends on T066, T067, T083)
- [ ] T085 [US1] Implement `internal/service/exportjob/purge.go` and register `medigo_purge_artifacts` through the envelope (depends on T053, T074)
- [ ] T086 [US1] Implement `internal/web/page/reports.go` and `internal/web/page/fragments_reporting.go` (the `/reports/selection` and `/reports/jobs` page actions), plus the templ components in `internal/web/views/reports/{builder.templ,counts.templ,produced.templ}` and the ids in `internal/web/views/ids/ids.go` (depends on T077, T079)
- [ ] T087 [P] [US1] Add `e2e/specs/reports.spec.ts` — `/reports` at 1440×900 and 390×844, populated **and** as `empty@medigo.local`, asserting the landmarks, `body[data-signals]`, zero console/page/network errors, the count updating as the selection changes, and the two page-action routes being exercised

**Checkpoint**: a person can build a selection, see exactly what it will contain, produce a document
and hand it to a clinician. This is the MVP and it is demonstrable on its own.

---

## Phase 4: User Story 2 — Take everything with me (Priority: P2)

**Goal**: an archive that proves leaving is possible, readable with nothing of MediGo installed.

**Independent Test**: seed an account with several people, records across every kind, and attached
documents. Ask for a complete export. Confirm it is acknowledged immediately, reports progress and
completes. Download the archive, open it with the application stopped, and confirm the manifest
describes every file, every record and document is present and readable, the format version is
stated, and nothing belonging to any other account appears anywhere in it.

### Tests for User Story 2 ⚠️ write first, confirm red

- [ ] T088 [P] [US2] Failing tests in `internal/render/archive/writer_test.go` running `ArchiverContract`, plus: an account holding **nothing** still yields an archive with a manifest and empty arrays, so holding nothing is provable rather than indistinguishable from failure (US2 AS-1)
- [ ] T089 [P] [US2] Failing tests in `internal/render/archive/manifest_test.go`: the manifest states the format version, the moment of production, the account, the people, the kinds and a count each, whether documents were included, the mapping from every archive path to its attachment and record, the withdrawn people, and **the meaning of every other file present** (US2 AS-3, FR-038)
- [ ] T090 [P] [US2] Failing tests in `internal/render/archive/writer_test.go` for contents: every record of every selected kind is present in structured form; `tables/<kind>.csv` is present **only** when tabular files were asked for; every included document appears under `<attachment_id>__<original name>` with path separators and control characters replaced and **nothing else altered** (US2 AS-4, FR-037, FR-040)
- [ ] T091 [P] [US2] Failing tests in `internal/render/archive/writer_test.go` for narrowing: one person, a subset of kinds and a date range produce exactly that, **and the manifest says so**; excluding documents produces a smaller archive whose manifest states the exclusion (US2 AS-5, AS-6, FR-036)
- [ ] T092 [P] [US2] Failing tests in `internal/service/exportjob/export_test.go` for withdrawal: a person whose access ends between request and production is **absent from the archive and named in the manifest as withdrawn** (US2 AS-7, FR-011, research D-09)
- [ ] T093 [P] [US2] Failing **byte-scanning** tests in `internal/render/archive/secrets_test.go` (SC-006, FR-043): a produced archive over the seeded fixture contains none of the seeded passwords, any `tokenKey`, any `MEDIGO_`-prefixed string, the SMTP password, the Sentry DSN, or any id belonging to an account the exporter cannot reach — **plus** a reflection test over every DTO reachable from the exporter asserting no field name matches `(?i)(password|secret|token|key|dsn)` (research D-30)
- [ ] T094 [P] [US2] Failing tests in `internal/render/archive/writer_test.go` for the audit slice: `data/audit_events.json` carries the entries concerning the exported people plus the exporter's own, every other actor as an **opaque id only**, and **no `ip` field at all** — there is none to omit, the column does not exist (FR-041, FR-042, research D-51)
- [ ] T095 [P] [US2] Failing tests in `internal/service/exportjob/queue_test.go`: a second request while one runs is **accepted**, shows its position, and begins when the first finishes; at most one job runs at a time on the instance (US2 AS-10, FR-045)
- [ ] T096 [P] [US2] Failing tests in `internal/service/exportjob/cancel_test.go` (research D-41): cancelling a `queued` job is a conditional write; cancelling a `running` job sets the flag and the worker abandons the scratch file between records; **nothing partial is downloadable** either way; cancelling a terminal job is `409 not_cancellable` naming the state; each writes exactly one `job_cancelled` row (US2 AS-11, FR-046)
- [ ] T097 [P] [US2] Failing tests in `internal/service/exportjob/reconcile_test.go`: a `running` row at `OnServe` becomes `failed` with `error_code: "interrupted"`, `finished_at` set and `artifact` cleared, is offered for retry, and **is never left reporting itself as running**; the reconciliation runs **before** the worker starts (US2 AS-9, FR-049, research D-06)
- [ ] T098 [P] [US2] Failing tests in `internal/service/exportjob/export_test.go` for storage failure: a write that cannot be accepted fails with `error_code: "storage_full"`, the message **names no storage location**, nothing partial is downloadable, and the failure appears both to the account holder and on the operator overview (US2 AS-12, FR-050, FR-085, FR-118)
- [ ] T099 [P] [US2] Failing tests in `internal/store/exportjob/repo_test.go` for account deletion: deleting an account destroys its requests **and its stored artifacts**, asserted against `app.NewFilesystem()`, immediately rather than at the window (US2 AS-14, FR-051, FR-063)
- [ ] T100 [P] [US2] Failing **memory-bounded** tests in `internal/render/archive/writer_scale_test.go` (`//go:build scale`): a complete export of 10,000 records and 2,000 documents completes within **300 s** and **256 MiB RSS**, and the first archive bytes are written before the last record is read (SC-005, FR-123)
- [ ] T101 [P] [US2] Failing tests in `internal/render/archive/format_test.go` — the build gate: every top-level key `docs/export-format-v1.md` describes is produced, and every key produced is described (FR-039)
- [ ] T102 [P] [US2] Failing templ render tests in `internal/web/views/exports/exports_templ_test.go`: the list, a queued row with its position, a running row with its stage and progress, a finished row with its size, an expired row and the **empty state** each render inside `region[name="Exports"]` (FR-044)

### Implementation for User Story 2

- [ ] T103 [US2] Implement `internal/render/archive/{writer.go,manifest.go,csv.go,documents.go}` — `archive/zip` onto the scratch file under `.pb_temp_to_delete`, `jsontext.Encoder` one record at a time, `encoding/csv` for the tabular files, documents copied with `io.Copy` (depends on T088–T091, T094, T100)
- [ ] T104 [US2] Implement `internal/service/exportjob/export.go` — scope resolution, per-person re-authorization at dequeue, the withdrawn list, progress and stage updates (depends on T083, T092, T098)
- [ ] T105 [US2] Implement `internal/service/exportjob/cancel.go` and `internal/web/api/exports.go` op 91 per `contracts/exports.md` (depends on T096)
- [ ] T106 [US2] Implement `internal/service/exportjob/reconcile.go` and call it from `OnServe` **before** the worker starts (depends on T097)
- [ ] T107 [US2] Implement `internal/web/page/exports.go`, the `/exports/jobs` page action and the templ components in `internal/web/views/exports/{list.templ,row.templ,request.templ}` (depends on T102)
- [ ] T108 [P] [US2] Write `docs/export-format-v1.md` — the published, versioned format of research D-29, readable without the application (depends on T101)
- [ ] T109 [P] [US2] Add `e2e/specs/exports.spec.ts` — `/exports` at both viewports, populated **and** empty, plus a request-cancel-and-re-run walk and an assertion that polling stops when no job is active

**Checkpoint**: an account can take everything it holds, read it on another machine, and prove nothing
of anybody else's is in it.

---

## Phase 5: User Story 3 — Ask the same question again next time (Priority: P3)

**Goal**: a definition that outlives the moment it was written, so a recurring visit is one
click rather than fifteen.

**Independent Test**: build a selection, save it with a name, sign out and back in, open it, confirm
every choice is as it was, produce from it, then change it and confirm the change affects only the
next document and not the one already produced.

### Tests for User Story 3 ⚠️ write first, confirm red

- [ ] T110 [P] [US3] Failing HTTP tests in `internal/web/api/report_templates_http_test.go` for ops 71 and 73: the list is the caller's own only, paged by the shared cursor envelope, sorted from the allowlist; another account's saved report is `404` **byte-identical** to a non-existent id and writes one `access_denied`; every response carries an `ETag` (US3 AS-10, FR-025, FR-031)
- [ ] T111 [P] [US3] Failing HTTP tests in `internal/web/api/report_templates_http_test.go` for op 72: `201` with `Location`; every choice — person, kinds, range, tags, charts, presentation settings — round-trips **exactly**; a name colliding case-insensitively with the caller's own is `409 duplicate_name` **naming the existing one and offering to replace it**; the identical name under a **different account** is accepted (US3 AS-2, AS-3, AS-8, FR-026, FR-027, research D-45)
- [ ] T112 [P] [US3] Failing HTTP tests for ops 74 and 75: `PATCH` and `DELETE` **require** `If-Match`; **every part is editable — name, description, criteria, charts and presentation settings — and the change takes effect the next time the saved report is used**, asserted by producing from it afterwards; a stale `If-Match` is `412 version_mismatch` and changes nothing; deletion is `204` and **destroys no document already produced from it**, asserted by downloading one afterwards (US3 AS-6, AS-7, AS-9, FR-028, FR-029, FR-030, FR-034)
- [ ] T113 [P] [US3] Failing tests in `internal/service/report/template_test.go` for FR-033 and US3 AS-12: producing from a saved report with an overridden date range produces the override **and leaves the saved definition untouched**, verified by re-reading it
- [ ] T114 [P] [US3] Failing tests in `internal/service/report/template_test.go` for FR-004, FR-026 and the zero-resolution case of US3 AS-3: a saved report whose criteria now match nothing is **still openable and still editable** and reports a current count of zero, and only the act of producing is refused, with an explanation of which criterion matched nothing (it cited FR-035 and SC-007 and tested neither; those are T118a and T114a — ANALYSIS N1)
- [ ] T114a [P] [US3] Failing resolver tests in `internal/service/report/template_test.go` — **the task that discharges SC-007**, both halves, one subtest each: (a) a saved report is opened and its resolved count noted, a **new record matching its criteria** is added, and reopening resolves to the new total with **nothing edited, no version bump and no staleness warning anywhere in the response** (US3 AS-4, FR-026, FR-027); (b) a record the saved report **previously resolved to is deleted**, and reopening resolves to exactly one fewer with **no error, no warning and no reference to the missing record** (US3 AS-5, FR-026). The count is read from the same field the editor renders, so a resolver that silently stored a record list fails here rather than in front of a person
- [ ] T115 [P] [US3] Failing tests in `internal/store/reporttemplate/repo_test.go` and `internal/web/api/report_templates_http_test.go` for FR-032 and US3 AS-11: deleting the referenced person **empties** `patient` and leaves the row; the editor renders the person-is-gone panel; producing is `409 patient_unreachable`; **choosing a different person makes it usable again**
- [ ] T116 [P] [US3] Failing tests for the last-produced line on a saved report — a page-level convenience derived at render time, adding no field to `ReportTemplate` and no column anywhere, and mandated by **no** functional requirement, so it cites none (it cited FR-028, which is T112's — ANALYSIS N2): a saved report shows when it was last produced and by which document, derived from `export_jobs.template` with **no extra column on the template**, and a template whose jobs have all expired reads "never produced since …" rather than an error
- [ ] T117 [P] [US3] Failing tests in `internal/service/audit/writer_test.go`: saving, changing and deleting a template each write exactly one row — `create`, `update` and `delete` with `target_kind = report_template` (`contracts/report-templates.md`), the declared vocabulary and not a bespoke one — naming the template's opaque id and **never its name** (FR-014, SC-014)
- [ ] T118 [P] [US3] Failing templ render tests in `internal/web/views/reports/templates_templ_test.go` and `internal/web/views/reports/editor_templ_test.go`: the saved-report list, its empty state, the editor, the person-is-gone panel and the delete confirmation each render inside their landmark (`region[name="Reports"]`, `article[name="Report template"]`)
- [ ] T118a [P] [US3] Failing archive-content tests in `internal/render/archive/writer_test.go` for **FR-035 and US3 AS-13**: a complete export (`kind = data_export`) of an account holding saved reports writes `data/report_templates.json` carrying **every** saved report that account owns, each as the same `api.ReportTemplate` DTO op 73 returns **minus `resolved`** — name, description, person, kinds, range, tags, charts and presentation settings — so what leaves is **the criteria, never a resolved record list** (FR-026); a saved report belonging to **another** account appears nowhere in the archive; an account with **no** saved reports still gets the file with `[]`, never `null` and never an absent key (US2 AS-1's rule applied here); and `manifest.json`'s `files[]` describes the file, which is what makes the T101 build gate cover it (FR-038, FR-039, research D-29)
- [ ] T118b [P] [US3] Failing **round-trip** test in `internal/render/archive/roundtrip_test.go` for FR-035 and US3 AS-13: produce a complete export, reopen the `.zip` with `archive/zip` and `encoding/json/v2` alone — **no MediGo service, no repository, no live instance**, which is FR-039's "readable without the application" applied to this file — and assert every object read back equals the DTO op 73 serves for that saved report **field for field with `resolved` absent** — the archive carries the question, not a count taken at production time — that feeding those criteria back through the resolver yields the same record count the API reports for it, and that a name, a description or a tag containing a comma, a quote or a non-Latin script survives the round trip unaltered (research D-29, D-30)

### Implementation for User Story 3

- [ ] T119 [US3] Implement `internal/service/report/template.go` — create, read, update, delete, the case-insensitive conflict, the override path and the unreachable-person state (depends on T110–T116)
- [ ] T120 [US3] Implement `internal/web/api/report_templates.go` — ops 71–75 per `contracts/report-templates.md`, including `ETag`/`If-Match` (depends on T119)
- [ ] T121 [US3] Implement `internal/web/page/report_template.go` and the templ components in `internal/web/views/reports/{templates.templ,editor.templ,gone.templ}` (depends on T118, T120)
- [ ] T122 [P] [US3] Extend `e2e/specs/reports.spec.ts` with `/reports/{id}` at both viewports: the editor, the delete confirmation, the person-is-gone panel, and the no-saved-reports empty state as `empty@medigo.local`
- [ ] T122a [US3] Extend `internal/render/archive/{writer.go,manifest.go}` **[EDIT]** — stream the account's saved reports into `data/report_templates.json` through the same `api.ReportTemplate` encoder op 73 uses, one object at a time, and add the file to `manifest.json`'s `files[]`; and extend `docs/export-format-v1.md` **[EDIT]** with its entry so the T101 build gate stays green (FR-035, FR-038, FR-039, US3 AS-13; depends on T118a, T118b, T108)

**Checkpoint**: a recurring visit is one click. US1, US2 and US3 all work independently.

---

## Phase 6: User Story 4 — See the shape of a number over time (Priority: P4)

**Goal**: a chart that makes a trend obvious, with no unit conversion anywhere in the product.

**Independent Test**: seed a person with a year of blood-pressure and weight readings and a lab
component recorded in two units. Open the chart picker, confirm only series with enough readings are
offered as chartable and the rest are shown with their counts, confirm the two-unit component appears
**twice**, add both to a report, produce it, and confirm each chart is labelled with its unit and its
range and no value was converted.

### Tests for User Story 4 ⚠️ write first, confirm red

- [ ] T123 [P] [US4] Failing HTTP tests in `internal/web/api/reports_http_test.go` for op 70: one entry per numeric vitals column with at least one reading; labs grouped by `(canonical_name, unit)`; **a component in two units appears twice, each with `multi_unit: true`, and there is no combined entry**; `minimum` and `max_charts` are the published configuration values (US4 AS-1, AS-4, FR-016, FR-018, FR-023)
- [ ] T124 [P] [US4] Failing tests for FR-017 and US4 AS-3: a series below the minimum is present with `chartable: false`, `readings` and `readings_needed` — **never hidden** — and the page states both numbers
- [ ] T125 [P] [US4] Failing tests for FR-019 and US4 AS-5: a candidate `from`/`to` narrowing that leaves too few readings is reported **at the moment the range is chosen**, `422 not_enough_readings`, with the count it has and the count it needs
- [ ] T126 [P] [US4] Failing tests in `internal/render/pdf/chart_test.go` — **re-open the PDF**: each chart carries its title, its unit, its date range and its readings count; the axis labels are legible; a chart is never drawn without its unit (US4 AS-2, FR-020, FR-022)
- [ ] T127 [P] [US4] Failing tests in `internal/render/pdf/chart_test.go` for gaps and density: a series with a six-month gap shows the gap **rather than interpolating across it** (asserted on the drawn coordinates, not the image); a series over `MEDIGO_REPORT_MAX_CHART_POINTS` is drawn without becoming unreadable and the document **states that the series was summarised for display** (US4 AS-6, AS-8, FR-021, research D-05)
- [ ] T128 [P] [US4] Failing tests for FR-023 and US4 AS-7: selecting more than `MEDIGO_REPORT_MAX_CHARTS` is `422 too_many_charts` **stating the limit**, and the same limit is enforced in the domain, so a saved report cannot smuggle past it
- [ ] T129 [P] [US4] Failing tests for US4 AS-9: charts and records selected together produce **one** document in which the charts precede the records in the documented order
- [ ] T130 [P] [US4] Failing tests for FR-024 and US4 AS-10: op 70 against a person the caller cannot reach is `404` identical to a non-existent id and writes one `access_denied` row
- [ ] T131 [P] [US4] Failing **grep** test in `internal/render/pdf/noconvert_test.go` plus the `task lint:noconvert` gate: no unit-conversion function exists anywhere in the module (FR-018, SC-008)
- [ ] T132 [P] [US4] Failing templ render tests in `internal/web/views/reports/charts_templ_test.go`: the picker, the two-unit pair, the not-yet-chartable rows with their counts, and the no-measured-values empty state each render inside `region[name="Reports"]`

### Implementation for User Story 4

- [ ] T133 [US4] Implement `internal/service/report/trends.go` — the `(canonical_name, unit)` grouping, the chartability calculation and the candidate-range check, over the same resolver as op 69 (depends on T123, T124, T125, T130)
- [ ] T134 [US4] Implement `internal/web/api/reports.go` op 70 per `contracts/reports.md` (depends on T133)
- [ ] T135 [US4] Implement `internal/render/pdf/{chart.go,axis.go,decimate.go}` — the line chart drawn with `fpdf` primitives, the nice-number axis, the gap handling and the largest-triangle decimation with its stated note (depends on T081, T126, T127)
- [ ] T136 [US4] Wire the chart section into `internal/service/report/document.go` and `internal/render/pdf/renderer.go` in the documented order (depends on T129, T135)
- [ ] T137 [US4] Implement the picker in `internal/web/views/reports/charts.templ` and its wiring in `internal/web/page/reports.go` (depends on T132, T134)
- [ ] T138 [P] [US4] Extend `e2e/specs/reports.spec.ts` with the chart picker: the two-unit pair rendered as two rows, a not-yet-chartable row showing both counts, and the over-limit refusal stating the limit

**Checkpoint**: a trend is visible, and the product still contains no conversion.

---

## Phase 7: User Story 5 — Run the instance without reading the source (Priority: P5)

**Goal**: an operator sees what the instance holds and how it is running, and administers accounts,
without ever seeing a person's records.

**Independent Test**: sign in as the administrator, open `/admin`, confirm every figure carries its
definition and the moment it was computed, confirm the posture warnings name exactly what is missing,
open `/admin/users`, change a tier, disable an account, force a password change, and confirm each is
refused where the rules say it must be and recorded where it succeeds.

### Tests for User Story 5 ⚠️ write first, confirm red

- [ ] T139 [P] [US5] Failing tests in `internal/service/admin/opsfig/catalogue_test.go` — **three build-failing gates** (research D-47, FR-080, SC-011): every key of `AdminStats` and `AdminSystem` exists in the catalogue; every catalogue entry is rendered on `/admin`; **every entry has a non-empty definition and a unit from the permitted set**
- [ ] T140 [P] [US5] Failing reflection test in `internal/web/api/dto_reporting_test.go`: no field of `AdminStats` or `AdminSystem` is a free-text type outside the enumerated `State` and `Version` cases (FR-086, research D-47)
- [ ] T141 [P] [US5] Failing HTTP tests in `internal/web/api/admin_instance_http_test.go` for op 80: every figure of the catalogue present with its `definition`, `unit` and `computed_at`; the two expensive figures carry `age_seconds` and `refreshed`; on an **empty instance every figure is zero with its definition** — never blank, never an error (US5 AS-1, FR-078, FR-081)
- [ ] T142 [P] [US5] Failing HTTP tests for op 81: `ready`, `uptime_seconds`, `version`, `migrations`, `backup.state ∈ {ok, stale, never}`, `posture`, `retention[]`, `limits[]` and `attention[]` all as `contracts/admin-instance.md` specifies (FR-077, FR-082, FR-085, FR-087)
- [ ] T143 [P] [US5] Failing tests for FR-082, US5 AS-3 and AS-4: an instance never backed up reports `never` **as a warning, not a blank or a zero**; an instance past `MEDIGO_BACKUP_WARN_AFTER` reports `stale` **stating the age**
- [ ] T144 [P] [US5] Failing tests for FR-083, SC-012 and US5 AS-5: with MFA off and no allowlist, `warnings` names exactly those two, the page renders an **unmistakable** warning naming what is missing and what to do, the same warning is emitted at **every** boot, and the MFA message states PocketBase's two-auth-method precondition (research D-17)
- [ ] T145 [P] [US5] Failing tests for FR-084 and US5 AS-6: with SMTP unconfigured the posture says so and names the features of earlier phases that refuse without it — phase 001's password recovery and address confirmation and phase 005's invitations
- [ ] T145a [P] [US5] Failing tests for FR-136: `posture.oauth2` is `configured` when any provider is enabled and `unconfigured` otherwise, is **not** a warning either way, and no client id or secret appears in the DTO, the page or any log line
- [ ] T146 [P] [US5] Failing tests for FR-085 and US5 AS-7: a failed scheduled job appears in `attention[]` **exactly once** with what failed and when, and is retried on its next scheduled run rather than in a loop
- [ ] T147 [P] [US5] Failing HTTP tests in `internal/web/api/admin_users_http_test.go` for op 82: every account with its tier, its state, when it was created and when it last signed in, and **nothing about what it holds beyond counts** — asserted by a field-level allowlist (FR-088, FR-089, US5 AS-8)
- [ ] T148 [P] [US5] Failing HTTP tests for op 83 running the `adminuser` rules over the wire: promotion and demotion take effect on the account's **next request** (FR-090, US5 AS-9); an actor may not change their own tier; the **last enabled administrator** may not be demoted or disabled by anybody; each refusal names the reason (FR-095, FR-096, US5 AS-12, AS-13)
- [ ] T149 [P] [US5] Failing tests for FR-091, FR-092 and US5 AS-10 — the disable path, research D-49: disabling **immediately** ends every session by calling `record.RefreshTokenKey()` inside the same transaction; the account's next request is `401`; **signing in as a disabled account is byte-identical to a wrong password**, asserted on status, body and elapsed time; its data is untouched and re-enabling restores access unchanged
- [ ] T150 [P] [US5] Failing tests for FR-093 and US5 AS-11: forcing a password change sets `must_change_password`, the account is met with the change **before anything else** (`403 password_change_required` on every route but the change), and completing it restores normal access and writes one row
- [ ] T150a [P] [US5] Failing tests for FR-097 in `internal/web/api/me_privilege_test.go` **[EDIT of phase 001 T194]** and `internal/web/api/auth_test.go`: neither `role`, nor the disabled state, nor `must_change_password` can be set through registration or through `PATCH /api/v1/me`, by any spelling — an unknown field, a null, a nested object — and each attempt is `422 unknown_field` with the stored account unchanged. Driven from the account field list by reflection, so a privileged field added later fails this test by default. Ops 82 and 83 are the only routes that change any of the three
- [ ] T151 [P] [US5] Failing tests for FR-094 and FR-098: an administrator has **no** privileged access to any person's records — a direct attempt on every patient-scoped operation of phases 001–006 is `404` — and every administrative act writes one entry naming the actor, the target account and what changed, with **nothing about the account's contents** (US5 AS-14, AS-15, SC-013, research D-48)
- [ ] T152 [P] [US5] Failing tests in `internal/web/api/admin_guard_test.go`: **every** operator route reached by `role = user` is `404` identical to the route not existing, and writes exactly one `access_denied` row (FR-076, SC-010)
- [ ] T153 [P] [US5] Failing templ render tests in `internal/web/views/admin/{overview_templ_test.go,users_templ_test.go}`: the figure tiles with their definitions, the posture warning, the retention table, the limits table, the attention list, the account list and the three confirmations render inside `region[name="Administration"]` and `region[name="Users"]`, and every figure at zero still renders its tile

### Implementation for User Story 5

- [ ] T154 [US5] Implement `internal/service/admin/opsfig/catalogue.go` — key, label, definition, unit — as the single source both DTOs and the page render from (depends on T139)
- [ ] T155 [US5] Implement `internal/service/admin/stats.go` and `internal/web/api/admin_instance.go` op 80 (depends on T051, T141, T154)
- [ ] T156 [US5] Implement `internal/service/admin/system.go` and op 81 — posture, retention with last-run/last-success from the job envelope, limits and the attention list (depends on T053, T055, T142–T146)
- [ ] T157 [US5] Implement the boot-time posture warning in `internal/platform/pb/boot.go`, emitted at **every** boot alongside the existing admin-UI warning (depends on T144)
- [ ] T158 [US5] Implement `internal/service/admin/users.go` and `internal/web/api/admin_users.go` ops 82 (`listAdminUsers`) and 83 (`updateAdminUser`) — the only path by which the administrative tier, the disabled state or a forced password change ever moves (FR-097) — calling into `domain/adminuser` and refreshing the token key inside the transaction (depends on T024, T147–T151)
- [ ] T159 [US5] Implement the `must_change_password` interception in `internal/web/middleware/mustchange.go` and register it ahead of every route but the password change (depends on T150)
- [ ] T160 [US5] Implement the operator-tier guard in `internal/web/middleware/adminonly.go`, driven by the registry's `AdminOnly` flag, returning the shared `404` and writing the `access_denied` row (depends on T060, T152)
- [ ] T161 [US5] Implement `internal/web/page/admin.go` and `internal/web/page/admin_users.go` with the templ components in `internal/web/views/admin/{overview.templ,figure.templ,posture.templ,users.templ,confirm.templ}` (depends on T153, T155, T156, T158)
- [ ] T162 [P] [US5] Add `e2e/specs/admin.spec.ts` and `e2e/specs/admin-users.spec.ts` — both viewports, populated and empty, the posture warning visible, every figure tile present at zero, and the three account confirmations
- [ ] T163 [P] [US5] Add `e2e/specs/operator-denied.spec.ts` — the four operator pages as a non-administrator: the shared 404 view and an `access_denied` row per attempt (FR-076, SC-010)
- [ ] T163a [P] [US5] Failing tests in `internal/web/api/oauth2_test.go` for op 4 (`contracts/auth-oauth2.md`, FR-134, FR-135, FR-137, SC-028): with no provider configured the route is `404` and `GET /api/v1/auth/config` reports `[]`; an unknown provider name is byte-identical to that `404`; `role`/`disabled_at`/`verified` in the body is `422` **and** any created account is `user`, asserted against the database; a disabled account is refused byte-identically to a wrong password and writes `login_failed`; a success writes exactly one `login` row through phase 001's `OnRecordAuthRequest` hook rather than from the handler; a provider identity already linked to one account cannot be attached to a second
- [ ] T163b [P] [US5] Failing templ render test in `internal/web/views/auth/login_templ_test.go` (phase 001 component, **[EDIT]**): with no provider configured the sign-in page renders **no** provider control at all — the assertion SC-028 measures
- [ ] T163c [US5] Implement `internal/web/api/oauth2.go` and the `OAuth2SignIn` DTO, register op 4, extend `AuthConfig` with `oauth2_providers` (names only), and extend the sign-in page and its Playwright case with the provider control that appears only when a provider is enabled (depends on T060, T163a, T163b)

**Checkpoint**: the instance is operable from the interface, and the operator still cannot read a
single record.

---

## Phase 8: User Story 6 — Find out what happened (Priority: P6)

**Goal**: a trustworthy, complete, immutable, narrowable account of what happened.

**Independent Test**: perform a mixture of actions across several accounts and people, open
`/admin/audit`, narrow by person, by actor, by action and by date, page through under a continuous
writer, export the narrowing as CSV, and confirm the export matches the narrowing and that reading
recorded nothing.

### Tests for User Story 6 ⚠️ write first, confirm red

- [ ] T164 [P] [US6] Failing HTTP tests in `internal/web/api/audit_http_test.go` for op 68: newest first; every narrowing singly and combined; the **narrowing in force echoed** in `filters`; unknown `action`, `target_kind`, `sort` or `format` is `400`; `from > to` is `400` (FR-065, US6 AS-2, AS-3)
- [ ] T165 [P] [US6] Failing tests for FR-064, FR-114, US6 AS-2 and AS-6: every entry states the actor, the action, the kind of thing acted on, that thing's opaque reference, the person it concerned where there is one, and exactly when — in the account's timezone with the underlying instant unambiguous — and carries no content of any kind
- [ ] T166 [P] [US6] Failing **`ApiScenario` with `ExpectedEvents`** tests in `internal/web/api/audit_http_test.go`: reading a page fires **zero** record-collection hooks — the reader resolves nothing, including for targets that still exist (FR-068)
- [ ] T167 [P] [US6] Failing tests for FR-069 and US6 AS-7: an entry about something since deleted renders exactly like one about something that still exists — kind and opaque reference — and never an error
- [ ] T168 [P] [US6] Failing tests for FR-070, FR-071 and US6 AS-9 — the scoping: a `role = user` caller sees only `patient IN (:accessible) OR actor = :me`, resolved per request with **no cache**; `?count=true` counts that same set; there is **no** `total_all` and **no** "N entries hidden" affordance anywhere in the DTO or the page, asserted by reflection and by an HTML scan
- [ ] T169 [P] [US6] Failing tests for FR-072 and US6 AS-10: an administrator sees sign-in failures, admin-UI sessions, break-glass sessions, backups, restores, scheduled clean-ups and refusals, with `actor_kind ∈ {system, superuser}` carrying `actor: null` and attributed to the system, never to a person
- [ ] T170 [P] [US6] Failing tests for FR-073 and US6 AS-11: a refused act is recorded with a bounded reason — probing leaves a trace
- [ ] T171 [P] [US6] Failing tests for FR-074, FR-053 and US6 AS-12: **every** page, including an empty one, carries the retention window in force and the age of the oldest entry the instance holds, and an entry past the window is gone with the page saying so
- [ ] T172 [P] [US6] Failing **paging** tests for FR-066, FR-122, SC-016 and US6 AS-4: 50 consecutive pages under a continuous writer repeat **0** and skip **0**; a tampered cursor is rejected; a cursor from a different narrowing is rejected
- [ ] T173 [P] [US6] Failing tests for FR-067, SC-014, SC-016 and US6 AS-5 — the CSV branch: the same narrowing semantics; the documented column order, which has **no `ip`** because the trail has no such column; the values identical to the DTO's; **streamed** so 1,000,000 rows complete within **128 MiB** RSS with first bytes before the last row is read; exactly **one** `audit_export` row written **before** the stream opens (`//go:build scale` for the volume case)
- [ ] T174 [P] [US6] Failing tests for FR-075: reading the trail writes **nothing** — asserted by reading a page and comparing the `audit_events` row count before and after. (No US6 scenario states this; US6 AS-13 is about reading *records*, not the trail, and is T174a's — ANALYSIS N2)
- [ ] T174a [P] [US6] Failing whole-application tests in `internal/web/api/audit_readsensitive_test.go` — **the task that discharges SC-015**, and the only place the rule is asserted end to end over the finished product (US6 AS-13, FR-115): with one account owning a person and a second reaching the same person through a phase-005 arrangement, exercise **every** operation that opens a record of each registered kind and every document download, then read the trail and assert (a) the owner's own reads produced **0** entries at any privilege level, (b) the grantee's produced **exactly one `read_sensitive` per record opened and per document downloaded**, no more and no fewer, and (c) the break-glass credential reading the same things produced exactly one each, attributed to the superuser. Counted by differencing the `audit_events` row count around each read, so an unconditional writer and a silent non-writer both fail. The rule itself is stated once, in phase 005's [`widened-authorization.md`](../005-sharing-and-collaboration/contracts/widened-authorization.md) §"Where `read_sensitive` is written"; this asserts it against every kind phases 001–004 registered, which no earlier phase could do
- [ ] T175 [P] [US6] Failing tests for FR-062 and US6 AS-8: no route, no service method and no CLI subcommand can alter or delete an entry other than by the retention job, asserted by the registry, by a reflection scan of the `audit` service and by an `ApiScenario` proving the collection subtree is `404`
- [ ] T176 [P] [US6] Failing templ render tests in `internal/web/views/admin/audit_templ_test.go`: the list, the narrowing controls, the retention line, the paging controls, the CSV action and the **"Nothing has been recorded on this instance yet"** empty state all inside `region[name="Audit trail"]`

### Implementation for User Story 6

- [ ] T177 [US6] Implement `internal/service/audit/reader.go` — the typed narrowing, the per-request scope resolution and the retention envelope (depends on T026, T049, T164–T171)
- [ ] T178 [US6] Implement `internal/web/api/audit.go` op 68 and its CSV branch, streaming through `encoding/csv` over the same keyset cursor (depends on T173, T177)
- [ ] T179 [US6] Implement `internal/web/page/admin_audit.go` and `internal/web/views/admin/audit.templ` (depends on T176, T178)
- [ ] T180 [P] [US6] Add `e2e/specs/admin-audit.spec.ts` — both viewports, populated and empty, the narrowing controls, a paging walk, and the CSV download

**Checkpoint**: the trail answers the question, and reading it changes nothing.

---

## Phase 9: User Story 7 — Survive a disaster (Priority: P7)

**Goal**: an operator can take a complete copy of the instance, carry it away, and put it back.

**Independent Test**: take an archive, record more data, download the archive to another machine and
confirm it opens, then restore it and confirm the instance is exactly as it was at the moment the
archive was taken, that a safety copy of the pre-restore state exists, and that the trail records both.

### Tests for User Story 7 ⚠️ write first, confirm red

- [ ] T181 [P] [US7] Failing HTTP tests in `internal/web/api/admin_backups_http_test.go` for op 84: newest first, paged by the shared cursor, each row carrying when it was taken, its size, **how it came to be taken**, who took it and its note; the empty list explains what an archive is (FR-099, US7 AS-1)
- [ ] T182 [P] [US7] Failing tests for op 85 and FR-100: a manual archive is taken with a note, contains the whole instance, and is listed immediately; `409 archive_operation_in_progress` when `core.StoreKeyActiveBackup` is held, **checked before any work**, and PocketBase's own concurrent-backup error maps to the same code if the race is lost (US7 AS-2, AS-10)
- [ ] T183 [P] [US7] Failing tests in `internal/platform/backup/schedule_test.go` for FR-101 and US7 AS-3: `Settings().Backups.Cron` and `CronMaxKeep` are written from configuration at boot; a scheduled archive writes `backup_create` on success through `OnBackupCreate`; a **failed** scheduled archive writes `job_failed`, appears in `attention[]` and is not silently swallowed; and **both rows carry a non-empty `request_id` taken from the run's `run_id`** — a scheduled archive has no HTTP request, and `target_id` is the archive name, the `≤64` bounded exception (research D-43, 001 [data-model](../001-walking-skeleton/data-model.md) §3)
- [ ] T184 [P] [US7] Failing tests for op 86 and FR-102, US7 AS-4: an uploaded archive is validated **before it is stored** (opens as a zip, contains `data.db`, within `MEDIGO_EXPORT_MAX_BYTES`), then listed and treated **identically** to one taken here — same preview, same restore path, same version rules; `413`, `415` and `422 archive_unreadable` each covered
- [ ] T185 [P] [US7] Failing tests for op 87 and FR-103, US7 AS-5: the preview states when it was taken, its size, its note, the producing version, what exists **now** that would be replaced, **the loss stated as a sentence**, the confirmation phrase, and **every blocker** — including a running export — so an administrator learns of one before typing anything
- [ ] T186 [P] [US7] Failing tests in `internal/platform/backup/version_test.go` for the three version cases of research D-25 (FR-107): `schema_version` at or below the binary's highest → `compatible: true`; above → `compatible: false` with `archive_version_unsupported`; **absent `medigo.json`** → `compatible: null` with `version_unknown`, the restore refused unless `accept_unknown_version: true`, **and that acceptance recorded** (US7 AS-9)
- [ ] T187 [P] [US7] Failing tests in `internal/platform/backup/reader_test.go` for research D-26: a local archive is read with `zip.OpenReader` directly off `pb_data/backups/`; with S3 backup storage configured it is streamed into a scratch file under `.pb_temp_to_delete` first (because `archive/zip` needs an `io.ReaderAt`) and the scratch file is removed afterwards
- [ ] T188 [P] [US7] Failing tests for op 88 and FR-109, SC-018 — the most sensitive action the instance offers: authenticated → `role = admin` → password verified → stream, **in that order, on every request**; possession of the address grants nothing; the body is **byte-for-byte** what was stored, asserted by hashing both; a wrong password is `401` and records `login_failed`; `backup_download` is written on **every** call, successful or not (US7 AS-12)
- [ ] T189 [P] [US7] Failing tests in `internal/service/admin/restore_test.go` for the **ordered precondition sequence** of `contracts/admin-backups.md` op 89 (FR-107 — an archive that cannot be read, that fails its integrity check or that was produced by an incompatible version is refused with a reason naming no storage location, before anything is touched): password, confirmation phrase, active-backup check, running-job check, archive validation, safety copy — each failing step stops **without touching anything**, verified by asserting the database is byte-identical afterwards (FR-104, FR-108, US7 AS-6, AS-11)
- [ ] T190 [P] [US7] Failing tests for FR-105, SC-017 and US7 AS-7: the safety copy is taken **synchronously and waited for**; if it fails the restore does **not** proceed and the response says `safety_backup_failed`; it is **not skippable** for a recent archive
- [ ] T191 [P] [US7] Failing tests in `internal/platform/backup/journal_test.go` for research D-23 — the sharpest correctness problem in the phase: the journal is written to `MEDIGO_STATE_DIR` (**outside `pb_data`**, asserted); it survives the wholesale replacement of `pb_data`; the next `OnBootstrap` replays it into the **restored** database writing `backup_create` and `backup_restore` with both references, then deletes it; a crash **during** the restore leaves the journal present with the database unchanged and the next boot records `job_failed` (FR-111, US7 AS-15). **Plus the correlation case**: all three replayed rows carry the journal's recorded `request_id` — the correlation id of the restore request — and a journal with none falls back to the boot run's `run_id`, so a row written from `OnBootstrap`, where there is no HTTP request, never has the empty string in a `Required` column (research D-23, 001 [data-model](../001-walking-skeleton/data-model.md) §3)
- [ ] T192 [P] [US7] Failing tests in `internal/platform/env/snapshot_test.go` for research D-24: the snapshot is taken as the **first statement** of `main()`, before `config.Load()`; a variable read with `os.Getenv` + `,unset` semantics is still present in the snapshot; the snapshot is re-applied before `app.Restart()`, so the `execve` inherits every secret; and a **guard test** fails the build if `config.Load` is called before the snapshot in `cmd/medigo/main.go`
- [ ] T193 [P] [US7] Failing tests for FR-106, SC-017 and US7 AS-8: the response is `202` **before** the restore begins, states the expected downtime, and the instance returns either fully restored or unchanged, never partly; requests during the restore are `503 restore_in_progress`
- [ ] T194 [P] [US7] Failing tests for op 90, FR-110 and US7 AS-13: deleting an archive removes **only that archive** — other archives, safety copies and the trail are untouched — and the confirmation names the archive; `409 archive_operation_in_progress` covered
- [ ] T195 [P] [US7] Failing tests for FR-112 and US7 AS-14: the storage figures on `/admin` include the archives and their count, and an instance whose archives dominate its storage shows that plainly
- [ ] T196 [P] [US7] Failing tests in `internal/service/admin/restore_test.go` for research D-28: a restore is refused with `job_in_progress` while any `export_jobs` row is `queued` or `running`, with a message telling the administrator to wait or cancel — because a restore replaces the storage the worker is writing into
- [ ] T197 [P] [US7] Failing `ArchivesContract` run in `internal/platform/backup/archives_test.go` against the PocketBase-backed adapter (depends on T043)
- [ ] T198 [P] [US7] Failing templ render tests in `internal/web/views/admin/backups_templ_test.go`: the list, the empty state, take-with-note, upload, the preview panel with its loss sentence and blockers, the download password form (a plain `<form method="post">`, **no inline script**) and the restore confirmation, all inside `region[name="Backups"]`

### Implementation for User Story 7

- [ ] T199 [US7] Implement `internal/platform/env/snapshot.go` and make it the **first statement** of `cmd/medigo/main.go` (depends on T192)
- [ ] T200 [US7] Implement `internal/platform/backup/{archives.go,sidecar.go}` behind `admin.Archives` — list, create, upload, preview, download, delete, and the `StoreKeyActiveBackup` check (depends on T181, T182, T184, T197)
- [ ] T201 [US7] Implement `internal/platform/backup/{reader.go,version.go}` — the `medigo.json` read, the local and S3 paths, and the one compatibility function (depends on T186, T187)
- [ ] T202 [US7] Implement `internal/platform/backup/journal.go` — write, replay at `OnBootstrap`, delete (depends on T191)
- [ ] T203 [US7] Implement `internal/service/admin/restore.go` — the ordered preconditions, the safety copy, the journal, the `202`, the delayed goroutine and the `503` gate (depends on T189, T190, T193, T196, T199, T202)
- [ ] T204 [US7] Implement `internal/web/api/admin_backups.go` — ops 84–90 per `contracts/admin-backups.md`, with op 88 as `POST` (depends on T188, T194, T200, T203)
- [ ] T205 [US7] Write `Settings().Backups.Cron`, `CronMaxKeep` and the `OnBackupCreate` binding in `internal/platform/backup/schedule.go`, wired at boot (depends on T183)
- [ ] T206 [US7] Implement `internal/web/page/admin_backups.go` and `internal/web/views/admin/backups.templ` (depends on T198, T204)
- [ ] T207 [P] [US7] Add `e2e/specs/admin-backups.spec.ts` — both viewports, populated and empty, take-with-note, upload, preview, the download password form, and the restore confirmation **stopping short of executing a restore**
- [ ] T208 [US7] Add `internal/platform/backup/restore_e2e_test.go` (`//go:build slowsse`) — a **real** end-to-end restore against a throwaway instance: take, mutate, restore, and assert the instance matches the archive, the safety copy exists, and both entries are present in the restored trail (SC-017)

**Checkpoint**: disaster recovery works, and it recorded itself doing so.

---

## Phase 10: User Story 8 — Know what is kept and for how long (Priority: P8)

**Goal**: retention is visible, stated, and enforced identically to how it is described.

**Independent Test**: open `/admin`, read each window with what it applies to and when it last ran,
change one, confirm the interface reflects it and the next run behaves accordingly, then confirm a
produced document past its window is gone and says so plainly.

### Tests for User Story 8 ⚠️ write first, confirm red

- [ ] T209 [P] [US8] Failing tests for FR-052 and US8 AS-1: every retention window is stated on `/admin` with **what it applies to in plain words**, its value, its job, its last run and its last success — from the job envelope, with no `job_runs` collection (research D-43)
- [ ] T210 [P] [US8] Failing tests for FR-047, FR-054 and US8 AS-2: past its window a produced document's **content is gone from storage** while the request row remains, reading it is a plain statement not an error, re-production is offered, and the window that applied is named
- [ ] T211 [P] [US8] Failing tests for FR-053 and US8 AS-3: an activity entry past its window is gone; the trail states the window in force **and the age of the oldest entry**; and the count of what was removed is recorded once
- [ ] T212 [P] [US8] Failing tests for FR-056 and US8 AS-4: phase 004's `/documents?deleted=true` — the only trash surface there is, and the one the overview links to — shows each deleted document's remaining days, and the figure comes from **the same `retention.DaysRemaining`** the purge uses — asserted by a test that moves the clock and compares both (research D-14, D-50)
- [ ] T213 [P] [US8] Failing tests for FR-057 and US8 AS-5: nothing is removed **before** its window elapses, proven by a clock moved to one second before the boundary; and nothing is removed **late** without appearing in `attention[]`, proven by a job stubbed to fail
- [ ] T214 [P] [US8] Failing tests for FR-058, FR-085 and US8 AS-7: a purge job that fails is recorded, surfaces in `attention[]` **exactly once**, and is retried on its next scheduled run rather than in a loop
- [ ] T215 [P] [US8] Failing tests for FR-059, FR-060 and US8 AS-8: changing a window changes the interface immediately and the next run's behaviour, with no restart of anything else; a window outside its documented bounds **refuses to boot, naming the value and the bound** (research D-46)
- [ ] T216 [P] [US8] Failing tests for FR-061 and US8 AS-9: the clock moved **backwards** removes nothing early and the clock moved **forwards** removes nothing that has not elapsed — the same `retention.Window` table-driven suite exercised through the real jobs
- [ ] T217 [P] [US8] Failing tests for FR-063 and US8 AS-10: deleting an account destroys its requests, its produced documents and its saved reports **immediately**, not at the next window, and its activity entries follow the audit window with the account referenced by opaque id only
- [ ] T218 [P] [US8] Failing tests for FR-055 and US8 AS-6: each window's `applies_to` sentence is asserted against a fixed table, so a window can never be shown without saying what it governs
- [ ] T219 [P] [US8] Failing tests in `internal/service/audit/retention_test.go` for `medigo_purge_audit`: it deletes only entries past the window, in bounded batches inside `RunInTransaction`, writes exactly one `job_succeeded` with the count, and touches no other collection; its `purge` row and its `job_succeeded` row **share the run's `run_id` in `request_id`** and neither is empty, which is what makes the nightly purge survive the `Required` column it is itself purging (research D-43, 001 [data-model](../001-walking-skeleton/data-model.md) §3)

### Implementation for User Story 8

- [ ] T220 [US8] Implement `internal/service/audit/retention.go` and register `medigo_purge_audit` through the envelope (depends on T053, T219)
- [ ] T221 [US8] Implement the retention section of `internal/service/admin/system.go` and `internal/web/views/admin/retention.templ` — window, applies-to, job, last run, last success (depends on T156, T209, T218)
- [ ] T222 [US8] Point phase 004's remaining-days label on `/documents?deleted=true` at `internal/domain/retention` in `internal/web/views/files/library.templ`, deleting the local calculation. **This edits phase 004's one trash surface; it does not add a page** (depends on T015, T212)
- [ ] T223 [US8] Implement the account-deletion cascade for `report_templates` and `export_jobs` including artifact bytes, in `internal/service/account/delete.go` (depends on T099, T217)

**Checkpoint**: retention is described, visible, and enforced exactly as described.

---

## Phase 11: User Story 9 — Ship it without holding my breath (Priority: P9)

**Goal**: the gates that make a release boring, across the **whole product**, not only this phase.

**Independent Test**: run the full gate suite on a clean checkout and confirm every user-facing page
of phases 001–006 loads without a console error at both viewports, the performance budgets are met,
the privacy review passes, and a deliberately broken page turns the gate red and names itself.

### Tests for User Story 9 ⚠️ write first, confirm red

- [ ] T224 [P] [US9] Add `e2e/specs/full-sweep.spec.ts` — every route from `medigo routes` with `Page: true`, at 1440×900 and 390×844, as the populated account: `200`, the four shell landmarks, the route's declared landmark, `body[data-signals]`, **zero console, page and network errors** (FR-126, SC-021, US9 AS-1)
- [ ] T225 [P] [US9] Add `e2e/specs/empty-account.spec.ts` — the same sweep as `empty@medigo.local`: every page that can be empty proves its explanation renders **inside** its own landmark, and the gate stays green on a legitimately empty instance (FR-125, US9 AS-2)
- [ ] T226 [P] [US9] Add `e2e/specs/gate-negative.spec.ts` — two deliberate negatives run in CI as **expected failures**: a page instrumented to throw a console error turns the gate red **and the failure names the page** (US9 AS-3); a route added to the inventory with no spec fails `routes.gate` (US9 AS-4)
- [ ] T227 [P] [US9] Extend `e2e/routes.gate.spec.ts`: every `Page: true` route has a smoke case, and every `page_action` route names an **existing** spec that references it — a page added without coverage fails the build (FR-127, US9 AS-4)
- [ ] T228 [P] [US9] Add `internal/web/perf_test.go` (`//go:build scale`) asserting the whole published budget table of `plan.md`, one budget per success-criterion journey, so a regression fails the build (FR-131): the reports view under **2 s** and a re-count under **500 ms** on 10,000 records (SC-023); `/admin` under **2 s**; a page of the trail under **1 s** against 1,000,000 entries (SC-024, FR-121, US9 AS-5)
- [ ] T229 [P] [US9] Extend `internal/testsupport/phileak/scan_test.go` (`//go:build phileak`) to the **whole application** (FR-130): exercise every operation of phases 001–006, every scheduled job and every production worker against the seeded fixture and assert no log line, no error message, no page title, no URL and no HTTP header carries a person's name, a record value, a file name, a tag name or a storage location (FR-117, FR-118, FR-119, SC-020, US9 AS-6)
- [ ] T230 [P] [US9] Extend phase 002's egress harness — `internal/testsupport/netgate/dial_test.go` (`//go:build netgate`) **[EDIT]** — so its `net.Dialer` control hook fails the test on **any** outbound connection while the **whole-product** smoke suite of phases 001–006 runs, proving no third party receives patient data (FR-116, FR-119, SC-019, US9 AS-7). There is one egress harness in the suite and phase 002 owns it (cross-artifact finding M6's rule)
- [ ] T231 [P] [US9] Add `internal/web/security_test.go` asserting the Principle VII review points as executable checks: every patient-scoped operation re-authorizes per request; every refusal is `404` not `403`; every FileField is `Protected: true`; no file token is ever minted; the CSP is exactly the documented one and `script-src` carries `'unsafe-eval'` and nothing else beyond `'self'` (FR-120, US9 AS-8)
- [ ] T232 [P] [US9] Add `internal/httproute/gate_test.go` — the three-way agreement gate: every registry entry with an `operationId` appears in `api/openapi.json`; every `openapi.json` operation appears in the registry; every `Page: true` route has a Playwright case; every `page_action` is **absent** from `openapi.json` (FR-127, SC-022)
- [ ] T233 [P] [US9] Add `internal/platform/pb/logstream_test.go` — Principle VI end to end: a request, a job run, a PocketBase-internal log and a panic all arrive on **one** zerolog stream in JSON with a request id, `Logs.MaxDays` is `1` and never `0`, and no `log.Printf`, `fmt.Print*` or `slog` call exists (asserted by forbidigo)
- [ ] T234 [P] [US9] Add `internal/platform/pb/writetimeout_test.go` (`//go:build slowsse`) — the SSE lifetime trap: a subscriber held for **more than ten continuous minutes** stays connected, proving the `ServeEvent` `WriteTimeout` override is in force and that a shorter test would have passed regardless (FR-129 — ten, not the six that would already clear PocketBase's hardcoded five)
- [ ] T235 [P] [US9] Add `internal/store/migrations/reversibility_test.go` — every migration this phase adds applies **up then down then up** against a throwaway app with the expected schema each time, and the documented caveats are asserted rather than assumed

### Implementation for User Story 9

- [ ] T236 [US9] Implement `internal/web/middleware/restoregate.go` — the `503 restore_in_progress` gate, and register it ahead of every route (depends on T193)
- [ ] T237 [US9] Regenerate `api/openapi.json` from the registry and **commit the diff**, then confirm `task gate:openapi` is clean (depends on T060, T232)
- [ ] T238 [US9] Run `task lint` — golangci-lint v2 with depguard and forbidigo — and fix every violation in this phase's packages, changing the rules only where T004 justified it (depends on T004)
- [ ] T239 [US9] Run `task lint:isnull` and `task lint:noconvert` and fix every violation (depends on T005, T006, T131)
- [ ] T240 [US9] Write `docs/operations.md` — take and restore an archive, read the trail, administer accounts, every configuration value with its bounds, every retention window, what the posture warnings mean and how to clear them, and **the explicit statement that reads are not recorded while exports are** (FR-115) — the handbook that takes somebody who has never run the application from a clean machine to a running, backed-up and restored instance, documenting every setting, every retention window, every limit and every figure on the operator overview (FR-133, `contracts/README.md` §Audit) — **discharges SC-026**, and is written to be sufficient on its own: a reader who has never run the application gets from a clean machine to a running instance, takes an archive and restores it **from this document alone**, and can state from it what every setting, every retention window and every operator figure means
- [ ] T241 [US9] Write `docs/security-review-006.md` — the Principle VII walk-through **and the privacy review of the finished application against the stated privacy rules**, each point linked to the test in T231 or T229 that proves it, every finding either fixed or recorded with its reason, and the accepted `'unsafe-eval'` recorded as permanent with its reason. Written as a re-runnable checklist so a later run produces a comparable result (FR-132) — **discharges SC-027**: every finding is either fixed with the change named or recorded with its reason, and re-running the checklist after a change produces a comparable written result
- [ ] T242 [US9] Add the four tagged suites and the three gates to `.github/workflows/ci.yaml` in the documented order, so a red gate blocks a merge (depends on T010, T232, T237)

**Checkpoint**: a release is boring. Every page of the product is proven to render, the budgets hold,
and nothing leaks.

---

## Phase 12: Polish and cross-cutting

- [ ] T243 [P] Add `internal/service/access/coverage_test.go` entries for **every** operation this phase adds, so the actor matrix of `contracts/README.md` is enforced rather than described, and the build fails on an operation with no entry (FR-128, SC-025)
- [ ] T244 [P] Extend `internal/cli/routes.go` so `medigo routes` prints the new `Kind: page_action` and `AdminOnly` columns, and update `docs/routes.md`
- [ ] T245 [P] Add `/Users/krzysztof.wiatrzyk/private/monorepo/.dockerignore` and `/Users/krzysztof.wiatrzyk/private/monorepo/.github/workflows/build-image.yaml` entries covering any new top-level path this phase introduced, then build the image from the monorepo root to prove it (the failure is otherwise a misleading "file not found")
- [ ] T246 [P] Confirm the image is still distroless and `CGO_ENABLED=0` after `fpdf` and the embedded fonts, and record the size delta in `docs/operations.md` (depends on T003, T011)
- [ ] T247 [P] Run `quickstart.md` start to finish on a clean checkout and correct anything that does not behave as written, including the nine story walkthroughs; then run the SC-026 walk — clean machine to running instance, take an archive, restore it — **using `docs/operations.md` (T240) alone**, consulting neither the specs nor the code, and answer every question it raises by amending that document rather than by asking anybody
- [ ] T248 [P] Re-read `plan.md`'s Constitution Check against the code as built and record any drift in Complexity Tracking — a tracked deviation that turned out unnecessary is deleted, not kept
- [ ] T248a Write `specs/006-reporting-and-operations/traceability.md` — the mechanical join, generated from `spec.md` and `tasks.md` rather than written by hand: one row per functional requirement (all 137) naming the task ids that satisfy it and the named test that proves it, one row per acceptance scenario naming its test, and one row per success criterion (all 28) naming its task or its exit criterion. **A functional requirement with no task, or a success criterion that is neither mapped nor marked `[outcome metric]` in `spec.md`, fails the phase** (cross-artifact finding M7)
- [ ] T249 Run `task gate` — lint, unit, integration, templ render, smoke, `phileak`, `netgate`, `scale`, `slowsse`, the OpenAPI diff and the route inventory — on a clean checkout, and confirm Criterion 0 and the twelve Phase Exit Criteria of `plan.md` are all met

---

## Dependencies and execution order

### Phase order

```
Setup (T001–T013)
   │
   ├── T002 gates T003 gates T081 (the PDF renderer)
   ▼
Foundational (T014–T063)   ← BLOCKS EVERYTHING
   │
   ├──────────┬──────────┬──────────┬──────────┬──────────┬──────────┬──────────┐
   ▼          ▼          ▼          ▼          ▼          ▼          ▼          ▼
 US1 (P1)   US2 (P2)   US3 (P3)   US4 (P4)   US5 (P5)   US6 (P6)   US7 (P7)   US8 (P8)
 T064–087   T088–109   T110–122   T123–138   T139–163   T164–180   T181–208   T209–223
   │          │          │          │          │          │          │          │
   └──────────┴──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘
                                     │
                                     ▼
                             US9 (P9) T224–242   ← needs every page to exist
                                     │
                                     ▼
                             Polish T243–249
```

### Cross-story dependencies, stated honestly

The stories are independently **demonstrable**, but four edges are real and pretending otherwise
would cost a day:

| Edge | Why |
|---|---|
| US2 → US1 | the worker, the queue and the download route are US1's (T083, T084). US2 adds a second `kind` to them |
| US4 → US1 | a chart is drawn into the document US1's renderer produces (T081) |
| US3 → US1 | a saved report produces through US1's request path (T084) |
| US8 → US5 | the retention section renders inside US5's `/admin` page (T156) |

US6 and US7 depend on nothing but the Foundational phase. US9 depends on every page existing, which
is why it is last regardless of its priority letter.

### Within a story

Test tasks marked `[P]` are all parallel with one another — different files, no shared state. The
implementation tasks that follow are mostly sequential, because they converge on a handful of files
(`internal/web/api/exports.go`, `internal/render/pdf/renderer.go`, `internal/service/admin/system.go`).

---

## Parallel execution opportunities

**Setup**: T004, T005, T006, T007, T009, T010, T011, T013 all run together. T002 is the long pole and
should start first — nothing about the PDF renderer can be trusted until it lands.

**Foundational**: three independent tracks —

- *domain*: T014, T016, T017, T018, T020, T021, T023, T025, T027
- *collections*: T028, T030, T032, T033, T034, T036
- *ports and contract suites*: T037 then T038–T043 together

**Story tests**: within any story every `[P]` test task is parallel-safe. The largest single batch is
US5's T139–T153 — fifteen files, no overlap.

**Across stories**: after the Foundational checkpoint, US6 and US7 can be developed by a second pair
in complete isolation from the US1/US2/US3/US4 chain, because they share no file with it.

**Example — the second week, three tracks at once**

```
Track A: T064 T065 T066 T067 T068 T069 T070 T071 T072 T073 T074 T075 T076 T077   (US1 tests)
Track B: T164 T165 T166 T167 T168 T169 T170 T171 T172 T173 T174 T174a T175 T176  (US6 tests)
Track C: T181 T182 T183 T184 T185 T186 T187 T188 T189 T190 T191 T192 T193 T194   (US7 tests)
```

---

## Implementation strategy

**MVP first.** Setup → Foundational → US1 → stop and demonstrate. A person can build a selection, see
exactly what it will contain, produce a document and hand it to a consultant. That is the phase's
reason to exist and everything after it is addition.

**Then, in priority order.** US2 proves leaving is possible and is the second-most valuable thing
here. US3 and US4 make the first two pleasant. US5 through US8 make the instance operable. US9 is the
gate, and it runs last because it tests everything before it.

**Two things to do early even though they are late in the list.** T002 (the PDF spike), because a
wrong answer changes `internal/render/pdf` wholesale; and T192/T199 (the environment snapshot),
because it is a one-line ordering constraint in `main()` that is nearly free now and extremely
expensive to discover after a real restore.

**Count.** **261 task lines** — 249 numbered ids, `T001`–`T249` with no gaps, plus **twelve suffixed
ones**. **162 of them are test tasks**, and every one precedes the code it covers.

The test-task figure is reproducible by one rule, stated here because an earlier revision published a
figure no rule reproduced (ANALYSIS N8): a task line is a **test task** when it names a `_test.go` or
a `.spec.ts` file, or instructs a failing test to be written.

```
grep -cE '^- \[ \] T[0-9]+[a-z]?' tasks.md                                  # 261
grep -E  '^- \[ \] T[0-9]+[a-z]?' tasks.md | grep -cE '_test\.go|\.spec\.ts|ailing'   # 162
```

The twelve suffixed ids, and why each is suffixed rather than renumbered — so every task id cited
elsewhere still points at the same task:

| Id(s) | Why it exists |
|---|---|
| T032a | the complete-set audit-vocabulary assertion (ANALYSIS C1) |
| T114a | SC-007's resolver test — US3 AS-4 and AS-5 (ANALYSIS N1, N4) |
| T118a, T118b, T122a | FR-035 and US3 AS-13 — saved reports in the export archive, and its round trip (ANALYSIS N1) |
| T145a, T163a–T163c | external sign-in, which cross-artifact finding **H7** allocated to this phase |
| T150a | FR-097 — privilege fields unsettable through registration or self-service |
| T174a | SC-015 and US6 AS-13 — `read_sensitive` asserted over the finished product (ANALYSIS N4) |
| T248a | the traceability join |
