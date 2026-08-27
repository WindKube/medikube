# Contract: `/api/v1/practitioners`

Five operations. Requirements covered: FR-032, FR-033, FR-036 … FR-040.

The directory belongs to the **account**, not to a patient and not to the installation (spec
Assumptions). The authorization anchor is therefore `owner == actor.UserID`, and the refusal shape
is identical to the patient one: **404, indistinguishable from not existing** (FR-037, US5-6).

## DTOs

```go
type PractitionerSummary struct {
    ID        string       `json:"id"`
    Name      string       `json:"name"`
    Specialty string       `json:"specialty,omitempty"`
    Facility  *FacilityRef `json:"facility"`
    UpdatedAt string       `json:"updated_at"`
}

type Practitioner struct {
    PractitionerSummary
    Phone   string `json:"phone,omitempty"`
    Email   string `json:"email,omitempty"`
    Website string `json:"website,omitempty"`
    Notes   string `json:"notes,omitempty"`
    Usage   Usage  `json:"usage"`             // detail only
}

// Usage answers FR-040 without a second round trip (research D-26).
type Usage struct {
    Patients int `json:"patients"`   // profiles naming this practitioner as primary
    Records  int `json:"records"`    // clinical records referencing them (medications in this phase)
}

type FacilityRef struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Kind string `json:"kind"`
}

type PractitionerCreate struct {
    Name      string `json:"name"`                  // required, 1..200
    Specialty string `json:"specialty,omitempty"`   // must be in the vocabulary when non-empty
    Facility  string `json:"facility,omitempty"`
    Phone     string `json:"phone,omitempty"`
    Email     string `json:"email,omitempty"`
    Website   string `json:"website,omitempty"`
    Notes     string `json:"notes,omitempty"`
}

type PractitionerPatch struct {
    Name      *string  `json:"name,omitempty"`
    Specialty *string  `json:"specialty,omitempty"`
    Facility  **string `json:"facility,omitempty"`   // inner nil clears the reference
    Phone     *string  `json:"phone,omitempty"`
    Email     *string  `json:"email,omitempty"`
    Website   *string  `json:"website,omitempty"`
    Notes     *string  `json:"notes,omitempty"`
}
```

`owner` appears in neither write DTO.

## The specialty vocabulary

A fixed Go enum of 42 values including the catch-all `other`, mirrored into a
`core.SelectField{MaxSelect: 1}` from the same source of truth. **It is not extensible by ordinary
use** (FR-033) and there is **no endpoint that serves it** — the values are compiled into the
binary and rendered directly into the form's `<select>` (research D-23). A value outside the
vocabulary is `422 validation_failed` with code `invalid_value`, never stored as free text.

---

## `listPractitioners` — `GET /api/v1/practitioners`

**Query**: `?q=` (case-insensitive substring of `name`), `?specialty=`, `?facility=`, `?limit=`,
`?cursor=`. **Sort**: `name, id`.

**200** `{ "items": [PractitionerSummary], "next_cursor": … }` (`total` only with `?count=true`).

This one operation serves the directory page **and** the type-ahead behind every practitioner
picker (FR-039). `?q=` with a short prefix and `?limit=10` is the autocomplete call; there is no
separate `/search` or `/autocomplete` operation.

| Status | When |
|---|---|
| 200 | always (an empty directory returns `items: []`, never `null`) |
| 400 `invalid_cursor` | bad cursor |
| 401 | no session |

**Isolation is a test, not a comment** (FR-037, US5-6, SC-014): Account B's list never contains an
Account A entry, and `?q=` matching an Account A name returns `[]`.

---

## `createPractitioner` — `POST /api/v1/practitioners`

**201** `Practitioner` + `Location` + `ETag`.

| Status | When | Requirement |
|---|---|---|
| 201 | created | FR-032, US5-1 |
| 409 `conflict` | an entry with the same `owner`, the same `LOWER(name)` and the same `specialty` already exists; the message explains which | FR-038, US5-4 |
| 422 | `name` missing/too long, `specialty` outside the vocabulary, malformed email/website, unknown field | FR-032, FR-033 |
| 404 | `facility` names a facility the actor does not own | FR-042 |
| 401 | no session | |

**The uniqueness test that must not be forgotten**: two practitioners with the same name and **no
specialty at all** → the second is 409. SQLite treats NULLs as distinct in a unique index, so this
only works because the select field stores `''` and never `NULL` (research D-25). The test exists
to catch a future field-type change that would silently disable FR-038.

Also creatable inline from a record form without losing the record being written (FR-039) — that is
a UI behaviour built on this same operation: the drawer `@post`s here, then patches the picker.

---

## `getPractitioner` — `GET /api/v1/practitioners/{id}`

**200** `Practitioner` (including `usage`) + `ETag`.
**404** when not owned or not existing. **401** when unauthenticated.

`usage` is two indexed `COUNT(*)`s — on `patients (primary_practitioner)` and
`medications (practitioner)`. It exists so the delete confirmation can state the number FR-040
requires without a second round trip.

---

## `updatePractitioner` — `PATCH /api/v1/practitioners/{id}`

`If-Match` **required**. **200** + new `ETag`.

| Status | When |
|---|---|
| 200 | applied |
| 409 | the edit would collide with the `(owner, LOWER(name), specialty)` uniqueness |
| 412 | `If-Match` mismatch; body carries the current representation |
| 422 | invalid or unknown field |
| 404 | not owned / does not exist |

---

## `deletePractitioner` — `DELETE /api/v1/practitioners/{id}`

`If-Match` **required**. **204**.

**Every referencing record survives with the reference cleared.** This is PocketBase's
`deleteRefRecords` behaviour, not MediGo code: a non-cascading, non-required relation has the id
unset and the referencing record re-saved (`core/record_model.go:1618-1626`, research D-06). It
applies to `patients.primary_practitioner` and `medications.practitioner`.

| Status | When | Requirement |
|---|---|---|
| 204 | deleted; references cleared | FR-040, US5-5 |
| 412 | `If-Match` mismatch | |
| 404 | not owned / does not exist | FR-037 |
| 401 | no session | |

**Mandatory tests**
- Create a practitioner, name it as a patient's `primary_practitioner` and as a medication's
  `practitioner`, delete it → both records still exist, both references are now empty (US5-5).
- The `usage` numbers reported before the delete equal the number of records actually modified.
- Deleting one account's practitioner never touches another account's identically named one.

**Documented side effect.** The unset path calls `SaveNoValidate`, which fires the update hooks, so
clearing the reference on N records writes N `update` audit rows. That is correct — those records
did change — and the number equals the `usage.records` the user was warned about.
