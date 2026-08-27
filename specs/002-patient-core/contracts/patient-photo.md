# Contract: `/api/v1/patients/{id}/photo`

Three operations. Requirements covered: FR-008, FR-009, FR-044, FR-045, FR-046, and the Edge Cases
"Photographs that are not photographs" and "Retrieving a photograph without permission".

**The three structural rules, restated because they are the point of this file**

1. `patients.photo` is `Protected: true`. PocketBase's file handler runs its authorization check
   only inside `if fileField.Protected` and has no else branch, so an unprotected field is served
   to any anonymous caller who knows the URL. The application **refuses to start** if the assertion
   fails (constitution VII).
2. Bytes are served **only** from these routes, through `app.NewFilesystem()` → `fsys.Serve`, after
   the service authorizes. PocketBase's `/api/files/` route and its file-token mechanism are never
   used: FR-044 forbids "any link that carries its own credential".
3. Thumbnails are generated **eagerly** on upload, using PocketBase's own key layout
   `<collectionId>/<recordId>/thumbs_<filename>/<size>_<filename>`, so that PocketBase's own
   replace-and-delete cleanup (`core/field_file.go:612`, `DeletePrefix(".../thumbs_<filename>/")`)
   still finds and removes them. Anything else orphans thumbnails on every replacement.

---

## `putPatientPhoto` — `PUT /api/v1/patients/{id}/photo`

**Request**: `multipart/form-data`, one part named `photo`.
**Response 200**:

```json
{ "photo_url": "/api/v1/patients/pat_abc/photo?size=100x100t",
  "sizes": ["original", "100x100t", "400x400f"],
  "updated_at": "2026-08-26T11:04:00Z" }
```

Idempotent whole-resource replace, hence `PUT`. Replacing removes the previous file and its
thumbnails; **the previous photograph is not retrievable afterwards** (FR-008, US1-5).

| Status | When | Requirement |
|---|---|---|
| 200 | stored, both thumbnails generated | FR-008, FR-009 |
| 415 `unsupported_media_type` | the **sniffed** content type is not `image/jpeg`, `image/png` or `image/webp` | FR-008, Edge case |
| 413 `payload_too_large` | over `MEDIGO_FILES_PHOTO_MAX_BYTES` (default 15 MiB) | FR-008 |
| 422 `validation_failed` | no `photo` part, or more than one | |
| 404 | not owned / does not exist. **Nothing is stored.** | FR-042 |
| 401 | no session | FR-043 |

**Detection is content-based, and PocketBase does it.** `core/field_file.go:298` →
`core/validators/file.go:70-79` opens the reader and calls `mimetype.DetectReader`. The client's
declared `Content-Type` and the filename are never consulted — which is exactly FR-008's "what the
file claims to be is never trusted over what it is".

**PocketBase's rejection message must not be propagated.** `core/validators/file.go:63` formats it
as `Failed to upload %q due to unsupported file type.` with the **original filename** interpolated.
A filename is PHI (constitution VII). The handler maps the PB validation error into MediGo's
envelope with a fixed, PHI-free message.

**Mandatory tests**

- A `.png` file renamed to `.jpg` is accepted (sniffed as PNG, PNG is allowed).
- A PDF renamed to `photo.jpg` is **rejected 415** and nothing lands on the filesystem.
- The 415 response body and the captured zerolog stream contain **no substring** of the uploaded
  filename (FR-046, SC-008).
- After a successful upload, `100x100t` and `400x400f` exist on the filesystem **before** any
  request for them is made (FR-009, eager).
- After a replacement, the previous original and both previous thumbnails are gone (US1-5).
- Account B uploading to Account A's patient → 404, and Account A's photo is unchanged.

---

## `getPatientPhoto` — `GET /api/v1/patients/{id}/photo`

**Query**: `?size=original|100x100t|400x400f` (default `100x100t`). Any other value → `422`.

**200**: the image bytes.

| Header | Value | Why |
|---|---|---|
| `Content-Type` | the stored MIME type | |
| `Cache-Control` | `private, no-store` | FR-044: never cached by a shared cache |
| `Vary` | `Cookie, Authorization` | |
| `ETag` | derived from the stored file key (which carries PocketBase's random filename suffix, so it changes on every replacement) | cheap 304s without leaking |
| `Content-Disposition` | `inline; filename="photo.jpg"` | **a generic name.** The uploaded filename is PHI and is never echoed. |

| Status | When |
|---|---|
| 200 / 304 | the actor owns the patient |
| 404 | not owned, does not exist, or the patient has no photo — all three indistinguishable (FR-042, FR-044) |
| 401 | no session |
| 422 | unrecognised `?size=` |

A successful fetch writes a `read_sensitive`/`patient` audit row when — and only when — the
resolved grant is not the actor's own ownership: an owner fetching their own person's photograph
writes **no** row, a superuser fetching somebody else's writes one, and from phase 005 a share
recipient writes one. FR-045 requires the trail to record changes of photograph and refused
attempts, not an owner's own reads; the single statement of the sensitive-read rule lives in
phase 005's [`contracts/widened-authorization.md`](../../005-sharing-and-collaboration/contracts/widened-authorization.md)
§"Where `read_sensitive` is written" and governs here too (FR-045, 005 D-25).

**Mandatory tests**

- Anonymous `GET` → 401, and the body contains no patient information (FR-043).
- Account B → 404, identical body to a nonexistent id (SC-005).
- No route anywhere in `medigo routes` serves patient files without going through the authorizer —
  asserted by a test that walks the route inventory looking for anything under `/api/files`.

---

## `deletePatientPhoto` — `DELETE /api/v1/patients/{id}/photo`

**204** on success. Removes the file and both thumbnails.

| Status | When |
|---|---|
| 204 | removed, or the patient already had no photo (idempotent) |
| 404 | not owned / does not exist |
| 401 | no session |

Writes an `update`/`patient` audit row. **The photo is hard deleted** — soft delete arrives with
attachments in phase 004 and applies to attachments only (constitution VII).
