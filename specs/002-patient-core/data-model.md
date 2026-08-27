# Phase 1 Data Model: Patient Core

Consistent with SHARED-DESIGN §1.0–§1.3. Every rule in §1.0 holds without exception:
15-character opaque PocketBase text ids; `id`/`created`/`updated` on every collection and not
repeated below; every one of the five API rules `nil` on every collection; every `FileField`
`Protected: true`; every enum is both a `core.SelectField{MaxSelect: 1}` and a Go string type with
`Valid()`, generated from one source of truth; all enum values `snake_case`; **no `deleted_at`
anywhere**; uniqueness is always a collection index.

## 0. What this phase changes, at a glance

| Collection | Change | Migration |
|---|---|---|
| `facilities` | **new** | `1756200100_facilities.go` |
| `practitioners` | **new** | `1756200200_practitioners.go` |
| `patients` | **new** | `1756200300_patients.go` |
| `users` | + `active_patient` | `1756200400_users_active_patient.go` |
| `audit_events` | + `patient`, + index | `1756200500_audit_events_patient.go` |
| `medications` | + `patient` (required), + `practitioner`, + `pharmacy`, − `owner`; data backfill | `1756200600_medications_repoint.go` |

Running total of collections after this phase: **6** (`users`, `medications`, `audit_events` from
phase 001, plus the three above).

---

## 1. `facilities` — new

A place where care happens. One collection for all six kinds (research D-24).

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `owner` | relation → `users` | **yes** | `MaxSelect: 1`, `CascadeDelete: true` | FR-037: the directory is the account's own. Closing the account destroys it. |
| `kind` | select | **yes** | `MaxSelect: 1`, values below | FR-034 |
| `name` | text | **yes** | 1..200 | PHI-adjacent; redacted in log marshalling |
| `brand` | text | no | ≤120 | e.g. the chain a branch belongs to |
| `street` | text | no | ≤200 | |
| `city` | text | no | ≤120 | |
| `region` | text | no | ≤120 | state / province / county |
| `postal_code` | text | no | ≤20 | |
| `country` | text | no | ≤80 | |
| `phone` | text | no | ≤40 | |
| `fax` | text | no | ≤40 | |
| `email` | email | no | PB email validator | |
| `website` | url | no | PB url validator | |
| `portal_url` | url | no | PB url validator | patient portal |
| `hours` | text | no | ≤300 | free text; FR-034 |
| `open_24h` | bool | no | default `false` | |
| `drive_through` | bool | no | default `false` | |
| `services` | text | no | ≤500 | |
| `notes` | text | no | ≤5000 | PHI |

**Enum `facility.kind`** (`directory.FacilityKind`):
`practice` `pharmacy` `hospital` `lab` `imaging` `other`

**Indexes**

```sql
CREATE INDEX idx_facilities_owner       ON facilities (owner);
CREATE INDEX idx_facilities_owner_kind  ON facilities (owner, kind);
CREATE INDEX idx_facilities_owner_name  ON facilities (owner, name);
```

**Deliberately no unique index on name** — FR-035 and US5-3: two branches of one chain are two
rows sharing a name.

**Validation (`directory.Facility.Validate()`, all offending fields reported at once)**

- `kind` must satisfy `FacilityKind.Valid()`.
- `name` trimmed, 1..200 after trimming.
- `email` parses as an address when non-empty; `website`/`portal_url` parse as absolute `http(s)`
  URLs when non-empty.
- Every length bound above.

---

## 2. `practitioners` — new

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `owner` | relation → `users` | **yes** | `MaxSelect: 1`, `CascadeDelete: true` | FR-037 |
| `name` | text | **yes** | 1..200 | PHI-adjacent |
| `specialty` | select | no | `MaxSelect: 1`, 42 values | FR-032, FR-033. Stored as `''` when unset — **never NULL** (research D-25) |
| `facility` | relation → `facilities` | no | `MaxSelect: 1`, `CascadeDelete: false` | deleting the facility unsets this |
| `phone` | text | no | ≤40 | |
| `email` | email | no | | |
| `website` | url | no | | |
| `notes` | text | no | ≤5000 | PHI |

**Enum `practitioner.specialty`** (`directory.Specialty`, 42 values, `snake_case`):
`allergy_immunology` `anesthesiology` `cardiology` `dentistry` `dermatology` `emergency_medicine`
`endocrinology` `family_medicine` `gastroenterology` `general_surgery` `genetics` `geriatrics`
`gynecology` `hematology` `hepatology` `infectious_disease` `internal_medicine` `nephrology`
`neurology` `neurosurgery` `nutrition` `obstetrics` `occupational_therapy` `oncology`
`ophthalmology` `optometry` `oral_surgery` `orthopedics` `otolaryngology` `pain_medicine`
`palliative_care` `pathology` `pediatrics` `physical_therapy` `plastic_surgery` `podiatry`
`psychiatry` `psychology` `pulmonology` `radiology` `rheumatology` `urology` `other`

*(43 entries are listed; `other` is the mandated catch-all of FR-033. If the count is trimmed to
exactly 42 the `other` value is the one that must never be removed. The Go generator emits the list
and the select field from the same slice, so the two cannot disagree.)*

**Indexes**

```sql
CREATE UNIQUE INDEX idx_practitioners_owner_name_specialty
    ON practitioners (owner, LOWER(name), specialty);   -- FR-038
CREATE INDEX idx_practitioners_owner      ON practitioners (owner);
CREATE INDEX idx_practitioners_owner_spec ON practitioners (owner, specialty);
CREATE INDEX idx_practitioners_facility   ON practitioners (facility);
```

**Validation**

- `name` trimmed 1..200.
- `specialty`, when non-empty, must satisfy `Specialty.Valid()`.
- Duplicate `(owner, lower(name), specialty)` → `domain.ErrConflict` → **409** with the message
  FR-038 requires. Checked in the service *and* guaranteed by the index (research D-25).

---

## 3. `patients` — new

The phase's central entity. Field list is SHARED-DESIGN §1.2 verbatim.

| Field | PB type | Req (collection) | Req (DTO) | Constraints | Notes |
|---|---|---|---|---|---|
| `owner` | relation → `users` | **yes** | server-set | `MaxSelect: 1`, `CascadeDelete: true` | FR-002. Absent from every request DTO, so it is immutable by construction. |
| `first_name` | text | no ⚠ | **yes** | 1..100 | **PHI**. ⚠ see CT-2 / research D-09 |
| `last_name` | text | no ⚠ | **yes** | 1..100 | **PHI** |
| `birth_date` | date | no ⚠ | **yes** | not future, not >150y ago | **PHI**. FR-003. Age derived, never stored (FR-006) |
| `sex` | select | no | no | 4 values | |
| `blood_type` | select | no | no | 8 values | |
| `height_cm` | number | no | no | 30..272 | **canonical SI** (FR-007) |
| `weight_kg` | number | no | no | 0.5..450 | **canonical SI** |
| `address` | text | no | no | ≤500 | **PHI**, single blob |
| `relationship_to_owner` | select | no | no | 8 values | |
| `primary_practitioner` | relation → `practitioners` | no | no | `MaxSelect: 1`, `CascadeDelete: false` | FR-001, FR-040 |
| `is_self_record` | bool | **yes** | no (server-set) | default `false` | FR-004; partial unique index below |
| `photo` | file | no | no | `MaxSelect: 1`, **`Protected: true`**, `MimeTypes: image/jpeg,image/png,image/webp`, `MaxSize: 15 MiB`, `Thumbs: 100x100t,400x400f` | FR-008, FR-009, FR-044 |

⚠ **The three marked fields are collection-optional and DTO-required.** This is the deliberate,
recorded deviation CT-2 / research D-09: only server-provisioned self-records may carry an empty
value, and an integration test asserts no other row ever does.

**Enums**

| Go type | Field | Values |
|---|---|---|
| `person.Sex` | `sex` | `female` `male` `intersex` `unspecified` |
| `person.BloodType` | `blood_type` | `a_pos` `a_neg` `b_pos` `b_neg` `ab_pos` `ab_neg` `o_pos` `o_neg` |
| `person.RelationshipToOwner` | `relationship_to_owner` | `self` `spouse` `partner` `parent` `child` `sibling` `ward` `other` |

`sex` collapses upstream's seven aliases (`M, F, MALE, FEMALE, OTHER, U, UNKNOWN`) for three
concepts, per SHARED-DESIGN §1.2.

**Indexes**

```sql
CREATE UNIQUE INDEX idx_patients_self ON patients (owner) WHERE is_self_record = 1;   -- FR-004
CREATE INDEX idx_patients_owner_name  ON patients (owner, last_name, first_name, id); -- list + cursor
CREATE INDEX idx_patients_primary_pr  ON patients (primary_practitioner);             -- FR-040 usage count
```

**Validation (`person.Patient.Validate()` — every offending field in one `*ValidationError`)**

| Rule | Requirement | Error `code` |
|---|---|---|
| `first_name` trimmed 1..100 | FR-001 | `required` / `too_long` |
| `last_name` trimmed 1..100 | FR-001 | `required` / `too_long` |
| `birth_date` present, a real calendar date | FR-001, FR-003 | `required` / `invalid_date` |
| `birth_date` not after today (server clock, UTC) | FR-003, US1-3 | `date_in_future` |
| `birth_date` not before `today − 150y` | FR-003, US1-3 | `date_too_old` |
| `sex`, `blood_type`, `relationship_to_owner` in vocabulary when non-empty | FR-001 | `invalid_value` |
| `height_cm` in 30..272 when set | | `out_of_range` |
| `weight_kg` in 0.5..450 when set | | `out_of_range` |
| `address` ≤500 | | `too_long` |
| setting `is_self_record` when one already exists for the owner | FR-004, US1-4 | `conflict` (409, not 422) |

**All-at-once reporting is mandatory** (FR-003, US1-3: "together with every other invalid field in
the same submission"). `Validate()` accumulates; it never returns on the first failure. A
table-driven test submits a payload with four simultaneous faults and asserts four `fields[]`
entries.

**Derived, never stored**

- `age` — `person.AgeAt(birth_date, now)` (FR-006, D-20). Renders "0 days" for a patient born
  today (US4-4) and "not recorded" for an empty `birth_date`.
- `display.height` / `display.weight` — converted from SI per the actor's `unit_system` (FR-007,
  D-21).
- `bmi` — **not modelled at all** in this phase. SHARED-DESIGN drops stored `bmi` and this phase's
  requirements never ask for it.

**State transitions.** A patient has no lifecycle state. The only transitions are:

```
(absent) --create--> exists --update--> exists --delete--> (gone, permanently)
                       |
                       +--set photo / replace photo / remove photo
```

There is no draft, no archive, no soft delete, no reactivation. FR-049: deletion is permanent and
complete, with no recovery path offered in the application.

---

## 4. `users` — amended

Phase 001 owns this collection. This phase adds exactly one field.

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `active_patient` | relation → `patients` | no | `MaxSelect: 1`, **`CascadeDelete: false`** | FR-013. UI convenience only; **never consulted for authorization** (FR-015, D-08). Auto-unset when the patient is deleted (D-07). |

**`CascadeDelete: false` is load-bearing and gets its own test.** With `true`, deleting a patient
would delete the *account*. The migration sets it explicitly and
`migrations/assertions.go` asserts it at boot.

`unit_system` (`metric` | `imperial`), `locale`, `date_format` and `theme` already exist from phase
001 (FR-011 of that phase). This phase consumes `unit_system` for FR-007 and adds nothing to the
preference set.

**No new index.** `active_patient` is read by id on the authenticated user's own row.

---

## 5. `audit_events` — amended

Phase 001 owns this collection; phase 006 owns its read surface. This phase adds the field that
makes a per-patient activity view possible (spec Key Entities: "Extended here to carry the person a
recorded action concerned").

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `patient` | relation → `patients` | no | `MaxSelect: 1`, `CascadeDelete: false` | null for non-patient actions. Auto-unset when the patient is deleted, so a historical entry survives without pointing at a ghost. |

**New index**

```sql
CREATE INDEX idx_audit_patient_time ON audit_events (patient, occurred_at DESC, id DESC);  -- FR-029, SC-004; `id DESC` so phase 006's keyset reader stays index-only and creates no second index (ANALYSIS)
```

**Vocabulary additions** to the existing select fields. Phase 001's migration declares the shared
design contract's **complete** vocabulary — twenty actions and twenty-three target kinds — so this
phase adds only what the contract does not name, and its migration test asserts the **complete**
expected set rather than this delta (ANALYSIS C1):

- `action` — adds **`switch_patient`** for FR-020/FR-045's "every change to the person in view".
  The values this phase *writes* — `create`, `update`, `delete`, `read_sensitive` (a photo fetched
  by somebody who is not the owner) and `access_denied` — all exist from phase 001.
  **After this phase: twenty-one actions.**
- `target_kind` — adds `practitioner` and `facility`. `patient` exists from phase 001.
  **After this phase: twenty-five target kinds.**

**The content rule is unchanged and absolute** (FR-045, FR-046, SC-009): actor, action,
target_kind, opaque target_id, patient id, request_id, occurred_at. **No `ip`** — phase 001
deliberately does not create the column (001 research D-19, 001 plan post-design re-check item 1).
No names, no values, no diffs, no file names. This is what makes FR-029's "entries for records that
have since been deleted carry no identifying detail" true structurally rather than by filtering.

`request_id` is `Required` and the `system` row below is written from a migration with no HTTP
request, so it carries the backfill run's **run id** instead — same helper, same value on that run's
log lines (001 [data-model](../001-walking-skeleton/data-model.md) §3).

**Events this phase must produce** (each has a test):

| Trigger | actor_kind | action | target_kind | patient |
|---|---|---|---|---|
| create a patient (user) | `user` | `create` | `patient` | the new patient |
| create a self-record (registration or backfill) | `system` | `create` | `patient` | the new patient |
| update a patient | `user` | `update` | `patient` | the patient |
| delete a patient | `user` | `delete` | `patient` | the patient (unset immediately after by the cascade) |
| set / replace / remove photo | `user` | `update` | `patient` | the patient |
| fetch a photo **as a non-owner** (superuser here; a share recipient from phase 005) | `user` | `read_sensitive` | `patient` | the patient. An owner fetching their own person's photograph writes **no** row — 005 [`widened-authorization.md`](../005-sharing-and-collaboration/contracts/widened-authorization.md) §"Where `read_sensitive` is written" |
| change the person in view | `user` | `switch_patient` | `patient` | the newly chosen patient |
| any refused access to a patient, photo or record | `user` | `access_denied` | as addressed | as addressed |
| create/update/delete a practitioner or facility | `user` | `create`/`update`/`delete` | `practitioner`/`facility` | null |

---

## 6. `medications` — amended and re-anchored

Phase 001 modelled a medication as belonging to a `users` row. After this phase it belongs to a
`patients` row, and nothing else changes about its clinical fields.

| Field | Change | PB type | Req | Constraints |
|---|---|---|---|---|
| `patient` | **added** | relation → `patients` | **yes** (after backfill) | `MaxSelect: 1`, **`CascadeDelete: true`** — FR-026, FR-049, SC-010 |
| `practitioner` | **added** | relation → `practitioners` | no | `MaxSelect: 1`, `CascadeDelete: false` — the prescriber (US5, FR-039) |
| `pharmacy` | **added** | relation → `facilities` | no | `MaxSelect: 1`, `CascadeDelete: false` |
| `owner` | **removed** | — | — | replaced entirely by `patient` |

**New indexes**

```sql
CREATE INDEX idx_medications_patient        ON medications (patient);                      -- SC-004 counts
CREATE INDEX idx_medications_patient_start  ON medications (patient, started_on DESC, id); -- list + cursor
CREATE INDEX idx_medications_practitioner   ON medications (practitioner);                 -- FR-040 usage count
CREATE INDEX idx_medications_pharmacy       ON medications (pharmacy);                     -- FR-040 usage count
```

The old `(owner, …)` indexes are dropped in the same migration.

**Attribution rules**

- FR-021: `patient` is `Required`, so a medication attributed to nobody is impossible at the
  storage layer as well as the service layer.
- FR-024: `MedicationPatch` **has no `patient` field**. Re-attribution is refused by DTO shape, not
  by a runtime check. A test posts `{"patient":"…"}` to `PATCH` and asserts `422 validation_failed`
  with `unknown_field` — because unknown fields are rejected (D-32), the same test covers both.
- FR-025: `MedicationCreate.patient` is required; a create with an empty or absent `patient` is
  `422`, never a fallback to the person in view (US2-3, D-08).
- FR-023: every list requires `?patient=`; its absence is `400 patient_required`.

---

## 7. Relationship map

```
users ──1:N (cascade)──> patients ──1:N (cascade)──> medications
  │                          │                            │
  │ 0..1 active_patient      │ 0..1 primary_practitioner   │ 0..1 practitioner
  │ (no cascade, auto-unset) │ (no cascade, auto-unset)    │ 0..1 pharmacy
  ├──1:N (cascade)──> practitioners ──0..1 facility───────>┤    (no cascade, auto-unset)
  └──1:N (cascade)──> facilities <─────────────────────────┘

audit_events ──0..1 actor──> users
             ──0..1 patient──> patients   (no cascade, auto-unset)
```

Reading the diagram for the two destructive paths:

- **Delete an account** → cascades to its `patients`, its `practitioners`, its `facilities`; each
  deleted patient cascades to its `medications`; every surviving reference to a deleted
  practitioner or facility is unset. One transaction.
- **Delete a patient** → cascades to its `medications`; unsets `users.active_patient` on every
  account pointing at it and `audit_events.patient` on every historical entry; the photo and its
  thumbnails are removed with the record by PocketBase's file-field cleanup. One transaction.

Both behaviours are PocketBase's (`core/record_model.go:1587-1626`), not MediGo's, and both are
asserted by integration tests because MediGo depends on them (research D-06).

---

## 8. Migrations

All six register into `core.AppMigrations` via
`migrations.Register(up func(core.App) error, down func(core.App) error, filename)` — a signature
that **requires** both directions (VERIFIED-SOURCE-FACTS FACT 8), which is how Principle IX's
reversibility rule is enforced by the API itself.

**All pending migrations share one transaction** (`core/migrations_runner.go:129-131`), so the
six either all apply or none do.

| # | File | Up | Down |
|---|---|---|---|
| 1 | `1756200100_facilities.go` | create collection, 5 nil rules, fields, 3 indexes | delete collection |
| 2 | `1756200200_practitioners.go` | create collection, nil rules, fields incl. `facility` relation, 4 indexes | delete collection |
| 3 | `1756200300_patients.go` | create collection, nil rules, fields incl. `primary_practitioner` and the `Protected` photo field, 3 indexes | delete collection |
| 4 | `1756200400_users_active_patient.go` | add `active_patient` (`CascadeDelete: false`) | remove field |
| 5 | `1756200500_audit_events_patient.go` | add `patient`, extend `action` and `target_kind` vocabularies, add `(patient, occurred_at)` index | remove field + index, restore vocabularies |
| 6 | `1756200600_medications_repoint.go` | the six-step backfill of research D-13 | re-add `owner`, backfill from `patients.owner`, drop `patient`/`practitioner`/`pharmacy`, restore old indexes |

**Migration 6's `down` carries a written irreversibility note in the file**, as Principle IX
requires: reverting restores medication ownership exactly, but a full revert of the batch also
drops the `patients` collection and therefore discards every profile detail, photograph and
directory entry recorded after the migration. That is stated in a comment at the top of the `down`
function, not in a README.

**`migrations/assertions.go`** is extended and runs at boot (constitution V and VII):

1. every non-system collection has all five API rules `nil` — now covering `facilities`,
   `practitioners`, `patients`;
2. every `FileField` in the schema has `Protected: true` — now covering `patients.photo`, and the
   application **refuses to start** otherwise;
3. **new in this phase:** the `Required`/`CascadeDelete` matrix of research D-06 matches the
   declared schema exactly, field by field. A silent flip of `users.active_patient` to
   `CascadeDelete: true` would otherwise mean deleting a patient deletes the account.

---

## 9. Fixture and seed data

`internal/testdata/pb_data` (cloned by every `tests.NewTestApp`) and `medigo seed` produce the same
deterministic set. Ids are exported from `internal/testsupport/fixtures.go` so no test contains a
literal.

| Fixture | Content |
|---|---|
| Account A | 3 patients: her self-record (with photo), a child, a parent. Medications on the self-record and on the parent. 1 practitioner (with a specialty and a facility), 1 facility of kind `practice`, 1 of kind `pharmacy`. |
| Account B | 1 patient (self-record only), 1 medication, 1 practitioner. **The isolation counterparty**: every stranger-refused test addresses Account A's ids as Account B. |
| Account C | Registered but with **no** patients created by the seed — used to prove FR-005's automatic self-record provisioning actually ran. |

The seed additionally leaves `/practitioners` and `/facilities` **empty for Account B**, because
SC-013 requires the smoke gate to pass on empty screens and an always-populated seed would never
exercise the empty states (research D-35).
