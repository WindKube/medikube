# Contract: invitations (operations 63–66)

Conventions and status codes are in [README.md](./README.md). The state machine is
[data-model.md §3](../data-model.md#3-the-invitation-state-machine).

## DTOs

```go
// api.InvitationSummary — the sender's and the recipient's list rows, and the 201 body of op 58.
type InvitationSummary struct {
    ID            string  `json:"id"`
    Kind          string  `json:"kind"`             // "patient_share" | "family_history_share"
    Level         string  `json:"level"`            // "view" | "edit"
    Status        string  `json:"status"`           // pending|accepted|rejected|cancelled|revoked|expired
    Lapsed        bool    `json:"lapsed"`           // status=="pending" && expires_at < now (FR-029)
    ResourceCount int     `json:"resource_count"`   // a COUNT, never the ids, never the names
    Note          string  `json:"note,omitempty"`
    ExpiresAt     string  `json:"expires_at"`
    CreatedAt     string  `json:"created_at"`
    RespondedAt   *string `json:"responded_at"`
    ResponseNote  string  `json:"response_note,omitempty"`
    Delivery      string  `json:"delivery"`         // "email" | "in_app_only"  (FR-022)

    // Exactly one of the two is present, decided by the caller's role in the invitation:
    Sender    *AccountRef `json:"sender,omitempty"`     // on direction=received: display name only
    Recipient *InviteeRef `json:"recipient,omitempty"`  // on direction=sent
}

type InviteeRef struct {
    Email       string `json:"email"`                   // the address the SENDER typed (FR-062)
    DisplayName string `json:"display_name,omitempty"`  // only after acceptance resolved an account
}

// api.InvitationPreview — the PUBLIC, pre-acceptance view. FR-023 / SC-010 are enforced by the
// SHAPE of this struct: there is no field capable of carrying a patient name, a relative's name,
// a diagnosis, a medication or any other clinical value.
type InvitationPreview struct {
    SenderName    string `json:"sender_name"`
    Kind          string `json:"kind"`
    Level         string `json:"level"`
    ResourceCount int    `json:"resource_count"`
    Note          string `json:"note,omitempty"`     // the sender's own words, verbatim
    ExpiresAt     string `json:"expires_at"`
    RecipientHint string `json:"recipient_hint"`     // "k****@e******.org" — enough to tell the
                                                     // reader which of their addresses to use
}
```

`InvitationPreview` is the FR-023 control. A reviewer checks one struct, not a redaction pass.

---

## 63. `GET /api/v1/invitations` — my invitations, both ways

**Query**: `?direction=received|sent` (**required**),
`&status=pending|accepted|rejected|cancelled|revoked|expired`, `&kind=`, `&limit=`, `&cursor=`,
`&count=true`, `&sort=` ∈ `{-created_at, created_at, expires_at}`.

**Response** `200` — `Page[InvitationSummary]`.

- `direction=received` matches on `LOWER(recipient_email) = LOWER(actor.email)` — **the address, not
  the account**, which is why an invitation sent before the recipient registered is waiting for them
  the moment they sign in (FR-014, US5 scenario 2).
- A `pending` row whose `expires_at` has passed comes back with `status: "pending", lapsed: true`
  and is excluded from the "still to act on" default view (FR-029, FR-033). The tidy pass renames it
  later; the read path never waits for that.
- Empty is `200` with `items: []` and an `@EmptyState` on the page (FR-040).

**Authorization**: the caller sees only invitations they sent or that are addressed to their
address. Nothing else, by any filter.

**Audit**: none. Listing is not an accountable act.

---

## 64. `GET /api/v1/invitations/token/{token}` — the public preview

**Public**: no session required. This is what `/invite/{token}` renders, and it is the only
unauthenticated endpoint in the phase.

**Response** `200` — `InvitationPreview`.

**Rules**

| Rule | Requirement |
|---|---|
| the token is looked up by `sha256` of the path segment against `invitations.token_hash` | [D-15](../research.md#d-15) |
| **`404` unless `status == pending` and `expires_at > now`** — answered, cancelled, withdrawn and lapsed invitations are indistinguishable from a token that never existed | FR-024, SC-012 |
| the response carries no patient name, no relative name and no clinical content, by DTO shape | FR-023, SC-010 |
| rate limited (a bound on token guessing, on top of 256 bits of entropy) | FR-024 |
| the token is never written to a log, a span, a metric or a Sentry event | FR-072, SC-016 |

**Errors**: `404 not_found` for an unknown, malformed, answered, cancelled or lapsed token —
one response, no distinction.

**Audit**: none. A preview is not an access to patient data, and auditing it would create a
denial-of-log vector on a public endpoint.

---

## 65. `POST /api/v1/invitations/{id}/respond` — accept or decline

**Request**

```json
{ "response": "accepted", "note": "Thanks — I'll keep an eye on it." }
```

`response` ∈ `{accepted, rejected}` (required); `note` ≤500 (optional, FR-027).

**Response** `200` — `InvitationSummary` with the new status, plus, on `accepted`, a
`grants` array of the `ShareSummary` rows created, so the UI can render the result without a second
call.

**Authorization** — three conditions, in this order, all inside the transaction:

1. the caller is signed in (`401` otherwise, FR-063);
2. `LOWER(actor.email) == LOWER(invitation.recipient_email)` — otherwise **`404`**, disclosing
   nothing about who it was for or what it covered (FR-025, US5 scenario 3, SC-009);
3. `status == pending` and `expires_at > now` — otherwise `409 conflict` naming the state and
   `responded_at`, or `410 resources_unavailable` where relevant (FR-029, FR-032).

**Accept semantics** ([D-10](../research.md#d-10), [D-11](../research.md#d-11)) — one
`app.RunInTransaction`:

1. re-read the invitation, assert `pending` and unlapsed (compare-and-set under SQLite's single
   writer — this is what makes a double accept impossible);
2. for **every** `resource_ids[i]`: assert it still exists, assert `owner == invitation.sender`
   **now** (FR-016, edge "the sender stops owning the resource"), assert no active grant already
   exists for `(resource, actor)`;
3. create every grant, copying `note` onto each ([D-16](../research.md#d-16));
4. set `status = accepted`, `responded_at = now`, `recipient = actor`, `response_note`.

Any failure in (2) aborts the whole transaction: **no partial set of grants ever exists**
(FR-028, SC-013), the invitation moves to a terminal state, and the response is
`410 resources_unavailable` with a message that names **nothing** about what was covered
(edge "an invitation covers something that is deleted before it is answered").

**Decline semantics**: `status = rejected`, `responded_at`, `response_note`. No grant is created.
The sender sees the note and the time (FR-027, US5 scenario 5).

**Errors**

| Code | When |
|---|---|
| `404 not_found` | not addressed to this account, or the id does not exist |
| `409 conflict` | already answered, cancelled, withdrawn or lapsed — body names the state and when |
| `410 resources_unavailable` | all-or-nothing acceptance failed |
| `422 validation_failed` | `response` not one of the two values, or `note` too long |

**Audit**: `invite_respond` (with `reason = accepted|rejected`), plus one `share_grant` per grant
created — all post-commit, so a rolled-back accept produces no rows.

**Notice**: hub event to the **sender** — "your invitation was accepted/declined", naming the
recipient's display name and nothing else (FR-064, FR-065).

**Race with revocation** (edge case): the accept and a concurrent
`DELETE /api/v1/shares/{id}` serialise on the single writer. Either the accept commits and the
revoke then revokes it, or the revoke commits first and the accept's step (2) finds no conflicting
grant and proceeds — in which case the owner's revoke targeted a grant that did not exist yet and
returns `404`. **There is no half-applied state, and each event produces exactly one audit row**,
which is what the edge case demands.

---

## 66. `DELETE /api/v1/invitations/{id}` — cancel, or withdraw

One operation, two transitions, chosen by the invitation's current status — upstream needed two
endpoints in two routers for this (domain-platform §5.2).

| Current status | Transition | Effect | Audit |
|---|---|---|---|
| `pending` | → `cancelled` | the link stops working **immediately**; no grant is ever created (FR-030) | `invite_cancel` |
| `accepted` | → `revoked` | **every** grant this invitation created is revoked in the same transaction, `revoked_by = grantor` (FR-031, US5 scenario 8) | `invite_withdraw` + one `share_revoke` per grant |

**Response** `204`.

**Authorization**: caller is the `sender`. Anyone else, including the recipient, is `404`.

**Errors**: `409 conflict` when the invitation is already `rejected`, `cancelled`, `revoked` or
lapsed — nothing changes and the body names the state.

**Notice**: on withdraw, a hub event to each affected grantee — "access ended" — and their open
streams cut, exactly as for op 61.

---

## Retention (FR-033)

Answered, cancelled, withdrawn and lapsed invitations are excluded from the "still to act on" views
immediately, retained for `MEDIKUBE_SHARING_INVITATION_RETENTION_DAYS` (default 90) for
accountability, and then **deleted** by the tidy pass — their audit events outlive them
([D-19](../research.md#d-19)). A share whose `invitation` relation is emptied by that deletion keeps
its own `note` (copied at accept time) and its `from_invitation: true` provenance flag.
