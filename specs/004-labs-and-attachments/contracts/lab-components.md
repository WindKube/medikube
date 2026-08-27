# Contract: comparing a component over time

**Operations added: 2.** Shared design contract §2.3 entries 55–56.

```
GET /api/v1/lab-components         listLabComponents      the per-patient rollup
GET /api/v1/lab-components/trend   getLabComponentTrend   one series, one unit
```

Components have **no** CRUD of their own — they are managed through their parent lab result's
payload (see `lab-results.md` §3). These two operations are read-only projections.

**Nothing here is stored.** Every figure is computed from the readings, so correcting a reading
corrects the comparison (spec, Key Entities: "Component series (derived)").

---

## 1. `GET /api/v1/lab-components` — every distinct component, once

| Parameter | Notes |
|---|---|
| `patient` | **required**; absence is `400 patient_required` |
| `q` | case-insensitive substring over `test_name` / `canonical_name` |
| `sort` | allowlist: `-latest_on` (default), `test_name`, `-reading_count` |
| `limit`, `cursor`, `count` | standard |

`200`:

```json
{ "items": [
    { "series_key": "cat0000000000002",
      "match": "catalog",
      "test_name": "Creatinine",
      "abbreviation": "CREA",
      "catalog_test": { "id": "cat0000000000002", "loinc_code": "2160-0", "name": "Creatinine [Mass/volume] in Serum" },
      "result_type": "quantitative",
      "latest_value": 128.0,
      "latest_value_text": null,
      "latest_unit": "umol/L",
      "latest_status": null,
      "latest_assessment": "above",
      "latest_on": "2026-03-04",
      "reading_count": 12,
      "units": [ { "unit": "umol/L", "reading_count": 10 }, { "unit": "mg/dL", "reading_count": 2 } ],
      "multi_unit": true },

    { "series_key": "haemoglobin a1c",
      "match": "name",
      "test_name": "Haemoglobin A1c",
      "catalog_test": null,
      "result_type": "quantitative",
      "latest_value": 6.1, "latest_unit": "%", "latest_assessment": "within",
      "latest_on": "2026-03-04", "reading_count": 4,
      "units": [ { "unit": "%", "reading_count": 4 } ], "multi_unit": false }
  ],
  "next_cursor": null }
```

**Grouping** (FR-024, FR-025, research D-06). Each distinct component appears **once**. Identity is
the catalogue entry where one was matched, and otherwise the normalised test name — NFKC, trimmed,
internal whitespace collapsed, case-folded. `match` states which of the two applied, so the UI can
say why two spellings became one row.

- Two components recorded under different spellings but matched to the **same catalogue entry**
  appear as one row with two readings (FR-041, US4 scenario 4).
- Two components recorded as `"  Glucose "` and `"glucose"` with no catalogue match appear as one
  row with two readings (US4 scenario 5).
- A component the catalogue has never heard of is listed and trended in its own right (FR-042,
  US4 scenario 6).

`units[]` carries every distinct unit ever recorded for this identity with its reading count;
`multi_unit` is `len(units) > 1`. **The rollup never merges units into a value and never converts
one into another** (FR-027, FR-028) — it reports the latest reading's unit as `latest_unit` and
lists the rest.

`test_name` is the **most recent** spelling recorded, so the list reads the way the account holder
last typed it.

Empty result: `{"items": [], "next_cursor": null}` with `200`. The page renders the "nothing to
compare yet" empty state with a route to recording a first result (US3 scenario 2).

---

## 2. `GET /api/v1/lab-components/trend` — one series

| Parameter | Notes |
|---|---|
| `patient` | **required** |
| `series_key` | **required** — the `series_key` from the rollup: a catalogue id, or a normalised name |
| `unit` | **required when the identity has readings in more than one unit** (research D-31); optional otherwise |
| `from`, `to` | `YYYY-MM-DD`, inclusive, applied to the parent result's `sort_date` (FR-029) |

`200`, numeric series:

```json
{ "series_key": "cat0000000000002",
  "match": "catalog",
  "test_name": "Creatinine",
  "result_type": "quantitative",
  "unit": "umol/L",
  "units": [ { "unit": "umol/L", "reading_count": 10 }, { "unit": "mg/dL", "reading_count": 2 } ],
  "multi_unit": true,
  "range_start": "2024-03-04",
  "range_end":   "2026-03-04",
  "capped": false,
  "cap_limit": 500,
  "band": { "ref_low": 60.0, "ref_high": 110.0, "ref_text": null,
            "from_reading": "cmp0000000000009", "from_reading_on": "2026-03-04" },
  "readings": [
    { "component_id": "cmp0000000000001",
      "lab_result": { "id": "rec0000000000001", "test_name": "CMP" },
      "recorded_on": "2024-03-04",
      "value": 92.0, "value_text": null, "unit": "umol/L",
      "ref_low": 60.0, "ref_high": 110.0, "ref_text": null,
      "status": null, "assessment": "within" }
  ],
  "summary": {
    "reading_count": 10,
    "earliest_on": "2024-03-04", "latest_on": "2026-03-04",
    "latest_value": 128.0,
    "min_value": 88.0, "max_value": 131.0, "mean_value": 104.7,
    "in_range_count": 6,
    "direction": "rising",
    "direction_rule": "Split the readings into an older half and a newer half, discarding the middle reading when the count is odd. Rising if the newer mean exceeds the older mean by more than five per cent of the older mean, falling if it is below by more than five per cent, steady otherwise.",
    "insufficient_readings": false } }
```

`200`, categorical or textual series (FR-033):

```json
{ "series_key": "urine culture", "result_type": "qualitative", "unit": "",
  "readings": [ { "component_id": "…", "recorded_on": "2026-01-04", "value_text": "no growth", "assessment": "not_assessed" } ],
  "value_history": [ { "value_text": "no growth", "count": 3 }, { "value_text": "E. coli", "count": 1 } ],
  "summary": null }
```

`summary` is `null` and `value_history` is present — **no mean, no minimum, no maximum and no
direction is offered** for a non-numeric series.

### 2.1 The rules this response encodes

| Rule | Requirement |
|---|---|
| readings are in date order, by the parent's `sort_date` then id then `display_order` | FR-026 |
| each reading carries **the reference range recorded with it**, not the newest one | FR-035 |
| `band` names which reading's range it is drawing | FR-035, US3 scenario 10 |
| readings in different units are **never** in one `readings` array | FR-027 |
| `units[]` and `multi_unit` tell the caller other units exist and let it choose | FR-027, US3 scenario 4 |
| the `unit` being shown is always stated | FR-027 |
| **no value is converted from one unit into another, anywhere** | FR-028, US3 scenario 5 |
| `summary` is computed over the same range as `readings` | FR-029 |
| `direction` requires ≥ 3 readings and is derived by the published rule, which is returned with it | FR-031 |
| fewer than 3 readings → `direction: null`, `insufficient_readings: true`, and the one reading is still returned | FR-032, US3 scenario 6 |
| `capped: true` plus `range_start`/`range_end` when the series was capped; the summary follows the returned window | FR-034, research D-08 |

### 2.2 Errors

| Case | Response |
|---|---|
| `patient` absent | `400 patient_required` |
| `series_key` absent | `400 bad_request` |
| more than one unit exists and `unit` absent | `400`, code `unit_required`, with the available units and their counts in `fields[]` |
| `unit` given but no readings in it | `200` with an empty `readings` array and `summary: null` — not a 404, because the identity exists |
| `series_key` unknown for this patient | `404 not_found` |
| a patient the actor cannot reach | `404 not_found`, indistinguishable from that patient not existing (FR-072, US3 scenario 12) |
| unauthenticated | `401` |

### 2.3 Performance

SC-003: a component with **100 readings across 50 lab results** returns its series and its summary
within **2 s**. Both operations are single parameterised SQL statements through `app.DB()`
(research D-27), scoped by the `patient` from the authorizer's `Grant` and never from the raw
request, indexed by `idx_lab_components_canonical` and `idx_lab_results_list`. An injection test
pushes `%'; DROP TABLE` shaped input through `q`, `series_key` and `unit`.
