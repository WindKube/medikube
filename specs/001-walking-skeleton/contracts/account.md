# Contract: `/api/v1/me`

Four operations. Requirements covered: FR-009 … FR-014, FR-032, FR-036, FR-045, FR-047, and
FR-075's "show the account holder whether their address is confirmed".

## Shared DTOs

```go
// internal/web/api

type Me struct {
    ID          string `json:"id"`
    Email       string `json:"email"`
    EmailConfirmed bool `json:"email_confirmed"`     // read-only; FR-075. Set only by
                                                     // confirmEmailVerification (auth.md)
    Name        string `json:"name"`
    Role        string `json:"role"`                 // read-only; "user" | "admin"
    UnitSystem  string `json:"unit_system"`
    Locale      string `json:"locale"`
    DateFormat  string `json:"date_format"`
    Theme       string `json:"theme"`
    CreatedAt   string `json:"created_at"`           // RFC3339 UTC
    Counts      MeCounts `json:"counts"`
}

// Counts is what the danger zone reads so the deletion confirmation can state what will be
// destroyed, rather than asking the person to take it on trust (FR-013).
type MeCounts struct {
    Medications int `json:"medications"`
}

type MePatch struct {                                // no `email`, no `role`, no `disabled_at`
    Name       *string `json:"name,omitempty"`
    UnitSystem *string `json:"unit_system,omitempty"`
    Locale     *string `json:"locale,omitempty"`
    DateFormat *string `json:"date_format,omitempty"`
    Theme      *string `json:"theme,omitempty"`
}

type ChangePasswordRequest struct {
    CurrentPassword string `json:"current_password"`
    NewPassword     string `json:"new_password"`
}

type DeleteAccountRequest struct {
    Password     string `json:"password"`
    Confirmation string `json:"confirmation"`        // must be exactly "DELETE MY ACCOUNT"
}
```

**`role` and `disabled_at` are absent from `MePatch`, and `Role` is read-only on `Me`.** FR-012 is
enforced by shape. A `PATCH` carrying either is `422 validation_failed` with field code
`unknown_field`, and the test asserts both the `422` **and** that the stored record is unchanged.

**`email` is absent from `MePatch` too.** FR-011 enumerates what a person may change about
themselves — display name, units, language, date presentation, appearance — and the sign-in address
is not among them. Changing it is a distinct two-step flow of its own (PocketBase's
`request-email-change` / `confirm-email-change`, which remain reachable as documented externals and
which nothing in this phase requires), and the audit vocabulary already declares the
`email_change` action for the phase that claims it. **This is not the confirmation flow**: FR-075's
confirmation proves the person controls the address already on the account, ships in this phase, and
surfaces as `Me.EmailConfirmed` plus `requestEmailVerification`. The settings page therefore shows
the address, shows whether it is confirmed with the action to send the message again, and says the
address itself cannot be changed in this version.

---

## `getMe` — `GET /api/v1/me`

**200** — the `Me` DTO above. `401` when anonymous.

`Counts.Medications` is one indexed `COUNT(*)` on `(owner)`. It exists for FR-013's confirmation
and for the dashboard's overview; it is not paginated and not filtered.

`Cache-Control: private, no-store` and **no `ETag`** — the response contains the person's own
profile and there is no concurrency question to answer.

---

## `updateMe` — `PATCH /api/v1/me`

| Case | Status | Body |
|---|---|---|
| success | `200` | the updated `Me` |
| any invalid value | `422` | `validation_failed`, **every** offending field at once |
| `role`, `disabled_at`, `email` or `verified` present | `422` | `validation_failed`, `unknown_field` |
| anonymous | `401` | `unauthenticated` |

**FR-011**: display name and the four preferences — measurement units, language, date presentation
and appearance. **FR-045**: `theme` is `system` | `light` | `dark`, is stored on the account so it
follows the person to another device, and is applied by the server as a class on `<html>` at first
paint rather than by a script after the fact (research D-36).

**FR-047**: the response is also the feedback. The settings page patches its own region with the
result and announces it through the shell's `role="status"` live region, so a change is never
silently applied or silently lost.

Writes an `update` / `user` audit row — with no field values, no before, no after.

**Mandatory tests.** Four simultaneous invalid values return four `fields[]` entries; a `theme`
outside the vocabulary is `422 invalid_value`; a body with `role: "admin"` is `422` and the stored
role is still `user`; signing in on a second session sees the new theme.

---

## `changePassword` — `PUT /api/v1/me/password`

`PUT` rather than `PATCH`: this replaces one resource wholly and idempotently.

| Case | Status | Body |
|---|---|---|
| success | `204` + `Set-Cookie: medigo_session` (a fresh token) | — |
| `current_password` absent or wrong | `422` | `validation_failed`, field `current_password`, code `incorrect` |
| new password violates a published rule | `422` | `validation_failed`, field `new_password` |
| anonymous | `401` | `unauthenticated` |

**FR-009**: the change is possible **only** by supplying the current password, and is refused when
it is absent or wrong. The refusal is `422` on the field rather than `401`, because the caller is
authenticated — the *password* is what failed, not the session.

**FR-010, and the ordering that is easy to get backwards**: `SetPassword` rotates the record's
`tokenKey`, which invalidates **every** outstanding token for the account. So the sequence, inside
one transaction, is: validate the current password → set the new one → save → mint a fresh token
from the **saved** record → set the cookie. The person who made the change stays signed in where
they made it; every other session stops working (research D-16).

Writes a `password_change` / `user` audit row. **The row records that the password changed and
nothing about the password.**

**Mandatory test.** Sign in from two sessions; change the password in the first; assert the first
still works with its new cookie, the second is `401`, the old password no longer signs in, and the
new one does.

---

## `deleteMe` — `DELETE /api/v1/me`

The one irreversible operation in this phase.

| Case | Status | Body |
|---|---|---|
| success | `204` + cookie cleared | — |
| `password` wrong or absent | `422` | `validation_failed`, field `password`, code `incorrect` |
| `confirmation` not exactly `DELETE MY ACCOUNT` | `422` | `validation_failed`, field `confirmation`, code `mismatch` |
| anonymous | `401` | `unauthenticated` |

**FR-013**: re-entry of the password **and** an explicitly typed confirmation phrase. The
interface states plainly, before either is offered, that the action cannot be undone and names how
many medications will be destroyed — read from `Me.Counts` (FR-028's confirmation standard,
applied to the account).

**FR-014 and SC-012**: the account and **every** medication recorded under it are permanently
removed. This is PocketBase's behaviour, not MediGo's: `medications.owner` is
`CascadeDelete: true`, so `deleteRefRecords` destroys them in the same transaction. Because it is
behaviour MediGo *depends on* rather than behaviour MediGo *wrote*, it is asserted directly:

```sql
SELECT COUNT(*) FROM medications WHERE owner = '<deleted id>';   -- must be 0
```

The audit row is written **before** the delete, and `audit_events.actor` is `CascadeDelete: false`,
so the `account_delete` row survives with its actor unset and `actor_kind = user` (research D-22).
A test asserts the row still exists after the account is gone — otherwise account deletion would
delete the only evidence that it happened.

**FR-032 / FR-033**: there is no `DELETE /api/v1/users/{id}`. A person can delete their own
account and nobody else's, because the only account this operation can reach is the authenticated
one. There is no id to guess.

**The Edge Case this operation owns**: "An account is deleted while one of its live views is open
elsewhere." The open view's SSE stream re-authorises on every event, so it stops receiving
patches immediately; the staleness detector (research D-37) reveals the banner within ten seconds;
and the view's next action lands on the sign-in page rather than on a broken page.

**Mandatory tests.** The wrong password does not delete; the wrong confirmation phrase does not
delete; a successful deletion leaves zero medications and an intact `account_delete` audit row;
the credentials no longer sign in; and a second account's data is untouched, asserted by count.
