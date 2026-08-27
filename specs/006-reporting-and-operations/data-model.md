# Data Model: Reporting and Operations (phase 006)

**Feature**: `006-reporting-and-operations` | **Date**: 2026-08-27

**Binding inputs**: the constitution v1.3.0 → [`VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) → [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) §1 →
[research.md](./research.md) → [spec.md](./spec.md).

This phase adds **2 collections** (taking the instance to the shared design contract's **30**, §1.6) and
amends **3**. It adds **one** file field, **no** `deleted_at`, and **no** collection that exists only
to record that something happened.

---

## 0. The three things to read before writing a query

1. **PocketBase has no NULL.** `core/field_date.go:110` declares every `DateField` column as
   `TEXT DEFAULT '' NOT NULL`, and `core/field_relation.go:161` does the same for a single relation.
   Every predicate in this phase is written against `= ''`, never `IS NULL` — the same gate phase 005
   introduced (`task lint:isnull`) covers this phase's store packages.
2. **A non-cascading relation is *emptied*, not deleted, when its target goes**
   (`core/record_model.go:1618-1626`). That is the whole mechanism behind
   `report_templates.patient` surviving the deletion of a person ([D-45](./research.md#d-45)).
3. **Deleting a record deletes its file field's blobs.** That is what makes an expired artifact's
   bytes actually stop being stored ([D-42](./research.md#d-42)) and what makes account deletion
   destroy archives for free (FR-051, FR-063).

---

## 1. `report_templates` — new

A **saved report**: the question, never the answer. SHARED-DESIGN §1.2's field list, with the patient
lifted out of the JSON blob and a uniqueness index added ([D-45](./research.md#d-45)).

| Field | Type | Req | Constraints / notes |
|---|---|---|---|
| `owner` | relation → `users` | **yes** | MaxSelect 1, **CascadeDelete: true** (FR-051) |
| `patient` | relation → `patients` | no | MaxSelect 1, **CascadeDelete: false** — emptied when the person is deleted (FR-032). Empty is a legal, renderable state |
| `name` | text | **yes** | 1..120 |
| `description` | text | no | ≤ 500 |
| `criteria` | json | **yes** | validated `report.Criteria`, §1.2 below |
| `charts` | json | no | validated `[]report.ChartSelection`, §1.3 below; `[]` when absent, never `null` |
| `settings` | json | **yes** | validated `report.Settings`, §1.4 below |

### 1.1 Indexes

```
CREATE UNIQUE INDEX idx_report_templates_name  ON report_templates (owner, LOWER(name))
CREATE INDEX        idx_report_templates_owner ON report_templates (owner, updated DESC, id DESC)
CREATE INDEX        idx_report_templates_pat   ON report_templates (patient)
```

`idx_report_templates_name` is FR-030: a duplicate name, ignoring capitalisation, is
`409 duplicate_name` and **nothing is overwritten**. `idx_report_templates_owner` is the keyset
pagination index for the saved-report list. `idx_report_templates_pat` answers "does this account
have saved reports about this person" for the delete-a-person confirmation.

### 1.2 `criteria` — the validated struct

```go
package report // internal/domain/report

type Criteria struct {
    Kinds    []kind.Kind        `json:"kinds"`              // 1..15, unique, all registered
    From     *Date              `json:"from,omitempty"`     // YYYY-MM-DD, inclusive
    To       *Date              `json:"to,omitempty"`       // YYYY-MM-DD, inclusive
    TagIDs   []string           `json:"tags"`               // ids only; never tag names (VII)
    Statuses []string           `json:"statuses"`           // per-kind lifecycle values
}
```

| Rule | Error | Requirement |
|---|---|---|
| `len(Kinds) >= 1`, every value registered in `records.Registry`, no duplicates | `422 validation_failed` | FR-002 |
| `From <= To` when both present | `422 validation_failed` | FR-002 |
| every `TagIDs[i]` resolves and belongs to the **patient's owner** | `422 unknown_tag`, identical for "not yours" and "does not exist" | FR-002, phase 005 D-22 |
| every `Statuses[i]` is a legal value of at least one selected kind | `422 validation_failed` naming the value | FR-002 |
| `Kinds`, `TagIDs`, `Statuses` marshal as `[]`, never `null` | — | Go 1.27 `encoding/json/v2` |

### 1.3 `charts` — the validated struct

```go
type ChartSelection struct {
    Source       ChartSource `json:"source"`                 // "vitals" | "lab"
    Metric       string      `json:"metric"`                 // vitals column name, e.g. "systolic_mmhg"
    CanonicalName string     `json:"canonical_name,omitempty"` // lab component canonical name
    Unit         string      `json:"unit"`                   // REQUIRED; never converted (FR-018)
    From, To     *Date       `json:"from,omitempty"`         // the chart's own range (FR-019)
}
```

| Rule | Error | Requirement |
|---|---|---|
| `Source ∈ {vitals, lab}` | `422 validation_failed` | FR-016 |
| `Source == vitals` → `Metric` is one of the `vitals` numeric columns and `CanonicalName` is empty | `422 validation_failed` | FR-016 |
| `Source == lab` → `CanonicalName` non-empty and `Metric` empty | `422 validation_failed` | FR-016 |
| `Unit` non-empty, and matches a unit actually recorded for that series | `422 unknown_unit` | FR-018 |
| `len(charts) <= MEDIGO_REPORT_MAX_CHARTS` | `422 too_many_charts`, message states the limit | FR-023 |
| the series resolves to `>= MEDIGO_REPORT_MIN_CHART_POINTS` readings in `[From, To]` | `422 not_enough_readings`, message states how many it has and how many it needs | FR-019 |
| **no conversion is performed under any circumstance** | — | FR-018; a `Convert` function does not exist, asserted by a grep test |

### 1.4 `settings` — the validated struct

```go
type Settings struct {
    Sort            SortOrder `json:"sort"`             // "date_desc" | "date_asc" | "name_asc"
    Group           Grouping  `json:"group"`            // "none" | "kind" | "year"
    IncludeHeader   bool      `json:"include_header"`   // the identifying header (FR-002)
    IncludePhoto    bool      `json:"include_photo"`    // the photograph (FR-002)
}
```

Defaults: `date_desc`, `none`, `true`, `false`. `IncludeHeader = false` does **not** make the document
anonymous: it still carries the opaque patient reference on every page (US1 AS-8, FR-006).

### 1.5 Validation rules on the record as a whole

| Rule | Error | Requirement |
|---|---|---|
| `name` 1..120, trimmed, non-blank | `422 validation_failed` | FR-025 |
| `(owner, LOWER(name))` unique | `409 duplicate_name` naming the conflict | FR-030 |
| `patient` — when set, the owner must be able to reach it **at write time** | `404 not_found` | FR-001, FR-031 |
| `patient` empty → the template is readable and editable, and **producing is refused** with `409 patient_unreachable` | — | FR-032, US3 AS-11 |
| `owner` is never settable from a DTO | — | SHARED-DESIGN §5.5 |

---

## 2. `export_jobs` — new

One row per **request to produce something** — a report document or a portable archive. One
collection, one queue, one worker ([D-05](./research.md#d-05), [D-53](./research.md#d-53)).

| Field | Type | Req | Constraints / notes |
|---|---|---|---|
| `owner` | relation → `users` | **yes** | MaxSelect 1, **CascadeDelete: true** (FR-051, FR-063) |
| `kind` | select | **yes** | `data_export` \| `report` |
| `patient` | relation → `patients` | conditional | MaxSelect 1, CascadeDelete **false**; **required when `kind = report`** (FR-001) |
| `template` | relation → `report_templates` | no | MaxSelect 1, CascadeDelete **false** — deleting a saved report must not affect a produced document (FR-034) |
| `criteria` | json | **yes** | **the snapshot** taken at request time: the resolved `report.Criteria` + `charts` + `settings` for a report, or the `exportjob.Scope` for an export. Written once, never refreshed (Complexity Tracking entry 5) |
| `format` | select | **yes** | `pdf` \| `zip`. `kind=report → pdf`; `kind=data_export → zip` |
| `options` | json | no | validated `exportjob.Options`: `include_csv` (bool), `include_documents` (bool) — the tabular files and the document content of FR-037 and FR-036 |
| `status` | select | **yes** | `queued` \| `running` \| `succeeded` \| `failed` \| `cancelled` \| `expired`. Default `queued` |
| `stage` | select | no | `resolving` \| `records` \| `documents` \| `rendering` \| `packaging` — a **bounded** progress label, never free text (FR-117) |
| `progress` | number | **yes** | 0..100, default 0 |
| `record_count` | number | no | what the job actually produced (FR-003's figure, after the fact) |
| `error_code` | text | no | a **bounded token**, never a message: `interrupted`, `owner_unavailable`, `storage_full`, `too_many_records`, `nothing_matched`, `patient_unreachable`, `render_failed`, `archive_too_large` (FR-050, FR-118) |
| `artifact` | file | no | MaxSelect 1, **`Protected: true`** (constitution VII, no exception), MaxSize `MEDIGO_EXPORT_MAX_BYTES` |
| `bytes` | number | no | the finished size (FR-044) |
| `cancel_requested` | bool | **yes** | default false — the cooperative-cancel flag ([D-41](./research.md#d-41)) |
| `expires_at` | date | no | set on success = `retention.DueAt(finished_at, MEDIGO_RETENTION_EXPORT_DAYS)` |
| `started_at` | date | no | RFC3339 UTC |
| `finished_at` | date | no | RFC3339 UTC |

### 2.1 Indexes

```
CREATE INDEX idx_export_jobs_owner  ON export_jobs (owner, kind, created DESC, id DESC)
CREATE INDEX idx_export_jobs_queue  ON export_jobs (status, created ASC, id ASC)
CREATE INDEX idx_export_jobs_expiry ON export_jobs (status, expires_at)
```

`idx_export_jobs_owner` serves both `/reports` and `/exports` (filtered by `kind`) with keyset paging.
`idx_export_jobs_queue` is what the worker reads — the oldest `queued` row — and what the **position**
count scans: `COUNT(*) WHERE status='queued' AND (created,id) < (mine)`, plus one (FR-045).
`idx_export_jobs_expiry` is what the artifact purge scans.

### 2.2 The state machine

```
                       worker dequeues                    success
      ┌────────┐  ───────────────────────▶  ┌─────────┐ ──────────▶ ┌───────────┐   purge   ┌─────────┐
      │ queued │                            │ running │             │ succeeded │ ────────▶ │ expired │
      └────────┘  ◀── (no edge back) ──     └─────────┘             └───────────┘           └─────────┘
           │                                   │    │
           │ cancel                     cancel │    │ error / restart-reconcile
           ▼                                   ▼    ▼
      ┌───────────┐                       ┌───────────┐        ┌────────┐
      │ cancelled │  ◀────────────────────│ cancelled │        │ failed │
      └───────────┘                       └───────────┘        └────────┘
```

| From | Event | To | Notes |
|---|---|---|---|
| `queued` | worker dequeues | `running` | conditional write; only one worker exists |
| `queued` | cancel | `cancelled` | `UPDATE … WHERE id=:id AND status='queued'`; zero rows means the worker won, so the running path applies ([D-41](./research.md#d-41)) |
| `running` | finished | `succeeded` | `artifact`, `bytes`, `record_count`, `expires_at`, `finished_at` set in one write |
| `running` | error | `failed` | `error_code` set; artifact empty; scratch file abandoned in `.pb_temp_to_delete` |
| `running` | cancel observed | `cancelled` | the worker checks `ctx.Err()` and `cancel_requested` between records and at every progress update |
| `running` | **process restart** | `failed` (`interrupted`) | `Runner.Reconcile` at `OnServe`, before the worker starts ([D-06](./research.md#d-06), FR-049) |
| `succeeded` | retention window closes | `expired` | the purge clears `artifact` and zeroes `bytes` ([D-42](./research.md#d-42), FR-012, FR-047) |

**Terminal states are terminal**: `succeeded → expired` is the only edge out of a terminal state, and
there is **no edge back to `queued`** — re-running is a **new row** built from the old row's snapshot
(FR-015: producing the same definition twice yields two independent documents).

An exhaustive table test enumerates every `(status, event)` pair; the legal edges above succeed and
every other pair returns `*exportjob.TransitionError` carrying the state the row is actually in.

### 2.3 Validation rules

| Rule | Error | Requirement |
|---|---|---|
| `kind = report` → `patient` set and reachable **by the owner at request time and again at dequeue** | `404 not_found` | FR-001, FR-011, [D-09](./research.md#d-09) |
| `kind = report` → resolved record count `>= 1` | `422 nothing_matched`, selection left intact | FR-004 |
| `kind = report` → resolved record count `<= MEDIGO_REPORT_MAX_RECORDS` | `422 too_many_records`, message states the limit | FR-010 |
| `kind = report` → `len(charts) <= MEDIGO_REPORT_MAX_CHARTS` | `422 too_many_charts` | FR-023 |
| `format` matches `kind` | `422 validation_failed` | §2 |
| `progress ∈ [0,100]` | `422` | FR-005 |
| `error_code` ∈ the bounded set | — | FR-050, FR-118 |
| `artifact` is empty unless `status = succeeded` | — | FR-046, FR-049 |
| `owner`, `status`, `progress`, `stage`, `artifact`, `bytes`, `error_code`, `expires_at` are **absent from every request DTO** | — | SHARED-DESIGN §5.5 |

---

## 3. `users` — amended

| Field | Type | Req | Notes |
|---|---|---|---|
| `must_change_password` | bool | **yes** | default `false`. **New in this phase** ([D-20](./research.md#d-20)), a deviation from SHARED-DESIGN §1.2 recorded in plan.md |

Behaviour, all of it FR-093:

- set only by `PATCH /api/v1/admin/users/{id}`, in the same transaction as `RefreshTokenKey()`;
- while `true`, every `/api/v1` route except the password-change route answers
  `403 password_change_required`, and every page redirects to `/settings`'s forced-change form —
  including `/reports`, `/exports` and every operator view;
- cleared in the same transaction that sets the new password;
- both the setting and the clearing are recorded (`admin_user_update`, `password_change`).

`role` (`user` | `admin`) and `disabled_at` already exist (SHARED-DESIGN §1.2). This phase adds their
**behaviour**, not their storage: `disabled_at != ''` refuses sign-in (FR-091,
[D-49](./research.md#d-49)), and `role = admin` is the administrative tier every operator route
requires (FR-076). Neither is settable through registration or any self-service DTO (FR-097) — the
DTOs have no field for them, which is the control.

---

## 4. `audit_events` — amended

One new column, two vocabulary extensions and three indexes.

### 4.0 `affected` — a new column

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `affected` | number | no | integer ≥ 0 | how many rows a run actually touched. Written by `job_succeeded` and `purge`, empty everywhere else |

It is a real column created by this phase's migration, because this phase is the first that needs
one: [D-43](./research.md#d-43)'s job envelope reports `affected` on success and FR-054/FR-060
require a purge to record what it removed. Both were written against a field no migration created
(ANALYSIS C2) — a write to a field a collection does not have is a runtime failure.

A **count** is not content: it names nothing and identifies nobody, and it is the one number that
makes "the retention window actually removed something" answerable without reading the rows.

`job_failed`'s bounded `error_code` needs no column of its own — it is a bounded token and rides
`reason`, the column phase 005 creates ([005 data-model §4.1](../005-sharing-and-collaboration/data-model.md)),
whose closed Go set this phase extends with the phase's failure codes: `interrupted`,
`owner_unavailable`, `storage_full`, `too_many_records`, `artifact_expired`, `archive_unreadable`,
`archive_version_unsupported`, `safety_backup_failed`, `job_in_progress`, `nothing_matched`,
`patient_unreachable`, `render_failed`, `archive_too_large`, and `bad_credentials` for a
`login_failed` ([D-49](./research.md#d-49)). One bounded-token column, one Go source of truth, no
second free-text field in a medical audit trail.

### 4.1 `action` — ten new values

| Value | Written when | Requirement |
|---|---|---|
| `export` *(exists)* | a report or an export is **requested** — the act the person performed | FR-014, FR-116 |
| `export_download` | a produced document or an archive is downloaded | FR-014, FR-048, FR-116 |
| `job_cancelled` | a queued or running request is cancelled | FR-046, FR-116 |
| `job_succeeded` | any scheduled job or production job completes, with an `affected` count | FR-055, FR-058, [D-43](./research.md#d-43) |
| `job_failed` | any scheduled job or production job fails, with a bounded `error_code` | FR-058, FR-085 |
| `audit_export` | the trail is exported as a tabular file | FR-067, FR-075 |
| `admin_user_update` | any administrative change to an account, marked administrative | FR-098 |
| `backup_upload` | an archive taken elsewhere is uploaded | FR-102 |
| `backup_download` | an archive is downloaded — the most sensitive action the instance offers | FR-109 |
| `backup_delete` | an archive is removed | FR-110 |
| `purge` | a retention window actually removed something, with a count | FR-054, FR-060 |

`export`, `backup_create`, `backup_restore`, `admin_session`, `account_delete`, `login`,
`login_failed` and `access_denied` already exist — phase 001's migration declares the shared design
contract's complete vocabulary, and phase 005 adds its five. A **refused** attempt at any operation
in this phase writes `access_denied` with a bounded `reason` (FR-073, FR-076, SC-009, SC-010).

**After this phase: thirty-six actions.**

### 4.2 `target_kind` — one new value

`report_template`. `export` and `backup` already exist — phase 001 declares them with the rest of
the contract's vocabulary — and cover a produced document, an export request and an archive;
`system` covers a scheduled job, whose `target_id` is the **bounded job name**
([D-43](./research.md#d-43)) — never a record id, never content.

**`target_id` and the three kinds that carry a name.** 001's rule is that `target_id` is an opaque
id and never a name, with one bounded exception it states in full: when `target_kind` is `system`,
`backup` or `export` there is no record to point at, so the column carries the **job name or archive
name** instead. This phase is where all three land — `medigo_purge_artifacts` and its siblings (18–29
characters), and `medigo_safety_<YYYYMMDDHHMMSS>_<name>` on `backup_create`, `backup_upload`,
`backup_download`, `backup_delete` and `backup_restore` — **54 at its longest**, composed over a
manual archive `medigo_<YYYYMMDDHHMMSS>.zip`. Timestamps are compact, not RFC3339, precisely so
this fits: spelled RFC3339 the same name is 66 against a `Max 64` column. Op 86 normalises and
bounds an uploaded archive's key to 64 on the same grounds (ANALYSIS N2). They are operator-facing identifiers
the operator already chose — the archive name is the same string its route is addressed by — not
personal data, and 001 sizes the column at **`≤64`** for exactly this (001
[data-model](../001-walking-skeleton/data-model.md) §3, ANALYSIS). An `export` row still points at
the opaque job id; the `export` **kind** is in the exception because an archive addressed by name is
the same shape of thing.

**After this phase: twenty-eight target kinds.** The migration's test asserts both **complete**
sets, not these deltas, so a value this phase writes but no migration declared is a red test rather
than a failed `SelectField` validation in production (ANALYSIS C1).

### 4.3 New indexes

**This phase creates no audit index.** The four it needs already exist, created wide enough on
purpose by the phases that own the columns:

| Index | Created by | Serves |
|---|---|---|
| `idx_audit_occurred` `(occurred_at DESC, id DESC)` | 001 | [D-52](./research.md#d-52)'s keyset first page |
| `idx_audit_actor_time` `(actor, occurred_at DESC, id DESC)` | 001 | narrowing by actor |
| `idx_audit_patient_time` `(patient, occurred_at DESC, id DESC)` | 002 | [D-12](./research.md#d-12)'s scoped reader |
| `idx_audit_target` `(target_kind, target_id, occurred_at DESC)` | 001 | "last ran / last succeeded" |

An earlier draft of this phase created `idx_audit_page`, `idx_audit_patient` and `idx_audit_actor`
beside them and re-created `idx_audit_target` with different columns under the same name — three
redundant b-trees on the highest-write-volume collection, and a `CREATE INDEX` that fails outright
because the name is taken. The tiebreaker columns moved into 001's and 002's migrations instead
(ANALYSIS).

These make [D-52](./research.md#d-52)'s keyset paging and [D-12](./research.md#d-12)'s scoped reader
index-only, which is what the 2-second first-page budget over 1,000,000 rows depends on (SC-016).
The `EXPLAIN QUERY PLAN` assertions of T034 run against these four and are what prove the
phase never silently falls back to a scan.

### 4.4 What an entry still may not contain

Restated because this phase is where the entries become **readable**: actor, action, target kind,
opaque target id, patient id, timestamp, request id, a bounded `reason` token and an `affected`
count. **No `ip`** — phase 001 does not create the column and this phase does not publish one (001
research D-19). **No** name, no diagnosis, no recorded value, no note, no tag name, no file name, no
template name, no archive path, no query string (FR-068, FR-114). The reader's DTO has no field capable of carrying any of them, and the
handler performs no lookup against any other collection ([D-11](./research.md#d-11)).

---

## 5. Domain types (no PocketBase, no HTTP, no templ)

| Package | Types | Notes |
|---|---|---|
| `internal/domain/report` | `Definition`, `Criteria`, `ChartSelection`, `ChartSource`, `Settings`, `SortOrder`, `Grouping`, `Counts`, `Document`, `Section`, `Chart`, `Series` | `Document` is what the renderer draws: already resolved, already authorized, already ordered. It carries no repository and no context |
| `internal/domain/exportjob` | `Job`, `Status`, `Stage`, `Scope`, `Options`, `ErrorCode`, `*TransitionError` | `Transition(from, event) (Status, error)` is the only state machine |
| `internal/domain/retention` | `DueAt`, `Expired`, `DaysRemaining` | [D-50](./research.md#d-50); whole days from the recorded moment, clock-jump safe |
| `internal/domain/adminuser` | `CanChangeRole`, `CanDisable`, `LastEnabledAdminGuard` | [D-19](./research.md#d-19); pure functions, exhaustive table tests, so `medigo seed` and a future CLI cannot lock the instance out |
| `internal/domain/audit` | `Query`, `Action`, `TargetKind`, `ActorKind` | the reader's typed narrowing; no free-text field exists on it |

Every one of these implements `MarshalZerologObject` emitting **only** ids, enum values and counts.
`Definition` and `Criteria` in particular must never emit a tag id list as names, a patient name or a
template name (FR-117).

---

## 6. Migrations

Five, each with a real `down` (VERIFIED-SOURCE-FACTS FACT 8 makes both functions structural).

### `1757xxx100_report_templates.go`

Creates the collection of §1 with all five API rules `nil` and the three indexes of §1.1.
`down` drops it. **Reversible.**

### `1757xxx200_export_jobs.go`

Creates the collection of §2 with all five API rules `nil`, the three indexes of §2.1, and
`artifact` as a `FileField` with `Protected: true`. `down` drops it — **and the migration file
documents that dropping it destroys any artifact still stored**, which is the reversibility note
Principle IX requires. **Reversible with a documented data loss.**

### `1757xxx300_users_must_change_password.go`

Adds the bool with default `false`. `down` removes the field. **Reversible.**

### `1757xxx400_audit_vocab_ops.go`

Adds the `affected` column of §4.0, extends `audit_events.action` by the ten values of §4.1 and
`target_kind` by `report_template`. `down` removes all three **and the file documents that rows
already carrying a removed value would fail validation on their next write** — they are never
rewritten, so the down is safe in practice and the caveat is recorded rather than hidden. Its test
asserts the **complete** vocabulary after this phase, thirty-six actions and twenty-eight target
kinds, never a delta. **Reversible with a documented caveat.**

### ~~`1757xxx500_audit_page_indexes.go`~~ — **not created**

There is no audit-index migration in this phase. §4.3 says why: the four indexes the reader
needs are created wide enough on purpose by 001 and 002, and re-creating them here would collide
by name on `idx_audit_target` and fail `CREATE INDEX` at first boot. T034 asserts the four with
`EXPLAIN QUERY PLAN` instead of migrating anything.

### Boot assertions extended (`internal/store/migrations/assertions.go`)

1. All five API rules `nil` on `report_templates` and `export_jobs` — MediGo refuses to start
   otherwise.
2. The file-field assertion now names **three** fields — `patients.photo`, `attachments.file`,
   `export_jobs.artifact` — and every one must be `Protected: true`. A fourth file field anywhere on
   the instance fails the assertion by count, so it cannot be added unprotected
   ([D-08](./research.md#d-08)).
3. No collection carries a `deleted_at` except `attachments` (constitution VII: soft delete is files
   only).

---

## 7. Configuration (`internal/config`)

Eleven values, all `MEDIGO_`-prefixed, all with published defaults, all validated at boot
([D-10](./research.md#d-10)), all **rendered on `/admin`** so an operator never reads source to learn
one (FR-087), and all bounded so the instance refuses to start on a value it cannot honour
([D-46](./research.md#d-46), FR-113).

| Variable | Default | Bound | Governs |
|---|---|---|---|
| `MEDIGO_REPORT_MAX_RECORDS` | `5000` | 1..100000 | FR-010 |
| `MEDIGO_REPORT_MAX_CHARTS` | `12` | 1..50 | FR-023 |
| `MEDIGO_REPORT_MIN_CHART_POINTS` | `3` | 2..100 | FR-017 |
| `MEDIGO_REPORT_MAX_CHART_POINTS` | `200` | ≥ MIN..5000 | [D-03](./research.md#d-03) |
| `MEDIGO_REPORT_EXTRA_FONT_DIR` | *(empty)* | empty or readable dir | [D-04](./research.md#d-04) |
| `MEDIGO_EXPORT_MAX_BYTES` | `10 GiB` | 1 MiB..1 TiB | FR-050's `archive_too_large` |
| `MEDIGO_RETENTION_EXPORT_DAYS` | `7` *(exists)* | 1..3650 | FR-012 **and** FR-047 — one window for both |
| `MEDIGO_RETENTION_AUDIT_DAYS` | `730` *(exists)* | 1..3650 | FR-074 |
| `MEDIGO_RETENTION_TRASH_DAYS` | `30` *(exists)* | 1..3650 | phase 004's window; **stated** here, not enforced here |
| `MEDIGO_BACKUP_WARN_AFTER` | `168h` | 1h..8760h | FR-082 |
| `MEDIGO_BACKUP_KEEP` | `14` | 1..365 | written into `Settings().Backups.CronMaxKeep` (FR-101) |
| `MEDIGO_STATE_DIR` | `/data/medigo_state` | writable, **outside `DataDir`** | [D-23](./research.md#d-23) |

---

## 8. Scheduled work

Three new jobs, all registered through the envelope of [D-43](./research.md#d-43), all writing
exactly one `job_succeeded` or `job_failed` entry per run, none retrying in a loop.

A job has no HTTP request and `audit_events.request_id` is `Required`, so the envelope mints one
**`run_id`** per run onto the context it hands the job body. Its own `job_succeeded`/`job_failed`
row carries it, and so does every row the body writes — this phase's `purge` rows, phase 004's
`delete` rows, phase 005's `share_expire` and `invite_expire` — along with that run's log lines
([D-43](./research.md#d-43), 001 [data-model](../001-walking-skeleton/data-model.md) §3). The job
name in `target_id` is likewise legal only because 001 sizes that column at `≤64` for exactly this
bounded exception.

| Job name (the bounded `target_id`) | Cadence | Does |
|---|---|---|
| `medigo_purge_artifacts` | daily 03:10 | `succeeded` rows past `expires_at` → clear `artifact`, zero `bytes`, `status = expired` (FR-012, FR-047) |
| `medigo_purge_audit` | daily 03:20 | delete `audit_events` older than `MEDIGO_RETENTION_AUDIT_DAYS` whole days (FR-074) |
| `medigo_storage_refresh` | every 15 min + once at boot | recompute database bytes and document bytes into the in-memory gauge with its `computed_at` ([D-16](./research.md#d-16)) |

Wrapped, not rewritten: phase 004's `medigo_attachment_maintenance` (its trash purge and orphan sweep), phase 005's
share/invitation tidy, and PocketBase's own auto-backup (via `OnBackupCreate`, which is how a failed
scheduled backup reaches the overview and the trail — FR-101, US7 AS-3).

---

## 9. Seed fixtures (`medigo seed`) — deterministic, and shaped by what the tests need

| Fixture | Why it exists |
|---|---|
| `owner@medigo.local` with 3 saved reports: one over a populated person, one over a person with **no** records, one whose `patient` has been **emptied** | US3 AS-1/AS-3/AS-11, FR-032 |
| one `succeeded` job with a downloadable artifact, one `expired` job, one `failed` (`interrupted`) job, one `cancelled` job, one `queued` job behind another | US1 AS-11, US2 AS-8/AS-9/AS-10/AS-11, US8 AS-1/AS-2 |
| `empty@medigo.local` — an account with no people, no records, no saved reports, no jobs | every empty state, and the second Playwright pass (FR-125) |
| `admin@medigo.local` (`role = admin`) and `admin2@medigo.local` | the last-admin refusals of FR-096, and the "a second administrator changed my tier" edge case |
| `disabled@medigo.local` (`disabled_at` set) | FR-091, [D-49](./research.md#d-49) |
| `mustchange@medigo.local` (`must_change_password = true`) | FR-093 |
| a person with 12 readings of one lab component in `mmol/L`, 3 of the **same** component in `mg/dL`, and 1 reading of a second component | US4's independent test verbatim — enough, not enough, and two units |
| a person whose name, a tag and a document description carry Arabic, Hebrew, CJK and `<script>` text | FR-006's unrenderable-character statement, and the "never interpreted as markup" edge case |
| two archives in `pb_data/backups`, one with `medigo.json` and one without | US7 AS-5/AS-9, [D-25](./research.md#d-25) |
| a trail with entries of every action, including `system` and `superuser` actors and one refusal | US6 AS-2/AS-10/AS-11, FR-072 |

`medigo seed --print-ids` prints the ids the quickstart and the Playwright specs substitute.

## 10. Scale fixture (`internal/testsupport/scale`)

Generated once into a throwaway test app, behind the `scale` build tag
([D-33](./research.md#d-33)): **10,000 records** across the kinds, **2,000 documents**, **3 people**,
**1,000,000 activity entries**, **500 readings** of one lab component, **200 export requests**,
**500 accounts**, **60 archives**. Every budget in plan.md's Performance Goals is asserted against it,
and a regression beyond one fails the build (FR-131, SC-023).
