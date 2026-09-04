# Spec defects

> **Numbering.** Entries here are **defects** and are cited as `defect D15`, never as a
> bare `D15`. `specs/001-walking-skeleton/research.md` separately numbers **decisions**
> `D-01`…`D-39`, cited as `research D-22`. The two namespaces already collide — defect
> D18 (the phileak gate) and research D-18 (registration closed by default) are both
> live, and `internal/config/config_test.go` cites the latter. Always write the word
> `defect` or `research` before the number; a bare `D18` is ambiguous.

Contradictions and stale figures found while implementing, recorded rather than
silently resolved. Each entry names the authority that was followed and why.

The general rule this project applies: **`data-model.md` outranks `tasks.md` for
schema and vocabulary**, because tasks.md is a derived work plan and drifts when
the model is revised. `plan.md` outranks both for versions and package layout.

## D1 — T049 undercounts the audit actions

`tasks.md` T049 asks for "`Action` (all ten values including `access_denied`)".
`data-model.md` §3 declares **twenty** actions, and T071 in the same file says
"**twenty** actions" three sections later. tasks.md contradicts itself.

*Followed:* data-model.md. `internal/domain/audit/enums.go` ships twenty.
*Fix:* correct T049 to twenty.

## D2 — Length arithmetic predates the MediKube rename

The old project's snake_case prefix was 7 characters; `medikube_` is 9. Every proof
that sized a field by counting a prefixed identifier is off by two per occurrence.
`audit_events.target_id` is specified at `Max 64` and phase 006 writes
~40-character archive names into it, so the margin was never large.

The old spelling is deliberately not written out above. `scripts/check-naming.sh`
sweeps every tracked file and excludes only itself, so prose that quotes the old
name fails the gate — and an allowlist for prose is exactly how the name comes
back.

*Action:* re-derive any length proof before relying on it; do not trust a figure
that was written before the rename.

## D3 — T030 and T043 disagree about where redaction lives

T030 implements `internal/logging/redact.go` as "the shared redaction helpers the
domain packages use", but T043 forbids `internal/domain/**` from importing
anything outside stdlib and zerolog — and `internal/logging` is neither. As
written the domain cannot reach the helper.

*Followed:* the boundary. Domain-side redaction is expressed through
`MarshalZerologObject` on the domain types themselves; `internal/logging/redact.go`
serves the logging side only.
*Fix:* reword T030.

## D4 — T135 and data-model disagree on future start dates

T135 and `data-model.md` §2 state different rules for whether a medication may
start in the future. Unresolved at the time of writing; flagged for the phase that
implements it.

## D5 — Cursor key rotates with `AuthRule`

Not a spec defect but an under-documented consequence. CT-3 derives the cursor key
from PocketBase's persisted auth-token secret. That secret is **per collection**
(`core.TokenConfig.Secret`, reached as `collection.AuthToken.Secret`) and
PocketBase rotates it whenever the collection's `AuthRule` changes
(`core/collection_model.go:864`). Every outstanding cursor therefore dies on an
`AuthRule` change.

This is acceptable, but only because the same change already invalidates every
session — users are logged out regardless, so a dead cursor is strictly the
smaller disruption. Worth knowing before someone treats cursor stability as
unconditional.

## D6 — T053 asserts token TTLs on a field that does not exist

T053 says "rate limits and token TTLs match the MediKube config" after reading
`app.Settings()`. In v0.40.1 there is **no token config on `core.Settings`**.
Every TTL is per auth collection: `AuthToken`, `PasswordResetToken`,
`EmailChangeToken`, `VerificationToken` and `FileToken`, each a
`core.TokenConfig{Secret string; Duration int64}`
(`core/collection_model_auth_options.go:139-143`, `:292-296`).

*Followed:* the API that exists. `internal/platform/pb/settings.go` reaches TTLs
through `collection.AuthToken.Duration`, not `app.Settings()`.
*Fix:* reword T053. Written as specified the test asserts a field that does not
compile.

Two related facts worth keeping, both verified against a genuinely empty data
dir: `Batch.Enabled` already defaults to `false`, so T053's assertion there is a
regression guard rather than a change; `Logs.MaxDays` defaults to **5** and
MediKube needs **1**, so that one is a real write.

## D7 — T060/T061 cannot pass against a booted stock instance

The `users` collection PocketBase ships (`migrations/1640988000_init.go:315-346`)
has **five non-nil API rules** and an **unprotected `avatar` FileField**. Both are
exactly what T060's boot assertions refuse to start on. So on an untouched
instance the assertions fire against PocketBase's own schema, and T060/T061 cannot
pass until T069's migration nulls those rules and drops or protects `avatar`.

The `System: true` qualifier in T060 is load-bearing and correct: `_superusers` has
all five rules nil, while `_mfas`, `_otps`, `_externalAuths` and `_authOrigins`
carry non-nil list/view rules and are system collections.

*Followed:* T060's test builds its assertion input synthetically, in memory, rather
than against a booted app — which is what T062 already implies when it asks for
"a synthetic collection with a FileField whose Protected is false". That removes
the ordering dependency on T069 entirely.

## D8 — The boot assertions must bind to `OnServe`, not `OnBootstrap`

`Bootstrap()` runs `RunSystemMigrations()` only (`core/base.go:436`, impl
`:833-836`). MediKube's migrations register into `core.AppMigrations` and are
applied by `RunAllMigrations()`, which `apis.Serve` calls at `apis/serve.go:67` —
*before* `NewRouter` and *before* `OnServe`. Assertions bound to `OnBootstrap`
therefore run before MediKube's schema exists and assert against nothing.

*Followed:* `OnServe` at a very negative priority. There they work, and returning
a non-nil error aborts the serve with no listener created
(`apis/serve.go:265-267`) — which is precisely the "refuse to start" T060 asks
for.

## D9 — The cursor is encrypted, not signed: an amendment to research D-25

D-25 specifies an opaque cursor **HMAC-SHA256 signed**, with the key derived by
HKDF from the `users` collection's persisted auth-token secret under
`info = "medikube-cursor-v1"`. `internal/store/cursor.go` keeps the derivation
exactly — same HKDF, same secret, same label — and then **encrypts** the payload
with AES-256-GCM instead of signing it.

This is deliberate and it stays. FR-022 orders by name, so the keyset boundary
value **is a drug name**. A signed-but-readable cursor carries that name in the
query string of every "next page" request, which puts it in browser history, in
the `Referer` of anything the page links to, and in the access log of every
reverse proxy between the person and the instance — the identical disclosure
D-29 unbinds PocketBase's activity logger over, arriving by a different route.

Nothing is given up by the change. GCM is authenticated encryption: a tampered
or forged cursor fails to open exactly as a bad signature fails to verify, and
the sort order is bound into the sealed payload, so a cursor cannot be replayed
under a different ordering. Encryption is a strict superset of signing here.

*Followed:* the code. *Fix:* amend D-25 to say AES-256-GCM over the same derived
key, so the spec and the implementation stop disagreeing about a security
control.

## D10 — Three lockdown defects the green suite could not see (fixed in `c51a563`)

Recorded because each was found by adversarial verification *after* 449 tests
were green, and in each case the reason the suite could not see it is more
instructive than the fix.

1. **The lockdown's priority moved from `-1019` to `-1009`.** `plan.md` and
   research D-07 both name `-1019`, derived as "after `loadAuthToken` at
   `-1020`". That is the right lower bound and the wrong number: the lockdown
   short-circuits, so every middleware bound *after* it is skipped — including
   PocketBase's `securityHeaders` at `-1010`. A locked route answered `404`
   with no `X-Content-Type-Options`, no `X-Frame-Options`, no
   `X-Xss-Protection` and no COOP, while a genuinely unknown path answered
   `404` carrying all four. One header told an anonymous caller which of the
   two it had hit, which is the route-existence disclosure the middleware
   exists to prevent. The record-level guarantee always held; the
   route-existence one did not. The suite could not see it because the test
   helper never captured response headers.
   *Fix:* amend plan.md and research D-07 to `-1009`
   (`DefaultSecurityHeadersMiddlewarePriority + 1`).

2. **`POST /api/realtime` is now one of the locked routes.** Neither the plan
   nor the research names it. Subscribing delivered create and update payloads
   in full, record included, to an anonymous caller. Auth travels on the POST
   because `EventSource` cannot set headers, so closing the POST leaves the
   admin UI's GET stream connected and empty. Under MediKube's schema the nil
   `ListRule`/`ViewRule` already stopped this — which is the point: record CRUD
   had two independent controls and realtime had one, and the lockdown exists
   to be the second.
   *Fix:* add the route to the locked set in plan.md and research D-07.

3. **The boot assertions run after `opts.Routes.Bind`, not before it.** Any
   `RouteBinder` can call `se.Router.Unbind(LockdownMiddlewareID)` —
   deliberately or by reaching for a group — and `RouterGroup.Unbind` strips the
   handler with no error and no log line. Asserted ahead of that call the check
   is structurally incapable of catching the only thing it is for.
   `AssertLockdownBound` also rejects a lockdown rebound at the wrong priority,
   which is the same failure wearing a disguise.
   *Fix:* say so in T067's wording; "the OnServe binding" does not imply an
   order and this one is load-bearing.

## D11 — Two independent implementations of the same data-model §5 assertions

`tasks.md` mandates both `internal/platform/pb/assert.go` (T061) and
`internal/store/migrations/assertions.go` (T072), and each was written to
data-model §5 independently. They now overlap without agreeing on anything but
the answer:

| | `pb.Assert*` | `migrations.Assert*` |
|---|---|---|
| API-rule sentinel | `ErrAPIRuleSet` | `ErrAPIRuleNotNil` |
| File sentinel | `ErrFileFieldUnprotected` | `ErrFileFieldUnprotected` (a second, unrelated value) |
| Settings input | `*core.Settings` | `core.App` |
| Settings scope | `Batch.Enabled` only | `Batch.Enabled` **and** `Logs.MaxDays == 1` |
| Relations | not covered | `ErrRelationMismatch` over the §4 matrix |
| Fatal / strict split | none | `AssertFatal` vs `AssertStrict` |

They agree today. Nothing makes them agree tomorrow: a phase that tightens one
leaves the other passing, and `errors.Is` against the wrong package's sentinel
compiles and is always false — which is the failure mode, because it reads as
"the condition did not fire".

Three options, in the order they were considered:

1. **`internal/store/migrations` owns the assertions; `internal/platform/pb`
   calls them.** The schema is what is being asserted, migrations is where the
   schema is declared, and it already has the wider version (relations,
   `Logs.MaxDays`, the fatal/strict split the boot sequence needs). `pb` keeps
   `AssertLockedDown` as the boot entry point and delegates. Costs: the [PB]
   package `internal/platform/pb` would import `internal/store/migrations`,
   which is a new edge between two packages that today share nothing.
2. **`internal/platform/pb` owns them; migrations calls them.** Matches the
   direction of the boot sequence, but inverts the dependency — the platform
   package would then have to know the §4 relation matrix, which is schema
   knowledge that belongs with the migrations that write it.
3. **Keep both, add a test that runs both over the same input and requires the
   same verdict.** Cheapest, changes no structure, and turns silent divergence
   into a red test. It does not remove the two sentinels, so `errors.Is` against
   the wrong one stays a live trap.

*Recommendation:* option 1, deferred to phase 002, with option 3 landed now as
the guard. 002 is the first phase that adds a collection (`patients`) and a file
field (`patients.photo`), so it is the first phase where a divergence between
the two would have something real to diverge about, and it is the natural moment
to move the assertions rather than to move them twice.

*Not done unilaterally:* tasks.md mandates both files by name, so collapsing
them is a tasks amendment and not an implementation detail. Neither
implementation has been deleted.

## D12 — The one authorization checkpoint had no test of its own

`internal/service/access` shipped with **no test file at all**, and the whole
suite stayed green with its central decision deleted: replacing the owner
comparison in `authorizer.go` with `_ = owner`, so that it grants
unconditionally, left `go test -race ./...` at exit 0 across all 32 packages —
the ownership matrix (T147) included.

It behaved correctly anyway because `internal/store/medication`'s `owned()`
predicate is a second, independent refusal. That is good defence in depth and it
is exactly why the hole was invisible. The mirror holds too: deleting the store's
owner predicate leaves `internal/web/api` entirely green, and only the repository
contract catches it. T147 bites only when **both** layers are broken, which is
the one state neither layer being present was supposed to allow.

*Followed:* constitution Principle V — the checkpoint is the thing that must be
tested where it decides, not where its effects happen to be visible.

- `internal/service/access/authorizer_test.go` is the checkpoint alone: hand-written
  `Owners` fake, no database, no HTTP, no repository. Owner, stranger, guest,
  superuser, undeclared kind, a miss, and a lookup that could not answer.
- `internal/store/owner_integration_test.go` composes the real checkpoint over the
  real owner lookup against a real instance, which is the wiring
  `cmd/medikube/handlers.go` builds and the one no test exercised.
- `internal/store/medication/repo_integration_test.go` names the second layer's
  guard explicitly, with no checkpoint anywhere in the path, over all four
  operations.

Each was proved to bite by breaking only its own layer: neutering the owner
comparison reds the first two and nothing else; neutering the store predicate
reds the third and nothing else.

*Worth keeping:* two independent refusals mean a single-layer defect is
undetectable from the outside by construction. Every layer needs a guard that
can see it alone, or defence in depth degrades silently into one layer.

## D13 — The checkpoint failed open on any database error

`internal/store/owner.go` collapsed **every** error from the owner lookup into
`domain.ErrNotFound`. `internal/service/access/authorizer.go` turns `ErrNotFound`
into a **full grant** — research D-20's deliberate "grant for a record that is
not there", which is correct on its own. Composed, a failed read was a grant:

```
a cancelled owner lookup returns: store: reading the owner of a medication: not found
the checkpoint answered {Level:own} for account B on account A's record
```

The second consequence was quieter. The authorizer's own defensive branch — the
one whose comment reads "what stops a database outage reading as *that record is
not yours*" — was **unreachable dead code**, because the only production `Owners`
implementation could never produce anything but `ErrNotFound`. So
`internal/web/stream`'s `TestACheckpointThatFailsEndsTheStreamRatherThanPatchingAnyway`
passed against an injected fake while asserting a behaviour the shipped binary
did not have.

*Followed:* the API that exists. `dbx` returns `sql.ErrNoRows` and nothing else
for an empty result set (`dbx/rows.go:244`), and PocketBase's `execLockRetry`
deliberately leaves that one sentinel unwrapped while wrapping every other error
with `%w` (`core/db_retry.go:34`). Only `sql.ErrNoRows` is now a miss; everything
else propagates as itself, and the checkpoint's defensive branch is reachable
and covered.

## D14 — A live stream outlived the session that opened it

Authorization was re-run per event **for the record** and never **for the
identity**. `access.Actor` is built once by `web.WithActor` at subscribe and was
then frozen for the life of the connection. Revoking the session — a password
change re-randomises the record's token key (`core/record_model.go:1449`) and
kills every token signed for that account — stopped every ordinary request with
a 401 and did nothing whatever to an open stream. Measured on a real socket, a
row written *after* the revocation arrived on the revoked connection in full and
rendered.

spec.md:49 / FR-007: "the ended session MUST NOT be usable again from anywhere it
was still open." An open SSE connection is exactly that. FR-032 — "authorized
against the signed-in person **at the moment of access**" — says the same thing
from the other side.

Three individually-correct decisions removed every upper bound on the
consequence: `WriteTimeout: 0` in `cmd/medikube/main.go`, `clearWriteDeadline`
removing the per-request deadline (D-34, and necessary), and
`internal/httproute/routes.go` unbinding the rate limiter from this route. A
token stolen for ten seconds bought an indefinite live feed, and the victim's
standard remedy did not close it.

*Followed:* FR-007 and FR-032. `internal/web/stream/session.go` re-checks the
identity on every event **and** every heartbeat tick, and the check it applies is
deliberately the identical one PocketBase's `loadAuthToken` applies to every
ordinary request (`apis/middlewares.go:199`): would a request bearing this token
still be authenticated? Signature, expiry, the record's token key and the
collection's auth secret are all inside that one call, so revocation, sign-out
and expiry are one check rather than three, and the stream cannot end up with a
more generous notion of "signed in" than the rest of the application has.

Three notes on scope, since two of them were considered and declined:

1. **Maximum stream lifetime: covered, and by an existing authority.** Token
   expiry is part of the check above, so a stream cannot outlive the configured
   session TTL (`internal/platform/pb`'s `applySessionTTL`,
   `MEDIKUBE_AUTH_SESSION_TTL`) by more than one heartbeat. A second, invented
   number would be a way for the two to disagree.
2. **Concurrent-stream cap: deliberately not added here.** It is a resource
   control and not an authorization one — it bounds how many connections one
   account may hold, which is a different failure from a connection outliving
   its authority, and it needs a shared counter, a limit in `internal/config`
   and a documented refusal status. Nothing in this phase's spec fixes that
   number, and picking one here would be an amendment made in a bug fix.
   Recorded as open.
3. **A revoked stream ends silently.** A session ending is routine — a sign-out,
   a password change, an expiry — so the connection closes and nothing is
   reported; the browser's reconnect is refused by the route's own
   authentication, which is what an ended session should look like from every
   direction. A re-check that could not be *made* also ends the stream, and that
   one **is** reported.

## D15 — `/register` when registration is closed: a three-way contradiction

*Found by:* US2 recon, before a line of T206 or T220 was written.

Five documents disagreed about what a closed instance answers, and one of them
had already been built:

| Source | `POST /api/v1/auth/register` | the `/register` page |
|---|---|---|
| `spec.md` FR-002 (normative) | "every attempt … MUST be refused" | "MUST render an explanation inside the normal application frame" |
| `contracts/auth.md` | `403 registration_closed` | renders normally |
| `research.md` D-18 | `403 registration_closed` | renders normally |
| `contracts/pages.md` | — | `404` |
| `tasks.md` T192, T206, T220 | `404` | `404` |
| `internal/web/errors.go` (built) | `403` | — |

*Resolved:* **`403 registration_closed` for the API; the page renders an
explanation inside the normal frame.** FR-002 is the only normative source and
it says *render an explanation*, in as many words. `contracts/auth.md`,
`research.md` and the code already agree with it; `tasks.md` and
`contracts/pages.md` are the outliers and are amended.

`contracts/auth.md`'s stated reason for the page rendering — "a bare 404 would
fail the smoke gate's landmark assertion" — is *also* wrong, though its
conclusion is right: `e2e/instance.mjs` sets `MEDIKUBE_AUTH_REGISTRATION_OPEN:
'true'` and `data-model.md` says the smoke environment opens registration, so
the gate never reaches the closed branch. The reason is replaced; the answer
stands.

There is no privacy argument for the 404 here. A 404 is what this codebase
answers for **owner-scoped data**, so that a stranger cannot learn a record
exists. Whether an operator has opened self-registration is an instance-wide
configuration fact, identical for every caller and discoverable from the sign-up
page itself. Hiding it buys nothing and costs the person an explanation.

T220's actual concern — that `/register` is registered **unconditionally**, so a
route cannot vanish under configuration where the inventory gate cannot check
it — is untouched and still holds.

## D16 — A duplicate email is a `409`, and that is deliberate

*Found by:* US2 recon.

`contracts/auth.md` answers a duplicate address `409 conflict`, "that address
cannot be used". `tasks.md` T192 required "a duplicate email answered **exactly**
as a successful-looking outcome that reveals nothing". These are opposites.

*Resolved:* **`409`, per the contract.** FR-003 says the system "MUST refuse to
create a second account for an email address that already has one". It says
refuse. It does not say answer indistinguishably — and the spec is explicit
where it does want that: FR-005 requires it for sign-in ("so that neither can be
probed") and FR-073 for password recovery. Registration is deliberately not on
that list. T192's clause is an addition to the normative requirement, not a
reading of it, and is amended.

*The residual exposure, recorded so it stays a decision rather than an
accident:* registration is therefore an account-existence oracle. Anyone may
probe whether an address is registered by attempting to register it. Three
things bound it and none of them removes it: the response is rate-limited
(`429`), the message is deliberately vague — "that address cannot be used" does
not say *already registered*, and an address may be unusable for other reasons —
and on a closed instance the endpoint refuses everyone identically. If a later
phase wants this closed properly, the mechanism is the standard one: answer every
registration identically and move the truth into the email that follows, which
costs a working mail configuration (see FR-076, and T223e for what this instance
does when it has none).

## D17 — T198's SQL asserts something that is never true (measured, not argued)

*Found by:* US2 recon, empirically, against a real instance.

`tasks.md` T198 required that after `deleteMe`,
`SELECT COUNT(*) FROM audit_events WHERE actor = '<id>'` is **greater than 0**,
describing the actor as becoming "a dangling id". Measured result: **0**.

```
medications before delete: 12
medications after delete:   0
audit_events WHERE actor = '<id>':  0
audit_events total rows after delete: 1
surviving row: actor="" target_id="mkacctamara0001" action="account_delete"
account B's medications after A's delete: 3
```

*Mechanism:* `core/record_model.go` `deleteRefRecords`. `audit_events.actor` is
`CascadeDelete: false` **and** `Required: false`, which is neither the cascade
branch nor the hard-error branch — so PocketBase **unsets the field and
`SaveNoValidate`s the row**. The actor becomes the empty string. It does not
dangle.

*Resolved:* `data-model.md` is right — it says "unset rather than cascaded … so
the `account_delete` row survives", which is exactly what happens.
`contracts/account.md` is right. `tasks.md` T198 misread "unset" as "dangling"
and is amended to assert what the design actually produces:

- `SELECT COUNT(*) FROM medications WHERE owner = '<id>'` is `0`
- `SELECT COUNT(*) FROM audit_events WHERE target_id = '<id>' AND action = 'account_delete'` is `> 0`
- and that surviving row's `actor` is `''`

The third clause matters. Asserting the row count alone would pass on a row that
was about somebody else entirely, and `actor_kind` is the only surviving evidence
that a person rather than the system did it.

*Also measured, and now load-bearing:* deleting an account fires
`OnRecordAfterDeleteSuccess` **13 times** — once per cascaded medication, once
for the user. `internal/platform/pb/hooks.go`'s record stream filters by
collection name, so the `users` event is dropped and twelve delete events are
published. That is correct as written, but it is now a property something
depends on rather than an incidental one.

## D18 — `task test:phileak` asserts nothing and exits 0

*Found by:* US3 recon, while surveying what T235 has to build on.

`Taskfile.yaml` runs the PHI-leak suite as `go test -tags=phileak …` and its own
comment says it is "Build-tagged and therefore invisible to `task test`".
`CLAUDE.md` repeats the claim: "`task test:phileak` is build-tagged and therefore
invisible to `task test`: it boots an instance and drives every endpoint against
sentinel data."

Neither is true today. The repository contains exactly **one** build tag:

```
$ grep -rn '//go:build' --include=*.go .
internal/web/stream/timeout_test.go:1://go:build sselive
```

There is no `phileak` tag on any file. `internal/testsupport/phileak/` holds
`capture.go`, `capture_test.go`, `doc.go` and `sole_test.go` — no `exercise.go`
and no `phileak_test.go`. So `-tags=phileak` selects nothing extra, and
`task test:phileak` re-runs the same untagged tests `task test` already ran, in
about 0.09s, and exits 0.

This is not merely "T235 is unbuilt", which would be fine and expected — T235 is
a Phase 5 task. The defect is that the gate **reports success while asserting
nothing**. Anyone reading a green `task test:phileak` — a person, CI, or an
agent writing an integration report — concludes the PHI-leak property holds. It
has never been checked. A gate that cannot fail is worse than an absent one,
because an absent one is visible.

*Resolution:* T235 builds `exercise.go` and `phileak_test.go` **behind a real
`//go:build phileak` tag**, which is what makes `-tags=phileak` mean something.
Until then the Taskfile comment and `CLAUDE.md`'s description are aspirational
and are marked as such rather than read as fact. Whoever closes T235 must
verify the tag actually selects the new files — by confirming `task test`
does **not** run them and `task test:phileak` does — rather than trusting an
exit code that was already 0 before the suite existed.

*Related, same shape:* T229, T236 and T237 assert that tracing, Sentry and
metrics are inactive when unconfigured. No production code imports otel,
sentry-go or prometheus at all — the only importer is the phileak capture. Those
three properties are currently true **by absence**, which is a different fact
from true by design, and a test written today would pass without exercising
anything. Each needs the corresponding subsystem to exist before its "inactive
when unconfigured" test carries meaning.

## D19 — Nothing serves the browser assets, and no task closes the gap

*Found by:* Phases 6–9 recon.

MediKube builds two browser assets and delivers neither. Four facts, each
measured:

1. **They exist.** `internal/web/static/` holds `app.css` (20.8 KB, generated by
   the Tailwind step) and `datastar.js` (34 KB, the vendored v1.0.2 runtime).
2. **Only one is embedded.** The single `//go:embed` directive in the entire
   repository is `internal/web/static/embed.go`'s `//go:embed datastar.js`.
   `app.css` is embedded by nothing, because it is gitignored and absent from a
   clean checkout, so it cannot be named in a pattern that must compile before
   `task gen` runs.
3. **Neither is served.** `httproute.KindAsset` is declared
   (`internal/httproute/registry.go:37`) and **no route of that kind is
   registered anywhere**. The only other mention is a `switch` arm in
   `internal/openapi/generate.go:56`.
4. **The shell therefore links neither, deliberately.**
   `internal/web/views/shell/document.templ:20-25`: "No stylesheet and no script
   tag yet, deliberately. Nothing in this build serves internal/web/static — no
   route table row declares an asset route and no contract fixes the URL — so a
   link here would be a failed network request on every page … **T261 adds both
   alongside the asset route they need.**"

That deferral is sound reasoning and it is correctly documented. The defect is
where it points. T261 reads, in full: "Implement
`internal/web/views/layout.templ` — the shell: skip link, banner, nav, main,
`#error-banner`, `#toast`, footer." It does not mention a stylesheet, a script
tag, or an asset route. `grep -in 'KindAsset|asset route|serve.*static'` over
`tasks.md` returns **nothing**. No task in the plan registers the asset route.

So the deferral has no owner, and the current state is not a stub anyone will
trip over: every test passes, the smoke gate passes, and the application serves
**unstyled HTML with no client runtime**. The `class` attributes in every
`.templ` resolve to nothing in a browser. This is the single largest gap between
"the suite is green" and "the application works" in the repository.

*Resolution:* T261 is amended to name the three pieces of work — register the
`KindAsset` route, extend the embed to cover `app.css` once `task gen` has
produced it, and add the `<link>` and `<script>` to the shell — or a new task is
added for the route and the embed. Whoever does it must also confirm the CSP in
`internal/web/security.go` admits both, and re-check
`contracts/pages.md` smoke assertion 7, which
`document.templ` cites as the reason the links are absent today.

*Related, and it compounds this:* `assets/input.css`'s `@source` glob is
currently redundant. Tailwind v4 auto-detects sources from the stylesheet's
location upward unless told otherwise, and `@source` is additive rather than
restrictive, so the bundle is built from the whole checkout. Measured evidence:
`app.css` contains utilities named `[install:tailwind]`, `[tailwind:install]`,
`[templ:install]` and `[test:cover]` — strings that appear in `Taskfile.yaml`
and in `specs/**/*.md` and **in no `.templ` file**. Three consequences: the
failure mode `CLAUDE.md` and `assets/input.css` both warn about ("a glob that
matches nothing produces an empty stylesheet and exits 0") **cannot occur**, so
anyone who "fixes" the glob on the strength of that comment gets no signal
either way; a mistyped class name in a `.templ` still reaches the bundle if the
same string appears anywhere in the repo, including in prose, which defeats the
"does this class actually compile" assertion `.github/workflows/go.yaml:47-52`
says it is deferring; and the shipped artefact carries documentation-derived
noise. The fix is `@import "tailwindcss" source(none);` alongside the explicit
`@source`. This belongs with T265, the only task naming `assets/input.css`, and
T265 does not mention it.

## D20 — T197 and `contracts/account.md` disagree about a password-change refusal

`tasks.md` T197 requires that a refused password change "does not confirm whether the
supplied current password was the wrong one or the new one invalid" — one message for
both halves.

`contracts/account.md`'s own table gives them **two different answers**:

| Case | Status | Body |
|---|---|---|
| `current_password` absent or wrong | `422` | field `current_password`, code `incorrect` |
| new password violates a published rule | `422` | field `new_password` |

Those are opposites. A response naming `current_password` with code `incorrect` *is* a
confirmation of which half failed.

*Followed:* T197. `identity.Service.ChangePassword` builds one `*domain.ValidationError`
in one constructor — `refusedPasswordChange` — reported on **both** fields with **one**
message, whichever half actually failed. `TestAPasswordChangeRefusalDoesNotSayWhichHalfFailed`
compares the five refusal cases as marshalled bytes, so a message, a field name, a code or
an ordering that differed between them fails.

*And a dissent, recorded because implementing it is not the same as agreeing with it.*
T197 buys very little and costs real usability. The caller is **already authenticated** and
is changing **their own** password, so there is no account-existence oracle to close. An
attacker with a live session who wants to distinguish the two need only send a new password
they know is valid — any refusal then means the current password was wrong, and the merged
message has told them exactly what the split message would have. Meanwhile a person who
mistyped a 7-character new password is now told their current password might be wrong.
`contracts/account.md`'s split is the better design and the requirement it fails is one
nobody can state a threat model for. If T197 is ever revisited, the fix is to adopt the
contract's two field codes and delete `refusedPasswordChange`.

*Fix, whichever way it goes:* make T197 and `contracts/account.md` say the same thing.

## D21 — T202's "fixed dummy hash" is not what PocketBase does

`tasks.md` T202 and `research.md` D-17 both say the unknown-address sign-in path compares
against "the **fixed** dummy hash". PocketBase's own anti-enumeration check does something
else — `apis/record_auth_with_password.go:125-136`:

```go
record := &core.Record{}
err := app.RecordQuery(collection).Limit(1).One(record)   // ANY existing record
if err != nil { return }                                  // EARLY RETURN, no bcrypt at all
_ = record.ValidatePassword("")
```

It samples a row, and on an **empty table it performs no comparison whatsoever**, which
restores the whole oracle on a fresh instance. It is also unexported, so MediKube cannot
call it.

*Followed:* the spec, over PocketBase. MediKube supplies its **own fixed dummy**, which has
no "no rows" branch and costs one extra round trip less. `identitytest`'s authenticator
holds `dummyCredential` and counts every comparison through one seam, so
`TestEverySignInRefusalCostsOneComparison` asserts the **mechanism** — one comparison for a
wrong password, one comparison *and* one dummy comparison for an address with no account —
rather than a clock (Constitution VIII; the latency is T202a's, which does not gate).

*Consequence for `internal/store/identity`:* the PocketBase adapter must do the same, and
must expose the same counting seam. A `Compare` that fell back to `dummyPasswordCheck`'s
sampled row would pass the service tests and reopen the oracle on an empty `users` table.
The dummy's bcrypt cost must equal the collection's `PasswordField.Cost` (0 → `bcrypt.DefaultCost`
= 10 today) or the equalisation drifts the day an operator raises it; assert it rather than
assume it.

## D22 — T200's file cannot hold the half of T200 that matters

`tasks.md` T200 names `internal/service/identity/revocation_test.go` and asks it to prove that
"after a password change, **every** session issued before it stops working". A test in
`internal/service/**` cannot prove that. depguard forbids PocketBase there (Principle II), so
nothing in that package can mint a token, present one, or watch one be refused: the strongest
statement available is that the service reached `Authenticator.SetPassword`, against a fake whose
"session" is an integer generation.

That statement is worth having and it is already made — `service_test.go`'s
`TestAPasswordChangeEndsEverySessionIssuedBeforeIt` and `TestSignOutEndsEverySessionAndRecordsIt`,
plus `identitytest.RunRepositoryContract`'s `TestSetPasswordSpendsEveryLinkMintedBeforeIt`, which
holds **both** implementations to it. A second file in the same package asserting the same thing
against the same fake would be duplication wearing coverage's clothes.

*Followed:* the requirement, over the path. FR-010 is asserted at every layer that can observe it,
each with a guard that fails when only that layer breaks:

| Layer | File | What it would miss alone |
|---|---|---|
| service, against fakes | `internal/service/identity/service_test.go` | an adapter that never rotates |
| both adapters, one suite | `internal/service/identity/identitytest/contract.go` | a transport that does not re-check |
| ordinary requests, real token, **both transports** | `internal/web/api/session_revocation_test.go` | an open stream that outlives the session |
| an already-open stream | `internal/web/stream/revocation_test.go` | — |

Verified by deletion: with the password changed by a raw `UPDATE` that bypasses
`onRecordSaveExecute` — the exact defect `internal/store/identity`'s `SetPassword` doc warns about
— `internal/web/api` goes red on all four transport rows while the service suite stays green.

*Fix:* point T200 at `internal/web/api/session_revocation_test.go`, or split it into T200 (service,
already satisfied) and T200a (real instance). Nothing needs to be built either way.

## D23 — `contracts/auth.md`'s per-call refusal messages contradict the envelope

`contracts/auth.md` prescribes the *wording* of the duplicate-address refusal:
`409 conflict` with the message "that address cannot be used". `contracts/README.md`'s error
envelope prescribes something incompatible with that: **one message per code**, so that the
message is a property of the code and never of the call site — which is what lets a client
localise the code and lets a reader of the log stream know that two `conflict` lines mean the
same thing. `internal/web/errors.go`'s `Message` is that table, and it has one entry for
`CodeConflict`: "that conflicts with something already recorded".

Only one of the two can hold. Both cannot: the moment `register` answers `conflict` with its own
sentence, `conflict` has two messages and the envelope's invariant is gone — and it is gone for
every future operation too, because the precedent is the whole of the rule.

*Followed:* the envelope, over the sentence. The **security** properties `contracts/auth.md`
attaches to that message are the reason it names one at all, and both of them survive intact and
are asserted (`internal/web/api/auth_test.go`,
`TestRegisteringAnAddressThatAlreadyHasAnAccountIsRefused`, over three letter-cases):

- the body does not contain the submitted address, in any case; and
- the body does not say "registered" or "exists" — it does not confirm to an anonymous caller
  that a specific person has an account on this instance (D16).

What is lost is only the exact phrasing, which no requirement depends on. Verified by deletion:
rewriting `Message(CodeConflict)` to "that address is already registered" turns all three
sub-tests red.

*Fix:* either drop the prescribed sentence from `contracts/auth.md` and let it say what the
refusal must **not** contain (which is the part that matters), or add a `conflict`-family code
whose one message is "that address cannot be used" — at the cost of a published code that exists
to carry a sentence. The first is the smaller change and the honest one.

## D24 — `contracts/account.md`'s `MeCounts` cannot be written in Go

`contracts/account.md` gives `MeCounts` as a struct with a member per kind:

```go
type MeCounts struct {
    Medications int `json:"medications"`
}
```

That file cannot exist in this repository. `internal/architecture/kind_literals_test.go` (T046,
research D-05, cross-artifact finding H1) fails any Go file outside `internal/domain/kind/kind.go`
whose source contains a kind's path segment or collection as a string literal — and a struct tag
**is** a string literal. `medications` is medication's segment *and* its collection, so the tag is
two offences in one, and there is no accessor call available inside a tag: `json:"…"` is fixed at
compile time and `kind.Medication.Segment()` is not.

It is not a spurious catch either. The tag is precisely finding H1's failure mode — a second place
that spells the plural, which drifts silently the day a kind's segment is not the mechanical plural
of its name (`insurance`, `family-history`). Phases 002 through 006 add five more kinds, each of
which would be another member and another literal here.

*Followed:* the wire shape, over the Go shape. `MeCounts` is
`map[string]int`, keyed by `kind.Kind.Segment()`, populated from
`records.Handler.Segments()` — every kind the build actually serves. The published JSON is
byte-identical to what the contract describes (`"counts": {"medications": 12}`), the key comes from
the kind table rather than from a tag, and the object gains a key on the day a kind is registered
rather than on the day somebody remembers to add a member.

The map costs one property the struct had for free — a missing key and a zero count are the same
value to a reader — so `internal/web/api/me_counts_test.go`'s
`TestTheCountsObjectNamesEveryKindThisBuildServes` asserts the key set against
`Records.Segments()`, which is what a struct would have given by compiling.

*Fix:* replace the struct in `contracts/account.md` with the wire shape and a sentence saying the
keys are kind segments. Nothing needs to be built.

## D25 — T205's file cannot hold the half of T205 that matters

T205 asks for `internal/platform/pb/hooks_auth_test.go` to assert that `OnRecordAuthRequest`
writes the `login` row **for both paths to a session** — MediKube's `/api/v1/auth/login` and
PocketBase's native `/api/collections/users/auth-with-password`. Driving MediKube's own route
needs MediKube's router, which is `internal/web/apitest`; that package's harness reaches
`internal/testsupport`, and `internal/testsupport/app.go` blank-imports
`internal/store/migrations`.

`core.AppMigrations` is a **package-level registry**. An import anywhere in a test binary applies
MediKube's migrations to every `tests.NewTestApp` in it, including the ones that are supposed to
meet PocketBase's stock schema. `internal/platform/pb/hooks_records_test.go`'s `stockSchema` exists
as the tripwire for exactly this, and it fired: adding the import turned
`TestRecordRequestHooksDoFireForASuperuser` into a 400 with six "Cannot be blank" refusals from
columns that collection is not supposed to have.

*Followed:* the tripwire. The split is by what each layer can see:

- `internal/platform/pb/hooks_auth_test.go` and `hooks_admin_session_test.go` — the hook's
  mechanics against PocketBase's own fixture and a fake trail: the row's shape, the renewal that
  writes nothing, the refusal that names an account and never an address, the priority that puts
  the row ahead of the response, the superuser's empty `actor`.
- `internal/web/api/auth_audit_test.go` and `auth_audit_trail_test.go` — the both-paths proof,
  where the router already is: one `login` row per path, one `login_failed` per refused path, none
  for a renewal on either, and a sign-in that could not be recorded handing over no session.

*Fix:* reword T205 to name both files, or note that the second half belongs to the HTTP tier. The
assertion T205 asks for exists; it is one directory away from where the task put it.

## D26 — `OnRecordAuthRequest` fires before the second factor, so the `login` row can outrun the sign-in

Measured, not argued. PocketBase raises `OnRecordAuthRequest` from `apis.RecordAuthResponse`
(`apis/record_helpers.go:71`), and on the password route that call is reached once the **first**
factor is accepted — `checkMFA` runs earlier in the same handler and, when a second factor is
required, answers `401 {"mfaId": …}` through a path that has already raised the hook. On a
collection with MFA enabled, the audit trail therefore gains a `login` row for a sign-in that never
completed, and `login_failed` alongside it from the wrapper on the refusal.

This is not live: `internal/store/migrations/1756100100_users_profile.go:165` sets
`users.MFA.Enabled = false`, and `data-model.md` gives the account collection one factor. It was
found because PocketBase's own test fixture ships MFA **on**, which is what the pb-tier tests boot
against — they configure it the way MediKube's migration does, in `harness.withoutMFA`.

The assumption is now load-bearing rather than incidental:
`internal/platform/pb/hooks_auth_test.go`'s
`TestTheAccountCollectionHasNoSecondFactorForTheLoginRowToOutrun` pins
`users.MFA.Enabled == false` and says why.

*Fix:* whichever phase introduces a second factor must move the `login` row off
`OnRecordAuthRequest` and onto whatever fires after `checkMFA` succeeds. FR-036's row would
otherwise say a person signed in when they demonstrably did not — the worst possible failure for an
audit trail, because it is wrong in the direction of claiming access happened.

## D27 — Two composition roots, and the suite only ever exercised one

Every hook MediKube binds into PocketBase is bound **twice**: in `cmd/medikube/handlers.go`, which
is what the binary runs, and in `internal/web/apitest/apitest.go`, which is what every HTTP test
drives. No test drove the first.

Measured: deleting `pb.BindRecordAudit` from `cmd/medikube/handlers.go` — the whole call, in the
real composition root — leaves `go test ./...` **green**. So does deleting `pb.BindAuthAudit`. A
deployment would stop writing FR-036's rows entirely while the suite went on proving that it wrote
them, because the suite was proving it about the other wiring.

This is the failure mode the phase's own rule names: a control defended in two layers needs a guard
that fails when **only that layer** breaks, and "both layers broken" is the wrong sensitivity.

*Followed:* held the two roots together.
`internal/architecture/TestTheBinaryAndTheHTTPHarnessBindTheSamePlatformHooks` parses both packages
and compares the set of `pb.Bind…` calls. Delete a binding from one root and it fails; delete it
from both and the HTTP suite fails, because the harness no longer binds what its assertions are
about. Neither guard is sufficient alone, which is why the new one is a separate file rather than
one more assertion inside an existing test.

*Fix:* the durable version is one wiring function both roots call, which would make the guard
unnecessary. That is a refactor of two composition roots and belongs to whichever task owns them;
until then the equality is what stands between the binary and a silent loss of the audit trail.

## D20 — The PHI-leak suite works, and it found three leaks on its first real run

*Found by:* T235, the moment it existed. Recorded here rather than fixed,
because two of the three are in code this story did not touch and the fix for
one of them is a design decision rather than an edit.

Defect D18 recorded that `task test:phileak` exited 0 while asserting nothing:
the repository had no `phileak` build tag, so `-tags=phileak` selected nothing
and the command re-ran the ordinary suite. That is now fixed — the tag exists and
selects, measured: 16 tests untagged, 24 tagged. The suite then failed, which is
the first time it has ever been able to.

**1. An email address reaches the log stream.** `internal/obs/request_log.go`'s
`record()` writes `Err(Cause(err))`, and PocketBase's own native recovery routes
produce `failed to fetch users record with email`, carrying the address. The
sink is the zerolog stream, so the address lands in the operational record of
every deployment. Two possible fixes, and choosing between them is the decision:
record the *classified public* error rather than the raw cause for statuses
MediKube answered as a success, or drop the documented native recovery routes
from the reachable set in `contracts/README.md`.

**2. The same address reaches Sentry.** `internal/obs/sentry.go`'s `scrub()`
does not touch `event.Exception`. sentry-go serialises the whole `Unwrap` chain,
so reporting `Cause(err)` is not sufficient — the allowlist has to cover the
exception values too, or `Report` must be handed an error whose chain has been
severed.

**3. A medication name reaches an OTel span attribute.** The recorder carried
`hereditary-angioedema-prophylaxis` in `deployment.environment.name`. This one is
NOT yet confirmed as a production leak: the sentinel may have been seeded into
the environment name by the harness itself, in which case it is a test artefact
rather than a defect. It must be resolved either way before the suite can be put
on the merge gate, and it is called out here so nobody reads a red phileak run as
"only the known two".

*Why this is not fixed in this commit:* leaks 1 and 2 are in `internal/obs`,
written during the edge-hardening work, and both fixes change what an operator
sees in their logs and their error reporter — that is an operability decision, not
a tidy-up. Leak 3 needs a measurement before it is even known to be real. What
this story delivers is the instrument: a suite that can no longer pass by
asserting nothing, and that names the sink and the sentinel on failure.

*Consequence for CI, stated plainly:* `task test:phileak` is NOT on any workflow,
so nothing in CI runs it and the PR is green with these three open. That is the
same shape of hole D18 described, one level up. Putting it on the gate is the
obvious next step and it cannot happen until all three are resolved.

**Resolution (2026-09-04).** All three are closed and the suite is on the gate.

1. and 2. share one fix, `internal/obs.Recordable`: the request logger and the
   Sentry reporter both record the *recordable* error rather than the raw
   `Cause`. At or above 500 that is still the cause — the driver message is what
   makes a 500 actionable. Below 500 it is the cause only when the chain ends
   in one of `internal/domain`'s sentinels, whose messages are PHI-free by
   contract, and otherwise a fixed stand-in that names the status and says the
   cause was withheld. PocketBase's 204-with-error therefore produces a line
   that is identical whether or not the address has an account, which closes
   the enumeration oracle as well as the disclosure. The Sentry scrubber
   additionally keeps one exception value rather than the whole `Unwrap` chain
   (`MaxErrorDepth` cannot express "outermost only": zero means the default).
   `Reporter.Report` now takes the request event, so the status decision is the
   reporter's own and not the caller's.
3. was a harness artefact, as suspected: `exercise_test.go` passed the
   `Indication` sentinel as the tracing `Environment` where every other sink got
   the `environment` constant. Nothing in the application composes a span
   attribute from a record.

The `knownLeaks` register is empty, `task test:phileak` runs as the `phi-leak`
job in `.github/workflows/go.yaml`, and a leak now fails the merge.

## D28 — `contracts/cli.md`'s Cobra subcommands contradict `plan.md`'s own transitive-pin rule

`plan.md` pins `github.com/spf13/cobra` as transitive-only, never a direct require, and
`internal/architecture/transitive_pins_test.go` enforces it: no MediKube file may import it, and
go.mod must keep it `// indirect`. `contracts/cli.md` and research D-12 describe MediKube's own
commands (`routes`, `openapi`, `healthcheck`, `seed`) as `*cobra.Command` values added to
`app.RootCmd` — but constructing one requires importing cobra in the file doing the constructing,
which is exactly what the pin forbids. The two cannot both hold.

The pin wins: it has a measured failure mode (a stray `go get -u` pinning a cobra version
PocketBase wasn't built against) and the Cobra-subcommand design doesn't have one of its own.
`cmd/medikube/main.go`'s `run()` now dispatches MediKube's four commands out of `os.Args` before
`app.Execute()` (`internal/cli.Dispatch`), each with its own `flag.FlagSet`. Only `migrate` stays a
real RootCmd subcommand, via `migratecmd.Register(app, app.RootCmd, ...)`, which takes `app.RootCmd`
already typed as `*cobra.Command` without MediKube spelling that type itself. What's lost:
PocketBase's RootCmd no longer lists MediKube's four commands on its own, so `medikube help`/`-h`
now prints `internal/cli.Usage` first and a disposable RootCmd's own `--help` beneath it.
