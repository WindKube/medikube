# Spec defects

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
