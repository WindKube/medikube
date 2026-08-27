# Contract: the record family, applied to the thirteen new kinds

**Operations added by this phase: 0.** The six operations below were registered by phase 001.
This phase adds thirteen values to `{kind}` and thirteen `oneOf` branches to their schemas.

```
GET    /api/v1/records                    listRecords        cross-kind
GET    /api/v1/records/{kind}             listRecordsOfKind
POST   /api/v1/records/{kind}             createRecord
GET    /api/v1/records/{kind}/{id}        getRecord
PATCH  /api/v1/records/{kind}/{id}        updateRecord
DELETE /api/v1/records/{kind}/{id}        deleteRecord
```

`{kind}` after this phase is an enum of fourteen plural kebab-case segments:
`medications` · `allergies` · `conditions` · `encounters` · `procedures` · `treatments` ·
`symptoms` · `vitals` · `immunizations` · `injuries` · `insurance` · `equipment` ·
`emergency-contacts` · `family-history`

An unrecognised `{kind}` is `404 not_found` — **not** `400` — so probing the segment reveals
nothing about which kinds exist on this instance.

---

## 1. `GET /api/v1/records/{kind}` — list one kind

### Query parameters

**Universal** (validated against the kind's declared filter allowlist; an unknown parameter is
`400 bad_request`, never ignored):

| Parameter | Type | Notes |
|---|---|---|
| `patient` | id | **required**. Absent → `400 patient_required`. No fallback to the active patient (FR-081, FR-070). |
| `q` | string | substring over the kind's declared searchable fields |
| `tags` | csv of tag ids | |
| `match` | `any` \| `all` | default `any` (FR-067) |
| `from`, `to` | `YYYY-MM-DD` | over the kind's primary date, inclusive |
| `sort` | csv, `-` prefix | from the kind's allowlist; default per `data-model.md` §3 |
| `limit` | int 1..100 | default 25 |
| `cursor` | opaque | |
| `count` | bool | adds `total` |

**Per-kind narrowings** (research D-05). Each declared on the registry entry; each maps to exactly
one status view in FR-078, and each returns exactly what the equivalent hand narrowing returns
(FR-079):

| Kind | Parameter | Meaning | Row `basis` |
|---|---|---|---|
| allergy | `status`, `severity`, `critical=true` | `critical` ≡ severity ∈ {severe, life_threatening} ∧ status ∈ {active, chronic} | `critical` |
| condition | `status`, `severity`, `active=true` | `active` ≡ status ∈ {active, chronic, healing} | — |
| medication | `status`, `active=true` | `active` ≡ status = active | — |
| encounter | `visit_type`, `priority`, `condition` | | — |
| procedure | `status`, `scheduled=true` | `scheduled` ≡ status ∈ {ordered, scheduled} | `scheduled` \| `ordered` |
| treatment | `status`, `ongoing=true` | `ongoing` ≡ status ∈ {active, on_hold} | — |
| symptom | `name`, `severity`, `status`, `is_chronic` | | — |
| vitals | *(dates only)* | | — |
| immunization | *(dates only)* | | — |
| injury | `status`, `severity`, `type`, `laterality`, `unresolved=true` | `unresolved` ≡ status ∈ {active, healing} | — |
| insurance | `type`, `status`, `is_primary`, `expiring_within_days` (default 60) | | `expiring` |
| equipment | `type`, `status`, `service_due_within_days` (default 30) | | `overdue` \| `due_soon` |
| emergency_contact | `is_active`, `is_primary` | | — |
| family_member | `relationship`, `is_deceased` | | — |

### Response `200`

```json
{
  "items": [ { "...": "<Kind>Summary" } ],
  "next_cursor": "eyJ...=" ,
  "total": 42,
  "criteria": { "patient": "rec123...", "status": ["active"], "tags": [], "match": "any" }
}
```

`criteria` is the server's **resolved** narrowing, echoed so the page can render removable chips
and so a status view can state its basis (FR-078, FR-079). `items` is `[]` and never `null`.

Every `<Kind>Summary` carries at minimum:

```json
{ "id": "…", "kind": "condition", "patient": "…",
  "title": "…", "occurred_on": "2024-03-11"|null, "occurred_at": null,
  "status": "active", "tags": [ {"id":"…","name":"…","color":"#aa3311"} ],
  "basis": [], "updated_at": "2026-08-26T10:00:00Z" }
```

plus that kind's own summary fields (`data-model.md` §4). `symptom` additionally carries
`episode_count` and `last_occurred_at` (FR-031). `allergy` additionally carries `critical` (FR-018).
`vitals` summary carries the measured values present and a derived `bmi` when both height and
weight are present (FR-037).

### Errors

`400 patient_required` · `400 bad_request` (unknown filter / bad cursor / bad sort key) ·
`401 unauthenticated` · `404 not_found` (unknown kind, or a patient the actor cannot reach —
identical response for both).

### Authorization

`Authorizer.Patient(actor, ?patient, PermView)`. A stranger's request is
`404 not_found` — byte-identical to the response for a patient id that does not exist.

---

## 2. `GET /api/v1/records` — cross-kind list (timeline, dashboard)

Same parameters, plus `kinds=` (csv of path segments; default = all registered kinds the actor may
see). Returns one flat page of mixed `<Kind>Summary` objects, discriminated by `kind`, ordered by
each row's primary date descending, `id` descending as tie-break, with **null primary dates last**
(FR-007, FR-077, research D-06).

Rows with a null primary date carry `"occurred_on": null` and the UI groups them under
"Date not recorded" — the API never invents a date. This is the operation `/timeline` renders
(research D-13). No new operation is added for the timeline.

---

## 3. `POST /api/v1/records/{kind}` — create

Body is `<Kind>Create` — every writable field, plus `patient` (required). **`id`, `created`,
`updated` and every server-owned field are absent from the type by construction**, which is how
privilege escalation is prevented structurally rather than by a check.

- `201` with `Location: /api/v1/records/{kind}/{id}` and the full `<Kind>` detail body.
- `422 validation_failed` with **every** offending field in `fields[]` (FR-004).
- `404 not_found` if `patient` is not reachable by the actor.
- `409 conflict` on a uniqueness violation.
- Setting `is_primary: true` on insurance or emergency_contact displaces the previous primary in
  one transaction and the response carries `"displaced": {"id":"…","kind":"insurance"}` (FR-045,
  FR-051, research D-16).

Authorization: `Authorizer.Patient(actor, body.patient, PermEdit)`.

---

## 4. `GET /api/v1/records/{kind}/{id}` — read

`200` with the full `<Kind>` detail DTO and `ETag: W/"<updated>"`.

Beyond the summary fields, the detail carries:

```json
{ "links": { "medications": [ {"id":"…","kind":"medication","title":"…"} ],
             "conditions": [ … ] },
  "back_links": { "encounters": [ … ], "procedures": [ … ] },
  "references": { "total": 3, "by_kind": { "medication": 2, "symptom": 1 } } }
```

- `links` are the kind's own stored multi-relation fields, resolved to `*Ref` summaries (FR-059:
  enough identifying detail to be recognised, and openable — never the full detail of the target).
- `back_links` are traversals; they are **read-only** and a `PATCH` containing them is
  `422 validation_failed` (`code: read_only`).
- `references` is the count used by the delete confirmation (FR-006, research D-17). No separate
  operation exists for it.

Errors: `404 not_found` for absent, for a foreign patient, and for a wrong-kind id — all identical.

---

## 5. `PATCH /api/v1/records/{kind}/{id}` — update

**`If-Match` is required.** Absent → `428 precondition_required`. Stale → `412 version_mismatch`,
and the `412` body carries the current detail DTO so the form can show current values without a
second request (FR-005, SC-009).

Body is `<Kind>Patch`: every field a pointer, so absent and explicit-null are distinguishable.
`patient` is **not** a member of the type; a body containing it is `422` (unknown field), and any
other attempt to re-file is `409 conflict` (FR-002).

Multi-relation fields are **replace-set** semantics: sending `"medications": ["a","b"]` sets the
set. Every id in it is validated for existence, reachability and same-patient before the write
(`data-model.md` §7.4); any failure is `404 not_found` disclosing nothing (FR-057).

`200` with the full detail and a new `ETag`.

---

## 6. `DELETE /api/v1/records/{kind}/{id}` — delete

**`If-Match` is required** (same rules as PATCH). `204` on success.

The delete is **hard** and there is no recovery path (spec: "Records are deleted permanently").
PocketBase's relation cleanup removes this record's id from every multi-relation field that
referenced it, and the `treatment_medications` rows cascade — so linked records survive with the
link gone and no dangling reference exists (FR-058, SC-006). This is asserted, not assumed:
`internal/store/<kind>/repo_test.go` proves it per kind.

Deleting the patient cascades to every record of every kind, every link and every
`search_index` row (FR-087, SC-005).

---

## 7. Live updates

A create, update or delete on any of these kinds publishes `realtime.Event{Kind, RecordID,
PatientID}` — **IDs only, never bodies** — from a post-commit hook. `GET /api/v1/streams/records`
(phase 001) re-fetches, **re-authorises for that subscriber**, renders `<Kind>Row` and patches by
`ids.RecordRow(kind, id)`. No new operation. FR-010 and SC-017 apply to all thirteen kinds.

---

## 8. Per-kind DTO surfaces

Four types per kind, in `internal/web/api/<kind>.go`:
`<Kind>Summary` (lists, search results, links), `<Kind>` (detail; embeds `<Kind>Summary`),
`<Kind>Create`, `<Kind>Patch`. Field-by-field content is `data-model.md` §4; the mapping is
one DTO field per stored field, with these deliberate exceptions:

| DTO field | Not stored — derived |
|---|---|
| `allergy.critical` | severity ∧ status (FR-018) |
| `symptom.episode_count`, `symptom.last_occurred_at` | aggregate over `(patient, LOWER(name))` (FR-031) |
| `vitals.bmi` | `weight_kg / (height_cm/100)²` (FR-037) |
| `vitals.*` in imperial | converted at the edge from SI storage (FR-037) |
| `*.references` | back-relation counts (FR-006) |
| `*.basis` | the narrowing that selected the row (FR-026/046/049/078) |
| `insurance.displaced`, `emergency_contact.displaced` | the primary displacement result (FR-045/051) |
