# Contract: `GET /api/v1/patients/{id}/summary`

One operation, `operationId: getPatientChart`. Requirements covered: FR-027 … FR-031, FR-048,
SC-004.

This is the phase's only derived read. **Nothing in it is stored separately from the data it
summarises** (spec Key Entities). It is computed on every request; there is no counter column, no
cache and no summary table (research D-22).

## Response 200

```json
{
  "patient": { /* Patient, exactly as getPatient returns it, including `display` */ },
  "counts": [
    { "kind": "medication", "path": "/medications", "label": "Medications", "count": 12 }
  ],
  "total_records": 12,
  "recent_activity": [
    { "occurred_at": "2026-08-25T09:12:00Z",
      "action": "create",
      "target_kind": "medication",
      "target_id": "rec_9f2…",
      "target_exists": true,
      "actor_kind": "user" }
  ]
}
```

### `counts`

One entry per kind registered in `internal/records`, **including kinds with a count of zero** — the
UI needs the tile to render an empty state and an "add the first one" action (FR-030, US4-2). One
kind exists today (`medication`); fifteen will by phase 004, and this endpoint's code does not
change when they are added (Principle II open/closed, research D-22).

`count` counts only records attributed to this patient (FR-028, SC-007) and only those the viewer
is entitled to see — which in this phase is the same set, and in phase 005 will not be.

### `recent_activity`

The last **10** `audit_events` rows for this patient, newest first, from the
`(patient, occurred_at DESC)` index.

Each entry states **what kind of record changed, what happened to it, and when** (FR-029) and
carries **no content whatsoever** — no name, no value, no note, no filename. That is structural:
`audit_events` never held content in the first place (SHARED-DESIGN §1.2), so "entries for records
that have since been deleted carry no identifying detail about them" (FR-029, US4-5) is true by
construction rather than by filtering. `target_exists` tells the renderer whether to offer a link;
when it is `false` the entry renders as e.g. "A medication was deleted — 25 August" and links
nowhere.

### The empty state

A patient with nothing recorded returns `counts` with every kind at 0 and `recent_activity: []`
(never `null` — Go 1.27 json/v2 nil-vs-empty, research D-32). The page renders `@EmptyState` inside
its `region[name="Patient chart"]` landmark, so the landmark assertion holds on a freshly seeded
instance (FR-030, US4-2, SC-013).

## Statuses

| Status | When |
|---|---|
| 200 | the actor owns the patient |
| 404 `not_found` | not owned / does not exist — indistinguishable, audited (FR-042) |
| 401 | no session (FR-043) |

`ETag` is **not** issued: the response aggregates several collections, so a single `updated`
timestamp would be a lie. `Cache-Control: private, no-store`.

## Performance contract

SC-004: **within 2 seconds for a patient holding 50,000 records.** The query plan is:

- 1 indexed row read for the patient;
- N `SELECT COUNT(*) … WHERE patient = ?` (N = registered kinds), each on that collection's
  `(patient)` index;
- 1 `LIMIT 10` scan of `audit_events (patient, occurred_at DESC)`;
- 0..1 row read for `primary_practitioner`.

A benchmark task seeds 50,000 medications on one patient and asserts a p95 under 2 s, so the claim
is measured rather than asserted (Principle IX).

## Second consumer: the delete confirmation

FR-048 requires the delete dialog to name the person and state how many records will be destroyed.
Both are in this response, which the `/patients/{id}` page has already loaded. **There is no
separate preview endpoint** (research D-26).
