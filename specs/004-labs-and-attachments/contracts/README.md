# Contracts: Labs and Attachments (phase 004)

These are the contracts the contract tests are written against. Anything not stated here is
inherited from the shared design contract §2.1 and from phase 001's `internal/web` layer.

## Files

| File | Covers |
|---|---|
| [lab-results.md](./lab-results.md) | the six-operation record family as it applies to `lab_result`, including the component replace-set — **0 new operations** |
| [lab-components.md](./lab-components.md) | 2 operations — the per-patient rollup and the trend series |
| [catalog-lab-tests.md](./catalog-lab-tests.md) | 1 operation — the read-only standardized test catalogue |
| [attachments.md](./attachments.md) | 6 operations — upload, list, stream, describe, trash, restore |
| [pages.md](./pages.md) | 4 page routes and 4 page-action routes, with landmarks and smoke expectations |

**Phase 004 adds 9 `/api/v1` operations and 0 record routes.**

| # | Method | Path | `operationId` |
|---|---|---|---|
| 44 | GET | `/api/v1/catalog/lab-tests` | `listCatalogLabTests` |
| 49 | POST | `/api/v1/attachments` | `uploadAttachment` |
| 50 | GET | `/api/v1/attachments` | `listAttachments` |
| 51 | GET | `/api/v1/attachments/{id}` | `getAttachmentContent` |
| 52 | PATCH | `/api/v1/attachments/{id}` | `updateAttachment` |
| 53 | DELETE | `/api/v1/attachments/{id}` | `deleteAttachment` |
| 54 | POST | `/api/v1/attachments/{id}/restore` | `restoreAttachment` |
| 55 | GET | `/api/v1/lab-components` | `listLabComponents` |
| 56 | GET | `/api/v1/lab-components/trend` | `getLabComponentTrend` |

Operation 57 (`GET /api/v1/search`) is **not** in this phase — phase 003 delivered it. Operation 44
moves here from phase 002, which never built the catalogues. Net effect on the 94-operation
budget: zero.

## Conventions that apply to every operation below

1. Base path `/api/v1`. **No trailing slashes** — PocketBase has done no trailing-slash
   normalisation since v0.23, so `/api/v1/attachments/` is a different, unregistered route.
2. `Content-Type: application/json` in and out, with exactly **two** documented exceptions in this
   phase: `POST /api/v1/attachments` takes `multipart/form-data`, and
   `GET /api/v1/attachments/{id}` returns the document's own media type.
3. **Unknown request fields are rejected** with `422`; duplicate JSON keys are rejected
   (Go 1.27 `encoding/json/v2`).
4. Slices always marshal as `[]`, never `null`.
5. Dates named `*_on` are `"YYYY-MM-DD"`. Instants named `*_at` are RFC3339 **UTC**. A date-only
   value renders as the same calendar date regardless of the viewer's time zone.
6. Pagination is cursor-based: `{"items":[…], "next_cursor":"…"|null, "total":12}` — `total` only
   when `?count=true`. `?limit=` default 25, max 100. Cursors are opaque, HMAC-signed, and encode
   `(sort keys, last values, last id)` — never an offset, so a concurrent insert or delete cannot
   duplicate or skip a row (FR-009, FR-069, and the "never shows the same item twice" edge case).
7. Sorting is `?sort=` with a comma list and a `-` prefix for descending, from a per-resource
   allowlist. An unknown key is `400 bad_request`.
8. Filtering is explicit named parameters only. **PocketBase's filter DSL never reaches the wire.**
9. `PATCH` and `DELETE` on a **lab result** require `If-Match`; every record response carries
   `ETag: W/"<updated>"`. **Attachments do not require `If-Match`** (research D-30).
10. **A resource the caller may not see returns `404`, never `403`.** For anything patient-scoped,
    existence is itself PHI (FR-073). `403` is reserved for resources whose existence the caller
    already knows about — which, in this phase, is nothing.
11. Error envelope, on every non-2xx, without exception:

```json
{ "error": {
    "code": "validation_failed",
    "message": "human-readable, PHI-free",
    "request_id": "3f2b…",
    "fields": [ { "field": "collected_on", "code": "date_order",
                  "message": "must not be earlier than the ordered date" } ] } }
```

`message` for a `500` is always the literal `"internal error"`. A refusal shown to an uploader
**may** name their own file back to them in `message`; that name **must not** appear in any log,
metric, span or Sentry event (FR-079), and a test asserts the asymmetry.

## Status codes used in this phase

| Code | Envelope `code` | When |
|---|---|---|
| 200 | — | read, update, list, stream |
| 201 | — | create, with `Location` |
| 204 | — | delete |
| 206 | — | a satisfied `Range` request on document content |
| 304 | — | a matched `If-None-Match` on document content |
| 400 | `patient_required` | a patient-scoped list without `?patient=` |
| 400 | `unit_required` | a trend with more than one unit and no `unit` (research D-31) |
| 400 | `bad_request` | an unknown filter, a bad cursor, a bad `sort` key, a malformed `size` |
| 401 | `unauthenticated` | no session |
| 404 | `not_found` | absent **or** not reachable by this actor — byte-identical responses |
| 409 | `retention_expired` | restore attempted after the retention window closed |
| 409 | `owner_record_missing` | restore attempted when the owning record no longer exists |
| 412 | `version_mismatch` | stale `If-Match` on a lab result |
| 413 | `payload_too_large` | a document over `MEDIGO_FILES_MAX_UPLOAD_BYTES`; the message states the limit |
| 415 | `unsupported_media_type` | not `application/json` where JSON is required; a document whose **sniffed** type is not accepted — the message names the accepted types |
| 422 | `validation_failed` | domain validation, unknown field, duplicate key — with `fields[]` |
| 428 | `precondition_required` | `PATCH`/`DELETE` on a lab result with no `If-Match` |
| 429 | `rate_limited` | |
| 500 | `internal_error` | message always `"internal error"` |

Field-level codes introduced by this phase: `panel_and_value`, `ref_range_inverted`,
`value_kind_mismatch`, `date_order`, `empty_file`, `unsupported_file_type`, `file_too_large`,
`too_long`, `invalid_value`.

## The authorization rule, stated once

Every operation resolves an `access.Actor` from the PocketBase auth record on the request, then
calls exactly one of:

- `Authorizer.Patient(ctx, actor, patientID, need)` — for lists, creates and trends;
- `Authorizer.Record(ctx, actor, kind, recordID, need)` — for read, update and delete of a lab
  result;
- `Authorizer.Patient(...)` on the **attachment's own `patient`** — for every attachment
  operation. The attachment's patient is the anchor; the owning record is validated for integrity,
  never for permission.

Resolution order: superuser → allow **and audit**; patient owner → `PermOwn`; otherwise
`ErrNotFound`. Shares arrive in phase 005 and slot in between as a purely additive widening — this
phase's rules are written so that widening changes **who passes the check and nothing else**
(spec, permission boundaries).

"and audit" above is precise, and it is the whole of the `read_sensitive` condition: a
`read_sensitive` row is written when the resolved grant is **anything other than the reader's own
ownership**, and never when it is `PermOwn`. So a superuser reading somebody else's document writes
one, a share recipient in phase 005 writes one, and an owner reading their own writes none. The
rule is stated once for records and documents alike in phase 005's
[`contracts/widened-authorization.md`](../../005-sharing-and-collaboration/contracts/widened-authorization.md);
this phase implements it (FR-076). `access_denied`, by contrast, is unconditional — a refusal was
never an owner's own read (FR-073).

**The active patient is never consulted for authorization.** A client-supplied `patient` on a
create is validated against the actor's grants, never trusted. A `patient` on a patch is refused.

The one exception is `GET /api/v1/catalog/lab-tests`, which is authenticated but not patient-scoped
because it contains nothing about anyone (FR-043).

Every patient-touching operation carries, at minimum, these tests (FR-080, SC-005):

- `owner succeeds`
- `stranger receives a 404 byte-identical to the response for an id that never existed`
- `unauthenticated receives 401 with no information about the patient`
- `the refusal appears in the activity trail as access_denied, by opaque id, with no content`

For document content, the third case is tested three ways because FR-075 names three: by opening
the address directly, by guessing the identifier, and while signed out.
