# Contract: `/api/v1/auth/*`

Nine operations. Requirements covered: FR-001 … FR-010, FR-034, FR-036, FR-041, FR-054,
FR-073 … FR-077.

PocketBase owns the mechanism; MediKube owns the shapes. Login completes through
`apis.RecordAuthResponse(e, rec, core.RequestInfoContextPasswordAuth, nil)`, which mints the
token, fires `OnRecordAuthRequest` and records the auth origin. MediKube never mints a token, never
hashes a password and never re-implements refresh (research D-13).

**Password recovery and email confirmation are in this document** (cross-artifact finding H7).
Both are wired to PocketBase's own flows: `mails.SendRecordPasswordReset` and
`mails.SendRecordVerification` build and send the message,
`app.FindAuthRecordByToken(token, core.TokenTypePasswordReset)` — or `core.TokenTypeVerification` —
validates the token, and `SetPassword` + `Save` rotates `tokenKey`, which is the same mechanism
that makes `logout` and `changePassword` end every other session. MediKube mints no token, renders no
email template and stores no reset state. The token lifetimes are PocketBase's collection defaults:
**30 minutes** for a reset token, **24 hours** for a confirmation token
(`core/collection_model_auth_options.go`).

**Sign-in through an external identity provider is NOT here.** `POST /api/v1/auth/oauth2` (shared
design contract operation 4) belongs to **phase 006**, with the operator surface that configures
providers. `OAuth2.Enabled` is `false` on `users` in this phase.

**Everything in this file depends on the operator having configured outgoing mail**, which lives in
PocketBase's `Settings()` store rather than in `MEDIKUBE_*` — the carve-out the constitution's
Technology Constraints make for exactly this reason. When `Settings().SMTP.Enabled` is false,
`app.NewMailClient()` returns `&mailer.Sendmail{}`, a shell-out to a local `sendmail` binary the
distroless image does not contain, so the send **fails** rather than silently disappearing. FR-076
makes that a specified behaviour: `503 mail_unconfigured` to the person, one log line per burst,
and a boot warning that stands alongside the superuser-MFA and IP-allowlist warnings until it is
fixed.

## Shared DTOs

```go
// internal/web/api

type AuthConfig struct {
    RegistrationOpen bool     `json:"registration_open"`
    PasswordRules    PwRules  `json:"password_rules"`
}

// PwRules is published so the sign-up form can state the rules BEFORE the person chooses,
// not only report them after they fail (FR-004).
type PwRules struct {
    MinLength      int  `json:"min_length"`       // 8
    MaxLength      int  `json:"max_length"`       // 200
    RejectsEmail   bool `json:"rejects_email"`    // true
    RejectsName    bool `json:"rejects_name"`     // true
}

type RegisterRequest struct {           // no `role`, no `disabled_at`, no `verified`
    Email    string `json:"email"`
    Name     string `json:"name"`
    Password string `json:"password"`
}

type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

// Session is the response of register, login and refresh. It carries NO token: the token is
// set as an HttpOnly cookie and is never readable by a script (research D-15).
type Session struct {
    User      Me     `json:"user"`
    ExpiresAt string `json:"expires_at"`   // RFC3339 UTC
}
```

**`role`, `disabled_at` and `verified` are absent from `RegisterRequest` by construction.**
FR-012 — "the system MUST NOT allow a person to set or change their own permission tier or
account status" — is therefore enforced by **shape**, not by a runtime check somebody can forget.
Because unknown fields are rejected, a request carrying any of them is `422 validation_failed`
with field code `unknown_field`, and the same test covers both.

---

## `getAuthConfig` — `GET /api/v1/auth/config`

**Public.** Tells the sign-up page whether to render a form or an explanation, and tells it the
password rules to publish.

**200**

```json
{ "registration_open": false,
  "password_rules": { "min_length": 8, "max_length": 200,
                      "rejects_email": true, "rejects_name": true } }
```

**Never** reveals how many accounts exist, whether a given address is registered, or anything
about the instance's data. It is safe to serve to an anonymous caller precisely because it says
nothing about anybody.

---

## `register` — `POST /api/v1/auth/register`

**Public.** Creates the account and signs the person in, inside one transaction.

| Case | Status | Body |
|---|---|---|
| success | `201` + `Location: /api/v1/me` + `Set-Cookie: medikube_session` | `Session` |
| registration closed | `403` | `registration_closed` |
| email already registered, in any letter case | `409` | `conflict`, message "that address cannot be used" |
| invalid email / name / password | `422` | `validation_failed`, **every** offending field at once |
| body carries `role`, `disabled_at` or `verified` | `422` | `validation_failed`, field code `unknown_field` |
| repeated attempts from one source | `429` | `rate_limited` |

**Requirements and how they are met.**

- **FR-001**: email, display name and password. Nothing else is required, and nothing else is
  accepted.
- **FR-002**: when `MEDIKUBE_AUTH_REGISTRATION_OPEN=false`, every attempt is refused. The `/register`
  page still renders inside the normal shell with a plain explanation, because a bare 404 would
  fail the smoke gate's landmark assertion.
- **FR-003**: the `LOWER(email)` unique index makes `Amara@x.test` and `amara@x.test` the same
  address. The whole operation runs in `app.RunInTransaction`, so **no partially created account
  survives a failure** — the losing side of two simultaneous registrations of one address gets a
  constraint violation, the transaction rolls back, and exactly one account exists (Edge Cases).
- **FR-004**: the password rules are validated by `identity.ValidatePassword` in the domain and
  again by PocketBase's `MinPasswordLength`. A password equal to the submitted email or name is
  refused with `same_as_email` / `same_as_name`.
- **FR-036**: writes one `create` / `user` audit row and one `login` / `user` row.

**The conflict message names no address.** "That address cannot be used" rather than "`amara@x.test`
is already registered": the second confirms to an anonymous caller that a specific person has an
account here, which on a self-hosted medical instance is a disclosure. The registering person
already knows which address they typed.

**Mandatory tests.** `201` and a usable session; the same address in different case is `409`; two
concurrent registrations of one address leave exactly one account (`SELECT COUNT(*)` asserted);
a `role: "admin"` body is `422` **and** the created account, if any, is `user` — asserted
directly against the database; a closed instance refuses and leaves no row.

---

## `login` — `POST /api/v1/auth/login`

**Public.**

| Case | Status | Body |
|---|---|---|
| success | `200` + `Set-Cookie: medikube_session` | `Session` |
| unknown address | `401` | `unauthenticated` |
| known address, wrong password | `401` | `unauthenticated` — **byte-identical to the line above** |
| account has `disabled_at` set | `401` | `unauthenticated` — **byte-identical again** |
| too many failures | `429` | `rate_limited` |

**FR-005 in full.** The three `401` bodies are identical apart from `request_id`, and a test
asserts that directly. They are also **built to take comparable time**: when the record is not
found the handler still performs a bcrypt comparison against a fixed dummy hash before returning,
because an identical body with a microsecond-versus-millisecond latency difference is still an
enumeration oracle (research D-17). **The gate asserts the dummy comparison, not the clock**
(T202, through a counting seam on the hash comparer): the mechanism is deterministic and the
latency is not, and Constitution VIII forbids a flaky gate assertion. The latency itself is
reported by the non-gating benchmark T202a (amendment 2026-08-27, ANALYSIS N13).

The disabled case is deliberately folded into the same response. Telling somebody "your account
is disabled" tells an attacker that the address is registered.

**FR-006**: every failure writes a `login_failed` / `user` audit row. PocketBase's rate limiter is
**enabled** at boot — it defaults to off — with a rule on this route, configured from
`MEDIKUBE_AUTH_LOGIN_RATE`. `/api/v1/streams/records` is exempted from the limiter, or a server
restart plus Datastar's reconnect loop locks every open tab out (research D-18).

**The `login_failed` row never carries the attempted email address** — `target_id` is the account
id when the address is known and **empty** when it is not. Writing the typed string would put a
real person's address, possibly a stranger's, into a two-year medical audit trail.

**Audit for success is written from `OnRecordAuthRequest("users")`, not from this handler.**
PocketBase's own `POST /api/collections/users/auth-with-password` also remains reachable, and a
handler-side audit would leave that path silently unaudited. The hook covers both (research
D-14).

**Mandatory tests.** The three `401` bodies are byte-identical apart from `request_id`; a
successful login is followed by an authenticated `GET /api/v1/me` using only the cookie; a login
through PocketBase's native route also produces a `login` audit row.

---

## `refreshSession` — `POST /api/v1/auth/refresh`

**Requires a session.** Exchanges the current token for a fresh one and re-sets the cookie.

| Case | Status |
|---|---|
| success | `200` + `Session` + `Set-Cookie` |
| no or expired session | `401 unauthenticated` |
| the token was invalidated by a password change or a sign-out elsewhere | `401 unauthenticated` |

**FR-008.** The session lifetime is `MEDIKUBE_AUTH_SESSION_TTL`, default **7 days**, written into
the `users` collection's `AuthToken.Duration` at boot. When it expires the person is asked to sign
in again **and told why** — the sign-in page renders an explanation when it is reached with
`?reason=expired`, which the page layer sets on a `401` redirect (FR-008, and the Edge Case about
a session expiring mid-form).

---

## `logout` — `POST /api/v1/auth/logout`

**Requires a session.**

| Case | Status |
|---|---|
| success | `204` + `Set-Cookie: medikube_session=; Max-Age=0` |
| no session | `401 unauthenticated` |

**FR-007 in full: the ended session must not be usable again from anywhere it was still open.**
That is implemented by `rec.RefreshTokenKey()` and a save, which rotates the per-record signing
key and invalidates **every** outstanding token for the account in one write (research D-16).

**This means signing out on a phone also signs out the laptop.** FR-007 asks for exactly that
("from anywhere it was still open"), so it is the specified behaviour rather than a surprise —
and the settings page says so in plain language before the person clicks.

Writes a `logout` / `user` audit row.

**Mandatory test.** Sign in from two sessions; sign out from one; assert the other's next request
is `401`, and that it lands on the sign-in page rather than on a broken page (Edge Cases: "A
person signs out in one place while another place has a list open").

---

## `requestPasswordReset` — `POST /api/v1/auth/password-reset`

**Public.** Asks for a recovery message for one address.

```go
type PasswordResetRequest struct {
    Email string `json:"email"`
}
```

| Case | Status | Body |
|---|---|---|
| the address has an account | `202` | `{ "status": "sent_if_registered" }` |
| the address has **no** account | `202` | **byte-identical to the line above** |
| the address is not a valid address at all | `202` | **byte-identical again** |
| outgoing mail is not configured | `503` | `mail_unconfigured` |
| repeated requests from one source or for one address | `429` | `rate_limited` |

**FR-073 is an enumeration rule, not a convenience.** The three `202` bodies are identical apart
from `request_id`, and a test asserts it directly. The handler also **performs the same work
whether or not the record is found**, for the same reason `login` does (research D-17) — that is
the design, and it is what a test may assert, structurally. **What no test asserts is a wall
clock**: a latency comparison has no threshold anybody can defend, and Constitution VIII forbids a
flaky gate assertion outright, so the measurement lives in the non-gating benchmark T202a and the
gate asserts the fixed work instead (amendment 2026-08-27, ANALYSIS N13). A recovery form that
answers "no such account" is an account-existence oracle, and on a self-hosted medical instance the
set of people with accounts is itself sensitive.

**`503` is deliberately not folded into the `202`.** FR-076 forbids accepting the request as
though it had succeeded, and an instance with no mail configured cannot succeed. The message names
the condition and tells the person to contact whoever runs the instance; it names no address and
does not say whether the account exists.

**Audit.** No new action value: a *request* writes nothing (there is nothing yet to record about a
record that may not exist, and writing the typed address would put a stranger's address into a
two-year medical trail — the same rule as `login_failed`). The *completion* writes
`password_change`. Rate-limited attempts write `login_failed` / `user` with an empty `target_id`.

**Mandatory tests.** The three `202` bodies byte-identical and produced by the same fixed-work path
(structurally asserted — no clock, T223i); `503` with SMTP disabled and **no** message queued
anywhere; the audit trail after a burst contains no address. Latency is watched by T202a, which
does not block merge.

---

## `confirmPasswordReset` — `POST /api/v1/auth/password-reset/confirm`

**Public.** Sets a new password using the link's token.

```go
type PasswordResetConfirm struct {
    Token           string `json:"token"`
    Password        string `json:"password"`
    PasswordConfirm string `json:"password_confirm"`
}
```

| Case | Status | Body |
|---|---|---|
| success | `204` | — |
| token expired, already used, or tampered with | `400` | `invalid_token`, **one message for all three** |
| password fails the published rules | `422` | `validation_failed`, every offending field at once |
| repeated attempts | `429` | `rate_limited` |

**FR-074 in full.** The token is resolved with
`app.FindAuthRecordByToken(form.Token, core.TokenTypePasswordReset)`; the password is set with
`SetPassword` and saved, which rotates `tokenKey` and therefore ends **every** session issued
before the change — the same single write that makes `logout` and `changePassword` global. The
caller is **not** signed in by this operation: they are returned to `/login`, which is one more
place the new password is proven to work.

**One message for expired, used and forged.** Distinguishing them tells an attacker which tokens
once existed. A single `invalid_token` with the offer to request another is both safer and, for
the person who let a link go stale, exactly as useful.

**PocketBase marks the record verified here when the token's email claim still matches the
account's address** (`apis/record_auth_password_reset_confirm.go`). That is inherited behaviour and
it is correct — the person demonstrably received mail at that address — so it is documented rather
than suppressed, and it writes the same `update` / `user` audit row as a confirmation would.

**Writes a `password_change` / `user` audit row.**

**Mandatory tests.** A token used twice: second use `400`; a token from a different account: `400`;
an expired token: `400`, with all three responses identical apart from `request_id`; after
success, a session opened before the reset is `401` on its next request.

---

## `requestEmailVerification` — `POST /api/v1/auth/verify-email`

**Requires a session.** Sends the confirmation message again to the signed-in account's own
address. It takes **no body**: the address is read from the authenticated record, never from the
request, because accepting one would let a signed-in caller aim MediKube's mailer at a stranger.

| Case | Status |
|---|---|
| success | `202` `{ "status": "sent" }` |
| the address is already confirmed | `202` — same body; there is nothing to disclose and nothing to fail |
| no session | `401 unauthenticated` |
| outgoing mail is not configured | `503 mail_unconfigured` |
| repeated requests | `429 rate_limited` |

---

## `confirmEmailVerification` — `POST /api/v1/auth/verify-email/confirm`

**Public**, because the person following the link may not be signed in on that device.

```go
type EmailVerificationConfirm struct {
    Token string `json:"token"`
}
```

| Case | Status | Body |
|---|---|---|
| success | `204` | — |
| token expired, already used, or tampered with | `400` | `invalid_token` — one message for all three |
| repeated attempts | `429` | `rate_limited` |

Resolved with `app.FindAuthRecordByToken(form.Token, core.TokenTypeVerification)`. Writes an
`update` / `user` audit row (FR-036); `GET /api/v1/me` reports the confirmed state, which is what
the settings page renders.

**Mandatory tests.** A second use of one token is `400` and identical to an expired one; the
confirmed state appears on `GET /api/v1/me` immediately afterwards; an unauthenticated caller can
complete the flow end to end without ever holding a session.
