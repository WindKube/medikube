# Contract: pages, fragments and the whole-product browser gate (phase 006)

**Pages added: 7.** **Page-action routes added: 3.** **API operations: 0** (they are in the other
files).

The shared design contract originally allocated **eight** pages to this phase. `/trash` is
**removed**: phase 004 already recovers a deleted document as a filter on the document library, and
the specification forbids a second surface ([D-14](../research.md#d-14),
[D-15](../research.md#d-15)). The contract itself has been amended — SHARED-DESIGN §3.1 no longer
lists `/trash` and records the removal — so the page inventory, the `medigo routes` output and the
Playwright route list remain one list. The contract's total moves 57 → **56**, which is recorded in
[plan.md's Deviations](../plan.md#deviations-from-the-shared-design-contract).

**One page of phase 001 is edited, and no page is added for it.** External sign-in (op 4, claimed
from the contract's unowned set — cross-artifact finding **H7**) adds a provider control to the
existing `/login` page, rendered **only** when the operator has enabled a provider. No route, no
landmark and no page count changes, and SC-028 is asserted as the absence of that control on an
instance with no provider configured — in the templ render test and in the browser gate.

## 1. What every page asserts

The shell is SHARED-DESIGN §3.0's and is unchanged. Every page, at **1440×900** and **390×844**,
asserts:

- HTTP `200`;
- `banner`, `navigation[name="Primary"]`, `main`, `contentinfo` visible;
- `combobox[name="Active patient"]` visible on every **authenticated** page (introduced by phase 002; asserted from phase 002 onward — a switcher that throws must break the gate everywhere at once, which is the desired blast radius);
- **the page's own landmark** (below) visible;
- `body[data-signals]` present, proving Datastar booted;
- **zero** browser console errors, **zero** uncaught page errors, **zero** failed network requests.

The landmarks live outside every patch target and are never morphed. Empty states render the shared
`@EmptyState(title, body, action)` **inside** the page's own landmark, so the landmark assertion holds
on an instance holding nothing — the most common way a smoke gate goes falsely red.

## 2. Pages

| Route | Purpose | Page landmark |
|---|---|---|
| `/reports` | Three regions in one page ([D-53](../research.md#d-53)): the **builder** (per-kind counts, the selection, the chart picker, the resolved count, and generate), the account's **saved reports**, and the **documents produced** from them with progress, download and re-run | `region[name="Reports"]` |
| `/reports/{id}` | A saved report's editor: name, description, person, criteria, charts, presentation settings, what it resolves to right now, produce-with-override, and delete-with-confirmation | `article[name="Report template"]` |
| `/exports` | The account's portable-export requests: request form (people, kinds, range, tabular files, documents), position while waiting, progress while running, finished size, download, cancel and re-run | `region[name="Exports"]` |
| `/admin` | The operator overview: every figure with its definition and the moment it was computed, the posture warnings, the retention windows with their last run and last success, every limit, and the attention list | `region[name="Administration"]` |
| `/admin/audit` | The activity trail: newest first, the narrowing controls, the window in force and the age of the oldest entry, paging, and the CSV export | `region[name="Audit trail"]` |
| `/admin/users` | The account list, and the tier / disable / force-password-change actions behind confirmations | `region[name="Users"]` |
| `/admin/backups` | The archive list, take-with-note, upload, preview, download (password re-entry), restore (phrase + password) and delete | `region[name="Backups"]` |

### 2.1 Empty states

| Page | Empty state | Requirement |
|---|---|---|
| `/reports` (person with nothing recorded) | every kind shown at **zero**, with an explanation of what a report is and how to start recording, **and asking for a report is refused with that same explanation** rather than producing an empty document | US1 AS-1, FR-004 |
| `/reports` (no saved reports) | what a saved report is, and the action to create the first | US3 AS-1 |
| `/reports` (nothing produced yet) | "Nothing produced yet — build one above" | US1 AS-1 |
| `/reports/{id}` | n/a — a saved report always exists here; the **person-is-gone** panel is the interesting state (FR-032) | US3 AS-11 |
| `/exports` | "An export gives you everything this account holds, in a documented format you can read without this application" + the action | US2 AS-1 |
| `/admin` | every figure **zero with its definition**, never a blank tile, never an error, never anything mistakable for a failure to compute | FR-081, US5 AS-1 |
| `/admin/audit` | "Nothing has been recorded on this instance yet" — inside the landmark | US6 AS-1 |
| `/admin/users` | n/a — at least one account always exists | — |
| `/admin/backups` | "An archive is a complete copy of this instance" + take the first one | US7 AS-1 |
| the chart picker (no measured values) | charts need repeated readings, and how to record them — inside the standard page structure | US4 AS-1 |

### 2.2 Assertions specific to this phase

| Page | Additional assertion | Requirement |
|---|---|---|
| `/reports` | the resolved count updates as the selection changes, and the figure shown equals what the produced document contains | FR-003, SC-002 |
| `/reports` | a selection over the limit states the limit and asks for a narrower selection **before** anything is produced | FR-010, US1 AS-14 |
| `/reports` | a value below the charting minimum is **shown** as not yet chartable with the count it has and the count it needs — never hidden | FR-017, US4 AS-3 |
| `/reports` | a series recorded in more than one unit forces a unit choice and states that other units are excluded; **no converted value appears anywhere** | FR-018, SC-008 |
| `/reports`, `/exports` | a queued request shows its **position**, so waiting never looks like a hang | FR-045, US2 AS-10 |
| `/reports`, `/exports` | an expired request is shown as **expired with the window that applied**, is not downloadable, and offers to produce it again | FR-047, US8 AS-1, AS-2 |
| `/admin` | the posture warning is unmistakable, names exactly which protection is missing and what to do, and **keeps appearing** until it is fixed | FR-083, SC-012 |
| `/admin` | every figure carries a definition and a `computed_at`; refreshed figures state their age | FR-080, SC-011 |
| `/admin` | the deleted-document figures show **no file name, no description and no person**, and link to `/documents?deleted=true` rather than offering a second recovery surface | FR-056, FR-057 |
| `/admin/audit` | no entry displays a name, a diagnosis, a value, a note, a tag name or a file name — asserted against seeded sentinels; and the page issues **no** request that would fetch one | FR-068, SC-014 |
| `/admin/audit` | the window in force and the age of the oldest entry are stated on every page, including an empty one | FR-074 |
| `/admin/users` | disabling names the account and states that its sessions end immediately; demoting the last administrator is refused with the reason | FR-090, FR-096 |
| `/admin/backups` | the restore preview states, in words, that everything recorded since the archive was taken will be lost, and the confirm control is inert until the phrase matches and the password is entered | FR-103, FR-104 |
| `/admin/backups` | the download control is an ordinary `<form method="post">` with a password field — no JavaScript, no token in a URL | FR-109, [D-27](../research.md#d-27) |
| every operator page | reached with `role = user`, the response is the shared **404** view — identical to a route that does not exist — and one `access_denied` row is written | FR-076, SC-010 |
| every page | reached by an account with `must_change_password = true`, the response redirects to the forced-change form and **nothing else is reachable** | FR-093 |

## 3. Page-action routes

Neither navigable pages nor part of the public API. Each appears in `medigo routes` with
`Kind: page_action`, is **deliberately excluded** from `api/openapi.json`, has no ARIA landmark, and
**declares the Playwright spec that exercises it**. `e2e/routes.gate.spec.ts` fails the build if that
spec does not exist or does not reference the route. This follows phase 004's precedent exactly.

| Route | Purpose | Response | Covering spec |
|---|---|---|---|
| `GET /reports/selection` | the resolved count and per-kind breakdown as the builder's signals change | `text/html` fragment, patched into `ids.ReportCounts()` | `e2e/specs/reports.spec.ts` |
| `GET /reports/jobs` | the produced-documents region, polled every 2 s while any report job is `queued` or `running`, and **not** polled otherwise | `text/html` fragment | `e2e/specs/reports.spec.ts` |
| `GET /exports/jobs` | the export list region, polled on the same rule | `text/html` fragment | `e2e/specs/exports.spec.ts` |

All three are **plain `text/html` responses, not SSE** — Datastar honours an HTML response as an
element patch, and a job list is not worth a long-lived connection or exposure to PocketBase's
five-minute `WriteTimeout` ([D-31](../research.md#d-31)).

Polling is `data-on-interval__2s`, which is in the **free** Datastar attribute set. The Pro
attributes remain banned by the lint rule, and the inline-script SDK family
(`ExecuteScript`, `ConsoleLog`, `ConsoleError`, `Redirect`, …) remains banned outright: it appends a
literal inline `<script>`, which fails under `script-src 'self' 'unsafe-eval'` and logs a CSP
violation that fails the console gate.

## 4. The whole-product browser sweep

This phase owns the gate for **every user-facing page delivered by phases 001–006**, not only its own
(FR-126, SC-021).

| Spec | What it does |
|---|---|
| `e2e/specs/full-sweep.spec.ts` | Every route emitted by `medigo routes` with `Page: true`, at both viewports, as the seeded populated account: `200`, the four shell landmarks, the route's declared landmark, `body[data-signals]`, zero console/page/network errors |
| `e2e/specs/empty-account.spec.ts` | The same sweep as `empty@medigo.local`, which holds **nothing** — so every page that can be empty proves its explanation renders **inside** its own landmark and the gate stays green on a legitimately empty instance (FR-125, US9 AS-2) |
| `e2e/specs/operator-denied.spec.ts` | The four operator pages as a non-administrator: the shared 404 view, and an `access_denied` row per attempt (FR-076, SC-010) |
| `e2e/routes.gate.spec.ts` | The inventory gate: every `Page: true` route has a smoke case; every `page_action` route names an existing spec that references it. **A page added without coverage fails the build** (US9 AS-4) |
| `e2e/specs/gate-negative.spec.ts` | Two deliberate negatives, run in CI as *expected failures*: a page instrumented to throw a console error must turn the gate **red** and the failure must **name the page** (US9 AS-3); and a route added to the inventory with no spec must fail `routes.gate` |

The route list under test is derived from the application itself — `medigo routes` — and never
maintained by hand (Principle VIII). The seeded instance deliberately holds an account with nothing so
the `@EmptyState` path is what the landmark assertion exercises.
