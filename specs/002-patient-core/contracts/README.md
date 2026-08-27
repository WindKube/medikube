# Phase 002 API Contracts

These files are what the contract tests are written against. They follow SHARED-DESIGN §2.1
without exception:

- Base path `/api/v1`. **No trailing slashes** — PocketBase has done no trailing-slash
  normalisation since v0.23, and the route registry rejects a path ending in `/`.
- Plural kebab-case resource segments; path parameters are `{id}`.
- Nesting depth at most one.
- **Patient scope is explicit and mandatory.** Every list over patient-scoped data requires
  `?patient=`; its absence is `400 patient_required`, never a fallback to the person in view.
- Cursor pagination: `?limit=` (default 25, max 100), `?cursor=` (opaque, HMAC-signed).
  Envelope `{"items":[…],"next_cursor":…}`; `total` only when `?count=true` — with the one
  documented exception on `GET /api/v1/patients` (research D-29).
- Filtering is explicit named parameters. **PocketBase's filter DSL never reaches the wire.**
- No `?fields=`. List endpoints return `*Summary`; detail endpoints return the full DTO.
- `POST` → `201` + `Location`. `PATCH` → `200`. `PUT` → `200`/`204`. `DELETE` → `204`.
- **`ETag` on every read; `If-Match` required on `PATCH` and `DELETE`; mismatch is `412`.**
- `Content-Type: application/json` in and out, except the multipart upload and the photo download.
  **Unknown JSON fields are rejected (`422`)**, not ignored. Slices marshal as `[]`, never `null`.
- Every operation has a stable `operationId`, asserted by the Principle IX gate to exist in both
  the route registry and `api/openapi.json`.

## The error envelope, on every non-2xx

```json
{ "error": {
    "code": "validation_failed",
    "message": "human-readable, PHI-free",
    "request_id": "…",
    "fields": [ { "field": "birth_date", "code": "date_in_future", "message": "…" } ] } }
```

| Sentinel | Status | `code` |
|---|---|---|
| `ErrUnauthenticated` | 401 | `unauthenticated` |
| `ErrNotFound` **and every authorization failure on patient-scoped data** | 404 | `not_found` |
| `ErrVersionMismatch` | 412 | `version_mismatch` |
| `*ValidationError` | 422 | `validation_failed` (+ `fields[]`) |
| `ErrConflict` | 409 | `conflict` |
| `ErrTooLarge` | 413 | `payload_too_large` |
| `ErrUnsupportedMedia` | 415 | `unsupported_media_type` |
| missing `?patient=` on a scoped list | 400 | `patient_required` |
| anything else | 500 | `internal_error`, message always the literal `"internal error"` |

## The universal authorization rule for this phase

Every operation below, except where explicitly noted, resolves as:

1. no valid session → **401 `unauthenticated`**, no patient information in the body;
2. `access.Authorizer.Patient(ctx, actor, patientID, need)` where `patientID` comes from the
   **request** (query, body, or the stored record) and never from `users.active_patient`;
3. the actor owns the patient → allowed;
4. otherwise → **404 `not_found`**, byte-identical (apart from `request_id`) to the response for an
   id that never existed, and an audit row is written.

For `practitioners` and `facilities` the anchor is `owner == actor.UserID` rather than patient
ownership; the refusal shape is identical.

## Operation inventory (20 new)

| operationId | Method | Path | File |
|---|---|---|---|
| `listPatients` | GET | `/api/v1/patients` | patients.md |
| `createPatient` | POST | `/api/v1/patients` | patients.md |
| `getPatient` | GET | `/api/v1/patients/{id}` | patients.md |
| `updatePatient` | PATCH | `/api/v1/patients/{id}` | patients.md |
| `deletePatient` | DELETE | `/api/v1/patients/{id}` | patients.md |
| `putPatientPhoto` | PUT | `/api/v1/patients/{id}/photo` | patient-photo.md |
| `getPatientPhoto` | GET | `/api/v1/patients/{id}/photo` | patient-photo.md |
| `deletePatientPhoto` | DELETE | `/api/v1/patients/{id}/photo` | patient-photo.md |
| `getPatientChart` | GET | `/api/v1/patients/{id}/summary` | patient-chart.md |
| `setActivePatient` | PUT | `/api/v1/me/active-patient` | active-patient.md |
| `listPractitioners` | GET | `/api/v1/practitioners` | practitioners.md |
| `createPractitioner` | POST | `/api/v1/practitioners` | practitioners.md |
| `getPractitioner` | GET | `/api/v1/practitioners/{id}` | practitioners.md |
| `updatePractitioner` | PATCH | `/api/v1/practitioners/{id}` | practitioners.md |
| `deletePractitioner` | DELETE | `/api/v1/practitioners/{id}` | practitioners.md |
| `listFacilities` | GET | `/api/v1/facilities` | facilities.md |
| `createFacility` | POST | `/api/v1/facilities` | facilities.md |
| `getFacility` | GET | `/api/v1/facilities/{id}` | facilities.md |
| `updateFacility` | PATCH | `/api/v1/facilities/{id}` | facilities.md |
| `deleteFacility` | DELETE | `/api/v1/facilities/{id}` | facilities.md |

Amended (no new operationId): `getMe`, `updateMe`, the six `records`/`medication` operations, and
`GET /api/v1/streams/records`. See `medications-rescope.md` and `active-patient.md`.
