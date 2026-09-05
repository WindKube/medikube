---

description: "Task list for MediKube phase 002 — Patient Core"
---

# Tasks: Patient Core

**Input**: Design documents from `/specs/002-patient-core/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: **MANDATORY, not optional.** Constitution Principle III is non-negotiable, the
specification requests tests explicitly (FR-054, FR-055, FR-056) and the success criteria repeat it
(SC-012, SC-013). Every test task is sequenced **before** the implementation task it covers, and no
implementation task is started until its test is red.

**Organization**: grouped by user story in the spec's priority order, so each story is
independently implementable, testable and demonstrable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelisable — different file, no dependency on an incomplete task
- **[Story]**: `[US1]`…`[US6]`, mapping to the spec's user stories. Setup, Foundational and Polish
  carry no story label.

## Path Conventions

Single Go module rooted at `medikube/`. All paths below are relative to that directory and match the
Project Structure section of [plan.md](./plan.md).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: make the workspace ready. No behaviour ships in this phase.

- [x] T001 [P] Create `internal/domain/person/doc.go` and `internal/domain/directory/doc.go` with package comments stating that these packages import only the standard library (Principle II)
- [x] T002 [P] Create service package skeletons with `doc.go` in `internal/service/patient/`, `internal/service/practitioner/`, `internal/service/facility/` and their `patienttest/`, `practitionertest/`, `facilitytest/` subpackages
- [x] T003 [P] Create store package skeletons with `doc.go` in `internal/store/patient/`, `internal/store/practitioner/`, `internal/store/facility/`, each stating it is a `[PB]` package permitted to import PocketBase
- [x] T004 Extend `.golangci.yml`: add the new `internal/domain/**` and `internal/service/**` paths to the `depguard` rule `domain-and-services-stay-pure`, and add the three new `internal/store/*` packages to the `forbidigo` exclusion list alongside the existing adapter exclusions
- [x] T005 Add `FilesConfig{PhotoMaxBytes int64, PhotoMimeTypes []string, PhotoThumbs []string}` under `envPrefix:"FILES_"` to `internal/config/config.go` (defaults 15 MiB, `image/jpeg,image/png,image/webp`, `100x100t,400x400f`) with a defaults-and-validation table test in `internal/config/config_test.go`
- [x] T006 [P] Add `fixture:rebuild` and `bench:chart` targets to `Taskfile.yaml`, following the existing `gen`/`test` conventions
- [x] T007 [P] Add `internal/store/migrations/doc.go` recording this phase's six migration filenames, their required order and why the order is forced by the relation graph (research D-15)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the domain types, the schema, the authorization anchor and the test scaffolding that
every user story below depends on.

**⚠️ CRITICAL**: no user story work begins until this phase is complete.

### Domain — tests first

- [x] T008 [P] Table test for `person.Sex`, `person.BloodType`, `person.RelationshipToOwner` `Valid()` covering every listed value plus three rejects, in `internal/domain/person/enums_test.go`
- [x] T009 [P] Table test for `person.AgeAt` covering: born today ("0 days", never "0"), born yesterday, the day before a birthday, on a birthday, a 29 February birth date evaluated in a non-leap year, and an unset birth date, in `internal/domain/person/age_test.go`
- [x] T010 [P] Table test for metric↔imperial height and weight formatting and for the invariant that conversion never mutates the SI value, in `internal/domain/person/measure_test.go`
- [x] T011 [P] Table test for `person.Patient.Validate()` asserting a payload with four simultaneous faults returns four `fields[]` entries in one `*domain.ValidationError` (FR-003, US1-3), plus each individual rule from data-model.md §3, in `internal/domain/person/validate_test.go`
- [x] T012 [P] Test that `person.Patient.MarshalZerologObject` emits only `id` and `owner_id` and that no name, birth date or address appears in the rendered event (FR-046), in `internal/domain/person/patient_test.go`
- [x] T013 [P] Table test for `directory.FacilityKind.Valid()` and `directory.Specialty.Valid()`, asserting the catch-all `other` is present and that the Go slice and the generated select vocabulary are identical, in `internal/domain/directory/enums_test.go`
- [x] T014 [P] Table test for `directory.Practitioner.Validate()` / `Facility.Validate()` and their redacting marshallers, in `internal/domain/directory/validate_test.go`

### Domain — implementation

- [x] T015 [P] Implement `person.Sex`, `person.BloodType`, `person.RelationshipToOwner` with `Valid()` and the generated select vocabularies in `internal/domain/person/enums.go`
- [x] T016 [P] Implement `person.AgeAt(birth, on) person.Age` with a `String()` that degrades to months and days below one year, in `internal/domain/person/age.go`
- [x] T017 [P] Implement `person.FormatHeight` / `person.FormatWeight` over canonical SI in `internal/domain/person/measure.go` (kept here rather than a shared `units` package until vitals in phase 003 makes a second consumer — research D-21)
- [x] T018 Implement `person.Patient` plus `Validate()` and `MarshalZerologObject` in `internal/domain/person/patient.go` and `internal/domain/person/validate.go` (depends on T015–T017)
- [x] T019 [P] Implement `directory.FacilityKind` — practice, pharmacy, hospital, laboratory, imaging centre, other (FR-034) — and the 42-value `directory.Specialty` in `internal/domain/directory/enums.go`
- [x] T020 Implement `directory.Practitioner` and `directory.Facility` with `Validate()` and redacting marshallers in `internal/domain/directory/practitioner.go` and `internal/domain/directory/facility.go` (depends on T019)

### Schema — tests first

- [x] T021 Integration test asserting each of the five migrations applies and reverts cleanly and that all five API rules are `nil` on `facilities`, `practitioners` and `patients`, in `internal/store/migrations/migrations_test.go`
- [x] T022 Integration test asserting the `Required`/`CascadeDelete` matrix of research D-06 field by field — including that `users.active_patient` is `CascadeDelete: false` and `medications.patient` is `CascadeDelete: true` — in `internal/store/migrations/assertions_test.go`
- [x] T023 Integration test asserting `patients.photo` is `Protected: true` — so no PocketBase file token and no link carrying its own credential can reach it (FR-044) — that `MaxSize` is the configured 15 MiB (FR-008) and `Thumbs` is `["100x100t","400x400f"]`, plus a case proving the boot assertion **refuses to start** when `Protected` is flipped to false, in `internal/store/migrations/assertions_test.go`

- [x] T023a [P] Extend `internal/store/migrations/audit_vocab_test.go` (phase 001, T070a) to assert the **complete** expected vocabulary after this phase — **twenty-one** actions and **twenty-five** target kinds, set-equal, not a delta — so a value this phase writes but no migration declared fails here rather than failing `SelectField` validation in production (ANALYSIS C1). **Fails until T028 lands**, which is the point.

### Schema — implementation

- [x] T024 [P] Implement migration `internal/store/migrations/1756200100_facilities.go` (collection, nil rules, all fields from data-model.md §1 — the full recorded set of FR-034 — three indexes, reversible `down`)
- [x] T025 Implement migration `internal/store/migrations/1756200200_practitioners.go` including the `facility` relation and the `(owner, LOWER(name), specialty)` unique index (depends on T024)
- [x] T026 Implement migration `internal/store/migrations/1756200300_patients.go` including `primary_practitioner`, the `Protected` photo field and the partial unique index `(owner) WHERE is_self_record = 1` (depends on T025)
- [x] T027 Implement migration `internal/store/migrations/1756200400_users_active_patient.go` adding `active_patient` with `CascadeDelete: false` (depends on T026)
- [x] T028 Implement migration `internal/store/migrations/1756200500_audit_events_patient.go` adding `patient`, extending the `action` vocabulary with `switch_patient` and `target_kind` with `practitioner`/`facility` (`patient` already exists — phase 001 declares the contract's complete vocabulary), and adding the `(patient, occurred_at DESC)` index (depends on T026; **turns T023a green**)
- [x] T029 Extend `internal/store/migrations/assertions.go` with the nil-rule check over the three new collections, the `Protected: true` check over `patients.photo`, and the new `Required`/`CascadeDelete` matrix assertion
- [x] T030 Register the five migrations and the extended assertions in `internal/platform/pb/boot.go` and `cmd/medikube/main.go`

### Shared ports, authorization and test scaffolding

- [x] T031 Test then implement `records.Registry.Kinds() []kind.Kind` and per-kind patient counting dispatch in `internal/records/registry.go` and `internal/records/registry_test.go` — the extension point the chart summary consumes so that nothing switches on kind (Principle II)
- [x] T032 [P] Test then implement the `Patient` field on `audit.Event` and `audit.RecentForPatient(ctx, patientID, limit)` in `internal/service/audit/writer.go`, `internal/service/audit/recent.go` and their `_test.go` files, asserting no content field can be written
- [x] T033 Unit test for `access.Authorizer.Patient` with `t.Parallel()`: owner allowed with `PermOwn`, stranger returns `domain.ErrNotFound` (never `ErrForbidden`), anonymous returns `ErrUnauthenticated`, and every refusal produces exactly one audit row — in `internal/service/access/authorizer_test.go`
- [x] T034 Implement patient-ownership resolution and audited refusal in `internal/service/access/authorizer.go` (depends on T033)
- [x] T035 [P] Implement `testsupport.RunOwnershipMatrix(t, []Case)` — the table-driven owner-succeeds / stranger-404-byte-identical helper that every story's authorization tests use and that phase 005 extends with share rows — in `internal/testsupport/authz.go`
- [x] T036 [P] Rebuild the committed fixture data dir `internal/testdata/pb_data` with the five migrations applied, and export the seeded ids as constants from `internal/testsupport/fixtures.go` (accounts A, B and C per data-model.md §9)
- [x] T037 Extend `internal/cli/seed.go` to produce the deterministic set of data-model.md §9, leaving Account B's directories **empty** so the smoke gate exercises the empty states (depends on T036)
- [x] T038 [P] Extend the ETag/If-Match helper in `internal/web/etag.go` to cover patients, practitioners and facilities, with a test asserting a missing `If-Match` is `422` with field `If-Match` and a mismatch is `412` carrying the current representation so the account holder is told what happened and shown the current values (FR-011), in `internal/web/etag_test.go`
- [x] T039 [P] Register the three new cursor sort-key sets of research D-29 in `internal/web/cursor.go`, with a test asserting the id tiebreaker is always present, in `internal/web/cursor_test.go`
- [x] T040 [P] Map PocketBase file-validation errors into MediKube's PHI-free envelope in `internal/web/errors.go`, with a test asserting the uploaded filename never appears in the response or the log stream (research D-17), in `internal/web/errors_test.go`
- [x] T041 [P] Extend the route registry test in `internal/httproute/registry_test.go` to assert that every registered `page` route carries a landmark and a `smokeUrl`, that no registered path ends in `/`, and that every operation has a unique `operationId`

**Checkpoint**: the schema exists, the domain types validate, ownership is decidable, and the test
harness can express "owner succeeds, stranger is refused". User stories may now begin.

---

## Phase 3: User Story 1 — Keep a profile for each person I care for (Priority: P1) 🎯 MVP

**Goal**: an account can hold many people, each with identifying and baseline health details and a
photograph; its own profile already exists; and no other account can see or discover any of them.

**Independent Test**: sign in as a new account, confirm the account holder's own profile already
exists and is marked as theirs, add two more people with full and partial details, correct one, and
confirm from a second account that none of the three is visible or discoverable.

**Covers**: FR-001 … FR-012, FR-041 … FR-046. Acceptance scenarios US1-1 … US1-8.

### Tests for User Story 1 ⚠️ write first, watch them fail

- [x] T042 [P] [US1] Hand-written fakes for `Repository`, `PhotoStore`, `Authorizer` and `Auditor` in `internal/service/patient/patienttest/fake.go`
- [x] T043 [P] [US1] `patienttest.RepositoryContract(t, factory)` — the shared Liskov suite every implementation including the fake must pass — in `internal/service/patient/patienttest/contract.go`
- [x] T044 [P] [US1] `patienttest.PhotoStoreContract(t, factory)` covering store, replace, remove and thumbnail presence, in `internal/service/patient/patienttest/contract_photo.go`
- [x] T045 [P] [US1] Unit tests for `patient.Service` Create/Get/List/Update against the fakes with `t.Parallel()`, covering US1-2 (saved and owned), US1-3 (all invalid fields at once), US1-4 (second self-record refused 409, FR-004), US1-6 (absent details stay absent) and US1-7 (stale save refused, FR-011), in `internal/service/patient/service_test.go`
- [x] T046 [P] [US1] Unit tests for the photo service: one photograph per person, a non-image is refused and nothing is stored, an oversize file is refused, a replacement removes the previous file and a removal leaves none (FR-008) — in `internal/service/patient/photo_test.go`
- [x] T047 [P] [US1] Integration test running `patienttest.RepositoryContract` against the PocketBase adapter with `tests.NewTestApp`, in `internal/store/patient/repo_integration_test.go`
- [x] T048 [P] [US1] Integration test running `patienttest.PhotoStoreContract` and additionally asserting both thumbnails exist on the filesystem under `<collectionId>/<recordId>/thumbs_<filename>/<size>_<filename>` **before** any request for them (FR-009, eager), and that a replacement removes the old original and both old thumbnails (US1-5), in `internal/store/patient/photo_integration_test.go`
- [x] T049 [P] [US1] Integration test asserting the partial unique index refuses a second `is_self_record` row for one owner even under a direct write (FR-004), in `internal/store/patient/selfrecord_integration_test.go`
- [x] T050 [P] [US1] `tests.ApiScenario` suite for `listPatients`, `createPatient`, `getPatient`, `updatePatient` covering 200/201/401/404/409/412/422, the predictable order and the unconditional `total` and `owned_count` (FR-010), and `422 unknown_field` when a body carries `owner` or `is_self_record` — the owning account is fixed at creation and no edit can move it (FR-002) — in `internal/web/api/patients_test.go`
- [x] T051 [P] [US1] `tests.ApiScenario` suite for `putPatientPhoto`, `getPatientPhoto`, `deletePatientPhoto`: the type is decided from the content, not the name or the stated type (FR-008) — a PDF renamed `photo.jpg` is `415` and nothing lands on disk; a `.png` renamed `.jpg` is accepted; the `415` body and the captured log stream contain no substring of the uploaded filename; the download carries `Cache-Control: private, no-store` and a generic `Content-Disposition` filename — in `internal/web/api/patient_photo_test.go`
- [x] T052 [P] [US1] Authorization test using `testsupport.RunOwnershipMatrix` over all eight patient and photo operations, asserting the anonymous caller is refused with nothing about the person in the refusal (FR-043), that the photograph is reachable only through an authorized request (FR-044), that the stranger response is byte-identical to a genuine not-found apart from `request_id` (FR-042, US1-8, SC-005) and that each refusal writes an audit row — in `internal/web/api/patients_authz_test.go`
- [x] T053 [P] [US1] `tests.ApiScenario` asserting registration creates exactly one `is_self_record` patient for the new account, marked as theirs, with `relationship_to_owner = self` (FR-005, US1-1) — in `internal/web/api/register_selfrecord_test.go`
- [x] T054 [P] [US1] `tests.ApiScenario` with `ExpectedEvents` asserting a patient write fires the audit hooks and fires **zero** record-CRUD request events, proving MediKube's route did not go through PocketBase's auto-API — in `internal/web/api/patients_test.go`
- [x] T055 [P] [US1] templ render tests for `PatientList`, `PatientRow`, `PatientDetail`, `PatientForm` and `PatientPhoto`, asserting the `region[name="Patients"]` landmark is present in both the populated and the empty case, that the list marks which person is the account holder and states how many there are (FR-010), that absent details render as absent rather than as `0`/blank (US1-6), and that name and date of birth appear together wherever people are listed (Edge case: two people with the same name) — in `internal/web/views/patients/list_test.go` and `internal/web/views/patients/detail_test.go`
- [x] T056 [P] [US1] JSON round-trip tests for `PatientSummary`, `Patient`, `PatientCreate`, `PatientPatch` under Go 1.27 `encoding/json/v2`: slices marshal as `[]` not `null`, unknown fields are rejected, duplicate keys are rejected, dates are `YYYY-MM-DD` — in `internal/web/api/dto_patient_test.go`

### Implementation for User Story 1

- [x] T057 [US1] Declare the consumer-side ports `Repository`, `PhotoStore`, `Authorizer`, `Auditor` (each 1–4 methods, no omnibus) in `internal/service/patient/ports.go`
- [x] T058 [US1] Implement `patient.Service` List/Get/Create/Update with `s.authz.Patient(...)` as the first act of every method and `owner` set from the actor on create and never from a request thereafter (FR-002), in `internal/service/patient/service.go` (depends on T057)
- [x] T059 [US1] Implement the self-record invariant and the 409 path (FR-004) in `internal/service/patient/selfrecord.go`
- [x] T060 [US1] Implement `SetPhoto`/`GetPhoto`/`DeletePhoto` policy in `internal/service/patient/photo.go`
- [x] T061 [US1] Implement the PocketBase repository — `*core.Record` ↔ `person.Patient` mapping, typed queries, no filter DSL above this package — in `internal/store/patient/repo.go`
- [x] T062 [US1] Implement the photo store: `filesystem.NewFileFromMultipart`, eager `fsys.CreateThumb` for both sizes inside a `TxInfo().OnComplete` callback using PocketBase's `thumbs_<filename>/` key layout, and `fsys.Serve` for download — in `internal/store/patient/photo.go`
- [x] T063 [US1] Implement `PatientSummary`, `Patient`, `Display`, `PatientCreate`, `PatientPatch` in `internal/web/api/dto_patient.go`
- [x] T064 [US1] Implement the four CRUD handlers with ETag/If-Match in `internal/web/api/patients.go`
- [x] T065 [US1] Implement the three photo handlers in `internal/web/api/patient_photo.go`
- [x] T066 [US1] Register `listPatients`, `createPatient`, `getPatient`, `updatePatient`, `putPatientPhoto`, `getPatientPhoto`, `deletePatientPhoto` and the `/patients`, `/patients/{id}` page routes with the landmarks and smoke URLs of `contracts/pages.md` §2 in `internal/httproute/routes.go`
- [x] T067 [US1] Bind the `OnRecordAfterCreateSuccess` / `…UpdateSuccess` / `…DeleteSuccess` audit hooks for `patients`, tolerating a nil request context, in `internal/platform/pb/hooks.go`
- [x] T068 [US1] Provision the self-record inside the registration transaction in `internal/web/api/auth.go`, using the display-name split rule of research D-10
- [x] T069 [US1] Implement the templ views `list.templ`, `detail.templ`, `form.templ`, `photo.templ` in `internal/web/views/patients/`, each rooted at a deterministic id from `internal/web/views/ids`
- [x] T070 [US1] Implement the `/patients` and `/patients/{id}` page handlers in `internal/web/page/patients.go`
- [x] T071 [US1] Wire the patient service, repository and photo store into the container in `internal/di/providers.go`

**Checkpoint**: profiles exist, belong to exactly one account, carry photographs, and are invisible
to everyone else. US1 is demonstrable on its own.

---

## Phase 4: User Story 2 — Every clinical record belongs to one person (Priority: P2)

**Goal**: medications recorded before this phase sit under their recording account holder's own
profile; new records are filed against a named person; lists never mix.

**Independent Test**: with medications recorded before the change, confirm every one now appears
under the recording account holder's own profile and under no other; record a medication against a
second person and confirm the two lists never mix and that a medication cannot be created without
naming its person.

**Covers**: FR-016, FR-019, FR-021 … FR-026. Acceptance scenarios US2-1 … US2-6.

**Depends on**: Phase 3 (patients must exist to attribute to).

### Tests for User Story 2 ⚠️ write first

- [x] T072 [P] [US2] Integration test of the re-attribution against a database seeded in the phase-001 shape: 0 medications unattributed, 0 medications on a non-self-record patient, per-account medication counts unchanged before and after (FR-022, SC-006) — in `internal/store/migrations/repoint_test.go`
- [x] T073 [P] [US2] Integration test asserting a failed post-condition rolls the **entire** migration batch back, leaving the database exactly as it was (research D-13) — in `internal/store/migrations/repoint_test.go`
- [x] T074 [P] [US2] Integration test asserting the backfill produces one `create`/`patient` audit row with `actor_kind = system` per provisioned self-record; that the audit hook does not panic without a request context (research D-14); and that **each of those rows carries a non-empty `request_id` taken from the migration run's `run_id`** — the same value on every row of one backfill, and the same value the migration's own log lines carry, so a `system` row written with no HTTP request still correlates (001 [data-model](../001-walking-skeleton/data-model.md) §3, 001 T240; `audit_events.request_id` is `Required`, so an empty one fails validation) — in `internal/store/migrations/repoint_audit_test.go`
- [x] T075 [P] [US2] Unit tests for `medication.Service`: Create refuses an empty patient with `422` field `patient` (US2-3); List is scoped to the requested patient — in `internal/service/medication/service_test.go`
- [x] T076 [P] [US2] `tests.ApiScenario` suite: patient X with 3 medications and patient Y with 2 in the same account return exactly 3 and 2 with no bleed — a list of a person's records contains only records attributed to that person (FR-023, US2-2, SC-007); a list without `?patient=` is `400 patient_required` and mentions no patient (FR-016) — in `internal/web/api/medications_scope_test.go`
- [x] T077 [P] [US2] `tests.ApiScenario` asserting `PATCH` with a `patient` field is `422 unknown_field` **and** the stored record is unchanged (FR-024, US2-4) — in `internal/web/api/medications_scope_test.go`
- [x] T078 [P] [US2] Authorization test over the six record operations via `testsupport.RunOwnershipMatrix`, asserting another account's medication is `404` and the attempt is audited (US2-5) — in `internal/web/api/medications_authz_test.go`
- [x] T079 [P] [US2] templ render test asserting every medication list and detail view names the patient whose records are shown (FR-019, US2-6, SC-003) — in `internal/web/views/records/medication_test.go`
- [x] T080 [P] [US2] Integration test asserting two sessions on two accounts each streaming their own patient receive only their own frames (phase 001's live-update promise, now patient-scoped) — in `internal/web/stream/records_scope_test.go`

### Implementation for User Story 2

- [x] T081 [US2] Implement migration `internal/store/migrations/1756200600_medications_repoint.go` with the six ordered steps of research D-13, the raw `dbx` UPDATE backfill (Complexity Tracking CT-1), the zero-unattributed post-condition, and a `down` carrying the written irreversibility note Principle IX requires
- [x] T082 [US2] Add `PatientID` to `medication.Query` and make `patient` a required, immutable input to `Create` in `internal/service/medication/ports.go` and `internal/service/medication/service.go`
- [x] T083 [US2] Re-anchor the repository on `patient` so every list query is filtered by the requested person in SQL rather than after the fact (FR-023), add the `practitioner` and `pharmacy` relation mapping, and switch to the `(patient, started_on DESC, id)` index in `internal/store/medication/repo.go`
- [x] T084 [US2] Update the medication DTOs: `patient` required on create, absent from the patch, `practitioner`/`pharmacy` added — in `internal/web/api/dto_medication.go`
- [x] T085 [US2] Enforce `?patient=` on every list handler with `400 patient_required` and no fallback, in `internal/web/api/medications.go`
- [x] T086 [US2] Implement the `@PatientContextHeader` component naming the patient inside `#main`, in `internal/web/views/shell/patient_context.templ`
- [x] T087 [US2] Render the context header and a hidden, render-time-fixed `patient` field on the medication form so a later switch cannot re-file the record (FR-025, US3-6) — in `internal/web/page/medications.go` and `internal/web/views/records/medication_form.templ`
- [x] T088 [US2] Make `/api/v1/streams/records` require `?patient=` and re-run `access.Authorizer.Patient` for the subscriber on every event before re-fetching and patching, in `internal/web/stream/records.go`

**Checkpoint**: every clinical record is attributed to exactly one person, historical data included,
and no list mixes two people.

---

## Phase 5: User Story 3 — Switch who I am looking at (Priority: P3)

**Goal**: choosing a person in the switcher makes the application about them, the choice survives
sign-out and device, and it never grants access to anything.

**Independent Test**: select a person, navigate across several screens confirming each shows that
person, sign out and back in and confirm the selection survived, then delete the selected person and
confirm the application lands on the list of people rather than an error or somebody else's data.

**Covers**: FR-013 … FR-020. Acceptance scenarios US3-1 … US3-6.

**Depends on**: Phase 3. Interacts with Phase 4 (the redirect targets patient-scoped lists).

### Tests for User Story 3 ⚠️ write first

- [x] T089 [P] [US3] Unit tests for the active-patient service: the target is authorized before the pointer is written (FR-020), the pointer resolves to null when unreachable (FR-017), auto-selection when exactly one patient is reachable (FR-018, US3-4) — in `internal/service/patient/active_test.go`
- [x] T090 [P] [US3] `tests.ApiScenario` suite for `setActivePatient`: 200, 404 for another account's patient with the pointer left unchanged, 422 for a malformed body, 401 anonymous — in `internal/web/api/active_patient_test.go`
- [x] T091 [P] [US3] **The authorization-independence test**: set the pointer to an owned patient, then request another account's patient's records and assert `404`; changing the selection grants nothing (FR-015, US3-5) — in `internal/web/api/active_patient_test.go`
- [x] T092 [P] [US3] `tests.ApiScenario` asserting the pointer survives sign-out and a fresh sign-in on a different session (US3-2, SC-014) — in `internal/web/api/active_patient_test.go`
- [x] T093 [P] [US3] `tests.ApiScenario` for the amended `GET /api/v1/me` (active patient, `owned_count`) and for `PATCH /api/v1/me` rejecting `active_patient` as `422 unknown_field` — in `internal/web/api/me_test.go`
- [x] T094 [P] [US3] Integration test asserting deleting a patient leaves `users.active_patient` null on every account that pointed at it, and that the account itself still exists — proving `CascadeDelete: false` (research D-07) — in `internal/store/patient/active_unset_integration_test.go`
- [x] T095 [P] [US3] Page-layer tests: a null pointer with several reachable patients redirects to `/patients` with an explanation; a bare `/medications` `303`s to `/medications?patient={active}`; a pointer at a now-unreachable patient never renders another person's data (US3-3) — in `internal/web/page/redirect_test.go`
- [x] T096 [P] [US3] templ render test for `PatientSwitcher` (FR-014): `role="combobox"` named "Active patient" showing the person in view by name and photograph and offering a switch to any of them, each option carries name **and** date of birth so twins and same-named relatives are distinguishable, and the component renders outside `#main` so it can never be morphed away — in `internal/web/views/shell/patient_switcher_test.go`
- [x] T097 [P] [US3] Playwright stateful flow **discharging SC-002**: choose a person, navigate three screens asserting each names them, reload, assert the choice survived; at both viewports. **Both halves of SC-002 asserted from each of the three screens**: reaching any other reachable person takes **no more than two interactions** (open the switcher, choose), counted from the accessibility tree rather than from a fixed selector; and the newly chosen person's information is on screen within the **1 s** budget of [plan.md](./plan.md)'s Performance Goals, taken as the **median of five switches** on the seeded 25-patient account so the gate is a budget and not a coin flip — in `e2e/patient-switch.spec.ts`

### Implementation for User Story 3

- [x] T098 [US3] Implement `SetActivePatient` and `ResolveActivePatient` (authorize first, audit `switch_patient`, auto-select on exactly one) in `internal/service/patient/active.go`
- [x] T099 [US3] Implement `PUT /api/v1/me/active-patient` including the `text/html` negotiation that returns the re-rendered switcher as a plain element patch, in `internal/web/api/active_patient.go`
- [x] T100 [US3] Amend `GET`/`PATCH /api/v1/me` in `internal/web/api/me.go`
- [x] T101 [US3] Implement the `PatientSwitcher` templ component using only free Datastar attributes and the v1 colon syntax (`data-on:click`), in `internal/web/views/shell/patient_switcher.templ`
- [x] T102 [US3] Mount the switcher inside `#primary-nav` in `internal/web/views/shell/layout.templ`, outside every patch target, so it is present on every screen a signed-in account holder can reach (FR-014)
- [x] T103 [US3] Extend `NavState` and implement the page redirects of contracts/active-patient.md in `internal/web/page/shell.go`
- [x] T104 [US3] Register `setActivePatient` in `internal/httproute/routes.go`

**Checkpoint**: several people are usable rather than merely stored, and the selection is provably
not a permission.

---

## Phase 6: User Story 4 — See a person's chart at a glance (Priority: P4)

**Goal**: a demographic header, per-kind record counts and recent changes for one person, correct
and fast even on a very large chart, and helpful when empty.

**Independent Test**: open the chart of a person with records and confirm the header, counts and
recent-change list all match the underlying data; then open the chart of a person with no records
and confirm a helpful empty state rather than zeros, blanks or an error.

**Covers**: FR-006, FR-007, FR-027 … FR-031. Acceptance scenarios US4-1 … US4-6.

**Depends on**: Phase 3 (the person) and Phase 4 (records attributed to them).

### Tests for User Story 4 ⚠️ write first

- [x] T105 [P] [US4] Fake `RecordCounter` and `RecentActivityReader` in `internal/service/patient/patienttest/counter.go`
- [x] T106 [P] [US4] Unit tests for the chart service: counts include kinds with zero records (US4-2), the header shows absent details as absent (FR-027), age renders meaningfully for a person born today (US4-4), and the display block follows the actor's unit system while the SI values are untouched (US4-3) — in `internal/service/patient/chart_test.go`
- [x] T107 [P] [US4] Integration test asserting the count for a kind equals the rows attributed to that patient and excludes every other patient's (FR-028, US4-1, SC-007) — in `internal/store/patient/chart_integration_test.go`
- [x] T108 [P] [US4] `tests.ApiScenario` for `getPatientChart`: 200 shape, `recent_activity: []` and never `null`, `404` for another account's patient, `401` anonymous — in `internal/web/api/patient_chart_test.go`
- [x] T109 [P] [US4] Test asserting each recent-activity entry states kind, action and time and carries no name, value, note or filename, and that an entry whose target has been deleted reports `target_exists: false` and links nowhere (FR-029, US4-5) — in `internal/web/api/patient_chart_test.go`
- [x] T110 [P] [US4] Test asserting that changing `unit_system` changes only the `display` block and leaves `height_cm`/`weight_kg` byte-identical (FR-007, US4-3) — in `internal/web/api/patient_chart_units_test.go`
- [x] T111 [P] [US4] Benchmark seeding 50,000 medications on one patient and asserting the chart summary p95 is under 2 seconds (SC-004, US4-6) — in `internal/store/patient/chart_bench_test.go`
- [x] T112 [P] [US4] templ render tests: the `region[name="Patient chart"]` landmark is present in **both** the populated and the entirely empty case, and `@EmptyState` renders **inside** the landmark (FR-030, US4-2, SC-013) — in `internal/web/views/patients/detail_test.go`

### Implementation for User Story 4

- [x] T113 [US4] Declare the `RecordCounter` and `RecentActivityReader` ports (1 method each) in `internal/service/patient/ports.go`
- [x] T114 [US4] Implement the counter adapter over the kind registry — one indexed `COUNT(*)` per registered kind, nothing switching on kind — in `internal/records/counter.go`
- [x] T115 [US4] Implement `patient.Service.Summary` in `internal/service/patient/chart.go` (depends on T113, T114)
- [x] T116 [US4] Implement the `PatientChart` DTO and the `getPatientChart` handler with `Cache-Control: private, no-store` and no ETag, in `internal/web/api/patient_chart.go`
- [x] T117 [US4] Implement the chart view — header, per-kind tiles with their own empty states, recent-activity list — in `internal/web/views/patients/detail.templ`
- [x] T118 [US4] Register `getPatientChart` in `internal/httproute/routes.go`

**Checkpoint**: the chart is the landing place after switching, correct at 50,000 records and
helpful at zero.

---

## Phase 7: User Story 5 — Keep a directory of clinicians and places (Priority: P5)

**Goal**: an account-private directory of practitioners and places of care, reusable wherever one is
needed, searchable, and safe to delete from.

**Independent Test**: add a practitioner, a practice and a pharmacy; attach the practitioner to a
person as their primary practitioner and to a medication as its prescriber; delete the practitioner
and confirm the medication survives with its prescriber cleared and the account holder warned first.

**Covers**: FR-032 … FR-040. Acceptance scenarios US5-1 … US5-6.

**Depends on**: Phase 2 (the collections) and Phase 3 (`patients.primary_practitioner`). Phase 4 for
the medication prescriber link.

### Tests for User Story 5 ⚠️ write first

- [x] T119 [P] [US5] Fakes and `practitionertest.RepositoryContract` in `internal/service/practitioner/practitionertest/fake.go` and `contract.go`
- [x] T120 [P] [US5] Fakes and `facilitytest.RepositoryContract` in `internal/service/facility/facilitytest/fake.go` and `contract.go`
- [x] T121 [P] [US5] Unit tests for `practitioner.Service` including the `(owner, LOWER(name), specialty)` duplicate returning `409` with an explanation (FR-038, US5-4) and a specialty outside the vocabulary returning `422` (FR-033) — in `internal/service/practitioner/service_test.go`
- [x] T122 [P] [US5] Unit tests for `facility.Service` including that `kind` is required and that two identically named facilities are both accepted (FR-035, US5-3) — in `internal/service/facility/service_test.go`
- [x] T123 [P] [US5] Integration test running `practitionertest.RepositoryContract` against the adapter, in `internal/store/practitioner/repo_integration_test.go`
- [x] T124 [P] [US5] Integration test asserting two practitioners with the same name and **no specialty at all** collide — the SQLite NULL-distinctness trap that only fails closed because the select field stores `''` (research D-25) — in `internal/store/practitioner/unique_integration_test.go`
- [x] T125 [P] [US5] Integration test running `facilitytest.RepositoryContract` and asserting two same-named branches are both stored and both listed, in `internal/store/facility/repo_integration_test.go`
- [ ] T126 [P] [US5] Integration test asserting that deleting a practitioner referenced by `patients.primary_practitioner` and by `medications.practitioner` leaves both records alive with the reference cleared, and that the number of records modified equals the `usage.records` reported beforehand (FR-040, US5-5) — in `internal/store/practitioner/delete_unset_integration_test.go`
- [x] T127 [P] [US5] Same test for a facility referenced by `practitioners.facility` and `medications.pharmacy`, in `internal/store/facility/delete_unset_integration_test.go`
- [x] T128 [P] [US5] `tests.ApiScenario` suite for the five practitioner operations covering 200/201/401/404/409/412/422, `?q=` search by name with `?specialty=` and `?facility=` filtering (FR-036) and the `usage` block on detail, in `internal/web/api/practitioners_test.go`
- [x] T129 [P] [US5] `tests.ApiScenario` suite for the five facility operations including `?kind=` filtering and `?q=` search by name (FR-036), in `internal/web/api/facilities_test.go`
- [x] T130 [P] [US5] Authorization test over all ten directory operations via `testsupport.RunOwnershipMatrix`, plus an explicit assertion that `?q=` matching another account's entry returns `[]` (FR-037, US5-6, SC-014) — in `internal/web/api/directory_authz_test.go`
- [x] T131 [P] [US5] templ render tests for the six directory components asserting the `region[name="Practitioners"]` / `region[name="Facilities"]` and `article` landmarks in both populated and empty states, in `internal/web/views/directory/practitioner_list_test.go` and `internal/web/views/directory/facility_list_test.go`

### Implementation for User Story 5

- [x] T132 [P] [US5] Declare ports and implement `practitioner.Service` in `internal/service/practitioner/ports.go` and `service.go`
- [x] T133 [P] [US5] Declare ports and implement `facility.Service` in `internal/service/facility/ports.go` and `service.go`
- [x] T134 [P] [US5] Implement the PocketBase repository in `internal/store/practitioner/repo.go`, including the two `usage` counts as indexed `COUNT(*)`s
- [x] T135 [P] [US5] Implement the PocketBase repository in `internal/store/facility/repo.go`, including its two `usage` counts
- [x] T136 [US5] Implement the directory DTOs including `Usage`, `PractitionerRef` and `FacilityRef` in `internal/web/api/dto_directory.go`
- [x] T137 [US5] Implement the five practitioner handlers in `internal/web/api/practitioners.go`
- [x] T138 [US5] Implement the five facility handlers in `internal/web/api/facilities.go`
- [x] T139 [US5] Register the ten operations — `listPractitioners`, `createPractitioner`, `getPractitioner`, `updatePractitioner`, `deletePractitioner`, `listFacilities`, `createFacility`, `getFacility`, `updateFacility`, `deleteFacility` — and the four page routes with the landmarks and smoke URLs of `contracts/pages.md` §2 in `internal/httproute/routes.go`, under those `operationId`s so the Principle IX gate finds the same ten names in the registry and in `api/openapi.json`
- [x] T140 [US5] Implement the six templ views in `internal/web/views/directory/`
- [x] T141 [US5] Implement the `/practitioners`, `/practitioners/{id}`, `/facilities`, `/facilities/{id}` page handlers in `internal/web/page/practitioners.go` and `internal/web/page/facilities.go`
- [x] T142 [US5] Implement the type-ahead picker with an inline create drawer that does not lose the record being written (FR-039), in `internal/web/views/shared/directory_picker.templ`
- [x] T143 [US5] Bind the audit hooks for `practitioners` and `facilities` in `internal/platform/pb/hooks.go` and wire both services into `internal/di/providers.go`

**Checkpoint**: the directory removes repeated typing, stays private to its account, and is safe to
delete from.

---

## Phase 8: User Story 6 — Remove a person permanently and deliberately (Priority: P6)

**Goal**: a deliberate, confirmed, complete and recorded removal, with nothing left behind.

**Independent Test**: create a person with records and a photograph, delete them through the
confirmation flow, and confirm the person, their records and their photograph are all gone, that no
record remains pointing at a person who no longer exists, and that the deletion is in the activity
trail.

**Covers**: FR-026, FR-045, FR-048 … FR-052. Acceptance scenarios US6-1 … US6-6.

**Depends on**: Phases 3, 4 and 6 (the confirmation reads its numbers from the chart summary).

### Tests for User Story 6 ⚠️ write first

- [x] T144 [P] [US6] Unit tests for `patient.Service.Delete`: a self-record returns `409` with the "closing the account is what removes it" explanation (FR-051, US6-4), a missing `If-Match` is `422` and a mismatch is `412` — in `internal/service/patient/delete_test.go`
- [x] T145 [P] [US6] Integration test asserting deletion destroys every medication attributed to the patient and the photograph together with both thumbnails, and that `SELECT COUNT(*) FROM medications WHERE patient = '<deleted id>'` is 0 — permanent and complete, with no recovery path (FR-049, US6-2, US6-3, SC-010) — in `internal/store/patient/delete_integration_test.go`
- [x] T146 [P] [US6] Integration test walking every collection asserting that no row anywhere references the deleted patient id after the delete, and that nothing in the application offers to bring it back (FR-049, US6-3) — in `internal/store/patient/delete_integration_test.go`
- [x] T147 [P] [US6] `tests.ApiScenario` for `deletePatient` covering 204/409/412/404/401 and asserting exactly one `delete`/`patient` audit row exists carrying no name and no record content (US6-5, SC-009) — in `internal/web/api/patients_delete_test.go`
- [x] T148 [P] [US6] Authorization test asserting Account B deleting Account A's patient is `404`, that nothing was deleted, and that the attempt was audited — only the owning account may delete (FR-050, US6-6, SC-005) — in `internal/web/api/patients_delete_test.go`
- [x] T149 [P] [US6] templ render test asserting the confirmation names the person and states how many records will be destroyed before anything is removed (FR-048, US6-1) — in `internal/web/views/patients/delete_confirm_test.go`

### Implementation for User Story 6

- [x] T150 [US6] Implement `Delete` with the self-record refusal and the `If-Match` requirement in `internal/service/patient/service.go`
- [x] T151 [US6] Implement the transactional delete over `app.RunInTransaction` in `internal/store/patient/repo.go`, relying on `medications.patient`'s cascade rather than deleting records by hand
- [x] T152 [US6] Implement the `deletePatient` handler in `internal/web/api/patients.go` and register it in `internal/httproute/routes.go`
- [x] T153 [US6] Implement the confirmation dialog, reading its record count from the chart summary the page already loaded, in `internal/web/views/patients/delete_confirm.templ`
- [x] T154 [US6] Implement the post-delete redirect to `/patients` with an explanation, and the same landing when a stale window acts on a now-deleted person (FR-017, US3-3), in `internal/web/page/patients.go`

**Checkpoint**: all six stories are independently functional.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: the gates that make the phase's claims machine-checked rather than asserted.

- [x] T155 Regenerate and commit `api/openapi.json` via `task openapi`, and review the diff operation by operation (Principle IX)
- [x] T156 Implement the registry↔OpenAPI gate test asserting every registered `operationId` appears in the committed document and vice versa, failing the build on any mismatch, in `internal/openapi/gate_test.go`
- [x] T157 Implement the route-inventory gate test asserting every `page` route has a landmark and a resolvable `smokeUrl` and that the Playwright target list derived from `medikube routes` covers all six new pages **and asserts `combobox[name="Active patient"]` on every authenticated page the inventory knows about, with no phase ceiling** — the assertion is written against the inventory, not against a hard-coded phase list, so every page a later phase registers inherits it the day it is registered (ANALYSIS, SHARED-DESIGN §3.0), in `internal/httproute/registry_test.go` (`contracts/pages.md` §4)
- [x] T158 [P] Demonstrate the smoke gate goes **red**: temporarily break one new page (remove its landmark, then separately throw in a script) and record both failures, then revert — closing open risk R11 for this phase's pages, documented in `e2e/README.md`
- [x] T159 [P] Extend phase 001's PHI-leak harness — `internal/testsupport/phileak/exercise.go` **[EDIT]**, run by `task test:phileak` — with this phase's sentinels (name, date of birth, address, file name), exercise **every endpoint this phase adds** against the seeded fixture, and assert zero occurrences in the zerolog stream, the Prometheus registry, the OTel span recorder and the Sentry transport (FR-046, SC-008). There is one harness in the suite and phase 001 owns it (cross-artifact finding M6)
- [x] T159a [P] Create `internal/testsupport/netgate/{netgate.go,dial_test.go}` (`//go:build netgate`) and the `task test:netgate` wrapper in `Taskfile.yaml` — a `net.Dialer` control hook that fails the test on **any** outbound connection — and run this phase's whole endpoint exercise under it on an instance with no destination configured, proving nothing about a person leaves the installation unless the operator asked for it (FR-047). **This phase owns the harness**; phases 003, 005 and 006 extend it rather than declaring a second one (cross-artifact finding M6's rule, applied to egress)
- [x] T160 [P] Add the phase's metrics (`medikube_records_total{kind}`, `medikube_files_photo_bytes`, `medikube_files_thumb_duration_seconds{size}`, `medikube_patients_switch_total{outcome}`) and the `service.patient.*` / `store.patients.*` spans with allowlisted attributes only, in `internal/obs/metrics.go` and the three service packages
- [x] T161 [P] Re-run the >5-minute SSE liveness helper against the now patient-scoped record stream, proving the `WriteTimeout` override still holds (open risk R7), in `internal/web/stream/timeout_test.go`
- [x] T162 [P] Pagination stability test: insert and delete rows while paging each of the three new lists and assert no entry is repeated or skipped (FR-053, Edge case "Paging while data changes"), in `internal/web/api/pagination_test.go`
- [x] T163 [P] Keyboard and viewport check: the switcher, the create drawers and the delete confirmation are fully operable by keyboard with a visible focus indicator at 1440×900 and 390×844, in `e2e/a11y.spec.ts`
- [x] T164 [P] Scale check: 25 patients in one account keeps the switcher usable and a named person findable within ten seconds (SC-011), in `e2e/patient-switch.spec.ts`
- [x] T165 Run `task check` and fix every `golangci-lint` finding, confirming `depguard` rejects a deliberate PocketBase import in `internal/service/patient` and `forbidigo` rejects a deliberate `app.Logger()` call, then revert both probes
- [x] T166 Add the migration-from-phase-001 job and the chart benchmark to CI in `.github/workflows/`, ensuring `GOTOOLCHAIN` is not pinned to `local`
- [x] T167 [P] Add this phase's three PocketBase-behaviour dependencies — `deleteRefRecords` unset semantics, the `thumbs_<filename>/` key layout, the single-transaction migration runner — to the PocketBase upgrade checklist in `docs/pocketbase-upgrade-checklist.md` (open risk R8)
- [x] T168 [P] Update `CLAUDE.md` and the project README with the rules this phase establishes: patient scope is explicit, the person in view is never authorization, records are hard deleted, files are the only soft-deletable thing
- [ ] T169 Write `specs/002-patient-core/traceability.md` mapping each of the 38 acceptance scenarios to its named test, **and each of the 56 functional requirements to the task ids that satisfy it, and each success criterion to its task or to a Phase Exit Criterion**, generated from `spec.md` and `tasks.md` rather than written by hand; fail the phase if any row is empty or if a success criterion is neither mapped nor marked `[outcome metric]` in `spec.md` (FR-054, SC-012, cross-artifact finding M7)
- [ ] T170 Run [quickstart.md](./quickstart.md) end to end on a clean checkout and fix whatever it finds

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)** — no dependencies.
- **Foundational (Phase 2)** — depends on Setup. **Blocks every user story.**
- **US1 (Phase 3)** — depends on Foundational only. This is the MVP.
- **US2 (Phase 4)** — depends on US1 (there must be a person to attribute to).
- **US3 (Phase 5)** — depends on US1; its redirect targets are more useful after US2 but do not
  require it.
- **US4 (Phase 6)** — depends on US1 and US2 (counts need attributed records).
- **US5 (Phase 7)** — depends on Foundational and US1 (`patients.primary_practitioner`); the
  medication-prescriber half additionally needs US2.
- **US6 (Phase 8)** — depends on US1, US2 and US4 (the confirmation's record count comes from the
  chart summary).
- **Polish (Phase 9)** — depends on all six stories.

```
Setup ─> Foundational ─┬─> US1 ─┬─> US2 ─┬─> US4 ─> US6 ─> Polish
                       │        │        │           ^
                       │        └─> US3 ─┘           │
                       └─────────────> US5 ──────────┘
```

### Within each user story

Tests are written and failing before implementation. Then: domain → ports → service → repository →
DTO → handler → route registration → views → page handler → DI wiring.

### Parallel opportunities

- **Phase 1**: T001, T002, T003, T006, T007 all run together.
- **Phase 2 domain**: T008–T014 (seven test files) all run together; then T015, T016, T017, T019
  run together, with T018 and T020 after their respective enum tasks.
- **Phase 2 migrations are strictly sequential** (T024 → T025 → T026 → T027/T028) because each
  relation needs its target collection to exist.
- **Every story's test block is fully parallel** — they are separate files with no shared state,
  and `tests.NewTestApp` clones the fixture per test so `t.Parallel()` is safe. **Never share a
  `TestApp` across `ApiScenario` cases**: `bindUIExtensions` re-enters on every `OnServe` and the
  handler chain grows until the stack overflows.
- **US5 is independent of US2 and US4** apart from the prescriber link, so with two engineers one
  can take US2→US4→US6 while the other takes US3 and US5.
- **Phase 9**: T158–T164 and T167–T168 all run together.

### Parallel example: User Story 1's test block

```bash
# All eight run at once — different files, no shared state:
Task: "Fakes in internal/service/patient/patienttest/fake.go"                       # T042
Task: "RepositoryContract in internal/service/patient/patienttest/contract.go"      # T043
Task: "PhotoStoreContract in internal/service/patient/patienttest/contract_photo.go"# T044
Task: "Service unit tests in internal/service/patient/service_test.go"              # T045
Task: "Photo unit tests in internal/service/patient/photo_test.go"                  # T046
Task: "HTTP suite in internal/web/api/patients_test.go"                             # T050
Task: "Photo HTTP suite in internal/web/api/patient_photo_test.go"                  # T051
Task: "Authorization matrix in internal/web/api/patients_authz_test.go"             # T052
```

---

## Implementation Strategy

### MVP first (User Story 1 only)

1. Phase 1 Setup.
2. Phase 2 Foundational — **critical, blocks everything**.
3. Phase 3 US1.
4. **Stop and validate**: a household's profiles exist, are private, and carry photographs. That is
   already the thing a carer reaches for in an emergency room, and it is shippable.

### Incremental delivery

Each of US2 … US6 is a separate demo:

| Increment | What a stakeholder sees |
|---|---|
| + US2 | every medication filed under a named person, history included, no list ever mixing two people |
| + US3 | switching between people from any screen, the choice following you to another device |
| + US4 | the chart you open before a visit |
| + US5 | picking a clinician instead of retyping their phone number |
| + US6 | removing a person deliberately, completely and accountably |

### Parallel team strategy

Two engineers after Phase 2: **A** takes US1 → US2 → US4 → US6 (the spine); **B** takes US3 then
US5 (both of which touch the shell and the directory, and neither of which blocks the spine).
Both converge on Phase 9.

---

## Notes

- Every task names its exact file. If a task would touch a file another parallel task owns, it does
  not carry `[P]`.
- Verify each test is **red** before writing the code that makes it green. A test that passes the
  moment it is written is testing nothing.
- Commit after each task or logical group, Conventional Commits, scope `medikube`.
- Two rules from the constitution that this phase will tempt you to break, in order of how much it
  will cost:
  1. **Never authorize from `users.active_patient`.** If a handler reads it, the handler is wrong.
  2. **Never let a patient-scoped refusal be anything but a 404 identical to a not-found.** A `403`
     tells the caller the subject exists, and existence is itself PHI.
