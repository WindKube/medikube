# Implementation Plan: Clinical Records

**Branch**: `003-clinical-records` | **Date**: 2026-08-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-clinical-records/spec.md`

**Constitution**: [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.3.0 (binding)

**Shared design contract**: [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) (binding on design; see [Deviations](#deviations-from-the-shared-design-contract) for the two places this phase departs from its *phase table*)

---

## Summary

This phase turns MediGo from "an application that holds medications for a person" into a
complete clinical chart. It registers **thirteen new record kinds** — allergy, condition,
encounter, procedure, treatment, symptom, vitals, immunization, injury, insurance, equipment,
emergency contact and family member — into the record-kind registry that phase 001 built,
extends the medication kind registered there, and then adds the three things that make thirteen
separate lists into one clinical picture: **relationships between records**, **tags that cross
every kind**, and **one search plus one timeline over a person's whole chart**.

The technical approach is deliberately anti-climactic, and that is the point:

- **Zero new record routes.** The six-operation `/api/v1/records/{kind}` family from phase 001
  absorbs all thirteen kinds. Each kind still has its own fully typed DTO; the polymorphism lives
  in the routing table and in an OpenAPI `oneOf` discriminated on `kind`, never in the schema.
- **Eight new operations in total**: four for tags, three for the one payload-carrying join
  (`treatment_medications`), one for search. Phase 003 is the largest phase in the project by
  domain surface and the second-smallest by API surface.
- **Every kind is a package, not a case in a switch.** `records.Register(kind, service, views,
  schema)` wires a kind's service, DTO codec, templ components, audit hook, search-index hook,
  realtime publish hook, two pages, two smoke cases and its OpenAPI branch in one call.
  Principle II's open/closed clause is satisfied structurally: nothing in this phase switches on
  `kind.Kind`.
- **One contract suite, run thirteen times.** `recordstest.KindContract` and
  `recordstest.RepositoryContract` are written once in the Foundational phase and executed
  against every kind's service, every kind's repository and every kind's fake. That is
  Principle II's Liskov clause and it is what keeps 13 kinds from costing 13× the test effort.
- **One authorization checkpoint.** Every service method's first act is
  `access.Authorizer.Patient(...)` or `.Record(...)`. Repositories never authorize; handlers
  never authorize; the record registry never authorizes.

---

## Technical Context

**Language/Version**: **Go 1.27** (`go 1.27` + `toolchain go1.27.x` in `go.mod`). This is not the
monorepo's 1.26.5 house standard and the divergence is forced, not stylistic: PocketBase v0.40.1
declares `go 1.27` and imports the Go 1.27 stdlib package `encoding/json/v2` in 67 non-test files,
15 of them in `core/` and `apis/`. `GOTOOLCHAIN=local go build` on 1.26.5 fails outright
(VERIFIED-SOURCE-FACTS FACT 0). CI MUST NOT set `GOTOOLCHAIN=local`.

**Primary Dependencies** (all pinned; no new dependency is introduced by this phase):

| Module | Version | Role in this phase |
|---|---|---|
| `github.com/pocketbase/pocketbase` | v0.40.1 | 16 new collections as Go migrations; `RunInTransaction`; post-commit model hooks; cascade delete; multi-relation + back-relation traversal |
| `github.com/a-h/templ` | v0.3.1020 | 3 components per kind (Row/List/Detail) + tag manager, search, timeline pages |
| `github.com/starfederation/datastar-go` | v1.2.2 | Create/edit drawers, link editors, live list patches via the phase-001 SSE bridge |
| `github.com/caarlos0/env/v11` | v11.4.1 | `MEDIGO_SEARCH_*` and `MEDIGO_LIST_*` knobs only if genuinely required (default: none added) |
| `github.com/rs/zerolog` | v1.35.1 | the only logger; PHI-redacting `MarshalZerologObject` on 13 new entities |
| `github.com/getsentry/sentry-go` | v0.48.0 | errors/panics only, scrubbed |
| `github.com/prometheus/client_golang` | latest pinned | `medigo_records_*` counters; `kind` label now bounded at 14 values |
| `go.opentelemetry.io/otel` | latest pinned | `service.<kind>.<Method>` and `store.<collection>.<op>` spans |
| `github.com/samber/do` | v2 | container providers for 13 new services |
| `github.com/samber/lo` | v1.53.0 | sparingly, per Principle IV |
| `github.com/stretchr/testify` | v1.12.0 | the only assertion library |
| `github.com/spf13/cobra` | **transitive — pinned once in [001's plan](../001-walking-skeleton/plan.md#technical-context), never a direct `require`** | reached only via PocketBase's `RootCmd`, which already is a `*cobra.Command`; `medigo seed` gains 13 kinds. The version is whatever `pocketbase@v0.40.1`'s `go.mod` requires and is not restated here (cross-artifact finding M2) |
| `modernc.org/sqlite` | v1.57.0 | transitive via PocketBase; pure Go, so `CGO_ENABLED=0` holds |

**Forbidden and absent**: gin, huma, viper, `samber/mo`, `samber/ro`, `samber/slog-zerolog`, any
second router/logger/config/DI/assert library, PocketBase `jsvm`, any cgo dependency,
`datastar.WithCompression`, and the Datastar Pro attribute set.

**Storage**: PocketBase-embedded SQLite (`modernc.org/sqlite`), data dir `/data/pb_data`, WAL.
16 new collections, all five API rules `nil` on every one of them, **zero new file fields**
(the `Protected: true` boot assertion continues to cover exactly `patients.photo`).

**Testing**: `stretchr/testify` (`require` for preconditions, `assert` for independent
assertions), table-driven `t.Run` subtests. Five layers, all mandatory:
unit against hand-written fakes with `t.Parallel()`; integration against a throwaway
`tests.NewTestApp` cloning `internal/testdata/pb_data`; **contract suites** (`KindContract`,
`RepositoryContract`) executed against every implementation including fakes; HTTP via
`tests.ApiScenario` with `ExpectedEvents` assertions; templ components rendered to a
`bytes.Buffer` and asserted on. Plus Playwright CLI for the 29 new pages at 1440×900 and 390×844.

**Target Platform**: one static `linux/amd64` + `linux/arm64` binary, `CGO_ENABLED=0`,
`gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, `VOLUME ["/data"]`.
No Node.js in the runtime image.

**Project Type**: single server-rendered Go web application, a project inside the
`windkube` monorepo at `/medigo`, image `ghcr.io/windkube/medigo`.

**Performance Goals** (from the spec's success criteria, and they are the acceptance bar):
- SC-002: any kind list page, any status view and the cross-type timeline render within **2 s**
  for a patient holding **50,000 records** spread across every kind.
- SC-003: the first page of grouped search results within **3 s** at that same scale, with 100%
  of matching kinds represented.
- SC-017: a change in one open view appears in another within **5 s**, and a view left open for
  **60 continuous minutes** is still receiving updates (this is the PocketBase 5-minute
  `WriteTimeout` trap; the phase-001 `newStream()` helper is the fix and this phase adds the
  regression assertion).
- SC-007: renaming a tag carried by 500 records across ≥8 kinds is one row update.

**Constraints**:
- No cgo, one binary, no runtime Node, no CDN fetch, no outbound request the operator did not
  configure (FR-088).
- CSP: `script-src 'self' 'unsafe-eval'`; every other directive strict. No inline `<script>`, so
  the whole Datastar inline-script SDK family (`ExecuteScript`, `ConsoleLog`, `ConsoleError`,
  `Redirect`, `DispatchCustomEvent`, `ReplaceURL`, `Prefetch`) stays banned.
- Records are **hard** deleted; no `deleted_at` on any collection in this phase (Constitution VII).
- PocketBase's record CRUD subtree and `/api/batch` remain unreachable to non-superusers.
- `OnRecord*Request` hooks are dead code under the lockdown; only `OnRecord*Execute` /
  `…AfterCreateSuccess` / `…AfterUpdateSuccess` / `…AfterDeleteSuccess` are used.
- Single instance by construction; the realtime hub is a channel and a map, not a broker.
- Go 1.27 `encoding/json/v2` retrofit semantics apply to every new DTO: slices marshal as `[]`
  never `null`, unknown fields are rejected (422), duplicate keys are rejected.

**Scale/Scope**: 13 new record kinds (14 registered in total after this phase), 16 new
collections, 8 new `/api/v1` operations (22 from phase 001 + 20 from phase 002 + 8 = **50 registered** of the
94 across all six phases, SHARED-DESIGN §2.3 — cited, not re-derived), 29 new
user-facing pages (58 smoke cases at two viewports), 4 shared enum vocabularies + 20 per-kind
vocabularies, 6 multi-relation link fields + 1 payload-carrying join, ~50,000 records per patient
as the performance target.

**No `NEEDS CLARIFICATION` items remain.** Everything the spec left open is resolved in
[research.md](./research.md).

---

## Constitution Check

*GATE — evaluated before Phase 0 and re-evaluated after Phase 1 design. Both passes recorded.*

### I. Simplicity Is A Gate (KISS) — **PASS, with three tracked entries**

Thirteen record kinds land on **zero new record routes**; the whole phase adds eight operations.
The fifteen "specialised filter" endpoints the upstream application shipped
(`/conditions/patient/{id}/active`, `/procedures/scheduled`, `/medical-equipment/needing-service`,
`/insurances/expiring`, …) are query parameters on the existing list operation, which FR-079
independently demands. Symptoms are one flat collection of episodes rather than upstream's
definition/occurrence two-level model. Injury types are a select vocabulary rather than a
user-extensible FK table. `family_member.conditions` and `insurance.coverage`/`contact` are
validated Go structs in a `json` field rather than three more collections.

Three things strain this principle enough to be recorded in
[Complexity Tracking](#complexity-tracking): the thirteen near-identical service/store package
pairs, the `search_index` denormalisation, and the `treatment_medications` join with its
`effective_*` resolution.

**Explicit YAGNI decisions taken here**: no relevance ranking (FR-073 forbids claiming it); no
saved searches; no per-kind bespoke endpoints; no `catalog_vaccines` relation on immunizations
(the spec explicitly defers a standardised vaccine library); no soft delete; no idempotency keys;
no bulk-create/bulk-link operations; no `/new` or `/edit` routes (drawer state is a Datastar
signal, and adding them would cost 26 routes, 26 smoke cases and 26 OpenAPI entries for nothing).

### II. Interfaces At Every Seam (SOLID) — **PASS**

- **Single responsibility**: `internal/web/api` parses and renders; `internal/service/<kind>`
  decides; `internal/store/<kind>` persists. `internal/records` dispatches and nothing else.
- **Open/closed**: this is the phase that proves it. Thirteen kinds are added by satisfying the
  existing `records.Service` interface and calling `records.Register`. A test in
  `internal/records/registry_completeness_test.go` fails the build if any `kind.Kind` value lacks
  a registry entry, an OpenAPI branch, two page routes or two smoke cases. **No file in this
  phase contains a `switch k` over `kind.Kind`**, and a `gocritic`/custom vet check asserts it.
- **Liskov**: `recordstest.RepositoryContract` and `recordstest.KindContract` are single suites
  run against 13 real repositories, 13 fakes and 13 services. An implementation that cannot pass
  them means the interface is wrong, not the implementation.
- **Interface segregation**: every port is declared by its consumer and is 1–4 methods.
  `<kind>.Repository` is 4 (`Get`, `List`, `Save`, `Delete`); `<kind>.Authorizer` is 2;
  `<kind>.Auditor` is 1; `<kind>.Indexer` is 2 (`Index`, `Remove`); `tag.UsageCounter` is 1;
  `search.Query` is 1. There is no omnibus `Store` or `Service`.
- **Dependency inversion**: `internal/domain/**` and `internal/service/**` import neither
  PocketBase, nor `net/http`, nor templ. Enforced by the `depguard` rule already in
  `.golangci.yml`, extended in this phase's Setup tasks to cover the 13 new service packages by
  the existing `**/internal/service/**` glob (no new rule needed).

### III. Test-First With testify (NON-NEGOTIABLE) — **PASS**

Every one of the spec's **53** acceptance scenarios (US1:7 US2:5 US3:6 US4:5 US5:5 US6:6 US7:5
US8:5 US9:5 US10:4) becomes a named test before its implementation
task starts (FR-091, SC-013). Test tasks precede implementation tasks throughout `tasks.md`.

Authorization is tested as a first-class concern, exactly as FR-092 and SC-004 demand: for each
of the thirteen kinds, `internal/web/api/<kind>_http_test.go` proves a non-owning account is
refused with a response **indistinguishable from the record not existing** on read, list, create,
update, delete, relate, tag and search. The refusal is `404`, never `403`, for anything
patient-scoped — Constitution VII makes existence itself PHI.

Test code is production code: the per-kind suites are one shared table plus a per-kind fixture,
not thirteen copy-pasted files.

### IV. Idiomatic Go Over Clever Go — **PASS**

Errors are values, wrapped with `%w`, inspected with `errors.Is`/`errors.As`, mapped to status
codes by the single table in `internal/web`. `Patch` structs carry absent-vs-null with plain
pointers (`*string`, `**clinical.Date`) — `samber/mo` is forbidden precisely because `mo.Result`
severs the chain this table depends on. `context.Context` is the first parameter of every I/O
function and its cancellation is honoured (the SSE handlers and the 50k-row list queries both
depend on it). Generated `*_templ.go` is committed, marked generated, and excluded from lint and
coverage. `samber/lo` appears only where it removes a meaningless loop.

### V. PocketBase Is The Platform, Not A Detail — **PASS**

Nothing PocketBase provides is rebuilt. Sixteen collections arrive as reversible Go migrations
under `internal/store/migrations/`; schema is never changed in the admin UI. Cascade delete on the
`patient` relation is what satisfies FR-087/SC-005 (deleting a person destroys every record of
every kind), and PocketBase's relation cleanup on delete is what satisfies FR-058/SC-006 (deleting
a linked record leaves the other intact with the link gone) and FR-066 (deleting a tag destroys
no record). Both are *verified by test*, not assumed.

`app.RunInTransaction` wraps the three multi-write operations this phase adds: the
`treatment_medications` upsert, the "at most one primary" displacement (FR-045, FR-051), and
`family_member` create/update with its validated conditions array.

Hooks: only the post-commit `OnRecordAfterCreateSuccess` / `…UpdateSuccess` / `…DeleteSuccess`
model hooks are bound, registered once by `records.Register`. `OnRecord*Request` is not used and
is blocked by the existing `forbidigo` pattern — under the auto-API lockdown those hooks never
fire and anything placed there is silently dead code.

All five API rules stay `nil` on all 16 new collections, asserted at boot and proved per
collection by a `tests.ApiScenario` showing `/api/collections/<c>/records` returns 404 to a
normal user.

### VI. One Log Stream, One Trace Context — **PASS**

zerolog only. Every one of the 13 new domain entities implements `MarshalZerologObject` emitting
**only** its opaque id and patient id, so logging one by accident cannot leak a diagnosis, a
member number or a note (FR-086, SC-012). The `medigo_records_*` metric family adds a `kind`
metric label bounded at 14 values; no patient id, record id, tag name or search term ever becomes
a metric label value. **FR-075 gets its own gate**: a `forbidigo`-style check plus a runtime test asserts the
search term never reaches a log line, a span attribute, a metric label or a Sentry event.

FR-094/SC-012 is discharged by a real test, not a promise: phase 001's `internal/testsupport/phileak`, extended here rather than re-created, drives
every operation this phase defines against a seeded instance whose clinical values are
recognisable sentinels, captures the zerolog stream, the Prometheus registry, the OTel span
recorder and the Sentry transport, and asserts zero sentinel occurrences.

### VII. Patient Privacy Is Structural, Not Procedural — **PASS**

- 404-not-403 for every patient-scoped resource, including through a link, a tag, search, the
  timeline and a status view (FR-082, SC-004). One test matrix covers all six routes in.
- Cross-patient linking is refused with a response that discloses nothing about the other record,
  including whether it exists (FR-057, US6 scenario 3). The check is server-side against the
  *stored* patient of both records — never a client-supplied value.
- Insurance `member_id`, `group_number` and `holder_name` are PHI and are redacted everywhere
  (FR-047). So are notes, diagnoses, allergen names, symptom names and search terms.
- **Zero new file fields**, so the `Protected: true` boot assertion is unchanged and still
  trivially auditable.
- Hard delete with an explicit confirmation that names what will be destroyed and how many other
  records refer to it (FR-006). No `deleted_at` column is added.
- The audit trail records who/what/which patient/which opaque id/when and **never content**
  (FR-084/085, SC-011), including for refused access attempts.
- No outbound network request (FR-088). Nothing in this phase talks to anything.

### VIII. The UI Must Prove It Renders — **PASS**

29 new pages, each covered at 1440×900 and 390×844, asserting 200, the four shell landmarks plus
the page's own landmark, `body[data-signals]` present, and zero console errors / page errors /
failed requests. The route list is derived from `medigo routes`, so a page added without a smoke
case fails the build (FR-093, SC-015). The seeded instance deliberately leaves several of these
pages empty so the `@EmptyState` path is what the landmark assertion exercises — that is the most
common way a smoke gate goes falsely green *or* falsely red.

**The seven status views enter the gate too, and they are not seven more pages.** FR-078's views
are query strings on kind lists (`/conditions?active=true`, …) because FR-079 requires exactly
that, so `medigo routes` would not list them and the gate — which is derived from that inventory —
would not visit them, while FR-080 demands a helpful empty state on **every** one. The page spec
therefore gains one optional field, `SmokeVariants []string`: additional **concrete** URLs on an
already-registered route that the gate must also visit, emitted inside their route's entry, not
counted as pages, and declared from the same `internal/records/statusviews.go` catalogue the
filters read — so a status view cannot be added without a smoke case, nor a smoke case outlive its
view. Two of the seven are seeded **empty** on purpose. Closes cross-artifact finding **L2**;
specified in [contracts/pages.md](./contracts/pages.md) §3.5.

Keyboard-only completion of record, correct, relate, tag and delete at both viewports (SC-018)
is asserted by Playwright keyboard-driven specs with a visible-focus assertion.

### IX. Compliance Is A Build Gate, Not A README Paragraph — **PASS**

Four gates, all `go test` or CI steps, all failing the build:
1. `internal/openapi/gate_test.go` — the route registry and the committed `api/openapi.json`
   agree on every `operationId`; the regenerated document is byte-identical to the committed one.
2. `internal/records/registry_completeness_test.go` — every `kind.Kind` value has a registry
   entry, an OpenAPI `oneOf` branch with a `kind` discriminator, exactly two page routes, a
   default sort, a searchable-field declaration, a seed fixture and two smoke cases.
3. `e2e/routes.gate.spec.ts` — every route emitted by `medigo routes` with `Page: true` has a
   smoke case, **and every `SmokeVariants` entry on it is visited too**; an uncovered route, or a
   status-view catalogue entry with no variant, fails.
4. golangci-lint v2 with `depguard` (import boundary) and `forbidigo` (`app.Logger()`,
   `fmt.Print*`, `log.*`, `OnRecord*Request`, the Datastar inline-script family, the Datastar Pro
   attribute set).

Every one of the 16 migrations has a real `down`; `migrations.Register`'s signature makes that
structural (VERIFIED-SOURCE-FACTS FACT 8).

### Post-Design Re-Check (after Phase 1)

Re-evaluated against `data-model.md` and `contracts/`. No principle moved from PASS to FAIL. The
three Complexity Tracking entries below were opened during design and none was closed by it; two
new candidates were considered and **rejected as not needing an entry**:

- *`selection_basis` on list rows* (FR-026/049/078/079): looked like a special case, is not — it
  is a per-kind declared function on the registry, evaluated by the same filter that selected the
  row, with no branching anywhere central.
- *Per-kind default sort orders* (FR-007, FR-051): declared data on the registry entry, not code.

---

## Project Structure

### Documentation (this feature)

```text
specs/003-clinical-records/
├── plan.md              # This file
├── research.md          # Phase 0 output — every technical decision, with evidence
├── data-model.md        # Phase 1 output — 16 collections, fields, enums, migrations
├── quickstart.md        # Phase 1 output — run and verify this phase by hand
├── contracts/
│   ├── README.md            # conventions, error envelope, ETag rules, authorization table
│   ├── records-clinical.md  # the 6-op record family as it applies to the 13 new kinds
│   ├── treatment-medications.md  # the 3-op payload-carrying join
│   ├── tags.md              # the 4-op tag API
│   ├── search.md            # the 1-op grouped search
│   └── pages.md             # the 29 page routes, landmarks and smoke expectations
├── checklists/          # pre-existing
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root `/medigo`)

Only paths this phase **creates** or **touches** are listed. `[NEW]` = created here,
`[EDIT]` = an existing file this phase modifies. `<kind>` expands to the thirteen kinds:
`allergy condition encounter procedure treatment symptom vitals immunization injury insurance
equipment emergencycontact familymember`.

```text
medigo/
├── cmd/medigo/main.go                                  [EDIT] register 13 services in the container
│
├── internal/domain/
│   ├── kind/kind.go                                    [EDIT] +13 Kind values, path segments, plural labels
│   ├── kind/kind_test.go                               [EDIT] exhaustiveness + spelling round-trip
│   ├── clinical/vocab.go                               [NEW]  Severity, ConditionStatus, OrderStatus, TherapyStatus
│   ├── clinical/vocab_<per-kind>.go                    [NEW]  20 per-kind vocabularies with Valid()
│   ├── clinical/dates.go                               [NEW]  Date/Instant, ordering + not-future validators
│   ├── clinical/units.go                               [NEW]  SI storage <-> display conversion (FR-037)
│   ├── clinical/<kind>.go                              [NEW]  ×13 entity + Validate + MarshalZerologObject
│   ├── clinical/link.go                                [NEW]  LinkSet, same-patient invariant (FR-057)
│   ├── clinical/coursemedication.go                    [NEW]  join entity + effective_* resolution (FR-060)
│   ├── clinical/familycondition.go                     [NEW]  validated []FamilyCondition value object
│   ├── clinical/insurancecoverage.go                   [NEW]  validated Coverage + Contact value objects
│   ├── tag/tag.go                                      [NEW]  Tag entity, case-insensitive name rule
│   └── search/query.go                                 [NEW]  Term, Kinds, Tags, Match, Range value objects
│
├── internal/records/
│   ├── register.go                                     [EDIT] +SearchFields, +DefaultSort, +Filters, +Basis
│   ├── filters.go                                      [NEW]  the declared narrowing vocabulary per kind
│   ├── registry_completeness_test.go                   [EDIT] now asserts 14 kinds fully wired
│   ├── kinds/<kind>.go                                 [NEW]  ×13 the single Register call per kind
│   └── recordstest/{kindcontract.go,repositorycontract.go,fixtures.go}  [NEW] the shared suites
│
├── internal/service/
│   ├── <kind>/{service.go,ports.go,adapter.go,query.go,patch.go}        [NEW] ×13
│   ├── <kind>/{service_test.go,adapter_test.go}                          [NEW] ×13
│   ├── <kind>/<kind>test/fake.go                                         [NEW] ×13
│   ├── link/{service.go,ports.go,service_test.go}       [NEW]  cross-record link rules (US6)
│   ├── coursemedication/{service.go,ports.go,service_test.go}  [NEW]  the join (US6)
│   ├── tag/{service.go,ports.go,service_test.go}        [NEW]  tags (US7)
│   ├── search/{service.go,ports.go,indexer.go,service_test.go}  [NEW]  index + query (US8)
│   ├── timeline/{service.go,ports.go,service_test.go}   [NEW]  cross-kind chronology (US9)
│   └── access/authorizer.go                            [EDIT] Record() learns the 13 new kinds
│
├── internal/store/
│   ├── migrations/1756..._tags.go                      [NEW]  must precede every kind (tags relation)
│   ├── migrations/1756..._search_index.go              [NEW]
│   ├── migrations/1756..._<kind>.go                    [NEW]  ×13
│   ├── migrations/1756..._links.go                     [NEW]  the 6 multi-relation fields
│   ├── migrations/1756..._treatment_medications.go     [NEW]
│   ├── migrations/1756..._medication_tags.go           [NEW]  adds tags to the phase-001 kind
│   ├── migrations/1756..._audit_vocab.go               [NEW]  +tag, +search; asserts the complete set
│   ├── <kind>/{repo.go,mapper.go,repo_test.go}         [NEW]  ×13
│   ├── coursemedication/{repo.go,repo_test.go}         [NEW]
│   ├── tag/{repo.go,usage.go,repo_test.go}             [NEW]
│   ├── search/{repo.go,repo_test.go}                   [NEW]
│   ├── timeline/{repo.go,repo_test.go}                 [NEW]
│   └── filter.go                                       [EDIT] typed filter -> PB expression builder
│
├── internal/platform/pb/hooks.go                       [EDIT] kind registration binds hooks for 13 more
│
├── internal/web/
│   ├── api/<kind>.go                                   [NEW]  ×13 Summary/Detail/Create/Patch DTOs
│   ├── api/<kind>_test.go                              [NEW]  ×13 json/v2 round-trip
│   ├── api/<kind>_http_test.go                         [NEW]  ×13 ApiScenario + authorization matrix
│   ├── api/tags.go, tags_test.go, tags_http_test.go    [NEW]
│   ├── api/coursemedications.go(+tests)                [NEW]
│   ├── api/search.go(+tests)                           [NEW]
│   ├── api/references.go                               [NEW]  the pre-delete reference count (FR-006)
│   ├── page/{records.go,tags.go,search.go,timeline.go} [EDIT/NEW]
│   ├── stream/records.go                               [EDIT] 13 more kinds on the existing stream
│   └── views/
│       ├── records/<kind>.templ (+ _test.go)           [NEW]  ×13 Row/List/Detail
│       ├── records/links.templ                         [NEW]  the link editor, both ends
│       ├── records/coursemedications.templ             [NEW]  effective_* with provenance (FR-060)
│       ├── tags/{manager.templ,picker.templ}           [NEW]
│       ├── search/results.templ                        [NEW]  grouped, per-group paging
│       ├── timeline/timeline.templ                     [NEW]
│       ├── shared/{deleteconfirm.templ,basis.templ,emptystate.templ}  [NEW/EDIT]
│       └── ids/ids.go                                  [EDIT] deterministic ids for the new components
│
├── internal/cli/seed.go                                [EDIT] deterministic fixtures for 13 kinds + tags
├── internal/testsupport/
│   ├── phileak/{capture.go,exercise.go}                [EDIT] phase 001's harness, + this phase's sentinels (FR-094 / SC-012)
│   └── scale/generate.go                               [NEW]  50k-record generator for SC-002/003
│
├── api/openapi.json                                    [EDIT] regenerated, committed, diffed
├── e2e/specs/{records.spec.ts,tags.spec.ts,search.spec.ts,timeline.spec.ts,keyboard.spec.ts}  [NEW]
├── e2e/routes.gate.spec.ts                             [EDIT] +29 page routes and their SmokeVariants (L2)
├── internal/httproute/{registry.go,routes.go}          [EDIT] +SmokeVariants on the page spec (L2)
├── internal/records/statusviews.go                     [NEW]  the 7 status views, declared once (FR-078/079/080)
└── .golangci.yml                                       [EDIT] forbid `switch` over kind.Kind outside records/
```

**Structure Decision**: The existing single-project Go layout from phase 001 is kept unchanged;
this phase only populates it. The one structural addition is
`internal/records/recordstest/` — the shared contract suites. It is created here rather than in
001 because Principle I's "an abstraction needs two real implementations first" clause is only
satisfied now: phase 001 had one clinical kind, phase 003 has fourteen.

---

## Deviations from the shared design contract

The shared design contract is binding on **design**. Its §0 phase table is not consistent with the
specification set that was actually written (`001-walking-skeleton` … `006-reporting-and-operations`),
and phase 004's own specification already resolved that conflict in favour of the charters:
*"where the shared design contract's phase table places it here, the charter governs — exactly as
it did for phase 001."* This plan follows that established precedent. Most of the rows below alter
**no design in the contract — only the phase in which an already-designed piece lands.** Two are
genuine design deviations and say so: `catalog_vaccines` is not built, and `search_index` is an
ordinary collection matched with `LIKE` rather than an FTS5 virtual table with a `reindex` command
(§1.2 says **MAY** on ranking, and FR-073 declines it).

| Contract says | This plan does | Why |
|---|---|---|
| `tags` collection + ops 40–43 + `/tags` page are phase 002 | They land here | Phase 002's specification contains the word "tag" zero times. Phase 003 US7 owns tags. Shape, routes and page are the contract's, verbatim. |
| The concept is **`tags`** — collection `tags`, route `/tags`, relation field `tags`, query parameter `?tags=`, landmark `region[name="Tags"]`; and **`encounters`** — collection `encounters`, route `/encounters`, landmarks `region[name="Encounters"]` / `article[name="Encounter"]` | **The same, with no rename anywhere** | An earlier draft of this phase said "label" throughout its specification and rendered `region[name="Labels"]`, and said "appointment" while rendering `article[name="Appointment"]` — a second vocabulary for a concept whose collection, route, field and query parameter all said `tag` and `encounter` (cross-artifact finding **M5**). "Label" and "appointment" add nothing the settled words do not already carry, and a landmark string is a literal the Playwright gate contains, so two vocabularies is a gate defect waiting to happen. **Tags and encounters win, everywhere**, and this row exists so nobody re-opens it. This is a correction to *this plan*, not a deviation from the contract: the contract said `tags` and `encounters` all along. |
| `search_index` + op 57 + `/search` page are phase 004 | They land here | Phase 004's spec explicitly excludes cross-kind search and hands it back. Phase 003 US8 owns it. Shape, envelope and per-group paging are the contract's, verbatim. Phase 004 adds `lab_result` and `attachment` to it, exactly as both documents say. |
| `allergy` and `emergency_contact` are phase 001 | They land here | Phase 001 delivered `medication` as its one clinical kind. Phase 003 US1 owns allergies and emergency contacts. |
| `medication` is phase 003 | It was delivered in 001 and is only **extended** here | Phase 001's charter. This phase adds its `tags` relation and it becomes a link target. |
| `immunization.catalog_vaccine` relation, `catalog_vaccines` collection | Not built | Phase 003's spec explicitly defers a standardised vaccine library: *"nothing in this phase's requirements needs it, and adding it here would be work done ahead of a requirement."* The field is left out of the collection entirely rather than added and left null — adding it later is one reversible migration. |
| the contract's page inventory across all phases | **±0**: `/timeline` is added by this phase and SHARED-DESIGN §3.1 already counts it inside the authoritative **58**. When this plan was first written the contract listed 57 and this row claimed +1; the contract has since absorbed `/timeline` (and 005's `/invite/{token}`) into its reconciliation — *"56 + 2 = 58"* — so **the delta against the contract as published is zero** and this row records a page this phase originates, not a page it ships beyond the inventory (corrected 2026-08-27, ANALYSIS N6). | FR-076/077 require a narrowable cross-kind chronological view; no existing page can carry it (the dashboard's landmark and purpose are phase 001's). Cost: 1 route, 2 smoke cases, 0 API operations — it reads the existing `GET /api/v1/records`. |
| `search_index` is an **FTS5 virtual table** created by a raw SQL migration, kept in step by the post-commit hooks and rebuilt by **`medigo reindex`** (§1.2) | An **ordinary PocketBase collection** (`data-model.md` §5.3) matched with `LIKE '%term%'` over `title` and `body`, ordered by `occurred_on DESC, id DESC`. **No FTS5 virtual table, and no `reindex` subcommand** — the CLI keeps 001's six (`001/contracts/cli.md`) | FR-073 forbids claiming relevance ranking, and ranking is the only thing FTS5 buys; with results ordered by date there is nothing for `rank` to do. So the FTS5 route would charge this phase a raw SQL migration PocketBase's migrations cannot model, a second maintenance path beside the post-commit hooks, and a seventh CLI subcommand — for a column FR-073 forbids reading. **The reason is not availability**: risk R3 is CLOSED (VERIFIED-SOURCE-FACTS FACT 11 — FTS5, `MATCH` and `rank` all work in `modernc.org/sqlite` v1.57.0, the version PocketBase v0.40.1 pulls), and §1.2 says **MAY**, not MUST. An earlier draft of `contracts/search.md` and of research D-11 gave "FTS5 availability is unverified" as the reason; the dossier refutes it and this row replaces it (deviation recorded 2026-08-27, ANALYSIS N12). Revisit only if a future spec asks for ranking; SHARED-DESIGN §1.2 records what that would cost. |
| `audit_events.action` / `.target_kind` vocabularies | Extended, additively, by **two target-kind values only** | Phase 001 declares the contract's complete vocabulary, so the thirteen new record kinds and `access_denied` are already there — `access_denied` arrives with `audit_events` in 001 (001 research D-20) rather than here, so refusals are not encoded two ways either side of this phase, and FR-084 is satisfied by a value that already exists. This phase adds `tag` (tags are auditable resources, FR-059) and `search` (the target kind of a search's row, D-12). The migration's test asserts the **complete** set after this phase — twenty-one actions, twenty-seven target kinds — never a delta. Closes cross-artifact findings **C1**, **M1** and **L3**. |

Net effect of **this phase** on the contract's headline numbers: operations **±0** (none added or
removed by this phase; three moved between phases); collections **−1** (`catalog_vaccines` dropped;
`catalog_lab_tests` stays in 004); pages **+1 originated** (`/timeline`), already absorbed into the
contract's published inventory, so the delta against it is **zero**. The suite-wide totals are computed
once in SHARED-DESIGN §1.6, §2.3 and §3.1 — **30 collections, 94 operations, 58 pages + 3 error
views** — and this plan cites them rather than re-deriving them.

---

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| **Thirteen near-identical `internal/service/<kind>` + `internal/store/<kind>` package pairs** (~26 packages, ~104 files) against Principle I's "prefer deleting code" | Principle II's open/closed clause forbids the alternative. Each kind has genuinely different fields, different required-field rules, different cross-field date rules, a different default sort and a different searchable-field projection. Those differences have to live somewhere typed. | **(a) One generic `clinical.Service` parameterised by a schema descriptor** — this is a `switch k` wearing a costume: every per-kind rule becomes a table entry interpreted at runtime, validation errors lose their field names, and the compiler stops checking anything. **(b) Code generation from a DSL** — introduces a generator, a schema language and a drift risk (the exact failure the `tool` directive exists to prevent), to save typing in files nobody edits twice. **(c) Generics over a `Record` constraint** — Go generics cannot express "these 14 structs each have a different set of fields", so it degrades to (a). The repetition is bounded, mechanical, reviewed once per kind, and every instance is held to one shared contract suite — which is the Go-idiomatic trade and what Principle IV asks for. |
| **`search_index` — a denormalised second copy of clinical text** against Principle I (a cache) and Principle VII (a second store of PHI) | FR-069 requires one search across a person's records of every kind; SC-003 requires the first page in **3 s at 50,000 records**. The honest alternative is a 14-way `UNION ALL` with a `LIKE` per kind and a merge sort, which is 14 table scans per keystroke-batch and cannot meet SC-003. The index is patient-scoped with `CascadeDelete: true`, so FR-087/SC-005 hold without a second cleanup path; it is written by the same post-commit hook that writes the audit row, registered by `records.Register`, so a kind cannot be added and forgotten; and it stores **no content the source rows do not already store**, so the privacy surface is unchanged in kind and doubled only in copies. | **(a) `UNION ALL` at query time** — fails SC-003 and puts 14 table scans behind an interactive box. **(b) SQLite FTS5** — FR-073 forbids claiming relevance ranking, and ranking is the only thing FTS5 buys, so its raw SQL migration, its separate maintenance path and its rebuild command would all be paid for a `rank` column this phase must never read. Availability is **not** the reason: risk R3 is CLOSED (VERIFIED-SOURCE-FACTS FACT 11 — FTS5, `MATCH` and `rank` all work in the vendored `modernc.org/sqlite` v1.57.0). Resolved in research.md D-11: `LIKE` over the ordinary `search_index` collection, ordered by date. **(c) No index, search only within a kind** — contradicts FR-069 and US8 outright. |
| **`treatment_medications`: a join collection with payload plus `effective_*` COALESCE resolution in the read path**, against Principle I | FR-060 and FR-061 require it by name: a medication attached to a course of treatment carries a course-specific dose, frequency, duration, timing, prescriber, pharmacy and dates, falls back to the medication's own values for anything the course does not state, **and the screen must say which value came from where**. That is irreducibly a payload on an edge. Every *other* relationship in this phase is a plain PocketBase multi-relation field with no collection and no endpoints. | **(a) Duplicate the fields onto `medications`** — a medication in two courses at two doses becomes two medication rows, and the allergy that points at "that medication" now points at one of them. **(b) A free-text note on the link** — the upstream application's answer; it is unqueryable, unvalidatable and cannot express provenance, which FR-060 requires. **(c) Compute `effective_*` in the browser** — puts a clinical dose calculation in JavaScript under a CSP that forbids inline script, and duplicates the rule in two languages. The join is confined to one collection, one service, three operations and one templ component; nothing else in the phase resembles it. |
