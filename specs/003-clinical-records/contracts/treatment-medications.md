# Contract: course medications — the one payload-carrying join

**Operations added: 3.** Shared design contract §2.3 entries 46–48.

```
GET    /api/v1/treatments/{id}/medications                   listCourseMedications
PUT    /api/v1/treatments/{id}/medications/{medicationId}    upsertCourseMedication
DELETE /api/v1/treatments/{id}/medications/{medicationId}    deleteCourseMedication
```

Nesting depth is one, which is the maximum the shared design contract permits. These three replace
upstream's create/update/bulk-create trio **and** its mirror-image routes on the medication side.

---

## 1. `GET /api/v1/treatments/{id}/medications`

`200`:

```json
{ "items": [ {
    "medication": { "id":"…","kind":"medication","title":"Warfarin","status":"active" },
    "effective_dosage":     { "value": "3mg",  "source": "course" },
    "effective_frequency":  { "value": "daily","source": "medication" },
    "effective_duration":   { "value": "6 weeks", "source": "course" },
    "effective_timing":     { "value": null,  "source": "none" },
    "effective_prescriber": { "value": {"id":"…","name":"Dr Okafor"}, "source": "medication" },
    "effective_pharmacy":   { "value": null,  "source": "none" },
    "effective_started_on": { "value": "2026-03-02", "source": "course" },
    "effective_ended_on":   { "value": null,  "source": "none" },
    "updated_at": "2026-08-26T10:00:00Z"
  } ],
  "next_cursor": null }
```

**`source` is the contract's whole point.** FR-060 requires the screen to say which values came
from the course and which from the medication; a bare `COALESCE` loses that, so every effective
field is a `{value, source}` pair with `source ∈ {course, medication, none}`. The resolution runs in
`internal/service/coursemedication`, not in SQL and not in the browser (research D-09).

Ordering: `medication.name ASC, id ASC`. Authorization:
`Authorizer.Record(actor, kind.Treatment, id, PermView)`.
Errors: `401` · `404 not_found` (absent treatment, or one the actor cannot reach — identical).

---

## 2. `PUT /api/v1/treatments/{id}/medications/{medicationId}`

Idempotent upsert of the link and its payload. **`PUT`, not `POST`**, because attaching the same
medication to the same course twice must update rather than duplicate (FR-061, US6 scenario 6).

Body — every field optional; each absent field means "fall back to the medication's own value":

```json
{ "dosage": "3mg", "frequency": null, "duration": "6 weeks", "timing": null,
  "prescriber": "rec_practitioner_id", "pharmacy": null,
  "started_on": "2026-03-02", "ended_on": null }
```

- `200` on update, `201` with `Location` on create. Body is the same shape as one `items` entry.
- Runs inside `app.RunInTransaction`; guarded by unique index `(treatment, medication)`.
- `422 validation_failed` if `ended_on < started_on`, or on an unknown field.
- **`404 not_found`** if the medication does not exist, is not reachable, **or belongs to a
  different patient than the treatment** — one identical response for all three (FR-057, D-08).
  The response discloses nothing about the other record, including whether it exists.
- `412 version_mismatch` when `If-Match` (the treatment's ETag) is stale — the spec's Edge Cases
  section applies the concurrency rule to attaching and detaching a link, not only to records.

Authorization: `Authorizer.Record(actor, kind.Treatment, id, PermEdit)` **and**
`Authorizer.Record(actor, kind.Medication, medicationId, PermEdit)`. Both, every time.

---

## 3. `DELETE /api/v1/treatments/{id}/medications/{medicationId}`

`204`. Removes the link row only; **both the treatment and the medication survive untouched**
(FR-058). `If-Match` on the treatment required. Deleting either side cascades the link row away,
which is what `CascadeDelete: true` on both relations buys.

Errors: `401` · `404 not_found` · `412 version_mismatch` · `428 precondition_required`.

---

## 4. Tests this contract requires

| Test | Requirement |
|---|---|
| upsert twice → one row, second call `200` not `201` | FR-061, US6-6 |
| absent field falls back, and `source` says `medication` | FR-060, US6-5 |
| present field wins, and `source` says `course` | FR-060 |
| neither present → `{"value": null, "source": "none"}` | FR-060 |
| medication of another patient → `404`, body identical to a random id's `404` | FR-057, SC-004 |
| stranger on all three operations → `404` | FR-092 |
| delete the medication → the link disappears, the treatment survives | FR-058, SC-006 |
| delete the treatment → the link disappears, the medication survives | FR-058, SC-006 |
| delete the patient → treatment, medication and link all gone | FR-087, SC-005 |
| audit rows written for upsert and delete, carrying no dosage and no medication name | FR-084/085 |
