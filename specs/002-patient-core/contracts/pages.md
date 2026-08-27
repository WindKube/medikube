# Contract: pages, landmarks and smoke assertions (phase 002)

**Pages added: 6.** **Shell components added: 1** (the patient switcher). **API operations: 0** —
they are in the other files in this directory.

This file exists because phases 003–006 each have one and this phase did not, although it is the
phase with the widest smoke blast radius in the suite: it welds a control into the shell that
**every** page of every phase then renders (cross-artifact finding **M3**). Every landmark string
below is a literal that a Playwright selector contains, and **changing one is a breaking change to
the gate**.

## 1. The shell, inherited from phase 001 — and the one thing this phase changes

Every page renders inside phase 001's `Layout` and therefore asserts the four shell landmarks in
addition to its own: `banner`, `navigation[name="Primary"]`, `main`, `contentinfo`. Plus
`body[data-signals]` present, which is what proves Datastar booted.

**This phase adds the patient switcher to the shell**, in the banner, outside `main`:

| Component | Selector the gate uses | Asserted on |
|---|---|---|
| patient switcher | `combobox[name="Active patient"]` | **every authenticated page in the application**, phases 001–006 |

Three consequences, stated so nobody discovers them:

1. **Every existing smoke case now exercises the switcher.** A switcher that throws breaks the gate
   on every authenticated page at once. That is the desired blast radius, not a hazard to route
   around: the whole point of putting it in the shell is that it is always there.
2. **It renders outside `#main` and outside every patch target**, so no element patch can morph it
   away. Its own id comes from `internal/web/views/ids` like every other patch target.
3. **Each option carries the person's name *and* their date of birth**, because twins and
   same-named relatives are the ordinary case in a household (spec Edge Cases). It is a UI
   convenience and it is **never** an authorization input (FR-015) — every API handler takes its
   patient from the request.

An account whose switcher has nothing to offer still renders the landmark: a person with exactly
one patient sees their own self-record, which FR-005 guarantees exists for every account.

## 2. The pages

| # | Route | Auth | Landmark inside `main` | Title | FR |
|---|---|---|---|---|---|
| P1 | `/patients` | session | `region[name="Patients"]` | People — MediKube | FR-001, FR-010 |
| P2 | `/patients/{id}` | session | `region[name="Patient chart"]` | {name} — MediKube | FR-027 … FR-030 |
| P3 | `/practitioners` | session | `region[name="Practitioners"]` | Practitioners — MediKube | FR-032, FR-036 |
| P4 | `/practitioners/{id}` | session | `article[name="Practitioner"]` | {name} — MediKube | FR-032, FR-040 |
| P5 | `/facilities` | session | `region[name="Facilities"]` | Places of care — MediKube | FR-034, FR-036 |
| P6 | `/facilities/{id}` | session | `article[name="Facility"]` | {name} — MediKube | FR-034, FR-040 |

`region[name="X"]` is a `<section aria-label="X">` and `article[name="X"]` is an
`<article aria-label="X">` — ARIA role selectors, assertable with Playwright's `getByRole` without
test-only attributes in production markup. This is phase 001's rule, unchanged.

**Every one of the six is registered with a `Landmark` and a concrete `SmokeURL`** in
`internal/httproute/routes.go` (T066, T139). The registry **panics at registration time** on a page
spec missing either, so a page added without gate coverage is a boot failure rather than a quiet
gap (T041, T157).

**The detail routes' smoke URLs substitute deterministic ids** from `medikube seed` — never a literal
id typed into a spec file, which is what `internal/testsupport/fixtures.go` exists to prevent.

**No page in this phase is public.** All six require a session; an anonymous request lands on the
sign-in prompt, which is phase 001's `403` view, not a `404`.

## 3. Existing pages this phase changes

| Page | Owner | What changes |
|---|---|---|
| every authenticated page | 001–006 | the patient switcher appears in the shell (§1) |
| `/medications`, `/medications/{id}` | 001 | list is patient-scoped; `/medications` redirects to `/medications?patient={active}` when `?patient=` is absent and a person is in view, and to `/patients` when there is none or it is no longer reachable |
| `/` (dashboard) | 001 | counters become per-patient for the person in view |
| `/settings` | 001 | gains the unit-system preference |

**A redirect is not an error and must not read as one to the gate**: the smoke case follows it and
asserts the landmark of where it lands. A `/medications` visit with no reachable person in view
lands on `/patients` with `region[name="Patients"]` — an ordinary page, `200`, no console error.

## 4. What every smoke case asserts

At **1440×900** and **390×844**, for every route in §2 and for every page in §3:

1. HTTP `200` (after following any redirect in §3);
2. `banner`, `navigation[name="Primary"]`, `main`, `contentinfo` visible;
3. **`combobox[name="Active patient"]` visible** — this phase's addition;
4. the page's own landmark visible **and non-empty**;
5. `body[data-signals]` present;
6. **zero** browser console errors and warnings;
7. **zero** CSP violations;
8. **zero** failed network requests.

Assertion 4's non-empty clause is the load-bearing one: a landmark containing nothing passes a
naive presence check while the page is broken.

## 5. Empty states, and the seeded state that exercises them

`@EmptyState(title, body, action)` renders **inside** the page's own landmark, never instead of it
(phase 001's rule, and SC-013 requires several of this phase's screens to be empty on a freshly
seeded instance). Two distinct states, both covered:

- **"nothing recorded yet"**, offering the create action — `/practitioners` and `/facilities` for
  the second seeded account;
- **"nothing matches the narrowing in force"**, offering to clear it — the `?q=` and `?kind=`
  filters on the two directories.

The seed leaves account A with three patients (one of them its self-record), a practitioner, a
facility and medications on two of the three; account B with its self-record alone and both
directories empty. So one pass covers the populated list, the single-row list, the empty state and
the isolation path.

**The chart page is asserted in both states**: a patient with records in every registered kind, and
a patient with none — where every tile shows zero **with its label** rather than disappearing
(FR-030, US4-2). A chart that renders nothing for an empty person is the failure this row exists to
catch.

## 6. Proving the gate still goes red

Phase 001 demonstrated the gate failing on a removed landmark and on a deliberate browser error.
This phase re-demonstrates it **once**, on one of its own pages, because six new pages and a new
shell component are exactly the change that can silently fall outside a gate (T158, risk R11). Both
runs are recorded in `e2e/README.md`.
