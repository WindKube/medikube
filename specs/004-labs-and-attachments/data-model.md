# Data Model: Labs and Attachments (phase 004)

**Phase 1 output.** The entities this phase introduces or changes: fields, types, constraints,
enumerations with exact values, relationships, validation rules, state transitions, and the
PocketBase collection definitions and migrations implied.

Consistent with [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) §1. Where this document adds a field the contract does
not name, `plan.md`'s Deviations table records why.

**Rules inherited from the shared contract and not repeated per collection:**

1. Ids are PocketBase's 15-character opaque text ids. No integers anywhere, and no identifier ever
   lives in a `number` field.
2. Every collection has `id`, `created` (autodate) and `updated` (autodate).
3. Every patient-scoped collection carries `patient`
   (`RelationField{CollectionId: patients, Required: true, MaxSelect: 1, CascadeDelete: true}`).
   There is no other route to a patient.
4. **All five API rules are `nil` on every collection in this phase.** Superuser-only. Asserted at
   boot; MediGo refuses to start otherwise.
5. **Every `FileField` is `Protected: true`.** Asserted at boot. After this phase there are exactly
   two: `patients.photo` and `attachments.file`.
6. Enum fields are `core.SelectField{MaxSelect: 1}` **and** a Go string type with `Valid()` in
   `internal/domain`. All enum values are `snake_case`.
7. `notes` is `text`, max 5000, optional, on every clinical kind; it is PHI and is redacted in
   marshalling for logs.
8. `tags` is `RelationField{CollectionId: tags, MaxSelect: 0}` on every clinical kind.
9. **No `deleted_at` on any record collection.** It exists on `attachments` only.
10. A clinical event date with no meaningful time is a `date` field holding `YYYY-MM-DD`.
11. Uniqueness is a collection index, because PocketBase has no per-field `Unique`.

---

## 1. Enumerations introduced by this phase

All are defined once in `internal/domain/clinical/vocab_lab.go` as Go string types with `Valid()`
and `All()`, and once in the migrations as `SelectField.Values`. A table-driven test asserts the two
lists are identical, so they cannot drift.

### 1.1 `lab.category` — `LabCategory`

```
blood_work  urinalysis  microbiology  pathology  genetics  imaging  other
```

Used by `lab_results.category` and by `catalog_lab_tests.category`. The two are the same
vocabulary deliberately: FR-040 fills a component's category from the catalogue entry, which is
only meaningful if the vocabularies match.

### 1.2 `lab_component.result_type` — `ResultType`

```
quantitative  qualitative  textual
```

Default `quantitative`. Maps to the specification's prose (research D-32): numeric →
`quantitative`, categorical → `qualitative`, free text → `textual`.

### 1.3 `lab_component.status` — `ComponentStatus`

```
normal  high  low  critical  abnormal
```

Optional. When present it is the status shown and arithmetic never overrides it (FR-019).

### 1.4 `attachment.category` — `AttachmentCategory`

```
report  lab_result  imaging  prescription  insurance_card  correspondence  photo  other
```

Optional. Note `lab_result` here is a *document* category, not a record kind; the two spellings are
identical and that is fine because they never appear in the same field.

### 1.5 Reused, not redefined

- `OrderStatus` (phase 002): `ordered scheduled in_progress completed cancelled`. `lab_results.status`
  uses it; a new result defaults to `ordered` (FR-003).
- `kind.Kind` (phase 001, extended): gains `lab_result`, path segment `lab-results`, making fifteen.
- `audit_events.target_kind`: **nothing is added.** `lab_result` and `attachment` are both declared
  by phase 001's `audit_events` migration, which carries the shared design contract's complete
  vocabulary (001 data-model §3). This phase asserts them present (§7) and adds no value.

### 1.6 Derived, not stored — `RangeAssessment`

```
not_assessed  below  within  above
```

A pure return value of `labs.Classify` (research D-04). It is **never** a column and never
persisted; it is computed wherever a value is rendered.

### 1.7 Derived, not stored — `TrendDirection`

```
rising  falling  steady  insufficient_data
```

A pure return value of `labs.Summarise` (research D-07). Never a column.

---

## 2. `catalog_lab_tests` — read-only reference data

Not patient-scoped. Contains nothing about any person (FR-043). This is the only collection in
MediGo whose list operation does not require `?patient=`.

| Field | Type | Req | Constraints / notes |
|---|---|---|---|
| `loinc_code` | text | no | ≤ 40. May be empty for a MediGo-added entry that has no LOINC code; unique **when non-empty** |
| `name` | text | **yes** | 1..300. The standard name (FR-036) |
| `short_name` | text | no | ≤ 60 |
| `default_unit` | text | no | ≤ 40 |
| `category` | select | no | `LabCategory` |
| `synonyms` | json | no | validated `[]string`, ≤ 32 entries, each ≤ 120 chars. The "alternative names" of FR-036/FR-038 |
| `is_common` | bool | **yes** | default `false` |
| `ref_low` | number | no | typical lower bound |
| `ref_high` | number | no | typical upper bound |

**Indexes**

```
CREATE UNIQUE INDEX idx_catalog_lab_tests_loinc ON catalog_lab_tests (loinc_code) WHERE loinc_code != ''
CREATE INDEX        idx_catalog_lab_tests_name  ON catalog_lab_tests (LOWER(name))
CREATE INDEX        idx_catalog_lab_tests_cat   ON catalog_lab_tests (category, is_common)
```

**Validation**

- `ref_low <= ref_high` when both are present. An inverted pair in the vendored extract fails the
  migration loudly rather than seeding a bad reference range.
- `synonyms` is validated as a Go `[]string` before it is written; a free-form blob is never stored.

**Lifecycle.** Created and populated by one migration (§5.1). No create, update or delete path
exists through the application (FR-037): there is no non-`GET` route under `/api/v1/catalog`, and a
gate test asserts it. Amending the catalogue means shipping a new extract and a new migration.

**Loaded versus empty.** `GET /api/v1/catalog/lab-tests` reports `loaded: false` when the collection
holds zero rows, which is a different answer from "nothing matched" (FR: environment failures).

---

## 3. `lab_results` — the fifteenth record kind

Registered by `records.Register(kind.LabResult, …)`. Carries everything every clinical kind carries
plus its own fields.

| Field | Type | Req | Constraints / notes |
|---|---|---|---|
| `patient` | relation → patients | **yes** | MaxSelect 1, CascadeDelete. The authorization anchor |
| `tags` | relation → tags | no | MaxSelect 0 |
| `notes` | text | no | ≤ 5000. PHI |
| `test_name` | text | **yes** | 1..300. PHI. The only required field besides `patient` (FR-002) |
| `test_code` | text | no | ≤ 60 |
| `category` | select | no | `LabCategory` |
| `catalog_test` | relation → catalog_lab_tests | no | MaxSelect 1. Records which catalogue entry was chosen (FR-041) |
| `status` | select | **yes** | `OrderStatus`, default `ordered` (FR-003) |
| `ordered_on` | date | no | `YYYY-MM-DD` |
| `collected_on` | date | no | `YYYY-MM-DD` |
| `resulted_on` | date | no | `YYYY-MM-DD` |
| `sort_date` | date | **yes** | **derived** — see §3.2 |
| `interpretation` | text | no | ≤ 2000. PHI. The laboratory's overall comment |
| `is_panel` | bool | **yes** | **derived** — `len(components) > 0`. Never in a request DTO |
| `value` | number | no | the single overall value (FR-005) |
| `unit` | text | no | ≤ 40 |
| `ref_low` | number | no | |
| `ref_high` | number | no | |
| `ref_text` | text | no | ≤ 200. A range in words: "negative", "not detected" |
| `practitioner` | relation → practitioners | no | MaxSelect 1. Ordering clinician (FR-004) |
| `facility` | relation → facilities | no | MaxSelect 1. Place of care (FR-004) |
| `conditions` | relation → conditions | no | MaxSelect 0 (FR-044) |
| `medications` | relation → medications | no | MaxSelect 0 (FR-044) |
| `procedures` | relation → procedures | no | MaxSelect 0 (FR-044) |

**Back-relations** (no column here; owned by the other side, created in phase 003):
`encounters_via_lab_results`, `treatments_via_lab_results`. Exposed read-only on the DTO and edited
by patching the other record (research D-28).

### 3.1 Indexes

```
CREATE INDEX idx_lab_results_list     ON lab_results (patient, sort_date DESC, id DESC)
CREATE INDEX idx_lab_results_status   ON lab_results (patient, status)
CREATE INDEX idx_lab_results_category ON lab_results (patient, category)
CREATE INDEX idx_lab_results_catalog  ON lab_results (catalog_test)
```

`idx_lab_results_list` is the keyset-pagination index and is what makes SC-011 reachable.

### 3.2 Derived fields, and who writes them

| Field | Rule | Writer | Test that keeps it honest |
|---|---|---|---|
| `sort_date` | `COALESCE(resulted_on, collected_on, ordered_on, DATE(created))` (FR-008) | `labs.LabResult.Validate` → `Save`, in the same write as the source fields | repository contract: mutate `resulted_on`, re-read `sort_date` |
| `is_panel` | `len(components) > 0` | the component replace-set, in the same transaction | service test: add a component, assert `true`; remove the last, assert `false` |

Neither is settable by any client. Both appear in responses.

### 3.3 Validation rules (`labs.LabResult.Validate`)

`Validate` returns a `*domain.ValidationError` listing **every** offending field at once, so a form
shows all of its errors in one submission (FR-007's "reporting every offending field in the same
submission", US1 scenario 4).

| Rule | Error code | Requirement |
|---|---|---|
| `test_name` non-empty after trim, ≤ 300 | `required` / `too_long` | FR-002 |
| `patient` present and reachable by the actor | `not_found` (404 at the edge) | FR-001, FR-072 |
| `status ∈ OrderStatus` | `invalid_value` | FR-003 |
| `category ∈ LabCategory` when present | `invalid_value` | FR-002 |
| `collected_on >= ordered_on` when both present | `date_order` | FR-007 |
| `resulted_on >= collected_on` when both present | `date_order` | FR-007 |
| no date is in the future | `future_date` | consistency with phase 003 |
| **not** (`value` or `unit` or `ref_low` or `ref_high` or `ref_text` present **and** `len(components) > 0`) | `panel_and_value` | FR-005, US1 scenario 10 |
| `ref_low <= ref_high` when both present | `ref_range_inverted` | FR-017 |
| `interpretation` ≤ 2000, `notes` ≤ 5000 | `too_long` | edge case: exactly at the limit is accepted, one over is refused with the field and the limit named |
| `practitioner` / `facility` resolve and belong to the actor's directory | `not_found` | FR-004 |
| `conditions`/`medications`/`procedures` all belong to the same `patient` | `not_found` (discloses nothing) | FR-045 |

A result with **neither** an overall value nor components is **valid** — an ordered test that has
not come back yet — and is presented as awaiting a result, not as an error.

### 3.4 State transitions

`status` follows `OrderStatus` and is free to move between any two values; MediGo does not police a
workflow it cannot observe. The transition that *is* policed is the panel/scalar shape:

```
                 add components
   scalar  ──────────────────────▶  panel
   (value set,                       (components ≥ 1,
    components = 0)  ◀──────────────  value/unit/ref_* cleared)
                 remove all components
```

- Adding components to a result that carries an overall value in the **same** submission is
  refused (`panel_and_value`), and neither part is discarded — the client must clear one (FR-005,
  US1 scenario 10).
- Converting is allowed in both directions after creation (FR-006, US1 scenario 9). Converting
  scalar → panel clears `value`, `unit`, `ref_low`, `ref_high`, `ref_text` in the same
  transaction; the response shows them absent, and the audit trail records one update.
- `is_panel` follows the arrow; it never leads it.

### 3.5 Deletion

Hard delete, confirmed in the UI by an action that names the result and warns that it and its
components cannot be recovered (US1 scenario 12). In one transaction:

1. every `lab_components` row cascades away permanently (FR-015) and its readings disappear from
   every trend on the next query, because trends are derived (FR-024);
2. every `attachments` row whose `(owner_kind, owner_id)` is `(lab_result, id)` is **moved to the
   trash**, not destroyed (FR-067);
3. every multi-relation reference from other records is removed by PocketBase's relation cleanup,
   leaving those records otherwise intact (FR-047, US5 scenario 6);
4. the `search_index` row is removed;
5. one `delete` audit row is written, by opaque id, with no content.

The confirmation states both (1) and (2) before the account holder commits.

---

## 4. `lab_components` — a child collection, not a kind

Has no independent life: no CRUD endpoints, no notes, no tags, no `patient` column. Reached through
its parent (research D-29).

| Field | Type | Req | Constraints / notes |
|---|---|---|---|
| `lab_result` | relation → lab_results | **yes** | MaxSelect 1, **CascadeDelete** |
| `test_name` | text | **yes** | 1..300. PHI |
| `abbreviation` | text | no | ≤ 20, e.g. "WBC" |
| `canonical_name` | text | **yes** | **derived** — `labs.Normalise(test_name)` (FR-025, research D-05) |
| `catalog_test` | relation → catalog_lab_tests | no | MaxSelect 1 (FR-041) |
| `result_type` | select | **yes** | `ResultType`, default `quantitative` (FR-012) |
| `value` | number | no | present iff `result_type = quantitative` |
| `value_text` | text | no | ≤ 500; present iff `result_type ∈ {qualitative, textual}` |
| `unit` | text | no | ≤ 40. **Never converted** (FR-028) |
| `ref_low` | number | no | |
| `ref_high` | number | no | |
| `ref_text` | text | no | ≤ 200. A range in words |
| `status` | select | no | `ComponentStatus` — the laboratory's own judgement (FR-019) |
| `display_order` | number | **yes** | assigned from the payload array index (FR-012, FR-016) |

### 4.1 Indexes

```
CREATE INDEX idx_lab_components_parent    ON lab_components (lab_result, display_order)
CREATE INDEX idx_lab_components_canonical ON lab_components (canonical_name, unit)
CREATE INDEX idx_lab_components_catalog   ON lab_components (catalog_test)
```

`idx_lab_components_canonical` is what the rollup and the series queries group and filter on.

### 4.2 Validation rules (`labs.Component.Validate`)

| Rule | Error code | Requirement |
|---|---|---|
| `test_name` non-empty after trim, ≤ 300 | `required` / `too_long` | FR-012 |
| `result_type ∈ ResultType` | `invalid_value` | FR-013 |
| `quantitative` ⇒ `value` present, `value_text` empty | `value_kind_mismatch` | FR-013 |
| `qualitative`/`textual` ⇒ `value_text` present, `value` absent | `value_kind_mismatch` | FR-013 |
| `ref_low <= ref_high` when both present | `ref_range_inverted` | FR-017 |
| only one bound present is **accepted** | — | FR-017 |
| `status ∈ ComponentStatus` when present | `invalid_value` | FR-019 |
| duplicate `test_name` within one result is **accepted** | — | FR-016 |

A result reported below a detection limit ("<0.01") is a `textual` component; it is never coerced
into a number (edge case: partial and awkward data).

### 4.3 The replace-set

Components are created, updated and deleted **as the complete set belonging to their lab result**,
through the parent's payload (research D-03), inside `app.RunInTransaction`:

| Payload element | Stored state | Action |
|---|---|---|
| carries `id`, id belongs to this result | exists | update in place; `display_order` = array index |
| carries no `id` | — | create; `display_order` = array index |
| carries `id` belonging to another result | — | `422 not_found` on that element; the whole submission is refused |
| — | exists, id absent from payload | delete permanently |

Component ids are therefore stable across saves. `is_panel` is recomputed at the end of the
transaction. One `update` audit row is written **against the lab result**, not against each
component (FR-023).

### 4.4 Derived reading state

Nothing about a reading's in-range status is stored beyond the laboratory's own `status`. Both of
the following are computed on read by pure functions:

- `assessment` — `labs.Classify(value, ref_low, ref_high, status)` → `RangeAssessment` (FR-018,
  FR-019, FR-020).
- `out_of_range_count` on the parent — a count of components whose assessment is `below` or `above`,
  or whose explicit status is in `{high, low, critical, abnormal}` (FR-022).

---

## 5. `attachments` — the file collection

The first collection since `patients.photo` to hold bytes, and the only collection in MediGo with a
`deleted_at`.

| Field | Type | Req | Constraints / notes |
|---|---|---|---|
| `patient` | relation → patients | **yes** | MaxSelect 1, CascadeDelete. **The authorization anchor** |
| `owner_kind` | select | **yes** | one of the fifteen registered `kind.Kind` values |
| `owner_id` | text | **yes** | exactly 15 chars, `^[a-z0-9]{15}$`. No foreign key (research D-13) |
| `file` | file | **yes** | MaxSelect 1, **`Protected: true`**, `MaxSize` from `MEDIGO_FILES_MAX_UPLOAD_BYTES`, `MimeTypes` from `MEDIGO_FILES_ALLOWED_MIME`, `Thumbs: ["160x160t","1024x1024f"]` |
| `original_name` | text | **yes** | ≤ 255. **PHI** — patients name files after conditions |
| `size_bytes` | number | **yes** | > 0 (FR-054) |
| `mime` | text | **yes** | ≤ 120. **Sniffed server-side**, never taken from the client (FR-051) |
| `has_preview` | bool | **yes** | default `false`; set by the eager thumbnailer (research D-17) |
| `description` | text | no | ≤ 500. **PHI** |
| `category` | select | no | `AttachmentCategory` |
| `deleted_at` | date | no | RFC3339 UTC. **Non-null = in the trash** |
| `uploaded_by` | relation → users | **yes** | MaxSelect 1 |

### 5.1 Indexes

```
CREATE INDEX idx_attachments_owner   ON attachments (patient, owner_kind, owner_id)
CREATE INDEX idx_attachments_library ON attachments (patient, deleted_at, created DESC, id DESC)
CREATE INDEX idx_attachments_trash   ON attachments (deleted_at)
```

`idx_attachments_library` is the keyset-pagination index for the document library at the
2,000-document target; `idx_attachments_trash` is what the purge cron scans.

### 5.2 Validation rules (`files.Attachment.Validate` + the upload path)

| Rule | Error | Requirement |
|---|---|---|
| `size_bytes > 0` | `422 empty_file` | FR-054 |
| `size_bytes <= MEDIGO_FILES_MAX_UPLOAD_BYTES` | `413 payload_too_large`, message states the limit | FR-053 |
| sniffed `mime ∈ MEDIGO_FILES_ALLOWED_MIME` | `415 unsupported_media_type`, message names the accepted types | FR-051, FR-052 |
| `owner_kind ∈ kind.Kind` | `422 invalid_value` | FR-049 |
| `owner_id` resolves in `owner_kind`'s collection **and** belongs to `patient` | `404 not_found`, disclosing nothing | FR-050, FR-072, concurrency edge case |
| `original_name` non-empty, ≤ 255 | `422` | FR-050 |
| `description` ≤ 500 | `422 too_long` | FR-062 |
| `replaces` (when present) resolves, same patient, same `(owner_kind, owner_id)` | `404 not_found` | FR-061 |
| `original_name`, `size_bytes`, `mime` are **absent from the PATCH DTO** | — | FR-062: not editable |

Enforcement of the size limit happens three times and always before any byte is stored
(research D-15). A refusal may name the uploader's own file back to them, and that name must not
appear in any log, metric, span or Sentry event (FR-079) — asserted directly.

### 5.3 State transitions — the trash

```
                        DELETE /attachments/{id}
                        POST   /attachments (replaces=)          restore (within window)
   ┌──────────┐        record deleted (cleanup hook)         ┌──────────────┐
   │  active  │ ──────────────────────────────────────────▶  │   trashed    │
   │deleted_at│                                              │ deleted_at = │
   │  = null  │  ◀────────────────────────────────────────── │   <instant>  │
   └──────────┘        POST /attachments/{id}/restore         └──────────────┘
        │                                                            │
        │ patient or account deleted                                 │ deleted_at older than
        │ (cascade — destroyed outright, FR-068)                     │ RETENTION_TRASH_DAYS
        ▼                                                            ▼
   ┌────────────────────────────────────────────────────────────────────┐
   │  purged — row gone, blob gone, thumbnails gone, unrecoverable      │
   └────────────────────────────────────────────────────────────────────┘
```

| From | Event | To | Notes |
|---|---|---|---|
| active | owner deletes, confirmed | trashed | FR-063; the confirmation states the window |
| active | replaced by a new upload | trashed | FR-061; same transaction as the new row |
| active | its owning record is deleted | trashed | FR-067; the cleanup hook, same transaction |
| active | its patient or account is deleted | **purged** | FR-068; PocketBase cascade, no trash step |
| trashed | owner restores, within window, owning record exists | active | FR-064 |
| trashed | owner restores, owning record gone | **stays trashed** | FR-065, `409 owner_record_missing`; content still retrievable |
| trashed | owner restores, past the window | **stays trashed until the sweep** | `409 retention_expired` |
| trashed | the maintenance cron runs, `deleted_at` past the window | **purged** | FR-066 |
| purged | anything | — | terminal; nothing in the application can bring it back |

A trashed attachment is excluded from every list unless `?deleted=true`; its content remains
retrievable by an authorized caller until purge, and every such retrieval is audited.

### 5.4 What is derived, never stored

- **Storage usage** — `{documents, bytes, trashed_documents, trashed_bytes}` per patient, computed
  by one aggregate when `?usage=true` is passed (FR-071, research D-21).
- **Days remaining in the trash** — `RETENTION_TRASH_DAYS - age(deleted_at)`, computed at render.
- **Whether inline viewing is offered** — a lookup of the stored `mime` against the compile-time
  inline-safe set (FR-057, research D-16).

---

## 6. Changes to existing entities

| Entity | Change | Why |
|---|---|---|
| `kind.Kind` | `+ lab_result` (enum value), `lab-results` (path segment), "Lab result"/"Lab results" (labels), `article[name="Lab result"]` / `region[name="Lab results"]` (landmarks) | D-01. Fifteenth and final kind in the plan set |
| `records.Register` | binds the **attachment-cleanup hook** for every registered kind, centrally | FR-049, FR-067. Editing one function gives all fifteen kinds document support and cleanup |
| `records.Views` | gains an optional attachment strip on the detail component | FR-049, US2 |
| `audit_events.target_kind` | **no change** — `lab_result` and `attachment` are declared by phase 001 and asserted present here | FR-011, FR-076, FR-077 |
| `audit_events.action` | no change — `create update delete read_sensitive access_denied` already cover this phase. `read_sensitive` is written **only** when the resolved grant is not the reader's own ownership (FR-076, 005 [D-25](../005-sharing-and-collaboration/research.md#d-25)); `access_denied` is unconditional | FR-073, FR-076, FR-077 |
| `encounters.lab_results`, `treatments.lab_results` | no schema change; they become populated | FR-044, FR-046, research D-28 |
| `search_index` | gains `lab_result` rows via the registry hook. **No attachment rows** | FR-009, research D-23 |
| `internal/config.Config` | `+ Labs.MaxSeriesPoints` (`MEDIGO_LABS_MAX_SERIES_POINTS`, default 500); `Files.AllowedMIME` default set (research D-14); `Files.MaxUploadBytes` default 33554432 | FR-034, FR-052, FR-053 |
| CSP | pages gain `frame-src 'self'`; attachment responses carry their own, tighter policy | FR-057, FR-058, research D-16 |
| Boot assertions | `Protected: true` now verified on **two** file fields | Constitution VII |

---

## 7. Migrations

Four collection migrations and **no** vocabulary amendment, in this order. Every one has a real
`down` — `migrations.Register`'s signature requires both functions (VERIFIED-SOURCE-FACTS FACT 8).

| # | File | `up` | `down` |
|---|---|---|---|
| 1 | `<ts>_catalog_lab_tests.go` | create `catalog_lab_tests`, its three indexes, all five rules `nil`; then **idempotent upsert** of the embedded `assets/catalog/lab-tests.json` extract keyed on `loinc_code` | delete the collection |
| 2 | `<ts>_lab_results.go` | create `lab_results` with every field in §3, its four indexes, all five rules `nil` | delete the collection |
| 3 | `<ts>_lab_components.go` | create `lab_components` with every field in §4, its three indexes, `CascadeDelete` on `lab_result`, all five rules `nil` | delete the collection |
| 4 | `<ts>_attachments.go` | create `attachments` with every field in §5, its three indexes, `file` with **`Protected: true`**, all five rules `nil` | delete the collection **and its blobs** — the `down` explicitly calls the filesystem cleanup, and the file documents that this direction is destructive |

**Ordering constraints.** 1 must precede 2 and 3 (`catalog_test` relations). 2 must precede 3
(`lab_result` relation). 4 depends on nothing in this phase but must follow phase 003, because
`owner_kind`'s value set is the fifteen registered kinds.

**There is no fifth, vocabulary migration.** An earlier draft had one adding `lab_result` to
`audit_events.target_kind`. Phase 001 declares the complete vocabulary, so the value is already
there and the migration would have been a no-op that reads as coverage. What survives is the
assertion: the shared vocabulary test is extended to assert the **complete** expected set after
this phase — twenty-one actions, twenty-seven target kinds — so a value this phase writes but no
migration declared is a red test, not a `SelectField` validation failure on a live instance
(ANALYSIS C1).

**Seeding is a migration, not `medigo seed`** (research D-11), so a production instance that has
never been seeded still has the catalogue. `medigo seed` adds *patient data* only: lab results,
components and documents for the demo patient, plus a second patient deliberately left empty so
the Playwright gate exercises three empty states.

**Migration tests** (one file per migration): the collection exists with every declared field and
type; all five API rules are `nil`; every declared index exists; `attachments.file.Protected` is
`true`; `down` leaves no residue; and — for migration 1 — running `up` twice produces the same row
count.

---

## 8. Relationship map

```
users ──owner──▶ patients ──┬──▶ lab_results ──┬──▶ lab_components   (cascade, cascade)
                            │        │         │
                            │        │         └──▶ catalog_lab_tests (optional, per component)
                            │        │
                            │        ├──▶ catalog_lab_tests   (optional, per result)
                            │        ├──▶ practitioners, facilities  (optional)
                            │        ├──▶ conditions, medications, procedures   (multi, owned here)
                            │        └──◀── encounters.lab_results, treatments.lab_results (multi, owned there)
                            │
                            └──▶ attachments ──(owner_kind + owner_id, no FK)──▶ any of the 15 kinds
                                     │
                                     └──uploaded_by──▶ users

audit_events ──patient──▶ patients ;  ──target_id──▶ (opaque, any kind incl. lab_result, attachment)
search_index ──patient──▶ patients ;  holds lab_result rows, no attachment rows
```

**Cascade behaviour, and where it is tested**

| Delete | Consequence | Test |
|---|---|---|
| a component (by omission from the set) | permanent | `components_test.go` |
| a lab result | components purged; attachments trashed; links cleaned; index row removed | `labresult_delete_test.go`, FR-015/FR-067/SC-013 |
| a linked condition / encounter / medication / procedure / treatment | the reference disappears; the lab result survives intact | `links_test.go`, FR-047, US5 scenarios 5 and 6 |
| a patient | lab results → components; attachments **including trashed ones** destroyed outright, blobs and all | `cascade_test.go`, FR-068, SC-013 |
| an account | its patients, and everything above | `cascade_test.go`, FR-068 |
| a catalogue entry | **cannot happen** — no delete path exists | `catalog_readonly_test.go`, FR-037 |
