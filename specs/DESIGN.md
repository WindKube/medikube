# MediGo — Visual Design Specification

**Status**: imported, reconciled, **not implemented**. No phase task in this suite has been
started against it.
**Source**: Claude Design project `MediKeep UI design`
(`8811aba8-5354-421e-b27c-1c24e6a14751`), file `MediKeep.dc.html`, imported 2026-08-27.
**Stored in this repository at**: `design/source/MediKeep.dc.html` (the comp, verbatim),
`design/source/support.js` (the preview runtime, quarantined — see §0.2),
`design/tokens.css` (tokens extracted from the comp; wired into nothing).
**Sits beside**: [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) and
[`VERIFIED-SOURCE-FACTS.md`](./VERIFIED-SOURCE-FACTS.md), which moved into the repository from a
session scratchpad on 2026-08-27, and the nine dossiers under
[`research/`](./research/README.md).

## 0. What this document is, and what it is not

This is the **visual** contract: colour, type, spacing, radii, density, and the component
vocabulary the product is drawn from. It is authoritative on how MediGo **looks**.

It is **not** authoritative on:

- **Structure and landmarks** — [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) §3.0 owns the shell and every landmark
  string the Playwright gate asserts. Where this document and §3.0 disagree, §3.0 wins.
- **Which pages exist** — the six phase charters own that. This design covers 20 of the
  suite's 58 page routes (§5.1).
- **What the product does** — the phase `spec.md` files own that. This design draws several
  features MediGo has explicitly declined to build (§5.3).

Precedence, unchanged: the constitution → [`VERIFIED-SOURCE-FACTS.md`](./VERIFIED-SOURCE-FACTS.md) → [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) →
the phase charters → **this document**, on visual questions only.

### 0.1 The one thing to read before translating anything

**The comp is a picture, not markup.** Counted from the source:

| Element or attribute | Occurrences in the comp |
|---|---|
| `<h1>`–`<h6>` | **0** |
| `<section>` | **0** |
| `<table>`, `<thead>`, `<th>`, `<td>` | **0** |
| `<a>` | **0** |
| `<input>`, `<label>`, `<select>` | **0** |
| `role=` | **0** |
| `aria-*` | **0** |
| `<footer>` | **0** |

Every table is `display:grid` over `<div>`s. Every link is a `<div>` or a `<button>`. The search
field is a `<div>` containing the text "Search records…". The patient switcher is a `<div>`
ending in a `▾` character.

This is entirely normal output for a design tool, and it is **not** a criticism of the design.
It is the single most important fact in this document, because MediGo gates every page on
landmark assertions under Principle VIII, and **a faithful one-to-one translation of this comp
would fail that gate on all 58 pages and be unusable with a screen reader.** Structure is not
"polish to add later"; under this constitution it is the deliverable. §6 lists what has to be
added, element by element.

### 0.2 `support.js` is quarantined

`design/source/support.js` is the generated Claude Design preview runtime (`dc-runtime`). It
renders the `<x-dc>` template format — `sc-for`, `sc-if`, `{{ }}` interpolation, the `DCLogic`
class — **on React**.

React is a forbidden dependency under the constitution's Technology Constraints, alongside Gin,
Huma, Viper, HTMX and `samber/mo`. `support.js` is stored **only** so the comp can be reopened
and re-rendered locally for reference. It must never be vendored, imported, served, bundled, or
copied into `internal/web/static/`. It carries no design content: three hex colours, all of them
its own error-overlay chrome.

MediGo's front end is templ + Datastar + Tailwind, per the constitution. Nothing in the comp
changes that.

---

## 1. The visual language in one paragraph

A calm, clinical, low-chroma product. A single desaturated forest green carries every action and
every affirmative state; everything else is a near-neutral warm grey-green. Surfaces are white
cards on a `#f5f7f6` canvas, separated by a hairline `#e4e9e7` border and a generous 15px gutter,
with a large 15px corner radius that is the product's most recognisable signature. Body text is
Hanken Grotesk; **page titles and the few genuinely human moments — the dashboard greeting, the
next-appointment card, the report preview heading — are set in Newsreader, a serif.** That serif
is the whole personality of the product: it is what stops a dense medical table from reading like
a spreadsheet. Status is never carried by colour alone in a badge; every badge has a word in it.

---

## 2. Colour

Full token list with provenance in `design/tokens.css`. Roles:

### 2.1 Brand and surface

| Token | Value | Used for |
|---|---|---|
| `--mg-brand` | `#3f6f63` | primary buttons, active nav item, links, logo mark, toggle-on, checkbox-on, the filled feature card |
| `--mg-brand-tint` | `#eef3f1` | active nav background, icon chips, tag chips |
| `--mg-brand-surface` | `#e7f0ec` | "green" status badge background |
| `--mg-sage` | `#9cb8ae` | avatars, chart bars, dropzone glyph |
| `--mg-canvas` | `#f5f7f6` | app background, inset controls (search, switcher) |
| `--mg-surface` | `#ffffff` | cards, sidebar, topbar |
| `--mg-surface-sunken` | `#fafbfb` | table header row |
| `--mg-border` | `#e4e9e7` | every card, control and panel border |
| `--mg-rule` | `#eef2f0` | row divider inside a card |
| `--mg-rule-faint` | `#f1f4f3` | row divider inside a table |

### 2.2 The status palette — six pairs, and what each means

This is the most reusable part of the design. Every badge, chip and pill in the product draws
from exactly these six, and the comp applies them consistently enough to infer the semantics:

| Name | Foreground | Background | Means | Seen on |
|---|---|---|---|---|
| green | `#3f6f63` | `#e7f0ec` | active, normal, stable, resolved, completed successfully, primary | Active, Normal, Stable, Routine, Primary, ↓ improving |
| blue | `#3a5a8c` | `#eef3f8` | in progress, in flight, scheduled, informational classification | In progress, Scheduled, Healing, Medical |
| amber | `#a06a2c` | `#faf0e2` | attention, out of range but not urgent, awaiting a person | Slightly high, Chronic, Moderate, Pending, Recurring |
| red | `#b85c4a` | `#f8e9e6` | severe, urgent, out of range, life-threatening | Severe, Life-threatening, Urgent, Low |
| gray | `#5f6f6a` | `#eef2f0` | inactive, not applicable, no signal | As needed, stable, Off, Completed (neutral) |
| purple | `#6b4f8c` | `#f1ecf6` | a secondary classification axis | Dental |

**Note the deliberate desaturation.** `#b85c4a` is a muted brick, not a warning red. A personal
medical record is read by the person whose record it is, often about themselves; the palette
refuses to alarm. Keep that when extending it.

**Constraint for implementation:** a status is never communicated by colour alone. Every badge in
the comp contains a word. Preserve that — it is what makes the palette WCAG-defensible and it
matches the constitution's Principle VIII posture toward the gate.

### 2.3 What the design does not define

No dark palette. No focus-ring colour. No hover or active states of any kind (the comp is
static). No disabled state. No error state on a field. See §7.

---

## 3. Type and scale

### 3.1 Faces

| Role | Family | Weights loaded |
|---|---|---|
| UI, body, tables | **Hanken Grotesk** | 400, 500, 600, 700 |
| Display — page titles, greeting, feature card, report heading | **Newsreader** (serif, `opsz 6..72`) | 400, 500 |

Both come from Google Fonts in the comp. **Self-host them.** The constitution's Principle VII
posture and the CSP both point the same way, and a self-hosted record application should not
issue a third-party request on every page load to render a patient's name. Fallback stacks are in
`design/tokens.css`.

### 3.2 The scale, rationalised

The comp uses **17 distinct sizes** between 10px and 27px, many separated by half a pixel:
12 / 12.5 / 13 / 13.5 / 14. That is drift from hand-tuning, not a designed scale, and shipping it
verbatim means 17 Tailwind size utilities nobody can choose between. Collapsed to ten steps:

| Token | px | Absorbs | Applied to |
|---|---|---|---|
| `--mg-text-2xs` | 10 | 10 | uppercase nav-group eyebrow |
| `--mg-text-xs` | 11 | 10.5, 11, 11.5 | badges, table header, micro-labels, chart day |
| `--mg-text-sm` | 12 | 12, 12.5 | sub-detail, timestamps, card subtitles |
| `--mg-text-base` | 13 | 13, 13.5 | table cells, nav items, body copy |
| `--mg-text-md` | 14 | 14 | list-item titles, lead copy |
| `--mg-text-lg` | 15 | 15, 16 | card titles, brand wordmark |
| `--mg-text-xl` | 17 | 17 | topbar page title, patient-card figures |
| `--mg-text-2xl` | 21 | 21, 22 | feature-card display line |
| `--mg-text-3xl` | 24 | 24, 27 | page title, dashboard greeting |
| `--mg-text-4xl` | 26 | 26 | stat-card value |

Weights: 400 normal, 500 medium (nav, list titles), 600 semibold (card titles, table primary
cell, buttons, badges), 700 bold (brand, stat values, patient-card figures, eyebrows).

Letter-spacing: `-0.02em` on anything 17px and above; `+0.05em` on uppercase eyebrows and table
headers. Line-height is only ever set once in the comp (`1.3`, on a table cell); pick a body
default during implementation and record it here.

---

## 4. Geometry and layout

### 4.1 Radii

`4px` skeleton bars · `7px` badges, tags, checkbox · `9px` icon chips, inline buttons ·
`10px` nav items, controls, primary buttons · `12px` toggle track, preview area ·
**`15px` cards — the signature** · `50%` avatars and dots.

The comp also used 8px, 11px and 13px once or twice each; `design/tokens.css` folds them into
their neighbours and says so.

### 4.2 Layout constants

| Constant | Value |
|---|---|
| Sidebar width | `248px`, fixed, non-collapsing |
| Topbar height | `66px` |
| Content padding | `28px` top, `30px` sides, `48px` bottom |
| Grid / stack gutter | `15px` — used for **every** gap in the product |
| Card padding | `17–21px`; standardise on `20px` |
| Dashboard split | `1.6fr / 1fr` |
| Report builder split | `1.3fr / 1fr` |
| Stat row | 4 equal columns |
| Patient grid | 3 equal columns |
| Two-up panels (sharing, notifications, settings) | `1fr 1fr` |

Table columns are declared per record kind as a `grid-template-columns` `fr` string — see §4.4.

### 4.3 Component inventory

Seventeen components carry the entire product:

1. **Sidebar** — brand lockup, grouped nav, user card pinned to the bottom
2. **Topbar** — page title, patient switcher, search, notification button, add button
3. **Stat card** — label, value + unit, status badge
4. **List card** — titled card, "View all" affordance, hairline-separated rows
5. **Bar chart** — 7 bars, percentage heights, day labels, no axes
6. **Feature card** — brand-filled, serif display line, two inline actions
7. **Activity feed** — bulleted, two-line, timestamped
8. **Patient card** — avatar, name, relation · age, three-figure footer
9. **Data table** — configurable columns, uppercase header, two-line cells
10. **Status badge** — six semantic pairs (§2.2)
11. **Upload dropzone** — dashed, glyph, constraint copy
12. **Tag chip** — brand tint, used inline in table cells
13. **Person row** — avatar, name, sub-detail, right-aligned badge
14. **Selectable row** — checkbox, label, detail; selected state tints the whole row
15. **Preview pane** — hatched, skeleton bars
16. **Toggle** — 40×23 track, 19px thumb
17. **Settings card** — title, subtitle, label/value/action rows

### 4.4 The record table is one component, thirteen configurations

The comp's most important structural insight, and it matches the suite's own architecture
exactly: **one generic table renders every record kind.** Each kind supplies a title, a subtitle,
an add-button label, a column list and an `fr` string; rows are arrays of cells, and each cell is
one of three kinds — `cell` (semibold, primary, optional second line), `muted` (regular,
secondary), or `badge` (one of the six status pairs).

That is precisely the shape of `internal/records`' kind registry and the six-operation record
route family in [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) §2.2. The design and the architecture agree without having
been made to, which is a good sign for both. The per-kind column specifications in the comp are
worth keeping verbatim as the default column set for each kind's list page.

---

## 5. Reconciliation with the specification suite

### 5.1 Coverage — 20 of 58 page routes

**Covered** (the comp renders these): `/` · `/patients` · `/medications` · `/conditions` ·
`/allergies` · `/vitals` · `/lab-results` · `/treatments` · `/procedures` · `/immunizations` ·
`/injuries` · `/symptoms` · `/encounters` · `/equipment` · `/insurance` ·
`/emergency-contacts` · `/documents` · `/sharing` · `/reports` · `/settings`.

**Not covered — 38 routes plus 3 error views**, grouped:

| Group | Count | Routes |
|---|---|---|
| **Every detail page** | 19 | `/{kind}/{id}` for all 15 record kinds, plus `/patients/{id}`, `/practitioners/{id}`, `/facilities/{id}`, `/reports/{id}` |
| Authentication | 5 | `/login`, `/register`, `/forgot-password`, `/reset-password/{token}`, `/verify-email/{token}` |
| Reference entities | 2 | `/practitioners`, `/facilities` |
| Phase 003 standalone | 3 | `/tags`, `/search`, `/timeline` |
| Phase 004 | 1 | `/labs/trends` |
| Phase 005 | 2 | `/invitations`, `/invite/{token}` |
| Phase 006 operator surface | 5 | `/admin`, `/admin/audit`, `/admin/backups`, `/admin/users`, `/exports` |
| Family history | 1 | `/family-history` |
| Error views | 3 | not found, sign-in required, something went wrong |

**The detail-page gap is the significant one.** Nineteen of the suite's 58 pages are detail
pages, the comp draws none of them, and a detail page is where the actual clinical content of a
record lives — the linked records, the attachments, the audit-visible history. The list-page
design does not imply the detail-page design. This needs design work before phase 001 builds
`/medications/{id}`, which is one of that phase's nine pages.

### 5.2 Structural conflicts with [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) §3.0

The comp's shell and the specified shell are different shapes. §3.0 wins; these are the deltas an
implementer must resolve **before** writing `shell/layout.templ`.

| # | Comp | Specified shell (§3.0) | Resolution |
|---|---|---|---|
| 1 | `<header>` nested **inside** `<main>` | `<header id="app-header">` a sibling of `<main>`, `role=banner` | **Move it out.** A `<header>` descended from `<main>` does not map to `role=banner` at all — it is a generic sectioning header. As drawn, the gate's `banner` assertion fails on all 58 pages. |
| 2 | No `<footer>` anywhere | `<footer id="app-footer">`, `role=contentinfo`, carrying version and build stamp | Add it. The gate asserts `contentinfo` on every page. |
| 3 | `<nav>` with no accessible name | `<nav id="primary-nav" aria-label="Primary">` | Add `aria-label="Primary"`. The gate asserts `navigation[name="Primary"]`. |
| 4 | Nav lives in a 248px left `<aside>`; a separate 66px topbar holds the switcher | Nav inside the header, switcher inside the nav | **Keep the comp's sidebar** — it is better for 21 destinations than a horizontal bar, and §3.0 does not mandate visual placement. But the `<aside>` must not be the navigation landmark itself (an `<aside>` is `role=complementary`); put the `<nav aria-label="Primary">` inside it, and let the switcher live in the topbar as drawn. §3.0's assertion is about the landmark existing with that name, not about its position. |
| 5 | Patient switcher is a static `<div>` ending in `▾` | `combobox`, accessible name **"Active patient"**, asserted on every authenticated page with no phase ceiling | Build a real combobox. This is the single most-asserted selector in the suite. |
| 6 | No `#error-banner`, no `#toast` | `#error-banner` (`role=alert`, `aria-live=assertive`) and `#toast` (`role=status`, `aria-live=polite`), both outside every landmark and every patch target | Add both. Phase 005's notice stream patches into `#toast`. |
| 7 | No skip link | `<a href="#main">Skip to main content</a>` first in the body | Add it. |
| 8 | No page-level landmark; page titles are styled `<div>`s | Every page asserts its own `region[name="…"]` or `article[name="…"]` **and non-empty** | Every page body becomes a `<section aria-label="…">` or `<article aria-label="…">` with the exact literal from §3.1. |

### 5.3 Features drawn that MediGo has declined to build

Each of these is in the comp and has **no home** in any of the six phases. None is a defect in
the design — the comp was drawn from MediKeep, and MediGo deliberately scoped some of MediKeep
away. But an implementer working from the comp will build them by accident unless it is written
down.

| Drawn | Status in the suite |
|---|---|
| **Entire "Notifications" screen** — Discord, SMTP, Gotify and webhook channels, per-event delivery toggles | **Explicitly out of scope, twice.** 005: "Delivery over chat services, push services or arbitrary webhooks, digests, and per-event delivery preferences are not part of this phase **and are not planned for one**." 006 repeats it. Notices are in-application and by invitation email only. **Do not build this screen.** |
| **"Next appointment" card** with Reschedule / Details | MediGo's `encounters` record visits that **already happened**. There is no scheduling, no booking, no calendar. The comp's most prominent dashboard element is a feature the product does not have. |
| **"Today's medications"** with dose times, and "2 medication reminders" in the greeting | **Orphaned — see §5.4.** Phase 001 defers medication reminders to "a later phase"; no later phase claims them, and 005 and 006 both rule out the delivery mechanism they would need. |
| **"Next refill · 9 days"** stat card | No refill tracking anywhere in the suite. |
| **Dropzone: "auto-parses lab results"** | Phase 004 stores attachments and records lab results; it does not parse, OCR or extract anything from an uploaded document. |
| **Patient card "alerts" counter** | No alert concept exists. |
| **Share level "Full"** | The suite defines exactly two levels, `view` and `edit`. Three levels appear in the comp. |
| **Settings: "Paperless integration · Connected"** | No such integration. |

**Correctly matched, worth noting:** the comp's *"Family history sharing — View-only"* panel
matches phase 005 exactly, including that a family-history share may not be created at `edit`
level. And *"Single sign-on · Google · connected"* in Settings matches the OAuth2/SSO work now
allocated to phase 006.

### 5.4 A gap this import surfaced: medication reminders belong to no phase

Phase 001's Assumptions defer medication reminders with a reason that presumes an owner:

> Reminders and notifications for medications. Recording that a medication should prompt somebody
> is only useful once there is something to deliver the prompt; **both arrive together in a later
> phase.**

No later phase claims either half. Worse, the two phases that would plausibly own delivery close
the door on it:

- 005: "Delivery over chat services, push services or arbitrary webhooks, digests, and per-event
  delivery preferences are not part of this phase **and are not planned for one**."
- 006: "Chat services, push services, webhooks and digests are not added here."

So phase 001 defers a feature to a successor that does not exist, and the design — drawn from
MediKeep, which has the feature — puts it on the dashboard as one of four stat cards and one of
two primary panels. This is the same shape of defect as the orphaned password-reset flow that the
cross-artifact analysis caught: a phase hands work forward and nobody catches it.

**This needs a decision, and it is a product decision, not an editorial one:**

- **Strike it.** Amend 001's Assumptions to say reminders are out of scope for the suite, not
  deferred within it, and remove the dashboard affordances from the design. Consistent with 005's
  and 006's position, and honest.
- **Schedule it.** Give a phase the medication schedule model, the "due today" query and
  in-application-only reminders — which 005 already permits, since it rules out *external*
  delivery but not in-application notices.

Until it is decided, the dashboard cannot be built as drawn: two of its four stat cards
("Next refill", "Open alerts") and its "Today's medications" panel have no data behind them.

---

### 5.5 Nav structure versus the route inventory

The comp's sidebar has 21 destinations in 4 groups. Adopt the grouping — it is good, and it
matches the suite's own phase boundaries more closely than anything in [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) §3.1
currently does:

- **Overview** — Dashboard, Patients
- **Health records** — Medications, Conditions, Allergies, Vitals, Lab Results, Treatments,
  Procedures, Immunizations, Injuries, Symptoms, Encounters, Equipment
- **Profile** — Insurance, Emergency Contacts, Documents
- **Tools** — Report Builder, Sharing, Notifications, Settings

Changes required: **drop Notifications** (§5.3); **add Practitioners and Facilities** to Profile
or a new Directory group (phase 002 ships four pages the comp's nav cannot reach); **add Tags,
Search, Timeline** (phase 003) and **Lab trends** (phase 004); **add the operator surface**
behind an admin-only condition (phase 006). Search is drawn as a topbar field, which is a
reasonable home for it — but `/search` is a real page route with its own landmark, so the field
must navigate to it.

---

## 6. What implementation must add — the structural checklist

Ordered by the phase that first needs it. Nothing here is optional under Principle VIII.

**Phase 001, shell:**
- [ ] Skip link as the first focusable element
- [ ] `<header id="app-header">` **outside** `<main>` (§5.2 #1)
- [ ] `<nav id="primary-nav" aria-label="Primary">` inside the sidebar `<aside>`
- [ ] `<main id="main" tabindex="-1">`
- [ ] `<footer id="app-footer">` with version and build stamp
- [ ] `#error-banner` and `#toast`, outside every landmark and patch target
- [ ] Real `<a>` elements for every navigation destination — the comp uses `<button>`, which
      breaks middle-click, "open in new tab", and the browser's own history semantics
- [ ] A real combobox for the patient switcher, accessible name "Active patient"
- [ ] Focus-visible ring, defined once, on every interactive element (the comp defines none)

**Phase 001, every page:**
- [ ] One `<h1>` per page, and a heading outline that does not skip levels — the comp has zero
      headings
- [ ] Page body wrapped in `<section aria-label="…">` / `<article aria-label="…">` using the
      exact literal from [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) §3.1
- [ ] `<title>` matching the gate's expectation

**Phase 001, record list pages:**
- [ ] Real `<table>` / `<thead>` / `<th scope="col">` / `<td>` for the record table. The comp's
      CSS grid gives a screen reader an undifferentiated stream of text with no column
      association. Visual identity is preserved with `display:grid` on table elements if needed.
- [ ] Sort and filter controls as real form controls

**Phase 001, forms:**
- [ ] `<form>`, `<label for>`, `<input>` throughout — the comp contains none of these
- [ ] Error text associated by `aria-describedby`
- [ ] Auth pages designed (§5.1) — 5 of phase 001's 9 pages have no comp

**Phase 003:**
- [ ] Tags, Search and Timeline pages designed
- [ ] Search field in the topbar navigates to `/search`

**Phase 004:**
- [ ] Trend chart accessible alternative. The comp's bar chart is seven bare `<div>`s with no
      values, no axes and no text alternative; a lab trend that a person cannot read with a
      screen reader fails the phase's own accessibility posture.

**Phase 006:**
- [ ] Operator surface designed (5 pages)
- [ ] Report detail page designed

**All phases:**
- [ ] Responsive behaviour. The comp is a fixed 1320×880 desktop frame — `height:100vh`,
      `overflow:hidden`, a fixed 248px sidebar and 4-column grids. **The gate runs every page at
      390×844 as well as 1440×900.** No mobile layout exists for anything. This is the largest
      single piece of missing design work after the detail pages.

---

## 7. Open design decisions

None of these blocks phase 001 from starting; each blocks something later.

1. **Dark appearance.** The comp defines a single light palette. Guessing a dark palette for a
   medical record is worse than shipping only light — status colours carry meaning here and a
   naive inversion breaks the amber/red distinction. Decide explicitly.
2. **Focus, hover, active, disabled, and field-error states.** The comp is static and defines
   none. They must be designed once, centrally, before the shell is built.
3. **Empty states.** [`SHARED-DESIGN.md`](./SHARED-DESIGN.md) §3.1 requires a shared `@EmptyState(title, body, action)`
   inside every page landmark so the landmark assertion holds on a freshly seeded instance. The
   comp draws no empty state for any screen.
4. **Density at 5,000 records.** The comp shows tables of 3–6 rows. Phase 001's own scale target
   is 5,000 medications on one account, and phase 003's is ~50,000 records per patient. Pagination
   and virtual scrolling are undesigned.
5. **Detail-page layout** (§5.1) — the biggest gap.
6. **Mobile** (§6) — the second biggest.
7. **The serif's scope.** Newsreader is used for page titles and three specific moments. Write
   down the rule, or it will drift into being used everywhere or nowhere.

---

## 8. Repository integration

`medigo/design/` is reference material: the comp, the quarantined preview runtime and the token
file. So is the whole of `medigo/specs/` and `medigo/.specify/` — roughly **1.2 MB of markdown**
after the design dossiers moved in on 2026-08-27. **No build stage reads any of it**, and it must
be excluded from the Docker build context the way `arc-ui/docs/` and `arc-ui/chart/` already are.

Phase 001's task T011 adds `!medigo/` to the monorepo allowlist in `/.dockerignore`; it must also
add:

```
medigo/design/
medigo/specs/
medigo/.specify/
```

Two reasons, and the first is the sharp one. `design/source/support.js` is **57 KB of React**, and
without an exclusion it ships inside the build context of an application whose constitution
forbids React outright. Nothing would import it, but it has no business being there. The second is
ordinary hygiene: 1.2 MB of specification uploaded to the build daemon on every image build, all
of it invalidating layer cache for changes that cannot affect the binary.

---

## 9. Changelog

| Date | Change |
|---|---|
| 2026-08-27 | Imported from Claude Design; tokens extracted; reconciled against the six-phase suite. No implementation. |
