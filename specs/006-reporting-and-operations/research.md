# Phase 006 — Research

**Feature**: Reporting and Operations | **Date**: 2026-08-27 (revised)

**Revision note.** This file was first written against an earlier draft of `spec.md`. The
specification was rewritten on 2026-08-27 and three things changed materially: the attachment trash
moved wholly to phase 004 (so this phase adds no recovery surface and no second purge), a running
report or export became cancellable, and the release-hardening story acquired an explicit operator
handbook and privacy review. **D-14, D-15, D-33 and D-40 were rewritten** to match; **D-41 through
D-53 are new**; every other decision survived the rewrite unchanged and is carried forward as
written. Requirement numbers cited below are the numbers in the **current** `spec.md`.

**Inputs, in precedence order**: the constitution (v1.3.0) → [`VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) →
[`HOUSE-PATTERNS.md`](../research/HOUSE-PATTERNS.md) → [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) → the domain and library dossiers → PocketBase v0.40.1
source read directly from `/home/agent/go/pkg/mod/github.com/pocketbase/pocketbase@v0.40.1`.

Every decision below is stated as **Decision / Rationale / Alternatives considered**. Where a fact
came out of real source, the file and line are given so the next person can re-check it rather than
trust this document.

**No `NEEDS CLARIFICATION` item survives this file.**

---

## Index

| # | Decision |
|---|---|
| [D-01](#d-01) | PDF engine: `go-pdf/fpdf`, pinned, behind a `report.Renderer` port |
| [D-02](#d-02) | Charts are drawn as PDF vector primitives. No chart library. |
| [D-03](#d-03) | Series reduction: deterministic every-k-th decimation, stated on the page |
| [D-04](#d-04) | Font coverage, bidi, and the limitation MediGo states rather than hides |
| [D-05](#d-05) | One worker, one queue, position from the row order |
| [D-06](#d-06) | Startup reconciliation kills the "running forever" job (closes R9) |
| [D-07](#d-07) | Job scratch space is PocketBase's `.pb_temp_to_delete`, and that is not an accident |
| [D-08](#d-08) | The artifact is a `Protected: true` file field served by MediGo's own route |
| [D-09](#d-09) | Authorization is re-resolved when production *starts*, not when it is requested |
| [D-10](#d-10) | Limits and retention windows are configuration with published defaults |
| [D-11](#d-11) | The audit reader never joins to content — the DTO has nowhere to put it |
| [D-12](#d-12) | Audit scoping for a non-administrator, and why it cannot leak a count |
| [D-13](#d-13) | Audit CSV streams; reading is not audited, exporting is |
| [D-14](#d-14) | The attachment trash belongs to phase 004; this phase adds two figures and nothing else |
| [D-15](#d-15) | There is no `/trash` page in this phase — seven pages, not eight |
| [D-16](#d-16) | Operator figures: cheap counters live, expensive footprint from a cron-refreshed gauge |
| [D-17](#d-17) | MFA and IP-allowlist posture are readable from Go (closes R6 for this phase) |
| [D-18](#d-18) | "Last signed in" is derived from the audit trail, not a new column |
| [D-19](#d-19) | Disabling an account: `disabled_at` plus `RefreshTokenKey()` |
| [D-20](#d-20) | `users.must_change_password` is added back |
| [D-21](#d-21) | Backup and restore wrap PocketBase; `StoreKeyActiveBackup` is the mutex |
| [D-22](#d-22) | The safety backup is a precondition of the restore, not a courtesy |
| [D-23](#d-23) | The restore journal, because a restore destroys the audit rows that describe it |
| [D-24](#d-24) | `,unset` × `execve`: the environment must be re-applied before `RestoreBackup` |
| [D-25](#d-25) | `medigo.json` in `pb_data` is the archive's version marker |
| [D-26](#d-26) | Reading an archive for preview: local file directly, S3 via the temp dir |
| [D-27](#d-27) | Archive download is a `POST` with password re-entry |
| [D-28](#d-28) | A restore is refused while a job is running |
| [D-29](#d-29) | Export format v1: layout, manifest, streaming encode |
| [D-30](#d-30) | Nothing secret can enter an export, because the export is built from DTOs |
| [D-31](#d-31) | Job progress is polled, not streamed |
| [D-32](#d-32) | The >10-minute SSE liveness job (closes R7) |
| [D-33](#d-33) | Performance budgets: volumes, journeys, and how a regression goes red |
| [D-34](#d-34) | The PHI-leak sweep is a test, not a reading |
| [D-35](#d-35) | "No outbound connection" is asserted by a dialer, not by inspection |
| [D-36](#d-36) | The privacy review is a repeatable checklist with a written result |
| [D-37](#d-37) | Report section order, empty sections, and the criteria statement |
| [D-38](#d-38) | Saved reports use the existing `ETag`/`If-Match` convention |
| [D-39](#d-39) | Trends: the minimum, the unit scoping, and the refusal to convert |
| [D-40](#d-40) | Seven pages, seven empty states, and why the smoke gate stays green on an empty instance |
| [D-41](#d-41) | One queue for reports and exports; cancellation is cooperative |
| [D-42](#d-42) | `expired` is a state the purge writes, and re-production is offered |
| [D-43](#d-43) | One scheduled-job envelope, so "last ran / last succeeded" is a query, not a table |
| [D-44](#d-44) | One selection resolver, so the count shown and the document produced cannot disagree |
| [D-45](#d-45) | Saved-report identity: a unique name, and a patient relation that empties rather than cascades |
| [D-46](#d-46) | The instance refuses to start on a limit or a window it cannot honour |
| [D-47](#d-47) | The operator figure catalogue: no figure without a definition, enforced by a test |
| [D-48](#d-48) | The administrative tier is not the break-glass credential |
| [D-49](#d-49) | A disabled account is undetectable from outside |
| [D-50](#d-50) | Retention arithmetic: whole days from the recorded moment, one helper, clock-jump safe |
| [D-51](#d-51) | Activity entries inside an export, and the actors they may name |
| [D-52](#d-52) | Trail paging that cannot repeat or skip while entries are being written |
| [D-53](#d-53) | Where a produced document appears, and why `/exports` is not that place |

---

## D-01

**Decision.** Reports are rendered to PDF by **`github.com/go-pdf/fpdf`** (the maintained fork of the
archived `jung-kurt/gofpdf`; BSD-3-Clause; pure Go; no cgo), pinned to an exact patch version in
`go.mod`. It is reached only through a consumer-declared port:

```go
package report // internal/service/report

type Renderer interface {
    Render(ctx context.Context, w io.Writer, doc Document) error
}
```

implemented by `internal/render/pdf.Renderer` and by `reporttest.FakeRenderer`. **`fpdf` is imported
by exactly one package, `internal/render/pdf`**, and that package is added to the `depguard`
allow-list; `internal/service/**` and `internal/domain/**` never see it.

**A spike task gates the choice.** Before the renderer is written, one task builds a throwaway
binary that produces a two-page PDF with `AddUTF8FontFromBytes`, `SetHeaderFunc`, `SetFooterFunc`,
`AliasNbPages`, `Line`, `Rect`, `SetDashPattern` and `RegisterImageOptionsReader`, on
`CGO_ENABLED=0`, and asserts the output opens. If any of those symbols is absent from the pinned
version, the fallback is `github.com/signintech/gopdf` (also pure Go, also UTF-8 TrueType capable)
and only `internal/render/pdf` changes. **The port is what makes that a contained decision**, and it
is why the port exists at all rather than being a speculative seam.

**Rationale.** Constitution Technology Constraints: one static binary, `CGO_ENABLED=0`, no Node in
the runtime image. That eliminates every renderer that shells out. SHARED-DESIGN §8 R4 assigned this
decision to phase 006 and named the constraint set: pure Go, cgo-free, licence-clean. `fpdf` is the
only widely used library that satisfies all three and gives direct control of the header and footer
callbacks — which is precisely how FR-006's "numbered pages and the person identified on every page"
is implemented, rather than being simulated.

**Alternatives considered.**

- **Headless Chrome / `chromedp` / `wkhtmltopdf`.** Forbidden. Each requires a browser binary in the
  runtime image; `gcr.io/distroless/static-debian12` has no shell, no libc, and no room for one.
- **`unidoc/unipdf`.** Commercial licence. A self-hosted application a person runs in a cupboard
  cannot carry a per-deployment licence obligation.
- **`johnfercher/maroto/v2`.** A grid/row/column DSL layered on the same underlying engine. It is a
  pleasant API and it is an extra dependency and an extra layer for a requirement nobody stated.
  Principle I: rejected.
- **`gonum.org/v1/plot` + an image, then embed.** Solves charting, not documents. See D-02.
- **Emit a single self-contained HTML file and let the browser print it.** Genuinely tempting: it
  reuses templ, needs no new dependency, and is trivially "readable without the application". It was
  rejected because FR-006 asks for numbered pages and a repeating identity, which in HTML depends on
  CSS paged media (`@page` margin boxes, `position: fixed` repetition) that every browser implements
  differently and some not at all. A document handed to a consultant must paginate the same way
  everywhere. A PDF does; an HTML file does not.

---

## D-02

**Decision.** Charts inside a report are drawn **directly as PDF vector primitives** by
`internal/render/pdf/chart.go` — axes, gridlines, the reference band, the series polyline and its
point markers, using `Line`, `Rect`, `SetDashPattern`, `SetLineWidth` and `Text`. **No charting
library is added.** Nothing is rasterised, so a chart is crisp at any print resolution and adds no
image bytes to the document.

**Rationale.** The chart MediGo needs is fully described by FR-020 and FR-021: one time series, a
labelled x-axis of dates, a labelled y-axis carrying the unit, an optional reference band, and a
table of the same points beside it. That is roughly 250 lines of straight-line drawing code, all of
it testable by asserting on the emitted PDF content stream. A chart library would bring a font
stack, a colour theme, a layout engine and an opinion about legends, and MediGo would then have to
fight all four to satisfy FR-021's "no meaning by colour alone" (which is expressed here by giving
each series a distinct marker shape and each reference boundary a distinct dash pattern, in addition
to any colour).

**Alternatives considered.**

- **`wcharczuk/go-chart/v2` → PNG → embed.** Pure Go and would work. It rasterises, so a printed
  chart is soft; it embeds its own font handling, duplicating D-04's work; and it is a dependency for
  something the target format already draws natively.
- **`gonum.org/v1/plot`.** Larger, brings `vg` and its own font assets, same rasterisation problem.
- **SVG into the PDF.** `fpdf` cannot place SVG. Would require an SVG-to-PDF converter — a third
  dependency for a picture MediGo can just draw.

---

## D-03

**Decision.** When a chart's selected range resolves to more than `MEDIGO_REPORT_MAX_CHART_POINTS`
readings (default **200**), the series is reduced by **deterministic every-k-th decimation** with
`k = ceil(n / max)`, always keeping the **first and last** readings, and the document prints, in
words, directly beneath the chart: *"Showing 200 of 517 readings (every 3rd reading). The
accompanying table lists the readings shown."* The accompanying table lists exactly the plotted
points, so the table and the picture never disagree.

**Rationale.** The specification's large-data edge case requires that "where the series must be
reduced for legibility the document states that it was reduced and by what rule". Every-k-th is the
only reduction rule that can be stated in one sentence a patient and a clinician both understand.
Averaging buckets would invent readings that were never taken — in a medical document that is worse
than dropping some.

**Alternatives considered.**

- **Largest-Triangle-Three-Buckets.** Visually superior and standard for dashboards. Its rule cannot
  be stated in a sentence, and it selects points by a shape heuristic, which is hard to defend in a
  clinical document.
- **Bucket means.** Fabricates values. Rejected outright.
- **No reduction, just draw all of them.** 500 markers in 160 mm is a smear, and the accompanying
  table becomes 500 rows. Fails the same edge case from the other side.

---

## D-04

**Decision.** `internal/render/pdf/fonts` embeds, via `embed.FS`:

- **Noto Sans** Regular + Bold + Italic — Latin, Latin Extended, Greek, Cyrillic (~900 KB)
- **Noto Sans Arabic** Regular + Bold (~300 KB)
- **Noto Sans Hebrew** Regular + Bold (~200 KB)

A small `fontrun` splitter divides each string into runs by which embedded face covers the run's
runes, and draws each run with that face. Runs whose base direction is right-to-left are reordered
from logical to visual order with **`golang.org/x/text/unicode/bidi`** (already in the module graph,
pure Go) before drawing.

**MediGo states the limitation instead of hiding it.** Arabic and Hebrew are reordered but **not
contextually shaped** — Arabic letters are drawn in isolated forms. Scripts with no embedded face
(CJK, Devanagari, Thai, and every other) are drawn as U+FFFD, and the report's first page carries a
counted, plain-language note: *"N characters could not be rendered with the fonts available to this
instance and appear as ▯. The exported data file contains the exact text."* The operator may extend
coverage by pointing `MEDIGO_REPORT_EXTRA_FONT_DIR` at a directory of additional TrueType files,
which are added to the coverage chain at boot and listed on `/admin`.

**Rationale.** The specification's edge case requires that non-Latin, right-to-left and
markup-looking text is "rendered correctly in the produced document and in the export, and never
interpreted as anything other than text". The export half is unconditionally satisfied: JSON and CSV
carry the exact bytes (D-29). The document half cannot be satisfied for every script without either
embedding tens of megabytes of CJK fonts in a binary or adding a full pure-Go text-shaping engine
(`go-text/typesetting`), and neither is proportionate for a phase whose job is to hand a document to
a doctor. What is **not** acceptable is silently dropping characters, which is what every naive
implementation does: PDF core fonts are WinAnsi and simply discard anything outside it. Counting and
stating is the honest middle.

The markup half is structural: PDF has no markup. A `<script>` in a patient's name is drawn as the
literal characters, because `fpdf.Text` writes glyphs. The templ pages that show the same string
escape it by construction (Principle VII).

**Alternatives considered.**

- **Embed Noto Sans CJK too.** ~20 MB added to a ~40 MB binary, for one script family among many,
  and still no shaping for Indic. Rejected as disproportionate; the font directory is the escape
  hatch instead.
- **`go-text/typesetting` for real shaping.** Correct and heavy: a shaping engine, a font-fallback
  engine, and a dependency in the path of every produced document. Principle I. Rejected, and
  recorded as the upgrade path if a real operator asks for it.
- **PDF core fonts only (WinAnsi).** Silently mangles every non-Latin name. Rejected on Principle VII
  grounds: quietly corrupting a patient's name in a clinical document is a harm.
- **Draw nothing and refuse the report.** Refusing to produce a document because a tag contains a
  Japanese character is worse than producing one with a stated gap.

---

## D-05

**Decision.** Exports and reports are produced by **one worker goroutine** owned by
`internal/service/exportjob.Runner`, started from `OnServe` and stopped from `OnTerminate` with a
context. The queue is the `export_jobs` collection itself: the worker takes the oldest row with
`status = "queued"` ordered by `(created asc, id asc)`. A waiting request's **position** is
`COUNT(*) WHERE status='queued' AND (created,id) < (mine)` plus one, computed at read time. There is
no in-memory queue to lose.

FR-045 is therefore satisfied by construction: at most one job runs on an instance because there is
exactly one worker, and the order is the row order.

**Rationale.** MediGo is single-instance by construction (constitution, Technology Constraints), so a
broker, a work-stealing pool and a distributed lock are all forbidden speculative machinery. A
database-backed queue with one consumer is the whole of what FR-045 asks for, survives a restart
(D-06), and makes "your position is 3" a `COUNT`, not a bookkeeping structure that can drift from
reality.

**Alternatives considered.**

- **An in-memory `chan Job`.** Loses the queue on restart, cannot answer "what is my position"
  after a restart, and duplicates state that the row already holds.
- **A worker pool.** Directly contradicts FR-045, and two concurrent multi-gigabyte archive writers
  on a cupboard machine is the failure the requirement exists to prevent.
- **Producing synchronously in the request.** What upstream did. It is a timeout in a request thread
  holding a SQLite connection (SHARED-DESIGN §1.2), and FR-005/FR-044 forbid it explicitly.

---

## D-06

**Decision.** `Runner.Reconcile(ctx)` runs once during `OnServe`, **before** the worker starts, and
moves every `export_jobs` row with `status = "running"` to `status = "failed"`,
`error_code = "interrupted"`, `finished_at = now`, clearing `artifact`. Each transition writes one
`job_failed` audit entry with `actor_kind = "system"`.

Because scratch files live in `.pb_temp_to_delete` (D-07), which PocketBase deletes on every
`Bootstrap()` (`core/base.go:449`), there is nothing partial left on disk to clean up separately.

**Rationale.** This closes SHARED-DESIGN §8 **R9** and FR-049 and the environment-failure edge case
"the instance is restarted while an export or a report is being produced". A single-instance
application with an in-process worker has exactly one failure mode here, and it is deterministic, so
the fix is deterministic too.

**Alternatives considered.**

- **A heartbeat column and a staleness threshold.** Necessary only if more than one process could be
  running the job. MediGo forbids that (Technology Constraints). Rejected as machinery for a
  topology that cannot exist.
- **Leaving them and letting the retention purge collect them.** They would report themselves as
  running for up to a week, which is precisely what FR-049 forbids.

---

## D-07

**Decision.** Every in-progress artifact is written to
`filepath.Join(app.DataDir(), core.LocalTempDirName)` — that is `pb_data/.pb_temp_to_delete` —
and only moved into the `export_jobs.artifact` file field on success.

**This is load-bearing, not incidental**, and it was chosen after reading the source:

- `core/backup_create.go:83-89` sets the backup's exclude list to
  `{LocalBackupsDirName, LocalTempDirName, LocalNotifyDirName, LocalAutocertCacheDirName,
  lostFoundDirName}`. A half-written archive in the temp dir therefore **cannot** be captured by a
  backup taken at the same moment — which is exactly the specification's edge case *"A backup is
  taken while an export is running: both complete, and the archive either contains the finished
  artifact or does not, never a half-written one."*
- `core/base.go:449` does `os.RemoveAll(filepath.Join(app.DataDir(), LocalTempDirName))` on every
  `Bootstrap()`, so a restart mid-export erases the scratch file with no code of MediGo's own, which
  is the other half of D-06.
- `core/backup_restore.go:63` excludes the same directory from the restore, so a restore cannot be
  confused by it either.

**Rationale.** Using `os.TempDir()` would have failed both edge cases and, on a distroless image with
a `/data` volume, would also have put a multi-gigabyte archive on the container's writable layer
instead of the volume.

**Alternatives considered.**

- **`os.TempDir()` / `t.TempDir()`-style scratch.** Wrong filesystem, wrong lifetime, and it is
  outside the volume.
- **A MediGo-owned `pb_data/medigo_tmp`.** Would be **included** in backups, failing the edge case
  above, and would survive restarts, failing D-06.
- **Streaming straight into the file field.** PocketBase's file field takes a
  `*filesystem.File`, which wants a complete file or a byte slice
  (`filesystem.NewFileFromPath` / `NewFileFromBytes`). A multi-gigabyte export cannot be a byte
  slice.

---

## D-08

**Decision.** `export_jobs.artifact` is a `core.FileField` with **`Protected: true`** (constitution
VII, no exception). It is served exclusively by
`GET /api/v1/exports/{id}/download`, which authorizes the caller against `export_jobs.owner`,
checks `status = "succeeded"` and `expires_at > now`, then streams through
`app.NewFilesystem()` + `fsys.Serve(...)` with `Content-Disposition: attachment`. **No file token is
minted, ever** — a credential in a URL lands in logs, proxies and referrer headers.

The boot assertion that every file field on the instance is `Protected: true` now covers three
fields — `patients.photo`, `attachments.file`, `export_jobs.artifact` — and its test is extended to
name all three, so adding a fourth without protecting it fails the build.

**Rationale.** FR-013, FR-048 and the permission-boundary edge case *"The address of a produced
document or an archive is not a credential"*. Verified fact: PocketBase's file handler performs its
authorization check only inside `if fileField.Protected` with no else branch, so an unprotected field
is served to any anonymous caller who knows the URL (constitution VII, VERIFIED-SOURCE-FACTS).

**Alternatives considered.**

- **PocketBase's `?token=` file token.** Forbidden by the constitution for the reason above.
- **A signed expiring URL of MediGo's own.** Same defect with extra code: possession of the address
  becomes the credential, which FR-013 forbids in those words.
- **Storing artifacts outside PocketBase, on the raw filesystem.** Loses cascade deletion with the
  owning row, which is what makes FR-051 ("deleting an account destroys its archives") free.

---

## D-09

**Decision.** The worker resolves authorization **when it dequeues the job**, not when the job was
requested:

1. Reload `export_jobs.owner`; if the account is missing or `disabled_at != ''`, fail the job with
   `error_code = "owner_unavailable"`.
2. Build an `access.Actor` from that user record.
3. For every patient in scope, call `access.Authorizer.Patient(ctx, actor, patientID, PermView)`. A
   patient that no longer resolves is **dropped from the result and named in the manifest as
   withdrawn**, not silently included.
4. Re-check per record through the same repository queries every other read path uses, so a share
   revoked between request and production removes the data.

**Rationale.** FR-011 in words: *"only records the requester could reach at the moment production
began, re-checked at that moment"*. US2 acceptance scenario 7 says the same for exports. The
permission-boundary edge case says it a third time. Doing it at request time would let a queued job
carry stale access across a revocation, which is the exact failure phase 005 exists to prevent.

**Alternatives considered.**

- **Snapshotting the accessible-patient set at request time.** Faster and wrong: a revocation during
  the queue wait would still export the chart.
- **Re-checking per record only.** Would be correct but does the work twice; the patient-level check
  is the cheap gate and the record-level queries are already patient-scoped.

---

## D-10

**Decision.** New configuration, all under the existing `MEDIGO_` prefix, all with published
defaults, all validated at boot, and all shown on `/admin` so an operator never has to read the
source to learn a window:

| Variable | Default | Meaning |
|---|---|---|
| `MEDIGO_REPORT_MAX_RECORDS` | `5000` | FR-010's documented maximum for one document |
| `MEDIGO_REPORT_MAX_CHARTS` | `12` | FR-023's documented maximum |
| `MEDIGO_REPORT_MIN_CHART_POINTS` | `3` | FR-017's published minimum to be chartable |
| `MEDIGO_REPORT_MAX_CHART_POINTS` | `200` | D-03's decimation threshold |
| `MEDIGO_REPORT_EXTRA_FONT_DIR` | *(empty)* | D-04's escape hatch |
| `MEDIGO_EXPORT_MAX_BYTES` | `10 GiB` | refuse rather than fill the disk |
| `MEDIGO_RETENTION_EXPORT_DAYS` | `7` *(already exists)* | FR-012 **and** FR-047 |
| `MEDIGO_RETENTION_TRASH_DAYS` | `30` *(already exists)* | FR-057 |
| `MEDIGO_RETENTION_AUDIT_DAYS` | `730` *(already exists)* | FR-074 |
| `MEDIGO_BACKUP_WARN_AFTER` | `168h` | US5 AS-4's documented staleness threshold |
| `MEDIGO_STATE_DIR` | `/data/medigo_state` | D-23's restore journal |

Reports and exports share one retention window deliberately: FR-012 and FR-047 both say "a
documented window" and neither asks for two numbers. One knob, one sentence in the handbook.

**Rationale.** `caarlos0/env/v11` is the only configuration mechanism (constitution). Every limit the
specification calls "documented" or "published" has to be a value an operator can read and change, or
the word "documented" is a lie.

**Alternatives considered.**

- **Hard-coded constants.** FR-010, FR-017, FR-023 and FR-057 all use the word "documented" or
  "published"; a constant is neither.
- **Separate report and export retention.** Two knobs, no requirement. Principle I.

---

## D-11

**Decision.** `GET /api/v1/audit` returns `AuditEntry` DTOs whose fields are exactly:
`id, occurred_at, actor (opaque id | null), actor_kind, action, target_kind, target_id, patient
(opaque id | null), reason (bounded token | null), affected (count | null), request_id`. **There is
no `ip`** — the column does not exist; phase 001 dropped it deliberately (001 research D-19), and
publishing a field the trail does not store would have been a `nil` column read at best and a
schema error at worst. **There is no title, no name, no label, no
description and no summary field, and the handler performs no lookup against any other collection.**
A reflection test asserts the struct has no field of a type capable of carrying free text beyond the
enumerated vocabularies, and an HTTP test asserts that a request for a page of entries issues no
query against `patients`, `attachments` or any record collection (proved with
`tests.ApiScenario.ExpectedEvents`, which counts the hooks that fired).

**Rationale.** FR-068 does not merely say the entry must contain no content; it says *"the view MUST
NOT retrieve such content to display alongside an entry, including where the thing referred to still
exists"*. That is a statement about the implementation, not the payload, and the only way to make it
true and keep it true is for the DTO to have nowhere to put the answer. It also makes FR-069 free: an
entry about a deleted thing renders identically to one about a live thing, because neither was ever
resolved.

**Alternatives considered.**

- **Resolving names for live targets only.** Precisely what FR-068 forbids, and it would make the
  trail leak by inference: an entry that shows a name proves the record still exists.
- **A redaction pass over a richer DTO.** "Remember to redact" is not a control (constitution VII).

---

## D-12

**Decision.** For an actor whose `role != admin`, the audit query is constrained to
`patient IN (:accessible) OR actor = :me`, where `:accessible` is
`access.ShareReader.ActivePatientsFor(actor)` ∪ owned patient ids, resolved per request with no
cache. `?count=true` counts **that same constrained set**. There is no unfiltered count, no
`total_all`, and no "N entries hidden" affordance anywhere in the DTO or the page.

An administrator (`role = admin`) sees every row, including `actor_kind ∈ {system, superuser}` rows —
sign-in failures, admin-UI sessions, backups, restores, purges.

**Rationale.** FR-071 says the existence of other entries must not be disclosed *"not even as a
count"*. A filtered list plus an unfiltered total is the classic way that requirement is broken, so
the count is defined as a count of the visible set and tested as such.

**Alternatives considered.**

- **Filtering in Go after an unbounded query.** Correct output, unbounded memory on a
  several-million-row trail, and one refactor away from leaking the total.
- **A separate "my activity" endpoint plus an admin one.** Two operations and two DTOs for one
  question, which is exactly the five-projections-of-one-table mistake SHARED-DESIGN §2.3 op 68
  exists to delete.

---

## D-13

**Decision.** `GET /api/v1/audit?format=csv` streams `text/csv` with
`Content-Disposition: attachment`, writing header row then rows through `encoding/csv` directly onto
the response, page by page over the same keyset cursor as the JSON path — so a several-million-row
trail streams with bounded memory. The CSV columns are exactly the DTO fields of D-11, in a fixed
documented order.

**Reading the trail writes nothing. Exporting it writes one `audit_export` entry** carrying the
narrowing (as enum values and dates, never as free text) — and the operator handbook states this in
so many words, so that an absence of read entries is not mistaken for an absence of reads.

**Rationale.** FR-067 and FR-075, verbatim. Streaming rather than assembling is the large-data edge
case *"the export streams rather than being assembled in one piece"*.

**Alternatives considered.**

- **Auditing reads too.** FR-075 forbids it, and it would make the trail grow by reading it.
- **A separate `/audit/export` operation.** A format parameter on one operation is strictly more
  informative and keeps the narrowing semantics identical by construction.

---

## D-14

**Decision.** **This phase makes no change to `attachments`, adds no purge of its own, and offers no
recovery surface.** The trash — soft delete, `deleted_at`, the thirty-day window, restoring, purging
early, the scheduled purge and the orphan sweep — is phase 004's, shipped, and is recovered where
phase 004 put it: as the `?deleted=true` filter on the document library at `/documents`
(phase 004 `contracts/attachments.md`, ops 50/53/54). This phase adds exactly two things about
deleted documents, and both are numbers on the operator overview:

- `trashed_documents` — the count across the **whole instance**, and
- `trashed_bytes` — the bytes they occupy,

each carrying the window that applies, and **neither carrying a file name, a description, nor which
person any of them concerns** (FR-056). The overview links to `/documents?deleted=true`, which is a
pointer, not a second surface (FR-057).

An earlier revision of this decision added a `deleted_by` relation, a `medigo_purge_trash` cron and a
restore-versus-purge conditional write to this phase. All three were written against the earlier
draft of the specification, which restated phase 004's trash. **They are withdrawn.** Phase 004 owns
the purge and its race; `deleted_by` is not required by any requirement in the current specification
and is therefore not added (Principle I).

**One cross-phase discrepancy, now resolved in phase 004.** The current specification's Assumptions
say phase 004 "settled who may remove a document permanently before its window closes: its owner
behind a typed confirmation, the holder of the break-glass credential for any". Phase 004 as first
written made `DELETE /api/v1/attachments/{id}?purge=true` superuser-only. **That has been corrected
in phase 004**, which is where the rule belongs and where it is now stated once: 004 FR-066 and
`specs/004-labs-and-attachments/contracts/attachments.md` §5 allow an early purge to the owner
(behind a typed confirmation naming the file) and to a superuser for any document, and answer a
share recipient with `404`. This phase still **changes nothing and adds no attachment operation**;
the operator handbook documents 004's rule by reference rather than restating it.

**Rationale.** FR-056 and FR-057, and the specification's own decision: *"An earlier draft of this
specification restated soft delete … That contradicted phase 004 on both the authorization rule and
the surface, and it would have given the product two places to restore the same document."* Two
recovery surfaces for one document is the failure this decision exists to prevent, and the cheapest
way to prevent it is to write no code.

**Alternatives considered.**

- **Keeping the `/trash` page from the shared design contract.** It is a second surface for exactly
  the thing phase 004's document library already filters, and the current specification forbids it in
  as many words.
- **Adding `deleted_by` "because an operator will want it".** A column for a requirement nobody
  wrote, on the one collection in the application carrying a `deleted_at`. Principle I.
- **Moving phase 004's purge cron into this phase's job envelope by rewriting it.** Unnecessary: the
  envelope (D-43) wraps the existing registration; the job's body is untouched.

---

## D-15

**Decision.** **This phase ships seven pages, not eight.** `/trash` is removed from the shared design
contract's phase-006 page list, and this phase adds **zero** attachment operations.

| Page | Kept |
|---|---|
| `/reports` | yes |
| `/reports/{id}` | yes |
| `/exports` | yes |
| ~~`/trash`~~ | **removed** — phase 004's `/documents?deleted=true` |
| `/admin` | yes |
| `/admin/audit` | yes |
| `/admin/users` | yes |
| `/admin/backups` | yes |

The product's page total moves from the contract's original 57 to **56**, and the shared design
contract has since been amended to match: SHARED-DESIGN §3.1 no longer lists `/trash` and carries a
note recording the removal. This is also recorded in plan.md's Deviations table, because the contract
is binding on design and a page disappearing from it must be visible. (With phase 003's `/timeline`,
which that phase added on top of the contract, the product ships **57** pages in total.)

**Rationale.** D-14. A page whose entire content is a filtered list phase 004 already renders is a
duplicate, and the smoke gate would then assert two landmarks over one capability.

**Alternatives considered.**

- **Keeping `/trash` as a redirect to `/documents?deleted=true`.** A route that exists only to
  forward is still a route: it needs a landmark assertion it cannot satisfy (it returns a 302), an
  OpenAPI decision, and an entry in the inventory. Delete it instead.
- **Keeping `/trash` as the instance-wide operator view of deleted documents.** That is what the
  overview's two figures are for, and an operator-facing list of deleted documents would carry file
  names, which FR-056 forbids in those words.

---

## D-16

**Decision.** `/api/v1/admin/stats` and `/api/v1/admin/system` are split by **cost**, and every
figure in both carries its own `computed_at`:

- **Live per request** (indexed `COUNT`s, milliseconds): accounts, patients, records per kind,
  attachments, active shares, pending invitations, queued/running/failed jobs, trash rows and trash
  bytes.
- **From a cron-refreshed gauge** (`medigo_storage_refresh`, every 15 minutes, plus once at boot):
  database bytes (`data.db` + `-wal` + `auxiliary.db`) and document bytes (a walk of
  `<DataDir>/storage`). These are stored in memory with their computation time and served from there.

`/admin/system` additionally reports: readiness (the same check `/readyz` performs), process uptime,
build version, last-successful-backup name/time/size and its age against `MEDIGO_BACKUP_WARN_AFTER`,
superuser MFA posture, superuser IP-allowlist posture, SMTP posture, the migration state, and the
list of failed or abandoned work (D-18's query).

**A reflection test asserts every field of both DTOs is an `int`, `int64`, `float64`, `bool`,
`string`-typed enum, RFC3339 timestamp or duration** — never a free-text field — which is US5 AS-7
turned into a build gate.

**Rationale.** US5 AS-2 requires each figure to have "a stated definition and the moment it was
computed", and AS-1 requires zeros with explanations on a brand-new instance rather than blanks. A
directory walk over a 200 GB document store cannot be on the request path, and pretending it is cheap
is how an operator dashboard becomes the thing that takes the instance down.

**Alternatives considered.**

- **Everything live.** The storage walk makes `/admin` unbounded on a large instance.
- **Everything cached.** Then the counts an operator just changed do not move, and US5's independent
  test ("take a backup and confirm the last-backup figure moves") fails.
- **A `metrics` scrape as the data source.** `/metrics` is not publicly reachable (constitution VI)
  and re-parsing your own Prometheus text to render a page is absurd.

---

## D-17

**Decision.** Both postures are readable from Go in v0.40.1, verified in source, so this phase can
render them without a shell-out or a settings-page scrape:

- **Superuser IP allowlist** — `app.Settings().SuperuserIPs` is a `[]string` on the exported
  `settings` struct (`core/settings_model.go:123-125`), validated as IP-or-CIDR
  (`core/settings_model.go:287`) and consumed by the `pbSuperuserIPsWhitelist` middleware
  (`apis/middlewares.go:306-316`). Configured ⇔ `len(...) > 0`.
- **Superuser MFA** — `MFAConfig{Enabled, Duration, Rule}` (`core/settings_model.go:348-358`) hangs
  off the auth collection, so it is
  `app.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers).MFA.Enabled`.

`/admin` renders both, and the boot warning phase 001 emits is asserted here by a regression test
(US5 AS-5 requires the warning "when the instance starts **and** whenever the overview is opened").

This closes SHARED-DESIGN §8 **R6** for the operator surface.

**Rationale.** The constitution makes the admin UI shipping in production conditional on these two
protections. A posture the application cannot see is a posture it cannot warn about.

**Alternatives considered.**

- **Asking the operator to confirm they configured it.** A checkbox is not a control.
- **Probing by attempting a superuser request from a disallowed IP.** Absurd and untestable.

---

## D-18

**Decision.** "When it last signed in" (US5 AS-9) is
`SELECT MAX(occurred_at) FROM audit_events WHERE actor = :user AND action = 'login'`, and "failed or
abandoned work" (US5 AS-6) is a query over `audit_events` for
`actor_kind = 'system' AND action = 'job_failed'` in the last 30 days, unioned with `export_jobs`
rows in `status = 'failed'`. **No `last_login_at` column and no `job_runs` collection are added.**

**Rationale.** The trail already records every login (phase 001) and every scheduled-job failure
(D-14, D-06). Adding a column would create a second source of truth that can disagree with the trail,
and adding a `job_runs` collection would put a thirty-second query's worth of information into a new
table with its own retention, its own migration and its own purge.

**Alternatives considered.**

- **`users.last_login_at`.** A denormalisation with a write on every login, for one figure on one
  page. It also drifts: a restore rolls the column back but the trail is rebuilt from the journal.
- **A `job_runs` collection.** SHARED-DESIGN's entity count is **30** (§1.6) and this would make
  it 31 for something `audit_events` already holds. The rejection is unchanged by the correction:
  the objection was never the size of the number, it was paying a whole new collection — with its
  own migration, its own retention rule and its own purge — for a fact the audit trail already
  stores (corrected 2026-08-27, ANALYSIS N9).

---

## D-19

**Decision.** `PATCH /api/v1/admin/users/{id}` performs, in one `app.RunInTransaction`:

- **Disable** — set `disabled_at = now`, then `record.RefreshTokenKey()` and save. Rotating the
  per-record token key invalidates **every** outstanding token for that account, so open sessions end
  immediately rather than at expiry (this is PocketBase's documented "log out everywhere"). Login is
  refused by the existing auth path with a plain "contact the operator" message, never a reason that
  distinguishes disabled from wrong-password to an unauthenticated caller.
- **Promote / demote** — set `role`.
- **Force a password change** — set `must_change_password = true` (D-20) and `RefreshTokenKey()`.

Three refusals live in `internal/domain/adminuser` as pure functions with exhaustive table tests,
so they hold no matter which caller reaches them:

1. an actor may not change **their own** `role` or `disabled_at` (US5 AS-12);
2. the **last enabled account with `role = admin`** may not be demoted or disabled by anybody
   (US5 AS-13);
3. `role` and `disabled_at` are absent from every self-service DTO by construction (SHARED-DESIGN
   §1.2), so this route is the only way either can change.

**Rationale.** AS-10's "sessions end immediately" is not satisfiable by a flag alone — an issued JWT
stays valid until it expires unless the signing key rotates.

**Alternatives considered.**

- **A session table to revoke against.** PocketBase's tokens are stateless by design; adding a
  session table means re-implementing authentication, which Principle V forbids.
- **Waiting for token expiry.** "Immediately" is in the requirement.
- **Enforcing the last-admin rule only in the handler.** Then `medigo seed`, a future CLI subcommand
  and a test fixture can each lock the instance out. It belongs in the domain.

---

## D-20

**Decision.** `users` gains **`must_change_password`** (bool, default `false`). When it is true the
auth path issues a token whose only usable route is the password-change endpoint; every other
`/api/v1` route returns `403 password_change_required`, and every page redirects to `/settings`
with the forced-change form. Setting a new password clears the flag in the same transaction.

**This is a deliberate deviation from SHARED-DESIGN §1.2**, which lists `must_change_password` among
the fields "not carried over". §2.3's own op 83 already says "`role`, `disabled_at`, forced password
reset", so the route contract anticipated it and only the field list omitted it. Recorded in
plan.md's Deviations section.

**Rationale.** US5 AS-11 requires that the account "is asked to set a new one at its next sign-in and
cannot proceed without doing so". That is a persisted state, not an email.

**Alternatives considered.**

- **Sending a password-reset email instead.** Does not block sign-in, so it fails the requirement in
  its own words; and it depends on SMTP, which a self-hosted instance may not have (phase 005 R10).
- **Disabling the account and telling the person to ask the operator.** A different, blunter action
  that AS-10 already covers.

---

## D-21

**Decision.** All seven backup operations are thin wrappers over PocketBase, and MediGo adds no
second mechanism:

| Op | PocketBase call |
|---|---|
| list | `app.NewBackupsFilesystem()` → `fsys.List("")` → `blob.ListObject{Key, Size, ModTime}` |
| create | `app.CreateBackup(ctx, name)` |
| upload | `fsys.UploadMultipart(fh, key)` after validation |
| preview | filesystem attributes + `medigo.json` from inside the archive (D-25, D-26) |
| download | `fsys.Serve(...)` after re-authentication (D-27) |
| restore | `app.RestoreBackup(ctx, name)` (D-22, D-23, D-24) |
| delete | `fsys.Delete(key)` |

Concurrency is **PocketBase's own guard**: both `CreateBackup` (`core/backup_create.go:68`) and
`RestoreBackup` (`core/backup_restore.go:52`) refuse when `app.Store().Has(core.StoreKeyActiveBackup)`
(`core/backup.go:14`). MediGo reads the same key to answer "a restore is already in progress" with a
`409 conflict` **before** doing any work, and maps PocketBase's own error to the same code if it
loses the race (US7 AS-10).

The restore handler mirrors `apis/backup.go`'s shape, which exists because `RestoreBackup` ends in
`app.Restart()` → `execve` (`core/base.go:805-830`) and no response can be written after that: MediGo
responds **`202 Accepted`** with the safety-backup reference and the expected downtime, then performs
the restore in an owned goroutine after a short delay.

Scheduled backups are PocketBase's: `Settings().Backups.Cron` and `Backups.CronMaxKeep`
(`core/settings_model.go:476-490`), registered by `registerAutobackupHooks` (`core/backup.go:33-105`)
with the max-keep pruning already implemented. MediGo sets both from configuration at boot and
**surfaces failures**, which PocketBase logs but does not report: an `OnBackupCreate` hook records
`backup_create` on success and `job_failed` on error, which is what puts a failed scheduled backup on
`/admin` and in the trail (US7 AS-3).

**Rationale.** Principle V: a task that reimplements backup or restore is rejected unless the plan
records why PocketBase's version is unusable. It is usable. What it does not do is **tell anybody**
when it fails, and that is the gap this phase fills.

**Alternatives considered.**

- **A MediGo backup format.** Would have to reimplement the concurrent-safe zip of an open SQLite
  database that `core/backup_create.go` already implements with `VACUUM INTO` and an exclusion set.
- **Exposing PocketBase's `/api/backups` directly.** Its download route "relies on superuser file
  token" (`apis/backup.go:22`) — a credential in a URL, forbidden by constitution VII — and it is
  superuser-only, so a MediGo `admin` could not use it.
- **A cron of MediGo's own.** Duplicates `registerAutobackupHooks` including its `maxKeep` pruning.

---

## D-22

**Decision.** `POST /api/v1/admin/backups/{name}/restore` performs, in order, and stops at the first
failure without touching anything:

1. authorize (`role = admin`), re-authenticate (password in the body), and check the confirmation
   phrase equals the literal `restore {name}`;
2. refuse with `409` if `app.Store().Has(core.StoreKeyActiveBackup)`;
3. refuse with `409` if any `export_jobs` row is `queued` or `running` (D-28);
4. read and validate the archive: it exists, it opens as a zip, it contains `data.db`, and its
   `medigo.json` is version-compatible (D-25);
5. **take the safety backup** with `app.CreateBackup(ctx, "medigo_safety_<YYYYMMDDHHMMSS>_<name>")` and wait
   for it. **If it fails, the restore does not proceed** and the response says so;
6. write the restore journal (D-23) and re-apply the environment snapshot (D-24);
7. respond `202` with the safety-backup name;
8. after a one-second delay, call `app.RestoreBackup(ctx, name)` in an owned goroutine with a
   ten-minute context.

**Rationale.** US7 AS-7 in its own words: *"a safety copy of the current state is taken first and its
reference is reported to the administrator before anything is replaced; and given the safety copy
cannot be taken, then the restore does not proceed."* Steps 4 and 5 are the whole of US7's value over
what PocketBase already ships.

**Alternatives considered.**

- **Taking the safety backup asynchronously.** Then step 7 cannot report its reference, and a failure
  is discovered after the data is gone.
- **Skipping the safety backup when the archive is recent.** An optimisation that removes the only
  thing standing between a tired administrator at eleven at night and permanent loss.

---

## D-23

**Decision.** MediGo writes a **restore journal** to `MEDIGO_STATE_DIR` (default `/data/medigo_state`,
i.e. on the volume but **outside** `pb_data`) immediately before calling `RestoreBackup`:

```json
{ "started_at": "...", "archive": "...", "archive_taken_at": "...",
  "safety_backup": "...", "actor": "<opaque user id>", "request_id": "..." }
```

On the next `OnBootstrap` after `e.Next()`, if the journal exists MediGo writes the corresponding
`backup_create` (safety copy) and `backup_restore` audit entries into the **restored** database,
then deletes the journal. If the journal exists but the database is unchanged (the restore failed
before completing), the boot path writes a `job_failed` entry instead, so the trail records what
happened either way.

**All three replay rows take their `request_id` from the journal's recorded `request_id`** — the
correlation id of the restore request that wrote the journal — which is why that field is in the
journal at all. The boot path has no HTTP request of its own and `audit_events.request_id` is
`Required`, so a journal written by an older build with no `request_id` falls back to the boot run's
**`run_id`**, minted by the same helper (001
[data-model](../001-walking-skeleton/data-model.md) §3). Either way the row correlates to a real run;
neither way is the empty string.

**Rationale.** This is the sharpest correctness problem in the phase, and it is not obvious.
`RestoreBackup` replaces the entire `pb_data` with the archive's content
(`core/backup_restore.go:30-46`). **Any audit row written before the restore is therefore destroyed
by the restore.** US7 AS-14 requires the opposite: *"the restore, the safety copy and their
references appear, and the entries survive the restore itself being reflected in the data."* The only
place to keep that intent is somewhere the restore does not touch, and the only such places are
`pb_data/backups`, `pb_data/.pb_temp_to_delete` (deleted at every bootstrap — useless here) and
outside `pb_data` entirely. Outside is the honest choice: it is not a backup, so it does not belong
in the backups directory.

It also handles the environment-failure edge case *"the instance is restarted during a restore: it
returns either on the archive's data or on the data it had before, never on a mixture, and the trail
records what happened."*

**Alternatives considered.**

- **Writing the audit rows before the restore.** They are erased by the restore. This is the
  seductive wrong answer.
- **Writing them into `pb_data/backups/.medigo_restore_journal.json`.** Survives, but that directory
  is the backups filesystem: the file would appear in `fsys.List("")` and MediGo would have to filter
  its own dotfile out of the operator's archive list forever.
- **Re-deriving the restore from the archive's own contents at boot.** An archive does not record
  that it was restored.

---

## D-24

**Decision.** `cmd/medigo/main.go` captures `os.Environ()` into a package-level snapshot **as its
first statement, before `config.Load()` runs**. Immediately before calling `app.RestoreBackup`, the
admin service re-applies that snapshot with `os.Setenv` for every variable currently absent.

**Rationale.** Two facts that are individually harmless and jointly a production outage:

1. `core/base.go:829` restarts the process with `execve(execPath, os.Args, os.Environ())` — the
   **current** environment, read at restart time.
2. The constitution requires secrets to arrive with `caarlos0/env`'s `,file,unset`, and `unset`
   calls `os.Unsetenv` after reading, precisely so the secret does not linger in `os.Environ()`.

So by the time a restore restarts the process, every secret-bearing variable has been deleted from
the environment the new process image will inherit. The instance comes back with no Sentry DSN, no
OTLP headers and, depending on which variables carry `,unset`, possibly no `MEDIGO_PUBLIC_URL` — and
it comes back *after a disaster recovery*, which is the worst possible moment to discover it. This
was found by reading `Restart` rather than by testing, because a test shorter than an actual restore
never exercises it.

A regression test asserts the snapshot is captured before `config.Load` and that the pre-restore
re-apply restores every variable the process started with.

**Alternatives considered.**

- **Dropping `,unset`.** Weakens a constitution-mandated control to work around a framework detail.
- **Forking `Restart` to pass a saved environment.** Requires forking PocketBase; the constitution
  permits framework workarounds on unexported internals only where there is no alternative, and here
  there is a three-line one.
- **Relying on the container runtime to re-inject the environment.** `execve` replaces the process
  image; the container is never restarted, so the runtime never gets the chance.

---

## D-25

**Decision.** At every boot, MediGo writes `<DataDir>/medigo.json`:

```json
{ "app": "medigo", "app_version": "v1.4.2",
  "schema_version": "1756900300_export_jobs", "written_at": "..." }
```

`schema_version` is the id of the highest applied MediGo migration. Because `CreateBackup` zips the
whole of `pb_data` minus five known directories (`core/backup_create.go:83-89`), this file rides
inside every archive.

**The compatibility rule**, stated once and enforced in one function:

- archive `schema_version` **≤** running binary's highest known migration id → **allowed**
  (PocketBase runs app migrations up on the next bootstrap);
- archive `schema_version` **>** the binary's highest → **refused**, because MediGo cannot migrate
  down into a binary that does not know the schema;
- archive with **no** `medigo.json` (an archive taken by a bare PocketBase, or by a pre-006 MediGo) →
  the preview says "version unknown" and the restore is **refused** unless the operator passes
  `"accept_unknown_version": true` in the body, which is recorded in the trail.

**Rationale.** The specification's edge case: *"An archive uploaded from a different version of the
application: the preview states the version it was taken with, and a restore from an incompatible
version is refused with an explanation rather than attempted."* PocketBase's archive carries no
application version of its own, so MediGo has to put one there.

**Alternatives considered.**

- **Reading the `_migrations` table out of the archived `data.db`.** Requires opening a second SQLite
  database from inside a zip — a temp extraction of the whole database file, for information a 200-byte
  JSON file answers.
- **Trusting the archive's filename.** PocketBase's generated names carry a timestamp, not a version,
  and an uploaded archive arrives named anything — so op 86 normalises and bounds the storage key
  to 64 characters before storing it, because that key is what the `backup_upload` audit row writes
  into `target_id`, which is `Max 64` (ANALYSIS N2).
- **Allowing a newer archive and hoping.** "Attempted rather than refused" is what the edge case
  forbids.

---

## D-26

**Decision.** The preview reads `medigo.json` out of the archive as follows:

- when `app.Settings().Backups.S3.Enabled == false` (the default, and MediGo's shipped configuration):
  open `filepath.Join(app.DataDir(), core.LocalBackupsDirName, name)` directly with
  `zip.OpenReader`, read the one entry, close;
- when S3 backup storage **is** configured: stream `fsys.GetReader(key)` into a scratch file under
  `.pb_temp_to_delete` (D-07), open that, read, delete.

**Rationale.** `archive/zip` needs an `io.ReaderAt` and a size to read a central directory;
`*blob.Reader` is a sequential stream. The local branch avoids copying a several-gigabyte archive to
answer a two-line question (the specification's large-data edge case names multi-gigabyte archives
explicitly). The S3 branch is fifteen lines and exists so the feature does not simply break for an
operator who configured S3 backups in PocketBase's settings, which MediGo does not control.

**Alternatives considered.**

- **Always copy.** Correct and pays a multi-gigabyte copy on every preview.
- **Refuse the preview under S3.** Punishes a supported PocketBase configuration.
- **Reach into the local path always.** Wrong the moment S3 is enabled, and silently so.

---

## D-27

**Decision.** Archive download is **`POST /api/v1/admin/backups/{name}/download`** with
`{"password": "..."}` in the body, streaming `application/zip` with
`Content-Disposition: attachment`. **This deviates from SHARED-DESIGN §2.3 op 88, which specifies
`GET`** (path unchanged, method changed); recorded in plan.md's Deviations.

The page renders an ordinary `<form method="post">`, so the browser performs the download from the
response without JavaScript, without a custom header, and without a token anywhere near the URL.

**Rationale.** US7 AS-11 requires three things at once: *"they must re-enter their password, the
download is authorized on every request rather than by possessing its address, and it is recorded as
the most sensitive action on the instance."* A `GET` cannot carry a password without either putting
it in the query string (a credential in a URL, forbidden) or requiring a custom request header, which
a browser navigation cannot send — and under `script-src 'self' 'unsafe-eval'` with the Datastar
inline-script SDK family banned (SHARED-DESIGN §3.3) a `fetch`-plus-blob download is not available
either. `POST` with a form body satisfies all three constraints with no new machinery.

Every call, successful or not, writes a `backup_download` audit entry.

**Alternatives considered.**

- **A short-lived single-use download grant minted by a password-confirming POST.** Re-introduces
  "possession of the address grants access" for the grant's lifetime, which the requirement names.
- **Elevating the session for five minutes after a password re-entry.** A sudo-mode. More state, more
  edge cases, and it makes "authorized on every request" false.
- **HTTP Basic on the download route.** A browser credential prompt in the middle of an application
  is hostile, and the password would be re-sent on every subsequent request to the realm.

---

## D-28

**Decision.** A restore is refused with `409 conflict` and the message *"an export or report is being
produced; wait for it to finish or cancel it"* whenever any `export_jobs` row is `queued` or
`running`. The check happens before the safety backup (D-22 step 3).

**Rationale.** The specification's concurrency edge case: *"An export is running when a restore is
confirmed: the restore is refused until the export finishes or is cancelled, because a restore
replaces the storage the export is writing into."* Literally true here: the worker's scratch file
lives under `pb_data` (D-07) and its output row lives in the database the restore is about to
replace.

**Alternatives considered.**

- **Cancelling the jobs automatically.** Destroys a user's work to serve an operator's convenience,
  without asking either of them.
- **Letting both run.** The restore's `os.Rename` of `pb_data` under an open writer is the mixture
  outcome US7 AS-8 forbids.

---

## D-29

**Decision — the export archive, format version `1.0`.** One `.zip`, produced by `archive/zip`
streaming onto the scratch file:

```
manifest.json                       required; the map of everything else
data/patients.json                  JSON array, one object per patient
data/<kind>.json                    one file per exported kind, e.g. data/medications.json
data/tags.json
data/report_templates.json
data/preferences.json
data/audit_events.json              only entries concerning the exported people
tables/<kind>.csv                   present only when CSV was requested
documents/<kind>/<record_id>/<attachment_id>__<original name>
README.txt                          points at docs/export-format-v1.md and states the version
```

- **JSON is streamed token by token** with `encoding/json/v2`'s `jsontext.Encoder` — array open,
  one value per record, array close — so a 10,000-record export never holds more than one record in
  memory. Slices marshal as `[]`, never `null` (Go 1.27 semantics, SHARED-DESIGN §5.5).
- **The objects are the API's DTOs**, not domain types and not PocketBase records. See D-30.
- **CSV** is one file per kind, header row from the DTO's JSON field names in declaration order,
  nested objects flattened with a dotted key, written through `encoding/csv`.
- **Dates that are date-only stay `YYYY-MM-DD` strings** everywhere and are never parsed into a
  `time.Time` in the export path, which is what makes the edge case *"a date-only value appears as
  the same calendar date … regardless of the viewer's time zone"* true by construction rather than by
  timezone care.
- **Document names** preserve the original name and prefix it with the attachment id and a double
  underscore, so two attachments of the same name on the same record cannot collide. Path separators
  and control characters in the original name are replaced with `_`; nothing else is altered.
  Go's `archive/zip` sets the UTF-8 general-purpose bit for names that are not representable in
  CP437, so a non-Latin filename survives.
- **`manifest.json`** carries: `format_version`, `produced_at`, `app_version`, `account` (opaque id),
  `patients[]` (id, and display name only when the requester asked for patient information),
  `kinds[]` with a count each, `documents_included` (bool) and `documents[]` mapping every archive
  path to its `attachment_id`, `record_id`, `kind`, `patient`, `original_name`, `size_bytes` and
  `sha256`, `withdrawn[]` (patients dropped by D-09), and `files[]` describing the meaning of every
  other entry in the archive. FR-038 asks for exactly this list.
- The format is documented in **`docs/export-format-v1.md`**, published with the release, and a test
  asserts every top-level key the document describes is produced and every key produced is described.

An account holding nothing still produces an archive with a manifest and empty arrays (US2 AS-1), so
"I hold nothing" is provable rather than indistinguishable from a failure.

**Rationale.** FR-037 through FR-042. Streaming is the large-data edge case. DTO reuse is D-30.

**Alternatives considered.**

- **JSON Lines.** Streams just as well and is marginally less familiar to a person opening the
  archive with a text editor; a JSON array reads as one document and `jsontext` streams it natively.
- **One giant `export.json`.** Cannot be opened by a spreadsheet, cannot be read incrementally, and
  makes the per-kind counts in the manifest unverifiable by eye.
- **SQLite as the export format.** Machine-readable, not human-readable, and "readable without the
  application or any part of it" (FR-039) reads badly when the answer is "install a database tool".

---

## D-30

**Decision.** Every object in an export archive is produced by the **same DTO encoder the `/api/v1`
handlers use**. There is no export-specific serialiser, and there is no denylist of fields to strip.

FR-043 then holds structurally: `api.PatientSummary` has no `password` field, `api.UserPreferences`
has no `tokenKey` field, and no DTO anywhere carries a PocketBase settings value — so a credential,
a secret or an operator setting cannot enter an archive, because there is nowhere for it to come
from. Two tests keep it that way:

1. a **scanning test** that produces an export over the seeded fixture and asserts the archive bytes
   contain none of: the seeded passwords, any `tokenKey`, any `MEDIGO_`-prefixed string, the SMTP
   password, the Sentry DSN, or any patient id belonging to an account the exporter cannot reach;
2. a **reflection test** over every DTO reachable from the exporter asserting no field name matches
   `(?i)(password|secret|token|key|dsn)`.

**Where the encoder lives, because Principle II asks.** `internal/service/exportjob` may not import
`internal/web/api` — that package imports `net/http` — so the service declares a one-method port,
`exportjob.Archiver`, and the DTO encoding happens in the adapter `internal/render/archive`, which
may import `internal/web/api` and is added to the `depguard` allowlist for exactly that. The adapter
imports no PocketBase. This mirrors `report.Renderer` / `internal/render/pdf` ([D-01](#d-01)), and it
is why both adapters exist at all rather than being speculative seams.

**Rationale.** FR-043 lists five categories that must not appear. A denylist is a control that fails
open the first time somebody adds a field; DTO shape is a control that fails closed. This is the same
mechanism SHARED-DESIGN §5.5 credits with preventing privilege escalation on create.

**Alternatives considered.**

- **Marshalling PocketBase records directly.** Would emit every column including internal ones, which
  is how upstream's export leaked more than it meant to.
- **A redaction pass.** "Remember to redact" is not a control (constitution VII).

---

## D-31

**Decision.** Job progress on `/exports` and `/reports` is delivered by **polling** —
`data-on-interval__2s` re-requesting the list fragment while any job is `queued` or `running`, and
stopping when none is. No new SSE stream route is added.

**Rationale.** Three reasons, in order of weight. (a) SHARED-DESIGN §2.3 allocates phase 006 **25**
operations (corrected 2026-08-27, ANALYSIS N10 — this decision was written when the contract said 23,
before this plan's deviation table claimed op 91 and op 4) and **not one of the 25 is a stream**;
the suite has exactly two SSE routes, ops 29 and 67, both owned by earlier phases, so a
`/api/v1/streams/jobs` would be this phase's first and would change the shared contract for a
progress bar. The correction does not weaken the reason: the objection was never the size of the
allocation but that a stream is a category of operation this phase was given none of, and the two
operations that moved the figure from 23 to 25 are a `POST` cancel and an auth callback — neither
of them a stream, so the count of streams allocated to 006 is still zero.
(b) A stream is where the PocketBase five-minute `WriteTimeout` trap lives, and a job list is
not worth that exposure. (c) `data-on-interval` is in the free Datastar attribute set — it is not on
SHARED-DESIGN §3.3's Pro list — so it costs nothing.

**Alternatives considered.**

- **A `/api/v1/streams/jobs` SSE route.** A twenty-sixth operation, a new smoke exclusion, and a new
  long-lived connection, to avoid a request every two seconds on a page nobody leaves open.
- **Reusing phase 005's notifications stream.** Couples an operator-facing progress bar to the
  sharing notification vocabulary, and every subscriber would receive job events they cannot use.

---

## D-32

**Decision.** The regression test for PocketBase's hardcoded five-minute `WriteTimeout` is a
Go test behind the build tag `slowsse`, in `internal/web/stream/liveness_slow_test.go`. It opens
`GET /api/v1/streams/records` against a real test app, holds it for **11 minutes**, asserts a patch
arrives after the ten-minute mark, and asserts the connection was never closed by the server. A
`task test:slowsse` wrapper runs it, and a dedicated CI job runs it on merge to `main` and nightly —
not on every pull request, where twelve minutes of wall clock would be paid for every push.

**Rationale.** This closes SHARED-DESIGN §8 **R7**, which assigns the CI job to phase 006, and US9
AS-7 requires it in the specification's own words: *"proven by an automated test that holds one open
for longer than that rather than assumed."* The failure mode is silent — every test shorter than five
minutes passes — so the only defence is a test that is actually longer.

**Alternatives considered.**

- **Asserting the `WriteTimeout` field value instead.** Tests that the line of code exists, not that
  the stream survives; a middleware or a proxy could still close it.
- **Shortening the timeout in the test to make it fast.** Then the test proves nothing about the
  production configuration, which is the only thing at issue.
- **Running it on every PR.** Twelve minutes of CI per push to catch a regression that arrives once a
  year. Nightly plus merge is the proportionate answer.

---

## D-33

**Decision.** A build-tagged `scale` suite seeds, once, into a throwaway test app: **10,000 records
across the kinds, 2,000 attachments, 3 patients, 1,000,000 audit entries, 500 readings of one lab
component**, then asserts these budgets, each of which fails the build when exceeded:

| Journey | Budget |
|---|---|
| `/reports` builder, per-kind counts rendered | ≤ 2 s |
| resolving a selection's count as the selection changes | ≤ 500 ms |
| a 5,000-record report produced end to end | ≤ 120 s |
| a complete export of 10,000 records + 2,000 documents | ≤ 300 s, memory ≤ 256 MiB RSS |
| `/admin/audit` first page over 1,000,000 entries | ≤ 2 s |
| paging 50 pages of the audit trail | 0 repeated, 0 skipped |
| audit CSV export of 1,000,000 entries | streams, memory ≤ 128 MiB RSS |
| `/admin` overview | ≤ 1 s |
| `/exports` list with 200 requests | ≤ 2 s |
| `/admin/users` list with 500 accounts | ≤ 2 s |
| `/admin/backups` list with 60 archives | ≤ 2 s |

`task test:scale` runs it; CI runs it on merge to `main`.

**Rationale.** US9 AS-5 requires that "a regression beyond a budget fails the build rather than being
noticed later", and the specification's large-data edge cases name 10,000 records, 2,000 documents,
several million audit entries and 500 readings explicitly. The memory budgets are what make D-13's
and D-29's streaming claims real rather than aspirational.

**Alternatives considered.**

- **A load-testing tool against a deployed instance.** Not a build gate, and not reproducible on a
  contributor's laptop.
- **Timing assertions in the ordinary test suite.** Makes every unit test flaky on a busy CI runner.

---

## D-34

**Decision.** A build-tagged `phileak` suite (**phase 001 created `internal/testsupport/phileak` and
`task test:phileak`; phases 002–005 extended the harness; this phase extends it to the whole
application**) does the following in one process:

1. seeds a fixture whose PHI is a set of **unmistakable sentinel strings** — `ZZPATIENTNAMEZZ`,
   `ZZDIAGNOSISZZ`, `ZZNOTEZZ`, `ZZTAGZZ`, `ZZFILENAMEZZ`, `ZZ1970-01-01ZZ`;
2. installs a capturing `io.Writer` as zerolog's sink, an in-memory Prometheus registry, an
   in-memory OTel span recorder, and a Sentry transport that captures rather than sends;
3. exercises **every route in the registry** — the inventory drives the list, so a new route is
   automatically covered — plus every cron job and the export and report workers;
4. asserts none of the sentinels appears in the captured logs, in `/metrics` text, in any span name
   or attribute, in any Sentry payload, or in any metric label value;
5. additionally asserts no metric label value is an opaque id (bounded-cardinality rule,
   constitution VI).

**Rationale.** US9 AS-6. Sentinels rather than realistic names because a grep for "John" finds
nothing useful and a grep for `ZZPATIENTNAMEZZ` cannot false-negative.

**Alternatives considered.**

- **Reading the code and reasoning about it.** Not repeatable, and constitution IX forbids
  review-vigilance where a gate is possible.
- **A regex denylist over log output in production.** Discovers a leak after it has been written.

---

## D-35

**Decision.** A build-tagged `netgate` test replaces `http.DefaultTransport.DialContext` and
`net.Dialer` use in the process with a dialer that records every address and **fails the test on any
non-loopback dial**, then boots MediGo with Sentry, OTLP and SMTP all unconfigured and exercises
every route in the registry plus every cron job.

**Rationale.** US9 AS-8 and constitution VII: *"MediGo makes no outbound network request that the
operator has not explicitly configured."* An assertion at the socket layer is the only one that
cannot be fooled by a library that dials on its own.

**Alternatives considered.**

- **Running the container with no network and watching for errors.** Proves a request failed, not
  that none was made; and a swallowed error looks identical to no request.
- **Auditing dependencies for network calls.** Not a gate, and transitively unbounded.

---

## D-36

**Decision.** The privacy review is two files:

- `docs/privacy-review-checklist.md` — the repeatable procedure, one line per clause of constitution
  VII plus one per privacy-bearing FR of phases 001–006, each naming the exact test, gate or command
  that answers it;
- `docs/privacy-review.md` — the written result of running it against the finished application, dated
  and versioned, with every finding either **fixed** (naming the commit) or **recorded with a
  reason**.

Nothing in it is a judgement call that is not backed by a named artefact, so running it again after a
change produces a comparable result.

**Rationale.** US9 AS-10: *"it is written down, every finding is either fixed or recorded with a
reason, and the review is repeatable rather than a one-off reading."*

**Alternatives considered.**

- **A single review document.** Not repeatable; the next reviewer starts from nothing.
- **A linter.** Most clauses of constitution VII are not mechanically checkable; the ones that are
  already have gates (D-34, D-35, the `Protected: true` assertion, `depguard`, `forbidigo`), and the
  checklist points at them rather than duplicating them.

---

## D-37

**Decision — the produced document's shape**, fixed and documented so two reports of the same
selection are comparable:

1. **Cover / first page** — the person (name, date of birth, sex, reference) when the identifying
   header is included, otherwise the opaque patient reference alone; the photograph when included;
   the moment of production in the instance's timezone **and** in UTC; and the criteria written as
   prose ("Records of 4 kinds, dated 2025-08-26 to 2026-08-26, tagged rheumatology, statuses active
   and chronic"). When the definition came from a saved report that was edited while the document was
   being produced, the first page says which criteria were used and when production started.
2. **Contents** — the sections and their record counts.
3. **One section per selected kind**, in the fixed order published in the operator handbook:
   allergies, conditions, medications, treatments, procedures, encounters, lab results, vitals,
   immunizations, symptoms, injuries, equipment, insurance, emergency contacts, family history.
   **A selected kind that matched nothing gets its section and the sentence "No records of this kind
   matched the selection"** — never a silent omission (FR-008).
4. **Charts**, each with its accompanying table (FR-020).
5. Every page carries a running footer: the person's identity or reference, and "Page N of M" via
   `AliasNbPages`.

Presentation settings (FR-009) are `sort ∈ {date_desc, date_asc, name_asc}` and
`group ∈ {none, kind, year}`, both honoured by the renderer and both round-tripped through a saved
report.

**Rationale.** FR-006 through FR-009. The fixed order is what makes "a documented order" true and
what lets a clinician find the medication list in the same place every time.

**Alternatives considered.**

- **Omitting empty sections.** FR-008 forbids it, and for good reason: "no allergies recorded" and
  "allergies not included in this report" are clinically different statements.
- **Letting the user order the sections.** A knob nobody asked for, and it destroys comparability.

---

## D-38

**Decision.** Saved reports use the application's existing optimistic-concurrency convention
(SHARED-DESIGN §2.1 rule 10): every `report_templates` response carries an `ETag` derived from
`updated`; `PATCH` and `DELETE` require `If-Match`; a mismatch is `412 version_mismatch`. The page
handler re-fetches on `412` and re-renders the form with the current values and a plain message.

**Rationale.** FR-029 and US3 AS-7. Reusing the convention means no new mechanism, and it means the
templates editor behaves the same way as every clinical record editor.

**Alternatives considered.**

- **Returning the current representation inside the `412` body.** Would make this one error envelope
  shaped differently from every other, for a re-fetch the page performs anyway.
- **Last-write-wins.** The requirement says refuse.

---

## D-39

**Decision.** `GET /api/v1/reports/trends?patient=` returns, in one response, both halves of the
picker:

- **vitals metrics** — for each numeric column of the `vitals` kind, the reading count, the first and
  last reading dates, and the single fixed SI unit (vitals are stored in SI, SHARED-DESIGN §1.5, so a
  vitals metric has exactly one unit and no unit choice arises);
- **lab components** — grouped by `(canonical_name, unit)`, each group carrying its reading count and
  span. **A canonical name recorded in more than one unit appears once per unit**, and the response
  marks the name as `multi_unit: true`.

Every entry carries `chartable` (count ≥ `MEDIGO_REPORT_MIN_CHART_POINTS`) and, when false,
`readings` and `readings_needed`, so a value with one reading is **shown as not yet chartable**
rather than hidden (FR-017). Choosing a chart requires a unit whenever the name is `multi_unit`, and
readings in any other unit are excluded from that chart and the exclusion is stated in the document.
**No unit conversion is ever performed, anywhere, under any circumstance** — a `Convert` function does
not exist in the codebase, which is asserted by a `forbidigo`-style grep test.

Selecting a chart range that resolves to fewer than the minimum is rejected **at selection time** by
the same endpoint with the resolved count, not at production time (FR-019).

**Rationale.** FR-016 through FR-019. The upstream bug this exists to prevent — mg/dL and mmol/L
plotted on one axis — is named in SHARED-DESIGN §2.3 op 56 and in the domain dossier. Making the unit
part of the grouping key means the wrong chart cannot be constructed, rather than being detected.

**Alternatives considered.**

- **Converting between known unit pairs.** A conversion table is a place for a clinical error to
  live, and FR-018 forbids it in absolute terms.
- **Picking the most common unit and discarding the rest silently.** Discarding without saying so is
  the failure FR-018's "MUST state that they were excluded" targets.
- **Hiding values below the minimum.** FR-017 forbids it: a person needs to know that two more
  readings would make their blood pressure chartable.

---

## D-40

**Decision.** Each of the seven new pages renders its own ARIA landmark **and** a shared
`@EmptyState(title, body, action)` **inside** that landmark when it has nothing to show, so the
landmark assertion holds on a freshly seeded instance. The landmarks are SHARED-DESIGN §3.1's:

| Route | Landmark | Empty state |
|---|---|---|
| `/reports` | `region[name="Reports"]` | "A report turns this person's records into a document you can hand to a clinician…" |
| `/reports/{id}` | `article[name="Report template"]` | n/a (a template always exists here) |
| `/exports` | `region[name="Exports"]` | "An export gives you everything this account holds, in a documented format…" |
| `/admin` | `region[name="Administration"]` | zeros with definitions, never blanks |
| `/admin/audit` | `region[name="Audit trail"]` | "Nothing has been recorded on this instance yet." |
| `/admin/users` | `region[name="Users"]` | n/a (at least one account always exists) |
| `/admin/backups` | `region[name="Backups"]` | "An archive is a complete copy of this instance…" |

The Playwright gate runs each of the seven, at both viewports, **twice**: once against the populated
seeded account and once against `empty@medigo.local`, which holds nothing (FR-125). `/admin`,
`/admin/audit`, `/admin/users` and `/admin/backups` are additionally run as a non-administrator,
asserting the shared 404 view and an audit row (FR-076, SC-010).

**Rationale.** SHARED-DESIGN §3.0 names an empty state rendered outside the landmark as "the most
common way a smoke gate goes falsely red", and the specification devotes an entire edge-case section
to nothing-there-yet. The three non-admin pages are also reachable by an account with no data at all,
which is the state a brand-new instance is in.

**Alternatives considered.**

- **Redirecting an empty page elsewhere.** Then the route returns 302 and the gate cannot assert its
  landmark, and the person never learns the capability exists.
- **A generic "no data" partial outside the landmark.** Fails the landmark assertion, which is the
  precise trap.

---

## D-41

**Decision.** Reports and exports are **one queue with one worker** (D-05), and a queued or running
job is cancellable through a new operation:

```
POST /api/v1/exports/{id}/cancel        cancelExport        200 -> the job DTO, status "cancelled"
```

**This is a twenty-fourth operation for the phase and a deviation from SHARED-DESIGN §2.3**, which
allocates ops 68–90 and has no cancel; it is recorded in plan.md's Deviations table. The product's
operation count moves from 90 to **91**.

Cancellation is **cooperative**, not a kill:

- a `queued` job is cancelled by a conditional write —
  `UPDATE export_jobs SET status='cancelled' WHERE id=:id AND status='queued'`. If it affects zero
  rows the worker has already taken it, and the running path below applies;
- a `running` job is cancelled by setting `cancel_requested = true`; the worker checks
  `ctx.Err()` and that flag between records, at every progress update, and abandons the scratch file.
  The scratch file is under `.pb_temp_to_delete` (D-07), so nothing partial is reachable and nothing
  needs deleting;
- either way the terminal row has `status = "cancelled"`, an empty `artifact`, and exactly one
  `job_cancelled` audit entry naming the actor.

`DELETE` was rejected as the verb: the request row survives cancellation and is still listed as
cancelled (FR-046, US2 AS-11), so a `DELETE` that leaves the resource in place would be a lie.

**Rationale.** FR-046 and US2 AS-11 in the current specification. `POST /{id}/cancel` follows the
one precedent the application already has for "an act on a resource that is not a field edit",
`POST /api/v1/invitations/{id}/respond` (phase 005 op 65).

**Alternatives considered.**

- **`DELETE /api/v1/exports/{id}`.** Wrong semantics, as above; and it would then have to mean
  *delete the request* as well, which nothing asks for.
- **`PATCH /api/v1/exports/{id}` with `{"status":"cancelled"}`.** Client-supplied state transitions
  are how a state machine becomes a suggestion; the application has one such machine already
  (invitations) and it is not driven that way.
- **Killing the goroutine.** Go has no goroutine kill, and the honest version — a context cancel —
  is exactly what this decision describes.

---

## D-42

**Decision.** `expired` is a **state the purge writes**, not a computed predicate:

- `medigo_purge_artifacts` runs daily. For every `export_jobs` row with `status = "succeeded"` and
  `expires_at < now`, it clears `artifact` (which deletes the blob with it), zeroes `bytes`, and sets
  `status = "expired"`. It writes one `job_succeeded` entry carrying the count (D-43).
- The download route refuses an `expired` job with `410 gone`, code `artifact_expired`, and a plain
  message stating the window that applied — never an error suggesting something went wrong
  (FR-047, US8 AS-2).
- The list and detail views of an expired job offer **re-production**: the criteria are still on the
  row (D-44), so "run it again" is a `POST /api/v1/exports` prefilled from it (US8 AS-1).
- **Deleting an account is immediate and is not the purge's job**: `export_jobs.owner` is
  `CascadeDelete: true`, so the rows and their blobs go with the account in the same transaction
  (FR-051, FR-063, US2 AS-14). A test deletes an account with a succeeded job and asserts the blob is
  gone from `app.NewFilesystem()`, not merely unreferenced.

This is the one place where a computed predicate would have been wrong: FR-012 and FR-047 require the
**content** to stop being stored, which is a write, not a filter.

**Rationale.** FR-012, FR-047, FR-051, FR-060, FR-063, and US8's whole story. Once the purge has run,
nothing in the application can bring the artifact back — there is no code path that writes an
artifact except the worker, and it only ever writes a new one.

**Alternatives considered.**

- **Treating expiry as a read-path predicate, like phase 005's share expiry.** For a grant that is
  right, because nothing is stored. For an artifact it leaves the bytes on disk past the window,
  which is the promise FR-012 makes.
- **Deleting the whole row at expiry.** Then the request vanishes and "shown as expired with the
  window that applied" (US8 AS-1) is unimplementable.

---

## D-43

**Decision.** Every scheduled job in the **whole application** — phase 004's trash purge and orphan
sweep, phase 005's share/invitation tidy, this phase's artifact purge, audit purge and storage-gauge
refresh, and PocketBase's own auto-backup via its hook — is registered through one wrapper in
`internal/platform/pb/jobs.go`:

```go
// Run wraps a scheduled job so that it reports itself exactly once, whatever happens.
func Run(app core.App, name string, fn func(ctx context.Context) (affected int, err error)) func()
```

It writes **exactly one** audit entry per run: `job_succeeded` on success (with `affected` as a
count) or `job_failed` on error (with a bounded `error_code`), both with `actor_kind = "system"`,
`target_kind = "system"` and `target_id = name` — a **bounded job name**, never a record id and never
content. `target_id` is `text ≤64`, which is why the four names of 18–29 characters fit: 001's
`audit_events` sizes the column for exactly this bounded exception (001
[data-model](../001-walking-skeleton/data-model.md) §3).

**The envelope mints the run's correlation id, and it is the only place that has to.** A cron has no
HTTP request and `audit_events.request_id` is `Required`, so `Run` derives a **`run_id`** from the
same helper `internal/obs` uses for request ids, puts it on the `ctx` it hands `fn`, and fills
`request_id` with it on both of its own rows. Every audit row the *job body* writes on that context
— phase 004's per-document `delete` rows, phase 005's `share_expire` and `invite_expire`, this
phase's `purge` rows — inherits the same value, and so do the run's zerolog lines. One `run_id` per
run means "what else did the run that failed at 03:20 touch" is one query, and it means a job cannot
be registered through the envelope and still write a row that correlates to nothing. A panic is recovered and reported as `job_failed`. The wrapper never retries: the next
scheduled run is the retry, which is FR-058's "retried on its next scheduled run rather than
immediately" implemented rather than promised.

"When it last ran" and "when it last succeeded" (FR-055) are then two `MAX(occurred_at)` queries over
`audit_events` filtered by `target_id = name`, and "failed or abandoned work" (FR-085) is a query for
`action = 'job_failed'` in the last 30 days unioned with `export_jobs` rows in `failed`. **No
`job_runs` collection is added**, which keeps the entity count at the shared design contract's
**30** (§1.6; corrected 2026-08-27, ANALYSIS N9 — `006/data-model.md:8` already cited 30).

**Rationale.** FR-055, FR-058, FR-085, US8 AS-6 and AS-7, US5 AS-6. Three separate requirements ask
for the same fact — did this job run, and did it work — and the application already has a durable,
retention-managed, PHI-free place to put it. Wrapping the registration rather than editing each job
body means a job cannot be added without being observable, which is the same trick
`records.Register` plays for record kinds.

**Alternatives considered.**

- **A `job_runs` collection.** A thirty-second query's worth of information given a table, a
  migration, a retention window of its own and a purge of its own. Entity 32 for nothing.
- **Editing each job to log its own outcome.** Then a new job silently reports nothing, and the
  overview lies by omission — the exact failure FR-058 targets.
- **Prometheus counters as the source.** `/metrics` is not publicly reachable (constitution VI) and
  a counter cannot answer "when did it last succeed".

---

## D-44

**Decision.** The count shown in the builder before anything is produced and the records that end up
in the document come from **one** resolver:

```go
package report // internal/service/report

// Selection is the resolved question. It is built once from a definition and is the ONLY thing that
// turns criteria into records, for the count, for the document, and for the export.
type Selection struct { /* patient, kinds, from, to, tagIDs, statuses */ }

func (s Selection) Counts(ctx context.Context, actor access.Actor) (Counts, error)
func (s Selection) Each(ctx context.Context, actor access.Actor, fn func(kind.Kind, records.Payload) error) error
```

`Counts` and `Each` are two methods over one query builder and one authorization call sequence. A
contract test runs both against the same fixture and asserts, for a table of selections including
empty ones, that `Counts` equals the number of payloads `Each` yields, per kind and in total.

**Rationale.** SC-002 requires the per-kind counts to match the produced document *"in 100% of
cases"*, and FR-003 says the figures "MUST match exactly what a report over the same selection would
contain". Two code paths that must agree will eventually not; one code path cannot disagree with
itself. This is also what makes the 500 ms budget for a changing selection (SC-023) meaningful:
`Counts` is `COUNT` over the same indexed predicates `Each` walks.

**Alternatives considered.**

- **A counting query written for speed and a fetching query written for correctness.** The obvious
  design, and the one SC-002 exists to forbid.
- **Counting by fetching.** Correct and pays the full walk on every keystroke of the builder; at
  10,000 records it misses the 500 ms budget.

---

## D-45

**Decision.** Two things about a saved report's identity:

1. **Names are unique per account, ignoring capitalisation.** A unique index on
   `(owner, LOWER(name))`, exactly as `tags` does (SHARED-DESIGN §1.2). A duplicate is
   `409 conflict`, code `duplicate_name`, with a message naming the conflict, and **nothing is
   overwritten** (FR-030).
2. **`report_templates.patient` is a real relation with `CascadeDelete: false`.** SHARED-DESIGN §1.2
   put the patient inside the `criteria` JSON blob; it is lifted out. When the person is deleted,
   PocketBase **empties** a non-cascading relation rather than deleting the referencing row
   (`core/record_model.go:1618-1626`, the same behaviour phase 005 relies on for
   `shares.invitation`), so the saved report survives with an empty `patient`, reports *"the person
   this report was about is no longer available"*, stays editable so another person can be chosen,
   and can never be produced against a different person (FR-032, US3 AS-11).

A saved report whose `patient` is set but no longer **reachable** — access withdrawn rather than the
person deleted — behaves identically at the service layer, because the resolver returns `ErrNotFound`
and the page renders the same panel.

**Rationale.** FR-030 and FR-032. The relation also buys the query "does this account have a saved
report over this person" without parsing JSON, which the deletion-confirmation dialog wants.

**Alternatives considered.**

- **`CascadeDelete: true`.** Deleting a person would silently destroy the saved reports about them,
  which US3 AS-11 forbids: it requires the saved report to *say so plainly* and remain editable.
- **Keeping the patient in `criteria` JSON.** Then FR-032's "the person can no longer be reached" is
  a JSON lookup with no referential integrity, and a deleted person leaves a dangling id that reads
  identically to a withdrawn share.
- **Uniqueness enforced only in the service.** `medigo seed`, a future CLI subcommand and a test
  fixture would each be able to create a duplicate. Uniqueness that is not an index is a convention.

---

## D-46

**Decision.** `internal/config.Config.Validate()` refuses to start the instance, naming the offending
variable and the bound it violated, when any of the following does not hold (FR-113):

| Setting | Rule |
|---|---|
| `MEDIGO_RETENTION_EXPORT_DAYS` | `1 ≤ n ≤ 3650` |
| `MEDIGO_RETENTION_AUDIT_DAYS` | `1 ≤ n ≤ 3650` |
| `MEDIGO_RETENTION_TRASH_DAYS` | `1 ≤ n ≤ 3650` |
| `MEDIGO_REPORT_MAX_RECORDS` | `1 ≤ n ≤ 100000` |
| `MEDIGO_REPORT_MAX_CHARTS` | `1 ≤ n ≤ 50` |
| `MEDIGO_REPORT_MIN_CHART_POINTS` | `2 ≤ n ≤ 100` |
| `MEDIGO_REPORT_MAX_CHART_POINTS` | `MIN_CHART_POINTS ≤ n ≤ 5000` |
| `MEDIGO_EXPORT_MAX_BYTES` | `1 MiB ≤ n ≤ 1 TiB` |
| `MEDIGO_BACKUP_WARN_AFTER` | `1h ≤ d ≤ 8760h` |
| `MEDIGO_BACKUP_KEEP` | `1 ≤ n ≤ 365` |
| `MEDIGO_REPORT_EXTRA_FONT_DIR` | empty, or a readable directory |
| `MEDIGO_STATE_DIR` | writable, and **not** inside `DataDir` (D-23) |

The message is one line per violation, all of them reported at once, and it names the variable, the
value and the bound — never a path outside what the operator typed.

**Rationale.** FR-113 and the specification's environment-failure edge case: *"The instance is
started with a retention window, a limit or a threshold set to a value it cannot honour: it refuses
to start with a message naming the setting, rather than starting and quietly ignoring it."* A zero
retention window that is silently clamped to a default is how a promise about deletion becomes untrue
without anybody noticing.

**Alternatives considered.**

- **Clamping to the nearest legal value and warning.** The instance then enforces a window the
  operator did not choose and the handbook does not describe.
- **Validating lazily, when the job first runs.** The failure surfaces at 3 a.m. in a cron, which is
  the worst possible place for it.

---

## D-47

**Decision.** Every figure on the operator surface is declared **once**, in a Go table, and rendered
from that table:

```go
package opsfig // internal/service/admin/opsfig

type Figure struct {
    Key        string        // "patients_total"; stable, used as the DOM id and the DTO key
    Label      string        // "People"
    Definition string        // "How many people exist on this instance, across every account."
    Unit       Unit          // Count | Bytes | Duration | Version | State | Moment | Setting
    Cost       Cost          // Live | Refreshed
}
```

Three tests hold it together: every key in the DTO exists in the table; every figure in the table is
rendered on `/admin`; and **every figure has a non-empty `Definition`** — so a figure added without
one fails the build (FR-080, SC-011). A fourth, reflection-based test asserts that no field of either
admin DTO is a free-text type outside the enumerated `State` and `Version` cases, which is FR-086
made mechanical.

`Cost` drives D-16's split: `Live` figures are computed on the request; `Refreshed` figures come from
the cron-refreshed gauge and are rendered with the moment they were computed and their age. Both
kinds carry `computed_at`; only `Refreshed` renders an age.

**Rationale.** FR-080 ("every figure MUST carry a stated definition and the moment at which it was
computed"), FR-086, FR-087 and SC-011. A definition that lives in a templ file next to the number is
a definition somebody will forget; a definition that is a required struct field is one they cannot.

**Alternatives considered.**

- **Writing the definitions into the templ markup.** Nothing can then assert their presence, and
  FR-080 becomes review vigilance.
- **Putting the definitions only in the handbook.** FR-087's whole point is that an operator never
  has to read anything but the application to learn a limit.

---

## D-48

**Decision.** No operator view reads a person's records, and the code makes that structural rather
than careful: `internal/service/admin` **declares no port capable of reading a clinical record**. Its
ports are `Counter` (counts by collection), `Storage` (bytes), `Posture` (MFA/IP/SMTP/migration
state), `AccountAdmin` (the `users` operations of D-19) and `Archives` (D-21). None of them returns a
record, a patient name or a file name; the admin service physically cannot render one (FR-088,
US5 AS-15).

Reaching a person's records requires the **break-glass credential** — the PocketBase superuser, whose
admin UI ships in production under constitution VII's hardening. Every such session is already
recorded by phase 001 as `admin_session` with `actor_kind = "superuser"`; this phase is where those
entries become readable, and the trail view labels them as administrative sessions (FR-120,
US6 AS-10, US7 AS-15). The `/admin` overview states the posture of that credential and warns until it
is protected (FR-083, D-17).

**Rationale.** The specification says it three times — FR-088, US5 AS-15 and the permission-boundary
edge cases — and it is the distinction most likely to be eroded by a well-meaning "just show the
patient's name next to the count" change. An interface that has no method for it cannot acquire one
by accident in a rush.

**Alternatives considered.**

- **Giving the admin service a read-only record port "for support".** That is the break-glass
  credential, with none of its auditing and none of its warnings.
- **Relying on the authorizer to refuse.** It would refuse, and the temptation to run the admin
  service as a superuser to "make the dashboard work" is exactly how that gets bypassed.

---

## D-49

**Decision.** A disabled account is **undetectable to an unauthenticated caller**:

- wrong credentials → `401`, code `invalid_credentials`, byte-identical whether the account is
  disabled, enabled, or does not exist;
- **correct** credentials on a disabled account → `403`, code `account_disabled`, with the plain
  message *"This account has been disabled. Contact the person who runs this instance."* — which
  discloses nothing to somebody who does not already hold the password;
- both write an audit entry: `login_failed` with a bounded `reason` (`bad_credentials` /
  `disabled`), never the address.

Disabling also calls `record.RefreshTokenKey()` inside the same transaction, which invalidates every
outstanding token for that account, so open sessions end at once rather than at expiry (D-19,
FR-090, SC-013's 5 seconds).

**Rationale.** FR-091 requires exactly this asymmetry: *"an attempt with incorrect credentials MUST
answer identically whether or not the account is disabled, so that disabling cannot be detected from
outside"*, while a person who does know their password must be told what happened rather than left
believing they mistyped it.

**Alternatives considered.**

- **`401 invalid_credentials` for both.** Kind to the security model, cruel to the user, and it
  contradicts AS-10's "refused with a plain message telling the person to contact the operator".
- **`403` for both.** Turns the login endpoint into an account-existence oracle.

---

## D-50

**Decision.** One helper, `internal/domain/retention`, owns every window calculation in the
application:

```go
package retention

// DueAt returns the instant at which something recorded at `at` falls out of a window of `days`
// whole days. Both are UTC; `days` is a whole number; the arithmetic is calendar-day arithmetic on
// the recorded moment, never a duration multiplied out.
func DueAt(at time.Time, days int) time.Time
func Expired(at time.Time, days int, now time.Time) bool
func DaysRemaining(at time.Time, days int, now time.Time) int   // floors at 0
```

Every purge, every "expires in N days" label and every window statement in a view goes through it.
The clock is injected everywhere (the `Clock` port phase 005 introduced), so the table test moves
time backwards, forwards, across a DST boundary and across a leap day, and asserts that **nothing is
removed before its window has elapsed and nothing escapes removal once it has** (FR-061, US8 AS-9).

**Rationale.** FR-053 ("measured in whole days from the recorded moment"), FR-061 and US8 AS-9.
Three subsystems enforce windows — artifacts, the trail, the trash — and three implementations of
"whole days" would drift by an hour the first time one of them used `time.Now().Sub`.

**Alternatives considered.**

- **`now.Sub(at) > time.Duration(days)*24*time.Hour`.** The obvious one, and it is wrong across a DST
  transition and unclear about what "a day" means at the boundary.
- **A SQL predicate per job.** Three predicates in three places, none of them unit-testable without a
  database.

---

## D-51

**Decision.** `data/audit_events.json` inside an export contains the entries **concerning the
exported people**, plus the exporting account's own entries, and nothing else. Every actor other than
the exporting account appears as an **opaque id only** — no display name, no address — exactly as
inside the application (FR-042). The entries carry the same fields as the D-11 DTO, so the export
cannot contain a class of information the application itself refuses to show.

There is **no `ip` field to omit**: the shared design contract listed one, phase 001 does not create
it (001 research D-19), and no phase publishes it. An IP address is somebody's address, and it is
absent from the instance rather than merely filtered out of the archive — which is a stronger
guarantee than the one this decision originally made.

**Rationale.** FR-041 requires the entries concerning the exported people to be included; FR-042
constrains what they may name; FR-043 forbids anything concerning a person the account cannot reach.
Reusing the same DTO (D-30) makes all three structural.

**Alternatives considered.**

- **Exporting the whole trail for the account's own actions across every patient.** Would include
  entries about people whose access has since been withdrawn, which FR-043 forbids.
- **Resolving actor display names "so the file is readable".** The application will not do it for its
  own reader (D-11); an archive is not a licence to do it.

---

## D-52

**Decision.** The trail pages with the application's existing keyset cursor (SHARED-DESIGN §6.3) over
`(occurred_at DESC, id DESC)`, backed by
`idx_audit_occurred (occurred_at DESC, id DESC)` — created by phase 001, not here — and, for the scoped
reader, `(patient, occurred_at DESC, id DESC)` and `(actor, occurred_at DESC, id DESC)`. The cursor
is the HMAC-signed `(last occurred_at, last id)` pair — never an offset — so an entry written while
somebody is on page 12 cannot shift page 13.

Because entries are **append-only and immutable** (FR-070), the guarantee is stronger than for any
other list in the application: new rows sort to the front of a descending scan and can therefore
never enter a page already walked. The test walks 50 consecutive pages of 1,000,000 entries while a
writer inserts continuously, and asserts **0 repeats and 0 skips** by id set (SC-016).

The CSV export (D-13) walks the same cursor, which is what makes its memory bound real.

**Rationale.** FR-066, FR-122, SC-016. An `OFFSET` pager over a table being appended to repeats and
skips rows by construction, which is the defect the requirement names.

**Alternatives considered.**

- **`LIMIT/OFFSET`.** Cheap, familiar, and incorrect under concurrent writes — and this is the one
  table in the application that is always being written to.
- **Snapshotting the query into a temp table.** A materialised page set with its own lifetime, for a
  guarantee an index already provides.

---

## D-53

**Decision.** A **produced document appears on `/reports`**, beside the builder and the saved
reports, not on `/exports`. The two pages read the same `export_jobs` resource filtered by `kind`:

| Page | Shows | Reads |
|---|---|---|
| `/reports` | the builder (counts, selection, charts, generate), the account's saved reports, and **documents produced from them**, with progress, download and re-run | `GET /api/v1/exports?kind=report` |
| `/exports` | the account's portable-export requests, with position while waiting, progress while running, size when finished, download, cancel and re-run | `GET /api/v1/exports?kind=data_export` |

One queue underneath (D-05, FR-045) means a report can be told *"waiting behind one export"*, and
both pages say so in the same words.

**Rationale.** US1's person wants the document where she asked for it; US2's person is doing
something else entirely — leaving. Putting a produced report on a page called "Exports" is the kind of
implementation detail leaking into an interface that makes people distrust an application. The cost
is zero: it is a query parameter.

**Alternatives considered.**

- **One `/exports` page for both.** Cheaper by one region and it makes the report builder's output
  disappear to a page named after something else.
- **Two collections, one per kind.** Two queues, two workers, and FR-045's "at most one at a time on
  an instance" becomes a lock across them.

---

## Risks this phase closes, and the two it leaves open

| Risk | Status |
|---|---|
| **R4** PDF generation | **Closed** by D-01 (with a gating spike task and a named fallback) |
| **R6** MFA / IP-allowlist detection | **Closed** for the operator surface by D-17 |
| **R7** >5-minute SSE liveness in CI | **Closed** by D-32 |
| **R9** Export job durability | **Closed** by D-06 |
| **R8** PocketBase upgrade fragility | **Extended, not closed.** This phase adds four more upgrade-sensitive touch points — `core.StoreKeyActiveBackup`, the `Restart`/`execve` environment behaviour (D-24), the backup exclusion set (D-07), and the empties-rather-than-deletes behaviour of a non-cascading relation (D-45). All four go on the upgrade checklist phase 001 created. |
| **R3** FTS5 availability | Untouched. Phase 004 owns it; nothing in this phase depends on relevance ranking. |

**Left open, deliberately, and stated so nobody discovers it later:** MediGo cannot render every
script in a produced document (D-04), and Arabic is not contextually shaped. The export always carries
the exact text, the limitation is counted on the document's first page, and the operator handbook
says so.
