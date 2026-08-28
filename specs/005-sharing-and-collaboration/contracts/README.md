# Contracts: Sharing and Collaboration (phase 005)

These are the contracts the contract tests are written against. Anything not stated here is
inherited from the shared design contract §2.1 and from phase 001's `internal/web` layer.

## Files

| File | Covers |
|---|---|
| [shares.md](./shares.md) | ops 58–62 — create (an invitation), list, change, revoke, leave |
| [invitations.md](./invitations.md) | ops 63–66 — list, public token preview, respond, cancel/withdraw |
| [streams-notifications.md](./streams-notifications.md) | op 67 — the SSE notice stream, and the revocation cut-off on the record stream |
| [widened-authorization.md](./widened-authorization.md) | what changes on **every** endpoint phases 001–004 already shipped |
| [pages.md](./pages.md) | the 3 new pages, the new shell region, and the widened pages |

**Phase 005 adds 10 `/api/v1` operations and 0 record routes.**

## Conventions that apply to every operation below

1. Base path `/api/v1`. **No trailing slashes** — PocketBase has done no trailing-slash
   normalisation since v0.23.
2. `Content-Type: application/json` in and out. **Unknown request fields are rejected** with `422`;
   duplicate JSON keys are rejected (Go 1.27 `encoding/json/v2`).
3. Slices always marshal as `[]`, never `null`.
4. Dates named `*_on` are `"YYYY-MM-DD"`. Instants named `*_at` are RFC3339 **UTC**.
5. Pagination is cursor-based: `{"items":[…],"next_cursor":"…"|null,"total":12}`, `total` only when
   `?count=true`. `?limit=` default 25, max 100. Cursors are opaque, HMAC-signed and encode
   `(sort keys, last values, last id)`, never an offset — which is what makes FR-075's "neither
   repeated nor skipped while grants are being created and ended" true.
6. Sorting is `?sort=` with a comma list, `-` prefix for descending, from a per-resource allowlist.
7. Filtering is explicit named parameters only. **PocketBase's filter DSL never reaches the wire.**
8. `PATCH`/`DELETE` on a **clinical record** require `If-Match`. Shares and invitations are **not**
   clinical records and do not: their mutations are state transitions guarded by the state machine
   and by a single-writer transaction, which is a stronger guarantee than an ETag
   ([D-11](../research.md#d-11)).
9. **A resource the caller may not see returns `404`, never `403`** — for anything patient-scoped,
   existence is itself PHI.
10. Error envelope on every non-2xx, without exception:

```json
{ "error": {
    "code": "forbidden_view_only",
    "message": "human-readable, PHI-free",
    "request_id": "3f2b…",
    "fields": [ { "field": "expires_at", "code": "not_future", "message": "…" } ] } }
```

`message` for a `500` is always the literal `"internal error"`.

## The one `403` in this phase, and why it is the only one

Rule 9 has exactly one exception, stated here so no handler has to decide:

> **`403 forbidden_view_only`** is returned when, and only when, an actor holding an **active
> `view` grant** on the resource attempts a write on that resource.
>
> **`403 forbidden_owner_only`** is returned when, and only when, an actor holding an active grant
> at **either** level attempts an act FR-005 or FR-006 reserves to the owner — deleting the patient,
> changing identity fields, altering anybody's access, or sharing onward.

The caller demonstrably already knows the resource exists — they are looking at it — and FR-058 and
SC-006 require the refusal to name their level so they can act on it. The response is producible
only from a code path that has already resolved a `Grant`, which makes the boundary
machine-checkable, and the same holds for `forbidden_owner_only`. Every other refusal in this phase
is `404 not_found`, byte-identical to a
non-existent id: stranger · revoked · lapsed · disabled · family-history grantee reaching for the
patient · grantee reaching the owner's directory · grantee reaching another of the owner's patients.

## Status codes used in this phase

| Code | Envelope `code` | When |
|---|---|---|
| 200 | — | read, list, change |
| 201 | — | `POST /shares` — with `Location: /api/v1/invitations/{id}` |
| 204 | — | revoke, leave, cancel, withdraw |
| 400 | `bad_request` | a bad cursor, an unknown filter value, a bad `sort` key |
| 401 | `unauthenticated` | no session (FR-063) |
| 403 | `forbidden_view_only` | the exception above — a `view` grantee writing to a resource they can see |
| 403 | `forbidden_owner_only` | a grantee at **either** level attempting an owner-reserved act on a resource they can see: deleting the patient, changing identity fields, changing anybody's access, sharing onward (FR-005, FR-006) |
| 404 | `not_found` | absent **or** not reachable by this actor — indistinguishable |
| 409 | `conflict` | a state-machine violation; the body names the current state and `responded_at` (FR-032) |
| 409 | `already_shared` | FR-019 — an active grant of this thing to this address already exists |
| 409 | `invitation_outstanding` | FR-020 — a pending invitation for this thing to this address already exists |
| 410 | `resources_unavailable` | FR-028 — acceptance failed as a whole because a covered thing is gone. **Names nothing** |
| 422 | `validation_failed` | domain validation, unknown field, duplicate key — with `fields[]` |
| 422 | `self_share` | FR-021 / FR-011 — inviting your own address |
| 422 | `family_history_view_only` | FR-007 |
| 422 | `email_not_configured` | [D-06](../research.md#d-06) — SMTP off **and** the address has no account |
| 422 | `too_many_resources` | more than `MEDIKUBE_SHARING_MAX_RESOURCES_PER_INVITATION` |
| 429 | `rate_limited` | the public token preview, and the send endpoint |
| 500 | `internal_error` | message always `"internal error"` |

## The authorization rule, restated for this phase

Every operation resolves an `access.Actor` from the PocketBase auth record, then calls exactly one
of `Authorizer.Patient(ctx, actor, patientID, need)` or
`Authorizer.Record(ctx, actor, kind, recordID, need)`.

Resolution order, **after this phase**:

```
superuser                                       → allow, and audit as admin_session
actor owns the patient / the relative's patient → PermOwn
an ACTIVE grant exists for (actor, resource)    → the grant's level
otherwise                                       → ErrNotFound
```

where *active* is `revoked_at = '' AND (expires_at = '' OR expires_at > now)` and the actor is not
disabled. If a grant exists but does not carry the needed permission, the result is `ErrForbidden`
**with the grant attached**, which is what produces the one `403`.

**The active patient is never consulted for authorization.** Permission is a property of the route,
never a client-supplied parameter — upstream's `required_permission` query parameter does not exist
in MediKube.

## The six-actor matrix

Every operation in this phase, **and every patient-touching operation shipped in phases 001–004**,
carries these tests (FR-077, SC-018). `internal/service/access/coverage_test.go` fails the build
when a route in the registry lacks them.

| Actor | Read | Write |
|---|---|---|
| owner | 2xx | 2xx |
| grantee at `edit` | 2xx, identical to the owner's response | 2xx |
| grantee at `view` | 2xx, identical to the owner's response | **403 `forbidden_view_only`** |
| grantee, revoked | 404 | 404 |
| grantee, lapsed | 404 | 404 |
| stranger | 404, byte-identical to a non-existent id | 404 |
| unauthenticated | 401, disclosing nothing | 401 |

## Audit expectations, stated once

Every operation below names the audit action it must produce. Two rules cover all of them:

- Rows for `shares` and `invitations` are written from **post-commit** `OnRecordAfter*Success`
  hooks, so a rolled-back transaction never produces one. Refusals (`access_denied`) and sensitive
  reads (`read_sensitive`) are explicit `audit.Record` calls from the service.
- **No content, ever**: no address, no note, no display name, no token, no patient name.
  A `share_expire` row is written by the tidy pass rather than at the instant of lapse
  ([D-19](../research.md#d-19)) — the entry is guaranteed and deterministic, not instantaneous, and
  SC-015 is to be read that way.
