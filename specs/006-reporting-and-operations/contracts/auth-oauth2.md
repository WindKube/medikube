# Contract: `POST /api/v1/auth/oauth2` (operation 4)

Conventions, status codes and the audit rules are in [README.md](./README.md). Requirements
covered: FR-134, FR-135, FR-136, FR-137, SC-028.

**This operation is public**, and it is the only public operation in this phase. Everything else
here requires `role = admin`.

**The contract allocated operation 4 to nobody** (SHARED-DESIGN §2.3, "deferred out of the suite").
Phase 001 recorded external sign-in as out of scope and no later phase claimed it, so it belonged
to no phase at all — cross-artifact finding **H7** (analysis run 2026-08-27; this line read **H6**
until 2026-08-27, which was a different finding entirely — "002 has no deviation table". Every other
site in this phase and in 001 says H7: `plan.md:323`, `:635`, `contracts/pages.md:15`,
`contracts/README.md:27`, `tasks.md:613`, `001/plan.md:807`, `001/contracts/README.md:135`).
It lands here because providers are operator
configuration and this phase owns the operator surface. Password recovery and email confirmation,
which are day-one flows, went the other way and landed in phase 001.

## What PocketBase already does, and what MediGo adds

| PocketBase v0.40.1 | MediGo |
|---|---|
| Provider registry and credentials in `Settings().OAuth2.Providers`, edited in the admin UI | Reads whether any provider is enabled — nothing more |
| The whole authorize/callback exchange: `POST /api/collections/users/auth-with-oauth2`, `GET\|POST /api/oauth2-redirect` | Recorded as documented externals (§2.4), unchanged |
| Linking a provider identity to an existing auth record through `_externalAuths` | Nothing. FR-137 is satisfied by that mechanism, not by a MediGo one |
| Token issue, through `apis.RecordAuthResponse` | Nothing |

MediGo adds **one DTO wrapper**, so that a provider sign-in produces the same `Session` shape,
the same `HttpOnly` cookie and the same audit row as `POST /api/v1/auth/login` — and so that
`role` and `disabled_at` are unreachable from the request by construction, exactly as they are on
registration.

**No new configuration mechanism.** `Settings().OAuth2` is PocketBase's settings store, which the
constitution's Technology Constraints carve out of the `caarlos0/env`-only rule for precisely this
reason. MediGo neither mirrors providers into `MEDIGO_*` nor builds a screen to edit them.

---

## `signInWithOAuth2` — `POST /api/v1/auth/oauth2`

```go
type OAuth2SignIn struct {                 // no `role`, no `disabled_at`, no `verified`
    Provider     string `json:"provider"`
    Code         string `json:"code"`
    CodeVerifier string `json:"code_verifier"`
    RedirectURL  string `json:"redirect_url"`
}
```

| Case | Status | Body |
|---|---|---|
| success | `200` + `Set-Cookie: medigo_session` | `Session` (phase 001, `contracts/auth.md`) |
| no provider is configured on this instance | `404` | `not_found` — the route answers as though it does not exist, because on such an instance it effectively does not |
| the named provider is not one of the configured ones | `404` | `not_found` — **byte-identical to the line above** |
| the exchange is refused by the provider, or the code is stale | `401` | `unauthenticated` |
| the account the address resolves to has `disabled_at` set | `401` | `unauthenticated` — **byte-identical to the line above** (FR-135, and phase 001's `login` rule) |
| repeated attempts | `429` | `rate_limited` |
| body carries `role`, `disabled_at` or `verified` | `422` | `validation_failed`, field code `unknown_field` |

**FR-134's "offer no such route when no provider is configured" is enforced twice**: the page layer
renders no provider button when `GET /api/v1/auth/config` reports none, and this operation answers
`404` regardless of what a caller constructs by hand. A route that exists only under some
configurations would be a route the Principle IX inventory gate cannot check, so the route is
always registered and it is the **answer** that changes — the same rule phase 001 applies to
`/register` under closed registration.

`GET /api/v1/auth/config` (phase 001, operation 1) gains one field for this:

```go
type AuthConfig struct {
    RegistrationOpen bool     `json:"registration_open"`
    PasswordRules    PwRules  `json:"password_rules"`
    OAuth2Providers  []string `json:"oauth2_providers"`   // names only, never a client id or secret; [] when none
}
```

**FR-135 in full.** A provider sign-in reaches PocketBase's own `authWithOAuth2`, which either
matches an existing `_externalAuths` link, or links by verified email, or creates a record —
according to the operator's own provider settings. MediGo asserts the invariants on the way out:
the resulting record's `role` is whatever it already was (`user` for a newly created one), its
`disabled_at` is untouched, and a disabled account is refused with the same body as a wrong
password. The sign-in audit row is written by `OnRecordAuthRequest("users")`, the hook phase 001
binds — so a provider sign-in is audited by the same code path as a password sign-in, and neither
can be forgotten independently (phase 001 research D-14).

**FR-137.** One provider identity links to exactly one account, which is `_externalAuths`'
uniqueness rather than a MediGo rule; the mandatory test proves an attempt to attach a linked
provider identity to a second account is refused and audited, and that a second account is never
silently created for an address that already has one.

**FR-136.** `GET /api/v1/admin/stats` reports `posture.oauth2` as `configured` or `unconfigured`,
alongside `smtp`, `superuser_mfa` and `superuser_ip_allowlist` — see
[admin-instance.md](./admin-instance.md).

## Mandatory tests

1. With no provider configured: `404`, and `GET /api/v1/auth/config` returns `[]`, and no provider
   control appears in the rendered `/login` page (SC-028, asserted in the templ render test as well
   as in the browser gate).
2. A `role: "admin"` in the body is `422`, **and** the resulting account — if one is created — is
   `user`, asserted against the database rather than against the response.
3. A disabled account signing in through a provider gets a response byte-identical apart from
   `request_id` to a wrong-password refusal, and writes `login_failed`.
4. A successful provider sign-in writes exactly one `login` / `user` audit row, through the same
   hook as a password sign-in.
5. An unknown provider name and no-providers-configured produce byte-identical `404` bodies.
