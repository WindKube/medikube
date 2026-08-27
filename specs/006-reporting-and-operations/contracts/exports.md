# Contract: producing and taking away (operations 76–79, 91)

Conventions, status codes, the actor matrix and the audit rules are in [README.md](./README.md).

**One resource for both** a produced report document and a portable archive: one collection, one
queue, one worker, so FR-045's "at most one at a time on an instance" is a property of the design
rather than a lock ([D-05](../research.md#d-05)). They are distinguished by `kind` and presented on
different pages ([D-53](../research.md#d-53)).

**Operation 91 is new relative to SHARED-DESIGN §2.3** and is recorded in
[plan.md's Deviations](../plan.md#deviations-from-the-shared-design-contract).

## DTOs

```go
// api.Job — one request to produce something. The same shape for a report and an export.
type Job struct {
    ID          string  `json:"id"`
    Kind        string  `json:"kind"`                    // "report" | "data_export"
    Status      string  `json:"status"`                  // queued|running|succeeded|failed|cancelled|expired
    Stage       *string `json:"stage"`                   // bounded label while running; null otherwise
    Progress    int     `json:"progress"`                // 0..100
    Position    *int    `json:"position"`                // 1-based, present only while queued (FR-045)
    Patient     *string `json:"patient"`                 // opaque id; null for a whole-account export
    Template    *string `json:"template"`                // opaque id; null when built ad hoc
    Criteria    JobCriteria `json:"criteria"`            // the SNAPSHOT taken at request time
    Format      string  `json:"format"`                  // "pdf" | "zip"
    RecordCount *int    `json:"record_count"`            // set on success
    Bytes       *int64  `json:"bytes"`                   // set on success (FR-044)
    ErrorCode   *string `json:"error_code"`              // a bounded token, never a message (FR-050)
    ExpiresAt   *string `json:"expires_at"`              // RFC3339 UTC; null until it succeeds
    RetentionDays int   `json:"retention_days"`          // the window in force, stated (FR-053)
    RequestedAt string  `json:"requested_at"`
    StartedAt   *string `json:"started_at"`
    FinishedAt  *string `json:"finished_at"`
    Downloadable bool   `json:"downloadable"`            // succeeded AND not expired
}
```

`Job` carries **no** file name, **no** storage path, **no** error message and **no** patient name.
`error_code` is a token from the bounded set in
[data-model.md §2](../data-model.md#2-export_jobs--new); the interface maps it to a sentence, and the
sentence names no storage location (FR-050, FR-118).

---

## 76. `POST /api/v1/exports` — ask for it

`operationId: createExport`

**Request — a report**

```json
{ "kind": "report",
  "patient": "rec1234567890abc",
  "template": "rec0000000000tpl",
  "criteria": { "kinds": ["medication","lab_result"], "from": "2025-08-27", "to": "2026-08-27",
                "tags": ["rec0000000000tag"], "statuses": ["active"] },
  "charts": [ { "source": "vitals", "metric": "systolic_mmhg", "unit": "mmHg" } ],
  "settings": { "sort": "date_desc", "group": "kind", "include_header": true, "include_photo": false } }
```

`template` and the inline `criteria`/`charts`/`settings` are alternatives, with one documented
overlap: when `template` is present, any inline `from`/`to` **overrides the template's range for this
production only** and the saved definition is unchanged (FR-033, US3 AS-12). Anything else supplied
alongside `template` is `422`.

**Request — a portable export**

```json
{ "kind": "data_export",
  "patients": ["rec1234567890abc"],
  "kinds": ["medication","lab_result"],
  "from": "2020-01-01", "to": "2026-08-27",
  "options": { "include_csv": true, "include_documents": true } }
```

`patients` absent means **every person the account can reach**; `kinds` absent means every kind;
the date range is optional; `include_documents: false` produces a correspondingly smaller archive
whose manifest **states that documents were excluded** (FR-036, FR-006 of US2, US2 AS-5, AS-6).

**Response** `202 Accepted` + `Location: /api/v1/exports/{id}`, body `api.Job` with
`status: "queued"` and `position`. **The requester is never held waiting** and may navigate away
(FR-005, FR-044, SC-003: acknowledged in under 2 seconds).

**Validation, before anything is queued** — a refusal must not cost a queue slot:

| Code | When | Requirement |
|---|---|---|
| `404 not_found` | any named person is unreachable, or does not exist | FR-001, FR-011, US1 AS-9 |
| `422 nothing_matched` | the selection resolves to **zero** records; the message says nothing matched and **the selection is left intact** so it can be widened | FR-004, US1 AS-4 |
| `422 too_many_records` | more than `MEDIGO_REPORT_MAX_RECORDS`; the message states the limit and asks for a narrower selection, **before anything is produced** | FR-010, US1 AS-14 |
| `422 too_many_charts` | more charts than `MEDIGO_REPORT_MAX_CHARTS` | FR-023, US4 AS-9 |
| `422 not_enough_readings` | a chart range below the published minimum | FR-019 |
| `409 patient_unreachable` | `template` names a person the account can no longer reach | FR-032, US3 AS-11 |
| `422 validation_failed` | anything else in data-model §1.2–§1.4 or §2.3 | — |

**Queueing** ([D-05](../research.md#d-05)): the row *is* the queue. `position` is
`COUNT(*) WHERE status='queued' AND (created,id) < (mine)` + 1, computed at read time, so a waiting
request always shows a truthful position rather than appearing stalled (FR-045, US2 AS-10).

**Producing the same definition twice yields two independent rows**, each with its own production
moment and its own artifact (FR-015).

**Audit**: one `export` entry — actor, action, `target_kind: export`, opaque job id, patient id,
timestamp. **No criteria, no template name, no counts of anything named** (FR-014, FR-116).

---

## 77. `GET /api/v1/exports` — my requests

`operationId: listExports`

**Query**: `?kind=report|data_export` (optional; `/reports` passes `report` and `/exports` passes
`data_export` — [D-53](../research.md#d-53)), `?status=` (repeatable), `?patient=`, `?limit=`,
`?cursor=`, `?count=true`, `?sort=` ∈ `{-requested_at, requested_at}` (default `-requested_at`).

**Response** `200` — `Page[Job]`.

**Authorization**: rows where `owner = caller`, always. **An administrator sees no other account's
jobs through this or any other route** (FR-013, FR-048).

**Empty**: `200`, `items: []`; the page explains what an export is and offers the first one
(US2 AS-1 for the account holding nothing, FR-125).

**Audit**: none.

---

## 78. `GET /api/v1/exports/{id}` — how is it going

`operationId: getExport`

**Response** `200` — `Job`, carrying `status`, `stage`, `progress`, `position` (while queued),
`bytes` and `record_count` (when finished), and `retention_days`.

This is what the page polls every two seconds while any job is `queued` or `running`, and stops
polling when none is ([D-31](../research.md#d-31)) — there is deliberately **no** SSE stream for job
progress.

**Authorization**: `owner = caller`, else `404`, recorded.

**Audit**: none on success; one `access_denied` on refusal.

---

## 79. `GET /api/v1/exports/{id}/download` — take it

`operationId: downloadExport`

**Response** `200`, streamed from `app.NewFilesystem()` through `fsys.Serve`, with
`Content-Type: application/pdf` or `application/zip` and
`Content-Disposition: attachment; filename="medigo-report-<YYYYMMDDHHMMSS>.pdf"` (or `-export-…zip`). The
filename carries **no** patient name (FR-117).

**Authorization**, in this order, every time — possession of the address grants nothing (FR-013,
FR-048, SC-009):

1. authenticated, else `401`;
2. `owner = caller`, else `404` — **including for an administrator**, whose access to another
   account's document does not exist;
3. `status = succeeded`, else `409 not_downloadable`;
4. `expires_at > now`, else `410 artifact_expired` with a plain statement of the window that applied
   and an offer to produce it again — **not** an error suggesting something went wrong
   (FR-047, US8 AS-2).

**Audit**: one `export_download` entry on success; one `access_denied` on every refusal, whether the
address was guessed or copied from somewhere it should not have been (FR-014, US1 AS-12, SC-009).

**Ranged requests** are supported by `fsys.Serve`; every range request is authorized identically,
because authorization is per request and not per artifact.

---

## 91. `POST /api/v1/exports/{id}/cancel` — stop it

`operationId: cancelExport`

**New relative to SHARED-DESIGN §2.3** ([D-41](../research.md#d-41)). `DELETE` was rejected as the
verb: the row survives cancellation and is still listed as `cancelled`, so a `DELETE` that leaves the
resource in place would be a lie.

**Request**: empty body.

**Response** `200` — `Job` with `status: "cancelled"`, `artifact` absent, `progress` frozen where it
stopped.

**Semantics** — cooperative, never a kill:

- `queued` → a conditional write `WHERE id=:id AND status='queued'`. Zero rows affected means the
  worker took it first, and the running path applies.
- `running` → `cancel_requested = true`. The worker checks `ctx.Err()` and the flag between records
  and at every progress update, abandons the scratch file under `.pb_temp_to_delete`, and writes the
  terminal row. **Nothing partial is ever downloadable** (FR-046, US2 AS-11).
- already terminal → `409 not_cancellable`, naming the state it is actually in.

**Authorization**: `owner = caller`, else `404`, recorded.

**Audit**: one `job_cancelled` entry naming the actor and the opaque job id (FR-046, FR-116).

---

## What the worker does, and what the contract promises about it

Not an operation, but the behaviour these operations describe, so it belongs in the contract:

| Guarantee | Mechanism | Requirement |
|---|---|---|
| authorization is resolved **when production starts**, not when it was requested | the worker reloads the owner, rebuilds the actor and re-authorizes every person in scope at dequeue | FR-011, US2 AS-7, [D-09](../research.md#d-09) |
| a person dropped between request and run is **absent from the result and named in the manifest as withdrawn** | the manifest's `withdrawn[]` array | US2 AS-7 |
| a restart never leaves a job reporting itself as running | `Runner.Reconcile` at `OnServe`, before the worker starts: every `running` row becomes `failed` with `error_code: "interrupted"`, offered for retry | FR-049, US2 AS-9, [D-06](../research.md#d-06) |
| a backup taken mid-production captures a finished artifact or none, never a half-written one | the scratch file lives in `.pb_temp_to_delete`, which `core/backup_create.go:83-89` excludes from every backup | edge case, [D-07](../research.md#d-07) |
| nothing partial is downloadable after any failure | `artifact` is written only in the success transition | FR-046, FR-049 |
| storage that cannot accept the archive fails with a reason naming **no storage location** | `error_code: "storage_full"`, mapped to a sentence; the failure is visible to the account holder **and** on the operator overview | US2 AS-12, FR-050, FR-118, FR-085 |
| a disabled owner's running job stops | `error_code: "owner_unavailable"` at dequeue; the artifact is not produced | edge case |
| output streams within bounded memory | `jsontext.Encoder` per record, `archive/zip` onto the scratch file, documents copied with `io.Copy` — never assembled first | FR-123, SC-005 (≤ 256 MiB) |

## The archive itself

The layout is [D-29](../research.md#d-29), published as `docs/export-format-v1.md`:

```
manifest.json                       required; the map of everything else
data/patients.json
data/<kind>.json                    one file per exported kind
data/tags.json
data/report_templates.json          the account's saved reports (FR-035, US3 AS-13)
data/preferences.json
data/audit_events.json              only entries concerning the exported people
tables/<kind>.csv                   present only when CSV was requested
documents/<kind>/<record_id>/<attachment_id>__<original name>
README.txt                          points at docs/export-format-v1.md and states the version
```

**`data/report_templates.json` carries every saved report the account owns**, each as the same
`api.ReportTemplate` object operation 73 serves, minus `resolved` — name, description, person, kinds,
range, tags, charts, presentation settings — and therefore **the criteria, never a resolved record
list** (FR-026):
what leaves the instance is the question, the same thing that was stored. An account with no saved
reports gets `[]`, never `null` and never a missing key, so "I have none" is provable. Another
account's saved report appears nowhere. This is FR-035 and US3 AS-13, tested by T118a and read back
by the round-trip of T118b. Saved reports are **not** narrowed by `patients`, `kinds` or the date
range: they belong to the account, not to a person, and a partial export still carries all of them.

A build gate asserts that every key the document describes is produced and every key produced is
described (FR-039). Its manifest carries the format version, the moment of production, the account, the people,
the kinds and their counts, whether documents were included, the mapping from every archive path to
its attachment and record, the people withdrawn between request and production, and the meaning of
every other file present (FR-038). **Nothing secret can be in it**, because every object is produced
by the same DTO encoder the API uses and no DTO has a password, a token or an operator setting to
give ([D-30](../research.md#d-30), FR-043, SC-006).

**Reading an archive back into an instance is not offered** (FR-052). The supported route back is a
restore from an instance archive, and the handbook says so plainly.
