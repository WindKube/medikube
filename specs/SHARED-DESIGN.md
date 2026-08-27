# MediGo — THE Shared Cross-Phase Design Contract

**Status: BINDING ON DESIGN.** This document is authoritative on the **design** — the domain
model, the record route family and the API conventions, the UI shell and its landmark strings, the
package layout, the seams and the cross-cutting conventions. Every specification, plan and task
list for phases 001–006 is written against it, and a later document that disagrees with it on any
of those is wrong.

**It is NOT authoritative on allocation.** *Which phase ships what* is settled by the six accepted
phase charters under `/Users/krzysztof.wiatrzyk/private/monorepo/medigo/specs/`. Where §0, §1.2,
§1.3, §1.5, §1.6, §2.3 or §3.1 disagrees with a charter about the phase a collection, an operation
or a page belongs to, **the charter wins and this document is corrected**. Those sections were
rewritten to the accepted allocation on **2026-08-27** and now carry it; the `foundation` /
`reference-and-catalogs` split this document originally described is dead and no phase implements
it.

Where this document disagrees with
`/Users/krzysztof.wiatrzyk/private/monorepo/medigo/.specify/memory/constitution.md`, the
constitution wins.

**Inputs, in precedence order:** the constitution (v1.3.0) → `VERIFIED-SOURCE-FACTS.md` → the
six accepted phase charters → `HOUSE-PATTERNS.md` → `RECONCILIATION.md` → the domain and library
dossiers.

**The roll-up arithmetic is computed HERE, once — in §1.6, §2.3 and §3.1. Every plan, data model
and contract MUST cite these figures and MUST NOT re-derive them.** Four plans derived them
independently and no two agreed. A phase document stating a running total that this document does
not state is a defect, and Principle IX gates the route, page and OpenAPI inventories against these
numbers.

**Headline numbers.**

| | Count | Was | Why it moved |
|---|---|---|---|
| Collections (entities) | **30** | 31 | `catalog_vaccines` dropped — §1.3, §1.6 |
| `/api/v1` operations across all six phases | **94** | 90 | −1 `catalog/vaccines`, +5 new (91–95); the three formerly deferred auth operations are **allocated**, not dropped — §2.3 |
| Page routes | **58** (+3 error views) | 56 | +`/timeline`, +`/invite/{token}`, and the three auth pages are **built by phase 001**, not deferred — §3.1 |
| Page-action routes (neither pages nor API) | **7** | — | 004 adds 4, 006 adds 3 — §3.1 |
| Clinical record kinds riding one route family | **15** | 15 | unchanged — the budget holds |
| PocketBase collections whose CRUD API is public | **0** | 0 | unchanged |

---

## 0. The six phases

**This table is the accepted allocation and it is the one the six charters implement.** It replaces
the allocation this document originally carried.

| Phase | Name | Owns |
|---|---|---|
| **001** | `walking-skeleton` | Platform, config, zerolog + the two-mechanism PocketBase log bridge, observability, the CRUD lockdown, PB-native auth behind MediGo DTOs, accounts (`users`), the audit-trail collection, the record-kind registry with **medications** as its single registered kind end to end, the route registry + OpenAPI gate, the Datastar SSE bridge, the Playwright gate and its two negative controls |
| **002** | `patient-core` | Patients, ownership, active-patient switching, the patient chart and photo, the re-anchoring of medications onto `patient` — and the reference entities: practitioners, facilities (practices ∪ **pharmacies** ∪ hospitals ∪ labs ∪ imaging), and the practitioner **specialty** vocabulary as a fixed Go enum rather than a collection |
| **003** | `clinical-records` | The remaining thirteen record kinds, the multi-relation link model, the one payload-carrying join, tags, the search index and unified search, the timeline |
| **004** | `labs-and-attachments` | Lab results as the fifteenth kind, lab components and trends, the standardized lab-test catalogue, attachments (upload / serve / thumbnail / trash / restore) |
| **005** | `sharing-and-collaboration` | The single resource-generic `shares` collection, the invitation state machine, the notifications stream, and the **widening of authorization** across every endpoint and page phases 001–004 already shipped |
| **006** | `reporting-and-operations` | Reports, saved report templates, asynchronous export and download, the operator surface, the activity-trail reader, PB-backed backup/restore, the retention purges, release hardening and the whole-product browser sweep |

A phase ships when its acceptance scenarios pass as tests, its routes appear in the committed
OpenAPI document, its pages pass the Playwright gate at both viewports, and CI is green.

**What moved, relative to the allocation this document first published.** Medications 003 → 001
(the one kind that proves the registry end to end). Patients, multi-patient switching and the
patient chart 001 → 002. Allergies and emergency contacts 001 → 003. Tags 002 → 003. The search
index, unified search and the timeline 004 → 003. `catalog_lab_tests` 002 → 004. `catalog_vaccines`
dropped entirely (§1.3). Password reset and email verification are **built by phase 001** and
external identity providers by **phase 006** (§2.3, §3.1): an earlier revision of this document
deferred all three out of the suite, which left three operations and three pages owned by nobody
(cross-artifact finding **H7**) and left a self-hosted medical instance whose only password
recovery was a superuser editing the database. Recovery and confirmation are PocketBase-native and
belong where authentication is built; external sign-in needs provider configuration and belongs
with the operator surface that has it. Nothing about the *design* changed: the same
collections, the same route family, the same conventions, in a different order.

---

## 1. THE DOMAIN MODEL

### 1.0 Rules that hold for every collection

1. **Ids are PocketBase's 15-character opaque text ids.** No integers anywhere. Never store an
   identifier in a PocketBase `number` field (float64, ~2^53 safe).
2. Every collection has `id`, `created` (autodate), `updated` (autodate). They are not repeated
   in the tables below.
3. **Every patient-scoped collection has `patient`** — `RelationField{CollectionId: patients,
   Required: true, MaxSelect: 1, CascadeDelete: true}`. There is no other route to a patient.
4. **Every one of the five API rules on every collection is `nil`.** Superuser-only. Asserted at
   boot; MediGo refuses to start if any non-system collection has a non-nil rule. `AuthRule` is
   *not* one of the five and stays `""` on `users`, or PB native auth dies (VERIFIED-SOURCE-FACTS
   FACT 2).
5. **Every `FileField` is `Protected: true`.** Asserted at boot; MediGo refuses to start
   otherwise. Only two collections have file fields — `patients.photo` and `attachments.file` —
   which makes the assertion trivially auditable.
6. Enum fields are `core.SelectField{MaxSelect: 1}` **and** a Go string type with a `Valid()`
   method in `internal/domain`. The two are generated from one Go source of truth so they cannot
   drift. **All enum values are `snake_case`.** MediGo *chooses* these vocabularies; it does not
   inherit them (only six of upstream's were ever declared).
7. `notes` is `text`, max 5000, optional, on every clinical kind. It is PHI and is redacted in
   marshalling for logs.
8. `tags` is `RelationField{CollectionId: tags, MaxSelect: 0}` on every clinical kind. **Not** a
   string array.
9. **No `deleted_at` on any record collection.** Records are hard deleted. Soft delete exists on
   `attachments` only (constitution VII).
10. Dates: a clinical event date that has no meaningful time is `date` (stored as a date-only
    text field, `YYYY-MM-DD`). Anything with a real instant is `autodate`/`date` in RFC3339 UTC.
    Upstream's split of `occurrence_date` + `occurrence_time` into two columns is collapsed to
    one `occurred_at`.
11. Uniqueness is a collection index (`AddIndex`), because PocketBase has no per-field `Unique`.

### 1.1 Where MediGo collapses upstream — the seven big cuts, stated plainly

| # | Upstream | MediGo | Why |
|---|---|---|---|
| 1 | 17 link tables, 121 fields, ~44 endpoints | **6 multi-relation fields + 1 real join collection** | 12 of the 17 carried only `relevance_note`, which upstream's own bulk-create DTOs prove is per-*operation*, not per-*pair*. PocketBase multi-relations + back-relation traversal (`treatments_via_medications`) express the rest natively. Only `treatment_medications` (8 payload fields) survives as a collection. |
| 2 | `symptoms` (definition) + `symptom_occurrences` (episodes), 34 fields, two-level | **one `symptoms` collection, one row per episode** | It was the only two-level model in the app. "Occurrence count" and "last occurrence" are `GROUP BY name`, not columns. `time_of_day` is deleted outright — it duplicates `occurred_at`. |
| 3 | `practices` (17 fields, embedded locations JSON) + `pharmacies` (19 fields, flattened address columns) | **one `facilities` collection with `kind ∈ {practice, pharmacy, hospital, lab, imaging, other}`** | Two modellings of the same six address concepts in one codebase. One shape, one CRUD, one page. |
| 4 | `entity_files` (16 ops, polymorphic, sync state machine) + `lab_result_files` (18 ops) + `/lab-results/{id}/files` (3 ops) | **one `attachments` collection, 6 ops** | Two complete file subsystems, one of them lab-only, plus a Paperless/Papra sync state machine that is out of scope. |
| 5 | `medical_specialties` and `injury_types` as user-extensible FK tables | **select fields with MediGo vocabularies** | A required FK into a create-only, never-readable table (`medical_specialties` has no GET-by-id, no PUT, no DELETE) is not a reference model, it is an accident. |
| 6 | `patient_shares` (10 ops) + family-history shares (10 ops) + `report_templates.is_public`/`shared_with_family` | **one `shares` collection, `resource_kind ∈ {patient, family_member}`, 5 ops** | The product distinction (chart delegation vs pedigree exchange) is real and is preserved as a field. The second router, second bulk-invite, second revoke shape and second remove-my-access are not. |
| 7 | 13 clinical record types × ~5 near-identical CRUD routers, plus 21 legacy `/patients/me` duplicates and 9 `/patients/{id}/{type}` fan-outs | **one `/api/v1/records/{kind}` route family, 6 operations, 15 registered kinds** | See §2.2. This single decision is what makes an 80–120 endpoint budget reachable while keeping every DTO explicitly typed. |

Dropped without replacement, and why: `privacy_level` (nothing writes it, nothing documents it);
`custom_permissions` (free-form JSON nothing reads); `required_permission` as a wire parameter
(client-supplied authorization on 41 upstream operations, defaulting to `view` **on writes**);
`condition_name` (vestigial beside `diagnosis`); `treatment_category` (word-for-word duplicate of
`treatment_type`); `specialty`/`specialty_name` (the same denormalised value twice); stored `bmi`
(derivable from the same row); `mode` on treatments (UI state persisted as domain data);
`Injury.treatment_received` free text (the treatment link is the structured version); the
Paperless/Papra fields; the frontend-log endpoints.

### 1.2 Platform collections

#### `users` — PocketBase **auth** collection, extended. Phase 001.

PocketBase owns `id`, `email`, `emailVisibility`, `verified`, `password`, `tokenKey`. MediGo adds:

| Field | Type | Req | Notes |
|---|---|---|---|
| `name` | text ≤120 | yes | display name |
| `role` | select | yes | `user` \| `admin`. Default `user`. **Never settable through registration or self-update** — the DTOs omit it. `admin` is a MediGo application tier and is *not* a PocketBase superuser. |
| `active_patient` | relation → patients, MaxSelect 1 | no | UI convenience only. **Never consulted for authorization.** Resolves to null when access is gone. |
| `unit_system` | select | yes | `metric` \| `imperial`. Default `metric`. |
| `locale` | text ≤10 | yes | default `en` |
| `date_format` | select | yes | `iso` \| `dmy` \| `mdy`. Default `iso`. |
| `theme` | select | yes | `system` \| `light` \| `dark`. Default `system`. |
| `disabled_at` | date | no | non-null = login refused |

Index: unique `LOWER(email)`. The twelve Paperless/Papra preference fields, `must_change_password`,
`auth_method`, `external_id`, `sso_provider`, `sso_metadata`, `sso_linking_preference`,
`account_linked_at`, `last_sso_login` and the whole `user_preferences` 1:1 table are **not
carried over** — PocketBase's `_externalAuths` and its OAuth2 linking flow replace all of it.

#### `patients` — Phase 002.

| Field | Type | Req | Notes |
|---|---|---|---|
| `owner` | relation → users, MaxSelect 1, CascadeDelete | yes | ownership. Immutable after create except by an explicit transfer operation. |
| `first_name` | text 1..100 | yes | PHI |
| `last_name` | text 1..100 | yes | PHI |
| `birth_date` | date | yes | not future, not >150y ago. **`age` is derived at render time, never stored.** |
| `sex` | select | no | `female` \| `male` \| `intersex` \| `unspecified`. Collapses upstream's seven aliases (`M, F, MALE, FEMALE, OTHER, U, UNKNOWN`) for three concepts. |
| `blood_type` | select | no | `a_pos a_neg b_pos b_neg ab_pos ab_neg o_pos o_neg` |
| `height_cm` | number | no | 30..272. **SI storage**; `unit_system` is applied at the edge. |
| `weight_kg` | number | no | 0.5..450. SI storage. |
| `address` | text ≤500 | no | PHI, single blob (structuring it buys nothing here) |
| `primary_practitioner` | relation → practitioners, MaxSelect 1 | no | practitioners arrive in the same phase |
| `is_self_record` | bool | yes | default false; **partial unique index** `(owner) WHERE is_self_record = 1` |
| `relationship_to_owner` | select | no | `self spouse partner parent child sibling ward other` |
| `photo` | file, MaxSelect 1, **Protected: true**, MimeTypes `image/jpeg,image/png,image/webp`, MaxSize 15MiB, Thumbs `100x100t,400x400f` | no | thumbnails generated **eagerly** on upload (PB's lazy thumbnailer lives in the file route MediGo bypasses) |

#### `tags` — Phase 003.

`owner` (relation → users, cascade, required), `name` (text 1..40, required), `color` (text,
`^#[0-9a-fA-F]{6}$`, optional). Unique index on `(owner, LOWER(name))`.
Rename = one row update. Delete = one row delete, PocketBase's relation cleanup removes it from
every referencing record. This kills upstream's `rename`, `replace`, `delete`, `color` and the
O(all-rows) string-array rewrites.

#### `audit_events` — Phase 001, read surface in Phase 006.

| Field | Type | Notes |
|---|---|---|
| `occurred_at` | date | RFC3339 UTC |
| `actor` | relation → users, MaxSelect 1 | null for system and superuser actions |
| `actor_kind` | select | `user` \| `admin` \| `superuser` \| `system` |
| `action` | select | **Owned by `specs/001-walking-skeleton/data-model.md` §3**, which declares the complete set and names the phase that first writes each value. Do not restate it here — a second copy is how the vocabulary drifted in the first place (amendment 2026-08-27, ANALYSIS C1). |
| `target_kind` | select | Same owner, same rule. |
| `target_id` | text | **≤64.** An opaque id — never a name, never a path, never a filename — with **one bounded exception**: when `target_kind` is `system`, `backup` or `export` there is no record to point at, so it carries the job name or archive name instead. Owned by `001/data-model.md` §3, which states the exception in full; do not restate a narrower rule here (amendment 2026-08-27, ANALYSIS B2). |
| `patient` | relation → patients, MaxSelect 1 | null for non-patient actions; this is what makes per-patient activity views possible |
| `request_id` | text | **Required.** Correlates to the zerolog stream. A background run has no HTTP request and still fills it: cron, job, migration and backfill contexts mint a **run id** from the same helper that mints request ids. Owned by `001/data-model.md` §3 (amendment 2026-08-27, ANALYSIS B1). |

**No content, ever.** No field values, no old/new diffs, no names, no filenames. Constitution VII.
Written from post-commit record hooks (`OnRecordAfterCreateSuccess` / `…UpdateSuccess` /
`…DeleteSuccess`) registered once by the kind registry, plus explicit `audit.Record(...)` calls
for the non-record actions. Retention is a config value; purge is a PB cron.

#### `attachments` — Phase 004.

| Field | Type | Req | Notes |
|---|---|---|---|
| `patient` | relation → patients, cascade | yes | the authorization anchor. Every attachment is patient-scoped. |
| `owner_kind` | select | yes | one of the 15 record kinds |
| `owner_id` | text 15 | yes | the owning record's id |
| `file` | file, MaxSelect 1, **Protected: true**, MaxSize from config (32 MiB default), MimeTypes from config, Thumbs `160x160t,1024x1024f` | yes | |
| `original_name` | text ≤255 | yes | PHI (patients name files after conditions) |
| `size_bytes` | number | yes | |
| `mime` | text | yes | sniffed server-side, never trusted from the client |
| `description` | text ≤500 | no | PHI |
| `category` | select | no | `report lab_result imaging prescription insurance_card correspondence photo other` |
| `deleted_at` | date | no | **the trash.** Non-null = in trash, excluded from every list, purged by a cron after `MEDIGO_RETENTION_TRASH_DAYS`. |
| `uploaded_by` | relation → users, MaxSelect 1 | yes | |

Indexes: `(patient, owner_kind, owner_id)`, `(deleted_at)`.

**Design note, stated because it is a real trade-off.** `owner_kind`+`owner_id` is polymorphic
and therefore has no foreign key, which is exactly the shape criticised in upstream. It is chosen
anyway over fifteen nullable relation columns because: (a) the alternative adds a column per
record kind, which is a schema change on every future kind and 14 always-null columns per row;
(b) the cleanup hook is registered by `records.Register(kind, …)` itself, so it is impossible to
add a kind and forget it; (c) a nightly orphan sweep backstops it; (d) it keeps file fields
confined to two collections, which is what makes the `Protected: true` boot assertion auditable.

#### `search_index` — Phase 003.

`patient` (relation, cascade, required), `kind` (select), `record_id` (text), `title` (text),
`body` (text), `occurred_on` (date), `tags` (relation → tags, MaxSelect 0).
Maintained by the same post-commit hooks that write the audit trail. One query, one
authorization check, deterministic ordering. **MediGo MAY promise relevance ranking** — risk R3 is
CLOSED (VERIFIED-SOURCE-FACTS FACT 11 — FTS5, `MATCH` and the `rank` column all work in
`modernc.org/sqlite` v1.57.0, the version PocketBase v0.40.1 pulls), so the *capability* hedge this
document used to carry is withdrawn — **but MAY is not MUST, and phase 003 declined it.**
`003/spec.md:345` (FR-073) declines relevance ranking outright, which removes the only thing an
FTS5 table buys: with results ordered by date there is nothing for `rank` to do. **The design is
therefore an ordinary `search_index` collection matched with `LIKE`, ordered by date** — see 003's
Deviations table and `003/contracts/search.md` §5. No FTS5 virtual table is created, and **no
`medigo reindex` subcommand exists**; `001/contracts/cli.md` lists serve, migrate, seed, routes,
openapi and healthcheck, and nothing adds a seventh (amendment 2026-08-27, ANALYSIS N12).

Had FTS5 been used it would have been a *separate* table PocketBase's migrations do not model —
created by raw SQL, kept in step by the post-commit hooks, and rebuilt by a `reindex` command. That
is the cost FR-073 makes unnecessary; it is recorded here so a later phase that *does* want ranking
knows what it is buying. **Deleting a record MUST delete its index row** either way, or search leaks
the titles of deleted records — a Principle VII problem.

#### `shares` — Phase 005.

| Field | Type | Req | Notes |
|---|---|---|---|
| `resource_kind` | select | yes | `patient` \| `family_member` |
| `patient` | relation → patients, cascade | conditional | set iff `resource_kind = patient` |
| `family_member` | relation → family_members, cascade | conditional | set iff `resource_kind = family_member` |
| `grantor` | relation → users, cascade | yes | must own the resource at create time |
| `grantee` | relation → users, cascade | yes | |
| `level` | select | yes | `view` \| `edit`. **`full` is dropped** — upstream never defined it and delete is owner-only regardless. Family-member shares are `view`-only, enforced in the service. |
| `note` | text ≤500 | no | survives from family history's `sharing_note`; useful for both kinds |
| `expires_at` | date | no | null = never. **Evaluated in the read path**, not by a sweeper. |
| `revoked_at` | date | no | null = active. Replaces `is_active`; you get "when" for free. |
| `revoked_by` | relation → users | no | distinguishes owner-revoke from grantee-leave |
| `invitation` | relation → invitations, MaxSelect 1 | no | provenance |

Unique partial index on `(resource_kind, patient, family_member, grantee) WHERE revoked_at IS NULL`.

#### `invitations` — Phase 005.

`sender` (relation → users, cascade, required), `recipient_email` (text, email, required —
**MediGo can invite an address with no account yet**, which upstream could not),
`recipient` (relation → users, MaxSelect 1, optional — resolved on accept),
`kind` (select: `patient_share` | `family_history_share`),
`resource_ids` (json — a validated `[]string` of patient or family_member ids; one invitation may
cover several resources, accepted all-or-nothing),
`level` (select: `view` | `edit`), `note` (text ≤500),
`status` (select: `pending accepted rejected cancelled revoked expired`),
`token_hash` (text, required, unique index — the emailed token is never stored in the clear),
`expires_at` (date, required), `responded_at` (date), `response_note` (text ≤500).

The untyped `context_data` blob is replaced by the four typed columns above.
State machine: `pending → accepted | rejected | cancelled | expired`; `accepted → revoked`.
Expiry is a filter in the read path; the cron only tidies.

#### `report_templates` — Phase 006.

`owner` (relation → users, cascade), `name` (text 1..120), `description` (text ≤500),
`criteria` (json — validated Go struct: `{kinds: []Kind, patient: string, from, to: date,
tags: []string, statuses: []string}`), `charts` (json — validated: `{vitals: []{metric, from, to},
labs: []{canonical_name, unit, from, to}}`), `settings` (json — validated: sort, grouping,
include_patient_header, include_photo).

**Templates store criteria, not frozen record ids.** Upstream persisted hard-coded `record_ids`,
so a saved template silently rotted as records were added or deleted. `is_public` and
`shared_with_family` are deleted — if templates ever need sharing they become a third
`resource_kind`, not two booleans.

#### `export_jobs` — Phase 006.

`owner` (relation → users, cascade), `patient` (relation → patients, optional),
`kind` (select: `data_export` | `report`), `format` (select: `json` | `csv` | `pdf` | `zip`),
`scopes` (json `[]Kind`), `template` (relation → report_templates, optional),
`status` (select: `queued running succeeded failed expired`), `progress` (number 0..100),
`error_code` (text — a code, never a message with PHI),
`artifact` (file, MaxSelect 1, **Protected: true**), `bytes` (number),
`expires_at` (date — the artifact is purged by cron), `started_at`, `finished_at` (date).

Exports are asynchronous. Upstream generated a ZIP of everything synchronously inside a request
handler; that is a timeout in a request thread holding a SQLite connection.

### 1.3 Reference collections

#### `practitioners` — Phase 002.
`owner` (relation → users, cascade, required — reference data is per-user, not global),
`name` (text 1..200, required), `specialty` (select — MediGo vocabulary of 42 values, seeded from
the standard specialty list, `other` included), `facility` (relation → facilities, MaxSelect 1),
`phone` (text ≤40), `email` (email), `website` (url), `notes` (text ≤5000).
Unique index `(owner, LOWER(name), specialty)`.

#### `facilities` — Phase 002.
`owner` (relation → users, cascade, required), `kind` (select: `practice pharmacy hospital lab
imaging other`, required), `name` (text 1..200, required), `brand` (text ≤120),
`street` `city` `region` `postal_code` `country` (text), `phone` `fax` (text ≤40), `email` (email),
`website` (url), `portal_url` (url), `hours` (text ≤300), `open_24h` (bool), `drive_through` (bool),
`services` (text ≤500), `notes` (text ≤5000).
Upstream's `PracticeLocationSchema[]` embedded array is dropped: a second location is a second
facility row, which is what every query wanted anyway.

#### `catalog_lab_tests` — Phase 004. Read-only, seeded from a vendored LOINC-derived extract.
`loinc_code` (text, unique index), `name` (text, required), `short_name` (text),
`default_unit` (text), `category` (select — the lab category vocabulary), `synonyms` (json `[]string`),
`is_common` (bool), `ref_low` `ref_high` (number, optional).

#### ~~`catalog_vaccines`~~ — **DROPPED. Not built by any phase.**

**This is a design change, not an allocation change** (amendment 2026-08-27). The collection, the
`immunization.catalog_vaccine` relation and the `GET /api/v1/catalog/vaccines` operation (§2.3 op 45)
are all removed. Phase 003's specification defers a standardized vaccine library in as many words —
*"nothing in this phase's requirements needs it, and adding it here would be work done ahead of a
requirement"* — and no later phase claims it, so under Principle I's YAGNI clause it does not exist.
The field is left **out of the `immunizations` collection entirely** rather than added and left
null; re-introducing it later is one reversible migration plus one route.

Two consequences that must not be lost: the headline collection count is **30, not 31** (§1.6), and
the immunisation history's disease roll-up is a `GROUP BY vaccine_name`, not a
`GROUP BY catalog_vaccine.disease_keys`. Its twin `catalog_lab_tests` was **reallocated 002 → 004
and is delivered** — the two catalogues no longer share a fate, which is exactly why this had to be
written down.

### 1.4 The canonical enum vocabularies (every kind uses them)

MediGo **chooses** these. Only six of upstream's enums were ever declared; the rest lived in
Pydantic validators that FastAPI could not reflect. This is a licence, not a loss.

**Who defines what:** 001 defines `TherapyStatus` plus `medication.type` and `medication.route`
(they ship with the medications kind, so the shape is settled there and named for the *shape*, not
the kind); 002 defines the practitioner `specialty` vocabulary and `facilities.kind`; 003 defines
`Severity`, `ConditionStatus`, `OrderStatus` and every remaining per-kind vocabulary in
`internal/domain/clinical/vocab.go`; 004 defines `lab.category`, `lab_component.result_type`,
`lab_component.status` and `attachment.category`. Nobody redefines another phase's vocabulary.

**`Severity`** — one ladder, four entities (allergy, condition, injury, symptom):
`mild` `moderate` `severe` `life_threatening`

**`ConditionStatus`** — ongoing-condition shape (allergy, condition, injury, symptom):
`active` `healing` `inactive` `resolved` `chronic`

**`OrderStatus`** — order/event shape (procedure, lab_result):
`ordered` `scheduled` `in_progress` `completed` `cancelled`
(`ordered` and `scheduled` are both retained on one enum rather than maintaining two enums for
one state; procedures use `scheduled`, labs use `ordered`.)

**`TherapyStatus`** — course-of-therapy shape (medication, treatment, equipment):
`active` `on_hold` `completed` `stopped` `cancelled`

Per-kind vocabularies: `laterality` = `left right bilateral not_applicable`;
`medication.type` = `prescription otc supplement herbal`;
`medication.route` = `oral sublingual topical transdermal inhalation nasal ophthalmic otic rectal vaginal intramuscular subcutaneous intravenous other`;
`immunization.route` = `intramuscular subcutaneous intradermal oral intranasal`;
`immunization.site` = `left_arm right_arm left_thigh right_thigh oral nasal other`;
`procedure.setting` = `outpatient inpatient office`;
`procedure.type` = `surgical diagnostic therapeutic preventive other`;
`procedure.outcome` = `successful partial unsuccessful complications`;
`procedure.anesthesia` = `none local regional sedation general`;
`encounter.visit_type` = `office telehealth urgent_care emergency inpatient follow_up annual other`;
`encounter.priority` = `routine urgent emergency`;
`treatment.setting` = `inpatient outpatient home`;
`symptom.category` = `pain respiratory gastrointestinal neurological cardiovascular musculoskeletal dermatological psychological constitutional other`;
`symptom.impact` = `none mild moderate severe`;
`vitals.glucose_context` = `fasting before_meal after_meal random`;
`insurance.type` = `medical dental vision prescription other`;
`insurance.status` = `active inactive expired pending`;
`insurance.relationship` = `self spouse child dependent other`;
`contact.relationship` = `spouse partner parent child sibling friend guardian caregiver other`;
`equipment.type` = `cpap nebulizer wheelchair walker glucose_meter bp_monitor oximeter oxygen hearing_aid prosthetic orthotic other`;
`injury.type` = `sprain strain fracture dislocation laceration contusion burn concussion puncture abrasion other`
(this replaces the whole `injury_types` collection);
`lab.category` = `blood_work urinalysis microbiology pathology genetics imaging other`;
`lab_component.status` = `normal high low critical abnormal`;
`lab_component.result_type` = `quantitative qualitative textual`;
`family.relationship` = `mother father sister brother daughter son grandmother grandfather aunt uncle cousin niece nephew half_sibling other`.

### 1.5 The 15 record kinds

Every one of these is a collection registered in `internal/records`. Registering a kind
simultaneously registers: its CRUD service, its DTO codecs, its list and detail templ components,
its audit hook, its search-index hook, its attachment-cleanup hook, its `/api/v1/records/{kind}`
enum value, and its two pages. **That is the extension point Principle II's open/closed clause
demands; nothing switches on `kind`.**

All of them carry `patient`, `tags`, `notes`, plus their own fields.

| Kind | Phase | Distinctive fields | Links |
|---|---|---|---|
| `allergy` | 003 | `allergen` (text 2..200, req), `reaction` (text ≤500), `severity` (Severity, req), `status` (ConditionStatus, req, default `active`), `onset_on` (date) | `medications` (multi-relation → medications; upstream's single `medication_id` was wrong — a drug-class allergy touches many rows) |
| `emergency_contact` | 003 | `name` (text 2..100, req), `relationship` (contact.relationship, req), `phone` (text 1..40, req), `phone_alt` (text ≤40), `email` (email), `address` (text ≤500), `is_primary` (bool), `is_active` (bool, default true) | — |
| `condition` | 003 | `diagnosis` (text 2..500, req), `status` (ConditionStatus, req), `severity` (Severity), `onset_on` (date), `resolved_on` (date), `icd10_code` (text ≤10), `snomed_code` (text ≤20), `practitioner` (relation) | `medications`, back-relations from labs/injuries/symptoms |
| `medication` | **001** | `name` (text 1..200, req), `alternative_name` (text ≤200), `type` (medication.type), `dosage` (text ≤200), `frequency` (text ≤100), `route` (medication.route), `indication` (text ≤300), `started_on` `ended_on` (date), `status` (TherapyStatus), `side_effects` (text ≤1000), `practitioner` (relation), `pharmacy` (relation → facilities), `reminder_enabled` (bool), `reminder_times` (json `[]"HH:MM"`), `reminder_weekdays` (json `[]int` **1=Monday..7=Sunday, documented** — upstream never said), `reminder_message` (text ≤200) | back-relations only |
| `encounter` | 003 | `reason` (text 1..300, req — `chief_complaint` merged in), `occurred_on` (date, req), `visit_type`, `priority`, `assessment` (text ≤5000 — upstream's free-text `diagnosis`, renamed so it cannot be confused with a `condition`), `plan` (text ≤5000), `follow_up` (text ≤2000), `duration_minutes` (number ≥0), `practitioner`, `facility`, `condition` (relation) | `lab_results` (multi) |
| `procedure` | 003 | `name` (text 2..300, req), `type`, `code` (text ≤50), `description` (text ≤5000), `occurred_on` (date, req), `status` (OrderStatus, req), `outcome`, `setting`, `complications` (text ≤500), `duration_minutes` (number ≥0), `anesthesia` , `anesthesia_notes` (text ≤2000), `practitioner`, `facility`, `condition` | back-relations |
| `treatment` | 003 | `name` (text 2..300, req), `type` (text ≤120), `setting`, `description` (text ≤5000), `started_on` `ended_on` (date), `frequency` (text ≤100), `dosage` (text ≤200), `expected_outcome` (text ≤300), `status` (TherapyStatus), `practitioner`, `facility`, `condition` | `encounters`, `equipment`, `lab_results` (multi); **`treatment_medications` is a real collection** |
| `symptom` | 003 | one row per **episode**: `name` (text 1..200, req), `category`, `severity` (Severity, req), `occurred_at` (date, req), `duration_minutes` (number ≥0), `pain_scale` (number 0..10), `body_site` (text ≤120), `triggers` (json `[]string`), `relief_methods` (json `[]string`), `impact` (symptom.impact), `resolved_at` (date), `is_chronic` (bool), `status` (ConditionStatus) | `conditions`, `treatments`, and **two** medication relations: `treated_by_medications` and `caused_by_medications` (upstream's `relationship_type` was the only link table with real semantics — preserved as two fields, not a payload column) |
| `vitals` | 003 | `recorded_at` (date, req), `systolic_mmhg` `diastolic_mmhg` `heart_rate_bpm` `respiratory_rate_bpm` (number, **bounded**: 40..300 / 20..200 / 20..300 / 4..80), `temperature_c` (number 25..45), `spo2_pct` (number 50..100), `weight_kg` (number 0.5..450), `height_cm` (number 30..272), `glucose_mmol_l` (number 0.5..60), `glucose_context`, `hba1c_pct` (number 2..20), `pain_scale` (number 0..10), `device` (text ≤120), `practitioner` | none. **Upstream had zero numeric bounds on any vital.** `bmi` is derived, never stored. SI units throughout. |
| `immunization` | 003 | `vaccine_name` (text 2..200, req), `trade_name` (text ≤200), `administered_on` (date, req), `dose_number` (number ≥1), `lot_number` (text ≤50), `manufacturer` (text ≤200), `site`, `route`, `expires_on` (date), `practitioner`, `facility` | — (the disease roll-up is a `GROUP BY vaccine_name`; `catalog_vaccines` is dropped, §1.3) |
| `injury` | 003 | `name` (text 2..300, req), `type` (injury.type), `body_part` (text 1..100, req), `laterality`, `occurred_on` (date), `mechanism` (text ≤500), `severity` (Severity), `status` (ConditionStatus, default `active`), `recovery_notes` (text ≤2000), `practitioner` | `conditions`, `medications`, `procedures`, `treatments` (multi) |
| `insurance` | 003 | `type` (insurance.type, req), `company` (text 1..200, req), `plan_name` (text ≤200), `employer_group` (text ≤200), `member_name` (text 1..200, req), `member_id` (text 1..80, req), `group_number` (text ≤80), `holder_name` (text ≤200), `relationship_to_holder`, `effective_on` (date, req), `expires_on` (date), `status` (insurance.status), `is_primary` (bool), `coverage` (json — **validated struct**: deductible, oop_max, copay_primary, copay_specialist, copay_er, coinsurance_pct, currency), `contact` (json — validated struct: phone, claims_phone, website, portal_url, address) | — |
| `equipment` | 003 | `name` (text 2..200, req), `type` (equipment.type, req), `manufacturer` (text ≤200), `model` (text ≤100), `serial` (text ≤100), `prescribed_on` `serviced_on` `service_due_on` (date), `instructions` (text ≤5000), `status` (TherapyStatus), `supplier` (relation → facilities), `practitioner` | back-relation from treatments |
| `family_member` | 003 | `name` (text 1..100, req), `relationship` (family.relationship, req), `sex` (patients.sex vocabulary), `birth_year` `death_year` (number 1850..2200), `is_deceased` (bool), `conditions` (json — **validated `[]FamilyCondition{name, icd10_code, diagnosed_age, severity, status, notes}`**) | shareable as `resource_kind = family_member` |
| `lab_result` | **004** | `test_name` (text 1..300, req), `test_code` (text ≤60), `category` (lab.category), `catalog_test` (relation → catalog_lab_tests, MaxSelect 1), `status` (OrderStatus, req, default `ordered`), `ordered_on` `collected_on` `resulted_on` (date), `interpretation` (text ≤2000), `is_panel` (bool, **mutable** — upstream made it create-only), `value` (number), `unit` (text ≤40), `ref_low` `ref_high` (number), `ref_text` (text ≤200), `practitioner`, `facility` | `conditions`, `medications`, `procedures` (multi); back-relations from encounters and treatments |

**Why `family_member.conditions` and `insurance.coverage`/`contact` are validated JSON and not
collections:** they are value-object lists that are only ever read with their parent, never
queried, never shared independently, and never linked to. A collection would buy referential
integrity nobody uses and cost a join on every read. They are **typed Go structs** marshalled
into a `json` field with a `Validate()` — not upstream's free-form blobs.

#### `lab_components` — Phase 004, a child collection, not a kind

`lab_result` (relation, cascade, required), `test_name` (text 1..300, req), `abbreviation`
(text ≤20), `canonical_name` (text — normalised for trending), `catalog_test` (relation),
`result_type` (lab_component.result_type, default `quantitative`),
`value` (number) | `value_text` (text ≤500) — discriminated by `result_type`, validated in Go
(upstream had three parallel value columns with no cross-field validation),
`unit` (text ≤40), `ref_low` `ref_high` (number), `ref_text` (text ≤200),
`status` (lab_component.status), `display_order` (number).
Components are created, replaced and deleted **as a set through their parent lab result's
payload**. They have no CRUD endpoints of their own; they have two read endpoints for trending.

#### `treatment_medications` — Phase 003, the one surviving join

`treatment` (relation, cascade, req), `medication` (relation, cascade, req),
`dosage` (text ≤200), `frequency` (text ≤100), `duration` (text ≤100),
`timing` (text ≤300), `prescriber` (relation → practitioners),
`pharmacy` (relation → facilities), `started_on` `ended_on` (date).
Unique index `(treatment, medication)`.
The read DTO exposes `effective_*` fields — the link value COALESCEd over the medication's own
defaults. That was the single most interesting piece of derived logic upstream had and it is
kept.

### 1.6 Entity roll-up by phase — AUTHORITATIVE

**Cite this table. Do not re-derive it.**

| Phase | New collections | New | Running total |
|---|---|---|---|
| 001 | `users` (PB auth collection, extended), `medications`, `audit_events` | 3 | 3 |
| 002 | `patients`, `practitioners`, `facilities` | 3 | 6 |
| 003 | `allergies`, `conditions`, `encounters`, `procedures`, `treatments`, `symptoms`, `vitals`, `immunizations`, `injuries`, `insurances`, `equipment`, `emergency_contacts`, `family_members`, `tags`, `treatment_medications`, `search_index` | 16 | 22 |
| 004 | `catalog_lab_tests`, `lab_results`, `lab_components`, `attachments` | 4 | 26 |
| 005 | `shares`, `invitations` | 2 | 28 |
| 006 | `report_templates`, `export_jobs` | 2 | **30** |

**Total: 30 collections.** `catalog_vaccines` is not in this table and is not built (§1.3).

Collection *amendments* are not new collections and are not counted here: 002 amends `users`
(`active_patient`), `audit_events` (`patient`) and `medications` (re-anchored onto `patient`); 003
amends `medications` and the `audit_events` vocabulary; 004 amends `attachments`' owning-kind
vocabulary; 005 and 006 amend the `audit_events` vocabulary again and 006 amends `users`
(`must_change_password`). A migration count is therefore always larger than a collection count, and
the two must never be conflated in a roll-up.

**Registered record kinds:** 001 registers 1 (`medication`); 003 registers 13; 004 registers 1
(`lab_result`). **Total 15**, which is the number §2.2's budget depends on and it is unchanged.

---

## 2. THE PUBLIC API SURFACE

### 2.1 Conventions — stated once, then followed without exception

1. **Base path `/api/v1`. No trailing slashes, ever.** PocketBase has done no trailing-slash
   normalisation since v0.23, so `/api/v1/patients/` and `/api/v1/patients` are different routes.
   The registry rejects a path ending in `/`.
2. **Plural, kebab-case resource segments.** `/lab-components`, `/report-templates`. Path
   parameters are `{id}` or `{kind}`. **One spelling of every record kind everywhere** —
   singular `snake_case` in `kind` values and enums (`lab_result`), plural kebab-case in paths
   (`/lab-results`). Upstream had three spellings of `lab_result` in one API; MediGo has exactly
   these two, and they are generated from one Go constant.
3. **Nesting depth is at most one.** `/api/v1/treatments/{id}/medications` is legal;
   `/api/v1/patients/{id}/medications/{mid}/…` is not. Everything else is a top-level resource
   filtered by query parameter. This is what killed upstream's 9 `/patients/{id}/{type}` fan-out
   routes and the 21-operation `/patients/me` duplicate surface.
4. **Patient scope is explicit and mandatory.** Every list over patient-scoped data requires
   `?patient={id}`; its absence is `400`, never an implicit fallback to the active patient.
   Creates carry `patient` in the body. Reads, updates and deletes derive it from the record.
   **The active patient is never consulted for authorization.**
5. **Pagination is cursor-based.** `?limit=` (default 25, max 100) and `?cursor=` (opaque,
   server-minted). Envelope: `{"items":[…],"next_cursor":"…"|null}`. A total is returned only
   when `?count=true` is passed, because a COUNT over a shared patient's chart is not free.
6. **Sorting** is `?sort=` with a comma list, `-` prefix for descending, drawn from a per-resource
   allowlist. Default `-occurred_at` (or the kind's primary date), then `-created`.
7. **Filtering** is explicit named parameters only. PocketBase's filter DSL **never reaches the
   wire**: it is an injection surface and it leaks the schema.
8. **No partial responses.** There is no `?fields=`. List endpoints return a `*Summary` DTO;
   detail endpoints return the full DTO. Two shapes per resource, both in OpenAPI.
9. **Verbs:** `GET` read, `POST` create (`201` + `Location`), `PATCH` partial update (`200`),
   `PUT` idempotent whole-resource replace (used for `photo`, `active-patient`, link upsert),
   `DELETE` (`204`).
10. **Optimistic concurrency.** Every clinical record response carries an `ETag` derived from
    `updated`. `PATCH` and `DELETE` on a clinical record **require** `If-Match`; a mismatch is
    `412`. This matters the moment two carers share a chart.
11. **Idempotency keys are not used.** Creates are not idempotent; duplicates are prevented by
    uniqueness indexes where they matter and by a disabled submit button where they do not.
    (Explicit YAGNI decision under Principle I.)
12. **Error envelope**, always, on every non-2xx:
    ```json
    { "error": { "code": "validation_failed",
                 "message": "human-readable, PHI-free",
                 "request_id": "…",
                 "fields": [ { "field": "dosage", "code": "required", "message": "…" } ] } }
    ```
13. **A resource the caller may not see returns `404`, not `403`** — for anything patient-scoped,
    existence is itself PHI. `403` is reserved for operations on resources whose existence is
    already known to the caller (e.g. editing a patient shared to you at `view`).
14. `Content-Type: application/json` in and out, except multipart upload and file download.
    Unknown JSON fields are **rejected** (`422`), not ignored.
15. Every operation has a stable `operationId` (`listRecords`, `createShare`, …) asserted by the
    Principle IX gate to exist in both the registry and the committed OpenAPI document.

### 2.2 The record route family — the decision that makes the budget work

The thirteen clinical record types plus emergency contacts and family members share **one route
family with six operations**:

```
GET    /api/v1/records                    cross-kind list  (dashboard, timeline, report picker)
GET    /api/v1/records/{kind}             list one kind
POST   /api/v1/records/{kind}             create
GET    /api/v1/records/{kind}/{id}        read
PATCH  /api/v1/records/{kind}/{id}        update
DELETE /api/v1/records/{kind}/{id}        delete
```

`{kind}` is documented in OpenAPI as an `enum` of the fifteen plural kebab-case values. Request
and response bodies are `oneOf` with `kind` as the discriminator, so **every kind still has its
own explicit, fully typed DTO** — the polymorphism is in the routing table, not in the schema.

MediKeep spent roughly 65 operations, thirteen routers, and 21 legacy duplicates on this. MediGo
spends six. Adding a fourteenth clinical record type in a future phase adds **zero** routes and
zero OpenAPI paths — it adds one `oneOf` branch and one registry entry.

`GET /api/v1/records?patient=&kind=a,b,c&from=&to=&tags=&q=` also replaces upstream's
`/patients/me/recent-activity`, `/patients/recent-activity`, the nine `/patients/{id}/{type}`
fan-outs, and the report picker's `/custom-reports/data-summary` record listing.

The fifteen "specialised filter" endpoints upstream shipped (`/allergies/patient/{id}/critical`,
`/conditions/patient/{id}/active`, `/procedures/scheduled`, `/treatments/ongoing`,
`/medical-equipment/needing-service`, `/insurances/expiring`, `/encounters/patient/{id}/recent`,
…) are **all** query parameters on this family: `?status=`, `?severity=`, `?due_before=`,
`?from=`. Upstream returned the plain DTO from those routes anyway, so the client could not tell
*why* a row matched — a filter parameter is strictly more informative.

### 2.3 The complete route list — AUTHORITATIVE

**Cite these numbers. Do not re-derive them.** Operation numbers are STABLE IDENTITIES, not
positions: they are the numbers the phase contracts already cite (004's "ops 44, 49–56", 005's
"ops 58–62", 006's "op 91"), so they are **not** renumbered when an operation moves between phases,
is deferred, or is dropped. Numbers 1–90 are this document's original list; 91–95 are additions.

| Phase | New `/api/v1` operations | Running total |
|---|---|---|
| 001 walking-skeleton | 22 | 22 |
| 002 patient-core | 20 | 42 |
| 003 clinical-records | 8 | 50 |
| 004 labs-and-attachments | 9 | 59 |
| 005 sharing-and-collaboration | 10 | 69 |
| 006 reporting-and-operations | 25 | **94** |

**Total: 94 operations.** Reconciliation against the 90 this document first published:
−1 dropped (op 45, `catalog/vaccines`, with its collection — §1.3), +5 added (ops 91, 92, 93, 94,
95). 90 − 1 + 5 = 94. Ops 4, 7 and 8 are **allocated** — 7 and 8 to phase 001, 4 to phase 006 —
and were never removed from the 90.

**Phase 001 — walking-skeleton (22 operations)**

| # | Method | Path | Purpose |
|---|---|---|---|
| 1 | GET | `/api/v1/auth/config` | Is registration open; the published password rules, so the sign-up form can state them before the person chooses. Public. |
| 2 | POST | `/api/v1/auth/register` | Creates the account and signs the person in, in one transaction. `role`, `disabled_at` and `verified` are rejected as unknown fields. **The self-record patient is 002's**, because patients do not exist yet. |
| 3 | POST | `/api/v1/auth/login` | Password auth. Completes through `apis.RecordAuthResponse`. |
| 5 | POST | `/api/v1/auth/refresh` | Token refresh. |
| 6 | POST | `/api/v1/auth/logout` | Clears the session cookie and audits. |
| 7 | POST | `/api/v1/auth/password-reset` | Self-service password recovery. Public, rate limited. **Answers identically whether or not the address has an account**, and `503 mail_unconfigured` when the instance has no outgoing mail — never a false "check your email". Wraps `mails.SendRecordPasswordReset`. |
| 8 | POST | `/api/v1/auth/password-reset/confirm` | Token + new password. Resolves through `app.FindAuthRecordByToken(t, core.TokenTypePasswordReset)`, then `SetPassword` + `Save`, which rotates `tokenKey` and ends every prior session. Expired, used and forged are **one** answer: `400 invalid_token`. |
| **94** | POST | `/api/v1/auth/verify-email` | **NEW.** Send the address-confirmation message again. Authenticated, takes **no** address from the caller — it reads the signed-in record's own. |
| **95** | POST | `/api/v1/auth/verify-email/confirm` | **NEW.** Token → `verified`. Public, because the person following the link may not be signed in on that device. The contract allocated the `/verify-email/{token}` page but no operation to serve it; these two are that operation pair. |
| 9 | GET | `/api/v1/me` | Profile + preferences. Amended by 002 to carry the active patient and the accessible-patient counts. |
| 10 | PATCH | `/api/v1/me` | Profile + the four real preferences. `role`, `disabled_at` not in the DTO. |
| 11 | DELETE | `/api/v1/me` | Account deletion. Requires password re-entry and an explicit confirmation phrase; cascades; audited. |
| **92** | PUT | `/api/v1/me/password` | **NEW.** Change the password by supplying the current one. `PUT` because it replaces one resource wholly and idempotently. Rotates `tokenKey`, so every other session ends and the calling session is re-cookied from the saved record. |
| 21 | GET | `/api/v1/records` | Cross-kind list. See §2.2. |
| 22 | GET | `/api/v1/records/{kind}` | |
| 23 | POST | `/api/v1/records/{kind}` | |
| 24 | GET | `/api/v1/records/{kind}/{id}` | |
| 25 | PATCH | `/api/v1/records/{kind}/{id}` | `If-Match` required. |
| 26 | DELETE | `/api/v1/records/{kind}/{id}` | `If-Match` required. |
| 27 | GET | `/api/v1/healthz` | Process liveness. Public, no DB touch. |
| 28 | GET | `/api/v1/readyz` | DB + migration state. Public. |
| 29 | GET | `/api/v1/streams/records` | **SSE.** Datastar stream for the current record lists. Counted as an operation. |

`GET /api/v1/version` is served by the same handler as `/healthz`'s payload; it is **not** a
separate route (Principle I). `medications` is the single kind registered behind ops 21–26.

**Phase 002 — patient-core (20 operations)**

| # | Method | Path | Purpose |
|---|---|---|---|
| 13 | GET | `/api/v1/patients` | Owned ∪ shared, with `owned_count`/`shared_count`. |
| 14 | POST | `/api/v1/patients` | |
| 15 | GET | `/api/v1/patients/{id}` | |
| 16 | PATCH | `/api/v1/patients/{id}` | |
| 17 | DELETE | `/api/v1/patients/{id}` | Owner only. Hard cascade. Confirmed in the UI, audited. |
| 18 | PUT | `/api/v1/patients/{id}/photo` | Multipart. Thumbnails generated eagerly. |
| 19 | GET | `/api/v1/patients/{id}/photo` | `?size=100x100t\|400x400f\|original`. Served through MediGo's own route via `app.NewFilesystem()`. **No file token, ever.** |
| 20 | DELETE | `/api/v1/patients/{id}/photo` | |
| **93** | GET | `/api/v1/patients/{id}/summary` | **NEW.** The patient chart header and per-kind tile counts in one authorized read, plus the figures the delete confirmation must state. One round trip instead of one per kind. |
| 12 | PUT | `/api/v1/me/active-patient` | Sets the switcher. Collapses upstream's `/switch` + `/active/current` + `/self-record` + `/owned/list`. **Never consulted for authorization.** |
| 30–34 | GET / POST / GET{id} / PATCH{id} / DELETE{id} | `/api/v1/practitioners` | Per-user practitioner directory. `?q=&specialty=&facility=` |
| 35–39 | GET / POST / GET{id} / PATCH{id} / DELETE{id} | `/api/v1/facilities` | Practices + pharmacies + hospitals + labs + imaging, one resource. `?q=&kind=` |

Amended, adding no operation: ops 9 and 10 (`me` gains the active patient), ops 21–26 and 29
(medications re-anchor onto `patient`, and `?patient=` becomes mandatory on every list).

**Phase 003 — clinical-records (8 operations)**

Thirteen new record kinds register into the **existing** six record routes and add none. The eight
new operations are tags, the one payload-carrying join, and search.

| # | Method | Path | Purpose |
|---|---|---|---|
| 40–43 | GET / POST / PATCH{id} / DELETE{id} | `/api/v1/tags` | `GET` serves list, autocomplete (`?q=`) and popular (`?sort=-usage`) in one operation — upstream needed five. |
| 46 | GET | `/api/v1/treatments/{id}/medications` | The link rows with `effective_*` COALESCE resolution. |
| 47 | PUT | `/api/v1/treatments/{id}/medications/{medicationId}` | Idempotent upsert of the link + its payload. Replaces upstream's create/update/bulk-create trio **and** the mirror-image routes on the medication side. |
| 48 | DELETE | `/api/v1/treatments/{id}/medications/{medicationId}` | |
| 57 | GET | `/api/v1/search` | `?q=&patient=&kinds=&tags=&match=any\|all&from=&to=&limit=&cursor=`. Grouped by kind, **each group carrying its own cursor and `has_more`**. **Moved 004 → 003** with the search index. **Ordered by date, not by relevance**: R3 is CLOSED so ranking would be *available* (§8), but `003/spec.md:345` (FR-073) declines to claim it, so this operation must not promise it — there is no `rank`, no score and no `?sort=relevance` (amendment 2026-08-27, ANALYSIS N12). |

Every other link is a multi-relation field edited by `PATCH /api/v1/records/{kind}/{id}`.
That is 44 upstream endpoints and ~35 schemas deleted.

**Phase 004 — labs-and-attachments (9 operations)**

| # | Method | Path | Purpose |
|---|---|---|---|
| 44 | GET | `/api/v1/catalog/lab-tests` | `?q=&category=&loinc=&common=true`. Read-only. Replaces eight upstream routes. **Moved 002 → 004**, with its collection. |
| 49 | POST | `/api/v1/attachments` | Multipart: `patient`, `owner_kind`, `owner_id`, `file`, `description`, `category`. MIME sniffed server-side; thumbnails eager. |
| 50 | GET | `/api/v1/attachments` | Document library. `?patient=&owner_kind=&owner_id=&category=&q=&deleted=true` (the last one is the trash view). |
| 51 | GET | `/api/v1/attachments/{id}` | `?disposition=inline\|attachment&size=…`. Streams through `fsys.Serve` with Range/ETag. Authorized by the service. **No `?token=` in a URL, ever.** |
| 52 | PATCH | `/api/v1/attachments/{id}` | Description and category only. |
| 53 | DELETE | `/api/v1/attachments/{id}` | Soft: sets `deleted_at`. `?purge=true` is hard and early, and is allowed to the **owner** (behind a typed confirmation) and to a **superuser**; `404` to a share recipient. |
| 54 | POST | `/api/v1/attachments/{id}/restore` | Un-trash within the retention window. |
| 55 | GET | `/api/v1/lab-components` | Per-patient component catalog rollup: latest value, unit, status, reading count, trend direction. `?patient=&q=` |
| 56 | GET | `/api/v1/lab-components/trend` | `?patient=&canonical_name=&unit=&from=&to=`. **`unit` scoping is mandatory when more than one unit exists** — upstream plotted mg/dL and mmol/L on one axis. |

Lab results need no CRUD of their own — `lab_result` is the fifteenth record kind — and its
components are a validated array inside its payload with replace-set semantics.

**Phase 005 — sharing-and-collaboration (10 operations)**

| # | Method | Path | Purpose |
|---|---|---|---|
| 58 | POST | `/api/v1/shares` | Creates an **invitation** (never a direct grant). Body carries `resource_kind`, `resource_ids[]`, `recipient_email`, `level`, `note`, `expires_at`. Single and bulk are one operation. |
| 59 | GET | `/api/v1/shares` | `?direction=granted\|received&patient=&resource_kind=&active=true`. Replaces `shared-by-me`, `shared-with-me`, `/patient-sharing/{id}`, `/stats/user`, and their four family-history twins. |
| 60 | PATCH | `/api/v1/shares/{id}` | Grantor changes `level` or `expires_at`. |
| 61 | DELETE | `/api/v1/shares/{id}` | Grantor revokes. |
| 62 | DELETE | `/api/v1/shares/{id}/mine` | Grantee leaves. A genuinely different operation with a different actor — kept. |
| 63 | GET | `/api/v1/invitations` | `?direction=received\|sent&status=`. Expiry is a filter, not a cleanup endpoint. |
| 64 | GET | `/api/v1/invitations/token/{token}` | Public preview for accept-and-sign-up. Returns the sender's display name and the resource count — **never patient names or clinical content**. |
| 65 | POST | `/api/v1/invitations/{id}/respond` | `{"response":"accepted"\|"rejected","note":"…"}`. Materialises the share(s) in one transaction. |
| 66 | DELETE | `/api/v1/invitations/{id}` | Sender cancels a pending invitation; sender revokes an accepted one (the underlying share is revoked in the same transaction). Upstream needed two operations in two routers. |
| 67 | GET | `/api/v1/streams/notifications` | **SSE.** Invitation received/answered, share granted/revoked. IDs only; re-authorised per event. |

This phase **widens authorization on every endpoint phases 001–004 already shipped** and adds no
operation for it. That is the point of the single `access.Authorizer` seam.

**Phase 006 — reporting-and-operations (25 operations)**

| # | Method | Path | Purpose |
|---|---|---|---|
| 68 | GET | `/api/v1/audit` | `?patient=&actor=&action=&target_kind=&from=&to=&format=csv`. One endpoint replaces upstream's five projections of one table with four DTOs. Non-admins see only events for patients they can access. |
| 69 | GET | `/api/v1/reports/summary` | Per-kind counters + the selection's resolved count. **Replaces `patients/me/dashboard-stats`, `export/summary` and `custom-reports/data-summary` — three endpoints computing the same counters.** |
| 70 | GET | `/api/v1/reports/trends` | Which vitals metrics and lab canonical names have enough data to chart, with point counts. Folds upstream's `available-trend-data` + `trend-chart-counts`. |
| 71–75 | GET / POST / GET{id} / PATCH{id} / DELETE{id} | `/api/v1/report-templates` | Criteria-based, not frozen record ids. `If-Match` required on 74 and 75. |
| 76 | POST | `/api/v1/exports` | `202` + a job. Body: `kind`, `format`, `scopes[]`, `patient`, `template`, date range. |
| 77 | GET | `/api/v1/exports` | The caller's jobs. |
| 78 | GET | `/api/v1/exports/{id}` | Job status/progress/queue position. |
| 79 | GET | `/api/v1/exports/{id}/download` | Streams the artifact; authorized, expiring, audited. |
| **91** | POST | `/api/v1/exports/{id}/cancel` | **NEW.** Cancels a queued or running job. Without it a mistaken whole-account export cannot be stopped and the only remedy is waiting or restarting the process. |
| 80 | GET | `/api/v1/admin/stats` | Typed dashboard tiles, each with its definition and `computed_at`. Replaces nine untyped admin-dashboard routes including three unauthenticated debug endpoints. |
| 81 | GET | `/api/v1/admin/system` | DB status, storage footprint, last backup age, MFA/IP-allowlist posture, SMTP posture, migration state. |
| 82 | GET | `/api/v1/admin/users` | |
| 83 | PATCH | `/api/v1/admin/users/{id}` | `role`, `disabled_at`, forced password change. |
| 84 | GET | `/api/v1/admin/backups` | Thin wrapper over PocketBase's own backup list. |
| 85 | POST | `/api/v1/admin/backups` | Wraps `app.CreateBackup`. |
| 86 | POST | `/api/v1/admin/backups/upload` | Registers an external archive. |
| 87 | GET | `/api/v1/admin/backups/{name}` | Metadata + **restore preview**: what is about to be clobbered. |
| 88 | **POST** | `/api/v1/admin/backups/{name}/download` | **Method changed from `GET`.** The archive is the whole instance in one file, so the download re-authenticates: an ordinary `<form method="post">` with a password field — no JavaScript, and no credential in a URL. |
| 89 | POST | `/api/v1/admin/backups/{name}/restore` | **Always takes a safety backup first** and returns its id. Requires a confirmation phrase and the password in the body. |
| 90 | DELETE | `/api/v1/admin/backups/{name}` | |
| 4 | POST | `/api/v1/auth/oauth2` | Sign-in through an external identity provider the operator configured. **Public** — the only public operation in this phase. Wraps PocketBase's `authWithOAuth2` and its `_externalAuths` linking; MediGo adds one DTO so a provider sign-in yields the same session, cookie and audit row as a password sign-in, and so `role` and `disabled_at` are unreachable from the request. `404` — identical to an unknown provider name — when no provider is configured. |

**Formerly deferred, now allocated**

Ops **4**, **7** and **8** were listed here as "allocated to NO phase" in an earlier revision.
That is closed: **7 and 8 belong to phase 001** with the two new confirmation operations 94 and
95, and **4 belongs to phase 006**, with the operator surface that configures providers. Both
halves are recorded in the receiving phases' Deviations tables. PocketBase's own OAuth2, reset and
verification endpoints remain reachable as documented externals (§2.4) — MediGo's routes are the
supported paths, and both call the same code.

**Dropped**

| # | Method | Path | Status |
|---|---|---|---|
| 45 | GET | `/api/v1/catalog/vaccines` | **Dropped**, with the `catalog_vaccines` collection. §1.3. |

### 2.4 Documented exceptions — PocketBase-native paths that stay public

These are **not** `/api/v1` routes, are recorded in the route registry as `KindExternal`, and
appear in the OpenAPI document as documented externals so the Principle IX gate does not flag
them:

`POST /api/collections/users/auth-with-oauth2`, `GET|POST /api/oauth2-redirect`,
`POST /api/collections/users/request-otp`, `POST /api/collections/users/auth-with-otp`,
`POST /api/collections/users/request-password-reset`,
`POST /api/collections/users/confirm-password-reset`,
`POST /api/collections/users/request-verification`,
`POST /api/collections/users/confirm-verification`,
`POST /api/collections/users/confirm-email-change`,
and the superuser admin UI at `/_/` plus `/api/collections/_superusers/*`.

Everything under `/api/collections/{collection}/records` and `/api/batch` returns `404` to
non-superusers via a middleware bound at priority `-1019` (after `loadAuthToken` at `-1020`, so
`e.Auth` is populated), on top of the five-nil-rules lockdown. `Settings().Batch.Enabled = false`.

---

## 3. THE UI SURFACE

### 3.0 The shell — every page, every phase

```
<body data-signals="{…}">
  <a href="#main">Skip to main content</a>
  <header id="app-header">                            role=banner
    <nav id="primary-nav" aria-label="Primary">       role=navigation, name "Primary"
      @PatientSwitcher(...)                           role=combobox, name "Active patient"
    </nav>
  </header>
  <main id="main" tabindex="-1"> … </main>            role=main
  <div id="error-banner" role="alert" aria-live="assertive"></div>
  <div id="toast" role="status" aria-live="polite"></div>
  <footer id="app-footer"> … </footer>                role=contentinfo
</body>
```

**The landmarks live outside every patch target and are never morphed.** Every patch target is a
templ component whose root element carries a stable deterministic id produced by
`internal/web/views/ids`, and that component is the only thing that renders that id. Handlers
never type a raw selector.

Every page therefore asserts, at both 1440×900 and 390×844: `200`; `banner`, `navigation[name=
"Primary"]`, `main`, `contentinfo` visible; `combobox[name="Active patient"]` visible on every **authenticated** page (introduced by phase 002; asserted from phase 002 onward — a switcher that throws must break the gate everywhere at once, which is the desired blast radius); the page's own landmark (below) visible;
`body[data-signals]` present (proving Datastar booted); zero console errors; zero page errors;
zero failed network requests.

**Empty states** are a shared `@EmptyState(title, body, action)` component rendered inside the
page's own landmark, so the landmark assertion holds on a freshly seeded instance with no data —
this is the most common way a smoke gate goes falsely red.

**Error views** (not routes; rendered by the central error renderer, and each covered by a smoke
case at both viewports). Their landmark strings are settled by phase 001 and are gate assertions:
`404` → `region[name="Not found"]` (unknown path, **and every refused access to another account's
data**); `403` → `region[name="Sign in required"]` (a session-required page with no session — it
renders the sign-in prompt rather than a 404); `500` → `region[name="Something went wrong"]`
(carries the request id and nothing else). All three render the full shell so the landmark
assertions hold, and none of them echoes a request path, a stack trace or a server error message.

### 3.1 Pages — AUTHORITATIVE

**Cite these numbers. Do not re-derive them.** Landmark column = the page-specific assertion **in
addition** to the four shell landmarks. **Every landmark string in this table is a literal a
Playwright selector contains; changing one is a breaking change to the gate.**

| Phase | New page routes | Running total | Page-action routes |
|---|---|---|---|
| 001 walking-skeleton | 9 (+3 error views) | 9 | 0 |
| 002 patient-core | 6 | 15 | 0 |
| 003 clinical-records | 29 | 44 | 0 |
| 004 labs-and-attachments | 4 | 48 | 4 |
| 005 sharing-and-collaboration | 3 | 51 | 0 |
| 006 reporting-and-operations | 7 | **58** | 3 |

**Total: 58 page routes + 3 error views, and 7 page-action routes.** Reconciliation against the 56
this document last published: +`/timeline` (003), +`/invite/{token}` (005). 56 + 2 = 58. The three
auth pages are **built by phase 001**, not deferred, and are inside that 58. **30 of the 58** — two
per registered kind, 15 kinds — are generated from the kind registry and cost no handwritten
routing: 2 in 001, 26 in 003, 2 in 004.

**Phase 003's seven status views are covered by the gate without being pages.** `/conditions?active=true`
and its six siblings are query strings on registered kind lists (§3.2), because the specification
requires a status view to *be* a narrowing of that kind's list. They are registered as
`SmokeVariants` on their route — additional concrete URLs the Playwright gate must also visit,
emitted inside that route's `medigo routes` entry, **not counted as pages**. That is what keeps a
"helpful empty state on every status view" inside the gate rather than outside it.

**Phase 001 — 9 pages**

| Route | Purpose | Page landmark |
|---|---|---|
| `/login` | Password sign-in. Public, no nav landmark. | `form[name="Sign in"]` |
| `/register` | Sign-up. Registered unconditionally; renders inside the shell with an explanation when registration is closed, never a bare 404. Public. | `form[name="Create account"]` |
| `/` | Dashboard: counters, recent records. | `region[name="Overview"]` |
| `/medications` | Kind list page (see §3.2). | `region[name="Medications"]` |
| `/medications/{id}` | Kind detail page. | `article[name="Medication"]` |
| `/settings` | Profile, preferences, password, address-confirmation state, danger zone. | `region[name="Settings"]` |
| `/forgot-password` | Ask for a recovery message. Public. Answers identically for an address with and without an account. | `form[name="Reset password"]` |
| `/reset-password/{token}` | Choose a new password from a recovery link. Public. | `form[name="Choose a new password"]` |
| `/verify-email/{token}` | Confirm the address on an account. Public. | `region[name="Email confirmation"]` |

**The two token pages register a deliberately invalid `SmokeURL`** — `/reset-password/expired-token-for-smoke`
and `/verify-email/expired-token-for-smoke` — and answer it with **`200`** and the "this link is no
longer usable, request another" state inside their own landmark. A seeded real token cannot work: a
reset token lives 30 minutes and a confirmation token 24 hours, so any committed fixture is expired
by the time CI runs. The most likely real visit to these pages is a link opened too late, so that is
the state under the gate; the working path is covered by an end-to-end spec against a mail sink.

**Phase 002 — 6 pages**

| Route | Purpose | Page landmark |
|---|---|---|
| `/patients` | Owned + shared people, with the create action. | `region[name="Patients"]` |
| `/patients/{id}` | Chart header, photo, demographics, per-kind tiles, edit in place. | `region[name="Patient chart"]` |
| `/practitioners` | Directory list + create drawer. | `region[name="Practitioners"]` |
| `/practitioners/{id}` | Detail, linked records. | `article[name="Practitioner"]` |
| `/facilities` | Practices, pharmacies, hospitals, labs, imaging — one list, filtered by kind. | `region[name="Facilities"]` |
| `/facilities/{id}` | Detail. | `article[name="Facility"]` |

002 also welds the patient switcher into the shell (`combobox[name="Active patient"]`), so **every
existing page's smoke case now exercises it** — the desired blast radius.

**Phase 003 — 29 pages** = 13 kinds × 2 (§3.2) + 3 standalone.

| Route pair | Landmarks |
|---|---|
| `/allergies` · `/allergies/{id}` | `region[name="Allergies"]` · `article[name="Allergy"]` |
| `/conditions` · `/conditions/{id}` | `region[name="Conditions"]` · `article[name="Condition"]` |
| `/encounters` · `/encounters/{id}` | `region[name="Encounters"]` · `article[name="Encounter"]` |
| `/procedures` · `/procedures/{id}` | `region[name="Procedures"]` · `article[name="Procedure"]` |
| `/treatments` · `/treatments/{id}` | `region[name="Treatments"]` · `article[name="Treatment"]` |
| `/symptoms` · `/symptoms/{id}` | `region[name="Symptoms"]` · `article[name="Symptom episode"]` |
| `/vitals` · `/vitals/{id}` | `region[name="Measurements"]` · `article[name="Measurement set"]` |
| `/immunizations` · `/immunizations/{id}` | `region[name="Vaccinations"]` · `article[name="Vaccination"]` |
| `/injuries` · `/injuries/{id}` | `region[name="Injuries"]` · `article[name="Injury"]` |
| `/insurance` · `/insurance/{id}` | `region[name="Insurance"]` · `article[name="Insurance policy"]` |
| `/equipment` · `/equipment/{id}` | `region[name="Equipment"]` · `article[name="Equipment"]` |
| `/emergency-contacts` · `/emergency-contacts/{id}` | `region[name="Emergency contacts"]` · `article[name="Emergency contact"]` |
| `/family-history` · `/family-history/{id}` | `region[name="Family history"]` · `article[name="Relative"]` |

| Standalone route | Purpose | Page landmark |
|---|---|---|
| `/tags` | Tag manager: create, rename, recolour, delete, with usage counts and an "N records carry this" confirmation. **Moved 002 → 003.** | `region[name="Tags"]` |
| `/search` | One search over a person's whole chart, grouped by kind, each group paged independently. **Moved 004 → 003.** | `search` |
| `/timeline` | **NEW.** One chronological view across every kind, narrowable by kind, date range and tag. The page `GET /api/v1/records` was always designed for; it adds no operation. | `region[name="Timeline"]` |

**Two terminology decisions, settled here and since carried out in phase 003** (amendment
2026-08-27, ANALYSIS N5). A landmark string is a gate assertion, so a rename that is not
propagated to the collection, the route and the spec noun does not produce a nicer word — it
produces a red Playwright run and a spec nobody can grep. **One word per concept, used for the
spec noun, the collection, the route segment and the landmark:**

1. **`tags` / `/tags` / `region[name="Tags"]`.** An earlier draft of 003 rendered
   `region[name="Labels"]` and said "label" throughout its spec, while the collection was `tags`,
   the route `/tags`, the relation field on every clinical kind `tags` and the query parameter
   `?tags=`. "Label" added nothing that "tag" does not already carry. **Tags won.**
2. **`encounters` / `/encounters` / `region[name="Encounters"]` / `article[name="Encounter"]`.**
   The same draft rendered `region[name="Appointments"]` / `article[name="Appointment"]`.
   **Encounters won**, and not only for consistency: an appointment is a booking for something
   that has not happened yet, whereas this kind records a **visit that already took place** — its
   required field is `occurred_on` and it carries an assessment and a plan. "Appointments" was
   simply the wrong word for the data.

**Both corrections have landed.** `003/plan.md:449` records them complete in that phase's
deviation table, and `003/contracts/pages.md:37` and `:60` carry the settled landmarks
`region[name="Encounters"]` / `article[name="Encounter"]` and `region[name="Tags"]`. Both were
landmark-string changes only; neither touched the collection, the route, the DTO or the registry.

**`Labels`, `Label`, `Appointments` and `Appointment` are dead names.** They appear in no phase
document and must not be re-introduced; the only surviving mention is the past-tense deviation row
at `003/plan.md:449`. A document that re-asserts either is wrong.

**Phase 004 — 4 pages + 4 page-action routes**

| Route | Purpose | Page landmark |
|---|---|---|
| `/lab-results` | Kind list, panel-aware; each row shows how many components are out of range. | `region[name="Lab results"]` |
| `/lab-results/{id}` | The result with its component table, the link editor and the attachment strip. | `article[name="Lab result"]` |
| `/labs/trends` | Component catalog + trend chart, unit-scoped, direction stated with its rule. | `region[name="Lab trends"]` |
| `/documents` | Cross-record attachment library, including the trash as `?deleted=true`. | `region[name="Documents"]` |

**Phase 005 — 3 pages**

| Route | Purpose | Page landmark |
|---|---|---|
| `/sharing` | Granted and received shares, change level, change end date, revoke and leave. | `region[name="Sharing"]` |
| `/invitations` | Received and sent invitations, respond, cancel and withdraw. | `region[name="Invitations"]` |
| `/invite/{token}` | **NEW. Public.** Where the emailed link lands: sender display name, kind, item count, level, lapse date, note, masked address hint — then sign in or create an account. Without it operation 64 has no surface and an invited stranger has nowhere to go. | `region[name="Invitation"]` |

005 also widens existing pages (`/patients`, `/patients/{id}`, `/documents`, `/search`, `/timeline`
and every status view) to render shared charts, adding no page.

**Phase 006 — 7 pages + 3 page-action routes**

| Route | Purpose | Page landmark |
|---|---|---|
| `/reports` | Builder, saved reports, and the documents produced from them. | `region[name="Reports"]` |
| `/reports/{id}` | A saved template's editor. | `article[name="Report template"]` |
| `/exports` | Export jobs, queue position, progress, download, cancel. | `region[name="Exports"]` |
| `/admin` | Operator dashboard tiles with definitions and `computed_at`, MFA/IP-allowlist/SMTP posture warnings. | `region[name="Administration"]` |
| `/admin/audit` | Activity trail with filters and CSV export. | `region[name="Audit trail"]` |
| `/admin/users` | Account list, tier, disable, forced password change. | `region[name="Users"]` |
| `/admin/backups` | List, take, upload, preview, download, restore, delete. | `region[name="Backups"]` |

**Page-action routes are a fourth route class, and they are not pages.** `Kind: page_action` in
`medigo routes`, deliberately **excluded** from `api/openapi.json`, no ARIA landmark, and each one
**declares the Playwright spec that exercises it** — `e2e/routes.gate.spec.ts` fails the build if
that spec does not exist or does not reference the route. Seven exist: four in 004 (the multipart
upload action and three HTML fragments) and three in 006 (`GET /reports/selection`,
`GET /reports/jobs`, `GET /exports/jobs`). All are plain `text/html` responses, not SSE — Datastar
honours an HTML response as an element patch, and a job list is not worth a long-lived connection
or exposure to PocketBase's five-minute `WriteTimeout`.

**The three auth pages are phase 001's and ARE in the route registry.** An earlier revision of this
document deferred `/forgot-password`, `/reset-password/{token}` and `/verify-email/{token}` out of
the suite; they are built by phase 001 with operations 7, 8, 94 and 95 (§2.3), and they are inside
the 58. Under Principle IX the page inventory, the `medigo routes` output and the Playwright route
list are one list, so a page in this table and absent from the application fails the gate — and so
does the reverse. **External sign-in adds no page**: phase 006 adds a provider control to the
existing `/login` page, rendered only when a provider is configured.

> **`/trash` was removed from this inventory** (was phase 006's eighth page). Phase 004 already
> ships the attachment trash as the `?deleted=true` filter on `/documents` — listing, restore,
> days-remaining and the early purge — and a second surface for recovering the same document is
> exactly the duplication phase 006's specification forbids in as many words. Phase 006 owns only
> the instance-wide operator figures (count and bytes on `/admin`) and links to
> `/documents?deleted=true`.

### 3.2 The two-page-per-kind pattern

Each registered kind contributes exactly two pages:

- `/{kind-plural}` — list, filters, sort, empty state, and a **create drawer opened by a Datastar
  signal, not a route**. Live-updated by `/api/v1/streams/records`.
- `/{kind-plural}/{id}` — detail with inline edit, links, attachments, delete-with-confirmation.

There is deliberately no `/new` and no `/edit` route: those are UI states, and each one would
otherwise cost 15 routes and 15 smoke cases for nothing. Deep-linking to a
blank form is not a requirement anyone has.

Both pages, their landmarks and their `SmokeURL` (with the seeded id substituted) are emitted by
`records.Register(kind, …)`. **Adding a kind without adding a test is impossible**, which is what
Principle VIII asks for.

### 3.3 Facts about this UI that must be stated, not discovered

- **MediGo does not work with JavaScript disabled.** Datastar binds nothing, `data-bind`
  populates no signals, and a form submit sends an empty body. This is structural, not a bug, and
  the spec says so.
- `script-src 'unsafe-eval'` is permanent and accepted. Every other CSP directive is strict.
- The Datastar bundle (JS runtime **v1.0.2** — a different version line from the Go SDK's v1.2.2;
  do not "align" them) is vendored and embedded. No CDN.
- Only the **free** Datastar attribute set may be used. `data-persist`, `data-query-string`,
  `data-replace-url`, `data-scroll-into-view`, `data-view-transition`, `data-custom-validity`,
  `data-animate`, `data-match-media`, `data-on-raf`, `data-on-resize`, `@clipboard` and `@fit` are
  **Pro**. A lint rule enforces the allowlist. UI preferences persist on the `users` record and
  hydrate through `data-signals` — better than `localStorage` for a medical app anyway.
- The v1 attribute delimiter is a colon: `data-on:click`. `data-on-click` **silently does
  nothing**. `data-on-load` is now `data-init`.
- The inline-script SDK family is banned outright: `ExecuteScript`, `ConsoleLog`, `ConsoleError`,
  `Redirect`, `Redirectf`, `DispatchCustomEvent`, `ReplaceURL`, `ReplaceURLQuerystring`,
  `Prefetch`. All of them append a literal inline `<script>`, all of them fail under
  `script-src 'self'`, and each failure logs a CSP violation that fails the console gate.
  Redirect is a `303` issued *before* opening the stream; user-visible errors go to
  `#error-banner`.
- Most interactions need **no SSE at all** — Datastar honours a plain `text/html` response as an
  element patch. Streams are reserved for genuinely live views, which minimises exposure to the
  five-minute write-timeout trap.

---

## 4. THE PACKAGE LAYOUT

```
cmd/medigo/
  main.go                 Composition root. Builds config, logger, container, PocketBase, registry; wires Cobra subcommands; the ONLY place that panics.

internal/config/          The one caarlos0/env struct + boot validation. No other config mechanism exists.
internal/logging/         zerolog setup, request-scoped loggers, redaction helpers, the two-part PocketBase bridge (pb.App decorator + OnModelCreate("_logs") interceptor). [PB]
internal/obs/             Prometheus registry and collectors, OTel tracer bootstrap, Sentry init + BeforeSend scrubber, the otelsql DBConnect wiring. [PB]
internal/di/              samber/do v2 container and providers. Composition root only; no runtime MustInvoke.

internal/domain/          Entities, value objects, enums with Valid(), sentinel and typed errors, redacting MarshalJSON/MarshalZerologObject. Imports nothing but stdlib.
internal/domain/kind/     The Kind type, its 15 values, and the two spellings (enum value, path segment). One source of truth for the whole app.
internal/domain/access/   Actor, Permission, Grant, PatientRef. No behaviour, no I/O.

internal/records/         The kind registry: Register(kind, Service, Codec, Views, …). Owns the generic record handler's dispatch table, the audit/search/attachment hooks, the OpenAPI oneOf branches, and the two page routes per kind.
internal/service/allergy/     ─┐
internal/service/condition/    │ One package per record kind. Each owns: its Service (business rules,
internal/service/medication/   │ validation, authorization calls), the Repository interface AS THE
internal/service/…/           ─┘ SERVICE NEEDS IT, and its unit tests against a fake.
internal/service/patient/     Patients, ownership, self-record invariant, active-patient resolution.
internal/service/access/      THE Authorizer. The single authorization checkpoint for the whole application.
internal/service/attachment/  Upload, MIME sniffing, eager thumbnails, trash, retention purge.
internal/service/share/       Shares + invitations + the state machine.
internal/service/audit/       Audit writer and reader.
internal/service/search/      Index maintenance and query.
internal/service/report/      Summaries, trends, templates, export job orchestration.
internal/service/admin/       Dashboard aggregation, user administration, backup orchestration.

internal/store/               PocketBase adapter root: collection helpers, filter builders, transaction helper, record↔domain mapping. [PB]
internal/store/migrations/    Every collection as a reversible Go migration; the nil-rule and Protected:true assertions. [PB]
internal/store/<kind>/        One repository implementation per kind, satisfying the service's interface. [PB]

internal/platform/pb/         PocketBase construction, ServeEvent wiring, the WriteTimeout override, the lockdown middleware, boot assertions, cron registration, hook registration. [PB]
internal/realtime/            The in-process hub. Publishes record IDs, never bodies. Single-instance by construction; NOT behind a broker interface.

internal/httproute/           The declarative route registry: Route, Registry, Handle, Bind, Routes, MarshalJSON. Registering and describing a route are the same call. [PB]
internal/openapi/             Generates api/openapi.json from the registry + DTO reflection. Gate tests live here.

internal/web/                 Shared HTTP layer: error envelope, DTO decode/encode, ETag/If-Match, pagination cursors, actor middleware, CSP and security headers. [PB]
internal/web/api/             /api/v1 JSON handlers + request/response DTOs. Declares the service interfaces it consumes. [PB]
internal/web/page/            templ page handlers. Declares the service interfaces it consumes. [PB]
internal/web/stream/          Datastar SSE handlers and the mandatory newStream() helper (write deadline, X-Accel-Buffering, no-store, SkipSuccessActivityLog). [PB]
internal/web/views/           .templ sources + generated *_templ.go. Layout, shell, per-kind list/detail/row components, empty states, error views.
internal/web/views/ids/       Deterministic DOM ids, used by BOTH the templ render and the Go patch call.
internal/web/static/          Embedded assets: app.css (Tailwind output), datastar.js (vendored v1.0.2), icons.

internal/cli/                 Cobra subcommands: seed, routes, openapi, healthcheck, purge. [PB]
internal/testsupport/         Test app factory, seeded fixture helpers, contract-test suites, fakes.
internal/testdata/pb_data/    The committed fixture data dir that every tests.NewTestApp clones.

api/openapi.json              Committed, diffed, gated.
e2e/                          Playwright config + specs. Build-time only; no Node in the runtime image.
assets/input.css              Tailwind entrypoint; scans ../internal/web/**/*.templ.
```

### 4.1 Principle II, made mechanical

**The only packages permitted to import `github.com/pocketbase/pocketbase/...` are the ten marked
`[PB]` above:** `internal/logging`, `internal/obs`, `internal/store/**`, `internal/platform/pb`,
`internal/httproute`, `internal/web` (and its four subpackages), `internal/cli`, and
`cmd/medigo`. `internal/testsupport` may import `pocketbase/tests`.

**No other package may.** In particular `internal/domain/**` and `internal/service/**` may import
neither PocketBase, nor `net/http`, nor `github.com/a-h/templ`, nor any generated `*_templ.go`
package. This is enforced by the `depguard` rule in `.golangci.yml`, wired into CI — it is a build
gate, not a review convention (Principle IX).

`internal/realtime` imports neither PocketBase nor `net/http`: the hub trades in
`realtime.Event{Kind, RecordID, PatientID}` values. The publisher side lives in
`internal/platform/pb` and the subscriber side in `internal/web/stream`.

### 4.2 Interfaces: declared by the consumer, implemented elsewhere

| Interface | Declared in | Implemented in |
|---|---|---|
| `medication.Repository` (and one per kind) | `internal/service/medication` | `internal/store/medication` + a fake in `internal/service/medication/medicationtest` |
| `medication.Authorizer` (and one per service) | each `internal/service/<kind>` | `internal/service/access` |
| `medication.Auditor`, `medication.Indexer` | each `internal/service/<kind>` | `internal/service/audit`, `internal/service/search` |
| `api.MedicationService` / `records.Service` | `internal/web/api` and `internal/records` | `internal/service/medication` |
| `page.PatientReader` | `internal/web/page` | `internal/service/patient` |
| `stream.Subscriber` | `internal/web/stream` | `internal/realtime` |
| `access.ShareReader` | `internal/service/access` | `internal/store/share` |

Every interface is one to three methods. **There is no `Store` and no `Service` omnibus
interface.** Each interface has at least two implementations before it is introduced — the real
one and the fake that satisfies Principle III — and every implementation, fake included, runs the
same `suite.Suite` contract test (Principle II's Liskov clause).

---

## 5. THE SEAMS — the medications vertical slice, in full

This is the template. Phases 002–006 copy it verbatim, changing only the kind.

### 5.1 Domain — `internal/domain/clinical` (no PocketBase, no HTTP, no templ)

```go
package clinical

// Medication is the domain entity. It never crosses the wire; a DTO always mediates.
type Medication struct {
	ID          string
	PatientID   string
	Name        string        // PHI
	AltName     string        // PHI
	Type        MedicationType
	Dosage      string
	Frequency   string
	Route       MedicationRoute
	Indication  string        // PHI
	StartedOn   *Date
	EndedOn     *Date
	Status      TherapyStatus
	SideEffects string        // PHI
	Notes       string        // PHI
	PractitionerID string
	PharmacyID     string
	Reminder    Reminder
	TagIDs      []string
	CreatedAt, UpdatedAt time.Time
	Version     string        // the ETag source
}

// Validate returns a *domain.ValidationError describing every offending field at once.
func (m Medication) Validate() error

// MarshalZerologObject emits ONLY the id and the patient id. Constitution VII: logging a
// domain type by accident must not be able to leak PHI.
func (m Medication) MarshalZerologObject(e *zerolog.Event)

// MarshalJSON is deliberately NOT implemented: a domain type has no wire form.
```

### 5.2 Repository — declared by the service, implemented by the store

```go
package medication // internal/service/medication

// Repository is what THIS service needs. Four methods; nothing generic, nothing speculative.
type Repository interface {
	Get(ctx context.Context, id string) (clinical.Medication, error)
	List(ctx context.Context, q Query) (page.Page[clinical.Medication], error)
	Save(ctx context.Context, m clinical.Medication) (clinical.Medication, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

// Query is a typed filter. PocketBase's filter DSL never appears above internal/store.
type Query struct {
	PatientID string
	Status    []clinical.TherapyStatus
	TagIDs    []string
	Search    string
	From, To  *clinical.Date
	Sort      []page.SortKey
	Limit     int
	Cursor    page.Cursor
}
```

```go
package medication // internal/service/medication — the other two consumer-declared ports

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
	Record(ctx context.Context, actor access.Actor, k kind.Kind, recordID string, need access.Permission) (access.Grant, error)
}

type Auditor interface {
	Record(ctx context.Context, ev audit.Event) error
}
```

Implemented by `internal/store/medication.Repo` (which imports PocketBase and maps
`*core.Record` ↔ `clinical.Medication`) and by `internal/service/medication/medicationtest.Fake`.
Both run `medicationtest.RepositoryContract(t, factory)`.

### 5.3 Service — declared by the handler, implemented here

```go
package medication // internal/service/medication

type Service struct { // constructor takes interfaces, returns a concrete type
	repo  Repository
	authz Authorizer
	audit Auditor
	index Indexer
	log   zerolog.Logger
}

func New(repo Repository, authz Authorizer, aud Auditor, idx Indexer, log zerolog.Logger) *Service

func (s *Service) List(ctx context.Context, actor access.Actor, q Query) (page.Page[clinical.Medication], error)
func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (clinical.Medication, error)
func (s *Service) Create(ctx context.Context, actor access.Actor, m clinical.Medication) (clinical.Medication, error)
func (s *Service) Update(ctx context.Context, actor access.Actor, id, ifMatch string, patch Patch) (clinical.Medication, error)
func (s *Service) Delete(ctx context.Context, actor access.Actor, id, ifMatch string) error

// Patch carries absent-vs-zero with plain pointers. samber/mo is forbidden.
type Patch struct {
	Name      *string
	Dosage    *string
	Status    *clinical.TherapyStatus
	EndedOn   **clinical.Date // **T: outer nil = absent, inner nil = explicit null
	TagIDs    *[]string
	// …
}
```

**Every method's first act is `s.authz.Patient(...)` or `s.authz.Record(...)`.** The repository
never authorizes. The handler never authorizes. There is exactly one checkpoint.

### 5.4 The handler seam — `internal/web/api` and `internal/records`

```go
package records // internal/records

// Service is what the ONE generic record handler needs from every kind. Five methods, and the
// per-kind types are hidden behind the codec, so nothing ever switches on kind.Kind.
type Service interface {
	List(ctx context.Context, actor access.Actor, q ListQuery) (ListResult, error)
	Get(ctx context.Context, actor access.Actor, id string) (Payload, error)
	Create(ctx context.Context, actor access.Actor, body json.RawMessage) (Payload, error)
	Update(ctx context.Context, actor access.Actor, id, ifMatch string, body json.RawMessage) (Payload, error)
	Delete(ctx context.Context, actor access.Actor, id, ifMatch string) error
}

// Register wires one kind into: the record routes, the OpenAPI oneOf, the two pages, the audit
// hook, the search hook and the attachment-cleanup hook. Adding a kind is one call.
func Register(k kind.Kind, svc Service, views Views, schema openapi.Schema)
```

`internal/service/medication.Adapter` (a thin type in the same package, ~40 lines) implements
`records.Service` by decoding `api.MedicationCreate` / `api.MedicationPatch` and encoding
`api.Medication` / `api.MedicationSummary`. That adapter is the only place per kind that knows
about JSON.

### 5.5 DTOs — `internal/web/api`

```go
package api

// MedicationSummary is what list endpoints return. Detail endpoints return Medication.
type MedicationSummary struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`             // always "medication"; the oneOf discriminator
	Name      string  `json:"name"`
	Dosage    string  `json:"dosage,omitempty"`
	Frequency string  `json:"frequency,omitempty"`
	Status    string  `json:"status"`
	StartedOn *string `json:"started_on,omitempty"` // "YYYY-MM-DD"
	Tags      []TagRef `json:"tags"`                // never nil; json/v2 nil-vs-empty matters
	UpdatedAt string  `json:"updated_at"`
}

type Medication struct {
	MedicationSummary
	AltName      string           `json:"alt_name,omitempty"`
	Type         string           `json:"type,omitempty"`
	Route        string           `json:"route,omitempty"`
	Indication   string           `json:"indication,omitempty"`
	EndedOn      *string          `json:"ended_on,omitempty"`
	SideEffects  string           `json:"side_effects,omitempty"`
	Notes        string           `json:"notes,omitempty"`
	Practitioner *PractitionerRef `json:"practitioner,omitempty"`
	Pharmacy     *FacilityRef     `json:"pharmacy,omitempty"`
	Reminder     *Reminder        `json:"reminder,omitempty"`
	Treatments   []TreatmentRef   `json:"treatments"`   // back-relation, read-only
	Attachments  []AttachmentRef  `json:"attachments"`
}

type MedicationCreate struct { Patient string `json:"patient"`; /* …every writable field… */ }
type MedicationPatch  struct { Name *string `json:"name,omitempty"`; /* …pointers… */ }
```

Rules: unknown fields rejected; slices marshal as `[]` not `null`; dates are `YYYY-MM-DD`
strings, instants are RFC3339 UTC; no PocketBase type ever appears in a DTO; `MedicationCreate`
and `MedicationPatch` omit every server-owned field by construction, which is how privilege
escalation is prevented (upstream fixed the same class of bug twice, by DTO shape, after a CVE).

### 5.6 The templ component

`internal/web/views/records/medication.templ` provides exactly three components:

```go
templ MedicationRow(m api.MedicationSummary)    // root id = ids.RecordRow(kind.Medication, m.ID)
templ MedicationList(p api.Page[api.MedicationSummary]) // root id = ids.RecordList(kind.Medication)
templ MedicationDetail(m api.Medication)        // root id = ids.RecordDetail(kind.Medication, m.ID)
```

They are registered via `records.Views{Row: MedicationRow, List: MedicationList, Detail:
MedicationDetail}`. The page handler in `internal/web/page` renders `Layout` around them; the SSE
handler in `internal/web/stream` re-fetches by id, **re-authorises for that subscriber**, and
patches `MedicationRow` with `datastar.WithSelectorID(ids.RecordRow(...))`.

**A templ component never receives a domain type** — only a DTO. That is what keeps
`internal/web/views` out of the domain's dependency graph and keeps PHI redaction meaningful.

---

## 6. CROSS-CUTTING CONVENTIONS

### 6.1 Error taxonomy and HTTP mapping

`internal/domain` owns the taxonomy. One mapping function in `internal/web`, used by every
handler; no handler writes a status code literal.

| Sentinel / type | Status | Envelope `code` |
|---|---|---|
| `ErrUnauthenticated` | 401 | `unauthenticated` |
| `ErrForbidden` (resource already known to the caller) | 403 | `forbidden` |
| `ErrNotFound`, **and every authorization failure on patient-scoped data** | 404 | `not_found` |
| `ErrVersionMismatch` (`If-Match`) | 412 | `version_mismatch` |
| `*ValidationError` | 422 | `validation_failed` (+ `fields[]`) |
| `ErrConflict` (uniqueness, invariant) | 409 | `conflict` |
| `ErrTooLarge` | 413 | `payload_too_large` |
| `ErrUnsupportedMedia` | 415 | `unsupported_media_type` |
| `ErrRateLimited` | 429 | `rate_limited` |
| `context.DeadlineExceeded` / `Canceled` | 499 / 504 | `client_closed` / `timeout` |
| anything else | 500 | `internal_error`, message always the literal `"internal error"` |

Errors are values, wrapped with `%w`, inspected with `errors.Is`/`errors.As`. This is precisely
why `samber/mo` is forbidden: `mo.Result` severs the chain that this table, the Sentry
integration and zerolog's `Err()` all depend on. Only the 500 branch reports to Sentry, and only
once (the zerolog Sentry hook checks whether the error is already captured).

### 6.2 Validation

Three layers, with one authority.

1. **Decode** (`internal/web`): shape only. Unknown fields, wrong types, malformed dates → 422
   before any business code runs. Go 1.27's `encoding/json/v2` semantics apply; DTO round-trip
   tests are mandatory because nil-vs-empty slices, `json.RawMessage` and duplicate keys are not
   fully backward compatible.
2. **Domain** (`internal/domain`): **the authority.** `Validate()` on every entity returns a
   `*ValidationError` listing *every* offending field, so a form shows all its errors at once.
   Cross-field rules live here (`ended_on >= started_on`; a `qualitative` lab component must have
   `value_text` and no `value`; `is_primary` is unique per patient; `is_self_record` is unique per
   owner; a `family_member` share is always `view`).
3. **Storage** (`internal/store/migrations`): PocketBase field constraints, select vocabularies
   and unique indexes as the last line of defence. Never the only line.

Validation lives in **none** of: handlers, templ components, repositories, or PocketBase hooks.

### 6.3 Pagination

```go
type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
	Total      *int    `json:"total,omitempty"` // only when ?count=true
}
```
Cursors are opaque, HMAC-signed, and encode `(sort keys, last values, last id)` — never an offset,
so a concurrent insert cannot duplicate or skip a row. `limit` default 25, max 100. Every list
endpoint in the API, without exception, uses this shape; `GET /api/v1/search` uses **one per
kind group**, because a per-type limit with a single global `has_more` is what made upstream's
search pagination incorrect.

### 6.4 The audit trail

Shape is §1.2. Three rules:

- **Populated by hooks, not by handlers.** `records.Register` binds
  `OnRecordAfterCreateSuccess` / `…UpdateSuccess` / `…DeleteSuccess` for the kind's collection.
  Post-commit, so a rolled-back transaction never produces an audit row. Non-record actions
  (login, export, restore, admin session) call `audit.Record` explicitly.
- **`OnRecord*Request` hooks are forbidden** — they are bound inside the built-in CRUD handlers,
  which the lockdown disables, so anything placed there is silently dead code. A `forbidigo`
  pattern enforces this.
- **Content never enters it.** Actor, action, target kind, opaque target id, patient id,
  timestamp, request id. No names, no values, no diffs, no filenames, and **no IP address** — it
  is personal data about the actor retained for two years in a medical-records application, no
  requirement asks for it, and no phase creates the column (amendment 2026-08-27, ANALYSIS C2).

Every admin-UI session produces an entry (constitution VII), sourced from `OnRecordAuthRequest`
on `_superusers` plus a per-request marker on `/_/` and `/api/collections/_superusers/*`.

### 6.5 The authorization checkpoint

**One place: `internal/service/access.Authorizer`.**

```go
type Authorizer interface {
	Patient(ctx context.Context, actor Actor, patientID string, need Permission) (Grant, error)
	Record(ctx context.Context, actor Actor, k kind.Kind, recordID string, need Permission) (Grant, error)
}
type Permission int // PermView | PermEdit | PermOwn
type Grant struct { Level Permission; ViaShare string; PatientID string }
```

Resolution, in order: superuser → allow (and audit); patient owner → `PermOwn`; an active share
(`revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now)`) → its level; otherwise
`ErrNotFound`.

- **Expiry is evaluated in the read path**, in the repository query, not by a cron sweep. That
  single choice deletes `patient-sharing/cleanup-expired` and `invitations/cleanup` from the
  correctness path; a cron remains only for tidiness.
- Permission is a property of the route, **never a client-supplied parameter**. Upstream's
  `required_permission` query parameter — present on 41 operations, defaulting to `view` even on
  writes — does not exist in MediGo.
- Collection API rules are **not** a second authorization layer: they are all `nil`. Defence in
  depth comes from the boot assertion, the `-1019` prefix middleware, and a
  `tests.ApiScenario` per collection proving the auto-CRUD route 404s for a normal user.
- Principle III makes this testable and tested: every patient-touching endpoint has three tests —
  a stranger is refused, a grantee succeeds, a revoked grantee is refused.

### 6.6 Active patient resolution per request

1. `internal/web` middleware reads the PocketBase auth record from `e.Auth` and builds
   `access.Actor{UserID, Role, IsSuperuser, RequestID}` into the request context. This is the only
   thing derived from the token.
2. **API handlers take the patient from the request** — `?patient=` (lists), the body (creates),
   or the record itself (reads/updates/deletes). Absent on a list → `400 patient_required`. There
   is no fallback.
3. **Page handlers** read `users.active_patient` purely to pre-fill the switcher and to redirect
   `/{kind}` to `/{kind}?patient={active}`. If the active patient is no longer accessible the
   pointer resolves to null and the user lands on `/patients`.
4. `PUT /api/v1/me/active-patient` authorizes the target before writing the pointer.

Upstream ran both mechanisms simultaneously, which is the single biggest source of its 500-route
sprawl. MediGo runs exactly one.

### 6.7 Configuration

One struct, `internal/config.Config`, `caarlos0/env/v11`, global prefix `MEDIGO_`, validated at
boot, no config files, no second mechanism **that MediGo defines**. Top-level: `Env`, `Dev`,
`DataDir` (`/data/pb_data`), `HTTPAddr`, `PublicURL` (`required,notEmpty`), `DrainDelay`,
`DrainMax`, `TrustedProxies`.
Nested with `envPrefix`: `LOG_` (level, pretty), `SENTRY_` (dsn `file,unset`, environment,
sample_rate), `METRICS_` (enabled, addr `127.0.0.1:9090`), `OTEL_` (enabled, endpoint, insecure,
sample_ratio, headers), `FILES_` (max_upload_bytes, allowed_mime, thumb sizes),
`AUTH_` (registration_open, require_verified_email, session_ttl, oauth2_enabled),
`RETENTION_` (trash_days=30, audit_days=730, export_days=7, backup_keep=14).

**There is no `MEDIGO_ADMIN_*` group, and there must not be.** Constitution v1.3.0 carves
PocketBase's own `Settings()` store — SMTP, S3, backup schedule, OAuth2 providers, the superuser IP
allowlist and superuser MFA — out of the "caarlos0/env is the only configuration mechanism" rule:
it is database-backed, edited through the admin UI, and part of the platform Principle V forbids
reimplementing. **MediGo MUST NOT mirror any of it into environment variables and MUST NOT build
its own UI for it.** What MediGo does instead is *read* it at boot and warn loudly when a setting
its features depend on is unconfigured — the IP allowlist from `app.Settings().SuperuserIPs`, MFA
from the superusers collection's `MFA.Enabled`, SMTP from `Settings().SMTP` (§8 R6 and R10, and
VERIFIED-SOURCE-FACTS FACT 10). The posture is surfaced by operation 81 and on `/admin`.

Secrets arrive via `,file,unset` so they never sit in `os.Environ()` after parsing, are never
logged, and are never persisted in plaintext.

### 6.8 Metric and span naming

**Metrics:** `medigo_<subsystem>_<name>_<unit>`. Subsystems: `http`, `db`, `sse`, `records`,
`auth`, `shares`, `files`, `exports`, `backup`, `build`.
Labels are bounded and allowlisted: `route` (the **registered pattern** from
`http.Request.Pattern`, never the resolved path), `method`, `status`, `kind` (15 values),
`op`, `outcome`, `reason`. **No patient id, user id, record id, filename, tag name or test name
ever becomes a label** — that is unbounded cardinality *and* PII in the monitoring system.
`/metrics` binds to `127.0.0.1` and is not publicly reachable.

**Spans:** server spans are `HTTP {METHOD} {route pattern}`; service spans are
`service.{package}.{Method}`; store spans are `store.{collection}.{op}`; otelsql emits `db.query`.
Attributes are an allowlist: `http.*`, `db.system`, `medigo.kind`, `medigo.op`, `medigo.result`,
`medigo.request_id`. Never `medigo.patient_id` and never a free-text field.

**Logs:** zerolog only, JSON to stdout, one event per meaningful occurrence, every line carrying
`request_id` and — when tracing is on — `trace_id` and `span_id`. `app.Logger()` is banned by
`forbidigo`. PocketBase's own logs arrive through both bridge mechanisms with
`Settings().Logs.MaxDays = 1` (never 0) and an `OnModelCreate("_logs")` interceptor that returns
without calling `e.Next()`, so `_logs` stays permanently empty and nothing is lost.

Sentry receives errors and panics only, scrubbed, with request bodies disabled and PII off. The
three systems must not double-report one event.

---

## 7. POCKETBASE PROVIDES vs MEDIGO BUILDS

| Concern | PocketBase provides (do NOT rebuild) | MediGo builds |
|---|---|---|
| **HTTP server & router** | `apis.NewRouter`, route groups, middleware chain, `WrapStdHandler`/`WrapStdMiddleware` for OTel/Prometheus/Sentry, rate limiting | The `httproute.Registry`, every `/api/v1` handler, the CSP and security headers (PB's `securityHeaders()` sets **no** CSP for app routes), the `-1019` lockdown middleware, the `WriteTimeout` override |
| **Persistence** | SQLite (pure-Go `modernc.org/sqlite`), migrations with enforced up+down, `RunInTransaction`, indexes, relations, cascade delete, back-relation traversal | Collection definitions as Go migrations, typed repositories, the domain↔record mapping, all query building |
| **Authentication** | Password auth, token issue/refresh/duration, **password reset**, email verification, email change, MFA/OTP, `_externalAuths`, superuser impersonation | Thin `/api/v1/auth/*` DTO wrappers completing through `apis.RecordAuthResponse`, session cookie handling, the registration transaction (user + self-record patient), login auditing |
| **OAuth2 / SSO** | Provider config, the whole authorize/callback exchange, **linking a provider to an existing auth record**, `/api/oauth2-redirect` | One `POST /api/v1/auth/oauth2` DTO wrapper (op 4, **phase 006**). **The entire hand-rolled SSO subsystem is deleted** — `initiate`, `callback`, `resolve-conflict`, `resolve-github-link`, `temp_token`, `sso_metadata`, `sso_linking_preference`, `account_linked_at`, `external_id`, `auth_method` |
| **File storage** | Local or S3 backends (config, not code), MIME/size validation, `MaxSelect`, deletion with the owning record, `filesystem.Serve` with Range/ETag/Content-Disposition quoting, `CreateThumb` | **All file serving.** PB's `/api/files/` route is unusable both ways: unprotected fields are served to anonymous callers (`apis/file.go:109` has no `else`), and protected fields 404 for everyone under nil `ViewRule`. So: `Protected: true` everywhere + boot assertion, MediGo's own `/api/v1` routes over `app.NewFilesystem()`, **eager** thumbnails in a `TxInfo().OnComplete` callback, no file tokens (a credential in a URL is a PHI leak) |
| **Backup / restore** | `app.CreateBackup` / `RestoreBackup`, `/api/backups`, cron auto-backup with `maxKeep`, optional S3 backup storage, the dashboard UI | Four thin wrappers, plus the two things worth keeping from upstream: **restore preview** and **an automatic safety backup before every restore**. Upstream's `create-database`/`create-files`/`create-full` split, three cleanup variants, `retention/stats`, schedule settings and history CSV are all deleted |
| **Admin UI** | `/_/` — collection browser, per-record CRUD, filters, relation navigation, schema view, settings, backups, cron inspector, logs viewer. **It ships in production.** | **No bespoke replacement.** Hardening only: mandatory superuser MFA, mandatory IP allowlist, every admin session audited, a loud boot warning when either is unconfigured. Upstream's 9 `/admin/models/*` routes, its reflection layer, both `/admin/bulk/*` routes and its generic CSV exporter are deleted |
| **Collection API rules** | A full rule language with relation traversal and `@now` comparison | **Not used as authorization.** All five rules are `nil` on every collection — superuser-only — because a rule expressive enough to encode sharing would re-open `GET /api/collections/patients/records` as a second, undocumented, un-versioned public API. Defence in depth is the boot assertion + the prefix middleware + a per-collection `ApiScenario` proving the CRUD route 404s. `AuthRule` stays `""`; it is not one of the five |
| **Realtime** | `/api/realtime`, `SubscriptionsBroker` | **100% MediGo's.** PB's native realtime is unusable for three independent verified reasons (rules derived from nil `ViewRule`/`ListRule` so every broadcast is silently skipped; its event names are not Datastar's two; its two-step handshake is impossible from a Datastar attribute). MediGo runs an in-process hub fed by post-commit record hooks that publishes **IDs only**, and per-subscriber Datastar SSE handlers that re-fetch, **re-authorise**, render and patch |
| **Logging** | `_logs` aux DB, retention setting, dashboard viewer | Everything. `_logs` is intercepted to zero rows; zerolog is the only stream. PB's own framework logs are bridged by both mechanisms |
| **Scheduling** | `app.Cron()` | The jobs: trash purge, audit purge, export-artifact purge, invitation/share tidy sweep, storage-footprint gauge refresh, export worker |
| **CLI** | `RootCmd` is a real `*cobra.Command`, fully bootstrapped for custom subcommands | `medigo seed`, `medigo routes`, `medigo openapi`, `medigo healthcheck`, `medigo purge`. **No second CLI framework.** Two traps: the root sets `FParseErrWhitelist{UnknownFlags: true}` (validate flags in `RunE`) and `SetErr(&nopWrite{})` discards cobra's error output (print it in `main`) |
| **Testing** | `tests.NewTestApp` (clones a fixture data dir into a temp dir; genuinely isolated, `t.Parallel()`-safe, ~11 ms), `tests.ApiScenario` with `ExpectedEvents` | The fixture data dir, the fakes, the contract suites, the Playwright gate. **Never share a `TestApp` across scenarios** — `bindUIExtensions` re-enters on every `OnServe` and the handler chain grows until the stack overflows |
| **Not provided at all** | — | Row-level sharing, invitations, the domain audit trail, cross-collection search, reporting, export, tags-as-relations, multi-patient ownership and switching, the entire UI |

---

## 8. RISK REGISTER

**Three risks are CLOSED by evidence read from real module source and recorded in
`VERIFIED-SOURCE-FACTS.md`. They are kept in the table, marked, rather than deleted — a risk that
vanishes without a record looks like a risk nobody ever thought about.**

| # | Risk | Why it matters | Phase to close it |
|---|---|---|---|
| ~~R1~~ | **CLOSED — VERIFIED-SOURCE-FACTS FACT 9.** OpenAPI `oneOf` + discriminator generation from Go types. **Proven by building and running it**, ahead of phase 001, with `getkin/kin-openapi` **v0.144.0** (pure Go, cgo-free, a document model and not an HTTP framework, so it does not collide with the ban on a second router; admitted to the stack by constitution v1.3.0). Two kinds were registered, one component schema emitted per kind, one `Record` schema with `oneOf` + `discriminator {propertyName: "kind", mapping}`, one path `POST /api/v1/records/{kind}` with `kind` as an `enum`; the document was marshalled, **reloaded through `openapi3.NewLoader()`** and validated, and every registered kind was asserted present in the discriminator mapping. Two traps for whoever implements it: the gate MUST marshal-then-load (validating in place fails with "found unresolved ref", because a programmatically built `SchemaRef` carries only `Ref` and no `Value`), and `Discriminator.Mapping` is `map[string]openapi3.MappingRef`, not `map[string]string`. | **This was the one risk that could have invalidated a phase, and it did not.** The 90-operation shape held: §2.3's budget (now 94 for unrelated reasons) and phase 003's **3-route** shape both stand, and the record route family is safe to build on. | **CLOSED.** 001 still carries the task — with two kinds — as a permanent regression gate rather than as an open question. |
| R2 | **Go 1.27 `encoding/json/v2` retrofit semantics in MediGo's own DTOs.** nil-vs-empty slices, `json.RawMessage`, duplicate-key rejection and case-insensitive matching are not fully backward compatible, and `tests.ApiScenario` normalises bodies through `jsontext` before substring matching — so `ExpectedContent` compares against *re-encoded* JSON. | Silent wire-shape changes in a medical API. | 001 |
| ~~R3~~ | **CLOSED — VERIFIED-SOURCE-FACTS FACT 11.** FTS5 **is** compiled into `modernc.org/sqlite` v1.57.0, the exact version PocketBase v0.40.1 pulls transitively, and so are `MATCH` and the `rank` column (FTS3 and FTS4 are not, which is irrelevant — FTS5 supersedes both). The JSON functions are present too, despite `JSON` not appearing in `compile_options`, which in modern SQLite is expected and is not evidence of absence. | **Search MAY legitimately claim relevance ranking**; §1.2's `LIKE`-with-date-order hedge is withdrawn. **The capability is available and deliberately unused**: FR-073 declines relevance ranking, so 003 builds `LIKE` over an ordinary `search_index` collection ordered by date, no FTS5 virtual table and no `medigo reindex` command (§1.2; amendment 2026-08-27, ANALYSIS N12). The one thing that binds either way: deleting a record MUST delete its index row, or search leaks the titles of deleted records — Principle VII. | **CLOSED.** Owned by 003, which holds the search index and unified search and has recorded the mechanism choice in its Deviations table. |
| R4 | **PDF generation.** No library chosen. Pure-Go and cgo-free is mandatory (`maroto`, `gofpdf`, `unipdf` licensing). Headless Chrome is forbidden — it is Node-adjacent and would not fit distroless. | Reports are a phase 006 deliverable and PDF is one of three export formats. | 006 — decide in that phase's plan, not before |
| R5 | **Thumbnail generation for non-image attachments.** PB's `CreateThumb` handles images. PDFs, HEIC and TIFF need either a pure-Go decoder or a generic type icon. HEIC in particular has no cgo-free decoder. | Attachments are mostly lab PDFs and phone photos. | 004 — decide: generic icons for anything PB cannot thumb |
| ~~R6~~ | **CLOSED — VERIFIED-SOURCE-FACTS FACT 10.** Both are readable from Go at boot, **but from two DIFFERENT places, and a plan that assumes they sit together is wrong.** The IP allowlist is on global settings: `core/settings_model.go:125`, `SuperuserIPs []string` — warn on `len(app.Settings().SuperuserIPs) == 0`. MFA is on the **superusers auth collection**, not on settings: find the collection by `core.CollectionNameSuperusers` and read `collection.MFA.Enabled` (`MFAConfig` at `core/collection_model_auth_options.go:348`). | Constitution VII, and it gates the "admin UI ships in production" decision. Two things the boot warning must say rather than merely reporting "MFA off": PocketBase **refuses** to enable MFA unless the collection has at least two auth methods enabled (`validation_mfa_not_enough_auths`), so it is not a single toggle; and a **non-empty** `MFAConfig.Rule` is a partial rollout — some superuser can still sign in without a second factor — and MUST trigger the warning too. | **CLOSED.** Implemented by 001; the posture is surfaced by operation 81 and on `/admin`. |
| R7 | **The >5-minute SSE liveness test in CI.** PocketBase's hardcoded 5-minute `WriteTimeout` passes every test shorter than five minutes. A CI job that genuinely holds a stream open for >5 minutes is slow and awkward, but without it the fix regresses invisibly the first time someone refactors `newStream`. | The failure mode is silent and user-visible. | 001 writes the helper; 006 owns the CI job |
| R8 | **PocketBase upgrade fragility.** Three workarounds sit on unexported internals: the `pb.App` logger decorator, the `OnModelCreate("_logs")` interception, and the copied `DefaultDBConnect` pragma string for otelsql. All three must be re-verified on every PB upgrade. | A pragma drift silently loses WAL or foreign keys. | 001 creates the checklist; every phase re-runs it |
| R9 | **Export job durability.** MediGo is single-instance with an in-process worker. A restart mid-export leaves a `running` job. Needs a startup reconciliation that marks orphaned jobs `failed`. | Small, but it will look like a hang to a user. | 006 |
| R10 | **Email deliverability for invitations.** Invitations to an address with no account depend on PocketBase's SMTP settings being configured. An unconfigured self-hosted instance silently drops them. | Phase 005's headline feature. | 005 — surface SMTP state in `/api/v1/admin/system` and warn at boot |
| R11 | **The Playwright gate has never actually run a browser** in any research environment. An always-green gate is worse than no gate. | Principle VIII. | **001 exit criterion: prove the gate goes RED on a deliberately broken page.** |
| R12 | **`ETag`/`If-Match` on PATCH and DELETE** adds a required header to every clinical mutation. If the Datastar forms cannot carry it cleanly it becomes friction rather than safety. | §2.1 rule 10 is a design commitment. | 001 — prove it on medications, its single registered kind |

**The one risk that could have invalidated a phase was R1, and it is CLOSED.** Had a discriminated
`oneOf` proved ungeneratable or ungateable, the record route family would have collapsed back into
per-kind routes, the operation count would have gone from 90 to roughly 150, and phase 003 would
have grown from 3 routes to 62. It was built and run instead (FACT 9). **The record route family,
the operation budget in §2.3 and phase 003's three-route shape are settled, not assumed** — which
is why §2.3's arithmetic is worth trusting at all.

**Still open, and each still owned:** R2 (json/v2 retrofit, 001), R4 (PDF engine — narrowed, not
closed: constitution v1.3.0 admits `go-pdf/fpdf` behind a renderer interface in exactly one
package, and headless-browser PDF remains forbidden; 006), R5 (non-image thumbnails, 004), R7 (the
>5-minute SSE liveness job, 001 writes the helper and 006 owns the CI job), R8 (PocketBase upgrade
fragility — three workarounds on unexported internals, re-verified every phase), R9 (export job
durability, 006), R10 (SMTP deliverability for invitations — note that constitution v1.3.0 now
carves PocketBase's own `Settings()` store out of the caarlos0/env rule and *requires* MediGo to
surface an unconfigured SMTP state and warn at boot rather than fail quietly; 005 and 006),
R11 (prove the Playwright gate goes RED, 001), R12 (`ETag`/`If-Match` through Datastar forms, 001).
