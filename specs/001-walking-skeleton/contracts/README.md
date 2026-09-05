# Phase 001 API Contracts

These files are what the contract tests are written against. This is the first phase, so this
document **establishes** the conventions rather than inheriting them; SHARED-DESIGN §2.1 is the
source and every rule below is followed without exception from here on.

## Conventions

- **Base path `/api/v1`. No trailing slashes, ever.** PocketBase has done no trailing-slash
  normalisation since v0.23, so `/api/v1/records/` and `/api/v1/records` are different routes.
  `httproute.Registry.Handle` **panics** on a path ending in `/`.
- **Plural, kebab-case resource segments.** Path parameters are `{id}` or `{kind}`. There is
  exactly **one** spelling of a record kind in paths and it comes from `kind.Kind.Segment()`:
  `medications`. (Phase 002's contracts once used the singular; they were corrected on 2026-08-27 and now
read the plural. See research D-05.)
- **Nesting depth is at most one.** `/api/v1/me/password` is legal;
  `/api/v1/me/medications/{id}/notes` is not.
- **Ownership is derived from the data, never from the caller.** There is no `?owner=` and no
  `?user=` parameter anywhere. Reads, updates and deletes take the owner from the stored record;
  creates take it from the authenticated actor. A client-supplied identity is never an
  authorization input.
- **Pagination is cursor-based.** `?limit=` (1..100, default 25) and `?cursor=` (opaque,
  HMAC-signed, encoding the sort keys and the last id — never an offset). Envelope
  `{"items": [...], "next_cursor": "..." | null}`. `total` appears **only** when `?count=true` is
  passed.
- **Sorting** is `?sort=` with a comma list and a `-` prefix for descending, drawn from a
  per-resource allowlist. A value outside the allowlist is `422`, never silently ignored.
- **Filtering is explicit named parameters only.** PocketBase's filter DSL **never reaches the
  wire** — it is an injection surface and it leaks the schema. It does not leave
  `internal/store`.
- **No partial responses.** There is no `?fields=`. List endpoints return a `*Summary` DTO;
  detail endpoints return the full DTO. Two shapes per resource, both in OpenAPI.
- **Verbs.** `GET` read; `POST` create → `201` + `Location`; `PATCH` partial update → `200`;
  `PUT` idempotent whole-resource replace; `DELETE` → `204`.
- **Optimistic concurrency.** Every medication response carries an `ETag` derived from `updated`.
  `PATCH` and `DELETE` on a medication **require** `If-Match`. A missing header is `422` with
  field `If-Match`; a mismatch is `412` **and the body carries the current representation** so
  the person can see what changed (FR-026, US1-9).
- **`Content-Type: application/json` in and out.** **Unknown JSON fields are rejected (`422`)**,
  not ignored. Slices marshal as `[]`, never `null`. Dates are `YYYY-MM-DD`; instants are RFC3339
  UTC. Duplicate keys are rejected. (Go 1.27 `encoding/json/v2`; research D-28.)
- **Every operation has a stable `operationId`**, asserted by the Principle IX gate to exist in
  **both** the route registry and `api/openapi.json`.

## The error envelope, on every non-2xx

```json
{ "error": {
    "code": "validation_failed",
    "message": "human-readable, PHI-free",
    "request_id": "01JQ8Z…",
    "fields": [ { "field": "ended_on", "code": "end_before_start", "message": "…" } ] } }
```

`fields` is present only for `validation_failed`. `request_id` is on **every** error, including
`500`, and is the same value that appears on every log line for the request (FR-054) — which is
what lets a person quote a reference to an operator without disclosing anything about themselves.

| Sentinel / condition | Status | `code` |
|---|---|---|
| `ErrUnauthenticated` — no valid session | 401 | `unauthenticated` |
| `ErrForbidden` — the resource's existence is already known to the caller | 403 | `forbidden` |
| `ErrNotFound`, **and every authorization failure on owner-scoped data** | 404 | `not_found` |
| `ErrVersionMismatch` — `If-Match` did not match | 412 | `version_mismatch` |
| `*ValidationError` | 422 | `validation_failed` (+ `fields[]`) |
| unknown or malformed JSON field | 422 | `validation_failed` with field code `unknown_field` |
| `ErrConflict` — uniqueness or invariant | 409 | `conflict` |
| registration attempted while closed | 403 | `registration_closed` |
| a recovery or confirmation token that is expired, already used or tampered with | 400 | `invalid_token` — one code for all three, so no token's former existence is disclosed |
| a recovery or confirmation message that cannot be sent because the instance has no outgoing mail configured | 503 | `mail_unconfigured` (FR-076) |
| a forged, tampered or unparseable cursor | 400 | `invalid_cursor` |
| `ErrRateLimited` | 429 | `rate_limited` |
| `ErrUnsupportedMedia` — an upload's sniffed content type is not accepted (phase 002 research D-17) | 415 | `unsupported_media_type` |
| an upload over `MEDIKUBE_FILES_PHOTO_MAX_BYTES` (phase 002 contracts/patient-photo.md) | 413 | `payload_too_large` |
| a patient-scoped list with no `?patient=` (phase 002 research D-13, contracts/medications-rescope.md) | 400 | `patient_required` |
| deleting a self-record (phase 002 FR-051, contracts/patients.md) — closing the account is what removes it | 409 | `self_record_protected` |
| `context.Canceled` / `context.DeadlineExceeded` | 499 / 504 | `client_closed` / `timeout` |
| anything else | 500 | `internal_error`, message always the literal `"internal error"` |

**The 500 message is a constant.** No handler ever echoes an internal error string to a client:
a PocketBase validation message can embed a filename, a driver error can embed a query, and both
are PHI leaks in this application. The real error goes to zerolog and, once, to Sentry.

## The universal authorization rule for this phase

Every operation below, except the five explicitly marked public, resolves as:

1. no valid session → **`401 unauthenticated`**, with a body that names no resource (FR-034);
2. `access.Authorizer.Record(ctx, actor, kind, id, need)` — or `.Owner(ctx, actor, ownerID, need)`
   for the account surface — where the id comes from the **stored record** or from the
   authenticated actor, and never from a caller-supplied owner parameter (FR-032);
3. the actor owns the record → allowed;
4. otherwise → **`404 not_found`**, **byte-identical apart from `request_id`** to the response for
   an id that never existed, and an `access_denied` audit row is written (FR-033, FR-036).

**A `403` is never returned for a medication.** Existence is itself information about a person's
health, so a refusal that distinguishes "not yours" from "not there" is a disclosure. `403` is
reserved for `registration_closed`, where nothing about any person is revealed.

**The public eight**, and nothing else: `GET /api/v1/auth/config`, `POST /api/v1/auth/register`,
`POST /api/v1/auth/login`, `POST /api/v1/auth/password-reset`,
`POST /api/v1/auth/password-reset/confirm`, `POST /api/v1/auth/verify-email/confirm`,
`GET /api/v1/healthz`, `GET /api/v1/readyz`. Everything else requires a session — including
`POST /api/v1/auth/verify-email`, which resends the confirmation to the **signed-in** account's own
address and takes no address from the caller. The three public recovery operations are public
because a person who cannot sign in is the only person who needs them; each is rate limited, and
each answers identically whether or not the address it was given has an account (FR-073).

## Operation inventory — 22

| # | operationId | Method | Path | Auth | File |
|---|---|---|---|---|---|
| 1 | `getAuthConfig` | GET | `/api/v1/auth/config` | public | [auth.md](./auth.md) |
| 2 | `register` | POST | `/api/v1/auth/register` | public | [auth.md](./auth.md) |
| 3 | `login` | POST | `/api/v1/auth/login` | public | [auth.md](./auth.md) |
| 4 | `refreshSession` | POST | `/api/v1/auth/refresh` | user | [auth.md](./auth.md) |
| 5 | `logout` | POST | `/api/v1/auth/logout` | user | [auth.md](./auth.md) |
| 6 | `getMe` | GET | `/api/v1/me` | user | [account.md](./account.md) |
| 7 | `updateMe` | PATCH | `/api/v1/me` | user | [account.md](./account.md) |
| 8 | `deleteMe` | DELETE | `/api/v1/me` | user | [account.md](./account.md) |
| 9 | `changePassword` | PUT | `/api/v1/me/password` | user | [account.md](./account.md) |
| 10 | `listRecords` | GET | `/api/v1/records` | user | [records.md](./records.md) |
| 11 | `listRecordsOfKind` | GET | `/api/v1/records/{kind}` | user | [records.md](./records.md) |
| 12 | `createRecord` | POST | `/api/v1/records/{kind}` | user | [records.md](./records.md) |
| 13 | `getRecord` | GET | `/api/v1/records/{kind}/{id}` | user | [records.md](./records.md) |
| 14 | `updateRecord` | PATCH | `/api/v1/records/{kind}/{id}` | user | [records.md](./records.md) |
| 15 | `deleteRecord` | DELETE | `/api/v1/records/{kind}/{id}` | user | [records.md](./records.md) |
| 16 | `streamRecords` | GET | `/api/v1/streams/records` | user | [streams.md](./streams.md) |
| 17 | `healthz` | GET | `/api/v1/healthz` | public | [health.md](./health.md) |
| 18 | `readyz` | GET | `/api/v1/readyz` | public | [health.md](./health.md) |
| 19 | `requestPasswordReset` | POST | `/api/v1/auth/password-reset` | public | [auth.md](./auth.md) |
| 20 | `confirmPasswordReset` | POST | `/api/v1/auth/password-reset/confirm` | public | [auth.md](./auth.md) |
| 21 | `requestEmailVerification` | POST | `/api/v1/auth/verify-email` | user | [auth.md](./auth.md) |
| 22 | `confirmEmailVerification` | POST | `/api/v1/auth/verify-email/confirm` | public | [auth.md](./auth.md) |

Operations 19–22 are the shared design contract's operations **7** and **8** plus the two the
contract never allocated for email confirmation, numbered **94** and **95** in the contract's own
additive scheme. The rows above are numbered by position in this phase's inventory; the contract's
stable identities are 7, 8, 94 and 95. Cross-artifact finding **H7**, and `plan.md`'s Deviations
table.

Operations 10–15 are **six operations that serve every clinical kind**, now and in every later
phase. Phase 003 registers eleven more kinds and adds **zero** routes.

`streamRecords` is `Kind: stream` in the registry and **is** documented in `api/openapi.json` as a
`text/event-stream` response, because FR-064 says the published description covers "every
operation in its public interface" and an SSE endpoint is one. The Principle IX gate therefore
asserts agreement over `api` **and** `stream` routes.

## Documented PocketBase-native paths that stay public

These are **not** `/api/v1` routes. They are recorded in the registry as `Kind: external` and
appear in the OpenAPI document as documented externals, so the Principle IX gate does not flag
them and so nobody discovers them by accident:

| Path | Why it stays reachable |
|---|---|
| `/_/` and its assets | The PocketBase superuser admin UI. It **ships in production**, hardened: mandatory superuser MFA, mandatory IP allowlist, every session audited, and a loud boot warning until both are configured (constitution VII, research D-32). |
| `/api/collections/_superusers/*` | The admin UI's own authentication. |
| `/api/collections/users/auth-with-password`, `/auth-refresh`, `/auth-methods` | PocketBase-native authentication. The lockdown is scoped to `/records` precisely so these survive (FACT 2). MediKube's `/api/v1/auth/login` is the **supported** path, but both are audited, because the audit row is written from `OnRecordAuthRequest("users")` rather than from MediKube's handler (research D-14). |
| `/api/collections/users/request-password-reset`, `/confirm-password-reset`, `/request-verification`, `/confirm-verification` | The same PocketBase-native family, reachable for the same reason. MediKube's `/api/v1/auth/password-reset*` and `/api/v1/auth/verify-email*` are the **supported** paths and the only ones the pages use; the native ones are recorded as externals so the Principle IX gate does not flag them and so nobody believes they were closed. They enforce the same token rules, because they are the same code MediKube's handlers call. |

Everything under `/api/collections/{collection}/records` and `/api/batch` returns **404** to
non-superusers, via a middleware bound at priority `-1019` — after `loadAuthToken` at `-1020`, so
`e.Auth` is populated — on top of the five-nil-rules lockdown and the boot assertion.
`Settings().Batch.Enabled = false`.

## What every contract test asserts

For each operation, at minimum:

1. the documented success status and response shape, round-tripped through
   `encoding/json/v2` (slices are `[]` not `null`, unknown fields rejected, duplicate keys
   rejected);
2. `401` for an anonymous caller, with a body naming no resource;
3. `404` for a second account, **byte-identical apart from `request_id`** to a genuine
   not-found, with an `access_denied` audit row written;
4. every documented error case, by its `code` rather than by its message;
5. `ExpectedEvents` proving the MediKube route fired the audit hooks and **zero** record-CRUD
   request events — i.e. that it did not go through PocketBase's auto-API.
