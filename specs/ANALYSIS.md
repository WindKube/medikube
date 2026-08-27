# MediGo specification suite — final readiness report

**Run**: 2026-08-27, closing pass. Supersedes every earlier revision of this file.
**Scope**: `specs/001`–`specs/006`, `.specify/memory/constitution.md` v1.3.0, the cross-phase design
contract [`SHARED-DESIGN.md`](./SHARED-DESIGN.md), and [`VERIFIED-SOURCE-FACTS.md`](./VERIFIED-SOURCE-FACTS.md).
**Method**: adversarial re-verification. Nothing carried forward on trust. Every figure below was
produced by a command against the files in this repository, including the figures the previous two
rounds published. Where a repair was claimed, the repair was re-derived from the file rather than
read back from the claim.

---

## Verdict

**READY TO IMPLEMENT. Zero blocking findings.**

Both prior rounds' findings — 23 in the first, 22 in the second — are closed. The five
runtime-fatal defects repaired by hand before this pass all hold at source, and I could not break
any of them from downstream. This pass raised four residual defects, N1–N4; **all four have since
been repaired by hand and verified clean** (see the closure notes under each). No finding of any
severity is open against this suite.

One question **is** open, and it came from outside this analysis: importing the visual design on
2026-08-27 surfaced that **medication reminders belong to no phase.** Phase 001 defers them to
"a later phase", no later phase claims them, and 005 and 006 both rule out the delivery mechanism
they would need. It is the same shape as the orphaned password-reset flow round one caught. It is
a product decision rather than an editorial one, so it is recorded in
[`DESIGN.md`](./DESIGN.md) §5.4 and left for a human. It does not block phase 001; it blocks
building the dashboard as drawn, because two of its four stat cards and its main panel have no
data behind them.

In plain language: **an implementer can open `specs/001-walking-skeleton/tasks.md` at T001 and
build.** Phase 001 is internally consistent, its audit schema now accepts every value any of the six
phases will write into it, its shell contract no longer contradicts its own spec, and the three
audit indexes it creates are sized on day one for the reader phase 006 builds on them. The task list
is sound in every mechanical respect I can check: 1332 tasks, zero duplicate ids, one documented
numbering hole, zero dangling dependencies, zero forward dependencies, and tests preceding
implementation in all 42 user stories.

The two that mattered were both in phase 006 and both are now fixed. **N1** — a propagation gap
where four sites still instructed an implementer to write the audit-index migration that
`006/data-model.md` §4.3 forbids in bold, which would have failed `CREATE INDEX` at first boot —
is closed: the migration entry, its plan tree line and its `highest_applied` sample are gone, T034
is now an `EXPLAIN QUERY PLAN` assertion with no migration, and D-52 cites the four indexes that
actually exist. **N2** — the restore safety copy computing to 66 characters against a `Max 64`
column — is closed by making every archive timestamp compact rather than RFC3339 (the longest
composed name is now 54) and by bounding op 86's uploaded key to 64. A colon is not a legal
filename character on Windows or a clean S3 key either, so the compact form was the right call
regardless of the arithmetic.

---

## Per-phase inventory

| Phase | User stories | FRs | SCs | Acceptance scenarios | Tasks | Test tasks | New ops | Page routes | New collections |
|---|---:|---:|---:|---:|---:|---:|---:|---|---:|
| 001 walking-skeleton | 6 | 77 | 16 | 54 | 345 | 189 | 22 | 9 **+ 3 error views** | 3 |
| 002 patient-core | 6 | 56 | 14 | 38 | 172 | 80 | 20 | 6 | 3 |
| 003 clinical-records | 10 | 94 | 18 | 53 | 222 | 122 | 8 | 29 | 16 |
| 004 labs-and-attachments | 5 | 85 | 16 | 56 | 188 | 94 | 9 | 4 **+ 4 page-actions** | 4 |
| 005 sharing-and-collaboration | 6 | 80 | 20 | 47 | 144 | 71 | 10 | 3 | 2 |
| 006 reporting-and-operations | 9 | 137 | 28 | 122 | 261 | 162 | 25 | 7 **+ 3 page-actions** | 2 |
| **Total** | **42** | **529** | **112** | **370** | **1332** | **718** | **94** | **58 + 3 + 7** | **30** |

Test-task figures use the one reproducible rule `006/tasks.md:617` states and every phase can be
measured by — a task line is a test task when it names a `_test.go` or a `.spec.ts` file, is marked
`TEST`, or instructs a failing test to be written:

```
grep -E '^- \[ \] T[0-9]+[a-z]?' tasks.md | grep -cE '_test\.go|\.spec\.ts|ailing|TEST '
```

`006`'s own published figure of **162** reproduces exactly under its own narrower rule. `001`
correctly withdrew its unreproducible figure rather than re-guess it (`001/tasks.md:1300-1302`).
`003`'s hand tally of 113 is a per-block count, not this regex; both are defensible and the block
table sums to 222.

**Task ids.** 1332 unchecked task lines; **zero** duplicates in any phase; **one** numbering hole
suite-wide — `004/tasks.md:98` `~~T037~~ **Deliberately vacant.**`, documented as such. 42 suffixed
ids (001: 20, 002: 2, 003: 5, 004: 0, 005: 3, 006: 12). Numbered ranges are gapless: 001 T001–T325,
002 T001–T170, 003 T001–T217, 004 T001–T189 less T037, 005 T001–T141, 006 T001–T249.

---

## Authoritative roll-ups

Computed once in `SHARED-DESIGN` §§1.6 / 2.3 / 3.1. **Cite these; do not re-derive them.** Every
figure below was independently re-derived here from the phase data models, contract tables and page
inventories, and agrees.

| Roll-up | Per phase | Running | Total |
|---|---|---|---:|
| Collections | 3 / 3 / 16 / 4 / 2 / 2 | 3 / 6 / 22 / 26 / 28 / 30 | **30** |
| `/api/v1` operations | 22 / 20 / 8 / 9 / 10 / 25 | 22 / 42 / 50 / 59 / 69 / 94 | **94** |
| Page routes | 9 / 6 / 29 / 4 / 3 / 7 | 9 / 15 / 44 / 48 / 51 / 58 | **58** |
| Error views | 001 only | — | **3** |
| Page-action routes | 004: 4, 006: 3 | — | **7** |
| Registered record kinds | 001: 1, 003: 13, 004: 1 | — | **15** |
| Audit actions | 20 / 21 / 21 / 21 / 26 / 36 | — | **36** |
| Audit target kinds | 23 / 25 / 27 / 27 / 27 / 28 | — | **28** |

Operation numbers are stable identities, not positions. Machine-parsing §2.3's six tables with range
expansion yields 94 operations, **zero duplicates**, and exactly one number in 1–95 unallocated:
**45** (`catalog/vaccines`), which §2.3's Dropped table records. Per-phase parsed counts match every
table's own claimed count exactly.

---

## Invariants checked, with evidence

### The five hand-repaired runtime-fatal defects

| # | Repair | Holds | Evidence, and what would have contradicted it |
|---|---|---|---|
| R1 | `audit_events.target_id` widened `≤15` → `≤64`, with a bounded "never a name" exception | **yes** | `001/data-model.md:267` reads `≤64` and states the exception for `target_kind ∈ {system, backup, export}` with the reason. `001/tasks.md:258` (T070a) asserts `Max 64` and `:264` (T071) creates it at `Max 64`. **`grep -rn '≤15\|Max 15\|15 characters' specs/` returns zero** — no surviving `≤15` anywhere. Every job name written (`medigo_purge_artifacts` 22, `medigo_purge_audit` 18, `medigo_storage_refresh` 22, `medigo_attachment_maintenance` 29) fits. `006/data-model.md:294-300` restates 001's rule rather than re-deriving it. **One composition still overflows — non-blocking N2.** |
| R2 | `request_id` stays `Required`; background runs mint a `run_id` from the same helper | **yes** | `001/data-model.md:268` states it with the failure it prevents. Propagated to `001/plan.md:412`, T070a (`Max 64` **and `Required`**), T071, T231 (the bare-`context.Background()` case), T240 (`Record` resolves from run id when there is no request). **Every background audit writer in the suite is covered**: 001's retention purge (`data-model.md:268`, T231/T240), 002's backfill (T074), 004's attachment purge and orphan sweep (T093, T107), 005's tidy pass (`005/data-model.md:227-231`, T111, T116), 006's job envelope (T052, T053), scheduled archive (T183), boot-time journal replay (T191, `admin-backups.md:225-229`) and audit purge (T219). 003 has no background audit writer (`grep -i cron specs/003-clinical-records/` → one rejected-alternative line). **Zero audit write sites left with neither an HTTP request nor a run id.** |
| R3 | Signed-out pages **do** carry `navigation[name="Primary"]` | **yes** | `001/contracts/pages.md:38-46` now reads *"Signed-out pages render the same shell, navigation landmark included… on **every** page in the application; what changes signed out is its contents"*, and names `/invite/{token}` as the reason it cannot be conditional. The smoke assertion at `:131` reads *"the four shell landmarks are present, **signed in or out**"*. That now agrees with `001/spec.md:274` (FR-043), `001/plan.md:466`, `001/tasks.md:920` (T249), `contracts/pages.md:120` (all three error views in the full shell) and `005/contracts/pages.md:26`. No "three, signed out" survives. |
| R4 | `#notice-region` is dead at all seven 005 sites; 005 makes no `layout.templ` edit for it | **yes** | `grep -rn 'notice-region' specs/` returns **one** line — `005/contracts/pages.md:47`, explaining why it is *not* added. `005/plan.md:309` and `:512` (`[UNCHANGED]`), `005/tasks.md:316` (T124), `:322` (T127), `:339` (T136) and `005/contracts/streams-notifications.md:24` all now name `#toast`. `#live-region` returns zero hits. **But §2's blanket "None" overshot — non-blocking N3.** |
| R5 | Phase 006 creates no audit index; 001's and 002's are pre-widened with the tiebreakers | **yes at source** | `001/data-model.md:336-348` creates `idx_audit_occurred (occurred_at DESC, id DESC)`, `idx_audit_actor_time (actor, occurred_at DESC, id DESC)` and `idx_audit_target (target_kind, target_id, occurred_at DESC)` and states *"phase 006 creates no audit index at all"* with the name-collision reason. `002/data-model.md:231` adds `idx_audit_patient_time` with `id DESC` for the same reason. `006/data-model.md:311-326` §4.3 reads *"**This phase creates no audit index.**"* and names the earlier draft's three redundant b-trees and the `idx_audit_target` collision. `001/tasks.md:264` (T071) builds the three *"carrying phase 006's tiebreaker columns so 006 creates no index of its own and none collides by name"*. **Four sites in 006 did not get the memo — non-blocking N1.** |

### The runtime-fatal invariants, rebuilt from scratch

| # | Invariant | Holds | Evidence |
|---|---|---|---|
| 1 | Every `action` and `target_kind` a phase writes is declared by a migration **by that phase** | **yes** | Vocabulary chain rebuilt from the six data models: actions 20 → 21 (`switch_patient`, 002 T028) → 21 → 21 → 26 (005's five: `share_update`, `share_leave`, `invite_cancel`, `invite_withdraw`, `invite_expire`) → 36 (006's ten). Target kinds 23 → 25 (`practitioner`, `facility`) → 27 (`tag`, `search`) → 27 → 27 → 28 (`report_template`). Machine-checked: every action token used in a phase's documents is in that phase's declared set. The one apparent violation — `` `purge` `` in `001/research.md:405` — is the CLI-command sense in a paragraph titled *"Why no `purge` command"*. Six complete-set assertion tasks (T070a, T023a, T032a, T036, T019a, T032a) assert set equality, never a delta. |
| 2 | Every field a DTO, CSV or content rule publishes exists as a real column | **yes** | `audit_events` columns: `occurred_at`, `actor`, `actor_kind`, `action`, `target_kind`, `target_id`, `request_id` (001) + `patient` (002) + `reason` (005) + `affected` (006). `006/contracts/audit.md:14-27`'s `AuditEntry` publishes exactly those eleven, no more. The one CSV header in the suite — `006/contracts/audit.md:128` — is column-for-column the same list. No `ip` field, no content field. `reason`'s longest bounded token across 005 and 006 is `archive_version_unsupported` (27 ≤ 40). |
| 3 | Every Playwright selector asserted is produced by an element some phase builds | **yes** | 68 distinct ARIA-role landmark literals across `specs/`; 66 appear in a phase `contracts/pages.md`. The two that do not — `region[name="Labels"]` and `article[name="Appointment"]` — occur **only** inside `003/plan.md:449`, the deviation row that declares them dead names. Zero live assertions on an unbuilt element. |
| 4 | Every route linked to is registered by some phase | **yes** | Every backticked path across all six phase directories resolved against §2.3's 94 operations, the phase page tables and the page-action routes. Residuals are file paths (`/.dockerignore`, `/specs/…`), PocketBase-native endpoints under `/api/collections/`, upstream-endpoint narratives, prose path fragments (`/photo`, `/autocomplete`), rejected alternatives (`/api/v1/streams/jobs`, D-46) or paths a sentence says are **not** built (`/medications/new`, `/patients/{id}/new`). 006's three page-action routes — `/reports/selection`, `/reports/jobs`, `/exports/jobs` — match its declared count of 3 exactly. |
| 5 | Every `CREATE INDEX` name is unique across the suite | **yes, as written in the data models** | 44 distinct index names parsed from all six `data-model.md` files. The only repeated strings are `idx_patients_self` and `idx_practitioners_owner_name_specialty`, each appearing twice **within phase 002** (research + data model, identical definition). Zero cross-phase collisions. This holds *because* 006 creates none — see N1 for the four sites that would break it. |

### Traceability, both directions

| # | Check | Result |
|---|---|---|
| 6 | FR ids in `spec.md` vs cited in `tasks.md`, per phase | **100% both ways, all six phases.** `comm` on sorted unique ids: both differences empty for 001 (77), 002 (56), 003 (94), 004 (85), 005 (80), 006 (137). No document cites an FR its phase does not define. |
| 7 | Every SC either cited by a task or marked `[outcome metric]` | **complete.** Uncited SCs: 001 {SC-001}, 002 {SC-001}, 003 {SC-001}, 004 {SC-001}, 005 {SC-001, SC-002}, 006 {SC-001}. Every one of those seven carries `[outcome metric]` in its own spec, and no other SC does. The nine-uncited-SC gap of the previous round is closed: `004` SC-007 and SC-012 and `006` SC-003, SC-015, SC-026, SC-027 are now cited by real tasks. |
| 8 | **Semantic** discharge — the cited task actually does the work | **16 requirements read in full across all six phases; all 16 genuinely discharge.** 001: FR-003 → T191/T192 (case-insensitive unique index + the enumeration-safe duplicate response), FR-037 → T232/T234 (immutability through every path, configurable retention), FR-054 → T240/T244 (the correlation id on every line and on the background row), FR-067 → T268 (the smoke list shells out to `medigo routes --json`). 002: FR-004 → T045/T049/T059 (partial unique index refuses a second self-record even under a direct write), FR-029 → T109 (kind/action/time and `target_exists: false`, no name or value). 003: FR-073 → T165 (`occurred_on DESC, id DESC`, nulls last, no ranking claim), FR-059 → T136/T142 (both ends of the relation, kind + summary + openable link). 004: FR-074 → T005/T095 (a `forbidigo` gate on `NewFileToken` plus the three-way authorization matrix), FR-077 → T093/T096. 005: FR-033 → T111 (idempotent tidy, terminal invitations removed while their audit rows survive), FR-023 → T036/T050/T104 (the preview DTO has no field for a name). 006: FR-101 → T183, FR-114 → T061/T165 (a reflection test that fails the build if a free-text field is added). **The FR-035 defect class is fixed at its source**: `006/tasks.md:249` (T114) now names its own former miscitation in the task text, and FR-035 is discharged by T118a, T118b and T122a, which actually put `data/report_templates.json` into the archive. SC-007 is discharged by the new T114a, both halves. |
| 9 | Acceptance-scenario citations | **zero overflows suite-wide.** Every `USn AS-m` citation in all six task lists checked against that story's own scenario count (001: 9/12/8/8/9/8; 002: 8/6/6/6/6/6; 003: 7/5/6/5/5/6/5/5/5/4; 004: 13/16/12/8/7; 005: 9/8/7/7/10/6; 006: 15/15/13/12/16/13/15/11/12). The previous round's `US6 AS-14` dangler is gone, and 006's scrambled US3 block is corrected — T110→AS-10, T112→AS-6/7/9, T113→AS-12, T114→AS-3, T114a→AS-4/AS-5. |
| 10 | Task-id integrity | **clean.** Zero dangling `depends on` targets and zero forward dependencies in any phase. |
| 11 | Section cross-references | **clean.** Every `<file>.md §N` and `SHARED-DESIGN §N` reference in the suite machine-resolves to a heading that exists. The previous round's one failure (`001/contracts/records.md:249` → `pages.md` §4) is repaired — it now names the section by title. |

### Constitution v1.3.0 — all nine principles

| Principle | Result |
|---|---|
| I Simplicity is a gate | Every phase carries a Complexity Tracking table naming what it added and why. 001 declares three (the log bridge, a one-kind registry, the cursor key derivation) with mitigation tasks; 003–006 each justify their own. The `job_runs` collection is rejected on record (`006/research.md:667`) rather than quietly not built. |
| II Interfaces at every seam | Consumer-declared ports in every phase (`006/tasks.md:104` T037 declares eleven, none an omnibus interface). The two deliberate conflicts with the five-method segregation cap (001's identity port, 002's) are resolved *in the plan*, in writing, as Principle I requires. |
| III Test-first (NON-NEGOTIABLE) | **Holds in all 42 user stories.** Every `### Implementation for User Story N` heading is preceded by a `### Tests for User Story N` heading with a lower line number, in every phase. **Every suffixed id checked individually**: 001 T221a(TEST)→T221b(impl), T223a–T223i(TEST)→T223j–T223p(impl) inside a block headed *"⚠️ tests first"*, T070a→T071, T202a is a non-gating benchmark; 002 T023a→T028 (*"turns T023a green"*); 003 T032a→T032 (*"depends on T032a"*, not `[P]`, excluded from the parallel list); 004 none; 005 T019a→impl; 006 T032a→T033, T114a/T118a/T118b→T122a, T163a/T163b→T163c, T145a/T150a/T174a all tests, T248a the traceability gate. Zero `Implement …` lines sit inside a tests block. |
| IV Idiomatic Go | `samber/lo` pinned at v1.53.0 and marked *"sparingly, per Principle IV"* in every plan that uses it. |
| V PocketBase is the platform | Backups wrap `app.CreateBackup` / `NewBackupsFilesystem()` / `core.StoreKeyActiveBackup`; recovery wraps `mails.SendRecordPasswordReset` and `FindAuthRecordByToken`; scheduled archives use `Settings().Backups.Cron` and PocketBase's own max-keep pruning; OAuth2 is one DTO wrapper over `_externalAuths`. Nothing is reimplemented. |
| VI One log stream, one trace context | One correlation id per request **and per background run**, on the zerolog line and on the audit row (R2). OTel is present but inert — no exporter, no span, no outbound connection (001 T229). |
| VII Privacy is structural | `audit_events` has no content column in any phase and no `ip` column anywhere. Every surviving `ip` string in the suite is a prohibition or a rationale for one. `internal/testsupport/phileak` drives every endpoint against a sentinel-seeded instance (001 T235), extended by every later phase. Search logs `term_len`/`result_count`, never `term`. `login_failed` never carries the attempted address. |
| VIII The UI must prove it renders | Seven smoke assertions per page per viewport, both viewports, including the non-empty-landmark clause and zero console errors. Two negative controls prove the gate goes red. The one wall-clock assertion that had no definable tolerance was moved out of the gate into a non-blocking Go benchmark (`001/contracts/pages.md:106-116`, T202a) — the correct resolution under *"a flaky assertion is fixed or removed, never retried into passing"*. |
| IX Compliance is a build gate | Every migration in every phase carries a reversibility note; `006/data-model.md` §6 documents the two `down`s that lose data. Each phase's Phase Exit Criterion 0 is its `traceability.md` join (T217a, T140a, T248a and their siblings). |

### Forbidden dependencies and specification hygiene

| Check | Result |
|---|---|
| Gin, Huma, Viper, samber/mo, samber/ro, samber/slog-zerolog, React, HTMX, Alpine, Node in the runtime, jsvm, cgo, `datastar.WithCompression` | **Zero admissions.** 17 patterns grepped across `specs/` and `.specify/memory/`. Every hit is a prohibition, a rationale for one, or ordinary English. `cgo` appears 26 times, always as `CGO_ENABLED=0`, "no cgo", "pure Go, cgo-free" or a rejected alternative. Node appears only as build-time Playwright, explicitly *"never in the runtime image"*. `datastar.WithCompression` appears six times, always as "is not used". |
| Implementation leak in any `spec.md` | **Zero.** Case-sensitive token-bounded grep for PocketBase, `templ`, Datastar, zerolog, Sentry, Prometheus, OTel, Tailwind, Playwright, testify, cobra, SQLite, `Go 1.`, SSE, JSON, HTTP, `/api/`, `.go`, `internal/`, `SQL`, `DTO`, `CSS`, `HTML`, the five HTTP verbs and `migration` across all six specs returns **one** line: `001/spec.md:348`, *"Nothing here is a migration of an existing…"* — ordinary English. All six pass. |
| `NEEDS CLARIFICATION` | Zero open markers. Every occurrence is a research or plan file declaring that none survives. |
| Checklists | 96 boxes across six `checklists/requirements.md`, all ticked, all **true** — verified item by item against the specs they describe. |
| Constitution version | `001`–`006` `plan.md:7` all read *"v1.3.0 (binding)"*. No v1.2.0 reference survives anywhere. |

---

## Blocking findings

**None.**

A finding is blocking here only if implementing phase 001 as written produces something broken, or
if an implementer cannot proceed without a decision only a human can make. Neither is true of
anything in this suite. Phase 001 is internally consistent on every point the last two rounds
raised against it, and the four residuals below each have an authoritative resolution already
written down in the same phase.

---

## Non-blocking findings — all four repaired 2026-08-27

Recorded in full below as the record of what was wrong and why, **not** as open work. Each carries
a closure note. Verified clean by grep after repair: no `_audit_page_indexes` instruction survives,
no `<rfc3339>` archive name survives, no "adds no shell element" claim survives, and the last two
stale `H2` citations are now `H1`.

### High

**N1 — Phase 006 still carries the audit-index migration that `006/data-model.md` §4.3 forbids, in

> **CLOSED 2026-08-27.** `006/data-model.md` §6 now records the migration as *not created* with the reason; `006/tasks.md` T034 is an `EXPLAIN QUERY PLAN` assertion task that states there is no migration; `006/plan.md`'s tree line is deleted; `admin-instance.md`'s sample `highest_applied` is `1757xxx400_audit_vocab_ops`; D-52 cites `idx_audit_occurred`, created by 001. Verified: zero `_audit_page_indexes` instructions remain.

four live instruction sites. It would fail the migration outright.**

`006/data-model.md:311` states, in bold: *"**This phase creates no audit index.**"* and explains
that the earlier draft's `idx_audit_page` / `idx_audit_patient` / `idx_audit_actor` were three
redundant b-trees, and that re-creating `idx_audit_target` under a name 001 already holds *"fails
outright"*. `001/data-model.md:340-348` says the same from the other side. The repair did not reach:

| Site | Text |
|---|---|
| `006/data-model.md:385-386` | `### 1757xxx500_audit_page_indexes.go` — *"Adds the four indexes of §4.3. `down` drops them. **Reversible.**"* |
| `006/tasks.md:100` (T034) | *"Write failing tests then **implement** `internal/store/migrations/1757xxx500_audit_page_indexes.go` — the four indexes of `data-model.md` §4.3"* |
| `006/plan.md:548` | `│   ├── migrations/1757xxx500_audit_page_indexes.go            [NEW]` |
| `006/contracts/admin-instance.md:96` | `"migrations": { "state": "up_to_date", "highest_applied": "1757xxx500_audit_page_indexes" }` |

Plus `006/research.md:1795` (D-52), which still gives its rationale as *"backed by `CREATE INDEX
idx_audit_page ON audit_events (occurred_at DESC, id DESC)`"* — an index no phase creates.

**Consequence if followed**: the migration re-creates `idx_audit_occurred`, `idx_audit_actor_time`,
`idx_audit_patient_time` and `idx_audit_target` under names 001 and 002 already hold. SQLite refuses
the first `CREATE INDEX`, the migration fails, and the instance will not boot. Worse, its `down`
would drop three indexes it does not own, leaving 001's and 002's readers unindexed.

**Why it is not blocking**: T034's own text points at §4.3, and §4.3 says not to do it, with the
reason. Two authoritative documents in two different phases settle it identically. No human decision
exists here.

*Fix (mechanical)*: T034 becomes *"Write failing `EXPLAIN QUERY PLAN` tests in
`internal/store/audit/reader_test.go` asserting each narrowing uses one of the four indexes of
`data-model.md` §4.3 — this phase adds no migration"*; delete `data-model.md` §6's
`1757xxx500_audit_page_indexes.go` entry and `plan.md:548`'s `[NEW]` line; change
`admin-instance.md:96`'s sample to `1757xxx400_audit_vocab_ops`; and rewrite D-52's index sentence
to cite the four that exist. `006/tasks.md:120` (T048) already reads correctly and needs no change.

**N2 — Two archive names can exceed the `≤64` `target_id` that the widening repair sized, and one of

> **CLOSED 2026-08-27.** Every archive timestamp is compact `<YYYYMMDDHHMMSS>`, never RFC3339 — the longest composed name, `medigo_safety_20260827120000_medigo_20260827120000.zip`, is **54**. Op 86 now normalises and bounds the uploaded storage key to 64 before storing, since that key is what `backup_upload` writes into `target_id`. `001/data-model.md:267` now shows the arithmetic that sizes the column rather than asserting "~40". Verified: zero `<rfc3339>` archive names remain.

them is not bounded at all.**

`001/data-model.md:267` sized the column at 64 on the stated basis that *"a PocketBase record id is
15 and an archive name is ~40"*, exemplified as `pb_backup_medigo_20260827120000.zip` (35
characters, compact timestamp). Two compositions in phase 006 break that budget:

1. **The safety copy.** `006/contracts/admin-backups.md:203` — `app.CreateBackup(ctx,
   "medigo_safety_<rfc3339>_<name>")`, where `<name>` is the archive being restored, itself
   `medigo_<rfc3339>.zip` per `:73`. Spelling `<rfc3339>` literally (`2026-08-27T12:00:00Z`, 20
   chars) gives `14 + 20 + 1 + 31 =` **66 characters**. That name is written into `target_id` by the
   journal-replayed `backup_create` row (`admin-backups.md:220`, research D-23, T191) — the row that
   exists specifically to survive the restore. With the compact timestamp 001 exemplifies it is 54
   and fits; the suite never says which spelling wins.
2. **An uploaded archive.** `006/research.md:931` states plainly that *"an uploaded archive can be
   named anything"*, and op 86's validation (`admin-backups.md:100-102`) bounds only zip-ness,
   `data.db` and byte size — never the name. `backup_upload` then writes that name into `target_id`
   (`:111`), and a later restore of it composes a safety name from it.

**Consequence**: a `TextField` with `Max: 64` rejects the write. The restore audit trail is the one
place in the product where losing the row also loses the account of what happened.

**Why it is not blocking**: phase 001 is unaffected — it writes ids and empty strings — and the fix
is an engineering choice the documents already lean toward, not a product decision.

*Fix*: state the archive timestamp format once, compactly (`medigo_YYYYMMDDHHMMSS.zip`,
`medigo_safety_YYYYMMDDHHMMSS_<name>`), and bound or normalise the upload key in op 86's validation
list so `<name>` has a stated maximum. Then add the arithmetic to `001/data-model.md:267`'s sizing
sentence so the next reader can check it.

### Medium

**N3 — `005/contracts/pages.md` §2's blanket "None" is false: the same file's §3, two tasks and the

> **CLOSED 2026-08-27.** 005's §2 now reads "No new region… the one change to `layout.templ` is §3's unanswered-invitation badge, which sits *inside* the existing `#primary-nav` and therefore adds no landmark of its own." `plan.md` marks the file `[EDIT]`, and T136's rationale is "added no landmark and no live region". T128 is unchanged and now consistent.

notification stream all add the unanswered-invitation badge to the primary nav, which lives in
`shell/layout.templ`.**

The `#notice-region` repair rewrote §2 as:

> ## 2. Shell change
>
> **None.** This phase adds no element to `internal/web/views/shell/layout.templ`.

Twenty-two lines later, `005/contracts/pages.md:62`:

> | the primary nav | an unanswered-invitation badge, driven by a Datastar signal patched by the notice stream | FR-064 |

and `005/tasks.md:323` (T128) — *"Add the unanswered-invitation badge to the primary nav in
`internal/web/views/shell/layout.templ`"*. The primary nav is `#primary-nav` inside `layout.templ`
(`SHARED-DESIGN:758-761`; `002/tasks.md:236` already mounts the switcher there). `005/plan.md:512`
marks the file `[UNCHANGED]`, and `005/tasks.md:339` (T136) gives its rationale as *"prove this
phase added no shell element"* — which T128 falsifies.

Nothing breaks: the badge is additive inside an existing landmark, so no landmark assertion changes
and every smoke case still passes. But §2 as written is untrue, and it is untrue in the direction
that gets a settled decision re-opened — a reader who spots the contradiction may conclude the
`#toast` decision is also unsettled.

*Fix*: §2 becomes *"**No new region.** This phase adds no live region, no landmark and no patch
target to `shell/layout.templ`; the one change to that file is §3's unanswered-invitation badge
inside the existing primary nav."* Then `plan.md:512` reads `[EDIT] invitation badge in #primary-nav;
notices patch into 001's #toast`, and T136's rationale becomes *"prove this phase
added no landmark and no live region"*.

### Low

**N4 — Two stale finding-id citations and one deviation row whose archaeology does not reconcile.**

> **CLOSED 2026-08-27.** `001/tasks.md:172` and `:180` now cite **H1**. The two deviation rows in `001/plan.md` carrying unreconstructable pre-amendment figures are stamped as provenance-only; their current-state halves (22 operations, 9 pages + 3 error views) were already correct and agree with §2.3 and §3.1.

- `001/tasks.md:172` and `:180` still cite the singular/plural medication-path drift as
  *"cross-artifact finding H2"*. The settled id is **H1**; `001/plan.md:533`, `:821` and
  `001/research.md:173` all record *"it was H2 in the run before it"*. These are the last two sites
  of a drift otherwise fully repaired — `006/contracts/auth-oauth2.md:11` now reads H7 with the same
  parenthetical, and every other H-id in the suite is current.
- `001/plan.md:814`'s deviation row reads *"Phase 001 is 29 operations | 22 operations | The **eight**
  not built are the patient and photo surface (→ 002) and OAuth2 (op 4, → 006)"*. The patient and
  photo surface is ops **13–20**, which is eight on its own; op 4 makes nine. And 29 − 8 = 21, which
  does not reach 22 under any reading, given ops 7, 8, 94 and 95 are added in this phase. The row
  describes a pre-amendment state of `SHARED-DESIGN` that cannot be reconstructed from the amended
  document. The current-state half of every such cell — 22 operations, 9 pages + 3 error views — is
  correct and agrees with §2.3 and §3.1. The neighbouring page row (`:818`, *"Phase 001 is 13
  pages"*) has the same property.

*Fix*: change the two `001/tasks.md` sites to H1; and either drop the "contract says" numbers from
those two rows or stamp them *"as the contract read before the 2026-08-27 amendment; the amended
figures are 22 and 9 + 3"*. Documentation only — nothing reads these numbers.

---

## What was already litigated — do not re-open

Three rounds have run against this suite. This section exists so the fourth reader does not spend a
day re-deriving a decision that is already made and already written down.

### Round 1 — the original 23 findings: all closed

**All four criticals are dead.** **C1** (the audit vocabularies had no owner) — `001/data-model.md`
§3 declares the complete twenty-action, twenty-three-kind vocabulary and every later phase asserts
the *complete* set, never a delta; six assertion tasks enforce it. **C2** (006 published columns 001
did not create) — `reason`, `affected` and `error_code` are real columns or bounded tokens on real
columns, and the `ip` column is gone from the contract as well as from the phases. **C3**
(`navigation[name="Main"]` vs `"Primary"`) — `Primary` is the only navigation landmark string in the
suite. **C4** (`/signin` vs `/login`) — `/login` throughout.

**All seven highs are closed.** H1 the singular medication path (002 reads `/records/medications`
throughout; `grep -rn 'records/medication\b' specs/002-patient-core/` returns nothing). H2 002's
recovery assumption. H3 `SHARED-DESIGN`'s dead allocation. H4 the roll-up arithmetic — computed once
in §§1.6/2.3/3.1 and now cited, not re-derived, at every phase site. H5 the four added modules
(admitted to the constitution at v1.3.0). H6 002's missing deviation table. H7 recovery,
verification and SSO ownership (001 takes ops 7, 8, 94, 95; 006 takes op 4).

**All eight mediums and all four lows are closed**, including M5 (the Labels/Appointments landmark
rename, reversed in 003) and M7 (104 of 520 FRs uncited — now 100% both ways in all six phases).

### Round 2 — the 15 verification residuals: all closed

O1 the `ip` mandate in `SHARED-DESIGN` §1.2 and §6.4 — gone. O2 ten stale roll-up sites — all
repaired; `003/plan.md:450` now reads ±0 with the reconciliation, `003/plan.md:181` reads 53
acceptance scenarios (verified: 53), `006/tasks.md:614` reads 261 tasks and 162 test tasks **with
the rule that reproduces them**, `006/research.md:667`/`:1504` and `006/plan.md:227` read 30
entities, `006/research.md:1112` reads 25 operations. O3 the vocabulary-assertion task ordering. O4
the two 004 `ip` publications. O5 the constitution version — all six plans now pin v1.3.0, including
001 and 002. O6 the switcher's phase ceiling. O7 the uncited SCs — all discharged or marked; 006
SC-007 now has T114a and SC-015 now has T174a. O8 T037. O9/O13 the stale statements and superseded
finding ids — repaired but for the two sites in N4. O10 `#notice-region` — dead (R4). O11 the
route-table arithmetic. O12 op 4's history. O14 the "belong to no phase yet" claim. O15 the last
"label".

Also closed this round without being raised: the `medigo reindex` / FTS5 contradiction
(`003/plan.md:466` now records the deviation and states that risk R3 is **CLOSED** on
VERIFIED-SOURCE-FACTS FACT 11, so availability is *not* the reason — FR-073 declining ranking is);
`SHARED-DESIGN` §3.1's terminology block, now past tense with *"`Labels`, `Label`, `Appointments`
and `Appointment` are dead names"*; the untestable *"comparable time"* assertion, moved out of the
Playwright gate into `timing_bench_test.go` (T202a) with the deterministic dummy-hash mechanism left
on the gate instead; and 006's use of "appointment" in prose, which now appears nowhere.

### Round 3 — the five hand repairs: all hold

R1 through R5 in the table above. Do not regress them, and do not re-derive their reasoning: each is
stated at source with the failure it prevents.

### Settled decisions — a contradiction with these is a defect in the contradicting document

- The audit trail has **no `ip` column**, in any phase, and no content column of any kind.
- `001/data-model.md` §3 owns the **complete** audit vocabulary and names the phase that first
  writes each value. Later phases assert the complete set, never a delta.
- `audit_events.request_id` is **`Required`**, and a background run fills it from a **`run_id`**
  minted by the same helper that mints request ids.
- `audit_events.target_id` is **`≤64`**, opaque, *never a name* — with one bounded exception:
  `target_kind ∈ {system, backup, export}`, which carries the job or archive name.
- **Phase 006 creates no audit index.** 001's three and 002's one are pre-widened with the
  `id DESC` / `occurred_at DESC` tiebreakers 006's keyset reader needs.
- `navigation[name="Primary"]` is on **every** page, signed in or out. What changes signed out is
  its contents, not its existence. `combobox[name="Active patient"]` is the conditional one.
- **`#toast`** is the one polite live region for the whole suite, built by 001. `#live-region` and
  `#notice-region` are dead names.
- Password recovery and email confirmation are phase **001**; OAuth2/SSO is phase **006**.
- **`tags`** and **`encounters`** are the words. `Labels`, `Label`, `Appointments` and `Appointment`
  are dead names.
- `004`'s **T037 is deliberately vacant** and documented as such.
- Search is `LIKE` over an ordinary `search_index` collection, **not** FTS5 — because FR-073
  declines relevance ranking, not because FTS5 is unavailable. `medigo reindex` is not built.
- Roll-ups are computed once in `SHARED-DESIGN` §§1.6/2.3/3.1: **30 collections, 94 operations,
  58 pages + 3 error views + 7 page-action routes, 15 record kinds**. Cite them; never re-derive.

---

## What an implementer should do first

1. **Start phase 001 at T001.** Nothing gates it. The task list is sound: 345 tasks, tests before
   implementation in all six user stories, no forward dependency, no dangling id, the FR join
   complete both ways, and every audit value it writes declared by the migration it writes. The two
   defects that stopped the previous round — the required `request_id` with no background source,
   and the signed-out navigation landmark — are both settled at source and propagated to every
   dependent site.
2. **Do the N1 edit before phase 006's foundational block** — it is four sites plus a research
   sentence, no judgement required, and `006/data-model.md` §4.3 already tells you the answer. Do it
   early anyway: it is easier to delete a migration nobody has written than to unpick one that
   shipped.
3. **Settle N2 in the same sitting as N1**, because it is a naming decision that becomes expensive
   once archives exist with the wrong-shaped names. Compact timestamp, bounded upload key, one
   sentence in `001/data-model.md:267` showing the arithmetic.
4. **Do the N3 rewrite before phase 005 starts** — one paragraph in `contracts/pages.md`, one tree
   annotation, one task rationale.
5. **N4 whenever.** It is two words and two table cells.
6. **Keep the six complete-set vocabulary assertions honest.** T070a, T023a, T032a (003), T036,
   T019a and T032a (006) are the mechanism that keeps a `SelectField` write from failing in front of
   a person. They assert set equality, so *adding* a value without updating them fails red — which
   is the point. Do not relax them to subset checks.
7. **Treat each phase's `traceability.md` task as Exit Criterion 0, not paperwork.** T217a, T140a,
   T248a and their siblings are the generated join that catches the FR-035 class of defect — a task
   citing a requirement it has nothing to do with. That defect got through two rounds of an id-level
   join and was only caught by reading the task. The generator cannot read; the reviewer must.

---

*Every count in this report was produced by a command against the files, not read back from a
document's claim about itself. The five hand repairs were verified from their source lines and then
attacked from every downstream site that could contradict them; four of the five are clean end to
end, and the fifth (R5) is clean at both authoritative sources with a propagation gap recorded as
N1. Where this report disagrees with an earlier revision of itself — on the task totals (1332, not
1326), on the test-task rule, on 006's suffixed-id count (twelve, not seven) and on whether any
finding is blocking (none is) — the file was re-read and the earlier figure withdrawn.*
