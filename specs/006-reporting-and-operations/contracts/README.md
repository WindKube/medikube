# Contracts: Reporting and Operations (phase 006)

Twenty-five `/api/v1` operations, seven pages, three page-action routes. These files are what the
contract tests assert against; where one of them disagrees with `research.md`, `research.md` is the
reasoning and this is the wire.

## Files

| File | Operations | Subject |
|---|---|---|
| [audit.md](./audit.md) | 68 | The activity trail reader and its streaming CSV |
| [reports.md](./reports.md) | 69–70 | Per-kind counts, a selection's resolved count, the chart picker |
| [report-templates.md](./report-templates.md) | 71–75 | Saved reports |
| [exports.md](./exports.md) | 76–79, **91** | Request, list, status, download, cancel — for both reports and exports |
| [admin-instance.md](./admin-instance.md) | 80–81 | The operator overview and the instance's posture |
| [admin-users.md](./admin-users.md) | 82–83 | Account administration |
| [admin-backups.md](./admin-backups.md) | 84–90 | List, take, upload, preview, download, restore, delete |
| [auth-oauth2.md](./auth-oauth2.md) | **4** | Sign-in through an external identity provider |
| [pages.md](./pages.md) | — | 7 pages, 3 page-action routes, and the whole-product browser sweep |

Operation 91 (`POST /api/v1/exports/{id}/cancel`) is **new relative to SHARED-DESIGN §2.3** and is
recorded in [plan.md's Deviations](../plan.md#deviations-from-the-shared-design-contract), as is the
method change on operation 88.

**Operation 4 is not new — it was unowned.** An earlier revision of SHARED-DESIGN §2.3 listed
`POST /api/v1/auth/oauth2` among the operations "deferred out of the suite", belonging to no
phase (cross-artifact finding **H7**); §2.3 now allocates it here under "Formerly deferred, now
allocated". This phase claims it, for the reason in the Deviations table: external sign-in is a
deployment integration and the operator surface that configures providers is here. It is the one
operation in this phase that is **public** and the one that is not `AdminOnly`.

---

## Conventions that apply to every operation below

Stated once here and not repeated per operation. All of them are SHARED-DESIGN §2.1's and are
unchanged by this phase.

1. Base path `/api/v1`. **No trailing slashes, ever** — the registry rejects a path ending in `/`.
2. `Content-Type: application/json` in and out, except the archive upload (multipart), the archive
   download (`application/zip`), the artifact download (`application/pdf` or `application/zip`) and
   the trail's CSV branch (`text/csv`).
3. **Unknown JSON fields are rejected** with `422`, never ignored. Duplicate keys are rejected.
   Slices marshal as `[]`, never `null` (Go 1.27 `encoding/json/v2`).
4. Pagination is the shared cursor envelope: `?limit=` (default 25, max 100), `?cursor=` (opaque,
   HMAC-signed keyset — never an offset), `{"items":[…],"next_cursor":…}`, and a `total` only when
   `?count=true`. Every list in this phase pages this way, which is what makes FR-122's
   never-repeat-never-skip guarantee true under concurrent writes.
5. Sorting is `?sort=` from a per-resource allowlist. Filtering is **explicit named parameters
   only**; PocketBase's filter DSL never reaches the wire.
6. Optimistic concurrency: `report_templates` responses carry an `ETag` from `updated`; `PATCH` and
   `DELETE` **require** `If-Match`; a mismatch is `412 version_mismatch`
   ([D-38](../research.md#d-38), FR-029). Job rows are not editable, so they carry no `If-Match`
   requirement.
7. The error envelope, always, on every non-2xx:

   ```json
   { "error": { "code": "artifact_expired",
                "message": "human-readable, PHI-free, naming no storage location",
                "request_id": "…",
                "fields": [ { "field": "charts", "code": "too_many_charts", "message": "…" } ] } }
   ```

8. **A resource the caller may not see returns `404`, not `403`**, for anything patient-scoped and
   for every operator route (FR-076: "indistinguishable from the view or action not existing").
9. Every operation has a stable `operationId`, asserted by the Principle IX gate to exist in both the
   route registry and the committed `api/openapi.json`.

---

## Status codes used in this phase

| Code | Meaning here |
|---|---|
| `200` | read, or an act whose result is the resource (cancel, admin update) |
| `201` | a saved report was created (`Location` header) |
| `202` | a job was accepted (`POST /exports`), or a restore was accepted and the process will restart |
| `204` | a saved report or an archive was deleted |
| `400` | a required query parameter is missing or unknown |
| `401` | unauthenticated |
| `403` | `forbidden_view_only`, `password_change_required`, `account_disabled` — the three cases where the caller demonstrably already knows the thing exists |
| `404` | not found, **and every authorization failure**, including every operator route reached without the administrative tier |
| `409` | `duplicate_name`, `job_in_progress`, `archive_operation_in_progress`, `patient_unreachable`, `not_cancellable` |
| `410` | `artifact_expired` — it existed, its window closed, and it is stated plainly rather than as an error (FR-047, US8 AS-2) |
| `412` | `version_mismatch` on a saved report |
| `413` | `archive_too_large` on an archive upload |
| `415` | the archive upload is not a zip |
| `422` | `validation_failed`, `nothing_matched`, `too_many_records`, `too_many_charts`, `not_enough_readings`, `unknown_unit`, `unknown_tag` |
| `500` | `internal_error`, message always the literal `"internal error"` |
| `503` | `restore_in_progress` — the instance is briefly unavailable (FR-106) |

**No message produced by this phase names a storage location, a file name, a record value, a person's
name or a tag name** (FR-118). Every `error_code` is drawn from the bounded set in
[data-model.md §2](../data-model.md#2-export_jobs--new).

---

## The actor matrix, applied to every operation in this phase

FR-128 and SC-025 require it, and `internal/service/access/coverage_test.go` fails the build if an
operation this phase adds has no entry.

| Actor | Patient-scoped operation | Operator operation |
|---|---|---|
| unauthenticated | `401`, disclosing nothing | `401` |
| a stranger to the person / the resource | `404`, **byte-identical** to a non-existent id | `404`, identical to the route not existing |
| the owner | success | — |
| a grantee at `view` | success on reads; `403 forbidden_view_only` on writes | — |
| a grantee at `edit` | success | — |
| a grantee whose access **ended** between request and use | `404`; and for a running job, the person is **dropped from the result and named in the manifest as withdrawn** ([D-09](../research.md#d-09)) | — |
| an account with `role = user` | — | `404` **and one `access_denied` audit row** (FR-076, SC-010) |
| an account with `role = admin` | no privileged access to any person's records, ever ([D-48](../research.md#d-48)) | success |
| an account with `must_change_password = true` | `403 password_change_required` on **every** route but the password change | same |
| a disabled account | `401` — its token was invalidated when it was disabled ([D-49](../research.md#d-49)) | same |

The two rows that are easiest to get wrong, and are therefore tested first in every HTTP suite:

- **an administrator downloading somebody else's produced document or archive is `404`**, not
  success. FR-013 says so in as many words; an administrator sees counts, never contents.
- **`role = admin` is not the break-glass credential.** Reaching a person's records requires the
  PocketBase superuser, whose sessions are recorded as `admin_session` (FR-120).

---

## Audit expectations, stated once

Every operation below names its audit action. Three rules hold for all of them:

1. **Content never enters an entry** — actor, action, target kind, opaque target id, patient id,
   timestamp, request id, a bounded `reason` token, an `affected` count, and nothing else (FR-114,
   FR-068). There is no `ip`: the trail has no such column (001 research D-19).
2. **A refusal is recorded too** (FR-073): every `404` and `403` produced by an operator route, an
   artifact download, an archive operation or a saved report belonging to somebody else writes one
   `access_denied` row with a bounded `reason`. Probing leaves a trace.
3. **Reading is not recorded; exporting is** (FR-075). Reading the trail writes nothing. Exporting it
   writes exactly one `audit_export`. The operator handbook states this so that an absence of read
   entries is not mistaken for an absence of reads.

## Where the counts come from

`GET /api/v1/reports/summary` and the records a document actually contains are produced by **one**
resolver, `report.Selection` ([D-44](../research.md#d-44)). A contract test asserts, over a table of
selections including empty ones, that `Counts()` equals the number of payloads `Each()` yields, per
kind and in total. SC-002 is therefore true by construction rather than by care.
