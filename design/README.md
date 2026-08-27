# design/

Reference material imported from Claude Design. **Nothing here is wired into a build, and no
build stage reads this directory.**

| Path | What it is |
|---|---|
| `source/MediKeep.dc.html` | The comp, verbatim as imported. 21 screens driven by a `screen` state value. |
| `source/support.js` | **Quarantined.** The Claude Design preview runtime — React. See below. |
| `tokens.css` | Colour, type, radius and layout tokens extracted from the comp. Imported by nothing. |

The specification that reconciles this design against the six-phase suite is
[`../specs/DESIGN.md`](../specs/DESIGN.md). Read that first — it records what the design covers
(20 of 58 page routes), where it conflicts with the specified shell, and which drawn features
have no home in the product.

## Re-rendering the comp

Open `source/MediKeep.dc.html` in a browser. It loads `support.js` alongside React from the page
context; if it renders blank, it is because React is not present — the file was authored to run
inside Claude Design's own harness. To browse the design as intended, use the source project:
<https://claude.ai/design/p/8811aba8-5354-421e-b27c-1c24e6a14751?file=MediKeep.dc.html>

## Why `support.js` is quarantined and not deleted

It is kept so the comp is self-describing — the `<x-dc>`, `sc-for`, `sc-if` and `DCLogic`
constructs in the HTML are meaningless without it.

**React is a forbidden dependency** under the constitution's Technology Constraints. This file
must never be vendored, imported, served, bundled, or copied into `internal/web/static/`. It
carries no design content — three hex colours, all of them its own error-overlay chrome.

MediGo's front end is templ + Datastar + Tailwind. Nothing in this directory changes that.

## Docker

`medigo/design/` must be excluded from the monorepo build context — see `specs/DESIGN.md` §8.
It is reference material in the same category as `arc-ui/docs/` and `arc-ui/chart/`.
