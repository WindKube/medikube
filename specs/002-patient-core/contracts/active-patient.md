# Contract: `PUT /api/v1/me/active-patient` and the `/api/v1/me` changes

Requirements covered: FR-013 … FR-020, FR-045, FR-052.

## The rule this file exists to enforce

**The person in view is never an authorization input.** `users.active_patient` is read in exactly
three places, all presentation:

1. rendering the switcher's current selection,
2. redirecting a bare `/medications` to `/medications?patient={active}`,
3. pre-filling the `patient` field on a create form.

No `/api/v1` handler consults it. Every list takes `?patient=`; every create takes `patient` in the
body; every read/update/delete derives it from the stored record. A list without `?patient=` is
**400 `patient_required`**, never a fallback (FR-015, FR-016, US3-5, research D-08).

---

## `setActivePatient` — `PUT /api/v1/me/active-patient`

**Body**

```json
{ "patient": "pat_abc123def4567" }   // or { "patient": null } to clear
```

**200**

```json
{ "active_patient": { /* PatientSummary */ } }   // or { "active_patient": null }
```

Idempotent whole-value replace, hence `PUT`. Collapses upstream's `/switch`, `/active/current`,
`/self-record` and `/owned/list` into one operation (SHARED-DESIGN §2.3 route 12).

| Status | When | Requirement |
|---|---|---|
| 200 | the pointer is set (or cleared) | FR-013 |
| 404 `not_found` | the id does not exist or the actor does not own it. **The pointer is unchanged.** | FR-020, US3-5 |
| 422 | body is not `{"patient": string\|null}`, or carries an unknown field | |
| 401 | no session | FR-043 |

**The target is authorized before the pointer is written** (FR-020) and the change writes a
`switch_patient`/`patient` audit row (FR-045). Because authorization happens here *and* on every
subsequent read, a pointer that somehow became stale still grants nothing (US3-5).

**HTML negotiation.** When the request `Accept`s `text/html` (the switcher's Datastar `@put`), the
same handler responds with the re-rendered `@PatientSwitcher` component as plain `text/html`, which
Datastar treats as an element patch. No SSE, no inline script, no Pro attribute (research D-33).

**Mandatory tests**

- Set → read `GET /api/v1/me` → the pointer is there (FR-013).
- Sign out, sign in, `GET /api/v1/me` → still there (US3-2, SC-014).
- Account B setting Account A's patient → 404, and B's pointer is unchanged (US3-5).
- **The authorization-independence test**: set the pointer to patient X, then
  `GET /api/v1/records/medications?patient={Y}` where Y belongs to another account → 404. Changing
  the selection grants nothing (FR-015, US3-5).

---

## `getMe` — `GET /api/v1/me` (amended)

Adds to phase 001's response:

```json
{ "active_patient": { /* PatientSummary */ },   // null when unset or unreachable
  "patients": { "owned_count": 3 } }
```

**Resolution rules**

- The pointer resolves to `null` when the patient no longer exists — which PocketBase has already
  done for us by unsetting the relation on delete (research D-07) — or when the actor no longer
  owns it. FR-017.
- **FR-018 auto-selection**: when `active_patient` is null and the actor can reach **exactly one**
  patient, the response returns that one and the pointer is persisted as a side effect, so a new
  account never faces an empty application (US3-4). With two or more reachable patients and a null
  pointer, `active_patient` stays null and the UI sends the user to `/patients`.

## `updateMe` — `PATCH /api/v1/me` (amended)

Unchanged in shape. `active_patient` is **not** settable here — it has its own operation, so the
audit action and the authorization check live in one place. A `PATCH /api/v1/me` carrying
`active_patient` is `422 unknown_field`.

`unit_system` (from phase 001) becomes load-bearing this phase: it drives the `display` block on
every patient response (FR-007). Changing it must never alter `height_cm`/`weight_kg` — a test
reads a patient, patches the preference, re-reads and asserts the canonical numbers are identical
and only `display` changed (US4-3).

---

## Page-layer behaviour (not an API operation, but part of this contract)

| Situation | Behaviour | Requirement |
|---|---|---|
| pointer null, exactly one reachable patient | auto-select, render | FR-018, US3-4 |
| pointer null, several reachable patients | redirect to `/patients` with an explanation | FR-017 |
| pointer set but patient now unreachable | pointer reads as null → redirect to `/patients` with an explanation, **never another person's data** | FR-017, US3-3 |
| `/medications` requested with no `?patient=` | `303` to `/medications?patient={active}`; if there is no active patient, `303` to `/patients` | FR-016 |
| any person-scoped page | renders the patient's name (and photo) inside `#main` | FR-019, SC-003 |
