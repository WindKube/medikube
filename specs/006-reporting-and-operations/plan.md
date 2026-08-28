# Implementation Plan: Reporting and Operations

**Branch**: `006-reporting-and-operations` | **Date**: 2026-08-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-reporting-and-operations/spec.md`

**Constitution**: [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.3.0 (binding)

**Shared design contract**: [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) (binding on design; see
[Deviations](#deviations-from-the-shared-design-contract))

---

## Summary

Five phases built an application that holds a family's medical history correctly. This phase is
where a person gets to **take it out** — as a document to hand a consultant, or as an archive to
carry to another country — and where an operator gets to **see, audit, back up and recover** the
instance they are running in a cupboard.

It is also the phase whose largest wins come from **not building things**. PocketBase already ships
backup, restore, scheduled backups with retention, and an admin UI, and Principle V forbids
reimplementing any of them. The specification agrees in its own words: *"Backup and restore wrap what
the instance already ships with."* So the split this plan holds to, stated once and then obeyed:

| Genuinely bespoke (MediKube builds it) | Thin glue over PocketBase (MediKube wraps it) |
|---|---|
| The PDF renderer and its chart drawing — `internal/render/pdf`, ~1,500 lines, the single largest new artefact in the phase | Backup list / create / upload / delete — four calls on `app.NewBackupsFilesystem()` and `app.CreateBackup` |
| The export archive writer and the documented v1 format — `internal/render/archive` | Restore — `app.RestoreBackup`, wrapped with the preview, the mandatory safety copy, the confirmation and the journal |
| The one-worker job queue, its reconciliation and its cancellation | Scheduled backups — `Settings().Backups.Cron` + `CronMaxKeep`, already implemented including pruning; MediKube only makes failures **visible** |
| The audit reader, its scoping, its keyset paging and its streaming CSV | Superuser MFA / IP-allowlist posture — two field reads (D-17) |
| The report builder, the selection resolver and the trends picker | The admin UI itself — shipped, hardened, **not** replaced |
| The saved-report resource | Cascade deletion of an account's artifacts — a `CascadeDelete: true` relation |
| The operator figure catalogue and the account-administration rules | Concurrency control on archives — PocketBase's own `StoreKeyActiveBackup` |

And the phase carries the release hardening for the **whole product**: the browser gate across every
page phases 001–006 deliver, the performance budgets, the PHI-leak sweep, the no-outbound-connection
proof, the >10-minute live-view proof, the privacy review and the operator handbook.

The load-bearing commitments, each read out of PocketBase v0.40.1's real source rather than its
documentation:

1. **A restore destroys the audit rows that describe it.** `core/backup_restore.go:30-46` replaces
   the whole of `pb_data`. An audit entry written before the restore is erased by the restore, so
   FR-111 is unimplementable without a durable record **outside** `pb_data`. That is the restore
   journal, [D-23](./research.md#d-23), and it is the sharpest correctness problem in the phase.
2. **A restore restarts the process with `execve(path, args, os.Environ())`**
   (`core/base.go:829`) — the *current* environment. The constitution requires secrets to arrive
   with `caarlos0/env`'s `,file,unset`, which deletes them from `os.Environ()` at boot. Without
   [D-24](./research.md#d-24)'s environment snapshot, an instance comes back from disaster recovery
   with no Sentry DSN and possibly no `MEDIKUBE_PUBLIC_URL`.
3. **A half-written artifact must live in `.pb_temp_to_delete`.** `core/backup_create.go:83-89`
   excludes that directory from every backup and `core/base.go:449` deletes it on every
   `Bootstrap()`. Both edge cases the specification names — a backup taken mid-export, and a restart
   mid-export — are satisfied by choosing the right directory ([D-07](./research.md#d-07)).
4. **The count shown and the document produced come from one resolver.** SC-002 demands they agree in
   100% of cases; two code paths that must agree eventually will not ([D-44](./research.md#d-44)).
5. **An activity entry has nowhere to put content.** FR-068 constrains the *implementation*, not the
   payload: the reader performs no lookup against any other collection, and the DTO has no field
   capable of holding a name ([D-11](./research.md#d-11)).
6. **Nothing secret can enter an export**, because the archive is built from the same DTOs the API
   returns, and no DTO has a `password`, a `tokenKey` or an operator setting to give
   ([D-30](./research.md#d-30)).

---

## Technical Context

**Language/Version**: **Go 1.27** (`go 1.27` + `toolchain go1.27.x` in `go.mod`). Not the monorepo's
1.26.5 house standard: PocketBase v0.40.1's `go.mod` declares `go 1.27` and 67 non-test files import
the Go 1.27 stdlib package `encoding/json/v2`, 15 of them under `core/` and `apis/`;
`GOTOOLCHAIN=local go build` on 1.26.5 fails outright (VERIFIED-SOURCE-FACTS FACT 0). CI MUST NOT set
`GOTOOLCHAIN=local`. This phase leans on `encoding/json/v2` deliberately: the export writer streams
with `jsontext.Encoder` ([D-29](./research.md#d-29)).

**Primary Dependencies** — this phase introduces **two** new modules, and only two:

| Module | Version | Role in this phase |
|---|---|---|
| `github.com/go-pdf/fpdf` | **v0.9.0**, pinned exactly | **New.** The PDF engine, behind the `report.Renderer` port, imported by `internal/render/pdf` and nowhere else. BSD-3-Clause, pure Go, no cgo. Gated by a spike task with `github.com/signintech/gopdf` as the named fallback ([D-01](./research.md#d-01)) |
| `golang.org/x/text` | **v0.30.0**, pinned | **New as a direct dependency** (already in the module graph transitively). `unicode/bidi` for logical→visual reordering of right-to-left runs in a produced document ([D-04](./research.md#d-04)) |
| `github.com/pocketbase/pocketbase` | v0.40.1 | 2 new collections + 3 amendments as reversible Go migrations; `CreateBackup` / `RestoreBackup` / `NewBackupsFilesystem` / `StoreKeyActiveBackup`; `app.Cron()` for three jobs; `RunInTransaction` for the admin-user writes and the accept-side of a restore; `NewFilesystem()` + `fsys.Serve` for artifact and archive streaming; post-commit `OnRecordAfter*Success` hooks on `report_templates`; `RefreshTokenKey()` for disabling; `Settings().Backups.*`, `Settings().SuperuserIPs`, the superusers collection's `MFA` |
| `github.com/a-h/templ` | v0.3.1020 | 7 pages, 3 page-action fragments, the report builder, the trends picker, the operator tiles, the trail table, the archive list and every empty state |
| `github.com/starfederation/datastar-go` | v1.2.2 | Builder interactions and job-progress polling as plain `text/html` patches. **No new SSE stream** ([D-31](./research.md#d-31)) |
| `github.com/caarlos0/env/v11` | v11.4.1 | `MEDIKUBE_REPORT_*`, `MEDIKUBE_EXPORT_MAX_BYTES`, `MEDIKUBE_BACKUP_WARN_AFTER`, `MEDIKUBE_BACKUP_KEEP`, `MEDIKUBE_STATE_DIR`, and boot validation that refuses a value the instance cannot honour ([D-10](./research.md#d-10), [D-46](./research.md#d-46)) |
| `github.com/rs/zerolog` | v1.35.1 | the only logger; redacting marshallers on every new domain type |
| `github.com/getsentry/sentry-go` | v0.48.0 | errors and panics only, scrubbed |
| `github.com/prometheus/client_golang` | pinned | `medikube_exports_*`, `medikube_reports_*`, `medikube_backup_*`, `medikube_jobs_*`; every label from a bounded set |
| `go.opentelemetry.io/otel` | pinned | `service.report.*`, `service.exportjob.*`, `service.admin.*`, `store.audit.*` spans, allowlisted attributes |
| `github.com/samber/do` | v2 | container providers for the report, export-job, admin and audit-reader services and the two render adapters |
| `github.com/samber/lo` | v1.53.0 | sparingly, per Principle IV |
| `github.com/stretchr/testify` | v1.12.0 | the only assertion library |
| `github.com/spf13/cobra` | **transitive — pinned once in [001's plan](../001-walking-skeleton/plan.md#technical-context), never a direct `require`** | via PocketBase's `RootCmd`; `medikube purge` gains the artifact and audit sweeps; `medikube seed` gains reports, jobs, accounts and a million-row trail generator. The version is whatever `pocketbase@v0.40.1`'s `go.mod` requires and is not restated here (cross-artifact finding M2) |
| `modernc.org/sqlite` | v1.57.0 | transitive; pure Go, so `CGO_ENABLED=0` holds |

Stdlib does the rest of the heavy lifting: `archive/zip`, `encoding/csv`, `encoding/json/v2` +
`jsontext`, `crypto/sha256` (archive digests in the manifest), `embed` (the font faces).

**Forbidden and absent**: gin, huma, viper, `samber/mo`, `samber/ro`, `samber/slog-zerolog`, any
second router/logger/config/DI/assert library, PocketBase `jsvm`, any cgo dependency,
`datastar.WithCompression`, PocketBase's native realtime, PocketBase's file-token mechanism, the
Datastar Pro attribute set, headless Chrome, `wkhtmltopdf`, `unidoc/unipdf` (commercial), any
charting library, any second backup mechanism, any Node.js in the runtime image.

**Storage**: PocketBase-embedded SQLite (`modernc.org/sqlite`), data dir `/data/pb_data`, WAL. Two new
collections (`report_templates`, `export_jobs`), three amendments (`users` gains
`must_change_password`; `audit_events` gains ten actions and one target kind; `audit_events` gains
three paging indexes). **One new file field** — `export_jobs.artifact`, `Protected: true` — taking
the instance to three, all three named in the boot assertion ([D-08](./research.md#d-08)). **No new
`deleted_at` anywhere**: an expired artifact is deleted, not flagged ([D-42](./research.md#d-42)).
One non-database durable file, the restore journal under `MEDIKUBE_STATE_DIR`, deliberately outside
`pb_data` ([D-23](./research.md#d-23)).

**Testing**: `stretchr/testify` (`require` for preconditions, `assert` for independent assertions),
table-driven `t.Run` subtests. Seven layers, all mandatory:

- **unit** — the selection resolver, the job state machine, the retention arithmetic, the
  admin-account rules, the manifest builder, the decimation rule and the font-run splitter against
  hand-written fakes, with `t.Parallel()` and an injected `Clock`.
- **integration** — `internal/store/reporttemplate`, `internal/store/exportjob` and
  `internal/store/audit` against a throwaway `tests.NewTestApp` cloning `internal/testdata/pb_data`.
- **contract** — `reporttest.RendererContract` (real PDF renderer + fake), `exporttest.ArchiverContract`,
  `reporttemplatetest.RepositoryContract`, `audittest.ReaderContract`, `admintest.ArchivesContract`
  (the PocketBase-backed archive port + a fake), each run against **every** implementation
  (Principle II's Liskov clause).
- **HTTP** — `tests.ApiScenario` per operation, each carrying the actor matrix, plus `ExpectedEvents`
  proving the record-CRUD hooks that must not fire did not, and proving the audit reader issues no
  query against any record collection.
- **UI render** — every templ component rendered to a `bytes.Buffer`, including the operator tiles
  (definition present) and the trail row (no content).
- **artifact** — a produced PDF is re-opened and asserted on (page count, `Page N of M`, the running
  identity, the criteria sentence, the empty-section sentence, the chart's companion table); a
  produced archive is re-opened with `archive/zip` and asserted against its own manifest.
- **browser** — Playwright over the seven new pages and, in this phase, **every page phases 001–006
  deliver**, at 1440×900 and 390×844, against a populated account and an account holding nothing.

Four build-tagged suites, each with a Taskfile wrapper and a CI job:
`slowsse` ([D-32](./research.md#d-32), >10 minutes), `scale` ([D-33](./research.md#d-33)),
`phileak` ([D-34](./research.md#d-34)), `netgate` ([D-35](./research.md#d-35)).

`tests.TestApp` is **never shared across `ApiScenario` cases** (VERIFIED-SOURCE-FACTS FACT 7:
`bindUIExtensions` re-enters on every `OnServe` until the stack overflows).

**Target Platform**: one static `linux/amd64` + `linux/arm64` binary, `CGO_ENABLED=0`,
`gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, `VOLUME ["/data"]`. No Node.js in
the runtime image; Playwright is build-time only. The embedded font faces add roughly **1.4 MB** to
the binary ([D-04](./research.md#d-04)).

**Project Type**: single server-rendered Go web application; a project inside the `windkube` monorepo
at `/medikube`, image `ghcr.io/windkube/medikube`.

**Performance Goals** — the specification's success criteria are the acceptance bar, and every one of
them is asserted by the `scale` suite against the documented volumes (10,000 records, 2,000
documents, 1,000,000 activity entries, 500 readings of one measured value):

| Journey | Budget | Source |
|---|---|---|
| `/reports` builder with per-kind counts | ≤ **2 s** | SC-023 |
| a selection's resolved count as it changes | ≤ **500 ms** | SC-023 |
| a 5,000-record report produced end to end | ≤ **120 s** | SC-023 |
| a complete export of 10,000 records + 2,000 documents | ≤ **300 s**, ≤ **256 MiB** RSS | SC-005 |
| `/admin` overview | ≤ **1 s** | SC-023 |
| `/admin/audit` first page over 1,000,000 entries | ≤ **2 s** | SC-016 |
| 50 consecutive pages of the trail while it is being written | **0** repeated, **0** skipped | SC-016 |
| CSV export of 1,000,000 entries | streams, ≤ **128 MiB** RSS | SC-016 |
| acknowledging a report or export request | ≤ **2 s**, without holding the requester | SC-003 |
| ending a disabled account's sessions | ≤ **5 s** | SC-013 |
| a live view still receiving updates | > **10 continuous minutes** | SC-024 |

**Constraints**:

- No cgo, one binary, no runtime Node, no CDN fetch. **This phase makes no outbound network request
  at all**, and proves it at the socket layer ([D-35](./research.md#d-35), FR-119).
- CSP `script-src 'self' 'unsafe-eval'`; every other directive strict. The Datastar inline-script SDK
  family stays banned, so the archive download is an ordinary `<form method="post">`
  ([D-27](./research.md#d-27)) and every redirect is a `303` issued before any stream is opened.
- Records are hard deleted (constitution VII). Soft delete is files only and belongs to phase 004;
  this phase adds **no** recovery surface and **no** second purge ([D-14](./research.md#d-14)).
- PocketBase's record CRUD subtree and `/api/batch` remain unreachable to non-superusers; both new
  collections have all five API rules `nil`, asserted at boot.
- `OnRecord*Request` hooks are dead code under the lockdown; only post-commit `OnRecordAfter*Success`
  hooks are bound (`forbidigo` enforces it).
- Single instance by construction: one worker, one queue, no broker, no work-stealing pool, no
  distributed lock ([D-05](./research.md#d-05)).
- Go 1.27 `encoding/json/v2` semantics: slices marshal as `[]` never `null`, unknown fields are
  rejected (422), duplicate keys are rejected.
- **Every predicate over a nullable-looking PocketBase column uses `= ''`, never `IS NULL`** (phase
  005 D-01: `core/field_date.go:110` declares every date column `TEXT DEFAULT '' NOT NULL`). The
  `task lint:isnull` gate is extended to this phase's store packages.
- A restore is refused while any job is queued or running, and only one archive operation runs at a
  time ([D-28](./research.md#d-28), [D-21](./research.md#d-21)).
- No operator view can read a person's records; the admin service declares no port capable of it
  ([D-48](./research.md#d-48)).

**Scale/Scope**: 2 new collections (**30** running total, the figure SHARED-DESIGN §1.6 computes —
`catalog_vaccines` is dropped), 3 collection amendments, **5 migrations**, **25 new `/api/v1`
operations** — the 23 the contract allocates, plus op 91 (`exports/{id}/cancel`) and op 4
(external sign-in, claimed from the contract's formerly unowned set), bringing the product to the
**94** SHARED-DESIGN §2.3 computes — **7 new pages** (14 smoke cases at two viewports, run twice — populated and empty) plus **3 page-action
routes**, **3 new scheduled jobs** plus one job envelope wrapped around every existing job, **11 new
configuration values**, 4 new service packages, 2 new render adapter packages, 1 new platform adapter
package, 3 new store packages, and 4 build-tagged verification suites that run against the whole
application.

**No `NEEDS CLARIFICATION` items remain.** Every open question the specification raises — including
the cross-phase discrepancy it introduces about who may purge a document early — is resolved in
[research.md](./research.md) with a decision, a rationale and the rejected alternatives.

---

## Constitution Check

*GATE — evaluated before Phase 0 and re-evaluated after Phase 1 design. Both passes recorded.*

### I. Simplicity Is A Gate (KISS) — **PASS, with five tracked entries**

This is the phase where the simplicity gate earns its keep, because it is the phase with the most
opportunities to rebuild something that already exists. What was **not** built:

- **No second backup mechanism.** Seven operations, all thin wrappers ([D-21](./research.md#d-21)).
  Upstream's `create-database` / `create-files` / `create-full` split, its three cleanup variants,
  its `retention/stats`, its schedule settings and its history CSV — roughly 20 operations — are all
  deleted, and PocketBase's `CronMaxKeep` pruning is used as-is.
- **No bespoke admin UI.** `/_/` ships, hardened. Upstream's nine `/admin/models/*` routes, its
  reflection layer, both `/admin/bulk/*` routes and its generic CSV exporter are gone.
- **No `job_runs` collection.** One wrapper writes one audit row per run and two `MAX()` queries
  answer "last ran / last succeeded" ([D-43](./research.md#d-43)). Entity count stays at **30**.
- **No `users.last_login_at` column.** The trail already knows ([D-18](./research.md#d-18)).
- **No `/trash` page, no second purge, no `deleted_by` column** — phase 004 owns all of it
  ([D-14](./research.md#d-14), [D-15](./research.md#d-15)). This deletes a page, a cron, a column and
  a race from the earlier draft of this phase.
- **No charting library** — the target format draws lines ([D-02](./research.md#d-02)).
- **No new SSE stream** for job progress — a two-second poll on a page nobody leaves open
  ([D-31](./research.md#d-31)).
- **No unit conversion table**, anywhere, ever. A `Convert` function does not exist in the codebase
  and a grep test asserts it ([D-39](./research.md#d-39)).
- **No export re-import.** FR-052 makes it explicit: the supported route back is a restore.
- **No scheduled report delivery, no notification channels, no analytics** — all excluded by the spec
  and none leaves a seam behind.

Five things genuinely strain the principle and are written down in
[Complexity Tracking](#complexity-tracking): the PDF subsystem, the restore journal, the environment
snapshot, `must_change_password`, and the criteria snapshot on a job row.

### II. Interfaces At Every Seam (SOLID) — **PASS**

- **Single responsibility.** `internal/web/api/*` parses and renders; `internal/service/report`,
  `internal/service/exportjob`, `internal/service/admin` and `internal/service/audit` decide;
  `internal/store/*` persists; `internal/render/pdf` draws; `internal/render/archive` packs;
  `internal/platform/backup` talks to PocketBase's backup filesystem. The renderer knows nothing
  about authorization and the service knows nothing about PDF.
- **Open/closed.** The report's section order and the export's per-kind files are driven by the
  **kind registry**, so a sixteenth record kind in some future phase appears in reports and exports
  with no change here. Nothing in this phase switches on `kind.Kind`;
  `records.Registry.Each(...)` is the iteration.
- **Liskov.** Five contract suites, each executed against every implementation including the fakes:
  `report.Renderer`, `exportjob.Archiver`, `reporttemplate.Repository`, `audit.Reader`,
  `admin.Archives`. The `Archives` suite is the interesting one — it runs against the real
  PocketBase-backed adapter and a fake, which is what makes the restore preconditions testable
  without taking a backup in a unit test.
- **Interface segregation.** Every port is consumer-declared and small: `report.Renderer` (1),
  `report.Selection`'s two methods, `exportjob.Archiver` (1: `Write`), `exportjob.Repository` (5),
  `exportjob.Clock` (1), `admin.Counter` (2), `admin.Storage` (1), `admin.Posture` (1),
  `admin.AccountAdmin` (3), `admin.Archives` (7), `audit.Reader` (2: `Page`, `Stream`),
  `audit.Retention` (1). `admin.Archives` at seven methods **exceeds the five-method ceiling** and is
  justified in [research D-21](./research.md#d-21): the seven are exactly the seven archive
  operations the specification names, they share one lifecycle and one concurrency guard, and
  splitting them would produce three interfaces implemented by one type with one purpose. There is no
  omnibus `Store` and no omnibus `Service`.
- **Dependency inversion.** `internal/domain/**` and `internal/service/**` import neither PocketBase,
  nor `net/http`, nor templ, nor `fpdf`. The two render adapters may import `internal/web/api` for
  the DTOs ([D-30](./research.md#d-30)) and are added to the `depguard` allowlist for exactly that;
  neither imports PocketBase. `internal/platform/backup` is a new `[PB]` package.

### III. Test-First With testify (NON-NEGOTIABLE) — **PASS**

All **122** acceptance scenarios across the specification's nine stories become named failing tests
before their implementation task starts (15 + 15 + 13 + 12 + 15 + 13 + 15 + 11 + 12). FR-124 and
SC-025 demand exactly that, and `tasks.md` orders every pair test-then-implementation.

Authorization is tested as a first-class concern, and FR-128 fixes the matrix this phase must apply
to **every operation it adds**:

| Actor | Expected |
|---|---|
| stranger (no relationship to the person or the resource) | `404`, byte-identical to a non-existent id |
| the entitled account | success |
| an account whose access ended between request and use | `404` (and, for a running job, dropped from the result and named in the manifest) |
| a `view`-level grantee attempting a write | `403 forbidden_view_only` |
| an account without the administrative tier, on any operator operation | `404`, indistinguishable from the route not existing, **and an audit row** |
| unauthenticated | `401`, disclosing nothing |

Three further suites exist because the specification asks for proofs a unit test cannot give:
the artifact suite (a produced PDF and a produced archive are re-opened and asserted on), the
`phileak` sweep across every route and every job, and the >10-minute live-view test.

### IV. Idiomatic Go Over Clever Go — **PASS**

Errors are values wrapped with `%w` and inspected with `errors.Is` / `errors.As`; the job state
machine returns a typed `*exportjob.TransitionError` carrying the state it is actually in. `Patch`
types carry absent-vs-null with plain pointers. `context.Context` is first everywhere and **honoured
by the worker**, which is what makes cancellation cooperative rather than a lie
([D-41](./research.md#d-41)). The single worker goroutine has an owner (`exportjob.Runner`), a
context-bounded lifetime, and a defined shutdown path from `OnTerminate`. `panic` appears only at
startup, in the composition root — and the job envelope recovers a panic in a scheduled job and
reports it as a failure rather than taking the process down ([D-43](./research.md#d-43)). Generated
`*_templ.go` is committed, marked generated, excluded from lint and coverage.

### V. PocketBase Is The Platform, Not A Detail — **PASS**

Nothing PocketBase provides is rebuilt; the table at the top of this plan is the audit. Specifically:

- **Backup and restore** are `app.CreateBackup` and `app.RestoreBackup`, with PocketBase's own
  `StoreKeyActiveBackup` as the mutex and PocketBase's `Backups.Cron`/`CronMaxKeep` as the schedule
  and the retention. MediKube adds the preview, the safety copy, the confirmation, the authorization
  and the visibility of failures — and nothing else.
- **The admin UI ships** and is hardened, not replaced.
- **External sign-in** is `Settings().OAuth2.Providers` plus PocketBase's own `authWithOAuth2` and
  `_externalAuths` linking, wrapped in one DTO so a provider sign-in yields the same `Session`, the
  same cookie and the same audit row as a password sign-in, and so `role` and `disabled_at` are
  unreachable from the request by construction. MediKube builds no provider registry, no linking
  screen and no second configuration mechanism ([contracts/auth-oauth2.md](./contracts/auth-oauth2.md),
  FR-134 … FR-137). Claimed from the contract's unowned set — cross-artifact finding **H7**.
- **Files** — the one new file field is `Protected: true`, served only through MediKube's own route,
  with no file token ([D-08](./research.md#d-08)).
- **Scheduling** is `app.Cron()`; the three new jobs and the existing ones all go through one
  envelope.
- **Atomicity** — `app.RunInTransaction` wraps every admin-account write and the expiry sweep's
  per-row transition.
- **Cascade** — `export_jobs.owner` and `report_templates.owner` are `CascadeDelete: true`, which is
  FR-051 and FR-063 implemented by `core/record_model.go` rather than by MediKube, and proved by test.
  `report_templates.patient` is deliberately **not** cascading ([D-45](./research.md#d-45)).
- **Hooks** — only post-commit `OnRecordAfterCreateSuccess` / `…UpdateSuccess` / `…DeleteSuccess`,
  plus `OnBackupCreate` for backup outcome reporting.
- **Realtime** — PocketBase's is not used, and this phase adds no stream of its own.
- Both new collections keep all five API rules `nil`, asserted at boot and proved per collection by a
  `tests.ApiScenario` showing `/api/collections/export_jobs/records` returns `404` to a normal user.
  An `export_jobs` row is an attractive target: it names an account, a patient and an artifact.

The one place this phase **does** step outside what PocketBase offers is the restore journal, and the
reason is recorded: PocketBase's restore has no hook that survives it, because it replaces the
database and then `execve`s.

### VI. One Log Stream, One Trace Context — **PASS**

zerolog only. Every new domain type implements `MarshalZerologObject` emitting **only** opaque ids,
enum values and counts — never a template name, never a report criterion, never a file name, never an
archive path, never an error message from a storage layer. `app.Logger()` remains banned by
`forbidigo`.

Metric labels stay bounded: `medikube_exports_total{kind,outcome}`,
`medikube_exports_duration_seconds{kind}`, `medikube_exports_bytes{kind}`,
`medikube_reports_pages{}`, `medikube_jobs_runs_total{job,outcome}`,
`medikube_backup_last_success_timestamp{}`, `medikube_backup_bytes{}`, `medikube_audit_rows{}`. `job` is
the bounded job-name set from [D-43](./research.md#d-43); nothing else can become a label, and the
`phileak` sweep asserts no label value is an opaque id (FR-130).

Failure messages are the phase's other privacy surface, and FR-118 is treated as a hard rule: every
`error_code` is a bounded token (`interrupted`, `owner_unavailable`, `storage_full`,
`too_many_records`, `artifact_expired`, `archive_unreadable`, `archive_version_unsupported`,
`safety_backup_failed`, `job_in_progress`), and **no operator-facing reason names a storage
location** — the storage-full case says "storage could not accept the archive", never a path.

Datastar's `ConsoleLog`/`ConsoleError` remain banned on production paths, or the Principle VIII gate
would fight itself.

### VII. Patient Privacy Is Structural, Not Procedural — **PASS**

This is the phase that moves data **out** of the instance, so every control here is structural:

- **The artifact and the archive are `Protected: true` and reached only through an authorized
  request.** Possessing the address grants nothing (FR-013, FR-048, FR-109). No file token, ever.
- **Authorization is re-resolved when production starts**, not when it was requested, and a person
  dropped in between is named in the manifest as withdrawn ([D-09](./research.md#d-09), FR-011).
- **The archive cannot contain a secret**, because it is built from DTOs that have no field for one,
  and two tests — a byte scan of the produced archive and a reflection scan of every reachable DTO —
  keep it that way ([D-30](./research.md#d-30), SC-006).
- **The activity entry has nowhere to put content and the reader performs no lookup**
  ([D-11](./research.md#d-11), FR-068).
- **A non-administrator cannot learn that other entries exist, not even as a count**
  ([D-12](./research.md#d-12), FR-071).
- **Reading your own is not recorded; somebody else reading it is** (FR-115) — the asymmetry phase
  005 wrote and this phase finally makes readable, with the reasoning stated in the handbook so an
  absence of read entries is not mistaken for an absence of reads (FR-075).
- **No operator view is a route to a person's records** ([D-48](./research.md#d-48), FR-088).
- **The break-glass posture is detected and warned about** at boot and on every overview view until
  it is fixed ([D-17](./research.md#d-17), FR-083, SC-012).
- **Nothing dials out.** Sentry, OTLP and SMTP are off unless configured, and the `netgate` suite
  fails the build on any non-loopback dial (FR-119, SC-022).
- **The produced document states what it cannot render** rather than silently altering a person's
  name ([D-04](./research.md#d-04), FR-006) — quietly corrupting a name in a clinical document is a
  harm, and this is the one place the phase chooses honesty over completeness.
- **Purge means purge.** Once a window closes there is no code path that can bring the artifact back
  ([D-42](./research.md#d-42), FR-060).

### VIII. The UI Must Prove It Renders — **PASS**

Seven new pages, each covered at 1440×900 and 390×844 asserting `200`, the four shell landmarks, the
page's own landmark, `body[data-signals]` present, and zero console errors / page errors / failed
requests — run **twice**, once populated and once as `empty@medikube.local`
([D-40](./research.md#d-40)). The four operator pages are additionally run as a non-administrator,
asserting the shared 404 view.

This phase also owns the **whole-product sweep**: every user-facing route emitted by `medikube routes`
across phases 001–006, at both viewports, against both a populated and an empty account (FR-126,
SC-021). Two negative proofs are part of the gate, not an afterthought: a deliberately broken page
must turn the gate red and the failure must name the page, and a page added without a smoke case must
fail the build (US9 AS-3, AS-4).

Three page-action routes ([contracts/pages.md](./contracts/pages.md)) each declare the Playwright
spec that exercises them, following phase 004's precedent.

### IX. Compliance Is A Build Gate, Not A README Paragraph — **PASS**

Ten gates, all `go test` or CI steps, all failing the build:

1. `internal/openapi/gate_test.go` — registry and committed `api/openapi.json` agree on every
   `operationId`; the regenerated document is byte-identical to the committed one (FR-127).
2. `e2e/routes.gate.spec.ts` — every route emitted by `medikube routes` with `Page: true` has a smoke
   case; every `page_action` route names an existing spec that references it (FR-126).
3. `internal/service/access/coverage_test.go` — extended: every operation this phase adds that
   touches patient data has an entry in the actor matrix (FR-128).
4. `internal/store/migrations/assertions_test.go` — extended: all five rules `nil` on both new
   collections; the file-field assertion now names **three** fields; the `IS NULL` scan covers the
   new store packages.
5. `internal/service/admin/opsfig/figures_test.go` — every figure has a definition, is rendered, and
   is of a permitted unit type (FR-080, FR-086, SC-011).
6. `internal/render/archive/format_test.go` — every key `docs/export-format-v1.md` describes is
   produced and every key produced is described (FR-039).
7. `task test:phileak` — zero sentinels in logs, metrics, traces or Sentry across every route and
   every job; zero opaque-id metric labels (FR-130, SC-022).
8. `task test:netgate` — zero non-loopback dials with nothing configured (FR-119).
9. `task test:scale` — every published budget met (FR-131, SC-023).
10. golangci-lint v2 with `depguard` (extended for the three new adapter packages) and `forbidigo`,
    plus `task lint:isnull` and the `no-unit-conversion` grep.

All five migrations have a real `down` (VERIFIED-SOURCE-FACTS FACT 8 makes that structural).

### Post-Design Re-Check (after Phase 1)

Re-evaluated against `data-model.md` and `contracts/`. No principle moved from PASS to FAIL.

Four things surfaced during design and were resolved rather than tracked:

- *A `report_runs` collection separate from `export_jobs`* was considered, so that a produced document
  and a portable archive would not share a table. Rejected: FR-045 requires **one** queue across both,
  and two tables would make "at most one at a time on an instance" a lock across two collections
  rather than a property of one worker reading one ordered list. The two are distinguished by `kind`
  and presented on different pages ([D-53](./research.md#d-53)).
- *A `last_backup` column refreshed by the backup hook* was considered for the overview. Rejected as a
  denormalisation of `fsys.List("")`, which is a directory listing of at most `MEDIKUBE_BACKUP_KEEP`
  entries and is authoritative even after somebody deletes an archive by hand.
- *Resolving the patient's display name into the audit reader's DTO for administrators only* was
  considered, on the grounds that an administrator sees everything anyway. Rejected: FR-068
  constrains the reader, not the audience, and an administrator seeing a name would make the trail
  view the one operator surface that reads patient data — precisely what
  [D-48](./research.md#d-48) exists to prevent.
- *Rendering charts to PNG and embedding them* was reconsidered when the vector chart's axis-label
  code grew. Rejected again: it adds a dependency, softens print output, and duplicates the font work
  ([D-02](./research.md#d-02)).

One genuine conflict **between the specification and an earlier phase** was found and is resolved in
[D-14](./research.md#d-14): the specification's Assumptions describe a phase-004 rule
(owner-may-purge-early) that phase 004 as first written did not implement (`?purge=true` was
superuser-only). It has since been corrected **in phase 004**, which owns the mechanism — 004 FR-066
now allows the owner an early purge behind a typed confirmation and the superuser one for any
document, and refuses a share recipient with `404`. This phase changes nothing, adds no attachment
operation, and refers to 004's rule rather than restating it.

---

## Project Structure

### Documentation (this feature)

```text
specs/006-reporting-and-operations/
├── plan.md              # This file
├── research.md          # Phase 0 — 53 numbered decisions, with source evidence
├── data-model.md        # Phase 1 — 2 collections, 3 amendments, enums, state machine, migrations
├── quickstart.md        # Phase 1 — run and verify this phase by hand, end to end
├── contracts/
│   ├── README.md              # conventions, status codes, the actor matrix, audit expectations
│   ├── reports.md             # ops 69–70 — summary/selection counts, trends picker
│   ├── report-templates.md    # ops 71–75 — saved reports
│   ├── exports.md             # ops 76–79 + 91 — request, list, status, download, cancel
│   ├── audit.md               # op 68 — the trail reader and its streaming CSV
│   ├── admin-instance.md      # ops 80–81 — the operator overview and instance posture
│   ├── admin-users.md         # ops 82–83 — account administration
│   ├── admin-backups.md       # ops 84–90 — list, take, upload, preview, download, restore, delete
│   ├── auth-oauth2.md         # op 4 — external sign-in, claimed from the contract's unowned set
│   └── pages.md               # 7 pages + 3 page-action routes + the whole-product smoke sweep
├── checklists/          # pre-existing
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root `/medikube`)

Only paths this phase **creates** or **touches**. `[NEW]` = created here, `[EDIT]` = modified.

```text
medikube/
├── cmd/medikube/main.go                                      [EDIT] capture os.Environ() as the FIRST statement (D-24);
│                                                                    register the report, exportjob, admin and audit-reader
│                                                                    services, both render adapters and the backup adapter
│
├── internal/config/config.go                                 [EDIT] ReportConfig, ExportConfig, BackupConfig, StateDir,
│                                                                    and Validate() refusing an unhonourable value (D-46)
│
├── internal/domain/
│   ├── report/{definition.go,criteria.go,chart.go,settings.go}   [NEW] the report definition, its criteria, chart
│   │                                                                   selections, presentation settings, Validate()
│   ├── report/{errors.go,redact.go,*_test.go}                    [NEW] ErrNothingMatched, ErrTooManyRecords,
│   │                                                                   ErrTooManyCharts, ErrPatientUnreachable
│   ├── exportjob/{job.go,status.go,transition.go,errors.go}      [NEW] the job, its six states and the only transition table
│   ├── exportjob/{scope.go,*_test.go}                            [NEW] export scope (people, kinds, range, documents)
│   ├── retention/{window.go,window_test.go}                      [NEW] D-50 — DueAt, Expired, DaysRemaining
│   ├── adminuser/{rules.go,rules_test.go}                        [NEW] D-19 — the three refusals, as pure functions
│   └── audit/{query.go,vocab.go,*_test.go}                       [EDIT] the reader's typed narrowing + the 10 new actions
│
├── internal/service/
│   ├── report/{service.go,selection.go,trends.go,ports.go}       [NEW] D-44's one resolver; the trends picker
│   ├── report/{templates.go,document.go}                         [NEW] saved reports; building the Document the renderer draws
│   ├── report/reporttest/{fake.go,contract.go,fixtures.go}       [NEW] RendererContract + fakes
│   ├── exportjob/{runner.go,queue.go,reconcile.go,cancel.go}     [NEW] D-05, D-06, D-41 — one worker, one queue
│   ├── exportjob/{export.go,ports.go,purge.go}                   [NEW] the export build; the artifact expiry sweep (D-42)
│   ├── exportjob/exporttest/{fake.go,contract.go}                [NEW] ArchiverContract + fakes
│   ├── admin/{stats.go,system.go,accounts.go,archives.go}        [NEW] the operator surface
│   ├── admin/opsfig/{figures.go,figures_test.go}                 [NEW] D-47 — the figure catalogue and its gate
│   ├── admin/{ports.go,admintest/{fake.go,contract.go}}          [NEW] Counter, Storage, Posture, AccountAdmin, Archives
│   ├── audit/{reader.go,csv.go,retention.go}                     [NEW] the scoped reader, the streaming CSV, the purge
│   ├── audit/{writer.go,audittest/contract.go}                   [EDIT] the 10 new actions; ReaderContract
│   └── access/authorizer.go                                      [EDIT] admin-tier resolution; the operator-route rule
│
├── internal/render/                                          [NEW package family — adapters, no PocketBase]
│   ├── pdf/{renderer.go,layout.go,cover.go,section.go,chart.go}  [NEW] D-01, D-02, D-37
│   ├── pdf/{fontrun.go,bidi.go,unrenderable.go}                  [NEW] D-04
│   ├── pdf/fonts/{embed.go,NotoSans*.ttf}                        [NEW] ~1.4 MB embedded
│   ├── pdf/{renderer_test.go,chart_test.go,golden/}              [NEW] re-open the produced PDF and assert on it
│   ├── archive/{writer.go,manifest.go,csv.go,documents.go}       [NEW] D-29
│   └── archive/{writer_test.go,format_test.go}                   [NEW] the format-document gate
│
├── internal/store/
│   ├── migrations/1757xxx100_report_templates.go              [NEW]
│   ├── migrations/1757xxx200_export_jobs.go                   [NEW]
│   ├── migrations/1757xxx300_users_must_change_password.go    [NEW]
│   ├── migrations/1757xxx400_audit_vocab_ops.go               [NEW]
│   ├── migrations/assertions.go                               [EDIT] 3 file fields; nil rules on both new collections
│   ├── reporttemplate/{repo.go,mapper.go,repo_test.go}        [NEW]
│   ├── exportjob/{repo.go,mapper.go,queue.go,repo_test.go}    [NEW] the queue query and the position COUNT
│   ├── audit/{reader.go,cursor.go,reader_test.go}             [NEW] D-52's keyset paging
│   └── stats/{counter.go,counter_test.go}                     [NEW] the indexed COUNTs behind admin.Counter
│
├── internal/platform/
│   ├── pb/jobs.go(+_test)                                     [NEW] D-43 — the scheduled-job envelope
│   ├── pb/cron.go                                             [EDIT] register the 3 new jobs; route 004's and 005's
│   │                                                                 existing jobs through the envelope
│   ├── pb/boot.go                                             [EDIT] medikube.json (D-25); the restore-journal replay (D-23);
│   │                                                                 the MFA/IP warning restated on every overview view
│   ├── pb/storage.go                                          [NEW] the storage-footprint walk behind admin.Storage
│   ├── pb/posture.go                                          [NEW] MFA, SuperuserIPs, SMTP, migration state (D-17)
│   ├── pb/hooks.go                                            [EDIT] post-commit audit hooks for report_templates;
│   │                                                                 OnBackupCreate outcome reporting
│   └── backup/{archives.go,preview.go,journal.go,env.go}      [NEW] admin.Archives over PocketBase (D-21..D-27)   [PB]
│
├── internal/httproute/routes.go                               [EDIT] 25 API (incl. op 4) + 7 page + 3 page-action entries
│
├── internal/web/
│   ├── api/oauth2.go(+_test)                                  [NEW] op 4 — external sign-in (H7)
│   ├── api/reports.go(+_test,+_http_test)                     [NEW] ops 69–70
│   ├── api/report_templates.go(+_test,+_http_test)            [NEW] ops 71–75
│   ├── api/exports.go(+_test,+_http_test)                     [NEW] ops 76–79, 91
│   ├── api/audit.go(+_test,+_http_test)                       [NEW] op 68 incl. the CSV branch
│   ├── api/admin_instance.go(+_test,+_http_test)              [NEW] ops 80–81
│   ├── api/admin_users.go(+_test,+_http_test)                 [NEW] ops 82–83
│   ├── api/admin_backups.go(+_test,+_http_test)               [NEW] ops 84–90
│   ├── api/dto_reporting.go(+_test)                           [NEW] every DTO this phase adds, with round-trip tests
│   ├── errors.go                                              [EDIT] artifact_expired, job_in_progress, duplicate_name,
│   │                                                                 password_change_required, account_disabled
│   ├── middleware/mustchange.go(+_test)                       [NEW] D-20's request-wide gate
│   ├── page/{reports.go,exports.go,admin.go,admin_audit.go}   [NEW] 4 page handlers
│   ├── page/{admin_users.go,admin_backups.go}                 [NEW] 2 page handlers
│   ├── page/fragments_reporting.go                            [NEW] the 3 page-action fragments
│   └── views/
│       ├── reports/{builder.templ,counts.templ,charts.templ}          [NEW]
│       ├── reports/{templates.templ,editor.templ,produced.templ}      [NEW]
│       ├── exports/{list.templ,row.templ,request.templ}               [NEW]
│       ├── admin/{overview.templ,figure.templ,warning.templ}          [NEW]
│       ├── admin/{audit.templ,auditrow.templ,filters.templ}           [NEW]
│       ├── admin/{users.templ,userrow.templ,confirm.templ}            [NEW]
│       ├── admin/{backups.templ,preview.templ,restoreconfirm.templ}   [NEW]
│       ├── shell/layout.templ                                          [EDIT] the operator nav group, admin-only
│       └── ids/ids.go                                                  [EDIT] deterministic ids for the new components
│
├── internal/cli/{seed.go,purge.go,routes.go}                  [EDIT] seed reports/jobs/accounts/trail; purge runs the sweeps
├── internal/testsupport/
│   ├── phileak/exercise.go                                    [EDIT] extend to every route and every job (D-34)
│   ├── netgate/dialer.go                                      [NEW]  D-35
│   ├── scale/generate.go                                      [EDIT] the documented volumes (D-33)
│   └── artifact/{pdf.go,zip.go}                               [NEW]  re-open and assert helpers
│
├── docs/export-format-v1.md                                   [NEW] the published, versioned format (FR-039)
├── docs/operator-handbook.md                                  [NEW] FR-133
├── docs/privacy-review-checklist.md                           [NEW] D-36
├── docs/privacy-review.md                                     [NEW] D-36 — the written result
├── api/openapi.json                                           [EDIT] regenerated, committed, diffed
├── e2e/specs/{reports,exports,admin,admin-audit,admin-users,admin-backups}.spec.ts   [NEW]
├── e2e/specs/{full-sweep,empty-account,operator-denied}.spec.ts                      [NEW]
├── e2e/routes.gate.spec.ts                                    [EDIT] 7 page routes + 3 page-action routes
├── .golangci.yml                                              [EDIT] depguard for internal/render/** and internal/platform/backup
├── Taskfile.yaml                                              [EDIT] test:scale, test:netgate, test:slowsse, lint:noconvert
└── .github/workflows/*.yaml                                   [EDIT] the four build-tagged CI jobs
```

**Structure Decision**: the single-project Go layout from phase 001 is unchanged. Three packages are
added to it, all adapters, all for the same reason: something in this phase must talk to a library or
a platform that `internal/service/**` may not import. `internal/render/pdf` owns `fpdf`,
`internal/render/archive` owns `archive/zip` and the DTO encoders, and `internal/platform/backup`
owns PocketBase's backup filesystem. Each sits behind a consumer-declared port with a contract suite,
which is what makes [D-01](./research.md#d-01)'s "if `fpdf` fails the spike, only one package
changes" a true statement rather than a hope.

---

## Deviations from the shared design contract

The contract is binding on **design**. Eight departures, each named here because the contract's own
headline numbers move:

| Contract says | This plan does | Why |
|---|---|---|
| Phase 006 had **8 pages**, including `/trash` | **7 pages**; `/trash` is removed, and the shared design contract has been amended to match | The specification forbids a second surface for recovering deleted documents in as many words, and phase 004 already ships `/documents?deleted=true` with listing, restore, days-remaining and the early purge. SHARED-DESIGN §3.1 records the removal, so the page inventory and the Playwright gate stay one list; the suite-wide page total is computed there (**58** + 3 error views) and cited, never re-derived. [D-14](./research.md#d-14), [D-15](./research.md#d-15) |
| Phase 006 has **23 operations** (68–90) | **25** — adds `POST /api/v1/exports/{id}/cancel` (op 91) and **claims op 4, `POST /api/v1/auth/oauth2`** | FR-046 and US2 AS-11 require a queued or running request to be cancellable. `DELETE` would be a lie: the row survives as `cancelled`. [D-41](./research.md#d-41) |
| Contract op **4** (`POST /api/v1/auth/oauth2`, external sign-in) is "deferred out of the suite" and belongs to no phase (§2.3) | **This phase owns it** (FR-134 … FR-137, `contracts/auth-oauth2.md`) | Cross-artifact finding **H7**: phase 001 recorded the drop and no later phase claimed it, so a contract operation belonged to nobody. External sign-in is a deployment integration, not a day-one auth flow, and it needs provider configuration that this phase's operator surface already exposes — while password recovery and email confirmation, which are day-one flows, went the other way and landed in phase 001. PocketBase's `authWithOAuth2` and its `_externalAuths` linking do the work; this phase adds one DTO wrapper, one posture figure and their tests. **SHARED-DESIGN §2.3 has been amended to match**, and op 4 now appears in this phase's operation list there. |
| Op 88 is `GET /api/v1/admin/backups/{name}/download` | Same path, **`POST`** | FR-109 requires password re-entry **and** per-request authorization **and** no credential in the URL. A `GET` cannot carry a password without one of the two forbidden mechanisms, and the inline-script SDK family is banned so a `fetch`-plus-blob download is unavailable. [D-27](./research.md#d-27) |
| `users` does not carry `must_change_password` | **Added** (bool, default false) | FR-093 requires the account to be *blocked* until it sets a new password — persisted state, not an email. §2.3's own op 83 already says "forced password reset", so only the field list omitted it. [D-20](./research.md#d-20) |
| `export_jobs.status ∈ {queued, running, succeeded, failed, expired}` | **+ `cancelled`** | FR-046. US2 AS-11 names the four terminal states explicitly: finished, failed, cancelled, expired |
| `export_jobs.format ∈ {json, csv, pdf, zip}` | **`{pdf, zip}`**, with JSON and CSV as *contents* of the zip, selected by two booleans on `options` | FR-037: an export is *one archive* that may additionally contain tabular files. `json` and `csv` were never whole-artifact formats; treating them as such would make "the archive" ambiguous |
| `report_templates.criteria` carries the patient inside its JSON | **`patient` is a real relation**, `CascadeDelete: false`, plus a unique `(owner, LOWER(name))` index | FR-032 requires a saved report to survive the person being deleted and say so; a non-cascading relation is *emptied* rather than deleted. FR-030 requires case-insensitive name uniqueness, which is an index or it is a convention. [D-45](./research.md#d-45) |
| `audit_events` vocabulary as listed | **+10 actions**, **+1 `target_kind`** (`report_template`) | FR-116 and FR-098 require producing, downloading, cancelling, trail-exporting, every administrative action, every archive operation, every refusal and every scheduled-job outcome to be recorded and distinguishable. Listed in [data-model.md](./data-model.md) §4 |

Additive and worth stating, though not a contradiction: three packages the contract's §4 layout does
not name — `internal/render/pdf`, `internal/render/archive`, `internal/platform/backup` — are added,
each an adapter behind a port, each on the `depguard` allowlist for exactly one reason.

Everything else follows the contract verbatim: `report_templates` and `export_jobs` as the two new
collections (30 total, SHARED-DESIGN §1.6), ops 68–90 with their paths and semantics, criteria-not-frozen-ids, the audit
event shape, the one-endpoint audit reader, the thin backup wrappers, the `/api/v1` conventions,
cursor pagination, the error envelope and the 404-not-403 rule.

---

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| **A PDF subsystem — `internal/render/pdf`: a new third-party engine, ~1.4 MB of embedded font faces, a hand-written vector chart renderer, a script-run splitter and a bidi pass — roughly 1,500 lines, in the phase whose whole theme is not building things** (Principle I) | FR-006 requires a *single self-contained document that opens and prints on any device without the application*, with numbered pages and the person identified on **every** page. That is paged media, and nothing in the existing stack produces it: templ renders HTML, PocketBase serves bytes, and Datastar patches a DOM. The font work is not gold-plating either — FR-006 forbids silently dropping or substituting a character, and PDF core fonts are WinAnsi, so an unembedded stack mangles every non-Latin name without saying so. The blast radius is bounded by construction: `fpdf` is imported by exactly one package, behind a one-method port with a contract suite and a fake, so the named fallback (`signintech/gopdf`) is a one-package change. | **(a) Emit a self-contained HTML file and let the browser print it.** Genuinely tempting — no dependency, reuses templ, trivially "readable without the application". It fails FR-006: repeating headers and page numbers depend on CSS paged media, which browsers implement differently and some not at all, so two clinicians would get two different documents from one file. **(b) Headless Chrome / `wkhtmltopdf`.** A browser binary in `gcr.io/distroless/static-debian12`, which has no shell and no libc. Forbidden by the Technology Constraints. **(c) `unidoc/unipdf`.** Commercial per-deployment licence, on an application somebody runs in a cupboard. **(d) `maroto/v2`.** A grid DSL over the same engine: an extra dependency and an extra layer for a layout requirement nobody stated. **(e) Ship no document, only the export.** The export is US2; US1 is the reason anybody maintains these records at all. |
| **A second durable store — the restore journal, a JSON file under `MEDIKUBE_STATE_DIR`, outside `pb_data` and outside the database** (Principle I: one store; Principle V: PocketBase owns persistence) | FR-111 requires the account of a restore — the restore, the safety copy and both references — to be readable **after** the restore. `core/backup_restore.go:30-46` replaces the entire `pb_data` directory, so an audit row written before the restore is destroyed *by* the restore. There is no PocketBase hook that survives it, because the call ends in `app.Restart()` → `execve`. The only durable places are inside `pb_data` (erased), `pb_data/backups` (which is the operator's archive listing) and outside `pb_data`. The journal is ~200 bytes, is written immediately before the call, is replayed into the restored database on the next `OnBootstrap`, and is then deleted. It also answers the "restarted during a restore" edge case, because its presence with an unchanged database is exactly the interrupted state. | **(a) Write the audit rows before the restore.** The seductive wrong answer: the restore erases them. **(b) Put the journal in `pb_data/backups`.** It survives, and it then appears in `fsys.List("")` forever, so MediKube would have to filter its own dotfile out of every operator archive listing. **(c) Re-derive the restore at boot from the archive.** An archive does not record that it was restored. **(d) Accept the loss and document it.** The most destructive action the instance offers would be the one action with no record, which FR-111 forbids and US7 AS-15 tests. |
| **A package-level environment snapshot captured in `main()` before anything else runs, and re-applied with `os.Setenv` immediately before a restore** (Principle IV: no package-level globals; Principle I: three lines of subtle machinery) | Two individually harmless facts compose into a production outage: `core/base.go:829` restarts with `execve(path, args, os.Environ())` — the *current* environment — and the constitution requires secrets to arrive via `,file,unset`, which deletes them from `os.Environ()` at boot. So a restore restarts the process with no Sentry DSN, no OTLP headers and possibly no `MEDIKUBE_PUBLIC_URL`, **immediately after a disaster recovery**. This was found by reading `Restart`, not by testing, because any test shorter than a real restore never exercises it. The global is unavoidable: the value must be captured before `config.Load()` runs, and there is nothing else alive at that point to hold it. | **(a) Drop `,file,unset`.** Weakens a constitution-mandated control to work around a framework detail — and the control exists so a secret does not sit in `/proc/self/environ` for the process lifetime. **(b) Fork `Restart` to pass a saved environment.** Requires forking PocketBase for a three-line workaround. **(c) Rely on the container runtime to re-inject the environment.** `execve` replaces the process image; the container never restarts, so the runtime never gets the chance. **(d) Pass the snapshot through the DI container.** It must be captured before the container exists. |
| **`users.must_change_password` — a new column plus a request-wide gate that refuses every route except one**, against Principle I (a flag that changes global behaviour) and the shared design contract's explicit "not carried over" | FR-093 requires that an account required to set a new password "can reach the password change and nothing else until it has", and that both the requirement and its clearing are recorded. That is persisted state on the account, checked on every request; there is no way to express "blocked until" without both halves. The gate is one middleware with one predicate and an allowlist of exactly two routes, and it is tested per route family from the route registry so a new route cannot escape it by being new. | **(a) Send a password-reset email instead.** Does not block sign-in, so it fails the requirement in its own words; and it depends on SMTP, which a self-hosted instance may not have. **(b) Disable the account and tell the person to ask the operator.** A blunter, different action that FR-090 already covers, and it destroys the person's sessions rather than redirecting them. **(c) Expire the session and force re-authentication.** They would sign in with the same password, which is the thing being replaced. |
| **`export_jobs.criteria` snapshots the definition at request time, duplicating what `report_templates` already holds** (Principle I: one source of truth) | The concurrency edge case is explicit: *"A report is being produced while the saved report it came from is edited: the document reflects the criteria as they were when production started, and says so on its first page."* A job that read its template at dequeue time would produce a document whose first page states criteria that are no longer the template's, or — worse — that silently changed between the count the person saw and the document they got, which breaks SC-002. The snapshot is also what makes an **expired** job re-runnable (FR-047) after its template has been deleted, which FR-034 requires ("deleting a saved report must not affect a document already produced"). It is a snapshot, not a cache: it is written once and never refreshed, so it cannot drift. | **(a) Store only the template id and read it at dequeue.** Fails the edge case above and makes a deleted template break re-production. **(b) Freeze the resolved record ids instead.** That is precisely the upstream defect this phase exists to not repeat — a saved report that rots — and it would also break FR-011's "re-checked at the moment production began". **(c) Version the template and reference a version.** A whole versioning subsystem, with its own retention, to avoid one JSON column on a row that is deleted in seven days. |

---

## Phase Exit Criteria

**Criterion 0, before any of the below**: `specs/006-reporting-and-operations/traceability.md` (T248a) joins every functional requirement to the task ids that satisfy it, every acceptance scenario to its test and every success criterion to its task or to a criterion below, with no empty row. The join is mechanical, not a prose claim: an unmapped requirement, or a success criterion neither mapped nor marked `[outcome metric]` in `spec.md`, fails the phase (cross-artifact finding M7).

This phase — and, because it is the last, the product — is done when, and only when:

1. All **122** acceptance scenarios exist as named automated tests and pass (FR-124, SC-025).
2. Every operation this phase adds carries the FR-128 actor matrix, and
   `internal/service/access/coverage_test.go` fails the build if one is missing.
3. A produced document is re-opened and proved to carry numbered pages, a running identity, its
   criteria, an explicit empty-section sentence and a companion table beside every chart; a produced
   archive is re-opened with nothing of MediKube running and proved to match its own manifest.
4. The `phileak` sweep reports zero sentinels across logs, metrics, traces and Sentry over **every
   route and every job in the application**, and zero opaque-id metric labels (FR-130, SC-022).
5. The `netgate` suite reports zero non-loopback dials with nothing configured (FR-119).
6. The `scale` suite meets **every** published budget at the documented volumes, and 50 pages of the
   trail under concurrent writes repeat 0 and skip 0 (FR-131, SC-016, SC-023).
7. The `slowsse` job proves a live view still delivering after **11 minutes** (FR-129, SC-024).
8. The Playwright gate is green over **every user-facing page delivered by phases 001–006** at both
   viewports, against both a populated account and an account holding nothing — and is proved to go
   **red** for a deliberately broken page and for a page added without a smoke case (FR-125, FR-126,
   SC-021).
9. `api/openapi.json` is regenerated, committed and diff-clean; the route-inventory gate passes with
   all 24 new `operationId`s present in both the registry and the document (FR-127).
10. A restore is exercised end to end on a real instance: the safety copy is taken first and reported,
    the instance returns on the archive's data, and the restore, the safety copy and both references
    are readable in the trail **afterwards** (FR-111, SC-017, SC-018).
11. `docs/export-format-v1.md`, `docs/operator-handbook.md`, `docs/privacy-review-checklist.md` and
    `docs/privacy-review.md` are published, and the handbook has been followed end to end by somebody
    who has not seen the application, from a clean machine to a backed-up and restored instance, with
    zero questions asked (FR-132, FR-133, SC-026, SC-027).
12. CI is green: format, vet, golangci-lint v2, `go test -race -count=1 ./...`, the OpenAPI and route
    gates, the four build-tagged jobs, the container build, and the browser gate.
