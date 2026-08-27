# Contract: saved reports (operations 71–75)

Conventions, status codes, the actor matrix and the audit rules are in [README.md](./README.md).

A saved report stores **the question, not the answer** — criteria, never a frozen list of record ids
(FR-026). That is the defect in the system being reimagined that this resource exists to not repeat.

## DTOs

```go
// api.ReportTemplate — the full representation. The list representation is the same shape minus
// `charts` and `settings`, which are not useful in a row.
type ReportTemplate struct {
    ID          string              `json:"id"`
    Name        string              `json:"name"`
    Description string              `json:"description,omitempty"`
    Patient     *PatientRef         `json:"patient"`            // null when the person is gone (FR-032)
    Criteria    ReportCriteria      `json:"criteria"`
    Charts      []ChartSelection    `json:"charts"`             // [] never null
    Settings    ReportSettings      `json:"settings"`
    Resolved    *ReportSummaryBrief `json:"resolved,omitempty"` // present on GET {id} only (FR-027)
    Unreachable bool                `json:"unreachable"`        // the person cannot be reached right now
    CreatedAt   string              `json:"created_at"`
    UpdatedAt   string              `json:"updated_at"`         // the ETag source
}

type ReportCriteria struct {
    Kinds    []string `json:"kinds"`
    From     *string  `json:"from,omitempty"`      // YYYY-MM-DD
    To       *string  `json:"to,omitempty"`
    Tags     []TagRef `json:"tags"`                // id + name; the name is the OWNER's own label
    Statuses []string `json:"statuses"`
}

type ReportSettings struct {
    Sort          string `json:"sort"`             // "date_desc" | "date_asc" | "name_asc"
    Group         string `json:"group"`            // "none" | "kind" | "year"
    IncludeHeader bool   `json:"include_header"`
    IncludePhoto  bool   `json:"include_photo"`
}

type ReportSummaryBrief struct {
    Total int         `json:"total"`
    Kinds []KindCount `json:"kinds"`
    At    string      `json:"at"`                  // when it was resolved — RFC3339 UTC
}

type ReportTemplateCreate struct { /* name, description, patient, criteria, charts, settings */ }
type ReportTemplatePatch  struct { /* all of the above as pointers; owner is absent by construction */ }
```

`owner` appears in **no** request DTO and in **no** response DTO: a saved report is private to the
account that owns it (FR-031), so there is no context in which naming the owner is useful, and there
is no field through which one could be set.

**The portable export writes this same DTO** into `data/report_templates.json`, with `resolved`
absent — the archive carries the question, never a count taken at production time
([exports.md](./exports.md#the-archive-itself), FR-035, US3 AS-13).

---

## 71. `GET /api/v1/report-templates` — my saved reports

`operationId: listReportTemplates`

**Query**: `?patient={id}` (optional), `?limit=`, `?cursor=`, `?count=true`,
`?sort=` ∈ `{-updated_at, updated_at, name}` (default `-updated_at`).

**Response** `200` — `Page[ReportTemplate]` **without** `resolved`: resolving every row's criteria to
a count would make the list O(rows × records), and FR-027 asks for the count when a saved report is
**opened**, not when it is listed.

**Authorization**: rows where `owner = caller`. There is no route by which one account lists
another's, and `?owner=` does not exist.

**Empty**: `200`, `items: []`; the page renders the explanation and the create action inside
`region[name="Reports"]` (US3 AS-1).

**Audit**: none.

---

## 72. `POST /api/v1/report-templates` — save this question

`operationId: createReportTemplate`

**Request**

```json
{ "name": "Rheumatology, last 12 months",
  "description": "What Dr Nowak asks for every three months.",
  "patient": "rec1234567890abc",
  "criteria": { "kinds": ["medication","condition","allergy","lab_result"],
                "from": "2025-08-27", "to": "2026-08-27",
                "tags": ["rec0000000000tag"], "statuses": ["active","chronic"] },
  "charts": [ { "source": "lab", "canonical_name": "hba1c", "unit": "%", "from": "2024-08-27" } ],
  "settings": { "sort": "date_desc", "group": "kind",
                "include_header": true, "include_photo": false } }
```

Field rules are [data-model.md §1](../data-model.md#1-report_templates--new), in full, and are not
duplicated here.

**Response** `201` + `Location: /api/v1/report-templates/{id}`, body `ReportTemplate`.

**Authorization**: `PermView` on `patient` at write time. A person the caller cannot reach is `404`,
byte-identical to a person who does not exist (FR-001), and the attempt is recorded.

**Errors**

| Code | When | Requirement |
|---|---|---|
| `404 not_found` | `patient` unreachable or absent | FR-001 |
| `409 duplicate_name` | a saved report of that name exists for this account, ignoring capitalisation; **nothing is overwritten**, and the message names the conflict | FR-030, US3 AS-8 |
| `422 validation_failed` | any rule of data-model §1.2–§1.5 | FR-002 |
| `422 too_many_charts` | more charts than `MEDIGO_REPORT_MAX_CHARTS`; the message states the limit | FR-023, US4 AS-9 |
| `422 unknown_unit` | a chart unit that was never recorded for that series | FR-018 |
| `422 not_enough_readings` | a chart range resolving to fewer than the published minimum; the message states how many it has and how many it needs | FR-019, US4 AS-5 |
| `422 unknown_tag` | a tag id outside the patient owner's set | phase 005 D-22 |

**Audit**: `create`, target `report_template`, opaque id — **never the name, never the criteria**
(FR-114).

---

## 73. `GET /api/v1/report-templates/{id}` — open it

`operationId: getReportTemplate`

**Response** `200` — `ReportTemplate` **with** `resolved`: how many records the criteria resolve to
**right now**, per kind and in total, with the moment it was resolved (FR-027, US3 AS-3). `resolved`
comes from the same `report.Selection.Counts` op 69 uses, so a saved report and the builder can never
report different numbers for the same question ([D-44](../research.md#d-44)).

`ETag` is set from `updated_at`.

**Three states the response distinguishes**, all of them `200`:

| State | Body | Requirement |
|---|---|---|
| the criteria resolve to records | `resolved.total > 0`, `unreachable: false` | FR-027 |
| the criteria resolve to nothing | `resolved.total = 0`, `unreachable: false` — it opens, reports zero, stays editable, and is **never** silently produced as an empty document | US3 AS-3 (its zero case), FR-004's edge case |
| the person is gone or unreachable | `patient: null`, `unreachable: true`, `resolved: null`, and a plain statement; the row remains **fully editable** so another person can be chosen | FR-032, US3 AS-11 |

**A record the saved report previously matched having been deleted is not an error** — the criteria
simply resolve to one fewer record (US3 AS-5, FR-026).

**Authorization**: `owner = caller`, else `404` — indistinguishable from a saved report that does not
exist (FR-031, US3 AS-10), and recorded.

**Audit**: none on success; one `access_denied` on refusal.

---

## 74. `PATCH /api/v1/report-templates/{id}` — change it

`operationId: updateReportTemplate`

**`If-Match` is required.** A mismatch is `412 version_mismatch`, the account holder is told it
changed underneath them, and the page re-fetches and re-renders with the current values
(FR-029, US3 AS-7, [D-38](../research.md#d-38)).

**Request**: any subset of `name`, `description`, `patient`, `criteria`, `charts`, `settings` —
plain pointers for absent-vs-null, `**T` where an explicit null is meaningful. Every part of a saved
report is changeable (FR-028).

**Response** `200` — `ReportTemplate` with a new `ETag`.

**Errors**: as op 72, plus `412 version_mismatch` and `428 precondition_required` when `If-Match` is
absent.

**Audit**: `update`, target `report_template`, opaque id. The old and new values are **not** in the
row: the trail is not a diff log (FR-114).

---

## 75. `DELETE /api/v1/report-templates/{id}` — remove it

`operationId: deleteReportTemplate`

**`If-Match` is required.**

**Response** `204`.

**Effect**: the row goes. **Documents already produced from it are untouched** — `export_jobs.template`
is a non-cascading relation, so the produced document keeps its own criteria snapshot and stays
downloadable until its window closes (FR-034, US3 AS-9, edge case "a saved report is deleted while a
document is being produced from it": the production completes and the document remains).

The interface confirmation **names the saved report** and states that produced documents are
unaffected (FR-028, US3 AS-9).

**Authorization**: `owner = caller`, else `404`.

**Audit**: `delete`, target `report_template`, opaque id.

---

## Producing from a saved report, with an override

There is **no** `POST /report-templates/{id}/produce`. Producing is
`POST /api/v1/exports` with `{"kind":"report","template":"{id}", "from":…, "to":…}`
([exports.md](./exports.md)): the template supplies the question, the optional `from`/`to` override
the date range **for that production only**, and the saved definition is unchanged (FR-033,
US3 AS-12). The snapshot written onto the job row is the overridden criteria, which is what makes the
document's first page state the criteria that actually produced it.
