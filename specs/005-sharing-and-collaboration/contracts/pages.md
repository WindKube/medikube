# Contract: pages (phase 005)

Every page asserts, at **1440×900 and 390×844**: `200`; the four shell landmarks (`banner`,
`navigation[name="Primary"]`, `main`, `contentinfo`) visible; `combobox[name="Active patient"]` visible on every **authenticated** page (introduced by phase 002; asserted from phase 002 onward — a switcher that throws must break the gate everywhere at once, which is the desired blast radius);
the page's own landmark visible;
`body[data-signals]` present (proving Datastar booted); **zero** console errors; **zero** page
errors; **zero** failed network requests.

The route list under test is derived from `medigo routes`, so a page added without a smoke case
fails the build (FR-079, SC-019).

## 1. New pages

| Route | Auth | Purpose | Page landmark |
|---|---|---|---|
| `/sharing` | session | Granted and received panels: who has access to each of my people and relatives, at what level, since when, until when, the note and whether it came from an invitation; change level, change end date, revoke, leave. Filters by kind, state and counterparty; paged. | `region[name="Sharing"]` |
| `/invitations` | session | Received and sent invitations with their state; accept or decline with a note; cancel; withdraw. | `region[name="Invitations"]` |
| `/invite/{token}` | **public** | The emailed link lands here. Renders `InvitationPreview` (op 64) — sender display name, kind of thing, item count, level, lapse date, the sender's note, and a masked hint of the invited address — then offers **Sign in** or **Create an account** with that address. | `region[name="Invitation"]` |

`/invite/{token}` is the one deviation from the shared design contract's page table, justified in
[plan.md](../plan.md#deviations-from-the-shared-design-contract).

### `/invite/{token}` rules

- Public, and it is the **only** unauthenticated page this phase adds. It renders the full shell
  (minus the patient switcher) so the landmark assertions hold.
- It shows **no patient name, no relative name and no clinical content** — structurally, because
  `InvitationPreview` has no field for one (FR-023, SC-010).
- For an unknown, answered, cancelled, withdrawn or lapsed token it renders the shared `404 Not
  found` error view — the same view as a token that never existed (FR-024, SC-012).
- Signing in or registering returns to `/invitations` with the invitation waiting (US5 scenarios
  1–2). The redirect is a `303` issued by the handler, never a Datastar `Redirect` (the SDK's
  inline-script family is banned under the CSP).
- Following the link while signed in **as a different address** shows a plain "this invitation is
  not addressed to this account" panel and **nothing** about who it was for or what it covered
  (FR-025, US5 scenario 3).

## 2. Shell change

**No new region.** This phase adds no live region, no landmark and no patch target. The one
change to `internal/web/views/shell/layout.templ` is §3's unanswered-invitation badge, which
sits *inside* the existing `#primary-nav` and therefore adds no landmark of its own.

Notices are appended by `GET /api/v1/streams/notifications` into `#toast` — the polite live
region phase 001 already builds (`001/contracts/pages.md` §1), which is already `role="status"`,
already `aria-live="polite"`, already visible because a notice is a user-facing event, and
already outside every landmark and every patch target. A second polite live region on the same
page is not a second channel, it is two announcers competing for one screen reader, so this
phase reuses the one that exists rather than adding `#notice-region` beside it (ANALYSIS L,
2026-08-27).

Adding no element changes no landmark assertion, which is asserted by re-running the full
phase-001–004 smoke suite unchanged.

## 3. Pages this phase changes

| Route | Change | Requirement |
|---|---|---|
| `/patients` | owned and shared people in one list, visibly distinguished, each shared row naming who shared it and at what level, each group counted | FR-055, US1 scenario 5 |
| `/patients/{id}` | a **Share** action opening the share drawer — a Datastar signal, not a route; for a shared chart, a banner naming the grantor, the level and the end date, and the sender's note | FR-009, FR-036 |
| `/patients/{id}` delete confirmation | states how many accounts currently have access before the owner confirms | edge case 1 |
| every kind list and detail page (001/003/004) | reachable for a shared patient; write controls **absent** at `view` level, and a `view` grantee who forces a write sees the `forbidden_view_only` message in `#error-banner` | FR-058, SC-006 |
| `/documents`, `/search`, the timeline and the status views | reachable for a shared patient; no other patient's rows appear | FR-056, FR-057 |
| the primary nav | an unanswered-invitation badge, driven by a Datastar signal patched by the notice stream | FR-064 |

## 4. Empty states (FR-040, US1 scenario 8, SC-019)

`/sharing` and `/invitations` each render the shared `@EmptyState(title, body, action)` **inside
their own landmark**, so the landmark assertion holds on a freshly seeded instance with nothing to
show. The seed provides `empty@medigo.local` — an account with nothing shared in either direction —
and both pages are smoke-tested **as that account** as well as as the populated one. This is the
most common way a smoke gate goes falsely red, and it is the reason the fixture exists.

Three empty states, all with an action and never a blank screen or a row of zeros: nothing shared,
no invitations, and a person with no relatives (the last on the existing family-history page).

## 5. Playwright specs added

| Spec | Asserts |
|---|---|
| `e2e/specs/sharing.spec.ts` | `/sharing` at both viewports, populated and empty; revoke and leave complete and the row updates |
| `e2e/specs/invitations.spec.ts` | `/invitations` at both viewports, populated and empty; accept and decline complete |
| `e2e/specs/invite-landing.spec.ts` | `/invite/{token}` with the seeded pending token; a lapsed token renders the 404 view; the page contains no patient name (a sentinel assertion against the seeded patient's name) |
| `e2e/specs/sharing-live.spec.ts` | **two browser contexts**: a notice reaches the recipient with no refresh; a revoke empties an open shared list and states that access ended, within 5 s (FR-045, SC-005, SC-017) |
| `e2e/specs/sharing-keyboard.spec.ts` | SC-020: send an invitation, answer one, change a level and end access using **only** the keyboard, at both viewports, with the focus ring visible at every step |

`e2e/routes.gate.spec.ts` is extended so the three new routes are covered, with the seeded token
substituted into `/invite/{token}` — the same `SmokeURL` mechanism the kind pages already use.
