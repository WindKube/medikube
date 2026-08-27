# Contract: `/api/v1/records` — the record family

**Six operations that serve every clinical record kind, now and in every later phase.** Phase 003
registers eleven more kinds and adds **zero** routes; phase 004 adds lab results the same way.
This is the single decision that makes the suite's 94-operation budget (SHARED-DESIGN §2.3) reachable while keeping
every kind's DTO explicitly typed.

Requirements covered: FR-015 … FR-033, FR-036, FR-054, FR-064.

```
GET    /api/v1/records                        listRecords          cross-kind
GET    /api/v1/records/{kind}                 listRecordsOfKind
POST   /api/v1/records/{kind}                 createRecord
GET    /api/v1/records/{kind}/{id}            getRecord
PATCH  /api/v1/records/{kind}/{id}            updateRecord         If-Match required
DELETE /api/v1/records/{kind}/{id}            deleteRecord         If-Match required
```

`{kind}` is documented in OpenAPI as an `enum` of the registered path segments. In this phase that
enum has exactly one member: **`medications`** (plural — research D-05). Request and response
bodies are `oneOf` with `kind` as the discriminator, so **every kind keeps its own fully typed
DTO**; the polymorphism is in the routing table, not in the schema.

A `{kind}` outside the enum is **`404 not_found`**, not `400`. An unregistered kind is
indistinguishable from a path that does not exist, which is the same reasoning that makes another
person's record a 404.

## The medication DTOs

```go
// internal/web/api

// MedicationSummary is what list endpoints return.
type MedicationSummary struct {
    ID        string  `json:"id"`
    Kind      string  `json:"kind"`                    // always "medication" — the oneOf discriminator
    Name      string  `json:"name"`
    Dosage    string  `json:"dosage,omitempty"`
    Frequency string  `json:"frequency,omitempty"`
    Status    string  `json:"status"`
    StartedOn *string `json:"started_on"`              // "YYYY-MM-DD" or null
    UpdatedAt string  `json:"updated_at"`              // RFC3339 UTC
}

// Medication is what detail endpoints return.
type Medication struct {
    MedicationSummary
    AlternativeName string  `json:"alternative_name,omitempty"`
    Type            string  `json:"type,omitempty"`
    Route           string  `json:"route,omitempty"`
    Indication      string  `json:"indication,omitempty"`
    EndedOn         *string `json:"ended_on"`
    SideEffects     string  `json:"side_effects,omitempty"`
    Notes           string  `json:"notes,omitempty"`
    CreatedAt       string  `json:"created_at"`
}

type MedicationCreate struct {                          // no `owner`, no `id`, no timestamps
    Name            string  `json:"name"`               // required
    AlternativeName string  `json:"alternative_name,omitempty"`
    Type            string  `json:"type,omitempty"`
    Dosage          string  `json:"dosage,omitempty"`
    Frequency       string  `json:"frequency,omitempty"`
    Route           string  `json:"route,omitempty"`
    Indication      string  `json:"indication,omitempty"`
    StartedOn       *string `json:"started_on,omitempty"`
    EndedOn         *string `json:"ended_on,omitempty"`
    Status          string  `json:"status,omitempty"`   // defaults to "active"
    SideEffects     string  `json:"side_effects,omitempty"`
    Notes           string  `json:"notes,omitempty"`
}

type MedicationPatch struct {                           // no `owner`, no `id`, no timestamps
    Name            *string  `json:"name,omitempty"`
    AlternativeName *string  `json:"alternative_name,omitempty"`
    Type            *string  `json:"type,omitempty"`
    Dosage          *string  `json:"dosage,omitempty"`
    Frequency       *string  `json:"frequency,omitempty"`
    Route           *string  `json:"route,omitempty"`
    Indication      *string  `json:"indication,omitempty"`
    StartedOn       **string `json:"started_on,omitempty"`  // **T: outer nil absent, inner nil explicit null
    EndedOn         **string `json:"ended_on,omitempty"`
    Status          *string  `json:"status,omitempty"`
    SideEffects     *string  `json:"side_effects,omitempty"`
    Notes           *string  `json:"notes,omitempty"`
}
```

**`owner` is absent from both write DTOs.** FR-032 — authorization is never inferred from a
client-supplied identifier — is therefore enforced by shape rather than by a runtime check.
Because unknown fields are rejected, a request carrying `owner` is `422 validation_failed` with
field code `unknown_field`, and the test asserts both the `422` **and** that the stored record is
unchanged. Phase 002 relies on exactly this mechanism to make re-attribution to another patient
impossible.

**Absent optional fields are absent, not empty.** FR-024 requires the detail view to omit fields
that were never filled in "rather than presenting empty placeholders", and `omitempty` on the
string fields is what makes that a property of the wire format rather than a rendering
convention. `StartedOn` and `EndedOn` are `*string` **without** `omitempty` so a client can
distinguish "not recorded" (`null`) from "not in this response" — a distinction that matters the
moment a partial response exists, which it never will, so the explicit `null` is the honest shape.

**`**string` on the two dates in the patch** carries absent-versus-explicit-null with plain
pointers. `samber/mo` is forbidden and `*T`/`**T` is stdlib, marshals correctly under
`encoding/json/v2`, and needs no custom `MarshalJSON` to round-trip through the OpenAPI generator.

---

## `listRecordsOfKind` — `GET /api/v1/records/{kind}`

**Query parameters** — explicit and named. PocketBase's filter DSL never reaches the wire.

| Parameter | Values | Requirement |
|---|---|---|
| `q` | case-insensitive substring over `name` and `alternative_name` | FR-022 |
| `status` | comma list of `TherapyStatus` values | FR-022 |
| `sort` | `-started_on` (default), `started_on`, `name`, `-name`, `-updated`, `updated` | FR-022 |
| `limit` | 1..100, default 25 | FR-023 |
| `cursor` | opaque, server-minted | FR-023 |
| `count` | `true` to include `total` | — |

**200**
```json
{ "items": [ /* MedicationSummary */ ],
  "next_cursor": "eyJrIjoi…" }
```

**Sorting.** Default `-started_on`, with `id DESC` always appended as the tiebreaker, backed by
`idx_medications_owner_start (owner, started_on DESC, id DESC)`. A `sort` value outside the
allowlist is `422 validation_failed` with field `sort` and code `invalid_value` — never silently
ignored, because a silently ignored sort produces a list that looks right and is not.

**Rows with a null `started_on` sort last** under both directions, and the ordering of nulls is
stated here rather than left to SQLite's default, because a person with a medication whose start
date they never recorded should not find it at the top of "most recently started".

**FR-023 — paging never repeats and never skips.** The cursor encodes the sort key values and the
last id, not an offset, so a row inserted or deleted between two page requests cannot shift a
boundary. A test inserts and deletes rows between pages and asserts the union of the pages
contains no duplicate and misses no row that existed for the whole traversal.

A forged, tampered or unparseable cursor is **`400 invalid_cursor`** and writes an `access_denied`
audit row (research D-25).

**SC-002**: with 1,000 medications on the account, every page renders within 2 s. The benchmark
lives in `internal/store/medication/list_bench_test.go` and seeds 1,000 rows; the Edge Case
"five thousand medications" is a second row in the same table asserting the last page is not
materially slower than the first.

---

## `listRecords` — `GET /api/v1/records`

The cross-kind list. Same envelope, same pagination, same authorization. Additional parameters:
`kind` (comma list of registered segments; absent means all), `from` and `to` (calendar dates
narrowing the kind's primary date).

In this phase there is one kind, so this operation returns the same rows as
`/api/v1/records/medications`. **It ships anyway**, because it is the operation the dashboard's
"recent changes" reads and because phases 003 and 004 make it the timeline and the report picker
— and because a route that appears in phase 003 is a phase-003 OpenAPI diff, whereas a route that
exists from the start is not. Its response items are the `oneOf` union, so a client written
against it today keeps working when thirteen kinds arrive.

---

## `createRecord` — `POST /api/v1/records/{kind}`

| Case | Status | Body |
|---|---|---|
| success | `201` + `Location: /api/v1/records/medications/{id}` + `ETag` | `Medication` |
| any invalid value | `422` | `validation_failed`, **every** offending field at once |
| body carries `owner`, `id`, `created` or `updated` | `422` | `validation_failed`, `unknown_field` |
| `{kind}` unregistered | `404` | `not_found` |
| anonymous | `401` | `unauthenticated` |

**FR-015**: a name alone is sufficient. **FR-027**: every problem is reported in one response,
each attached to its field, and the interface preserves what the person typed — the form is
re-rendered from the submitted values plus the field errors, never cleared.

**US1-4 is a named test**: an end date earlier than the start date **and** a blank name produce
**two** `fields[]` entries in one response, and the form still holds everything else that was
typed.

`owner` is set from the authenticated actor. Writes a `create` / `medication` audit row from the
post-commit hook, never from this handler.

---

## `getRecord` — `GET /api/v1/records/{kind}/{id}`

**200** — the full `Medication`, with an `ETag` derived from `updated` and
`Cache-Control: private, no-store`.

**404** — for an id that does not exist **and** for an id belonging to somebody else, with bodies
that are **byte-identical apart from `request_id`**. FR-033 and SC-006 make this a property the
tests assert directly rather than a behaviour the code is trusted to have. The refusal writes an
`access_denied` / `medication` audit row carrying the id **as addressed** — which is the only
record that the attempt happened, since the response itself discloses nothing.

**FR-024**: fields never filled in are absent from the response, not present and empty.

---

## `updateRecord` — `PATCH /api/v1/records/{kind}/{id}`

| Case | Status | Body |
|---|---|---|
| success | `200` + new `ETag` | the updated `Medication` |
| `If-Match` absent | `422` | `validation_failed`, field `If-Match`, code `required` |
| `If-Match` does not match | `412` | `version_mismatch`, **and the current representation** |
| any invalid value | `422` | `validation_failed` |
| body carries `owner` | `422` | `validation_failed`, `unknown_field` |
| not yours, or not there | `404` | `not_found` |
| anonymous | `401` | `unauthenticated` |

**FR-026 and US1-9 — the stale-save rule, which is the interesting one.** `If-Match` is
**required**, not merely honoured: an optional precondition is a precondition nobody sends. On a
mismatch the `412` body carries the server's current representation, so "the current values are
shown so they can decide what to do" is a property of the response rather than a second request
the page has to remember to make.

Through the interface this is one Datastar signal and one header: the detail page renders the
ETag into `$etag`, the form's action sends
`@patch('/api/v1/records/medications/{id}', {headers: {'If-Match': $etag}})`, and the `412`
response patches the form region with the current values plus a `role="alert"` explanation.
This closes shared design risk **R12** on medications before phases 002–005 apply the same
mechanism to eight more resources (research D-24).

**Only supplied fields change.** A `PATCH` with one field leaves the other twelve untouched; an
explicit `null` on `started_on` or `ended_on` clears it. Both are table rows.

Writes an `update` / `medication` audit row from the post-commit hook.

---

## `deleteRecord` — `DELETE /api/v1/records/{kind}/{id}`

| Case | Status |
|---|---|
| success | `204` |
| `If-Match` absent | `422 validation_failed`, field `If-Match` |
| `If-Match` does not match | `412 version_mismatch` |
| not yours, or not there | `404 not_found` |
| anonymous | `401 unauthenticated` |

**FR-028**: the deletion is permanent — no recycle bin, no undo — and the interface requires an
explicit confirmation that **names the medication** and states that it cannot be undone before the
request is made. That is a page-layer requirement and it has its own render test
(`contracts/pages.md`, **"Rendering rules that are contract, not style"** — the delete confirmation
is a rendered element carrying `region[name="Confirm delete"]`, never a `window.confirm`).

`If-Match` is required on delete for the same reason as on update: deleting the row you last saw
is a different act from deleting whatever is there now.

**The Edge Case this operation owns**: "One place deletes a medication while another has it open."
The second place's next action returns `404`, and the page layer turns that into a plain message
and a return to the list — not a failure page.

Writes a `delete` / `medication` audit row from the post-commit hook. The row survives the record
it describes, which is the point.

---

## Authorization tests, for all six operations

Driven by `testsupport.RunOwnershipMatrix`, the table-driven helper this phase creates and phases
002–005 extend by adding rows:

| Case | Expectation |
|---|---|
| owner, valid | the documented success |
| **a second account, valid id** | `404`, **byte-identical apart from `request_id`** to a non-existent id, plus one `access_denied` audit row |
| anonymous | `401`, body naming no resource |
| an id that never existed | `404` |
| a superuser through the auto-CRUD route `GET /api/collections/medications/records` | `200` for a superuser only |
| an ordinary account through the same route | **`404`** — the lockdown, asserted per collection |

FR-069 and SC-006 require this for **every** operation touching clinical data, which is all six.
