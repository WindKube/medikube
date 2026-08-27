# Phase 0 Research: Patient Core

Every decision this phase makes, with the evidence behind it. Citations of the form
`core/record_model.go:1611` are line references into
`github.com/pocketbase/pocketbase@v0.40.1` as downloaded into the module cache and read directly.
Where a dossier and this document disagree, [`VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) wins, and it is cited by
name wherever it settles a question.

**No NEEDS CLARIFICATION survives this document.**

---

## A. Scope and sequencing

### D-01 — This phase's charter governs where it disagrees with the shared design contract's phase table

**Decision.** SHARED-DESIGN §0 allocates patients, multi-patient switching and the record-kind
registry to phase 001, and allocates practitioners/facilities/tags/catalogs to a phase 002 called
*reference-and-catalogs*. The actual specification set in `specs/` does not decompose that way:
`001-walking-skeleton` delivers accounts plus **medications** as the single clinical record type
owned directly by the account, and `002-patient-core` introduces patients, ownership, switching,
the chart summary, the re-attribution of medications, and the practitioner/facility directories.
**The specifications govern.** Everything else in SHARED-DESIGN — the collection field lists, the
API conventions, the package layout, the seams, the error taxonomy, the enum vocabularies — is
binding and is followed to the letter.

**Rationale.** `001-walking-skeleton/spec.md` already anticipated and settled this: *"Where that
contract's phase table allocates medications to a later phase, this phase's charter governs."* The
same sentence must hold in the other direction, or the two documents deadlock. The contract's own
preamble makes the constitution supreme and itself binding only "until it is amended"; a
specification that has been written, clarified and accepted is that amendment for scope purposes.

**Alternatives considered.**
- *Follow SHARED-DESIGN's table and treat this phase as reference-and-catalogs.* Rejected: it
  would leave `002-patient-core/spec.md`, an accepted specification with 56 functional
  requirements, unimplemented by any phase.
- *Amend SHARED-DESIGN.* Out of scope for a phase plan, and unnecessary — only the six-row phase
  table is affected, and this decision records the mapping.

**Consequences to carry forward.**
- Tags, `catalog_lab_tests` and `catalog_vaccines` belonged to no phase when D-25 was written.
  All three are now settled: `tags` went to phase 003, `catalog_lab_tests` to phase 004, and
  `catalog_vaccines` was **dropped from the suite entirely** (SHARED-DESIGN §1.3, amendment
  2026-08-27), which is why the headline collection count is 30 rather than 31. See D-25.
- The record-kind registry (`internal/records`) is assumed to exist from phase 001 with
  `medication` as its single registered kind, per SHARED-DESIGN §5.4. If it does not, task
  **T031** creates the minimal `Kinds()` surface this phase needs; the phase depends on the
  *interface*, not on the registry's internals.

### D-02 — Go 1.27, not 1.26.5

**Decision.** `go 1.27` plus an explicit `toolchain go1.27.x` in `go.mod`. CI must not set
`GOTOOLCHAIN=local`.

**Rationale.** VERIFIED-SOURCE-FACTS **FACT 0**: PocketBase v0.40.1's `go.mod` line 3 declares
`go 1.27`; 67 non-test files import `encoding/json/v2` (15 of them under `core/` and `apis/`);
`GOTOOLCHAIN=local go build` on 1.26.5 fails outright with `go.mod requires go >= 1.27`, while
go1.27.0 builds it clean. This is not a preference. The task brief's "Go 1.26.5" line is superseded
by the constitution's Technology Constraints, which record the same verification.

**Alternatives considered.** Pinning PocketBase to v0.39.x to stay on the house standard —
rejected in the constitution: it forfeits v0.40's filesystem hooks and backup fixes and invalidates
every file:line citation the plans rest on.

### D-03 — Tags stay out of this phase

**Decision.** No `tags` collection, no tag fields, no tag UI. Deferred to phase 003.

**Rationale.** `001-walking-skeleton/spec.md` says "a prescriber, a pharmacy and tags on a
medication … are reference data that phase 002 introduces". Two of those three appear in this
phase's requirements (FR-032 practitioners, FR-034 places of care); **tags appear in none of its 56
functional requirements, none of its user stories and none of its success criteria.** Principle I
is explicit: abstractions must not be introduced for requirements that are not in the current
phase's specification. Adding a tag collection now would be building to a sentence in a *different*
phase's assumptions section.

**Alternatives considered.** Adding `tags` anyway "since 001 promised it" — rejected as
speculative. The cost of adding it in 003 is one collection, one relation field per kind and one
page; the cost of adding it now is the same work plus a phase of carrying untested, unspecified
code.

### D-04 — `medications.practitioner` and `medications.pharmacy` DO land here

**Decision.** This phase adds both relation fields to the `medications` collection, exposes them
on the medication DTOs, and offers them in the medication form.

**Rationale.** Unlike tags, these are *required by this phase's requirements*: US5's independent
test is "attach the practitioner to a person as their primary practitioner **and to a medication as
its prescriber**", and FR-039 requires a practitioner or place of care to be selectable "wherever
one is needed". FR-040's deletion warning ("how many people and records reference it") is
meaningless without at least one record type referencing them.

---

## B. Ownership, and what PocketBase already implements

### D-05 — Ownership is `patients.owner`, a required cascading relation to `users`

**Decision.**
`patients.owner = RelationField{CollectionId: users, MaxSelect: 1, Required: true, CascadeDelete: true}`.
Set at create, never writable through any DTO in this phase.

**Rationale.** FR-002 (exactly one owning account, immutable) and the phase-001 edge case "closing
an account takes every person it owns with it". `CascadeDelete: true` makes account closure
destroy the patients, which then cascade to the medications (D-06), all inside one transaction —
FR-014 of phase 001 and this phase's "Closing an account" edge case, for free.

Immutability is enforced by DTO shape, not by a runtime check: `PatientCreate` sets `owner` from
the authenticated actor and `PatientPatch` has no `owner` field at all. SHARED-DESIGN §5.5 notes
this is the same technique that closed upstream's privilege-escalation class of bug twice.

**Alternatives considered.** A separate `patient_owners` join table for future multi-owner
support — rejected under Principle I/YAGNI; phase 005 widens *reachability* via `shares`, and never
widens *ownership* (spec Assumptions: "Ownership of a person cannot be transferred to another
account in this phase").

### D-06 — The `Required`/`CascadeDelete` matrix does most of the specification's work, and it is verified

**Decision.** The exact matrix below is what the migrations declare, and it is asserted by an
integration test against a real `tests.NewTestApp`.

| Relation | Required | CascadeDelete | Deleting the target does… | Requirement satisfied |
|---|---|---|---|---|
| `patients.owner → users` | yes | **yes** | deletes the patient | phase-001 account closure |
| `medications.patient → patients` | yes | **yes** | deletes the medication | FR-026, FR-049, SC-010 |
| `users.active_patient → patients` | no | no | **unsets the pointer** | FR-017, FR-052 |
| `patients.primary_practitioner → practitioners` | no | no | **unsets the reference** | FR-040 |
| `medications.practitioner → practitioners` | no | no | **unsets the reference** | FR-040 |
| `medications.pharmacy → facilities` | no | no | **unsets the reference** | FR-040 |
| `practitioners.facility → facilities` | no | no | **unsets the reference** | FR-040 |
| `practitioners.owner → users` | yes | **yes** | deletes the practitioner | FR-037 privacy on account closure |
| `facilities.owner → users` | yes | **yes** | deletes the facility | FR-037 |
| `audit_events.patient → patients` | no | no | **unsets the pointer** | FR-029: a deleted record's activity entry keeps no identifying detail |

**Rationale — read from source, not documentation.** `core/record_model.go:1587-1626`,
`deleteRefRecords`:

```go
// unset the record id
for i := len(ids) - 1; i >= 0; i-- { if ids[i] == mainRecord.Id { ids = append(ids[:i], ids[i+1:]...); break } }
// cascade delete the reference (only if there are no other active references)
if relField.CascadeDelete && len(ids) == 0 { return app.Delete(refRecord) }
if relField.Required && len(ids) == 0 { return fmt.Errorf("the record cannot be deleted because it is part of a required reference in record %s (%s collection)", ...) }
refRecord.Set(relField.Name, ids)
return app.SaveNoValidate(refRecord)
```

Three behaviours fall out of it, and all three are load-bearing here:

1. **Non-cascade, non-required → the id is unset and the referencing record is re-saved.** FR-040
   ("every one of those records survives with the reference cleared") needs *no MediGo code*.
2. **Non-cascade, required, no other ids → the delete FAILS with an error.** This is a trap: if
   `medications.patient` had `CascadeDelete: false`, deleting a patient with medications would
   return a 500 instead of cascading. The matrix above is therefore not decorative; the test that
   asserts it is a real regression gate.
3. **The unset path calls `SaveNoValidate`,** which still fires the model hooks. So clearing a
   practitioner reference on 40 medications writes 40 `update` audit rows. That is *correct* —
   those records did change — and FR-040's "you are warned how many records are affected" is
   exactly the number of rows it will write. It is documented so nobody later mistakes it for a
   bug. Note also the sweep runs in batches of 4000 (`core/record_model.go:1558`) inside the
   caller's transaction.

**Alternatives considered.** Implementing the unset ourselves in the service layer, for
explicitness — rejected under Principle V ("MediGo MUST NOT rebuild what PocketBase provides") and
because a hand-rolled sweep would be a second, slower, less correct copy of a function that already
batches.

### D-07 — Deleting a patient clears every account's "person in view" for free

**Decision.** `users.active_patient` is `MaxSelect: 1, Required: false, CascadeDelete: false`. No
MediGo code clears it on patient deletion.

**Rationale.** By D-06 case 1, deleting a patient unsets the id on every `users` row pointing at
it. FR-052 ("resolve the person in view to nobody when the person in view is the one deleted") and
FR-017 (both windows, "the other window's next action reports that the person no longer exists")
are then a *read-path* concern only: the page handler sees a null pointer and redirects to
`/patients` with an explanation. Two tests are mandatory: one asserting the field is
`CascadeDelete: false` (if someone flips it, deleting a patient would delete the **account**), and
one asserting the pointer is null after the delete.

### D-08 — The person in view is never an authorization input

**Decision.** `users.active_patient` is consulted in exactly three places, all of them
presentation:
1. rendering the switcher's current selection,
2. redirecting a bare `/medications` to `/medications?patient={active}`,
3. pre-filling the `patient` field on a create form.

Every `/api/v1` handler takes the patient from `?patient=` (lists), the request body (creates) or
the stored record (read/update/delete). A list request without `?patient=` is **400
`patient_required`**, never an implicit fallback.

**Rationale.** FR-015 and FR-016 say so, SHARED-DESIGN §6.6 says so, and §6.6 also records *why*:
running both mechanisms simultaneously is "the single biggest source of [upstream's] 500-route
sprawl", and a fallback turns a UI convenience into a permission grant. Acceptance scenario US3-5
is the direct test: changing the selection must never grant access to anything.

FR-025's "file the record against the person shown on the submitted form regardless of any
subsequent change to the person in view" is the same rule from the other end: the form carries a
hidden `patient` field rendered at page-render time, and the create handler uses it — it never
re-reads the pointer.

**Alternatives considered.** A server-side "current patient" in the session, resolved by middleware
into the request context — rejected: it is the same mistake with a shorter storage lifetime, and it
makes every handler's authorization implicit.

---

## C. Provisioning and re-attribution

### D-09 — `first_name`, `last_name` and `birth_date` are storage-optional and DTO-required

**Decision.** All three fields are `Required: false` on the `patients` collection.
`PatientCreate` requires all three; `PatientPatch` may set them but may not clear them to empty.
The **only** rows permitted to carry an empty value are server-provisioned self-records, and an
integration test asserts that directly:
`SELECT COUNT(*) FROM patients WHERE (birth_date='' OR first_name='') AND is_self_record=0` must be
0.

**Rationale.** FR-005 requires a self-record to exist for every account — the ones created from now
on and every account that already exists at migration time. Those accounts hold a display name and
nothing else. PocketBase's `Required` on a text field rejects the empty string, so a required
`birth_date` makes FR-005 literally unsatisfiable for pre-existing accounts.

This is a deliberate, recorded weakening of SHARED-DESIGN §6.2's third validation layer, and it is
carried in the plan's Complexity Tracking table as **CT-2**.

**Alternatives considered.**
- *Fabricate a placeholder date of birth* (e.g. 1900-01-01). Rejected on principle: writing a value
  nobody supplied into a medical record is worse than an honest null, and FR-006 derives a
  displayed age from it — a fabricated birth date becomes a fabricated age on a clinical screen.
- *Block registration until a date of birth is supplied.* Rejected: it rewrites phase 001's
  registration contract (FR-001 of that phase names email, display name and password), and it does
  nothing for accounts that already exist.
- *Create the self-record lazily, on first use.* Rejected: FR-005 says "MUST be created
  automatically when an account is created", and lazy creation races between two tabs, producing
  two self-records and violating FR-004.

**Compensating controls.** FR-030's empty-state rule applies: the chart header renders "Date of
birth not recorded" and an inline "complete this profile" action, never a blank, a zero or an
"unknown" that reads like a recorded value (spec Edge Cases: "must not present an absent blood type
as an unknown-but-recorded value").

### D-10 — Self-record provisioning splits the display name on the last space; it never invents data

**Decision.** When a self-record is provisioned (at registration and during the backfill):

```
name := strings.TrimSpace(user.name)
if i := strings.LastIndex(name, " "); i > 0 {
    first, last = name[:i], name[i+1:]
} else {
    first, last = name, ""            // no space: everything is the first name
}
is_self_record = true
relationship_to_owner = "self"
birth_date = ""                        // never fabricated
```

**Rationale.** It is the only transformation that is reversible by eye and never invents a
character. "Amara Okonkwo" → ("Amara", "Okonkwo"). "amara" → ("amara", ""). A single-token name is
common and must not become `("amara", "—")` or `("amara", "amara")`.

**Alternatives considered.** Splitting on the *first* space (wrong for "Maria del Carmen Ruiz");
leaving both empty and forcing completion (loses information the account already gave us).

### D-11 — `is_self_record` uniqueness is a partial index, and the service check is a second layer

**Decision.** `CREATE UNIQUE INDEX idx_patients_self ON patients (owner) WHERE is_self_record = 1`,
declared via `collection.AddIndex`. The service additionally refuses a create or patch that would
set `is_self_record = true` when one already exists, returning `409 conflict` with the message
FR-004 asks for.

**Rationale.** FR-004 and acceptance scenario US1-4 ("the attempt is refused with an explanation,
and the existing marking is unchanged"). PocketBase has no per-field `Unique`; uniqueness is always
a collection index (SHARED-DESIGN §1.0 rule 11), and SQLite supports partial indexes, which is
exactly the shape needed. The service-layer check exists because an index violation surfaces as an
opaque SQLite error, and FR-003/FR-027-style "tell them which field is wrong and why" needs a
typed `*ValidationError`. The index is the guarantee; the check is the message.

**Alternatives considered.** Service check only — rejected, it loses to a concurrent write.
Index only — rejected, the user gets a 500-shaped error.

### D-12 — The self-record cannot be deleted while its account exists, and that is a service rule

**Decision.** `patient.Service.Delete` returns `domain.ErrConflict` (→ 409) with the explanation
"removing this profile means closing your account" when the target has `is_self_record = true`.
There is no schema-level enforcement.

**Rationale.** FR-051 and US6-4. It cannot be a schema rule: account closure *must* delete the
self-record, and it does so via the `owner` cascade (D-06), which bypasses the service entirely.
Putting the rule in the service is therefore both necessary and sufficient — the only path that
deletes a self-record is the cascade, which is the path FR-051 wants to allow.

### D-13 — The re-attribution runs as raw SQL inside the single migration transaction

**Decision.** Migration `1756200600_medications_repoint.go`, in order:
1. add `medications.patient` as a **non-required, non-cascading** relation;
2. for every `users` row without a self-record patient, insert one per D-10 (via `app.Save`, so
   validation and hooks apply);
3. `UPDATE medications SET patient = (SELECT p.id FROM patients p WHERE p.owner = medications.owner AND p.is_self_record = 1)`
   as a single `app.DB().NewQuery(...).Execute()`;
4. assert `SELECT COUNT(*) FROM medications WHERE patient = '' OR patient IS NULL` = 0, returning
   an error (which rolls the whole thing back) if not;
5. flip `patient` to `Required: true, CascadeDelete: true`;
6. drop `medications.owner`.

**Rationale.** Step 3 is the Complexity Tracking entry **CT-1**. Doing it record-by-record through
`app.Save` would fire `OnRecordAfterUpdateSuccess` once per medication, writing a spurious
"system updated medication X" audit row for every row in the table — noise that would pollute
exactly the per-patient recent-activity list FR-029 introduces. It would also require the
repositories, which the DI container builds at boot, to be reachable from a migration that runs
*before* the container exists (`apis/serve.go:67` calls `app.RunAllMigrations()` before
`apis.NewRouter`).

Step 4 is what makes the whole thing safe: **`core/migrations_runner.go:129-131` wraps every
pending migration in one `AuxRunInTransaction(RunInTransaction(...))`**, so returning an error from
step 4 rolls back steps 1–3 *and every other migration in the batch*. There is no partially
migrated state to design a recovery for. That is FR-022 and SC-006 ("none is lost, duplicated, or
left without a person") enforced by construction rather than by hope.

Step 5 must follow step 3: `Required: true` on a column with empty values fails validation.

**Alternatives considered.**
- *Per-record `Save`.* Rejected on audit-noise and cost.
- *A global "hooks off" toggle for the duration.* Rejected — a hidden mode is precisely what
  Principle I forbids, and it would be reachable at runtime.
- *A separate `medigo migrate-medications` subcommand run by the operator.* Rejected: FR-022 says
  the attribution happens "when the change is applied", and an operator-run step is an operator
  who forgets.

**`down`.** Reversible in shape: re-add `owner`, backfill it from `patients.owner` via the same
kind of statement, drop `patient`. It is **lossy in substance** — reverting also drops the
`patients` collection, discarding any profile detail entered after the migration. Per Principle IX
that statement goes in the migration file itself.

### D-14 — Record hooks DO fire during migrations, and step 2 relies on it

**Decision.** The self-record inserts in step 2 use `app.Save`, and the audit hook registered on
`patients` writes an `actor_kind = system, action = create, target_kind = patient` row for each.
The audit hook must therefore tolerate the absence of a request context — and, because
`audit_events.request_id` is `Required`, it must fill that column from the **run id** the migration
context carries, minted by the same helper that mints request ids (001 [data-model](../001-walking-skeleton/data-model.md) §3, 001 T240). Every row of one backfill carries the same
`run_id`, and so do that run's log lines.

**Rationale.** Hooks are registered on the `PocketBase` value in `main` before `Start()`;
`apis.Serve` runs migrations at `apis/serve.go:67`, after that registration. So a `Save` inside a
migration goes through the full hook chain. This is desirable — FR-045 requires every person
creation to be audited, including these — but it means the hook cannot assume `e.Request` exists.
A unit test covers the nil-request path, and an integration test asserts the migration produces
exactly one `create/patient` audit row per provisioned self-record, each with a non-empty
`request_id` equal to that run's `run_id` (T074).

Second consequence, from VERIFIED-SOURCE-FACTS **FACT 1**: `createTxApp` shallow-copies a
`*BaseApp`, so inside a migration `app.Logger()` bypasses the zerolog decorator entirely. Since
`app.Logger()` is already banned by `forbidigo`, nothing changes — but the `OnModelCreate("_logs")`
interceptor is what actually catches PocketBase's own migration-time complaints, and it must stay
registered.

### D-15 — Migration order is fixed by the relation graph

**Decision.** `facilities` → `practitioners` → `patients` → `users.active_patient` →
`audit_events.patient` → `medications` repoint. Filenames are timestamp-prefixed in that order.

**Rationale.** A `RelationField` needs its target collection to exist. `practitioners.facility`
needs `facilities`; `patients.primary_practitioner` needs `practitioners`;
`users.active_patient`, `audit_events.patient` and `medications.patient` all need `patients`.
Since all six share one transaction (D-13), a mis-ordering fails the whole boot rather than
half-migrating — loud, which is what we want.

---

## D. The photograph

### D-16 — `Protected: true`, MediGo-owned route, eager thumbnails, no file token

**Decision.**
```go
&core.FileField{
    Name: "photo", MaxSelect: 1, Protected: true,
    MimeTypes: []string{"image/jpeg", "image/png", "image/webp"},
    MaxSize:   cfg.Files.PhotoMaxBytes,          // 15 MiB default
    Thumbs:    []string{"100x100t", "400x400f"},
}
```
Bytes are served only from `GET /api/v1/patients/{id}/photo?size=…` after
`access.Authorizer.Patient(...)` succeeds, streamed through `app.NewFilesystem()` →
`fsys.Serve(w, r, key, name)`. PocketBase's `/api/files/` route and its file-token mechanism are
never used.

**Rationale.** Constitution VII is unambiguous, and the reason is in the source: PocketBase's file
handler runs its authorization check **only inside `if fileField.Protected`** with no else branch,
so an unprotected field is served to any anonymous caller who knows the URL. Under the Principle V
lockdown the opposite is also true — a protected field 404s for everyone because `ViewRule` is
`nil` — so MediGo must own the route in either case. FR-044 forbids "any link that carries its own
credential", which is precisely what a file token is.

**Eager thumbnails, and the exact key layout.** Because MediGo bypasses PB's file route, PB's
*lazy* thumbnailer (`apis/file.go:179-181`, which creates the thumb on first request) never runs.
Thumbnails are therefore generated on upload, in a `TxInfo().OnComplete` callback so they happen
after the record commit but inside the request, using PB's own naming so that PB's cleanup still
finds them:

```
original : <collectionId>/<recordId>/<normalisedFilename>
thumbnail: <collectionId>/<recordId>/thumbs_<normalisedFilename>/<size>_<normalisedFilename>
```

verified at `apis/file.go:177` and `core/field_file.go:612` — the latter is
`fsys.DeletePrefix(record.BaseFilesPath() + "/thumbs_" + filename + "/")`, which is what makes
FR-008's "the previous one is no longer retrievable" true when a photo is replaced. Generating
thumbs anywhere else would orphan them on replace. `fsys.CreateThumb` is
`tools/filesystem/filesystem.go:612` and is wrapped in `routine.SafeWrap` against decoder panics.

**Alternatives considered.**
- *Serve through PB's route with a file token.* Rejected by constitution VII: a credential in a URL
  lands in logs, proxies and referrer headers.
- *Generate thumbs lazily in MediGo's own route.* Rejected: it makes the first list render slow and
  unpredictable, and SHARED-DESIGN §1.2 already mandates eager.
- *Store thumbs under a MediGo-specific prefix.* Rejected: PB's replace/delete cleanup would not
  find them and every replaced photo would leak its old thumbnails onto disk forever.

### D-17 — The MIME type is sniffed by PocketBase; PocketBase's rejection message is not shown to the user

**Decision.** Rely on `MimeTypes` on the field for detection. **Map the resulting PocketBase
validation error into MediGo's own envelope** with code `unsupported_media_type` and a fixed,
PHI-free message; never propagate PB's message.

**Rationale.** Detection is genuinely content-based: `core/field_file.go:298-303` calls
`validators.UploadedFileMimeType`, which at `core/validators/file.go:70-79` opens the reader and
calls `mimetype.DetectReader(f)`. The client's declared `Content-Type` and the filename are not
consulted. That is FR-008's "determining the file's true type from its content rather than its name
or its stated type", satisfied by PocketBase.

But `core/validators/file.go:63` builds the error as
`fmt.Sprintf("Failed to upload %q due to unsupported file type.", cutStr(v.OriginalName, 300))` —
**it embeds the original filename**, which constitution VII names explicitly as PHI. Echoing that
message to the client, into a log line, or into Sentry would be a leak. So the mapping is not
cosmetic; it is a privacy control, and it gets a test that asserts the response body and the
captured log stream contain no part of the uploaded filename.

### D-18 — The photo size limit is explicit, and configurable

**Decision.** `MEDIGO_FILES_PHOTO_MAX_BYTES`, default 15 MiB, applied to the field's `MaxSize` at
migration time and re-asserted at boot.

**Rationale.** `core/field_file.go:28` sets `DefaultFileFieldMaxSize = 5 << 20` — 5 MiB — which
`maxSize()` returns whenever `MaxSize <= 0`. A modern phone photograph exceeds that routinely, so
leaving it unset would reject ordinary uploads with FR-008's "over the size limit" path for the
wrong reason. SHARED-DESIGN §1.2 specifies 15 MiB.

**Note on the boot assertion.** The Protected-and-limits assertion walks every collection's fields
at boot and refuses to start if any `FileField` has `Protected: false` (constitution VII, "the
application MUST refuse to start"). This phase extends the existing assertion to cover `patients`.

### D-19 — Photo responses are private and uncacheable by shared caches

**Decision.** `Cache-Control: private, no-store`, `Vary: Cookie, Authorization`, an `ETag` derived
from the stored file key (which contains PocketBase's random filename suffix, so it changes on
every replacement), and `Content-Disposition: inline` with a generic filename — never the
uploaded one.

**Rationale.** FR-044 and constitution VII. The uploaded filename is PHI; `photo.jpg` is not.

---

## E. Derived values

### D-20 — Age is derived at render time and never stored

**Decision.** `person.AgeAt(birth, on time.Time) Age`, where `Age` carries years, months and days
and formats itself. A patient born today renders as "0 days" / "newborn", never "0".

**Rationale.** FR-006 says so ("MUST NOT store it, so that no chart can display an age that has
gone stale") and US4-4 tests it directly. The `Age` type rather than a bare `int` exists because
the acceptance scenario demands a meaningful rendering below one year, which an integer year count
cannot give. Table-driven tests cover: born today; born yesterday; 1 day before a birthday; on a
birthday; a 29 February birth date evaluated in a non-leap year; and an unset birth date (D-09)
which renders as "not recorded" and not as an age.

### D-21 — Measurements are stored in SI and converted at the edge, and the DTO carries both

**Decision.** `patients.height_cm` (30..272) and `patients.weight_kg` (0.5..450) are the canonical
values, always. Every response carries the canonical numbers **and** a `display` object computed by
the service from the actor's `unit_system`:

```json
"height_cm": 175, "weight_kg": 70.5,
"display": { "unit_system": "imperial", "height": "5 ft 9 in", "weight": "155 lb 7 oz" }
```

**Rationale.** FR-007: "accept and display height and weight in the account holder's chosen unit
system while recording the measurement in one canonical form, so that changing the display
preference never alters what was recorded" — and US4-3 tests exactly that. Returning both means the
templ component stays dumb (it prints a string) and a future API client can still compute. The
conversion is arithmetic, so it lives in the service, not in the view (Principle II: views render,
services decide).

**Where the code lives.** `internal/domain/person/measure.go`, not a general `internal/domain/units`
package. Height and weight on a patient are the only two consumers in this phase; phase 003's
vitals will be the second, and *that* is when extracting a shared package stops being speculative.
Principle I.

**Alternatives considered.** Storing whatever the user typed with a unit tag — rejected, it is
upstream's mistake (spec Assumptions: "Upstream stored imperial and converted for export, which
makes the recorded value depend on the recorder's preference"). Converting in the templ component —
rejected, it puts a decision in the view layer.

### D-22 — The chart summary is computed, never materialised

**Decision.** `GET /api/v1/patients/{id}/summary` returns header + per-kind counts + the last 10
activity entries, computed on every request. No counter column, no cache, no summary table.

Counts come from a consumer-declared 1-method port:
```go
// internal/service/patient
type RecordCounter interface {
    CountByKind(ctx context.Context, patientID string) (map[kind.Kind]int, error)
}
```
implemented once over `internal/records`' registry — one `SELECT COUNT(*) WHERE patient = ?` per
registered kind, each hitting that collection's `(patient)` index. One kind exists today; fifteen
will by phase 004.

Recent activity comes from a second 1-method port, `RecentActivityReader`, implemented by
`internal/service/audit` over `audit_events` with an index on `(patient, occurred_at)`.

**Rationale.** SC-004 requires the summary within 2 s for a patient with 50,000 records. An indexed
`COUNT(*)` on a 50,000-row SQLite table is sub-millisecond; fifteen of them plus one `LIMIT 10`
index scan is nowhere near the budget. A counter table would add a write to every record mutation
and a drift class of bug, to buy nothing measurable. Principle I settles it. If phase 006's
reporting later shows the counts are the bottleneck across *all* patients at once, that is phase
006's problem to solve, with data.

**Open/closed (Principle II).** Nothing in `internal/service/patient` switches on kind. Registering
eleven more kinds in phase 003 changes zero lines here — which is the whole point of routing the
counts through the registry rather than writing `count(medications) + count(conditions) + …`.

**FR-029's deletion rule is structural.** Activity entries come from `audit_events`, which by
construction hold "actor, action, target kind, opaque target id, patient id, timestamp" and never
content (SHARED-DESIGN §1.2). So "entries for records that have since been deleted carry no
identifying detail about them" is true because there was never any identifying detail to carry.
The renderer links to the target only when the target still resolves.

---

## F. The directories

### D-23 — Medical specialty is a fixed Go enum, not a collection, and needs no endpoint

**Decision.** `directory.Specialty` — a Go string type with a `Valid()` method and 42 values
including `other` — mirrored into a `core.SelectField{MaxSelect: 1}` on `practitioners.specialty`,
both generated from the same Go source of truth. The vocabulary is rendered directly into the
practitioner form's `<select>` by the templ component. **No `/api/v1/catalog/specialties`
endpoint.**

**Rationale.** FR-033 requires a fixed vocabulary with a catch-all that "MUST NOT be extended by
ordinary use", and the spec's Assumptions explain why upstream's version was broken: a *required*
FK into a create-only table with no read-by-id, no update and no delete
(`domain-clinical.md:984-985`) is not a reference model. SHARED-DESIGN §1.1 row 5 already made this
cut.

No endpoint, because the only consumer is a server-rendered form and the values are compiled into
the binary. Adding a route to serve a constant list would cost one operation, one OpenAPI path and
one gate entry for zero capability — Principle I. If a future phase needs the list over the wire it
is one route away.

**Alternatives considered.** A seeded read-only `specialties` collection with a `GET` endpoint —
rejected as the same cost with an added migration and the risk of the table drifting from the enum.

### D-24 — Practices, pharmacies, hospitals, labs and imaging centres are one `facilities` collection

**Decision.** One collection, `kind ∈ {practice, pharmacy, hospital, lab, imaging, other}`,
required. One list page filtered by kind, one form, one search.

**Rationale.** FR-034 asks for exactly this ("keep a directory of places of care **in one list**,
each classified as…"), and SHARED-DESIGN §1.1 row 3 records the upstream problem it fixes:
`practices` (17 fields, locations as embedded JSON) and `pharmacies` (19 fields, address flattened
into columns) were two modellings of the same six address concepts in one codebase.

**Branches are separate rows.** FR-035 and US5-3. No embedded `PracticeLocationSchema[]` array, and
consequently **no uniqueness constraint on facility name** — two "Boots, Camden" and "Boots,
Kentish Town" rows are correct and must both be offered.

### D-25 — Practitioner uniqueness is `(owner, LOWER(name), specialty)`, and the empty specialty must be a string, not NULL

**Decision.** `CREATE UNIQUE INDEX idx_practitioners_owner_name_specialty ON practitioners (owner, LOWER(name), specialty)`.
The `specialty` select field stores `''` when unset, never `NULL`.

**Rationale.** FR-038 and US5-4. The trap: **SQLite treats NULLs as distinct in a UNIQUE index**,
so if an unset specialty were `NULL`, two practitioners named "Dr Chen" with no specialty would both
insert and FR-038 would be silently unenforced. PocketBase's `SelectField` with `MaxSelect: 1`
stores the empty string for an unset value, so the index works as intended — but this is a property
worth a direct test (`two practitioners, same name, both with no specialty → the second is refused
409`) because it is invisible in the schema and would break if the field type ever changed.

`LOWER(name)` because "Dr Chen" and "dr chen" are the same person to a user. Facilities get no such
index (D-24).

### D-26 — The "how many things reference this?" warning is a field on the detail DTO, not an endpoint

**Decision.** `GET /api/v1/practitioners/{id}` and `GET /api/v1/facilities/{id}` return a `usage`
object:
```json
"usage": { "patients": 1, "records": 12 }
```
The delete confirmation dialog reads it from the detail response it already has.

**Rationale.** FR-040 requires the warning "before they delete a directory entry"; the UI already
loads the detail before offering delete. A `GET /practitioners/{id}/usage` endpoint would be a
second round trip and a second operation for data the first response can carry. Principle I.

The same reasoning applies to FR-048's delete-a-person confirmation, which states "how many records
will be destroyed": that number is already in the chart summary (D-22), which the
`/patients/{id}` page has loaded. **No `DELETE …/preview` endpoint exists in this phase.**

---

## G. HTTP surface

### D-27 — Optimistic concurrency extends from clinical records to patients and directory entries

**Decision.** `GET` on a patient, practitioner or facility returns an `ETag` derived from the
record's `updated` timestamp. `PATCH` and `DELETE` on those resources **require** `If-Match`; a
mismatch is `412 version_mismatch` carrying the current representation.

**Rationale.** FR-011 and US1-7 demand it for patients ("the save is refused, they are shown the
current values, and no change is silently overwritten"), and the spec's Edge Cases extend the same
rule to "the records attributed to a person". SHARED-DESIGN §2.1 rule 10 already mandates it for
clinical records; applying the identical mechanism to the three new resources means one helper in
`internal/web`, not four conventions.

**Datastar carries it fine** — this was flagged as open risk R12. The `PATCH` is issued by
`data-on:submit="@patch('/api/v1/patients/…', {headers: {'If-Match': $etag}})"`, with `$etag`
seeded into the form's `data-signals` at render time. No Pro attribute is involved.

### D-28 — Refusal is 404, is byte-identical to a genuine not-found, and is audited

**Decision.** Every authorization failure on patient-scoped data maps to `domain.ErrNotFound` →
`404` with the standard envelope. A test asserts the response for "someone else's patient id" and
for "an id that never existed" are byte-identical apart from `request_id`. The authorizer writes an
`audit_events` row on every refusal.

**Rationale.** FR-042, FR-045, SC-005 and SHARED-DESIGN §2.1 rule 13: for anything patient-scoped,
existence is itself PHI. `403` is reserved for cases where the caller already knows the resource
exists — which in this phase is exactly one case, FR-051's "you cannot delete your own profile",
answered `409 conflict` rather than 403 because it is an invariant, not a permission.

**Audit amplification, acknowledged.** Auditing refusals means an attacker enumerating ids writes
one small row per attempt. Mitigations already in place: PocketBase's rate-limit middleware is
bound by default on MediGo's routes (it is only unbound on the record-CRUD subtree —
VERIFIED-SOURCE-FACTS FACT 2), audit retention is a configured purge, and the row carries no
content. Accepted; recorded so it is not rediscovered as a surprise.

### D-29 — Cursor pagination, with explicit sort keys per new list

**Decision.** The shared `Page[T]` envelope and opaque HMAC-signed cursors from SHARED-DESIGN §6.3.
Sort keys:

| List | Default sort | Cursor tuple |
|---|---|---|
| `/api/v1/patients` | `last_name, first_name, id` | those three |
| `/api/v1/practitioners` | `name, id` | those two |
| `/api/v1/facilities` | `kind, name, id` | those three |

`limit` defaults to 25 and caps at 100.

**Rationale.** FR-053 and the Edge Case "Paging while data changes": a cursor encoding the last
row's sort values plus its id cannot duplicate or skip when a row is inserted mid-page, which an
offset can. The id tiebreaker is mandatory — without it, two patients with the same name (an
explicitly supported case, spec Edge Cases: twins, father and son) make the cursor ambiguous.

**One documented exception.** `GET /api/v1/patients` returns `total` unconditionally rather than
only under `?count=true`. FR-010 requires the list to "state how many there are", a household is
tens of rows, and SHARED-DESIGN §2.3 route 13 already specifies count fields on this endpoint.

### D-30 — Deleting a patient is one transaction, and its cost is understood

**Decision.** `DELETE /api/v1/patients/{id}` runs inside `app.RunInTransaction`, deleting the
patient and letting `medications.patient`'s cascade destroy the records and the photo go with the
record.

**Rationale.** FR-049, SC-010. The cost is stated rather than discovered: PocketBase's cascade
sweep loads referencing records in batches of 4000 (`core/record_model.go:1558`) and deletes them
one by one, all inside the caller's transaction. For the SC-004 design point of 50,000 records that
is a long single-writer transaction on an embedded SQLite. It is accepted for this phase because
deleting a person is rare, deliberate, confirmed, and correctness outranks latency here; the UI
shows a progress state and the request gets an extended server timeout. If it ever becomes a
problem the fix is a background job, and that is a phase-006 concern with data behind it.

### D-31 — The record stream becomes patient-scoped and re-authorises per event

**Decision.** `GET /api/v1/streams/records` gains a required `?patient=` parameter. The hub still
publishes `{Kind, RecordID, PatientID}` and never bodies; the per-subscriber handler filters on
patient, **re-runs `access.Authorizer.Patient` for that subscriber on every event**, then re-fetches,
renders and patches.

**Rationale.** Constitution V's realtime clause requires exactly this, and this phase is where
"re-authorise it for that subscriber" acquires a real permission to check. Phase 001's assumption
carried forward — "Lists this phase makes person-scoped continue to update live, for the person
they identify and for nobody else" — is a test: two browsers, two accounts, a write on one, nothing
on the other.

The 5-minute `WriteTimeout` override from phase 001 is untouched and re-verified by this phase's
regression test; it remains the trap that passes every test shorter than five minutes.

### D-32 — DTOs are written against `encoding/json/v2` semantics from the first line

**Decision.** Every new DTO gets a round-trip test asserting: slices marshal as `[]` and never
`null`; unknown request fields are rejected with `422`; duplicate keys are rejected; dates are
`YYYY-MM-DD` strings and instants RFC3339 UTC.

**Rationale.** SHARED-DESIGN open risk R2. Go 1.27's json v2 is not fully backward compatible
around nil-versus-empty slices, `json.RawMessage` and duplicate keys, and — the sharp edge —
`tests.ApiScenario` normalises bodies through `jsontext` before substring matching, so
`ExpectedContent` compares against *re-encoded* JSON. Writing the tests up front is cheaper than
debugging a substring match that fails for a reason that has nothing to do with the handler.

---

## H. UI

### D-33 — The patient switcher is a shell component, uses only free Datastar attributes, and needs no SSE

**Decision.** `@PatientSwitcher(nav)` renders inside `#primary-nav` — outside `#main` and therefore
outside every patch target, so it can never be morphed away. Selecting a patient issues
`data-on:click="@put('/api/v1/me/active-patient', {...})"`; the handler responds with **plain
`text/html`** (the re-rendered switcher), which Datastar treats as an element patch, and a `303`
to the current page's patient-scoped URL where a full re-render is wanted.

**Rationale.** SHARED-DESIGN §3.3: most interactions need no SSE at all, and reserving streams for
genuinely live views minimises exposure to the WriteTimeout trap. The v1 colon delimiter is
mandatory — `data-on-click` **silently does nothing**. `data-persist` is a Pro attribute and is not
used; the selection persists on the `users` record instead, which is better for a medical
application anyway (FR-013 requires it to survive a sign-out and follow the user to another device
— `localStorage` cannot do that).

Accessibility: `role="combobox"` with `aria-expanded`, name "Active patient", per SHARED-DESIGN
§3.0. The list shows name **and date of birth**, because the spec's Edge Cases require twins and
same-named father/son to be distinguishable at the point of choosing.

### D-34 — Every person-scoped page names its person, and that is asserted in the smoke gate

**Decision.** The `@PatientContextHeader(p)` component renders the current patient's name (and
photo thumbnail) inside `#main` on `/patients/{id}`, `/medications` and `/medications/{id}`. The
Playwright smoke assertion for those routes includes the seeded patient's name being visible.

**Rationale.** FR-019 and SC-003 ("100% of screens showing person-specific information name that
person on screen") is a claim, and Principle IX says a claim MediGo makes about itself is
machine-checked or it is not made.

### D-35 — Empty states are components inside the landmark, not instead of it

**Decision.** `/patients` (never empty in practice — the self-record guarantees one row),
`/practitioners`, `/facilities` and the chart's per-kind tiles each render `@EmptyState(...)`
**inside** their own `region[...]` landmark.

**Rationale.** FR-030, SC-013 ("on a freshly seeded installation where several of those screens are
empty") and SHARED-DESIGN §3.0's warning that rendering an empty page *instead of* the landmark is
"the most common way a smoke gate goes falsely red".

---

## I. Testing

### D-36 — Authorization tests are one table-driven suite that phase 005 extends by adding rows

**Decision.** `internal/testsupport/authz.go` exposes
`RunOwnershipMatrix(t, cases []Case)` where each case is `{name, method, path, body, actorFixture,
wantStatus}`. Every one of the 20 operations contributes a row for **owner → success** and a row for
**stranger → 404, body byte-identical to a genuine not-found**.

**Rationale.** Constitution III requires the third case ("a user who was granted access succeeds and
a user whose access was revoked is refused") *once sharing exists*. Sharing arrives in phase 005.
Writing the suite table-driven now means phase 005 adds two rows per operation to an existing table
instead of writing a parallel suite — which is the constitution's own "duplicated setup is
extracted into helpers" clause applied ahead of time, not a speculative abstraction.

### D-37 — Test apps are never shared, and the fixture is rebuilt as part of this phase

**Decision.** `internal/testdata/pb_data` is regenerated with the three new collections and the six
migrations applied. Every `tests.ApiScenario` gets its own `TestApp` via `TestAppFactory`.

**Rationale.** VERIFIED-SOURCE-FACTS FACT 7: `tests.NewTestApp` clones the fixture data dir into a
temp dir and bootstraps, so tests are isolated and `t.Parallel()`-safe — but sharing one `TestApp`
across `ApiScenario` cases causes `bindUIExtensions` to re-enter on every `OnServe`, growing the
handler chain until the stack overflows. This is a documented trap, not a style preference.

`tests.ApiScenario.ExpectedEvents` is used deliberately in two places this phase: to assert that a
`/api/v1/patients` write fires the audit hooks it should, and to assert that it fires **zero**
`OnRecordsListRequest` events — i.e. that MediGo's route did not accidentally go through
PocketBase's CRUD API.

---

## J. Risks carried into this phase

| Risk | Status entering phase 002 | This phase's action |
|---|---|---|
| **R2** json/v2 DTO retrofit | closed in 001 for medications; reopens for 12 new DTOs | D-32: round-trip test per DTO, mandatory |
| **R7** the >5-minute SSE liveness test | helper written in 001, CI job owned by 006 | D-31: re-run the helper against the now patient-scoped stream |
| **R8** PocketBase upgrade fragility | checklist created in 001 | Add three items: `deleteRefRecords` unset semantics (D-06), the `thumbs_<filename>/` key layout (D-16), the single-transaction migration runner (D-13). Each is a workaround resting on observed behaviour, and each has a test that fails loudly if v0.41 changes it. |
| **R11** the Playwright gate must go red | demonstrated in 001 | Re-demonstrate once on a new page, since 6 pages are added |
| **R12** ETag/If-Match through Datastar forms | proven in 001 on medications | D-27: extended to 3 more resources, same mechanism |
| **New** long delete transaction for a 50k-record patient | — | D-30: accepted, documented, measured in the phase's perf task |
| **New** audit amplification on enumerated refusals | — | D-28: accepted, mitigated by rate limiting and retention |
