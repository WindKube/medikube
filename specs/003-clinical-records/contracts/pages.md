# Contract: pages and the browser gate

**Pages added: 29.** 26 from the kind registry (13 kinds × 2) plus `/tags`, `/search` and
`/timeline`. Every one is emitted by `medigo routes` with `Page: true`, and
`e2e/routes.gate.spec.ts` fails the build if any registered page route has no smoke case
(FR-093, SC-015).

---

## 1. The shell, inherited from phase 001

Every page renders inside the shell and therefore asserts these four landmarks in addition to its
own: `banner`, `navigation[name="Primary"]`, `main`, `contentinfo`, and `combobox[name="Active patient"]` visible on every **authenticated** page (introduced by phase 002; asserted from phase 002 onward — a switcher that throws must break the gate everywhere at once, which is the desired blast radius). Plus `body[data-signals]`
present, which is what proves Datastar booted.

**Landmarks live outside every patch target and are never morphed.** Every patch target's root
element carries a deterministic id from `internal/web/views/ids`, and that component is the only
thing that renders that id. Handlers never type a raw selector.

---

## 2. The two-page-per-kind pattern

Each registered kind contributes exactly two routes, both emitted by `records.Register`:

- `/{segment}` — list, filter chips, sort, empty state, and a **create drawer opened by a Datastar
  signal, not a route**. Live-updated by `/api/v1/streams/records`.
- `/{segment}/{id}` — detail with inline edit, links from both ends, delete-with-confirmation.

There is deliberately **no `/new` and no `/edit` route**: those are UI states, and each would cost
13 routes, 13 smoke cases and 13 OpenAPI entries for a deep link nobody asked for.

| Route | Landmark |
|---|---|
| `/allergies` · `/allergies/{id}` | `region[name="Allergies"]` · `article[name="Allergy"]` |
| `/conditions` · `/conditions/{id}` | `region[name="Conditions"]` · `article[name="Condition"]` |
| `/encounters` · `/encounters/{id}` | `region[name="Encounters"]` · `article[name="Encounter"]` |
| `/procedures` · `/procedures/{id}` | `region[name="Procedures"]` · `article[name="Procedure"]` |
| `/treatments` · `/treatments/{id}` | `region[name="Treatments"]` · `article[name="Treatment"]` |
| `/symptoms` · `/symptoms/{id}` | `region[name="Symptoms"]` · `article[name="Symptom episode"]` |
| `/vitals` · `/vitals/{id}` | `region[name="Measurements"]` · `article[name="Measurement set"]` |
| `/immunizations` · `/immunizations/{id}` | `region[name="Vaccinations"]` · `article[name="Vaccination"]` |
| `/injuries` · `/injuries/{id}` | `region[name="Injuries"]` · `article[name="Injury"]` |
| `/insurance` · `/insurance/{id}` | `region[name="Insurance"]` · `article[name="Insurance policy"]` |
| `/equipment` · `/equipment/{id}` | `region[name="Equipment"]` · `article[name="Equipment"]` |
| `/emergency-contacts` · `/emergency-contacts/{id}` | `region[name="Emergency contacts"]` · `article[name="Emergency contact"]` |
| `/family-history` · `/family-history/{id}` | `region[name="Family history"]` · `article[name="Relative"]` |

Every list route redirects to `/{segment}?patient={active}` when `?patient=` is absent and an
active patient is set; when it is not set, or is no longer reachable, the user lands on
`/patients`. **The active patient is a UI convenience and is never an authorization input**
(FR-081).

---

## 3. The three standalone pages

| Route | Purpose | Landmark |
|---|---|---|
| `/tags` | Tag manager: create, rename, recolour, delete, with usage counts and a "N records carry this" confirmation | `region[name="Tags"]` |
| `/search` | One search over a named person's whole chart, grouped by kind, each group paged independently | `search` |
| `/timeline` | One chronological view across every kind, narrowable by kind, date range and tag | `region[name="Timeline"]` |

`/search` and `/timeline` both require `?patient=`; without one they render an explicit
"choose a person" state rather than guessing (FR-070, US8-3).

---

## 3.5 The seven status views — registered smoke targets, not pages

FR-078 requires a view of what is currently true for a person; FR-079 requires each to be
**a narrowing of that kind's own list**, reachable equally by narrowing by hand. So they are query
strings on registered routes, not routes of their own:

| Status view | Kind list it narrows | FR |
|---|---|---|
| `/conditions?active=true` | conditions | FR-078 |
| `/medications?active=true` | medications (phase 001's kind) | FR-078 |
| `/procedures?scheduled=true` | procedures | FR-026, FR-078 |
| `/injuries?unresolved=true` | injuries | FR-078 |
| `/allergies?critical=true` | allergies | FR-078 |
| `/equipment?service_due_within_days=30` | equipment | FR-049, FR-078 |
| `/insurance?expiring_within_days=60` | insurance | FR-046, FR-078 |

**They are not page routes and they must not become page routes.** Seven more routes would be seven
more OpenAPI-adjacent inventory entries and seven landmarks for pages that are the same page, and
FR-079 says in as many words that a status view *is* the list.

**But FR-080 requires a helpful empty state on every one of them**, and a requirement the browser
gate cannot see is exactly what Principle VIII exists to prevent (cross-artifact finding **L2**).
So the page spec in `internal/httproute` gains **one optional field**:

```go
SmokeVariants []string   // additional concrete URLs on THIS route that the gate must also visit
```

- Each variant is a full, concrete URL on an already-registered page route — **no unbound
  `{param}`**, patient id substituted from the seed, asserted by the same registry gate that
  asserts `SmokeURL` is concrete.
- Variants are **not** counted as pages: `medigo routes` emits them inside their route's entry, the
  page total stays **29**, and the OpenAPI document is untouched.
- `internal/records/statusviews.go` declares the catalogue above **once**, and both the filter
  implementation (T186) and the variants read from it — so a status view added later cannot be
  added without a smoke case, and a smoke case cannot outlive the view it covers.
- Every variant is asserted at **both** viewports with the same seven assertions in §4, against a
  seeded patient for whom **at least two of the seven are deliberately empty** (§5), so the FR-080
  empty state is what the landmark assertion actually exercises.

---

## 4. What every smoke case asserts

At **1440×900** and **390×844**, for every route above:

1. HTTP `200`
2. `banner`, `navigation[name="Primary"]`, `main`, `contentinfo` visible, and `combobox[name="Active patient"]` visible on every **authenticated** page (introduced by phase 002; asserted from phase 002 onward — a switcher that throws must break the gate everywhere at once, which is the desired blast radius)
3. the page's own landmark visible
4. `body[data-signals]` present
5. **zero** browser console errors
6. **zero** uncaught page errors
7. **zero** failed network requests

Detail routes substitute an id from the deterministic `medigo seed` fixture.

---

## 5. Seeded state — deliberately mixed

The seed leaves a documented subset of these pages **empty**, so the `@EmptyState(title, body,
action)` path is what the landmark assertion exercises (FR-008, FR-080, SC-014: "on a seeded
installation where several of those screens have nothing to show"). Empty by design in the seed:
`/family-history`, `/equipment`, `/immunizations`, and `/search` before a term is entered — and,
among the §3.5 status views, `/injuries?unresolved=true` and `/insurance?expiring_within_days=60`,
so that FR-080's empty state is exercised by the browser gate and not only by a render test.

Two empty states are distinct and both are covered: **"nothing recorded yet"** (offering the create
action) and **"nothing matches the narrowing in force"** (offering to clear it) — FR-008, US8-2.

---

## 6. Keyboard specs (SC-018)

`e2e/specs/keyboard.spec.ts` completes, using only the keyboard, at both viewports:
record → correct → relate → tag → delete, for one kind from each user story
(condition, encounter, vitals, immunization, insurance, family member). Asserts a visible focus
indicator at every step and that focus is never lost into a closed drawer.

---

## 7. Datastar rules these pages are held to

- Only the **free** attribute set. `data-persist`, `data-query-string`, `data-replace-url`,
  `data-scroll-into-view`, `data-view-transition`, `data-custom-validity`, `data-animate`,
  `data-match-media`, `data-on-raf`, `data-on-resize`, `@clipboard` and `@fit` are **Pro** and are
  blocked by a lint rule.
- The v1 delimiter is a colon: `data-on:click`. `data-on-click` **silently does nothing**.
  `data-on-load` is `data-init`.
- The inline-script SDK family is banned outright — `ExecuteScript`, `ConsoleLog`, `ConsoleError`,
  `Redirect`, `Redirectf`, `DispatchCustomEvent`, `ReplaceURL`, `ReplaceURLQuerystring`,
  `Prefetch`. Each appends a literal inline `<script>`, each fails under `script-src 'self'`, and
  each failure logs a CSP violation that fails assertion 5. Redirects are a `303` issued **before**
  the stream opens; user-visible errors patch `#error-banner`.
- Most interactions need **no SSE**: Datastar honours a plain `text/html` response as an element
  patch. Streams are reserved for the live list, which minimises exposure to the five-minute
  write-timeout trap.
- UI preferences (chosen filters, list density) persist on the `users` record and hydrate through
  `data-signals`. Not `localStorage`.
