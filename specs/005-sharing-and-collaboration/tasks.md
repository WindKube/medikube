---
description: "Task list for phase 005 — Sharing and Collaboration"
---

# Tasks: Sharing and Collaboration

**Input**: Design documents from `/specs/005-sharing-and-collaboration/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: **MANDATORY.** Constitution Principle III makes test-first non-negotiable and the
specification demands it by name (FR-076, FR-077, FR-078, FR-079, FR-080, SC-018, SC-019). Every
test task below precedes the implementation task it covers. A red-to-green transition that was never
red is a defect.

**Organization**: by user story, in the spec's priority order. Each story is independently
implementable, testable and demonstrable once the Foundational phase is complete.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel with its siblings (different files, no incomplete dependency)
- **[Story]**: `[US1]`…`[US6]`; Setup, Foundational and Polish carry no story label
- Every task names the exact file path it touches

## Path conventions

All paths are relative to `/Users/krzysztof.wiatrzyk/private/monorepo/medikube`.

## Read before writing a single query

[research.md D-01](./research.md#d-01): **PocketBase has no NULL.** Optional date and single-relation
columns are `TEXT DEFAULT '' NOT NULL` (`core/field_date.go:110`, `core/field_relation.go:161`).
`revoked_at IS NULL` matches nothing; `revoked_at IS NULL OR …` matches everything **including
revoked grants**. Every predicate in this phase is `= ''`, and T009/T010 make that a build gate.

---

## Phase 1: Setup

**Purpose**: linters, configuration and fixtures ready for a phase that widens every authorization
decision in the application. No domain logic here.

- [ ] T001 Verify the toolchain precondition and record it in `specs/005-sharing-and-collaboration/quickstart.md` §0: `go.mod` declares `go 1.27` with a `toolchain go1.27.x` line, and `GOTOOLCHAIN` is unset in `Taskfile.yaml` and every file under `.github/workflows/`
- [ ] T002 [P] Add `internal/platform/mail` to the `depguard` `[PB]` allowlist in `.golangci.yml` (it legitimately imports both PocketBase and templ) and confirm the existing `**/internal/service/**` and `**/internal/domain/**` deny globs already cover `internal/service/share` and `internal/domain/share` with no new rule
- [ ] T003 [P] Add a `forbid-is-null` step to `Taskfile.yaml` (`task lint:isnull`) that greps `internal/store/share/`, `internal/store/invitation/`, `internal/store/access/` and `internal/store/migrations/` for the literal `IS NULL` and fails, with the message pointing at research D-01
- [ ] T004 [P] Add `SharingConfig` to `internal/config/config.go` with `envPrefix:"SHARING_"` — `InvitationTTL` (default `168h`), `InvitationTTLMin` (`1h`), `InvitationTTLMax` (`8760h`), `MaxResourcesPerInvitation` (`50`), `InvitationRetentionDays` (`90`) — plus boot validation that min ≤ default ≤ max
- [ ] T005 [P] Write failing tests in `internal/config/config_test.go` for the five `MEDIKUBE_SHARING_*` variables: defaults, bounds, and a boot failure when min > max
- [ ] T006 [P] Add `task test:slowsse` and extend `task test:scale` and `task test:phileak` wrappers in `Taskfile.yaml` for this phase's build-tagged suites
- [ ] T007 Extend `internal/cli/seed.go` with the nine accounts and six invitation states from `data-model.md` §7, including the unregistered `newcomer@medikube.local` address and the printed plaintext token, then run `task fixture:regen` to rewrite `internal/testdata/pb_data`

**Checkpoint**: linters, configuration and fixtures in place. No production behaviour has changed.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the collections, the domain, the single authorization checkpoint and the shared test
suites. **No user story may start until this phase is complete** — every story reads a grant.

### The domain (no PocketBase, no HTTP, no templ)

- [ ] T008 [P] Write failing table-driven tests in `internal/domain/share/level_test.go` and `internal/domain/share/resource_test.go`: `Level.Valid()` accepts exactly `view`/`edit` and rejects the empty string, `full`, and any upper-case or hyphenated form; `Level.Allows(PermOwn)` is **false for both levels** (FR-005, FR-006); there is no third level and no caller-defined permission (FR-002); `ResourceKind.Valid()` accepts exactly `patient`/`family_member`, both carried by the **one** `Share` type (FR-001)
- [ ] T009 [P] Write failing tests in `internal/domain/share/share_test.go` for invariants S1–S6 of `data-model.md` §1 — exactly one of `patient`/`family_member` set and matching `resource_kind`; `grantor != grantee`; family-history implies `view`; `expires_at` empty or strictly future; `revoked_by != ""` implies `revoked_at != ""`; `revoked_by ∈ {grantor, grantee, ""}` — each producing a `*domain.ValidationError` naming the field, and one case proving two violations are reported together. `expires_at` empty means open-ended and an end date not strictly in the future at the moment it is set is refused (FR-008)
- [ ] T010 [P] Write failing tests in `internal/domain/share/share_test.go` for `Share.Active(now)` — an absent end date is open-ended, a past one is not (FR-008): empty `revoked_at` + empty `expires_at` → active; empty `revoked_at` + future `expires_at` → active; empty `revoked_at` + **past** `expires_at` → inactive; non-empty `revoked_at` → inactive regardless of expiry. Table-driven over the empty string, never over `nil`
- [ ] T011 [P] Write failing tests in `internal/domain/share/invitation_test.go` for invariants I1–I6 of `data-model.md` §2, including the case-insensitive self-invitation refusal (FR-021) and the `[now+1h, now+1y]` window (FR-017)
- [ ] T012 [P] Write failing **exhaustive** tests in `internal/domain/share/transition_test.go` enumerating every `(Status, Event)` pair from `data-model.md` §3: the five legal edges succeed, every other pair returns `*share.TransitionError` carrying the current state, and **no edge returns to `pending`** (FR-026, SC-012)
- [ ] T013 [P] Write failing tests in `internal/domain/share/redaction_test.go`: `Share.MarshalZerologObject` and `Invitation.MarshalZerologObject` emit only ids and enum values, and a rendered log line contains **none** of a seeded email address, note, response note, display name or token hash (FR-072)
- [ ] T014 Implement `internal/domain/share/{level.go,resource.go,status.go,share.go,invitation.go,transition.go,errors.go}` — entities, the pure transition table, `ErrAlreadyShared`, `ErrInvitationOutstanding`, `ErrSelfShare`, `ErrNotAddressedToYou`, `ErrLapsed`, `*TransitionError`, and the redacting marshallers (depends on T008–T013)
- [ ] T015 Extend `internal/domain/access/access.go`: `Grant` gains `ViaShare`, `Note`, `ExpiresAt`; `Permission` is unchanged; add a test asserting no `Level` maps to `PermOwn`

### The collections

- [ ] T016 [P] Write failing migration tests in `internal/store/migrations/shares_test.go`: all five API rules `nil`; the five indexes of `data-model.md` §1 exist; at most one active grant of a thing to an account is a storage-level invariant (FR-010) — **`idx_shares_active_unique` is unique and its `WHERE` clause is the literal `revoked_at = ''`, asserted on the index SQL string**; a second active grant for the same `(resource_kind, patient, family_member, grantee)` is rejected and one after a revoke is accepted; deleting a `patients`, `family_members`, grantor `users` or grantee `users` row deletes the dependent shares; the `down` restores the previous schema
- [ ] T017 Implement `internal/store/migrations/1756xxx100_shares.go` per `data-model.md` §1 and §6, with a real `down` (depends on T016)
- [ ] T018 [P] Write failing migration tests in `internal/store/migrations/invitations_test.go`: all five rules `nil`; `idx_invitations_token` unique on `token_hash`; the three remaining indexes exist; deleting a `users` sender cascades; deleting an `invitations` row **empties** `shares.invitation` rather than deleting the share (`core/record_model.go:1618-1626`); the `down` removes `shares.invitation` and then the collection
- [ ] T019 Implement `internal/store/migrations/1756xxx200_invitations.go` per `data-model.md` §2 and §6, including adding `shares.invitation` (depends on T017, T018)
- [ ] T019a [P] Write failing tests in `internal/store/migrations/audit_vocab_sharing_test.go`: `audit_events` has a `reason` column (text, ≤40, optional) after `up` and does not after `down`; and the **complete** expected vocabulary after this phase — **twenty-six** actions and **twenty-seven** target kinds, set-equal, not a delta — extending the shared vocabulary test from phase 001 (T070a). A value this phase writes but no migration declared fails here rather than failing `SelectField` validation in production (ANALYSIS C1, C2)
- [ ] T020 [P] Implement `internal/store/migrations/1756xxx300_audit_vocab_sharing.go` adding the `reason` column (`data-model.md` §4.1 — text, ≤40, optional, bounded by the Go `audit.Reason` set) and extending `audit_events.action` with `share_update`, `share_leave`, `invite_cancel`, `invite_withdraw`, `invite_expire`, with a reversing `down` documented in the file (depends on T019a)
- [ ] T021 Extend `internal/store/migrations/assertions.go` and its test so boot refuses to start when either new collection has a non-nil API rule, and re-assert that the only file fields on the instance remain `patients.photo` and `attachments.file`, both `Protected: true`

### The ports, the fakes and the contract suites

- [ ] T022 Declare the consumer-side ports in `internal/service/share/ports.go`: `Repository` (Get, List, Save, Delete), `InvitationRepository` (Get, GetByTokenHash, List, Save, Delete), `Authorizer` (Patient, Record), `Auditor` (Record), `Mailer` (Configured, SendInvitation), `Notifier` (Notify), `Clock` (Now), `TokenMinter` (Mint), `UserDirectory` (FindByEmail) — every one consumer-declared, none an omnibus interface
- [ ] T023 Declare `access.ShareReader` (`ActiveFor`, `ActivePatientsFor`) in `internal/service/access/ports.go`
- [ ] T024 [P] Implement in-memory fakes for all nine share ports plus `ShareReader` in `internal/service/share/sharetest/fake.go`, with an injectable clock so lapse is tested by moving time, never by sleeping
- [ ] T025 [P] Implement `internal/service/share/sharetest/contract.go` — `RepositoryContract` and `InvitationRepositoryContract` as `testify/suite`s, parameterised by a factory so both run against the real repository and the fake: not-found; the active predicate excludes revoked and lapsed rows; the unique-active constraint; cursor stability under a concurrent insert and a concurrent revoke; cascade removal; case-insensitive `recipient_email` lookup; `GetByTokenHash` misses on a non-pending invitation
- [ ] T026 [P] Implement `internal/service/access/accesstest/contract.go` — the `ShareReader` contract suite, run against the real reader and the fake, covering: owner-only patient, active grant, revoked grant, lapsed grant, disabled grantee — refused for as long as the account is disabled, with the grant row still present afterwards (FR-048) — and a grantee of a **different** patient of the same owner
- [ ] T027 [P] Implement `internal/service/share/sharetest/fixtures.go` — builders for a share in each of its four observable states and an invitation in each of its six statuses, used by the suites, the HTTP tests and `medikube seed`

### The one authorization checkpoint — the change the whole phase turns on

- [ ] T028 Write failing tests in `internal/service/access/authorizer_test.go` for the widened resolution order of `contracts/README.md`: superuser allowed and audited; owner → `PermOwn`; active `view` grant → `PermView`; active `edit` grant → `PermEdit`; **no grant of any level satisfies `PermOwn`**; a grant lacking the needed permission returns `ErrForbidden` **with the `Grant` attached**; revoked, lapsed, disabled-grantee and stranger all return `ErrNotFound` and are **byte-identical** to each other; every refusal writes exactly one `access_denied` audit row carrying a `reason` and no content (FR-069)
- [ ] T029 Implement the widening in `internal/service/access/authorizer.go`: owner → grant → `ErrNotFound` — the level read from the stored grant and never from anything the caller sends (FR-012), and every read and write in the application decided here from stored ownership or an active grant, never from the person in view (FR-054) — with no cross-request cache (FR-037) and an optional per-request memo (depends on T028)
- [ ] T030 [P] Write failing integration tests in `internal/store/access/sharereader_test.go` running `accesstest.ShareReaderContract` against a real `tests.NewTestApp`
- [ ] T031 Implement `internal/store/access/sharereader.go` — `ActiveFor` and `ActivePatientsFor`, each one indexed query, **the D-01 predicate written exactly once** in `internal/store/share/query.go` and reused (depends on T030)
- [ ] T032 [P] Write failing integration tests in `internal/store/share/repo_test.go` and `internal/store/invitation/repo_test.go` running the two contract suites from T025 against a real test app
- [ ] T033 Implement `internal/store/share/{repo.go,mapper.go,query.go}` and `internal/store/invitation/{repo.go,mapper.go}` (depends on T032)

### The realtime hub, the mail adapter and the audit hooks

- [ ] T034 [P] Write failing tests in `internal/realtime/hub_test.go` for the second event shape: an event addressed to user A reaches A and **not** B; a patient-topic event is unaffected; unsubscribing on context cancellation returns the subscriber count to its starting value (no goroutine leak); the hub carries **ids and kinds only**, asserted by the `Event` struct having no body field
- [ ] T035 Implement `internal/realtime/event.go` (`Event` gains `Type ∈ {RecordChanged, AccessChanged, Notice}` and `UserID`) and `internal/realtime/hub.go` (a per-user topic map alongside the per-patient one, context-bounded subscribers, no broker interface) (depends on T034)
- [ ] T036 [P] Write failing tests in `internal/platform/mail/mailer_test.go` using `tests.NewTestApp`: `SendInvitation` produces exactly one message in `app.TestMailer.Messages()`, the `To` is the invited address, the `From` comes from `Settings().Meta`, and the rendered HTML contains the sender's display name, the item count, the level, the lapse date and the note — and **contains none of the seeded patient's name, any diagnosis or any medication** (FR-023, SC-010)
- [ ] T037 Implement `internal/web/views/mail/invitation.templ` and `internal/platform/mail/mailer.go` — `share.Mailer` over `app.NewMailClient().Send()` (research D-03) and `Configured()` over `Settings().SMTP.Enabled` (research D-05), reusing the same `Settings().SMTP.Enabled` read phase 001's mail-unconfigured refusal already performs rather than adding a second one (depends on T036)
- [ ] T038 [P] Extend the failing tests in `internal/platform/pb/boot_test.go` **[EDIT]**: booting with `SMTP.Enabled == false` emits **exactly one** loud warning naming outbound email — phase 001 already emits it for password recovery and address confirmation (001 FR-076, T223f), and this phase only adds invitations to the features it names, rather than emitting a second warning — and booting with it enabled emits none
- [ ] T039 Implement the boot warning in `internal/platform/pb/boot.go` (FR-022) (depends on T038)
- [ ] T040 [P] Write failing tests in `internal/platform/pb/hooks_test.go` asserting the post-commit audit hooks on `shares` and `invitations` fire on create/update/delete, do **not** fire for a rolled-back transaction, and write rows carrying no address, note, display name or token — **the invitation-and-grant half of SC-015**: one entry for each of send, accept, decline, cancel, withdraw and lapse and for each of grant, change and end, asserted as a set over a fixture that performs all nine, and **zero** of the resulting rows containing a name, an email address, a note, a label or a clinical value
- [ ] T041 Implement the `OnRecordAfterCreateSuccess`/`…UpdateSuccess`/`…DeleteSuccess` hooks for both collections in `internal/platform/pb/hooks.go`, and extend `internal/service/audit/writer.go` with the five new actions and the `reason` field — every invitation sent, accepted, declined, cancelled, withdrawn and lapsed and every grant created, changed and ended, with `share_revoke`, `share_leave` and `share_expire` distinguishing an owner's revocation from a grantee's departure from a lapse, by opaque identifier and time (FR-068) — including the `audit.Reason` closed type and its `Valid()`, which the writer consults before every write so an undeclared token is a refused write rather than a stored one (depends on T040, T020)

### The route registry and the DTOs

- [ ] T042 Add the ten API routes and three page routes to `internal/httproute/routes.go`, each with its `operationId`, landmark and `SmokeURL` (the `/invite/{token}` entry substitutes the seeded token), and mark every patient-scoped route `PatientScoped: true`
- [ ] T043 [P] Write failing `encoding/json/v2` round-trip tests in `internal/web/api/dto_sharing_test.go` for `ShareSummary`, `ResourceRef`, `AccountRef`, `InvitationSummary`, `InviteeRef`, `InvitationPreview` and `Notice`: slices marshal as `[]` never `null`, unknown fields are rejected, duplicate keys are rejected, and **a reflection test asserts `InvitationPreview` and `Notice` have no field capable of carrying a patient name or a clinical value** (FR-023, FR-065)
- [ ] T044 Implement `internal/web/api/dto_sharing.go` — `AccountRef` carrying a display name and, for the sender's own view, the invited address, and no other account attribute anywhere in the phase (FR-062) (depends on T043)
- [ ] T045 Add `forbidden_view_only` and `forbidden_owner_only` to the error-mapping table in `internal/web/errors.go` with a test asserting each is producible **only** from a path that already resolved a `Grant`

**Checkpoint**: both collections exist, the domain is complete and exhaustively tested, the single
checkpoint is widened, the hub carries user-addressed events, and mail is sendable and assertable.
**User stories may now start, and may run in parallel.**

---

## Phase 3: User Story 1 — Let somebody I trust see a person's records (Priority: P1) 🎯 MVP

**Goal**: one person, one recipient, one level, one acceptance — and every screen of that chart
opening for the recipient exactly as it does for the owner.

**Independent Test**: with two seeded accounts, invite the second by email to view one person's
chart, accept as the second, and confirm the person appears marked as shared, that every screen of
that chart opens with identical content, that no other person or record of the first account is
reachable, and that a third account can reach none of it.

### Tests for User Story 1 ⚠️ write first, confirm red

- [ ] T046 [P] [US1] Failing unit tests in `internal/service/share/service_test.go` for `Invite`: `PermOwn` required on every resource id; a self-invitation is `ErrSelfShare`; an existing active grant is `ErrAlreadyShared`; an existing pending invitation is `ErrInvitationOutstanding`; the default lapse is now+`InvitationTTL`; the token is minted once and only its hash is stored. Sharing with the account that owns the thing is refused (FR-011) and a second active grant of the same thing to the same account is impossible (FR-010) (US1 scenario 1, FR-010, FR-011, FR-016, FR-019, FR-020, FR-021)
- [ ] T047 [P] [US1] Failing unit tests in `internal/service/share/invitations_test.go` for `Respond(accepted)`: grants are created for every resource, `note` is **copied** onto each (research D-16), `recipient` is resolved, `status`/`responded_at` are set, and the transaction is atomic. Acceptance is the **only** path that creates a grant (FR-013) and the copied note is what the grantee is shown for as long as the grant lasts (FR-009) (US1 scenario 3, FR-009, FR-013, FR-028)
- [ ] T048 [P] [US1] Failing HTTP tests in `internal/web/api/shares_http_test.go` for `POST /api/v1/shares` (op 58): `201` + `Location: /api/v1/invitations/{id}`; the six-actor matrix; **and the enumeration-safety pair — with SMTP enabled the responses for a known and an unknown address are byte-identical apart from the id; with SMTP disabled they differ exactly as research D-06 specifies and in no other way** (FR-018, FR-022, SC-011)
- [ ] T049 [P] [US1] Failing HTTP tests in `internal/web/api/shares_http_test.go` for `GET /api/v1/shares?direction=granted` (op 59): the owner sees every account with access, the level, since when, until when, the note and the invitation provenance, and nothing about the grantee beyond its display name and the address that was invited (FR-062); another account sees none of it; a missing `direction` is `400` (US1 scenario 7, FR-035)
- [ ] T050 [P] [US1] Failing HTTP tests in `internal/web/api/invitations_http_test.go` for `GET /api/v1/invitations?direction=received` (op 63) and `POST /api/v1/invitations/{id}/respond` (op 65, accept path): the recipient sees sender name, kind, count, level, lapse date and note **and nothing identifying the person and no clinical content** — everything shared with them and nothing more (FR-009, FR-036), each row carrying its state and the actions the caller's role in it permits (FR-041), and nothing about the sender beyond a display name (FR-062); accepting creates the grants and returns them (US1 scenarios 2–3, FR-023)
- [ ] T051 [P] [US1] Failing tests in `internal/web/api/authz_matrix_test.go` — the six-actor matrix table **derived from the route registry**, applied to every route marked `PatientScoped: true`, proving a `view` grantee reads every one identically to the owner — the chart, every kind of record, lab results, documents and their contents, search, the timeline, the status views and the live views, and nothing else of the owner's (FR-003, FR-056), a grantee's search returning only that person's records and a search naming an unreachable person answered as though they did not exist (FR-057) — and a stranger is refused with a response byte-identical to a non-existent id, an unauthenticated caller refused with nothing about the thing in the refusal (FR-063) (US1 scenarios 4 and 6, FR-003, FR-056, FR-057, FR-063, FR-077, SC-003, SC-004)
- [ ] T052 [P] [US1] Failing tests in `internal/service/access/coverage_test.go` — the build gate: every registry route marked `PatientScoped: true` has an entry in the T051 matrix table; a route without one fails the build. This is what makes FR-054's "anywhere in the application" and FR-056's "every list, chart, search, timeline, status view, document and live view" mechanically true rather than asserted (FR-054, FR-056, FR-077)
- [ ] T053 [P] [US1] Failing tests in `internal/web/api/patients_test.go` for the widened `GET /api/v1/patients`: owned ∪ shared in one page, the two groups contiguous and visibly distinguished, each shared row carrying `shared_by` and `level`, and `owned_count`/`shared_count` in the envelope (US1 scenario 5, FR-055)
- [ ] T054 [P] [US1] Failing tests in `internal/service/audit/writer_test.go` and `internal/web/api/records_http_test.go` for `read_sensitive`, one subtest per row of [`contracts/widened-authorization.md`](./contracts/widened-authorization.md) §"Where `read_sensitive` is written": opening a record, and retrieving a document's content **or a preview**, as a grantee or as a superuser reading somebody else's data writes exactly one row naming actor, action, kind, opaque id and patient — and **no diagnosis, medication, measurement, note or document name**; **an owner's own read writes no row at all, at any privilege level**, asserted by counting rows before and after; list paging writes nothing (US1 scenario 9, FR-070, FR-071, SC-015's reading half, [D-25](./research.md#d-25))
- [ ] T055 [P] [US1] Failing templ render tests in `internal/web/views/sharing/sharing_templ_test.go` and `internal/web/views/invitations/invitations_templ_test.go`: the granted panel, the received panel and the empty state each render inside their landmark, and the empty state carries a title, a body and an action (US1 scenario 8, FR-040)

### Implementation for User Story 1

- [ ] T056 [US1] Implement `internal/service/share/service.go` — `Invite` and `ListShares`, with `TokenMinter` producing 32 `crypto/rand` bytes base64url-encoded and stored as hex SHA-256 (research D-15) (depends on T046, T049)
- [ ] T057 [US1] Implement `internal/service/share/invitations.go` — `ListInvitations` and `Respond`'s accept path inside `app.RunInTransaction`, the only code path in the application that creates a grant (FR-013), re-checking ownership per resource at accept time (research D-10) (depends on T047, T050)
- [ ] T058 [US1] Implement `internal/web/api/shares.go` — ops 58 and 59 per `contracts/shares.md` (depends on T048, T049, T056)
- [ ] T059 [US1] Implement `internal/web/api/invitations.go` — ops 63 and 65 per `contracts/invitations.md` (depends on T050, T057)
- [ ] T060 [P] [US1] Extend `internal/service/patient/service.go` and `internal/store/patient/repo.go` with `ListAccessible` (owner ∪ grants) and its keyset cursor per research D-24, and update `internal/web/api/patients.go` (depends on T053)
- [ ] T061 [P] [US1] Add the `read_sensitive` call to `internal/web/api/records.go` and `internal/web/api/attachments.go`, fired when — and only when — the resolved grant is not `PermOwn`, which includes a superuser reading somebody else's data and excludes a superuser reading a patient they own; the ownership outcome comes from the authorizer's result and is never re-derived from the request. Amend phase 004's `internal/service/attachment/serve.go` to the same single condition, replacing its unconditional write (004 T105) (depends on T054)
- [ ] T062 [US1] Implement `internal/web/page/sharing.go` and the templ components in `internal/web/views/sharing/{granted.templ,received.templ,row.templ}` plus the deterministic ids in `internal/web/views/ids/ids.go` (depends on T055)
- [ ] T063 [US1] Implement `internal/web/page/invitations.go` and `internal/web/views/invitations/{received.templ,sent.templ,respond.templ}` (depends on T055)
- [ ] T064 [P] [US1] Implement `internal/web/views/sharing/sharedrawer.templ` and wire it into `internal/web/views/patients/detail.templ` as a **Datastar signal, not a route**
- [ ] T065 [P] [US1] Update `internal/web/views/patients/list.templ` — owned and shared groups, counts, and a "shared by X (view)" badge on each shared row (depends on T060)
- [ ] T066 [P] [US1] Add `e2e/specs/sharing.spec.ts` and `e2e/specs/invitations.spec.ts` — both pages at 1440×900 and 390×844, populated **and** as `empty@medikube.local`, asserting the landmarks, `body[data-signals]`, and zero console/page/network errors

**Checkpoint**: an owner can share a chart, a recipient can accept it and read everything, and
nothing else of the owner's is reachable. This is the MVP and it is demonstrable on its own.

---

## Phase 4: User Story 2 — Take access away, and be certain it is gone (Priority: P2)

**Goal**: revocation that bites on the very next request, an open screen that stops and says why,
and an end date that needs nobody to remember it.

**Independent Test**: grant access, confirm the chart reads, end the access from the owner's side,
and confirm every route to that person is refused as though they never existed — without a sign-out.
Repeat for the grantee leaving, and for a grant whose end date passes.

### Tests for User Story 2 ⚠️ write first, confirm red

- [ ] T067 [P] [US2] Failing unit tests in `internal/service/share/service_test.go` for `Revoke` and `Leave`: `Revoke` requires `PermOwn` and sets `revoked_by = grantor`; `Leave` requires the caller to be the grantee and sets `revoked_by = grantee`; a grantor calling `Leave` and a grantee calling `Revoke` are both `ErrNotFound`; both are idempotent (US2 scenarios 1 and 4, FR-038, FR-039)
- [ ] T068 [P] [US2] Failing HTTP tests in `internal/web/api/shares_http_test.go` for ops 61 and 62 — `204`, the matrix, and the owner's list showing `ended_by: "grantee"` after a leave versus `"owner"` after a revoke
- [ ] T069 [P] [US2] Failing tests in `internal/web/api/revocation_immediacy_test.go` (FR-042, SC-005): with a session token issued **before** the revoke, the grantee's very next request on **every** `PatientScoped` route is `404` — no sign-out, no cron, no cache flush — driven by the same registry-derived table as T051 — after access ends the thing is unreachable by every route and every attempt is answered exactly as though it did not exist (FR-044)
- [ ] T070 [P] [US2] Failing tests in `internal/service/access/authorizer_test.go` for lapse immediacy (FR-043, SC-004): with the injected clock advanced past `expires_at` and **no tidy pass run**, every route is refused, and the grant is reported as lapsed to both sides by op 59
- [ ] T071 [P] [US2] Failing tests in `internal/web/stream/records_test.go` (FR-045, US2 scenario 2): with a stream open on a shared patient, revoking the grant produces exactly one patch containing the "access has ended" panel and **no clinical content**, then closes the stream; and the keepalive tick alone also cuts the stream when the hub event is dropped
- [ ] T072 [P] [US2] Failing tests in `internal/service/patient/active_test.go` (FR-046, US2 scenario 3): when the grant on the active patient ends, the pointer resolves to null and a page request lands on `/patients`, never an error
- [ ] T073 [P] [US2] Failing tests in `internal/web/api/records_http_test.go` (FR-047, US2 scenarios 6–7): after a revoke, every record, correction and attachment the former grantee created is present and unchanged; and a save from a form loaded before the revoke is refused as though the record did not exist
- [ ] T074 [P] [US2] Failing tests in `internal/service/share/service_test.go` (US2 scenario 8): after a revoke, re-sharing requires a **fresh** invitation — no code path resurrects the old grant, proved by the unique-active index accepting a second row only after the first is revoked

### Implementation for User Story 2

- [ ] T075 [US2] Implement `Revoke` and `Leave` in `internal/service/share/service.go`, publishing `Event{Type: AccessChanged, UserID: grantee}` to the hub (depends on T067)
- [ ] T076 [US2] Implement ops 61 and 62 in `internal/web/api/shares.go` per `contracts/shares.md` (depends on T068, T075)
- [ ] T077 [US2] Extend `internal/web/stream/records.go` — subscribe to the subscriber's user topic, re-authorise on `AccessChanged`, patch `sharing/accessended.templ` and close; re-authorise on the keepalive tick as the guarantee path (depends on T071)
- [ ] T078 [P] [US2] Implement `internal/web/views/sharing/accessended.templ` with a render test asserting it contains no clinical content and offers a link back to `/patients`
- [ ] T079 [P] [US2] Update `internal/service/patient/active.go` so a lost grant resolves the active patient to null (depends on T072)
- [ ] T080 [P] [US2] Add the revoke and leave controls with confirmation to `internal/web/views/sharing/{row.templ,revokeconfirm.templ}`

**Checkpoint**: access can be ended by either side or by a date, is gone on the next request, and
destroys nothing.

---

## Phase 5: User Story 3 — Let a carer keep the record up to date (Priority: P3)

**Goal**: `edit` that is exactly the owner's write surface minus identity, minus deletion of the
person, minus access management, minus onward sharing.

**Independent Test**: grant one account `view` and another `edit` on the same person. Confirm the
editor can create, correct and delete records of every type and manage documents; that every write
by the viewer is refused with an explanation; and that neither can delete the person, change
anybody's access or share it onward.

### Tests for User Story 3 ⚠️ write first, confirm red

- [ ] T081 [P] [US3] Failing tests in `internal/web/api/authz_matrix_test.go` extending T051's table with the **write** rows: an `edit` grantee succeeds on create, update and delete of every registered kind under exactly the owner's validation, `If-Match` and confirmation rules (US3 scenario 1, FR-004)
- [ ] T082 [P] [US3] Failing tests in the same table for the `view` grantee's writes: **`403 forbidden_view_only`** on every write route, with a message naming their level, and **nothing changed** — asserted by re-reading the record (US3 scenario 2, FR-058, SC-006)
- [ ] T083 [P] [US3] Failing tests in `internal/web/api/owner_only_test.go`: a grantee at **either** level attempting to delete the patient, change the identifying profile, change the photo, alter anybody's access, or share the person onward gets `403 forbidden_owner_only`, and none of it changes anything (US3 scenario 3, FR-005, FR-006, SC-007)
- [ ] T084 [P] [US3] Failing tests in `internal/service/share/service_test.go` and `internal/web/api/shares_http_test.go` for `ChangeShare` (op 60): raising `view` → `edit` takes effect on the grantee's **next action with no new invitation**, lowering `edit` → `view` refuses the next write and undoes nothing, and both sides see the new values (US3 scenarios 4–5, FR-037)
- [ ] T085 [P] [US3] Failing tests in `internal/web/api/records_http_test.go` (US3 scenario 6): an editor and the owner with the same record open — the second save is `412 version_mismatch` and the current values are returned, exactly as for one person editing in two places
- [ ] T086 [P] [US3] Failing tests in `internal/platform/pb/hooks_test.go` (US3 scenario 7): a deletion by an editor produces an audit row attributed to **the editor's** account, not the owner's
- [ ] T087 [P] [US3] Failing tests in `internal/service/tag/service_test.go` and `internal/web/api/tags_http_test.go` (FR-059, research D-22): a grantee sees the owner's tags **on the records they may see**; `GET /api/v1/tags` returns only the grantee's own vocabulary; an editor may apply and remove only ids already in the **owner's** set and an id outside it is `422 unknown_tag` identical for "not yours" and "does not exist"; creating, renaming, recolouring and deleting a tag in the owner's set is refused
- [ ] T088 [P] [US3] Failing tests in `internal/web/api/practitioners_http_test.go` and `facilities_http_test.go` (FR-060, research D-23): a grantee reads a practitioner's name embedded in a shared record, **and** gets `404` from `/api/v1/practitioners/{id}` for that same practitioner and an empty list from `/api/v1/practitioners`
- [ ] T089 [P] [US3] Failing tests in `internal/web/api/attachments_http_test.go` (FR-061): a `view` grantee opens and downloads documents; an `edit` grantee adds, corrects and removes them under the same recoverable-deletion rules; a `view` grantee's write is `403 forbidden_view_only`

### Implementation for User Story 3

- [ ] T090 [US3] Implement `ChangeShare` in `internal/service/share/service.go` and op 60 in `internal/web/api/shares.go`, publishing an `AccessChanged` notice (depends on T084)
- [ ] T091 [P] [US3] Implement the owner-only guards by permission — `PermOwn` on `PATCH`/`DELETE /api/v1/patients/{id}`, the photo write routes, and every sharing mutation — in `internal/web/api/patients.go` and `internal/web/api/patient_photo.go` (depends on T083)
- [ ] T092 [P] [US3] Implement the shared-record tag rules in `internal/service/tag/service.go` — resolve a shared record's labels as stored, validate submitted ids against the patient owner's set, keep vocabularies private (depends on T087)
- [ ] T093 [P] [US3] Update `internal/web/views/records/*.templ` and `internal/web/views/patients/detail.templ` so write controls are **absent** at `view` level, and a forced write's `forbidden_view_only` message lands in `#error-banner`
- [ ] T094 [P] [US3] Implement `internal/web/views/sharing/leveldialog.templ` — change level and end date from the sharing screen

**Checkpoint**: two levels behave exactly as specified, and the owner keeps identity, deletion and
access management.

---

## Phase 6: User Story 4 — Share what runs in the family, and only that (Priority: P4)

**Goal**: one relative's entry and its conditions, and demonstrably nothing else.

**Independent Test**: record a relative with conditions, share that one relative with a second
account, and confirm it reads exactly that entry, cannot reach the person it is filed against, their
records or any other relative, cannot change anything, and sees the sender's note; then end the
share and confirm it becomes unreachable.

### Tests for User Story 4 ⚠️ write first, confirm red

- [ ] T095 [P] [US4] Failing HTTP tests in `internal/web/api/familyshare_http_test.go` (US4 scenarios 1, 4, 5): a `family_member` grantee reads name, relationship, sex, birth and death years, deceased flag and every condition with its name, code, age at diagnosis, severity, status and notes, plus the sender's display name and note; the owner's corrections are visible; the grantee's every write is refused (FR-049, FR-050, FR-051)
- [ ] T096 [P] [US4] Failing **hostile** tests in `internal/web/api/familyshare_isolation_test.go` (FR-078, SC-008): from the grantee's session, every one of these is `404` — the patient the entry is filed against; that patient's records of **every registered kind, iterated from the kind registry**; that patient's other relatives; its attachments; a search naming it; the timeline; the status views; the owner's practitioners and facilities. Exactly **one** relative is reachable per grant (US4 scenario 2)
- [ ] T097 [P] [US4] Failing tests in `internal/domain/share/invitation_test.go` and `internal/web/api/shares_http_test.go` (US4 scenario 3, FR-007): a family-history invitation or grant at `edit` is refused with `422 family_history_view_only` at **both** the API and the domain boundary
- [ ] T098 [P] [US4] Failing tests in `internal/service/share/invitations_test.go` (US4 scenario 6, FR-015, FR-052): one invitation covering several things of one kind, all owned by the sender, is accepted or declined as a whole — four relatives produce four **separately listed** grants, each endable on its own
- [ ] T099 [P] [US4] Failing integration tests in `internal/store/share/repo_test.go` (US4 scenario 7, FR-048, FR-053): deleting a relative cascades away every grant of it, deleting an account ends every grant it gave and received, and every other grant held by the same accounts is untouched

### Implementation for User Story 4

- [ ] T100 [US4] Implement `resource_kind = family_member` end to end in `internal/service/share/service.go` and `internal/store/share/query.go` — the level ceiling, the `family_member` relation column and its cascade. It reuses the one mechanism: the same invitation, the same list, the same ending and the same trail, with no parallel family-history path anywhere (FR-001) (depends on T095, T097, T099)
- [ ] T101 [P] [US4] Extend `internal/service/access/authorizer.go` so `Record(kind.FamilyMember, id, need)` resolves a family-history grant **without** granting anything about the relative's patient (depends on T096)
- [ ] T102 [P] [US4] Implement the shared-relative view in `internal/web/views/records/familymember.templ` — the sender's display name and note shown alongside for as long as the grant lasts, and no path to the patient
- [ ] T103 [P] [US4] Extend the share drawer in `internal/web/views/sharing/sharedrawer.templ` so a relative can be shared from the family-history page, with the level fixed to viewing and stated as such

**Checkpoint**: a pedigree can be exchanged without exposing a living person's chart, and the
isolation is proved hostilely.

---

## Phase 7: User Story 5 — Run the invitations, including to somebody with no account yet (Priority: P5)

**Goal**: the whole lifecycle around the happy path — a stranger, a cancel, a decline, a lapse, a
withdrawal, and the refusals that keep it coherent.

**Independent Test**: send four invitations — one to an address with no account, one cancelled, one
declined with a note, one whose lapse date passes during the test — and confirm the first is
acceptable after sign-up, the cancelled link stops working, the declined note reaches the sender,
the lapsed one cannot be answered, and all four show the right state on both sides.

### Tests for User Story 5 ⚠️ write first, confirm red

- [ ] T104 [P] [US5] Failing HTTP tests in `internal/web/api/invitations_http_test.go` for `GET /api/v1/invitations/token/{token}` (op 64, US5 scenario 1): the public preview returns sender display name, kind, count, level, lapse date, note and a masked address hint — **and `404` for an unknown, answered, cancelled, withdrawn or lapsed token, one response, no distinction** (FR-023, FR-024, SC-012)
- [ ] T105 [P] [US5] Failing tests in `internal/web/api/invitations_http_test.go` (US5 scenarios 2–3): an invitation sent to an unregistered address is waiting after that address registers and signs in; and answering while signed in as a **different** address is `404`, disclosing nothing about who it was for or what it covered (FR-014, FR-025, SC-009)
- [ ] T106 [P] [US5] Failing tests for op 66 in `internal/web/api/invitations_http_test.go` (US5 scenarios 4 and 8): cancelling a `pending` invitation kills the link immediately and creates no grant; withdrawing an `accepted` one revokes **every** grant it created in one transaction and both sides see it as withdrawn (FR-030, FR-031)
- [ ] T107 [P] [US5] Failing tests in `internal/service/share/invitations_test.go` (US5 scenarios 5–7): decline with a note records the note and the time and creates nothing; a lapsed invitation is refused for **both** sides with `ErrLapsed` before the transition table is consulted; a second answer is `409` naming the state and `responded_at` (FR-027, FR-029, FR-032)
- [ ] T108 [P] [US5] Failing tests in `internal/service/share/invitations_test.go` (US5 scenario 9, FR-028, SC-013): an invitation covering three resources where one has been deleted fails **as a whole** — zero grants exist afterwards, the invitation reaches a terminal state, and the response is `410 resources_unavailable` naming **nothing**
- [ ] T109 [P] [US5] Failing tests in `internal/web/api/shares_http_test.go` (US5 scenario 10, FR-019): inviting an address that already holds active access to the same thing is `409 already_shared` and the message directs the sender to change the existing access
- [ ] T110 [P] [US5] Failing concurrency tests in `internal/service/share/invitations_test.go`: two simultaneous accepts of one invitation leave exactly one set of grants and the loser gets `409`; an accept racing a revoke ends in one of the two consistent outcomes with **one audit row per event** and never a half-applied state (edge cases, research D-11)
- [ ] T111 [P] [US5] Failing tests in `internal/service/share/tidy_test.go` (FR-033, research D-19): `Tidy` writes one `share_expire` per newly-observed lapsed grant and is idempotent on a second run; moves lapsed `pending` invitations to `expired`, writing one `invite_expire` each; deletes terminal invitations past the retention window while their audit rows survive; and **nothing in the read path changes behaviour whether or not it has run** (FR-034). **Plus the run-id case**: `Tidy` is called on a bare background context — the only kind it ever gets — and every `share_expire` and `invite_expire` row it writes carries a **non-empty `request_id` equal to that run's `run_id`**, identical across one run's rows and different between two runs, so the `Required` column is satisfied and the rows correlate to the run's log lines (001 [data-model](../001-walking-skeleton/data-model.md) §3)
- [ ] T112 [P] [US5] Failing templ render tests in `internal/web/views/invitations/landing_templ_test.go`: the public landing page renders the preview, offers sign-in and sign-up with the invited address, shows the "not addressed to this account" panel when signed in as someone else — and contains **no** patient name, asserted against a seeded sentinel

### Implementation for User Story 5

- [ ] T113 [US5] Implement `Preview`, `Cancel` and `Withdraw` in `internal/service/share/invitations.go`, and the lapse guard ahead of `Transition` (depends on T104, T106, T107)
- [ ] T114 [US5] Implement ops 64 and 66 in `internal/web/api/invitations.go`, with rate limiting on the public preview (depends on T104, T106, T113)
- [ ] T115 [US5] Implement the all-or-nothing accept failure path returning `410 resources_unavailable` in `internal/service/share/invitations.go` and `internal/web/api/invitations.go` — an invitation is answered as a whole or not at all (FR-015) (depends on T108)
- [ ] T116 [US5] Implement `internal/service/share/tidy.go` and expose it as `medikube purge --sharing` in `internal/cli/purge.go`, documenting in the file that **phase 006 owns its schedule**; `Tidy` derives one `run_id` per run onto its context so its `system` audit rows fill the `Required` `request_id` and correlate to the run's log lines, by both entry points (depends on T111)
- [ ] T117 [US5] Implement `internal/web/page/invite.go` (public) and `internal/web/views/invitations/{landing.templ,preview.templ}`, redirecting with a `303` issued before any stream is opened (depends on T112)
- [ ] T118 [P] [US5] Add `e2e/specs/invite-landing.spec.ts` — the seeded token at both viewports, a lapsed token rendering the shared 404 view, and a sentinel assertion that the seeded patient's name appears nowhere on the page
- [ ] T119 [P] [US5] Add the cancel, withdraw and decline-with-note controls to `internal/web/views/invitations/{sent.templ,respond.templ}` — the sent list and the received list each showing every invitation's state and offering only the actions that caller's role in it allows (FR-041)

**Checkpoint**: the invitation lifecycle is complete and coherent from both sides, including for
somebody who has never used the instance.

---

## Phase 8: User Story 6 — Know when something changes without watching for it (Priority: P6)

**Goal**: contentless notices that arrive without a refresh, and never to somebody who has lost the
entitlement they concern.

**Independent Test**: with two accounts signed in at once, send an invitation and confirm a notice
reaches the recipient without a refresh; accept and confirm the sender is notified; end the access
and confirm the former grantee is told. Leave a session open for an hour and confirm notices still
arrive. Confirm no notice contains a person's name or any clinical detail.

### Tests for User Story 6 ⚠️ write first, confirm red

- [ ] T120 [P] [US6] Failing tests in `internal/web/stream/notifications_test.go` (US6 scenarios 1–3, FR-064): an invitation, an answer, a grant, a level change and a revoke each produce exactly one notice to the correct account, naming the other account's display name and the kind of event
- [ ] T121 [P] [US6] Failing tests in the same file (US6 scenarios 5–6, FR-065, FR-066): an event for an account that lost its entitlement between publish and delivery is **dropped at delivery**; an event addressed to A is never delivered to B; and a rendered notice contains **no** patient name, diagnosis, medication, measurement or document name, asserted against seeded sentinels
- [ ] T122 [P] [US6] Failing tests in `internal/web/stream/notifications_test.go` (FR-067): every behaviour in this phase is reachable by reloading a page with the stream never opened — nothing depends on a notice arriving
- [ ] T123 [P] [US6] Failing build-tagged test `internal/web/stream/notifications_slow_test.go` (`//go:build slowsse`, US6 scenario 4, SC-017): the stream is still delivering after **6 minutes**, proving PocketBase's hardcoded 5-minute `WriteTimeout` is overridden, plus a goroutine-leak assertion on context cancellation
- [ ] T124 [P] [US6] Failing templ render test in `internal/web/views/shell/notice_templ_test.go`: the notice renders as the contents of phase 001's `#toast` region — which is already `role="status"` and `aria-live="polite"`, so this component asserts it does **not** re-declare either — and emits no text node beyond the display name, the event kind and the level

### Implementation for User Story 6

- [ ] T125 [US6] Implement `internal/web/stream/notifications.go` (op 67) through phase 001's `newStream()` helper, re-authorising per event before rendering (depends on T120, T121, T123)
- [ ] T126 [US6] Implement `internal/service/share/notifier` wiring — every service mutation publishes `Event{Type: Notice, UserID, …}` carrying ids and kinds only (depends on T120)
- [ ] T127 [P] [US6] Implement `internal/web/views/shell/notice.templ`, patched into phase 001's existing `#toast` region — which already sits outside every landmark and every patch target. **This phase makes no edit to `internal/web/views/shell/layout.templ` at all**: a second polite live region beside `#toast` is not a second channel, it is two announcers competing for one screen reader (`contracts/pages.md` §2) (depends on T124)
- [ ] T128 [P] [US6] Add the unanswered-invitation badge to the primary nav in `internal/web/views/shell/layout.templ`, driven by a `PatchSignals` from the notice stream
- [ ] T129 [P] [US6] Add `e2e/specs/sharing-live.spec.ts` — two browser contexts: a notice arrives with no refresh within 5 s, and a revoke empties an open shared list and states that access ended within 5 s (SC-005, SC-017)

**Checkpoint**: collaboration feels like collaboration, and nothing depends on it.

---

## Phase 9: Polish & Cross-Cutting Concerns

- [ ] T130 [P] Extend `internal/testsupport/phileak/exercise.go` with four new sentinel classes — email address, note, display name, invitation token — and drive **every operation this phase defines**, asserting zero occurrences in the zerolog stream, the Prometheus registry, the OTel span recorder and the Sentry transport (FR-080, SC-016)
- [ ] T130a [P] Extend phase 002's egress harness — `internal/testsupport/netgate/` **[EDIT]** — to run this phase's whole endpoint exercise under the `net.Dialer` control hook: with SMTP unconfigured **zero** outbound connections are made, and with it configured the **only** outbound connection is the invitation message to the invited address (FR-073)
- [ ] T131 [P] Extend `internal/testsupport/scale/generate.go` with the SC-014 fixture (one owner: 20 patients, 200 active grants across 10 grantees; one grantee: 50 shared patients across 12 grantors, mixed levels and expiries) and add `internal/web/api/scale_test.go` (`//go:build scale`) asserting both sharing panels, both invitation lists and `/patients` render inside 2 s, and that a paging walk with concurrent grant and revoke repeats and skips **0** rows (FR-074, FR-075)
- [ ] T132 [P] Add the `medikube_shares_*`, `medikube_invitations_*` and `medikube_access_denied_total` collectors in `internal/obs/metrics.go` with a test asserting every label value comes from a **bounded** set and that no address, user id, patient id or note can become one
- [ ] T133 [P] Add `service.share.*` and `store.shares.*` spans in `internal/service/share` and `internal/store/share` with an allowlisted attribute set, and a test asserting no free-text attribute is emitted
- [ ] T134 [P] Add the `access_denied` audit row with its `reason` (`no_grant`, `revoked`, `expired`, `view_only`, `not_addressed_to_you`, `disabled`) to `internal/service/access/authorizer.go`, with a test per reason (FR-069) — **SC-015's refusal half**: every refused attempt produces exactly one entry, and none of them carries a name, an address, a note, a label or a clinical value
- [ ] T135 [P] Add `e2e/specs/sharing-keyboard.spec.ts` — SC-020: send an invitation, answer one, change a level and end access using only the keyboard at both viewports, with a visible focus assertion at every step
- [ ] T136 Extend `e2e/routes.gate.spec.ts` for the three new page routes with the seeded token substituted into `/invite/{token}`, and re-run the full phases 001–004 smoke suite unchanged to prove this phase added no landmark and no live region, and broke no existing landmark assertion (FR-079, SC-019)
- [ ] T137 Regenerate and commit `api/openapi.json` via `task openapi`, then confirm `git diff --exit-code api/openapi.json` is clean and `internal/openapi/gate_test.go` passes with all ten new `operationId`s present in both the registry and the document (Principle IX)
- [ ] T138 [P] Add the PocketBase-upgrade checklist entries to the phase-001 checklist file: re-verify `core/base.go:713` (mailer substitution) and `core/field_date.go:110` (`TEXT DEFAULT '' NOT NULL`) on every PocketBase upgrade — the second silently changes the meaning of every predicate in this phase (research risk R8)
- [ ] T139 [P] Verify the monorepo integration is still complete: `/Users/krzysztof.wiatrzyk/private/monorepo/.dockerignore` readmits `medikube/` and excludes `medikube/pb_data/`, `medikube/**/*_templ.go` and `medikube/internal/web/static/app.css`; `.github/workflows/build-image.yaml` lists `medikube`
- [ ] T140 Run `quickstart.md` end to end by hand — all six walkthroughs, including the SMTP-off refusal and the SMTP-on delivery — and correct anything that does not behave as written
- [ ] T140a Write `specs/005-sharing-and-collaboration/traceability.md` — the mechanical join, generated from `spec.md` and `tasks.md` rather than written by hand: one row per functional requirement (all 80) naming the task ids that satisfy it and the named test that proves it, one row per acceptance scenario naming its test, and one row per success criterion (all 20) naming its task or its exit criterion. **A functional requirement with no task, or a success criterion that is neither mapped nor marked `[outcome metric]` in `spec.md`, fails the phase** (cross-artifact finding M7)
- [ ] T141 Final gate run: `task check`, `task openapi && git diff --exit-code api/openapi.json`, `task routes`, `task test:e2e`, `task test:phileak`, `go test -tags slowsse ./internal/web/stream/...`, `go test -tags scale ./...`, and the container build

---

## Dependencies & Execution Order

### Phase dependencies

```
Phase 1 Setup  ──▶  Phase 2 Foundational  ──┬──▶ Phase 3  US1 (P1)  🎯 MVP
                                            ├──▶ Phase 4  US2 (P2)
                                            ├──▶ Phase 5  US3 (P3)
                                            ├──▶ Phase 6  US4 (P4)
                                            ├──▶ Phase 7  US5 (P5)
                                            └──▶ Phase 8  US6 (P6)
                                                       └──▶ Phase 9 Polish
```

- **Setup (Phase 1)**: no dependencies.
- **Foundational (Phase 2)**: depends on Setup. **Blocks every user story** — each story reads a
  grant, and the widened `access.Authorizer` (T028–T031) is what makes a grant readable.
- **User stories (Phases 3–8)**: all depend on Foundational and are then independent of each other,
  with the honest caveats below.
- **Polish (Phase 9)**: depends on the stories being complete; T130, T131 and T136 exercise all of
  them.

### Cross-story caveats (stated rather than pretended away)

- **US2's revocation-immediacy suite (T069)** needs a grant to revoke, so it uses the seeded
  `viewer@medikube.local` fixture rather than US1's code path. It is independently runnable.
- **US3's write matrix (T081–T082)** extends the same registry-derived table US1 creates in T051.
  If US3 is built first, T051's table must be created there instead — the table is the shared
  artefact, not the story.
- **US5's tidy pass (T111, T116)** touches nothing on the read path by design, so it can land in any
  order.
- **US6 depends on nothing** and nothing depends on it (FR-067). It is genuinely last.

### Within each story

Tests are written and confirmed **red** before the implementation they cover. Domain before service,
service before handler, handler before page, page before browser spec.

### Parallel opportunities

- **Phase 1**: T002–T006 all `[P]`.
- **Phase 2**: the domain tests T008–T013 run together; the two migration test/impl pairs
  (T016/T017 and T018/T019) are sequential within a pair and parallel across; T024–T027, T034,
  T036, T038, T040 and T043 all `[P]`.
- **Within any story**: every test task is `[P]` with its siblings — they are separate files.
- **Across stories**: once Phase 2 is done, six developers can take one story each.

### Parallel example — User Story 1

```bash
# Launch the failing tests together (all different files):
Task: "T046 unit tests for Invite in internal/service/share/service_test.go"
Task: "T047 unit tests for Respond(accepted) in internal/service/share/invitations_test.go"
Task: "T048 HTTP tests for POST /api/v1/shares in internal/web/api/shares_http_test.go"
Task: "T051 the registry-derived six-actor matrix in internal/web/api/authz_matrix_test.go"
Task: "T053 widened GET /api/v1/patients in internal/web/api/patients_test.go"
Task: "T054 read_sensitive audit in internal/service/audit/writer_test.go"
Task: "T055 templ render tests in internal/web/views/sharing/sharing_templ_test.go"

# Then the implementation, respecting T056 → T058 and T057 → T059:
Task: "T060 ListAccessible in internal/store/patient/repo.go"
Task: "T064 the share drawer in internal/web/views/sharing/sharedrawer.templ"
Task: "T065 the patients list badges in internal/web/views/patients/list.templ"
```

---

## Implementation Strategy

### MVP first (User Story 1 only)

1. Phase 1 Setup → 2. Phase 2 Foundational → 3. Phase 3 US1 → **stop and validate**: run
   `quickstart.md` §3 by hand. At that point a family can already share the burden of caring for
   somebody, which is exactly what the story says it delivers.

### Incremental delivery

US1 (share) → US2 (revoke, and the phase is now safe to ship) → US3 (edit) → US4 (family history) →
US5 (the full lifecycle) → US6 (notices). **US2 is the one that must not be deferred**: sharing
without reliable revocation is worse than no sharing at all, which is the spec's own reasoning for
its priority.

### Notes

- `[P]` means different files and no incomplete dependency.
- Verify each test fails before implementing it. A test that was never red is a defect.
- Commit after each task or logical group, Conventional Commits scoped `medikube`.
- Never share a `tests.TestApp` across `ApiScenario` cases — it recurses until the stack overflows.
- Before writing any query in this phase, re-read [research D-01](./research.md#d-01).
