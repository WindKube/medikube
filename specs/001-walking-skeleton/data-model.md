# Phase 1 Data Model: Walking Skeleton

Consistent with SHARED-DESIGN §1.0–§1.2 and with [research.md](./research.md). Every rule in
§1.0 holds without exception: 15-character opaque PocketBase text ids; `id`, `created` and
`updated` on every collection and not repeated below; **every one of the five API rules `nil` on
every non-system collection**; every enum is both a `core.SelectField{MaxSelect: 1}` and a Go
string type with `Valid()`, generated from one source of truth; all enum values `snake_case`;
**no `deleted_at` anywhere**; uniqueness is always a collection index, because PocketBase has no
per-field `Unique`.

This is the first phase, so this document defines the vocabulary the whole application uses.
Where a later phase extends something declared here, that is noted so the extension is visibly
additive rather than a rewrite.

## 0. What this phase creates, at a glance

| Collection | Change | Migration |
|---|---|---|
| `users` | **amended** — PocketBase creates it; this phase adds seven fields, nils its five rules, adds the case-insensitive email index, and configures the token and password policy | `1756100100_users_profile.go` |
| `medications` | **new** | `1756100200_medications.go` |
| `audit_events` | **new** | `1756100300_audit_events.go` |

Running total of collections after this phase: **3**. Phase 002 adds `facilities`,
`practitioners` and `patients`, amends all three of these, and takes the total to 6.

**Zero `FileField`s exist in this phase.** The `Protected: true` boot assertion is written
anyway, because phase 002 adds `patients.photo` and an assertion that exists from the start is a
gate; an assertion added alongside the first file field is a retrofit that has to be trusted.

---

## 1. `users` — amended

PocketBase's initial system migration creates `users` as a `core.CollectionTypeAuth` collection
and owns `id`, `email`, `emailVisibility`, `verified`, `password`, `tokenKey`, `created` and
`updated`. **`verified` is PocketBase's, and this phase uses it**: it is set by
`confirmEmailVerification` (FR-075), never by a request DTO, and read back as
`Me.EmailConfirmed`. This phase adds MediGo's fields and, critically, **replaces PocketBase's default API
rules with `nil`** — the stock collection ships with rules like `id = @request.auth.id`, which
would leave `GET /api/collections/users/records` open to every authenticated caller.

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `name` | text | **yes** | 1..120 | display name. **PHI-adjacent** — redacted in log marshalling. FR-001, FR-011 |
| `role` | select | **yes** | `user` \| `admin`, default `user` | **Absent from every request DTO.** A MediGo application tier, *not* a PocketBase superuser. FR-012 |
| `unit_system` | select | **yes** | `metric` \| `imperial`, default `metric` | FR-011. Load-bearing from phase 002, where it drives the display block |
| `locale` | text | **yes** | ≤10, default `en` | FR-011. English is the only shipped text in this phase; the value governs date and number presentation only |
| `date_format` | select | **yes** | `iso` \| `dmy` \| `mdy`, default `iso` | FR-011 |
| `theme` | select | **yes** | `system` \| `light` \| `dark`, default `system` | FR-011, FR-045. Rendered as a class on `<html>` by the server — never read from `localStorage` (research D-36) |
| `disabled_at` | date | no | | non-null ⇒ sign-in refused. **Absent from every request DTO.** FR-012 |

**Not carried over**, and named so nobody adds them speculatively: `active_patient` (phase 002),
`must_change_password` (added by phase 006), `auth_method`, `external_id`, `sso_provider`, `sso_metadata`,
`sso_linking_preference`, `account_linked_at`, `last_sso_login`, the twelve upstream
Paperless/Papra preference fields, and the whole `user_preferences` 1:1 table. PocketBase's
`_externalAuths` and its OAuth2 linking flow replace the SSO group entirely if a later phase wants
it.

### Collection configuration set by this migration

| Setting | Value | Why |
|---|---|---|
| `ListRule`, `ViewRule`, `CreateRule`, `UpdateRule`, `DeleteRule` | **`nil`** | The lockdown. Superuser-only. Asserted at boot. |
| `AuthRule` | **`types.Pointer("")`** | **Not one of the five.** `nil` here disables authentication entirely — password, OAuth2, OTP, all of it. `""` means any record may authenticate. FR-005 depends on this. |
| `ManageRule` | `nil` | Nobody manages another account's auth record through the API. |
| `PasswordAuth.Enabled` | `true`; `IdentityFields` = `["email"]` | FR-005 |
| `PasswordAuth.MinPasswordLength` | `8` | FR-004's published floor, enforced at the storage layer as well as in the domain |
| `AuthToken.Duration` | `MEDIGO_AUTH_SESSION_TTL`, default **7 days** | FR-008, and the specification's Assumptions |
| `OAuth2.Enabled` | `false` | **Phase 006 owns external sign-in** (contract operation 4). Nothing in this phase turns it on. |
| `PasswordResetToken.Duration` | PocketBase's default, **1800 s (30 min)** | FR-074's "expires after a documented period". Left at the default deliberately: short enough to limit a leaked link, long enough for somebody reading mail on another device. |
| `VerificationToken.Duration` | PocketBase's default, **86400 s (24 h)** | FR-075. Same reasoning, longer window — confirming an address is not a credential reset. |
| `MFA.Enabled` | `false` on `users` | Ordinary accounts do not use MFA in this phase. **Superusers do**, and the boot warning checks the `_superusers` collection, not this one (research D-32) |
| `VerificationTemplate`, `ResetPasswordTemplate` | **left as PocketBase's defaults**, with `Meta.SenderName` / `Meta.SenderAddress` supplied by the operator through the settings store | FR-074, FR-075. MediGo renders no email template of its own: a second template system for two messages is exactly what Principle V forbids. |

### Indexes

```sql
CREATE UNIQUE INDEX idx_users_email_lower ON users (LOWER(email));   -- FR-003
```

FR-003 requires addresses differing only in letter case to be treated as the same address, and
requires that no partially created account survive a failure. The `LOWER(email)` unique index
gives the first for free and, because registration runs inside `app.RunInTransaction`, the second
follows: the losing side of a simultaneous double registration gets a constraint violation, the
transaction rolls back, and exactly one account exists (spec Edge Cases, "Two visitors register
the same email address simultaneously").

### Enums

| Go type | Field | Values |
|---|---|---|
| `identity.Role` | `role` | `user` `admin` |
| `identity.UnitSystem` | `unit_system` | `metric` `imperial` |
| `identity.DateFormat` | `date_format` | `iso` `dmy` `mdy` |
| `identity.Theme` | `theme` | `system` `light` `dark` |

### Validation — `identity.User.Validate()`, every offending field reported at once

| Rule | Requirement | Error `code` |
|---|---|---|
| `email` parses as an address, ≤255 | FR-001 | `required` / `invalid_email` |
| `name` trimmed, 1..120 | FR-001, FR-011 | `required` / `too_long` |
| `role`, `unit_system`, `date_format`, `theme` in vocabulary | FR-011 | `invalid_value` |
| `locale` matches `^[a-z]{2}(-[A-Za-z]{2})?$` | FR-011 | `invalid_value` |

### Password rules — `identity.ValidatePassword(pw, email, name)`, published to the person

| Rule | Requirement | Error `code` |
|---|---|---|
| at least **8** characters | FR-004 | `too_short` |
| not equal to the account's email address, case-insensitively | FR-004 | `same_as_email` |
| not equal to the account's display name, case-insensitively | FR-004 | `same_as_name` |
| at most 200 characters | denial-of-service floor on bcrypt | `too_long` |

**The rules are rendered on the sign-up and change-password forms** before the person chooses,
not only reported after they fail — FR-004 requires them to be published, and a rule discovered by
violating it is not published.

### State transitions

```
(absent) --register--> active --change password--> active --delete--> (gone, permanently)
                          |
                          +--operator sets disabled_at--> disabled --operator clears--> active
```

There is no self-service disable, no archive, no soft delete and no reactivation flow. `disabled_at`
is set only by a superuser through the admin UI in this phase; phase 006 adds
`PATCH /api/v1/admin/users/{id}`.

---

## 2. `medications` — new

The single clinical record kind of this phase, and the template every kind in phases 003 and 004
copies. The field list is SHARED-DESIGN §1.5's `medication` row **minus** the reference relations
and reminder fields this phase's charter defers (research D-03).

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `owner` | relation → `users` | **yes** | `MaxSelect: 1`, **`CascadeDelete: true`** | The authorization anchor. **Server-set, absent from every request DTO.** FR-014, FR-032. Phase 002 replaces this with `patient`. |
| `name` | text | **yes** | 1..200 | **PHI** |
| `alternative_name` | text | no | ≤200 | **PHI**. FR-015 |
| `type` | select | no | `MedicationType` | FR-015, FR-016 |
| `dosage` | text | no | ≤200 | "how much". FR-015 |
| `frequency` | text | no | ≤100 | "how often". FR-015 |
| `route` | select | no | `MedicationRoute` | FR-015, FR-016 |
| `indication` | text | no | ≤300 | "why it is taken". **PHI** — an indication names a condition. FR-015 |
| `started_on` | date | no | calendar date | the primary date; drives the default sort. FR-015, FR-019 |
| `ended_on` | date | no | calendar date, `>= started_on` | FR-018 |
| `status` | select | **yes** | `TherapyStatus`, default `active` | FR-015, FR-016 |
| `side_effects` | text | no | ≤1000 | **PHI**. FR-015 |
| `notes` | text | no | ≤5000 | **PHI**. FR-015. `notes` is `text ≤5000` on every clinical kind in every phase |

**Deliberately absent**, each with the phase that adds it: `patient` (002), `practitioner` (002),
`pharmacy` (002), `tags` (003), `reminder_enabled` / `reminder_times` / `reminder_weekdays` /
`reminder_message` (005, with notifications), and `deleted_at` (never — constitution VII scopes
soft delete to files).

### Enums

**`clinical.MedicationType`** — FR-016's "prescription, over-the-counter, supplement or herbal":

`prescription` `otc` `supplement` `herbal`

**`clinical.MedicationRoute`** — FR-016's "by mouth, under the tongue, on the skin, inhaled,
injected and the rest", fourteen values, verbatim from SHARED-DESIGN §1.4 so phase 003 inherits
it unchanged:

`oral` `sublingual` `topical` `transdermal` `inhalation` `nasal` `ophthalmic` `otic` `rectal`
`vaginal` `intramuscular` `subcutaneous` `intravenous` `other`

**`clinical.TherapyStatus`** — FR-016's "currently taking, paused, finished, stopped or
cancelled". This is the shared course-of-therapy ladder; phase 003 reuses it for treatments and
equipment, which is why it is named for the shape and not for the kind:

| Value | What the interface calls it |
|---|---|
| `active` | Currently taking |
| `on_hold` | Paused |
| `completed` | Finished |
| `stopped` | Stopped |
| `cancelled` | Cancelled |

FR-016 requires any value outside a published set to be **refused rather than stored as free
text**, at both the domain layer and the storage layer. A table-driven test submits an
unrecognised value for each of the three fields and asserts `422 validation_failed` with code
`invalid_value` and the field named.

### Indexes

```sql
CREATE INDEX idx_medications_owner        ON medications (owner);
CREATE INDEX idx_medications_owner_start  ON medications (owner, started_on DESC, id DESC);
CREATE INDEX idx_medications_owner_name   ON medications (owner, LOWER(name), id DESC);
CREATE INDEX idx_medications_owner_upd    ON medications (owner, updated DESC, id DESC);
CREATE INDEX idx_medications_owner_status ON medications (owner, status);
```

The three composite indexes each back one of FR-022's orderings — most recently started, by name,
most recently changed — and each ends in `id` because the keyset cursor's tiebreaker is always the
id (research D-25). Without the id column in the index, two medications started on the same day
would page unstably, which is exactly what FR-023 forbids. `idx_medications_owner_status` backs
FR-022's narrowing by state.

**Deliberately no unique index on `name`.** A person may legitimately record two courses of the
same drug at different doses or in different periods.

### Validation — `clinical.Medication.Validate()`

Accumulates; never returns on the first failure (FR-027).

| Rule | Requirement | Error `code` |
|---|---|---|
| `name` trimmed, 1..200 | FR-015, FR-017 | `required` / `too_long` |
| `alternative_name` ≤200, `dosage` ≤200, `frequency` ≤100, `indication` ≤300, `side_effects` ≤1000, `notes` ≤5000 | FR-017 | `too_long`, message naming the field **and its limit** |
| `type`, `route` in vocabulary when non-empty | FR-016 | `invalid_value` |
| `status` present and in vocabulary | FR-016 | `required` / `invalid_value` |
| `started_on`, `ended_on` are real calendar dates when present | FR-018 | `invalid_date` |
| `ended_on >= started_on` when both are present — **equality accepted** | FR-018, Edge Cases | `end_before_start` |
| a future `started_on` is **accepted** | Edge Cases ("a course beginning next week") | — |

**Text at exactly the documented limit is accepted; one character over is refused with a message
naming the field and the limit** (FR-017 and Edge Cases). The boundary is measured in Unicode
code points, not bytes, so a name in a non-Latin script is not silently shorter than a Latin one —
a table row asserts a 200-code-point CJK name is accepted.

Names in non-Latin scripts, names containing right-to-left text, and names containing characters
that look like markup are stored, displayed and searched as text and never interpreted (Edge
Cases). templ escapes by default and that escaping is load-bearing under the CSP (research D-35);
a render test asserts a name of `<script>alert(1)</script>` appears as visible text.

### State transitions

```
(absent) --create--> exists --update--> exists --delete--> (gone, permanently)
                                |
                                +-- status: active <-> on_hold -> completed | stopped | cancelled
```

`status` is a free-form choice among the five values, not a state machine: a person may correct a
mistake by moving a medication back from `stopped` to `active`, and no requirement forbids it.
There is no draft, no archive, no soft delete and no undo. FR-028: deletion is permanent, and the
confirmation says so **before** the person commits.

### Derived, never stored

- `is_current` — `status == active`. Used for the list's default narrowing and for the detail
  view's heading. Never a column, because it is a function of a column.
- `version` / `ETag` — derived from `updated` (research D-24). Never a column.

---

## 3. `audit_events` — new

An immutable record that something happened. Its defining property is negative: **there is no
field in this collection that a value, a name, a note or a diff could be written into.** That is
what makes FR-036's content rule structural rather than procedural.

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `occurred_at` | date | **yes** | RFC3339 UTC | server clock, never a client-supplied time |
| `actor` | relation → `users` | no | `MaxSelect: 1`, **`CascadeDelete: false`** | null for system and superuser actions, and **unset rather than cascaded** when an account is deleted, so the `account_delete` row survives (research D-22) |
| `actor_kind` | select | **yes** | `ActorKind` | carries what kind of actor it was even after `actor` is unset |
| `action` | select | **yes** | `Action` | |
| `target_kind` | select | **yes** | `TargetKind` | |
| `target_id` | text | no | ≤64 | an opaque id — **never a name, never a path, never a filename** — with one bounded exception: when `target_kind` is `system`, `backup` or `export`, there is no record to point at and this carries the **job name or archive name** instead (`medigo_purge_artifacts`, `medigo_20260827120000.zip`). **64 is sized, not guessed**: the longest name the suite composes is phase 006's restore safety copy, `medigo_safety_<YYYYMMDDHHMMSS>_<name>` over a manual archive `medigo_<YYYYMMDDHHMMSS>.zip` — 14 + 14 + 1 + 25 = 54. Timestamps are compact throughout for exactly this reason; spelled as RFC3339 the same name is 66 and would not fit. Phase 006 bounds uploaded archive keys to 64 on the same grounds (ANALYSIS N2). Those are operator-facing identifiers the operator already chose, not personal data, and the backup name is the same string its route is addressed by. Sized 64 because a PocketBase record id is 15 and an archive name is ~40 (ANALYSIS: 006 writes 18–29-character job names into a 15-character column). |
| `request_id` | text | **yes** | ≤64 | correlates to the zerolog stream (FR-054). **A background run has no HTTP request and still fills this**: the cron, job, migration and backfill contexts mint a *run id* from the same helper that mints request ids, and the run's zerolog lines carry the same value — so the retention purge's own row correlates to the log of the purge that wrote it. The column is required precisely so a row that correlates to nothing cannot be written; without the run id the nightly purge would fail `Required` validation on its first tick in production (ANALYSIS). |

**No `ip` column** (research D-19). **No content column of any kind**, ever.

### Enums

**`audit.ActorKind`**: `user` `admin` `superuser` `system`

**`audit.Action`** — declared **in full**, not narrowed. This phase declares the shared design
contract's complete nineteen-value vocabulary plus `access_denied` (post-design re-check item 2),
for **twenty** values. It is declared here in full, rather than grown phase by phase, because a
`SelectField` write with an undeclared value fails validation: a vocabulary assembled from six
deltas fails in production on the first share, the first non-owner photo fetch and the first
backup, not in a test. Later phases extend it additively, and **every** later phase's vocabulary
migration asserts the *complete* expected set rather than its own delta, so a missing value is a
red test rather than a `400` in front of a person.

Ten values are **written by this phase**, each with a test:

| Value | Written when | Requirement |
|---|---|---|
| `create` | a medication or an account is created | FR-036 |
| `update` | a medication or a profile is changed | FR-036 |
| `delete` | a medication is deleted | FR-036 |
| `access_denied` | any refused attempt to reach a record, including a forged cursor | FR-036, research D-20 |
| `login` | any successful sign-in, through either path | FR-036, research D-14 |
| `login_failed` | any refused sign-in | FR-006, FR-036 |
| `logout` | sign-out | FR-036 |
| `password_change` | a successful password change | FR-036 |
| `account_delete` | self-service account deletion | FR-036 |
| `admin_session` | a superuser session begins | FR-040 |

Ten values are **declared here and first written later**. The migration that declares them is
this phase's; the phase that writes each is named so that no later reader goes looking for a
migration that does not exist:

| Value | First written by |
|---|---|
| `read_sensitive` | 002 (a photo fetched by somebody who is not the owner), then 004 and 005 |
| `share_grant`, `share_revoke`, `share_expire` | 005 |
| `invite_send`, `invite_respond` | 005 |
| `export` | 006 |
| `backup_create`, `backup_restore` | 006 |
| `email_change` | **no phase in 001–006 writes it.** PocketBase's native email-change endpoints stay reachable (the lockdown is scoped to record CRUD), so the day a hook audits them the value must already exist. Declaring one unwritten enum value costs nothing; omitting it costs a validation failure at the worst moment. The shared design contract lists it. |

**`audit.TargetKind`** — declared **in full** for the same reason: the shared design contract's
fifteen record kinds and eight platform kinds, **twenty-three** values.

- the fifteen record kinds — `medication` `allergy` `condition` `encounter` `procedure`
  `treatment` `symptom` `vitals` `immunization` `injury` `insurance` `equipment`
  `emergency_contact` `family_member` `lab_result`
- the eight platform kinds — `patient` `user` `share` `invitation` `attachment` `export`
  `backup` `system`

This phase **writes** three of them (`medication`, `user`, `system`); the other twenty are written
by the phase that builds the thing they name. The declared value list is a contract, not an
inventory of what exists yet — a `SelectField` value is a string, and declaring it early is what
makes each later phase a collection migration rather than a collection migration plus a vocabulary
migration nobody remembered to write.

Five further values are **not** declared here, because they name things the shared design contract
does not: `practitioner` and `facility` (phase 002), `tag` and `search` (phase 003), and
`report_template` (phase 006). Each arrives in its own phase's vocabulary migration, whose test
asserts the complete set at that point.

### Indexes

```sql
CREATE INDEX idx_audit_occurred    ON audit_events (occurred_at DESC, id DESC);          -- retention purge + reader
CREATE INDEX idx_audit_actor_time  ON audit_events (actor, occurred_at DESC, id DESC);   -- "what did this account do"
CREATE INDEX idx_audit_target      ON audit_events (target_kind, target_id, occurred_at DESC);
```

**These are created wide enough for phase 006's reader on day one, deliberately.** Each carries
the `id DESC` (or `occurred_at DESC`) tiebreaker that 006's keyset paging needs to stay
index-only, so **phase 006 creates no audit index at all** and there is nothing to drop, widen or
rename later. The alternative — 001 creating three narrow indexes and 006 creating three wider
ones beside them — puts six b-trees on the highest-write-volume collection in the instance, and
006's `idx_audit_target` would collide by name with the one above and fail the migration outright
(ANALYSIS). `id DESC` costs nothing here: it is already the row's primary key.

Phase 002 adds `patient` and `idx_audit_patient_time`, wide for the same reason; phase 005 adds
`reason`; phase 006 adds `affected` and builds its read surface on these four indexes unchanged.

### The events this phase must produce, each with a test

| Trigger | actor_kind | action | target_kind | target_id |
|---|---|---|---|---|
| an account is created | `user` | `create` | `user` | the new account |
| a successful sign-in, by either path | `user` | `login` | `user` | the account |
| a refused sign-in | `system` | `login_failed` | `user` | the account **when it exists**, empty otherwise |
| sign-out | `user` | `logout` | `user` | the account |
| a password change | `user` | `password_change` | `user` | the account |
| a password replaced through a recovery link | `user` | `password_change` | `user` | the account |
| an address confirmed through a confirmation link | `user` | `update` | `user` | the account |
| a profile or preference change | `user` | `update` | `user` | the account |
| self-service account deletion | `user` | `account_delete` | `user` | the account (actor unset immediately after by the delete) |
| a medication is created / changed / deleted | `user` | `create` / `update` / `delete` | `medication` | the medication |
| any refused attempt to reach a medication | `user` | `access_denied` | `medication` | the id **as addressed** |
| an anonymous request to a signed-in route | `system` | `access_denied` | `system` | empty |
| a superuser session begins | `superuser` | `admin_session` | `user` | the superuser record |
| the retention purge runs | `system` | `delete` | `system` | empty |

**Recovery and confirmation introduce no new action value, deliberately.** A completed recovery
*is* a password change and is recorded as one; a confirmed address *is* a change to the account
record and is recorded as `update`. The *request* halves write nothing: there may be no account to
point at, and writing the address somebody typed — possibly a stranger's, possibly a typo of one —
into a two-year medical audit trail is exactly the disclosure the `login_failed` rule below
forbids. Rate-limited request bursts write `login_failed` / `user` with an empty `target_id`, which
is the signal an operator actually needs. The consequence worth stating: the vocabulary stays at
**twenty** actions and **twenty-three** target kinds, so every later phase's complete-set assertion
(T070a and its four successors) is unchanged by cross-artifact finding H7.

**The `login_failed` row never carries the attempted email address.** When the address is unknown
there is no account to point at, and writing the typed string would put a real person's address —
possibly a stranger's, possibly a typo of one — into a two-year medical audit trail. `target_id`
is empty and `request_id` carries the correlation an operator needs (research D-17).

### Immutability and retention

- All five API rules `nil`, and MediGo exposes no write path.
- `OnRecordUpdate("audit_events")` rejects **unconditionally**.
- `OnRecordDelete("audit_events")` rejects unless the context carries the retention job's marker,
  which is set by the cron and nowhere else.
- A PocketBase cron runs daily, deleting rows older than `MEDIGO_RETENTION_AUDIT_DAYS`
  (**default 730**, per the specification's Assumptions), and writes one `delete`/`system` row
  recording that it ran — never one per deleted row.

A test attempts an update and a delete through every reachable surface and asserts both are
refused (FR-037).

---

## 4. Relationship map

```
users ──1:N (CascadeDelete: true)──> medications
  ^
  └──0..1 actor (CascadeDelete: FALSE, auto-unset)──── audit_events
```

Two collections, one cascade, one deliberate non-cascade. Reading it for the one destructive path
this phase has:

- **Delete an account** → PocketBase's `deleteRefRecords` (`core/record_model.go:1587-1626`)
  deletes every medication whose `owner` points at it, because that relation is `CascadeDelete:
  true`; and **unsets** `audit_events.actor` on every historical entry, because that one is not.
  One transaction. FR-014 and SC-012 are satisfied by PocketBase's behaviour rather than by MediGo
  code — which is exactly why they are asserted by an integration test rather than assumed:
  `SELECT COUNT(*) FROM medications WHERE owner = '<deleted id>'` must be `0`, and the
  `account_delete` audit row must still exist with a null actor and `actor_kind = user`.

**The `Required` / `CascadeDelete` matrix is asserted at boot**, field by field, by
`internal/store/migrations/assertions.go`. A silent flip of `audit_events.actor` to
`CascadeDelete: true` would make deleting an account destroy the record that it was deleted; a
flip to `Required: true` with `CascadeDelete: false` would make deleting an account **fail
outright** (`core/record_model.go:1619` fails the delete when an emptied relation is required),
silently breaking FR-014 the first time anybody closed an account with history. Both are one
character away and neither would produce a compile error, so the assertion is the control.

| Relation | Required | CascadeDelete | Deleting the target does… | Requirement |
|---|---|---|---|---|
| `medications.owner → users` | yes | **true** | deletes the medication | FR-014, SC-012 |
| `audit_events.actor → users` | **no** | **false** | unsets the reference, keeps the row | FR-036, FR-037 |

---

## 5. Migrations

All three register into `core.AppMigrations` via
`migrations.Register(up func(core.App) error, down func(core.App) error, filename)` — a signature
that **requires both directions**, which is how Principle IX's reversibility rule is enforced by
the API itself (VERIFIED-SOURCE-FACTS FACT 8). None of the three needs the documented-irreversibility
escape; all three genuinely reverse.

**All pending migrations share one transaction** (`core/migrations_runner.go:129-131`), so this
phase's three either all apply or none do. There is no half-migrated state to design for, which
is what makes FR-063 ("start successfully against an empty storage location") a single assertion.

| # | File | Up | Down |
|---|---|---|---|
| 1 | `1756100100_users_profile.go` | add the seven fields; set all five rules `nil` and `AuthRule` to `""`; set `PasswordAuth.MinPasswordLength = 8` and `IdentityFields = ["email"]`; set `AuthToken.Duration`; add `idx_users_email_lower` | remove the seven fields and the index; restore PocketBase's stock rules |
| 2 | `1756100200_medications.go` | create the collection, five nil rules, all thirteen fields, five indexes | delete the collection |
| 3 | `1756100300_audit_events.go` | create the collection, five nil rules, all seven fields, three indexes | delete the collection |

**Migration 1's `down` restores PocketBase's stock `users` rules rather than leaving them `nil`**,
because a reversal that leaves a collection in a state PocketBase's own migrations did not create
is not a reversal. The migration file says so in a comment.

**Ordering is forced**: 1 before 2 (`medications.owner` relates to `users`) and 1 before 3
(`audit_events.actor` relates to `users`). 2 and 3 are independent of each other and are ordered
only for determinism.

### `internal/store/migrations/assertions.go` — run at boot, refuses to start

1. **Every non-system collection has all five API rules `nil`.** Constitution V. The process exits
   with a message naming the collection and the offending rule.
2. **Every `FileField` in the schema has `Protected: true`.** Constitution VII. Zero file fields
   exist in this phase; the assertion is written and tested now, with a test that flips a
   synthetic field to `false` and asserts the assertion fires, so phase 002's `patients.photo`
   lands into an existing gate.
3. **The `Required` / `CascadeDelete` matrix of §4 matches the declared schema, field by field.**
4. **`Settings().Batch.Enabled` is `false`** and **`Settings().Logs.MaxDays` is `1`** — never `0`,
   for the reason in research D-29.

Assertions 1 and 2 are *refusals to start*. Assertions 3 and 4 are refusals to start in
production and loud warnings in dev, because a developer mid-migration should get a message
rather than a process that will not boot.

---

## 6. Fixture and seed data

`internal/testdata/pb_data` — the directory every `tests.NewTestApp` clones — and `medigo seed`
produce **the same deterministic set**. Ids are exported as constants from
`internal/testsupport/fixtures.go`, so no test anywhere contains a literal id. FR-060 requires
determinism and at least two accounts holding medications; SC-003 requires the smoke gate to pass
on a legitimately empty page.

| Fixture | Content |
|---|---|
| **Account A** — `amara@example.test` | 12 medications spanning all five `status` values and all four `type` values, including one with **only a name** (the "partial data" edge case), one whose name contains right-to-left text and markup-like characters, one single-day course (`started_on == ended_on`), and one with a future `started_on`. |
| **Account B** — `boris@example.test` | 3 medications. **The isolation counterparty**: every stranger-refused test addresses Account A's ids while signed in as B, and vice versa. |
| **Account C** — `chidi@example.test` | **No medications at all.** This is what the empty-state smoke case navigates to, so the `@EmptyState` path inside the `region[name="Medications"]` landmark is exercised on every run rather than asserted (research D-39). |
| Superuser | `admin@example.test`, so the admin-UI smoke and the `admin_session` audit test have a credential |

The seed sets `MEDIGO_AUTH_REGISTRATION_OPEN=true` in the documented smoke environment, because
`/register` must be reachable for its smoke case and because the sign-up path is one of the six
user stories (FR-002, and the specification's Assumptions).

**A 1,000-medication account is generated by a separate benchmark helper, not by the seed** —
SC-002's latency budget needs it and the smoke gate does not, and a seed that takes ten seconds to
run is a seed people stop running.

---

## 7. What this data model deliberately does not contain

Stated because the next reader will look for each of these and their absence should read as a
decision, not an oversight:

- **No `patients` collection.** Phase 002. In this phase a medication belongs to the account
  holder, and the specification's Assumptions say so explicitly and say why: "the seam is
  introduced once, with real requirements behind it".
- **No `tags`, `practitioners`, `facilities` or catalogs.** Phases 002 and 003.
- **No `shares`, `invitations`, `attachments`, `search_index`, `report_templates` or
  `export_jobs`.** Phases 004 to 006.
- **No `deleted_at` on any collection.** Constitution VII scopes soft delete to files, and this
  phase has none. Adding one would put a filter on every query in the application to buy a
  capability FR-028 explicitly refuses.
- **No `patient` column on `audit_events`.** Phase 002 adds it, because it is what makes a
  per-patient activity view possible — and there are no patients yet.
- **No `reason` column on `audit_events`.** Phase 005 adds it, with the refusal reasons of a
  sharing model that does not exist yet. A refusal recorded in this phase is an `access_denied`
  row with no reason, which is all FR-036 asks for.
- **No `affected` column on `audit_events`.** Phase 006 adds it, with the scheduled-job envelope
  that is the only thing that writes a count.
- **No `ip` column on `audit_events`.** Research D-19. Every phase's audit content rule is written
  without one; nothing in 001–006 publishes an `ip` field.
- **No sessions table.** PocketBase's token *is* the session, and rotating a record's `tokenKey`
  is the revocation mechanism (research D-16). A second one would need its own invalidation story
  and would be a second place for the two to disagree.
- **No settings collection.** Configuration comes from the environment, validated at boot, with no
  configuration files and no second mechanism (FR-051). Where PocketBase persists settings of its
  own — rate limits, token durations, log retention — MediGo **writes them at boot from its own
  validated config** and nobody edits them in the admin UI, which is what keeps the environment
  the single source (research D-18).
