# Implementation Plan: Patient Core

**Branch**: `002-patient-core` | **Date**: 2026-08-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-patient-core/spec.md`

**Constitution**: [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.3.0 (binding)

## Summary

Phase 001 delivered a working instance in which an *account* owns its medications directly.
Phase 002 introduces the entity that every later phase hangs off: the **patient** — a person
whose health is being recorded, who may or may not be the account holder. It gives an account
many patients, re-anchors phase 001's medications from `medications.owner → users` to
`medications.patient → patients`, adds a non-authoritative "person in view" pointer, a per-patient
chart summary, and the account-scoped directories of practitioners and places of care that
phase 001 explicitly deferred.

Technically this is: three new PocketBase collections (`facilities`, `practitioners`, `patients`),
three collection amendments (`users.active_patient`, `audit_events.patient`, `medications`
re-scoped and given `practitioner`/`pharmacy`), one irreversible-in-practice data migration that
provisions a self-record patient per existing account and re-attributes every existing medication
to it, 20 new `/api/v1` operations, 6 new pages plus a patient switcher welded into the shell, and
a single new authorization anchor — **patient ownership** — which becomes the one fact every
read and write in the application is decided against.

The load-bearing design commitments, all verified against PocketBase v0.40.1 source rather than
documentation:

1. **Ownership is a relation, and PocketBase's own delete semantics implement most of the spec.**
   `core/record_model.go:1611` deletes a referencing record only when the relation is
   `CascadeDelete`; otherwise it *unsets the id and re-saves* (`:1618-1626`), and it *fails the
   delete* when the emptied relation is `Required` (`:1619`). That one function gives us FR-026
   (deleting a patient destroys their records), FR-040 (deleting a practitioner clears the
   reference and preserves the record) and FR-017/FR-052 (deleting the patient in view clears
   `users.active_patient`) with no MediGo code at all. The plan's job is to get the
   `Required`/`CascadeDelete` matrix right and then *test* it, not to reimplement it.
2. **The person in view is never consulted for authorization.** `users.active_patient` exists to
   pre-fill a switcher and to redirect `/medications` to `/medications?patient=…`. Every API
   handler takes its patient from the request. This is FR-015 and constitution VII, and it is
   what stops MediGo growing upstream's second, implicit, un-authorized addressing scheme.
3. **The photograph never leaves through PocketBase's file route.** `patients.photo` is
   `Protected: true` (asserted at boot), thumbnails are generated eagerly into PocketBase's own
   `thumbs_<filename>/<size>_<filename>` layout so PB's replace/delete cleanup still finds them,
   and bytes are served only from `GET /api/v1/patients/{id}/photo` after the service authorizes.
   No file token, ever.
4. **The re-attribution runs in one transaction with everything else.**
   `core/migrations_runner.go:129-131` wraps *all* pending migrations in a single
   `AuxRunInTransaction(RunInTransaction(...))`, so the six migrations this phase adds either all
   land or none do. There is no half-migrated state to design for.

## Technical Context

**Language/Version**: Go **1.27** (toolchain `go1.27.x` pinned in `go.mod`). Not the monorepo's
1.26.5 house standard — PocketBase v0.40.1's `go.mod` declares `go 1.27` and 67 non-test files
import the 1.27 stdlib package `encoding/json/v2`, 15 of them under `core/` and `apis/`
(VERIFIED-SOURCE-FACTS FACT 0). CI MUST NOT set `GOTOOLCHAIN=local`.

**Primary Dependencies** (versions fixed by the constitution's Technology Constraints and pinned
to arc-ui's where they overlap, per HOUSE-PATTERNS):

| Module | Version | Used in this phase for |
|---|---|---|
| `github.com/pocketbase/pocketbase` | v0.40.1 | collections, migrations, relation cascade/unset, file storage + `CreateThumb`, router, test harness |
| `github.com/a-h/templ` | v0.3.1020 | 6 new pages, the patient switcher, chart header, empty states |
| `github.com/starfederation/datastar-go` | v1.2.2 | switcher patch, create/edit drawers, list live-update |
| `github.com/caarlos0/env/v11` | v11.4.1 | `MEDIGO_FILES_*` photo limits |
| `github.com/rs/zerolog` | v1.35.1 | the only logger; redacted patient/practitioner marshallers |
| `github.com/getsentry/sentry-go` | v0.48.0 | scrubbed error reporting (already wired in 001) |
| `github.com/prometheus/client_golang` | as 001 | `medigo_records_*`, `medigo_files_*` counters |
| `go.opentelemetry.io/otel` | as 001 | spans on the new services and stores |
| `github.com/samber/do` | as 001 | container providers for the three new services |
| `github.com/samber/lo` | v1.53.0 | sparingly |
| `github.com/stretchr/testify` | v1.12.0 | the only assertion library |
| `modernc.org/sqlite` | v1.57.0 (transitive) | pure-Go SQLite; `CGO_ENABLED=0` holds |
| Playwright CLI | build-time only | 6 new smoke targets × 2 viewports |

**Storage**: Embedded SQLite through PocketBase (`modernc.org/sqlite`, no cgo), data dir
`MEDIGO_DATA_DIR` (`/data/pb_data`). Schema is code: reversible Go migrations registered into
`core.AppMigrations`. Patient photographs live in PocketBase's local file storage under
`<collectionId>/<recordId>/`, thumbnails under `.../thumbs_<filename>/`.

**Testing**: `stretchr/testify` (`require` for preconditions, `assert` for independent
assertions), table-driven `t.Run` subtests. Five mandatory layers per constitution III:
unit against hand-written fakes with `t.Parallel()`; integration against `tests.NewTestApp`
(a filesystem clone of `internal/testdata/pb_data` into a temp dir, so it is `t.Parallel()`-safe
— VERIFIED-SOURCE-FACTS FACT 7); contract suites run against every implementation of an interface
including the fakes; HTTP-level `tests.ApiScenario` (never sharing a `TestApp` across scenarios);
templ render-to-buffer tests. Plus the Playwright smoke + console-error gate at 1440×900 and
390×844, with its route list derived from `medigo routes`.

**Target Platform**: Linux server, single static binary from
`gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, built for linux/amd64 and
linux/arm64 through the monorepo's shared `build-image.yaml`. Browser target: current desktop and
mobile browsers with scripting enabled.

**Project Type**: Server-rendered Go web application with an embedded framework (PocketBase),
hypermedia interactivity (templ + Datastar), and a hand-written JSON API. One module, one binary,
no separate frontend build service, no Node at runtime.

**Performance Goals**:
- SC-004: patient chart summary — header, per-kind counts, recent activity — within **2 s** for a
  patient holding 50,000 records. Budget: one indexed `COUNT(*)` per registered kind (one kind
  today, fifteen by phase 004) plus one `LIMIT 10` read of `audit_events` on
  `(patient, occurred_at)`.
- SC-002: switching the person in view renders the newly chosen person within **1 s** in no more
  than two interactions.
- SC-011: an account with 25 patients can find and choose a named one within **10 s** — the
  switcher filters client-side over an already-loaded list of ≤100.
- Photo upload: a 15 MiB JPEG accepted, sniffed, stored and two thumbnails generated within the
  request; thumbnail generation happens in a `TxInfo().OnComplete` callback so it is outside the
  write transaction but inside the request.

**Constraints**:
- Single instance by construction; no horizontal scaling, no broker abstraction over the realtime
  hub (constitution Technology Constraints).
- `CGO_ENABLED=0`; nothing in this phase may add a cgo dependency (relevant: image decoding is
  `golang.org/x/image` via PocketBase's `imaging`, which is pure Go).
- `script-src 'unsafe-eval'` is accepted and permanent; every other CSP directive stays strict, so
  the switcher and the photo `<img>` must be same-origin and inline-script-free.
- Records are hard deleted. **No `deleted_at` on `patients`, `practitioners` or `facilities`.**
- No PHI in logs, metrics, traces or Sentry. `patient_id` as an opaque 15-char id is permitted;
  a name, a date of birth, an address or a file name is not.
- Go 1.27 `encoding/json/v2` retrofit semantics apply to every new DTO: slices marshal as `[]`
  and never `null`, unknown fields are rejected, duplicate keys are rejected.

**Scale/Scope**: A self-hosted household instance. Design points: ≤25 patients per account
(SC-011), 50,000 records on one patient (SC-004), a few hundred directory entries per account.
This phase adds 3 collections, 3 collection amendments, 20 API operations, 6 pages, 1 shell
component, 3 service packages, 3 store packages and 6 migrations.

**No NEEDS CLARIFICATION remain.** Every open question raised by the specification is resolved in
[research.md](./research.md) with a decision, a rationale and the alternatives rejected.

## Constitution Check

*GATE: evaluated before Phase 0 and re-evaluated after Phase 1 design. Both passes recorded.*

### I. Simplicity Is A Gate (KISS) — **PASS (with one recorded entry)**

- The specification asks for two directories (practitioners, places of care) plus a specialty
  vocabulary. MediGo ships **one** `facilities` collection with a `kind` discriminator and a
  **fixed Go enum** for specialty rather than a fourth collection — the two cuts SHARED-DESIGN
  §1.1 rows 3 and 5 already justified, and FR-033/FR-034 explicitly bless.
- No caching layer, no counter table, no materialised chart. FR-028's per-kind counts are
  `COUNT(*)` over an index; if measurement ever shows that is too slow, phase 006 owns the fix.
  Building it now would be speculative.
- No `deleted_at`, no undo, no transfer-of-ownership operation, no `/patients/{id}/new` routes.
- **Every abstraction introduced has ≥2 implementations before it lands**: each consumer-declared
  interface ships with its real implementation *and* a fake used by the unit tests, and both run
  the same contract suite (Principle II's Liskov clause).
- One entry goes to Complexity Tracking: the re-attribution backfill issues a raw `dbx` UPDATE
  rather than going through the repository. See the table below.
- **Conflict with Principle II, resolved explicitly here** as Principle I requires: Principle II
  would have `internal/web/api` declare one `PatientService` interface of ten methods, which
  Principle II's own interface-segregation clause caps at five. Rather than justify a ten-method
  interface, the surface is split into four consumer-declared interfaces — `PatientService` (5),
  `PatientPhotoService` (3), `PatientChartReader` (1), `ActivePatientWriter` (1). Simplicity is
  served by the split, not by the omnibus.

### II. Interfaces At Every Seam (SOLID) — **PASS**

- **Single responsibility**: `internal/web/api/patients.go` parses and renders; `internal/service/
  patient` decides; `internal/store/patient` persists. Photo *sniffing and thumbnailing* live in
  the store adapter because they are PocketBase filesystem concerns; the *policy* (which sizes,
  which MIME types, who may) lives in the service.
- **Open/closed**: the chart summary does not switch on record kind. It consumes
  `patient.RecordCounter`, whose sole implementation walks `internal/records`' registry. Phase 003
  registering eleven more kinds changes zero lines in `internal/service/patient`.
- **Liskov**: `patienttest.RepositoryContract`, `practitionertest.RepositoryContract` and
  `facilitytest.RepositoryContract` each run against the PocketBase implementation *and* the fake.
- **Interface segregation**: every interface this phase introduces is 1–4 methods. The largest is
  `patient.Repository` at 4 (`Get`, `List`, `Save`, `Delete`). No omnibus `Store` or `Service`.
- **Dependency inversion**: `internal/domain/person` and `internal/domain/directory` import only
  stdlib. `internal/service/**` imports neither PocketBase, nor `net/http`, nor templ. Enforced by
  the `depguard` rule in `.golangci.yml`, extended in this phase's setup tasks to cover the new
  package globs — a build gate, not a convention.
- Constructors take interfaces and return concrete types; wiring happens once in `internal/di`.

### III. Test-First With testify (NON-NEGOTIABLE) — **PASS**

- The specification asks for it twice (FR-054, FR-055) and the success criteria repeat it
  (SC-012). Every one of the **38 acceptance scenarios** across the six user stories maps to a
  named test in `tasks.md`, and every test task is sequenced *before* the implementation task it
  covers.
- All five pyramid layers are present and mandatory, and `tasks.md` names the file for each.
- **Authorization is first-class**: every one of the 20 new operations gets a "a second account is
  refused, indistinguishably from not-found" test. Because sharing does not exist until phase 005,
  the third case the constitution asks for ("a revoked grantee is refused") is written now as a
  *placeholder-free* two-case suite and the third case is added by phase 005 to the same table —
  the suite is table-driven precisely so that costs one row.
- The cascade/unset matrix (§Foundational) is tested directly against a real `tests.NewTestApp`,
  because it is behaviour we are *relying on PocketBase for* rather than behaviour we wrote.

### IV. Idiomatic Go Over Clever Go — **PASS**

- Errors are values, wrapped with `%w`, inspected with `errors.Is`/`errors.As`, mapped once in
  `internal/web`. `samber/mo` and `samber/ro` are absent and stay absent.
- `Patch` structs carry absent-vs-null with plain pointers (`*string`, `**person.Date`).
- `context.Context` first on every I/O function, honoured.
- Eager thumbnail generation runs in a `TxInfo().OnComplete` callback — an owned, bounded,
  request-scoped goroutine-free path — not a fire-and-forget `go func()`.
- Generated `*_templ.go` is committed, marked generated, excluded from lint and coverage.

### V. PocketBase Is The Platform, Not A Detail — **PASS**

- Nothing PocketBase provides is reimplemented. The relation cascade/unset semantics, the file
  storage, `CreateThumb`, the MIME sniffing (`gabriel-vasile/mimetype.DetectReader`, verified at
  `core/validators/file.go:76` — content, never the client's declared type, satisfying FR-008),
  the migration runner's single-transaction guarantee and the admin UI are all used as-is.
- All five API rules on all three new collections are `nil`; the boot assertion is extended to
  cover them; a `tests.ApiScenario` per new collection proves
  `GET /api/collections/patients/records` returns 404 to an ordinary user.
- Schema is code: six reversible migrations, none applied by clicking.
- The patient delete and the medication re-attribution both run in `app.RunInTransaction`.
- `OnRecord*Request` hooks are not used anywhere in this phase — they never fire under the
  lockdown. The audit hooks are `OnRecordAfterCreateSuccess` / `…UpdateSuccess` / `…DeleteSuccess`.
- Realtime stays MediGo's: `/api/v1/streams/records` becomes patient-scoped and re-authorises the
  **patient** per subscriber per event. PocketBase's native realtime remains unused.

### VI. One Log Stream, One Trace Context — **PASS**

- zerolog only. `app.Logger()` is banned by `forbidigo`; the ban list is extended to the new
  packages in setup.
- `person.Patient`, `directory.Practitioner` and `directory.Facility` each implement
  `MarshalZerologObject` emitting **only** the id and, for the patient, the owner id — so logging
  one by accident cannot leak a name or a date of birth.
- New metrics: `medigo_records_total{kind}`, `medigo_files_photo_bytes`,
  `medigo_files_thumb_duration_seconds{size}`, `medigo_patients_switch_total{outcome}`. Label sets
  are bounded and contain no identifier.
- Spans: `service.patient.*`, `store.patients.*`, `store.practitioners.*`, `store.facilities.*`.
  Attributes from the allowlist only; `medigo.patient_id` is **not** on the allowlist.
- Datastar's `ConsoleLog`/`ConsoleError` are not used; the whole inline-script SDK family stays
  banned (it would fail both the CSP and the Principle VIII console gate).

### VII. Patient Privacy Is Structural, Not Procedural — **PASS**

This is the phase where this principle earns its keep, so it is enumerated rather than asserted.

| Requirement | How it is structural |
|---|---|
| FR-041 authorize from the data, never the caller | One checkpoint: `access.Authorizer.Patient(ctx, actor, patientID, need)`. Every service method's *first act*. The repository never authorizes; the handler never authorizes. |
| FR-042 unreachable is indistinguishable from non-existent | The error taxonomy maps **every authorization failure on patient-scoped data** to `ErrNotFound` → 404 with the identical envelope a genuinely missing id produces. A response-body-equality test asserts the two are byte-identical. |
| FR-044 no self-authorizing photo link | `Protected: true` + boot assertion + MediGo-owned route. PocketBase's file-token mechanism is never called. |
| FR-045 audit everything, content never | `audit_events` gains `patient`. Person create/update/delete, photo set/delete, active-patient change and **every refused access** (as `access_denied`) write a row of (actor, action, target_kind, target_id, patient, request_id, occurred_at) and nothing else — there is no `ip` column and no content column (001 research D-19). |
| FR-046 no PHI in diagnostics | Redacting `MarshalZerologObject` on all three new domain types; allowlisted span attributes; bounded metric labels; a test that greps the captured log stream after exercising every endpoint. |
| FR-008 trust content, not the client | PocketBase sniffs with `mimetype.DetectReader`. **Its rejection message embeds the original filename** (`core/validators/file.go:63`) — a PHI leak — so MediGo maps PB validation errors into its own PHI-free envelope and never echoes PB's message. |
| Hard delete | No `deleted_at`. Deleting a patient cascades through `medications.patient` in one transaction and destroys the photo with the record. |

### VIII. The UI Must Prove It Renders — **PASS**

- 6 new pages (`/patients`, `/patients/{id}`, `/practitioners`, `/practitioners/{id}`,
  `/facilities`, `/facilities/{id}`), each declaring its landmark in the route registry, each
  therefore automatically in the Playwright gate at both viewports.
- The patient switcher is added to the shell, so **every existing page's smoke case now also
  exercises it** — a switcher that throws breaks all of them, which is the desired blast radius.
- Every new page ships an `@EmptyState` inside its own landmark so the landmark assertion holds on
  a freshly seeded instance (SC-013 explicitly requires several of these screens to be empty).
- The seed set is extended with a second account, three patients on the first, a practitioner, a
  facility and medications on two patients, so authenticated and isolation paths are exercised.

### IX. Compliance Is A Build Gate — **PASS**

- The 20 new operations enter one declarative route table that simultaneously registers them,
  emits `medigo routes` and drives OpenAPI generation, so registry, document and smoke coverage
  cannot drift. `api/openapi.json` is regenerated, committed and diffed in CI.
- All six migrations supply a real `down`. The one whose reversal is lossy (dropping `patients`
  discards profile detail entered after the migration) carries that statement in the migration
  file itself, which is exactly the escape Principle IX names.
- `depguard` and `forbidigo` globs are extended to the new packages in the setup phase, so the
  Principle II and VI boundaries are enforced by CI on this phase's code from its first commit.

### Post-Design Re-Check (after Phase 1 artefacts)

Re-evaluated against `data-model.md`, `contracts/` and `quickstart.md`: **all nine still pass.**
Two things changed during design and are recorded here rather than buried:

1. `patients.first_name`, `patients.last_name` and `patients.birth_date` are `Required: false`
   **in the collection** while being required in every request DTO. This deliberately makes the
   storage constraint weaker than the API contract, which §6.2 of the shared design calls "the
   last line of defence". It is the only way FR-005's automatic provisioning can succeed for an
   account that has only a display name. Recorded in Complexity Tracking.
2. The chart summary's recent-activity list reads `audit_events`, which phase 001 owns. Rather
   than a second read path, `internal/service/patient` declares a 1-method
   `RecentActivityReader` port implemented by `internal/service/audit`. No new collection, no
   duplication, and phase 006's audit reader consumes the same store.

## Project Structure

### Documentation (this feature)

```text
specs/002-patient-core/
├── plan.md                       # This file
├── research.md                   # Phase 0: 21 decisions with rationale + alternatives
├── data-model.md                 # Phase 1: collections, fields, enums, indexes, migrations
├── quickstart.md                 # Phase 1: run it and verify it by hand, end to end
├── contracts/
│   ├── patients.md               # 5 ops: list/create/read/update/delete
│   ├── patient-photo.md          # 3 ops: put/get/delete photo
│   ├── patient-chart.md          # 1 op: chart summary (header, counts, recent activity)
│   ├── active-patient.md         # 1 op + the /api/v1/me DTO changes
│   ├── practitioners.md          # 5 ops
│   ├── facilities.md             # 5 ops
│   ├── medications-rescope.md    # changes to phase 001's medication operations
│   └── pages.md                  # 6 pages, their landmarks, the shell change and the smoke assertions
├── checklists/
│   └── requirements.md           # pre-existing
└── tasks.md                      # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root: `medigo/`)

New files are marked `+`, amended files `~`. Everything else already exists from phase 001.

```text
cmd/medigo/
~ main.go                                     register 3 new services + 6 migrations

internal/config/
~ config.go                                   + FilesConfig{PhotoMaxBytes, PhotoMimeTypes, PhotoThumbs}

internal/domain/
+ person/patient.go                           Patient entity, Date, redacting marshallers
+ person/enums.go                             Sex, BloodType, RelationshipToOwner (+ Valid())
+ person/age.go                               AgeAt(birth, on) — derived, never stored (FR-006)
+ person/measure.go                           Height/Mass canonical SI + imperial formatting (FR-007)
+ person/validate.go                          Validate() -> *domain.ValidationError, all fields at once
+ directory/practitioner.go                   Practitioner entity + redacting marshaller
+ directory/facility.go                       Facility entity + redacting marshaller
+ directory/enums.go                          FacilityKind, Specialty (42 values incl. `other`)
~ access/actor.go                             + PatientRef, PermOwn resolution inputs

internal/records/
~ registry.go                                 + Kinds() and CountByPatient dispatch for the chart

internal/service/patient/
+ service.go                                  List/Get/Create/Update/Delete
+ photo.go                                    SetPhoto/GetPhoto/DeletePhoto
+ chart.go                                    Summary (header + counts + recent activity)
+ active.go                                   SetActivePatient / ResolveActivePatient
+ ports.go                                    Repository, PhotoStore, RecordCounter,
+                                             RecentActivityReader, Authorizer, Auditor
+ patienttest/fake.go                         in-memory fakes for every port
+ patienttest/contract.go                     shared Repository + PhotoStore contract suites
+ *_test.go                                   unit tests against the fakes, t.Parallel()

internal/service/practitioner/                same shape: service.go, ports.go,
+                                             practitionertest/{fake,contract}.go, *_test.go
internal/service/facility/                    same shape

internal/service/access/
~ authorizer.go                               patient ownership resolution + audited refusals
~ authorizer_test.go                          owner allowed / stranger 404 / anonymous 401

internal/service/audit/
~ writer.go                                   + Patient field on Event
+ recent.go                                   RecentForPatient(ctx, patientID, limit)

internal/service/medication/
~ service.go                                  patient-scoped List; patient fixed at Create
~ ports.go                                    Query gains PatientID; Create refuses empty patient

internal/store/
+ patient/repo.go                             *core.Record <-> person.Patient
+ patient/photo.go                            multipart -> filesystem.File, eager thumbs
+ patient/repo_integration_test.go            runs patienttest.RepositoryContract
+ practitioner/repo.go  + repo_integration_test.go
+ facility/repo.go      + repo_integration_test.go
~ medication/repo.go                          patient filter, practitioner/pharmacy relations
~ migrations/1756200100_facilities.go
~ migrations/1756200200_practitioners.go
~ migrations/1756200300_patients.go
~ migrations/1756200400_users_active_patient.go
~ migrations/1756200500_audit_events_patient.go
~ migrations/1756200600_medications_repoint.go     the backfill; lossy `down` documented in file
~ migrations/assertions.go                    nil-rule, Protected:true, CascadeDelete matrix

internal/platform/pb/
~ boot.go                                     boot assertions extended to the 3 new collections
~ hooks.go                                    audit hooks for patients/practitioners/facilities

internal/httproute/
~ routes.go                                   20 API + 6 page entries, each with landmark+smokeUrl

internal/web/api/
+ patients.go        + patients_test.go       5 ops, DTOs, ETag/If-Match
+ patient_photo.go   + patient_photo_test.go  3 ops, multipart, streaming
+ patient_chart.go   + patient_chart_test.go  1 op
+ practitioners.go   + practitioners_test.go  5 ops
+ facilities.go      + facilities_test.go     5 ops
~ me.go                                       + active_patient, patient counts; PUT active-patient
~ dto.go                                      PatientSummary/Patient/Create/Patch, Practitioner*,
~                                             Facility*, PatientChart, Display block

internal/web/page/
+ patients.go                                 /patients, /patients/{id}
+ practitioners.go                            /practitioners, /practitioners/{id}
+ facilities.go                               /facilities, /facilities/{id}
~ shell.go                                    NavState carries patients + current patient

internal/web/stream/
~ records.go                                  ?patient= required; per-event patient re-authorisation

internal/web/views/
+ patients/list.templ  + list_test.go
+ patients/detail.templ (chart header, counts, recent activity) + detail_test.go
+ patients/form.templ                         create/edit drawer
+ patients/photo.templ                        upload + preview + remove
+ directory/practitioner_list.templ / _detail.templ / _form.templ
+ directory/facility_list.templ / _detail.templ / _form.templ
+ shell/patient_switcher.templ + patient_switcher_test.go
~ shell/layout.templ                          switcher mounted inside #primary-nav
~ ids/ids.go                                  PatientRow, PatientSwitcher, PatientChart, DirectoryRow

internal/cli/
~ seed.go                                     2 accounts, 3+1 patients, 1 practitioner,
~                                             1 facility, medications on two patients

internal/testsupport/
~ fixtures.go                                 seeded ids as exported constants
+ authz.go                                    ownerAllowedStrangerRefused table-driven helper

internal/testdata/pb_data/                    fixture rebuilt with the new collections

e2e/
~ smoke.spec.ts                               unchanged — targets come from `medigo routes`
+ patient-switch.spec.ts                      the one genuinely stateful browser flow

api/openapi.json                              regenerated, committed, diffed
```

**Structure Decision**: this is the package layout mandated by SHARED-DESIGN §4, unchanged. The
phase adds one domain subpackage per bounded concept (`person`, `directory`), one service package
per aggregate (`patient`, `practitioner`, `facility`), one store package per repository, and keeps
every PocketBase import inside the ten `[PB]`-marked packages. The only structural addition to the
contract is `internal/service/patient/patienttest` (and its two siblings), which is the standard
`<pkg>test` convention SHARED-DESIGN §5.2 already establishes for `medicationtest`.

## Deviations from the shared design contract

The contract is binding on **design**. Its §0 phase table is not the allocation the six written
specifications actually use, and phases 001 and 003–006 each record their departures in a table
like this one. This phase's were, until now, the only ones undocumented — cross-artifact finding
**H6** — although this phase deviates the most. **No design decision in the contract is changed;
what changes is which phase owns a piece, one operation's existence, and one envelope rule.**

| Contract says | This phase does | Why |
|---|---|---|
| Patients, multi-patient switching and the patient chart are **phase 001**, and phase 001 is the `foundation` phase | They land **here**, and phase 001 ships accounts and medications instead | Phase 001's charter governs, and its own plan records the mirror-image decision: *"Phase 001 owns accounts and medications as its single kind. Patients arrive in 002."* This is the single largest relocation in the suite, and it is why this phase exists at all. The design — `patients` as the ownership anchor, `users.active_patient` as a non-authoritative pointer, the photo through MediGo's own route — is the contract's, verbatim. |
| Phase 002 is 19 operations | **20** — the contract's 19 plus `getPatientChart`, `GET /api/v1/patients/{id}/summary` (operation **93** in the contract's additive numbering) | FR-029 and FR-030 require the chart header, the per-kind tile counts and the figures the delete confirmation must state, in one authorized read. Without it the chart page is one request per registered kind — 1 today, 15 by phase 004 — each re-running the same authorization for the same patient. One round trip instead of fifteen, and one place where the counts are decided. Contract operation total moves by **+1**; §2.3 records 93. |
| Every list envelope carries `total` **only** under `?count=true` (§2.1 rule 5) | `GET /api/v1/patients` returns `total` and `owned_count` **unconditionally** | FR-010 requires the list itself to state how many people there are, and a household is tens of rows — the count is a `COUNT(*)` over a handful of ids, not the open-ended scan rule 5 exists to prevent. It is the **one** documented exception in the phase; every other list in this phase and every list in phases 003–006 follows rule 5 unchanged. `owned_count` is in the envelope from the start so phase 005 adds `shared_count` without changing the shape. [research D-29](./research.md#d-29). |
| The record kind path segment, per contract rule 2, is the plural generated from one Go constant | Corrected to `/api/v1/records/**medications**` throughout this phase's contracts and quickstart | An earlier draft of this phase used the singular `/records/medication`. The constant is created in phase 001, so the spelling is settled there, and phase 003's kind registry already declares the plural. Closes cross-artifact finding **H1**; phase 001's plan records the same correction from its side (its research D-05). |
| `tags` and the `/tags` page are phase 002 | Not built here; **phase 003 owns them** | This phase's specification contains no requirement for tagging, and phase 003's US7 owns it end to end. Phase 003's own deviation table records the receiving half. |
| `catalog_lab_tests` is phase 002 | Not built here; **phase 004 owns it** | Nothing in this phase's requirements needs a laboratory test catalogue, and phase 004 builds it where the results that use it live. Phase 004's plan records the receiving half. |
| `audit_events` gains its `action` vocabulary here | The vocabulary is **phase 001's**, declared in full; this phase adds only the `patient`, `practitioner` and `facility` target kinds and the `patient` column | Declaring a `SelectField` vocabulary in deltas fails in production rather than in a test — the reasoning is in phase 001's deviation table. This phase's vocabulary migration asserts the **complete** expected set after it (T028a), never its own delta. |
| Phase 001 delivers password reset and email verification | It does — and this phase's Assumptions now say so accurately | The first draft of this phase's Assumptions asserted it while phase 001 had deferred both, which is cross-artifact finding **H2**. The allocation has since been settled the other way: **phase 001 builds password recovery by email and address confirmation**, and **phase 006 builds external sign-in** (finding **H7**). Nothing in this phase depends on any of the three, and this phase adds no auth page. |

Net effect on the contract's headline numbers from this phase alone: operations **+1** (op 93);
pages **±0** (six, exactly the six §3.1 lists for this phase); collections **±0**.

---

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| **CT-1.** `migrations/1756200600_medications_repoint.go` writes `medications.patient` with a raw `dbx` UPDATE statement, crossing the Principle II boundary that says only repositories touch storage and only the domain validates. | The backfill must set one column on every existing medication row. Doing it through `patient.Repository`/`medication.Repository` would (a) load and re-save every record, firing `OnRecordUpdate` and writing one spurious "system updated medication X" audit row per medication, and (b) require the repositories — which are wired by the DI container at boot — to be reachable from a migration, which runs before the container exists. | Per-record `app.Save` was rejected on the audit-noise and O(n) round-trip grounds above. Suppressing the hooks for the duration was rejected as worse: a global hook toggle is exactly the kind of hidden mode Principle I forbids. The mitigation is containment — the raw SQL is 4 lines in one migration file, the migration has an integration test asserting the before/after row counts and that zero medications are left unattributed (FR-022, SC-006), and `depguard` is *not* relaxed anywhere: `internal/store/migrations` is already a `[PB]` package. |
| **CT-2.** `patients.first_name`, `patients.last_name` and `patients.birth_date` are `Required: false` at the collection level while being required in `PatientCreate` and non-clearable in `PatientPatch`. The storage layer is therefore deliberately weaker than the API contract. | FR-005 requires a self-record patient to exist for **every** account — the ones created from now on *and* every account that already exists when this migration runs. Those accounts have a display name and nothing else. There is no lawful birth date to write. | Fabricating a placeholder date of birth was rejected outright: writing a value nobody supplied into a medical record is worse than a null, and FR-006 derives age from it. Blocking registration until a date of birth is supplied was rejected because it rewrites phase 001's registration contract and still cannot fix the accounts that already exist. Making the field required and creating the self-record lazily on first use was rejected because FR-005 says "MUST be created automatically", and a lazy record is a race between two tabs. The mitigation: the only rows that may carry a null are server-provisioned self-records, an integration test asserts that invariant directly against the database, and the chart header renders "Date of birth not recorded" with a completion prompt rather than a blank or a zero (FR-030). |

## Phase Exit Criteria

This phase is done when, and only when:

1. All 38 acceptance scenarios exist as named, passing automated tests (FR-054, SC-012), and
   `specs/002-patient-core/traceability.md` (T169) joins every acceptance scenario to its test,
   every functional requirement to the tasks that satisfy it and every success criterion to its
   task or to a criterion below, with no empty row. An unmapped requirement, or a success
   criterion neither mapped nor marked `[outcome metric]` in `spec.md`, fails the phase
   (cross-artifact finding M7).
2. Every one of the 20 new operations has an owner-succeeds / stranger-refused pair, and the
   refusal is byte-identical to a not-found (FR-055, SC-005).
3. `medigo routes`, `api/openapi.json` and the Playwright coverage agree; the diff of
   `api/openapi.json` in the phase's PR is reviewed and intentional (FR-056, Principle IX).
4. The 6 new pages pass the smoke gate at both viewports on a freshly seeded instance, including
   in their empty states (SC-013).
5. A migration run against a phase-001 database leaves 0 medications without a patient and 0
   medications on a patient other than their recording account holder's self-record (SC-006).
6. A grep of the captured log/metric/trace stream after exercising every endpoint finds 0 names,
   dates of birth, addresses or file names (SC-008).
