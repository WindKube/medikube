# Contract: archives — backup and restore (operations 84–90)

Conventions, status codes, the actor matrix and the audit rules are in [README.md](./README.md).
Every operation here requires `role = admin`; anything else is `404` **and a recorded attempt**
(FR-114 of US7: "the answer is indistinguishable from the capability not existing and the attempt is
recorded").

**MediGo adds no second backup mechanism** (FR-112, Principle V). Every operation below is a thin
wrapper over PocketBase v0.40.1 ([D-21](../research.md#d-21)). What this phase adds is exactly five
things PocketBase does not do: the **preview**, the mandatory **safety copy**, the **confirmation**,
the **authorization**, and the **visibility of failures**.

## DTOs

```go
type Archive struct {
    Name       string  `json:"name"`               // the key in the backups filesystem
    TakenAt    string  `json:"taken_at"`           // RFC3339 UTC, from the object's ModTime
    Bytes      int64   `json:"bytes"`
    Origin     string  `json:"origin"`             // "manual" | "scheduled" | "uploaded" | "safety"
    TakenBy    *string `json:"taken_by"`           // opaque user id; null for scheduled
    Note       string  `json:"note,omitempty"`
    AppVersion *string `json:"app_version"`        // from medigo.json inside the archive; null if absent
    Compatible *bool   `json:"compatible"`         // null when app_version is null (D-25)
}

type RestorePreview struct {
    Archive       Archive       `json:"archive"`
    Now           InstanceScale `json:"now"`         // what exists on the instance right now
    WillBeLost    string        `json:"will_be_lost"` // the sentence, in words (FR-103)
    Blockers      []string      `json:"blockers"`     // bounded tokens: job_in_progress, archive_operation_in_progress,
                                                     // archive_unreadable, archive_version_unsupported, version_unknown
    ConfirmPhrase string        `json:"confirm_phrase"` // literally "restore <name>"
}

type InstanceScale struct {
    Patients  int   `json:"patients"`
    Records   int   `json:"records"`
    Documents int   `json:"documents"`
    Bytes     int64 `json:"bytes"`
}
```

`Origin`, `TakenBy` and `Note` are MediGo's own metadata, held in a sidecar `medigo-backups.json`
under `MEDIGO_STATE_DIR` and joined to the filesystem listing by name; an archive present in the
filesystem with no sidecar entry lists with `origin: "uploaded"` and no note, rather than being
hidden.

---

## 84. `GET /api/v1/admin/backups` — what the instance holds

`operationId: listBackups`

`app.NewBackupsFilesystem()` → `fsys.List("")` → `blob.ListObject{Key, Size, ModTime}`, joined to the
sidecar. Sorted newest first. Paged with the shared cursor (FR-122).

**Response** `200` — `Page[Archive]`, each row carrying when it was taken, its size, **how it came to
be taken**, who took it and its note (FR-099).

**Empty**: `200`, `items: []`; the page explains what an archive is and offers to take the first one
(US7 AS-1).

**Audit**: none on success; one `access_denied` on refusal.

---

## 85. `POST /api/v1/admin/backups` — take one now

`operationId: createBackup`

**Request**: `{ "note": "before the upgrade" }` — optional, ≤ 500.

**Effect**: `app.CreateBackup(ctx, name)` with `name = medigo_<YYYYMMDDHHMMSS>.zip` — compact, **not** RFC3339: the name reaches `audit_events.target_id` (`Max 64`) through the safety-copy composition below, and it must also be legal as a filename and as an S3 key, which a colon is not (ANALYSIS N2), plus a sidecar entry
recording `origin: "manual"`, the actor and the note.

**Response** `201` + `Location: /api/v1/admin/backups/{name}`, body `Archive`.

**Errors**: `409 archive_operation_in_progress` when `app.Store().Has(core.StoreKeyActiveBackup)` —
checked **before** any work, and PocketBase's own error is mapped to the same code if we lose the
race (FR-108, US7 AS-10).

**Audit**: `backup_create`, with `target_kind: backup` and the **archive name** in `target_id` — the
bounded exception to "never a name", sized for by 001's `≤64` column, and the same string this
archive's own routes are addressed by (001 [data-model](../../001-walking-skeleton/data-model.md) §3).

**Scheduled archives** are PocketBase's own: `Settings().Backups.Cron` and
`Backups.CronMaxKeep = MEDIGO_BACKUP_KEEP`, written at boot from configuration, with the max-keep
pruning already implemented upstream (FR-101). MediGo binds `OnBackupCreate` so a scheduled archive
writes `backup_create` on success and `job_failed` on error — which is what puts a failed scheduled
backup on the operator overview and in the trail instead of letting it pass silently (US7 AS-3).

---

## 86. `POST /api/v1/admin/backups/upload` — one taken elsewhere

`operationId: uploadBackup`

**Request**: `multipart/form-data` with `file` (the archive) and an optional `note`.

**Validation, before it is stored**: it opens as a zip; it contains `data.db`; it is within
`MEDIGO_EXPORT_MAX_BYTES`; and **the storage key is normalised and bounded to 64 characters** —
the uploader's filename is not trusted, and `audit_events.target_id` is `Max 64` (ANALYSIS N2).
Then `fsys.UploadMultipart(fh, key)`.

**Response** `201` — `Archive` with `origin: "uploaded"`. Once accepted it is **listed and treated
identically to one taken here** (FR-102, US7 AS-4): the same preview, the same restore path, the same
version rules.

**Errors**: `413 archive_too_large`; `415` when it is not a zip; `422 archive_unreadable` when it has
no `data.db`; `409 archive_operation_in_progress`.

**Audit**: `backup_upload`, `target_kind: backup`, the archive name in `target_id` (§ `backup_create`).

---

## 87. `GET /api/v1/admin/backups/{name}` — what would a restore do

`operationId: getBackupPreview`

**Response** `200` — `RestorePreview`, stating (FR-103, US7 AS-5):

- when the archive was taken, its size and its note;
- **which version of the application produced it** — read from `medigo.json` inside the archive
  ([D-25](../research.md#d-25), [D-26](../research.md#d-26)): locally with `zip.OpenReader` straight
  off `pb_data/backups/<name>`, or, when PocketBase's S3 backup storage is configured, by streaming
  into a scratch file under `.pb_temp_to_delete` first, because `archive/zip` needs an `io.ReaderAt`;
- how much exists on the instance **now** that would be replaced (`now`);
- **in plain words, that everything recorded since it was taken will be lost** — `will_be_lost` is a
  sentence, not a flag, because that is what the requirement asks for;
- the `confirm_phrase` the restore will require;
- every `blocker` that would refuse the restore right now, so an administrator learns about a running
  export **before** typing a confirmation phrase.

**Version compatibility**, decided in one function ([D-25](../research.md#d-25)):

| Archive | Result |
|---|---|
| `schema_version` ≤ the binary's highest known migration | `compatible: true` — PocketBase runs app migrations up on the next bootstrap |
| `schema_version` > the binary's highest | `compatible: false`, blocker `archive_version_unsupported` — MediGo cannot migrate down into a binary that does not know the schema |
| no `medigo.json` (a bare-PocketBase or pre-006 archive) | `app_version: null`, `compatible: null`, blocker `version_unknown` — the restore is **refused** unless the body carries `"accept_unknown_version": true`, which is recorded |

**Errors**: `404 not_found` for an unknown name; `422 archive_unreadable` when it does not open —
with a reason that **names no storage location** (FR-107, FR-118).

**Audit**: none on success; one `access_denied` on refusal.

---

## 88. `POST /api/v1/admin/backups/{name}/download` — take a copy away

`operationId: downloadBackup`

**`POST`, not `GET` — a deviation from SHARED-DESIGN §2.3 op 88**, recorded in
[plan.md's Deviations](../plan.md#deviations-from-the-shared-design-contract) and reasoned in
[D-27](../research.md#d-27): FR-109 requires password re-entry **and** per-request authorization
**and** no credential in a URL, and a `GET` cannot carry a password without a query string (forbidden)
or a custom header (a browser navigation cannot send one, and the Datastar inline-script SDK family
is banned under the CSP). The page renders an ordinary `<form method="post">`, so the browser
downloads the response with no JavaScript at all.

**Request**: `{ "password": "…" }` — the **administrator's own** password, re-entered.

**Response** `200`, `Content-Type: application/zip`,
`Content-Disposition: attachment; filename="<name>"`, streamed through `fsys.Serve`. It is
**byte-for-byte what was taken** — asserted by a test that hashes the stored object and the response
body.

**Authorization**, every request, in order: authenticated → `role = admin` → password verified →
stream. **Possession of the address grants nothing** (FR-109).

**Errors**: `401` when the password is wrong (and `login_failed` is recorded); `404` for an unknown
name or a non-administrator.

**Audit**: `backup_download` on every call, successful or not — **the most sensitive action the
instance offers**, recorded as such (FR-109, SC-018). `target_kind: backup`, the archive name in
`target_id` (§ `backup_create`).

---

## 89. `POST /api/v1/admin/backups/{name}/restore` — put it back

`operationId: restoreBackup`

**Request**

```json
{ "confirm_phrase": "restore medigo_2026-08-26T02-00-00Z.zip",
  "password": "…",
  "accept_unknown_version": false }
```

**The sequence, in order, stopping at the first failure without touching anything**
([D-22](../research.md#d-22)):

1. `role = admin`; password re-entered and verified; `confirm_phrase` equals the literal
   `restore <name>`. Missing or wrong → `403`/`422`, **nothing replaced** (FR-104, US7 AS-6).
2. `409 archive_operation_in_progress` if `app.Store().Has(core.StoreKeyActiveBackup)` (FR-108).
3. `409 job_in_progress` if any `export_jobs` row is `queued` or `running`, with the message *"an
   export or report is being produced; wait for it to finish or cancel it"* — because a restore
   replaces the storage the worker is writing into ([D-28](../research.md#d-28), FR-108, US7 AS-11).
4. Validate the archive: it exists, it opens, it contains `data.db`, its version is compatible.
   Anything else → `422`, **before anything on the instance is touched**, with a reason naming no
   storage location (FR-107, US7 AS-9).
5. **Take the safety copy** — `app.CreateBackup(ctx, "medigo_safety_<YYYYMMDDHHMMSS>_<name>")`, synchronously,
   and wait for it. **If it fails, the restore does not proceed** and the response says so
   (`safety_backup_failed`). This is not skippable for a recent archive (FR-105, US7 AS-7, SC-017).
6. Write the **restore journal** to `MEDIGO_STATE_DIR` ([D-23](../research.md#d-23)) and re-apply the
   environment snapshot ([D-24](../research.md#d-24)).
7. Respond **`202 Accepted`** with the safety copy's reference and the expected downtime — the
   administrator is told the instance will be briefly unavailable (FR-106).
8. After a one-second delay, `app.RestoreBackup(ctx, name)` in an owned goroutine with a ten-minute
   context. It ends in `app.Restart()` → `execve`, so no response can be written after it; that is
   why step 7 comes first, and it is the same shape `apis/backup.go` uses.

**Atomicity** is PocketBase's: the instance returns either fully restored or unchanged, never partly
(FR-106, US7 AS-8, SC-017). A restart during a restore leaves the journal present with the database
unchanged, which the next boot detects and records as `job_failed` — so the trail says what happened
either way (edge case).

**The account of the restore survives the restore** (FR-111, US7 AS-15). The journal is replayed on
the next `OnBootstrap` into the **restored** database, writing the `backup_create` (safety copy) and
`backup_restore` entries with both references, and is then deleted. This is the sharpest correctness
problem in the phase: a restore destroys the very rows the trail is kept in.

**While the restore runs**, requests are answered `503 restore_in_progress` (FR-106).

**Audit**: `backup_restore`, written **through the journal**, not before the call. The replayed rows
carry the journal's recorded `request_id` — the correlation id of the restore request — because the
boot path that writes them has no request of its own and `request_id` is `Required`; a journal
without one falls back to the boot run's `run_id` ([D-23](../research.md#d-23), 001
[data-model](../../001-walking-skeleton/data-model.md) §3).

---

## 90. `DELETE /api/v1/admin/backups/{name}` — remove one

`operationId: deleteBackup`

**Response** `204`.

**Effect**: `fsys.Delete(key)` plus the sidecar entry. **Only that archive** — other archives, safety
copies and the activity trail are untouched (FR-110, US7 AS-13). The interface confirmation names the
archive.

**Errors**: `404 not_found`; `409 archive_operation_in_progress`.

**Audit**: `backup_delete`, `target_kind: backup`, the archive name in `target_id` (§ `backup_create`).
