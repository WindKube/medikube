# Contract: tags

**Operations added: 4.** Shared design contract §2.3 entries 40–43.

```
GET    /api/v1/tags          listTags
POST   /api/v1/tags          createTag
PATCH  /api/v1/tags/{id}     updateTag
DELETE /api/v1/tags/{id}     deleteTag
```

One `GET` serves list, autocomplete and popularity; upstream needed five operations for this.
Tags belong to the **account**, not to a patient (FR-062).

---

## 1. `GET /api/v1/tags`

| Parameter | Notes |
|---|---|
| `q` | prefix/substring over `name`, case-insensitive — this is the autocomplete (FR-068) |
| `sort` | `name` (default) \| `-usage` (FR-068) |
| `limit`, `cursor`, `count` | standard |

`200`:

```json
{ "items": [ { "id":"…", "name":"cardiology", "color":"#aa3311", "usage_count": 37 } ],
  "next_cursor": null }
```

`usage_count` is **derived** — a count of referencing records across all fourteen kinds for this
owner (FR-068, FR-066). It is never stored, so it cannot go stale (SC-007).

Authorization: the actor's own tags only. **Another account's tags are neither offered nor
discoverable** (FR-062, US7-5) — there is no parameter by which a caller can name an owner, and the
owner filter is applied from the actor, never from the request.

---

## 2. `POST /api/v1/tags`

Body: `{ "name": "cardiology", "color": "#aa3311" }`. `owner` is not in the DTO; it is the actor.

- `201` with `Location`.
- `409 conflict`, `code: duplicate_name`, when a tag with the same name **ignoring letter case**
  already exists for this account (FR-063, US7-2). Enforced by the unique index on
  `(owner, LOWER(name))` *and* checked in the service so the error is MediGo's envelope.
- `422 validation_failed` for an empty name, a name over 40 characters, or a `color` not matching
  `^#[0-9a-fA-F]{6}$`.

---

## 3. `PATCH /api/v1/tags/{id}`

Body: `{ "name": "…"?, "color": "…"? }`. `200` with the tag.

**A rename is one row update.** Every record carrying the tag follows automatically because the
tag is a relation, not a copied string — no record loses it, and the operation is O(1) regardless
of how many records carry it (FR-065, SC-007: 500 records across ≥8 kinds).

`409 conflict` on a case-insensitive duplicate. `404 not_found` for another account's tag —
identical to a non-existent id.

`If-Match` is **not** required here: a tag is not a clinical record and FR-005's concurrency rule is
scoped to records. This is a deliberate, recorded narrowing, not an omission.

---

## 4. `DELETE /api/v1/tags/{id}`

`204`. PocketBase's relation cleanup removes the tag from every referencing record;
**no record is destroyed** (FR-066, US7-4).

The "tell them how many records carry it first" half of FR-066 is served by `usage_count` on the
`GET` the tag manager already holds — no extra operation, no `?dry_run=`. The UI must present that
count in the confirmation; `e2e/specs/tags.spec.ts` asserts it.

`404 not_found` for another account's tag.

---

## 5. Applying tags

There is **no** `/api/v1/records/{kind}/{id}/tags` operation. Tags are the `tags` field on the
record's `Patch` DTO, with replace-set semantics:
`PATCH /api/v1/records/conditions/{id}` with `{"tags": ["tagA","tagB"]}`.
Any number of tags on any record of any kind, including medications from phase 001 (FR-064).
Every id is validated to belong to the actor; a foreign tag id is `404 not_found`.

Narrowing by tag is `?tags=a,b&match=any|all` on every list, on `/api/v1/search` and on the
cross-kind `GET /api/v1/records` (FR-067).

---

## 6. Tests this contract requires

| Test | Requirement |
|---|---|
| `"Cardiology"` after `"cardiology"` → `409 duplicate_name` | FR-063, US7-2 |
| rename → all 500 carriers show the new name, none loses it, one UPDATE statement | FR-065, SC-007 |
| delete → every carrier still exists, none carries the tag | FR-066, SC-007, US7-4 |
| `usage_count` correct across ≥8 kinds and after a carrier is deleted | FR-068 |
| another account's tags absent from `GET`, `404` on `PATCH`/`DELETE`, `404` when used in a record patch | FR-062, US7-5 |
| `?tags=a,b&match=all` returns only records carrying both | FR-067 |
| a tag name never appears in a log line, span, metric label or audit row | FR-085, FR-086, SC-011 |
