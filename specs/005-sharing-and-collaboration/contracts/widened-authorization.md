# Contract: what this phase changes about every endpoint already shipped

This is the half of phase 005 that adds no route. FR-054 to FR-063 require every existing query and
screen to honour shared access as well as ownership. Because phases 001–004 route **every** decision
through `access.Authorizer`, the widening is one change — but its consequences are contractual and
are stated here so the contract tests can be written against them.

## The change, in one place

```
BEFORE                                    AFTER
superuser  → allow                        superuser              → allow
owner      → PermOwn                      owner                  → PermOwn
otherwise  → ErrNotFound                  active grant           → the grant's level
                                          grant, wrong level     → ErrForbidden (+ the Grant)
                                          otherwise              → ErrNotFound
```

*Active* is `revoked_at = '' AND (expires_at = '' OR expires_at > now)` **and** the actor is not
disabled (`users.disabled_at = ''`). Nothing caches a grant, which is what makes FR-037's "effective
on the grantee's next action" true without an invalidation protocol.

## Per-endpoint consequences

| Endpoint (phase) | What changes | Requirement |
|---|---|---|
| `GET /api/v1/patients` (001) | returns **owned ∪ shared** in one page, ordered so the two groups are contiguous; each shared row carries `shared_by` (display name) and `level`; the envelope carries `owned_count` and `shared_count` | FR-055, US1 scenario 5 |
| `GET /api/v1/patients/{id}` (001) | readable at `view`; identical body to the owner's | FR-003, SC-003 |
| `PATCH /api/v1/patients/{id}` (001) | needs `PermOwn`. A grantee at **either** level gets `403 forbidden_owner_only`: a person's name, birth date, sex and photograph are identity, not care | FR-005, US3 scenario 3, SC-007 |
| `DELETE /api/v1/patients/{id}` (001) | `PermOwn`. Before confirming, the UI is told how many accounts currently have access; on delete, every grant of that patient is destroyed by cascade and every grantee's list loses it | FR-005, FR-048, edge case 1 |
| `PUT/GET/DELETE /api/v1/patients/{id}/photo` (001) | `GET` at `view`; `PUT`/`DELETE` need `PermOwn` (the photograph is identity, not care) | FR-005 |
| `GET /api/v1/records`, `GET /api/v1/records/{kind}`, `GET /api/v1/records/{kind}/{id}` (001/003/004) | readable at `view`, byte-identical to the owner's response | FR-003, FR-056, SC-003 |
| `POST/PATCH/DELETE /api/v1/records/{kind}[/{id}]` (001/003/004) | allowed at `edit` under exactly the owner's validation, `If-Match` and confirmation rules; refused at `view` with `403 forbidden_view_only` | FR-004, FR-058, US3 scenarios 1–2 |
| `GET /api/v1/records/{kind}/{id}` by a non-owner | additionally writes a `read_sensitive` audit row — see [Where `read_sensitive` is written](#where-read_sensitive-is-written) | FR-070, [D-25](../research.md#d-25) |
| `GET /api/v1/attachments`, `GET /api/v1/attachments/{id}` (004) | readable and downloadable at `view`; a retrieval of content **or a preview** by a non-owner writes `read_sensitive` — the same one rule below | FR-061, FR-070 |
| `POST/PATCH/DELETE/restore /api/v1/attachments…` (004) | allowed at `edit` under the same recoverable-deletion rules; refused at `view` | FR-061 |
| `DELETE /api/v1/attachments/{id}?purge=true` (004) | **not widened at all**: `404` to a grantee at either level. Early permanent purge is the owner's and the superuser's alone, and phase 004 [FR-066](../../004-labs-and-attachments/spec.md) states that rule once. A grant confers reach, never destruction | 004 FR-066 |
| `GET /api/v1/search` (003) | scoped to the caller's **accessible** patients; a `?patient=` the caller cannot reach is `404`, indistinguishable from a non-existent patient | FR-057, SC-004 |
| the timeline and status views (003) | widened by the same checkpoint; no endpoint change | FR-056 |
| `GET /api/v1/tags` (003) | still returns **only the caller's own** labels. A grantee never sees the owner's vocabulary | FR-059, [D-22](../research.md#d-22) |
| tag ids submitted on a shared record (003) | validated against the **patient owner's** tag set; an id outside it is `422 unknown_tag`, identical for "not yours" and "does not exist" | FR-059 |
| `POST/PATCH/DELETE /api/v1/tags` (003) | unchanged: a caller manages only their own labels; a grantee cannot create, rename or delete one in the owner's set | FR-059 |
| `GET /api/v1/practitioners[/{id}]`, `GET /api/v1/facilities[/{id}]` (002) | unchanged and owner-scoped: **`404` to a grantee**, including for the very practitioner whose name is embedded in a shared record they can read | FR-060, [D-23](../research.md#d-23) |
| `PUT /api/v1/me/active-patient` (001) | authorizes the target through the widened checkpoint, so a shared patient may be selected | FR-054 |
| active-patient resolution (002) | resolves to **null** when the grant ends, and a page request lands on `/patients`, never an error | FR-046, US2 scenario 3 |
| `GET /api/v1/streams/records` (001) | subscribes to the subscriber's user topic as well; cuts and explains on revoke | FR-045, [streams-notifications.md](./streams-notifications.md) |
| `DELETE /api/v1/me` (001) | destroys every grant given **and** received by that account, by cascade | FR-048, edge cases 2–3 |
| every list endpoint over patient data | **must not** disclose any other patient's data while serving a grantee — asserted by a cross-contamination test per endpoint | FR-056, SC-003 |

## Where `read_sensitive` is written

**This is MediGo's only statement of the rule**, and it governs every phase: opening an individual
record (001, 003, 004), retrieving a document's content or preview (004), and fetching a patient's
photograph (002). Those phases reference it rather than restating it.

> A `read_sensitive` audit entry is written when, and only when, **the grant the authorizer resolved
> is something other than the reader's own ownership** — that is, access through a share, or a
> superuser reading somebody else's data. An owner reading their own record or retrieving their own
> document writes **no** entry at all.

| Reader | What they opened | `read_sensitive`? |
|---|---|---|
| the owner | their own record, their own document, their own preview | **no row** |
| a grantee at `view` or `edit` | a record they can reach through the grant | one row per record opened |
| a grantee at `view` or `edit` | a document's content or a preview | one row per retrieval |
| a superuser | somebody else's record, document or preview | one row |
| a superuser | a patient they themselves own | **no row** |
| anybody | paging a list, at any size | **no row** (list paging is never a sensitive read — FR-070) |
| anybody refused | anything | no `read_sensitive`; an `access_denied` row instead, unconditionally |

A superuser session additionally produces its own `admin_session` entry (Constitution VII). That
records the *session*; the `read_sensitive` rows record the individual reads within it. Both exist,
and neither replaces the other.

**Out of scope of this rule, and unchanged:** phase 003's `read_sensitive` / `target_kind: search`
row ([003 D-12](../../003-clinical-records/research.md)), which records *that a search happened*
without its term. It is not a read of a named record and the ownership condition does not apply to
it.

**Why owner reads are not recorded.** The trail exists for accountability about who reached data
they do not own. Recording every time a person opens their own lab report produces unusable noise,
and — worse — builds a detailed timeline of when somebody read their own most sensitive results,
which is itself a privacy exposure under Principle VII. It is the same asymmetry phase 006 FR-075
already applies to the trail itself: reading it is not an auditable event, exporting it is.

**Implementation.** `internal/web/api/records.go` and `internal/web/api/attachments.go` call
`audit.Record` on the resolved `Grant`, when `Grant.Level != PermOwn`. The ownership outcome comes
from the authorizer's result and is never re-derived from the request. Phase 004's
`internal/service/attachment/serve.go` is amended to the same condition (004 T105); its
`contracts/attachments.md` §3.6 points here.

---

## The one refusal that is not `404`

`403 forbidden_view_only` — a `view` grantee writing to a resource they can see (FR-058, SC-006).
Its sibling `403 forbidden_owner_only` covers the owner-reserved operations a grantee at **any**
level attempts: deleting the patient, changing identity fields, changing anybody's access, sharing
onward (FR-005, FR-006, SC-007). Both are safe for the same reason: the caller already knows the
resource exists, because they are entitled to see it.

Everything else stays `404`.

## The tests this section obliges

1. **The six-actor matrix on every patient-touching route** (README.md), driven by a table in
   `internal/web/api/authz_matrix_test.go` that is **derived from the route registry**, not
   hand-written. `internal/service/access/coverage_test.go` fails the build when a registry route
   marked `PatientScoped: true` has no matrix entry — this is FR-077 as a gate rather than a habit.
2. **Family-history isolation** (`internal/web/api/familyshare_isolation_test.go`, FR-078, SC-008):
   from a `family_member` grantee's session, every one of these is `404` —
   the patient the entry is filed against; that patient's records of **every registered kind**
   (iterated from the kind registry, so a future kind cannot be forgotten); that patient's other
   relatives; that patient's attachments; a search naming that patient; the timeline; the status
   views; the owner's practitioners and facilities. Exactly **one** relative's entry is reachable
   per grant.
3. **Cross-contamination**: a grantee holding a grant on patient A while the owner also owns patient
   B receives **zero** rows about B from any list, search, timeline or stream.
4. **Revocation immediacy** (FR-042, SC-005): with a live session token issued **before** the
   revoke, the very next request on every route above is `404` — no sign-out, no cron, no cache
   flush.
5. **Lapse immediacy** (FR-043): the same, with an injected clock advanced past `expires_at` and no
   tidy pass run.
6. **Nothing is destroyed by revocation** (FR-047): after a revoke, every record, correction,
   attachment and relative the former grantee created is present and unchanged, and is still
   attributed to them in the audit trail.
