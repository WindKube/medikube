# Contract: pages, landmarks and smoke assertions

Nine pages and three error views. Requirements covered: FR-043 … FR-050 (the shell and its behaviour), FR-046 (the error views),
FR-066, FR-067 and FR-072 (the render gate), SC-003, SC-004, SC-009, SC-010, SC-014. Constitution Principle VIII — "The UI Must Prove It Renders" — is
the reason this file exists, and every landmark string below is a literal that a Playwright
selector will contain. **Changing one is a breaking change to the gate**, and phases 002-005 add
rows to this table rather than editing it.

## The shell

Every page renders inside one `Layout` component with four landmarks, always present, always in
this order:

```html
<html lang="en" class="{theme}">          <!-- "dark" or "" — server-rendered, see below -->
  <body>
    <a href="#main" class="sr-only focus:not-sr-only">Skip to content</a>
    <header role="banner">                 <!-- product name, active account, sign out -->
    <nav role="navigation" aria-label="Primary">
    <main id="main" role="main">           <!-- the page -->
    <footer role="contentinfo">            <!-- version, build stamp -->
    <div id="error-banner" role="alert" aria-live="assertive"></div>
    <div id="toast" role="status" aria-live="polite"></div>
```

| Landmark | Selector the gate uses |
|---|---|
| banner | `banner` |
| navigation | `navigation[name="Primary"]` |
| main | `main` |
| contentinfo | `contentinfo` |

`#error-banner` and `#toast` are **empty containers rendered on every page**, because Datastar
patches by id and an element that does not exist cannot be patched. They are the only two
general-purpose patch targets in the shell, and the SSE contract in [streams.md](./streams.md)
depends on them existing.

**Signed-out pages render the same shell, navigation landmark included.** `navigation[name=
"Primary"]` is on **every** page in the application; what changes signed out is its *contents*,
not its existence — it holds "Sign in" and "Create account" instead of the record kinds, and it
holds **no patient switcher**, because a signed-out person has no patients.

The five signed-out pages are `/login`, `/register`, `/forgot-password`, `/reset-password/{token}`
and `/verify-email/{token}`, and phase 005 adds a sixth, the public `/invite/{token}`. That page
is why the landmark cannot be conditional: someone opening an invitation link while signed out
needs exactly the two links this nav holds. The `combobox[name="Active patient"]` assertion is
what is conditional on a session, not the landmark around it (ANALYSIS).

## The pages

| # | Path | Auth | Landmark inside `main` | Title | FR |
|---|---|---|---|---|---|
| P1 | `/login` | public | `form[name="Sign in"]` | Sign in — MediGo | FR-034 |
| P2 | `/register` | public | `form[name="Create account"]` | Create account — MediGo | FR-002 |
| P3 | `/` | session | `region[name="Overview"]` | Overview — MediGo | FR-050 |
| P4 | `/medications` | session | `region[name="Medications"]` | Medications — MediGo | FR-021 |
| P5 | `/medications/{id}` | session | `article[name="Medication"]` | {name} — MediGo | FR-024 |
| P6 | `/settings` | session | `region[name="Settings"]` | Settings — MediGo | FR-011 |
| P7 | `/forgot-password` | public | `form[name="Reset password"]` | Reset password — MediGo | FR-073 |
| P8 | `/reset-password/{token}` | public | `form[name="Choose a new password"]` | Choose a new password — MediGo | FR-074 |
| P9 | `/verify-email/{token}` | public | `region[name="Email confirmation"]` | Confirm your address — MediGo | FR-075 |

`region[name="X"]` is a `<section aria-label="X">`; `article[name="X"]` is an
`<article aria-label="X">`; `form[name="X"]` is a `<form aria-label="X">`. These are ARIA role
selectors, which is what makes them assertable by Playwright's `getByRole` without test-only
attributes in production markup.

**`/register` is registered unconditionally and renders `404` when open registration is disabled**
(the default). It does not disappear from the route table — a page that exists only under some
configurations is a page the route-inventory gate cannot check.

**P8 and P9 carry a deliberately invalid token in their `SmokeURL`**:
`/reset-password/expired-token-for-smoke` and `/verify-email/expired-token-for-smoke`. A seeded
real token cannot work — a reset token lives 30 minutes and a confirmation token 24 hours, so any
committed fixture is expired by the time CI runs it — and inventing a way to mint one for the gate
would be test-only code in a security path. Both pages therefore answer their smoke URL with
**`200`** and the "this link is no longer usable, request another" state **inside** their own
landmark, which is what FR-074 requires anyway. The most likely real-world visit to these pages is
a link opened too late, and that is the state under the browser gate. The working path is covered
by `e2e/recovery.spec.ts` against a mail sink, and by the HTTP contract tests.

**A `4xx` on a page route would fail the gate**, which is why the expired-link state is a `200`
page and not an error view. The one deliberate exception in this phase remains `/register` under
closed registration.

**Every one of the nine is declared in the route registry with a `Landmark` and a `SmokeURL`.** The
registry **panics at registration time** on a page spec missing either, so the failure is a boot
failure and not a quiet gap in the gate (FR-067).

## The three error views

| View | Status | Landmark | Rendered when |
|---|---|---|---|
| E1 not found | 404 | `region[name="Not found"]` | unknown path, **and every refused access to another account's data** |
| E2 forbidden | 403 | `region[name="Sign in required"]` | a session-required page with no session (renders the sign-in prompt, does not 404) |
| E3 error | 500 | `region[name="Something went wrong"]` | anything unhandled; carries the request id, nothing else |

**E1 is the privacy view.** FR-033 and the acceptance scenario in US3 require that a request for
another account's medication produce a response byte-identical to a request for a medication that
does not exist. That is not "similar" — the smoke run **diffs the two response bodies and fails
if they differ by a single byte**. That half is deterministic, so it belongs on the gate.

**The wall-clock half does not sit in the gate.** An earlier draft had the same browser test also
assert the two were served "in comparable time" against an "agreed tolerance" that no document ever
defined. Constitution VIII forbids a flaky gate assertion outright — *"a flaky assertion is fixed or
removed, never retried into passing"* — and a latency comparison taken through a browser, over CI
hardware that is shared, throttled and unpredictable, is the definition of one: the tolerance loose
enough never to flake is loose enough to pass an oracle, and the tolerance tight enough to catch an
oracle flakes. So the timing comparison moved to **`internal/web/api/timing_bench_test.go`**, a Go
benchmark that does **not** block merge (amendment 2026-08-27, ANALYSIS N13; T202a).

What blocks merge instead is the **mechanism**, which is deterministic: the unknown-address path
performs a bcrypt comparison against a fixed dummy hash before refusing (research D-17), and T202
asserts that comparison happens. Timing is the symptom; the dummy-hash comparison is the cause, and
a cause you can assert exactly is a better gate than a symptom you can only assert approximately.

**E3 never renders a stack trace, a driver message or a query.** The request id is the only
correlation handle, and it matches the `request_id` on the zerolog line (FR-054).

**All three render inside the full shell**, so a person who hits an error still has navigation.
This is also why the error views carry landmarks at all: a 404 with no `main` would fail the gate
for the wrong reason.

## The smoke assertions

For **every** row in the page table and **every** error view, at **both** viewports — desktop
`1440×900` and mobile `390×844` — the Playwright gate asserts all seven (FR-066, SC-003):

1. the HTTP status is the expected one;
2. the four shell landmarks are present, signed in or out;
3. the page's own landmark is present **and non-empty**;
4. `<title>` matches;
5. **zero** browser console errors or warnings;
6. **zero** CSP violations;
7. **zero** failed network requests.

**Assertion 3's non-empty clause is the load-bearing one.** A landmark containing nothing passes a
naive presence check while the page is broken, which is exactly the failure mode Principle VIII
exists to catch.

**Empty states render inside the landmark, never instead of it** (FR-029). An account with no
medications gets `region[name="Medications"]` containing an explanation and a create action — not
a bare centred paragraph where the region should be. Phase 003 depends on this rule holding.

**The seed data is deliberately mixed** (research D-39): account A has medications spanning all
three status values including one with every optional field empty; account B has exactly one;
account C has none **and an unconfirmed address**. So the smoke run covers the populated list, the
single-item list, the empty state, the widest and narrowest row and the "address not confirmed —
send it again" state in one pass, without a fixture per case (FR-075).

## Proving the gate goes red

**A gate nobody has watched fail is a gate nobody should trust.** Two negative controls run in CI
and both must produce a **failing** Playwright run (FR-072, and Phase Exit Criterion 5 in the
plan):

- a build with one landmark removed → assertion 2 or 3 fails;
- a build with a deliberate `console.error` on one page → assertion 5 fails.

They run against throwaway builds; they do not ship. Without them the first true positive is also
the first time anyone learns the gate works, which is too late.

## Rendering rules that are contract, not style

- **Server-rendered HTML only.** No client-side routing, no hydration, no virtual DOM. Datastar
  patches elements the server rendered.
- **Templ components render to a buffer in unit tests** (Principle VIII, SC-014): every component
  has a test asserting its output contains the landmark and the fields it was given. That is the
  fast layer; Playwright is the slow layer; both are mandatory.
- **The delete confirmation is a rendered element with its own landmark**
  (`region[name="Confirm delete"]`), not a `window.confirm`. A browser dialog is invisible to the
  render gate and untestable in the smoke run (FR-028).
- **A `<noscript>` block on every page** states plainly that MediGo requires JavaScript, inside
  `main`, so the page is not a blank rectangle (FR-049).
- **Theme is a server-rendered class on `<html>`** read from the account's stored preference, plus
  a Tailwind `dark` variant configured to respond to **both** the class and
  `prefers-color-scheme`. The CSP bans inline scripts, so the usual no-flash inline snippet is not
  available, and `data-persist` is Datastar **Pro**. A signed-out visitor gets the media query; a
  signed-in one gets their choice with no flash (research D-36, FR-045).
- **Focus is moved to the patched region's heading after a full-region patch**, and form errors
  are rendered adjacent to their field with `aria-describedby` (FR-047, FR-048).
- **CSP**: `script-src 'self' 'unsafe-eval'` — `'unsafe-eval'` is required by Datastar's expression
  evaluator, is accepted deliberately and permanently, and is the **only** relaxed directive.
  `default-src 'none'`, `object-src 'none'`, `frame-ancestors 'none'`, `base-uri 'self'`,
  `form-action 'self'`. Every violation fails assertion 6, which is how the ban on Datastar's
  inline-script SDK family is enforced mechanically rather than by review.
