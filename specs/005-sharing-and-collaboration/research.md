# Research: Sharing and Collaboration (phase 005)

Every technical decision this phase makes, as Decision / Rationale / Alternatives considered.
Evidence is cited to the dossiers, and to [`VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) (abbreviated **VSF**) or to a
`file:line` in `github.com/pocketbase/pocketbase@v0.40.1` wherever it settles something. Where a
claim was not in a dossier it was read out of the module source and the file and line are given, so
a reviewer can re-check it in one command.

**No `NEEDS CLARIFICATION` survives.** Two of the specification's own requirements collide
([D-06](#d-06)) and one of the shared design contract's index definitions is inert as written
([D-01](#d-01)); both are resolved here rather than discovered in implementation.

---

## A. The traps that would have shipped silently

### D-01

**Decision — every predicate over an "empty" PocketBase column is `= ''`, never `IS NULL`, and a
build gate enforces it.**

`shares.revoked_at`, `shares.expires_at`, `shares.revoked_by`, `shares.invitation`,
`shares.patient`, `shares.family_member`, `invitations.recipient`, `invitations.responded_at` are
all "optional" fields. In every other database they would be nullable. In PocketBase they are not:

```
core/field_date.go:110      func (f *DateField) ColumnType(app App) string { return "TEXT DEFAULT '' NOT NULL" }
core/field_relation.go:156  func (f *RelationField) ColumnType(app App) string {
core/field_relation.go:158      if f.IsMultiple() { return "JSON DEFAULT '[]' NOT NULL" }
core/field_relation.go:161      return "TEXT DEFAULT '' NOT NULL"
```

There is **no NULL** in a PocketBase collection column. Consequences, all of them silent:

- The shared design contract's `UNIQUE (resource_kind, patient, family_member, grantee) WHERE
  revoked_at IS NULL` matches zero rows, so the index is a no-op and FR-010 ("at most one active
  grant of a given thing to a given account") is unenforced at the storage layer.
- An active-grant filter written `revoked_at IS NULL AND (expires_at IS NULL OR expires_at > :now)`
  returns **nothing at all**, so the first symptom is not a leak but a total outage — which is at
  least loud. The dangerous variant is `revoked_at IS NULL OR revoked_at > :now`, which returns
  **everything**, including revoked grants. That is a medical-record disclosure produced by a
  three-character habit.

So: the active predicate is, verbatim and in one place only
(`internal/store/share/query.go`):

```
revoked_at = '' AND (expires_at = '' OR expires_at > {:now})
```

and the unique index is `UNIQUE (resource_kind, patient, family_member, grantee) WHERE revoked_at = ''`.

**Rationale**: this is the single highest-consequence detail in the phase, it is invisible in
review, and it contradicts a binding document, so it is written down, indexed, tested and linted.
`internal/store/migrations/assertions_test.go` greps `internal/store/share` and
`internal/store/invitation` for `IS NULL` and fails on sight; `sharetest.RepositoryContract` proves
a revoked row and a lapsed row are both excluded and that a second active grant for the same
`(resource, grantee)` is rejected by the index rather than by a service check.

**Alternatives considered**:

- *Use PocketBase's filter DSL (`revoked_at = null`), which normalises this internally.* Rejected:
  the DSL never appears above `internal/store` (shared design §2.1 rule 7), and hiding the trap
  behind a DSL means the next person writing raw dbx hits it again.
- *Store `revoked_at` as a boolean `is_revoked` plus a timestamp.* Rejected: that is upstream's
  `is_active`, whose whole failure mode is that a lapsed share stays `is_active = true` until a cron
  runs (domain-platform §3.3). Two columns that can disagree about one fact.
- *Add a `CHECK` constraint.* PocketBase collections are managed schema; hand-added constraints are
  lost the moment the collection is saved from Go. The index and the test are the durable form.

### D-02

**Decision — a lapsed grant is refused by the read path, and no scheduled job is on the correctness
path.**

`access.Authorizer` resolves a grant by asking `ShareReader.ActiveFor(ctx, granteeID, resource,
now)`, whose query carries the D-01 predicate. `now` is supplied by an injected `Clock`, so a lapse
is tested by moving the clock, never by sleeping.

**Rationale**: FR-034 and FR-043 state it, and the system being reimagined got it wrong in exactly
this way — `POST /patient-sharing/cleanup-expired` "deactivates expired shares", which means an
expired share was honoured until the cron ran (domain-platform §3.2, §3.3). A predicate cannot be
late.

**Alternatives considered**:

- *A sweeper that flips `revoked_at` when `expires_at` passes.* Rejected: it makes correctness
  depend on a job, which FR-034 forbids by name. It survives only as tidy-up ([D-19](#d-19)).
- *Evaluate expiry in the service after loading the row.* Rejected: it works, but it moves a
  security predicate out of the query, so a future list endpoint that forgets the post-filter leaks.
  In the query, forgetting is impossible.

### D-03

**Decision — the invitation email is sent with `app.NewMailClient().Send(msg)` and nothing else.**

```
core/base.go:638   func (app *BaseApp) NewMailClient() mailer.Mailer
core/base.go:656   if h, ok := client.(mailer.SendInterceptor); ok { h.OnSend().Bind(... OnMailerSend ...) }
core/base.go:713   if client != ae.Mailer { return ae.Mailer.Send(e.Message) }
tests/app.go:500   t.OnMailerSend().Bind(... e.Mailer = t.TestMailer ...)   // Priority: -99999
```

Read together: any message sent through `NewMailClient()` passes through `OnMailerSend`, and the
test harness binds a handler that swaps the mailer — so `core/base.go:713` routes the message to
`TestApp.TestMailer` instead of a real server. Every invitation email is therefore **captured and
assertable in tests with no MediKube-owned seam at all**: `app.TestMailer.Messages()` gives the
`*mailer.Message` (`tools/mailer/mailer.go:12` — `From, To, Subject, HTML, Text, Headers`).

`internal/platform/mail.Mailer` implements the consumer-declared `share.Mailer` port, renders the
body with `internal/web/views/mail/invitation.templ`, sets `From` from
`Settings().Meta.SenderName`/`SenderAddress` (`core/settings_model.go:521-531`), and calls `Send`.

**Rationale**: Principle V forbids rebuilding what PocketBase provides, and the test harness is the
reason it is worth obeying here — a hand-rolled `net/smtp` client would need a fake SMTP server in
CI to assert FR-023's "the email names nobody".

**Alternatives considered**:

- *`net/smtp` directly with config from `MEDIKUBE_SMTP_*`.* Rejected: a second configuration
  mechanism for a subsystem PocketBase already configures, encrypted at rest, editable in the admin
  UI that ships in production.
- *Reuse `OnMailerRecord*Send` templates.* Rejected: those are bound to auth flows on an auth
  collection (password reset, verification, OTP); an invitation is not an auth record event.

### D-04

**Decision — the notice stream and the record stream are opened by phase 001's `newStream()`
helper, and a build-tagged test holds the notification stream open for more than five minutes.**

PocketBase's server sets a hardcoded 5-minute `WriteTimeout` that kills every long-lived SSE stream
and passes every test shorter than five minutes (VSF, constitution). Phase 001 owns the override on
the `ServeEvent`'s `*http.Server` and the `newStream()` helper (write deadline cleared,
`X-Accel-Buffering: no`, `Cache-Control: no-store`, `SkipSuccessActivityLog`). This phase adds a
second stream and therefore a second way to regress it.

**Rationale**: SC-017 requires a session left open for 60 continuous minutes still to receive
notices. Shared-design risk R7 assigns the long-running CI job to phase 006; this phase ships the
Go-level assertion (`//go:build slowsse`, ~6 minutes) so the regression is catchable now.

**Alternatives considered**:

- *Trust the phase-001 helper.* Rejected: the failure is silent, user-visible and reintroduced by
  any handler that opens a stream by hand.
- *Short-poll notices instead of streaming.* Rejected — see Complexity Tracking (a).

### D-05

**Decision — "outbound email is configured" means `app.Settings().SMTP.Enabled == true`, read as a
flag; it is never inferred from a send attempt.**

```
core/base.go:641   if app.Settings().SMTP.Enabled { client = &mailer.SMTPClient{...} }
core/base.go:652   } else { client = &mailer.Sendmail{} }
tools/mailer/sendmail.go:60   cmdPath, err := findSendmailPath()
tools/mailer/sendmail.go:85   sendmail := exec.Command(cmdPath, "-i", "-t")
```

With SMTP disabled, PocketBase falls back to exec'ing a local `sendmail` binary. MediKube's runtime
image is `gcr.io/distroless/static-debian12:nonroot` — no shell, no `sendmail`, so the call fails at
`exec`. Worse, on a developer's machine it might *succeed* and silently drop mail into a local
queue.

MediKube therefore exposes `share.Mailer.Configured(ctx) bool` returning `Settings().SMTP.Enabled`,
and:

1. warns loudly at boot when it is false — **this warning already exists**: phase 001 emits it for
   password recovery and address confirmation (001 FR-076), alongside the superuser-MFA and
   IP-allowlist warnings constitution VII requires. This phase **extends its text** to name
   invitations as a third affected feature; it does **not** add a second warning, because two boot
   warnings for one condition is how an operator learns to ignore both;
2. refuses an invitation to an address with no account, with `422 email_not_configured` and a plain
   explanation (FR-022);
3. still creates the invitation for an address that **does** have an account, delivering in-app,
   and tells the sender no email could be sent (FR-022);
4. surfaces the flag for `GET /api/v1/admin/system`, which **phase 006 builds** — this phase only
   guarantees the value is readable.

**Rationale**: shared-design risk R10 is this phase's to close, and the honest closure is a flag
read before the user presses the button, not an error discovered after.

**Alternatives considered**:

- *Send and report the failure.* Rejected: FR-022 requires refusing *up front*, and a `Sendmail`
  that succeeds into a black hole is worse than an error.
- *A `MEDIKUBE_MAIL_ENABLED` config flag.* Rejected: a second source of truth that can disagree with
  the setting that actually decides, editable in a second place.

---

## B. Where the specification collides with itself, and with the platform

### D-06

**Decision — FR-018's account-enumeration guarantee holds whenever outbound email is configured. In
the unconfigured state FR-022 wins, the difference becomes observable, and that is stated in the
contract, tested for, and confined by the boot warning.**

The collision, plainly:

- **FR-018 / SC-011**: a send must produce the same observable result whether or not the address has
  an account, so invitations cannot be used to enumerate accounts.
- **FR-022**: when outbound email is not configured, refuse an address with **no** account and
  explain why, but still deliver in-app to an address that **has** one.

When `SMTP.Enabled == false`, one case is refused and the other succeeds. No wording of the refusal
can hide that, because the outcomes differ.

Resolution, in order:

1. **Configured (the supported state)**: `POST /api/v1/shares` answers `201` with an identical
   body shape, identical status and no timing signal, for a known and an unknown address alike. The
   invitation row is created either way; `invitations.recipient` is left empty and resolved at
   accept time. FR-018 holds fully.
2. **Unconfigured**: an unknown address is refused `422 email_not_configured`; a known address gets
   `201` with `delivery: "in_app_only"`. FR-022 holds; FR-018 is narrowed.
3. The narrowing is bounded: it exists only on an instance whose operator has been warned at boot
   ([D-05](#d-05)) and, from phase 006, on the operator screen; and it discloses only "this address
   has an account here", never who or what.
4. `internal/web/api/shares_http_test.go` asserts **both** halves: with SMTP enabled, the known and
   unknown responses are byte-identical apart from the invitation id; with SMTP disabled, they
   differ exactly as specified and in no other way.

**Rationale**: refusing to decide leaves an implementer to pick, silently, in a handler. Both
requirements are real; one of them is conditional on a configuration the spec itself calls a
misconfiguration.

**Alternatives considered**:

- *Refuse all invitations when SMTP is off.* Preserves FR-018 perfectly, and contradicts FR-022's
  explicit instruction to keep delivering in-app. Rejected on the spec's own words.
- *Accept and silently drop the unknown-address invitation.* Preserves indistinguishability and
  creates an invitation nobody can ever answer, plus a lie in the sender's list. Rejected.
- *Queue it for when SMTP is configured.* A durable outbound queue, a retry policy and a state
  machine, for a case the spec resolves in one sentence. Rejected under Principle I.

### D-07

**Decision — `403`, not `404`, in exactly one situation: a `view`-level grantee attempting a write
on a resource they can already see. Everywhere else, refusal is `404`.**

Shared design §2.1 rule 13 makes `404` the default because existence is PHI; `403` is reserved for
resources whose existence the caller already knows. FR-058 and SC-006 require a viewer's write to be
refused **with an explanation naming their level** — which is only sayable if the response admits
the thing exists, and is only safe because the viewer is looking at it.

The envelope is `403 { "error": { "code": "forbidden_view_only", "message": "You have viewing
access to this record. Ask the owner for editing access to make changes." } }`.

Every other refusal in this phase is `404 not_found`, byte-identical to a non-existent id: stranger,
revoked grantee, lapsed grantee, disabled grantee, family-history grantee reaching for the patient,
grantee reaching the owner's directory, grantee reaching another patient of the same owner.

**Rationale**: an explanation the user can act on is worth exactly one carefully bounded exception,
and the boundary is machine-checkable — the `403` is producible only from a code path that has
already resolved a `Grant` for that resource.

**Alternatives considered**:

- *`404` everywhere, including the viewer's write.* Rejected: a chart that is visibly on screen and
  answers "does not exist" when you press Save is a bug report, not a security control.
- *`403` for every refusal.* Rejected: it converts every 404 into an existence oracle over the whole
  instance.

### D-08

**Decision — a viewer's write refusal is decided in `access.Authorizer` (which returns
`domain.ErrForbidden` wrapped with the grant's level), never in a handler.**

`Authorizer.Patient/Record(ctx, actor, …, need Permission)` returns
`(Grant, error)`. Resolution order is: superuser → allow and audit; owner → `PermOwn`; active grant
→ its level; otherwise `ErrNotFound`. If a grant exists but `grant.Level.Allows(need)` is false, the
error is `ErrForbidden` and the `Grant` is returned alongside it so the handler can name the level
without a second lookup.

**Rationale**: one checkpoint (shared design §6.5, constitution VII). A handler that decides "this
is a write, so 403" is a second authorization implementation, and there are ~40 handlers.

**Alternatives considered**: a `RequireEdit` middleware — rejected, it would have to know each
route's patient, which is exactly what the service already resolves.

### D-09

**Decision — owner-only powers are a third permission, `PermOwn`, and they are never reachable
through a grant.**

FR-005 reserves to the owner: deleting the patient, changing the identifying profile (name, birth
date, sex, photo), granting/changing/ending anybody's access, and transferring ownership. FR-006
forbids onward sharing at any level. `Permission` already has `PermView | PermEdit | PermOwn`
(shared design §6.5); a grant can only ever carry `PermView` or `PermEdit`, so
`Level.Allows(PermOwn)` is `false` for both levels — by construction, with no rule to remember.

`PATCH /api/v1/patients/{id}` therefore needs `PermOwn` for the identity fields and `PermEdit` for
nothing (a patient has no care fields), and `POST /api/v1/shares` needs `PermOwn` on every named
resource, which is also what makes FR-006 automatic: a grantee never holds `PermOwn`.

**Rationale**: an enum whose values cannot express the forbidden thing beats a check that must be
repeated in five handlers.

**Alternatives considered**: a `can_reshare` boolean on the grant — rejected, FR-006 says there is
no delegation at any level, so the column would be permanently `false` (upstream's
`custom_permissions` all over again).

### D-10

**Decision — ownership is re-checked inside the accept transaction, against the stored resource,
never trusted from the invitation.**

FR-016 and the spec's "Decisions taken here" section both require it, and the edge case "the sender
stops owning the resource between sending and acceptance" makes it concrete. The accept path, inside
`app.RunInTransaction`:

1. re-read the invitation `FOR UPDATE`-equivalent (SQLite's single writer, [D-11](#d-11)) and assert
   `status == pending` and `expires_at > now`;
2. assert the signed-in actor's email matches `recipient_email` case-insensitively (FR-025);
3. for **every** `resource_ids[i]`: re-read the resource and assert `owner == invitation.sender`,
   assert it still exists, assert no active grant already exists for `(resource, actor)`;
4. create every grant;
5. set `status = accepted`, `responded_at`, `recipient`.

Any failure in (3) aborts the whole transaction, the invitation moves to a terminal state, and the
recipient is told "the items are no longer available" **without being told what they were** (FR-028,
SC-013).

**Rationale**: an invitation is an offer, and an offer goes stale. Trusting it is how a former owner
grants access to somebody else's chart.

**Alternatives considered**: revalidating at send time only — rejected by FR-016 in as many words.

### D-11

**Decision — the double-accept race is closed by SQLite's single-writer transaction plus a
compare-and-set on `status`, not by an advisory lock.**

MediKube is single-instance by construction (constitution Technology Constraints) and PocketBase's
data store is one SQLite database with one writer. `app.RunInTransaction` holds the write lock, so
a re-read of `invitations.status` inside the transaction followed by a write is a genuine
compare-and-set: the losing transaction re-reads `accepted` and returns
`*share.TransitionError{From: accepted}`, which the handler maps to `409 conflict` naming the state
and when it was answered (FR-032, US5 scenario 7).

The unique partial index from [D-01](#d-01) is the second line: even a hypothetical two-writer
future cannot produce two active grants for one `(resource, grantee)`.

**Rationale**: the simplest thing that is actually correct on this platform. Principle I forbids the
speculative distributed-lock seam, and the Technology Constraints forbid the deployment that would
need it.

**Alternatives considered**:

- *An advisory lock table.* Rejected: machinery for a concurrency model this instance cannot have.
- *Rely on the unique index alone.* Rejected: it yields a constraint violation, not a state-aware
  error message, and FR-032 requires telling the caller which state it is in.

---

## C. Shape of the API and the model

### D-12

**Decision — `POST /api/v1/shares` is kept as the create-a-share-request operation, answering `201`
with the invitation DTO and `Location: /api/v1/invitations/{id}`.**

The shared design contract assigns op 58 this path and annotates it "creates an invitation (never a
direct grant)". Keeping it means a `POST` whose response body is a different resource than its
path — which was reconsidered against moving it to `POST /api/v1/invitations`.

Kept, because: the caller's act is "share this with this person"; there is **no** operation anywhere
that creates a grant directly, so `POST /shares` cannot be mistaken for one that exists; and the
contract is binding, so an aesthetic re-path is a deviation with no functional payoff. The oddity is
removed by documentation rather than by silence: `contracts/shares.md` states the status, the body
type and the `Location` header explicitly, and OpenAPI declares the response schema as
`InvitationSummary`.

**Alternatives considered**:

- *`POST /api/v1/invitations`.* Cleaner REST, one resource owning its own state machine. Rejected as
  a deviation from a binding contract that buys nothing a documented `Location` header does not.
- *`202 Accepted`.* Rejected: nothing is deferred; the invitation exists synchronously when the call
  returns.

### D-13

**Decision — one `share.Service` owns both shares and invitations; `share.InvitationRepository` is
allowed its five methods.**

Principle II calls one-to-three methods normal and requires justification past five. The invitation
port is `Get`, `GetByTokenHash`, `List`, `Save`, `Delete` — five, at the ceiling. `GetByTokenHash`
exists because the public preview must look an invitation up by a credential without loading a list;
`Delete` exists because FR-033 requires answered invitations to be removed after a retention period.

Keeping shares and invitations in **one** service is deliberate: FR-028's all-or-nothing acceptance
writes both collections in one transaction, and splitting them would put a transaction boundary
across a service boundary, which is where distributed-transaction bugs are born.

**Alternatives considered**:

- *Split into `share.Service` and `invitation.Service`.* Rejected: the accept path needs both inside
  one transaction; the split would need a shared unit-of-work abstraction, which is more machinery
  than it removes.
- *Fold invitations into a repository method on shares.* Rejected: two collections, two lifecycles,
  two audit vocabularies.

### D-14

**Decision — an invitation covers 1..50 resources of one kind, and `resource_ids` is a validated Go
`[]string` in a PocketBase `json` field.**

FR-015 allows one invitation to cover several things of the same kind. A bound is needed, because
"several" is otherwise an unbounded transaction and an unbounded email. 50 is chosen as a round
number above the spec's own scale target (20 patients, SC-014) with headroom for relatives.
Exceeding it is `422 too_many_resources`, naming the limit.

The field is a `json` column carrying a Go `[]string` validated for: non-empty, ≤50, no duplicates,
every entry a 15-character PocketBase id, all of one kind, all owned by the sender at send time
(FR-016, re-checked at accept per [D-10](#d-10)).

**Rationale**: shared design §1.2 specifies `resource_ids (json — a validated []string)`. Validated
Go structs in a `json` field are the established MediKube pattern for value lists that are only ever
read with their parent (shared design §1.5).

**Alternatives considered**:

- *An `invitation_resources` join collection.* Rejected: referential integrity nobody uses, a join
  on every read, and a second cleanup path — for a list read only with its parent and discarded
  when the invitation reaches a terminal state.
- *One invitation per resource.* Rejected: FR-015 requires all-or-nothing acceptance across the set,
  which N invitations cannot express, and the recipient would get N emails.

### D-15

**Decision — the invitation credential is 256 bits from `crypto/rand`, base64url-encoded in the
link, stored only as its SHA-256 hex digest under a unique index, and never logged.**

`token = base64url(crypto/rand 32 bytes)` (43 characters, no padding);
`token_hash = hex(sha256(token))`; lookup is by `token_hash`. The link is
`{MEDIKUBE_PUBLIC_URL}/invite/{token}`.

FR-024 is then satisfied in all four of its parts: unguessable (256 bits); not readable back out of
the instance (only the digest is stored, including in the superuser admin UI); dead the moment the
invitation leaves `pending` (the preview and respond paths both require `status == pending`, and the
row is deleted at the end of the retention window); and worthless alone (FR-025 — answering requires
a session on the invited address).

No HMAC, no expiry encoded in the token, no signature: expiry and status live in the row and are
read anyway.

**Rationale**: stdlib, one hash, one index, nothing to rotate. `SHA-256` rather than a password KDF
because a 256-bit random value has no guessable preimage — a KDF would defend against a dictionary
attack that cannot exist.

**Alternatives considered**:

- *PocketBase's own token machinery (`NewStaticAuthToken`, file tokens).* Rejected: those mint
  credentials for auth records, and constitution VII forbids the file-token pattern outright — a
  credential in a URL. This credential *must* be in a URL (it is an emailed link), which is exactly
  why it is single-purpose, hashed at rest, status-gated and useless without a session.
- *Store the token in the clear so the sender can resend the link.* Rejected by FR-024. Resending is
  cancel-and-reinvite.
- *A signed JWT.* Rejected: a second credential format, a key to manage, and a token that stays
  valid after the invitation is answered unless a denylist is added — i.e. the row we already have.

### D-16

**Decision — `shares.note` is copied from the invitation at accept time, not read through the
relation.**

FR-009 and FR-050 require the note to be shown to the grantee for as long as the grant lasts. The
invitation is removed after its retention window (FR-033), so a grant that read its note through
`shares.invitation` would silently lose it. The relation is kept for provenance ("whether it came
from an invitation", FR-035) and is allowed to become empty.

**Alternatives considered**: keeping the invitation forever — rejected by FR-033's own retention
requirement.

### D-17

**Decision — the level ceiling for `resource_kind = family_member` is enforced in
`domain/share.Invitation.Validate` and `domain/share.Share.Validate`, and again by the collection's
select field being unable to help.**

FR-007: a family-history grant is always `view`. Validation lives in the domain (shared design
§6.2), which is the authority; the service calls it; the storage layer cannot express "level = view
when resource_kind = family_member", so it is not the last line here — the domain is. A test asserts
that a hand-crafted `edit` family-history invitation is rejected at both the API boundary (`422
family_history_view_only`) and the domain boundary.

**Alternatives considered**: two collections with different level vocabularies — that is the
two-mechanism design FR-001 forbids.

### D-18

**Decision — `shares.revoked_by` distinguishes the three endings, and there is no fourth column.**

- owner revoked → `revoked_by = grantor`, audit `share_revoke`
- grantee left → `revoked_by = grantee`, audit `share_leave`
- lapsed → `revoked_at` stays empty and `expires_at` is in the past; audit `share_expire` is written
  by the tidy pass ([D-19](#d-19))

FR-039 requires the owner's view to distinguish "they left" from "I revoked", and FR-068 requires the
audit trail to distinguish all three. One relation column carries both.

**Alternatives considered**: an `ending_reason` select — rejected as derivable from two columns that
must exist anyway.

### D-19

**Decision — `share.Service.Tidy(ctx)` is written and tested here; phase 006 schedules it. It does
three things, none of them load-bearing.**

1. writes one `share_expire` audit event per grant whose `expires_at` has passed and which has not
   yet been tidied, then stamps `revoked_at = expires_at, revoked_by = ''` so it is written once;
2. moves `pending` invitations whose `expires_at` has passed to `expired` (they are **already**
   refused by the read path — this only tidies the list);
3. deletes invitations in a terminal state older than `MEDIKUBE_SHARING_INVITATION_RETENTION_DAYS`
   (default 90), leaving their audit events behind (FR-033).

Both audit-writing steps run with **no HTTP request**, and `audit_events.request_id` is `Required`,
so `Tidy` mints a **run id** on its context — same helper as a request id — and every `share_expire`
and `invite_expire` row of one run carries it, as do that run's log lines (001
[data-model](../001-walking-skeleton/data-model.md) §3). Without it the first scheduled tidy fails
validation in production rather than in test.

Correctness never depends on it: FR-029, FR-034 and FR-043 are all satisfied by the read-path
predicate. The `share_expire` audit entry for a lapse therefore appears when the tidy runs, which is
stated in `contracts/README.md` so SC-015 is read correctly — the entry is guaranteed and
deterministic, not instantaneous.

**Rationale**: a lapse is the absence of an event; something has to notice it eventually, and a
scheduled pass is the cheapest honest place. Doing it in the read path would put a write behind
every refusal.

**Alternatives considered**:

- *Emit `share_expire` from the Authorizer the first time it observes a lapse.* Rejected: a write on
  a read path, on the hottest function in the application, triggered by an unauthenticated-ish
  request pattern — an amplification vector.
- *No lapse audit at all.* Rejected by FR-068.

### D-20

**Decision — there is no `notifications` collection. Notices live in the in-process hub and are
delivered to whoever is connected.**

FR-067 makes a notice a courtesy: nothing may depend on delivery, and a missed notice must never
change what an account may reach. The hub therefore publishes `realtime.Event{Type: Notice, UserID,
Kind, ResourceKind}` and `internal/web/stream/notifications.go` renders it for the subscribers that
are connected. An account not connected misses it, exactly as the spec permits.

FR-066 ("re-check entitlement at the moment of delivery") is satisfied because the stream handler
re-resolves the actor's grant before rendering — the same rule the record stream already follows
(constitution V: fan out ids, re-authorise per subscriber, then render).

**Rationale**: upstream shipped channels, per-event preferences, delivery status, retries and an
idempotency key (domain-platform §11). The spec excludes every one of them by name. A durable table
would be YAGNI with a migration.

**Alternatives considered**:

- *Persist notices so they survive a reconnect.* Rejected: a durable inbox, a read/unread state, a
  retention policy and a purge — for a courtesy.
- *Reuse `audit_events` as the notice source.* Rejected: the audit trail is deliberately
  content-free and is not the grantee's to read (phase 006 owns the reader), and a notice needs the
  other party's display name, which the trail must never store (FR-071).

### D-21

**Decision — the realtime hub gains `Event.Type` and `Event.UserID`, and a per-user topic map
alongside the per-patient one.**

Two things must reach a person rather than a patient: the notice ([D-20](#d-20)) and the revocation
cut-off (FR-045, SC-005). The revoke path publishes `Event{Type: AccessChanged, UserID: grantee,
PatientID: resource}`; `internal/web/stream/records.go` subscribes to its subscriber's user topic in
addition to the patient topic, re-authorises on receipt, and on `ErrNotFound` patches the "your
access to this has ended" panel and returns, closing the stream.

The hub remains a channel and a map with context-bounded subscribers, and is **not** put behind a
broker interface — the constitution forbids that speculative seam explicitly.

**Rationale**: an open screen that empties without explanation is the worst outcome in the phase,
and it is named in US2 scenario 2.

**Alternatives considered**: listed in the plan's Complexity Tracking (poll; re-authorise only on the
next record event; a second hub; a broker interface).

### D-22

**Decision — the grantee sees the owner's labels on records they may see, and an editor may apply
and remove only labels already in the owner's set. No grant ever exposes a label vocabulary.**

FR-059 and the spec's own reasoning: a label is part of how a record reads; the set of labels an
account uses across its household is not. Implementation:

- `tags` rows belong to `users`, not to patients (phase 003 data model §5.1).
- Reading a shared record resolves its `tags` relation to `TagRef{id, name, color}` **as stored** —
  those rows belong to the patient's owner, and reading them through the record is a read of the
  record.
- `GET /api/v1/tags` continues to list **the caller's own** tags only. A grantee gets their own set,
  never the owner's.
- `PATCH /api/v1/records/{kind}/{id}` by an editor validates every submitted tag id against the
  **patient owner's** tag set; an id outside it is `422 unknown_tag`, which discloses nothing (it is
  identical for "not yours" and "does not exist").
- Creating, renaming, recolouring or deleting a tag in the owner's set is `PermOwn` — refused for a
  grantee.

**Alternatives considered**:

- *Resolve a shared record's tags against the grantee's own vocabulary.* Rejected: the record would
  read differently to two people looking at the same chart.
- *Expose the owner's vocabulary for the picker.* Rejected by FR-059 and by the spec's stated
  reasoning; the editor's picker on a shared chart offers exactly the labels already present on that
  patient's records, which is a query over the shared data, not over the owner's account.

### D-23

**Decision — a practitioner or facility referred to by a shared record is embedded in the record's
DTO; the directories stay `PermOwn` and 404 for a grantee.**

FR-060. `PractitionerRef{id, name, specialty}` and `FacilityRef{id, name, kind}` already exist in
the record DTOs (shared design §5.5). Reading them through a shared record is a read of the record.
`GET /api/v1/practitioners`, `GET /api/v1/practitioners/{id}`, `GET /api/v1/facilities`,
`GET /api/v1/facilities/{id}` remain owner-scoped and answer `404` to a grantee — including for the
very practitioner whose name they can see on the record. That asymmetry is deliberate and is
asserted by a test.

### D-24

**Decision — the accessible-patient set is one indexed query, and the patient list pages over
`owned ∪ shared` with a stable cursor.**

`access.ShareReader.ActivePatientsFor(ctx, userID, now) ([]string, error)` runs one query over
`shares` with the D-01 predicate and the index `(grantee, revoked_at, resource_kind, patient)`.
`patient.Repository.ListAccessible` takes the owner id and that id set and returns one page ordered
by `(is_shared, LOWER(last_name), LOWER(first_name), id)`, so the two groups are contiguous, both
are counted, and the HMAC-signed cursor from shared design §6.3 encodes the sort keys — never an
offset — so a grant created or revoked mid-page cannot duplicate or skip a row (FR-075, SC-014).

At the spec's scale (50 shared patients) the id set is small enough to inline into an `IN (…)`.
A bound is set at 500 accessible patients per account, beyond which the query switches to a join;
crossing it is a logged warning, not an error.

**Rationale**: SC-014's 2 s budget over 200 grants and 20 people is not demanding, but the
correctness requirement (0 repeats, 0 skips, while grants are changing) is, and only a keyset cursor
gives it.

**Alternatives considered**:

- *Two separate lists, owned and shared.* Rejected by FR-055 (one list, visibly distinguished,
  each group counted) and it makes paging across the boundary incoherent.
- *A materialised `accessible_patients` table.* A cache with an invalidation protocol, for a query
  over an indexed two-column filter. Rejected.

### D-25

**Decision — `read_sensitive` is written for an individual record open and a content or preview
retrieval by anyone whose resolved grant is not their own ownership; list paging writes nothing.**

FR-070 and the spec's stated reasoning ("the ordinary paging of a list is not [recorded], because
that would drown the trail without answering the question anybody actually asks"). Implementation:
`GET /api/v1/records/{kind}/{id}` and `GET /api/v1/attachments/{id}` call `audit.Record` when the
resolved `Grant.Level != PermOwn`. That covers a grantee at either level **and a superuser reading
somebody else's data**; a superuser reading a patient they themselves own writes nothing, exactly as
any other owner. The superuser's `admin_session` entry records the *session* and does not replace
these per-read rows — both exist. The row carries actor, `read_sensitive`, target kind, opaque
target id, patient id, request id — there is no `ip` column (001 research D-19) — and no name,
path, filename or value (FR-071).

**This is the single statement of the rule for the whole product**, written out in
[`contracts/widened-authorization.md`](./contracts/widened-authorization.md) §"Where
`read_sensitive` is written". Phase 004's FR-076, its `contracts/attachments.md` §3.6 and its T096
were reconciled to it: an owner's own document retrieval writes **no** row, which is a change from
004's original "every retrieval". The reason is not convenience. The trail exists for accountability
about who reached data they do not own; recording every self-read produces unusable noise and builds
a timeline of when somebody read their own most sensitive results, which is itself an exposure under
Principle VII. Phase 006 FR-075 already applies the same asymmetry to the trail itself — reading it
is unaudited, exporting it is audited.

**Alternatives considered**:

- *Audit every read including lists.* Rejected by the spec, and it would multiply the trail by the
  page size on a screen that scrolls.
- *Audit the owner's own reads too* — phase 004's original FR-076. Rejected: it answers no question
  anybody asks, doubles the table, and the timeline it builds is the disclosure Principle VII
  forbids.
- *Exempt superusers, on the grounds that `admin_session` already covers them.* Rejected: the
  session entry says somebody used the break-glass credential, not **whose** records they opened,
  and the second question is the one an audit is for.

---

## D. Configuration added by this phase

One nested block on the existing `internal/config.Config`, `envPrefix: "SHARING_"`, all with
defaults, none required:

| Env | Default | Bounds | Why |
|---|---|---|---|
| `MEDIKUBE_SHARING_INVITATION_TTL` | `168h` | 1h..8760h | FR-017's default 7 days, settable 1 hour to 1 year |
| `MEDIKUBE_SHARING_INVITATION_TTL_MIN` | `1h` | — | FR-017's floor, validated at boot |
| `MEDIKUBE_SHARING_INVITATION_TTL_MAX` | `8760h` | — | FR-017's ceiling (1 year) |
| `MEDIKUBE_SHARING_MAX_RESOURCES_PER_INVITATION` | `50` | 1..200 | [D-14](#d-14) |
| `MEDIKUBE_SHARING_INVITATION_RETENTION_DAYS` | `90` | 1..3650 | FR-033's documented retention |

No secret is added. Nothing here changes an authorization decision, so nothing here can widen
access by misconfiguration.

---

## E. Risks this phase closes, and what it hands on

| Risk | Status |
|---|---|
| **R10 — email deliverability for invitations** (shared design §8, assigned to this phase) | **Closed** by [D-05](#d-05): SMTP state is read as a flag, warned at boot, refused up front for the case that cannot be delivered, and surfaced for phase 006's `GET /api/v1/admin/system`. |
| **R7 — the >5-minute SSE liveness test** | Partly: this phase ships the build-tagged Go assertion for the notifications stream ([D-04](#d-04)); phase 006 still owns the CI job. |
| **R8 — PocketBase upgrade fragility** | One item added to the phase-001 checklist: re-verify `core/base.go:713` (mailer substitution) and `core/field_date.go:110` (`TEXT DEFAULT '' NOT NULL`) on every PocketBase upgrade — the second one silently changes the meaning of every predicate in this phase. |
| Handed to **phase 006** | the tidy job's schedule ([D-19](#d-19)); the audit **reader** over the events this phase writes; the operator screen showing the SMTP warning; whether and how a grantee may export a shared person's data, under the rule this phase sets — no later feature may deliver to a grantee more than their level allows, and an export by a grantee is itself an accountable act. |
