# Contract: the activity trail (operation 68)

Conventions, status codes, the actor matrix and the audit rules are in [README.md](./README.md) and
are not repeated here.

One operation replaces what upstream spent five projections of one table and four DTOs on
(SHARED-DESIGN §2.3 op 68).

## DTO

```go
// api.AuditEntry — the complete list of fields. There is deliberately no title, no name, no label,
// no description and no summary field, and the handler performs NO lookup against any other
// collection ([D-11]). FR-068 constrains the implementation, not just the payload.
type AuditEntry struct {
    ID         string  `json:"id"`
    OccurredAt string  `json:"occurred_at"`            // RFC3339 UTC
    Actor      *string `json:"actor"`                  // opaque user id; null for system/superuser
    ActorKind  string  `json:"actor_kind"`             // "user" | "admin" | "superuser" | "system"
    Action     string  `json:"action"`                 // the bounded vocabulary
    TargetKind string  `json:"target_kind"`            // the bounded vocabulary
    TargetID   string  `json:"target_id"`              // opaque id, or a bounded job name for system rows
    Patient    *string `json:"patient"`                // opaque patient id; null for non-patient actions
    Reason     *string `json:"reason"`                 // bounded token on a refusal or a failure; null otherwise
    Affected   *int64  `json:"affected"`               // rows a job or purge touched; null otherwise
    RequestID  string  `json:"request_id"`         // always populated: the request's correlation id,
                                                   // or the background run's run_id ([D-43])
}

// There is deliberately no IP field. The trail has no `ip` column: phase 001 dropped it with a
// written rationale (001 research D-19) and no phase creates one, so there is nothing to publish
// to an administrator or to anybody else.
```

**Two tests keep this honest**, and both fail the build:

1. a reflection test asserting no field of `AuditEntry` is a free-text type outside the enumerated
   vocabularies — so nobody can add `patient_name string` in a hurry;
2. an `ApiScenario` with `ExpectedEvents` asserting a page of entries fires **zero** record-collection
   hooks — proving the reader resolved nothing, including for targets that still exist (FR-068).

---

## 68. `GET /api/v1/audit` — read the trail

`operationId: listAuditEvents`

**Query**

| Parameter | Rules |
|---|---|
| `patient` | optional, opaque id. Narrows to one person (FR-065) |
| `actor` | optional, opaque user id |
| `action` | optional, repeatable, from the bounded vocabulary; unknown value → `400` |
| `target_kind` | optional, repeatable, bounded |
| `from`, `to` | optional, RFC3339 or `YYYY-MM-DD`; `from <= to` |
| `format` | optional, `csv`. Anything else → `400` |
| `limit`, `cursor`, `count` | the shared pagination convention |
| `sort` | `-occurred_at` only (the default). Any other value → `400`: a trail read in any other order cannot give FR-066's guarantee |

Narrowings combine with AND. **The narrowing in force is echoed in the envelope** (FR-065) as
`"filters": { … }` carrying exactly the parameters that were applied.

**Response** `200` — `Page[AuditEntry]`, newest first, plus:

```json
{ "items": [ … ],
  "next_cursor": "…",
  "filters": { "patient": "rec…", "action": ["create","update"] },
  "retention": { "window_days": 730, "oldest_entry_at": "2024-09-01T10:11:12Z" } }
```

`retention` is FR-074: the window in force and the age of the oldest entry the instance still holds.
It is computed from configuration and one indexed `MIN(occurred_at)`, and it is present on **every**
page, including an empty one.

**Authorization** ([D-12](../research.md#d-12)):

- `role = user` → the query is constrained to `patient IN (:accessible) OR actor = :me`, where
  `:accessible` is the owned patients ∪ `access.ShareReader.ActivePatientsFor(actor)`, resolved per
  request with no cache. `?count=true` counts **that same constrained set**. There is no unfiltered
  count, no `total_all`, and no "N entries hidden" affordance anywhere in the DTO or the page
  (FR-071). Entries about anything else are indistinguishable from not existing.
- `role = admin` → every row on the instance, including `actor_kind ∈ {system, superuser}`: sign-in
  failures, admin-UI sessions, break-glass sessions, backups, restores, scheduled clean-ups and
  refusals (FR-072). System actions carry `actor: null` and are attributed to the system, never to a
  person.

**Paging** ([D-52](../research.md#d-52)): keyset over `(occurred_at DESC, id DESC)`, HMAC-signed,
never an offset. Because entries are append-only and immutable, new rows sort ahead of a descending
scan and can never enter a page already walked — so 50 consecutive pages under a continuous writer
repeat **0** and skip **0** (FR-066, FR-122, SC-016).

**An entry about something since deleted** renders exactly like one about something that still
exists — kind and opaque reference — and **never** an error, because nothing was ever resolved
(FR-069, US6 AS-7).

**Errors**

| Code | When | Requirement |
|---|---|---|
| `400 bad_request` | unknown `action`, `target_kind`, `sort` or `format`; `from > to` | FR-065 |
| `401 unauthenticated` | no session | — |

There is no `403` and no `404` on this operation: a narrowing that resolves to nothing the caller may
see returns `200` with `items: []`, which is what makes the scoping undetectable.

**Audit**: **none.** Reading the trail writes nothing (FR-075). This is asserted by an
`ApiScenario` that reads a page and then asserts the row count of `audit_events` is unchanged.

**Empty state**: `200` with `items: []`; the page renders `@EmptyState` inside
`region[name="Audit trail"]` (US6 AS-1, FR-125).

---

## 68b. `GET /api/v1/audit?format=csv` — export the narrowing

The **same** operation, the same narrowing semantics, a different representation
([D-13](../research.md#d-13)). It is not a separate operation, because a second one would need its
own narrowing parameters and they would drift.

**Response** `200`, `Content-Type: text/csv; charset=utf-8`,
`Content-Disposition: attachment; filename="medigo-activity-<YYYYMMDDHHMMSS>.csv"`.

Columns, in this fixed documented order, one row per entry, header row first:

```
id,occurred_at,actor,actor_kind,action,target_kind,target_id,patient,reason,affected,request_id
```

The values are exactly the DTO's, so the CSV cannot carry a class of information the JSON refuses
to — and, like the DTO, it has no `ip` column, because the trail does not store one.

**Streaming** is mandatory, not an optimisation: rows are written through `encoding/csv` directly
onto the response, page by page over the same keyset cursor, so a 1,000,000-row export completes
within **128 MiB** RSS (FR-123, SC-016). A test asserts the memory ceiling and asserts that the first
bytes reach the client before the last row is read.

**Audit**: exactly **one** `audit_export` entry, carrying the narrowing as enum values and dates —
never as free text — and written **before** the stream opens, so a client disconnect cannot lose it
(FR-067, SC-014).

**Errors**: as above, plus `500` if the stream fails after headers are sent, in which case the
partial file is not resumable and the failure is logged with the request id and no content.
