# MediKeep CLINICAL Domain Model — extracted from the vendored OpenAPI document

**Source:** `/Users/krzysztof.wiatrzyk/private/monorepo/medikeep-mcp/internal/spec/openapi.json`
(MediKeep v0.69.0, FastAPI-generated OpenAPI 3.1; 376 paths / 500 operations / 325 component schemas)

**Method:** schemas queried surgically with `python3 -m json`; every field below is transcribed from
`components.schemas.*`, every constraint from the JSON Schema keywords, every enum from either a
declared `enum`, a `pattern`, a field `description`, or the semantics of a dedicated endpoint.

---

## 0. CRITICAL CAVEAT ON ENUMS — read this first

**The OpenAPI document declares only SIX enums.** Everything else that is semantically an enum
(allergy severity, condition status, medication status/route, procedure status, insurance type,
symptom severity, lab component status, …) is emitted as a **bare `{"type": "string"}`** with no
`enum`, no `pattern`, and usually only a `default`.

This is a MediKeep implementation artefact: those values are validated by Pydantic
`@field_validator` functions and by frontend constant files, neither of which FastAPI can reflect
into the schema. The allowed-value sets are therefore **not machine-recoverable from this
document**. What follows separates three confidence tiers, and the tier is stated on every enum:

| Tier | Meaning |
|---|---|
| **DECLARED** | A real `enum`/`pattern` in the OpenAPI document. Exact and complete. |
| **DOCUMENTED** | Values written out in a field `description` or a query-param `description`. Exact as written; completeness is upstream's claim, not verified. |
| **DERIVED** | Reconstructed from `default` values, the existence of filter endpoints (`/active`, `/critical`, `/scheduled`, `/ongoing`, `/abnormal`, `/needing-service`), and cross-entity consistency. **Treat as a strong starting proposal, not as upstream truth.** |

MediKube has **no wire-compat obligation**, so the DERIVED sets are a licence rather than a problem:
we get to define the canonical enums ourselves. Section 30 does exactly that. But the spec must be
explicit that we are *choosing* these values, not *inheriting* them.

### 0.1 The six DECLARED enums

| Name | Values |
|---|---|
| `EntityType` (file attachment targets) | `lab-result`, `insurance`, `visit`, `encounter`, `procedure`, `vitals`, `medication`, `immunization`, `allergy`, `condition`, `treatment`, `symptom`, `injury` |
| `VaccineCategory` | `Viral`, `Bacterial`, `Combined`, `Toxoid`, `Parasitic`, `Other` |
| `ExportFormat` | `json`, `csv`, `pdf` |
| `ExportScope` | `all`, `allergies`, `conditions`, `emergency_contacts`, `encounters`, `family_history`, `immunizations`, `injuries`, `insurance`, `lab_results`, `medications`, `pharmacies`, `practitioners`, `procedures`, `symptoms`, `treatments`, `vitals` |
| `ChannelType` (notifications) | `discord`, `email`, `gotify`, `ntfy`, `webhook` |
| `EventType` (notifications) | `backup_completed`, `backup_failed`, `invitation_received`, `invitation_accepted`, `share_revoked`, `password_changed`, `medication_reminder_due` |

Plus one DECLARED `pattern`: `unit_system` on export = `^(imperial|metric)$`.

Note `EntityType` carries **both** `visit` and `encounter` — a live rename that was never finished.
`ExportScope` uses `lab_results` (snake) while `EntityType` uses `lab-result` (kebab) and the URL
path uses `/lab-results/`. Three spellings of one entity.

---

## 1. Global conventions in upstream's model

Facts that hold across nearly every clinical entity, so they are not repeated in each table:

1. **`id`**: `integer`, required in every `*Response`. Postgres bigserial. MediKube replaces this with
   PocketBase's 15-char text id.
2. **`patient_id`**: `integer`, **required** on create, `exclusiveMinimum: 0`. Every clinical record
   hangs directly off a patient — there is no encounter-centric or episode-centric grouping.
   Exception: `EmergencyContactCreate` omits it from the body and takes it as an optional **query
   parameter** instead. `SymptomOccurrence` has no `patient_id` at all (reached only via its symptom).
3. **`patient_id` as a query param**: nearly every list endpoint accepts
   `?patient_id=` described as *"Patient ID for Phase 1 patient switching"* — an unfinished migration
   from "current patient in session" to explicit scoping. Both mechanisms are live simultaneously.
4. **`required_permission`**: query param, `string`, default `"view"`, on **41** operations. DERIVED
   values `view` / `edit` / `full` (matching `PatientShareInvitationRequest.permission_level`, whose
   description says *"Permission level: view, edit, or full"*). **A client-supplied authorization
   parameter** — the caller tells the server which permission to check. See §31.
5. **`tags`**: `array<string> | null`, default `[]`, present on allergies, conditions, medications,
   encounters, procedures, treatments, symptoms, immunizations, injuries, medical equipment, lab
   results. **Absent** on vitals, symptom occurrences, insurance, emergency contacts, lab test
   components, and all reference entities.
6. **`notes`**: nearly universal, `string | null`, `maxLength: 5000`.
7. **Timestamps are inconsistent.** `created_at`/`updated_at` appear on: symptoms, symptom
   occurrences, vitals, injuries, injury types, insurance, medical equipment, lab results, lab test
   components, practitioners, practices, pharmacies, medical specialties. They are **absent from the
   response schemas** of: allergies, conditions, medications, encounters, procedures, treatments,
   immunizations, emergency contacts, patients. The columns almost certainly exist in the DB; the
   Pydantic response models just never exposed them.
8. **Dates vs date-times**: clinical event dates are `format: date` (no time, no zone) everywhere
   **except** `vitals.recorded_date` (`date-time`) and the two symptom-occurrence `*_time` fields
   (`format: time`, stored separately from their `date` siblings).
9. **Pagination**: `skip`/`limit`. Default `limit` is **10000** on most list endpoints, 100–200 on
   labs, 2000 on `/lab-test-components/patient/{id}/all`. There is no cursor and no total count on
   most list endpoints (only `VitalsPaginatedResponse` and `ComponentCatalogResponse` return `total`).

---

## 2. Entity inventory (this document's scope)

| # | Entity | Create / Response / Update schemas | Distinct fields |
|---|---|---|---|
| 1 | Patient | `PatientCreateRequest` / `PatientResponse` (×2 variants) / `PatientUpdateRequest` | 15 |
| 2 | Allergy | `AllergyCreate` / `AllergyResponse` / `AllergyUpdate` | 10 |
| 3 | Condition | `ConditionCreate` / `ConditionResponse` / `ConditionUpdate` | 14 |
| 4 | Medication | `MedicationCreate` / `MedicationResponse` / `MedicationUpdate` | 21 |
| 5 | Encounter | `EncounterCreate` / `EncounterResponse` / `EncounterUpdate` | 16 |
| 6 | Procedure | `ProcedureCreate` / `ProcedureResponse` / `ProcedureUpdate` | 19 |
| 7 | Treatment | `TreatmentCreate` / `TreatmentResponse` / `TreatmentUpdate` | 18 |
| 8 | Symptom | `SymptomCreate` / `SymptomResponse` / `SymptomUpdate` | 15 |
| 9 | Symptom Occurrence | `SymptomOccurrenceCreate` / `…Response` / `…Update` | 19 |
| 10 | Vitals | `VitalsCreate` / `VitalsResponse` / `VitalsUpdate` | 23 |
| 11 | Immunization | `ImmunizationCreate` / `ImmunizationResponse` / `ImmunizationUpdate` | 18 |
| 12 | Injury | `InjuryCreate` / `InjuryWithRelations` / `InjuryUpdate` | 19 |
| 13 | Injury Type | `InjuryTypeCreate` / `InjuryTypeResponse` | 6 |
| 14 | Insurance | `InsuranceCreate` / `Insurance` / `InsuranceUpdate` | 20 |
| 15 | Medical Equipment | `MedicalEquipmentCreate` / `…Response` / `…Update` | 18 |
| 16 | Emergency Contact | `EmergencyContactCreate` / `…Response` / `…Update` | 11 |
| 17 | Practitioner | `PractitionerCreate` / `Practitioner` / `PractitionerUpdate` | 14 |
| 18 | Practice | `PracticeCreate` / `Practice` / `PracticeUpdate` (+ `PracticeLocationSchema`) | 17 |
| 19 | Pharmacy | `PharmacyCreate` / `Pharmacy` / `PharmacyUpdate` | 19 |
| 20 | Medical Specialty | `MedicalSpecialtyCreate` / `MedicalSpecialty` | 6 |
| 21 | Lab Result | `LabResultCreate` / `LabResultResponse` / `LabResultUpdate` | 22 |
| 22 | Lab Test Component | `LabTestComponentCreate` / `…Response` / `…Update` | 20 |
| 23 | Standardized Test | `StandardizedTestResponse` (read-only catalog) | 8 |
| 24 | Standardized Vaccine | `StandardizedVaccineResponse` (read-only catalog) | 10 |
| 25 | Tag | no entity schema — see §26 | ~3 |
| | **Core subtotal** | | **378** |
| | **17 link tables** (§27) | | **121** |
| | **Grand total** | | **~499 fields** |

---

## 3. Patient

Two divergent response shapes exist for the same table, in two different modules:
`app.schemas.patient.PatientResponse` (a.k.a. `Patient`) and
`app.api.v1.endpoints.patient_management.PatientResponse`. The second adds the ownership/sharing
fields; the first does not. Both are served, from different routers, for the same rows.

### Fields

| Field | Type | Req | Nullable | Format | Constraints | Notes |
|---|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — | PK |
| `first_name` | string | **REQ** | no | — | `minLength 1`, `maxLength 100` | |
| `last_name` | string | **REQ** | no | — | `minLength 1`, `maxLength 100` | |
| `birth_date` | string | **REQ** | no | `date` | — | |
| `gender` | string | opt | yes | — | `maxLength 20` | free text, no enum |
| `blood_type` | string | opt | yes | — | `maxLength 5` | free text, no enum |
| `height` | number | opt | yes | — | `exclusiveMinimum 0`; on update also `maximum 108` | **inches** ("Height in inches (1-9 feet)") |
| `weight` | number | opt | yes | — | `exclusiveMinimum 0`; on update also `maximum 992` | **pounds** ("Weight in pounds (1-992 lbs)") |
| `address` | string | opt | yes | — | `maxLength 500` | single blob, not structured |
| `physician_id` | integer | opt | yes | — | `exclusiveMinimum 0` (update only) | FK → practitioner (primary care) |
| `is_self_record` | boolean | opt | no | — | default `false` | create-only; not in `PatientUpdateRequest` |
| `relationship_to_self` | string | opt | yes | — | `maxLength 30` | "e.g. self, spouse, child" — free text |
| `owner_user_id` | integer | REQ | no | — | — | *only* on `patient_management.PatientResponse` |
| `privacy_level` | string | REQ | no | — | — | *only* on `patient_management.PatientResponse`; no enum declared |
| `permission_level` | string | opt | yes | — | — | *only* on `patient_management.PatientResponse`; the **caller's** permission on this patient, DERIVED `view`/`edit`/`full` |

**Note the min/max asymmetry:** `PatientCreateRequest.height` has only `exclusiveMinimum: 0`, but
`PatientUpdateRequest.height` adds `maximum: 108`. You can create a 400-inch patient and then never
be able to update them. Same for `weight` (992) and `physician_id` (`exclusiveMinimum` on update only).

### Foreign keys

- `physician_id` → Practitioner. **N:1**, optional.
- `owner_user_id` → User. **N:1**, required.
- Patient is the parent of every clinical entity: **1:N** to all of allergies, conditions,
  medications, encounters, procedures, treatments, symptoms, vitals, immunizations, injuries,
  insurance, medical equipment, emergency contacts, lab results.

### Related endpoints / derived shapes

- `PatientListResponse`: `patients[]`, `total_count`, `owned_count`, `shared_count`.
- `PatientDashboardStats`: `patient_id`, `total_records`, `active_medications`, `total_lab_results`,
  `total_procedures`, `total_treatments`, `total_conditions`, `total_allergies`,
  `total_immunizations`, `total_encounters`, `total_vitals` — all computed counts.
- `PatientPhotoResponse`: `id`, `patient_id`, `file_name`, `file_path`, `file_size`, `mime_type`,
  `original_name`, `width`, `height`, `uploaded_by` (FK user), `uploaded_at`, `updated_at`.
- Multi-patient switching: `POST /api/v1/patient-management/switch` (`SwitchPatientRequest`),
  `GET /active/current`, `GET /self-record`, `GET /owned/list`.

**`age` is NOT a field and NOT returned.** The task brief expected it; the API only ships
`birth_date`. Age is computed client-side. Same for `days_until_expiry` (nowhere in the document)
and `is_active` (only a stored boolean on emergency contacts / specialties / shares, never derived).

---

## 4. Allergy

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `allergen` | string | **REQ** | no | — | `minLength 2`, `maxLength 200` |
| `reaction` | string | opt | yes | — | `maxLength 500` |
| `severity` | string | **REQ** | no | — | *(enum not declared)* |
| `status` | string | opt | no | — | default `"active"` *(enum not declared)* |
| `onset_date` | string | opt | yes | `date` | — |
| `notes` | string | opt | yes | — | `maxLength 5000` |
| `medication_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

No `created_at` / `updated_at` in the response.

### Enums

- **`severity`** — DERIVED: `mild`, `moderate`, `severe`, `life-threatening`.
  Evidence: `GET /api/v1/allergies/patient/{id}/critical` is documented as
  *"Get critical (**severe and life-threatening**) allergies for a patient."*, and `InjuryCreate.severity`
  documents the identical 4-value ladder. Note the hyphen in `life-threatening` (not underscore).
- **`status`** — DERIVED: `active`, `inactive`, `resolved`. Evidence: default `"active"` plus a
  dedicated `/patient/{id}/active` endpoint. Filterable via `?status=`.

### Foreign keys

- `patient_id` → Patient. **N:1**, required.
- `medication_id` → Medication. **N:1**, optional — *"ID of the medication causing this allergy"*.
  A single direct FK, **not** a link table: an allergy can name at most one culprit medication.

### Nested read shape

`AllergyWithRelations` = `AllergyResponse` + `patient` (full `PatientResponse`) + `medication`
(full `MedicationResponse`). Full objects inlined, not summaries.

### Endpoints

`POST|GET /allergies/`, `GET|PUT|DELETE /allergies/{id}`,
`GET /allergies/patient/{pid}/active`, `/critical`, `/check/{allergen}` (untyped 200 —
*"Check if a patient has any active allergies to a specific allergen"*),
plus duplicate list routes `/allergies/patients/{pid}/allergies/` and `/patients/{pid}/allergies/`.
Filters on the collection: `severity`, `allergen`, `status`, `tags`, `tag_match_all`, `patient_id`.

---

## 5. Condition

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `condition_name` | string | opt | yes | — | `maxLength 500` |
| `diagnosis` | string | **REQ** | no | — | `minLength 2`, `maxLength 500` |
| `status` | string | **REQ** | no | — | *(enum not declared, no default)* |
| `severity` | string | opt | yes | — | *(enum not declared)* |
| `onset_date` | string | opt | yes | `date` | — |
| `end_date` | string | opt | yes | `date` | "Date when the condition was resolved" |
| `icd10_code` | string | opt | yes | — | `maxLength 10` |
| `snomed_code` | string | opt | yes | — | `maxLength 20` |
| `code_description` | string | opt | yes | — | `maxLength 500` |
| `notes` | string | opt | yes | — | `maxLength 5000` |
| `practitioner_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

**`condition_name` vs `diagnosis` are both here and both free text up to 500 chars.** `diagnosis` is
the required one; `condition_name` is optional and nullable. Nothing in the spec distinguishes them.
`ConditionDropdownOption` exposes only `diagnosis`, so `diagnosis` is the de-facto display name and
`condition_name` is vestigial.

### Enums

- **`status`** — DERIVED: `active`, `inactive`, `resolved`, `chronic`, `recurrence`.
  Evidence: required with **no default** (so the client must always supply it), `/patient/{id}/active`
  endpoint, `?active_only=` on `/conditions/dropdown`, `?status=` filter.
- **`severity`** — DERIVED: `mild`, `moderate`, `severe`, `life-threatening` (same ladder as allergy/injury).

### Foreign keys

- `patient_id` → Patient. **N:1** required.
- `practitioner_id` → Practitioner. **N:1** optional (diagnosing clinician).
- **Inbound**, as a direct FK on the child: `Encounter.condition_id`, `Procedure.condition_id`,
  `Treatment.condition_id` — each **N:1** into Condition.
- **Many-to-many** via link tables: Medication, Lab Result, Injury, Symptom (§27).

### Nested read shape

`ConditionWithRelations` = fields + `patient` (untyped object) + `practitioner` (untyped object)

+ `treatments` (untyped array). Note this one uses untyped `object` where `AllergyWithRelations`
used typed refs — the nesting strategy is per-endpoint improvisation.

---

## 6. Medication

The widest clinical entity, and the only one carrying a reminder/scheduling subsystem inline.

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | — |
| `medication_name` | string | **REQ** | no | — | *(no maxLength!)* |
| `alternative_name` | string | opt | yes | — | — |
| `medication_type` | string | opt | yes | — | default `"prescription"` |
| `dosage` | string | opt | yes | — | free text ("500mg") |
| `frequency` | string | opt | yes | — | free text ("twice daily") |
| `route` | string | opt | yes | — | *(enum not declared)* |
| `indication` | string | opt | yes | — | why it's prescribed |
| `effective_period_start` | string | opt | yes | `date` | — |
| `effective_period_end` | string | opt | yes | `date` | — |
| `status` | string | opt | yes | — | *(no default, nullable)* |
| `practitioner_id` | integer | opt | yes | — | prescriber |
| `pharmacy_id` | integer | opt | yes | — | — |
| `notes` | string | opt | yes | — | — |
| `side_effects` | string | opt | yes | — | free text |
| `reminder_enabled` | boolean | opt | no | — | default `false` |
| `reminder_times` | array&lt;string&gt; | opt | yes | — | wall-clock times, no `format: time` |
| `reminder_message` | string | opt | yes | — | — |
| `reminder_days` | array&lt;integer&gt; | opt | yes | — | days of week; **no min/max, no encoding documented** (0–6? 1–7? Mon-first?) |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

**Medication is the only clinical entity with NO `maxLength` on its own name field**, while
`allergen` gets 200 and `procedure_name` gets 300.

### Enums

- **`medication_type`** — DERIVED: `prescription`, `otc`, `supplement`, `herbal`. Evidence: default
  `"prescription"`; no other signal in the document.
- **`status`** — DERIVED: `active`, `stopped`, `on-hold`, `completed`, `cancelled`. Evidence:
  `?active_only=` on `/medications/patient/{pid}`, `?status=` filter, and `active_medications` in
  `PatientDashboardStats`. Nullable with no default, so "unknown" is representable.
- **`route`** — DERIVED, **nothing in the spec constrains it**. Proposed set (FHIR-ish, matching
  what a personal-records app needs): `oral`, `sublingual`, `topical`, `inhalation`, `nasal`,
  `ophthalmic`, `otic`, `rectal`, `vaginal`, `intramuscular`, `subcutaneous`, `intravenous`,
  `transdermal`, `other`. **This set is our invention.** Upstream accepts any string.
- **`frequency`** is deliberately free text upstream — dose-schedule modelling was never attempted.

### Foreign keys

- `patient_id` → Patient **N:1** required.
- `practitioner_id` → Practitioner **N:1** optional.
- `pharmacy_id` → Pharmacy **N:1** optional.
- **Inbound direct FK:** `Allergy.medication_id` → Medication (N:1).
- **Many-to-many:** Condition, Lab Result, Treatment, Injury, Symptom (§27).

### Nested read shape

`MedicationResponseWithNested` = fields + `practitioner` (typed `Practitioner`) + `pharmacy` (typed
`Pharmacy`).

### `GET /medications/{id}/treatments` — the reverse of a link table, with its own DTO tree

`MedicationTreatmentResponse`: `id`, `treatment_id`, `medication_id`, `specific_dosage`,
`specific_frequency`, `specific_duration`, `timing_instructions`, `relevance_note`,
`specific_prescriber_id`, `specific_pharmacy_id`, `specific_start_date`, `specific_end_date`,
`treatment` → `MedicationTreatmentInfo` {`id`, `treatment_name`, `treatment_type`, `status`, `mode`,
`start_date`, `end_date`, `condition` → `MedicationTreatmentCondition` {`id`, `condition_name`}}.
Three purpose-built schemas to render one join in one direction.

`POST /medications/{id}/reminders/test` fires a test notification.

---

## 7. Encounter (a.k.a. "visit")

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | — |
| `reason` | string | **REQ** | no | — | *(no maxLength)* |
| `date` | string | **REQ** | no | `date` | — |
| `visit_type` | string | opt | yes | — | *(enum not declared)* |
| `chief_complaint` | string | opt | yes | — | — |
| `diagnosis` | string | opt | yes | — | free text, **duplicates Condition.diagnosis** |
| `treatment_plan` | string | opt | yes | — | — |
| `follow_up_instructions` | string | opt | yes | — | — |
| `duration_minutes` | integer | opt | yes | — | *(no minimum)* |
| `location` | string | opt | yes | — | free text, **not** an FK to Practice |
| `priority` | string | opt | yes | — | *(enum not declared)* |
| `notes` | string | opt | yes | — | — |
| `practitioner_id` | integer | opt | yes | — | — |
| `condition_id` | integer | opt | yes | — | — |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

`reason` is required and unbounded; `chief_complaint` is optional. They mean the same thing.

### Enums

- **`visit_type`** — DERIVED: `office`, `telehealth`, `urgent-care`, `emergency`, `inpatient`,
  `follow-up`, `annual`, `other`. Nothing in the spec constrains this.
- **`priority`** — DERIVED: `routine`, `urgent`, `emergency`. Nothing in the spec constrains this.

### Foreign keys

- `patient_id` → Patient **N:1** required.
- `practitioner_id` → Practitioner **N:1** optional.
- `condition_id` → Condition **N:1** optional.
- **Many-to-many:** Lab Result (bidirectional route pair), Treatment (§27).

### Nested read shape

`EncounterWithRelations` = fields + `patient_name` (string) + `practitioner_name` (string).
A **third** nesting style: flattened denormalised name strings rather than nested objects.

### Endpoints

`POST|GET /encounters/`, `GET|PUT|DELETE /encounters/{id}`,
`GET /encounters/patient/{pid}/recent?days=30`,
duplicate lists `/encounters/patients/{pid}/encounters/` and `/patients/{pid}/encounters/`.

---

## 8. Procedure

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `procedure_name` | string | **REQ** | no | — | `minLength 2`, `maxLength 300` |
| `procedure_type` | string | opt | yes | — | `maxLength 50` — "e.g., surgical, diagnostic" |
| `procedure_code` | string | opt | yes | — | `maxLength 50` — "e.g., CPT code" |
| `description` | string | opt | yes | — | `maxLength 5000` |
| `date` | string | **REQ** | no | `date` | — |
| `status` | string | **REQ** | no | — | *(enum not declared, no default)* |
| `outcome` | string | opt | yes | — | `maxLength 50` |
| `facility` | string | opt | yes | — | `maxLength 300` — free text, not an FK |
| `procedure_setting` | string | opt | yes | — | `maxLength 100` — "outpatient, inpatient, office" |
| `procedure_complications` | string | opt | yes | — | `maxLength 500` |
| `procedure_duration` | integer | opt | yes | — | `exclusiveMinimum 0` — minutes |
| `anesthesia_type` | string | opt | yes | — | `maxLength 100` |
| `anesthesia_notes` | string | opt | yes | — | `maxLength 5000` |
| `notes` | string | opt | yes | — | `maxLength 5000` |
| `practitioner_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `condition_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

Every field is prefixed `procedure_*` inside a resource already called `procedure`. Four times.

### Enums

- **`status`** — DERIVED: `scheduled`, `in-progress`, `completed`, `cancelled`, `postponed`.
  Evidence: `GET /procedures/scheduled` exists; `?status=` filter; required with no default.
- **`procedure_setting`** — DOCUMENTED (description): `outpatient`, `inpatient`, `office`.
  Stored as `maxLength 100` free text.
- **`procedure_type`** — DOCUMENTED-ish (description "e.g., surgical, diagnostic" — explicitly
  exemplary, not exhaustive). DERIVED set: `surgical`, `diagnostic`, `therapeutic`, `preventive`, `other`.
- **`outcome`** — DERIVED: `successful`, `partial`, `unsuccessful`, `complications`. `maxLength 50`.
- **`anesthesia_type`** — DERIVED: `none`, `local`, `regional`, `sedation`, `general`.

### Foreign keys

- `patient_id` → Patient **N:1** required; `practitioner_id` → Practitioner **N:1** optional;
  `condition_id` → Condition **N:1** optional.
- **Many-to-many:** Lab Result, Injury (§27).

### Nested read shape

`ProcedureWithRelations` = fields + `patient` (typed `PatientResponse`) + `practitioner`
(typed `PractitionerSummary`). A **fourth** nesting style — full patient, summary practitioner.

---

## 9. Treatment

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `treatment_name` | string | **REQ** | no | — | `minLength 2`, `maxLength 300` |
| `treatment_type` | string | opt | yes | — | `maxLength 300` — desc: "Category of treatment (optional)" |
| `treatment_category` | string | opt | yes | — | `maxLength 200` — desc: "Category of treatment (e.g., 'inpatient', 'outpatient')" |
| `description` | string | opt | yes | — | `maxLength 5000` |
| `start_date` | string | opt | yes | `date` | — |
| `end_date` | string | opt | yes | `date` | — |
| `frequency` | string | opt | yes | — | `maxLength 100` |
| `dosage` | string | opt | yes | — | `maxLength 200` |
| `outcome` | string | opt | yes | — | `maxLength 200` — "Expected outcome" |
| `location` | string | opt | yes | — | `maxLength 200` |
| `mode` | string | opt | no | — | default `"simple"` — "Treatment mode: 'simple' or 'advanced'" |
| `status` | string | opt | yes | — | default `"active"` |
| `notes` | string | opt | yes | — | `maxLength 5000` |
| `practitioner_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `condition_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

**`treatment_type` and `treatment_category` have literally the same description word-for-word**
("Category of treatment"), different `maxLength` (300 vs 200), and both are optional free text.

### Enums

- **`mode`** — DOCUMENTED: `simple`, `advanced`. Default `simple`. This is a **UI-complexity flag
  persisted as domain data**: `advanced` mode unlocks the treatment↔medication/encounter/equipment/
  lab-result link editors. Presentation state in the database.
- **`status`** — DERIVED: `active`, `completed`, `on-hold`, `cancelled`. Evidence: default `"active"`,
  `/treatments/patient/{pid}/active`, `/treatments/ongoing`, `?status=` filter.
- **`treatment_category`** — DOCUMENTED (exemplary): `inpatient`, `outpatient`.

### Foreign keys

- `patient_id` → Patient **N:1** required; `practitioner_id` **N:1** optional;
  `condition_id` → Condition **N:1** optional.
- **Many-to-many:** Encounter, Medical Equipment, Lab Result, Medication, Injury, Symptom (§27).
  Treatment is the most heavily linked entity in the schema — **six** of the seventeen link tables
  touch it.

### Nested read shape

`TreatmentWithRelations` = fields + `patient` + `practitioner` + `condition`, all untyped `object`.

---

## 10. Symptom (the definition/header record)

Upstream splits symptoms into a **definition** ("Migraine") and N **occurrences** (each episode).
This is the only clinical entity with a two-level model, and the only one whose response ships a
computed count.

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | — |
| `symptom_name` | string | **REQ** | no | — | *(no maxLength)* |
| `category` | string | opt | yes | — | *(enum not declared)* |
| `status` | string | opt | no | — | default `"active"` |
| `is_chronic` | boolean | opt | no | — | default `false` |
| `first_occurrence_date` | string | **REQ** | no | `date` | required on create |
| `last_occurrence_date` | string | opt | yes | `date` | **response only** — maintained by the server |
| `resolved_date` | string | opt | yes | `date` | — |
| `typical_triggers` | array&lt;string&gt; | opt | yes | — | — |
| `general_notes` | string | opt | yes | — | — |
| `occurrence_count` | integer | opt | yes | — | default `0` — **response only, computed** |
| `created_at` | string | REQ (resp) | no | `date-time` | — |
| `updated_at` | string | REQ (resp) | no | `date-time` | — |
| `tags` | array&lt;string&gt; | opt | yes | — | *(no default `[]` here, unlike every other entity)* |

### Enums

- **`status`** — DERIVED: `active`, `resolved`, `monitoring`, `inactive`. Default `"active"`,
  `?status=` filter, plus a `resolved_date` field implying a `resolved` state.
- **`category`** — DERIVED: `pain`, `respiratory`, `gastrointestinal`, `neurological`,
  `cardiovascular`, `musculoskeletal`, `dermatological`, `psychological`, `constitutional`, `other`.
  Nothing in the spec constrains it.

### Foreign keys

- `patient_id` → Patient **N:1** required.
- **1:N** to Symptom Occurrence.
- **Many-to-many:** Condition, Medication (with a relationship type), Treatment (§27).

### Endpoints

`POST|GET /symptoms/` (`?status=`, `?search=`), `GET|PUT|DELETE /symptoms/{id}`,
`GET /symptoms/stats` (untyped object), `GET /symptoms/timeline?start_date&end_date` (untyped array),
occurrence sub-resource, and six `link-*`/`unlink-*` routes (§27).

---

## 11. Symptom Occurrence

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `symptom_id` | integer | opt (create) / REQ (resp) | yes on create | — | taken from the URL path if omitted |
| `occurrence_date` | string | **REQ** | no | `date` | — |
| `occurrence_time` | string | opt | yes | `time` | separate column from the date |
| `severity` | string | **REQ** | no | — | *(enum not declared)* |
| `pain_scale` | integer | opt | yes | — | **no minimum/maximum declared** (0–10 intended) |
| `duration` | string | opt | yes | — | free text ("3 hours") |
| `time_of_day` | string | opt | yes | — | *(enum not declared)* |
| `location` | string | opt | yes | — | body location, free text |
| `triggers` | array&lt;string&gt; | opt | yes | — | — |
| `relief_methods` | array&lt;string&gt; | opt | yes | — | — |
| `associated_symptoms` | array&lt;string&gt; | opt | yes | — | **free-text strings, not FKs to Symptom** |
| `impact_level` | string | opt | yes | — | *(enum not declared)* |
| `resolved_date` | string | opt | yes | `date` | — |
| `resolved_time` | string | opt | yes | `time` | — |
| `resolution_notes` | string | opt | yes | — | — |
| `notes` | string | opt | yes | — | — |
| `created_at` | string | REQ (resp) | no | `date-time` | — |
| `updated_at` | string | REQ (resp) | no | `date-time` | — |

No `patient_id`, no `tags`.

### Enums

- **`severity`** — DERIVED: `mild`, `moderate`, `severe`. The MCP server's own symptom-intake prompt
  says: *"How severe was it? Take the user's own word for it (for example "mild", "moderate", "severe")."*
  Note this is a **3-value** ladder, not the 4-value allergy/injury one.
- **`impact_level`** — DERIVED: `none`, `mild`, `moderate`, `severe`. The same prompt says:
  *"How much did it interfere with the day - none, mild, moderate, or did it stop them entirely?"*
  — so the top value is "stopped me entirely"; `severe` is our naming.
- **`time_of_day`** — DERIVED: `morning`, `afternoon`, `evening`, `night`. Redundant with
  `occurrence_time`, which stores the actual clock time.
- **`pain_scale`** — integer 0–10 intended, **unvalidated**.

### Foreign keys

- `symptom_id` → Symptom **N:1** required (implicitly, via path).

---

## 12. Vitals

One row = one measurement *session*, with every possible vital as a nullable column. A wide sparse
table, not an observation/value model.

| Field | Type | Req | Nullable | Format | Unit (implied) |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | |
| `patient_id` | integer | **REQ** | no | — | |
| `practitioner_id` | integer | opt | yes | — | |
| `recorded_date` | string | **REQ** | no | `date-time` (create) / **untyped nullable string (response!)** | the only clinical `date-time` |
| `systolic_bp` | integer | opt | yes | — | mmHg |
| `diastolic_bp` | integer | opt | yes | — | mmHg |
| `heart_rate` | integer | opt | yes | — | bpm |
| `temperature` | number | opt | yes | — | **°F** (imperial default) |
| `weight` | number | opt | yes | — | **lbs** |
| `height` | number | opt | yes | — | **inches** |
| `oxygen_saturation` | number | opt | yes | — | % |
| `respiratory_rate` | integer | opt | yes | — | breaths/min |
| `blood_glucose` | number | opt | yes | — | **mg/dL** |
| `a1c` | number | opt | yes | — | % |
| `glucose_context` | string | opt | yes | — | see enum |
| `bmi` | number | opt | yes | — | **client-supplied, not computed** |
| `pain_scale` | integer | opt | yes | — | 0–10, unvalidated |
| `notes` | string | opt | yes | — | |
| `location` | string | opt | yes | — | |
| `device_used` | string | opt | yes | — | |
| `import_source` | string | opt | yes | — | **create-only; absent from `VitalsUpdate`** |
| `created_at` | string | REQ (resp) | yes | untyped | |
| `updated_at` | string | REQ (resp) | yes | untyped | |

**No numeric bounds anywhere.** No `minimum`/`maximum` on blood pressure, heart rate, temperature,
SpO2 or glucose. A systolic of 40000 validates.

**`recorded_date` is `format: date-time` in `VitalsCreate` but a bare nullable `string` (no format)
in `VitalsResponse`, and required-but-nullable.** Same for `created_at`/`updated_at`. Round-tripping
a vitals record is not type-safe.

**`bmi` is stored, not derived**, even though `weight` and `height` are on the same row.

### Enums

- **`glucose_context`** — DOCUMENTED (query-param description, three separate endpoints):
  `fasting`, `before_meal`, `after_meal`, `random`.
- **`vital_type`** (filter parameter, not a stored field) — DOCUMENTED: `blood_pressure`,
  `heart_rate`, `temperature`, `weight`, `oxygen_saturation`, `blood_glucose`, `a1c`.
  (The `/vitals/` collection route's description omits `a1c`; the three `patient/{id}` routes include
  it. A filter vocabulary that disagrees with itself across endpoints.)
- **`import_source`** — DERIVED from `VitalsImportDevice.key` values served at runtime by
  `GET /vitals/import/devices` (`{key, name}` pairs — **not enumerable from the document**).
  Also used as a path segment: `DELETE /vitals/patient/{pid}/import/{import_source}/date/{date}`.
- **`unit_system`** — DECLARED elsewhere as `^(imperial|metric)$` (on export and on
  `UserPreferences.unit_system`). Vitals themselves are **always stored imperial**; conversion is a
  presentation concern.

### Foreign keys

- `patient_id` → Patient **N:1** required; `practitioner_id` → Practitioner **N:1** optional.
- **No link tables at all.** Vitals connect to nothing else.

### Derived / aggregate shapes

- `VitalsStats`: `total_readings`, `latest_reading_date`, `avg_systolic_bp`, `avg_diastolic_bp`,
  `avg_heart_rate`, `avg_temperature`, `current_temperature`, `current_weight`, `current_bmi`,
  `weight_change`, `current_blood_glucose`, `current_a1c`. All computed.
- `VitalsPaginatedResponse`: `items`, `total`, `skip`, `limit`.
- Import: `VitalsImportPreviewResponse` {`device_name`, `total_readings`, `preview_rows[]`,
  `duplicate_count`, `new_count`, `skipped_rows`, `errors[]`, `warnings[]`, `date_range_start`,
  `date_range_end`}; `VitalsPreviewRow` {`recorded_date`, `blood_glucose`, `device_used`,
  `is_duplicate` (computed)}; `VitalsImportResponse` {`imported_count`, `skipped_duplicates`,
  `errors[]`, `total_processed`}. Import body: `device_key`, `skip_duplicates` (default `true`), `file`.
  **The preview row only carries `blood_glucose`** — the importer is a glucose-meter importer wearing
  a generic name.

---

## 13. Immunization

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `vaccine_name` | string | **REQ** | no | — | `minLength 2`, `maxLength 200` |
| `vaccine_trade_name` | string | opt | yes | — | `maxLength 200` — "e.g., Flublok TRIV 2025-2026 PFS" |
| `date_administered` | string | **REQ** | no | `date` | — |
| `dose_number` | integer | opt | yes | — | `minimum 1` |
| `lot_number` | string | opt | yes | — | `maxLength 50` |
| `ndc_number` | string | opt | yes | — | `maxLength 50` |
| `manufacturer` | string | opt | yes | — | `maxLength 200` |
| `site` | string | opt | yes | — | `maxLength 100` — injection site |
| `route` | string | opt | yes | — | `maxLength 50` |
| `expiration_date` | string | opt | yes | `date` | vaccine's own expiry |
| `location` | string | opt | yes | — | `maxLength 200` — "clinic, hospital, pharmacy, etc." |
| `notes` | string | opt | yes | — | `maxLength 5000` |
| `practitioner_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `standardized_vaccine_id` | integer | opt | yes | — | `exclusiveMinimum 0` — **response only** |
| `standardized_vaccine_who_code` | string | opt | yes | — | `maxLength 100` — **write only** |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

**Asymmetric catalog link:** you *write* `standardized_vaccine_who_code` (a WHO PCMT business key)
and you *read back* `standardized_vaccine_id` (a surrogate FK). The server resolves one to the other.
`ImmunizationUpdate.standardized_vaccine_who_code` documents *"Pass explicit null to clear the link
(e.g., user converted to free-text)"* — so this is an explicit tri-state PATCH field.

### Enums

- **`route`** — DERIVED: `intramuscular`, `subcutaneous`, `intradermal`, `oral`, `intranasal`.
  `maxLength 50`, otherwise unconstrained.
- **`site`** — DERIVED: `left-arm`, `right-arm`, `left-thigh`, `right-thigh`, `left-deltoid`,
  `right-deltoid`, `oral`, `nasal`, `other`. Unconstrained upstream.

### Foreign keys

- `patient_id` → Patient **N:1** required; `practitioner_id` **N:1** optional.
- `standardized_vaccine_id` → Standardized Vaccine **N:1** optional (catalog).
- No link tables.

### Derived shapes — the immunization *history* endpoint

`GET /immunizations/patient/{pid}/history?start_date&end_date` → `ImmunizationHistoryResponse`:

| Field | Type | Notes |
|---|---|---|
| `items` | array&lt;`ImmunizationHistoryItem`&gt; | REQ |
| `diseases_index` | object&lt;string, array&lt;integer&gt;&gt; | **computed**: canonical disease key ("Polio", "Hepatitis B") → immunization ids conferring protection. *"Pre-aggregated server-side so the client doesn't re-derive groups."* |
| `unmatched_count` | integer | **computed**, `minimum 0`, default `0` |

`ImmunizationHistoryItem` = `ImmunizationResponse` + three computed fields:

- `components`: array&lt;string&gt; — *"Canonical disease keys this immunization covers (e.g. ["Polio"],
  ["Diphtheria","Tetanus","Pertussis"]). Sourced from the library's disease_keys, not raw antigen
  labels, so combination and single-disease vaccines bucket consistently."*
- `is_combined`: boolean, default `false` — from the linked catalog entry.
- `is_library_matched`: boolean, default `false`.

`GET /immunizations/patient/{pid}/booster-check/{vaccine_name}?months_interval=12` — untyped 200,
*"Check if a patient is due for a booster shot."*

---

## 14. Injury

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `injury_name` | string | **REQ** | no | — | `minLength 2`, `maxLength 300` |
| `injury_type_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `body_part` | string | **REQ** | no | — | `minLength 1`, `maxLength 100` — free text |
| `laterality` | string | opt | yes | — | *(enum documented in description)* |
| `date_of_injury` | string | opt | yes | `date` | "optional if unknown" |
| `mechanism` | string | opt | yes | — | `maxLength 500` — "How the injury happened" |
| `severity` | string | opt | yes | — | *(enum documented in description)* |
| `status` | string | opt | no | — | default `"active"` *(enum documented)* |
| `treatment_received` | string | opt | yes | — | free text — **overlaps the injury↔treatment link table** |
| `recovery_notes` | string | opt | yes | — | — |
| `notes` | string | opt | yes | — | — |
| `practitioner_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `created_at` | string | REQ (resp) | no | `date-time` | — |
| `updated_at` | string | REQ (resp) | no | `date-time` | — |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

### Enums — the only clinical entity that documents its own

- **`laterality`** — DOCUMENTED: `left`, `right`, `bilateral`, `not_applicable`.
- **`severity`** — DOCUMENTED: `mild`, `moderate`, `severe`, `life-threatening`.
- **`status`** — DOCUMENTED: `active`, `healing`, `resolved`, `chronic`. Default `active`.
  (Also repeated in the `?status=` filter description on `GET /injuries/`.)

### Foreign keys

- `patient_id` → Patient **N:1** required; `practitioner_id` **N:1** optional.
- `injury_type_id` → Injury Type **N:1** optional.
- **Many-to-many:** Condition, Medication, Procedure, Treatment (§27) — four link tables, all
  carrying only `relevance_note`.

### Nested read shape

`InjuryWithRelations` = fields + `injury_type` (typed `InjuryTypeResponse`) + `practitioner`
(untyped object).

---

## 15. Injury Type (user-extensible reference table)

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `name` | string | **REQ** | no | — | `minLength 2`, `maxLength 100` — "e.g., 'Sprain', 'Fracture'" |
| `description` | string | opt | yes | — | `maxLength 300` |
| `is_system` | boolean | REQ (resp) | no | — | "True = system default, False = user-created" |
| `created_at` | string | REQ (resp) | no | `date-time` | — |
| `updated_at` | string | REQ (resp) | no | `date-time` | — |

`InjuryTypeDropdownOption` = {`id`, `name`, `is_system` (default `false`)}.
Endpoints: `GET|POST /injury-types/`, `GET /injury-types/dropdown`, `GET|DELETE /injury-types/{id}`
— **no PUT**. System types can be created and deleted but never renamed.

**This is the only clinical entity given its own reference table.** Allergen, medication, condition,
procedure and symptom names all stay free text. There is no principle here, just history.

---

## 16. Insurance

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | — |
| `insurance_type` | string | **REQ** | no | — | *(enum not declared)* |
| `company_name` | string | **REQ** | no | — | *(no maxLength)* |
| `employer_group` | string | opt | yes | — | — |
| `member_name` | string | **REQ** | no | — | — |
| `member_id` | string | **REQ** | no | — | the policy member number |
| `group_number` | string | opt | yes | — | — |
| `plan_name` | string | opt | yes | — | — |
| `policy_holder_name` | string | opt | yes | — | — |
| `relationship_to_holder` | string | opt | yes | — | *(enum not declared)* |
| `effective_date` | string | **REQ** | no | `date` | — |
| `expiration_date` | string | opt | yes | `date` | — |
| `status` | string | opt | no | — | default `"active"` |
| `is_primary` | boolean | opt | no | — | default `false` |
| `coverage_details` | object | opt | yes | — | **free-form JSON blob** |
| `contact_info` | object | opt | yes | — | **free-form JSON blob** |
| `notes` | string | opt | yes | — | — |
| `created_at` | string | REQ (resp) | no | `date-time` | — |
| `updated_at` | string | REQ (resp) | no | `date-time` | — |

Two untyped JSON columns (`coverage_details`, `contact_info`) with no declared inner shape anywhere
in the document. Whatever the frontend puts in them is the schema.

### Enums

- **`insurance_type`** — DERIVED: `medical`, `dental`, `vision`, `prescription`, `other`. Required,
  no default, unconstrained. (The frontend certainly has a fixed list; the API does not express it.)
- **`status`** — DERIVED: `active`, `inactive`, `expired`, `pending`. Default `"active"`; there is a
  dedicated `PATCH /insurances/{id}/status` taking `InsuranceStatusUpdate {status: string REQ}`.
- **`relationship_to_holder`** — DERIVED: `self`, `spouse`, `child`, `dependent`, `other`.

### Foreign keys

- `patient_id` → Patient **N:1** required. No link tables, no practitioner.
- `is_primary` is a per-patient singleton flag enforced by `PATCH /insurances/{id}/set-primary`
  (the same pattern as emergency contacts).

### Endpoints

`POST|GET /insurances/`, `GET|PUT|DELETE /insurances/{id}`,
`GET /insurances/expiring?days=` → `Insurance[]` (*"Get insurance records expiring within specified
days"* — computed server-side from `expiration_date`; **no `days_until_expiry` field is returned**),
`GET /insurances/search?company=`, `PATCH /{id}/status`, `PATCH /{id}/set-primary`.

---

## 17. Medical Equipment

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `equipment_name` | string | **REQ** | no | — | `minLength 2`, `maxLength 200` |
| `equipment_type` | string | **REQ** | no | — | `minLength 2`, `maxLength 100` *(enum not declared)* |
| `manufacturer` | string | opt | yes | — | `maxLength 200` |
| `model_number` | string | opt | yes | — | `maxLength 100` |
| `serial_number` | string | opt | yes | — | `maxLength 100` |
| `prescribed_date` | string | opt | yes | `date` | — |
| `last_service_date` | string | opt | yes | `date` | — |
| `next_service_date` | string | opt | yes | `date` | — |
| `usage_instructions` | string | opt | yes | — | `maxLength 5000` |
| `status` | string | opt | yes | — | default `"active"`, `maxLength 50` |
| `supplier` | string | opt | yes | — | `maxLength 200` |
| `notes` | string | opt | yes | — | `maxLength 5000` |
| `practitioner_id` | integer | opt | yes | — | `exclusiveMinimum 0` |
| `created_at` | string | opt | yes | `date-time` | — |
| `updated_at` | string | opt | yes | `date-time` | — |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

### Enums

- **`status`** — DERIVED: `active`, `inactive`, `maintenance`, `retired`. Default `"active"`,
  `?status=` filter, `/medical-equipment/active` endpoint.
- **`equipment_type`** — DERIVED: `cpap`, `nebulizer`, `wheelchair`, `walker`, `glucose-meter`,
  `bp-monitor`, `oxygen`, `hearing-aid`, `prosthetic`, `orthotic`, `other`. Required, `maxLength 100`,
  otherwise unconstrained, and `?equipment_type=` filterable.

### Foreign keys

- `patient_id` → Patient **N:1** required; `practitioner_id` **N:1** optional.
- **Many-to-many:** Treatment (§27) — the only link table equipment participates in.

### Endpoints

`GET /medical-equipment/needing-service` — *"Get equipment that needs service soon (within 30 days
or overdue)"*. Computed from `next_service_date`; **returns plain `MedicalEquipmentResponse[]` with
no `days_until_service` / `is_overdue` field**. The 30-day window is hard-coded server-side (no
`?days=` param, unlike insurance's `/expiring`).

---

## 18. Emergency Contact

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | REQ (resp) | no | — | `exclusiveMinimum 0` — **NOT in the create body**; passed as `?patient_id=` |
| `name` | string | **REQ** | no | — | `minLength 2`, `maxLength 100` |
| `relationship` | string | **REQ** | no | — | `minLength 2`, `maxLength 50` *(enum not declared)* |
| `phone_number` | string | **REQ** | no | — | `minLength 1`, `maxLength 20` — no pattern |
| `secondary_phone` | string | opt | yes | — | `maxLength 20` |
| `email` | string | opt | yes | — | `maxLength 100` — **no `format: email`** (contrast Pharmacy) |
| `is_primary` | boolean | opt | no | — | default `false` |
| `is_active` | boolean | opt | no | — | default `true` |
| `address` | string | opt | yes | — | `maxLength 500` |
| `notes` | string | opt | yes | — | `maxLength 5000` |

No timestamps.

### Enums

- **`relationship`** — DERIVED: `spouse`, `partner`, `parent`, `child`, `sibling`, `friend`,
  `guardian`, `caregiver`, `other`. `maxLength 50` free text upstream.

### Foreign keys

- `patient_id` → Patient **N:1** required. Nothing else.
- `is_primary` singleton per patient, enforced by `POST /emergency-contacts/{id}/set-primary`;
  `GET /emergency-contacts/patient/{pid}/primary` reads it back.

`EmergencyContactWithRelations` adds `patient` (untyped object).
Filters: `?is_active=`, `?is_primary=`.

---

## 19. Practitioner

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `name` | string | **REQ** | no | — | *(no maxLength)* |
| `specialty_id` | integer | **REQ** | no | — | `exclusiveMinimum 0` |
| `practice` | string | opt | yes | — | **free-text practice name (legacy)** |
| `practice_id` | integer | opt | yes | — | **FK to Practice (new)** |
| `phone_number` | string | opt | yes | — | — |
| `email` | string | opt | yes | — | no `format: email` |
| `website` | string | opt | yes | — | no `format: uri` |
| `rating` | number | opt | yes | — | **no min/max** |
| `specialty` | string | opt | yes | — | **response-only, denormalised** |
| `specialty_name` | string | opt | yes | — | **response-only, denormalised — duplicate of `specialty`** |
| `practice_name` | string | opt | yes | — | **response-only, denormalised** |
| `created_at` | string | opt | yes | `date-time` | — |
| `updated_at` | string | opt | yes | `date-time` | — |

**Three denormalised display fields, two of which (`specialty`, `specialty_name`) are the same
value under two names.** Plus `practice` (string) and `practice_id` (FK) coexisting — a half-done
normalisation, exactly like `condition_name`/`diagnosis`.

`PractitionerSummary` = {`id`, `name`, `specialty`}.

### Foreign keys

- `specialty_id` → Medical Specialty **N:1 required**. The only *required* FK to a reference table
  in the whole clinical model.
- `practice_id` → Practice **N:1** optional.
- **Inbound N:1** from: Patient (`physician_id`), Condition, Medication, Encounter, Procedure,
  Treatment, Immunization, Injury, Medical Equipment, Vitals, Lab Result, and the
  treatment↔medication link (`specific_prescriber_id`). Twelve inbound references.

---

## 20. Practice

| Field | Type | Req | Nullable | Format |
|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — |
| `name` | string | **REQ** | no | — |
| `phone_number` | string | opt | yes | — |
| `fax_number` | string | opt | yes | — |
| `website` | string | opt | yes | — |
| `patient_portal_url` | string | opt | yes | — |
| `notes` | string | opt | yes | — |
| `locations` | array&lt;`PracticeLocationSchema`&gt; | opt | yes | — |
| `created_at` | string | REQ (resp) | no | `date-time` |
| `updated_at` | string | REQ (resp) | no | `date-time` |

`PracticeLocationSchema` (all optional, all nullable, no constraints):
`label`, `address`, `city`, `state`, `zip`, `phone`.
**Locations are an embedded JSON array, not a table.** Contrast Pharmacy, which flattens the same
six concepts into columns. Two different modelling choices for one concept in one codebase.

`PracticeSummary` = {`id`, `name`}. `PracticeWithPractitioners` = Practice + `practitioner_count`
(integer, default `0`, **computed**).

### Foreign keys

- **1:N** to Practitioner (via `Practitioner.practice_id`).
- Endpoints: `POST|GET /practices/` (`?search=`), `GET /practices/summary`,
  `GET /practices/search/by-name?name=`, `GET|PUT|DELETE /practices/{id}`.

---

## 21. Pharmacy

| Field | Type | Req | Nullable | Format |
|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — |
| `name` | string | **REQ** | no | — |
| `brand` | string | opt | yes | — |
| `street_address` | string | opt | yes | — |
| `city` | string | opt | yes | — |
| `state` | string | opt | yes | — |
| `zip_code` | string | opt | yes | — |
| `country` | string | opt | yes | — |
| `store_number` | string | opt | yes | — |
| `phone_number` | string | opt | yes | — |
| `fax_number` | string | opt | yes | — |
| `email` | string | opt | yes | **`format: email`** |
| `website` | string | opt | yes | — (no `format: uri`) |
| `hours` | string | opt | yes | — (free text) |
| `drive_through` | boolean | opt | yes | default `false` |
| `twenty_four_hour` | boolean | opt | yes | default `false` |
| `specialty_services` | string | opt | yes | — (free text) |
| `created_at` | string | REQ (resp) | no | `date-time` |
| `updated_at` | string | REQ (resp) | no | `date-time` |

**`format: email` on Pharmacy but not on Practitioner or Emergency Contact.** Three email fields,
one validated. `drive_through`/`twenty_four_hour` are `boolean | null` with default `false` — a
pointless tri-state.

### Foreign keys

- **Inbound N:1** from Medication (`pharmacy_id`) and from the treatment↔medication link
  (`specific_pharmacy_id`). No outbound FKs.
- Endpoints: `POST|GET /pharmacies/`, `GET|PUT|DELETE /pharmacies/{id}`. No search endpoint.

---

## 22. Medical Specialty

| Field | Type | Req | Nullable | Format |
|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — |
| `name` | string | **REQ** | no | — |
| `description` | string | opt | yes | — |
| `is_active` | boolean | opt | no | default `true` |
| `created_at` | string | REQ (resp) | no | `date-time` |
| `updated_at` | string | REQ (resp) | no | `date-time` |

`MedicalSpecialtySummary` = {`id`, `name`, `description`, `is_active`} (`is_active` required here,
optional-with-default on the main schema).

Endpoints: `GET|POST /medical-specialties/` **only**. No read-by-id, no update, no delete —
yet `Practitioner.specialty_id` is a *required* FK into it. Create-only, append-forever table.

---

## 23. Lab Result

Lab results are dual-natured: a single scalar result **or** a panel header owning N components.

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `patient_id` | integer | **REQ** | no | — | — |
| `practitioner_id` | integer | opt | yes | — | ordering clinician |
| `test_name` | string | **REQ** | no | — | *(no maxLength)* |
| `test_code` | string | opt | yes | — | LOINC-ish, free text |
| `test_category` | string | opt | yes | — | *(enum not declared)* |
| `test_type` | string | opt | yes | — | *(enum not declared)* |
| `facility` | string | opt | yes | — | free text |
| `status` | string | opt | yes | — | default `"ordered"` |
| `labs_result` | string | opt | yes | — | **free-text overall interpretation; note the field name** |
| `ordered_date` | string | opt | yes | `date` | — |
| `completed_date` | string | opt | yes | `date` | — |
| `notes` | string | opt | yes | — | — |
| `is_panel` | boolean | opt | no | — | default `false`; **create-only, absent from `LabResultUpdate`** |
| `value` | number | opt | yes | — | scalar result (when not a panel) |
| `unit` | string | opt | yes | — | — |
| `ref_range_min` | number | opt | yes | — | — |
| `ref_range_max` | number | opt | yes | — | — |
| `ref_range_text` | string | opt | yes | — | — |
| `created_at` | string | opt | yes | `date-time` | — |
| `updated_at` | string | opt | yes | `date-time` | — |
| `tags` | array&lt;string&gt; | opt | yes | — | default `[]` |

`value`/`unit`/`ref_range_*` are **duplicated verbatim on Lab Test Component**. A non-panel lab
result stores its value on itself; a panel stores values on children. `is_panel` discriminates, and
**it cannot be changed after creation** (not in the Update schema).

### Enums

- **`status`** — DERIVED: `ordered`, `in-progress`, `completed`, `cancelled`. Default `"ordered"`.
  `EncounterLabResultWithDetails.lab_result_status` echoes it for display.
- **`test_category`** — DERIVED: `blood-work`, `imaging`, `pathology`, `microbiology`, `genetics`,
  `urinalysis`, `other`. Also the filter vocabulary on the standardized-test catalog
  (`?category=` on four `/standardized-tests/*` routes) — the two are almost certainly the same
  vocabulary, sourced from the LOINC-derived library, and **not enumerable from the document**.
- **`test_type`** — DERIVED: `routine`, `follow-up`, `emergency`, `screening`. Unconstrained.

### Foreign keys

- `patient_id` → Patient **N:1** required; `practitioner_id` → Practitioner **N:1** optional.
- **1:N** to Lab Test Component; **1:N** to Lab Result File (legacy) / Entity File (current).
- **Many-to-many:** Condition, Medication, Procedure, Treatment, Encounter (§27) — five link
  tables, the most of any entity after Treatment.

### Nested read shape

`LabResultWithRelations` = fields + `patient_name` + `practitioner_name` + `files[]` (untyped, default `[]`).

### Lab Result File (legacy parallel file table)

`LabResultFileResponse`: `id`, `lab_result_id` (REQ), `file_name` (REQ), `file_path` (REQ),
`file_type`, `file_size`, `description`, `uploaded_at` (`date-time`).
**This exists alongside the generic `EntityFileResponse`** (which covers all 13 `EntityType`s
including `lab-result`) and has its own router (`/lab-result-files/*`, 18 operations) with batch
operations, filters by type/date-range/recent, filename search, and storage health. Two complete
file subsystems, one of them lab-only.

---

## 24. Lab Test Component

| Field | Type | Req | Nullable | Format | Constraints |
|---|---|---|---|---|---|
| `id` | integer | REQ (resp) | no | — | — |
| `lab_result_id` | integer | **REQ** | no | — | parent panel |
| `test_name` | string | **REQ** | no | — | *(no maxLength)* |
| `abbreviation` | string | opt | yes | — | e.g. "WBC" |
| `test_code` | string | opt | yes | — | LOINC-ish |
| `canonical_test_name` | string | opt | yes | — | normalised name for trending |
| `value` | number | opt | yes | — | quantitative result |
| `unit` | string | opt | yes | — | — |
| `ref_range_min` | number | opt | yes | — | — |
| `ref_range_max` | number | opt | yes | — | — |
| `ref_range_text` | string | opt | yes | — | — |
| `status` | string | opt | yes | — | *(enum not declared)* |
| `category` | string | opt | yes | — | *(enum not declared)* |
| `display_order` | integer | opt | yes | — | **presentation state in the DB** |
| `result_type` | string | opt | yes | — | default `"quantitative"` |
| `qualitative_value` | string | opt | yes | — | for `result_type = qualitative` |
| `textual_value` | string | opt | yes | — | for `result_type = textual` |
| `notes` | string | opt | yes | — | — |
| `created_at` | string | opt | yes | `date-time` | — |
| `updated_at` | string | opt | yes | `date-time` | — |

**No `tags`, no `patient_id`** (reached via the parent lab result). Three parallel value columns
(`value`, `qualitative_value`, `textual_value`) discriminated by `result_type` — an untagged union
spread across columns with no cross-field validation declared.

### Enums

- **`status`** — DOCUMENTED (endpoint description): the abnormal set is `high`, `low`, `critical`,
  `abnormal` — *"Get all abnormal test results (high, low, critical, abnormal)"*. Add `normal` and
  the full DERIVED set is `normal`, `high`, `low`, `critical`, `abnormal`. Filterable via
  `?status=` on three routes, including *"Filter by latest status"* on the catalog.
- **`result_type`** — DERIVED from the value columns + the default: `quantitative`, `qualitative`,
  `textual`. Echoed on `LabTestComponentTrendResponse`, `LabTestComponentTrendStatistics`,
  `ComponentCatalogEntry`, and `LabTestComponentTrendDataPoint`.
- **`category`** — same DERIVED lab-category vocabulary as `LabResult.test_category`.
- **`trend_direction`** (computed, on `LabTestComponentTrendStatistics` REQ and
  `ComponentCatalogEntry` default `"stable"`) — DERIVED: `increasing`, `decreasing`, `stable`
  (probably plus `insufficient_data`).

### Foreign keys

- `lab_result_id` → Lab Result **N:1** required. Nothing else. No link tables.

### Derived / analytic shapes (all computed, read-only)

- `LabTestComponentForStack` = component + parent's `completed_date`, `ordered_date`, `facility`.
- `LabTestComponentTrendResponse`: `test_name`, `unit`, `category`, `data_points[]`, `statistics`,
  `is_aggregated` (default `false`), `aggregation_period`, `result_type`.
- `LabTestComponentTrendDataPoint`: `id`, `value`, `unit`, `status`, `ref_range_min`,
  `ref_range_max`, `ref_range_text`, `recorded_date`, `created_at`,
  `lab_result` → `LabResultBasicForTrend` {`id`, `test_name`, `completed_date`}, `result_type`,
  `qualitative_value`, `textual_value`.
- `LabTestComponentTrendStatistics`: `count`, `latest`, `average`, `min`, `max`, `std_dev`,
  `trend_direction` (REQ), `time_in_range_percent`, `normal_count`, `abnormal_count`, `result_type`,
  `qualitative_summary` (object&lt;string,integer&gt; — value→frequency histogram).
- `ComponentCatalogEntry` (per unique test name, per patient): `test_name`, `trend_test_name`,
  `abbreviation`, `latest_value`, `latest_qualitative_value`, `latest_textual_value`, `unit`,
  `status`, `category`, `result_type`, `reading_count` (default `1`), `trend_direction`
  (default `"stable"`), `latest_date`, `ref_range_min`, `ref_range_max`, `ref_range_text`.
  Wrapped in `ComponentCatalogResponse` {`items`, `total`}.
- `TestComponentDefaults` (form prefill from the most recent prior entry): `unit`, `ref_range_min`,
  `ref_range_max`, `ref_range_text`, `category`, `abbreviation`.
- `LabTestComponentBulkResponse`: `created_count`, `components[]`, `errors[]`.

The `?unit=` param on `/patient/{pid}/trends` documents a real bug being worked around:
*"Scopes the trend to a single unit so values recorded in different units are not merged. Omit on
legacy templates for backward-compatible merged behavior."* — i.e. upstream has historically
plotted mg/dL and mmol/L on one axis.

---

## 25. Standardized catalogs (read-only reference libraries)

### 25.1 Standardized Test (`StandardizedTestResponse`)

| Field | Type | Req | Nullable |
|---|---|---|---|
| `id` | integer | REQ | no |
| `loinc_code` | string | REQ | **yes** |
| `test_name` | string | REQ | no |
| `short_name` | string | REQ | **yes** |
| `default_unit` | string | REQ | **yes** |
| `category` | string | REQ | **yes** |
| `common_names` | array&lt;string&gt; | REQ | **yes** |
| `is_common` | boolean | REQ | no |

Every field is `required` in the JSON-Schema sense **and** nullable — the FastAPI "always present,
possibly null" style. Read-only: no Create/Update schemas exist.

Endpoints: `/standardized-tests/search`, `/autocomplete` (→ `AutocompleteOption` {`value`, `label`,
`loinc_code`, `default_unit`, `category`}), `/common`, `/count`, `/by-loinc/{code}`,
`/by-name/{name}`, `/by-category/{category}`, `POST /batch-match`
(`BatchMatchRequest {test_names[]}` → `BatchMatchResult {test_name, matched_test}`).
Maintained via `/admin/maintenance/test-library/{info,reload,sync}`.

### 25.2 Standardized Vaccine (`StandardizedVaccineResponse`)

| Field | Type | Req | Nullable |
|---|---|---|---|
| `id` | integer | REQ | no |
| `who_code` | string | REQ | **yes** — WHO PCMT code, the business key |
| `vaccine_name` | string | REQ | no |
| `short_name` | string | REQ | **yes** |
| `category` | `VaccineCategory` | REQ | **yes** — **DECLARED enum** |
| `common_names` | array&lt;string&gt; | REQ | **yes** |
| `is_combined` | boolean | REQ | no |
| `components` | array&lt;string&gt; | REQ | **yes** — canonical disease keys |
| `default_manufacturer` | string | REQ | **yes** |
| `is_common` | boolean | REQ | no |

`VaccineAutocompleteOption` = {`value`, `label`, `who_code`, `short_name`, `category`,
`is_combined`, `components`}.
Endpoints mirror the test catalog: `/search`, `/autocomplete`, `/common`, `/count`,
`/by-who-code/{code}`, `/by-name/{name}`, `/by-category/{category}`.

**Only these two catalogs exist.** There is no standardized medication, condition, allergen or
procedure library — so `medication_name`, `diagnosis`, `allergen` and `procedure_name` are
un-normalisable free text, and cross-record trending/deduplication is impossible for them.

---

## 26. Tags

There is **no Tag entity schema** in the document. Tags are:

1. a `array<string>` column on 11 clinical entities (§1.5), and
2. a separate "user tags registry" row with an `id` and a `color`, exposed only through verbs.

| Schema | Fields |
|---|---|
| `TagCreateRequest` | `tag` (string, REQ) |
| `TagColorUpdateRequest` | `color` (string \| null, opt) |

Endpoints (all responses untyped `object`/`array<string>`):
`GET /tags/autocomplete?q&limit`, `GET /tags/popular?entity_types&limit`,
`GET /tags/suggestions?entity_type&limit`, `GET /tags/search?tags&entity_types&limit_per_entity&match_mode&patient_id`,
`POST /tags/create`, `PUT /tags/rename?old_tag&new_tag`, `PUT /tags/replace?old_tag&new_tag`,
`DELETE /tags/delete?tag`, `PATCH /tags/{tag_id}/color`.

Two enum-ish things here:

- **`match_mode`** — DOCUMENTED: `any` (OR) / `all` (AND). Default `"any"`. The same concept is
  spelled `tag_match_all` (boolean) on every entity list endpoint. **Two encodings of one filter.**
- **`entity_types`** — DECLARED as a *default array value* on `/tags/popular` and `/tags/search`:
  `["lab_result", "medication", "condition", "procedure", "immunization", "treatment", "encounter", "allergy"]`.
  **Eight, snake_case** — versus the 13 kebab-case values in `EntityType`. Symptoms, injuries,
  insurance, vitals and medical equipment carry tags (or don't) but are not searchable by them.
  **A third spelling of the entity-type vocabulary.**

Tag rename/replace/delete operate by **string matching across every entity's JSON array** — there is
no join table, so these are O(entities) write amplification and can't be transactional per-tag.

---

## 27. The many-to-many LINK tables — all 17

Every link table is an independent Postgres table with a surrogate `id`, its own
`created_at`/`updated_at`, its own REST sub-resource, and its own 3–4 Pydantic schemas
(`*Create`, `*Response`, `*Update`, `*WithDetails`, sometimes `*BulkCreate`).
**121 fields across 17 tables, of which 98 are `id` + two FKs + `created_at` + `updated_at`
boilerplate.** Only 23 fields carry actual relationship data.

### 27.1 Full matrix

| # | Link | Left | Right | Card. | Payload fields on the relationship | Bulk? | Update? |
|---|---|---|---|---|---|---|---|
| 1 | `condition_medications` | Condition | Medication | M:N | `relevance_note` | ✅ `medication_ids[]` + shared `relevance_note` (`maxLength 500`, `minItems 1`) | ✅ |
| 2 | `lab_result_conditions` | Lab Result | Condition | M:N | `relevance_note` | ❌ | ✅ |
| 3 | `lab_result_medications` | Lab Result | Medication | M:N | `relevance_note` | ❌ | ✅ |
| 4 | `lab_result_procedures` | Lab Result | Procedure | M:N | `relevance_note` | ❌ | ✅ |
| 5 | `lab_result_treatments` | Lab Result | Treatment | M:N | `purpose` (`maxLength 50`), `expected_frequency` (`maxLength 100`), `relevance_note` | ❌ | (via #9) |
| 6 | `encounter_lab_results` | Encounter | Lab Result | M:N | `purpose`, `relevance_note` | ✅ `lab_result_ids[]` | ✅ |
| 7 | `treatment_encounters` | Treatment | Encounter | M:N | `visit_label` (`maxLength 50`), `visit_sequence` (int, `minimum 1`), `relevance_note` | ✅ `encounter_ids[]` (`minItems 1`) | ✅ |
| 8 | `treatment_equipment` | Treatment | Medical Equipment | M:N | `usage_frequency` (`maxLength 100`), `specific_settings` (`maxLength 300`), `relevance_note` | ✅ `equipment_ids[]` | ✅ |
| 9 | `treatment_lab_results` | Treatment | Lab Result | M:N | `purpose` (`maxLength 50`), `expected_frequency` (`maxLength 100`), `relevance_note` | ✅ `lab_result_ids[]` + `purpose` | ✅ |
| 10 | `treatment_medications` | Treatment | Medication | M:N | `specific_dosage` (200), `specific_frequency` (100), `specific_duration` (100), `timing_instructions` (300), `specific_prescriber_id` (FK Practitioner), `specific_pharmacy_id` (FK Pharmacy), `specific_start_date` (date), `specific_end_date` (date), `relevance_note` | ✅ `medication_ids[]` | ✅ |
| 11 | `injury_conditions` | Injury | Condition | M:N | `relevance_note` | ❌ | ❌ |
| 12 | `injury_medications` | Injury | Medication | M:N | `relevance_note` | ❌ | ❌ |
| 13 | `injury_procedures` | Injury | Procedure | M:N | `relevance_note` | ❌ | ❌ |
| 14 | `injury_treatments` | Injury | Treatment | M:N | `relevance_note` | ❌ | ❌ |
| 15 | `symptom_conditions` | Symptom | Condition | M:N | `relevance_note` | ❌ | ❌ |
| 16 | `symptom_medications` | Symptom | Medication | M:N | `relationship_type` (string, default `"related_to"`), `relevance_note` | ❌ | ❌ |
| 17 | `symptom_treatments` | Symptom | Treatment | M:N | `relevance_note` | ❌ | ❌ |

**No `linked_at` field exists anywhere** — the task brief guessed one; upstream uses `created_at`.
All 17 `*Response` link schemas carry `created_at` and `updated_at`, but not identically: the three
`Symptom*Response` links declare them **required, non-nullable** `date-time`, while the other
fourteen declare them **optional and nullable**. One more inconsistency in a table type that is
otherwise pure boilerplate.

### 27.2 Relationship payload enums

- **`symptom_medications.relationship_type`** — DERIVED, default `"related_to"`. Plausible full set
  given the clinical meaning: `related_to`, `treats`, `causes`, `worsens`. **The only link table with
  a semantic type**, and the only one that needs one: a medication can *treat* a symptom or *cause*
  it as a side effect, and upstream can't tell those apart in the other 16 tables.
- **`purpose`** (on lab↔treatment, lab↔encounter, treatment↔lab) — `maxLength 50`, unconstrained.
  DERIVED: `baseline`, `monitoring`, `diagnostic`, `follow-up`.
- **`visit_label`** (treatment↔encounter) — `maxLength 50`, unconstrained. DERIVED: `initial`,
  `follow-up`, `final`, `check-in`.

### 27.3 The bidirectional-route duplication

Three pairs are reachable from **both** sides with **different DTOs for the same rows**:

| Table | Left-side route + DTO | Right-side route + DTO |
|---|---|---|
| `encounter_lab_results` | `POST /encounters/{id}/lab-results` — `EncounterLabResultCreate {lab_result_id, purpose, relevance_note}` | `POST /lab-results/{id}/encounters` — `LabResultEncounterCreate {encounter_id, purpose, relevance_note}` |
| " (bulk) | `EncounterLabResultBulkCreate {lab_result_ids[]}` | `LabResultEncounterBulkCreate {encounter_ids[]}` |
| `treatment_lab_results` | `POST /treatments/{id}/lab-results` — `TreatmentLabResultCreate` | `POST /lab-results/{id}/treatments` — `LabResultTreatmentCreate` |
| `condition_medications` | `POST /conditions/{id}/medications` — `ConditionMedicationCreate` | `GET /conditions/medication/{medication_id}/conditions` (read-only reverse) |
| `treatment_medications` | `POST /treatments/{id}/medications` — `TreatmentMedicationCreate` | `GET /medications/{id}/treatments` → `MedicationTreatmentResponse` + `MedicationTreatmentInfo` + `MedicationTreatmentCondition` (three bespoke schemas) |

Every one of these pairs writes the same table. That's ~8 extra schemas and ~10 extra endpoints
purchasing exactly nothing.

### 27.4 Also note: two link tables are *not* modelled as links

- `Allergy.medication_id` — a plain N:1 FK where a link table would be more consistent (an allergy
  to a drug *class* affects many medication records).
- `Injury.treatment_received` (free text) sits **beside** the `injury_treatments` link table. The
  same information, twice, one structured and one not.

---

## 28. Complete FK / cardinality graph

```
User ──1:N──▶ Patient ──1:N──▶ { Allergy, Condition, Medication, Encounter, Procedure,
                                 Treatment, Symptom, Vitals, Immunization, Injury,
                                 Insurance, MedicalEquipment, EmergencyContact, LabResult }
Patient ──N:1──▶ Practitioner            (physician_id, primary care)
Patient ──1:1──▶ PatientPhoto

MedicalSpecialty ◀──N:1 (REQUIRED)── Practitioner ──N:1──▶ Practice
Practice ⊃ locations[]  (embedded JSON, not a table)

Practitioner ◀──N:1 (optional)── Condition, Medication, Encounter, Procedure, Treatment,
                                  Immunization, Injury, MedicalEquipment, Vitals, LabResult,
                                  treatment_medications.specific_prescriber_id
Pharmacy     ◀──N:1 (optional)── Medication, treatment_medications.specific_pharmacy_id

Condition ◀──N:1── Encounter.condition_id, Procedure.condition_id, Treatment.condition_id
Medication ◀──N:1── Allergy.medication_id
InjuryType ◀──N:1── Injury.injury_type_id
StandardizedVaccine ◀──N:1── Immunization.standardized_vaccine_id

Symptom ──1:N──▶ SymptomOccurrence
LabResult ──1:N──▶ LabTestComponent
LabResult ──1:N──▶ LabResultFile            (legacy, lab-only)
{13 EntityTypes} ──1:N──▶ EntityFile        (generic, current)

M:N (17 link tables):
  Condition   ↔ Medication
  LabResult   ↔ { Condition, Medication, Procedure, Treatment, Encounter }
  Treatment   ↔ { Encounter, MedicalEquipment, LabResult, Medication }
  Injury      ↔ { Condition, Medication, Procedure, Treatment }
  Symptom     ↔ { Condition, Medication, Treatment }
```

Entities participating in **zero** link tables: Patient, Vitals, Insurance, EmergencyContact,
Immunization, LabTestComponent, InjuryType, Practitioner, Practice, Pharmacy, MedicalSpecialty,
and the two standardized catalogs.

---

## 29. Computed / derived read-only fields the API returns

Nothing in the document is marked `readOnly: true`. The following are computed server-side and
appear only in response schemas:

| Field | On | Meaning |
|---|---|---|
| `occurrence_count` | `SymptomResponse` | count of child occurrences (default `0`) |
| `last_occurrence_date` | `SymptomResponse` | max occurrence date; absent from Create/Update |
| `practitioner_count` | `PracticeWithPractitioners` | count of practitioners |
| `patient_name`, `practitioner_name` | `EncounterWithRelations`, `LabResultWithRelations` | denormalised display strings |
| `specialty`, `specialty_name`, `practice_name` | `Practitioner` | denormalised display strings (two of them identical) |
| `effective_dosage`, `effective_frequency`, `effective_start_date`, `effective_end_date`, `effective_prescriber`, `effective_pharmacy` | `TreatmentMedicationWithDetails` | **COALESCE of link-level `specific_*` over medication-level defaults** — the most interesting derived logic in the API |
| `components`, `is_combined`, `is_library_matched` | `ImmunizationHistoryItem` | resolved from the standardized-vaccine catalog |
| `diseases_index`, `unmatched_count` | `ImmunizationHistoryResponse` | server-side aggregation |
| `trend_direction`, `average`, `min`, `max`, `std_dev`, `time_in_range_percent`, `normal_count`, `abnormal_count`, `latest`, `count`, `qualitative_summary` | `LabTestComponentTrendStatistics` | full trend stats |
| `reading_count`, `trend_direction`, `latest_value`, `latest_qualitative_value`, `latest_textual_value`, `latest_date` | `ComponentCatalogEntry` | per-test rollup |
| `is_aggregated`, `aggregation_period` | `LabTestComponentTrendResponse` | downsampling metadata |
| `completed_date`, `ordered_date`, `facility` | `LabTestComponentForStack` | lifted from the parent lab result |
| `is_duplicate` | `VitalsPreviewRow` | import dedup flag |
| all 12 fields | `VitalsStats` | averages + current values + `weight_change` |
| all 11 counts | `PatientDashboardStats` | per-type record counts |
| `total_count`, `owned_count`, `shared_count` | `PatientListResponse` | |
| `permission_level` | `patient_management.PatientResponse` | caller's effective permission |
| `paperless_has_token`, `paperless_has_credentials`, `papra_has_token` | `UserPreferencesResponse` | "is a secret set" flags (secret never returned) |
| `count`, `has_more` | `CategorySummary` | |
| `total_records`, `last_updated` | `DataSummaryResponse` | |

**Derived-by-endpoint, never as a field** (the server filters but returns the plain DTO, so the
client cannot show *why* a row matched):
`/allergies/patient/{pid}/critical`, `/allergies/patient/{pid}/active`,
`/conditions/patient/{pid}/active`, `/treatments/ongoing`, `/treatments/patient/{pid}/active`,
`/procedures/scheduled`, `/injuries/patient/{pid}/active`, `/medical-equipment/active`,
`/medical-equipment/needing-service` (30 days or overdue), `/insurances/expiring?days=`,
`/lab-test-components/components/abnormal`, `/encounters/patient/{pid}/recent?days=30`,
`/procedures/patient/{pid}/recent?days=90`, `/immunizations/patient/{pid}/recent?days=365`,
`/immunizations/patient/{pid}/booster-check/{vaccine}?months_interval=12`.

**Explicitly NOT present anywhere in the document** (contrary to the brief's expectations):
`age`, `days_until_expiry`, `is_active` as a derived field, `is_overdue`, `bmi` as a derived field,
`linked_at`.

---

## 30. Canonical enum set for MediKube

Consolidating §0–§27. `*` marks a value MediKube introduces or renames; everything unmarked is
DECLARED or DOCUMENTED upstream. Every one of these becomes a PocketBase `select` field
(single-select unless noted) **and** a Go string-typed constant set with a `Valid()` method.

### Severity — ONE ladder, used everywhere

`mild` · `moderate` · `severe` · `life_threatening`
(upstream spells it `life-threatening`; MediKube standardises on snake_case for all enum values.)
Used by: allergies, conditions, injuries, symptom occurrences. Symptom occurrences upstream only
used 3 values; the 4th is simply never selected there. **One enum, four entities.**

### Clinical lifecycle status — ONE ladder per shape

- **Ongoing-condition shape** (conditions, allergies, symptoms, injuries):
  `active` · `inactive` · `resolved` · `chronic`*
  (injuries additionally use `healing` — DOCUMENTED — which MediKube keeps as a 5th value on this set.)
- **Order/event shape** (procedures, lab results):
  `ordered`*/`scheduled` · `in_progress` · `completed` · `cancelled`
  (procedures use `scheduled`, labs use `ordered`, for the same state. MediKube: keep both names on
  one enum rather than two enums — `scheduled` for procedures, `ordered` for labs, both meaning
  "not yet done".)
- **Course-of-therapy shape** (medications, treatments, medical equipment):
  `active` · `on_hold` · `completed` · `stopped` · `cancelled`
  (equipment reads `stopped` as "retired"; MediKube adds `retired`* for equipment only.)

### Per-entity enums

| Entity.field | Values | Tier |
|---|---|---|
| `injury.laterality` | `left`, `right`, `bilateral`, `not_applicable` | DOCUMENTED |
| `medication.medication_type` | `prescription`, `otc`, `supplement`, `herbal`* | DERIVED |
| `medication.route` | `oral`, `sublingual`, `topical`, `transdermal`, `inhalation`, `nasal`, `ophthalmic`, `otic`, `rectal`, `vaginal`, `intramuscular`, `subcutaneous`, `intravenous`, `other` | DERIVED (all `*`) |
| `immunization.route` | `intramuscular`, `subcutaneous`, `intradermal`, `oral`, `intranasal` | DERIVED (all `*`) |
| `immunization.site` | `left_arm`, `right_arm`, `left_thigh`, `right_thigh`, `oral`, `nasal`, `other` | DERIVED (all `*`) |
| `procedure.procedure_setting` | `outpatient`, `inpatient`, `office` | DOCUMENTED |
| `procedure.procedure_type` | `surgical`, `diagnostic`, `therapeutic`*, `preventive`*, `other`* | part DOCUMENTED |
| `procedure.outcome` | `successful`, `partial`, `unsuccessful`, `complications` | DERIVED (all `*`) |
| `procedure.anesthesia_type` | `none`, `local`, `regional`, `sedation`, `general` | DERIVED (all `*`) |
| `encounter.visit_type` | `office`, `telehealth`, `urgent_care`, `emergency`, `inpatient`, `follow_up`, `annual`, `other` | DERIVED (all `*`) |
| `encounter.priority` | `routine`, `urgent`, `emergency` | DERIVED (all `*`) |
| `treatment.treatment_category` | `inpatient`, `outpatient` | DOCUMENTED |
| `symptom_occurrence.impact_level` | `none`, `mild`, `moderate`, `severe` | DOCUMENTED-ish |
| `symptom_occurrence.time_of_day` | **DROP** — redundant with `occurrence_at` | — |
| `symptom.category` | `pain`, `respiratory`, `gastrointestinal`, `neurological`, `cardiovascular`, `musculoskeletal`, `dermatological`, `psychological`, `constitutional`, `other` | DERIVED (all `*`) |
| `vitals.glucose_context` | `fasting`, `before_meal`, `after_meal`, `random` | DOCUMENTED |
| `insurance.insurance_type` | `medical`, `dental`, `vision`, `prescription`, `other` | DERIVED (all `*`) |
| `insurance.relationship_to_holder` | `self`, `spouse`, `child`, `dependent`, `other` | DERIVED (all `*`) |
| `emergency_contact.relationship` | `spouse`, `partner`, `parent`, `child`, `sibling`, `friend`, `guardian`, `caregiver`, `other` | DERIVED (all `*`) |
| `patient.relationship_to_self` | same set as above + `self` | DERIVED |
| `patient.blood_type` | `A+`,`A-`,`B+`,`B-`,`AB+`,`AB-`,`O+`,`O-`,`unknown` | DERIVED (all `*`) |
| `patient.gender` | `female`, `male`, `other`, `prefer_not_to_say` | DERIVED (all `*`) |
| `medical_equipment.equipment_type` | `cpap`, `nebulizer`, `wheelchair`, `walker`, `glucose_meter`, `bp_monitor`, `oxygen`, `hearing_aid`, `prosthetic`, `orthotic`, `other` | DERIVED (all `*`) |
| `lab_test_component.status` | `normal`, `high`, `low`, `critical`, `abnormal` | DOCUMENTED |
| `lab_test_component.result_type` | `quantitative`, `qualitative`, `textual` | DERIVED |
| `lab_result.test_category` / `component.category` | one shared vocabulary, seeded from the standardized-test library's `category` values (not enumerable from the spec) | DERIVED |
| `link.relation` (see §31.3) | `treats`, `caused_by`, `worsens`, `monitors`, `related_to` | DERIVED, replaces `relationship_type` |
| `trend_direction` (computed) | `increasing`, `decreasing`, `stable`, `insufficient_data`* | DERIVED |
| `share.permission_level` | `view`, `edit`, `full` | DOCUMENTED |
| `unit_system` | `imperial`, `metric` | DECLARED (`pattern`) |
| `export.format` | `json`, `csv`, `pdf` | DECLARED |
| `vaccine.category` | `viral`, `bacterial`, `combined`, `toxoid`, `parasitic`, `other` | DECLARED (lowercased) |
| `file.entity_type` | one canonical list — see §31.2 | DECLARED, deduplicated |

---

# REIMAGINED MODEL RECOMMENDATIONS

## R1 — Collapse the 17 link tables into ~6 PocketBase multi-relation fields (the single biggest win)

PocketBase's `relation` field type has `maxSelect > 1` natively: one field on one collection stores
an ordered set of related record ids, with referential integrity, cascade-delete options, and
`?expand=` on read. **Any link table whose only payload is `relevance_note` does not need to exist.**

Twelve of the seventeen tables carry nothing but `relevance_note`: `lab_result_conditions`,
`lab_result_medications`, `lab_result_procedures`, all four `injury_*`, `symptom_conditions`,
`symptom_treatments`, `condition_medications`, and effectively `encounter_lab_results`.

**Kill all of them.** Replace with multi-relation fields, choosing an owning side per pair:

| Owning collection | Multi-relation field | Targets |
|---|---|---|
| `lab_results` | `conditions`, `medications`, `procedures`, `encounters` | 4 tables gone |
| `injuries` | `conditions`, `medications`, `procedures`, `treatments` | 4 tables gone |
| `symptoms` | `conditions`, `treatments` | 2 tables gone |
| `conditions` | `medications` | 1 table gone |

That's **11 tables, ~44 endpoints and ~35 schemas deleted** for eleven field declarations. Reverse
navigation comes free via PocketBase back-relations (`?expand=lab_results_via_conditions`) — no
second route, no second DTO, which also kills the bidirectional duplication in §27.3 outright.

**What about `relevance_note`?** It is `string | null`, unconstrained, on 16 of 17 tables, and there
is no evidence in the spec that it is ever populated per-pair meaningfully rather than as a shared
blanket note (the `*BulkCreate` schemas take **one** `relevance_note` for the whole batch — proof
that upstream itself treats it as per-operation, not per-pair). MediKube drops it. If a user needs to
say why two records relate, the owning record's `notes` field is where that belongs.

## R2 — Keep exactly five real join collections, because they carry real clinical data

These five have payload that is genuinely a property *of the relationship*, so they stay as
collections with two single-relation fields plus their payload:

| Collection | Pair | Kept payload | Why it must stay |
|---|---|---|---|
| `treatment_medications` | Treatment ↔ Medication | `dosage`, `frequency`, `duration`, `timing_instructions`, `prescriber` (rel), `pharmacy` (rel), `start_date`, `end_date` | A drug's dose *within a specific treatment protocol* differs from its standing dose. This is the one place upstream's `effective_*` COALESCE logic (§29) earns its keep — port it verbatim as a computed response field. |
| `treatment_encounters` | Treatment ↔ Encounter | `visit_label`, `visit_sequence` | An ordered visit series is a property of the pair, not of either side. |
| `treatment_equipment` | Treatment ↔ Equipment | `usage_frequency`, `specific_settings` | CPAP pressure for *this* therapy. |
| `treatment_lab_results` | Treatment ↔ Lab Result | `purpose`, `expected_frequency` | Monitoring cadence. |
| `symptom_links` | Symptom ↔ {Condition, Medication, Treatment} | `relation` (enum) | See R3. |

Note four of the five are Treatment's. Treatment is the only entity in MediKeep that is genuinely a
*protocol* rather than a *record*, and that is exactly why it needs qualified joins. This also
retires the `mode: simple|advanced` flag (R6).

## R3 — Give symptom links the one thing they were missing: a typed relation

`symptom_medications.relationship_type` (default `related_to`) is the only semantic edge type in the
whole model, and it is the most clinically valuable field in any link table: it distinguishes
"ibuprofen treats my headache" from "lisinopril causes my cough". Upstream has it on exactly one of
seventeen tables.

MediKube: **one** `symptom_links` collection —
`symptom` (rel), `target_collection` (enum: `conditions`|`medications`|`treatments`),
`target_id` (text), `relation` (enum: `treats`|`caused_by`|`worsens`|`monitors`|`related_to`).

Three tables → one, and it gains meaning rather than losing it. (Polymorphic target is acceptable
here precisely because it is *one* small collection queried from one direction; do **not** generalise
this into a universal edge table — see R9.)

## R4 — Deduplicate the fields that say the same thing twice

Upstream has eight confirmed redundant pairs. Every one collapses:

| Drop | Keep | Evidence |
|---|---|---|
| `condition.condition_name` | `condition.diagnosis` | `ConditionDropdownOption` exposes only `diagnosis` |
| `treatment.treatment_type` | `treatment.treatment_category` | identical descriptions, differing only in `maxLength` |
| `encounter.chief_complaint` | `encounter.reason` | same concept; `reason` is the required one |
| `practitioner.specialty` **and** `practitioner.specialty_name` | `?expand=specialty` | two names for one denormalised string |
| `practitioner.practice` (string) | `practitioner.practice` (relation) | half-finished normalisation |
| `symptom_occurrence.time_of_day` | `symptom_occurrence.occurred_at` (date-time) | a bucketed duplicate of a timestamp |
| `injury.treatment_received` (text) | `injury.treatments` (multi-relation) | structured beats prose |
| `vitals.bmi` | computed from `weight`+`height` | both are on the same row |
| `lab_result.labs_result` | `lab_result.interpretation` (rename) | the name is a typo-grade artefact |

Also merge the **two file subsystems** (`/lab-result-files/*`, 18 operations, and
`/entity-files/*`, 16 operations) into one PocketBase `file` field per collection plus one `attachments` collection for
metadata. And merge the **two Patient response shapes** into one, always including
`owner`, `is_self_record`, `privacy_level`, `permission_level`.

Field count drops from ~499 to roughly **330** with zero loss of clinical information.

## R5 — Fix the type-safety holes upstream left open

These are cheap and they are all real bugs:

1. **`vitals` has no numeric bounds at all.** Add `min`/`max` to every column
   (systolic 40–300, diastolic 20–200, heart_rate 20–300, temperature 90–115 °F, SpO2 50–100,
   respiratory_rate 4–80, glucose 20–800, a1c 3–20, pain_scale 0–10). PocketBase number fields take
   `min`/`max` natively.
2. **`recorded_date` round-trips lossily** (`date-time` in, formatless nullable string out).
   One `autodate`-adjacent `date` field, RFC3339, always non-null.
3. **`pain_scale` is unvalidated** in two places (vitals, symptom occurrences). Bound both 0–10.
4. **`patient.height`/`weight` have `maximum` only on update.** Same constraints on both paths —
   trivially free when create and update share one DTO validated by one function.
5. **Store vitals in SI, convert at the edge.** Upstream stores imperial and has an
   `imperial|metric` preference; that guarantees float drift on every round-trip. Store kg/cm/°C/
   mmol-L, convert in the response DTO using the user's `unit_system`.
6. **Split `{date, time}` column pairs into single timestamps.** Symptom occurrences have
   `occurrence_date`+`occurrence_time` and `resolved_date`+`resolved_time`; that's four columns doing
   two jobs and it makes "did this resolve before that started" unanswerable in SQL.
7. **Validate emails.** Three email fields, one `format: email`. PocketBase has an `email` field type.
8. **Document `reminder_days`.** Upstream ships `array<integer>` with no declared encoding.
   Use ISO-8601 weekday numbers (1=Mon…7=Sun) and say so.

## R6 — Delete presentation state from the domain

Three fields are UI state that leaked into Postgres:

- `treatment.mode` (`simple`|`advanced`) — a form-complexity toggle. Under R2 the "advanced" link
  editors are just optional relations; a treatment with no links *is* the simple case. Delete.
- `lab_test_component.display_order` — belongs in the client, or is `test_code` ordering from the
  standardized-test catalog. Delete.
- `tags` colour registry — colour is a display preference; keep it, but on the tag record, not
  smeared across entities (R7).

## R7 — Make tags a real relation

Upstream stores tags as a JSON `array<string>` on eleven entities **and** keeps a separate registry
row for id+colour, so `rename`/`replace`/`delete` are string rewrites across eleven tables, with a
tag vocabulary that is spelled three different ways (§26). It also means the tag filter has two
encodings (`tag_match_all` boolean vs `match_mode: any|all`).

MediKube: one `tags` collection (`name` unique per owner, `color`), and a multi-relation `tags` field
on every taggable collection. Rename becomes a one-row update. Tag search becomes one PocketBase
filter expression. One `match` param (`any`|`all`), one vocabulary.

## R8 — Collapse the reference-data tier

Upstream has four reference tables with four different lifecycle rules:
`medical_specialties` (create+list only — yet a *required* FK), `injury_types` (create/list/delete,
no update, with an `is_system` flag), `standardized_tests` (read-only, admin-synced),
`standardized_vaccines` (read-only, admin-synced).

MediKube: **two** shapes.

- **Catalogs** (read-only, seeded/synced): `standardized_tests`, `standardized_vaccines`. Keep both;
  they are genuinely valuable (the vaccine one powers `diseases_index`, the test one powers unit and
  ref-range prefill). Full CRUD is admin-only via PocketBase's own UI.
- **User-extensible lookups** (full CRUD + `is_system`): `medical_specialties`, `injury_types`, and
  — new — `condition_catalog` and `medication_catalog` if we ever want cross-record trending on
  those names. One collection shape, one set of rules, `is_system` guarding deletes.

And make `practitioner.specialty` **optional**. A required FK into a table with no read-by-id
endpoint is upstream's worst single modelling decision.

## R9 — Do NOT build a generic polymorphic attachment/link table

The temptation, having deleted eleven link tables, is to add one universal
`(from_collection, from_id, to_collection, to_id, note)` edge table. Resist it. PocketBase's
`?expand=` and filter syntax only work across declared relation fields; a polymorphic edge table
would forfeit referential integrity, cascade deletes, expansion, and every API-rule-based access
check, and would push all of that into hand-written Go. The eleven explicit multi-relation fields
in R1 are more code in the schema and dramatically less code in the service layer.

The one exception is `symptom_links` (R3), where the polymorphism is bounded to three targets, the
collection is small, and it is only ever read from the symptom side.

## R10 — Replace `?required_permission=` with server-side authorization

Upstream lets the **client** state which permission the server should enforce
(`?required_permission=view`, default `view`, on **41** operations). Even if the server also checks the
share record, this is an authorization parameter under caller control and it is duplicated forty
times. MediKube: permission is derived from `(auth token, patient id)` inside a single middleware, the
patient is resolved from the route or the active-patient claim, and the parameter does not exist.

Similarly, retire the dual patient-scoping mechanism (§1.3): one canonical form,
`/api/v1/patients/{patientID}/<resource>`, with `me` as an alias for the self-record. That alone
removes the four-way duplicated list routes (`/allergies/`, `/allergies/patients/{pid}/allergies/`,
`/patients/{pid}/allergies/`, and the `?patient_id=` variant of the first) — a large share of
upstream's 500 operations is this one pattern repeated per entity.

## R11 — Promote the derived fields upstream computes but never returns

Fifteen endpoints (§29) filter on a derived predicate and then return the plain DTO, so the client
re-derives what the server just computed. Add these as computed response fields and delete most of
those endpoints in favour of one list endpoint with filters:

| Add to | Field |
|---|---|
| `patients` | `age` (int, from `birth_date`) |
| `insurance` | `days_until_expiry` (int, nullable), `is_expired` (bool) |
| `medical_equipment` | `days_until_service` (int, nullable), `needs_service` (bool) |
| `allergies` | `is_critical` (bool — `severity in (severe, life_threatening) and status == active`) |
| `conditions`, `medications`, `treatments`, `injuries`, `symptoms` | `is_active` (bool) |
| `vitals` | `bmi` (computed), and MAP if wanted |
| `symptoms` | keep `occurrence_count`, `last_occurrence_date` (upstream already does this well) |
| `immunizations` | keep `components`, `is_combined`, `is_library_matched` |
| `lab_test_components` | `is_abnormal` (bool), derived from `status` |

`/allergies/patient/{pid}/critical`, `/critical`, `/active` × 5, `/scheduled`, `/ongoing`,
`/expiring`, `/needing-service`, `/abnormal`, `/recent` × 3 → **one** `GET` per collection with
`?status=`, `?is_active=`, `?severity=`, `?since=`. Roughly 15 endpoints become 0.

## R12 — Where upstream is right, keep it

Not everything needs reimagining:

- **Symptom / SymptomOccurrence split** is correct and is the only entity modelled as
  definition + episodes. Extend the pattern rather than flattening it.
- **LabResult / LabTestComponent split** with `is_panel` is correct. Make `is_panel` derived
  (`components.length > 0`) instead of a create-only column, and move the scalar
  `value`/`unit`/`ref_range_*` off `lab_results` entirely: a single-analyte result is a lab result
  with exactly one component. That deletes five duplicated columns and the `is_panel` discriminator.
- **Standardized catalogs with an asymmetric business-key write / surrogate-id read**
  (`standardized_vaccine_who_code` in, `standardized_vaccine_id` out, explicit `null` to unlink) is
  a genuinely good API pattern for "pick from library or type free text". Keep it, and apply the
  same shape to the test catalog.
- **`diseases_index` pre-aggregation** on immunization history is the right call — the client
  should not re-derive vaccine→disease coverage.
- **The `effective_*` COALESCE** on treatment↔medication is real domain logic. Port it.
- **`Practice.locations` as an embedded array** is right for a practice with three offices. Apply
  the same to Pharmacy (which flattens the identical six fields into columns) rather than the reverse.
