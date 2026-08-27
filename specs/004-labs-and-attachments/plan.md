# Implementation Plan: Labs and Attachments

**Branch**: `004-labs-and-attachments` | **Date**: 2026-08-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-labs-and-attachments/spec.md`

**Constitution**: [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.3.0 (binding)

**Shared design contract**: [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) (binding on design; see
[Deviations](#deviations-from-the-shared-design-contract) for the four places this phase departs
from its *phase table*, and the one place it departs from a route's shape)

---

## Summary

This phase adds the two things MediKube's chart is missing and cannot fake: **a laboratory history
whose individual lines are real, comparable values**, and **the paperwork that hangs off every
record in the application**.

It does that with **one new record kind, one child collection, one read-only catalogue, one file
collection, and nine `/api/v1` operations**. Lab results are a `kind` in the registry phase 001
built and phase 003 filled, so they add **zero record routes**; their components are a validated
array inside the lab result's own payload with replace-set semantics, so they add none either.
Trending is derived — nothing about a series is stored apart from the readings it is computed
from, so correcting a reading corrects the comparison. Attachments are the first collection since
`patients.photo` to hold bytes, which makes Constitution VII's file rules — `Protected: true`,
MediKube-owned serving routes, no file tokens, eager thumbnails — load-bearing for the first time at
scale.

The technical spine, stated plainly:

- **Lab results ride the existing six-operation record family.** `records.Register(kind.LabResult, …)`
  wires the service, the DTO codec, the templ components, the audit hook, the search-index hook,
  the realtime publish hook, the attachment-cleanup hook, two pages and two smoke cases in one
  call. Nothing in this phase switches on `kind.Kind`.
- **Components are a set, not a resource.** A lab result's `PATCH` carries the complete component
  array; the service diffs it by id inside `app.RunInTransaction`, creating what is new, updating
  what changed and deleting what was omitted. Component ids are stable across saves because they
  are the opaque ids the trend view and the audit trail refer to.
- **Trend arithmetic is pure domain code.** `internal/domain/labs` owns the range classifier and
  the series statistics, including FR-031's halves rule verbatim. It imports nothing but the
  standard library, so every one of US3's acceptance scenarios is a table-driven unit test with no
  database in sight.
- **No value is ever converted between units, anywhere.** This is enforced structurally: a
  `depguard` rule forbids `internal/domain/labs` and `internal/service/lab*` from importing phase
  003's `internal/domain/clinical/units` package. A conversion cannot be added by accident.
- **Attachments are patient-anchored and polymorphic.** `owner_kind` + `owner_id` carries no
  foreign key, and the cleanup hook that keeps it honest is registered by `records.Register`
  itself, so a record kind added in a future phase inherits document support without knowing
  documents exist (FR-049).
- **Files never leave through PocketBase.** `attachments.file` is `Protected: true`; the boot
  assertion that already covers `patients.photo` now covers two fields; content is streamed from
  MediKube's own `/api/v1/attachments/{id}` through `app.NewFilesystem()`, authorized by the service,
  audited on every success, and never addressed by a token in a URL.
- **One thing in this phase is deliberately not Datastar.** A 32 MiB file cannot go through
  `data-bind` on a file input: Datastar v1 shapes that signal as `{name, contents, mime}[]` with
  base64 contents, which is 43 MiB of JSON in browser memory and over the request body limit. The
  upload form is therefore a native multipart form post to a page-layer action route. That is the
  single transport exception in the application and it is recorded in Complexity Tracking.

**Nine operations, four pages, four page-action routes, four collections.** Phase 004 is the
phase where MediKube starts holding files, and the whole plan is arranged around not getting that
wrong.

---

## Technical Context

**Language/Version**: **Go 1.27** (`go 1.27` plus a `toolchain go1.27.x` line in `go.mod`). Not the
monorepo's 1.26.5 house standard, and the divergence is forced rather than stylistic: PocketBase
v0.40.1 declares `go 1.27` and imports the Go 1.27 stdlib package `encoding/json/v2` in 67 non-test
files, 15 of them under `core/` and `apis/`. `GOTOOLCHAIN=local go build` on 1.26.5 fails outright
(VERIFIED-SOURCE-FACTS FACT 0). CI MUST NOT set `GOTOOLCHAIN=local`.

**Primary Dependencies** (all pinned; **this phase introduces no new module**):

| Module | Version | Role in this phase |
|---|---|---|
| `github.com/pocketbase/pocketbase` | v0.40.1 | 4 new collections as reversible Go migrations; `app.NewFilesystem()` for file storage, `fsys.Serve` for streaming with Range/ETag, `fsys.CreateThumb` for eager previews; `RunInTransaction` for the component replace-set and the replace-document flow; `TxInfo().OnComplete` for post-commit thumbnailing; `app.Cron()` for the retention purge; `app.DB()` for the two analytics queries; `tests.ApiScenario` |
| `github.com/a-h/templ` | v0.3.1020 | lab result row/list/detail, the component table, the trend page and its inline-SVG chart, the document library, the upload form, the catalogue suggestion listbox |
| `github.com/starfederation/datastar-go` | v1.2.2 | catalogue autocomplete, component-row add/remove, filter and paging fragments, live lab-result lists on the existing `/api/v1/streams/records`. **Not** used for the file upload itself (research D-24) |
| `github.com/caarlos0/env/v11` | v11.4.1 | one new knob, `MEDIKUBE_LABS_MAX_SERIES_POINTS`; reuses `MEDIKUBE_FILES_*` and `MEDIKUBE_RETENTION_TRASH_DAYS` from phase 001 |
| `github.com/rs/zerolog` | v1.35.1 | the only logger; PHI-redacting `MarshalZerologObject` on `LabResult`, `Component`, `Attachment` |
| `github.com/getsentry/sentry-go` | v0.48.0 | errors and panics only, scrubbed; a storage write failure reports a code, never a path or a file name |
| `github.com/prometheus/client_golang` | latest pinned | `medikube_files_*` and `medikube_labs_*`; label sets bounded, **no patient id and no file name ever becomes a label** |
| `go.opentelemetry.io/otel` | latest pinned | `service.attachment.*`, `service.labresult.*`, `store.lab_components.*` spans |
| `github.com/samber/do` | v2 | container providers for the four new services |
| `github.com/samber/lo` | v1.53.0 | sparingly, per Principle IV |
| `github.com/stretchr/testify` | v1.12.0 | the only assertion library |
| `github.com/spf13/cobra` | **transitive — pinned once in [001's plan](../001-walking-skeleton/plan.md#technical-context), never a direct `require`** | via PocketBase's `RootCmd`; `medikube seed` gains lab results, components and documents; `medikube purge` gains the trash sweep. The version is whatever `pocketbase@v0.40.1`'s `go.mod` requires and is not restated here (cross-artifact finding M2) |
| `modernc.org/sqlite` | v1.57.0 | transitive via PocketBase; pure Go, so `CGO_ENABLED=0` holds |

**Forbidden and absent**: gin, huma, viper, `samber/mo`, `samber/ro`, `samber/slog-zerolog`, any
second router/logger/config/DI/assert library, PocketBase `jsvm`, any cgo dependency,
`datastar.WithCompression`, the Datastar Pro attribute set, **any charting library**, **any image
or PDF processing dependency beyond what PocketBase already vendors**, and **any MIME-detection
dependency** (research D-14 explains why the standard library plus a 9-entry magic table is the
right size for this).

**Storage**: PocketBase-embedded SQLite (`modernc.org/sqlite`), data dir `/data/pb_data`, WAL. Four
new collections — `catalog_lab_tests`, `lab_results`, `lab_components`, `attachments`. All five API
rules `nil` on every one of them. **One new file field**, `attachments.file`, `Protected: true`,
bringing the boot assertion's coverage from one field to two. Document bytes live under
`<dataDir>/storage` (or the operator's S3 bucket) and are reached only through
`app.NewFilesystem()`.

**Testing**: `stretchr/testify` (`require` for preconditions, `assert` for independent assertions),
table-driven `t.Run` subtests. Six layers, all mandatory:

- **unit** — `internal/domain/labs` (classification, statistics, the halves rule, normalisation),
  `internal/service/**` against hand-written fakes. `t.Parallel()` throughout, no database.
- **integration** — repositories and the file adapter against a throwaway `tests.NewTestApp`
  cloning `internal/testdata/pb_data`. Never shared across `ApiScenario` cases
  (VERIFIED-SOURCE-FACTS FACT 7).
- **contract** — phase 003's `recordstest.RepositoryContract` and `recordstest.KindContract` run
  against `lab_result`; a new `attachmenttest.StoreContract` runs against the real store and its
  fake.
- **HTTP** — `tests.ApiScenario` for all nine operations, with `ExpectedEvents` asserting the
  audit and hook counts, and an authorization matrix per operation.
- **UI render** — templ components rendered to a `bytes.Buffer` and asserted on, including the
  inline-SVG chart's `role="img"`, its accessible name and its non-colour out-of-range marking.
- **browser** — Playwright CLI over the four new pages at 1440×900 and 390×844, plus
  behaviour specs for upload, download, inline view, replace, delete, restore, trend selection and
  catalogue autocomplete.

Plus two build-tagged suites inherited from phase 003 and extended here: `test:scale` (FR-085) and
`test:phileak` (FR-078/079, SC-008).

**Target Platform**: one static `linux/amd64` + `linux/arm64` binary, `CGO_ENABLED=0`,
`gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, `VOLUME ["/data"]`. No Node.js in
the runtime image.

**Project Type**: single server-rendered Go web application, a project inside the monorepo at
`/medikube`, image `ghcr.io/windkube/medikube`.

**Performance Goals** (from the spec's success criteria — these are the acceptance bar, not
aspirations):

- **SC-011 / FR-085**: 5,000 lab results for one person — every page of the list within **2 s**,
  and any one result locatable within 10 s using only ordering and narrowing. Delivered by the
  derived `sort_date` column plus the `(patient, sort_date DESC, id DESC)` index and HMAC-signed
  keyset cursors.
- **SC-003**: a component with 100 readings across 50 results presents its series and summary
  within **2 s**, and two units are never one series.
- **FR-085**: a single panel of 100 components displays as one page without silent truncation.
- **FR-085**: 2,000 documents for one person page, narrow and sort without degrading.
- **SC-014**: catalogue suggestions within **1 s** of the third character.
- **SC-001**: a ten-component panel enterable in under 3 minutes.
- **SC-007**: a deleted document restored in under 30 seconds.
- A 32 MiB upload on a slow connection must not be killed by an invisible timeout — PocketBase's
  hardcoded 5-minute `WriteTimeout` bounds slow *uploads* as well as SSE streams, and phase 001's
  `ServeEvent` override is what makes that survivable (research D-33).

**Constraints**:

- No cgo, one binary, no runtime Node, no CDN fetch, no outbound request the operator did not
  configure. **Nothing in this phase talks to anything** — in particular
  `filesystem.NewFileFromURL` is never called, so the SSRF sink Constitution VII names is not
  present.
- CSP: the application's own pages keep `default-src 'self'`, `script-src 'self' 'unsafe-eval'`,
  no `unsafe-inline`, no external origins, `frame-ancestors 'none'`, `object-src 'none'`,
  `base-uri 'self'`, and **gain `frame-src 'self'`** so the inline document viewer can frame
  MediKube's own attachment route. Attachment responses carry their own, much tighter CSP
  (research D-16).
- Records are **hard** deleted. `deleted_at` exists on `attachments` and on nothing else — that is
  Constitution VII's files-only soft delete, landing here for the first time.
- PocketBase's record CRUD subtree and `/api/batch` remain unreachable to non-superusers.
- `OnRecord*Request` hooks are dead code under the lockdown; only `OnRecord*Execute` and the
  post-commit `…AfterCreateSuccess` / `…AfterUpdateSuccess` / `…AfterDeleteSuccess` hooks are used.
- Single instance by construction. The purge is one `app.Cron()` entry, not a queue.
- Go 1.27 `encoding/json/v2` semantics apply to every new DTO: slices marshal as `[]` never `null`,
  unknown request fields are rejected (422), duplicate keys are rejected.
- The Playwright gate asserts **zero failed network requests**, which is why `has_preview` is a
  stored column rather than a guess from the MIME type (research D-17).

**Scale/Scope**: 1 new record kind (15 registered after this phase), 4 new collections (**26** running
total), **9 new `/api/v1` operations**, 4 new pages (8 smoke cases at two viewports), 4 new
page-action routes, 1 new scheduled job, 1 new configuration knob, ~100,000 component rows and
2,000 documents per patient as the performance target.

**No `NEEDS CLARIFICATION` items remain.** Everything the specification left open is resolved in
[research.md](./research.md) as a numbered decision.

---

## Constitution Check

*GATE — evaluated before Phase 0 and re-evaluated after Phase 1 design. Both passes recorded.*

### I. Simplicity Is A Gate (KISS) — **PASS, with four tracked entries**

The phase's headline simplification is that **the two biggest features add almost no API surface**.
Lab results are a record kind: zero new record routes. Components are an array in their parent's
payload: zero routes, no CRUD, no bulk-create endpoint. Upstream spent 22 operations on lab
results, 20 on lab test components and 34 across two parallel file subsystems — 76 operations for
what this phase does in nine.

Explicit YAGNI decisions taken and recorded:

- **No document version history.** Replace keeps exactly one prior copy, for exactly the retention
  window, and the spec says so.
- **No per-account or per-person storage quota.** Storage is reported, not rationed.
- **No OCR, no document parsing, no external DMS sync.** Out of scope for MediKube entirely.
- **No unit conversion table.** Not deferred — forbidden, and enforced by a lint rule.
- **No `deleted_reason` column.** Nothing in the spec filters on why a document was trashed.
- **No document ordering column.** Listings are chronological, which is what FR-069 asks for.
- **No separate metadata `GET` for one attachment.** The list DTO already carries everything the
  library and the record strip render, and the shared contract's op 51 is the content stream.
- **No realtime for documents.** No requirement asks for it; lab results ride the existing stream.
- **No attachment rows in `search_index`.** No requirement asks for cross-kind document search.
- **No `/new` and no `/edit` routes.** Drawer state is a Datastar signal, as in every prior phase.
- **No broker abstraction, no queue, no worker pool.** The purge is a cron function.

Four things strain this principle enough to be recorded in
[Complexity Tracking](#complexity-tracking): three derived columns on the lab and attachment
collections, the polymorphic attachment ownership, the two hand-written analytics queries, and the
`KindPageAction` route class the multipart upload and the HTML fragments require.

### II. Interfaces At Every Seam (SOLID) — **PASS**

- **Single responsibility.** `internal/web/api` parses and renders; `internal/service/labresult`
  and `internal/service/attachment` decide; `internal/store/**` persists; `internal/domain/labs`
  computes and knows nothing else. The file adapter (`internal/store/filestore`) does exactly one
  thing — put bytes in, take bytes out, make a thumbnail — and holds no policy.
- **Open/closed.** `lab_result` is added by satisfying `records.Service` and calling
  `records.Register`. Crucially, **attachment support is added to every existing kind by extending
  `records.Register` itself**, not by editing fifteen registration files: the cleanup hook and the
  detail page's attachment strip are wired centrally, which is precisely what FR-049 demands
  ("without that record's kind having been anticipated by this phase"). No file in this phase
  contains a `switch` over `kind.Kind`; the phase-003 vet check still passes.
- **Liskov.** `recordstest.RepositoryContract` and `recordstest.KindContract` run unchanged against
  `lab_result`. A new `attachmenttest.StoreContract` runs against `internal/store/attachment` and
  against its fake; a new `filestoretest.Contract` runs against the PocketBase-backed file adapter
  and against an in-memory fake, so every service unit test is filesystem-free.
- **Interface segregation.** Every port is consumer-declared and small:
  `labresult.Repository` (4), `labresult.ComponentRepository` (3: `ListByResult`, `ReplaceSet`,
  `DeleteByResult`), `labtrend.Reader` (2: `Catalog`, `Series`), `attachment.Repository` (5),
  `attachment.FileStore` (4: `Put`, `Open`, `Thumb`, `Delete`), `attachment.RecordLocator` (1:
  `Exists(ctx, kind, id) (patientID string, err error)`), `catalog.Reader` (2). No omnibus `Store`,
  no omnibus `Service`.
- **Dependency inversion.** `internal/domain/**` and `internal/service/**` import neither
  PocketBase, nor `net/http`, nor templ — enforced by the existing `depguard` rule, which already
  covers the new packages by glob. `internal/domain/labs` additionally imports nothing outside the
  standard library.

### III. Test-First With testify (NON-NEGOTIABLE) — **PASS**

Every one of the specification's **56 acceptance scenarios** across the five stories becomes a
named test before the implementation task it covers starts (FR-083, SC-010). `tasks.md` orders
them that way and the checkpoint at the end of each story is "the story's scenarios are green".

Authorization is first-class, exactly as FR-080 and SC-005 demand. Every one of the nine
operations carries, at minimum, four tests: the owner succeeds; a stranger receives a `404`
byte-identical to the response for an id that never existed; an unauthenticated caller receives
`401` with no information about the patient; and the refused attempt appears in the activity trail
by opaque identifier with no content. For attachments this is tested **three ways** because
FR-075 names three: by opening the address directly, by guessing the identifier, and while signed
out.

Two properties get dedicated harnesses rather than assertions sprinkled about:

- **SC-004 byte-for-byte fidelity** — upload a fixture of each accepted type at 1 byte, 1 KiB and
  exactly the configured limit, download each, compare SHA-256. A single flipped byte fails.
- **SC-008 PHI leak** — phase 001's `internal/testsupport/phileak` harness (extended by 002 and 003) is extended again with file
  names, descriptions, test names, values, units, reference ranges and interpretations as
  recognisable sentinels, then every operation in this phase is exercised and the zerolog stream,
  the Prometheus registry, the OTel span recorder and the Sentry transport are asserted to contain
  zero occurrences.

### IV. Idiomatic Go Over Clever Go — **PASS**

Errors are values, wrapped with `%w`, inspected with `errors.Is`/`errors.As`, mapped to status
codes by the single table in `internal/web`. This phase adds six sentinels
(`ErrEmptyFile`, `ErrUnsupportedFileType`, `ErrFileTooLarge`, `ErrRetentionExpired`,
`ErrOwnerRecordMissing`, `ErrUnitAmbiguous`) and no new mapping mechanism. `Patch` structs carry
absent-vs-null with plain pointers. `context.Context` is first and is honoured — the streaming
download passes it to `fsys.SetContext` so a cancelled browser request stops the read, and the
analytics queries are cancellable. Goroutines: exactly one is introduced, the thumbnail generation
inside `TxInfo().OnComplete`, and it is bounded by the app's shutdown context with a defined
failure path (log once, set `has_preview = false`, never fail the upload). Generated `*_templ.go`
is committed, marked generated, excluded from lint and coverage.

### V. PocketBase Is The Platform, Not A Detail — **PASS**

Nothing PocketBase provides is rebuilt. File storage is `app.NewFilesystem()` — local or S3 by the
operator's configuration, with no code difference. Streaming with Range, ETag and a correctly
quoted `Content-Disposition` is `fsys.Serve` (v0.40 fixed the quoting for names with special
characters, which is exactly the FR about non-Latin and right-to-left file names). Thumbnails are
`fsys.CreateThumb`. Deletion of bytes is PocketBase's own cleanup when the record is deleted.
Scheduling is `app.Cron()`. Four collections arrive as reversible Go migrations; schema is never
changed in the admin UI.

The one place MediKube deliberately does **not** use PocketBase is the file *route*: `/api/files/`
serves an unprotected field to anonymous callers and 404s a protected one under nil `ViewRule`, so
it is unusable in both directions. Files are served from MediKube's own `/api/v1` route through the
filesystem abstraction, authorized by the service. PocketBase's file-token mechanism is not called
— a credential in a URL is a PHI leak, and FR-074 says so independently.

`app.RunInTransaction` wraps the three multi-write operations this phase adds: the component
replace-set, the replace-a-document flow (create new + soft-delete old), and restore (which
re-reads `deleted_at` inside the transaction so it cannot race the purge).

Hooks: only post-commit model hooks. `OnRecordAfterCreateSuccess("attachments")` schedules the
eager thumbnail through `TxInfo().OnComplete` so thumbnailing never extends the write transaction.

All five API rules stay `nil` on all four new collections, asserted at boot and proved per
collection by a `tests.ApiScenario` showing `/api/collections/<c>/records` returns 404 to a normal
user.

### VI. One Log Stream, One Trace Context — **PASS**

zerolog only. `LabResult`, `Component` and `Attachment` implement `MarshalZerologObject` emitting
**only** opaque ids — never a test name, a value, a unit, a reference range, an interpretation, a
file name or a description. FR-078 and FR-079 get their own gate: a refusal shown to an uploader
may name their own file to them, and a test asserts that the same file name does **not** appear in
the log line, the span, the metric or the Sentry event produced by the same refusal. That
asymmetry is the single most likely place for this phase to leak, and it is tested directly.

New metrics, all with bounded label sets: `medikube_files_uploads_total{outcome,reason}`,
`medikube_files_bytes_total` (a gauge refreshed by the same cron as the purge, instance-wide, **no
patient label**), `medikube_files_serve_duration_seconds{disposition,outcome}`,
`medikube_labs_series_points` (histogram), `medikube_labs_series_capped_total`. No file name, patient
id, attachment id, test name or unit is ever a label value.

Datastar's `ConsoleLog`/`ConsoleError` remain banned by `forbidigo`.

### VII. Patient Privacy Is Structural, Not Procedural — **PASS**, and this is the phase where it costs the most

This principle outranks every other and this phase is where it bites, so the discharge is itemised:

- **`attachments.file` is `Protected: true`.** The boot assertion that refuses to start on an
  unprotected file field now covers two fields instead of one, and a test asserts that flipping
  `Protected` to false in a migration makes the application refuse to boot.
- **Files are served only from `/api/v1/attachments/{id}`**, authorized by
  `attachment.Service.OpenContent`, which calls the `Authorizer` before it touches the filesystem.
  `e.Auth.NewFileToken()` is never called; a `forbidigo` pattern makes calling it a build failure.
- **A document's address is not a credential** (FR-074). The id is a PocketBase opaque id; nothing
  in the URL authorizes; sharing the URL with somebody who cannot reach that patient gives them a
  `404` identical to a non-existent id.
- **Previews are protected exactly as strictly as their document** (FR-060). The thumbnail is
  served by the same handler, through the same authorization call, under the same audit rule.
- **A retrieval of content or a preview writes one `read_sensitive` audit row when, and only when,
  the resolved grant is not the reader's own ownership** — a superuser reading somebody else's
  document here, and from phase 005 a share recipient too. An owner reading their own document
  writes no row. The row carries actor, attachment id, patient id and timestamp — and never the
  file name, the description or any bytes (FR-076, SC-006). The rule is stated once in phase 005's
  `contracts/widened-authorization.md`; the reasoning and the volume consequence are in research
  D-20.
- **Nothing uploaded can run.** The accepted-type allowlist excludes every active type by default;
  the set of types MediKube will serve inline is a **compile-time constant that an operator cannot
  widen**; and every attachment response carries `X-Content-Type-Options: nosniff` plus its own
  `Content-Security-Policy: default-src 'none'; …` (research D-16).
- **404, never 403**, for everything patient-scoped, including a document, its preview, a lab
  result, a component and a trend.
- **Hard delete for records, soft delete for files only.** The `deleted_at` column exists on
  exactly one collection, and Constitution VII's retention rule is enforced by a scheduled purge
  that is part of this phase — because a retention window nothing enforces is not a retention
  window.
- **Deleting a patient destroys their documents including those awaiting purge** (FR-068), by
  PocketBase's cascade on `attachments.patient`, verified by looking for the rows and the blobs
  afterwards rather than assuming.
- **No outbound request.** `filesystem.NewFileFromURL` is never called and a `forbidigo` pattern
  makes calling it a build failure.

One consequence must be stated rather than discovered: the inline document viewer frames MediKube's
own attachment route, so the application's page CSP gains `frame-src 'self'`. That is additive —
it weakens none of the directives the constitution names — and the framed response is itself
locked down far harder than any page.

### VIII. The UI Must Prove It Renders — **PASS**

Four new pages, each covered at 1440×900 and 390×844, asserting 200, the four shell landmarks plus
the page's own landmark, `body[data-signals]` present, zero console errors, zero page errors and
zero failed network requests. The route list comes from `medikube routes`, so a page added without a
smoke case fails the build.

Three things in this phase are unusually good at making a smoke gate go falsely red, and each has a
deliberate answer:

1. **A missing thumbnail is a failed network request.** Hence `has_preview` is stored, set by the
   thumbnailer, and the listing renders a type icon — not a broken `<img>` — when it is false.
2. **An empty page still needs its landmark.** The seed leaves one patient with no lab results and
   no documents so that `@EmptyState` is what the landmark assertion exercises on three of the
   four pages.
3. **An inline `<svg>` chart with no accessible name is an accessibility failure, not a console
   error** — so the chart carries `role="img"` and an accessible name, out-of-range points are
   marked by shape *and* by a text marker in the accompanying table, and a templ render test
   asserts both (FR-021, SC-002).

The four page-action routes are not pages and have no landmark; each declares the Playwright spec
that exercises it, and a gate test fails the build if that spec does not reference it.

### IX. Compliance Is A Build Gate, Not A README Paragraph — **PASS**

Five gates, all `go test` or CI steps, all failing the build:

1. `internal/openapi/gate_test.go` — the route registry and the committed `api/openapi.json` agree
   on every `operationId`; the regenerated document is byte-identical to the committed one
   (FR-084, SC-016).
2. `internal/records/registry_completeness_test.go` — now asserts fifteen kinds fully wired, and
   additionally that every registered kind has an attachment-cleanup hook bound (the FR-049 gate).
3. `e2e/routes.gate.spec.ts` — every route emitted by `medikube routes` with `Page: true` has a
   smoke case; every route with `Kind: page_action` names an existing spec that references it.
4. `internal/platform/pb/assertions_test.go` — all five API rules `nil` on all **26** collections, and
   `Protected: true` on both file fields; the app refuses to boot otherwise.
5. golangci-lint v2 with `depguard` (import boundary, plus the new labs↔units ban) and `forbidigo`
   (`app.Logger()`, `fmt.Print*`, `log.*`, `OnRecord*Request`, the Datastar inline-script family,
   the Datastar Pro attributes, `NewFileToken`, `NewFileFromURL`).

Every one of the four migrations has a real `down`; `migrations.Register`'s signature makes that
structural (VERIFIED-SOURCE-FACTS FACT 8). The catalogue seed migration's `down` removes the
collection, and its `up` is an idempotent upsert keyed on `loinc_code` so re-running it on an
instance that already has data is safe.

### Post-Design Re-Check (after Phase 1)

Re-evaluated against `data-model.md` and `contracts/`. No principle moved from PASS to FAIL.

Two candidates were considered during design and **rejected as not needing a Complexity Tracking
entry**:

- *The `replaces` field on the upload operation* (FR-061). It looked like a special case; it is
  not. Replacement is "attach this document in place of that one", executed as create-then-trash in
  one transaction. It adds one optional multipart field and one transaction, and it avoids a
  seventh attachment operation.
- *`has_preview` as a stored boolean.* It is derived data, but it is derived from an operation that
  can fail — and the Playwright gate's zero-failed-requests rule makes guessing it a defect. One
  writer, written post-commit, never read for authorization.

One candidate was **promoted into** Complexity Tracking during design: the `KindPageAction` route
class. It was expected to be one route (the multipart upload) and turned out to be four, which is a
new category in a shared mechanism rather than a one-off.

---

## Project Structure

### Documentation (this feature)

```text
specs/004-labs-and-attachments/
├── plan.md              # This file
├── research.md          # Phase 0 — 35 numbered decisions with evidence
├── data-model.md        # Phase 1 — 4 collections, fields, enums, indexes, migrations, state machine
├── quickstart.md        # Phase 1 — run and verify this phase by hand, end to end
├── contracts/
│   ├── README.md              # conventions, error envelope, status codes, the authorization rule
│   ├── lab-results.md         # the record family as it applies to lab_result, incl. the component set
│   ├── lab-components.md      # 2 operations — the per-patient rollup and the trend series
│   ├── catalog-lab-tests.md   # 1 operation — the read-only standardized test catalogue
│   ├── attachments.md         # 6 operations — upload, list, stream, describe, trash, restore
│   └── pages.md               # 4 pages + 4 page-action routes, landmarks, smoke expectations
├── checklists/          # pre-existing
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root `/medikube`)

Only paths this phase **creates** or **touches**. `[NEW]` = created here, `[EDIT]` = modified.

```text
medikube/
├── cmd/medikube/main.go                                     [EDIT] register 4 services + the purge cron in the container
│
├── internal/config/config.go                                [EDIT] +MEDIKUBE_LABS_MAX_SERIES_POINTS; FILES_ALLOWED_MIME default set
├── internal/config/config_test.go                           [EDIT] defaults, validation, the accepted-type parse
│
├── internal/domain/
│   ├── kind/kind.go                                         [EDIT] +LabResult (15th kind), segment "lab-results", labels, landmarks
│   ├── kind/kind_test.go                                    [EDIT] exhaustiveness + spelling round-trip at 15
│   ├── clinical/vocab_lab.go                                [NEW]  lab.category, lab_component.status, lab_component.result_type
│   ├── labs/labresult.go                                    [NEW]  LabResult entity, Validate (incl. FR-005/007), MarshalZerologObject
│   ├── labs/component.go                                    [NEW]  Component entity, value/kind XOR rule (FR-013), ref-range rule (FR-017)
│   ├── labs/refrange.go                                     [NEW]  RefRange value object + Classify (FR-018/019/020)
│   ├── labs/canonical.go                                    [NEW]  Normalise (FR-025): NFKC, trim, collapse, casefold
│   ├── labs/series.go                                       [NEW]  Series, Summary, Direction — the FR-031 halves rule verbatim
│   ├── labs/sortdate.go                                     [NEW]  the FR-008 COALESCE, one function, one caller
│   ├── labs/*_test.go                                       [NEW]  table-driven, no I/O, t.Parallel()
│   ├── files/attachment.go                                  [NEW]  Attachment entity, Validate, MarshalZerologObject
│   ├── files/mime.go                                        [NEW]  Sniff + the 9-entry magic table + InlineSafe (compile-time set)
│   ├── files/trash.go                                       [NEW]  TrashState, retention arithmetic, restore preconditions
│   └── files/*_test.go                                      [NEW]
│
├── internal/records/
│   ├── register.go                                          [EDIT] bind the attachment-cleanup hook for EVERY kind, centrally
│   ├── register_test.go                                     [EDIT] a kind registered without the cleanup hook fails the test
│   ├── registry_completeness_test.go                        [EDIT] 15 kinds fully wired
│   └── kinds/labresult.go                                   [NEW]  the single Register call for lab_result
│
├── internal/service/
│   ├── labresult/{service.go,ports.go,adapter.go,query.go,patch.go,components.go}   [NEW]
│   ├── labresult/{service_test.go,components_test.go,adapter_test.go}               [NEW]
│   ├── labresult/labresulttest/fake.go                                              [NEW]
│   ├── labtrend/{service.go,ports.go,service_test.go}                               [NEW]  rollup + series (US3)
│   ├── labtrend/labtrendtest/fake.go                                                [NEW]
│   ├── catalog/{service.go,ports.go,service_test.go}                                [NEW]  read-only catalogue (US4)
│   ├── attachment/{service.go,ports.go,upload.go,serve.go,trash.go,purge.go}        [NEW]
│   ├── attachment/{service_test.go,upload_test.go,trash_test.go,purge_test.go}      [NEW]
│   ├── attachment/attachmenttest/{fake.go,filestorefake.go,storecontract.go}        [NEW]
│   └── access/authorizer.go                                 [EDIT] Record() learns kind.LabResult; Attachment() anchors on patient
│
├── internal/store/
│   ├── migrations/<ts>_catalog_lab_tests.go                 [NEW]  collection + idempotent seed from the vendored extract
│   ├── migrations/<ts>_lab_results.go                       [NEW]
│   ├── migrations/<ts>_lab_components.go                    [NEW]
│   ├── migrations/<ts>_attachments.go                       [NEW]  the ONLY new file field; Protected: true
│   ├── migrations/*_test.go                                 [NEW]  rules nil, indexes present, Protected true, down clean
│   ├── labresult/{repo.go,mapper.go,repo_test.go}           [NEW]
│   ├── labcomponent/{repo.go,mapper.go,replaceset.go,repo_test.go}  [NEW]
│   ├── labtrend/{repo.go,sql.go,repo_test.go}               [NEW]  the two parameterised analytics queries
│   ├── catalog/{repo.go,repo_test.go}                       [NEW]
│   ├── attachment/{repo.go,mapper.go,repo_test.go}          [NEW]
│   └── filestore/{filestore.go,thumb.go,filestore_test.go}  [NEW]  app.NewFilesystem() adapter; the ONLY package that touches bytes
│
├── internal/platform/pb/
│   ├── assertions.go                                        [EDIT] Protected:true assertion now covers two fields
│   ├── hooks.go                                             [EDIT] OnRecordAfterCreateSuccess("attachments") -> eager thumb
│   └── cron.go                                              [EDIT] register the attachment purge + the storage gauge refresh
│
├── internal/web/
│   ├── api/labresults.go(+_test,+_http_test)                [NEW]  LabResult DTOs incl. the component array
│   ├── api/labcomponents.go(+_test,+_http_test)             [NEW]  rollup + trend DTOs
│   ├── api/cataloglabtests.go(+_test,+_http_test)           [NEW]
│   ├── api/attachments.go(+_test,+_http_test)               [NEW]  multipart in, JSON out, stream out
│   ├── page/{labresults.go,labtrends.go,documents.go}       [NEW/EDIT]
│   ├── page/actions.go                                      [NEW]  the 4 KindPageAction handlers
│   ├── security/csp.go                                      [EDIT] +frame-src 'self' on pages; the attachment response CSP
│   └── views/
│       ├── records/labresult.templ (+_test)                 [NEW]  Row/List/Detail + the component table
│       ├── labs/componenteditor.templ (+_test)              [NEW]  add/remove rows, catalogue suggestions
│       ├── labs/suggest.templ (+_test)                      [NEW]  the listbox fragment
│       ├── labs/trends.templ (+_test)                       [NEW]  catalogue list, unit chooser, summary
│       ├── labs/chart.templ (+_test)                        [NEW]  inline SVG, role="img", range band, shaped markers
│       ├── files/{library.templ,strip.templ,upload.templ,viewer.templ} (+_test)  [NEW]
│       ├── shared/emptystate.templ                          [EDIT] three new empty states
│       └── ids/ids.go                                       [EDIT] deterministic ids for the new components
│
├── internal/httproute/registry.go                           [EDIT] +KindPageAction and its gate metadata
├── internal/cli/{seed.go,purge.go,routes.go}                [EDIT] seed labs+documents; `medikube purge` runs the sweep once
├── internal/testsupport/
│   ├── phileak/exercise.go                                  [EDIT] +file names, descriptions, test names, values, ranges
│   ├── scale/generate.go                                    [EDIT] +5,000 results, +100-component panel, +500 readings, +2,000 documents
│   └── files/fixtures.go                                    [NEW]  one minimal valid file per accepted type + the hostile set
│
├── internal/testdata/
│   ├── pb_data/                                             [EDIT] regenerated fixture including the 4 new collections
│   └── files/                                               [NEW]  committed binary fixtures (all small; the size-limit case is generated)
│
├── assets/catalog/lab-tests.json                            [NEW]  the vendored LOINC-derived extract, embedded
├── api/openapi.json                                         [EDIT] regenerated, committed, diffed
├── e2e/specs/{labs.spec.ts,trends.spec.ts,documents.spec.ts} [NEW]
├── e2e/routes.gate.spec.ts                                  [EDIT] +4 pages, +4 page-action assertions
├── .golangci.yml                                            [EDIT] labs↔units depguard; NewFileToken/NewFileFromURL forbidigo
└── Taskfile.yaml                                            [EDIT] `task purge`, `task fixture:regen` covers the new collections
```

**Structure Decision**: the single-project Go layout from phase 001 is kept unchanged; this phase
populates it. Two structural additions are made and both are justified above:
`internal/store/filestore` — the one package in the application permitted to move bytes, which is
what makes "no file leaves except through an authorized service call" auditable in one place — and
`internal/domain/labs`, a pure-computation package with no I/O so that US3's arithmetic is testable
without a database.

---

## Deviations from the shared design contract

The shared design contract is binding on **design**. Its §0 phase table is not consistent with the
specification set that was actually written, and this phase's own specification resolves that
conflict explicitly in favour of the charters. Phase 003's plan established the precedent and this
plan follows it. **No design in the contract is altered except where noted in the last row.**

| Contract says | This plan does | Why |
|---|---|---|
| `search_index`, op 57 `GET /api/v1/search` and the `/search` page are phase 004 | **Not built here.** Phase 003 delivered them. | This phase's spec: *"A unified search across every record kind. The charter for this phase does not include it, and where the shared design contract's phase table places it here, the charter governs."* `lab_result` is added to the existing index automatically by `records.Register`; **attachments are not indexed**, because no requirement asks for cross-kind document search. |
| `catalog_lab_tests` and op 44 `GET /api/v1/catalog/lab-tests` are phase 002 | They land here | Phase 002's charter delivered patients, practitioners and facilities; it never built the catalogues. US4 owns the standardized test catalogue and cannot ship without it. Shape, route and query parameters are the contract's, verbatim. |
| Phase 004 = 9 operations (49–57) | Phase 004 = 9 operations (49–56 **minus** 57, **plus** 44) | Net zero against the 94-operation budget (SHARED-DESIGN §2.3, cited not re-derived). |
| Phase 004 = 5 pages, including `/search` | 4 pages | `/search` shipped in 003. |
| `attachments` has no `has_preview` field | It has one | Derived from an operation that can fail. Guessing it from the MIME type produces a broken `<img>`, and the Principle VIII gate asserts **zero failed network requests**. One writer, post-commit. |
| `lab_results.is_panel` is a mutable client-settable boolean | It is **derived and server-owned**, present in responses and absent from every request DTO | A discriminator a client can set independently of the data it discriminates is a defect waiting to happen; FR-005 makes "both" invalid, so the flag is a projection of the data, not an input. |
| `lab_results` carries no sort key; `lab_components` carries no `patient` | `lab_results` gains a derived `sort_date`; `lab_components` still carries no `patient` | FR-008's four-level COALESCE ordering plus SC-011's 2-second page at 5,000 rows needs an index, and an index over a COALESCE expression is not something to bet a gate on. `lab_components` stays joined through its parent — cascade is transitive and is tested. |
| Charts belong to phase 006 | The trend chart component lands here | US3 acceptance scenario 10 requires a reference-range **band**, which is a chart. Phase 006 embeds this same component in reports rather than building a second one. Cost: one templ component and a pure Go scale function; no new dependency, no new route. |
| Route classes are API, page and external | A fourth class, `page_action`, is added | Four page-layer routes in this phase are neither navigable pages nor part of the public API: the multipart upload action and three HTML fragments. See Complexity Tracking. |

Net effect on the contract's headline numbers, all cited from SHARED-DESIGN §§1.6/2.3/3.1 rather
than re-derived: operations **59 running** after this phase, of a suite total of **94**;
collections **26 running**, of **30**; pages **48 running**, of **58**, of which four are this
phase's.

---

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| **Three derived columns** — `lab_results.sort_date`, `lab_results.is_panel`, `lab_components.canonical_name` (plus `attachments.has_preview`), against Principle I's dislike of stored derivations | Each removes a per-query computation that a gate depends on. `sort_date` makes FR-008's four-level COALESCE ordering a plain indexed column, which is what makes SC-011 (5,000 results, every page in 2 s) reachable. `canonical_name` is the grouping key for every trend query (FR-025) and normalising 100,000 rows per query is not a plan. `is_panel` and `has_preview` are read by list renderers that must not do extra I/O per row. All four have **exactly one writer** — the domain, inside the same `Save` that writes the source fields — so none can drift, and each is asserted by a repository contract test that mutates a source field and re-reads the derivation. | **(a) Compute at query time.** `ORDER BY COALESCE(resulted_on, collected_on, ordered_on, created)` cannot use a plain index; an expression index would work in SQLite but PocketBase's `AddIndex` path for expression indexes is unverified, and betting a performance gate on unverified framework behaviour is how a phase fails late. **(b) Compute in Go after fetching.** Requires fetching all 5,000 rows to order 25 of them, which is the exact failure SC-011 describes. **(c) A materialised view.** SQLite has none; a trigger is a second writer in a language nobody in this codebase reviews. |
| **`attachments.owner_kind` + `owner_id` — polymorphic ownership with no foreign key**, against Principle II's preference for real relations and Principle I's preference for the boring thing | FR-049 requires that a record kind added in a future phase inherits document support **without that kind having been anticipated by this phase**. That is a structural requirement, not a convenience. The integrity risk is closed three ways: the cleanup hook is bound by `records.Register` itself, so a kind cannot be registered without it (asserted by `registry_completeness_test.go`); every write validates `owner_id` against `owner_kind`'s collection *and* against the same patient, inside the transaction; and a nightly orphan sweep in the same cron as the purge reports and quarantines anything that slipped. It also keeps file fields confined to two collections, which is what keeps the `Protected: true` boot assertion trivially auditable. | **(a) Fifteen nullable relation columns.** A schema migration on every future kind, fourteen always-null columns per row, and fifteen indexes; and it still needs a discriminator to know which one to read. **(b) A join table per kind.** Fifteen collections and fifteen cleanup hooks. **(c) Attach documents to the patient only, with a free-text label.** Loses FR-070 (open the record this belongs to) and makes FR-067 (record deletion trashes its documents) unimplementable. |
| **Two hand-written SQL queries through `app.DB()`** in `internal/store/labtrend/sql.go`, bypassing the typed-repository shape every other store package uses | FR-024 needs "every distinct component ever recorded for this person, once, with its latest value, unit, status, reading count and latest date" — a `GROUP BY canonical_name` with a per-group latest, over a join to the parent for the patient scope and the date. FR-026 needs the readings of one component ordered by the parent's date. Through PocketBase's record API both are N+1: one query per distinct component name, of which a four-year chart has dozens. At the 100,000-component-row target that is minutes, and SC-003 allows two seconds. The queries are confined to one file, are fully parameterised (`dbx` named bindings; **no string interpolation of `q`, `canonical_name` or `unit` — ever**), take their `patient` value from the `access.Grant` and never from the raw request, and are covered by integration tests against a real test app plus an injection test that feeds `%'; DROP TABLE` shaped input through every text parameter. | **(a) The record API plus in-Go grouping.** Fetch every component row for the patient to group 40 of them — the SC-003 failure again. **(b) A denormalised `component_rollup` collection.** A genuine cache with a second writer, which is what Principle I is for. **(c) Store the trend.** Contradicts the spec outright: *"Nothing about it is stored separately from the readings it is computed from, so correcting a reading corrects the comparison."* |
| **A fourth route class, `KindPageAction`**, for four page-layer routes that are neither navigable pages nor public API: `POST /documents/upload`, `GET /documents/list`, `GET /lab-results/component-suggest`, `GET /labs/trends/series` | Two independent forces produce them. **(1) The upload cannot be Datastar.** `data-bind` on a file input produces a `{name, contents, mime}[]` signal with base64 contents; at the 32 MiB limit that is ~43 MiB of JSON held in browser memory and posted as a request body, which exceeds the body limit before the file is even read. The upload is therefore a native `<form enctype="multipart/form-data">` posting to a page-layer action that answers `303`. **(2) Datastar patches HTML, and the API returns JSON.** A filter, a page change, a post-delete refresh and an autocomplete listbox all need server-rendered HTML fragments; `/api/v1` is JSON-in/JSON-out by convention rule 14 and must stay that way. Making these a *declared class* rather than untracked handlers is what keeps Principle IX honest: each appears in `medikube routes`, is excluded from OpenAPI deliberately rather than accidentally, and must name the Playwright spec that exercises it or the gate fails. | **(a) Content-negotiate inside `/api/v1`.** Puts browser redirect semantics and HTML rendering into the JSON API, breaks the "two shapes per resource, both in OpenAPI" rule, and makes the OpenAPI document lie about what the route returns. **(b) Serve fragments from the page route under a magic query parameter.** A hidden mode on a gated route: the smoke gate covers the page and silently never covers the fragment. **(c) Base64 the upload through Datastar anyway.** Fails at the request body limit, triples memory, and makes SC-004's byte-for-byte guarantee depend on a base64 round trip in two languages. **(d) A second origin for attachments.** Would be the textbook answer to the inline-viewing risk, but MediKube is single-instance with one `PublicURL`; a second origin means a second certificate, a second cookie domain and a second CSP surface for one iframe. |
