# Data Model: Sharing and Collaboration (phase 005)

Two new collections, one amendment, one state machine, three reversible migrations.
Consistent with [`SHARED-DESIGN.md`](../SHARED-DESIGN.md) §1.2 except where [research.md D-01](./research.md#d-01)
corrects an index that would have been inert.

**Rules inherited from the shared design contract §1.0 and not repeated per collection**: ids are
PocketBase's 15-character opaque text ids; every collection carries `id`, `created`, `updated`;
**all five API rules are `nil`** (superuser-only), asserted at boot; enum fields are
`core.SelectField{MaxSelect: 1}` **and** a Go string type with `Valid()` in `internal/domain`,
generated from one source of truth; all enum values are `snake_case`; uniqueness is a collection
index because PocketBase has no per-field `Unique`.

---

## 0. The one thing to read before anything else

**PocketBase has no NULL.** `core/field_date.go:110` and `core/field_relation.go:161` declare
optional date and single-relation columns as `TEXT DEFAULT '' NOT NULL`. Throughout this document,
"unset" means the **empty string**, and every predicate is written `= ''`. A predicate written
`IS NULL` compiles, migrates and matches nothing; its dangerous inverse
(`revoked_at IS NULL OR …`) matches everything, including revoked grants. See
[D-01](./research.md#d-01). A build gate greps for `IS NULL` in this phase's store packages.

**The active-grant predicate, stated once, and it appears in exactly one file
(`internal/store/share/query.go`):**

```sql
revoked_at = '' AND (expires_at = '' OR expires_at > {:now})
```

---

## 1. `shares` — new

The single grant table. One row is one account's live permission over one shareable thing.

| Field | Type | Req | Notes |
|---|---|---|---|
| `resource_kind` | select, MaxSelect 1 | ✓ | `patient` \| `family_member`. No third value. |
| `patient` | relation → `patients`, MaxSelect 1, **CascadeDelete** | conditional | set **iff** `resource_kind = patient`, else `''` |
| `family_member` | relation → `family_members`, MaxSelect 1, **CascadeDelete** | conditional | set **iff** `resource_kind = family_member`, else `''` |
| `grantor` | relation → `users`, MaxSelect 1, **CascadeDelete** | ✓ | must own the resource at create time and is re-checked then ([D-10](./research.md#d-10)) |
| `grantee` | relation → `users`, MaxSelect 1, **CascadeDelete** | ✓ | never equal to `grantor` (FR-011) |
| `level` | select, MaxSelect 1 | ✓ | `view` \| `edit`. `edit` is refused when `resource_kind = family_member` (FR-007) |
| `note` | text ≤500 | | **copied from the invitation at accept time**, not read through the relation ([D-16](./research.md#d-16)). PHI-adjacent free text: redacted in logs, never in the audit trail |
| `expires_at` | date | | `''` = open-ended. Must be strictly in the future when set (FR-008). **Evaluated in the read path** |
| `revoked_at` | date | | `''` = active. Replaces upstream's `is_active`; "when" comes free |
| `revoked_by` | relation → `users`, MaxSelect 1 | | `= grantor` → owner revoked; `= grantee` → grantee left; `''` with a past `expires_at` → lapsed ([D-18](./research.md#d-18)) |
| `invitation` | relation → `invitations`, MaxSelect 1 | | provenance only (FR-035). Becomes `''` when the invitation is removed at the end of its retention window |

**Indexes**

| Name | Unique | Columns | Where | Why |
|---|---|---|---|---|
| `idx_shares_active_unique` | ✓ | `resource_kind, patient, family_member, grantee` | `revoked_at = ''` | FR-010: at most one **active** grant of a thing to an account. Revoked rows accumulate freely, which is what makes "sharing again needs a fresh invitation" (US2 scenario 8) provable |
| `idx_shares_grantee_active` | | `grantee, revoked_at, resource_kind, patient` | | the hot path: `ShareReader.ActiveFor` and `ActivePatientsFor` ([D-24](./research.md#d-24)) |
| `idx_shares_grantor` | | `grantor, resource_kind, revoked_at, created` | | the owner's "granted" panel, paged |
| `idx_shares_patient` | | `patient, revoked_at` | | "how many people have access?" before a patient delete |
| `idx_shares_family_member` | | `family_member, revoked_at` | | the same, per relative |

**Invariants** (domain-enforced in `internal/domain/share.Share.Validate`, each with a named test)

| # | Rule | Requirement |
|---|---|---|
| S1 | exactly one of `patient` / `family_member` is set, matching `resource_kind` | FR-001 |
| S2 | `grantor != grantee` | FR-011 |
| S3 | `resource_kind = family_member` ⇒ `level = view` | FR-007 |
| S4 | `expires_at` is `''` or strictly greater than *now at the moment it is set* | FR-008 |
| S5 | `revoked_by != ''` ⇒ `revoked_at != ''` | consistency |
| S6 | `revoked_by ∈ {grantor, grantee, ''}` | FR-039, FR-068 |

**Derived, never stored**: `active` = the D-01 predicate. There is **no** `status` column; see the
plan's post-design re-check for why a denormalised one is a disclosure bug waiting to happen.

**Redaction** — `MarshalZerologObject` emits `id`, `resource_kind`, `level`, `grantor_id`,
`grantee_id`, `patient_id`, and nothing else. Never `note`.

---

## 2. `invitations` — new

The offer, and the only way a grant comes into being (FR-013).

| Field | Type | Req | Notes |
|---|---|---|---|
| `sender` | relation → `users`, MaxSelect 1, **CascadeDelete** | ✓ | |
| `recipient_email` | email | ✓ | **may belong to no account** (FR-014). Stored as typed; matched **case-insensitively** everywhere (FR-025, edge case "different casing") |
| `recipient` | relation → `users`, MaxSelect 1 | | `''` until accepted; resolved **at accept time**, so a later address change does not break a grant (edge case "the invited address changes") |
| `kind` | select, MaxSelect 1 | ✓ | `patient_share` \| `family_history_share` |
| `resource_ids` | json | ✓ | validated Go `[]string`: 1..`MEDIGO_SHARING_MAX_RESOURCES_PER_INVITATION` (default 50), no duplicates, every entry a 15-char id, all of the kind implied by `kind` ([D-14](./research.md#d-14)) |
| `level` | select, MaxSelect 1 | ✓ | `view` \| `edit`. `edit` refused when `kind = family_history_share` (FR-007) |
| `note` | text ≤500 | | the sender's own words. Shown in the preview and the email (FR-023) and copied onto every grant it creates |
| `status` | select, MaxSelect 1 | ✓ | `pending` \| `accepted` \| `rejected` \| `cancelled` \| `revoked` \| `expired` — §3 |
| `token_hash` | text, 64 chars | ✓ | hex SHA-256 of the emailed credential. **The credential itself is never stored** ([D-15](./research.md#d-15)) |
| `expires_at` | date | ✓ | required. Default now+`MEDIGO_SHARING_INVITATION_TTL` (168h); settable between 1h and 1y (FR-017) |
| `responded_at` | date | | set by `respond` only |
| `response_note` | text ≤500 | | the recipient's optional note on accept or decline (FR-027) |
| `delivery` | select, MaxSelect 1 | ✓ | `email` \| `in_app_only`. Records what actually happened at send, so the sender's list can say "no email could be sent" truthfully (FR-022) |

**Indexes**

| Name | Unique | Columns | Where | Why |
|---|---|---|---|---|
| `idx_invitations_token` | ✓ | `token_hash` | | the public preview's only lookup |
| `idx_invitations_recipient_pending` | | `LOWER(recipient_email), status, expires_at` | | the recipient's list, and the FR-020 duplicate-pending check |
| `idx_invitations_sender` | | `sender, status, created` | | the sender's list, paged |
| `idx_invitations_status_expiry` | | `status, expires_at` | | the tidy pass ([D-19](./research.md#d-19)) |

**Invariants** (`internal/domain/share.Invitation.Validate`)

| # | Rule | Requirement |
|---|---|---|
| I1 | `recipient_email` is a syntactically valid address and is **not** the sender's, compared case-insensitively | FR-021 |
| I2 | `kind = family_history_share` ⇒ `level = view` | FR-007 |
| I3 | `expires_at` strictly in the future at send, and within [now+1h, now+1y] | FR-017, edge "an end date in the past" |
| I4 | `resource_ids` non-empty, ≤ max, unique, all 15-char ids | [D-14](./research.md#d-14) |
| I5 | `status != pending` ⇒ `responded_at != ''` **or** the status is `cancelled`/`expired` | FR-027 |
| I6 | `token_hash` is 64 lower-case hex characters | [D-15](./research.md#d-15) |

**Redaction** — `MarshalZerologObject` emits `id`, `kind`, `level`, `status`, `sender_id`,
`resource_count`, and nothing else. **Never** `recipient_email`, `note`, `response_note`,
`token_hash`.

**Lapsed is not a status.** An invitation with `status = pending` and `expires_at` in the past is
refused everywhere as lapsed (FR-029) *before* any tidy pass renames it. The `expired` status is
tidy-up, not truth.

---

## 3. The invitation state machine

Exactly one implementation: `internal/domain/share.Transition(from Status, ev Event) (Status, error)`,
a pure function over a table, with an exhaustive test.

```
                      ┌───────────────── (lapse is a predicate, not a transition) ─────────────┐
                      │                                                                        │
   send ──▶  PENDING ─┼─ recipient: accept ──▶ ACCEPTED ── sender: withdraw ──▶ REVOKED         │
                      │        (creates every grant, all-or-nothing, one transaction)          │
                      ├─ recipient: reject  ──▶ REJECTED                                        │
                      ├─ sender:    cancel  ──▶ CANCELLED                                       │
                      └─ tidy pass observes expires_at < now ──▶ EXPIRED  ◀────────────────────┘
```

| From | Event | To | Actor | Effect | Audit |
|---|---|---|---|---|---|
| `pending` | `accept` | `accepted` | recipient, signed in as `recipient_email` | creates every grant in `resource_ids`, all-or-nothing | `invite_respond` + one `share_grant` per grant |
| `pending` | `reject` | `rejected` | recipient | none | `invite_respond` |
| `pending` | `cancel` | `cancelled` | sender | none; the link dies immediately | `invite_cancel` |
| `pending` | `lapse` | `expired` | system (tidy) | none | `invite_expire` |
| `accepted` | `withdraw` | `revoked` | sender | revokes **every** grant this invitation created, in one transaction | `invite_withdraw` + one `share_revoke` per grant |
| anything else | anything | — | — | `*share.TransitionError{From, Event}` → `409 conflict` naming the state and `responded_at` | — |

**Terminal is terminal.** `accepted`, `rejected`, `cancelled`, `revoked` and `expired` never return
to `pending` by any route (FR-026, SC-012). The transition table has no edge that does, and the
exhaustiveness test enumerates every `(status, event)` pair.

**A lapsed `pending` invitation is refused for every event except `lapse`.** The guard is in the
service, before `Transition` is consulted: `if inv.Status == pending && inv.ExpiresAt.Before(now)
{ return ErrLapsed }` (FR-029, FR-032).

---

## 4. `audit_events` — amended

One new column and an additive vocabulary extension.

### 4.1 `reason` — a new column

| Field | PB type | Req | Constraints | Notes |
|---|---|---|---|---|
| `reason` | text | no | ≤40, drawn from the closed Go set `audit.Reason` (`Valid()`); the writer refuses anything else | why access was refused, or how an invitation was answered. **A bounded token, never a message and never content** |

It is a real column created by this phase's migration, because this phase is the first that needs
one: FR-069 requires a refusal to record *why*, and `invite_respond` records *how* it was answered.
Both were previously written as if a column existed (ANALYSIS C2) — it did not, and a write to a
field a collection does not have is a runtime failure, not a documentation gap.

It is `text` with a bounded Go vocabulary rather than a `SelectField`, exactly as
`export_jobs.error_code` is in phase 006 and for the same reason: the token set is owned by several
subsystems (this phase's authorizer, phase 006's auth path, job envelope and export worker), and a
`SelectField` assembled from their deltas is the failure mode ANALYSIS C1 records. The single
writer is `internal/service/audit`, the Go type is the one source of truth, and `phileak` asserts
no free text reaches the column.

This phase's values: `accepted`, `rejected` (an `invite_respond`), and `no_grant`, `revoked`,
`expired`, `view_only`, `not_addressed_to_you`, `disabled` (an `access_denied`, FR-069). Phase 006
extends the Go set with its own bounded failure codes; nothing else may.

An `access_denied` row written by phases 001–004 carries an empty `reason`, which is what those
phases' requirements ask for.

### 4.2 Vocabulary

Phase 001's migration declares the shared design contract's **complete** vocabulary, so
`share_grant`, `share_revoke`, `share_expire`, `invite_send`, `invite_respond`, `read_sensitive`
and `access_denied` already exist, as do the `share` and `invitation` target kinds.

`action` gains: `share_update`, `share_leave`, `invite_cancel`, `invite_withdraw`, `invite_expire`.

`target_kind` gains nothing.

**After this phase: twenty-six actions, twenty-seven target kinds.** The migration's test asserts
that **complete** set, not this delta (ANALYSIS C1).

**What each event carries** — and, as always, **no content** (FR-071):

| action | actor | target_kind | target_id | patient | Notes |
|---|---|---|---|---|---|
| `invite_send` | sender | `invitation` | invitation id | `''` | resource ids are **not** listed; the count is not a content leak but is also not needed |
| `invite_respond` | recipient | `invitation` | invitation id | `''` | the response (`accepted`/`rejected`) is the `reason` field |
| `invite_cancel` | sender | `invitation` | invitation id | `''` | |
| `invite_withdraw` | sender | `invitation` | invitation id | `''` | |
| `invite_expire` | — (`actor_kind = system`) | `invitation` | invitation id | `''` | written by the tidy pass |
| `share_grant` | recipient (the accepting account) | `share` | share id | the patient, when `resource_kind = patient` | |
| `share_update` | grantor | `share` | share id | as above | level and/or expiry changed |
| `share_revoke` | grantor | `share` | share id | as above | owner ended it |
| `share_leave` | grantee | `share` | share id | as above | grantee ended it |
| `share_expire` | — (`system`) | `share` | share id | as above | written by the tidy pass ([D-19](./research.md#d-19)) |
| `read_sensitive` | grantee, or a superuser reading somebody else's data | the record's kind, or `attachment` | record id | the patient | written **only** when the resolved grant is not `PermOwn` — never for an owner's own read, at any privilege level ([D-25](./research.md#d-25), [widened-authorization.md](./contracts/widened-authorization.md)) |
| `access_denied` | actor or `''` | the attempted kind | attempted id | the patient when known | `reason ∈ {no_grant, revoked, expired, view_only, not_addressed_to_you, disabled}` (FR-069) |

Never written: an email address, a note, a display name, an invitation token or hash, a level-change
free text, a patient's name.

**The two `actor_kind = system` rows — `invite_expire` and `share_expire` — are written by the tidy
pass, which has no HTTP request.** `audit_events.request_id` is `Required`, so both fill it from the
**run id** on the tidy's context, minted by the same helper that mints request ids and carried on
that run's zerolog lines; every row of one tidy run shares one value, so "which run lapsed this
grant" is one query (001 [data-model](../001-walking-skeleton/data-model.md) §3, 001 T240). The same
holds when the pass is run once from `medigo purge --sharing`.

---

## 5. Domain types (`internal/domain/share`)

```
type ResourceKind string   // "patient" | "family_member"
type Level        string   // "view" | "edit"
type Status       string   // "pending" | "accepted" | "rejected" | "cancelled" | "revoked" | "expired"
type Event        string   // "accept" | "reject" | "cancel" | "withdraw" | "lapse"

type Resource struct { Kind ResourceKind; ID string }

func (l Level) Allows(need access.Permission) bool   // view→PermView; edit→PermView|PermEdit; never PermOwn
func (s Share) Active(now time.Time) bool            // revoked_at == "" && (expires_at == "" || expires_at > now)
func Transition(from Status, ev Event) (Status, error)
```

`Level.Allows` is the single expression of FR-002, FR-003, FR-004, FR-005 and FR-006:
**no level ever returns true for `PermOwn`**, so deleting a patient, editing identity, changing
anyone's access and sharing onward are unreachable through a grant by construction, not by a check
([D-09](./research.md#d-09)).

`access.Grant` (phase 002, extended) becomes
`{Level Permission; ViaShare string; PatientID string; Note string; ExpiresAt *time.Time}` — the
extra fields exist so a handler can render "shared by X (view), until Y" and name the level in the
one `403` ([D-07](./research.md#d-07)) without a second query.

---

## 6. Migrations

Three, in this order, each with a real `down` (`migrations.Register` requires both — VSF FACT 8).

### `1756xxx100_shares.go`

Creates `shares` per §1: five select/relation/date/text fields, five indexes, all five API rules
`nil`. Cascade matrix asserted in the migration test: deleting a `patients` row, a `family_members`
row, a grantor `users` row or a grantee `users` row deletes the dependent share rows (FR-048,
FR-053) — and deleting an `invitations` row does **not** delete the share, it empties the relation
(`core/record_model.go:1618-1626`), which is why `shares.invitation` is not `CascadeDelete`.
`down`: delete the collection.

### `1756xxx200_invitations.go`

Creates `invitations` per §2: four indexes, `token_hash` unique, all rules `nil`.
`shares.invitation` is added in this migration (after `invitations` exists) rather than in the
first, so the two are independently reversible.
`down`: remove `shares.invitation`, then delete the collection.

### `1756xxx300_audit_vocab_sharing.go`

Extends `audit_events.action` with the five values in §4.
`down`: removes them, and is safe because it does not touch existing rows — a row carrying a removed
value is still readable; this is documented in the migration file as required by Principle IX.

**Migration tests** (`internal/store/migrations/*_test.go`), all failing first:

- all five API rules are `nil` on both new collections;
- **no new file field exists anywhere** (the `Protected: true` boot assertion still covers exactly
  `patients.photo` and `attachments.file`);
- `idx_shares_active_unique` exists, is unique, and its `WHERE` clause is `revoked_at = ''` —
  **asserted on the index SQL string**, because this is the one that fails silently
  ([D-01](./research.md#d-01));
- inserting a second active grant for the same `(resource_kind, patient, family_member, grantee)`
  fails; inserting one after the first is revoked succeeds;
- every `down` restores the previous schema exactly, verified by a round-trip on a throwaway app.

---

## 7. Seed fixtures (`medigo seed`) — deterministic, and shaped by what the tests need

| Account | Holds | Exists for |
|---|---|---|
| `owner@medigo.local` | 2 patients (one with 3 relatives), records of every kind, 1 attachment | the grantor in every story |
| `viewer@medigo.local` | an **accepted** `view` grant on patient A | US1, US3's viewer, the widened-authorization matrix |
| `editor@medigo.local` | an **accepted** `edit` grant on patient A | US3 |
| `cousin@medigo.local` | an accepted `view` grant on **one relative** of patient A | US4 and the isolation suite |
| `stranger@medigo.local` | nothing at all | the 404 row of every matrix |
| `empty@medigo.local` | nothing shared in **either** direction | FR-040 / SC-019 — the empty-state landmark assertion |
| `left@medigo.local` | a **revoked** grant (`revoked_by = grantee`) on patient A | US2, and the "they left" vs "I revoked" distinction |
| `lapsed@medigo.local` | a grant with `expires_at` in the past | US2 scenario 5, and the lapsed row of every matrix |
| `disabled@medigo.local` | an active grant, `users.disabled_at` set | FR-048's disabled-account clause |

Plus invitations in every state: one `pending` to an address with **no account**
(`newcomer@medigo.local`, deliberately unregistered — US5 scenarios 1–2), one `pending` whose
`expires_at` is in the past, one `accepted`, one `rejected` with a note, one `cancelled`, one
`revoked`. The pending invitation's **plaintext token is written to `medigo seed`'s output** so the
Playwright spec can visit `/invite/{token}`; it is a seed fixture on a throwaway instance and is
never printed by the server at runtime.

`internal/testdata/pb_data` is regenerated from these fixtures by `task fixture:regen`. Forgetting
that step makes every integration test run against the previous schema — which is why the task
exists and is documented in [quickstart.md](./quickstart.md).

---

## 8. Scale fixture (SC-014)

`internal/testsupport/scale` gains a generator producing: one owner with **20 patients** and
**200 active grants** spread across them and 10 grantee accounts, and one grantee holding
**50 shared patients** across 12 grantors, with a mix of `view`/`edit`, expiring/open-ended and
revoked rows so the predicate is exercised rather than trivially satisfied. Assertions: both sharing
panels, both invitation lists and `/patients` inside 2 s; and a paging walk that creates and revokes
grants between pages and finds **0** repeated and **0** skipped ids.
