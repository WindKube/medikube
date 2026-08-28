# Contract: the report builder (operations 69–70)

Conventions, status codes, the actor matrix and the audit rules are in [README.md](./README.md).

These two operations are the **builder**. Producing a document is `POST /api/v1/exports`
([exports.md](./exports.md)); saving the question is `/api/v1/report-templates`
([report-templates.md](./report-templates.md)).

## DTOs

```go
// api.ReportSummary — what the builder shows before anything is chosen, and what it shows after
// every change to the selection. One shape for both, so the two figures cannot diverge.
type ReportSummary struct {
    Patient   string          `json:"patient"`            // opaque id, echoed
    Kinds     []KindCount     `json:"kinds"`              // ALWAYS every registered kind, zeros included
    Total     int             `json:"total"`
    Limit     int             `json:"limit"`              // MEDIKUBE_REPORT_MAX_RECORDS, so the UI can warn early
    OverLimit bool            `json:"over_limit"`         // Total > Limit (FR-010)
    Filters   ReportFilters   `json:"filters"`            // the narrowing that produced these numbers
}

type KindCount struct {
    Kind  string `json:"kind"`        // singular snake_case, e.g. "lab_result"
    Label string `json:"label"`       // "Lab results" — a UI label, never patient data
    Count int    `json:"count"`
}

// api.TrendsResponse — the chart picker. Both halves in one response (D-39).
type TrendsResponse struct {
    Patient   string        `json:"patient"`
    Vitals    []TrendSeries `json:"vitals"`
    Labs      []TrendSeries `json:"labs"`
    Minimum   int           `json:"minimum"`     // MEDIKUBE_REPORT_MIN_CHART_POINTS, published (FR-017)
    MaxCharts int           `json:"max_charts"`  // MEDIKUBE_REPORT_MAX_CHARTS, published (FR-023)
}

type TrendSeries struct {
    Source         string   `json:"source"`                    // "vitals" | "lab"
    Metric         string   `json:"metric,omitempty"`          // vitals column
    CanonicalName  string   `json:"canonical_name,omitempty"`  // lab component
    Label          string   `json:"label"`                     // "Systolic blood pressure"
    Unit           string   `json:"unit"`                      // exactly one unit per series
    MultiUnit      bool     `json:"multi_unit"`                // this name was recorded in >1 unit
    Readings       int      `json:"readings"`
    FirstAt        string   `json:"first_at"`                  // YYYY-MM-DD
    LastAt         string   `json:"last_at"`                   // YYYY-MM-DD
    Chartable      bool     `json:"chartable"`                 // Readings >= Minimum
    ReadingsNeeded int      `json:"readings_needed"`           // 0 when chartable (FR-017)
    RefLow         *float64 `json:"ref_low,omitempty"`         // when recorded with the readings
    RefHigh        *float64 `json:"ref_high,omitempty"`
}
```

`Kinds` always lists **every registered kind**, including those at zero, because US1 AS-1 and AS-2
require a person with nothing recorded to see every kind at zero rather than an empty list.

---

## 69. `GET /api/v1/reports/summary` — the counts, before and during

`operationId: getReportSummary`

Replaces three upstream endpoints that each computed the same counters
(`patients/me/dashboard-stats`, `export/summary`, `custom-reports/data-summary`).

**Query**

| Parameter | Rules |
|---|---|
| `patient` | **required**; absent is `400 patient_required`, never an implicit fallback to the active patient (SHARED-DESIGN §2.1 rule 4) |
| `kinds` | optional, comma list of registered kinds. Absent = every kind |
| `from`, `to` | optional, `YYYY-MM-DD`, inclusive; `from <= to` |
| `tags` | optional, comma list of **tag ids** |
| `statuses` | optional, comma list of lifecycle values |

With no narrowing beyond `patient`, this is the builder's opening view: how much there is, per kind,
with a total (US1 AS-2). With a narrowing, it is the resolved count that updates as the selection
changes (US1 AS-3, FR-003) — **the same operation**, because two operations would eventually
disagree.

**Response** `200` — `ReportSummary`.

**These figures are what a report over the same selection contains, exactly** (FR-003, SC-002). They
come from `report.Selection.Counts`, and the document comes from `report.Selection.Each`, which are
two methods over one query builder and one authorization sequence ([D-44](../research.md#d-44)). The
contract test that asserts their equality is the enforcement.

**Authorization**: `access.Authorizer.Patient(actor, patient, PermView)`. A person the caller cannot
reach is `404`, byte-identical to a person who does not exist (FR-001, US1 AS-9), **and the attempt
is recorded** as `access_denied`.

**Errors**

| Code | When | Requirement |
|---|---|---|
| `400 patient_required` | `patient` absent | SHARED-DESIGN §2.1 |
| `400 bad_request` | unknown kind, unknown status, `from > to` | FR-002 |
| `404 not_found` | the person is not reachable, or does not exist | FR-001 |
| `422 unknown_tag` | a tag id outside the patient owner's set — identical response for "not yours" and "does not exist" | phase 005 D-22 |

**Audit**: none on success (reading your own is not recorded — FR-115); one `access_denied` on
refusal.

**Scale**: SC-023 — the opening view within **2 s** and a narrowed re-count within **500 ms** on a
person with 10,000 records.

---

## 70. `GET /api/v1/reports/trends` — what is worth charting

`operationId: getReportTrends`

Folds upstream's `available-trend-data` and `trend-chart-counts` into one response
(SHARED-DESIGN §2.3 op 70).

**Query**

| Parameter | Rules |
|---|---|
| `patient` | **required**; absent is `400 patient_required` |
| `from`, `to` | optional, `YYYY-MM-DD` — a **candidate** chart range, so the picker can tell the person *at the moment they choose it* that the range holds too few readings (FR-019, US4 AS-5) |

**Response** `200` — `TrendsResponse`.

Semantics, all of them FR-016 through FR-019 and [D-39](../research.md#d-39):

- **`vitals`** — one entry per numeric column of the `vitals` kind that has at least one reading.
  Vitals are stored in SI (SHARED-DESIGN §1.5), so a vitals series has exactly one unit and no unit
  choice ever arises.
- **`labs`** — grouped by `(canonical_name, unit)`. **A component recorded in more than one unit
  appears once per unit**, each entry carrying its own `readings`, `first_at` and `last_at`, and both
  entries carrying `multi_unit: true`. There is no combined entry, ever, because a combined entry is
  where a conversion would have to live.
- **A series below the minimum is present with `chartable: false`, `readings` and
  `readings_needed`** — never hidden (FR-017, US4 AS-3). A person needs to know that two more
  readings would make their blood pressure chartable.
- **No unit is ever converted, anywhere.** A `Convert` function does not exist in the codebase; a
  grep test asserts it (FR-018, SC-008).
- `minimum` and `max_charts` are the **published** values of the two configuration knobs, so the
  interface states the limits rather than the handbook alone (FR-017, FR-023, FR-087).

**Empty**: a person with no measured values returns `vitals: []`, `labs: []` and the two limits; the
picker renders its explanation inside the page landmark (US4 AS-1, FR-125).

**Authorization**: identical to op 69 — `PermView` on the person, `404` otherwise, recorded (FR-024,
US4 AS-10).

**Errors**: `400 patient_required`; `400 bad_request` for `from > to`; `404 not_found`.

**Audit**: none on success; one `access_denied` on refusal.
