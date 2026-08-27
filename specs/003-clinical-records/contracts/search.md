# Contract: search

**Operations added: 1.** Shared design contract §2.3 entry 57, moved into this phase (see
`plan.md` — Deviations).

```
GET /api/v1/search        search
```

---

## 1. Request

| Parameter | Type | Notes |
|---|---|---|
| `q` | string 1..200 | **required**. `422` if empty or over 200. |
| `patient` | id | **required**. Absent → `400 patient_required`. There is no search across every person an account can reach (FR-070). |
| `kinds` | csv of path segments | default = every registered kind |
| `tags` | csv of tag ids | |
| `match` | `any` \| `all` | default `any` |
| `from`, `to` | `YYYY-MM-DD` | over `occurred_on` |
| `status` | csv | narrows to rows whose source status is in the set (FR-071) |
| `limit` | int 1..100 | default 25 — **per group** |
| `cursor` | csv of `kind:cursor` pairs | opaque, **one cursor per group** |

---

## 2. Response `200`

```json
{ "groups": [
    { "kind": "medication",
      "items": [ { "id":"…","kind":"medication","title":"Warfarin",
                   "snippet":null,"occurred_on":"2024-01-08","tags":[] } ],
      "next_cursor": "eyJ…", "has_more": true },
    { "kind": "encounter", "items": [ … ], "next_cursor": null, "has_more": false }
  ],
  "criteria": { "q_present": true, "kinds": ["medication","encounter"], "tags": [], "match": "any" },
  "empty_reason": null
}
```

- **One group per kind, each with its own `next_cursor` and `has_more`** (FR-072). Upstream had one
  global pagination block over a per-type limit, which cannot be correct.
- Only kinds with at least one match appear in `groups`. A kind the actor filtered out never
  appears (FR-071).
- Ordering **within** a group is `occurred_on DESC, id DESC`, nulls last (FR-073, research D-06).
  Group order is the registry's declared kind order — stable, and not a relevance signal.
- **`snippet` is always `null` in this phase.** The field exists so phase 004 can populate it
  without a wire change; MediGo does not highlight matches and does not claim relevance ranking
  (FR-073).
- `criteria` echoes the narrowing so the page can render removable chips — and note that it echoes
  `q_present`, **not `q`**: the term is never reflected into a body that could be logged
  downstream (FR-075, research D-12).
- `empty_reason` is `null`, `"no_matches"` or `"no_records"` — FR-072 and US8 scenario 2 require
  "nothing matched that" to be a *distinct* state from "nothing has been recorded yet".

---

## 3. Errors

`400 patient_required` · `400 bad_request` (unknown kind segment, malformed cursor) ·
`401 unauthenticated` · `404 not_found` (a patient the actor cannot reach — identical to a
non-existent patient) · `422 validation_failed` (empty or over-long `q`).

---

## 4. Authorization

`Authorizer.Patient(actor, ?patient, PermView)` once, before any query. The index is patient-scoped,
so a term matching only another account's records returns **`groups: []`** with
`empty_reason: "no_matches"` — byte-identical to a term that genuinely matches nothing (FR-074,
SC-004). Nothing in the response, its length, its timing budget or its headers discloses that a
match exists elsewhere.

---

## 5. Implementation contract

- Source of rows: the `search_index` collection (`data-model.md` §5.3), maintained by the
  post-commit hooks that `records.Register` binds. A create/update upserts one row; a delete
  removes it; a patient delete cascades.
- Matching is `LIKE '%term%'` over `title` and `body`, case-insensitive. **Not FTS5** — FR-073
  forbids claiming relevance ranking, and ranking is the only thing an FTS5 table buys, so its raw
  SQL migration, its out-of-band maintenance and its rebuild command would cost this phase a
  mechanism and buy it nothing (research D-11). **FTS5 availability is not the reason**: risk R3 is
  CLOSED — VERIFIED-SOURCE-FACTS FACT 11 proves FTS5, `MATCH` and `rank` all work in
  `modernc.org/sqlite` v1.57.0, the version PocketBase v0.40.1 pulls. The capability exists and is
  deliberately unused (corrected 2026-08-27, ANALYSIS N12).
- The term is escaped for `LIKE` (`%`, `_`, the escape character) in the repository. It is **never**
  concatenated into a PocketBase filter DSL string.
- Each kind declares `SearchFields(entity) (title, body string)` on its registry entry; the
  registry completeness test fails the build if a kind does not.

---

## 6. Tests this contract requires

| Test | Requirement |
|---|---|
| a term in 3 kinds → 3 groups, each independently paged | FR-069, FR-072, US8-1 |
| absent `patient` → `400 patient_required`, no fallback to the active patient | FR-070, US8-3 |
| a term matching only another account → `groups: []`, `empty_reason: no_matches`, byte-identical to a nonsense term | FR-074, SC-004, US8-4 |
| `no_matches` vs `no_records` are distinct | US8-2 |
| narrowing by kind, by tag and by date range each reflected in `criteria` and in the rows | FR-071, US8-6 |
| ordering `occurred_on DESC, id DESC`, nulls last, identical across repeated requests | FR-073 |
| deleting a record removes its index row within the same transaction's commit | FR-058, D-11 |
| deleting a patient removes every index row for that patient | FR-087, SC-005 |
| **the term appears in no log line, span attribute, metric label or Sentry event** | FR-075, SC-012 |
| first page within 3 s at 50,000 indexed rows (`-tags=scale`) | SC-003, FR-089 |
