# Specification Quality Checklist: Localisation

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-09-06
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Validation record

Three user stories (P1–P3), 17 functional requirements, 6 success criteria, 3 key entities, 0
clarification markers. Validated in one pass against `spec.md` as committed; no defects found
that required correction.

Notes on the items above, so the ticks are honest rather than decorative:

- **No implementation details.** FR-014 requires "a widely used open-source library with plural
  support," which names a property, not a product — the spec never says `go-i18n`, TOML, or
  `golang.org/x/text`; those choices are `plan.md`'s and `research.md`'s to make. FR-012's "JSON
  API" and edge-case mentions of "the JSON API" are pre-existing stakeholder-facing vocabulary
  from `001-walking-skeleton`'s contract (the API is a product surface, not a code detail), the
  same treatment `003-clinical-records` gave its own contract terms.
- **Success criteria technology-agnostic.** SC-005 ("100% of JSON API codes… byte-identical")
  reads as a technical assertion but is stated at the level a non-technical reviewer already
  reads the rest of this project's specs at — "the API" is the same product-facing noun FR-012
  uses, and byte-identical is a plain-language way to say "the codes never change," which is the
  actual stakeholder concern (a client integration must not break when a user switches language).
- **Testable and unambiguous.** FR-008's "Polish's four forms… MUST all be exercised by at least
  one phrase in use" is a testable MUST without pinning which phrase; which phrase satisfies it
  is a planning decision (`data-model.md` §3, the `kind.*` namespace), not a stakeholder one, and
  pinning it here would be guessing in the specification — the same pattern 003's checklist
  recorded for its own deferred numeric bounds.
- **Acceptance criteria per requirement.** As in 003, acceptance scenarios are written per user
  story rather than one-to-one per requirement. FR-013 (diagnostic output stays English) and
  FR-015 (Polish ships complete, reviewed by a person) are not restated as their own
  Given/When/Then, but each is an individually checkable MUST and both are covered by the edge
  cases and by US1's independent test respectively.
- **Scope is clearly bounded.** The Assumptions section is doing real boundary-setting work here,
  not decoration: it excludes date/unit localisation (already has its own preference), RTL,
  region variants, and platform email templates by name, each with the reason it is out of scope
  — exactly the shape a reviewer needs to confirm the phase is not silently expected to also ship
  those.
