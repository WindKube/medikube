# Spec defects

Contradictions and stale figures found while implementing, recorded rather than
silently resolved. Each entry names the authority that was followed and why.

The general rule this project applies: **`data-model.md` outranks `tasks.md` for
schema and vocabulary**, because tasks.md is a derived work plan and drifts when
the model is revised. `plan.md` outranks both for versions and package layout.

## D1 — T049 undercounts the audit actions

`tasks.md` T049 asks for "`Action` (all ten values including `access_denied`)".
`data-model.md` §3 declares **twenty** actions, and T071 in the same file says
"**twenty** actions" three sections later. tasks.md contradicts itself.

*Followed:* data-model.md. `internal/domain/audit/enums.go` ships twenty.
*Fix:* correct T049 to twenty.

## D2 — Length arithmetic predates the MediKube rename

`medigo_` is 7 characters; `medikube_` is 9. Every proof that sized a field by
counting a prefixed identifier is off by two per occurrence. `audit_events.target_id`
is specified at `Max 64` and phase 006 writes ~40-character archive names into it,
so the margin was never large.

*Action:* re-derive any length proof before relying on it; do not trust a figure
that was written before the rename.

## D3 — T030 and T043 disagree about where redaction lives

T030 implements `internal/logging/redact.go` as "the shared redaction helpers the
domain packages use", but T043 forbids `internal/domain/**` from importing
anything outside stdlib and zerolog — and `internal/logging` is neither. As
written the domain cannot reach the helper.

*Followed:* the boundary. Domain-side redaction is expressed through
`MarshalZerologObject` on the domain types themselves; `internal/logging/redact.go`
serves the logging side only.
*Fix:* reword T030.

## D4 — T135 and data-model disagree on future start dates

T135 and `data-model.md` §2 state different rules for whether a medication may
start in the future. Unresolved at the time of writing; flagged for the phase that
implements it.

## D5 — Cursor key rotates with `AuthRule`

Not a spec defect but an under-documented consequence. CT-3 derives the cursor key
from PocketBase's persisted auth-token secret. That secret is **per collection**
(`core.TokenConfig.Secret`, reached as `collection.AuthToken.Secret`) and
PocketBase rotates it whenever the collection's `AuthRule` changes
(`core/collection_model.go:864`). Every outstanding cursor therefore dies on an
`AuthRule` change.

This is acceptable, but only because the same change already invalidates every
session — users are logged out regardless, so a dead cursor is strictly the
smaller disruption. Worth knowing before someone treats cursor stability as
unconditional.
