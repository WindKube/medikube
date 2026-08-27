# Contract: attached documents

**Operations added: 6.** Shared design contract §2.3 entries 49–54.

```
POST   /api/v1/attachments              uploadAttachment       multipart, 201 + Location
GET    /api/v1/attachments              listAttachments        the library and the per-record strip
GET    /api/v1/attachments/{id}         getAttachmentContent   streams bytes or a preview
PATCH  /api/v1/attachments/{id}         updateAttachment       description and category only
DELETE /api/v1/attachments/{id}         deleteAttachment       soft; ?purge=true is owner-or-superuser
POST   /api/v1/attachments/{id}/restore restoreAttachment      un-trash within the window
```

There is deliberately **no** metadata `GET` for a single attachment: the list DTO carries
everything the library and the record strip render, and `GET /attachments/{id}` is the content
stream. Replacement is not a seventh operation either — it is `POST /attachments` with `replaces`
(§1.3, research D-18).

---

## 1. `POST /api/v1/attachments`

`Content-Type: multipart/form-data`. This is one of exactly two non-JSON operations in MediGo.

| Part | Required | Notes |
|---|---|---|
| `file` | **yes** | the bytes |
| `patient` | **yes** | the patient the document concerns |
| `owner_kind` | **yes** | one of the fifteen registered kinds, `snake_case` |
| `owner_id` | **yes** | the owning record's opaque id |
| `description` | no | ≤ 500, PHI |
| `category` | no | `AttachmentCategory` |
| `replaces` | no | an existing attachment id this one supersedes (§1.3) |

`201` with `Location: /api/v1/attachments/{id}` and an `AttachmentSummary` body (§2.1).

### 1.1 What the server decides, and what it refuses

| Check | Order | Failure |
|---|---|---|
| body size, via `http.MaxBytesReader` **before** the multipart parse | 1 | `413 payload_too_large`, message states the limit in bytes; **nothing is stored and nothing is spilled to disk** (FR-053) |
| `size_bytes > 0` | 2 | `422`, code `empty_file` (FR-054) |
| **type sniffed from the content**, never from the part's `Content-Type` and never from the file name | 3 | — |
| sniffed type ∈ `MEDIGO_FILES_ALLOWED_MIME` | 4 | `415`, code `unsupported_file_type`, message **names the accepted types** (FR-052) |
| `owner_kind` is a registered kind | 5 | `422`, `invalid_value` |
| `owner_id` resolves in `owner_kind`'s collection **and** belongs to `patient` | 6 | `404 not_found`, disclosing nothing (FR-050) |
| the actor may reach `patient` | 0 (first) | `404 not_found` |

The type is decided by `http.DetectContentType` plus a nine-entry magic table for WebP, HEIC/HEIF
and TIFF (research D-14). **A `.csv` sniffs as `text/plain`** — there is no byte signature that
separates comma-separated values from prose — so the default allowlist contains `text/plain` and
not `text/csv`, a CSV stores with `mime: "text/plain"`, and that is the correct answer under
FR-051. Renaming a PDF to `.png` does not make it a PNG, and claiming `image/png` in the part
header does not either; both are tested.

The whole operation runs in one transaction, so a record deleted in another place while the upload
is in flight makes the transaction fail cleanly with **nothing stored and no document pointing at a
record that does not exist** (edge case: two things happening at once).

**Repeated uploads of identical content are two documents**, each with its own id, description and
timestamp; neither is discarded as a duplicate (FR-055).

### 1.2 After the commit

A `TxInfo().OnComplete` callback generates two thumbnails (`160x160t`, `1024x1024f`) for
`image/jpeg`, `image/png`, `image/gif` and `image/webp` only, and sets `has_preview`. **PDF, HEIC,
TIFF, text and CSV get a type icon and no preview** (FR-059, research D-17). A generation failure is
logged once, leaves `has_preview: false`, and **never fails the upload**.

### 1.3 `replaces` — how replacement works

When `replaces` names an attachment that belongs to the same patient **and** the same
`(owner_kind, owner_id)`, the operation, in one transaction:

1. creates the new attachment, inheriting the replaced one's `description` and `category` unless
   the request supplies its own;
2. sets `deleted_at = now` on the replaced one.

The result: the corrected version is on the record, and the replaced version is recoverable for the
full retention window (FR-061). A `replaces` id that resolves to another patient or another record
is `404`, disclosing nothing.

### 1.4 Errors and PHI

A refusal **may** name the uploader's own file back to them in `error.message` (FR-079). That name
must not appear in any log line, metric label, span attribute or Sentry event produced by the same
request, and `internal/testsupport/phileak` asserts exactly that asymmetry. A storage failure
returns `500` with the literal message `"internal error"` and reveals nothing about where files are
kept (edge case: environment failures).

---

## 2. `GET /api/v1/attachments`

| Parameter | Notes |
|---|---|
| `patient` | **required**; absence is `400 patient_required` |
| `owner_kind` | one kind — the per-record strip passes this with `owner_id` |
| `owner_id` | requires `owner_kind` |
| `category` | comma list from `AttachmentCategory` (FR-069) |
| `q` | case-insensitive substring over `original_name` and `description` (FR-069) |
| `deleted` | `true` returns **only** trashed documents; omitted or `false` returns only live ones (FR-069) |
| `usage` | `true` adds the `usage` block (FR-071) |
| `sort` | allowlist: `-created` (default), `created`, `original_name`, `size_bytes` |
| `limit`, `cursor`, `count` | standard |

### 2.1 `AttachmentSummary`

```json
{ "id": "att0000000000001",
  "patient": "pat0000000000001",
  "owner": { "kind": "lab_result", "id": "rec0000000000001", "label": "Comprehensive metabolic panel", "href": "/lab-results/rec0000000000001" },
  "original_name": "riverside-labs-2026-03-04.pdf",
  "size_bytes": 481203,
  "mime": "application/pdf",
  "category": "report",
  "description": "Laboratory's own printed report",
  "has_preview": false,
  "can_view_inline": true,
  "uploaded_by": { "id": "usr0000000000001", "name": "Tomas" },
  "created": "2026-03-04T10:22:00Z",
  "deleted_at": null,
  "days_until_purge": null }
```

- `owner.label` and `owner.href` are what satisfy FR-070 ("show which record it belongs to and
  offer a way to open that record"). `label` comes from the owning kind's registry entry, so a
  future kind gets it without this phase knowing.
- `has_preview` is stored, not guessed — a guessed preview URL is a failed network request and the
  Playwright gate asserts there are none (research D-17).
- `can_view_inline` is a lookup of `mime` against the **compile-time** inline-safe set (FR-057).
- `days_until_purge` is computed from `deleted_at` and the retention window; it is `null` for live
  documents.
- An owner whose record has since been deleted renders as
  `{"kind": "…", "id": "…", "label": null, "href": null}`, which is what the restore refusal in
  §6 explains.

### 2.2 The `usage` block

With `?usage=true`:

```json
{ "items": [ … ], "next_cursor": null,
  "usage": { "documents": 412, "bytes": 903112884,
             "trashed_documents": 7, "trashed_bytes": 14220110 } }
```

Trashed documents are counted **separately**, never folded into the live total (FR-071). The
instance-wide total is an operator concern and belongs to phase 006's `/api/v1/admin/system`; this
phase publishes it as the unlabelled Prometheus gauge `medigo_files_bytes_total`.

### 2.3 Paging and scale

Keyset cursors over `(created, id)` — paging the library while documents are being attached never
shows the same item twice and never skips one (edge case: scale and duration). 2,000 documents for
one patient page, narrow and sort without degrading (FR-085), served by `idx_attachments_library`.

Empty result: `200` with `items: []`; the library renders its empty state with guidance on where to
attach the first document (US2 scenario 1), inside the same page structure as a populated library.

---

## 3. `GET /api/v1/attachments/{id}` — the content stream

This operation returns **bytes**, not JSON.

| Parameter | Notes |
|---|---|
| `disposition` | `attachment` (default) or `inline` |
| `size` | omitted for the original; `160x160t` or `1024x1024f` for a preview. Any other value is `400 bad_request` |

`200` (or `206` for a satisfied `Range`, or `304` for a matched `If-None-Match`) streaming through
`app.NewFilesystem()` → `fsys.Serve`, which handles `Range`, `ETag` and the correctly quoted
`Content-Disposition` filename.

### 3.1 Response headers, every time

```
Content-Type: <the stored, sniffed mime>
Content-Disposition: attachment; filename="…"      (or inline; see 3.2)
X-Content-Type-Options: nosniff
Cache-Control: private, no-store
Content-Security-Policy: default-src 'none'; img-src 'self'; style-src 'none';
                         script-src 'none'; object-src 'none'; frame-ancestors 'self'; sandbox
```

The `sandbox` token is **omitted for `application/pdf`** and only for that type, because an
unkeyworded `sandbox` disables the browsers' built-in PDF viewers and PDF is the commonest
attachment there is; `script-src 'none'` and `object-src 'none'` remain in force (research D-16).

### 3.2 Inline versus download

`disposition=inline` is honoured **only** for types in the compile-time inline-safe set:
`application/pdf`, `image/jpeg`, `image/png`, `image/gif`, `image/webp`, `text/plain`. For any
other type the response is served with `attachment` disposition instead — not an error (FR-057,
US2 scenario 6). **An operator can widen what is accepted; nobody can widen what is inlined**
(FR-058).

### 3.3 Fidelity

`disposition=attachment` with no `size` returns the uploaded bytes **byte for byte**, under the
original name including non-Latin script, right-to-left text and characters that look like markup
— stored, listed and downloaded as text and never interpreted as anything else (FR-056, SC-004,
and the awkward-data edge cases). A very long name is displayed truncated in the UI and downloaded
in full.

### 3.4 Previews

`?size=` serves a thumbnail through the **same handler, the same authorization call and the same
audit row** as the original (FR-060). A `?size=` request for a document with `has_preview: false`
is `404` — the client should not have asked, and the list DTO told it not to.

### 3.5 Errors

| Case | Response |
|---|---|
| the actor cannot reach the patient | `404`, byte-identical to an id that never existed (FR-073) |
| unauthenticated, address opened directly | `401`; no name, size, type or preview is disclosed (FR-075) |
| a guessed id | `404` |
| the row exists but the bytes cannot be read | `500`, message `"internal error"`; the failure is recorded **once** for the operator and reveals nothing about storage locations (edge case: environment failures) |
| an unknown `size` value | `400 bad_request` |

**A document's address is not a credential** (FR-074): there is no `?token=`, no signed URL and no
`e.Auth.NewFileToken()` call anywhere in MediGo — a `forbidigo` pattern makes calling it a build
failure. Possessing the URL gives nothing to anyone who could not already reach that patient.

### 3.6 Audit

**A successful response — original, inline or preview — writes exactly one `read_sensitive` audit
row when, and only when, the resolved grant is something other than the reader's own ownership.**
An owner retrieving their own document writes **no** row at all. A superuser retrieving somebody
else's document writes one; a superuser retrieving their own writes none; from phase 005, a
recipient of a share writes one on every retrieval.

The rule is stated once, for records and documents alike, in phase 005's
[`contracts/widened-authorization.md`](../../005-sharing-and-collaboration/contracts/widened-authorization.md)
§"Where `read_sensitive` is written". This phase implements it and does not restate it
(FR-076, SC-006, research [D-20](../research.md#d-20), 005 [D-25](../../005-sharing-and-collaboration/research.md#d-25)).

The row carries actor, `target_kind: attachment`, the attachment id, the patient id, the request
id and the timestamp. Never the file name, never the description, never the mime, never any
bytes. A refusal writes `access_denied` with the same shape, **regardless of who was refused** —
`access_denied` is not conditioned on ownership, because a refusal by definition was not an
owner's own read (FR-073).

---

## 4. `PATCH /api/v1/attachments/{id}`

```json
{ "description": "Corrected scan from the laboratory", "category": "report" }
```

`200` with the `AttachmentSummary`. **`original_name`, `size_bytes` and `mime` are absent from this
DTO by construction** and are therefore not editable (FR-062). `patient`, `owner_kind` and
`owner_id` are absent too — a document is never re-filed onto another record or another patient by
a patch.

`If-Match` is **not** required (research D-30). A trashed document may be described; a purged one
is `404`.

---

## 5. `DELETE /api/v1/attachments/{id}`

`204`. Sets `deleted_at = now`. The document stops being listed with its record and in the library,
and remains recoverable for `MEDIGO_RETENTION_TRASH_DAYS` (default **30**) — a window the UI states
at the moment the account holder confirms (FR-063).

`?purge=true` is hard: the row, the blob and the thumbnails go, permanently, before the window has
closed. It is accepted from exactly two callers (FR-066):

| Caller | `?purge=true` |
|---|---|
| the **owner** of the patient the document concerns | `204`; the UI has already required a typed confirmation naming the file and stating that this cannot be undone |
| a **superuser**, for any document | `204` |
| a grantee reaching the document through somebody else's share (phase 005, either level) | `404` — byte-identical to the attachment not existing |
| anybody else | `404` |

An owner may destroy their own medical records on demand; a share recipient never gets a
destructive power the grant did not confer, and the `404` means the existence of the mode is not
disclosed to them. Phase 006 states no second rule — see its
[`research.md` D-14](../../006-reporting-and-operations/research.md#d-14).

The typed confirmation is a UI obligation, not a request field: the API takes no confirmation
token, because a second parameter that the caller supplies is not a control.

Deleting a document twice is idempotent: the second `DELETE` is `204` and does not extend the
window.

---

## 6. `POST /api/v1/attachments/{id}/restore`

Empty body. Runs inside a transaction that **re-reads `deleted_at` inside the transaction**, so it
cannot race the purge (research D-19).

| State inside the transaction | Response |
|---|---|
| trashed, within the window, owning record resolves | `200` with the `AttachmentSummary`, `deleted_at: null`; it is listed with its record again (FR-064, US2 scenario 11) |
| trashed, within the window, **owning record no longer exists** | `409`, code `owner_record_missing`; the message explains why and states that the content can still be retrieved until it is purged (FR-065, US2 scenario 13) |
| trashed, `deleted_at` older than the window | `409`, code `retention_expired` — the account holder is told the document is gone rather than shown a broken restore (edge case: the restore/purge race) |
| already live | `200`, idempotent |
| purged, or another patient's | `404`, byte-identical (FR-066, US2 scenario 12) |

Once the window closes and the maintenance cron has run, **nothing in the application can bring the
document back and its content is no longer stored** (FR-066, SC-007).

---

## 7. Scheduled maintenance (not an operation)

One `app.Cron()` entry, `medigo_attachment_maintenance`, daily, also runnable once as
`medigo purge`:

1. hard-delete every attachment whose `deleted_at` is older than the retention window; PocketBase
   removes the blob and the thumbnails with the record (FR-066);
2. sweep for orphans — rows whose `(owner_kind, owner_id)` no longer resolves — and move them to
   the trash, reporting the count as a gauge (research D-13);
3. refresh `medigo_files_bytes_total`.

A failure is logged, counted and retried on the next run. Each row is its own delete, so documents
remain **wholly** in the trash rather than half-deleted (edge case: environment failures).

### 7.1 The audit rows this cron writes, and where their `request_id` comes from

Step 1 writes one `delete` / `attachment` row per purged document, and step 2 one per quarantined
orphan — moving a row to the trash is a delete, and the vocabulary this phase uses is 001's
`create update delete read_sensitive access_denied`, never a bespoke value ([data-model](../data-model.md) §6). Both carry `actor_kind = system`: FR-077 makes
deleting and purging auditable, and a cron purge is still a purge. Step 3 writes nothing.

**A cron has no HTTP request, and `audit_events.request_id` is `Required`.** Every row this job
writes therefore fills `request_id` from the **run id** on the job's context, minted by the same
helper that mints request ids, and carried on that run's zerolog lines — so "which purge deleted
this document" is one query (001 [data-model](../../001-walking-skeleton/data-model.md) §3, 001
T240). All rows of one run share one `run_id`. `medigo purge`, which runs the same function from
the CLI, mints its own the same way.

---

## 8. Authorization matrix

| Actor | Operation | Result |
|---|---|---|
| owner of the patient | all six | success |
| a stranger | any, by id | `404`, byte-identical to an id that never existed; `access_denied` audited |
| a stranger | list with another patient's `?patient=` | `404` |
| unauthenticated | content, preview, list | `401`; nothing disclosed (FR-075) |
| anyone holding the URL but no access | content | `404` — the URL is not a credential (FR-074) |
| superuser | all six | success **and** an audit row |
| **phase 005 preview** | a grantee at `view` | may list, stream and preview; **may not** upload, replace, describe, delete or restore, and **may not purge early** (`?purge=true` is `404` to them — FR-066). Every retrieval by a grantee is audited, precisely because it is *not* an owner's: `read_sensitive` is conditioned on the resolved grant, per §3.6. This phase writes the permission checks so that widening changes only which actors pass them |
