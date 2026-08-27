# Phase 1 Data Model: Clinical Records

**Feature**: `003-clinical-records` | **Date**: 2026-08-26

Sixteen new PocketBase collections, one altered existing collection, four shared enum
vocabularies, twenty per-kind vocabularies, six multi-relation link fields and one payload-carrying
join. Consistent with the shared design contract §1; deviations are listed in `plan.md`.

---

## 0. Rules that hold for every collection in this phase

1. **Ids are PocketBase's 15-character opaque text ids.** No integers. Never an identifier in a
   `number` field (float64, ~2^53 safe).
2. Every collection has `id`, `created` (autodate), `updated` (autodate). Not repeated below.
   `updated` is the ETag source (`W/"<updated as RFC3339Nano>"`).
3. **Every record collection has `patient`** —
   `RelationField{CollectionId: patients, Required: true, MaxSelect: 1, CascadeDelete: true}`.
   `patient` is **immutable after create** (FR-002); the `Patch` DTO omits it, and the service
   refuses a differing value with `409 conflict`.
4. **All five API rules (`ListRule`, `ViewRule`, `CreateRule`, `UpdateRule`, `DeleteRule`) are
   `nil`** on every collection here. Superuser-only. Asserted at boot; MediKube refuses to start
   otherwise. Proved per collection by a `tests.ApiScenario` returning 404 to a normal user.
5. **Zero file fields are added by this phase.** The `Protected: true` boot assertion continues to
   cover exactly `patients.photo`.
6. Enum fields are `core.SelectField{MaxSelect: 1}` whose `Values` are populated from the Go
   vocabulary's `All()`, so the two cannot drift.
7. `notes` is `text`, max 5000, optional, on every clinical kind. It is PHI and is redacted in
   `MarshalZerologObject`.
8. `tags` is `RelationField{CollectionId: tags, MaxSelect: 0}` on every clinical kind, including
   `medications` (added here by migration).
9. **No `deleted_at` anywhere.** Records are hard deleted (Constitution VII).
10. `*_on` = date-only (`YYYY-MM-DD`, never timezone-shifted). `*_at` = instant (RFC3339 UTC).
11. Uniqueness is a collection index (`AddIndex`); PocketBase has no per-field `Unique`.
12. Every migration is registered via `migrations.Register(up, down, filename)` — the signature
    requires both directions, so Principle IX's reversibility rule is structural.

---

## 1. Shared enum vocabularies

Defined once in `internal/domain/clinical/vocab.go` as string types with `Valid() bool` and
`All() []string`. All values `snake_case`.

### `Severity` — one ladder for four kinds

`mild` · `moderate` · `severe` · `life_threatening`
Used by: allergy, condition, injury, symptom, and `FamilyCondition`.
**No catch-all**: the ladder is complete and ordered; an `other` would break the ordering that
FR-018 depends on.

### `ConditionStatus` — the "is it still going on" ladder

`active` · `healing` · `inactive` · `resolved` · `chronic`
Used by: allergy, condition, injury, symptom, and `FamilyCondition`.
**No catch-all**: complete.

### `OrderStatus` — the ordered-event ladder

`ordered` · `scheduled` · `in_progress` · `completed` · `cancelled`
Used by: procedure (and lab_result in phase 004). Procedures use `scheduled`; labs use `ordered`.
**No catch-all**: complete.

### `TherapyStatus` — the course-of-therapy ladder

`active` · `on_hold` · `completed` · `stopped` · `cancelled`
Used by: medication (phase 001), treatment, equipment.
**No catch-all**: complete.

### State transitions

The four ladders are **not** state machines: any value may be set to any other, because a carer
correcting a mis-record must be able to. Two transition *rules* exist and are validated:

| Rule | Enforced by |
|---|---|
| `condition.status = resolved` **requires** `resolved_on`, which must be ≥ `onset_on` and ≤ today | `Condition.Validate()` (FR-020) |
| `procedure.status = completed` **forbids** a future `occurred_on`; `ordered`/`scheduled` permit it | `Procedure.Validate()` (FR-025) |

Leaving `resolved` clears nothing automatically; `resolved_on` may be cleared only by moving the
status off `resolved` in the same patch.

---

## 2. Per-kind vocabularies

Each in `internal/domain/clinical/vocab_<kind>.go`. `†` marks a catch-all.

| Vocabulary | Values |
|---|---|
| `Laterality` | `left` `right` `bilateral` `not_applicable` |
| `MedicationType` | `prescription` `otc` `supplement` `herbal` |
| `MedicationRoute` | `oral` `sublingual` `topical` `transdermal` `inhalation` `nasal` `ophthalmic` `otic` `rectal` `vaginal` `intramuscular` `subcutaneous` `intravenous` `other`† |
| `ImmunizationRoute` | `intramuscular` `subcutaneous` `intradermal` `oral` `intranasal` |
| `ImmunizationSite` | `left_arm` `right_arm` `left_thigh` `right_thigh` `oral` `nasal` `other`† |
| `ProcedureSetting` | `outpatient` `inpatient` `office` |
| `ProcedureType` | `surgical` `diagnostic` `therapeutic` `preventive` `other`† |
| `ProcedureOutcome` | `successful` `partial` `unsuccessful` `complications` |
| `Anesthesia` | `none` `local` `regional` `sedation` `general` |
| `VisitType` | `office` `telehealth` `urgent_care` `emergency` `inpatient` `follow_up` `annual` `other`† |
| `VisitPriority` | `routine` `urgent` `emergency` |
| `TreatmentSetting` | `inpatient` `outpatient` `home` |
| `SymptomCategory` | `pain` `respiratory` `gastrointestinal` `neurological` `cardiovascular` `musculoskeletal` `dermatological` `psychological` `constitutional` `other`† |
| `SymptomImpact` | `none` `mild` `moderate` `severe` |
| `GlucoseContext` | `fasting` `before_meal` `after_meal` `random` |
| `InsuranceType` | `medical` `dental` `vision` `prescription` `other`† |
| `InsuranceStatus` | `active` `inactive` `expired` `pending` |
| `HolderRelationship` | `self` `spouse` `child` `dependent` `other`† |
| `ContactRelationship` | `spouse` `partner` `parent` `child` `sibling` `friend` `guardian` `caregiver` `other`† |
| `EquipmentType` | `cpap` `nebulizer` `wheelchair` `walker` `glucose_meter` `bp_monitor` `oximeter` `oxygen` `hearing_aid` `prosthetic` `orthotic` `other`† |
| `InjuryType` | `sprain` `strain` `fracture` `dislocation` `laceration` `contusion` `burn` `concussion` `puncture` `abrasion` `other`† — **this replaces upstream's user-extensible `injury_types` collection entirely** (FR-040, US4 scenario 3) |
| `FamilyRelationship` | `mother` `father` `sister` `brother` `daughter` `son` `grandmother` `grandfather` `aunt` `uncle` `cousin` `niece` `nephew` `half_sibling` `other`† |
| `Sex` (reused from `patients`, phase 002) | `female` `male` `intersex` `unspecified` |

---

## 3. The kind registry

`internal/domain/kind/kind.go` gains thirteen values. Each carries three spellings from one
constant (shared design contract §2.1 rule 2), plus its registry metadata.

| `kind.Kind` (enum / `snake_case`) | Path segment (plural kebab) | Collection | Page landmark (list / detail) |
|---|---|---|---|
| `allergy` | `allergies` | `allergies` | `region[name="Allergies"]` / `article[name="Allergy"]` |
| `condition` | `conditions` | `conditions` | `region[name="Conditions"]` / `article[name="Condition"]` |
| `encounter` | `encounters` | `encounters` | `region[name="Encounters"]` / `article[name="Encounter"]` |
| `procedure` | `procedures` | `procedures` | `region[name="Procedures"]` / `article[name="Procedure"]` |
| `treatment` | `treatments` | `treatments` | `region[name="Treatments"]` / `article[name="Treatment"]` |
| `symptom` | `symptoms` | `symptoms` | `region[name="Symptoms"]` / `article[name="Symptom episode"]` |
| `vitals` | `vitals` | `vitals` | `region[name="Measurements"]` / `article[name="Measurement set"]` |
| `immunization` | `immunizations` | `immunizations` | `region[name="Vaccinations"]` / `article[name="Vaccination"]` |
| `injury` | `injuries` | `injuries` | `region[name="Injuries"]` / `article[name="Injury"]` |
| `insurance` | `insurance` | `insurances` | `region[name="Insurance"]` / `article[name="Insurance policy"]` |
| `equipment` | `equipment` | `equipment` | `region[name="Equipment"]` / `article[name="Equipment"]` |
| `emergency_contact` | `emergency-contacts` | `emergency_contacts` | `region[name="Emergency contacts"]` / `article[name="Emergency contact"]` |
| `family_member` | `family-history` | `family_members` | `region[name="Family history"]` / `article[name="Relative"]` |

Existing: `medication` / `medications` / `medications` (phase 001, extended here).

Two path segments are deliberately not mechanical plurals (`insurance`, `family-history`), which is
why the segment is a declared constant rather than a derived string. `vitals` and `equipment` are
already plural. `kind_test.go` asserts the mapping is total, injective in both directions, and that
every value round-trips.

---

## 4. Record collections

Every one carries the universal fields from §0 (`patient`, `tags`, `notes`) plus the fields below.
`R` = required. Text lengths are maxima. Every text field listed as PHI is redacted in logging.

### 4.1 `allergies` — US1

| Field | Type | R | Notes |
|---|---|---|---|
| `allergen` | text 2..200 | ✓ | PHI |
| `reaction` | text ≤500 | | PHI |
| `severity` | select `Severity` | ✓ | FR-016 |
| `status` | select `ConditionStatus` | ✓ | default `active` |
| `onset_on` | date | | primary date |
| `medications` | relation → `medications`, MaxSelect 0 | | FR-017 — a drug-class allergy touches many rows |

Index: `idx_allergies_patient_onset (patient, onset_on, id)`.
**FR-018**: `AllergySummary.critical` is derived, not stored:
`severity ∈ {severe, life_threatening} AND status ∈ {active, chronic}`.

### 4.2 `conditions` — US1

| Field | Type | R | Notes |
|---|---|---|---|
| `diagnosis` | text 2..500 | ✓ | PHI |
| `status` | select `ConditionStatus` | ✓ | |
| `severity` | select `Severity` | | |
| `onset_on` | date | | primary date |
| `resolved_on` | date | | required when `status = resolved` (FR-020) |
| `icd10_code` | text ≤10 | | |
| `snomed_code` | text ≤20 | | |
| `practitioner` | relation → `practitioners`, MaxSelect 1 | | cleared, not cascaded, on practitioner delete |
| `medications` | relation → `medications`, MaxSelect 0 | | |

Index: `idx_conditions_patient_onset (patient, onset_on, id)`, `idx_conditions_patient_status (patient, status)`.
**FR-021** is satisfied entirely by back-relation traversal: `encounters_via_condition`,
`procedures_via_condition`, `treatments_via_condition`, `symptoms_via_conditions`,
`injuries_via_conditions`, and the forward `medications` field. Nothing is recorded twice.

### 4.3 `encounters` — US2

| Field | Type | R | Notes |
|---|---|---|---|
| `reason` | text 1..300 | ✓ | PHI. Upstream's `chief_complaint` is merged in. |
| `occurred_on` | date | ✓ | primary date |
| `visit_type` | select `VisitType` | | |
| `priority` | select `VisitPriority` | | |
| `assessment` | text ≤5000 | | PHI. **Named `assessment`, not `diagnosis`** (FR-023) |
| `plan` | text ≤5000 | | PHI |
| `follow_up` | text ≤2000 | | PHI |
| `duration_minutes` | number ≥0 | | |
| `practitioner` | relation → `practitioners`, MaxSelect 1 | | |
| `facility` | relation → `facilities`, MaxSelect 1 | | |
| `condition` | relation → `conditions`, MaxSelect 1 | | FR-022; drives `encounters_via_condition` |
| `lab_results` | relation → `lab_results`, MaxSelect 0 | | **declared in phase 004's migration, not here** |

Index: `idx_encounters_patient_date (patient, occurred_on, id)`.

### 4.4 `procedures` — US2

| Field | Type | R | Notes |
|---|---|---|---|
| `name` | text 2..300 | ✓ | PHI |
| `type` | select `ProcedureType` | | |
| `code` | text ≤50 | | |
| `description` | text ≤5000 | | PHI |
| `occurred_on` | date | ✓ | primary date. Future permitted iff `status ∈ {ordered, scheduled}` (FR-025) |
| `status` | select `OrderStatus` | ✓ | |
| `outcome` | select `ProcedureOutcome` | | |
| `setting` | select `ProcedureSetting` | | |
| `complications` | text ≤500 | | PHI |
| `duration_minutes` | number ≥0 | | |
| `anesthesia` | select `Anesthesia` | | |
| `anesthesia_notes` | text ≤2000 | | PHI |
| `practitioner` / `facility` / `condition` | relations, MaxSelect 1 | | |

Index: `idx_procedures_patient_date (patient, occurred_on, id)`, `idx_procedures_patient_status (patient, status)`.
**FR-026**: `?scheduled=true` ≡ `status ∈ {ordered, scheduled}`; row `basis` is `scheduled` or `ordered`.

### 4.5 `treatments` — US2

| Field | Type | R | Notes |
|---|---|---|---|
| `name` | text 2..300 | ✓ | PHI |
| `type` | text ≤120 | | free text; the domain has no closed vocabulary here |
| `setting` | select `TreatmentSetting` | | |
| `description` | text ≤5000 | | PHI |
| `started_on` | date | | primary date |
| `ended_on` | date | | ≥ `started_on` |
| `frequency` | text ≤100 | | |
| `dosage` | text ≤200 | | |
| `expected_outcome` | text ≤300 | | PHI |
| `status` | select `TherapyStatus` | | |
| `practitioner` / `facility` / `condition` | relations, MaxSelect 1 | | |
| `encounters` | relation → `encounters`, MaxSelect 0 | | FR-028 |
| `equipment` | relation → `equipment`, MaxSelect 0 | | FR-028 |
| `lab_results` | relation → `lab_results`, MaxSelect 0 | | **phase 004's migration** |

Index: `idx_treatments_patient_started (patient, started_on, id)`.
Medications attach through `treatment_medications` (§5.2), never through a relation field.

### 4.6 `symptoms` — US3. **One row per episode.**

| Field | Type | R | Notes |
|---|---|---|---|
| `name` | text 1..200 | ✓ | PHI. The grouping key for FR-031. |
| `category` | select `SymptomCategory` | | |
| `severity` | select `Severity` | ✓ | |
| `occurred_at` | **instant** | ✓ | primary date |
| `duration_minutes` | number ≥0 | | |
| `pain_scale` | number 0..10 | | |
| `body_site` | text ≤120 | | PHI |
| `triggers` | json `[]string` | | validated: ≤20 entries, each ≤80 chars |
| `relief_methods` | json `[]string` | | same validation |
| `impact` | select `SymptomImpact` | | |
| `resolved_at` | instant | | ≥ `occurred_at` |
| `is_chronic` | bool | | default false |
| `status` | select `ConditionStatus` | | |
| `conditions` | relation → `conditions`, MaxSelect 0 | | FR-032 |
| `treatments` | relation → `treatments`, MaxSelect 0 | | FR-032 |
| `treated_by_medications` | relation → `medications`, MaxSelect 0 | | FR-032 role 1 |
| `caused_by_medications` | relation → `medications`, MaxSelect 0 | | FR-032 role 2 |

Index: `idx_symptoms_patient_at (patient, occurred_at, id)`, `idx_symptoms_patient_name (patient, name, occurred_at)`.

**Derived, never stored** (FR-031, FR-090, SC-016): `SymptomSummary.episode_count` and
`.last_occurred_at` are a correlated aggregate over `(patient, LOWER(name))`. There is **no**
`symptom_definitions` collection, no `occurrence_count` column and no `time_of_day` column
(it duplicated `occurred_at`).

### 4.7 `vitals` (measurement sets) — US3

| Field | Type | R | Range |
|---|---|---|---|
| `recorded_at` | **instant** | ✓ | primary date, not future |
| `systolic_mmhg` | number | | 40..300 |
| `diastolic_mmhg` | number | | 20..200 |
| `heart_rate_bpm` | number | | 20..300 |
| `respiratory_rate_bpm` | number | | 4..80 |
| `temperature_c` | number | | 25..45 |
| `spo2_pct` | number | | 50..100 |
| `weight_kg` | number | | 0.5..450 |
| `height_cm` | number | | 30..272 |
| `glucose_mmol_l` | number | | 0.5..60 |
| `glucose_context` | select `GlucoseContext` | | |
| `hba1c_pct` | number | | 2..20 |
| `pain_scale` | number | | 0..10 |
| `device` | text ≤120 | | |
| `practitioner` | relation, MaxSelect 1 | | |

Index: `idx_vitals_patient_at (patient, recorded_at, id)`.

**Cross-field rules**

- **FR-034**: at least one of the thirteen measurement fields must be non-null. A set with none is
  refused with `code: at_least_one_measurement`.
- **FR-036**: `systolic_mmhg` and `diastolic_mmhg` are both-or-neither, and
  `diastolic_mmhg < systolic_mmhg`.
- **FR-035**: an out-of-range value is refused with the range named in `fields[].message`.

**Storage is SI; conversion is at the edge (FR-037).** `bmi` is derived at render from
`weight_kg / (height_cm/100)²` and **is never stored** — upstream stored it.

### 4.8 `immunizations` (vaccinations) — US4

| Field | Type | R | Notes |
|---|---|---|---|
| `vaccine_name` | text 2..200 | ✓ | PHI |
| `trade_name` | text ≤200 | | |
| `administered_on` | date | ✓ | primary date, not future |
| `dose_number` | number | | **integer ≥ 1** (FR-039) |
| `lot_number` | text ≤50 | | |
| `manufacturer` | text ≤200 | | |
| `site` | select `ImmunizationSite` | | |
| `route` | select `ImmunizationRoute` | | |
| `expires_on` | date | | ≥ `administered_on` |
| `practitioner` / `facility` | relations, MaxSelect 1 | | |

Index: `idx_immunizations_patient_date (patient, administered_on, id)`.
**No `catalog_vaccine` relation.** The spec explicitly defers a standardised vaccine library;
adding it later is one reversible migration.

### 4.9 `injuries` — US4

| Field | Type | R | Notes |
|---|---|---|---|
| `name` | text 2..300 | ✓ | PHI |
| `type` | select `InjuryType` | | fixed vocabulary with a catch-all (FR-040, US4-3) |
| `body_part` | text 1..100 | ✓ | |
| `laterality` | select `Laterality` | | includes `not_applicable` (FR-041) |
| `occurred_on` | date | | primary date, not future |
| `mechanism` | text ≤500 | | PHI |
| `severity` | select `Severity` | | |
| `status` | select `ConditionStatus` | | default `active` |
| `recovery_notes` | text ≤2000 | | PHI |
| `practitioner` | relation, MaxSelect 1 | | |
| `conditions` / `medications` / `procedures` / `treatments` | relations, MaxSelect 0 | | FR-042 |

Index: `idx_injuries_patient_date (patient, occurred_on, id)`, `idx_injuries_patient_status (patient, status)`.
**FR-040 note**: upstream's free-text `treatment_received` is dropped — the `treatments` relation is
the structured version.
**US4-5**: `?unresolved=true` ≡ `status ∈ {active, healing}`.

### 4.10 `insurances` — US5

| Field | Type | R | Notes |
|---|---|---|---|
| `type` | select `InsuranceType` | ✓ | |
| `company` | text 1..200 | ✓ | |
| `plan_name` | text ≤200 | | |
| `employer_group` | text ≤200 | | |
| `member_name` | text 1..200 | ✓ | **PHI** (FR-047) |
| `member_id` | text 1..80 | ✓ | **PHI** (FR-047, US5-5) |
| `group_number` | text ≤80 | | **PHI** (FR-047) |
| `holder_name` | text ≤200 | | **PHI** (FR-047) |
| `relationship_to_holder` | select `HolderRelationship` | | |
| `effective_on` | date | ✓ | primary date |
| `expires_on` | date | | ≥ `effective_on` |
| `status` | select `InsuranceStatus` | | |
| `is_primary` | bool | | default false |
| `coverage` | json → `Coverage` | | validated struct, §6.2 |
| `contact` | json → `Contact` | | validated struct, §6.3 |

Indexes: `idx_insurances_patient_eff (patient, effective_on, id)`,
`idx_insurances_patient_expires (patient, expires_on)`,
**partial unique** `uniq_insurances_primary (patient) WHERE is_primary = 1`.
**FR-046**: `?expiring_within_days=` (default 60) ≡ `expires_on BETWEEN today AND today+N`; row
`basis` is `expiring`.
**FR-045**: setting primary displaces the previous primary in one transaction and returns
`displaced`.

### 4.11 `equipment` — US5

| Field | Type | R | Notes |
|---|---|---|---|
| `name` | text 2..200 | ✓ | |
| `type` | select `EquipmentType` | ✓ | fixed vocabulary with catch-all |
| `manufacturer` | text ≤200 | | |
| `model` | text ≤100 | | |
| `serial` | text ≤100 | | PHI-adjacent; redacted |
| `prescribed_on` | date | | primary date |
| `serviced_on` | date | | |
| `service_due_on` | date | | ≥ `serviced_on` |
| `instructions` | text ≤5000 | | PHI |
| `status` | select `TherapyStatus` | | |
| `supplier` | relation → `facilities`, MaxSelect 1 | | |
| `practitioner` | relation, MaxSelect 1 | | |

Indexes: `idx_equipment_patient_presc (patient, prescribed_on, id)`,
`idx_equipment_patient_due (patient, service_due_on)`.
**FR-049**: `?service_due_within_days=` (default 30). Row `basis` is `overdue`
(`service_due_on < today`) or `due_soon` (`today ≤ service_due_on ≤ today+N`) — the two are
distinguished per row, which FR-049 requires by name.

### 4.12 `emergency_contacts` — US1

| Field | Type | R | Notes |
|---|---|---|---|
| `name` | text 2..100 | ✓ | PHI |
| `relationship` | select `ContactRelationship` | ✓ | |
| `phone` | text 1..40 | ✓ | PHI |
| `phone_alt` | text ≤40 | | PHI |
| `email` | email | | PHI |
| `address` | text ≤500 | | PHI |
| `is_primary` | bool | | default false |
| `is_active` | bool | | default true |

Indexes: `idx_contacts_patient_sort (patient, is_active, is_primary, name)`,
**partial unique** `uniq_contacts_primary (patient) WHERE is_primary = 1`.
**No primary date.** Default sort is `is_active DESC, is_primary DESC, LOWER(name) ASC, id DESC`
(FR-051). Displacement behaves as insurance (§4.10).

### 4.13 `family_members` — US10

| Field | Type | R | Notes |
|---|---|---|---|
| `name` | text 1..100 | ✓ | PHI |
| `relationship` | select `FamilyRelationship` | ✓ | |
| `sex` | select `Sex` | | reuses the `patients` vocabulary |
| `birth_year` | number | | integer 1850..2200 |
| `death_year` | number | | integer 1850..2200, ≥ `birth_year` (FR-054) |
| `is_deceased` | bool | | default false |
| `conditions` | json → `[]FamilyCondition` | | validated, §6.1 (FR-053) |

Index: `idx_family_patient (patient, relationship, name, id)`.
**No primary date.** Default sort `relationship ASC, LOWER(name) ASC, id DESC`.
`family_members` is a record kind here and becomes a `shares.resource_kind` in phase 005; nothing
in this phase anticipates that.

### 4.14 `medications` — **altered**, phase 001's collection

One migration adds `tags RelationField{CollectionId: tags, MaxSelect: 0}`. Nothing else changes.
Medications become the target of five relation fields defined on other collections
(`allergies.medications`, `conditions.medications`, `injuries.medications`,
`symptoms.treated_by_medications`, `symptoms.caused_by_medications`) and of the
`treatment_medications` join. All of those live on the *other* side, so no further migration of
`medications` is needed. Its `down` removes the `tags` field.

---

## 5. Non-record collections

### 5.1 `tags` — US7

| Field | Type | R | Notes |
|---|---|---|---|
| `owner` | relation → `users`, MaxSelect 1, CascadeDelete | ✓ | **tags belong to the account, not the patient** |
| `name` | text 1..40 | ✓ | PHI-adjacent; a tag may name a condition. Redacted in logs. |
| `color` | text, pattern `^#[0-9a-fA-F]{6}$` | | |

Index: **unique** `uniq_tags_owner_name (owner, LOWER(name))` — FR-063, case-insensitive.
`usage_count` is **derived** at read time by counting referencing records across all kinds
(FR-068, FR-066), never stored.

**Why `owner` and not `patient`**: FR-062 and the spec's Decisions section — "An account holder
organises their whole household's care with one set of tags; a shared installation never
discloses one household's tags to another" (FR-005 of tag privacy, FR-062, US7-5).

### 5.2 `treatment_medications` — US6, the one payload-carrying join

| Field | Type | R | Notes |
|---|---|---|---|
| `treatment` | relation → `treatments`, MaxSelect 1, CascadeDelete | ✓ | |
| `medication` | relation → `medications`, MaxSelect 1, CascadeDelete | ✓ | |
| `dosage` | text ≤200 | | course-specific |
| `frequency` | text ≤100 | | course-specific |
| `duration` | text ≤100 | | course-specific |
| `timing` | text ≤300 | | course-specific |
| `prescriber` | relation → `practitioners`, MaxSelect 1 | | course-specific |
| `pharmacy` | relation → `facilities`, MaxSelect 1 | | course-specific |
| `started_on` / `ended_on` | date | | course-specific, `ended_on ≥ started_on` |

Index: **unique** `uniq_treatment_medication (treatment, medication)` — FR-061.
**No `patient` field**: the patient is the treatment's, and the same-patient invariant (D-08)
is validated in the service on every upsert. Adding a third `patient` copy would create a third
place for it to disagree.
**`effective_*` and `*_source` are computed, never stored** (FR-060, research D-09).
Cascade delete on both sides is what makes FR-058 true for this edge.

### 5.3 `search_index` — US8

| Field | Type | R | Notes |
|---|---|---|---|
| `patient` | relation → `patients`, MaxSelect 1, CascadeDelete | ✓ | the authorization anchor; cascade is what makes FR-087 true here |
| `kind` | select — the 14 kind values | ✓ | |
| `record_id` | text 15 | ✓ | |
| `title` | text ≤500 | ✓ | PHI |
| `body` | text ≤8000 | | PHI |
| `occurred_on` | date | | the source row's primary date; null-last ordering |
| `tags` | relation → `tags`, MaxSelect 0 | | mirrors the source row, so `?tags=` narrows search |

Indexes: **unique** `uniq_search_record (kind, record_id)`,
`idx_search_patient_kind (patient, kind, occurred_on, id)`.
Written by the post-commit hooks registered by `records.Register`; a create/update upserts, a
delete removes. **It stores no content the source row does not already store.**

### 5.4 `audit_events` — **altered**, phase 001's collection

Additive vocabulary extension only (research D-19). Phase 001's migration declares the shared
design contract's **complete** vocabulary — twenty actions and twenty-three target kinds,
including all fifteen record kinds — so the thirteen kinds this phase builds are already declared
and this phase adds only the two values the contract does not name:

- `target_kind` gains **`tag`** (a tag is an auditable resource, FR-059) and **`search`** (the
  target kind of the row a search writes, [D-12](./research.md#d-12--fr-075-the-search-term-is-a-first-class-secret)). The thirteen new record
  kinds are **already present** from phase 001 and are asserted, not added.
- `action` gains **nothing**. `access_denied` arrives with `audit_events` in phase 001
  (001 plan post-design re-check item 2, 001 research D-20), not here, so that refusals are not
  encoded two ways either side of this phase. `read_sensitive` likewise exists from phase 001.

**After this phase: twenty-one actions, twenty-seven target kinds.** The migration's test asserts
that **complete** set, not this delta, so a value this phase writes but no migration declared is a
red test rather than a failed `SelectField` validation in production (ANALYSIS C1).

No new fields, no content, no diffs. Down migration removes the added values.

---

## 6. Validated JSON value objects

Typed Go structs with `Validate()`, marshalled into a PocketBase `JSONField`. They are **not**
free-form blobs and **not** collections (research D-18).

### 6.1 `FamilyCondition` — `family_members.conditions`

```
name           text 1..300   required, PHI
icd10_code     text ≤10
diagnosed_age  int 0..130
severity       Severity
status         ConditionStatus
notes          text ≤2000    PHI
```

List max 50 entries. Each entry validated independently; **all** offences reported together.

### 6.2 `Coverage` — `insurances.coverage`

```
deductible         decimal ≥0
oop_max            decimal ≥0        must be ≥ deductible when both present
copay_primary      decimal ≥0
copay_specialist   decimal ≥0
copay_er           decimal ≥0
coinsurance_pct    number 0..100
currency           text, ISO-4217 3-letter uppercase, required when any amount is present (FR-044)
```

Money is stored as a string-encoded decimal in JSON and handled as an integer minor unit in Go —
never `float64`. FR-044 requires "each with a stated currency", so an amount without `currency` is
`422`.

### 6.3 `Contact` — `insurances.contact`

```
phone         text ≤40    PHI
claims_phone  text ≤40    PHI   (FR-044 names it specifically)
website       url ≤300
portal_url    url ≤300          (FR-044 names it specifically)
address       text ≤500   PHI
```

---

## 7. Relationships — the complete map

### 7.1 Multi-relation fields (no collection, no endpoints)

| From | Field | To | Requirement |
|---|---|---|---|
| `allergies` | `medications` | `medications` | FR-017 |
| `conditions` | `medications` | `medications` | FR-021 |
| `injuries` | `conditions`, `medications`, `procedures`, `treatments` | 4 targets | FR-042 |
| `symptoms` | `conditions`, `treatments`, `treated_by_medications`, `caused_by_medications` | 4 targets | FR-032 |
| `treatments` | `encounters`, `equipment` | 2 targets | FR-028 |

### 7.2 Single relations (`MaxSelect: 1`, reference cleared on target delete)

`conditions.practitioner`; `encounters.{practitioner, facility, condition}`;
`procedures.{practitioner, facility, condition}`; `treatments.{practitioner, facility, condition}`;
`symptoms` (none); `vitals.practitioner`; `immunizations.{practitioner, facility}`;
`injuries.practitioner`; `equipment.{supplier, practitioner}`.

Deleting a practitioner or facility **clears the reference and preserves the record** — established
in phase 002, re-asserted here per kind by a test (spec Edge Cases).

### 7.3 Back-relations (read-only, no storage)

`conditions` reads `encounters_via_condition`, `procedures_via_condition`,
`treatments_via_condition`, `symptoms_via_conditions`, `injuries_via_conditions`.
`medications` reads `allergies_via_medications`, `conditions_via_medications`,
`injuries_via_medications`, `symptoms_via_treated_by_medications`,
`symptoms_via_caused_by_medications`, and its `treatment_medications` rows.
`equipment` reads `treatments_via_equipment`. `encounters` reads `treatments_via_encounters`.

FR-055 ("editable from either end, recorded once") is satisfied because there is exactly one stored
side; the other end is a traversal. FR-059 ("show what type it is and enough identifying detail")
is satisfied by returning the target's `*Summary` DTO, never its full detail.

### 7.4 Invariants on every link mutation

| Invariant | Rule | Failure |
|---|---|---|
| Same patient | both records' **stored** `patient` must match | `404 not_found`, disclosing nothing (FR-057, D-08) |
| No duplicate | a multi-relation set is a set; re-adding is idempotent | silent no-op, not an error (FR-056) |
| Actor may edit both | `Authorizer.Record(actor, kind, id, PermEdit)` on **both** ends | `404 not_found` |
| Version | `If-Match` on the owning record | `412 version_mismatch` (spec Edge Cases: "the same rule applies to attaching and detaching a link") |

---

## 8. Migration plan and ordering

Migrations are hand-written Go under `internal/store/migrations/`, registered into
`core.AppMigrations`, each with a real `down`. Order matters — a relation field cannot reference a
collection that does not exist yet.

| # | Migration | Creates / alters | Must run after |
|---|---|---|---|
| 1 | `..._tags.go` | `tags` + unique index | — |
| 2 | `..._search_index.go` | `search_index` + indexes | — |
| 3 | `..._medication_tags.go` | `medications.tags` | 1 |
| 4 | `..._conditions.go` | `conditions` (no `medications` field yet) | 1 |
| 5 | `..._allergies.go` | `allergies` (no `medications` field yet) | 1 |
| 6 | `..._emergency_contacts.go` | `emergency_contacts` + partial unique | 1 |
| 7 | `..._encounters.go` | `encounters` (incl. `condition`) | 1, 4 |
| 8 | `..._procedures.go` | `procedures` | 1, 4 |
| 9 | `..._equipment.go` | `equipment` | 1 |
| 10 | `..._treatments.go` | `treatments` (incl. `encounters`, `equipment`) | 1, 4, 7, 9 |
| 11 | `..._symptoms.go` | `symptoms` (incl. all four link fields) | 1, 4, 10 |
| 12 | `..._vitals.go` | `vitals` | 1 |
| 13 | `..._immunizations.go` | `immunizations` | 1 |
| 14 | `..._injuries.go` | `injuries` (incl. all four link fields) | 1, 4, 8, 10 |
| 15 | `..._insurances.go` | `insurances` + partial unique | 1 |
| 16 | `..._family_members.go` | `family_members` | 1 |
| 17 | `..._links_medications.go` | adds `medications` to `allergies`, `conditions`, `injuries`; `treated_by_/caused_by_` to `symptoms` | 4, 5, 11, 14 |
| 18 | `..._treatment_medications.go` | `treatment_medications` + unique index | 10 |
| 19 | `..._audit_vocab.go` | adds `tag` and `search` to `audit_events.target_kind`; asserts the complete twenty-one-action / twenty-seven-target-kind set | — |

Migration 17 exists because `allergies`, `conditions`, `injuries` and `symptoms` all point at
`medications` **and** at each other; splitting the link fields into a later migration is what keeps
the dependency graph acyclic. Its `down` removes those fields.

Every migration's `up` sets all five API rules to `nil` explicitly, and a boot assertion
(`internal/platform/pb/assert.go`, phase 001) refuses to start if any is non-nil.

After this phase the committed fixture data dir `internal/testdata/pb_data` is regenerated by
running the migrations against a clean database plus `medikube seed`, and committed — every
`tests.NewTestApp` clones it (VERIFIED-SOURCE-FACTS FACT 7).

---

## 9. Entity roll-up

| | Before 003 | Added | After 003 |
|---|---|---|---|
| Collections | 6 (`users`, `patients`, `medications`, `audit_events`, `practitioners`, `facilities`) | 16 | **22** |
| Registered record kinds | 1 (`medication`) | 13 | **14** |
| `/api/v1` operations | 42 registered (22 from 001, 20 from 002) | 8 | **50** of the 94 across six phases (SHARED-DESIGN §2.3) |
| Enum vocabularies | — | 4 shared + 20 per-kind | 24 |
| Multi-relation link fields | 0 | 11 fields across 5 kinds | 11 |
| Payload-carrying joins | 0 | 1 | 1 |
| Validated JSON value objects | 0 | 3 | 3 |
| File fields | 1 (`patients.photo`) | **0** | 1 |
