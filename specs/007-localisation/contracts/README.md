# Contracts: Localisation (phase 007)

This phase adds **zero HTTP operations**. `PATCH /api/v1/me` (`OpID: updateMe`, already
registered by phase 001, `internal/httproute/routes.go:254,260-264`) gains no new field — `locale`
already exists in its DTO since phase 001 — and gains one new failure mode: a `locale` the shape
check accepts but no catalogue file ships now refuses.

## Files

| File | Covers |
|---|---|
| [catalogue.md](./catalogue.md) | the phrase catalogue contract — file naming, TOML shape, id rules, per-language plural-form contract, fallback rule, what never appears in the catalogue, the three build-time invariants |

## What changes on existing contracts

- **`PATCH /api/v1/me`** (`contracts` inherited from `001-walking-skeleton`): `locale` accepted
  values narrow from "matches `^[a-z]{2}(-[A-Za-z]{2})?$`" to "matches that pattern **and** names
  a shipped language" (research D-10). The refusal is the existing `422 validation_failed`
  envelope, field `locale`, no new error code.
- **Every JSON error envelope** (`contracts/README.md` of `003-clinical-records`, §"Error
  envelope"): `Failure.Message` now follows the caller's resolved language (research D-04); every
  other field of the envelope — `Code`, `Fields[].Field`, `Fields[].Code`, `basis`, `criteria`,
  and all vocabulary wire values across every operation in every prior phase — is unchanged and
  byte-identical between an English and a Polish caller (spec SC-005). No operation's schema
  changes; this is a runtime behaviour of a field whose shape was already free-text.
- **Every page route** (`contracts/pages.md` of `003-clinical-records`): the rendered document
  now declares `<html lang="...">` correctly (it did not before this phase — `plan.md`
  "Accessibility — PASS, strengthened") and, when the account's language is Polish, contains no
  application-owned English text (spec SC-001). No route, landmark or smoke URL changes.

## Conventions unchanged from phase 003

Base path `/api/v1`, JSON in/out, the cursor-pagination shape, `ETag`/`If-Match` on clinical
records, the ten-field error envelope — none of it is touched here. See
`specs/003-clinical-records/contracts/README.md` for the full list; this phase inherits it
without amendment.
