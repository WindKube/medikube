# Contract: shares (operations 58–62)

Conventions, status codes, the single `403` and the six-actor matrix are in
[README.md](./README.md) and are not repeated per operation.

## DTOs

```go
// api.ShareSummary — every list row, both directions.
type ShareSummary struct {
    ID           string       `json:"id"`
    ResourceKind string       `json:"resource_kind"`          // "patient" | "family_member"
    Resource     ResourceRef  `json:"resource"`               // id + display label, see below
    Level        string       `json:"level"`                  // "view" | "edit"
    Direction    string       `json:"direction"`              // "granted" | "received"
    Counterparty AccountRef   `json:"counterparty"`           // display name only (FR-062)
    Note         string       `json:"note,omitempty"`
    Active       bool         `json:"active"`                 // computed, never stored
    GrantedAt    string       `json:"granted_at"`             // RFC3339 UTC
    ExpiresAt    *string      `json:"expires_at"`             // null = open-ended
    EndedAt      *string      `json:"ended_at"`               // null while active
    EndedBy      *string      `json:"ended_by"`               // "owner" | "grantee" | "lapse"
    FromInvite   bool         `json:"from_invitation"`        // FR-035
}

// api.ResourceRef — what the caller is allowed to know the grant is over.
//   direction="granted": the owner may see their own patient/relative, so Label is its name.
//   direction="received": the grantee already holds the grant, so Label is its name.
// There is no context in which a ResourceRef is rendered to somebody without access to it.
type ResourceRef struct {
    ID    string `json:"id"`
    Kind  string `json:"kind"`   // "patient" | "family_member"
    Label string `json:"label"`  // PHI. Never present in an invitation DTO — see invitations.md
}

type AccountRef struct {
    ID          string `json:"id"`
    DisplayName string `json:"display_name"`      // FR-062: nothing else about the other account
    Email       string `json:"email,omitempty"`   // ONLY on direction="granted"; the address the
                                                  // owner themselves typed (FR-062)
}
```

---

## 58. `POST /api/v1/shares` — offer access

**Creates an invitation, never a grant** (FR-013). The path is the shared design contract's; the
reasoning and the alternative considered are in [D-12](../research.md#d-12).

**Request**

```json
{ "resource_kind": "patient",
  "resource_ids": ["rec1234567890abc"],
  "recipient_email": "kwame@example.org",
  "level": "view",
  "note": "So you can see Dad's chart when you drive him.",
  "expires_at": "2027-01-31T00:00:00Z" }
```

| Field | Rules |
|---|---|
| `resource_kind` | required, `patient` \| `family_member` |
| `resource_ids` | required, 1..50, unique, all of `resource_kind`, **all owned by the caller** (`PermOwn`) |
| `recipient_email` | required, valid address, **not** the caller's (case-insensitive) |
| `level` | required, `view` \| `edit`; `edit` refused when `resource_kind = family_member` |
| `note` | optional, ≤500 |
| `expires_at` | optional; the **invitation's** lapse date. Default now+168h, min now+1h, max now+1y |

**Response** `201` + `Location: /api/v1/invitations/{id}`, body `api.InvitationSummary`
(see [invitations.md](./invitations.md)) with `delivery` telling the sender what happened.

**Authorization**: `PermOwn` on **every** id in `resource_ids`, re-checked at accept time
([D-10](../research.md#d-10)).

**Errors**

| Code | When | Requirement |
|---|---|---|
| `404 not_found` | any id is not the caller's, or does not exist — indistinguishable | FR-016 |
| `422 self_share` | `recipient_email` is the caller's, ignoring case | FR-021 |
| `409 already_shared` | an **active** grant of that thing to that address already exists; the message directs the caller to change it instead | FR-019 |
| `409 invitation_outstanding` | a **pending, unlapsed** invitation for that thing to that address already exists; the message says it can be cancelled | FR-020 |
| `422 family_history_view_only` | `edit` on `family_member` | FR-007 |
| `422 too_many_resources` | more than the configured maximum | [D-14](../research.md#d-14) |
| `422 validation_failed` | `expires_at` not in the future, or outside [1h, 1y] | FR-017, FR-008 |
| `422 email_not_configured` | SMTP disabled **and** the address has no account | FR-022, [D-06](../research.md#d-06) |

**Enumeration safety (FR-018, SC-011)**: with SMTP configured, the response for an address with an
account and one without is **identical** — same status, same body shape, same headers, and the
invitation row is created either way with `recipient` empty. The only case in which the two differ
is the SMTP-disabled state, which is the documented, warned-about narrowing in
[D-06](../research.md#d-06). Two `ApiScenario` cases assert both halves.

**Audit**: `invite_send`, target `invitation`, opaque id, **no address, no note, no resource ids**.

**Side effects**: one email to `recipient_email` when SMTP is enabled ([D-03](../research.md#d-03));
one hub notice to the recipient's user topic when the address belongs to an account (FR-064).

---

## 59. `GET /api/v1/shares` — who has access, and to what

**Query**: `?direction=granted|received` (**required**, no default — the two lists answer different
questions and a default would silently pick one), `&resource_kind=patient|family_member`,
`&patient={id}`, `&active=true|false`, `&counterparty={userId}`, `&limit=`, `&cursor=`, `&count=true`,
`&sort=` ∈ `{-granted_at, granted_at, -expires_at, level}`.

**Response** `200` — `Page[ShareSummary]`.

- `direction=granted` returns grants the caller **gave**, over things they own (FR-035).
- `direction=received` returns grants the caller **holds** (FR-036).
- `active` is computed per row from `revoked_at = '' AND (expires_at = '' OR expires_at > now)`;
  omitting the parameter returns both, which is how a lapsed or revoked grant is "shown as lapsed to
  both sides" (FR-029, US2 scenario 5).
- Empty result is `200` with `items: []` — the page renders `@EmptyState`, never a blank screen
  (FR-040).

**Authorization**: the caller sees only rows where they are the `grantor` (granted) or the `grantee`
(received). There is no route by which one account reads another's sharing list.

**Errors**: `400 bad_request` for a missing or unknown `direction`; `401` unauthenticated.

**Scale**: SC-014 — 200 granted rows for one owner, 50 received rows for one grantee, first page in
under 2 s, and a paging walk with concurrent grant/revoke repeats or skips 0 rows.

---

## 60. `PATCH /api/v1/shares/{id}` — change a grant

**Request** (both fields optional, at least one required):

```json
{ "level": "edit", "expires_at": "2027-06-30T00:00:00Z" }
```

`expires_at: null` explicitly clears the end date (open-ended). Absent means unchanged — plain
pointers, `**T` for the nullable case, per the shared design's `Patch` convention.

**Response** `200` — `ShareSummary`.

**Authorization**: `PermOwn` on the underlying resource, i.e. the caller must be the `grantor`
**and** still own the thing. FR-037's "effective on the grantee's next action" is free: nothing
caches a grant ([Principle I decision in plan.md](../plan.md)).

**Errors**

| Code | When | Requirement |
|---|---|---|
| `404 not_found` | not the caller's grant, revoked, or gone | FR-038 |
| `422 family_history_view_only` | raising a family-history grant to `edit` | FR-007 |
| `422 validation_failed` | `expires_at` not strictly in the future | FR-008, edge "an end date in the past" |

**Audit**: `share_update`, target `share`. The old and new levels are **not** in the row (FR-071);
the level is derivable from the row's own history of events, and the trail is not a diff log.

**Notice**: hub event to the grantee — "your access changed" and the new level, no patient name
(FR-064, FR-065).

---

## 61. `DELETE /api/v1/shares/{id}` — the owner ends it

**Response** `204`.

**Authorization**: caller is the `grantor` and owns the resource (`PermOwn`).

**Effect**: `revoked_at = now`, `revoked_by = grantor`. **Nothing else changes** — every record,
correction, document and relative the former grantee created or edited stays exactly as it is
(FR-047, US2 scenario 6). Idempotent: revoking an already-revoked grant is `204`, not `409`.

**Errors**: `404` for anything not the caller's.

**Audit**: `share_revoke`.

**Real-time**: publishes `Event{Type: AccessChanged, UserID: grantee, PatientID: …}` to the hub, so
the grantee's open record stream re-authorises, fails, patches "your access to this has ended" and
closes within 5 s (FR-045, SC-005) — see [streams-notifications.md](./streams-notifications.md).

**Downstream, on the grantee's very next request** (FR-042, FR-044, FR-046, SC-004): every route is
`404`, with no sign-out and no cron; their `users.active_patient` resolves to null and a page
request lands on `/patients`; an in-flight edit save is refused as though the record did not exist
(US2 scenario 7).

---

## 62. `DELETE /api/v1/shares/{id}/mine` — the grantee walks away

**Response** `204`.

**Authorization**: caller is the `grantee` on an **active** grant. A grantor calling this is `404`;
so is a stranger.

**Effect**: `revoked_at = now`, `revoked_by = grantee`. The owner's sharing screen shows
`ended_by: "grantee"` — "they left", not "I revoked" (FR-039, US2 scenario 4).

**Audit**: `share_leave` — a distinct action precisely so the trail distinguishes the two
([D-18](../research.md#d-18)).

**Why this is a separate operation and not a flag on 61**: different actor, different authorization
rule, different audit action, and different meaning to the owner. It is the one place this phase
declines to collapse two operations into one.
