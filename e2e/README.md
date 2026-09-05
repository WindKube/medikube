# MediKube's browser gate

Playwright, driving a real built binary. Constitution Principle VIII: the UI
must prove it renders.

## Running it

```bash
task build          # e2e/routes.ts shells out to ../medikube; it has to exist first
cd e2e
npm ci
npx playwright install --with-deps chromium   # first run only
npx playwright test                            # the whole suite, both viewports
npx playwright test smoke.spec.ts              # task smoke: the page-specific assertions alone
```

`task test:e2e` and `task smoke` both depend on `build`, so the usual entry
points never need the manual step above. `npx playwright test --list` is
enough to exercise collection — including `routes.ts`'s call into the binary —
without a browser, which is what this repository's CI-less environments use
to check that the inventory still loads.

## Where the page list comes from

`routes.ts` calls `medikube routes --json` at collection time and keeps every
`kind: "page"` row. That JSON **is** `internal/httproute/routes.go`'s route
table — nothing here is a hand-kept list of pages, and nothing here can drift
from it, because a page missing a `Landmark` or a `SmokeURL` cannot boot at all
(`internal/httproute/registry.go`'s `describePage` panics on either being
empty).

- `routes.gate.spec.ts` (T260, T295) runs `gate.ts`'s seven generic assertions
  — status, the four shell landmarks, the page's own landmark non-empty,
  title, zero console/CSP/network problems — against every page `routes.ts`
  lists, plus the two error views a URL can reach (404, 403). The third,
  500, is not reachable by any URL in a shipped build on purpose; see
  `contracts/pages.md` and `internal/web/page/errors_test.go` (T230).
- `a11y.spec.ts` (T256) and `responsive.spec.ts` (T257) reuse the same list
  for keyboard reachability and the 390px viewport.
- `smoke.spec.ts` is unchanged by any of the above: it holds the
  page-*specific* assertions (what the record list contains, what the
  settings danger zone says, the two account-recovery states) that a generic
  list can't derive.
- A page's credential and title are worked out from its `Auth` column and its
  landmark's name wherever that is enough (`routes.ts`'s `credentialFor`,
  `routes.gate.spec.ts`'s `titleFor`). The two pages where it is not enough —
  the record detail page titles itself after the record, not the landmark,
  and the email-confirmation page's title uses different words than its
  landmark's name — are named explicitly, with the reason, rather than
  guessed at. Anything else that doesn't fit fails loudly, naming the page's
  operation id, instead of being silently skipped.

## Red-gate demonstrations

Constitution Principle VIII: *"a flaky assertion is fixed or removed, never
retried into passing"* — and a gate nobody has watched fail is a gate nobody
should trust. These are the three demonstrations tasks.md's T295–T297 ask for.
**None of them can be run in this environment**: Chromium cannot launch here
(the system is missing the shared libraries `playwright install --with-deps`
would provide), so no command below has been executed and no output below is
real. They run in CI's `e2e` job, which does have a browser, and that is where
the actual failing output is captured.

### T295 — a page with no smoke case

```bash
# internal/httproute/routes.go: add a page route with no Landmark/SmokeURL,
# or comment one page's Landmark out of the struct literal.
task build
cd e2e && npx playwright test --list
```

Expected failure: the binary itself refuses to start `routes --json` (or any
command touching the registry) — `describePage` panics before Playwright ever
collects a test, naming the offending `OpID` and which of `Landmark` /
`SmokeURL` it is missing. If the route were instead added to the registry
*without* going through `Handle` (a defect the registry cannot itself catch),
`routes.gate.spec.ts` still fails: a page routes.ts lists is a page this file
runs, so a landmark or smoke URL that is merely wrong rather than empty comes
back as a failing `region[name="..."]` assertion.

### T296 — a removed landmark

```bash
# Delete one aria-label from a view's .templ source, e.g. change
# `aria-label="Settings"` to nothing on internal/web/views/settings' region.
task build
cd e2e && npx playwright test smoke.spec.ts
```

Expected failure: `routes.gate.spec.ts`'s generic assertion 2 or 3 (a shell
landmark, or the page's own landmark, present and non-empty) fails for that
page's op id, at both viewports. Revert the `.templ` change afterward.

### T297 — a deliberate console error

```bash
# Add `console.error("boom")` to one page's inline script, or trigger one via
# a broken selector in a Datastar attribute.
task build
cd e2e && npx playwright test smoke.spec.ts
```

Expected failure: `gate.ts`'s `open()` collects every console `error` and
`warning` before the first navigation, and assertion 5 (`expect(problems.console).toEqual([])`)
fails for that page, listing the exact message. Revert the change afterward.

### T158 — phase-002 pages: a removed landmark, then a thrown script

002-patient-core's own demonstration (open risk R11), run against one of this
phase's new pages instead of phase 001's. Same caveat as above: Chromium
cannot launch here, so neither command below has been run and neither output
is real — this is the procedure and the assertion text `gate.ts` and
`routes.gate.spec.ts` would produce, worked out by reading both files.

**Removing a landmark.** Delete `aria-label="Patients"` from
`internal/web/views/patient`'s list region (or `aria-label="Patient chart"`
from the detail region).

```bash
task build
cd e2e && npx playwright test routes.gate.spec.ts
```

Expected failure: `gate.ts`'s `open()` assertion 3 fails for `patientListPage`
(or `patientDetailPage`), at both viewports, with the message
`/patients is missing region[name="Patients"]` (or
`/patients/<id> is missing region[name="Patient chart"]`) — assertion 2 (the
four shell landmarks) still passes, since only the page's own landmark was
touched.

**A thrown script.** Add a script on the patient list or chart page that
throws during load, e.g. a Datastar attribute referencing an undefined
signal.

```bash
task build
cd e2e && npx playwright test routes.gate.spec.ts
```

Expected failure: `gate.ts`'s `page.on('pageerror', ...)` listener records the
exception into `problems.crashes`, and `open()`'s assertion 6
(`expect(problems.crashes).toEqual([])`) fails with the message
`uncaught page failures on /patients` (or the detail path), listing the
thrown error's text. Revert both changes afterward.
