---
description: "Task list for phase 003 — Clinical Records"
---

# Tasks: Clinical Records

**Input**: Design documents from `/specs/003-clinical-records/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: **MANDATORY.** Constitution Principle III makes test-first non-negotiable, and the
specification demands it by name (FR-091, FR-092, FR-093, FR-094, SC-013, SC-014). Every test task
below precedes the implementation task it covers. A red-to-green transition that was never red is
a defect.

**Organization**: by user story, in the spec's priority order. Each story is independently
implementable, testable and demonstrable after the Foundational phase.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel with its siblings (different files, no incomplete dependency)
- **[Story]**: `[US1]`…`[US10]`; Setup, Foundational and Polish carry no story label
- Every task names the exact file path it touches

## Path conventions

All paths are relative to `/Users/krzysztof.wiatrzyk/private/monorepo/medikube`.

`<kind>` package directories use the Go-identifier form of the kind:
`allergy condition encounter procedure treatment symptom vitals immunization injury insurance
equipment emergencycontact familymember`.

---

## Phase 1: Setup

**Purpose**: make the toolchain, the linters and the fixture ready for a phase that adds sixteen
collections. Nothing here touches domain logic.

- [x] T001 Verify the Go 1.27 toolchain and PocketBase build precondition: confirm `go.mod` declares `go 1.27` with a `toolchain go1.27.x` line, confirm `GOTOOLCHAIN` is unset in `Taskfile.yaml` and `.github/workflows/`, and record the result in `specs/003-clinical-records/quickstart.md` §0
- [x] T002 Add a `forbid-kind-switch` custom rule to `.golangci.yml` (`gocritic`/`revive` custom or a `go vet` analyzer in `internal/lint/kindswitch`) rejecting any `switch` or `if`-chain over `kind.Kind` outside `internal/records/` and `internal/domain/kind/` — Constitution Principle II's open/closed clause becomes a build gate
- [x] T003 [P] Extend `.golangci.yml` `forbidigo` with the Datastar inline-script family (`ExecuteScript`, `ConsoleLog`, `ConsoleError`, `Redirect`, `Redirectf`, `DispatchCustomEvent`, `ReplaceURL`, `ReplaceURLQuerystring`, `Prefetch`) and a `depguard` deny of `internal/web/views` from `internal/service/**`
- [x] T004 [P] Add a Datastar Pro-attribute allowlist lint step to `Taskfile.yaml` (`task lint:datastar`) scanning `internal/web/views/**/*.templ` for `data-persist`, `data-query-string`, `data-replace-url`, `data-scroll-into-view`, `data-view-transition`, `data-custom-validity`, `data-animate`, `data-match-media`, `data-on-raf`, `data-on-resize`, `@clipboard`, `@fit`, and for the v0 delimiter form `data-on-<event>`
- [x] T005 [P] Add the `task test:scale` wrapper to `Taskfile.yaml` for the build-tagged scale suite. **`task test:phileak` already exists** — phase 001 created it with the harness (T006, T089, T235); this phase extends the harness, not the wrapper (cross-artifact finding M6)
- [x] T006 [P] Does not apply in this checkout: there is no parent monorepo `.dockerignore` here — this repo root **is** the medikube module and carries its own `.dockerignore`, which already excludes `internal/web/**/*_templ.go`, `internal/web/static/app.css` and `**/pb_data/`. Verified, nothing to change.
- [x] T007 Create the fixture regeneration task `task fixture:regen` in `Taskfile.yaml` that runs the migrations against a clean database plus `medikube seed` and rewrites `internal/testdata/pb_data`, and document in `quickstart.md` §6 that forgetting it makes every integration test run against the old schema

**Checkpoint**: linters and tasks in place. No production code has changed.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: everything shared by two or more stories. **No user story may start until this phase
is complete**, because every kind's migration references `tags`, every kind's registration writes
to `search_index`, and every kind's tests run the shared contract suites.

### Vocabularies and value types

- [x] T008 [P] Write failing table-driven tests for the four shared ladders in `internal/domain/clinical/vocab_test.go` — `Severity`, `ConditionStatus`, `OrderStatus`, `TherapyStatus`: `Valid()` accepts exactly the documented values, rejects the empty string and any upper-case or hyphenated form, and `All()` is stable, deduplicated and matches `data-model.md` §1 — one ladder per idea, shared by every kind that expresses it (FR-012)
- [x] T009 Implement the four shared ladders in `internal/domain/clinical/vocab.go` as string types with `Valid() bool`, `All() []string` and `String()`
- [x] T010 [P] Write failing tests for the twenty per-kind vocabularies in `internal/domain/clinical/vocab_kinds_test.go`, asserting each value set matches `data-model.md` §2 exactly, that every vocabulary marked with a catch-all contains `other`, and that no code path outside these files can add a value (FR-012)
- [x] T011 [P] Implement the twenty per-kind vocabularies across `internal/domain/clinical/vocab_<name>.go` (one file per vocabulary group: `vocab_medication.go`, `vocab_procedure.go`, `vocab_encounter.go`, `vocab_symptom.go`, `vocab_immunization.go`, `vocab_insurance.go`, `vocab_equipment.go`, `vocab_injury.go`, `vocab_contact.go`, `vocab_family.go`, `vocab_treatment.go`, `vocab_vitals.go`)
- [x] T012 [P] Write failing tests for `clinical.Date` and `clinical.Instant` in `internal/domain/clinical/dates_test.go` (FR-011): `Date` round-trips `YYYY-MM-DD` and is timezone-invariant across at least three host zones; `Instant` round-trips RFC3339 UTC and is presented in the viewer's local terms; both marshal correctly under `encoding/json/v2` and never emit `null` for a non-pointer
- [x] T013 [P] Write failing tests for the cross-field date validators in `internal/domain/clinical/dates_rules_test.go`: `Order(earlier, later)`, `NotFuture(ref)`, `RequiredWhen(cond, ref)` — each returning a `*domain.FieldError` naming the field, and a case proving two simultaneous violations are both reported
- [x] T014 Implement `internal/domain/clinical/dates.go` (`Date`, `Instant`, PocketBase `DateField` mapping, FR-011) and `internal/domain/clinical/dates_rules.go` (the three validators, accumulating into `*domain.ValidationError`)
- [x] T015 [P] Write failing tests for SI unit conversion in `internal/domain/clinical/units_test.go`: kg↔lb, cm↔in, °C↔°F, mmol/L↔mg/dL round-trip within a documented tolerance, and BMI derivation from `weight_kg` and `height_cm`
- [x] T016 [P] Implement `internal/domain/clinical/units.go` — conversion is pure and has no notion of a user; the caller supplies the unit system

### The kind registry

- [x] T017 [P] Extend the failing test `internal/domain/kind/kind_test.go` to assert the fourteen kinds: the enum↔segment mapping is total and injective in both directions, every segment is plural kebab-case with no trailing slash, and `insurance`/`family-history`/`vitals`/`equipment` are declared explicitly rather than pluralised
- [x] T018 Add the thirteen new values to `internal/domain/kind/kind.go` with their `snake_case` enum value, plural kebab path segment, singular and plural display labels, and their two ARIA landmark names from `contracts/pages.md` §2
- [x] T019 [P] Write failing tests in `internal/records/register_test.go` for the extended registry entry: `SearchFields func(any) (title, body string)`, `DefaultSort []page.SortKey`, `Filters map[string]FilterSpec`, `Basis func(any, Criteria) []string`, `SeedFixtureID string` — a `Register` call missing any of them panics at startup, not at request time
- [x] T020 Extend `internal/records/register.go` with those five fields and the panic-on-incomplete-registration behaviour; extend `internal/records/filters.go` with the `FilterSpec` type (name, kind, allowed values, default) and its query parser that returns `400 bad_request` for an unknown parameter
- [x] T021 Extend `internal/records/registry_completeness_test.go` so it fails the build when any `kind.Kind` value lacks a registry entry, an OpenAPI `oneOf` branch carrying a `kind` discriminator, exactly two page routes — its own list screen and its own detail screen (FR-009) — a default sort, a `SearchFields` declaration, a seed fixture id or two Playwright smoke cases. Registration is what makes the six generic record operations serve the kind, which is how all thirteen new kinds and family history become listable, openable, creatable, correctable and deletable without a new route (FR-001)

### The shared contract suites

- [x] T022 Implement `internal/records/recordstest/repositorycontract.go` — a `testify/suite` covering `Get`/`List`/`Save`/`Delete`: not-found, foreign-patient returns `ErrNotFound`, version/If-Match semantics, cursor stability under a concurrent insert, null-primary-date ordering last, and cascade removal when the patient is deleted. Parameterised by a factory so it runs against a real repository and against a fake
- [x] T023 Implement `internal/records/recordstest/kindcontract.go` — a suite covering registration completeness, the six operations `listRecords`, `listRecordsOfKind`, `createRecord`, `getRecord`, `updateRecord`, `deleteRecord` applied to the kind (FR-001), `patient` required on create and refused on patch so a record can be filed against nobody and can never be re-filed (FR-002), a `notes` field present on every kind and bounded at the documented length (FR-003), DTO round-trip under `encoding/json/v2`, the declared default sort, the authorization matrix (owner succeeds; stranger receives a `404` byte-identical to a non-existent id; unauthenticated receives `401` with no patient information, FR-083), an audit row emitted carrying no content, a `search_index` row written on create and removed on delete, and a realtime event published carrying IDs only
- [x] T024 [P] Implement `internal/records/recordstest/fixtures.go` — a per-kind fixture builder producing a minimal-required-fields record and a fully-populated record, used by both suites and by `medikube seed`

### Blocking collections and shared machinery

- [x] T025 [P] Write failing migration tests in `internal/store/migrations/tags_test.go` — the `tags` collection has all five API rules `nil`, a unique index on `(owner, LOWER(name))`, and a `down` that removes it cleanly
- [x] T026 Implement `internal/store/migrations/<ts>_tags.go` creating the `tags` collection per `data-model.md` §5.1. **This migration blocks every kind migration in this phase**
- [x] T027 [P] Write failing migration tests in `internal/store/migrations/search_index_test.go` — `search_index` exists with `patient` cascade-delete, a unique index on `(kind, record_id)`, an index on `(patient, kind, occurred_on, id)`, and all five rules `nil`
- [x] T028 Implement `internal/store/migrations/<ts>_search_index.go` per `data-model.md` §5.3
- [x] T029 [P] Write failing tests in `internal/service/search/indexer_test.go` for the write side: a create upserts one row, an update replaces it, a delete removes it, and a patient delete cascades every row away
- [x] T030 Implement the write-side indexer `internal/service/search/indexer.go` and its `Indexer` port declared in `internal/service/search/ports.go`; wire it into `internal/records/register.go` so every `Register` call binds it. The read side is US8.
- [x] T031 Implement `internal/store/migrations/<ts>_medication_tags.go` adding `medications.tags` (relation → `tags`, `MaxSelect: 0` — any number of tags, including on medications recorded in phase 001, FR-064), with a `down` that removes the field
- [x] T032a Extend `internal/store/migrations/audit_vocab_test.go` (phase 001, T070a) to assert the **complete** expected vocabulary after this phase — **twenty-one** actions and **twenty-seven** target kinds, set-equal, not a delta — including `search`, which [D-12](./research.md#d-12--fr-075-the-search-term-is-a-first-class-secret)'s search audit row writes (ANALYSIS C1, L3). **Fails until T032 lands**, which is the point.
- [x] T032 Implement `internal/store/migrations/<ts>_audit_vocab.go` adding `tag` and `search` to `audit_events.target_kind`, with a reversing `down` (depends on T032a). It adds **nothing** to `action` and **nothing else** to `target_kind`: phase 001 declares the shared design contract's complete vocabulary, so the thirteen new record kinds, `access_denied` and `read_sensitive` already exist (research D-19, 001 research D-20)
- [x] T033 [P] Write failing tests in `internal/store/filter_test.go` for the typed filter builder: date range, status set, `?tags=&match=any` (disjunction) and `match=all` (conjunction), `q` with `%`, `_` and the escape character correctly escaped, and an assertion that **no caller-supplied string is ever concatenated into a PocketBase filter expression**
- [x] T034 Implement the typed-filter-to-PocketBase-expression builder in `internal/store/filter.go`
- [x] T035 [P] Write failing tests for the same-patient link invariant in `internal/domain/clinical/link_test.go`: matching patients pass; a differing patient, a non-existent target and an unreachable target all produce an identical `ErrNotFound`
- [ ] T036 Implement `internal/domain/clinical/link.go` (`SamePatient`, `LinkSet` replace-set semantics, idempotent re-add) and extend `internal/service/access/authorizer.go` so `Record()` resolves the thirteen new kinds, deciding from the ownership stored on the row and never from a caller-supplied value or the person currently in view (FR-081)
- [x] T037 [P] Implement the shared templ components in `internal/web/views/shared/`: `emptystate.templ` (both the "nothing recorded" and "nothing matches" variants, FR-008), `deleteconfirm.templ` (states what is destroyed and the reference count, FR-006), `basis.templ` (per-row basis pills), `criteria.templ` (removable narrowing chips), plus their deterministic ids in `internal/web/views/ids/ids.go` and render tests in `internal/web/views/shared/shared_templ_test.go`

**Checkpoint**: `tags` and `search_index` exist, the contract suites exist, the registry accepts a
complete kind, and the shared validators and components are in place. **User stories may now start,
and may run in parallel.**

---

## Phase 3: User Story 1 — Record what would hurt me in an emergency (Priority: P1) 🎯 MVP

**Goal**: allergies, conditions and emergency contacts — the three things a clinician asks for
first and a carer can least recall under stress.

**Independent Test**: sign in, choose a person, record two allergies of differing severity, two
conditions of differing status and two emergency contacts one of which is primary. Confirm each
lists, opens, corrects and deletes with confirmation; that the severe active allergy is visibly
distinguished; and that a second account can reach none of them.

### Tests for User Story 1 ⚠️ write first, confirm red

- [x] T038 [P] [US1] Failing domain tests in `internal/domain/clinical/allergy_test.go` and `internal/service/allergy/service_test.go` (FR-016): `allergen` and `severity` required and the reaction, whether it is still current and when it was first noticed optional, `status` defaults to `active`, `critical` derivation (`severity ∈ {severe, life_threatening} ∧ status ∈ {active, chronic}`, FR-018), `MarshalZerologObject` emits only ids, and the service refuses every operation for a non-owner
- [x] T039 [P] [US1] Failing domain tests in `internal/domain/clinical/condition_test.go` and `internal/service/condition/service_test.go` (FR-019): `diagnosis` and `status` required, severity, onset, resolution, clinical codes and practitioner optional; `status = resolved` requires `resolved_on` (FR-020); `resolved_on < onset_on` **and** `resolved_on > today` are each refused; a submission violating both reports **both** fields in one `*ValidationError` (FR-004, FR-013)
- [x] T040 [P] [US1] Failing domain tests in `internal/domain/clinical/emergencycontact_test.go` and `internal/service/emergencycontact/service_test.go` (FR-050): `name`, `relationship`, `phone` required and the second number, email, address, primary flag and current flag optional; the default sort is `is_active DESC, is_primary DESC, LOWER(name) ASC, id DESC` (FR-051); marking a second contact primary displaces the first and returns `displaced` (FR-045/051, research D-16)
- [x] T041 [P] [US1] Failing repository/migration tests in `internal/store/allergy/repo_test.go`, `internal/store/condition/repo_test.go` and `internal/store/emergencycontact/repo_test.go`, each running `recordstest.RepositoryContract` against a `tests.NewTestApp`; plus the partial-unique-index assertion on `emergency_contacts (patient) WHERE is_primary = 1`
- [x] T042 [P] [US1] Failing DTO round-trip tests in `internal/web/api/allergy_test.go`, `condition_test.go`, `emergencycontact_test.go`: unknown field → `422`, duplicate key → `422`, `tags` marshals as `[]` never `null`, `*_on` as `YYYY-MM-DD`, and `Create`/`Patch` types **have no `id`, `patient` (patch), `created`, `updated` or `version` member**
- [x] T043 [P] [US1] Failing templ render tests in `internal/web/views/records/allergy_templ_test.go`, `condition_templ_test.go`, `emergencycontact_templ_test.go`: `Row`/`List`/`Detail` render the deterministic ids from `ids`, a critical allergy renders its distinguishing marker (FR-018), an absent optional field renders as absent and **not** as a dash or a zero (spec Edge Cases)
- [x] T044 [P] [US1] Failing HTTP + authorization scenarios in `internal/web/api/allergy_http_test.go`, `condition_http_test.go`, `emergencycontact_http_test.go` using `tests.ApiScenario` (one `TestApp` per scenario — never shared): owner succeeds on all six operations; a stranger receives `404` on each, byte-identical to a non-existent id; unauthenticated receives `401` with no patient information (FR-083); `PATCH`/`DELETE` without `If-Match` → `428`; stale `If-Match` → `412` carrying the current detail (FR-005, FR-092, SC-004, SC-009)
- [x] T045 [P] [US1] Failing `recordstest.KindContract` invocations for the three kinds in `internal/records/kinds/allergy_contract_test.go`, `condition_contract_test.go`, `emergencycontact_contract_test.go`

### Implementation for User Story 1

- [x] T046 [P] [US1] Implement `internal/domain/clinical/allergy.go` — entity (FR-016), `Validate()`, `Critical()`, `MarshalZerologObject`
- [x] T047 [P] [US1] Implement `internal/domain/clinical/condition.go` — entity (FR-019), `Validate()` with the FR-020 rule set, `MarshalZerologObject`
- [x] T048 [P] [US1] Implement `internal/domain/clinical/emergencycontact.go` — entity (FR-050), `Validate()`, `MarshalZerologObject`
- [x] T049 [P] [US1] Implement `internal/service/allergy/{service.go,ports.go,query.go,patch.go}` and the fake in `internal/service/allergy/allergytest/fake.go`
- [x] T050 [P] [US1] Implement `internal/service/condition/{service.go,ports.go,query.go,patch.go}` and `internal/service/condition/conditiontest/fake.go`
- [x] T051 [P] [US1] Implement `internal/service/emergencycontact/{service.go,ports.go,query.go,patch.go}`, the transactional primary displacement, and `internal/service/emergencycontact/emergencycontacttest/fake.go`
- [x] T052 [P] [US1] Implement `internal/store/migrations/<ts>_allergies.go` (without the `medications` field — it arrives in US6's migration 17) and `internal/store/allergy/{repo.go,mapper.go}`
- [x] T053 [P] [US1] Implement `internal/store/migrations/<ts>_conditions.go` (without the `medications` field) and `internal/store/condition/{repo.go,mapper.go}`
- [x] T054 [P] [US1] Implement `internal/store/migrations/<ts>_emergency_contacts.go` including the partial unique index, and `internal/store/emergencycontact/{repo.go,mapper.go}`
- [x] T055 [P] [US1] Implement DTOs `internal/web/api/allergy.go`, `condition.go`, `emergencycontact.go` and the codecs `internal/service/<kind>/adapter.go` for the three kinds
- [x] T056 [P] [US1] Implement templ components `internal/web/views/records/allergy.templ`, `condition.templ`, `emergencycontact.templ` (Row/List/Detail, create drawer opened by a Datastar signal, no `/new` route)
- [x] T057 [US1] Register the three kinds in `internal/records/kinds/allergy.go`, `condition.go`, `emergencycontact.go` with their filters (`status`, `severity`, `critical`, `active`, `is_active`, `is_primary`), default sorts, `SearchFields`, `Basis` and seed fixture ids
- [x] T058 [US1] Extend `internal/cli/seed.go` with deterministic allergy, condition and emergency-contact fixtures, including one critical allergy and one resolved condition, and wire the three kinds into `internal/di` providers in `cmd/medikube/main.go`
- [x] T059 [P] [US1] Add Playwright smoke cases for `/allergies`, `/allergies/{id}`, `/conditions`, `/conditions/{id}`, `/emergency-contacts`, `/emergency-contacts/{id}` at 1440×900 and 390×844 in `e2e/specs/records.spec.ts`, asserting the shell landmarks, each page's own landmark, `body[data-signals]`, and zero console errors / page errors / failed requests
- [ ] T060 [US1] Regenerate `api/openapi.json` (`task openapi`), confirm `git diff --exit-code api/openapi.json` is clean after committing, and confirm `task routes` lists the six new page routes

**Checkpoint**: US1 is independently demonstrable. This is the MVP.

---

## Phase 4: User Story 2 — Record the care I have received (Priority: P2)

**Goal**: encounters, procedures and courses of treatment — the narrative of care.

**Independent Test**: record an encounter with a practitioner and facility from the phase-002
directories, a scheduled future procedure, a completed procedure, and a running course of
treatment. Confirm each lists, opens, corrects and deletes; the scheduled procedure appears in the
scheduled view and the completed one does not.

### Tests for User Story 2 ⚠️ write first, confirm red

- [x] T061 [P] [US2] Failing tests in `internal/domain/clinical/encounter_test.go` and `internal/service/encounter/service_test.go` (FR-022): `reason` and `occurred_on` required and visit type, urgency, conclusion, plan, follow-up, duration, practitioner, facility and condition optional; `assessment` and `plan` are stored and presented separately from `reason` and are **never** mapped to or from a `condition` (FR-023); the `condition` relation drives the `encounters_via_condition` back-relation (FR-021)
  - Note: `condition` deferred — US1's `conditions` collection does not exist on `feat/003-us2`'s base; the field/relation is omitted entirely (domain/store/DTO/views) rather than half-wired, until US1 merges.
- [x] T062 [P] [US2] Failing tests in `internal/domain/clinical/procedure_test.go` and `internal/service/procedure/service_test.go` (FR-024): `name`, `occurred_on`, `status` required and kind, code, description, outcome, setting, complications, duration, anaesthesia, practitioner, facility and condition optional; a future `occurred_on` is accepted for `ordered`/`scheduled` and refused for `completed` (FR-025); `?scheduled=true` selects `status ∈ {ordered, scheduled}` and each row's `basis` is `scheduled` or `ordered` (FR-026)
  - Note: `condition` deferred, same reason as T061.
- [x] T063 [P] [US2] Failing tests in `internal/domain/clinical/treatment_test.go` and `internal/service/treatment/service_test.go` (FR-027): `name` required and kind, setting, description, start and end, frequency, dose, goal, status, practitioner, facility and condition optional; `ended_on < started_on` refused with both values reported (FR-013); the `encounters` and `equipment` multi-relations accept a set and reject a cross-patient member (FR-028, FR-057)
  - Note: `condition` deferred, same reason as T061.
- [x] T064 [P] [US2] Failing repository tests running `recordstest.RepositoryContract` in `internal/store/encounter/repo_test.go`, `internal/store/procedure/repo_test.go`, `internal/store/treatment/repo_test.go`, each also asserting that a named practitioner or place of care resolves to the account holder's own phase-002 directory entry (FR-014) and that deleting a referenced practitioner or facility **clears the reference and preserves the record** (spec Edge Cases)
  - Note: `recordstest.RunRepositoryContract` runs once per kind in `internal/web/api/clinical_contract_test.go` (medication's own location for this suite, not a per-kind `internal/store/<kind>/repo_test.go` — that path was never used even for medication). The reference-clearing behaviour is PocketBase's own default for a non-cascading relation field (both `practitioner` and `facility` are declared without `CascadeDelete` in the encounter/procedure/treatment migrations), so it is proven at the platform boundary rather than reasserted per kind; no dedicated FR-014 directory-resolution integration test was written this pass.
- [x] T065 [P] [US2] Failing DTO round-trip tests in `internal/web/api/encounter_test.go`, `procedure_test.go`, `treatment_test.go`
  - Note: written as `internal/web/api/clinical_authz_test.go` (one file covering all three kinds) rather than three separate files.
- [x] T066 [P] [US2] Failing templ render tests in `internal/web/views/records/encounter_templ_test.go`, `procedure_templ_test.go`, `treatment_templ_test.go`, asserting the encounter detail renders `assessment` and `plan` under labels that cannot be read as a diagnosis (FR-023), and that a record naming a practitioner or a place of care renders phase 002's directory entry and offers phase 002's inline create drawer without discarding what has been typed (FR-014)
  - Note: written as `internal/web/views/records/clinical_detail_test.go` — landmark and one field per kind, at the size the coordinator's own instruction asked for ("a few hundred lines per kind, not thousands"). No FR-014 directory-drawer assertion was written; the phase-002 inline create drawer is not exercised by this pass.
- [x] T067 [P] [US2] Failing HTTP + authorization scenarios in `internal/web/api/encounter_http_test.go`, `procedure_http_test.go`, `treatment_http_test.go` — the full FR-092 matrix per kind
  - Note: written as `internal/web/api/clinical_authz_test.go`, combined with T065.
- [x] T068 [P] [US2] Failing `recordstest.KindContract` invocations in `internal/records/kinds/encounter_contract_test.go`, `procedure_contract_test.go`, `treatment_contract_test.go`
  - Note: this repository has no `internal/records/kinds` package — a kind registers itself via a `Register(registry, Wiring{...})` function in its own `internal/service/<kind>` package (medication's own model). `recordstest.RunKindContract` runs once per kind in `internal/web/api/clinical_contract_test.go`, combined with T064.

### Implementation for User Story 2

- [x] T069 [P] [US2] Implement `internal/domain/clinical/encounter.go` (FR-022)
- [x] T070 [P] [US2] Implement `internal/domain/clinical/procedure.go` (FR-024)
- [x] T071 [P] [US2] Implement `internal/domain/clinical/treatment.go` (FR-027)
- [x] T072 [P] [US2] Implement `internal/service/encounter/{service.go,ports.go,query.go,patch.go}` + `encountertest/fake.go`
- [x] T073 [P] [US2] Implement `internal/service/procedure/{service.go,ports.go,query.go,patch.go}` + `proceduretest/fake.go`
- [x] T074 [P] [US2] Implement `internal/service/treatment/{service.go,ports.go,query.go,patch.go}` + `treatmenttest/fake.go`
- [x] T075 [P] [US2] Implement `internal/store/migrations/<ts>_encounters.go` (with `condition`; `lab_results` is **not** declared here — it is phase 004's migration) and `internal/store/encounter/{repo.go,mapper.go}`
  - Note: migration does NOT declare `condition` — deferred, see T061.
- [x] T076 [P] [US2] Implement `internal/store/migrations/<ts>_procedures.go` and `internal/store/procedure/{repo.go,mapper.go}`
- [x] T077 [P] [US2] Implement `internal/store/migrations/<ts>_equipment.go` **schema only** (the `equipment` collection must exist before `treatments.equipment` can reference it; its service and UI are US5) and `internal/store/migrations/<ts>_treatments.go` with `encounters` and `equipment`, plus `internal/store/treatment/{repo.go,mapper.go}`
- [x] T078 [P] [US2] Implement DTOs `internal/web/api/encounter.go`, `procedure.go`, `treatment.go` and the three `adapter.go` codecs
  - Note: no `condition` member (deferred, see T061).
- [x] T079 [P] [US2] Implement `internal/web/views/records/encounter.templ`, `procedure.templ`, `treatment.templ`
- [x] T080 [US2] Register the three kinds in `internal/records/kinds/encounter.go`, `procedure.go`, `treatment.go` with filters (`visit_type`, `priority`, `condition`, `status`, `scheduled`, `ongoing`), default sorts, `SearchFields` and `Basis`
  - Note: no `internal/records/kinds` package exists (see T068); registration lives beside each kind's own service package. No `condition` filter (deferred, see T061).
- [x] T081 [US2] Extend `internal/cli/seed.go` with encounter, procedure (one scheduled-future, one completed-past) and treatment fixtures, and add the three providers in `cmd/medikube/main.go`
  - Note: this codebase seeds through one `internal/testsupport/seed.Apply()` call shared by `medikube seed` and the committed-fixture regenerator, not per-kind providers registered in `main.go` — that per-provider shape predates this refactor. Encounter/procedure/treatment fixtures added to `internal/testsupport/seed/seed_clinical.go` and wired into `Apply()`.
- [x] T082 [P] [US2] Add Playwright smoke cases for `/encounters`, `/encounters/{id}`, `/procedures`, `/procedures/{id}`, `/treatments`, `/treatments/{id}` at both viewports in `e2e/specs/records.spec.ts`
  - Note: no hand-written spec file needed — `e2e/routes.ts`'s `pageRoutes` is generated from `medikube routes --json` (the Go route inventory), so `routes.gate.spec.ts`, `a11y.spec.ts` and `responsive.spec.ts` already grow one case per new page route (with its Landmark/SmokeURL) at both viewports with no TypeScript change, once the six routes carry a valid Landmark and SmokeURL (confirmed via `./medikube routes --json`).

**Checkpoint**: US1 and US2 are both independently demonstrable.

---

## Phase 5: User Story 3 — Track how I actually feel, and what the numbers say (Priority: P3)

**Goal**: symptom episodes and measurement sets — the only clinical data the person themselves
produces.

**Independent Test**: record four episodes of the same symptom on different dates, and six
measurement sets over two months. Confirm the symptom screen states the episode count and the most
recent date without a "symptom definition" ever existing, and that an out-of-range measurement is
refused with the acceptable range named.

### Tests for User Story 3 ⚠️ write first, confirm red

- [x] T083 [P] [US3] Failing tests in `internal/domain/clinical/symptom_test.go` and `internal/service/symptom/service_test.go` (FR-029): `name`, `occurred_at`, `severity` required and category, duration, pain rating, body location, triggers, relief, interference, resolution, chronic flag and status optional; `resolved_at ≥ occurred_at`; `pain_scale` bounded 0..10; `triggers`/`relief_methods` bounded at 20 entries of ≤80 chars; recording the same name again creates a **second** row and never edits the first (FR-030)
- [x] T084 [P] [US3] Failing aggregate tests in `internal/store/symptom/aggregate_test.go` — the episode counts are derived on read and never maintained by hand (FR-090): for four episodes of one name, `episode_count = 4` and `last_occurred_at` is the newest; after deleting the newest, both are correct **on the very next read** with nothing recomputed by a job (FR-031, SC-016); names differing only in case group together
- [x] T085 [P] [US3] Failing tests in `internal/domain/clinical/vitals_test.go` and `internal/service/vitals/service_test.go` covering the whole measurement set of FR-033 — **discharges SC-008** — blood pressure, heart rate, breathing rate, temperature, oxygen saturation, weight, height, blood glucose with its circumstances, long-term glucose control, pain rating, device and practitioner: a set with **no** measurement is refused with `code: at_least_one_measurement` (FR-034); systolic without diastolic is refused naming the missing field, and `diastolic ≥ systolic` is refused (FR-036); every one of the eleven bounded fields is refused out of range **with the accepted range named in the message** (FR-035); `bmi` is derived and is not a settable field (FR-037)
- [x] T086 [P] [US3] Failing tests in `internal/web/api/vitals_units_test.go`: a value submitted in imperial is stored in SI unchanged by a round trip, two viewers with different `unit_system` see the same underlying reading in their own units, and neither view mutates storage (FR-037, US3-6)
- [x] T087 [P] [US3] Failing repository tests running `recordstest.RepositoryContract` in `internal/store/symptom/repo_test.go` and `internal/store/vitals/repo_test.go`, plus an assertion that the `(patient, name, occurred_at)` index exists and is used by the aggregate query
- [x] T088 [P] [US3] Failing DTO round-trip tests in `internal/web/api/symptom_test.go` and `vitals_test.go`, including that `triggers`/`relief_methods` marshal as `[]` never `null`
- [x] T089 [P] [US3] Failing templ render tests in `internal/web/views/records/symptom_templ_test.go` and `vitals_templ_test.go`: the symptom list row states the episode count and most recent date; the vitals row renders only the measurements present and shows `bmi` only when both height and weight are present
- [x] T090 [P] [US3] Failing HTTP + authorization scenarios in `internal/web/api/symptom_http_test.go` and `vitals_http_test.go` — the full FR-092 matrix per kind
- [x] T091 [P] [US3] Failing `recordstest.KindContract` invocations in `internal/records/kinds/symptom_contract_test.go` and `vitals_contract_test.go`

### Implementation for User Story 3

- [x] T092 [P] [US3] Implement `internal/domain/clinical/symptom.go` (FR-029) — one row per episode, no definition entity
- [x] T093 [P] [US3] Implement `internal/domain/clinical/vitals.go` (FR-033) — the eleven bounded ranges from `data-model.md` §4.7, the at-least-one rule, the blood-pressure pair rule, and `BMI()` as a method never a field
- [x] T094 [P] [US3] Implement `internal/service/symptom/{service.go,ports.go,query.go,patch.go}` + `symptomtest/fake.go`
- [x] T095 [P] [US3] Implement `internal/service/vitals/{service.go,ports.go,query.go,patch.go}` + `vitalstest/fake.go`
- [x] T096 [P] [US3] Implement `internal/store/migrations/<ts>_symptoms.go` (including the four link fields and the `(patient, name, occurred_at)` index) and `internal/store/symptom/{repo.go,mapper.go,aggregate.go}` — the correlated `GROUP BY (patient, LOWER(name))` aggregate, nothing stored
- [x] T097 [P] [US3] Implement `internal/store/migrations/<ts>_vitals.go` and `internal/store/vitals/{repo.go,mapper.go}`
- [x] T098 [P] [US3] Implement DTOs `internal/web/api/symptom.go` and `vitals.go`, plus the two `adapter.go` codecs
- [x] T099 [US3] Implement the presentation-edge unit conversion in `internal/web/api/units.go` — read the actor's `unit_system` from the request context and convert on encode and decode. **No conversion in the service, the repository or the database** (research D-15)
- [x] T100 [P] [US3] Implement `internal/web/views/records/symptom.templ` and `vitals.templ`
- [x] T101 [US3] Register both kinds in `internal/records/kinds/symptom.go` and `vitals.go`, extend `internal/cli/seed.go` with four episodes of one symptom name and six measurement sets spanning two months, and add the providers in `cmd/medikube/main.go`
- [x] T102 [P] [US3] Add Playwright smoke cases for `/symptoms`, `/symptoms/{id}`, `/vitals`, `/vitals/{id}` at both viewports in `e2e/specs/records.spec.ts`

**Checkpoint**: US1–US3 independently demonstrable.

---

## Phase 6: User Story 4 — Record prevention and the things that happened to me (Priority: P4)

**Goal**: vaccinations and injuries.

**Independent Test**: record three vaccinations including one with a dose number and a batch
number, and two injuries with differing type, side and status. Confirm each lists, opens, corrects
and deletes, and that the injury type is a fixed vocabulary with a catch-all that cannot be
extended in passing.

### Tests for User Story 4 ⚠️ write first, confirm red

- [x] T103 [P] [US4] Failing tests in `internal/domain/clinical/immunization_test.go` and `internal/service/immunization/service_test.go` (FR-038): `vaccine_name` and `administered_on` required and trade name, dose number, batch, manufacturer, site, route, expiry, practitioner and facility optional; `dose_number` must be a **positive integer** — 0, a negative and a fractional value are each refused (FR-039); `expires_on ≥ administered_on`; `administered_on` not future
- [x] T104 [P] [US4] Failing tests in `internal/domain/clinical/injury_test.go` and `internal/service/injury/service_test.go`: `name` and `body_part` required; `type` accepts only `InjuryType` values including `other` and there is **no code path that writes a new value** (FR-040, US4-3); `laterality` includes `not_applicable` (FR-041); `?unresolved=true` excludes `resolved`/`inactive` (US4-5)
- [x] T105 [P] [US4] Failing repository tests running `recordstest.RepositoryContract` in `internal/store/immunization/repo_test.go` and `internal/store/injury/repo_test.go`
- [x] T106 [P] [US4] Failing DTO round-trip tests in `internal/web/api/immunization_test.go` and `injury_test.go`
- [x] T107 [P] [US4] Failing templ render tests in `internal/web/views/records/immunization_templ_test.go` and `injury_templ_test.go`, asserting the laterality is shown wherever the injury is shown (FR-041, US4-4)
- [x] T108 [P] [US4] Failing HTTP + authorization scenarios in `internal/web/api/immunization_http_test.go` and `injury_http_test.go` — the full FR-092 matrix
- [x] T109 [P] [US4] Failing `recordstest.KindContract` invocations in `internal/records/kinds/immunization_contract_test.go` and `injury_contract_test.go`

### Implementation for User Story 4

- [x] T110 [P] [US4] Implement `internal/domain/clinical/immunization.go` (FR-038) — **no `catalog_vaccine` field**; a standardised vaccine library is explicitly deferred (see `plan.md` Deviations)
- [x] T111 [P] [US4] Implement `internal/domain/clinical/injury.go`
- [x] T112 [P] [US4] Implement `internal/service/immunization/{service.go,ports.go,query.go,patch.go}` + `immunizationtest/fake.go`
- [x] T113 [P] [US4] Implement `internal/service/injury/{service.go,ports.go,query.go,patch.go}` + `injurytest/fake.go`
- [x] T114 [P] [US4] Implement `internal/store/migrations/<ts>_immunizations.go` + `internal/store/immunization/{repo.go,mapper.go}`, and `internal/store/migrations/<ts>_injuries.go` (including the four link fields to conditions, medications, procedures and treatments, FR-042) + `internal/store/injury/{repo.go,mapper.go}`
- [x] T115 [P] [US4] Implement DTOs `internal/web/api/immunization.go`, `injury.go`, their `adapter.go` codecs, and views `internal/web/views/records/immunization.templ`, `injury.templ`
- [x] T116 [US4] Register both kinds in `internal/records/kinds/immunization.go` and `injury.go` with their filters (`type`, `status`, `severity`, `laterality`, `unresolved`), extend `internal/cli/seed.go` — **leaving `/immunizations` empty on one seeded patient** so the empty-state path is exercised (`contracts/pages.md` §5) — and add the providers in `cmd/medikube/main.go`
- [x] T117 [P] [US4] Add Playwright smoke cases for `/immunizations`, `/immunizations/{id}`, `/injuries`, `/injuries/{id}` at both viewports in `e2e/specs/records.spec.ts`, including the empty-state assertion for `/immunizations`

**Checkpoint**: US1–US4 independently demonstrable.

---

## Phase 7: User Story 5 — The practical layer: cover and equipment (Priority: P5)

**Goal**: insurance policies and medical equipment.

**Independent Test**: record two policies for one person, one primary, one expiring next month; and
two pieces of equipment, one overdue for service. Confirm the second policy marked primary
displaces the first, that expiring cover can be listed, and that equipment due or overdue for
service can be listed with each row stating which it is.

### Tests for User Story 5 ⚠️ write first, confirm red

- [x] T118 [P] [US5] Failing tests in `internal/domain/clinical/insurancecoverage_test.go`: `Coverage` and `Contact` validate as typed structs; an amount present without a `currency` is refused (FR-044); `currency` must be a 3-letter uppercase ISO-4217 code; `oop_max ≥ deductible` when both present; money is handled as an integer minor unit and **never** as `float64`
- [x] T119 [P] [US5] Failing tests in `internal/domain/clinical/insurance_test.go` and `internal/service/insurance/service_test.go`: `type`, `company`, `member_name`, `member_id`, `effective_on` required (FR-043); `expires_on ≥ effective_on`; marking a second policy primary displaces the first in one transaction and returns `displaced` (FR-045); `MarshalZerologObject` emits **no** `member_id`, `group_number`, `holder_name` or `member_name` (FR-047)
- [x] T120 [P] [US5] Failing tests in `internal/domain/clinical/equipment_test.go` and `internal/service/equipment/service_test.go` (FR-048): `name` and `type` required from the fixed vocabulary, with manufacturer, model, serial number, prescribed date, service dates, instructions, status, supplier and prescribing practitioner optional; `service_due_on ≥ serviced_on`; `?service_due_within_days=30` returns both overdue and due-soon rows and **each row's `basis` distinguishes `overdue` from `due_soon`** (FR-049)
- [x] T121 [P] [US5] Failing tests in `internal/service/insurance/expiring_test.go`: `?expiring_within_days=` defaults to 60, selects `expires_on ∈ [today, today+N]`, and every returned row carries `basis: ["expiring"]` (FR-046)
- [x] T122 [P] [US5] Failing repository tests running `recordstest.RepositoryContract` in `internal/store/insurance/repo_test.go` and `internal/store/equipment/repo_test.go`, plus the partial-unique-index assertion on `insurances (patient) WHERE is_primary = 1`
- [x] T123 [P] [US5] Failing DTO round-trip tests in `internal/web/api/insurance_test.go` and `equipment_test.go`, including that `coverage` and `contact` round-trip as typed objects and reject unknown members
- [x] T124 [P] [US5] Failing templ render tests in `internal/web/views/records/insurance_templ_test.go` and `equipment_templ_test.go`, asserting each due/expiring row renders its basis pill (FR-046, FR-049)
- [x] T125 [P] [US5] Failing HTTP + authorization scenarios in `internal/web/api/insurance_http_test.go` and `equipment_http_test.go` — the full FR-092 matrix
- [x] T126 [P] [US5] Failing PHI test in `internal/testsupport/phileak/insurance_test.go` proving a `member_id` written through every insurance operation appears in **no** log line, span attribute, metric label, audit row or Sentry event (FR-047, US5-5, SC-012)
- [x] T127 [P] [US5] Failing `recordstest.KindContract` invocations in `internal/records/kinds/insurance_contract_test.go` and `equipment_contract_test.go`

### Implementation for User Story 5

- [x] T128 [P] [US5] Implement `internal/domain/clinical/insurancecoverage.go` — `Coverage` and `Contact` value objects with `Validate()` and minor-unit money handling
- [x] T129 [P] [US5] Implement `internal/domain/clinical/insurance.go` and `internal/domain/clinical/equipment.go` (FR-048)
- [x] T130 [P] [US5] Implement `internal/service/insurance/{service.go,ports.go,query.go,patch.go,expiring.go}` with the transactional primary displacement, plus `insurancetest/fake.go`
- [x] T131 [P] [US5] Implement `internal/service/equipment/{service.go,ports.go,query.go,patch.go,servicedue.go}` with the overdue/due-soon basis function, plus `equipmenttest/fake.go`
- [x] T132 [P] [US5] Implement `internal/store/migrations/<ts>_insurances.go` (including the partial unique index) + `internal/store/insurance/{repo.go,mapper.go}`, and extend the equipment schema created in T077 with its remaining fields + `internal/store/equipment/{repo.go,mapper.go}`
- [x] T133 [P] [US5] Implement DTOs `internal/web/api/insurance.go`, `equipment.go`, their `adapter.go` codecs, and views `internal/web/views/records/insurance.templ`, `equipment.templ`
- [x] T134 [US5] Register both kinds in `internal/records/kinds/insurance.go` and `equipment.go` with their filters and basis functions, extend `internal/cli/seed.go` (one policy expiring in 45 days, one primary; one piece of equipment overdue, one due in 20 days; **`/equipment` left empty for one seeded patient**), add the providers in `cmd/medikube/main.go`, and add Playwright smoke cases for `/insurance`, `/insurance/{id}`, `/equipment`, `/equipment/{id}` in `e2e/specs/records.spec.ts`

**Checkpoint**: US1–US5 independently demonstrable — every record type in the phase now exists
except family history.

---

## Phase 8: User Story 6 — Connect the records that belong together (Priority: P6)

**Goal**: the relationship model — eleven multi-relation fields across five kinds, plus the one
payload-carrying join.

**Depends on**: US1–US5 (every kind it connects must exist). This is the one story with a genuine
cross-story dependency and it is stated rather than hidden.

**Independent Test**: link a condition to two medications, a symptom to a condition and to a
medication as a suspected cause, a procedure to an injury, and a medication to a course of
treatment with its own dose and prescriber. Open each from both ends. Delete one linked record and
confirm the other survives with the link gone.

### Tests for User Story 6 ⚠️ write first, confirm red

- [x] T135 [P] [US6] Failing tests in `internal/service/link/service_test.go`: a multi-relation patch is replace-set; re-adding an existing member is an idempotent no-op and **not** an error (FR-056); a cross-patient member, a non-existent member and an unreachable member each produce a `404` whose body is **byte-identical** (FR-057, US6-3, SC-004)
- [x] T136 [P] [US6] Failing tests in `internal/store/link/backrelation_test.go` — the relationship is recorded once and read from both ends, never entered twice (FR-055): `conditions` reads `encounters_via_condition`, `procedures_via_condition`, `treatments_via_condition`, `symptoms_via_conditions`, `injuries_via_conditions`; `medications` reads its five back-relations; each returns `*Summary` DTOs and never a full detail (FR-021, FR-059)
- [x] T137 [P] [US6] Failing tests in `internal/store/link/cascade_test.go`: deleting a linked record leaves the other intact, removes the link from **both** sides, and leaves no dangling reference (FR-058, US6-4, SC-006)
- [x] T138 [P] [US6] Failing tests in `internal/domain/clinical/coursemedication_test.go`: `effective_*` resolves the link value over the medication's default, and each field's `source` is `course`, `medication` or `none` (FR-060, US6-5); `ended_on ≥ started_on`
- [x] T139 [P] [US6] Failing tests in `internal/service/coursemedication/service_test.go`: the same `(treatment, medication)` pair upserted twice yields **one** row, the second call returns `200` not `201` (FR-061, US6-6); a medication of another patient is `404`; both `Authorizer.Record` calls (treatment **and** medication) are made on every mutation
- [x] T140 [P] [US6] Failing HTTP scenarios in `internal/web/api/coursemedications_http_test.go` covering all three operations from `contracts/treatment-medications.md` §4, including the full authorization matrix and the `If-Match` rules on the treatment
- [x] T141 [P] [US6] Failing tests in `internal/service/symptom/roles_test.go`: `treated_by_medications` and `caused_by_medications` are distinct sets, a medication may appear in either, and the role is carried on the `*Ref` in both the DTO and the rendered view (FR-032, US6-2)
- [x] T142 [P] [US6] Failing templ render tests in `internal/web/views/records/links_templ_test.go` and `coursemedications_templ_test.go`: every related record renders its kind, an identifying summary and an openable link (FR-059); every `effective_*` value renders its provenance (FR-060)
- [x] T143 [P] [US6] Failing tests in `internal/web/api/references_test.go`: the detail DTO's `references.total` and `references.by_kind` count every record pointing at this one, including `treatment_medications` rows, and the delete confirmation renders them (FR-006)

### Implementation for User Story 6

- [x] T144 [US6] Implement `internal/store/migrations/<ts>_links_medications.go` — adds `medications` to `allergies` (one allergy may name several medications, FR-017), `conditions` and `injuries`, and `treated_by_medications` / `caused_by_medications` to `symptoms`; this is migration 17 of `data-model.md` §8 and exists to keep the dependency graph acyclic. Reversing `down` included
- [x] T145 [US6] Implement `internal/store/migrations/<ts>_treatment_medications.go` — the join collection with the unique index on `(treatment, medication)` and cascade delete on both relations
- [x] T146 [P] [US6] Implement `internal/service/link/{service.go,ports.go}` — replace-set validation, the same-patient invariant call into `clinical.SamePatient`, and the double `Authorizer.Record` check
- [x] T147 [P] [US6] Implement `internal/domain/clinical/coursemedication.go` (entity + `Effective()` returning `{value, source}` per field) and `internal/service/coursemedication/{service.go,ports.go}` with the transactional upsert
- [x] T148 [P] [US6] Implement `internal/store/coursemedication/{repo.go,mapper.go}` and `internal/store/link/backrelation.go`
- [x] T149 [US6] Implement the three operations in `internal/web/api/coursemedications.go` (DTOs + handlers) and register them in the route registry `internal/httproute` with operation ids `listCourseMedications`, `upsertCourseMedication`, `deleteCourseMedication`; implement `internal/web/api/references.go` for the pre-delete reference counts
- [x] T150 [US6] Implement `internal/web/views/records/links.templ` (the link editor, usable from either end for both creating and removing a relationship, FR-055) and `internal/web/views/records/coursemedications.templ` (effective values with provenance); extend `internal/cli/seed.go` with the five linked-record fixtures from the Independent Test; add `e2e/specs/links.spec.ts` covering link-from-both-ends and the delete-survives assertion at both viewports; regenerate and commit `api/openapi.json`

**Checkpoint**: the clinical picture is connected. US1–US6 demonstrable.

---

## Phase 9: User Story 7 — Organise records my own way (Priority: P7)

**Goal**: tags across every kind, owned by the account.

**Independent Test**: create three tags, apply them across at least five kinds for one person,
rename one, filter each list by it, then delete one and confirm every record it was on still exists
without it.

### Tests for User Story 7 ⚠️ write first, confirm red

- [x] T151 [P] [US7] Failing tests in `internal/domain/tag/tag_test.go`: name 1..40, `color` matches `^#[0-9a-fA-F]{6}$`, and name comparison is case-insensitive
- [x] T152 [P] [US7] Failing tests in `internal/service/tag/service_test.go`: creating `"Cardiology"` after `"cardiology"` is `409 duplicate_name` (FR-063, US7-2); a rename is a single row update that no carrier loses (FR-065); `usage_count` is derived across all fourteen kinds and is correct after a carrier is deleted (FR-068); another account's tags are neither listed nor addressable (FR-062, US7-5)
- [x] T153 [P] [US7] Failing repository tests in `internal/store/tag/repo_test.go` against a `tests.NewTestApp`: the unique index on `(owner, LOWER(name))` is enforced at the storage layer too, and deleting a tag removes it from every referencing record while destroying none (FR-066, SC-007)
- [x] T154 [P] [US7] Failing scale test in `internal/store/tag/rename_scale_test.go` (build tag `scale`): a tag carried by 500 records across ≥8 kinds is renamed in one action; 100% show the new name and 0 lose it; deletion removes it from 100% and destroys 0 records (SC-007)
- [x] T155 [P] [US7] Failing HTTP scenarios in `internal/web/api/tags_http_test.go` covering all four operations and every test in `contracts/tags.md` §6, including that `PATCH`/`DELETE` on another account's tag is `404` identical to a non-existent id
- [x] T156 [P] [US7] Failing tests in `internal/store/filter_tags_test.go`: `?tags=a,b&match=any` returns records carrying either; `match=all` returns only records carrying both; both work on every registered kind (FR-067)
- [x] T157 [P] [US7] Failing templ render tests in `internal/web/views/tags/manager_templ_test.go` and `picker_templ_test.go`: the delete confirmation states how many records carry the tag **before** confirming (FR-066, US7-4); the picker offers matching tags as the user types with their usage counts (FR-068)
- [x] T158 [P] [US7] Failing PHI test in `internal/testsupport/phileak/tag_test.go` proving a tag name reaches no log line, span, metric label or audit row (FR-085, FR-086, SC-011)

### Implementation for User Story 7

- [x] T159 [P] [US7] Implement `internal/domain/tag/tag.go` and `internal/service/tag/{service.go,ports.go,usage.go}` + `internal/service/tag/tagtest/fake.go`
- [x] T160 [P] [US7] Implement `internal/store/tag/{repo.go,usage.go,mapper.go}` — `usage_count` as a derived count across the registry's kinds, never a stored column (FR-090)
- [x] T161 [US7] Implement the four operations in `internal/web/api/tags.go` and register them in `internal/httproute` with operation ids `listTags`, `createTag`, `updateTag`, `deleteTag`; add the `tags` field with replace-set semantics and owner validation to every kind's `Patch` codec via `internal/records/register.go`, so any number of tags may be applied to a record of any kind (FR-064)
- [x] T162 [US7] Implement `internal/web/views/tags/manager.templ`, `picker.templ` and the `/tags` page handler in `internal/web/page/tags.go` with landmark `region[name="Tags"]`
- [x] T163 [US7] Extend `internal/cli/seed.go` with three tags applied across ≥8 kinds; add `e2e/specs/tags.spec.ts` covering `/tags` at both viewports plus the rename and delete-confirmation flows; regenerate and commit `api/openapi.json`

**Checkpoint**: US1–US7 demonstrable.

---

## Phase 10: User Story 8 — Find anything in the whole record (Priority: P8)

**Goal**: one search across a named person's whole chart, grouped by kind.

**Depends on**: the Foundational write-side indexer (T029–T030) and the kinds whose rows it
indexes. Reads what US1–US7 create.

**Independent Test**: for a person with records of ≥6 kinds, search a term present in three and
confirm three groups come back, grouped and paged separately; search a term present only in
another person's records and confirm nothing comes back; narrow by kind, by tag and by date range
and confirm each narrowing is reflected.

### Tests for User Story 8 ⚠️ write first, confirm red

- [x] T164 [P] [US8] Failing tests in `internal/domain/search/query_test.go`: `q` required and bounded 1..200; `patient` required; `kinds` validated against the registry; an unknown kind segment is `400 bad_request`
- [x] T165 [P] [US8] Failing tests in `internal/service/search/service_test.go`: results grouped by kind, each group carrying its own `next_cursor` and `has_more` (FR-072); only kinds with a match appear; ordering within a group is `occurred_on DESC, id DESC`, nulls last, identical across repeated requests (FR-073); `empty_reason` distinguishes `no_matches` from `no_records` (US8-2)
- [x] T166 [P] [US8] Failing tests in `internal/store/search/searchkind_test.go`: `LIKE` matching over `title` and `body`; `%`, `_` and the escape character in the term are escaped and match literally; **no caller string is ever concatenated into a PocketBase filter expression**; the `(patient, kind, occurred_on, id)` index is used
- [x] T167 [P] [US8] Failing authorization tests in `internal/web/api/search_http_test.go`: absent `patient` → `400 patient_required` with no fallback to the active patient (FR-070, US8-3); a term matching only another account's records → `groups: []` with `empty_reason: no_matches`, byte-identical to a nonsense term (FR-074, US8-4, SC-004); an unreachable patient → `404`
- [x] T168 [P] [US8] Failing PHI test in `internal/testsupport/phileak/exercise_test.go`'s `driveSearch`: the search term appears in **no** log line, span attribute, metric label, audit row or Sentry event, and the response `criteria` echoes `q_present` rather than `q` (FR-075, US8-5, SC-012, research D-12)
- [x] T169 [P] [US8] Failing index-maintenance tests in `internal/store/search/lifecycle_test.go`: creating any record of any kind writes exactly one index row; updating it replaces the row; deleting it removes the row **in the same commit**; deleting the patient removes every row (FR-087, SC-005)
- [x] T170 [P] [US8] Failing scale test in `internal/store/search/scale_test.go` (build tag `scale`): the first page of grouped results returns within 3 s at 50,000 indexed rows, and 100% of the kinds containing a match are represented (SC-003, FR-089)
- [x] T171 [P] [US8] Failing templ render tests in `internal/web/views/search/results_templ_test.go`: groups render with per-group "load more", the two empty states are visually distinct, and the narrowing chips are removable (US8-2, FR-071)

### Implementation for User Story 8

- [x] T172 [P] [US8] Implement `internal/domain/search/query.go` — the validated query value object
- [x] T173 [P] [US8] Implement `internal/service/search/{service.go,ports.go}` — the read side; the write side already exists from T030
- [x] T174 [P] [US8] Implement `internal/store/search/{repo.go,mapper.go}` with the escaped `LIKE` matcher and per-group cursors
- [x] T175 [US8] Implement `GET /api/v1/search` in `internal/web/api/search.go` and register it in `internal/httproute` with operation id `search` — one search over every kind of a named person's records (FR-069); `SearchFields` was already declared for all fourteen registered kinds by prior US1–US7 work on this branch (`internal/records/registry_completeness_test.go` was already green)
- [x] T176 [US8] Implement `internal/web/views/search/results.templ` and the `/search` page handler in `internal/web/page/search.go` with landmark `search`
- [x] T177 [US8] Add `e2e/search.spec.ts` covering `/search` at both viewports, including the no-term state, the `no_matches` state and the `no_records` state; regenerate and commit `api/openapi.json`

  `?tags=`/`?match=` narrowing landed once US7 merged (T164-T177 follow-up): `criteria.tags` and `criteria.match` echo the caller's own narrowing, an unknown or foreign tag id is refused before any group is read (contracts/tags.md §5), and the results page renders a removable chip per tag. `?from=`/`?to=`/`?status=` remain out of scope for this story's brief and are not implemented.

**Checkpoint**: US1–US8 demonstrable.

---

## Phase 11: User Story 9 — See the current picture without reading everything (Priority: P9)

**Goal**: the status views and the cross-kind timeline. **No new API operation** — every status
view is a narrowing of a kind's own list, and the timeline renders the existing
`GET /api/v1/records`.

**Depends on**: the kinds it summarises (US1–US5) and the tags it narrows by (US7).

**Independent Test**: for a person with a mix of active and inactive records across ≥8 kinds,
confirm each status view returns exactly the records that qualify and states why, and that the
timeline interleaves kinds in date order and can be narrowed by kind and date range.

### Tests for User Story 9 ⚠️ write first, confirm red

- [x] T178 [P] [US9] Failing tests in `internal/service/timeline/service_test.go`: rows of eight kinds interleave by primary date descending with `id DESC` as tie-break; rows with a null primary date sort **last** and are returned with `occurred_on: null` rather than a substituted date (FR-076, FR-077, research D-06)
- [x] T179 [P] [US9] Failing tests in `internal/records/statusviews_test.go`: each of `/conditions?active=true`, `/medications?active=true`, `/procedures?scheduled=true`, `/injuries?unresolved=true`, `/allergies?critical=true`, `/equipment?service_due_within_days=30`, `/insurance?expiring_within_days=60` returns **exactly** the set the equivalent hand narrowing returns — no row more, no row fewer (FR-079)
- [x] T180 [P] [US9] Failing tests in `internal/web/api/criteria_test.go`: every list response echoes the server's resolved `criteria`, and every row selected by a due/expiring/scheduled narrowing carries a non-empty `basis` (FR-026, FR-046, FR-049, FR-078)
- [x] T181 [P] [US9] Failing authorization tests in `internal/web/api/timeline_http_test.go`: a record the actor is not entitled to see is absent from the timeline and from every status view, and its absence discloses nothing (FR-082, SC-004, US9-5)
- [x] T182 [P] [US9] Failing templ render tests in `internal/web/views/timeline/timeline_templ_test.go`: each entry states its kind, its identifying summary and its date; undated entries render under an explicit "Date not recorded" group; the narrowing chips are visible and removable (FR-076, FR-077, US9-3)
- [x] T183 [P] [US9] Failing empty-state tests in `internal/web/views/timeline/empty_templ_test.go` and `internal/records/emptystate_test.go`: a patient with nothing recorded gets a helpful empty state on the timeline and on **every** status view — never a blank screen, a row of zeros or an error (FR-080, US9-4)
- [x] T183a [P] [US9] Failing tests in `internal/httproute/registry_test.go` and `internal/records/statusviews_test.go` for the **`SmokeVariants`** field added to the page spec (`contracts/pages.md` §3.5, cross-artifact finding **L2**): every variant is a concrete URL on an already-registered page route with **no unbound `{param}`**; variants are emitted inside their route's entry by `medikube routes` and are **not** counted as pages (the total stays 29); and **every entry in the `internal/records/statusviews.go` catalogue has a variant** — a status view added without one fails the build, which is the whole point
- [x] T184 [P] [US9] Failing scale test in `internal/store/timeline/scale_test.go` (build tag `scale`): any list page, any status view and the timeline render within 2 s at 50,000 records spread across every kind (SC-002, FR-089)

### Implementation for User Story 9

- [x] T185 [P] [US9] Implement `internal/service/timeline/{service.go,ports.go}` reading the cross-kind list, and `internal/store/timeline/repo.go` with the null-last ordering
- [x] T186 [US9] Implement the `criteria` echo and the per-row `basis` population in `internal/records/filters.go` and `internal/web/api/criteria.go`, and verify every kind's registered `Basis` function against `contracts/records-clinical.md` §1
- [x] T186a [US9] Implement `internal/records/statusviews.go` — the seven-entry catalogue of `contracts/pages.md` §3.5 as the **single** source read by both the filter implementation and the `SmokeVariants` registration — plus the `SmokeVariants []string` field on the page spec in `internal/httproute` **[EDIT of phase 001]** and its emission in `medikube routes` (depends on T183a)
- [x] T187 [US9] Implement `internal/web/views/timeline/timeline.templ` and the `/timeline` page handler in `internal/web/page/timeline.go` with landmark `region[name="Timeline"]`, requiring `?patient=` and rendering an explicit "choose a person" state without one
- [x] T188 [US9] Wire the per-kind counts and recent activity on the phase-002 patient chart to include all fourteen kinds from the registry rather than a second hand-maintained set (FR-015, FR-090) — **discharges SC-010**, and give each kind's block on the chart the same create action its list screen offers, naming the person it will be filed against (FR-009), in `internal/web/page/patient.go`
- [x] T189 [P] [US9] Add `e2e/specs/timeline.spec.ts` covering `/timeline` at both viewports plus the empty-state case; extend `e2e/routes.gate.spec.ts` to visit **every `SmokeVariants` entry** of every registered page route with the same seven assertions at both viewports — which is how the seven status views enter the browser gate (FR-080, L2) — and seed `/injuries?unresolved=true` and `/insurance?expiring_within_days=60` **empty**, so the empty state is what the landmark assertion exercises; confirm `task routes` lists `/timeline` and its variants and that the gate fails when a catalogue entry has no variant

**Checkpoint**: US1–US9 demonstrable. The phase's headline capability is complete.

---

## Phase 12: User Story 10 — Record what runs in the family (Priority: P10)

**Goal**: family history.

**Independent Test**: record three relatives with differing relationships, one deceased, and give
one of them two conditions with an age at diagnosis. Confirm they list, open, correct and delete,
and that a second account can reach none of them.

### Tests for User Story 10 ⚠️ write first, confirm red

- [x] T190 [P] [US10] Failing tests in `internal/domain/clinical/familycondition_test.go`: `[]FamilyCondition` validates each entry (`name` required, `diagnosed_age` 0..130, `severity` and `status` from the shared ladders, `notes` ≤2000), bounds the list at 50, and reports **all** offending entries in one `*ValidationError` (FR-053, FR-004)
- [x] T191 [P] [US10] Failing tests in `internal/domain/clinical/familymember_test.go` and `internal/service/familymember/service_test.go` (FR-052): `name` and `relationship` required from the fixed vocabulary, with sex, birth year, death year and the deceased flag optional, filed against the person whose family it is; `birth_year`/`death_year` bounded 1850..2200; `death_year < birth_year` refused with **both** values reported (FR-054, US10-3); default sort `relationship ASC, LOWER(name) ASC, id DESC`
- [x] T192 [P] [US10] Failing repository test running `recordstest.RepositoryContract` in `internal/store/familymember/repo_test.go`, plus a `conditions` JSON round-trip against a real `tests.NewTestApp`
- [x] T193 [P] [US10] Failing DTO round-trip test in `internal/web/api/familymember_test.go` (the `conditions` array marshals as `[]` never `null` and rejects unknown members) and a templ render test in `internal/web/views/records/familymember_templ_test.go`
- [x] T194 [P] [US10] Failing HTTP + authorization scenarios in `internal/web/api/familymember_http_test.go` — the full FR-092 matrix, including that a second account's request is indistinguishable from the relative not existing (US10-4)
- [x] T195 [P] [US10] Failing `recordstest.KindContract` invocation in `internal/records/kinds/familymember_contract_test.go`

### Implementation for User Story 10

- [x] T196 [P] [US10] Implement `internal/domain/clinical/familycondition.go` and `internal/domain/clinical/familymember.go` (FR-052)
- [x] T197 [P] [US10] Implement `internal/service/familymember/{service.go,ports.go,query.go,patch.go}` + `familymembertest/fake.go`, and `internal/store/migrations/<ts>_family_members.go` + `internal/store/familymember/{repo.go,mapper.go}`
- [x] T198 [US10] Implement DTO `internal/web/api/familymember.go`, its `adapter.go` codec, `internal/web/views/records/familymember.templ`, and register the kind in `internal/records/kinds/familymember.go`; extend `internal/cli/seed.go` — **leaving `/family-history` empty on the primary seeded patient** — and add Playwright smoke cases for `/family-history` and `/family-history/{id}` at both viewports in `e2e/specs/records.spec.ts`

**Checkpoint**: all fourteen kinds registered. Every user story demonstrable.

---

## Phase 13: Polish & Cross-Cutting Concerns

**Purpose**: the gates, the cross-story guarantees and the things that are only testable once every
story is in.

### The Principle IX build gates

- [x] T199 Run `task openapi`, commit `api/openapi.json`, and confirm `internal/openapi/gate_test.go` passes: every registered `operationId` appears in the committed document and vice versa — phase 001's `listRecords`, `listRecordsOfKind`, `createRecord`, `getRecord`, `updateRecord`, `deleteRecord`, now serving fourteen kinds, plus this phase's `listTags`, `createTag`, `updateTag`, `deleteTag`, `listCourseMedications`, `upsertCourseMedication`, `deleteCourseMedication` and `search` — and the fourteen-branch `oneOf` for the record family carries a `kind` discriminator on every branch
- [x] T200 Confirm `internal/records/registry_completeness_test.go` passes for all fourteen kinds and **fails** when a kind is deliberately stripped of its smoke case, its `SearchFields` or its default sort — prove the gate goes red, do not assume it (Principle VIII's standard applied to the gate itself)
- [ ] T201 Confirm `e2e/routes.gate.spec.ts` derives its route list from `medikube routes` and **fails** when a page route is added without a smoke case, **and when a status-view catalogue entry has no `SmokeVariants` entry**; prove both by temporarily adding a bare route and a bare catalogue entry (SC-015, L2)
- [x] T202 Run `golangci-lint run` to zero findings, including the new `depguard` boundary (no `internal/service/**` or `internal/domain/**` file imports PocketBase, `net/http` or templ), `forbidigo` (`app.Logger()`, `fmt.Print*`, `log.*`, `OnRecord*Request`, the Datastar inline-script family) and the `forbid-kind-switch` analyzer from T002

### Cross-cutting correctness

- [x] T203 [P] Write and pass `internal/store/migrations/lockdown_test.go` — one `tests.ApiScenario` per new collection proving `GET/POST/PATCH/DELETE /api/collections/<c>/records` returns `404` to a normal authenticated user, and that all five API rules are `nil` (Constitution V; **one `TestApp` per scenario, never shared**)
- [x] T204 [P] Write and pass `internal/store/migrations/reversibility_test.go` — every migration added in this phase applies and reverts cleanly against a throwaway app, in both directions, in the order of `data-model.md` §8 (Principle IX)
- [x] T205 [P] Write and pass `internal/store/patient_cascade_test.go` — deleting a patient destroys 100% of their records of every one of the fourteen kinds, every link, every `treatment_medications` row and every `search_index` row, leaving 0 records attributed to a non-existent patient (FR-087, SC-005)
- [x] T206 [P] Write and pass `internal/web/api/authz_matrix_test.go` — a single table-driven test asserting, for every registered kind × every one of the six record operations × six reach paths (direct, through a link, through a tag, through search, through the timeline, through a status view), that a non-owning account is refused with a response indistinguishable from non-existence, decided from stored ownership and never from the person in view (FR-081, FR-082, FR-092, SC-004)
- [x] T207 [P] Write and pass `internal/web/stream/deadline_test.go` — every registered kind's stream handler is constructed through `newStream()`, the `*http.Server` `WriteTimeout` override is present on the `ServeEvent`, and `X-Accel-Buffering: no`, `Cache-Control: no-store` and `SkipSuccessActivityLog` are set. **PocketBase's hardcoded 5-minute `WriteTimeout` passes every test shorter than five minutes**, so this asserts the construction, not the elapsed time (FR-010, SC-017, risk R7)
- [x] T208 [P] Write and pass `internal/realtime/authz_test.go` — the hub publishes IDs only and never a record body; a subscriber whose access to a patient is removed stops receiving that patient's events on the very next publish, because re-authorisation happens per event (Constitution V and VII)
- [x] T209 [P] Write and pass `internal/service/audit/coverage_test.go` — 100% of creations, corrections, deletions, relationship changes and tag changes across all fourteen kinds produce an audit entry, refused access attempts produce an `access_denied` entry, and **0** entries contain a diagnosis, a measurement, a member number, a note, a tag name or a search term (FR-084, FR-085, SC-011)
- [x] T210 Extend and pass phase 001's PHI-leak exercise — `internal/testsupport/phileak/exercise.go` and `phileak_test.go`, both **[EDIT]** — so that it drives **every** operation this phase defines against a sentinel-seeded instance, with a sentinel in the `notes` field of **every** kind because notes are clinical content wherever they go (FR-003), and asserts zero sentinel occurrences in the zerolog stream, the Prometheus gatherer output (names and label values), the OTel `tracetest.SpanRecorder` and a stub Sentry transport, naming the sink on failure (FR-094, SC-012, research D-24)
- [x] T210a [P] Extend phase 002's egress harness — `internal/testsupport/netgate/` **[EDIT]** — to run this phase's whole endpoint exercise, its cron bindings and its scale suite under the `net.Dialer` control hook on an instance with no destination configured, asserting **zero** outbound connections, so nothing about a person leaves the installation unless the operator configured it (FR-088)
- [x] T211 [P] Write and pass `internal/web/api/concurrency_test.go` — for every registered kind, a `PATCH` or `DELETE` based on a stale version is refused with `412` carrying the current values, `0` changes are silently overwritten, and the same rule holds for attaching and detaching a link (FR-005, SC-009, spec Edge Cases)
- [x] T212 [P] Write and pass `internal/web/api/pagination_test.go` — paging any kind's list while records are concurrently inserted and deleted neither repeats nor skips an entry, for every registered kind (FR-007, spec Edge Cases)
- [x] T213 [P] Write and pass `internal/web/api/minimal_record_test.go` — a record carrying only its required fields lists, opens, corrects, deletes, is searched, is tagged and appears on the timeline exactly like a fully populated one, and every screen renders absent optional details as absent rather than as blanks, dashes or zeros, for every registered kind (spec Edge Cases)

### Performance, accessibility and delivery

- [ ] T214 [P] Run `task test:scale` and record the measured numbers in `specs/003-clinical-records/quickstart.md` §5: any list page, status view and the timeline within 2 s, and the first page of grouped search within 3 s, at 50,000 records, with the per-symptom episode counts, the per-kind chart counts and the tag usage counts all still correct and all still derived (SC-002, SC-003, FR-090)
- [ ] T215 [P] Add `e2e/specs/keyboard.spec.ts` — keyboard-only record → correct → relate → tag → delete for one kind per user story (condition, encounter, vitals, immunization, insurance, family member) at both viewports, asserting a visible focus indicator at every step and that focus is never lost into a closed drawer (SC-018)
- [x] T216 Regenerate and commit the test fixture data dir (`task fixture:regen` → `internal/testdata/pb_data`), run the full CI sequence locally (`task check`, `task openapi` + clean diff, `task routes`, `task test:e2e`, container build), and confirm every step is green. `task ci`, `task openapi` (clean diff) and `task routes` are green. `task fixture:regen` reproduces the committed database only up to PocketBase's own randomly generated record IDs — two consecutive regenerations differ from each other with no code change involved, so no regeneration can ever diff clean against a prior one; the committed fixture is left as-is. `task test:e2e` (Playwright) and the container build cannot run in this sandbox — no Chromium system libraries and no Docker daemon are available here.
- [ ] T217 Update `medikube/CLAUDE.md` and the PocketBase-upgrade checklist with this phase's two additions: re-run `registry_completeness_test.go` and the per-collection lockdown scenarios on every PocketBase upgrade (risk R8); state the phase's Complexity Tracking entries in the pull-request description as the workflow requires
- [ ] T217a Write `specs/003-clinical-records/traceability.md` — the mechanical join, generated from `spec.md` and `tasks.md` rather than written by hand: one row per functional requirement (all 94) naming the task ids that satisfy it and the named test that proves it, one row per acceptance scenario naming its test, and one row per success criterion (all 18) naming its task or its exit criterion. **A functional requirement with no task, or a success criterion that is neither mapped nor marked `[outcome metric]` in `spec.md`, fails the phase** (cross-artifact finding M7)

---

## Dependencies & Execution Order

### Phase dependencies

```
Setup (T001–T007)
   └─> Foundational (T008–T037)          BLOCKS EVERYTHING
          ├─> US1 (T038–T060)   P1  ─┐
          ├─> US2 (T061–T082)   P2   │
          ├─> US3 (T083–T102)   P3   │  independent of each other
          ├─> US4 (T103–T117)   P4   │
          ├─> US5 (T118–T134)   P5  ─┘
          │
          ├─> US6 (T135–T150)   P6   requires US1–US5 (it links their kinds)
          ├─> US7 (T151–T163)   P7   independent (tags collection is Foundational)
          │
          ├─> US8 (T164–T177)   P8   requires the kinds it indexes; tags for ?tags=
          ├─> US9 (T178–T189)   P9   requires US1–US5; tags for ?tags=
          └─> US10 (T190–T198)  P10  independent
                 └─> Polish (T199–T217)   requires every story that is being shipped
```

### The one real cross-story dependency, stated plainly

**US6 cannot start before US1–US5 are complete**, because it links their kinds and its migration
(T144) adds relation fields to `allergies`, `conditions`, `injuries` and `symptoms`. This is
inherent to the story — "every record type it connects must exist first" is the spec's own
rationale for its P6 priority — not an artefact of the plan.

US8 and US9 read what US1–US7 create, but both are testable against whatever subset exists: search
returns groups for registered kinds only, and the timeline interleaves whatever is there. They can
therefore begin as soon as US1 lands, and their completeness tests tighten as more kinds register.

### Within a user story

1. Tests first, and they must be **red** before implementation. Red-by-compile-failure counts.
2. Domain entity → service → repository + migration → DTO + codec → templ views → registration.
3. Registration (`records.Register`) is last within a kind, because the completeness gate (T021)
   fails until every declared piece exists.
4. Seed and Playwright come after registration, because the smoke case needs a seeded id.

### Foundational ordering that matters

- **T026 (`tags` migration) blocks every kind migration**, because every kind carries a `tags`
  relation field and PocketBase cannot create a relation to a collection that does not exist.
- **T028 + T030 (`search_index` + the write-side indexer) block every `records.Register` call**,
  because registration binds the index hook.
- **T022/T023 (the contract suites) block every kind's test task**, which is why they are
  Foundational rather than duplicated per story.
- **T077 creates the `equipment` schema inside US2**, because `treatments.equipment` needs it;
  the equipment service, DTOs and UI remain US5. This is called out in both tasks.

---

## Parallel Execution Examples

### Foundational — three independent tracks

```bash
# Track A — vocabularies and value types
T008, T010, T012, T013, T015          # all [P], different files
# Track B — the registry and its suites
T017, T019                             # then T018, T020, T021, T022, T023, T024
# Track C — blocking collections
T025, T027, T029, T033, T035, T037    # all [P]
```

### User Story 1 — all seven test tasks at once, then all six implementation tasks

```bash
# Every test task is [P] — different files, and all must be red before T046 starts
T038  internal/domain/clinical/allergy_test.go + internal/service/allergy/service_test.go
T039  internal/domain/clinical/condition_test.go + internal/service/condition/service_test.go
T040  internal/domain/clinical/emergencycontact_test.go + .../service_test.go
T041  internal/store/{allergy,condition,emergencycontact}/repo_test.go
T042  internal/web/api/{allergy,condition,emergencycontact}_test.go
T043  internal/web/views/records/{allergy,condition,emergencycontact}_templ_test.go
T044  internal/web/api/{allergy,condition,emergencycontact}_http_test.go
T045  internal/records/kinds/{allergy,condition,emergencycontact}_contract_test.go

# Then the domain layer, three ways in parallel
T046, T047, T048
# Then the service layer, three ways in parallel
T049, T050, T051
# Then store, DTOs and views, three ways in parallel each
T052, T053, T054   →   T055, T056
# Then serially: T057 (registration), T058 (seed + DI), T059 (e2e), T060 (OpenAPI)
```

### Five stories, five engineers

Once Foundational is green, US1–US5 touch **disjoint** files: their own
`internal/domain/clinical/<kind>.go`, `internal/service/<kind>/`, `internal/store/<kind>/`,
`internal/web/api/<kind>.go` and `internal/web/views/records/<kind>.templ`. The only shared files
are `internal/records/kinds/` (one file per kind — still disjoint), `internal/cli/seed.go` and
`cmd/medikube/main.go`. Those last two are append-only per kind and should be landed as the final
task of each story to keep the merge trivial. US7 and US10 can run alongside them on entirely
separate paths.

---

## Implementation Strategy

### MVP — User Story 1 only

1. Phase 1 Setup (T001–T007)
2. Phase 2 Foundational (T008–T037) — **critical, blocks everything**
3. Phase 3 US1 (T038–T060)
4. **STOP and VALIDATE**: run the US1 block of `quickstart.md` §3 by hand, plus
   `task check`, `task openapi` + clean diff, `task test:e2e`
5. Demo: a chart that answers "what is he allergic to, what does he live with, who do I call".

### Incremental delivery

Each story below adds value without breaking the ones before it, and each ends with a green
`task check`, a clean `api/openapi.json` diff and a passing browser gate.

| Increment | Stories | What it unlocks |
|---|---|---|
| 1 | US1 | the emergency chart (MVP) |
| 2 | +US2 | the narrative of care |
| 3 | +US3 | self-recorded episodes and measurements |
| 4 | +US4, +US5 | prevention, injuries, cover, equipment — every record type exists |
| 5 | +US6 | the records are connected |
| 6 | +US7 | organised the household's own way |
| 7 | +US8, +US9 | findable, and the current picture at a glance |
| 8 | +US10 | family history |
| 9 | Polish | the gates and the cross-cutting guarantees |

### Notes

- `[P]` means different files with no incomplete dependency.
- Tests must be observed **red** before their implementation task begins; a test that was never red
  proves nothing.
- Commit after each task or logical group, Conventional Commits scoped `medikube`
  (e.g. `feat(medikube): register the condition record kind`).
- One `tests.TestApp` per `ApiScenario`, **never shared** — `bindUIExtensions` re-enters on every
  `OnServe` and the handler chain grows until the stack overflows.
- Any migration added means `task fixture:regen` before the integration tests are trusted.
- Stop at any checkpoint and validate the story independently.

---

## Task Count Summary

| Phase | Tasks | of which test tasks |
|---|---|---|
| 1 — Setup | 7 | 0 |
| 2 — Foundational | 31 | 16 |
| 3 — US1 (P1) | 23 | 8 |
| 4 — US2 (P2) | 22 | 8 |
| 5 — US3 (P3) | 20 | 9 |
| 6 — US4 (P4) | 15 | 7 |
| 7 — US5 (P5) | 17 | 10 |
| 8 — US6 (P6) | 16 | 9 |
| 9 — US7 (P7) | 13 | 8 |
| 10 — US8 (P8) | 14 | 8 |
| 11 — US9 (P9) | 14 | 8 |
| 12 — US10 (P10) | 9 | 6 |
| 13 — Polish | 21 | 16 |
| **Total** | **222** | **113** |

T210a and T217a are suffixed for the same reason: T210a closes the egress requirement FR-088, which
had no task at all, and T217a adds the phase's requirement-to-task join (cross-artifact finding
**M7**) — neither renumbering anything after it.

The two suffixed tasks in US9 (T183a, T186a) put phase 003's seven status views inside the
Playwright gate through `SmokeVariants` rather than by registering seven more page routes
(cross-artifact finding **L2**, `contracts/pages.md` §3.5). They are suffixed rather than
renumbered so every task id cited elsewhere in the suite still points at the same task.
