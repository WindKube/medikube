# Phase 0 Research: Walking Skeleton

Every decision this phase makes, with the evidence behind it. Citations of the form
`apis/serve.go:145` are line references into `github.com/pocketbase/pocketbase@v0.40.1` as
downloaded into the module cache and read directly. Where a dossier and this document disagree,
[`VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) wins and is cited by name wherever it settles a question. Where the
shared design contract and this specification disagree on *scope*, this phase's charter governs
and the departure is recorded in [plan.md](./plan.md)'s Deviations table.

This is the first phase, so a decision recorded here is a decision for the whole application.
Forty decisions follow. **No NEEDS CLARIFICATION survives this document.**

---

## A. Scope and sequencing

### D-01 — This phase's charter governs where it disagrees with the shared design contract's phase table

**Decision.** [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) §0 allocates patients, multi-patient switching, and allergies +
emergency contacts to phase 001. The accepted specification set does not decompose that way:
`001-walking-skeleton` delivers accounts plus **medications** as the single clinical record type
owned directly by the account, and `002-patient-core` introduces patients and re-anchors
medications onto them. **The specifications govern on allocation.** Everything else in the
contract — the collection field lists, the API conventions, the package layout, the seams, the
error taxonomy, the enum vocabularies, the landmark strings — is binding and followed to the
letter.

**Rationale.** The specification says so in its own Assumptions: *"Where that contract's phase
table allocates medications to a later phase, this phase's charter governs, and the later phase
adds the remaining clinical record types."* Phases 002, 003, 004 and 005 have each already
recorded the mirror-image decision in their own plans. Four accepted, tasked phases resting on an
allocation is stronger evidence of intent than one unamended table. The contract's own preamble
makes it binding "until it is amended"; the accepted charters are that amendment for scope
purposes.

**Alternatives considered.**

- *Follow the contract's table and rebuild this phase around allergies and patients.* Rejected:
  it orphans `001-walking-skeleton/spec.md` — 72 requirements, 15 success criteria, six user
  stories, and the best-written document in the suite — and it contradicts phase 002's plan,
  which opens with "Phase 001 delivered a working instance in which an account owns its
  medications directly."
- *Amend [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) as part of this phase.* Out of scope for a phase plan and
  unnecessary; the Deviations table in `plan.md` records the mapping in the same format phases
  003–005 use. The suite-level amendment is worth doing once, separately, and is raised as such.

**Consequences carried forward.** This phase creates three collections, not five. `patients`,
`practitioners`, `facilities`, `tags`, `catalog_*`, `allergies` and `emergency_contacts` belong
to later phases and nothing here anticipates them.

### D-02 — Go 1.27, not the monorepo's 1.26.5

**Decision.** `go.mod` declares `module medikube`, `go 1.27` and an explicit `toolchain go1.27.x`.
CI must not set `GOTOOLCHAIN=local`. `golangci-lint` is **v2**.

**Rationale.** VERIFIED-SOURCE-FACTS **FACT 0**, established by building rather than by reading:
PocketBase v0.40.1's `go.mod` line 3 declares `go 1.27`; 67 non-test files import the Go 1.27
stdlib package `encoding/json/v2`, 15 of them under `core/` and `apis/`, with no build tags and
no fallback path; a further 10 import `encoding/json/jsontext`. `GOTOOLCHAIN=local go build` on
1.26.5 fails with `go.mod requires go >= 1.27`, and go1.27.0 builds the module clean. This is not
a preference and it cannot be worked around.

MediKube is therefore the only project in this monorepo off the 1.26.5 standard that `arc-ui`,
`gmod`, `appbase` and `medikeep-mcp` share. That divergence is deliberate, is recorded in the
constitution's Technology Constraints, and belongs in `CLAUDE.md` so the next contributor does
not "fix" it.

**Alternatives considered.** *Pin PocketBase to v0.39.x to stay on 1.26.5.* Rejected in the
constitution: it forfeits v0.40's filesystem hooks and backup fixes, puts MediKube on a branch
upstream will stop patching, and invalidates every file:line citation the six plans rest on.

**Trap.** With a `toolchain` directive present, `GOTOOLCHAIN=local` on a 1.26 machine fails with
"built with go1.26 < targeted go1.27", which reads like a broken build rather than a policy
choice. The Taskfile, the Dockerfile and the CI workflow each get an explicit comment.

### D-03 — Medications is the single kind, and it carries no reference relations in this phase

**Decision.** One registered kind: `medication`. Its collection carries `owner`, `name`,
`alternative_name`, `type`, `dosage`, `frequency`, `route`, `indication`, `started_on`,
`ended_on`, `status`, `side_effects`, `notes`. It carries **no** `practitioner`, **no**
`pharmacy`, **no** `tags` and **no** reminder fields.

**Rationale.** [`RECONCILIATION.md`](../research/RECONCILIATION.md) C8 warns that the walking skeleton's first record type must
have no reference foreign keys, or the phase drags the whole reference graph in and stops being a
skeleton — and it names medications as the wrong choice for exactly that reason. The
specification overrides the *choice* of kind but not the *reasoning*, and it resolves the tension
by stripping the references: its Assumptions state plainly that "a prescriber, a pharmacy and
tags on a medication … are reference data that phase 002 introduces; adding placeholder versions
now would be work thrown away." Phase 002's `medications-rescope.md` adds `practitioner` and
`pharmacy`; phase 003 adds `tags`. Reminders wait for notifications, which phase 005 delivers,
because recording that a medication should prompt somebody is only useful once something can
deliver the prompt.

The field list is otherwise the shared design contract's `medication` row verbatim, so phase 003
extends it rather than rewriting it.

**Alternatives considered.** *Ship `practitioner` as free text now and migrate later.* Rejected:
a free-text prescriber is data a person enters and then loses when the field becomes a relation.
*Ship the reminder fields unused.* Rejected under Principle I — unused columns in a medical
schema are a liability and a lie about what the product does.

### D-04 — Password recovery by email and email confirmation ARE in this phase; external sign-in belongs to phase 006

**Decision, superseding the first draft of this entry.** This phase builds password recovery by
email (contract operations 7 and 8) and email confirmation (two operations the contract never
allocated, numbered 94 and 95 in its additive scheme), together with the three public pages
`/forgot-password`, `/reset-password/{token}` and `/verify-email/{token}`. **Sign-in through an
external identity provider (operation 4) belongs to phase 006**, with the operator surface that
configures providers. `OAuth2.Enabled` stays `false` here.

**What the first draft said, and why it was wrong.** It deferred all three with the specification's
original Assumptions and recorded an operator workaround — a superuser resets a password through
the admin UI. The cross-artifact analysis then found (finding **H7**, previously H4) that no later
phase claimed any of the three, so three contract operations and three contract pages belonged to
nobody, while phase 002's Assumptions asserted that this phase delivered them. Two things settled
it against the deferral:

1. **The workaround is not acceptable in this product.** A self-hosted application holding
   somebody's medical history, in which a forgotten password can only be recovered by the person
   who runs the machine, is broken on the day it ships — and for the common case where the person
   who runs the machine *is* the person who is locked out, it is not a recovery path at all.
2. **The cost is wiring, not construction.** Verified in v0.40.1's source:
   `apis/record_auth.go` already binds `request-password-reset`, `confirm-password-reset`,
   `request-verification` and `confirm-verification`; `mails.SendRecordPasswordReset` and
   `mails.SendRecordVerification` build the message from the collection's own templates and send
   it; `app.FindAuthRecordByToken(token, core.TokenTypePasswordReset)` validates the token; and
   `SetPassword` + `Save` rotates `tokenKey`, which is the mechanism FR-010 already depends on, so
   "a recovery ends every other session" needs no new code. Principle V's rule — MediKube MUST NOT
   rebuild what PocketBase provides — points at building these, not at skipping them.

**What this phase pays for it.** Four thin DTO handlers, three pages with three landmarks and
three smoke cases, four OpenAPI entries, sixteen tasks (T223a–T223p), **zero** new audit action
values (a completed recovery is a `password_change`; a confirmed address is an `update`), and zero
new collections or fields — `verified` is PocketBase's and was already there.

**The dependency that must be stated rather than hidden.** Both flows need outgoing mail, which
lives in PocketBase's `Settings()` store — the carve-out the constitution's Technology Constraints
make precisely because it is platform state and not MediKube configuration. When `SMTP.Enabled` is
false, `core.BaseApp.NewMailClient()` returns `&mailer.Sendmail{}`, which shells out to a local
`sendmail` binary the distroless image does not contain, so the send **fails**. FR-076 turns that
into specified behaviour rather than a surprise: `503 mail_unconfigured` to the person, one log
line per burst rather than per attempt, and a third boot warning alongside the superuser-MFA and
IP-allowlist warnings until the operator fixes it. This is the same root cause as risk **R10**
(invitation deliverability, phase 005) and the same answer.

**Alternatives considered.** *Keep the deferral and let a later phase claim the three.* Rejected —
that was the state the analysis found, and three phases had already gone by without claiming them.
*Move all three, including OAuth2, into this phase.* Rejected: external sign-in needs provider
configuration, a linking flow and a redirect surface, none of which this phase has a requirement
for, and phase 006 already owns the operator surface where providers are configured. *Build
recovery without confirmation.* Rejected: confirmation is the same mechanism, the same page shape
and the same three sentences of specification, and a reset flow that will mark an address verified
anyway (PocketBase does, when the token's email claim matches) with no confirmation surface
anywhere is incoherent.

### D-05 — One spelling of a record kind, generated from one Go constant, and the path segment is plural

**Decision.** `internal/domain/kind` owns `Kind`, and each value carries three spellings from one
declaration:

| `kind.Kind` (enum value, `snake_case`) | Path segment (plural kebab-case) | Collection |
|---|---|---|
| `medication` | `medications` | `medications` |

So the routes are `/api/v1/records/medications` and `/api/v1/records/medications/{id}`, and the
pages are `/medications` and `/medications/{id}`.

**Rationale.** Shared design contract §2.1 rule 2 mandates exactly this, and states the reason:
upstream had three spellings of `lab_result` in one API. Phase 003's kind registry already
declares `medication` / `medications` / `medications`. Phase 002's contracts and quickstart once
**used the singular** `/api/v1/records/medication` throughout, while its own `contracts/README.md`
**stated** the plural rule two lines before breaking it. **They were corrected on 2026-08-27 and
now read `/records/medications` throughout** — `grep -rn 'records/medication\b' specs/002-patient-core/`
returns nothing. This closed cross-artifact finding **H1** (analysis run 2026-08-27; it was **H2**
in the run before it). The constant is created in this phase, so the spelling was settled in this
phase, and the plural is the one that satisfies the rule and matches phase 003.

`kind_test.go` asserts the mapping is total, injective in both directions, and that every value
round-trips through all three spellings. Two segments in later phases are deliberately not
mechanical plurals (`insurance`, `family-history`), which is precisely why the segment is a
declared constant and not a derived string.

**Consequence for phase 002.** Its `contracts/medications-rescope.md`,
`contracts/active-patient.md` and `quickstart.md` **were** corrected on 2026-08-27 from `/records/medication` to
`/records/medications`. That is a documentation fix; under Principle IX the alternative is an
unexplained breaking OpenAPI diff between 002 and 003 that no plan predicts.

---

## B. The platform

### D-06 — PocketBase is constructed with `NewWithConfig`, and every option is chosen deliberately

**Decision.**

```
pocketbase.NewWithConfig(pocketbase.Config{
    DefaultDataDir:  cfg.DataDir,
    DefaultDev:      cfg.Dev,
    HideStartBanner: true,
    DBConnect:       obs.InstrumentedDBConnect(...),
})
```

`InstallerFunc` is set to `nil` on the `ServeEvent`, and `jsvm` is never registered.

**Rationale.** `HideStartBanner: true` because the banner is PocketBase's own stdout write and
would be the one line in the process that is not JSON (Principle VI). `DefaultDev` gates
`Automigrate` and PocketBase's console log printing; `getLoggerMinLevel` returns `-99999` in dev,
so a dev build prints everything regardless of settings, which is fine and must not be relied on
in production. `DBConnect` is the injection point for otelsql (D-30). `InstallerFunc = nil` skips
PocketBase's first-run installer page, which is what makes a freshly seeded instance
deterministic for the Playwright gate — otherwise the first navigation lands on an installer.

**Alternatives considered.** *`pocketbase.New()` and configure afterwards.* Rejected: `DBConnect`
and `DefaultDataDir` must be set before bootstrap, and the config struct is the documented way.

### D-07 — The lockdown: five nil rules, `Batch.Enabled = false`, a `-1019` middleware, and a boot assertion that refuses to start

**Decision.** Four mechanisms, all of them, and each doing a different job:

1. **All five API rules `nil`** (`ListRule`, `ViewRule`, `CreateRule`, `UpdateRule`,
   `DeleteRule`) on every non-system collection, set in the migrations. `nil` means
   superuser-only. **`AuthRule` is not one of the five** and stays `""` on `users`, or
   PocketBase-native authentication dies outright (FACT 2).
2. **`Settings().Batch.Enabled = false`**, applied at boot.
3. **A `-1019` root middleware** — after `loadAuthToken` at `-1020`, so `e.Auth` is populated —
   returning `404` for any path with the `/api/collections/` prefix that also contains
   `/records`, and for `/api/batch`, when the caller is not a superuser.
4. **A boot assertion** that walks every non-system collection and **refuses to start** if any of
   the five rules is non-nil, or if any `FileField` is not `Protected: true`. This phase has no
   file fields; the assertion is written anyway, because phase 002 adds `patients.photo` and the
   assertion existing from the start is what makes it a gate rather than a retrofit.

**Rationale.** Mechanism 1 is the correct primary: rules are evaluated only in the `apis` HTTP
layer, never by `app.FindRecordById` / `app.Save` / `app.RecordQuery`, so internal Go access is
completely unaffected. That single property is what makes the whole MediKube design viable. The
admin UI survives because superusers bypass rules by design.

Mechanism 3 buys opacity — a 404 rather than a 403 tells an enumerator nothing — and it is scoped
precisely, because a blanket block on `/api/collections/*` would kill `auth-with-password`,
`auth-refresh` and every other PocketBase auth route, which share the prefix (FACT 2). The
middleware matches only paths containing `/records`.

`/api/batch` deserves its own mention: it is a transactional multiplexer that re-uses the record
CRUD handler bodies, so nil rules *do* cover it. It is disabled anyway — a transaction-holding
surface with a ~128 MB default body limit and no benefit to an application whose writes all go
through hand-written endpoints.

**Alternatives considered.** *Express sharing and ownership in the API rules as "defence in
depth"* — the approach the platform dossier proposed and [`RECONCILIATION.md`](../research/RECONCILIATION.md) C5 rejects. A
`viewRule` expressive enough to encode ownership does not add depth; it **re-opens**
`GET /api/collections/medications/records` as a second, undocumented, un-versioned public API
returning raw collection records with no DTO boundary. That is precisely what the hand-written
API exists to prevent. Defence in depth comes from the assertion, the middleware, and a
`tests.ApiScenario` per collection proving the CRUD route 404s for an ordinary user — written for
*every* collection, because it is the regression test for the whole design.

### D-08 — The record route family, and how the discriminated `oneOf` is gated with only one kind

**Decision.** Six operations serve every clinical kind:

```
GET    /api/v1/records                       cross-kind list
GET    /api/v1/records/{kind}                list one kind
POST   /api/v1/records/{kind}                create
GET    /api/v1/records/{kind}/{id}           read
PATCH  /api/v1/records/{kind}/{id}           update   (If-Match required)
DELETE /api/v1/records/{kind}/{id}           delete   (If-Match required)
```

`{kind}` is an OpenAPI `enum` of the registered path segments; request and response bodies are a
`oneOf` with `kind` as the discriminator, so every kind keeps its own fully typed DTO — the
polymorphism is in the routing table, not in the schema. The gate that asserts every registered
kind appears in the discriminator mapping runs against a **synthetic two-kind registry** built in
`internal/openapi/gate_test.go`, not against the production registry.

**Rationale.** Shared design risk **R1** was named "the one risk that could invalidate a phase":
if a discriminated `oneOf` cannot be generated and gated, the family collapses into per-kind
routes, the suite's operation count goes from 90 to roughly 150, and phase 003 grows from 3
routes to 62. VERIFIED-SOURCE-FACTS **FACT 9** closed it ahead of this phase by building and
running the thing: `kin-openapi` v0.144.0, two kinds registered, one `Record` schema whose
`oneOf` lists them and whose `discriminator` is `{propertyName: "kind", mapping: {...}}`, one
path `POST /api/v1/records/{kind}`, document marshalled, reloaded through `openapi3.NewLoader()`
and validated, gate asserting both kinds present in the mapping. **R1 is CLOSED and this phase
records it as closed with that evidence, rather than re-opening it.**

The two-kind fixture is the interesting part. This phase ships **one** kind, and a `oneOf` with a
single branch proves nothing about the mechanism phase 003 bets thirteen kinds on. So
`internal/records/recordstest` provides `FakeKindService`, a second fully registered synthetic
kind used only by `internal/records/registry_test.go` and `internal/openapi/gate_test.go`. It
gives the registry two implementations of `records.Service` on day one, which is what Principle
I's two-implementations clause actually asks for (plan Complexity Tracking CT-2), and it makes
the discriminator gate meaningful. A test asserts the fake kind is never registered in a
production build.

**Two API notes that each cost a compile error first**, recorded so they do not cost one twice:
`Discriminator.Mapping` is `map[string]openapi3.MappingRef`, not `map[string]string`; and
`MappingRef` is a struct (an alias of `SchemaRef`), so the value is
`openapi3.MappingRef{Ref: "#/components/schemas/Record_medications"}`, not a bare string. It
marshals to a plain string via `MarshalText`.

**Alternatives considered.** *Per-kind routes now, unify later.* Rejected: it is the 150-operation
outcome, and phases 002–005 are already written against the six-operation family. *Skip the
discriminator and use a free-form object.* Rejected: it discards the typed DTO boundary
Principle V requires and makes the OpenAPI document useless to a client.

### D-09 — Registering a route and describing it are one indivisible call

**Decision.** `internal/httproute.Registry`. `Handle(spec Route, handler)` records both;
`Bind(se *core.ServeEvent)` is the **only** place a route reaches PocketBase's router; `Routes()`
returns the inventory. `Route` carries `Method`, `Path`, `Kind` (`page` | `api` | `stream` |
`asset` | `external`), `Auth` (`public` | `user` | `admin`), `Landmark`, `OpID` and `SmokeURL`.
`Handle` **panics** at registration when a `page` route declares no `Landmark` or no `SmokeURL`,
or an `api`/`stream` route declares no `OpID`, or a path ends in `/`, or a `Method + Path` pair is
registered twice.

**Rationale.** The route table cannot be recovered from the router after the fact:
`RouterGroup.children` is unexported, and Go 1.27's `http.ServeMux` still exposes no
pattern-enumeration API — only `Handler(*Request)`, which requires you to already know the path.
A hand-maintained list is forbidden by Principle VIII because it rots silently. A registry that
routes flow *through* is the only sound answer, and separating description from binding is what
lets `medikube routes` be a pure function of the binary — no database, no port, no migrations, so
Playwright can call it during its collection phase before anything is running.

The registration-time panic is the strongest link in the whole Principle VIII chain: a page
without a landmark cannot boot, so it cannot be shipped, so it cannot escape the gate.

**Trailing slashes.** PocketBase has done no trailing-slash normalisation since v0.23, so
`/api/v1/records/` and `/api/v1/records` are different routes. The registry rejects a path ending
in `/`, and the smoke gate covers it.

**Alternatives considered.** *Reflect the router.* Impossible, as above. *Generate the registry
from the OpenAPI document.* Backwards — the document is the artifact, the code is the source.
*A `//go:generate` comment scanner.* Rejected: a second parser for Go source, defeated by any
indirection.

### D-10 — `kin-openapi` v0.144.0, and the gate must marshal-then-load

**Decision.** The OpenAPI 3 document is **built** from the route registry with
`github.com/getkin/kin-openapi/openapi3` and written to `api/openapi.json` by `medikube openapi`.
The staleness gate compares the generated bytes with the committed file as a JSON equality. The
validity gate marshals the document and reloads it through `openapi3.NewLoader()` before
validating.

**Rationale.** `kin-openapi` is pure Go, cgo-free, models the OpenAPI 3 object graph directly,
and — critically — **serves no HTTP and adds no router**, so it does not collide with the
constitution's ban on a second HTTP router or OpenAPI-serving framework. That ban is aimed at
`huma`, which owns routing; a document-construction library is a different thing and the
distinction is worth stating once rather than re-arguing.

The marshal-then-load requirement is not stylistic: validating a programmatically built document
**in place** fails with "found unresolved ref", because a constructed `SchemaRef` carries only
`Ref` and no `Value`. The round trip is also what a real consumer does, so it is the honest test
(FACT 9, step 4).

The committed document is diffed in the pull request, which is what makes an unintended API change
a reviewable diff rather than a surprise for a client (Principle IX, FR-064).

**Alternatives considered.** *Hand-write the OpenAPI document.* Rejected outright — it is a
hand-maintained list by another name. *`swaggo`-style comment annotations.* Rejected: a second
source of truth that a compiler does not check.

### D-11 — Migrations are hand-written, reversible by API, and `Automigrate` is on only in dev

**Decision.** Three migrations registered into `core.AppMigrations` via
`migrations.Register(up, down, filename)`. `migratecmd.MustRegister(app, app.RootCmd,
migratecmd.Config{Automigrate: cfg.Dev})`.

**Rationale.** The `Register` signature **requires** both an up and a down function
(VERIFIED-SOURCE-FACTS FACT 8), so Principle IX's reversibility rule is enforced by the API
itself; the only escape is a `down` that returns nil, which is what the "document the
irreversibility in the migration file" clause covers. None of this phase's three needs that
escape — all three genuinely reverse.

`Automigrate` in production would try to write `.go` files into a directory that does not exist
in a distroless image. It is gated on `MEDIKUBE_DEV`.

All pending migrations share **one transaction** (`core/migrations_runner.go:129-131` wraps them
in `AuxRunInTransaction(RunInTransaction(...))`), so this phase's three either all land or none
do. There is no half-migrated state to design for, which is also what makes FR-063 ("start
successfully against an empty storage location") a single assertion rather than a matrix.

**Uniqueness is an index, not a field option.** PocketBase has no per-field `Unique`; it is
`collection.AddIndex(name, unique, cols, where)`. That is an upgrade rather than a limitation —
case-insensitive unique email is `AddIndex("idx_users_email_lower", true, "LOWER(email)", "")`,
and partial indexes come free.

### D-12 — Cobra subcommands on PocketBase's `RootCmd`, and the two traps

**Decision.** `medikube serve` and `medikube migrate` are PocketBase's own. MediKube adds `seed`,
`routes`, `openapi` and `healthcheck` with `app.RootCmd.AddCommand(...)` before `app.Start()`.
No second CLI framework. **No `medikube purge`.**

**Rationale.** `app.RootCmd` is a real `*cobra.Command` (FACT 6), and a custom subcommand receives
a fully bootstrapped app — database open, settings loaded, migrations applied — because
`skipBootstrap()` only skips for help, version and unknown commands. `medikube routes` and
`medikube openapi` deliberately do **not** need that: they are built from the registry, which is a
pure function of the binary.

**Trap 1:** the root sets `FParseErrWhitelist{UnknownFlags: true}`, so a typo'd flag is silently
tolerated rather than rejected. Every MediKube subcommand validates its own flags in `RunE`.
**Trap 2:** `RootCmd.SetErr(&nopWrite{})` discards cobra's error output, so `main` prints the
error itself and sets `SilenceUsage: true`.

**Why no `purge` command.** FR-037 requires audit entries to be "purged automatically" after a
configured retention period. A PocketBase cron does that (D-21). A manual trigger is a knob no
requirement asks for; the shared design contract names one, and Principle I says do not build it
until something needs it. Phase 004 introduces the attachment trash and can add the command then,
with two jobs to justify it.

**Observability init must be gated.** `InitSentry`, `InitTracing` and `StartMetricsServer` would
otherwise run for *every* subcommand, including `medikube --help`, opening an OTLP connection and a
metrics port to print usage. They are gated on the command actually being `serve`, via a
`PersistentPreRunE`.

---

## C. Identity and sessions

### D-13 — PocketBase-native authentication behind hand-written MediKube DTOs

**Decision.** `POST /api/v1/auth/login` looks the record up with
`app.FindAuthRecordByEmail("users", email)`, validates with `rec.ValidatePassword(pw)`, and
completes through `apis.RecordAuthResponse(e, rec, core.RequestInfoContextPasswordAuth, nil)`.
MediKube does not mint tokens itself, does not hash passwords itself, and does not re-implement
refresh.

**Rationale.** Principle V: a task that reimplements what PocketBase provides must be rejected
unless the plan records why PocketBase's version is unusable. It is not unusable — it is
excellent. `apis.RecordAuthResponse` is exported and is the supported way to finish an auth flow
from a custom route: it mints the token, fires `OnRecordAuthRequest` (which is how sign-in gets
audited, D-14), and records the auth origin. Rolling our own token would skip both.

The DTO wrapper exists because Principle V equally requires the public surface to be hand-written
`/api/v1` routes with explicit DTOs. So MediKube owns the request and response *shapes* and
PocketBase owns the *mechanism*, which is exactly the division the constitution asks for.

**The one thing that cannot be cheaply re-implemented is MFA/OTP** — those flows are woven
through `apis/record_auth_*.go` with their own events. MediKube's ordinary accounts do not use MFA
in this phase; superusers do, through PocketBase's own admin UI flow, which is untouched.

### D-14 — `OnRecordAuthRequest` is the sign-in audit seam, and the `forbidigo` ban carves the auth family out

**Decision.** Sign-in, failed sign-in and sign-out audit rows are written from
`OnRecordAuthRequest("users")` and from the login handler's failure branch — not from the login
handler's success branch. The `forbidigo` rule bans the **CRUD** `OnRecord*Request` family and
**explicitly permits** the auth family.

**Rationale.** [`RECONCILIATION.md`](../research/RECONCILIATION.md) C13 is right that `OnRecord*Request` hooks are bound inside
the built-in CRUD handlers, which the lockdown disables, so business logic placed there is
silently dead code — and it says to ban `OnRecord.*Request` "outside the auth package". The
distinction is load-bearing and the shared design contract states the ban without it. `bindRecordAuthApi`
is **not** locked down (FACT 2 lists its fourteen routes and they must stay reachable), so
`OnRecordAuthRequest` genuinely fires — and `apis.RecordAuthResponse` explicitly triggers it.

This matters beyond tidiness. Because PocketBase's native
`POST /api/collections/users/auth-with-password` remains reachable, there are **two** paths to a
valid session: MediKube's and PocketBase's. Auditing in MediKube's handler would leave the native
path unaudited, which breaks FR-036's "record every sign-in" the moment anyone uses it. Binding
the hook covers both paths with one piece of code. That is the correct answer and it is only
available because the auth family is carved out.

**Consequence.** The `forbidigo` pattern is written against the CRUD verbs specifically —
`OnRecord(Create|Update|Delete|View|List|Auth.*Confirm)Request` is *not* what is banned; the
banned set is the record-CRUD request hooks, enumerated by name, with a comment explaining that
the auth family is deliberately absent from the list.

### D-15 — The session is an HttpOnly cookie, translated to a bearer token by a middleware at priority `-1021`

**Decision.** Login sets `medikube_session` — `HttpOnly`, `Secure` (unless the public URL is
loopback), `SameSite=Lax`, `Path=/`, `Max-Age` from `MEDIKUBE_AUTH_SESSION_TTL` — carrying the
PocketBase auth token. A middleware bound at priority **`-1021`** copies the cookie value into the
`Authorization` header when that header is absent, so PocketBase's `loadAuthToken` at `-1020`
populates `e.Auth` exactly as it would for an API client. Logout clears the cookie.

**Rationale.** The pages are server-rendered and Datastar's `@get`/`@post` actions issue ordinary
fetches; a cookie is the only credential the browser will attach to a plain navigation, and a
navigation is how a person reaches `/medications`. `HttpOnly` keeps the token out of reach of any
script, which matters more than usual given `script-src 'unsafe-eval'` (D-35). The `-1021`
priority is the whole trick: hook priorities run low-first, so `-1021` is guaranteed to run
before `loadAuthToken`, and after it every downstream consumer — `e.Auth`, `RequireAuth`, the
superuser check, the lockdown middleware — behaves identically for a browser and for `curl`.
Nothing else in the stack needs to know a cookie exists.

`SameSite=Lax` rather than `Strict` so that following a link into MediKube from an email or a
bookmark lands signed in; there is no cross-site form post to protect, `form-action 'self'` is in
the CSP, and every mutation goes through a fetch that carries the same-origin check.

**Alternatives considered.** *Store the token in `localStorage` and attach it with Datastar.*
Rejected: it puts a session credential where any script can read it, needs JavaScript for a plain
navigation, and `data-persist` is a Pro attribute anyway. *A server-side session table.*
Rejected: PocketBase's token already is the session, and a second one would need its own
invalidation story.

### D-16 — Sign-out and password change invalidate every other session, by rotating the record's token key

**Decision.** `rec.RefreshTokenKey()` followed by a save. `SetPassword` does this implicitly.
After a password change the person who made it is re-issued a fresh token and stays signed in
where they are.

**Rationale.** FR-007 ("the ended session MUST NOT be usable again from anywhere it was still
open") and FR-010 ("every session issued before the change MUST stop working, while the person
who made the change remains signed in where they made it") describe token-key rotation exactly.
PocketBase signs each record's tokens with a key derived from the collection secret **and** that
record's `tokenKey`, so rotating it invalidates every outstanding token for that record in one
write. This is PocketBase's mechanism and MediKube does not need a revocation list.

The "remains signed in where they made it" half is the ordering detail that is easy to get
backwards: rotate, save, then mint a new token from the *saved* record and set the cookie, all
inside one transaction. A test signs in from two sessions, changes the password in one, and
asserts the first still works and the second is refused.

**Note on sign-out.** Rotating on sign-out means signing out on a phone also signs out the laptop.
FR-007 asks for exactly that ("the ended session MUST NOT be usable again from **anywhere** it
was still open"), so this is the specified behaviour rather than a side effect, and the settings
page says so in plain language.

### D-17 — An unknown address and a wrong password produce the identical refusal, in comparable time

**Decision.** Both return `401 unauthenticated` with the identical envelope and the identical
message. When the record is not found, the handler still performs a bcrypt comparison against a
fixed dummy hash before returning, so the two paths take comparable time.

**Rationale.** FR-005 says the two must be answered the same way "so that neither can be probed".
An identical *body* with a different *latency* is still an oracle — a missing-record path returns
in microseconds while a real bcrypt comparison takes tens of milliseconds, which is trivially
measurable over a LAN. The dummy-hash comparison is four lines and closes it.

A test asserts the two response bodies are byte-identical apart from `request_id` **and that the
unknown-address path really performs the dummy comparison** (T202, through a counting seam on the
hash comparer — the mechanism is deterministic, the latency is not, so the gate asserts the
mechanism; the latency is reported by the non-gating benchmark T202a, ANALYSIS N13). A second
test asserts both write a `login_failed` audit row carrying the attempted **actor id when known
and nothing identifying when not** — never the attempted email address, which is PHI-adjacent
under Principle VII and would put a real person's address in the trail of a typo.

### D-18 — Registration is closed by default, and the auth routes are rate limited

**Decision.** `MEDIKUBE_AUTH_REGISTRATION_OPEN` defaults to `false`. When closed, `/register`
renders inside the normal shell with a plain explanation, and `POST /api/v1/auth/register`
returns `403 registration_closed`. PocketBase's `Settings().RateLimits` is **enabled** at boot
with rules on `/api/v1/auth/login` and `/api/v1/auth/register`, and the SSE stream route exempted.

**Rationale.** The specification's Assumptions settle the default: "A self-hosted medical-records
instance reachable from the internet must not accept accounts from strangers by default." FR-002
requires the closed page to be "otherwise a normal page of the application", which is what keeps
the smoke gate honest — a bare 404 would fail the landmark assertion.

FR-006 requires repeated failed sign-ins to be slowed or blocked. PocketBase's rate limiter is
built in but **`RateLimits.Enabled` defaults to `false`**, which is not what a self-hosted medical
application wants. It is enabled and configured programmatically at boot from `internal/config`
values, never through the admin UI — so the environment remains the only configuration mechanism
and the constitution's "no second configuration system" clause holds. This is worth stating
because phase 005 hits the same question with SMTP and the cross-artifact analysis raises it as
finding **M4**: settings PocketBase persists are acceptable **when MediKube writes them at boot
from its own validated config**, and are a second configuration mechanism when a human edits them
in a UI.

**The reconnect trap.** PocketBase's global rate limiter plus Datastar's reconnect loop
(`retryMaxCount: 10`, exponential backoff to 30 s) can lock a user out after a server restart, as
every open tab reconnects at once. `/api/v1/streams/records` is exempted from the limiter, and a
test asserts it.

---

## D. The audit trail

### D-19 — `audit_events` carries no `ip` column

**Decision.** The collection's fields are `occurred_at`, `actor`, `actor_kind`, `action`,
`target_kind`, `target_id`, `request_id`. There is no `ip`.

**Rationale.** The shared design contract's §1.2 lists one, annotated "operator-visible, never
exported to Sentry". No requirement asks for it. FR-036 enumerates the fields exhaustively — "who
acted, what they did, which record it concerned, when it happened and the correlation identifier
of the request" — and constitution Principle VII's audit clause enumerates "actor, action, target
ID, and timestamp". An IP address is personal data about the actor, stored for two years by
default, in an application whose governing principle is that privacy is structural. Carrying
unrequested PII on the strength of a design note is precisely the failure mode Principle VII
exists to prevent.

The cross-artifact analysis raises this as finding **M3** against phase 002. Closing it here, in
the phase where the collection is born, means phase 002 never inherits the column and no
migration is needed to drop it.

**What replaces it, if an operator ever needs the network context.** The `request_id` correlates
every audit row to the zerolog stream, where the request logger records the remote address for
the duration of the log retention the operator chooses. That gives an investigating operator the
same information with a *much* shorter lifetime, and it does not put it in a two-year medical
record. Recording that here so the trade is visible rather than assumed.

**Alternatives considered.** *Keep it and add a requirement.* Rejected: this specification is
accepted and its FR-036 is exhaustive; adding a field to satisfy a design note is the wrong
direction. *Keep it with a shorter retention.* Rejected as a second retention policy on one
collection for one column.

### D-20 — `access_denied` is in the action vocabulary from the start

**Decision.** This phase's `action` vocabulary is `create`, `update`, `delete`, `access_denied`,
`login`, `login_failed`, `logout`, `password_change`, `account_delete`, `admin_session`. Every
refused attempt to reach a record writes an `access_denied` row.

**Rationale.** FR-036 requires "every refused attempt to reach a record" to be audited, and
FR-033 requires the refusal itself to be indistinguishable from a not-found — so the audit row is
the *only* record that the attempt happened, which makes its encoding load-bearing. Phase 002's
data model was going to record refusals as `read_sensitive` with a parenthetical "denied variant"
— a qualifier with no field to carry it — and phase 003 was going to introduce a real
`access_denied` action. Anyone filtering the trail after phase 003, which phase 006's audit
reader must do, would get phase-002-era refusals under one action and everything later under
another, with no way to know. The cross-artifact analysis raises this as finding **M2**.

One enum value, introduced where the collection is born, closes it permanently. Later phases
extend the vocabulary additively, which is a migration that adds values to a `SelectField` and
removes them on `down`.

### D-21 — Audit rows are written by post-commit hooks, never by handlers, and the collection is immutable through the application

**Decision.** `records.Register(kind, ...)` binds `OnRecordAfterCreateSuccess`,
`OnRecordAfterUpdateSuccess` and `OnRecordAfterDeleteSuccess` for the kind's collection. Non-record
actions — login, logout, failed login, password change, account deletion, refused access, admin
session — call `audit.Record(ctx, ev)` explicitly. `OnRecordUpdate("audit_events")` rejects
unconditionally; `OnRecordDelete("audit_events")` rejects unless the context carries the retention
job's marker. A PocketBase cron runs the retention purge daily.

**Rationale.** Post-commit is the point: a hook that fires before commit would write an audit row
for a transaction that then rolls back, and "the trail says it happened but it did not" is worse
than no trail. Binding them from `records.Register` rather than per-kind is what makes it
impossible to add a kind in phase 003 and forget its audit hooks — the same argument the shared
design contract makes for the attachment-cleanup hook.

FR-037 requires entries to be neither editable nor deletable through the application. Nil rules
plus no MediKube write path is already two layers; the bare-hook guards are the third, and they are
what stops a future contributor's well-meaning "fix a typo in the trail" from being possible at
all. The retention marker on the delete guard is a context value set by the cron job and nowhere
else, so the only code path that can delete an audit row is the one FR-037 mandates.

**Handlers do not write audit rows for record CRUD**, and that is deliberate: a handler that
audits is a handler that can forget to, and it is also a handler doing two jobs (Principle II,
single responsibility).

### D-22 — `audit_events.actor` does not cascade

**Decision.** `actor` is `RelationField{CollectionId: users, MaxSelect: 1, CascadeDelete: false}`.

**Rationale.** FR-036 requires account deletion to be audited. With `CascadeDelete: true`, deleting
an account would delete the audit row recording that the account was deleted — the one row an
operator most needs. With `false`, PocketBase's delete semantics unset the relation and re-save the
referencing row (`core/record_model.go:1618-1626`), so the entry survives with a null actor,
which is exactly right: the *action* is the record, and the actor is a reference that may
legitimately no longer exist.

The `actor_kind` field carries `user` | `admin` | `superuser` | `system` independently of the
relation, so a row whose actor has been deleted still says *what kind of* actor it was. A test
deletes an account and asserts the `account_delete` row still exists with a null actor and
`actor_kind = user`.

**Trap.** Because the relation is not `Required`, PocketBase unsets rather than failing the
delete. If it were `Required` **and** non-cascading, `core/record_model.go:1619` would make
deleting a user **fail** — which would silently break FR-014 the first time anyone tried to close
an account with history. The migration sets both properties explicitly and
`migrations/assertions.go` asserts the pair at boot.

---

## E. Data and the API edge

### D-23 — What a medication is in this phase, and what it deliberately is not

**Decision.** The field list in [data-model.md](./data-model.md) §2 and nothing else. In
particular: `owner` is a required cascading relation to `users`; `status` is `TherapyStatus`
(`active`, `on_hold`, `completed`, `stopped`, `cancelled`); `type` is `prescription`, `otc`,
`supplement`, `herbal`; `route` is the fourteen-value administration vocabulary. There is no
`patient`, no `practitioner`, no `pharmacy`, no `tags`, no reminder fields, and no `deleted_at`.

**Rationale.** The three enums map exactly onto FR-016's plain-language sets: "currently taking,
paused, finished, stopped or cancelled" is `TherapyStatus`; "prescription, over-the-counter,
supplement or herbal" is `MedicationType`; "by mouth, under the tongue, on the skin, inhaled,
injected and the rest" is `MedicationRoute`. Using the shared design contract's canonical
vocabularies verbatim rather than inventing per-phase ones is what lets phase 003 reuse
`TherapyStatus` for treatments and equipment without a migration.

`owner` cascading is what makes FR-014 ("account deletion permanently removes the account and
every medication recorded under it") true with **no MediKube code at all** — PocketBase's
`deleteRefRecords` does it, in the same transaction. Phase 002 re-points this relation at
`patients` with the same cascade, and the same requirement continues to hold for free. The
behaviour is PocketBase's, so it is asserted by an integration test rather than assumed
(`SELECT COUNT(*) FROM medications WHERE owner = '<deleted id>'` must be 0).

**No `deleted_at`.** Constitution VII scopes soft delete to files only, and this phase has no
files. A `deleted_at` column would put a filter on every query in the application to buy a
capability FR-028 explicitly refuses ("this phase provides no recycle bin for records and no
undo").

### D-24 — `ETag` on every read, `If-Match` required on `PATCH` and `DELETE`

**Decision.** Every medication read carries an `ETag` derived from `updated`. `PATCH` and
`DELETE` **require** `If-Match`; its absence is `422 validation_failed` with field `If-Match`,
and a mismatch is `412 version_mismatch` **whose body carries the current representation**.

**Rationale.** US1-9 and FR-026: "the second save is refused with a plain explanation that the
record changed underneath it, and the current values are shown so they can decide what to do."
The current representation in the 412 body is what makes "the current values are shown" a
property of the API rather than a second request the page has to remember to make.

Shared design risk **R12** asked whether this can be carried cleanly through a Datastar form,
because a required header on every clinical mutation becomes friction rather than safety if it
cannot. **It can, and this phase closes R12**: the detail page renders the ETag into a Datastar
signal (`$etag`), the form's `data-on:submit` action sends it as an `If-Match` header via
`@patch('/api/v1/records/medications/{id}', {headers: {'If-Match': $etag}})`, and the 412
response patches the form region with the server's current values plus a `role="alert"`
explanation. The mechanism is one signal and one header and it is proven here on medications
before phases 002–005 apply it to eight more resources.

Requiring rather than merely honouring `If-Match` is the choice that matters: an optional
precondition is a precondition nobody sends.

### D-25 — Cursor pagination, and where the signing key comes from

**Decision.** `?limit=` (default 25, max 100) and `?cursor=` (opaque, HMAC-SHA256 signed).
Envelope `{"items": [...], "next_cursor": "..." | null}`, with `total` only when `?count=true` is
passed. The cursor encodes the sort keys, their last values and the last id — **never an offset**.
The HMAC key is derived by HKDF from the `users` collection's persisted auth-token secret with
`info = "medikube-cursor-v1"`.

**Rationale.** FR-023 requires that paging never repeat or skip an entry "because entries were
added or removed while the person was paging", and the spec's Edge Cases repeat it. An `OFFSET`
cannot satisfy that — it is defined in terms of a result set that is changing underneath it. A
keyset cursor over `(started_on DESC, id DESC)` can, because the boundary is a row, not a count.
It is also what keeps the five-thousand-medication edge case honest: no page is materially slower
than the first, whereas `OFFSET 4975` is a scan.

Signing is not decoration. An unsigned cursor lets a client choose the keyset boundary, and a
chosen boundary is a query the service never offered — a Principle VII problem, not an ergonomics
one. The key must also survive a restart, because SC-007 requires a list left open for sixty
minutes to still work, and a per-process random key breaks every open page on every deploy.

Deriving it from a PocketBase-persisted secret rather than from a new `MEDIKUBE_CURSOR_KEY` is
recorded as Complexity Tracking **CT-3** in the plan, with the reasoning and the mitigations. The
exact field is confirmed against v0.40.1 in the first setup task; if it is not readable from Go,
the documented fallback is a `MEDIKUBE_CURSOR_KEY` with `,file,unset`, generated by the operator
and required only in production — one extra line in the quickstart, and the phase does not stall
on it.

A forged or tampered cursor is `400 invalid_cursor` and writes an `access_denied` audit row.

### D-26 — Validation in three layers, with exactly one authority

**Decision.**

1. **Decode** (`internal/web/dto.go`): shape only. Unknown fields, wrong types, malformed dates →
   `422` before any business code runs.
2. **Domain** (`internal/domain/**`): **the authority.** `Validate()` returns a
   `*domain.ValidationError` listing **every** offending field. Cross-field rules live here.
3. **Storage** (`internal/store/migrations`): PocketBase field constraints, select vocabularies
   and unique indexes as the last line of defence — never the only line.

Validation lives in **none** of: handlers, templ components, repositories, or PocketBase hooks.

**Rationale.** FR-027 requires every validation problem to be reported in a single response, each
attached to the field it concerns, with what the person typed preserved — and US1-4 exercises it
with two simultaneous faults. `Validate()` therefore accumulates and never returns on the first
failure, and a table-driven test submits a payload with four simultaneous faults and asserts four
`fields[]` entries. Putting the authority in the domain is what lets that test run with no
database and no HTTP, which is Principle III's unit layer.

The cross-field rule this phase owns is FR-018: `ended_on >= started_on`, with equality accepted
(a single-day course is valid) and a future `started_on` accepted (a course beginning next week).
Both are stated in the specification's Edge Cases and both are table rows.

### D-27 — A date with no time is a calendar date, and it does not move

**Decision.** `clinical.Date` is a `YYYY-MM-DD` value with no time and no zone, stored in a
PocketBase `date` field and marshalled as a string. Instants (`created`, `updated`,
`occurred_at`) are RFC3339 **UTC**. A date is never converted, formatted in a viewer's zone, or
round-tripped through `time.Time` in a local zone.

**Rationale.** FR-019 requires a date-only value to show as the same calendar date "regardless of
the viewer's time zone or device clock", and the Edge Cases repeat it. The failure mode is
famous: a start date of `2026-03-01` stored as midnight UTC and rendered in `UTC-5` becomes
28 February, and a patient's medication history is off by a day. A distinct type with no time
component is what makes that unrepresentable rather than merely avoided.

`clinical.Date` implements `MarshalText`/`UnmarshalText` so it round-trips through
`encoding/json/v2` unchanged, and a test asserts that a value read in one process-level `TZ` and
rendered in another is byte-identical.

### D-28 — `encoding/json/v2` retrofit semantics apply to MediKube's own DTOs

**Decision.** Every DTO gets a round-trip test asserting: slices marshal as `[]` and never
`null`; unknown fields are rejected; duplicate keys are rejected; dates are `YYYY-MM-DD`;
instants are RFC3339 UTC; and `omitempty` on a pointer means absent rather than null.

**Rationale.** Shared design risk **R2**. Go 1.27 retrofits v1 `encoding/json` onto v2 and the
release notes explicitly warn it is not fully backward compatible — around nil-versus-empty
slices, `json.RawMessage`, duplicate keys and case-insensitive field matching. This affects
MediKube's own marshalling, not just PocketBase's. Compounding it, `tests.ApiScenario` normalises
bodies through `jsontext` before substring matching, so `ExpectedContent` compares against
*re-encoded* JSON — an assertion written against hand-typed JSON can fail for formatting reasons
that have nothing to do with the behaviour under test.

The rule that follows: **never assert on a raw substring of a body where a shape assertion will
do**, and where a substring is genuinely the point (the PHI-leak test), assert on the *absence*
of a string, which is stable under re-encoding.

Rejecting unknown fields is not merely strictness — it is how `PATCH` refuses to re-attribute a
record. Phase 002 relies on exactly this: `MedicationPatch` has no `patient` field, so a request
attempting to re-file a medication is refused by the decoder before any business code runs. The
same mechanism in this phase refuses a `PATCH` carrying `owner`.

---

## F. Observability and operations

### D-29 — The PocketBase log bridge is two mechanisms, and `Logs.MaxDays` is 1

**Decision.** Both of:

1. `pb.App = &loggedApp{App: pb.App, logger: slog.New(zerolog.NewSlogHandler(zl))}`, applied
   before bootstrap.
2. `app.OnModelCreate("_logs").BindFunc(...)` that emits the record into zerolog and returns
   **without** calling `e.Next()`, so the row is never inserted.

with `Settings().Logs.MaxDays = 1` — **never 0** — and `Settings().Logs.LogIP = false`.
PocketBase's own `activityLogger` middleware is unbound and replaced by MediKube's request logger.

**Rationale.** This is the mechanism the cross-artifact analysis found scheduled nowhere
(finding **C4**), and it is the one place in the phase where MediKube reaches past PocketBase's
public surface, so the evidence matters.

PocketBase's logger **cannot be injected**: `pocketbase.Config` and `core.BaseAppConfig` expose no
logger or handler field, `core/base.go:1472 initLogger()` hardcodes the batch handler, and
`app.logger` is unexported (FACT 1). But the bridge *is* achievable, by two mechanisms that cover
disjoint sets of lines:

- **Why mechanism 1 works and is safe.** `pocketbase.go` declares
  `type PocketBase struct { core.App; ...; RootCmd *cobra.Command }` — an **exported embedded
  interface field**. `Start()` passes `pb` itself into the serve command and `apis/base.go:24`
  does `event.App = app`, so `Logger()` resolves through the decorator dynamically at call time.
  Grepping all non-test v0.40.1 source for `.(*BaseApp)` / `.(*core.BaseApp)` /
  `.(*pocketbase.PocketBase)` returns **zero** hits, so nothing downcasts past the wrapper.
- **Why mechanism 1 is not enough.** `core/db_tx.go`'s `createTxApp` does `clone := *app` on a
  `*BaseApp`, so a transaction-scoped app is a bare `*BaseApp` whose `Logger()` returns the
  hardcoded internal logger. Every line logged inside `RunInTransaction` bypasses the decorator.
- **Why `MaxDays` is 1 and not 0.** `BeforeAddFunc` returns `app.Settings().Logs.MaxDays > 0`, so
  at zero the record never enters the batch at all and mechanism 2 never fires. In production
  `printLog` does not run either (it is gated on `IsDev()`), so PocketBase's backup failures,
  mailer failures, cron errors and OAuth2 failures would go to **nowhere**. `MaxDays = 1` keeps
  the pipeline alive; mechanism 2 guarantees no row is ever written, so there is still exactly
  one log store to consult.

**Costs, stated honestly.** Mechanism 2 batches on a roughly 3-second ticker and loses source PC
and request context. Both mechanisms rest on observed internal behaviour. Both go into
`docs/pocketbase-upgrade-checklist.md` with a test that fails loudly if either changes, and both
are Complexity Tracking **CT-1**.

**Alternatives considered.** *One mechanism.* Rejected on the `createTxApp` evidence. *`MaxDays =
0` and accept losing PocketBase's logs*, which the constitution originally said and v1.1.0
corrected. Rejected: silently discarding a framework's own failure reports in a medical-records
application is not a trade anyone would make deliberately. *Poll `_logs` on a timer.* Rejected: a
third log store with a polling delay. *Fork PocketBase.* Rejected outright.

### D-30 — otelsql attaches through `pocketbase.Config.DBConnect`, and the copied pragma string is a drift risk

**Decision.** `DBConnect` returns `dbx.NewFromDB(otelsql.Open("sqlite", dsn, ...), "sqlite")`.
The pragma string is copied from `core.DefaultDBConnect` and a test asserts it has not drifted.

**Rationale.** `pocketbase.Config.DBConnect` is the injection point and it is clean. Two details
each cost a day if missed:

- **Use `otelsql.Open`, not `otelsql.Register`.** `Register` opens with an empty DSN, and
  `DBConnect` is called **four times** (data and aux, concurrent and non-concurrent), which would
  burn four global driver slots.
- **Pass the logical name `"sqlite"` to `dbx.NewFromDB`.** Omit it and `dbx` falls back to
  `NewStandardBuilder` (`dbx/db.go:321`) and PocketBase's SQL quoting breaks.

The real liability is that PocketBase's pragma string is a **local variable** inside
`DefaultDBConnect` (`core/db_connect.go`), not an exported constant, so it has to be copied. Get
it wrong and WAL or foreign keys are silently lost — a class of failure that shows up as
corruption weeks later, not as an error at boot. The drift check is a test that reads the
expected pragmas and fails with a message naming the upgrade checklist. It is the third entry in
`docs/pocketbase-upgrade-checklist.md` (shared design risk **R8**).

### D-31 — Liveness, readiness, and a drain that makes PocketBase's one-second shutdown window irrelevant

**Decision.** `GET /api/v1/healthz` reflects the process only, touches no database, and is
excluded from the activity logger, the metrics middleware and the tracing middleware.
`GET /api/v1/readyz` runs a real `SELECT 1` with a 2-second context deadline, checks the
migration state, and respects a drain flag. Both are public. Shutdown binds an
`OnTerminate` handler at priority **`-10000`** that flips readiness to false, sleeps
`MEDIKUBE_DRAIN_DELAY`, and waits up to `MEDIKUBE_DRAIN_MAX` on an in-flight counter before calling
`e.Next()`.

**Rationale.** FR-052 requires the two signals to be distinct and to mean different things, and
FR-004's edge case requires that with storage unreachable, liveness still reports alive while
readiness reports not-ready with a non-revealing reason. A liveness probe that checks the database
gets the container killed and restarted into the same outage, which is the opposite of helpful.

The drain handler exists because `apis/serve.go:171` binds PocketBase's HTTP shutdown at priority
`-9999` with `context.WithTimeout(context.Background(), 1*time.Second)` — **one second**, not
configurable, not exposed on `ServeConfig`. Any request still running after one second has its
connection cut mid-response. Binding at `-10000` runs *before* it, so by the time PocketBase's
one-second window opens there is nothing in flight and its value no longer matters. This is the
standard fail-readiness-then-drain pattern and FR-062 describes it almost word for word.

`cancelBaseCtx()` fires at the start of terminate and cancels every request context, so the SSE
handler must select on `e.Request.Context().Done()` and return cleanly — otherwise the goroutine
leaks until process exit and the client sees a reset instead of a stream close. Note also that
`TerminateEvent.IsRestart` exists: on a restart PocketBase waits an extra 3 seconds for `execve`,
so terminate does not always mean exit.

**Neither endpoint reveals anything.** `readyz` returns a fixed vocabulary of check names and
`ok`/`error`, never an error message, a path or a credential (FR-052, and the Edge Case about
storage credentials).

### D-32 — The break-glass boot warning reads two settings from two different places

**Decision.** At boot, MediKube warns loudly — a distinct, repeated, unmissable `WARN` block — when
any of the following holds, and re-warns on every start until all are fixed:

- `len(app.Settings().SuperuserIPs) == 0` — the IP allowlist is unconfigured;
- the superusers collection's `MFA.Enabled` is false;
- the superusers collection's `MFA.Rule` is non-empty;
- the superusers collection has fewer than two auth methods enabled.

**Rationale.** Constitution VII requires the warning, and shared design risk **R6** left it
unverified. VERIFIED-SOURCE-FACTS **FACT 10** closes it — and closes it with a correction that a
plan assuming the two settings sit together would have got wrong:

- **The IP allowlist is on global settings**: `core/settings_model.go:125`,
  `SuperuserIPs []string`, validated as `IPOrSubnet` so CIDR ranges are accepted. It is already
  enforced in two places — the router's `superuserIPsWhitelist()` middleware, and `apis/file.go`,
  which drops superuser auth on a file request from a non-allowlisted IP.
- **MFA is on the superusers auth collection**, not on settings:
  `core/collection_model_auth_options.go:348`, `MFAConfig{Enabled, Duration, Rule}`, reached via
  the collection's `MFA` field.

The two extra conditions are the ones a naive check would miss. PocketBase **refuses to enable
MFA unless the collection has at least two auth methods** (`validation_mfa_not_enough_auths`), so
"turn on superuser MFA" is not a single toggle and the warning must say so rather than reporting a
bare "MFA off". And `MFAConfig.Rule` being non-empty is a *partial* rollout, meaning some
superuser can still sign in without a second factor — which is the situation the warning exists
to prevent, so a non-empty rule triggers it too.

**Why a warning and not a refusal to start.** FR-040 says warn; the specification's US3-6 says
"warns loudly and unmistakably … and continues to warn on every restart". Refusing to start would
make a first run impossible, since a superuser must exist before MFA can be enabled on the
collection. The warning is the correct escalation and the audit entry for every admin session
(FR-040) is the compensating control.

---

## G. The frontend, realtime, and the gates

### D-33 — Realtime is 100% Datastar's, and the hub fans out IDs

**Decision.** PocketBase's native realtime is not used. A post-commit record hook publishes
`realtime.Event{Kind, RecordID, OwnerID}` to an in-process hub — a channel and a map — and a
per-subscriber Datastar SSE handler re-fetches each id, **re-authorises it for that subscriber**,
renders the row component and patches it. **IDs, never bodies.** The hub is not behind a broker
interface.

**Rationale.** Three independent, verified reasons PocketBase's realtime cannot be used here:

1. `apis/realtime.go:605-613` builds its subscription rule map from each collection's `ViewRule`
   and `ListRule`, and `core/record_query.go:599-612` `CanAccessRecord` returns **false** when the
   rule is `nil`. Under this phase's own lockdown, every broadcast is therefore **silently
   skipped** — not an error, not a 403 on the stream, nothing. That is the worst possible failure
   mode and hours of debugging.
2. PocketBase emits its own JSON envelope on events named `<collection>/<id>`, while datastar-go
   v1.2.2 recognises exactly two event names — `datastar-patch-elements` and
   `datastar-patch-signals` — and discards everything else.
3. PocketBase requires a two-step subscribe handshake (`GET /api/realtime` yields a `clientId` in
   a `PB_CONNECT` event, which must then be echoed in a `POST /api/realtime`) that a Datastar
   attribute cannot perform.

**Fanning out IDs rather than bodies is the single most important rule in the realtime layer**,
and it is a Principle VII requirement, not an optimisation: it is what makes per-subscriber
authorization possible at all. A hub that carried record bodies would have to decide *at publish
time* who may see them, which is exactly the decision the authorizer is supposed to own, made in
the wrong place with the wrong information.

**No broker interface.** The constitution's Technology Constraints forbid the speculative seam by
name. The hub has exactly one consumer, and adding a broker later is a contained change.

**Most interactions need no SSE at all.** Datastar honours a plain `text/html` response as an
element patch, so create, edit and delete all use the non-SSE fast path. Streams are reserved for
the genuinely live list, which minimises exposure to D-34.

### D-34 — The five-minute `WriteTimeout`, the mandatory `newStream()` helper, and the CI job that proves it

**Decision.** Two fixes, both applied. (a) A mandatory `internal/web/stream.newStream(e)` helper
that clears the per-connection write deadline with
`http.NewResponseController(e.Response).SetWriteDeadline(time.Time{})`, sets
`X-Accel-Buffering: no` and `Cache-Control: no-store`, and returns the Datastar generator. Every
SSE handler goes through it. (b) An `OnServe` hook that adjusts `se.Server.WriteTimeout` before
the listener starts. Stream routes are registered with `Bind(apis.SkipSuccessActivityLog())`. A
CI job holds a stream open for **more than five minutes** and asserts it survives.

**Rationale.** `apis/serve.go:145-160` constructs the server as a struct literal with
`WriteTimeout: 5 * time.Minute` and no configuration field. `datastar.NewSSE` sets
`Cache-Control`, `Content-Type` and `Connection` and flushes — it **never touches the write
deadline**. So every long-lived Datastar stream dies at exactly five minutes with a write error
and the client reconnect-loops. **It passes every test shorter than five minutes**, which is
precisely what makes it dangerous, and SC-007 requires a view left open for sixty continuous
minutes to still be receiving updates.

Fix (a) reaches the real writer: PocketBase's `tools/router/router.go:312` implements
`func (rw *ResponseWriter) Unwrap() http.ResponseWriter` and PocketBase defines the `RWUnwrapper`
interface for exactly this, so `SetWriteDeadline`, `Flush` and `Hijack` all pass through. Fix (b)
is available because `core.ServeEvent` exposes `Server *http.Server` as an exported mutable field
(`core/events.go:110`), assigned at `apis/serve.go:212` before `OnServe().Trigger` at `:217` and
before `Serve(listener)` at `:304`.

`X-Accel-Buffering: no` matters because an upstream nginx otherwise buffers the entire stream and
nothing arrives until close. `Cache-Control: no-store` rather than Datastar's `no-cache` is the
correct choice for a medical application and matches what PocketBase's own realtime sets.
`SkipSuccessActivityLog` avoids a useless log line written thirty minutes after the fact.

**The CI job is the point.** Without it the fix regresses invisibly the first time somebody
refactors `newStream`. It is slow and awkward and it ships anyway; the failure mode it prevents
is user-visible and silent. This is shared design risk **R7**, and this phase writes both the
helper and the job.

### D-35 — `script-src 'unsafe-eval'` is accepted and permanent; everything else is strict, and the inline-script SDK family is banned

**Decision.**

```
default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self';
img-src 'self' data:; connect-src 'self'; form-action 'self';
frame-ancestors 'none'; base-uri 'none'; object-src 'none'
```

Banned outright: `ExecuteScript`, `ConsoleLog`, `ConsoleError`, `Redirect`, `Redirectf`,
`DispatchCustomEvent`, `ReplaceURL`, `ReplaceURLQuerystring`, `Prefetch`. Only the **free**
Datastar attribute set may be used. A lint rule enforces both lists.

**Rationale.** Verified in the shipped bundle rather than guessed: Datastar's expression compiler
is literally the `Function` constructor (`let l = Function("el","$","__action","evt",...)`), and
its signal parser falls back to it so that non-strict-JSON `data-signals="{foo: 1}"` works.
Without `'unsafe-eval'`, **every `data-*` expression on every page throws** — the application does
not partially degrade, it is entirely non-functional, and it fills the console with CSP
violations, failing the zero-console-error gate on every route. This is a genuine, permanent
security trade-off and FR-042 requires it to be recorded as one rather than buried.

What bounds the risk, stated honestly: `'unsafe-eval'` does not itself create an injection
vector; every Datastar expression is server-authored templ output and expression text never comes
from user input; `'unsafe-inline'` is **not** granted, so an injected `<script>` tag still does
not run; and `connect-src`/`form-action 'self'` block exfiltration while `object-src`/`base-uri
'none'` close the classic bypasses.

**Two rules keep it safe and both are lint-enforced, because they are what the compensation
rests on:**

1. **Never interpolate user data into a `data-*` expression** — not `data-text`, not `data-on:*`,
   not `data-signals`. User data reaches the client as a signal *value* or as escaped text
   content, never as expression source. A CI check greps for templ interpolation `{ ... }` inside
   a `data-on:` or `data-computed:` attribute value.
2. **Keep `'unsafe-inline'` out of `script-src` permanently.** That is exactly what banning the
   SDK family buys, and it must not be traded back for the convenience of `sse.Redirect()`.
   Every banned call appends a literal inline `<script>`; under this CSP each one silently fails
   *and* logs a violation that fails the console gate. Redirect becomes a plain `303` issued
   *before* the stream opens; user-visible errors go to the `#error-banner` region.

PocketBase's `securityHeaders()` middleware sets **no** CSP for application routes — its
`defaultCSP` applies only to `/_` — so this header is MediKube's to set.

**The free-attribute boundary.** `data-persist`, `data-query-string`, `data-replace-url`,
`data-scroll-into-view`, `data-view-transition`, `data-custom-validity`, `data-animate`,
`data-match-media`, `data-on-raf`, `data-on-resize`, `@clipboard` and `@fit` are **Pro**. Every
one has a free replacement, and for `data-persist` the replacement is better anyway: UI
preferences persist on the `users` record and hydrate through `data-signals`, which is where a
medical application's state belongs rather than scattered into `localStorage`.

**Two silent-failure traps** that must be in `CLAUDE.md`, because they produce no error at all:
the v1 delimiter is a colon, so `data-on-click` does **nothing** and it is `data-on:click`; and
`data-on-load` is now `data-init`. Also: the Go SDK is v1.2.2 and the browser runtime is v1.0.2 —
different repositories, independent version numbers. There is no `datastar.js v1.2.2`.

### D-36 — The theme is applied by the server, at first paint, with no inline script

**Decision.** The page handler resolves the theme from `users.theme` (`system` | `light` |
`dark`) and renders it as a class on `<html>`. Tailwind's `dark` variant is configured to respond
to **both** that class and `prefers-color-scheme`, so `system` needs no JavaScript and no class.
Changing the preference is a `PATCH /api/v1/me` followed by a full re-render.

**Rationale.** FR-045 requires the choice to be stored on the account so it follows the person to
another device, and applied "from the first moment a page is drawn rather than after a visible
change". The usual solution is a blocking inline `<script>` in `<head>` reading `localStorage` —
which is banned twice over here: `'unsafe-inline'` is not in the CSP, and `data-persist` is a Pro
attribute. Server-side resolution has neither problem, produces no flash by construction, and is
the only approach that satisfies "follows the person to another device" at all, since
`localStorage` does not travel.

For an anonymous visitor on `/login` and `/register` there is no account to read, so the
`prefers-color-scheme` half of the variant governs. US4-3's test signs in on a second device and
asserts the dark class is present in the **first** rendered byte, not applied afterwards.

### D-37 — Stream staleness is detected with a server heartbeat and free attributes only

**Decision.** The SSE handler emits a `datastar-patch-signals` heartbeat every 25 seconds setting
`$stream_beat` to the current server time. The list page carries
`data-on-interval__duration.10s` comparing `$stream_beat` against the clock and setting
`$stream_stale` when the gap exceeds 60 seconds; `$stream_stale` reveals a `role="alert"` banner
saying the live view has stopped updating and offering a reload.

**Rationale.** FR-031 requires the person to be told plainly when a live view can no longer be
kept current, "rather than continuing to present data that has quietly stopped changing". The
server cannot tell them — if the stream is dead, the server has no channel. So the detection has
to be client-side, and it has to use only free attributes: `data-on-interval` and `data-show` are
in the free set; `data-match-media` and `data-persist` are not.

The 25-second heartbeat also does useful secondary work: it keeps intermediaries from timing the
connection out, and it makes the >5-minute CI liveness test (D-34) assert something concrete —
that beats keep arriving — rather than merely that the socket is open.

**Alternatives considered.** *Bind a handler to Datastar's own SSE-error event.* Rejected as the
primary: the exact event name would have to be confirmed against the vendored bundle, and a
detector that depends on an undocumented event name is a detector that fails silently after an
upgrade. It may be added later as a faster secondary signal; the heartbeat comparison is
deterministic, testable without a browser, and cannot go stale.

### D-38 — The Playwright gate, its two flakiness traps, and the requirement to prove it goes red

**Decision.** Playwright is a build-time and test-time dependency only; **no Node in the runtime
image**. `e2e/routes.ts` shells out to the binary under test (`medikube routes`) at Playwright's
**collection** phase, before any browser starts, and derives the target list from the JSON. Two
projects, `desktop` (1440×900) and `mobile` (390×844). Every target asserts: HTTP 200; the four
shell landmarks; its own landmark; `body[data-signals]` present; zero console errors **and
warnings**; zero uncaught page errors; zero failed requests and no 4xx/5xx subresource.

**Rationale.** FR-067 and SC-009 require that adding a page without adding a check fails the
build, and a hand-maintained route list cannot deliver that. Deriving from the binary means a new
page ships with a new smoke test for free and there is nothing to keep in sync by hand — and
because `httproute.Handle` panics on a page without a landmark, the binary cannot even start with
an unsmokeable page.

**Warnings fail too.** Starting at zero tolerance and adding justified entries to an ignore list
is the only version of this that stays at zero; starting at "errors only" means warnings
accumulate until nobody reads them. A Datastar selector matching nothing warns rather than
throws, which is exactly the class of bug this gate is for. The ignore list is empty on day one
and every future entry carries a comment.

**Two traps that will look like real failures:**

- `waitUntil: 'networkidle'` **hangs forever** on any page holding an SSE stream — the stream
  never goes idle. The gate uses `domcontentloaded` plus an explicit landmark assertion, which is
  what Playwright's auto-waiting is for anyway.
- Aborted SSE streams produce `net::ERR_ABORTED` on teardown. It is filtered **exactly**; muting
  all `requestfailed` events would make half the gate decorative.

**And the criterion that matters most.** The research environment never actually ran a browser —
configuration and specs parsed and collected, but no assertion was ever executed against Chromium
(shared design risk **R11**). **An always-green gate is worse than no gate.** So this phase's exit
criterion is not "the gate passes", it is "the gate was demonstrated to go **red**": once with a
landmark removed, once with a deliberate `throw` in the page, both recorded in `e2e/README.md`,
before the phase is accepted. FR-072 and SC-010 say exactly this.

### D-39 — Every page ships an empty state inside its own landmark, and the seed is deliberately mixed

**Decision.** A shared `@EmptyState(title, body, action)` component renders **inside** the page's
own landmark, never in place of it. `medikube seed` produces two accounts: account A with
medications across several states, and account B — the isolation counterparty — with an
**empty** medication list.

**Rationale.** FR-029 requires the empty medication list to use "the same page structure as a
populated list", and the specification's Edge Cases name the reason: "so that the automated
browser check does not go falsely red on a page that is legitimately empty". Rendering the empty
state *instead of* the landmark is the single most common way a smoke gate fails on a fresh
install, and it fails in a way that looks like a broken page rather than a missing landmark.

Seeding one account empty is what makes that path exercised rather than asserted. FR-060 requires
the demo set to include at least two separate accounts holding medications so that isolation can
be tested automatically; account B holds one medication for the isolation tests and its *list
page* is what the empty-state smoke case uses — resolved by giving account B a medication whose
state is filtered out by the list's default narrowing, so the default view is legitimately empty
while the data exists. Simpler and equally honest: a third seeded account, C, with no medications
at all. **C is what ships**, because a filter-dependent empty state is a test that breaks when the
default filter changes.

Also seeded: a medication with **only a name**, because the specification's Edge Cases require
that case to render correctly everywhere with unfilled fields absent rather than blank; and a
medication whose name contains right-to-left text and characters that look like markup, because
that case must be stored, displayed and searched as text and never interpreted.

### D-40 — Monorepo integration, and the five Dockerfile traps that are not obvious

**Decision.** In the same commit that creates the project:

- `/.dockerignore` gains `!medikube/` in the allowlist block, plus `medikube/medikube`, `medikube/.bin/`,
  `medikube/**/*_templ.go`, `medikube/internal/web/static/app.css`, `medikube/pb_data/`,
  `medikube/**/*.db`, `medikube/**/*.db-wal`, `medikube/**/*.db-shm`.
- `/.github/workflows/build-image.yaml` gains `medikube` in
  `workflow_dispatch.inputs.project-name.options`.
- `medikube/.github/workflows/medikube-ci.yml` is written with the project and copied to the
  repository root, following the convention `tape-ci.yml` documents.

**Rationale.** `/.dockerignore` is a **deny-everything-then-readmit allowlist** — `*` on line 12.
A project that is not readmitted is **invisible to the build context**, and the failure is a
misleading "file not found" rather than anything that names the real cause. The constitution's
Development Workflow section makes this a same-commit requirement for exactly this reason. The
build-output exclusions matter separately: `*_templ.go` and `app.css` are regenerated in the
image's generate stage, and a host-built copy in the context would be copied over the source tree
and shadow what that stage produces. `pb_data/` must never enter a build context at all — it
holds the live database and, in later phases, uploaded medical records.

**The five traps, all of them recorded in `arc-ui` and all of which MediKube hits identically:**

1. **The build context is the repository root**, unconditionally, because that is what the shared
   workflow passes. Every `COPY` must be project-prefixed (`COPY medikube/go.mod medikube/go.sum ./`).
   A bare `COPY go.mod ./` works locally and breaks in CI, and is the likeliest single miss in
   the phase.
2. **The Tailwind release asset for x86_64 is named `x64`, not `amd64`.** Docker's `$BUILDARCH`
   says `amd64`, so the unmapped URL 404s and the failure reads like a network blip.
3. **The Tailwind standalone binary is a Bun build and is not statically linked** — it needs
   glibc, which is why the generate stage sits on the Debian `golang` image and pulls plain
   `linux-x64`/`linux-arm64`. The `-musl` variants are for Alpine builders and fail at exec time,
   not at download time.
4. **Distroless has no shell and no `mkdir`**, so `/data` must be created in a build stage with
   `install -d -m 0755 -o 65532 -g 65532 /data` and `COPY`ed. And `USER 65532:65532` numerically,
   not `USER nonroot`: Kubernetes' `runAsNonRoot` admission check must decide before start
   whether the user is root, and some runtimes cannot resolve a name from the image's
   `/etc/passwd`.
5. **No `HEALTHCHECK` in the Dockerfile** — there is no curl and no wget in distroless. That is
   why `medikube healthcheck` exists as a Cobra subcommand and why FR-058 asks for it: it is the
   binary probing itself.

Two more, less famous: **pin `TAILWIND_VERSION` by `ARG`**, or the generated CSS is not
reproducible between two builds of the same commit; and **point Tailwind at the `.templ`
sources**, because auto-detection skips them (generated files are gitignored) and the result is a
stylesheet with none of the application's utilities in it. Also commit
`internal/web/static/.gitkeep`, or `go:embed` fails on an empty directory.

Every stage before the last is `--platform=$BUILDPLATFORM`. With `CGO_ENABLED=0` Go
cross-compiles for free — which holds here because PocketBase's SQLite is `modernc.org/sqlite`,
pure Go — so nothing runs under QEMU. Emulating an arm64 builder to run templ and Tailwind costs
minutes and OOMs regularly, for zero benefit: both generators only emit source.

---

## Shared design risk register, as it stands after this phase

| # | Risk | Status after phase 001 |
|---|---|---|
| **R1** | Discriminated `oneOf` generation and gating | **CLOSED** by FACT 9 and made permanent here by `internal/openapi/gate_test.go` against a synthetic two-kind registry (D-08). The 94-operation budget (SHARED-DESIGN §2.3) and phase 003's three-route shape hold. |
| **R2** | Go 1.27 `encoding/json/v2` retrofit semantics in MediKube's DTOs | **Mechanism created.** Every DTO carries a mandatory round-trip test (D-28). Re-opens for each new DTO in every later phase; phase 002 already records that. |
| **R3** | FTS5 availability in the vendored SQLite | **CLOSED** by FACT 11 — FTS5, `MATCH` and `rank` all work in `modernc.org/sqlite` v1.57.0. Nothing here uses them; phase 004 may drop its `LIKE` hedge. |
| **R4** | PDF generation | Untouched. Phase 006's decision. |
| **R5** | Thumbnails for non-image attachments | Untouched. Phase 004's decision. |
| **R6** | Reading superuser MFA and the IP allowlist at boot | **CLOSED** by FACT 10 and implemented here, with the correction that they live in two different places and that two further conditions must also warn (D-32). |
| **R7** | The >5-minute SSE liveness test in CI | **Mechanism created and the job ships in this phase** (D-34), rather than waiting for 006. Phase 004 widens it to cover slow uploads. |
| **R8** | PocketBase upgrade fragility | **Checklist created** at `docs/pocketbase-upgrade-checklist.md` with its first three entries — the `pb.App` decorator, the `_logs` interception, and the copied `DefaultDBConnect` pragma string — each with a test that fails loudly (D-29, D-30). |
| **R9** | Export job durability | Untouched. Phase 006. |
| **R10** | Email deliverability for invitations | Same root cause as D-04's recovery flows, and now partly retired by them: this phase already ships the boot warning and the `503 mail_unconfigured` refusal for an instance with no outgoing mail, so phase 005 inherits the mechanism rather than inventing it. |
| **R11** | The Playwright gate has never run a real browser | **Closed only by demonstration.** The phase exit criterion is that the gate was proven to go RED twice, recorded in `e2e/README.md` (D-38, FR-072, SC-010). |
| **R12** | `ETag`/`If-Match` through Datastar forms | **CLOSED** here on medications: one signal, one header, and a 412 that patches the form with the server's current values (D-24). Phases 002–005 apply the same mechanism to eight more resources. |
