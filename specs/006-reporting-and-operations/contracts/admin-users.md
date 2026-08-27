# Contract: account administration (operations 82–83)

Conventions, status codes, the actor matrix and the audit rules are in [README.md](./README.md).
Both operations require `role = admin`; anything else is `404` **and a recorded attempt** (FR-076).

**An administrator sees counts about an account and nothing about what its people's records contain**
(FR-089, FR-088). The DTO below has no field capable of carrying a diagnosis, a value or a file name,
and the service has no port that could fetch one ([D-48](../research.md#d-48)).

## DTO

```go
type AdminAccount struct {
    ID                 string  `json:"id"`
    Email              string  `json:"email"`                  // the sign-in identity (FR-089)
    Name               string  `json:"name"`                   // the display name
    Role               string  `json:"role"`                   // "user" | "admin"
    Disabled           bool    `json:"disabled"`
    DisabledAt         *string `json:"disabled_at"`
    MustChangePassword bool    `json:"must_change_password"`
    CreatedAt          string  `json:"created_at"`
    LastSignInAt       *string `json:"last_sign_in_at"`        // derived from the trail (D-18)
    PatientCount       int     `json:"patient_count"`          // how many people it owns — and nothing else
}
```

`LastSignInAt` is `MAX(occurred_at) WHERE actor = :user AND action = 'login'`. **No
`users.last_login_at` column is added** ([D-18](../research.md#d-18)): a second source of truth would
disagree with the trail, and a restore would roll one back but not the other.

---

## 82. `GET /api/v1/admin/users` — the accounts on this instance

`operationId: listAdminUsers`

**Query**: `?q=` (substring of email or display name), `?role=`, `?disabled=true|false`,
`?limit=`, `?cursor=`, `?count=true`, `?sort=` ∈ `{-created_at, created_at, email, -last_sign_in_at}`
(default `-created_at`).

**Response** `200` — `Page[AdminAccount]`.

Paging is the shared keyset cursor, so accounts being created underneath a pager are neither repeated
nor skipped (FR-122).

**Authorization**: `role = admin`, else `404` + `access_denied`.

**Audit**: none on success; one `access_denied` on refusal.

**Scale**: 500 accounts, first page within **2 s**.

---

## 83. `PATCH /api/v1/admin/users/{id}` — change what an account may do

`operationId: updateAdminUser`

**Request** — any subset, at least one field:

```json
{ "role": "admin", "disabled": true, "must_change_password": true }
```

Nothing else is accepted. **There is no password field, no email field and no name field**: an
administrator changes what an account *may do*, never what it *is*.

**Response** `200` — `AdminAccount`.

**Effects**, all inside one `app.RunInTransaction` ([D-19](../research.md#d-19)):

| Field | Effect | Requirement |
|---|---|---|
| `disabled: true` | `disabled_at = now`, **then `record.RefreshTokenKey()`** — rotating the per-record token key invalidates every outstanding token, so open sessions end **immediately** rather than at expiry | FR-090, SC-013 (within 5 s) |
| `disabled: false` | `disabled_at = ''`; the account signs in normally again | FR-092 |
| `role` | promote or demote | FR-094 |
| `must_change_password: true` | the flag plus `RefreshTokenKey()`; at the next sign-in the account can reach the password change **and nothing else** — every other `/api/v1` route is `403 password_change_required` and every page redirects to the forced-change form. Setting a new password clears the flag in the same transaction | FR-093, [D-20](../research.md#d-20) |

**Three refusals, enforced in `internal/domain/adminuser` as pure functions** so they hold no matter
which caller reaches them — a handler, `medigo seed`, a future CLI subcommand or a test fixture:

| Code | When | Requirement |
|---|---|---|
| `409 self_demotion` | the actor is changing **their own** `role` away from `admin`, or disabling themselves; the message explains why | FR-095, US5 AS-12 |
| `409 last_administrator` | the target is the **last enabled account with `role = admin`** and the change would demote or disable it — refused for **anybody**, including a second administrator; the message names the reason | FR-096, US5 AS-13 |
| `404 not_found` | the account does not exist | — |
| `422 validation_failed` | an unknown `role`, or an empty patch | — |

**A tier change takes effect on the target's next action, not on their next sign-in.** An
administrator whose own tier is changed by a second administrator while they are looking at an
operator page has their **next** action authorized against the tier they now hold, because nothing
caches a role (edge case, phase 005's no-grant-cache rule extended to the tier).

**Sign-in behaviour of a disabled account** ([D-49](../research.md#d-49), FR-091):

| Attempt | Answer |
|---|---|
| **wrong** credentials, account disabled | `401 invalid_credentials` — **byte-identical** to a wrong password on an enabled account and to an address with no account, so disabling cannot be detected from outside |
| **correct** credentials, account disabled | `403 account_disabled`, plain message: *"This account has been disabled. Contact the person who runs this instance."* — which discloses nothing to somebody who does not already hold the password |

Both write `login_failed` with a bounded `reason` (`bad_credentials` / `disabled`) and **never the
address**.

**`role` and `disabled_at` are not settable through registration or any self-service action** — the
registration and `PATCH /api/v1/me` DTOs have no field for either, which is the control rather than a
check (FR-097). This operation is the only way either changes.

**Audit**: one `admin_user_update` entry per call, **marked administrative**, naming who acted, which
account was affected and what changed — as field names and enum values only. **No password, no
address, no record content** (FR-098, US5 AS-14).
