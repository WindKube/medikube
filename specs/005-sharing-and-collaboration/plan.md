# Implementation Plan: Sharing and Collaboration

**Branch**: `005-sharing-and-collaboration` | **Date**: 2026-08-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-sharing-and-collaboration/spec.md`

**Constitution**: [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.3.0 (binding)

**Shared design contract**: [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) (binding on design; see
[Deviations](#deviations-from-the-shared-design-contract))

---

## Summary

Phases 001–004 built an application in which exactly one fact decides every read and every write:
**you own the person, or you get a 404.** This phase adds the second — and last — fact: **somebody
who owns the person let you in.**

It ships two new collections (`shares`, `invitations`), ten `/api/v1` operations, three pages, one
transactional email, one new SSE stream, and one change to the single authorization checkpoint that
every earlier phase already funnels through. That last item is the whole phase: because
`access.Authorizer` is the only place in the application that decides anything, widening it from
`owner → PermOwn, else ErrNotFound` to `owner → PermOwn, active grant → its level, else
ErrNotFound` widens every list, every chart, every record, every document, every search, every
timeline and every live view at once, without touching a single handler.

The load-bearing commitments, each verified against PocketBase v0.40.1 source rather than
documentation:

1. **`revoked_at IS NULL` is a bug, not a filter.** `core/field_date.go:110` declares every
   PocketBase `DateField` column as `TEXT DEFAULT '' NOT NULL`, and `core/field_relation.go:161`
   does the same for a single relation. There is no NULL in a PocketBase collection. The shared
   design contract's "unique partial index … `WHERE revoked_at IS NULL`" would have compiled,
   migrated, and then silently matched **nothing** — every uniqueness guarantee and every
   active-grant filter in this phase would have been inert. Every predicate in this phase is
   written against `= ''`. This is research decision [D-01](./research.md#d-01) and it is asserted
   by a migration test, not by care.
2. **Expiry is a predicate, never a sweeper.** A grant is active when
   `revoked_at = '' AND (expires_at = '' OR expires_at > :now)`, evaluated inside the repository
   query on every single access decision. FR-034 and FR-043 are then true by construction, and the
   tidy job phase 006 schedules can never be load-bearing.
3. **Revocation reaches an already-open screen because the hub carries it.** The in-process
   realtime hub gains a second event shape — an access change addressed to one user — so a revoke
   both cuts the open SSE stream and tells the person why, inside FR-045's and SC-005's 5 seconds.
   The hub stays a channel and a map (constitution Technology Constraints); it is not put behind a
   broker interface.
4. **Invitation email goes through `app.NewMailClient()`, and only through it.**
   `core/base.go:713` sends via the event's mailer when a hook replaced it, and
   `tests/app.go:500` is exactly such a hook — so every invitation email is captured by
   `TestApp.TestMailer` with no seam of MediKube's own. A hand-rolled SMTP client would have been
   untestable and would have re-implemented what Principle V forbids re-implementing.
5. **"Outbound email is not configured" is `Settings().SMTP.Enabled == false`, not a failed send.**
   `core/base.go:637-653` falls back to `&mailer.Sendmail{}`, which `exec`s a `sendmail` binary
   that does not exist in `gcr.io/distroless/static-debian12`. Detecting the misconfiguration by
   trying to send would mean discovering it after the user pressed the button. MediKube reads the
   flag, warns at boot, and refuses FR-022's case up front.
6. **Nothing shown before acceptance names anybody.** The invitation preview DTO physically cannot
   carry a patient name: it has no field for one. That is the FR-023/SC-010 control — DTO shape,
   not a redaction pass.

---

## Technical Context

**Language/Version**: **Go 1.27** (`go 1.27` + `toolchain go1.27.x` in `go.mod`). Not the monorepo's
1.26.5 house standard: PocketBase v0.40.1's `go.mod` declares `go 1.27` and 67 non-test files import
the Go 1.27 stdlib package `encoding/json/v2`, 15 of them under `core/` and `apis/`;
`GOTOOLCHAIN=local go build` on 1.26.5 fails outright (VERIFIED-SOURCE-FACTS FACT 0). CI MUST NOT
set `GOTOOLCHAIN=local`.

**Primary Dependencies** — no new module is introduced by this phase:

| Module | Version | Role in this phase |
|---|---|---|
| `github.com/pocketbase/pocketbase` | v0.40.1 | 2 new collections + 1 amendment as reversible Go migrations; `RunInTransaction` for the all-or-nothing accept; `NewMailClient()` for the invitation email; post-commit `OnRecordAfter*Success` hooks on `shares`/`invitations` for the audit trail; `CascadeDelete` for FR-048/FR-053 |
| `github.com/a-h/templ` | v0.3.1020 | sharing screen, invitations screen, public invite-landing page, share drawer, notice toast, **and the invitation email body** — one renderer for web and mail |
| `github.com/starfederation/datastar-go` | v1.2.2 | the share drawer and respond/revoke actions (plain `text/html` patches, no stream), plus `GET /api/v1/streams/notifications` |
| `github.com/caarlos0/env/v11` | v11.4.1 | `MEDIKUBE_SHARING_*` (invitation default/min/max lapse, max resources per invitation, answered-invitation retention) |
| `github.com/rs/zerolog` | v1.35.1 | the only logger; redacting `MarshalZerologObject` on `Share` and `Invitation` (never the address, the note or the token) |
| `github.com/getsentry/sentry-go` | v0.48.0 | errors and panics only, scrubbed |
| `github.com/prometheus/client_golang` | latest pinned | `medikube_shares_*` and `medikube_invitations_*`; labels bounded to `resource_kind`, `level`, `outcome`, `reason` |
| `go.opentelemetry.io/otel` | latest pinned | `service.share.*` and `store.shares.*` spans; no address, note or token as an attribute |
| `github.com/samber/do` | v2 | container providers for the share service, the mail adapter and the notifier |
| `github.com/samber/lo` | v1.53.0 | sparingly, per Principle IV |
| `github.com/stretchr/testify` | v1.12.0 | the only assertion library |
| `github.com/spf13/cobra` | **transitive — pinned once in [001's plan](../001-walking-skeleton/plan.md#technical-context), never a direct `require`** | via PocketBase's `RootCmd`; `medikube seed` gains sharing fixtures, `medikube purge` gains the share/invitation tidy. The version is whatever `pocketbase@v0.40.1`'s `go.mod` requires and is not restated here (cross-artifact finding M2) |
| `modernc.org/sqlite` | v1.57.0 | transitive; pure Go, so `CGO_ENABLED=0` holds |

Stdlib only for the invitation credential: `crypto/rand` + `crypto/sha256` + `encoding/base64`.
No token library, no UUID library.

**Forbidden and absent**: gin, huma, viper, `samber/mo`, `samber/ro`, `samber/slog-zerolog`, any
second router/logger/config/DI/assert library, PocketBase `jsvm`, any cgo dependency,
`datastar.WithCompression`, PocketBase's native realtime, PocketBase's file-token mechanism, and
the Datastar Pro attribute set.

**Storage**: PocketBase-embedded SQLite (`modernc.org/sqlite`), data dir `/data/pb_data`, WAL.
Two new collections (`shares`, `invitations`) plus one amendment to `audit_events` — the new
`reason` column and five action values.
All five API rules `nil` on both new collections, asserted at boot. **Zero new file fields**, so
the `Protected: true` boot assertion still covers exactly `patients.photo` and `attachments.file`.
**Zero new `deleted_at` columns**: a revoked grant is a `revoked_at` timestamp on a live row, which
is a state, not a soft delete — the row is still read, listed and reported on.

**Testing**: `stretchr/testify` (`require` for preconditions, `assert` for independent assertions),
table-driven `t.Run` subtests. Six layers, all mandatory:

- **unit** — the share service and the invitation state machine against hand-written fakes, with
  `t.Parallel()`; an injected `Clock` so lapse is tested without sleeping.
- **integration** — `internal/store/share` and `internal/store/invitation` against a throwaway
  `tests.NewTestApp` cloning `internal/testdata/pb_data`.
- **contract** — `sharetest.RepositoryContract` and `sharetest.InvitationRepositoryContract` run
  against both the real repository and the fake (Principle II's Liskov clause).
- **HTTP** — `tests.ApiScenario` per operation, each carrying the four-row authorization matrix,
  and `ExpectedEvents` proving the record-CRUD hooks that must not fire did not.
- **UI render** — templ components (including the email body) rendered to a `bytes.Buffer`.
- **browser** — Playwright CLI over the three new pages plus every page this phase widens, at
  1440×900 and 390×844, including a two-session live-notice spec and a revocation cut-off spec.

`tests.TestApp` is **never shared across `ApiScenario` cases** (VERIFIED-SOURCE-FACTS FACT 7:
`bindUIExtensions` re-enters on every `OnServe` until the stack overflows).

**Target Platform**: one static `linux/amd64` + `linux/arm64` binary, `CGO_ENABLED=0`,
`gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, `VOLUME ["/data"]`. No Node.js in
the runtime image. **Note that distroless has no `sendmail`** — see decision [D-05](./research.md#d-05).

**Project Type**: single server-rendered Go web application; a project inside the `windkube`
monorepo at `/medikube`, image `ghcr.io/windkube/medikube`.

**Performance Goals** (the spec's success criteria are the acceptance bar):

- **SC-014**: with **200 active grants across 20 people** for one owner and a grantee reaching
  **50 shared people**, both sharing panels, both invitation lists and the list of people render
  within **2 s**, and paging repeats or skips **0** entries.
- **SC-005**: a revoked grantee's very next request is refused with **no sign-out and no cron**, and
  an open live view stops and says so within **5 s**.
- **SC-017**: a notice reaches the account concerned within **5 s** without a refresh, and a session
  left open for **60 continuous minutes** is still receiving them (the PocketBase 5-minute
  `WriteTimeout` trap; phase 001's `newStream()` helper is the fix and this phase adds the
  regression assertion for the notifications stream).
- **SC-002**: emailed link → reading the shared chart in under **2 minutes**, including sign-up.

**Constraints**:

- No cgo, one binary, no runtime Node, no CDN fetch. **The invitation email is the only outbound
  network request this phase makes** (FR-073), it goes only to the invited address, and only when
  the operator configured SMTP.
- CSP `script-src 'self' 'unsafe-eval'`; every other directive strict. The Datastar inline-script
  SDK family stays banned, so the invite-landing page redirects with a `303` issued *before* any
  stream is opened, never with `sse.Redirect`.
- Records are hard deleted (constitution VII). Revocation deletes **nothing** (FR-047).
- PocketBase's record CRUD subtree and `/api/batch` remain unreachable to non-superusers.
- `OnRecord*Request` hooks are dead code under the lockdown; only the post-commit
  `OnRecordAfter*Success` model hooks are bound.
- Single instance by construction; the realtime hub is a channel and a map, not a broker.
- Go 1.27 `encoding/json/v2` semantics: slices marshal as `[]` never `null`, unknown fields are
  rejected (422), duplicate keys are rejected.
- **Every predicate over a nullable-looking PocketBase column uses `= ''`, never `IS NULL`.**

**Scale/Scope**: 2 new collections, 1 collection amendment, 3 migrations, 10 new `/api/v1`
operations (phases 001–004 register 22 + 20 + 8 + 9 = 59, SHARED-DESIGN §2.3, so **69** after this
phase), 3 new pages (2 authenticated + 1 public) plus 1 shell region, 1 new SSE stream, 1
transactional email, 2 new service packages, 2 new store packages, and the widening of every
patient-scoped operation already shipped.

**No `NEEDS CLARIFICATION` items remain.** Every open question the specification raises — including
the two places where two of its own requirements collide — is resolved in
[research.md](./research.md) with a decision, a rationale and the rejected alternatives.

---

## Constitution Check

*GATE — evaluated before Phase 0 and re-evaluated after Phase 1 design. Both passes recorded.*

### I. Simplicity Is A Gate (KISS) — **PASS, with two tracked entries**

The upstream application ran **two** sharing subsystems — `patient_shares` and family-history
shares — with two invite paths, two bulk-invite shapes, two revoke shapes, two remove-my-access
routes and two `shared-by-me`/`shared-with-me` pairs: 20 operations. This phase ships **one**
`shares` collection with a `resource_kind` discriminator and **ten** operations, of which
`DELETE /shares/{id}` and `DELETE /shares/{id}/mine` are kept apart only because they are genuinely
different acts by genuinely different actors with a different audit meaning (FR-039).

Explicit YAGNI decisions taken here and recorded in research.md:

- **No `notifications` collection.** FR-067 makes notices a courtesy that nothing may depend on, so
  they are published to the in-process hub and delivered to whoever is connected. A durable
  notification table, a delivery-status machine, a retry count and a per-event preference matrix are
  all present upstream and all absent here.
- **No third permission level.** Upstream's `full` was never defined; delete is the owner's anyway.
- **No `custom_permissions`.** Free-form JSON that nothing reads is a liability (FR-002).
- **No delegation, no re-sharing, no organisation accounts, no public links** — all excluded by the
  spec and none of them leaves a seam behind.
- **No grant cache.** FR-037 requires a level change to bite on the next action; a cache with an
  invalidation protocol is exactly the machinery that makes that requirement subtly false.
- **No `POST /invitations/cleanup` endpoint.** Expiry is a predicate; tidying is a job phase 006
  schedules over a function this phase writes and tests.

Two things strain this principle enough to be written down in
[Complexity Tracking](#complexity-tracking): the second event shape on the realtime hub, and the
conditional relation pair (`patient` / `family_member`) on one row.

### II. Interfaces At Every Seam (SOLID) — **PASS**

- **Single responsibility.** `internal/web/api/shares.go` parses and renders;
  `internal/service/share` decides; `internal/store/share` persists; `internal/platform/mail`
  addresses an envelope. The state machine lives in `internal/domain/share` and touches no I/O.
- **Open/closed.** `shares.resource_kind` has two values and the code branches on it **once**, in
  `domain/share.Resource.Validate` and in the repository's column choice. Everything above that —
  the service, the handlers, the pages, the audit hook — is written against a
  `share.Resource{Kind, ID}` value. Adding `report_template` as a third kind in some future phase is
  a migration plus two switch arms, not a new subsystem. That is precisely what the upstream code
  proved is worth having.
- **Liskov.** `sharetest.RepositoryContract` and `sharetest.InvitationRepositoryContract` are single
  suites executed against the real repository and the fake. The `access.ShareReader` port has its
  own contract suite, run against both implementations, because it is the port that decides whether
  somebody sees a medical record.
- **Interface segregation.** Every port is consumer-declared and small:
  `share.Repository` (4: `Get`, `List`, `Save`, `Delete`), `share.InvitationRepository`
  (5: `Get`, `GetByTokenHash`, `List`, `Save`, `Delete`), `share.Authorizer` (2),
  `share.Auditor` (1), `share.Mailer` (2: `Configured`, `SendInvitation`),
  `share.Notifier` (1: `Notify`), `share.Clock` (1), `share.TokenMinter` (1),
  `access.ShareReader` (2: `ActiveFor`, `ActivePatientsFor`), `stream.Subscriber` (extended by one
  method). `share.InvitationRepository` at five methods is at the stated normal ceiling and is
  justified in research [D-13](./research.md#d-13); nothing here is an omnibus `Store`.
- **Dependency inversion.** `internal/domain/**` and `internal/service/**` import neither
  PocketBase, nor `net/http`, nor templ — including the new packages, which the existing
  `depguard` globs already cover. `internal/platform/mail` is a new `[PB]` package and is added to
  the `depguard` allowlist in the Setup phase.

### III. Test-First With testify (NON-NEGOTIABLE) — **PASS**

All **47** acceptance scenarios across the spec's six stories become named failing tests before
their implementation task starts; FR-076 and SC-018 demand exactly that and `tasks.md` orders every
pair test-then-implementation.

Authorization is tested as a first-class concern, and this phase is where the constitution's own
words land: *"once sharing exists — a test proving a user who was granted access succeeds and a
user whose access was revoked is refused."* FR-077 turns that into a matrix applied to **every**
operation in the application that touches a person's data — the six record routes, the patient
routes, the photo routes, the attachment routes, search, the timeline, the status views and both
streams:

| Actor | Expected |
|---|---|
| stranger | `404`, byte-identical to a non-existent id |
| grantee at the needed level | success, identical to the owner's response |
| grantee at `view` attempting a write | `403 forbidden_view_only`, nothing changed |
| grantee whose grant was revoked | `404` |
| grantee whose grant has lapsed | `404` |
| unauthenticated | `401`, disclosing nothing |

FR-078's family-history isolation gets its own hostile suite: from a `family_member` grantee's
session, attempt the patient the entry is filed against, that patient's records of **every**
registered kind, that patient's other relatives, its attachments and its search — all `404`.

### IV. Idiomatic Go Over Clever Go — **PASS**

Errors are values wrapped with `%w` and inspected with `errors.Is`/`errors.As`; the state machine
returns typed `*share.TransitionError` carrying the state it is actually in (FR-032). `Patch`
carries absent-vs-null with plain pointers (`*Level`, `**Date`) — `samber/mo` is forbidden precisely
because `mo.Result` severs the chain the error table depends on. `context.Context` is first
everywhere and honoured, which the notifications stream depends on for shutdown. The hub's
subscriber goroutines have an owner, a context-bounded lifetime and a defined shutdown path.
Generated `*_templ.go` is committed, marked generated, excluded from lint and coverage.

### V. PocketBase Is The Platform, Not A Detail — **PASS**

Nothing PocketBase provides is rebuilt:

- **Mail**: `app.NewMailClient()`, the `OnMailerSend` hook and `Settings().SMTP` are the mail
  subsystem. MediKube writes an envelope and a templ body and hands it over.
- **Atomicity**: `app.RunInTransaction` wraps the accept (FR-028) and the withdraw (FR-031). SQLite
  is single-writer, so the compare-and-set on `invitations.status` inside that transaction is what
  makes the double-accept edge case impossible without an advisory lock (research
  [D-11](./research.md#d-11)).
- **Cascade**: `shares.patient`, `shares.family_member`, `shares.grantor`, `shares.grantee` and
  `invitations.sender` are `CascadeDelete: true`, which is FR-048 and FR-053 implemented by
  `core/record_model.go` rather than by MediKube. Both are *proved by test*, not assumed.
- **Hooks**: only post-commit `OnRecordAfterCreateSuccess` / `…UpdateSuccess` / `…DeleteSuccess` on
  `shares` and `invitations`. `OnRecord*Request` is not used and is blocked by `forbidigo`.
- **Realtime**: PocketBase's is not used, for the three verified reasons in Principle V. The
  notifications stream is a MediKube Datastar SSE handler fed by the in-process hub, publishing
  **ids and event kinds only** and **re-authorising per subscriber at delivery** — which is exactly
  what FR-066 independently requires.
- Both new collections keep all five API rules `nil`, asserted at boot and proved per collection by
  a `tests.ApiScenario` showing `/api/collections/shares/records` returns `404` to a normal user.
  A share row is the single most attractive target on the instance: it names two accounts and a
  patient in one row.

### VI. One Log Stream, One Trace Context — **PASS**

zerolog only. `share.Share` and `share.Invitation` implement `MarshalZerologObject` emitting **only**
opaque ids and the enum values — never `recipient_email`, never `note`, never `response_note`, never
the token or its hash, never a display name. FR-072 gets its own gate: the phase-001
`internal/testsupport/phileak` harness is extended with four new sentinel classes (address, note,
display name, invitation token) and driven across **every operation this phase defines** (FR-080,
SC-016), asserting zero occurrences in the zerolog stream, the Prometheus registry, the OTel span
recorder and the Sentry transport.

Metric labels stay bounded: `medikube_shares_granted_total{resource_kind,level}`,
`medikube_shares_revoked_total{resource_kind,by}`, `medikube_invitations_sent_total{resource_kind}`,
`medikube_invitations_answered_total{response}`, `medikube_access_denied_total{reason}`,
`medikube_sse_streams_active{stream}`. No address, no user id, no patient id, no note.

Datastar's `ConsoleLog`/`ConsoleError` remain banned on production paths — the notice toast is a
`PatchElements` into `#toast`, never a console write, or the Principle VIII gate would fight
itself.

### VII. Patient Privacy Is Structural, Not Procedural — **PASS**

This is the principle the phase exists to serve, and the one it can most easily violate.

- **404, never 403, for anything whose existence is PHI** (FR-044, SC-004). `403` appears exactly
  once in this phase and only where the caller demonstrably already knows the resource exists: a
  `view`-level grantee attempting a write on a chart they are looking at (FR-058, SC-006). That
  exception is written into `contracts/README.md` as a rule, not left to a handler's judgement.
- **Nothing before acceptance names anybody** (FR-023, SC-010). Enforced by DTO shape: the preview
  DTO has no field capable of carrying a patient name, a relative's name or a clinical value.
- **The invitation link is a credential and is treated as one** (FR-024): 256 bits from
  `crypto/rand`, base64url in the email, **only its SHA-256 stored**, unique-indexed, invalidated by
  the state machine the instant the invitation leaves `pending`, and useless without a session on
  the invited address (FR-025). It is never logged, never traced, never a metric label, and it is
  not readable back out of the instance — including from the superuser admin UI, which sees the
  hash.
- **Reads by a non-owner are recorded, and only those** (FR-070): opening an individual record, and
  retrieving a document's content or a preview, writes a `read_sensitive` audit row when — and only
  when — the resolved grant is not the reader's own ownership. That covers a grantee at either level
  and a superuser reading somebody else's data; an owner's own read writes nothing, at any privilege
  level. List paging deliberately writes nothing either, per the spec's own reasoning. This is the
  product's single statement of the rule (`contracts/widened-authorization.md` §"Where
  `read_sensitive` is written"); phase 004's FR-076 and its serve path are reconciled to it, and
  phase 006's reader displays what it produces.
- **The audit trail carries no content** (FR-071): actor, action, target kind, opaque target id,
  patient id, timestamp, request id, and — new in this phase — a bounded `reason` token. **No
  `ip`**: phase 001 deliberately does not create the column (001 research D-19). No address, no
  note, no display name, no level-change free text.
- **No new file fields**, so the `Protected: true` boot assertion is unchanged and still trivially
  auditable.
- **One outbound request, to one address, only when configured** (FR-073). Nothing else in this
  phase talks to anything.
- **Break-glass is not a grant**: the superuser admin UI reaches everything by design, appears on
  nobody's sharing screen, and is audited as `admin_session` — restated here because this is the
  phase where somebody will otherwise be tempted to model it as one.

### VIII. The UI Must Prove It Renders — **PASS**

Three new pages (`/sharing`, `/invitations`, `/invite/{token}`) plus one new shell region, each
covered at 1440×900 and 390×844 asserting `200`, the four shell landmarks, the page's own landmark,
`body[data-signals]` present, and zero console errors / page errors / failed requests. The route
list is derived from `medikube routes`, so a page added without a smoke case fails the build
(FR-079, SC-019). The seeded instance deliberately holds an account with **nothing shared in either
direction** so that the `@EmptyState` path is what the landmark assertion exercises (FR-040, US1
scenario 8, SC-019).

Two browser specs exist because a Go test structurally cannot prove them:
`e2e/specs/sharing-live.spec.ts` (two browser contexts: a notice arrives without a refresh; a revoke
empties and explains an open screen within 5 s) and `e2e/specs/sharing-keyboard.spec.ts`
(SC-020: send, answer, change level and end access with a keyboard only, focus visible throughout,
at both viewports).

### IX. Compliance Is A Build Gate, Not A README Paragraph — **PASS**

Five gates, all `go test` or CI steps, all failing the build:

1. `internal/openapi/gate_test.go` — the route registry and committed `api/openapi.json` agree on
   every `operationId`; the regenerated document is byte-identical to the committed one.
2. `e2e/routes.gate.spec.ts` — every route emitted by `medikube routes` with `Page: true` has a smoke
   case, now including `/invite/{token}` with a seeded token.
3. `internal/service/access/coverage_test.go` — **new in this phase**: every registered record kind
   and every patient-scoped route in the registry has an entry in the authorization matrix suite. A
   route that touches patient data without the six-actor matrix fails the build. This is FR-077
   made mechanical and it is modelled on `medikeep-mcp/cmd/gen-tools/coverage_test.go`, the house
   precedent.
4. `internal/store/migrations/assertions_test.go` — extended: all five rules `nil` on both new
   collections; no new file field; and the **`= ''` assertion**, which parses every index and every
   repository predicate in `internal/store/share` and `internal/store/invitation` for the literal
   `IS NULL` and fails on sight (D-01).
5. golangci-lint v2 with `depguard` and `forbidigo`, extended for `internal/platform/mail`.

All three migrations have a real `down`; `migrations.Register`'s signature makes that structural
(VERIFIED-SOURCE-FACTS FACT 8).

### Post-Design Re-Check (after Phase 1)

Re-evaluated against `data-model.md` and `contracts/`. No principle moved from PASS to FAIL.

Two things surfaced during design and were resolved rather than tracked:

- *A `shares.status` column* was considered so a list could filter on one field. Rejected: it is a
  denormalisation of `revoked_at`/`expires_at`/`now` that can disagree with them, and the
  disagreement's failure mode is honouring a lapsed grant — which is the exact upstream bug this
  phase exists to not repeat. `active` is a computed predicate, everywhere, without exception.
- *A `patient_shares_count` counter on `patients`* was considered for the "tell the owner how many
  people have access before they confirm a delete" requirement (US edge case 1). Rejected as a cache
  with no invalidation story; it is a `COUNT` on a two-column index, on a confirmation dialog a
  human is reading.

One genuine conflict **between two of the spec's own requirements** was found and is resolved in
research [D-06](./research.md#d-06): FR-018 (a send must not reveal whether an address has an
account) and FR-022 (when outbound email is unconfigured, refuse an address with no account but
still deliver in-app to one that has an account) cannot both hold when SMTP is off. FR-022 wins in
that state, the disclosure is confined to an instance the operator has already been warned about
twice, and a test asserts the observable difference exists **only** when
`Settings().SMTP.Enabled == false`.

---

## Project Structure

### Documentation (this feature)

```text
specs/005-sharing-and-collaboration/
├── plan.md              # This file
├── research.md          # Phase 0 — every technical decision, with evidence
├── data-model.md        # Phase 1 — 2 collections, 1 amendment, enums, state machine, migrations
├── quickstart.md        # Phase 1 — run and verify this phase by hand, end to end
├── contracts/
│   ├── README.md                  # conventions, status codes, the 403 exception, the authz matrix
│   ├── shares.md                  # ops 58–62 — create (invitation), list, change, revoke, leave
│   ├── invitations.md             # ops 63–66 — list, public preview, respond, cancel/withdraw
│   ├── streams-notifications.md   # op 67 — the SSE notice stream and the revocation cut-off
│   ├── widened-authorization.md   # what changes on every endpoint phases 001–004 already shipped
│   └── pages.md                   # 3 new pages + 1 shell region + the widened pages
├── checklists/          # pre-existing
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root `/medikube`)

Only paths this phase **creates** or **touches**. `[NEW]` = created here, `[EDIT]` = modified.

```text
medikube/
├── cmd/medikube/main.go                                 [EDIT] register share service, mail adapter, notifier
│
├── internal/config/config.go                            [EDIT] SharingConfig (envPrefix "SHARING_")
│
├── internal/domain/
│   ├── share/share.go                                   [NEW]  Share entity, Active(now), Validate, redacting marshal
│   ├── share/invitation.go                              [NEW]  Invitation entity, Validate, redacting marshal
│   ├── share/level.go                                   [NEW]  Level (view|edit) + Allows(Permission)
│   ├── share/resource.go                                [NEW]  ResourceKind (patient|family_member) + Resource{Kind,ID}
│   ├── share/status.go                                  [NEW]  InvitationStatus + the transition table
│   ├── share/transition.go                              [NEW]  Transition(from, event) — the only state machine
│   ├── share/errors.go                                  [NEW]  ErrAlreadyShared, ErrInvitationOutstanding, ErrSelfShare,
│   │                                                            ErrNotAddressedToYou, ErrLapsed, *TransitionError
│   ├── share/*_test.go                                  [NEW]  table-driven, exhaustive over the transition table
│   └── access/access.go                                 [EDIT] Grant gains ViaShare, Note, ExpiresAt; Permission unchanged
│
├── internal/service/
│   ├── share/service.go                                 [NEW]  Invite, ListShares, ChangeShare, Revoke, Leave
│   ├── share/invitations.go                             [NEW]  ListInvitations, Preview, Respond, Cancel, Withdraw
│   ├── share/tidy.go                                    [NEW]  Tidy(ctx) — lapse audit + retention removal (006 schedules it)
│   ├── share/ports.go                                   [NEW]  Repository, InvitationRepository, Authorizer, Auditor,
│   │                                                            Mailer, Notifier, Clock, TokenMinter, UserDirectory
│   ├── share/sharetest/{fake.go,contract.go,fixtures.go} [NEW] fakes + the two contract suites
│   ├── share/*_test.go                                  [NEW]  unit, t.Parallel(), injected Clock
│   ├── access/authorizer.go                             [EDIT] the widening: owner → grant → ErrNotFound
│   ├── access/ports.go                                  [EDIT] + ShareReader (ActiveFor, ActivePatientsFor)
│   ├── access/accesstest/contract.go                    [NEW]  the ShareReader contract suite
│   ├── access/authorizer_test.go                        [EDIT] + grantee/revoked/lapsed/disabled/view-write rows
│   ├── patient/service.go                               [EDIT] List returns owned ∪ shared with both counts (FR-055)
│   ├── patient/active.go                                [EDIT] active patient resolves to null on grant loss (FR-046)
│   ├── tag/service.go                                   [EDIT] owner's labels on a shared record; apply-only for editors (FR-059)
│   ├── attachment/service.go                            [EDIT] viewer may read/download, not write (FR-061); read_sensitive
│   ├── attachment/serve.go                              [EDIT] read_sensitive now conditional on Grant != PermOwn (004 FR-076)
│   ├── search/service.go                                [EDIT] accessible-patient scoping (FR-057)
│   └── audit/writer.go                                  [EDIT] + the five new actions, + Reason on a denial
│
├── internal/store/
│   ├── migrations/1756xxx100_shares.go                  [NEW]
│   ├── migrations/1756xxx200_invitations.go             [NEW]
│   ├── migrations/1756xxx300_audit_vocab_sharing.go     [NEW]  + reason column; + share_update, share_leave, invite_cancel, invite_withdraw, invite_expire
│   ├── migrations/assertions.go                         [EDIT] nil-rule + no-file-field + no-`IS NULL` assertions
│   ├── share/{repo.go,mapper.go,query.go,repo_test.go}  [NEW]  the `= ''` predicates live here and nowhere else
│   ├── invitation/{repo.go,mapper.go,repo_test.go}      [NEW]
│   ├── access/sharereader.go                            [NEW]  ActiveFor / ActivePatientsFor, one indexed query each
│   └── patient/repo.go                                  [EDIT] ListAccessible(owner ∪ grants) with a stable cursor
│
├── internal/platform/
│   ├── pb/boot.go                                       [EDIT] SMTP-unconfigured boot warning (FR-022)
│   ├── pb/hooks.go                                      [EDIT] post-commit audit hooks for shares + invitations
│   └── mail/{mailer.go,mailer_test.go}                  [NEW]  share.Mailer over app.NewMailClient() + templ  [PB]
│
├── internal/realtime/
│   ├── event.go                                         [EDIT] Event gains Type and UserID (record | access | notice)
│   ├── hub.go                                           [EDIT] per-user fan-out alongside per-patient
│   └── hub_test.go                                      [EDIT] delivery, isolation, unsubscribe, no goroutine leak
│
├── internal/httproute/routes.go                         [EDIT] 10 API + 3 page entries, each with landmark + smokeUrl
│
├── internal/web/
│   ├── api/shares.go(+_test,+_http_test)                [NEW]  ops 58–62 + the authorization matrix
│   ├── api/invitations.go(+_test,+_http_test)           [NEW]  ops 63–66
│   ├── api/dto_sharing.go                               [NEW]  Share, ShareSummary, InvitationSummary, InvitationPreview
│   ├── api/patients.go                                  [EDIT] shared_by / level / shared_count on the list DTO
│   ├── errors.go                                        [EDIT] forbidden_view_only mapping (the one 403)
│   ├── stream/notifications.go(+_test)                  [NEW]  op 67, via newStream(); re-authorises per event
│   ├── stream/records.go                                [EDIT] subscribes to access events; cuts and explains on revoke
│   ├── page/{sharing.go,invitations.go,invite.go}       [NEW]  3 pages; /invite is public
│   ├── page/patients.go                                 [EDIT] owned/shared grouping, counts, badges
│   └── views/
│       ├── sharing/{granted.templ,received.templ,row.templ,leveldialog.templ,revokeconfirm.templ}  [NEW]
│       ├── sharing/sharedrawer.templ                    [NEW]  opened by a Datastar signal from the chart, not a route
│       ├── invitations/{received.templ,sent.templ,respond.templ,preview.templ}                     [NEW]
│       ├── invitations/landing.templ                    [NEW]  the public /invite/{token} page
│       ├── mail/invitation.templ(+_test)                [NEW]  the email body — same renderer as the web
│       ├── shell/layout.templ                           [EDIT] invitation badge inside #primary-nav; notices patch into 001's #toast
│       ├── shell/notice.templ                           [NEW]  the contentless toast
│       ├── patients/list.templ                          [EDIT] owned/shared groups, "shared by X (view)" badge
│       └── ids/ids.go                                   [EDIT] deterministic ids for the new components
│
├── internal/cli/{seed.go,purge.go}                      [EDIT] sharing fixtures; purge gains the share/invitation tidy
├── internal/testsupport/
│   ├── phileak/exercise.go                              [EDIT] + address, note, display-name and token sentinels
│   └── scale/generate.go                                [EDIT] + 200 grants / 20 people / 50 shared people (SC-014)
│
├── api/openapi.json                                     [EDIT] regenerated, committed, diffed
├── e2e/specs/{sharing.spec.ts,invitations.spec.ts,invite-landing.spec.ts,sharing-live.spec.ts,sharing-keyboard.spec.ts}  [NEW]
├── e2e/routes.gate.spec.ts                              [EDIT] 3 more page routes
└── .golangci.yml                                        [EDIT] depguard allowlist for internal/platform/mail
```

**Structure Decision**: the single-project Go layout from phase 001 is unchanged; this phase
populates it and widens one existing checkpoint. The only structural addition is
`internal/platform/mail` — a `[PB]` adapter package that exists because `internal/service/share`
may not import PocketBase or templ, and an email is both.

---

## Deviations from the shared design contract

The contract is binding on **design**. Two departures, both additive, both from its *page* and
*vocabulary* tables rather than from any design decision:

| Contract says | This plan does | Why |
|---|---|---|
| Phase 005 has **2 pages** (`/sharing`, `/invitations`) | **3** — adds public `/invite/{token}` | US5 scenario 1 requires the emailed link to land somewhere that shows who invited you, what kind of thing, how many items, the level, the lapse date and the note, and offers account creation — to somebody who is **not signed in**. Pointing the email at `/invitations` requires a session that does not exist yet and loses the preview outright; pointing it at op 64 shows a human raw JSON. Cost: 1 route, 2 smoke cases, **0** API operations (it renders op 64). |
| `audit_events.action` vocabulary | Extended additively by **5** values: `share_update`, `share_leave`, `invite_cancel`, `invite_withdraw`, `invite_expire` | FR-068 requires the trail to distinguish an owner's revocation from a grantee's departure from a lapse, and a cancel from a withdraw from an expiry. The contract's list already holds `share_grant`, `share_revoke`, `share_expire`, `invite_send`, `invite_respond` — declared in full by phase 001's migration — and these five complete it. `target_kind` already holds `share` and `invitation` and is not changed. After this phase: **twenty-six** actions, **twenty-seven** target kinds, asserted set-equal by the migration's test rather than as a delta. |
| `audit_events` gains no columns | It gains **one**: `reason`, a bounded token ≤40 | FR-069 requires a refusal to record *why*, and `invite_respond` records *how* an invitation was answered. Both were written against a field that no migration created — a runtime failure on the first refusal, not a documentation gap. It is `text` bounded by a closed Go type, exactly as phase 006's `export_jobs.error_code` is, so a token owned by another subsystem cannot fail `SelectField` validation. Closes cross-artifact finding **C2**. |

Everything else follows the contract verbatim: the `shares` and `invitations` field lists, ops
58–67 with their paths and semantics, `resource_kind ∈ {patient, family_member}`,
`level ∈ {view, edit}`, the six invitation statuses, `revoked_at`/`revoked_by` instead of
`is_active`, expiry as a read-path predicate, and one hub without a broker interface.

**Op 58 keeps its path.** `POST /api/v1/shares` creates an *invitation*, never a grant. This reads
oddly next to §2.1 rule 9 and was reconsidered against `POST /api/v1/invitations`; the contract's
choice is kept because the caller's act genuinely is "share this with that person" and there is no
other way to create a share, so a `POST` on `/shares` cannot be mistaken for a direct-grant path
that exists. It answers `201` with the invitation DTO and
`Location: /api/v1/invitations/{id}`, which is stated explicitly in `contracts/shares.md` so no
client has to guess. Recorded as research [D-12](./research.md#d-12).

Net effect on the contract's headline numbers (SHARED-DESIGN §§1.6/2.3/3.1, cited not
re-derived): operations **69 running** of a suite total of **94**; collections **+2** as
planned; pages **+1**.

---

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| **A second event shape on the realtime hub** — `Event` gains a `Type` and a `UserID`, and the hub fans out per-user as well as per-patient, against Principle I ("a channel and a map") | FR-045 and SC-005 require an already-open screen to stop and *say why* within 5 seconds of a revoke, and FR-064/SC-017 require a notice to reach an account without a refresh. Both are events addressed to a **person**, not to a patient; the existing per-patient topic cannot carry them, because the whole point is that the recipient just lost their claim to that patient. The addition is one field on a struct, one map keyed by user id, and the same `context`-bounded subscriber lifetime that already exists. | **(a) Poll from the browser** — a timer per open tab per user, 5 s resolution across every session, under a CSP that already forbids the machinery, and it turns a revocation into a request storm. **(b) Re-authorise only on the next record event** — a quiet chart never re-authorises, so a revoked grantee keeps a populated screen indefinitely; that is the exact silent-freeze FR-045 names as unacceptable. **(c) A separate notification hub** — two hubs, two lifetimes, two shutdown paths and two places to leak a goroutine, to avoid one field. **(d) A broker interface** — forbidden outright by the Technology Constraints, and it would buy nothing here. |
| **Two conditional relation columns on one row** — `shares.patient` and `shares.family_member`, exactly one of which is set, against Principle I (a nullable pair is a smell) and Principle II (a branch on kind) | The alternative to a discriminated row is the upstream design: two tables, two routers, two invite paths, two revoke shapes and two "shared with me" lists — 20 operations, two authorization code paths, and FR-001 forbidding exactly that. Keeping two **typed relation** columns rather than one polymorphic `resource_id` text column is what buys PocketBase's `CascadeDelete` for FR-048 and FR-053 for free, and referential integrity that a polymorphic column cannot have. The branch exists in exactly two places — `Resource.Validate` and the repository's column choice — and a `CHECK`-equivalent invariant test proves no row can ever have both or neither. | **(a) One polymorphic `resource_id` text column** — the shape criticised in `attachments`, and here it forfeits cascade delete, so FR-048/FR-053 become two hand-written cleanup paths that a future third resource kind can silently miss. **(b) Two collections** — FR-001 forbids it in as many words, and it is the single largest defect in the system being reimagined. **(c) A relation to a synthetic `shareables` table** — a join, a row and a lifecycle per shareable thing, to avoid one always-empty column. |

---

## Phase Exit Criteria

**Criterion 0, before any of the below**: `specs/005-sharing-and-collaboration/traceability.md` (T140a) joins every functional requirement to the task ids that satisfy it, every acceptance scenario to its test and every success criterion to its task or to a criterion below, with no empty row. The join is mechanical, not a prose claim: an unmapped requirement, or a success criterion neither mapped nor marked `[outcome metric]` in `spec.md`, fails the phase (cross-artifact finding M7).

This phase is done when, and only when:

1. All 47 acceptance scenarios exist as named automated tests and pass (FR-076, SC-018).
2. The authorization matrix suite covers **every** patient-scoped route in `medikube routes`, and
   `internal/service/access/coverage_test.go` fails the build if one is missing (FR-077).
3. The family-history isolation suite passes in full (FR-078, SC-008).
4. The PHI-leak exercise reports zero occurrences of an address, a note, a display name or an
   invitation token across logs, metrics, traces and Sentry (FR-080, SC-016).
5. The Playwright gate is green over all three new pages and every widened page at both viewports,
   including the empty-state account, the two-session live spec and the keyboard spec.
6. `api/openapi.json` is regenerated, committed and diff-clean; the route-inventory gate passes.
7. The scale fixture (200 grants / 20 people / 50 shared people) meets SC-014's 2 s budget with
   zero repeated or skipped rows while grants are being created and revoked underneath the pager.
8. CI is green: format, vet, golangci-lint v2, `go test -race -count=1 ./...`, the OpenAPI and route
   gates, the container build, and the browser gate.
