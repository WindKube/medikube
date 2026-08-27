# Phase 0 Research: Clinical Records

**Feature**: `003-clinical-records` | **Date**: 2026-08-26

Every decision this phase makes, with its evidence. Where [`VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) settles a
question it is cited by FACT number and it overrides every other source. Where the shared design
contract settles a shape it is cited by section. Nothing below is left as
`NEEDS CLARIFICATION`.

---

## D-01 — Thirteen kinds ride the existing six-operation record family; zero new record routes

**Decision.** `allergy`, `condition`, `encounter`, `procedure`, `treatment`, `symptom`, `vitals`,
`immunization`, `injury`, `insurance`, `equipment`, `emergency_contact` and `family_member` are
registered into the `/api/v1/records/{kind}` family that phase 001 built. Phase 003 adds **no
record routes**. `{kind}` is an OpenAPI `enum` of plural kebab-case segments; request and response
bodies are a `oneOf` discriminated on the `kind` property, so every kind keeps a fully typed DTO.

**Rationale.** Shared design contract §2.2. The upstream application spent ~65 operations,
thirteen routers and 21 legacy duplicates on this surface. Six operations do the same work, and
adding a fourteenth kind later costs zero routes and zero OpenAPI paths. It is also the only way
the 94-operation budget (SHARED-DESIGN §2.3) survives a phase that adds thirteen record types.

**The load-bearing precondition is discharged.** Shared-design risk **R1** made this contingent on
phase 001 proving that a discriminated `oneOf` can be generated from Go types *and* gated by the
Principle IX registry↔OpenAPI check, with two kinds registered. Phase 001 registered `medication`;
this plan therefore treats R1 as **closed by 001's exit criteria** and adds a hard assertion
rather than an assumption: `internal/records/registry_completeness_test.go` fails the build if any
`kind.Kind` value lacks a `oneOf` branch carrying a `kind` discriminator. If R1 is in fact still
open when this phase starts, the phase is blocked — it is not a thing to discover at task T140.

**Alternatives considered.**

- *Per-kind route families* (`/api/v1/conditions`, `/api/v1/procedures`, …): 13 × 5 = 65 new
  operations, taking the project from 90 to ~150 and breaking §2.1's budget. Rejected.
- *A single untyped `/api/v1/records` with a free-form body*: destroys the typed-DTO guarantee in
  Principle V and makes OpenAPI useless. Rejected.

---

## D-02 — One vocabulary per idea: four shared ladders, twenty per-kind vocabularies

**Decision.** Four shared enums serve every kind that expresses their idea:
`Severity{mild moderate severe life_threatening}` (allergy, condition, injury, symptom);
`ConditionStatus{active healing inactive resolved chronic}` (allergy, condition, injury, symptom);
`OrderStatus{ordered scheduled in_progress completed cancelled}` (procedure, and lab_result in 004);
`TherapyStatus{active on_hold completed stopped cancelled}` (medication, treatment, equipment).
Twenty further vocabularies are per-kind (shared design contract §1.4, reproduced exactly in
`data-model.md`). Every value is `snake_case`. Every vocabulary that the domain admits a catch-all
for has one (`other`); those that do not (`Severity`, the four ladders, `laterality`) are
documented as complete.

Each vocabulary is **one Go source of truth**: a string type in `internal/domain/clinical` with a
`Valid() bool` and an `All()` slice, from which the migration's `core.SelectField.Values` is
populated. The two cannot drift because the migration reads `All()`.

**Rationale.** FR-012 requires exactly this, and the spec's own Decisions section states why:
"Four ladders instead of a dozen near-duplicates is what makes a cross-type timeline and a
cross-type status view possible at all." Shared design contract §1.0 rule 6 mandates the
one-source-of-truth mechanism. MediKube *chooses* these vocabularies — only six of upstream's enums
were ever declared, the rest lived in Pydantic validators (domain-clinical.md §0), so there is no
compatibility obligation.

**Alternatives considered.**

- *Per-kind status enums* (upstream's shape): makes FR-078's cross-kind status views and FR-076's
  timeline require a per-kind translation table. Rejected.
- *User-extensible vocabularies*: FR-012 forbids extension by ordinary use, and the spec's
  Decisions section rejects it for injury types by name.

---

## D-03 — `_on` is a calendar date, `_at` is an instant, and the split is enforced by type

**Decision.** A field named `*_on` is a date-only value stored as `YYYY-MM-DD` and never shifted by
a viewer's time zone. A field named `*_at` is an instant stored as RFC3339 **UTC** and presented in
the viewer's local terms. Two distinct Go types (`clinical.Date`, `clinical.Instant`) make the
distinction unmixable; both are backed by PocketBase `DateField`, with `Date` normalising to
midnight UTC and marshalling as `YYYY-MM-DD`.

Applied: `onset_on`, `resolved_on`, `occurred_on`, `started_on`, `ended_on`, `administered_on`,
`expires_on`, `effective_on`, `prescribed_on`, `serviced_on`, `service_due_on` are dates.
`symptom.occurred_at`, `symptom.resolved_at`, `vitals.recorded_at` are instants, because a symptom
episode and a home blood-pressure reading have a meaningful time of day (US3: "her father's
dizziness on Tuesday morning").

**Rationale.** FR-011 requires precisely this distinction. Shared design contract §1.0 rule 10
mandates it and additionally collapses upstream's `occurrence_date` + `occurrence_time` pair into
one `occurred_at` — two columns for one instant was the source of upstream's timezone bugs.

**Alternatives considered.** A single `time.Time` everywhere with a `has_time` boolean: reproduces
the two-column problem in one column plus a flag, and every read site must remember the flag.
Rejected.

---

## D-04 — Cross-field date rules live in the domain and report every offence at once

**Decision.** `internal/domain/clinical/dates.go` provides `Order(earlier, later Ref) *FieldError`,
`NotFuture(Ref) *FieldError` and `RequiredWhen(cond bool, Ref) *FieldError`. Each entity's
`Validate()` accumulates into a `*domain.ValidationError` carrying **every** offending field, which
the error envelope renders as `fields[]`.

Rules this phase implements:
`condition.resolved_on` required when `status == resolved`, ≥ `onset_on`, ≤ today (FR-020);
`treatment.ended_on` ≥ `started_on` (FR-013, US2/4); `procedure.occurred_on` may be future when
`status ∈ {ordered, scheduled}` and MUST NOT be future when `status == completed` (FR-025);
`medication.ended_on` ≥ `started_on`; `insurance.expires_on` ≥ `effective_on`;
`equipment.service_due_on` ≥ `serviced_on`; `symptom.resolved_at` ≥ `occurred_at`;
`immunization.expires_on` ≥ `administered_on`;
`family_member.death_year` ≥ `birth_year`, both within `1850..2200` (FR-054).

**Rationale.** FR-004 and FR-013 require batch reporting; the spec's Edge Cases section repeats it
("every offending value in one submission is reported together rather than one at a time").
Shared design contract §6.2 names the domain as the validation authority and forbids validation in
handlers, templ components, repositories and PocketBase hooks.

**Alternatives considered.** PocketBase field constraints as the primary check: they fail one at a
time, produce PocketBase's error shape rather than MediKube's envelope, and cannot express
cross-field rules. They remain the third layer — the last line of defence, never the only one.

---

## D-05 — Status views and "due/expiring" lists are query parameters, and every row states its basis

**Decision.** No bespoke endpoints. Each kind declares its narrowing vocabulary on its registry
entry (`records.Filters`), and the generic list handler validates the query against it:
`?status=`, `?severity=`, `?from=`, `?to=`, `?tags=`, `?match=any|all`, `?q=`,
`?expiring_within_days=` (insurance, default 60), `?service_due_within_days=` (equipment, default
30), `?unresolved=true` (injury), `?scheduled=true` (procedure), `?active=true` (allergy,
condition, medication, treatment).

Two mechanisms answer "state the basis":

1. **Envelope echo** — every list response carries `"criteria"`, the server's resolved narrowing,
   which the page renders as removable chips.
2. **Per-row basis** — `Summary.basis: []string` is populated by the kind's `Basis()` function for
   narrowings where rows qualify for materially different reasons. Equipment returns `overdue` or
   `due_soon`; insurance returns `expiring`; procedure returns `scheduled` or `ordered`.

**Rationale.** FR-079 requires that every status view be expressible as a narrowing of the kind's
own list and return nothing the equivalent hand narrowing would not. FR-026, FR-046, FR-049 and
FR-078 each require the basis to be stated; FR-049 specifically requires distinguishing *overdue*
from *due within thirty days* per row. The shared design contract §2.2 already deleted upstream's
fifteen specialised filter endpoints for the same reason: "upstream returned the plain DTO from
those routes anyway, so the client could not tell *why* a row matched."

**Alternatives considered.** Separate `/active`, `/scheduled`, `/expiring` sub-routes: 15+
operations, each duplicating the list DTO, and FR-079 explicitly forbids a view that differs from
the equivalent narrowing. Rejected.

---

## D-06 — Deterministic ordering, and where undated records go

**Decision.** Every kind declares a default sort on its registry entry. It is
`ORDER BY (<primary_date> IS NULL) ASC, <primary_date> DESC, id DESC`. The tie-break is the
PocketBase 15-character `id`, descending — opaque, unique, stable, and never a wall-clock value.
Records whose primary date is null sort **last** in every default ordering and are rendered in the
timeline under an explicit "Date not recorded" group rather than at either extreme.

Primary dates: allergy `onset_on`; condition `onset_on`; encounter `occurred_on`;
procedure `occurred_on`; treatment `started_on`; symptom `occurred_at`; vitals `recorded_at`;
immunization `administered_on`; injury `occurred_on`; insurance `effective_on`;
equipment `prescribed_on`; medication `started_on`.

Two kinds have no meaningful date and declare a bespoke default sort:
`emergency_contact` → `is_active DESC, is_primary DESC, LOWER(name) ASC, id DESC` (FR-051 requires
current before non-current, primary first); `family_member` → `relationship ASC, LOWER(name) ASC,
id DESC`.

**Rationale.** FR-007 requires a documented tie-break so ordering is identical on every request;
FR-077 requires an undated entry to be *stated* rather than placed arbitrarily; the spec's Edge
Cases section demands both. Cursor pagination (shared design contract §6.3) encodes
`(sort keys, last values, last id)`, never an offset, which is what makes FR-007's
"neither repeats nor skips while records are added or removed" true.

**Alternatives considered.** `created DESC` as the tie-break: two records created in the same
millisecond tie, and PocketBase's autodate resolution is not guaranteed finer. Rejected.

---

## D-07 — Six multi-relation link fields, one payload-carrying join, and no link tables

**Decision.** Relationships are PocketBase `RelationField{MaxSelect: 0}` (multi) fields, edited by
`PATCH /api/v1/records/{kind}/{id}` and read from the other end by **back-relation traversal**
(`treatments_via_medications`). The six fields that carry a link are:

| Owning kind | Field | Points at |
|---|---|---|
| `allergy` | `medications` | medications (FR-017) |
| `condition` | `medications` | medications |
| `encounter` | `lab_results` | reserved for phase 004; declared, empty in 003 |
| `injury` | `conditions`, `medications`, `procedures`, `treatments` | four fields (FR-042) |
| `symptom` | `conditions`, `treatments`, `treated_by_medications`, `caused_by_medications` | FR-032 |
| `treatment` | `encounters`, `equipment` | FR-028 |

`treatment_medications` is the **one** real join collection, because FR-060/061 give the edge a
payload. Everything else is a field.

The symptom↔medication role (FR-032: "treats it" vs "suspected of causing it") is **two fields,
not a payload column** — `treated_by_medications` and `caused_by_medications`. That is the whole
of upstream's `relationship_type`, expressed in the schema where it can be indexed and where a
row cannot be in an undefined role.

**Rationale.** Shared design contract §1.1 cut #1: upstream had 17 link tables, 121 fields and
~44 endpoints, of which 12 carried only a `relevance_note` that upstream's own bulk-create DTOs
prove is per-*operation*, not per-*pair*. FR-055 ("editable from either end, recorded once") and
FR-058 ("the other record survives, the relationship disappears from both sides") are both native
PocketBase relation behaviour — a relation field is one row of truth and PocketBase's delete path
cleans references — so building link tables would be building a bug.

**Alternatives considered.**

- *A generic polymorphic `record_links` table*: needs `(from_kind, from_id, to_kind, to_id)` with
  no foreign keys, loses cascade cleanup, and reintroduces the exact orphan class the shared
  design contract accepted for `attachments` only under protest. Rejected.
- *A payload column on every link*: FR-055 says relationships carry no content, with one named
  exception. Rejected.

---

## D-08 — The same-patient invariant is checked server-side against stored values, and its refusal discloses nothing

**Decision.** `internal/domain/clinical/link.go` provides
`SamePatient(subject, targets []PatientRef) error`. Every link mutation resolves the *stored*
`patient` of every target id and compares it to the subject's stored `patient`. A mismatch, a
non-existent target, and a target the actor cannot reach all produce the **identical**
`ErrNotFound` → `404 not_found`, with the offending id echoed back **only** as the id the caller
already supplied and no other information.

**Rationale.** FR-057 requires the refusal to disclose nothing about the other record; US6
scenario 3 and SC-004 make it a measured outcome. Constitution VII makes existence itself PHI, and
shared design contract §2.1 rule 13 fixes 404 (not 403) as the answer for anything patient-scoped.
Deciding from the *stored* patient rather than a submitted one is FR-081 verbatim: authorization is
never inferred from a client-supplied value.

**Alternatives considered.** `409 conflict` with "records belong to different patients": tells an
attacker that the id exists and belongs to somebody. Rejected outright.

---

## D-09 — `effective_*` resolution for course medications is computed in the service and carries provenance

**Decision.** `GET /api/v1/treatments/{id}/medications` returns, per link row:
`effective_dosage`, `effective_frequency`, `effective_duration`, `effective_timing`,
`effective_prescriber`, `effective_pharmacy`, `effective_started_on`, `effective_ended_on`,
each accompanied by a `*_source` of `course` | `medication` | `none`. The resolution is
`COALESCE(link.value, medication.value)` computed in `internal/service/coursemedication`, never in
SQL and never in the browser.

`PUT /api/v1/treatments/{id}/medications/{medicationId}` is an idempotent upsert inside
`app.RunInTransaction`, guarded by a unique index on `(treatment, medication)` (FR-061).

**Rationale.** FR-060 requires the fallback *and* requires the screen to say which value came from
where — a bare COALESCE cannot, because it loses provenance. Shared design contract §1.5 calls
this "the single most interesting piece of derived logic upstream had" and keeps it. Computing it
in the service keeps it testable without a database and keeps it out of a CSP-restricted browser.

**Alternatives considered.** SQL `COALESCE` in the repository: provenance would need a second
`CASE` per field (16 expressions), and the rule would be untestable without a database.

---

## D-10 — Tags are a normalised `tags` collection with a relation field on every kind

**Decision.** `tags{owner, name, color}` with a unique index on `(owner, LOWER(name))`. Every
clinical kind carries `tags RelationField{CollectionId: tags, MaxSelect: 0}` — **not** a string
array. Rename is one row update (FR-065, SC-007). Delete is one row delete; PocketBase's relation
cleanup removes it from every referencing record and destroys none of them (FR-066).
`GET /api/v1/tags` serves list, autocomplete (`?q=`) and popularity (`?sort=-usage`) in one
operation, with `usage_count` computed per tag.

Filtering: `?tags=a,b&match=any|all` (FR-067). `any` is a disjunction of relation containments;
`all` is a conjunction. Both are built by `internal/store/filter.go` from a typed value — the
PocketBase filter DSL never reaches the wire (shared design contract §2.1 rule 7).

**Rationale.** FR-062–FR-068 in full. Shared design contract §1.2 `tags`: the normalised form
"kills upstream's rename, replace, delete, color and the O(all-rows) string-array rewrites".
SC-007 (500 records across ≥8 kinds, renamed in one action, none losing the tag) is only
achievable with a relation.

**Alternatives considered.** A `[]string` field per record (upstream's shape): a rename is an
O(all rows) rewrite across 14 collections, case-insensitive uniqueness is unenforceable, and
SC-007 fails by construction. Rejected.

---

## D-11 — Search is a maintained `search_index`, matched with `LIKE`, ordered by date, grouped by kind

**Decision.** `search_index{patient, kind, record_id, title, body, occurred_on, tags}` is written
by the same post-commit hooks that write the audit row, registered by `records.Register`. Each kind
declares `SearchFields(entity) (title, body string)` — the fields FR-069 calls "the details of each
type that carry identifying meaning, together with notes".

`GET /api/v1/search?q=&patient=&kinds=&tags=&match=&from=&to=&status=&limit=&cursor=` returns
**one group per kind, each group carrying its own `items`, `next_cursor` and `has_more`**.
Matching is `LIKE '%term%'` over `title` and `body`. Ordering within a group is
`occurred_on DESC, id DESC`. **MediKube does not claim relevance ranking.**

`?patient=` is mandatory; its absence is `400 patient_required` and there is no fallback to the
active patient.

**Rationale.** FR-069–FR-075. FR-073 explicitly forbids claiming relevance ordering, which removes
the only reason to want FTS5: with results ordered by `occurred_on DESC, id DESC` there is nothing
for `rank` to do, so an FTS5 virtual table would buy a raw SQL migration, a second maintenance path
and a `reindex` rebuild command in exchange for a column this phase must not read. **Availability
is not part of the reason.** Risk **R3** is CLOSED (VERIFIED-SOURCE-FACTS FACT 11: FTS5, `MATCH`
and `rank` all work in `modernc.org/sqlite` v1.57.0, the version PocketBase v0.40.1 pulls); an
earlier draft of this section cited it as "unverified", which the dossier refutes and which
overrides this document (corrected 2026-08-27, ANALYSIS N12). `LIKE` is chosen on cost against a
requirement, not on doubt about the driver. FR-072's per-group paging is the correctness fix for upstream's single global pagination
block over a per-type limit. FR-070's mandatory patient is shared design contract §2.1 rule 4 and
§6.6.

**Why an index at all** is argued in `plan.md`'s Complexity Tracking: a 14-way `UNION ALL` cannot
meet SC-003 at 50,000 records.

**Alternatives considered.**

- *FTS5*: available (R3 CLOSED, FACT 11) and rejected on cost — it buys ranking FR-073 forbids,
  and charges a raw SQL migration, a separate maintenance path and a rebuild command for it.
  Revisit only if a future spec asks for ranking.
- *Search across every patient an account can reach*: the spec's Decisions section rejects it —
  "keeps the permission decision to a single subject". Rejected.

---

## D-12 — FR-075: the search term is a first-class secret

**Decision.** The term is never written to a log line, a span attribute, a metric label, a Sentry
event, an audit row or a URL that reaches a log. Concretely: the search handler logs
`term_len` and `result_count`, never `term`; the span carries `medikube.op=search` and
`medikube.result`, never the term; the metric is `medikube_records_search_total{outcome}` with no term
dimension; the audit row for a search is `action=read_sensitive, target_kind=search, target_id=""`.
Both values exist by the time that row is written: `read_sensitive` is declared by phase 001's
`audit_events` migration and `search` by this phase's `audit_vocab` migration (D-19), and the
vocabulary test asserts both.
`internal/testsupport/phileak` asserts it.

**Rationale.** FR-075 and SC-012 state it; Constitution VI bounds metric labels and Constitution
VII forbids free text of any kind in logs, traces, metrics and Sentry. A search term in a medical
application is a diagnosis the user typed.

---

## D-13 — The cross-kind timeline reads `GET /api/v1/records`, and gets one page

**Decision.** No new API operation. `/timeline?patient=&kinds=&from=&to=&tags=&match=` is a page
that renders `GET /api/v1/records` — the cross-kind list phase 001 already built for exactly this
("dashboard, timeline, report picker", shared design contract §2.2). Entries are ordered by each
row's primary date descending with `id DESC` as tie-break; undated rows render in a trailing
"Date not recorded" group (D-06). The narrowing is shown as removable chips and is reflected in
the URL query string.

`/timeline` is the one page this plan adds beyond the shared design contract's 57. It is recorded
as a deviation in `plan.md`.

**Rationale.** FR-076/077. The dashboard `/` is phase 001's page with landmark
`region[name="Overview"]` and a fixed purpose; hanging a filterable cross-kind chronology off it
would overload both. One page, two smoke cases, zero operations is the cheapest honest answer.

**Alternatives considered.** A `GET /api/v1/timeline` operation: it would return the same rows,
with the same filters, from the same table, under a second `operationId`. Principle I rejects it.

---

## D-14 — Symptom aggregates are derived at read time, never stored

**Decision.** `SymptomSummary` carries `episode_count` and `last_occurred_at` for the row's
*symptom name group*, computed by a correlated aggregate in `internal/store/symptom/repo.go`
(`GROUP BY patient, LOWER(name)` joined back to the page), backed by an index on
`(patient, name, occurred_at DESC)`. Nothing is stored, nothing is incremented, nothing can go
stale.

**Rationale.** FR-030 (a new episode each time, no definition first), FR-031 (derive count and
most-recent date, "without either being stored as a maintained count"), FR-090 (correct at 50,000
records), SC-016 (correct on 100% of readings *including immediately after a delete*). The spec's
Decisions section names upstream's two-level model as the thing being deleted: "stored counts that
can quietly go stale."

**Alternatives considered.**

- *A `symptom_definitions` header row with maintained counters*: upstream's model. It is the only
  two-level model in the application, it obliges the user to define before recording, and SC-016's
  "immediately after an episode is deleted" is exactly where maintained counters fail. Rejected.
- *A materialised view refreshed by a cron*: stale between refreshes, which SC-016 forbids.

---

## D-15 — Vitals: SI storage, bounded ranges, presentation-edge conversion, derived BMI

**Decision.** Every measurement is stored in one canonical SI form: `weight_kg`, `height_cm`,
`temperature_c`, `glucose_mmol_l`. Conversion to and from the account holder's `unit_system`
happens in `internal/web` at the edge, using `internal/domain/clinical/units.go`; **no conversion
happens in the service, the repository or the database**. BMI is derived at render time from the
same row and is never stored.

Every numeric field carries a documented plausible range, checked in `Validate()` and reported
with the range named (FR-035): `systolic_mmhg 40..300`, `diastolic_mmhg 20..200`,
`heart_rate_bpm 20..300`, `respiratory_rate_bpm 4..80`, `temperature_c 25..45`, `spo2_pct 50..100`,
`weight_kg 0.5..450`, `height_cm 30..272`, `glucose_mmol_l 0.5..60`, `hba1c_pct 2..20`,
`pain_scale 0..10`.

Two cross-field rules: **at least one measurement must be present** (FR-034 — "a reading of
nothing is not a reading"), and **systolic and diastolic are both-or-neither with
`diastolic < systolic`** (FR-036).

**Rationale.** FR-033–FR-037. Shared design contract §1.5 notes "upstream had zero numeric bounds
on any vital" — the ranges are MediKube's, chosen and documented. US3 scenario 6 requires two
household members with different unit preferences to see the same underlying reading in their own
units with neither view altering what was recorded; that is only true if storage is canonical and
conversion is at the edge.

**Alternatives considered.**

- *Store the value with its unit*: every comparison, every trend and every range check needs a
  conversion first, and FR-089's 50,000-record list would convert per row per request.
- *Convert in the service*: the service would need the viewing user's preferences, which makes it
  depend on presentation. Principle II's single-responsibility clause rejects it.

---

## D-16 — "At most one primary" is a transactional displacement that explains itself

**Decision.** `insurance.is_primary` (FR-045) and `emergency_contact.is_primary` (FR-051) are
enforced by a service-layer displacement inside `app.RunInTransaction`: setting a record primary
clears the flag on the previously primary record of the same patient, and the response DTO carries
`displaced: {"id": "...", "kind": "insurance"}` so the UI can say *what changed* rather than
silently applying it. A partial unique index `(patient) WHERE is_primary = 1` is the storage-level
backstop.

**Rationale.** FR-045 and FR-051 both require the change to be *explained*, and US5 scenario 2 and
US1 scenario 6 make it an acceptance scenario. A PocketBase index alone would produce a
constraint error, not an explanation.

**Alternatives considered.** Refusing the second primary and telling the user to unset the first:
two round trips for one intent, and the spec describes displacement, not refusal.

---

## D-17 — Pre-delete reference counts come from the detail DTO, not a new endpoint

**Decision.** `GET /api/v1/records/{kind}/{id}` returns
`"references": {"total": N, "by_kind": {"medication": 2, "symptom": 1}}` — the count of records
that point at this one, resolved by back-relation traversal and by the `treatment_medications`
join. The delete-confirmation dialog is rendered from the detail the UI already holds. No
operation is added.

**Rationale.** FR-006 requires the confirmation to state what will be destroyed and how many other
records refer to it; the spec's Edge Cases section repeats it. Principle I forbids a second
operation for data the first one can carry.

**Alternatives considered.** `GET /api/v1/records/{kind}/{id}/references`: a 91st operation for a
field. Rejected.

---

## D-18 — `family_member.conditions` and `insurance.coverage`/`contact` are validated Go structs in a `json` field

**Decision.** Three value-object lists are stored as PocketBase `JSONField` and validated by typed
Go structs with `Validate()`:
`[]FamilyCondition{name, icd10_code, diagnosed_age, severity, status, notes}` (FR-053);
`Coverage{deductible, oop_max, copay_primary, copay_specialist, copay_er, coinsurance_pct,
currency}` (FR-044); `Contact{phone, claims_phone, website, portal_url, address}` (FR-044).
They are **not** free-form blobs and are **not** collections.

**Rationale.** Shared design contract §1.5: they are read only with their parent, never queried,
never shared independently, never linked to. A collection buys referential integrity nobody uses
and costs a join on every read. Typed structs with `Validate()` are what separates this from
upstream's untyped blobs. `currency` being explicit is FR-044 verbatim ("each with a stated
currency").

**Alternatives considered.** A `family_conditions` collection: a fourth collection, a fourth CRUD
surface and a join, for a list that has no independent life.

---

## D-19 — The audit vocabulary is extended additively; content never enters it

**Decision.** `audit_events.target_kind` gains `tag` and `search`. It does **not** gain the
thirteen new kinds and `action` does **not** gain `access_denied`: phase 001's migration declares
the shared design contract's complete vocabulary — all fifteen record kinds, and `access_denied`
alongside `read_sensitive` (001 research D-20) — so both are already there when this phase runs,
and this migration's test asserts the **complete** twenty-one-action / twenty-seven-target-kind
set rather than this delta. Audit rows are written from post-commit hooks bound by
`records.Register` (`OnRecordAfterCreateSuccess` / `…UpdateSuccess` / `…DeleteSuccess`), plus
explicit `audit.Record(...)` calls for refused access, which happens in `internal/service/access`
where the refusal is decided.

**No content, ever**: no diagnosis, no medication name, no measurement, no member number, no note,
no tag name, no search term (FR-085, SC-011).

Applying or removing a tag is an `update` on the record — the audit row carries the record's id
and kind and no field values, which is what FR-084 asks for and what FR-085 requires.

**Rationale.** FR-084/085 and SC-011. Shared design contract §6.4: "Populated by hooks, not by
handlers… post-commit, so a rolled-back transaction never produces an audit row" and
"`OnRecord*Request` hooks are forbidden — they are bound inside the built-in CRUD handlers, which
the lockdown disables, so anything placed there is silently dead code" (VERIFIED-SOURCE-FACTS
FACT 2 explains why the lockdown disables them).

---

## D-20 — Live lists: the thirteen kinds join the existing SSE stream, and the 5-minute trap gets a test

**Decision.** `records.Register` binds each kind's post-commit hooks to publish
`realtime.Event{Kind, RecordID, PatientID}` — **IDs only, never bodies** — to the in-process hub.
`GET /api/v1/streams/records` (phase 001) re-fetches each id, **re-authorises it for that
subscriber**, renders the kind's `Row` component and patches it by
`ids.RecordRow(kind, id)`. No new stream endpoint.

Two regression assertions this phase adds:

1. `internal/web/stream/deadline_test.go` — asserts the stream handler was constructed by
   `newStream()` and that the `*http.Server` `WriteTimeout` override is in place, because
   PocketBase's hardcoded 5-minute `WriteTimeout` kills every long-lived SSE stream and **passes
   every test shorter than five minutes**.
2. `internal/realtime/authz_test.go` — asserts that a subscriber whose access to a patient is
   removed stops receiving that patient's events on the next publish, because re-authorisation
   happens per event and not at subscribe time.

**Rationale.** FR-010 ("continue doing so for a list left open for at least an hour") and SC-017
("a view left open for 60 continuous minutes is still receiving updates") are the user-visible form
of the trap. Constitution V mandates the ID-only fan-out precisely so per-subscriber
re-authorisation is possible; Constitution VII requires it. Shared-design risk **R7** assigns the
>5-minute CI job to phase 006 and the helper to 001; this phase owns the per-kind assertion in
between.

**Alternatives considered.** PocketBase's native realtime: unusable for three independently fatal
reasons verified in v0.40.1's source (subscription rules derive from `ViewRule`/`ListRule`, which
the lockdown sets to `nil`, so every broadcast is silently dropped; its wire format is not one of
Datastar's two recognised event names; its two-step handshake is impossible from a Datastar
attribute). Forbidden by the constitution.

---

## D-21 — DTOs under Go 1.27 `encoding/json/v2`

**Decision.** Every new DTO gets a round-trip test asserting: slices marshal as `[]` and never
`null` (`tags`, `basis`, `conditions`, back-relation arrays); unknown request fields are **rejected**
with `422 validation_failed`; duplicate JSON keys are rejected; `*_on` marshals as `YYYY-MM-DD` and
`*_at` as RFC3339 UTC; every `Create` and `Patch` DTO **omits every server-owned field by
construction** (`id`, `patient` on patch, `created`, `updated`, `version`), which is how privilege
escalation is prevented structurally rather than by a check.

**Rationale.** Constitution's Technology Constraints require budgeting for json/v2 retrofit
semantics; shared-design risk **R2** records that nil-vs-empty slices, `json.RawMessage`,
duplicate-key rejection and case-insensitive matching are not fully backward compatible, and that
`tests.ApiScenario` normalises bodies through `jsontext` before substring matching — so
`ExpectedContent` compares against *re-encoded* JSON. That last point changes how every HTTP test in
this phase is written and is called out in `quickstart.md`.

---

## D-22 — Scale: what makes 50,000 records answer in two seconds

**Decision.** Per-kind index `(patient, <primary_date>, id)` matching the default sort exactly, so
the ordering is index-satisfied and the cursor is a range scan rather than an offset. Plus:
`search_index(patient, kind, occurred_on, id)`; `symptom(patient, name, occurred_at)` for D-14;
`treatment_medications(treatment, medication)` unique; `tags(owner, LOWER(name))` unique;
partial unique `(patient) WHERE is_primary = 1` on insurance and emergency_contact.
List queries never `COUNT` unless `?count=true` is passed.

Verification is a build-tagged test, not a hope: `go test -tags=scale ./internal/store/...` seeds
50,000 records with `internal/testsupport/scale` and asserts the SC-002 and SC-003 budgets. It runs
nightly, not on every push.

**Rationale.** FR-089, FR-090, SC-002, SC-003. Shared design contract §2.1 rule 5: "a total is
returned only when `?count=true` is passed, because a COUNT over a shared patient's chart is not
free."

**Alternatives considered.** Asserting the budget in the normal test run: a 50,000-row seed per run
makes the suite unusable, and Principle VIII's "a flaky assertion is fixed or removed, never
retried into passing" applies to slow ones too.

---

## D-23 — Thirteen kinds, one contract suite

**Decision.** `internal/records/recordstest` provides two `testify/suite` suites run against every
implementation:

- `RepositoryContract(t, factory)` — Get/List/Save/Delete semantics, cursor stability under
  concurrent insert, `ErrNotFound` on a foreign patient, version/If-Match behaviour, cascade on
  patient delete. Run against each of the 13 real repositories **and** each of the 13 fakes.
- `KindContract(t, kind, harness)` — registration completeness, DTO round-trip, default sort,
  authorization matrix (owner succeeds / stranger 404 on all six operations), audit row emitted
  with no content, search-index row written and removed, realtime event published with IDs only.

**Rationale.** Constitution Principle II's Liskov clause: "Every implementation of an interface
MUST satisfy the same contract tests. Where a contract test cannot be written to cover all
implementations, the interface is wrong." Principle III makes the suites mandatory. Practically,
this is what stops thirteen kinds costing thirteen times the test effort, and it is what makes a
fourteenth kind cheap.

**Why now and not in phase 001**: Principle I forbids an abstraction with one implementation.
Phase 001 had one clinical kind. This phase has fourteen.

---

## D-24 — FR-094 / SC-012 is a test, not a promise

**Decision.** `internal/testsupport/phileak` seeds an instance whose clinical values are
recognisable sentinels (`ZZALLERGEN`, `ZZDIAGNOSIS`, `ZZMEMBERNO`, `ZZNOTE`, `ZZTAG`, `ZZTERM`,
…), drives **every operation this phase defines**, and then asserts zero sentinel occurrences in:
the captured zerolog stream, `prometheus.Gatherer` output (names *and* label values), the OTel
`tracetest.SpanRecorder`, and a stub Sentry transport. It fails on the first occurrence and names
the sink.

**Rationale.** FR-094 requires "an automated exercise of every operation this phase defines, which
then inspects the installation's diagnostic output and finds no clinical or identifying content in
it". SC-012 makes it a measured outcome. Constitution VII says "Remember not to log this" is not a
control — this is the control.

---

## Risks carried into this phase, and their disposition

| Risk | Disposition here |
|---|---|
| **R1** discriminated `oneOf` generation | Assumed **closed by phase 001**. Re-asserted by `registry_completeness_test.go`. If it is open, this phase is blocked before task T001 — see D-01. |
| **R2** json/v2 retrofit semantics | Addressed by D-21: a round-trip test per DTO, and `ApiScenario` expectations written against re-encoded JSON. |
| **R3** FTS5 availability | **CLOSED upstream** (VERIFIED-SOURCE-FACTS FACT 11; SHARED-DESIGN §8) — FTS5 *is* available. Made **moot** here regardless by D-11: FR-073 forbids relevance ranking, so `LIKE` over `search_index` is chosen deliberately on cost, not as a fallback and not for want of a working FTS5. |
| **R7** the >5-minute SSE liveness test | Phase 003 adds the per-kind stream assertions and the `newStream()` construction assertion (D-20). The genuinely long-running CI job remains phase 006's. |
| **R8** PocketBase upgrade fragility | Unchanged. This phase adds no new dependency on unexported internals. The upgrade checklist gains one line: re-run `registry_completeness_test.go` and the per-collection lockdown scenarios. |
| **R12** ETag/If-Match friction in Datastar forms | Proved on medications in 001; this phase applies it to thirteen kinds. The version travels as a hidden `data-bind` signal and is echoed in `If-Match`; a `412` renders the current values into the form (FR-005, SC-009). |
