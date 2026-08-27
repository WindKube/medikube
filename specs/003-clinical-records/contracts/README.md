# Contracts: Clinical Records (phase 003)

These are the contracts the contract tests are written against. Anything not stated here is
inherited from the shared design contract §2.1 and from phase 001's `internal/web` layer.

## Files

| File | Covers |
|---|---|
| [records-clinical.md](./records-clinical.md) | the six-operation record family as it applies to the 13 new kinds — DTOs, filters, errors, authorization |
| [treatment-medications.md](./treatment-medications.md) | 3 operations — the payload-carrying join |
| [tags.md](./tags.md) | 4 operations — tags |
| [search.md](./search.md) | 1 operation — grouped cross-kind search |
| [pages.md](./pages.md) | the 29 page routes, landmarks and smoke expectations |

**Phase 003 adds 8 `/api/v1` operations and 0 record routes.**

## Conventions that apply to every operation below

1. Base path `/api/v1`. **No trailing slashes** — PocketBase has done no trailing-slash
   normalisation since v0.23, so `/api/v1/tags/` is a different (unregistered) route.
2. `Content-Type: application/json` in and out. **Unknown request fields are rejected** with
   `422`; duplicate JSON keys are rejected (Go 1.27 `encoding/json/v2`).
3. Slices always marshal as `[]`, never `null`.
4. Dates named `*_on` are `"YYYY-MM-DD"`. Instants named `*_at` are RFC3339 **UTC**.
5. Pagination is cursor-based:
   `{"items": [...], "next_cursor": "..."|null, "total": 12}` — `total` present only when
   `?count=true`. `?limit=` default 25, max 100. Cursors are opaque and HMAC-signed and encode
   `(sort keys, last values, last id)`, never an offset.
6. Sorting is `?sort=` with a comma list and a `-` prefix for descending, from a per-resource
   allowlist. Defaults are per-kind (`data-model.md` §3, research D-06).
7. Filtering is explicit named parameters only. **PocketBase's filter DSL never reaches the wire.**
8. `PATCH` and `DELETE` on a clinical record **require** `If-Match`. A mismatch is `412`.
   Every record response carries `ETag: W/"<updated>"`.
9. **A resource the caller may not see returns `404`, never `403`.** For anything patient-scoped,
   existence is itself PHI. `403` is reserved for resources whose existence the caller already
   knows about — which, in this phase, is nothing.
10. Error envelope, on every non-2xx, without exception:

```json
{ "error": {
    "code": "validation_failed",
    "message": "human-readable, PHI-free",
    "request_id": "3f2b...",
    "fields": [ { "field": "resolved_on", "code": "before_onset",
                  "message": "must not be earlier than onset_on" } ] } }
```

`message` for a `500` is always the literal `"internal error"`.

## Status codes used in this phase

| Code | Envelope `code` | When |
|---|---|---|
| 200 | — | read, update, list |
| 201 | — | create, with `Location: /api/v1/records/{kind}/{id}` |
| 204 | — | delete, link delete |
| 400 | `patient_required` | a list or search without `?patient=` |
| 400 | `bad_request` | an unknown filter, a bad cursor, a bad `sort` key |
| 401 | `unauthenticated` | no session |
| 404 | `not_found` | absent **or** not reachable by this actor — indistinguishable |
| 409 | `conflict` | uniqueness violated, or an attempt to re-file a record against another patient |
| 412 | `version_mismatch` | `If-Match` absent or stale |
| 413 | `payload_too_large` | body over the configured limit |
| 415 | `unsupported_media_type` | not `application/json` |
| 422 | `validation_failed` | domain validation, unknown field, duplicate key — with `fields[]` |
| 428 | `precondition_required` | `PATCH`/`DELETE` on a record with no `If-Match` header |
| 429 | `rate_limited` | |
| 500 | `internal_error` | message always `"internal error"` |

## The authorization rule, stated once

Every operation resolves an `access.Actor` from the PocketBase auth record on the request, then
calls exactly one of:

- `Authorizer.Patient(ctx, actor, patientID, need)` — for lists, creates and search.
- `Authorizer.Record(ctx, actor, kind, recordID, need)` — for read, update, delete, link.

Resolution order: superuser → allow and audit; patient owner → `PermOwn`; otherwise `ErrNotFound`.
(Shares arrive in phase 005 and slot in between as an additive widening.)

**The active patient is never consulted for authorization.** A client-supplied `patient` on a
create is validated *against the actor's grants*, never trusted. A `patient` on a patch is refused.

Every patient-touching operation carries, at minimum, these tests (FR-092, SC-004):
`owner succeeds` · `stranger receives 404 identical to a non-existent id` ·
`unauthenticated receives 401 with no patient information` ·
`cross-patient reference receives 404 disclosing nothing`.
