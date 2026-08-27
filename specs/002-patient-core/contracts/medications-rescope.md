# Contract: changes to phase 001's medication operations

**No new operationIds.** Phase 001's six record operations keep their paths and their names; this
file records exactly how their inputs, outputs and authorization change once a medication belongs
to a patient rather than to an account. Requirements covered: FR-021 … FR-026, FR-016, FR-019,
FR-023, SC-006, SC-007.

| Method | Path | Change |
|---|---|---|
| GET | `/api/v1/records` | `?patient=` becomes **required** |
| GET | `/api/v1/records/medications` | `?patient=` becomes **required** |
| POST | `/api/v1/records/medications` | body gains a **required** `patient`; gains optional `practitioner`, `pharmacy` |
| GET | `/api/v1/records/medications/{id}` | response gains `patient`, `practitioner`, `pharmacy` |
| PATCH | `/api/v1/records/medications/{id}` | patch DTO gains `practitioner`, `pharmacy`; **has no `patient` field** |
| DELETE | `/api/v1/records/medications/{id}` | authorization anchor moves from `owner` to `patient.owner` |
| GET | `/api/v1/streams/records` | `?patient=` becomes **required**; per-event re-authorisation is per patient |

## Authorization moves one hop

Before: `medication.owner == actor.UserID`.
After: `access.Authorizer.Patient(ctx, actor, medication.patient, need)` — the patient's `owner`.
The refusal is unchanged in shape: **404, indistinguishable from not existing, audited**
(FR-042, US2-5).

## Lists

`?patient=` is required on every list. Its absence is **400 `patient_required`**, never an implicit
fallback to `users.active_patient` (FR-016, FR-023, research D-08). The handler authorizes the
patient *before* touching the medication collection, so a list for another account's patient is a
404 that never runs a query.

The default sort becomes `-started_on, -created`, backed by
`idx_medications_patient_start (patient, started_on DESC, id)`.

**Mandatory tests**

- Patient X has 3 medications, patient Y (same account) has 2 → the list for X returns exactly 3
  and none of Y's (US2-2, SC-007).
- No `?patient=` → 400, and the response mentions no patient (FR-016).
- `?patient=` naming another account's patient → 404 (US2-5).

## Creates

`MedicationCreate.patient` is required. A create with an absent or empty `patient` is **422
`validation_failed`** with field `patient`, code `required` — the record is not created and the
message asks which person it is for (FR-021, US2-3).

**FR-025 in full**: the create form renders the target patient as a visible name **and** a hidden
`patient` field, both fixed at page-render time and defaulted to the person in view. The handler
files the record against the submitted value. Changing the person in view in another window
afterwards changes nothing about the submitted form (US3-6). A test opens the form for patient X,
switches the pointer to Y, submits, and asserts the medication is on X.

## Updates: re-attribution is impossible by shape

`MedicationPatch` **has no `patient` field**. Because unknown fields are rejected (`422
unknown_field`), a request attempting to re-file a medication is refused by the decoder before any
business code runs, and the medication remains attributed to its original person (FR-024, US2-4).

This is deliberate: SHARED-DESIGN §5.5 records that DTO shape — not a runtime check — is what
closed the same class of bug upstream. The test asserts both the 422 *and* that the stored record
is unchanged.

## Deletes

Unchanged in shape. The new behaviour is at the other end: deleting a **patient** destroys their
medications through `medications.patient`'s `CascadeDelete: true`, in one transaction (FR-026,
SC-010).

## The live stream

`GET /api/v1/streams/records?patient={id}&kind=medication`.

- `?patient=` required; absent → 400 before the stream opens.
- The hub still publishes `{Kind, RecordID, PatientID}` — **IDs, never bodies** (constitution V).
- The subscriber handler filters on patient, then **re-runs `access.Authorizer.Patient` for that
  subscriber on every single event** before re-fetching, rendering and patching. A subscriber who
  loses access mid-stream stops receiving patches without the stream erroring.
- The 5-minute `WriteTimeout` override from phase 001 is unchanged and re-verified by this phase's
  regression test.

**Mandatory test** (phase 001's assumption, now enforceable): two sessions, two accounts, each
streaming their own patient; a write on one produces no frame on the other.

## The data migration

`1756200600_medications_repoint.go`. Every medication recorded before this phase is attributed to
the person representing its recording account holder — the self-record patient, created by the same
migration for every account that lacks one (FR-022, FR-005).

**Acceptance, asserted by an integration test against a database seeded in the phase-001 shape**
(SC-006):

```sql
SELECT COUNT(*) FROM medications WHERE patient = '' OR patient IS NULL;          -- must be 0
SELECT COUNT(*) FROM medications m JOIN patients p ON p.id = m.patient
  WHERE p.is_self_record = 0;                                                     -- must be 0
-- and, before/after: the total medication count is unchanged, per account
```

None lost, none duplicated, none unattributed — and because all pending migrations share one
transaction (`core/migrations_runner.go:129-131`), a failure of any assertion rolls the entire
batch back rather than leaving a half-migrated database (research D-13).
