---
description: "Task list for phase 004 — Labs and Attachments"
---

# Tasks: Labs and Attachments

**Input**: Design documents from `/specs/004-labs-and-attachments/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: **MANDATORY.** Constitution Principle III makes test-first non-negotiable and the
specification demands it by name (FR-080, FR-082, FR-083, FR-085, SC-005, SC-009, SC-010). Every
test task below precedes the implementation task it covers. A red-to-green transition that was
never red is a defect.

**Organization**: by user story, in the spec's priority order. Each story is independently
implementable, testable and demonstrable after the Foundational phase.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel with its siblings (different files, no incomplete dependency)
- **[Story]**: `[US1]`…`[US5]`; Setup, Foundational and Polish carry no story label
- Every task names the exact file path it touches

## Path conventions

All paths are relative to `/Users/krzysztof.wiatrzyk/private/monorepo/medikube`.

---

## Phase 1: Setup

**Purpose**: toolchain, linters, configuration and fixtures ready for a phase that adds four
collections and the application's first bytes-holding collection. Nothing here touches domain logic.

- [ ] T001 Verify the Go 1.27 build precondition: confirm `go.mod` declares `go 1.27` with a `toolchain go1.27.x` line, confirm `GOTOOLCHAIN` is unset in `Taskfile.yaml` and in `.github/workflows/`, and record the result in `specs/004-labs-and-attachments/quickstart.md` §0
- [ ] T002 [P] Add `Labs.MaxSeriesPoints` (`MEDIKUBE_LABS_MAX_SERIES_POINTS`, default 500) to `internal/config/config.go`, and set the `Files.AllowedMIME` default to `application/pdf,image/jpeg,image/png,image/webp,image/heic,image/heif,image/tiff,image/gif,text/plain` and `Files.MaxUploadBytes` to 33554432 per research D-14/D-15
- [ ] T003 [P] Extend `internal/config/config_test.go` with the new defaults, the allowed-MIME list parse (comma list, trimmed, lower-cased, rejecting an empty entry), and a case asserting `MEDIKUBE_LABS_MAX_SERIES_POINTS=0` is rejected at boot
- [ ] T004 [P] Add the `labs-never-convert` `depguard` rule to `.golangci.yml`: files under `internal/domain/labs/**`, `internal/service/lab*/**` and `internal/store/lab*/**` may not import `medikube/internal/domain/clinical/units` (research D-09)
- [ ] T005 [P] Add `forbidigo` patterns to `.golangci.yml` for `\.NewFileToken\(` and `filesystem\.NewFileFromURL\(` with messages citing Constitution VII (a credential in a URL — FR-074's rule made a build gate; an SSRF sink)
- [ ] T006 [P] Add `task purge` (wrapping `medikube purge`) to `Taskfile.yaml`, and register `e2e/specs/labs.spec.ts`, `e2e/specs/trends.spec.ts` and `e2e/specs/documents.spec.ts` in the Playwright project config
- [ ] T007 [P] Create `internal/testdata/files/` with one minimal valid fixture per accepted type (pdf, jpeg, png, webp, gif, tiff, heic, txt, csv) plus the hostile set (an HTML file named `.pdf`, a PDF named `.png`, a zero-byte file, a name with right-to-left text, a name with `<script>` in it, a 300-character name), and a Go helper in `internal/testsupport/files/fixtures.go` that **generates** the at-the-limit file at test time rather than committing it
- [ ] T008 [P] Add `internal/web/views/labs/**` and `internal/web/views/files/**` to the Tailwind content globs in `assets/input.css` so the new components' utilities are not purged
- [ ] T009 [P] Confirm `/Users/krzysztof.wiatrzyk/private/monorepo/.dockerignore` still excludes `medikube/pb_data/` (which now holds uploaded documents) and `medikube/**/*_templ.go`, and that `medikube` is in the `build-image.yaml` project options

**Checkpoint**: toolchain, linters, config and fixtures in place. No production behaviour has changed.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: everything shared by two or more stories — the vocabularies, the pure domain
arithmetic, the four migrations, the registry's central attachment hook, the new route class and
the CSP change. **No user story may start until this phase is complete**, because every story
either reads a migration this phase writes or calls a domain function it defines.

### Vocabularies

- [ ] T010 [P] Write failing table-driven tests in `internal/domain/clinical/vocab_lab_test.go` for `LabCategory`, `ResultType`, `ComponentStatus` and `AttachmentCategory`: `Valid()` accepts exactly the values in `data-model.md` §1, rejects the empty string and any upper-case or hyphenated form, `All()` is stable and deduplicated, and each Go list is byte-identical to the `SelectField.Values` its migration declares
- [ ] T011 Implement `internal/domain/clinical/vocab_lab.go` — four string types with `Valid()`, `All()` and `String()`

### Pure domain arithmetic — `internal/domain/labs` (no I/O, `t.Parallel()` throughout)

- [ ] T012 [P] Write failing tests in `internal/domain/labs/canonical_test.go` for `Normalise`: NFKC composition, trim, internal whitespace collapse, Unicode case-fold; `"  Glucose "` and `"glucose"` collapse to one key (US4 scenario 5); `"Vitamin D 25-OH"` and `"Vitamin D-3"` stay distinct
- [ ] T013 Implement `internal/domain/labs/canonical.go`
- [ ] T014 [P] Write failing tests in `internal/domain/labs/refrange_test.go` for `Classify` — below range, within range or above range whenever there is a numeric value and at least one numeric bound (FR-018): an explicit `status` always wins over arithmetic (FR-019); a numeric value with one bound is judged on that bound (FR-017); no numeric bound and no explicit status yields `not_assessed` and **never** `within` (FR-020); a `ref_text`-only range yields `not_assessed`; a textual value is never coerced to a number
- [ ] T015 Implement `internal/domain/labs/refrange.go` — the `RefRange` value object and `Classify` (FR-018)
- [ ] T016 [P] Write failing tests in `internal/domain/labs/sortdate_test.go` for the FR-008 four-level fallback, including a result with no dates at all falling back to the creation date and still sorting deterministically
- [ ] T017 Implement `internal/domain/labs/sortdate.go`
- [ ] T018 [P] Write failing tests in `internal/domain/labs/series_test.go` for `Summarise`: the eight figures of FR-030; the FR-031 halves rule with an even count, an odd count (middle reading discarded), an older mean of exactly zero (no division, `steady`), readings on the same date (deterministic split); `insufficient_readings` below three (FR-032); a categorical series returning a value-frequency history and **no** mean, range or direction (FR-033); the capped-window summary (research D-08)
- [ ] T019 Implement `internal/domain/labs/series.go`, including `DirectionRuleText` as an exported constant so the rendered rule and the implementation cannot drift
- [ ] T020 [P] Write failing tests in `internal/domain/labs/labresult_test.go` and `component_test.go` for `Validate`: **every** offending field reported in one submission; `panel_and_value` (FR-005); `date_order` on both pairs (FR-007); `ref_range_inverted` (FR-017); a status outside the shared ladder refused rather than stored as free text and a new result defaulting to `ordered` (FR-003); only `test_name` and `patient` required on a result and only `test_name` and `value_kind` on a component (FR-002, FR-012); `value_kind_mismatch` in both directions (FR-013); a field at exactly its documented limit accepted and one character over refused with the field and the limit named; a result with neither a value nor components accepted; and `MarshalZerologObject` emitting **only** ids
- [ ] T021 Implement `internal/domain/labs/labresult.go` (the field set of FR-002, `status` from the shared `OrderStatus` ladder defaulting to `ordered`, FR-003) and `internal/domain/labs/component.go` (the field set of FR-012, test name and value kind required)

### Pure domain — `internal/domain/files`

- [ ] T022 [P] Write failing tests in `internal/domain/files/mime_test.go` for `Sniff`: the stdlib types; the nine-entry magic table for WebP, HEIC/HEIF and TIFF; a PDF renamed `.png` sniffs as PDF; an HTML file named `.pdf` sniffs as HTML and is refused; a declared `Content-Type` is ignored; a `.csv` sniffs as `text/plain` (research D-14); and `InlineSafe` is a **fixed** set that no configuration value can widen
- [ ] T023 Implement `internal/domain/files/mime.go` — `Sniff`, the magic table and `InlineSafe`
- [ ] T024 [P] Write failing tests in `internal/domain/files/trash_test.go` for the retention arithmetic and the three restore preconditions of `data-model.md` §5.3 (within window and record present; record missing; window closed), and for `DaysUntilPurge`
- [ ] T025 Implement `internal/domain/files/trash.go` and `internal/domain/files/attachment.go` (`Validate`, `MarshalZerologObject` emitting only ids — **never** `original_name`, `description` or `mime`)

### The fifteenth kind

- [ ] T026 [P] Extend `internal/domain/kind/kind_test.go` to assert fifteen kinds: the enum↔segment mapping is total and injective, `lab_result` maps to `lab-results`, and both landmark strings match `contracts/pages.md` §2
- [ ] T027 Add `kind.LabResult` to `internal/domain/kind/kind.go` with its `snake_case` value, plural kebab segment, singular and plural labels, and its two ARIA landmark names

### Migrations

- [ ] T028 [P] Write failing tests in `internal/store/migrations/catalog_lab_tests_test.go`: every field of `data-model.md` §2 with its type; all five API rules `nil`; the three indexes including the partial unique index on non-empty `loinc_code`; running `up` twice produces the same row count; an inverted `ref_low`/`ref_high` pair in the extract fails the migration loudly; `down` removes the collection cleanly
- [ ] T029 Implement `internal/store/migrations/<ts>_catalog_lab_tests.go` — the collection plus an idempotent upsert of the embedded `assets/catalog/lab-tests.json` keyed on `loinc_code`, so the instance ships with the catalogue rather than acquiring it (FR-036)
- [ ] T030 [P] Write failing tests in `internal/store/migrations/lab_results_test.go`: every field of `data-model.md` §3, with `ordering_practitioner` and `facility` as **relations** into the phase-002 directories and never text fields (FR-004); all five rules `nil`; the four indexes including `(patient, sort_date DESC, id DESC)`; `CascadeDelete` on `patient`; the three multi-relation link fields present; `down` clean
- [ ] T031 Implement `internal/store/migrations/<ts>_lab_results.go`
- [ ] T032 [P] Write failing tests in `internal/store/migrations/lab_components_test.go`: every field of `data-model.md` §4; all five rules `nil`; the three indexes; `CascadeDelete` on `lab_result`; **and a two-level cascade test proving deleting a patient removes the components** (FR-015, SC-013, research D-29); `down` clean
- [ ] T033 Implement `internal/store/migrations/<ts>_lab_components.go`
- [ ] T034 [P] Write failing tests in `internal/store/migrations/attachments_test.go`: every field of `data-model.md` §5 — content, original name, size in bytes, sniffed type, patient, owning record, uploader, timestamp, optional description and optional category (FR-050); all five rules `nil`; the three indexes; `file` is `MaxSelect 1` with `Thumbs ["160x160t","1024x1024f"]` and **`Protected: true`**; `MaxSize` and `MimeTypes` come from configuration; `down` removes the collection **and its blobs** and the file documents that the direction is destructive
- [ ] T035 Implement `internal/store/migrations/<ts>_attachments.go`
- [ ] T036 [P] Extend `internal/store/migrations/audit_vocab_test.go` (phase 001, T070a) to assert the **complete** expected vocabulary after this phase — **twenty-one** actions and **twenty-seven** target kinds, set-equal, not a delta. `lab_result` and `attachment` are both declared by phase 001's `audit_events` migration and `read_sensitive` and `access_denied` with them, so **this phase adds no vocabulary value and has no vocabulary migration**; the test is what proves it, and it fails if a later hand adds one (ANALYSIS C1)
- [ ] ~~T037~~ **Deliberately vacant.** It held this phase's `audit_vocab` migration until
  ANALYSIS C1 moved the whole vocabulary into phase 001's, leaving this phase with nothing to
  migrate and only T036's assertion to prove it. The id is not reused and the following tasks
  are not renumbered, because every dependency clause in this file and the cross-phase
  references in 005 and 006 cite ids that already exist.
- [ ] T038 Regenerate the committed test fixture with `task fixture:regen` so `internal/testdata/pb_data` contains the four new collections, and commit it — **every integration test in every later phase runs against a stale schema until this is done**

### Shared machinery

- [ ] T039 [P] Write a failing test in `internal/records/register_test.go` asserting that `Register` binds an attachment-cleanup hook for the kind, and that a kind registered without one fails `registry_completeness_test.go` — this is the FR-049 gate
- [ ] T040 Extend `internal/records/register.go` to bind `OnRecordAfterDeleteSuccess` for every registered kind's collection, moving that record's attachments to the trash in the same transaction (FR-067), and to expose an optional attachment strip slot on `records.Views` — **centrally, so no per-kind registration file is edited**
- [ ] T041 [P] Write a failing test in `internal/platform/pb/assertions_test.go` asserting the boot check now covers **two** file fields (`patients.photo`, `attachments.file`) and that the application refuses to start when either has `Protected: false`
- [ ] T042 Extend `internal/platform/pb/assertions.go` accordingly
- [ ] T043 [P] Write a failing test in `internal/httproute/registry_test.go` for `KindPageAction`: such a route appears in `medikube routes`, is excluded from the OpenAPI input set, has no landmark, and **must** declare a non-empty covering Playwright spec path or `Register` panics at startup (research D-25)
- [ ] T044 Implement `KindPageAction` in `internal/httproute/registry.go`, its `MarshalJSON` output and its exclusion in `internal/openapi/`
- [ ] T045 [P] Write failing tests in `internal/web/security/csp_test.go`: the page policy gains `frame-src 'self'` and loses nothing; the attachment response policy is `default-src 'none'; img-src 'self'; style-src 'none'; script-src 'none'; object-src 'none'; frame-ancestors 'self'; sandbox`, with the `sandbox` token **omitted for `application/pdf` only** (research D-16)
- [ ] T046 Implement `internal/web/security/csp.go` — the page policy change and the `AttachmentPolicy(mime)` builder

**Checkpoint**: the schema, the arithmetic, the vocabularies, the registry hook, the route class
and the CSP are in place. All five stories can now start in parallel.

---

## Phase 3: User Story 1 — Record a blood test and see at a glance what is off (Priority: P1) 🎯 MVP

**Goal**: a lab result that holds its individual lines as real, separate, comparable values, with
every out-of-range reading marked plainly and not by colour alone.

**Independent Test**: sign in as the seeded account, open the lab results for the seeded person,
record a panel with a name, an ordering practitioner, a laboratory, collection and result dates, an
interpretation and ten components with values, units and ranges; confirm the panel is in the list,
its detail shows every component in the order entered, the three deliberately out-of-range values
are marked and counted, and the seven in range are not. Testable with nothing else in this phase
implemented.

### Tests for User Story 1 ⚠️ write first, confirm they fail

- [ ] T047 [P] [US1] Write failing `tests.ApiScenario` cases in `internal/web/api/labresults_http_test.go` for create and read: required fields only — test name and person, everything else optional (FR-002) — (US1-2, values left blank are **absent** from the detail, not zero); every field populated and echoed back exactly, the ordering practitioner and place of care resolved from the account holder's own directories rather than typed (FR-002, FR-004) (US1-3); a collection date before the ordered date **plus** an out-of-vocabulary status in one submission reported together (US1-4, FR-007)
- [ ] T048 [P] [US1] Write failing cases in `internal/web/api/labresults_components_http_test.go` for the replace-set: ten components stored in the order entered (US1-5); three out of range counted on the result and marked (FR-022, US1-6); a change to the component set written to the activity trail as a change to the **lab result** (FR-023); one removed, one changed, one added yields exactly the submitted set (US1-8); duplicate `test_name` in one panel accepted in order (FR-016); a 100-component panel returned in one response without truncation (FR-085)
- [ ] T049 [P] [US1] Write failing cases for panel↔scalar conversion in both directions (US1-9), asserting that converting to a panel clears `value`/`unit`/`ref_*` and that submitting both is `422 panel_and_value` with **neither part discarded** (US1-10)
- [ ] T050 [P] [US1] Write failing cases for optimistic concurrency in `internal/web/api/labresults_http_test.go`: `PATCH` without `If-Match` is `428`; with a stale `If-Match` is `412` and the response carries the current representation (US1-11); a component set is never merged
- [ ] T051 [P] [US1] Write a failing integration test in `internal/service/labresult/delete_test.go`: deleting a result destroys every component permanently, moves its attachments to the **trash**, removes every inbound link leaving those records intact, removes the `search_index` row, and writes exactly one `delete` audit row by identifier containing no content (US1-12, FR-011, FR-015, FR-067, SC-013)
- [ ] T052 [P] [US1] Write the failing authorization matrix in `internal/web/api/labresults_authz_test.go` for all six record operations: owner succeeds; a stranger receives a `404` **byte-identical** to a non-existent id; unauthenticated receives `401` with no patient information; a superuser succeeds **and** an audit row is written; the refusal appears as `access_denied` by opaque id with no content (US1-13, FR-073, SC-005)
- [ ] T053 [P] [US1] Write failing DTO round-trip tests in `internal/web/api/labresults_test.go` under `encoding/json/v2`: slices marshal as `[]` never `null`; an unknown field is `422`; a duplicate JSON key is `422`; and `is_panel`, `sort_date`, `canonical_name`, `assessment`, `component_count`, `out_of_range_count`, `attachment_count`, `encounters` and `treatments` are **absent from the request DTOs by construction**
- [ ] T054 [P] [US1] Write failing service unit tests in `internal/service/labresult/service_test.go` against hand-written fakes: every method's first act is an `Authorizer` call; a result is attributed to exactly one person, a create naming an unreachable patient is `ErrNotFound` and a patch carrying `patient` is refused (FR-001)
- [ ] T055 [P] [US1] Write failing unit tests in `internal/service/labresult/components_test.go` for the replace-set diff: the four cases of `contracts/lab-results.md` §3; an element carrying an id belonging to another result refuses the **whole** submission; `display_order` comes from the array index; `is_panel` is recomputed at the end
- [ ] T056 [P] [US1] Write failing integration tests in `internal/store/labresult/repo_test.go`: `sort_date` is derived on save and re-derived when `resulted_on` changes; a result with no dates sorts last but deterministically; keyset cursors neither repeat nor skip a row while rows are inserted and deleted mid-page (FR-009)
- [ ] T057 [P] [US1] Write failing integration tests in `internal/store/labcomponent/repo_test.go`: `ReplaceSet` is atomic (a failure mid-set leaves the stored set untouched); components cascade away with the parent; `display_order` round-trips
- [ ] T058 [P] [US1] Wire `recordstest.RepositoryContract` and `recordstest.KindContract` against `lab_result` in `internal/service/labresult/contract_test.go`, running both against the real repository and against `labresulttest.Fake` — passing the same suite every earlier kind passes is what makes "behaves like every other record kind" testable rather than asserted (FR-010), and the suite's audit case is the creation/change/deletion trail by identifier (FR-011)
- [ ] T059 [P] [US1] Write failing templ render tests in `internal/web/views/records/labresult_templ_test.go`: `article[name="Lab result"]`; every out-of-range value carries a **text marker and a shape** plus an `aria-label` naming the assessment while in-range values carry none (FR-021, SC-002); a component with no bound and no explicit status reads "not assessed against a range" and never "normal" (FR-020); the out-of-range count is rendered; the delete confirmation names the result and states that components are destroyed and documents go to the trash
- [ ] T060 [P] [US1] Write failing render tests for the empty states in `internal/web/views/shared/emptystate_templ_test.go` — every one of FR-081's four views has its own explanatory empty state: `/lab-results` with nothing recorded renders `@EmptyState` **inside** `region[name="Lab results"]` with the record-first action (US1-1); a result with neither a value nor components renders "awaiting a result", not an error
- [ ] T061 [P] [US1] Write `e2e/specs/labs.spec.ts` covering `/lab-results` and `/lab-results/{id}` at 1440×900 and 390×844: 200, the four shell landmarks plus the page landmark, `body[data-signals]`, zero console errors, zero page errors, zero failed network requests; plus recording a ten-component panel, the non-colour marking, the stale-edit refusal and the delete confirmation wording

### Implementation for User Story 1

- [ ] T062 [US1] Declare the consumer-side ports in `internal/service/labresult/ports.go`: `Repository` (4 methods), `ComponentRepository` (3), `Authorizer` (2), `Auditor` (1), `Indexer` (2), `AttachmentTrasher` (1)
- [ ] T063 [US1] Implement `internal/service/labresult/service.go` — `List`, `Get`, `Create`, `Update`, `Delete`, each authorizing first against the stored patient (FR-001) and never touching PocketBase or `net/http`
- [ ] T064 [US1] Implement `internal/service/labresult/components.go` — the id-stable replace-set inside `app.RunInTransaction`, the `is_panel` recomputation, the out-of-range count on the parent result (FR-022), the single `update` audit row on the lab result (FR-023), and the scalar/panel field clearing on conversion (FR-005, FR-006, FR-014)
- [ ] T065 [US1] Implement `internal/service/labresult/query.go` (the typed filter of FR-009: status, category, date range, `q` over `test_name`, tags, sort allowlist) and `patch.go` (pointer-based absent-vs-null)
- [ ] T066 [US1] Implement `internal/service/labresult/adapter.go` — the ~40-line `records.Service` implementation that decodes and encodes the DTOs
- [ ] T067 [P] [US1] Implement `internal/service/labresult/labresulttest/fake.go` — the in-memory fake that satisfies the same contract suite
- [ ] T068 [US1] Implement `internal/store/labresult/repo.go` and `mapper.go` — `*core.Record` ↔ `labs.LabResult`, the keyset cursor, the typed filter → PocketBase expression, and the `sort_date` write
- [ ] T069 [US1] Implement `internal/store/labcomponent/repo.go`, `mapper.go` and `replaceset.go`
- [ ] T070 [US1] Implement the DTOs in `internal/web/api/labresults.go` per `contracts/lab-results.md` §1 — `LabResultSummary`, `LabResult` (FR-002), `Component` (FR-012), `LabResultCreate`, `ComponentInput`, `LabResultPatch`
- [ ] T071 [US1] Implement `internal/records/kinds/labresult.go` — the single `records.Register` call wiring the service, the codec, the views, the default sort, the searchable fields, the filters, the seed fixture id and the OpenAPI schema. One registration is what gives the kind phase 001's six operations, its confirmation, its `If-Match` refusal and its live list, unchanged (FR-010)
- [ ] T072 [P] [US1] Implement `internal/web/views/records/labresult.templ` — `LabResultRow`, `LabResultList`, `LabResultDetail` and the component table, each with a stable id from `views/ids`
- [ ] T073 [P] [US1] Implement `internal/web/views/labs/componenteditor.templ` — add and remove component rows through Datastar signals, no route, free attribute set only
- [ ] T074 [US1] Add the deterministic ids for the new components to `internal/web/views/ids/ids.go`
- [ ] T075 [US1] Extend `internal/service/access/authorizer.go` so `Record()` resolves `kind.LabResult`
- [ ] T076 [US1] Extend `internal/cli/seed.go` with patient A's eight lab results — one scalar, one with no dates at all, one ten-component panel with exactly three out of range, one component with a value and no range — and patient B with none
- [ ] T077 [US1] Extend `internal/testsupport/scale/generate.go` to produce 5,000 lab results for one patient and a single 100-component panel
- [ ] T078 [US1] Write the build-tagged scale test `internal/store/labresult/scale_test.go` asserting SC-011 (every page of a 5,000-row list within 2 s) and FR-085 (the 100-component panel renders as one page)

**Checkpoint**: US1 is fully functional and demonstrable on its own. A carer can keep a structured,
searchable laboratory history that answers the question a clinician actually asks.

---

## Phase 4: User Story 2 — Keep the paperwork with the record it belongs to (Priority: P2)

**Goal**: a private, per-record document store with recoverable deletion, on every record kind in
the application.

**Independent Test**: attach a document to a record of each available kind; confirm it appears on
that record and in the person's document library; open a supported one inline; download one and
compare the bytes with what was uploaded; replace one; change a description; delete one and restore
it; delete another and confirm it is no longer listed with its record. Testable with US1 not
implemented, using any record kind delivered by an earlier phase.

### Tests for User Story 2 ⚠️ write first, confirm they fail

- [ ] T079 [P] [US2] Write the failing `FileStore` contract suite in `internal/service/attachment/attachmenttest/filestorecontract.go` — `Put`, `Open`, `Thumb`, `Delete`; run it against both implementations from `internal/store/filestore/filestore_test.go`
- [ ] T080 [P] [US2] Write failing upload cases in `internal/web/api/attachments_upload_http_test.go`: over the limit is `413` stating the limit **with nothing stored and nothing spilled to a temporary file** (FR-053); zero bytes is `422 empty_file` (FR-054); a PDF named `.png` is stored as a PDF; an HTML file named `.pdf` is `415` naming the accepted types; a declared `Content-Type` is ignored; a `.csv` stores with `mime: text/plain` (FR-051, FR-052, research D-14) — **discharges SC-012**: each refusal case additionally asserts that **nothing was stored**, neither a row nor a byte in the filestore, so "0% of them leave anything stored" is proven per case rather than assumed
- [ ] T081 [P] [US2] Write the failing fidelity test `internal/web/api/attachments_fidelity_test.go` for SC-004 and FR-056: for each accepted type, upload at 1 byte, 1 KiB and **exactly** the configured limit, download, and compare SHA-256 with the original
- [ ] T082 [P] [US2] Write a failing test proving that a record deleted in another place while the upload is in flight makes the transaction fail cleanly with nothing stored and no document pointing at a missing record
- [ ] T083 [P] [US2] Write a failing test proving two uploads of byte-identical content produce **two** documents, neither discarded as a duplicate (FR-055)
- [ ] T084 [P] [US2] Write failing list cases in `internal/web/api/attachments_list_http_test.go` — one library per person, sorted by when each document was attached (FR-069): filters by `owner_kind`/`owner_id`, `category`, `q` over name and description, `deleted=true`; `usage=true` returns the four counters with trashed counted separately (FR-071); keyset paging neither repeats nor skips while documents are being attached
- [ ] T085 [P] [US2] Write failing content-stream cases in `internal/web/api/attachments_content_test.go`: `disposition=attachment` by default; a `Range` request returns `206`; a matching `If-None-Match` returns `304`; every response carries `X-Content-Type-Options: nosniff`, `Cache-Control: private, no-store` and the per-response CSP, with the `sandbox` token omitted **only** for `application/pdf`
- [ ] T086 [P] [US2] Write failing tests proving `disposition=inline` is honoured only for the compile-time inline-safe set, that any other type is served as `attachment` rather than refused (FR-057), and that **widening `MEDIKUBE_FILES_ALLOWED_MIME` does not widen what is inlined** (FR-058)
- [ ] T087 [P] [US2] Write failing tests for hostile and awkward file names: non-Latin script, right-to-left text, markup-like characters and a 300-character name are stored, listed as text, and downloaded byte-identically under the full original name (FR-056)
- [ ] T088 [P] [US2] Write failing preview tests (FR-059): `has_preview` is `true` only for JPEG, PNG, GIF and WebP, and the library renders a type icon for everything else; a `?size=` request when it is `false` is `404`; a preview goes through the **same authorization call and is audited under the same rule** as the original — a row for a non-owner's preview, none for the owner's (FR-060, FR-076); a thumbnail failure leaves `has_preview: false` and does not fail the upload (research D-17)
- [ ] T089 [P] [US2] Write failing `PATCH` tests: description and category change; `original_name`, `size_bytes` and `mime` are **absent from the DTO**; `patient`, `owner_kind` and `owner_id` are refused (FR-062)
- [ ] T090 [P] [US2] Write failing trash tests in `internal/service/attachment/trash_test.go` (FR-063, the window defaulting to 30 days from `MEDIKUBE_RETENTION_TRASH_DAYS`): delete is soft and idempotent and removes the document from the record's listing and from the library; `days_until_purge` is computed; restore within the window returns it to its record (FR-064); restore with the owning record gone is `409 owner_record_missing` and the download is still offered (FR-065); restore past the window is `409 retention_expired`; a purged id is `404`; and `?purge=true` (FR-066) is accepted from **the owner** and from **a superuser**, is `404` for a non-owner who can only view the document, and is `404` for everyone else — one case per caller, so the owner case cannot silently regress to superuser-only. **First half of SC-007**: the restore-within-the-window case runs through the owner's own operation and asserts the document is back on its record
- [ ] T091 [P] [US2] Write a failing test proving that deleting a clinical record of **any** kind moves its three attached documents to the trash while the record itself is destroyed permanently (FR-067, US2-14) — parameterised over at least three registered kinds so the central hook is what is under test
- [ ] T092 [P] [US2] Write a failing cascade test in `internal/store/attachment/cascade_test.go`: deleting a patient destroys every document **including those awaiting purge**, blobs and thumbnails included, verified by looking for the rows and the files afterwards (FR-068, SC-013); the same for deleting an account
- [ ] T093 [P] [US2] Write failing tests in `internal/service/attachment/purge_test.go`: the window arithmetic; a failure part-way leaves documents **wholly** in the trash and is retried on the next run; the orphan sweep quarantines a row whose `owner_id` no longer resolves; the storage gauge is refreshed. **Second half of SC-007**: after the purge, the document cannot be recovered by **anyone** through the application — the owner, a superuser and a grantee each get `404` from restore and from content — **and its content is no longer stored**, asserted against `app.NewFilesystem()` for the blob and both thumbnails, not against the row. **Plus the run-id case** (contracts/attachments.md §7.1): the cron writes one `delete`/`attachment` row per purged document and one per quarantined orphan, each `actor_kind = system` with a **non-empty `request_id` equal to that run's `run_id`** — identical across the rows of one run, different between two runs, and equal to the value on the run's log lines — asserted by running the maintenance function on a bare background context, which is the only context it ever has (FR-077, 001 [data-model](../001-walking-skeleton/data-model.md) §3)
- [ ] T094 [P] [US2] Write a failing race test proving restore re-reads `deleted_at` **inside** the transaction, so exactly one of restore and purge wins and the loser is told the document is gone rather than shown a broken restore (research D-19)
- [ ] T095 [P] [US2] Write the failing authorization matrix in `internal/web/api/attachments_authz_test.go` for all six operations, with content tested **three ways** per FR-074 and FR-075 — the address itself grants nothing: opening it directly while signed out (`401`), a guessed identifier (`404`), and another account's session (`404`, byte-identical) — plus a superuser succeeding with an audit row
- [ ] T096 [P] [US2] Write failing audit tests in `internal/service/attachment/audit_test.go` for the ownership condition on `read_sensitive` (FR-076, SC-006, 005 [D-25](../005-sharing-and-collaboration/research.md#d-25)), one subtest per case: **the owner retrieving their own content or preview writes NO row at all**; a superuser retrieving somebody else's writes **exactly one**; a superuser retrieving their own writes none; the row carries actor, attachment id, patient id and timestamp and **no** name, description, mime or bytes. Plus: an `access_denied` row on **every** refusal regardless of who was refused (FR-073); and rows for attach, replace, describe, delete, restore and purge (FR-077). The owner-writes-nothing case is asserted by counting rows before and after, so an unconditional write cannot pass
- [ ] T097 [P] [US2] Write the failing PHI-asymmetry test in `internal/testsupport/phileak/attachments_test.go`: a refusal whose `error.message` names the uploader's own file must produce **zero** occurrences of that name in the zerolog stream, the Prometheus registry, the OTel span recorder and the Sentry transport (FR-078, FR-079)
- [ ] T098 [P] [US2] Write failing `return_to` tests in `internal/web/page/actions_test.go`: `//evil.example`, `https://evil.example`, `/../`, and a valid-but-unregistered path are all ignored in favour of `/documents`; a registered page pattern is honoured (research D-34)
- [ ] T099 [P] [US2] Write failing templ render tests in `internal/web/views/files/library_templ_test.go`: `region[name="Documents"]`; every row names the record the document belongs to and offers a way to open that record (FR-070); a document with `has_preview: false` renders a **type icon**, never an `<img>` with a URL that would 404 (FR-059); deletion is confirmed before it happens and the confirmation states the retention window in days (FR-063); a trashed row shows its days remaining and the restore action; the owner's trashed row also offers **purge now** behind a typed confirmation naming the file and stating that it cannot be undone, and a row reached through somebody else's share offers no purge action at all (FR-066); the empty state renders inside the landmark with guidance on where to attach the first document (US2-1)
- [ ] T100 [P] [US2] Write `e2e/specs/documents.spec.ts` covering `/documents` at both viewports with the full gate assertions, plus upload, inline view, download, replace — the corrected version keeping the description, the category and its place on the record while the replaced one stays recoverable (FR-061) — describe, delete, restore, the owner's typed-confirmation purge-now, and the two page-action routes `POST /documents/upload` and `GET /documents/list`
- [ ] T101 [P] [US2] Write the build-tagged scale test `internal/store/attachment/scale_test.go`: 2,000 documents for one patient page, narrow and sort without degrading (FR-085)

### Implementation for User Story 2

- [ ] T102 [US2] Implement `internal/store/filestore/filestore.go` and `thumb.go` over `app.NewFilesystem()`, plus the in-memory fake in `internal/service/attachment/attachmenttest/filestorefake.go`; both pass the contract suite. **This is the only package in MediKube permitted to move bytes**
- [ ] T103 [US2] Declare the consumer-side ports in `internal/service/attachment/ports.go`: `Repository` (5), `FileStore` (4), `RecordLocator` (1), `Authorizer` (2), `Auditor` (1)
- [ ] T104 [US2] Implement `internal/service/attachment/upload.go` — the ordered checks of `contracts/attachments.md` §1.1 including the empty-content refusal (FR-054), the content sniff, and the `replaces` create-then-trash flow that carries the description, the category and the place on the record across to the corrected version and leaves the replaced one recoverable for the retention window (FR-061), all inside `app.RunInTransaction`
- [ ] T105 [US2] Implement `internal/service/attachment/serve.go` — authorize, resolve the file key, apply the disposition and the per-response CSP, stream through `fsys.Serve`, and write the `read_sensitive` audit row on success **only when the grant the authorizer resolved is not the caller's own ownership**; the ownership outcome is read from the authorizer's result, never re-derived from the request (depends on T096)
- [ ] T106 [US2] Implement `internal/service/attachment/trash.go` — soft delete, the early hard purge authorized to the owner and to a superuser and `404` to everyone else (FR-066), restore with the transactional `deleted_at` re-read, and the three refusals
- [ ] T107 [US2] Implement `internal/service/attachment/purge.go` — the maintenance function: purge, orphan sweep, gauge refresh, deriving one `run_id` for the run and passing it on the context so every audit row it writes fills the `Required` `request_id` and correlates to the run's log lines (depends on T093)
- [ ] T108 [US2] Implement `internal/service/attachment/service.go` — `List` with the typed filter and the optional `usage` aggregate
- [ ] T109 [P] [US2] Implement `internal/service/attachment/attachmenttest/fake.go` and the shared `StoreContract`
- [ ] T110 [US2] Implement `internal/store/attachment/repo.go` and `mapper.go` — keyset cursors over `(created, id)`, the `deleted` filter, and the usage aggregate
- [ ] T111 [US2] Implement the six handlers and DTOs in `internal/web/api/attachments.go`, registered under the `operationId`s `uploadAttachment`, `listAttachments`, `getAttachmentContent`, `updateAttachment`, `deleteAttachment` and `restoreAttachment`, wrapping the request body in `http.MaxBytesReader` **before** the multipart parse and overriding the route's body limit (research D-15)
- [ ] T112 [US2] Extend `internal/platform/pb/hooks.go` — `OnRecordAfterCreateSuccess("attachments")` schedules thumbnail generation in a `TxInfo().OnComplete` callback for the four decodable types so listings need no further work (FR-059), sets `has_preview`, and logs a failure once without failing the upload
- [ ] T113 [US2] Register `medikube_attachment_maintenance` in `internal/platform/pb/cron.go`
- [ ] T114 [US2] Implement `internal/cli/purge.go` — `medikube purge` runs the same maintenance function once
- [ ] T115 [P] [US2] Implement `internal/web/views/files/library.templ`, `strip.templ`, `upload.templ` and `viewer.templ` — the upload form is a **native multipart form with no `data-on:submit` and no `data-bind` on the file input** (research D-24)
- [ ] T116 [US2] Implement `internal/web/page/documents.go` and the two page actions in `internal/web/page/actions.go` (`POST /documents/upload` with `return_to` validation, `GET /documents/list`), registering both as `KindPageAction` naming `e2e/specs/documents.spec.ts`
- [ ] T117 [US2] Render the attachment strip on every kind's detail page through the `records.Views` slot added in T040, so no per-kind view file is edited (FR-049)
- [ ] T118 [US2] Extend `internal/cli/seed.go` with three documents across three different record kinds for patient A, one of them already in the trash, and none for patient B
- [ ] T119 [US2] Register `medikube_files_uploads_total{outcome,reason}`, `medikube_files_bytes_total` and `medikube_files_serve_duration_seconds{disposition,outcome}` in `internal/obs/`, with a test asserting **no** label value can be a file name, patient id or attachment id

**Checkpoint**: US1 and US2 both work independently. Documents can be attached to every record kind
in the application, including kinds this phase never named.

---

## Phase 5: User Story 3 — Watch one value move over time (Priority: P3)

**Goal**: pick one value and see every reading of it, in order, with the range it was measured
against — and never see two units drawn as one line.

**Independent Test**: seed a person with eight lab results spanning two years carrying the same
three component names, two numeric and one categorical, with two of the numeric readings recorded
in a different unit; open the trend view; confirm every distinct name is listed once with its
latest value, unit, status and reading count; select one and confirm the readings are in date order
with out-of-range readings marked and the summary figures correct; confirm the differing-unit
readings are not mixed and the view says which unit is shown; select the categorical one and
confirm a value history rather than a chart.

### Tests for User Story 3 ⚠️ write first, confirm they fail

- [ ] T120 [P] [US3] Write failing rollup cases in `internal/web/api/labcomponents_http_test.go`: every distinct component listed **once** with its latest value, unit, status, reading count and latest date (FR-024, US3-1); grouping by catalogue match where one exists and by normalised name otherwise, with `match` stating which (FR-025); `units[]` and `multi_unit`; `test_name` reflects the most recent spelling
- [ ] T121 [P] [US3] Write failing series cases in `internal/web/api/labcomponents_trend_test.go`: readings in date order with the range recorded **with each one** (FR-026, FR-035); `band` naming which reading's range it draws (US3-10); readings in different units never combined — `unit_required` `400` telling the account holder that more than one unit exists and listing them to choose from, and the chosen unit stated on the response (FR-027) — listing the available units when more than one exists (research D-31); a `unit` with no readings returns `200` with an empty array, not `404`; an unknown `series_key` is `404`
- [ ] T122 [P] [US3] Write failing summary cases: the eight figures of FR-030 (US3-7); the direction returned **together with** the rule that produced it (FR-031); fewer than three readings yields no direction plus the explicit statement, and the one reading is still returned (FR-032, US3-6); a categorical component yields a value history with per-value counts and **no** mean, range or direction (FR-033, US3-9)
- [ ] T123 [P] [US3] Write the failing no-conversion tests: a series requested in one unit contains no value converted from another anywhere in the response (FR-027, FR-028, US3-5); plus a compile-level assertion that no package under `internal/domain/labs`, `internal/service/lab*` or `internal/store/lab*` imports `internal/domain/clinical/units` (research D-09)
- [ ] T124 [P] [US3] Write failing cap tests: a 600-reading series returns 500 with `capped: true`, `cap_limit`, and `range_start`/`range_end` reflecting the returned window, with the summary computed over that window (FR-029, FR-034)
- [ ] T125 [P] [US3] Write failing date-range tests: `from`/`to` restrict the series **and** the summary together, never one without the other (FR-029, US3-11)
- [ ] T126 [P] [US3] Write the failing authorization test: a trend for a patient the actor cannot reach is `404`, indistinguishable from that patient not existing (FR-072, US3-12); unauthenticated is `401`
- [ ] T127 [P] [US3] Write failing integration tests in `internal/store/labtrend/repo_test.go` for both queries against a real test app, **including an injection matrix** pushing `%'; DROP TABLE` shaped input through `q`, `series_key` and `unit`, and asserting the `patient` value comes from the grant and not from the request (research D-27)
- [ ] T128 [P] [US3] Write failing render tests in `internal/web/views/labs/chart_templ_test.go` and `trends_templ_test.go`: `region[name="Lab trends"]`; one `<svg role="img">` with an accessible name naming the component and the unit; the reference band; out-of-range points marked by **shape and text** with the same readings in an adjacent table (FR-021, SC-002); the unit statement; the insufficient-readings message; the capped message; and the "nothing to compare yet" empty state inside the landmark (US3-2)
- [ ] T129 [P] [US3] Write `e2e/specs/trends.spec.ts` covering `/labs/trends` at both viewports with the full gate assertions, plus selecting a component, switching units, applying a date range, and the `GET /labs/trends/series` page action
- [ ] T130 [P] [US3] Write the build-tagged scale test asserting SC-003: a component with 100 readings across 50 results returns its series and summary within 2 s, and a 500-reading component stays responsive

### Implementation for User Story 3

- [ ] T131 [US3] Declare the ports in `internal/service/labtrend/ports.go`: `Reader` (2 methods — `Catalog`, `Series`), `Authorizer` (1)
- [ ] T132 [US3] Implement `internal/service/labtrend/service.go` — the rollup, the series, the unit rule, the cap, and the delegation of every figure to `internal/domain/labs.Summarise`
- [ ] T133 [P] [US3] Implement `internal/service/labtrend/labtrendtest/fake.go`
- [ ] T134 [US3] Implement `internal/store/labtrend/sql.go` — the two fully parameterised statements of research D-27, with a comment naming the indexes they rely on
- [ ] T135 [US3] Implement `internal/store/labtrend/repo.go` — the `labtrend.Reader` implementation over `app.DB()`
- [ ] T136 [US3] Implement `internal/web/api/labcomponents.go` — the two handlers `listLabComponents` and `getLabComponentTrend` and their DTOs per `contracts/lab-components.md`, registered under those `operationId`s
- [ ] T137 [P] [US3] Implement `internal/web/views/labs/trends.templ` — the component list, the unit chooser, the summary and the direction with its rule
- [ ] T138 [P] [US3] Implement `internal/web/views/labs/chart.templ` and the pure linear-scale function in `internal/domain/labs/scale.go` — inline SVG, no library, `role="img"`, band, shaped markers
- [ ] T139 [US3] Implement `internal/web/page/labtrends.go` and register `GET /labs/trends/series` as a `KindPageAction` naming `e2e/specs/trends.spec.ts`
- [ ] T140 [US3] Extend `internal/cli/seed.go` so patient A's readings include two in a different unit and one categorical component, and extend `internal/testsupport/scale/generate.go` with a 500-reading component

**Checkpoint**: US1, US2 and US3 all work independently. A component's history is comparable and
the multi-unit trap is closed.

---

## Phase 6: User Story 4 — Enter the same test the same way every time (Priority: P4)

**Goal**: catalogue-assisted entry that makes trending correct rather than merely present, without
ever preventing a test the catalogue has never heard of.

**Independent Test**: seed the instance with the catalogue; type three characters of a common test
and confirm suggestions appear; pick one and confirm the name, unit, category and typical range are
filled in and remain editable; save; enter a second component for the same test under a different
spelling and pick the same entry; confirm the trend view lists it once with two readings; then
enter a component the catalogue does not contain and confirm it saves and trends on its own.

### Tests for User Story 4 ⚠️ write first, confirm they fail

- [ ] T141 [P] [US4] Write failing cases in `internal/web/api/cataloglabtests_http_test.go`: `q` matches over standard name, alternative names and standard code (FR-038, US4-1); `category` and `common` filters; a `q` shorter than three characters is `400 query_too_short` (FR-039); a match-nothing query returns `loaded: true` with an empty list, while an unseeded instance returns `loaded: false` — **two different states** (US4-2, environment-failure edge case)
- [ ] T142 [P] [US4] Write the failing read-only gate test in `internal/httproute/catalog_readonly_test.go`: the route registry contains **no** method other than `GET` under `/api/v1/catalog`, and `catalog_lab_tests` has all five API rules `nil` so the auto-CRUD subtree is superuser-only (FR-037, US4-7)
- [ ] T143 [P] [US4] Write the failing disclosure test: two different accounts issuing the same catalogue query receive **byte-identical** responses, and no catalogue read writes an audit row (FR-043, US4-8)
- [ ] T144 [P] [US4] Write the failing catalogue-seed tests in `internal/store/migrations/catalog_seed_test.go`: `up` run twice yields the same row count; an entry in the extract with `ref_low > ref_high` fails the migration loudly rather than seeding a bad range
- [ ] T145 [P] [US4] Write the failing grouping test in `internal/service/labtrend/catalog_grouping_test.go`: two components saved under different spellings but carrying the same `catalog_test` appear as **one** rollup entry — **discharges SC-015** with two readings (FR-041, US4-4); two components with the same name in different letter case and surrounding spaces and no catalogue match also appear as one entry with two readings (FR-025, US4-5)
- [ ] T146 [P] [US4] Write the failing fill-in test: choosing an entry supplies `test_name`, `unit`, `category`, `ref_low` and `ref_high`, and **every one of them can still be changed before saving** (FR-040, US4-3, SC-014)
- [ ] T147 [P] [US4] Write the failing uncatalogued test: a component whose test is not in the catalogue saves without complaint and is trendable in its own right (FR-042, US4-6)
- [ ] T148 [P] [US4] Write failing render tests in `internal/web/views/labs/suggest_templ_test.go`: the fragment is a `role="listbox"`; "nothing matched that" and "the standard test catalogue has not been loaded on this instance" are **distinct strings** rendered from distinct states
- [ ] T149 [P] [US4] Extend `e2e/specs/labs.spec.ts`: nothing happens for two characters, suggestions appear from the third within one second (SC-014), the chosen entry fills four fields and all four remain editable, and the `GET /lab-results/component-suggest` page action is exercised

### Implementation for User Story 4

- [ ] T150 [US4] Declare the ports and implement `internal/service/catalog/ports.go` and `service.go` — `Search` and `Loaded`, with the three-character minimum enforced in the service, not in the query
- [ ] T151 [P] [US4] Implement `internal/service/catalog/catalogtest/fake.go`
- [ ] T152 [US4] Implement `internal/store/catalog/repo.go` — the `LIKE` search over name, synonyms and code, with keyset cursors
- [ ] T153 [US4] Implement `internal/web/api/cataloglabtests.go` — the single handler `listCatalogLabTests` and its DTOs per `contracts/catalog-lab-tests.md`, registered under that `operationId`
- [ ] T154 [P] [US4] Implement `internal/web/views/labs/suggest.templ` — the listbox fragment and the two distinct empty messages
- [ ] T155 [US4] Implement `GET /lab-results/component-suggest` in `internal/web/page/actions.go`, registered as a `KindPageAction` naming `e2e/specs/labs.spec.ts`, wired with `data-on:input__debounce.300ms` from the component editor
- [ ] T156 [US4] Wire catalogue selection through `internal/web/views/labs/componenteditor.templ` and `internal/service/labresult/components.go` so the chosen entry's id is stored as `catalog_test` on the component (FR-041) while every filled value remains an ordinary editable field
- [ ] T157 [US4] Add `assets/catalog/lab-tests.json` — the vendored LOINC-derived extract, each entry carrying its standard code, name, short name, default unit, category, alternative names, common-use flag and typical reference bounds (FR-036) — with a `PROVENANCE.md` beside it recording where it came from, its licence terms and how to regenerate it, and embed it with `embed.FS`

**Checkpoint**: catalogue-assisted entry works and makes US3 correct rather than merely present.

---

## Phase 7: User Story 5 — Say what a result was about (Priority: P5)

**Goal**: a lab result that records why it was taken, readable from both ends, and destroyed by
neither end's deletion.

**Independent Test**: seed a person with a condition, an encounter, a medication, a procedure and a
treatment; record a lab result; link it to one of each; confirm all five appear on the result and
the result appears on each of the five; remove one link and confirm both records survive intact;
delete one of the linked records and confirm the lab result is untouched apart from losing that
connection.

### Tests for User Story 5 ⚠️ write first, confirm they fail

- [ ] T158 [P] [US5] Write failing cases in `internal/web/api/labresults_links_test.go`: linking a lab result to conditions, medications and procedures through `PATCH /api/v1/records/lab-results/{id}` records all of them and shows them on both ends (FR-044, FR-046, US5-1, US5-2)
- [ ] T159 [P] [US5] Write failing back-relation tests: `encounters` and `treatments` are present on the lab result response and **absent from its request DTOs**; linking to them is a `PATCH` on the encounter or the treatment and is reflected on the lab result (research D-28)
- [ ] T160 [P] [US5] Write the failing cross-patient test: linking to a record belonging to a different person is refused and **the refusal discloses nothing about whether that record exists** (FR-045, US5-3)
- [ ] T161 [P] [US5] Write the failing unlink test: removing a link leaves both records otherwise unchanged (FR-047, US5-4)
- [ ] T162 [P] [US5] Write failing deletion tests: deleting a condition linked to two lab results leaves both intact and simply no longer referring to it (US5-5); deleting a lab result linked to five records leaves all five intact (US5-6) — both verified by reading the records afterwards
- [ ] T163 [P] [US5] Write the failing audit test: every link and unlink is recorded as a change to the **lab result**, by opaque id, with no content (FR-048, US5-7)
- [ ] T164 [P] [US5] Write the failing missing-kind test: where a kind named in this story is not registered on an instance, the reference to it is simply not offered and nothing in this phase fails
- [ ] T165 [P] [US5] Write failing render tests for the link editor in `internal/web/views/records/labresult_links_templ_test.go`: both the owned relations and the back-relations are editable from the lab result page, and the two directions are visually indistinguishable to the user
- [ ] T166 [P] [US5] Extend `e2e/specs/labs.spec.ts` with linking and unlinking from the lab result detail page at both viewports

### Implementation for User Story 5

- [ ] T167 [US5] Wire `internal/service/labresult/service.go` into phase 003's `internal/service/link` so link writes validate, server-side, that both records belong to the same **stored** patient — never a client-supplied value
- [ ] T168 [US5] Implement the back-relation projection in `internal/store/labresult/mapper.go` — `encounters_via_lab_results` and `treatments_via_lab_results` read into the domain entity as read-only sets
- [ ] T169 [US5] Extend `internal/web/api/labresults.go` with the five link arrays: `conditions`, `medications` and `procedures` writable; `encounters` and `treatments` read-only
- [ ] T170 [P] [US5] Extend `internal/web/views/records/labresult.templ` with the link editor covering both directions, patching whichever record owns the field

**Checkpoint**: all five user stories are independently functional. A pile of results has become a
chart.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: the gates, the cross-cutting proofs, and the things that must be true of the phase as
a whole rather than of any one story.

- [ ] T171 [P] Regenerate `api/openapi.json` with `task openapi` and commit it; confirm `git diff --exit-code api/openapi.json` is clean afterwards (FR-084, SC-016)
- [ ] T172 [P] Extend `internal/openapi/gate_test.go` to assert the nine new `operationId`s — `listCatalogLabTests`, `uploadAttachment`, `listAttachments`, `getAttachmentContent`, `updateAttachment`, `deleteAttachment`, `restoreAttachment`, `listLabComponents`, `getLabComponentTrend` — exist in both the route registry and the committed document, that phase 001's `listRecords`, `listRecordsOfKind`, `createRecord`, `getRecord`, `updateRecord` and `deleteRecord` now resolve `lab-results` as a fifteenth kind with no new route, that the `lab_result` `oneOf` branch carries a `kind` discriminator, and that **no `page_action` route appears in the document**
- [ ] T173 [P] Extend `internal/records/registry_completeness_test.go` to assert fifteen kinds fully wired — registry entry, OpenAPI branch, two page routes, default sort, searchable fields, seed fixture, two smoke cases — **and an attachment-cleanup hook bound for every one of them** (FR-049)
- [ ] T174 [P] Extend `e2e/routes.gate.spec.ts` so the four new pages of FR-081 — the lab result list, the lab result detail, the trend comparison and the document library — have smoke cases at both viewports, each inside the standard page structure with its own landmark, and each of the four `page_action` routes names an existing spec file that actually references it (research D-25)
- [ ] T175 [P] Run `task lint` to zero findings, confirming in particular the `labs-never-convert` `depguard` rule and the `NewFileToken`/`NewFileFromURL` `forbidigo` patterns fire on a deliberately broken commit before being removed
- [ ] T176 [P] Extend `internal/testsupport/phileak/exercise.go` with file names, descriptions, test names, values, units, reference ranges and interpretations as sentinels, exercise **every operation this phase defines**, and assert zero occurrences across the zerolog stream, the Prometheus registry, the OTel span recorder and the Sentry transport (FR-078, SC-008)
- [ ] T177 [P] Wire `task test:scale` into CI as a non-blocking nightly job and record its measured numbers against SC-003, SC-011 and FR-085 in `specs/004-labs-and-attachments/quickstart.md` §8
- [ ] T178 [P] Extend the phase-001 long-stream CI assertion to also hold a **slow upload** open for more than five minutes, proving the `ServeEvent` `WriteTimeout` override still covers the upload route (research D-33, shared-design risk R7)
- [ ] T179 [P] Add to the PocketBase upgrade checklist in `docs/pocketbase-upgrade.md`: re-verify `fsys.CreateThumb`'s decodable type set and `fsys.Serve`'s `Content-Disposition` quoting, because research D-17 and the non-Latin-filename requirement both depend on them (shared-design risk R8)
- [ ] T180 [P] Review every new metric and span in `internal/obs/` against Constitution VI: label values are bounded and allowlisted, and no file name, description, test name, unit, patient id, attachment id or record id is ever one
- [ ] T181 [P] Extend the Sentry `BeforeSend` scrubber in `internal/obs/sentry.go` to cover file names and descriptions, with a test that a `500` raised from the upload path carries neither
- [ ] T182 [P] Extend the boot log line in `internal/platform/pb/assertions.go` to state how many file fields were verified as `Protected: true`, so an operator can see the number go from one to two rather than trusting that it did
- [ ] T183 [P] Run the keyboard-only accessibility pass over upload, replace, delete, restore, component entry, catalogue selection and trend selection at both viewports, asserting a visible focus indicator at every step and that focus is never lost into a closed drawer
- [ ] T184 [P] Update the project README's route table and the operator notes covering `MEDIKUBE_FILES_*`, `MEDIKUBE_RETENTION_TRASH_DAYS` and `MEDIKUBE_LABS_MAX_SERIES_POINTS`, including the `text/csv` sniffing note from `quickstart.md` §1
- [ ] T185 Run `specs/004-labs-and-attachments/quickstart.md` end to end on a clean instance and fix anything it catches; a step that does not work as written is a defect in the code or in the document, and either way it is fixed here
- [ ] T186 [P] Confirm the container build is green from the repository root (`task docker:build`, `dir: ..`) with the four new collections and the embedded catalogue extract, and that `pb_data/` never enters the build context
- [ ] T187 [P] Review coverage, confirming generated `*_templ.go` is excluded and that `internal/domain/labs` and `internal/domain/files` — the two pure packages carrying this phase's clinical arithmetic and its file-safety rules — are at or near full statement coverage
- [ ] T188 Re-run the Constitution Check in `plan.md` against the delivered code, record the post-implementation result, and confirm the four Complexity Tracking entries are still the only ones
- [ ] T189 Write `specs/004-labs-and-attachments/traceability.md` — the mechanical join, generated from `spec.md` and `tasks.md` rather than written by hand: one row per functional requirement (all 85) naming the task ids that satisfy it and the named test that proves it, one row per acceptance scenario naming its test, and one row per success criterion (all 16) naming its task or its exit criterion. **A functional requirement with no task, or a success criterion that is neither mapped nor marked `[outcome metric]` in `spec.md`, fails the phase** (cross-artifact finding M7)

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1, T001–T009)** — no dependencies; can start immediately.
- **Foundational (Phase 2, T010–T046)** — depends on Setup. **BLOCKS every user story.**
- **User stories (Phases 3–7)** — all depend on Foundational. Once it is complete they can proceed
  in parallel or sequentially in priority order.
- **Polish (Phase 8, T171–T189)** — depends on every story that is being shipped.

### Critical path inside Foundational

```
T010 → T011 ─┐
T012 → T013 ─┤
T014 → T015 ─┼→ T020 → T021 ──┐
T016 → T017 ─┤                │
T018 → T019 ─┘                │
T022 → T023 ──┐               │
T024 → T025 ──┴───────────────┤
T026 → T027 ──────────────────┤
                              ▼
        T028 → T029 → T030 → T031 → T032 → T033      (migration order is forced:
                                    T034 → T035       catalog before results,
                                                      results before components)
                                          ▼
                                        T038  ← the fixture regeneration; everything downstream
                                                that touches a database depends on it
        T039 → T040   T041 → T042   T043 → T044   T045 → T046   (independent of each other)
```

### Cross-story dependencies

The five stories are independently implementable and demonstrable. Three soft couplings exist and
none of them blocks a story from being built or tested on its own:

| Coupling | Effect |
|---|---|
| US2's record-deletion cleanup (T091) is proved against **any** registered kind | US2 does not need US1. It is proved against phase-003 kinds |
| US1's delete test (T051) asserts attachments go to the trash | If US2 is not yet built, that assertion is skipped by a build tag and **must** be enabled when US2 lands. T091 is the durable version of it |
| US3 reads components that US1 writes | US3's tests seed components directly through the repository, so it can be built and tested before US1's HTTP surface exists |
| US4 fills in fields that US1 renders | US4's grouping test (T145) seeds `catalog_test` directly. The UI wiring (T156) is the only task that needs US1's editor |
| US5 links to kinds phase 003 delivered | US5 needs US1's lab result to exist as a record kind; it is last in priority for exactly that reason |

### Within each story

Tests are written and **fail** before the implementation they cover. Then: domain → ports →
service → store → DTOs and handlers → templ → pages → seed → scale.

---

## Parallel Opportunities

- **Setup**: T002–T009 all in parallel.
- **Foundational**: the six test/implementation pairs T010–T027 are independent of one another and
  of the migrations; the four shared-machinery pairs T039–T046 are independent of everything else in
  the phase. Only the migration chain T028–T035 and then T038 is strictly ordered; T036 is a test
  and runs whenever.
- **Within a story**: every task marked `[P]` in its test block can run at once — they are separate
  files with no shared state. The implementation tasks are ordered by the domain → store → web
  dependency and are mostly not parallel, with the marked exceptions (fakes, templ files).
- **Across stories**: after the Foundational checkpoint, five developers can take US1–US5
  simultaneously. The heaviest are US2 (41 tasks) and US1 (32).
- **Polish**: T171–T184, T186 and T187 are all parallel; T185, T188 and T189 are last and sequential.

### Parallel example: the Foundational domain block

```bash
# five failing-test tasks at once, five different files, no shared state:
Task: "T012 canonical_test.go — normalisation"
Task: "T014 refrange_test.go — Classify"
Task: "T016 sortdate_test.go — the FR-008 fallback"
Task: "T018 series_test.go — the halves rule"
Task: "T022 mime_test.go — sniffing and InlineSafe"
```

### Parallel example: User Story 2's test block

```bash
# twenty-three failing-test tasks, all independent files:
Task: "T080 upload refusals — size, empty, sniffed type"
Task: "T081 SC-004 byte-for-byte fidelity"
Task: "T085 content stream headers, Range, If-None-Match"
Task: "T090 trash: delete, restore, the three refusals, owner-or-superuser purge"
Task: "T095 the authorization matrix, content tested three ways"
Task: "T096 audit: read_sensitive only for a non-owner read, no content"
# … and the rest of T079–T101
```

---

## Implementation Strategy

### MVP first (User Story 1 only)

1. Phase 1: Setup.
2. Phase 2: Foundational — **critical, blocks everything**.
3. Phase 3: User Story 1.
4. **Stop and validate**: run `quickstart.md` §3 end to end. A carer can now keep a structured
   laboratory history in which every line is a real, comparable, correctly-marked value.
5. Ship or demo.

### Incremental delivery

1. Setup + Foundational → the schema and the arithmetic exist.
2. + US1 → a structured laboratory history. **MVP.**
3. + US2 → a private, per-record document store with recoverable deletion, across every record
   kind. This is a whole product on its own for somebody who mostly keeps paperwork.
4. + US3 → a component's history becomes comparable, with the multi-unit trap closed.
5. + US4 → entry becomes consistent, which makes US3 correct rather than merely present.
6. + US5 → the results acquire their reasons.
7. Polish → the gates go green and the phase is provably complete.

Each step adds value without breaking the previous ones.

---

## Notes

- **Test tasks: 91 of 188.** That ratio is the point, not an accident: in an application where an
  authorization mistake exposes somebody's medical history, the tests are the deliverable
  (Constitution III).
- Every one of the specification's 56 acceptance scenarios across US1–US5 maps to at least one
  named test task above. FR-083 makes the phase incomplete while any of them is missing or failing.
- `[P]` means different files and no incomplete dependency.
- Verify a test fails before implementing against it. A red-to-green transition that was never red
  proves nothing.
- Commit after each task or logical group, `feat(medikube): …` per the house convention.
- Stop at any checkpoint to validate a story independently.
- The single most expensive mistake available in this phase is forgetting **T038**. Every
  integration test downstream of it runs against a stale schema and fails in a way that looks like
  a code bug.
