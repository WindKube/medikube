# Contract: lab results (the record family, applied to `lab_result`)

**Operations added: 0.** `lab_result` is the fifteenth registered record kind and is served by the
six-operation family phase 001 built:

```
GET    /api/v1/records                    listRecords        (lab results now appear in the cross-kind list)
GET    /api/v1/records/lab-results        listRecordsOfKind
POST   /api/v1/records/lab-results        createRecord       201 + Location
GET    /api/v1/records/lab-results/{id}   getRecord
PATCH  /api/v1/records/lab-results/{id}   updateRecord       If-Match required
DELETE /api/v1/records/lab-results/{id}   deleteRecord       If-Match required, 204
```

The OpenAPI document gains one `oneOf` branch discriminated on `kind: "lab_result"`. No path is
added.

---

## 1. DTOs

### 1.1 `LabResultSummary` — what list endpoints return

```json
{
  "id": "rec0000000000001",
  "kind": "lab_result",
  "test_name": "Comprehensive metabolic panel",
  "test_code": "24323-8",
  "category": "blood_work",
  "status": "completed",
  "ordered_on": "2026-03-01",
  "collected_on": "2026-03-02",
  "resulted_on": "2026-03-04",
  "sort_date": "2026-03-04",
  "is_panel": true,
  "component_count": 10,
  "out_of_range_count": 3,
  "value": null,
  "unit": null,
  "assessment": null,
  "practitioner": { "id": "…", "name": "Dr Amina Okafor" },
  "facility": { "id": "…", "name": "Riverside Labs" },
  "attachment_count": 1,
  "tags": [ { "id": "…", "name": "kidney", "color": "#3355aa" } ],
  "updated_at": "2026-03-04T09:12:00Z"
}
```

- `out_of_range_count` (FR-022) is computed, never stored. For a scalar result it is 0 or 1.
- `assessment` is present only for a **scalar** result and is one of `not_assessed`, `below`,
  `within`, `above` (FR-018, FR-020). It is **never** the string "normal" by default.
- `sort_date` and `is_panel` are server-derived and read-only (data-model §3.2).
- `tags` and every other slice marshal as `[]`, never `null`.

### 1.2 `LabResult` — what the detail endpoint returns

`LabResultSummary` plus:

```json
{
  "catalog_test": { "id": "…", "loinc_code": "24323-8", "name": "Comprehensive metabolic panel" },
  "interpretation": "Creatinine mildly elevated; recheck in 3 months.",
  "notes": "…",
  "ref_low": null, "ref_high": null, "ref_text": null,
  "components": [ /* Component[] — see 1.3, in display_order */ ],
  "conditions":  [ { "id": "…", "kind": "condition",  "label": "Chronic kidney disease" } ],
  "medications": [ { "id": "…", "kind": "medication", "label": "Lisinopril" } ],
  "procedures":  [],
  "encounters":  [ { "id": "…", "kind": "encounter",  "label": "Nephrology follow-up" } ],
  "treatments":  [],
  "attachments": [ /* AttachmentSummary[] — see attachments.md */ ]
}
```

`encounters` and `treatments` are **read-only back-relations** (research D-28). They are present in
responses and absent from `LabResultCreate` and `LabResultPatch`; linking to them is a `PATCH` on
the encounter or the treatment.

### 1.3 `Component`

```json
{
  "id": "cmp0000000000001",
  "test_name": "Creatinine",
  "abbreviation": "CREA",
  "canonical_name": "creatinine",
  "catalog_test": { "id": "…", "loinc_code": "2160-0", "name": "Creatinine [Mass/volume] in Serum" },
  "result_type": "quantitative",
  "value": 128.0,
  "value_text": null,
  "unit": "umol/L",
  "ref_low": 60.0,
  "ref_high": 110.0,
  "ref_text": null,
  "status": null,
  "assessment": "above",
  "display_order": 3
}
```

- `canonical_name`, `assessment` and `display_order` are server-owned and read-only.
- `assessment` is `not_assessed` whenever there is no numeric bound and no explicit `status`
  (FR-020) — the API never says "normal" on a guess.
- `result_type` values are `quantitative | qualitative | textual`, mapping to the specification's
  numeric / categorical / free text (research D-32).

### 1.4 `LabResultCreate`

```json
{
  "patient": "pat0000000000001",
  "test_name": "Comprehensive metabolic panel",
  "test_code": "24323-8",
  "category": "blood_work",
  "catalog_test": "cat0000000000001",
  "status": "ordered",
  "ordered_on": "2026-03-01",
  "collected_on": "2026-03-02",
  "resulted_on": "2026-03-04",
  "interpretation": "…",
  "notes": "…",
  "value": null, "unit": null, "ref_low": null, "ref_high": null, "ref_text": null,
  "practitioner": "prc0000000000001",
  "facility": "fac0000000000001",
  "conditions": ["…"], "medications": [], "procedures": [],
  "tags": ["tag0000000000001"],
  "components": [ /* ComponentInput[] */ ]
}
```

Only `patient` and `test_name` are required (FR-002). **`is_panel`, `sort_date`,
`canonical_name`, `assessment`, `component_count`, `out_of_range_count`, `attachment_count`,
`encounters` and `treatments` are absent from this DTO by construction** — that is how privilege
over server-owned state is prevented, not by a check.

### 1.5 `ComponentInput`

```json
{ "id": "cmp0000000000001",
  "test_name": "Creatinine", "abbreviation": "CREA",
  "catalog_test": "cat0000000000002",
  "result_type": "quantitative",
  "value": 128.0, "value_text": null,
  "unit": "umol/L", "ref_low": 60.0, "ref_high": 110.0, "ref_text": null,
  "status": null }
```

`id` is optional and is what makes the replace-set stable (§3). `display_order` is **not** an input;
it is the array index.

### 1.6 `LabResultPatch`

Every writable field of `LabResultCreate` as an optional pointer, except `patient`, which is
**refused** with `422` if present — a record is never re-filed against another patient.
`components`, when present, is the **complete** set (§3). When absent, the component set is left
untouched.

---

## 2. `GET /api/v1/records/lab-results`

| Parameter | Notes |
|---|---|
| `patient` | **required**; absence is `400 patient_required` |
| `status` | comma list from `OrderStatus` (FR-009) |
| `category` | comma list from `LabCategory` (FR-009) |
| `from`, `to` | `YYYY-MM-DD`, inclusive, applied to `sort_date` (FR-009) |
| `q` | case-insensitive substring over `test_name` only (FR-009). **Not** over components, interpretations or notes |
| `tags` | comma list of tag ids |
| `sort` | allowlist: `sort_date` (default `-sort_date`), `test_name`, `status`, `created` |
| `limit`, `cursor`, `count` | standard |

`200` with `{"items": [LabResultSummary], "next_cursor": …}`.

Ordering is by `sort_date` descending then `id` descending, so **a result with no dates at all
still has a defined position** and never disappears from the list (FR-008, edge case). Paging is
keyset-based, so entries added or removed while paging never cause a repeat or a skip (FR-009).

**Performance**: 5,000 results for one patient, every page within 2 s (SC-011), served by
`idx_lab_results_list`. Proven by the build-tagged scale test, not assumed.

---

## 3. `POST` and `PATCH` — the component replace-set

`components` in a create or patch payload is the **complete set** belonging to the result. The
service diffs inside one transaction:

| Payload element | Action |
|---|---|
| has `id`, and that id belongs to this result | update in place; `display_order` = array index |
| has no `id` | create; `display_order` = array index |
| has `id` belonging to another result or absent from storage | the **whole submission** is refused, `422`, field `components[i].id`, code `not_found` |
| a stored component whose id is absent from the payload | deleted permanently |

Consequences that are contractual, not incidental:

- Component ids are **stable** across saves (US1 scenario 8: one removed, one changed, one added
  leaves exactly the submitted set).
- Two components may share a `test_name` in one result and both are kept, in the order given
  (FR-016).
- `is_panel` is recomputed at the end of the transaction (FR-006). Converting scalar → panel clears
  `value`, `unit`, `ref_low`, `ref_high`, `ref_text` in the same transaction; converting panel →
  scalar deletes every component.
- Submitting an overall value **and** components together is `422`, code `panel_and_value`, and
  **neither part is discarded** — the response echoes what was refused so the form keeps it
  (FR-005, US1 scenario 10).
- A change to the component set is audited as an **update to the lab result**, once, with no
  content (FR-023).
- A panel of **100 components** is accepted and returned in one response without truncation
  (FR-085). `limit`/`cursor` do not apply to a result's own components.

`PATCH` requires `If-Match`; a stale value is `412` and the response carries the current
representation so the client can show what changed underneath it (US1 scenario 11). **A component
set is never merged.**

---

## 4. `DELETE /api/v1/records/lab-results/{id}`

`If-Match` required. `204`. In one transaction (data-model §3.5):

1. every component is destroyed permanently and disappears from every trend (FR-015);
2. every attached document is moved to the **trash**, recoverable for the retention window
   (FR-067);
3. every link from another record is removed, leaving that record intact (FR-047);
4. the `search_index` row is removed;
5. one `delete` audit row is written, by opaque id, with no content.

The UI confirmation names the result and states both (1) and (2) before the account holder commits
(US1 scenario 12, and the deletion edge cases).

---

## 5. Validation errors, by requirement

| Submission | Response |
|---|---|
| `collected_on` before `ordered_on` **and** an unknown `status`, in one request | `422` with **both** in `fields[]`, codes `date_order` and `invalid_value` — every offending field in one submission (FR-007, US1 scenario 4) |
| `ref_low > ref_high` on the result or on any component | `422`, `ref_range_inverted` (FR-017) |
| only one of `ref_low`/`ref_high` | accepted; judged on the bound present (FR-017) |
| `quantitative` component with `value_text` set | `422`, `value_kind_mismatch`, naming both fields (FR-013) |
| `qualitative` component with `value` set | `422`, `value_kind_mismatch` (FR-013) |
| `interpretation` at exactly 2000 characters | accepted |
| `interpretation` at 2001 characters | `422`, `too_long`, message names the field and the limit |
| a `condition` id belonging to another patient | `422`, `not_found` on that element — the message discloses nothing about whether that record exists (FR-045, US5 scenario 3) |
| an unknown JSON field, or a duplicate JSON key | `422` (Go 1.27 `encoding/json/v2`) |

---

## 6. Authorization

| Case | Result |
|---|---|
| owner lists, reads, creates, updates, deletes | success |
| a stranger requests a lab result by id | `404`, byte-identical to a request for an id that never existed; an `access_denied` audit row is written (FR-073, US1 scenario 13) |
| a stranger lists with another patient's `?patient=` | `404` — not `403`, and not an empty list |
| unauthenticated, any operation | `401`, no information about the patient |
| a superuser | success, **and** an audit row (Constitution VII) |
| a create naming a patient the actor cannot reach | `404` |
| a patch attempting to change `patient` | `422` |

---

## 7. Realtime

Lab result lists are live-updated by the existing `GET /api/v1/streams/records?patient=&kind=lab_result`
stream. The hub publishes **record ids only**; the per-subscriber SSE handler re-fetches each id,
**re-authorises it for that subscriber**, then renders and patches (Constitution V). This phase
adds no stream and no hub change. A result deleted elsewhere stops contributing to an open trend on
the next update (edge case: two things happening at once).
