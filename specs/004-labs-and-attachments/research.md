# Research: Labs and Attachments (phase 004)

**Phase 0 output.** Every technical decision this phase makes, as Decision / Rationale /
Alternatives considered. Evidence is cited to the dossiers; [`VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) (VSF)
overrides every other dossier wherever they disagree, and the constitution overrides all of them.

**No `NEEDS CLARIFICATION` item survives this document.**

---

## Index

| # | Decision |
|---|---|
| [D-01](#d-01) | `lab_result` is a record kind, not a bespoke resource |
| [D-02](#d-02) | `is_panel` is derived and server-owned |
| [D-03](#d-03) | Components are an id-stable replace-set inside the parent's payload |
| [D-04](#d-04) | Range classification: an explicit status always wins; never "normal" by default |
| [D-05](#d-05) | `canonical_name` normalisation rule, stated exactly |
| [D-06](#d-06) | The trend grouping key is `(catalog_test ?? canonical_name, unit)` |
| [D-07](#d-07) | Series statistics are pure domain code; the halves rule verbatim |
| [D-08](#d-08) | Series cap: 500 points, disclosed, summary over the returned window |
| [D-09](#d-09) | No unit conversion anywhere — enforced by `depguard`, not by discipline |
| [D-10](#d-10) | `sort_date`: a derived column, because FR-008 meets SC-011 |
| [D-11](#d-11) | The catalogue ships as a seeded migration and is read-only by absence |
| [D-12](#d-12) | Catalogue autocomplete: 3 characters, 300 ms debounce, an HTML fragment |
| [D-13](#d-13) | Attachment ownership is polymorphic; the cleanup hook is central |
| [D-14](#d-14) | MIME sniffing: stdlib plus a nine-entry magic table; CSV sniffs as `text/plain` |
| [D-15](#d-15) | The size limit is enforced three times, and before anything is stored |
| [D-16](#d-16) | Nothing uploaded can run: a compile-time inline set, a per-response CSP, `frame-src 'self'` |
| [D-17](#d-17) | Thumbnails: eager, four types only, `has_preview` stored, failure is never fatal |
| [D-18](#d-18) | Replace = create-new + trash-old, in one transaction, via `replaces` on the upload |
| [D-19](#d-19) | The trash, the purge cron, and how restore and purge stop racing |
| [D-20](#d-20) | Every content retrieval is audited, previews included — and what that costs |
| [D-21](#d-21) | Storage accounting is a `?usage=true` block plus one unlabelled gauge |
| [D-22](#d-22) | Lab results ride the existing stream; documents are not live-updated |
| [D-23](#d-23) | Lab results enter `search_index`; attachments and components do not |
| [D-24](#d-24) | The upload is a native multipart form post, not a Datastar signal |
| [D-25](#d-25) | `KindPageAction`: the fourth route class and its gate |
| [D-26](#d-26) | The trend chart is server-rendered inline SVG with no library |
| [D-27](#d-27) | Two hand-written parameterised SQL queries for the analytics |
| [D-28](#d-28) | Link ownership for US5: three fields here, two back-relations from phase 003 |
| [D-29](#d-29) | `lab_components` carries no `patient`; the two-level cascade is tested, not assumed |
| [D-30](#d-30) | `If-Match` on lab results; not on attachments |
| [D-31](#d-31) | `unit` is mandatory on a trend when more than one unit exists |
| [D-32](#d-32) | `quantitative`/`qualitative`/`textual` is the wire vocabulary for the spec's three value kinds |
| [D-33](#d-33) | PocketBase's 5-minute `WriteTimeout` bounds slow uploads too |
| [D-34](#d-34) | `return_to` on the upload form is validated against the route table |
| [D-35](#d-35) | Which shared-design open risks this phase closes |

---

## D-01

**Decision.** `lab_result` is registered as the fifteenth record kind through
`records.Register(kind.LabResult, …)`. It adds **zero** new record routes: it is served by
`GET|POST /api/v1/records/lab-results` and `GET|PATCH|DELETE /api/v1/records/lab-results/{id}`,
which phase 001 built and phase 003 filled with thirteen more kinds.

**Rationale.** FR-010 states the requirement directly: *"Lab results MUST behave like every other
clinical record kind already delivered: the same create, view, edit and delete flow, the same
confirmation naming the record before a permanent deletion, the same refusal when the record
changed underneath the editor, and the same live updating."* Registering the kind is how that
sentence becomes true structurally rather than by re-implementation. Registration also brings the
audit hook, the search-index hook, the realtime publish hook, the two pages, the two smoke cases
and the OpenAPI `oneOf` branch for free (SHARED-DESIGN §1.5, §2.2).

**Alternatives considered.** *A bespoke `/api/v1/lab-results` resource family*, as upstream had
(22 operations). Rejected: it duplicates six handlers, six sets of authorization tests, the ETag
handling and the delete-confirmation flow, and it guarantees that lab results drift from the other
fourteen kinds the first time one of those mechanisms changes. It is also 6 operations over
budget.

---

## D-02

**Decision.** `lab_results.is_panel` is stored, but it is **derived and server-owned**: the domain
sets it to `len(components) > 0` on every save. It appears in every response DTO and in **no**
request DTO. A client cannot set it, and there is no operation to flip it.

**Rationale.** FR-005 makes "an overall value *and* components" invalid and FR-006 requires
conversion in both directions after creation. A client-settable discriminator that can disagree
with the data it discriminates is a defect waiting to be found in production; making it a
projection removes the disagreement by construction. Upstream's version was create-only and could
not be corrected at all (domain-clinical §23), which is exactly the defect FR-006 exists to fix.

**Alternatives considered.** *(a) Drop the column and derive it at render time.* Then every list
row needs a component count, which is a per-row query or a join on the hottest list in the phase.
*(b) Keep it settable and validate agreement.* Two sources of truth plus a rule that says they must
agree is strictly worse than one source of truth. *(c) Make it a client-settable "mode" like
upstream's `treatments.mode`.* That is presentation state in the database, which the shared design
contract deletes on sight.

---

## D-03

**Decision.** A lab result's create and update payloads carry the **complete** component array.
The service diffs it inside `app.RunInTransaction`:

- an element with an `id` that exists on this result → update it in place;
- an element with no `id` → create it;
- a stored component whose id is absent from the payload → delete it.

`display_order` is assigned from the array index, so ordering is the order given (FR-016). An
element carrying an `id` that belongs to a *different* lab result is `422`, not a silent move.

**Rationale.** FR-014 is explicit: *"saving a result stores exactly the components submitted,
creating those that are new, updating those that changed, and deleting those that were omitted."*
Diffing by **id** rather than by name is what makes FR-016 (two components with the same test name
in one panel — a fasting and a random glucose) expressible at all, and it keeps component ids
stable, which matters because those ids are the opaque identifiers the audit trail and the trend
data points refer to.

**Alternatives considered.** *(a) Delete every component and re-insert the payload.* Simplest to
write, and wrong: every save churns every component id, so the audit trail records a delete and a
create for an unchanged line, and any URL or reference to a component breaks on each edit.
*(b) Component CRUD endpoints.* Five more operations, and it makes the "one submission, one
consistent set" semantics of US1 scenario 8 impossible to guarantee. *(c) Diff by
`(test_name, display_order)`.* Breaks the moment a row is reordered or renamed, and FR-016 allows
duplicate names.

---

## D-04

**Decision.** One pure function in `internal/domain/labs`:

```
Classify(value *float64, refLow, refHigh *float64, explicit ComponentStatus) Assessment
```

returning one of `not_assessed`, `below`, `within`, `above`, together with the status to display.
The rules, in order:

1. If `explicit` is set, it is the status shown, and the arithmetic never overrides it (FR-019).
2. Otherwise, if `value` is numeric **and** at least one of `refLow`/`refHigh` is numeric, classify
   against the bound(s) present (FR-018, and FR-017's "a range with only one bound is accepted and
   judged on the bound it has").
3. Otherwise the assessment is `not_assessed`, and the reading is **never** presented as normal
   (FR-020).

A range expressed only as `ref_text` ("negative", "not detected") yields `not_assessed`. A value
recorded as text ("<0.01") is a `textual` component and is never coerced to a number.

**Rationale.** This is the phase's most safety-relevant piece of arithmetic and it is three
requirements' worth of rules, so it lives in one function with a table-driven test rather than
being spread across a template and a query. Making it pure means every US1 scenario about marking
is a unit test with no database.

**Alternatives considered.** *(a) Compute in SQL so a list query can count out-of-range rows.*
Puts clinical judgement in a query string, duplicates rule 1 in two languages, and cannot express
"never normal by default". The out-of-range count (FR-022) is instead computed once when the
components are loaded, which the detail view does anyway. *(b) Store the assessment.* It is a
function of two stored fields; storing it adds a fourth derived column for no query that needs it.

---

## D-05

**Decision.** `Normalise(testName)` = Unicode NFKC → trim leading/trailing whitespace → collapse
internal whitespace runs to one space → Unicode simple case-fold (lowercase). Nothing else: no
punctuation stripping, no synonym expansion, no stemming. It is stored on the component as
`canonical_name` and is written by the domain on every save.

**Rationale.** FR-025 requires grouping "by their own test name ignoring letter case and
surrounding spaces", and US4 scenario 5 requires that two components entered as `"  Glucose "` and
`"glucose"` trend as one. Doing more than that — stripping punctuation, or matching "Hgb" to
"Haemoglobin" — is what the **catalogue** is for (FR-041), and doing it by string surgery would
silently merge two genuinely different tests, which is a clinical error. NFKC is included because a
full-width or composed character typed on a phone keyboard must not create a second series.

**Alternatives considered.** *(a) Lowercase only.* Fails on leading/trailing whitespace, which
US4 scenario 5 names. *(b) Aggressive normalisation (strip non-alphanumerics, collapse).* Merges
"Vitamin D 25-OH" and "Vitamin D-3" — a wrong answer that looks right. *(c) Do not store it;
normalise in the query.* Normalising 100,000 rows per keystroke-scale query, and no index is
possible.

---

## D-06

**Decision.** The identity of a series is
`(catalog_test_id if the component was matched to a catalogue entry, else canonical_name)` **plus
the `unit` string as recorded**. The rollup list (`GET /api/v1/lab-components`) groups by the first
half only and reports the distinct units it saw; the series (`GET /api/v1/lab-components/trend`)
requires both halves.

**Rationale.** FR-025 says readings group by the catalogue match where one exists and otherwise by
the normalised name — that is the first half, and it is what makes US4 scenario 4 (two spellings,
one catalogue entry, one series) true. FR-027 says readings in different units are never combined —
that is the second half. Splitting the two levels is what lets the rollup list a component once
(FR-024) while the series stays honest about units.

**Alternatives considered.** *(a) Group by `catalog_test` only, and treat uncatalogued components
as ungroupable.* Contradicts FR-042. *(b) Include unit in the rollup key too.* Then a test whose
lab changed units appears twice in the list of "every distinct component ever recorded", which
FR-024 says is once.

---

## D-07

**Decision.** `internal/domain/labs/series.go` computes, for a numeric series, exactly the eight
figures FR-030 names — count, earliest and latest reading dates, latest value, minimum, maximum,
mean, and the count of readings within their own recorded range — plus a direction for series of
three or more readings, by FR-031's published rule implemented verbatim:

> split the readings chronologically into an older half and a newer half, discarding the middle
> reading when the count is odd; **rising** if the newer mean exceeds the older mean by more than
> five per cent of the older mean, **falling** if it is below by more than five per cent,
> **steady** otherwise.

The rule text itself is a Go constant, rendered on the view where the direction is shown, so the
statement and the implementation cannot drift. Fewer than three readings yields no direction and
the explicit statement that there are not enough readings (FR-032). A categorical or textual series
yields a value history with per-value frequencies and **no** mean, range or direction (FR-033).

Two edge rules are settled here because the specification does not state them and arithmetic
demands an answer: an older mean of exactly zero makes the five-per-cent test undefined, so the
direction is `steady` unless the newer mean is also zero (also `steady`) — a division is never
performed; and readings that fall on the same date keep their insertion order, which is the parent
result's `sort_date` then its id, so the halves split is deterministic.

**Rationale.** The rule is published in the requirement precisely so that the number shown is
explicable, and Constitution VII's neighbourhood — a person reading a trend about their own health
— makes "explicable" non-negotiable. Pure code with no I/O means all of US3's arithmetic scenarios
are unit tests.

**Alternatives considered.** *(a) Linear regression slope.* More defensible statistically and
completely unexplainable in a sentence; the spec chose the halves rule and it is not this plan's
place to substitute. *(b) Compute in SQL.* Two implementations of a clinical statement, one of them
untestable without a database.

---

## D-08

**Decision.** `MEDIKUBE_LABS_MAX_SERIES_POINTS`, default **500**. When a series query would return
more, the **most recent** N readings are returned and the response carries `capped: true`,
`cap_limit`, and the `range_start`/`range_end` actually used. The summary is computed **over the
returned window only**, and the view states both facts.

**Rationale.** FR-034 forbids silently returning part of a series as though it were the whole, and
FR-029 requires the summary to be computed over the same range as the series shown. Those two
together mean a cap must change the reported range, not just the point list. 500 is the
specification's own scale target ("a component with five hundred readings").

**Alternatives considered.** *(a) No cap.* A four-year daily glucose log is ~1,500 points, which is
an unreadable chart and a slow page. *(b) Down-sample.* Averaging readings invents values that were
never measured — unacceptable in a medical record, and it would also break FR-035 (each reading is
judged against its own range). *(c) Cap the oldest instead of the newest.* Nobody opens a trend to
see 2021.

---

## D-09

**Decision.** **No value is converted between units anywhere in MediKube**, and this is enforced
structurally rather than by review: a `depguard` rule denies `internal/domain/clinical/units`
(phase 003's SI converter) to `internal/domain/labs/**`, `internal/service/lab*/**` and
`internal/store/lab*/**`. A conversion cannot be added to the lab path by accident, and adding one
on purpose requires editing `.golangci.yml` in the same commit, which is a visible act.

**Rationale.** FR-028 is absolute. The spec's assumptions section explains why: *"Converting them
silently is how a laboratory history becomes actively misleading, and no conversion table is
trustworthy across every test in the catalogue."* Upstream's own API documents the bug being worked
around (domain-clinical §24: the `?unit=` parameter exists so that "values recorded in different
units are not merged"). Constitution IX says a claim MediKube makes about itself is machine-checked
or it is not made; this is that check.

**Alternatives considered.** *(a) A conversion table for the common pairs (mg/dL ↔ mmol/L).* Each
analyte has a different molar mass, so one table is wrong per-test; a per-test table is a second
catalogue nobody has asked for. *(b) Convert only when the catalogue entry declares a default
unit.* Same problem, dressed up. *(c) Trust reviewers.* Principle IX exists because that does not
hold.

---

## D-10

**Decision.** `lab_results.sort_date` is a stored `date`, written by the domain on every save as
`COALESCE(resulted_on, collected_on, ordered_on, DATE(created))`. It is the sort key for
`GET /api/v1/records/lab-results` and it is indexed as `(patient, sort_date DESC, id DESC)`.

**Rationale.** FR-008 requires exactly that ordering *and* requires a result with no dates at all
to hold a defined position; SC-011 requires every page of a 5,000-row list within 2 seconds. An
`ORDER BY COALESCE(...)` cannot use a plain index. SQLite supports expression indexes, but whether
PocketBase's `AddIndex` path creates and uses one correctly is unverified in the dossiers, and a
performance gate is a poor place to discover that it does not. One writer, in the same `Save` as
the source fields, with a repository contract test that mutates `resulted_on` and re-reads
`sort_date`.

**Alternatives considered.** *(a) An expression index.* Might work; unverified, and the failure
mode is a silent table scan. Recorded as the fallback if the column ever becomes a burden.
*(b) Order in Go.* Fetch 5,000 rows to render 25. *(c) Require a date.* Contradicts FR-002, which
makes every date optional.

---

## D-11

**Decision.** `catalog_lab_tests` is created and populated by a **migration**, not by `medikube seed`.
The vendored LOINC-derived extract lives at `assets/catalog/lab-tests.json`, is embedded with
`embed.FS`, and the migration's `up` performs an idempotent upsert keyed on `loinc_code` so
re-running it on an instance that already holds data is safe. The collection is read-only to every
account holder **by absence**: no `POST`, `PATCH`, `PUT` or `DELETE` route exists under
`/api/v1/catalog`, all five PocketBase API rules are `nil`, and a gate test asserts that the route
registry contains no non-`GET` method under that prefix.

"The catalogue has not been loaded on this instance" is a **distinct, first-class state** from
"nothing matched": the list response carries `"loaded": false` when the collection is empty, and
the UI says so plainly while manual entry continues to work (FR-042, and the spec's environment-
failure edge case).

**Rationale.** FR-036 says the catalogue *ships with the instance*; a production instance that has
never run `medikube seed` must still have it, so it belongs in a migration. FR-037 makes it read-only
— and read-only enforced by "there is no route" is stronger than read-only enforced by a check
inside a route that exists. FR-043 makes it non-PHI, which is why it is the one resource in the
application whose list does not require `?patient=`.

**Alternatives considered.** *(a) Seed it in `medikube seed`.* Then production instances have an
empty catalogue and US4 silently does nothing. *(b) Ship it as a static JSON asset queried in Go.*
Loses `?q=`, `?category=` and pagination, and puts a linear scan of ~2,000 entries behind an
interactive box. *(c) Fetch it from LOINC at boot.* Constitution VII: MediKube makes no outbound
request the operator did not configure.

---

## D-12

**Decision.** Catalogue suggestions are offered once **three** characters have been typed (FR-039),
debounced at 300 ms (`data-on:input__debounce.300ms`), and are served as a **server-rendered HTML
listbox fragment** by the page-layer route `GET /lab-results/component-suggest?q=` — not by the
JSON API. The three-character minimum is a fixed rule in the domain, not a configuration knob. A
query that matches nothing renders "nothing matched that", which is a different string from "the
catalogue has not been loaded".

**Rationale.** Datastar's free attribute set contains no client-side templating; the only way to
render a list of suggestions is for the server to send HTML, and `/api/v1` is JSON-in/JSON-out by
convention rule 14. SC-014 allows one second from the third keystroke, which a `LIKE` over ~2,000
indexed rows meets with room to spare. Making the minimum a constant rather than a knob is
Principle I: nobody has asked to tune it, and a knob is a second thing to test.

The JSON operation `GET /api/v1/catalog/lab-tests` still exists and is still the public API — the
fragment route calls the same service. Two renderings, one decision-maker.

**Alternatives considered.** *(a) Return HTML from the API route under content negotiation.*
Breaks the API's contract and makes the OpenAPI document describe only half of what the route does.
*(b) Fetch JSON and render client-side.* Requires client templating, which Datastar's free set does
not have and which `unsafe-eval`-adjacent string templating would make worse.

---

## D-13

**Decision.** `attachments` is patient-anchored (`patient` relation, required, cascade delete) and
identifies its owning record with `owner_kind` (a select over the fifteen registered kinds) plus
`owner_id` (a 15-character opaque id). There is no foreign key. Three mechanisms keep it honest:

1. **The cleanup hook is bound by `records.Register` itself**, once, for every kind — so a record
   kind added in a future phase inherits document support and its cleanup without knowing documents
   exist (FR-049). `registry_completeness_test.go` fails the build if any registered kind lacks it.
2. **Every write validates the pair inside the transaction**: `owner_id` must resolve in
   `owner_kind`'s collection **and** must belong to the same `patient`. A record deleted while an
   upload is in flight makes the transaction fail cleanly with nothing stored (the spec's
   concurrency edge case).
3. **A nightly orphan sweep** runs in the same cron entry as the purge, reports a count as a
   Prometheus gauge, and moves orphans to the trash rather than deleting them.

**Rationale.** FR-049's "without that record's kind having been anticipated by this phase" is a
structural requirement. The trade-off is stated in SHARED-DESIGN §1.2 and re-argued in this plan's
Complexity Tracking.

**Alternatives considered.** Fifteen nullable relation columns; a join table per kind; attaching to
the patient with a free-text label. All three are argued and rejected in Complexity Tracking.

---

## D-14

**Decision.** The stored `mime` is **sniffed from the bytes**, never taken from the multipart part's
declared `Content-Type` and never inferred from the file name (FR-051). Sniffing is
`http.DetectContentType` over the first 512 bytes, plus a nine-entry magic-number table in
`internal/domain/files/mime.go` for the accepted types the standard library does not identify:

| Type | Signature |
|---|---|
| `image/webp` | `RIFF` at 0, `WEBP` at 8 |
| `image/heic` / `image/heif` | `ftypheic`, `ftypheix`, `ftypmif1`, `ftypmsf1` at offset 4 |
| `image/tiff` | `II\x2a\x00` or `MM\x00\x2a` at 0 |

The sniffed type is then compared against `MEDIKUBE_FILES_ALLOWED_MIME`; anything not on the list is
`415` naming the accepted types (FR-052).

**One consequence is stated rather than discovered: a `.csv` sniffs as `text/plain`.** There is no
byte signature that distinguishes comma-separated values from prose, so the default allowlist
contains `text/plain` (which covers both `.txt` and `.csv`) and **not** `text/csv`. A CSV uploads
successfully, is stored with `mime = text/plain`, and is listed with a text icon. That is the
correct behaviour under FR-051 — "the content decides what type it is" — and it is documented in
`quickstart.md` so an operator does not add `text/csv` to the allowlist and wonder why nothing
matches it.

Default accepted types: `application/pdf, image/jpeg, image/png, image/webp, image/heic,
image/heif, image/tiff, image/gif, text/plain`.

**Rationale.** FR-051 is unambiguous and FR-052's test ("this holds even when the file's name or
the type claimed by the browser says otherwise") is written to catch exactly the shortcut of
trusting the header. `http.DetectContentType` implements the WHATWG sniffing algorithm and covers
PDF, JPEG, PNG, GIF and text natively; the three it misses are covered by 40 lines of table. A
dependency for that is more surface than the problem.

**Alternatives considered.** *(a) `gabriel-vasile/mimetype`.* Excellent library, ~3,000 signatures,
and a new dependency for nine types in an application whose constitution says "prefer standard
library to a dependency". *(b) Trust the extension.* Named in the requirement as the wrong answer.
*(c) Reject `text/plain` so CSV can be distinguished.* Then plain text uploads fail, which FR-052's
default type list forbids.

---

## D-15

**Decision.** The per-document size limit (`MEDIKUBE_FILES_MAX_UPLOAD_BYTES`, default **32 MiB**) is
enforced at three layers, in this order:

1. **`http.MaxBytesReader`** wraps the request body before `ParseMultipartForm` is reached, so the
   read fails at the limit rather than after the whole body has been buffered or spilled to a
   temporary file. The error is mapped to `413 payload_too_large` naming the limit.
2. **A per-route body limit** on the upload route, replacing PocketBase's global
   `BodyLimit(DefaultMaxBodySize)` for that route only.
3. **`FileField.MaxSize`** in the collection definition, as the last line of defence.

Nothing is written before all three pass: the blob is written by the record `Save`, which happens
after validation. An empty file (`size == 0`) is `422 empty_file` (FR-054).

**Rationale.** FR-053 requires refusal "before storing any of it" and "MUST leave nothing partially
stored". Layer 1 is the one that actually delivers that, because `ParseMultipartForm` — which
`e.FindUploadedFiles` calls — spills to disk above its memory threshold; without `MaxBytesReader` a
1 GiB upload writes 1 GiB to `/tmp` before anyone checks. Layer 3 exists because Constitution VII's
validation rule says storage constraints are the last line of defence and never the only one.

**Alternatives considered.** *(a) Rely on `FileField.MaxSize` alone.* PocketBase validates on save,
which is after the body has been read and spilled. *(b) Check `Content-Length`.* Client-supplied
and absent on chunked bodies.

---

## D-16

**Decision.** Three independent mechanisms, because FR-058 says "regardless of which types the
operator has accepted":

1. **The default allowlist contains no active type** — no `text/html`, no `image/svg+xml`, no
   `application/xml`, no JavaScript.
2. **The set of types MediKube will serve `inline` is a compile-time constant** in
   `internal/domain/files/mime.go`: `application/pdf`, `image/jpeg`, `image/png`, `image/gif`,
   `image/webp`, `text/plain`. An operator can widen what is *accepted*; nobody can widen what is
   *inlined*. Anything else is offered as a download only (FR-057), and a request for
   `?disposition=inline` on such a type is answered with `attachment` disposition, not an error.
3. **Every attachment response carries its own hardened headers**, set by the handler:
   `X-Content-Type-Options: nosniff`, `Content-Type` from the stored sniffed mime,
   `Content-Disposition` with the original name (quoted by `fsys.Serve`, which v0.40 fixed for
   names with special characters), `Cache-Control: private, no-store`, and
   `Content-Security-Policy: default-src 'none'; img-src 'self'; style-src 'none'; script-src 'none'; object-src 'none'; frame-ancestors 'self'; sandbox` —
   **with one deliberate exception**: for `application/pdf` the `sandbox` token is omitted, because
   an unkeyworded `sandbox` disables the browsers' built-in PDF viewers and PDF is the commonest
   attachment there is. A PDF is inert without a script context, and `script-src 'none'` plus
   `object-src 'none'` remain in force.

**A consequence for the application's own CSP.** The inline viewer frames MediKube's own attachment
route, so the page CSP gains `frame-src 'self'`, and the attachment response's `frame-ancestors` is
`'self'` rather than the pages' `'none'`. Both are additive and narrow: no directive the
constitution names is weakened, and the framed response is locked down harder than any page in the
application.

**Rationale.** FR-058 is a "regardless of configuration" requirement, which means at least one
mechanism must be outside the operator's reach — that is mechanism 2. Mechanism 3 exists because
attachments are served from the application's own origin, so an active type that somehow got
through would be same-origin.

**Alternatives considered.** *(a) A separate origin for attachments.* The textbook answer, and
genuinely better — rejected because MediKube is single-instance with one `PublicURL`, so it means a
second certificate, a second cookie domain and a second CSP surface for one iframe. Recorded here
so the trade-off is visible rather than forgotten. *(b) Force `attachment` disposition for
everything.* Contradicts FR-057 and US2 scenario 6. *(c) Rely on the allowlist alone.* The
requirement explicitly anticipates an operator widening it.

---

## D-17

**Decision.** Previews are generated **eagerly**, in a `TxInfo().OnComplete` callback after a
successful attachment create, by `fsys.CreateThumb`. Two sizes: `160x160t` for listings and
`1024x1024f` for the inline image viewer. They are generated **only** for `image/jpeg`,
`image/png`, `image/gif` and `image/webp` — the types PocketBase's thumbnailer can decode without
cgo. **PDF, HEIC, TIFF, text and CSV get a type icon and no preview**, which is what the spec's own
assumptions already state. A generation failure is logged once, leaves `has_preview = false`, and
**never fails the upload**.

`attachments.has_preview` is a stored boolean written by that callback.

**Rationale.** Constitution VII requires eager thumbnails because MediKube bypasses `/api/files/`,
where PocketBase's lazy thumbnailer lives (pocketbase.md §9). `OnComplete` keeps image decoding out
of the write transaction. The type restriction closes shared-design risk **R5** with the
conservative answer: HEIC has no cgo-free decoder at all, TIFF's availability in PocketBase's
thumbnailer is unverified, and PDF rasterisation would need a renderer this project must not have.

`has_preview` is stored rather than inferred **because the Playwright gate asserts zero failed
network requests**: inferring "this is a JPEG so a thumbnail exists" produces a broken `<img>` and
a red gate the first time generation fails. FR-060 is satisfied regardless: the preview is served by
the same handler, through the same authorization call, with the same audit row as the original.

**Alternatives considered.** *(a) Lazy generation in the download route.* Re-implements the
thundering-herd machinery (singleflight plus a weighted semaphore) that PocketBase has and MediKube
would be bypassing. *(b) Attempt a thumbnail for every type and let it fail.* Same outcome, plus a
decode attempt per upload of a 32 MiB PDF. *(c) A PDF first-page preview.* Needs a renderer; the
spec explicitly does not promise it.

---

## D-18

**Decision.** Replacement is not a separate operation. `POST /api/v1/attachments` accepts an
optional `replaces` field naming an existing attachment id. When present, the service, in **one
transaction**: validates that the named attachment belongs to the same patient and the same owning
record; creates the new attachment carrying the old one's `description` and `category` unless the
request overrides them; and sets `deleted_at = now` on the old one. The replaced version is
therefore recoverable for the full retention window (FR-061).

"Keeping its place on the record" is interpreted as *the same record, the same description, the
same category*. **No ordering column is added**: FR-069 orders the library "by when they were
attached", and a replacement is a new attachment.

**Rationale.** FR-061 specifies replacement by its effect and the spec says so explicitly
(*"Replacing a document is specified by its effect, not by a mechanism"*). Create-then-trash gives
that effect with zero new machinery, no version columns, and no seventh operation — and it makes
"undo a mistaken replacement" identical to "restore a deleted document", which is one flow to build
and one to test.

**Alternatives considered.** *(a) `PUT /api/v1/attachments/{id}` multipart.* A seventh operation,
and it needs a hidden previous-version row anyway to satisfy "recoverable for the retention
window". *(b) Overwrite the blob in place.* Destroys the recoverable copy the requirement demands,
and breaks SC-004's byte-for-byte guarantee for anyone holding the old bytes. *(c) A `version`
column and a self-relation.* Document version history, which the spec explicitly excludes.

---

## D-19

**Decision.** `attachments.deleted_at` is the only soft-delete column in MediKube. It is set by
`DELETE /api/v1/attachments/{id}` (which requires a confirmation in the UI, FR-063), by the replace
flow (D-18), and by the record-deletion cleanup hook (FR-067). A trashed attachment is excluded
from every list unless `?deleted=true`, and its content remains retrievable by the owner until it
is purged (FR-065's "offered the chance to download a copy before it is purged").

**The purge** is one `app.Cron()` entry, `medikube_attachment_maintenance`, running daily. It:
hard-deletes every attachment whose `deleted_at` is older than `MEDIKUBE_RETENTION_TRASH_DAYS`
(default 30) — PocketBase removes the blob and the thumbnails with the record; runs the orphan
sweep from D-13; and refreshes the storage gauge from D-21. A failure is logged, counted, and
retried on the next run; each row is its own delete, so a partial run leaves rows wholly in the
trash and never half-deleted (the spec's environment-failure edge case). `medikube purge` runs the
same function once, from the CLI.

Each purged document writes one `delete`/`attachment` audit row, and so does each quarantined orphan
— moving a row to the trash is a delete — both `actor_kind = system` (FR-077). A cron has no HTTP request and
`audit_events.request_id` is `Required`, so both fill it from the job context's **run id** — one
value per run, minted by the same helper as a request id and carried on that run's log lines (001
[data-model](../001-walking-skeleton/data-model.md) §3). Without it every purge row fails
validation on the first nightly tick.

**Restore versus purge** cannot race, because `POST /api/v1/attachments/{id}/restore` runs inside
`RunInTransaction` and **re-reads `deleted_at` inside the transaction**:

- row gone → `404 not_found` ("this document has been permanently deleted");
- `deleted_at` older than the window → `409 conflict`, code `retention_expired`;
- the owning record no longer resolves → `409 conflict`, code `owner_record_missing`, and the
  response tells the caller the content is still retrievable until the purge (FR-065);
- otherwise → `deleted_at` cleared, `200`.

**Rationale.** Constitution VII permits soft delete for files only and requires the window to be
enforced by a scheduled purge. The spec makes the purge this phase's responsibility explicitly
(*"a retention window nothing enforces is not a retention window"*) even though the operator-facing
view of the trash belongs to phase 006.

**Alternatives considered.** *(a) Purge on read.* Leaves bytes on disk indefinitely on an instance
nobody opens, which fails FR-066's "without an operator having to ask". *(b) A `deleted_reason`
column.* Nothing filters on it. *(c) Move blobs to a trash prefix.* Upstream's design; the identity
of a trash item becomes a filesystem path and ownership is lost (domain-platform §8.5).

---

## D-20

**Decision.** Every **successful** response that returns document bytes — original content,
inline view, or a preview — writes exactly one `read_sensitive` audit row **when, and only when,
the resolved grant is something other than the reader's own ownership.** An owner reading their own
document writes no row; a superuser reading somebody else's writes one; from phase 005, a share
recipient writes one on every retrieval. The row carries actor, action, `target_kind = attachment`,
the attachment id, the patient id, the request id and the timestamp. Never the file name,
never the description, never the mime, never any bytes. Refused attempts write an `access_denied`
row with the same shape, unconditionally — a refusal was never an owner's own read.

**The ownership condition is phase 005's rule, adopted here rather than invented here.** It is
stated once in `specs/005-sharing-and-collaboration/contracts/widened-authorization.md` and
[D-25](../005-sharing-and-collaboration/research.md#d-25) governs it for records and documents
alike. The trail exists to answer who reached data they do not own. Recording every time somebody
opens their own lab report produces unusable noise and — worse — builds a precise timeline of when
a person read their own most sensitive results, which is itself an exposure under Principle VII. It
is the same asymmetry phase 006 FR-075 already applies to the trail itself: reading it is not
auditable, exporting it is.

**The cost is accepted, stated, and much smaller than it first looked.** A document library page
showing 25 previews produces 25 audit rows **only when the viewer is not the owner** — the ordinary
case, an account holder browsing their own library, produces none. The worst case is therefore a
share recipient or a superuser browsing somebody else's library: at the spec's 2,000-document
scale, ~2,000 rows of roughly 100 bytes, and that is exactly the traffic the trail exists to record.
The mitigations are the existing `MEDIKUBE_RETENTION_AUDIT_DAYS` (730) and the audit purge cron phase
001 already runs; no new mechanism is introduced.

**Rationale.** FR-060 says a preview is protected exactly as strictly as the document, and a
preview is a rendering of the content — so a preview is audited on exactly the same terms as the
original, and exempting previews would be a reading chosen for convenience. FR-076 and SC-006 then
supply the *terms*: a non-owner's retrieval, every time; an owner's own retrieval, never.

**Alternatives considered.** *(a) Audit originals only, treat previews as listing metadata.*
Defensible, and it fails SC-006's "100% … by somebody who is not the owner". Rejected on the
principle that in this application the conservative reading of a privacy requirement wins.
*(a2) Audit every retrieval including the owner's own* — the earlier reading of FR-076, and the one
this decision replaces. Rejected in reconciliation with phase 005: it answers no question anybody
asks, it doubles the table, and the timeline it builds of a person's own reading habits is itself
the disclosure Principle VII forbids. *(b) Coalesce rows per-request.* Each preview is its own HTTP request, so there is nothing to coalesce. *(c) Inline
previews as `data:` URIs in the listing HTML.* Removes the requests, requires `img-src data:` in
the CSP, and inflates every listing response by ~33% of the preview bytes.

---

## D-21

**Decision.** `GET /api/v1/attachments?patient=…&usage=true` returns an additional `usage` block:
`{documents, bytes, trashed_documents, trashed_bytes}` — trashed counted separately, as FR-071
requires. It costs one aggregate query and adds **no** operation. The instance-wide total is an
operator concern and belongs to phase 006's `GET /api/v1/admin/system`; this phase publishes it as
the Prometheus gauge `medikube_files_bytes_total`, refreshed by the maintenance cron, **with no
patient, user or kind label** — that would be unbounded cardinality and PHI in the monitoring
system (Constitution VI).

**Rationale.** FR-071 wants three numbers on the library page and one number for the operator. A
query parameter on a list the page already fetches is the smallest thing that delivers the first
three; a gauge is the right shape for the fourth.

**Alternatives considered.** *(a) `GET /api/v1/attachments/usage`.* A tenth operation for one
object. *(b) Always compute usage.* A `SUM` on every library page and every record's attachment
strip, most of which do not display it.

---

## D-22

**Decision.** Lab result lists are live-updated by the **existing** `/api/v1/streams/records`
stream, filtered by `kind=lab_result` — a consequence of D-01, requiring no new route and no new
hub work. The trend page subscribes to the same stream so that deleting a result removes its
readings from an open trend on the next update (the spec's concurrency edge case).

**Documents are not live-updated in this phase.** The library and the per-record attachment strip
re-render from the fragment route in response to the actions taken in them.

**Rationale.** FR-010 requires live updating for lab results because it requires them to behave like
every other record kind. **Nothing in the specification requires it for documents**, and the
realtime hub trades in `realtime.Event{Kind, RecordID, PatientID}` where `Kind` is a `kind.Kind` —
attachments are not a record kind, so adding them means widening a type that fifteen kinds depend
on, for a requirement nobody wrote. Principle I settles it.

**Alternatives considered.** *(a) Add `attachment` to `kind.Kind`.* Pollutes the enum that drives
the record route family, the OpenAPI `oneOf` and fifteen registrations. *(b) A second stream for
attachments.* One more SSE route, one more subscriber budget, one more five-minute-timeout
exposure, for no requirement.

---

## D-23

**Decision.** `lab_result` rows enter `search_index` automatically, because `records.Register`
binds the indexing hook — title is the test name, body is the interpretation and notes. **Lab
components are not indexed** (FR-009 narrows a lab result list by words in the *test name*, not by
component names) and **attachments are not indexed** (no requirement asks for cross-kind document
search; the library's own `?q=` covers name and description with a `LIKE`).

**Rationale.** Shared-design risk **R3** (FTS5 availability) was closed by phase 003 with `LIKE`
over `search_index` ordered by date, and nothing here needs to reopen it. Adding attachments to a
cross-kind index would copy file names and descriptions — both PHI — into a second store for a
capability the charter excludes.

**Alternatives considered.** *(a) Index components.* Multiplies index rows by ~20 per result and
makes a search for "glucose" return every panel containing a glucose line, which is a different
product decision than any requirement states. *(b) Index attachments.* Excluded by the charter.

---

## D-24

**Decision.** The document upload form is a plain
`<form method="post" enctype="multipart/form-data" action="/documents/upload">` with **no**
`data-on:submit` and **no** `data-bind` on the file input. The browser performs a real multipart
navigation; the page-layer action handler calls the same `attachment.Service.Upload` the API route
calls and answers `303 See Other` back to the originating page. `POST /api/v1/attachments` remains
the API operation, unchanged, for API clients.

**Rationale.** Datastar v1 binds a file input as a signal shaped `{name, contents, mime}[]` with
base64 contents (frontend.md, "Other v1.0 semantic changes that will bite"). At the 32 MiB limit
that is ~43 MiB of base64 held as a JavaScript string, posted as a JSON body — over the request
body limit before the file is read, three times the memory, and it makes SC-004's byte-for-byte
guarantee depend on a base64 round trip in two languages. A native form post also gives the browser
its own upload progress, which the spec's scale edge case asks for, without MediKube implementing a
progress mechanism.

**Alternatives considered.** *(a) Base64 through Datastar anyway.* The arithmetic above.
*(b) Chunked upload with a resumable protocol.* A whole subsystem for a self-hosted single-user
application; Principle I. *(c) Content-negotiate on `/api/v1/attachments` so the form can post to
it directly.* Puts redirect semantics and HTML into the JSON API — see D-25.

---

## D-25

**Decision.** `internal/httproute` gains a fourth route class, `KindPageAction`, alongside
`KindAPI`, `KindPage` and `KindExternal`. A `page_action` route is a page-layer route that is not a
navigable page: it renders an HTML fragment or performs a form action. It appears in
`medikube routes`, is **deliberately excluded** from `api/openapi.json`, has no ARIA landmark, and
**must declare the Playwright spec file that exercises it**. `e2e/routes.gate.spec.ts` fails the
build if that file does not exist or does not reference the route.

The four routes in this phase:

| Route | Purpose | Covering spec |
|---|---|---|
| `POST /documents/upload` | the native multipart form action (D-24) | `e2e/specs/documents.spec.ts` |
| `GET /documents/list` | library table fragment: filters, paging, post-action refresh | `e2e/specs/documents.spec.ts` |
| `GET /lab-results/component-suggest` | catalogue suggestion listbox (D-12) | `e2e/specs/labs.spec.ts` |
| `GET /labs/trends/series` | series + summary + chart fragment | `e2e/specs/trends.spec.ts` |

**Rationale.** Principle IX requires the route inventory, the OpenAPI document and the Playwright
coverage to agree. Untracked handlers break that agreement quietly. Declaring the class is what
turns "these four are not in OpenAPI" from an omission into a checked statement.

**Alternatives considered.** All three are argued in Complexity Tracking: content negotiation
inside `/api/v1`, magic query parameters on the page routes, and skipping the declaration
altogether.

---

## D-26

**Decision.** The trend chart is an **inline `<svg>` rendered by templ**, from a pure Go scale
function in `internal/domain/labs`. No charting library, no canvas, no client-side plotting. It
carries `role="img"` and an accessible name naming the component and the unit; the reference-range
band is drawn from the range recorded with the reading the view names (FR-035); out-of-range points
are marked by **shape as well as colour** (a filled diamond versus an open circle) and every
reading also appears in an adjacent table with a text marker, so the distinction survives
monochrome, colour-blindness and a screen reader (FR-021, SC-002).

**Rationale.** The CSP forbids external origins and the runtime image forbids Node, so a charting
library would have to be vendored and embedded — for a single line chart with a band, which is
about 150 lines of templ plus a linear scale. US3 scenario 10 and FR-035 require a band, which
makes this a chart and not a sparkline. Phase 006 embeds this same component in reports rather than
building a second one (recorded as a deviation in `plan.md`).

**Alternatives considered.** *(a) Defer the chart to phase 006 and show only a table.* Fails US3
scenario 3 ("readings are shown in date order… plotted") and scenario 10 outright. *(b) Vendor a
JS charting library.* A build-time asset, a CSP surface, and a second rendering path for data the
server already has. *(c) Server-rendered PNG.* Needs an image encoder in the request path and is
not accessible.

---

## D-27

**Decision.** Two SQL statements in `internal/store/labtrend/sql.go`, executed through `app.DB()`:

- **the rollup** — for one patient, every distinct series identity (D-06 first half) with its
  reading count, its distinct units, and the value/unit/status/date of its most recent reading,
  via a window function over a join from `lab_components` to `lab_results`;
- **the series** — for one patient, one series identity and one unit, every reading with its
  value, unit, recorded reference range, status and its parent's `sort_date`, ordered by
  `(sort_date, lab_result.id, display_order)`.

Both are **fully parameterised** with `dbx` named bindings. **No string interpolation of `q`,
`canonical_name`, `unit`, `from` or `to`, ever.** The `patient` value comes from the
`access.Grant` returned by the authorizer, never from the raw request. Both are covered by
integration tests against a real test app and by an injection test that pushes `%'; DROP TABLE`
shaped input through every text parameter.

**Rationale.** Argued in full in Complexity Tracking: through the record API both queries are N+1
across dozens of distinct component names, which fails SC-003 at the phase's own scale target.
Constitution II is satisfied because these live in the store layer behind the consumer-declared
`labtrend.Reader` interface — the service and the domain never see SQL.

**Alternatives considered.** The record API plus in-Go grouping; a denormalised rollup collection;
storing the trend. All three in Complexity Tracking.

---

## D-28

**Decision.** US5's five link targets are split by which side owns the relation field, following
the shared design contract exactly:

- **`lab_results` owns** `conditions`, `medications`, `procedures` (multi-relations, edited by
  `PATCH /api/v1/records/lab-results/{id}`).
- **`encounters` and `treatments` own** their `lab_results` multi-relation field, created in phase
  003. The lab result's DTO exposes `encounters` and `treatments` as **read-only back-relation
  arrays**, and the lab result page's link editor patches the *other* record for those two.

Every link write validates, server-side, that both records belong to the same patient, refusing
with a response that discloses nothing about the other record's existence (FR-045). Removing a link
leaves both records otherwise unchanged (FR-047); deleting either record removes the reference by
PocketBase's relation cleanup, which is tested rather than assumed (FR-047, US5 scenarios 5 and 6).
A link carries no payload of its own (FR-048).

**Rationale.** FR-046 requires visibility from both ends, which back-relation traversal gives
without a second column. Duplicating the edge as fields on both sides is the upstream bug: two
sources of truth for one statement, which drift the first time one is written without the other.

**Alternatives considered.** *(a) Mirror fields on both sides.* Two sources of truth.
*(b) A link collection.* Fifteen link tables was upstream's answer and the shared design contract
deletes twelve of them for exactly this reason. *(c) Move `lab_results` ownership onto the lab
result for all five.* Would require editing two phase-003 collections and re-testing their link
editors, for a purely cosmetic gain in the DTO.

---

## D-29

**Decision.** `lab_components` carries **no** `patient` column. It is reached through
`lab_result`, whose `patient` relation is the authorization anchor. Deleting a patient cascades to
lab results, which cascades to components. That transitivity is **tested**, not assumed: an
integration test deletes a patient holding results with components and asserts zero component rows
survive (FR-015, SC-013).

**Rationale.** SHARED-DESIGN §1.5 specifies the shape. Denormalising `patient` onto components
would create a second authorization anchor that can disagree with the first, which is precisely the
class of bug Constitution VII's "authorized against the authenticated user at the point of access"
clause exists to prevent. The join to the parent is one index away and is needed for the date
anyway.

**Alternatives considered.** *(a) Denormalise `patient` for simpler queries.* A second anchor.
*(b) Rely on PocketBase's cascade without testing it.* SC-013 explicitly requires verification "by
looking for the data afterwards rather than by assumption".

---

## D-30

**Decision.** `PATCH` and `DELETE` on `/api/v1/records/lab-results/{id}` **require** `If-Match`;
a stale or absent header is `412`/`428` exactly as for every other record kind, and because the
component set travels inside the lab result's payload, a stale component set is refused by the same
check (US1 scenario 11, the spec's concurrency edge case, and "a component set is never merged").

`PATCH` and `DELETE` on `/api/v1/attachments/{id}` do **not** require `If-Match`. An attachment is
not a clinical record; the mutable fields are a description and a category; and phase 003 already
narrowed the `If-Match` rule to records.

**Rationale.** FR-010 inherits the concurrency rule for lab results. Nothing in the specification
asks for it on documents, and requiring a header on the description edit would be friction the
spec did not request.

**Alternatives considered.** *(a) `If-Match` everywhere.* Friction without a requirement.
*(b) `If-Match` on nothing.* Contradicts FR-010.

---

## D-31

**Decision.** `GET /api/v1/lab-components/trend` requires `unit` **when the series identity has
readings in more than one unit**. Omitting it in that case is `400`, code `unit_required`, with the
available units and their reading counts in the error's `fields[]` so the client can offer the
choice. When only one unit exists, `unit` may be omitted. The response always states the `unit`
being shown, along with `units[]` and `multi_unit`.

The UI never triggers that error in normal use: the rollup list already carries `units[]` per
component, so the link to a trend always names a unit.

**Rationale.** SHARED-DESIGN §2.3 op 56 states the requirement ("`unit` scoping is mandatory when
more than one unit exists"), FR-027 requires the account holder to be told and to be able to
choose, and FR-028 forbids conversion. A strict API plus a UI that pre-selects satisfies all three
without ever silently choosing on the user's behalf.

**Alternatives considered.** *(a) Default to the most recent reading's unit.* Friendlier, and it
makes a silent choice about a clinical value — and it contradicts the binding shared contract.
*(b) Return every unit's series in one response.* The response shape then varies with the data,
which OpenAPI describes badly and clients handle worse.

---

## D-32

**Decision.** The wire vocabulary for a component's value kind is
`quantitative | qualitative | textual`, default `quantitative`, exactly as
SHARED-DESIGN §1.4 specifies. It maps to the specification's prose as: numeric → `quantitative`,
categorical → `qualitative`, free text → `textual`. The mapping is stated in `data-model.md` and in
the contract, and the UI labels are the spec's words ("numeric", "categorical", "text").

FR-013's cross-field rule is enforced in the domain: a `quantitative` component carries `value` and
no `value_text`; a `qualitative` or `textual` component carries `value_text` and no `value`. Any
other combination is `422 value_kind_mismatch`, naming both fields.

**Rationale.** The shared contract's vocabulary is binding and is the one already used in
`internal/domain/clinical`'s enum family. Upstream had three parallel value columns with no
cross-field validation (domain-clinical §24); this is the fix.

**Alternatives considered.** *(a) Rename the enum to `numeric|categorical|text`.* Diverges from the
binding contract and from the vocabulary phase 002 established, for a cosmetic gain. *(b) One
`value` column typed as text with a parse.* Loses numeric sorting, numeric statistics and the range
comparison, all of which US3 needs.

---

## D-33

**Decision.** PocketBase's hardcoded 5-minute `WriteTimeout` bounds **slow uploads as well as SSE
streams**, because Go's `http.Server.WriteTimeout` clock starts when the request headers are read
and covers reading the body. Phase 001's `ServeEvent` override of `se.Server.WriteTimeout` is
therefore what makes a 32 MiB upload on a slow connection survivable, and this phase adds a
regression assertion that the override is still in place and covers the upload route.

**Rationale.** The trap is documented for SSE and its upload consequence is not obvious; a
refactor that scoped the override to stream routes only would break uploads silently for exactly
the users with the worst connections. The spec's scale edge case names it: "A document at the size
limit uploaded over a slow connection: progress is visible and the upload is not abandoned by a
timeout that the account holder cannot see."

**Alternatives considered.** *(a) A per-route timeout.* PocketBase's router exposes no per-route
server timeout; the `*http.Server` is one object. *(b) Assume the phase-001 override covers it.*
Assumption is what this project's gates exist to replace.

---

## D-34

**Decision.** The upload form carries a `return_to` field so the `303` lands back on the page the
user was on (a record detail page, or the library). Its value is **validated against the registered
page-route table** before it is used: it must match a registered `KindPage` pattern for this
application. Anything else is ignored and the redirect goes to `/documents`.

**Rationale.** An unvalidated redirect target taken from a form field is an open redirect, and an
open redirect in an application that holds medical records is a phishing primitive. Validating
against the route table rather than against a regex means the check cannot drift from the routes
that actually exist.

**Alternatives considered.** *(a) Always redirect to `/documents`.* Loses the user's place after
attaching a document to a record, which is the primary flow in US2. *(b) Use the `Referer` header.*
Client-supplied, often stripped, and no easier to validate.

---

## D-35

**Decision.** Two of the shared design contract's open risks are closed here, and one is confirmed
as already closed elsewhere:

| Risk | Status after this phase |
|---|---|
| **R5 — thumbnails for non-image attachments** | **Closed.** D-17: four decodable image types get previews; everything else, PDF included, gets a type icon. No HEIC decoder exists without cgo; no PDF rasteriser is admitted. |
| **R3 — FTS5 in the vendored `modernc.org/sqlite`** | **Not this phase's risk.** Phase 003 closed it with `LIKE` over `search_index` ordered by date, and D-23 adds nothing that would reopen it. |
| **R8 — PocketBase upgrade fragility** | **One item added to the checklist**: `fsys.CreateThumb`'s supported decoder set and `fsys.Serve`'s `Content-Disposition` quoting must be re-verified on every PocketBase upgrade, because D-17 and the non-Latin-filename requirement both depend on them. |
| **R7 — the >5-minute SSE liveness test** | **Widened.** D-33 makes the same override load-bearing for uploads, so the phase-001 assertion is extended to cover the upload route. |

**Rationale.** A risk register that is never closed is a list, not a control.
