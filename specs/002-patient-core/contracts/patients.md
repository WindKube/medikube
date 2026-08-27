# Contract: `/api/v1/patients`

Five operations. Requirements covered: FR-001 … FR-012, FR-041, FR-042, FR-048 … FR-053.

## Shared DTOs

```go
// internal/web/api — list rows
type PatientSummary struct {
    ID           string  `json:"id"`
    FirstName    string  `json:"first_name"`
    LastName     string  `json:"last_name"`
    BirthDate    *string `json:"birth_date"`            // "YYYY-MM-DD"; null only on an unprovisioned self-record
    Age          *string `json:"age"`                   // "7 years", "3 months", "0 days"; null when birth_date is null
    IsSelfRecord bool    `json:"is_self_record"`
    Relationship string  `json:"relationship_to_owner,omitempty"`
    PhotoURL     *string `json:"photo_url"`             // "/api/v1/patients/{id}/photo?size=100x100t" or null
    UpdatedAt    string  `json:"updated_at"`            // RFC3339 UTC
}

type Patient struct {
    PatientSummary
    Sex                 string           `json:"sex,omitempty"`
    BloodType           string           `json:"blood_type,omitempty"`
    HeightCm            *float64         `json:"height_cm"`            // canonical SI, always
    WeightKg            *float64         `json:"weight_kg"`            // canonical SI, always
    Address             string           `json:"address,omitempty"`
    PrimaryPractitioner *PractitionerRef `json:"primary_practitioner"`
    Display             Display          `json:"display"`
}

// Display is computed from the actor's unit_system. The recorded value never changes (FR-007).
type Display struct {
    UnitSystem string `json:"unit_system"`               // "metric" | "imperial"
    Height     string `json:"height,omitempty"`          // "175 cm" | "5 ft 9 in"
    Weight     string `json:"weight,omitempty"`          // "70.5 kg" | "155 lb 7 oz"
}

type PractitionerRef struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Specialty string `json:"specialty,omitempty"`
}

type PatientCreate struct {                              // no `owner`, no `is_self_record`
    FirstName           string   `json:"first_name"`     // required
    LastName            string   `json:"last_name"`      // required
    BirthDate           string   `json:"birth_date"`     // required, "YYYY-MM-DD"
    Sex                 string   `json:"sex,omitempty"`
    BloodType           string   `json:"blood_type,omitempty"`
    HeightCm            *float64 `json:"height_cm,omitempty"`
    WeightKg            *float64 `json:"weight_kg,omitempty"`
    Address             string   `json:"address,omitempty"`
    Relationship        string   `json:"relationship_to_owner,omitempty"`
    PrimaryPractitioner string   `json:"primary_practitioner,omitempty"`
}

type PatientPatch struct {                               // no `owner`, no `is_self_record`
    FirstName           *string   `json:"first_name,omitempty"`
    LastName            *string   `json:"last_name,omitempty"`
    BirthDate           *string   `json:"birth_date,omitempty"`
    Sex                 *string   `json:"sex,omitempty"`
    BloodType           *string   `json:"blood_type,omitempty"`
    HeightCm            **float64 `json:"height_cm,omitempty"`   // **T: outer nil absent, inner nil explicit null
    WeightKg            **float64 `json:"weight_kg,omitempty"`
    Address             *string   `json:"address,omitempty"`
    Relationship        *string   `json:"relationship_to_owner,omitempty"`
    PrimaryPractitioner **string  `json:"primary_practitioner,omitempty"`
}
```

**`owner` and `is_self_record` are absent from both write DTOs.** FR-002 (ownership immutable) and
FR-004 (at most one self-record) are therefore enforced by *shape*, not by a runtime check that
somebody can forget. A request carrying either field is rejected `422 validation_failed` with
`unknown_field`, because unknown fields are rejected.

---

## `listPatients` — `GET /api/v1/patients`

**Query**: `?q=` (case-insensitive substring over first+last name), `?limit=` (1..100, default 25),
`?cursor=`.

**Sort**: `last_name, first_name, id` ascending. Not configurable in this phase.

**200**
```json
{ "items": [ /* PatientSummary */ ],
  "next_cursor": null,
  "total": 3,
  "owned_count": 3 }
```
`total` is returned **unconditionally** on this endpoint, not only under `?count=true` — FR-010
requires the list to state how many there are, and a household is tens of rows (research D-29).
`owned_count` is present so phase 005 can add `shared_count` without changing the envelope.

**Authorization**: only patients where `owner == actor.UserID`. FR-037/FR-042: an account never
learns another account's patients exist. A test asserts Account B's list contains none of Account
A's ids.

| Status | When |
|---|---|
| 200 | always, for a signed-in account (never empty in practice — the self-record guarantees one row, FR-005) |
| 400 `invalid_cursor` | cursor fails HMAC verification or decodes to a different sort |
| 401 | no session |

---

## `createPatient` — `POST /api/v1/patients`

**Body**: `PatientCreate`. **Response 201** `Patient` + `Location: /api/v1/patients/{id}` + `ETag`.

`owner` is set from the authenticated actor. `is_self_record` is always `false` — the self-record
is created only by registration and by the migration (FR-005, research D-10).

| Status | When | Requirement |
|---|---|---|
| 201 | created | FR-001, US1-2 |
| 422 `validation_failed` | any invalid field; **every** offending field is listed in `fields[]` in one response | FR-003, US1-3 |
| 422 `unknown_field` | body carries `owner`, `is_self_record`, or anything not in the DTO | FR-002, FR-004 |
| 404 `not_found` | `primary_practitioner` names a practitioner the actor does not own | FR-042 |
| 401 | no session | FR-043 |

**Mandatory tests**
- Four simultaneous faults → four `fields[]` entries in one response (US1-3).
- `birth_date` = tomorrow → `date_in_future`; `birth_date` = today − 151y → `date_too_old`.
- A body with `"is_self_record": true` → `422 unknown_field`, and the account's existing
  self-record is unchanged (US1-4).
- Creating with a practitioner id belonging to Account B → 404, byte-identical to a random id.

---

## `getPatient` — `GET /api/v1/patients/{id}`

**200** `Patient` + `ETag: "<updated>"`.

Absent optional details are `null` or omitted — **never** `""`, `0`, or a value that reads as
recorded (FR-030, US1-6, and the Edge Case "must not present an absent blood type as an
unknown-but-recorded value").

| Status | When |
|---|---|
| 200 | the actor owns the patient |
| 404 `not_found` | the id does not exist **or** belongs to another account — indistinguishable (FR-042, US1-8, SC-005). Audited. |
| 401 | no session |

---

## `updatePatient` — `PATCH /api/v1/patients/{id}`

**Headers**: `If-Match: "<etag>"` — **required**.
**Body**: `PatientPatch`. **200** `Patient` + a new `ETag`.

| Status | When | Requirement |
|---|---|---|
| 200 | applied | FR-012 |
| 412 `version_mismatch` | `If-Match` does not match the stored `updated`. The body carries the **current** representation so the UI can show it. | FR-011, US1-7 |
| 428 → **not used**; a missing `If-Match` is `422 validation_failed` with field `If-Match`, code `required` | | keeps one error taxonomy |
| 422 | invalid field values, or an unknown field | FR-003 |
| 404 | not owned / does not exist | FR-042 |

An update writes an `update`/`patient` audit row (FR-012, FR-045).

---

## `deletePatient` — `DELETE /api/v1/patients/{id}`

**Headers**: `If-Match: "<etag>"` — **required**.
**204** on success, empty body.

Runs inside `app.RunInTransaction`. The cascade destroys every medication attributed to the
patient and the photograph goes with the record (FR-026, FR-049, SC-010). `users.active_patient`
is unset automatically on every account pointing at the patient (FR-052, research D-07).

| Status | When | Requirement |
|---|---|---|
| 204 | deleted | FR-049 |
| 409 `conflict` | the target has `is_self_record = true`, message: closing the account is what removes it | FR-051, US6-4 |
| 412 | `If-Match` mismatch | Edge case "simultaneous edits" |
| 404 | not owned / does not exist. **Nothing is deleted.** | FR-050, US6-6, SC-005 |
| 401 | no session | |

**The confirmation is the UI's job, and its numbers come from `getPatientChart`** — FR-048 requires
the person's name and the count of records to be shown before anything is removed, and the chart
summary already carries both (research D-26). **There is no preview endpoint.**

**Mandatory tests**
- Delete a patient with 3 medications → 204; `SELECT COUNT(*) FROM medications WHERE patient='<id>'`
  is 0; no medication anywhere references the deleted id (US6-3, SC-010).
- The photo file and its two thumbnails are gone from the filesystem (US6-2).
- One `delete`/`patient` audit row exists, carrying **no** name and no record content (US6-5,
  SC-009).
- Account B deleting Account A's patient → 404 and Account A's patient still exists (US6-6).
