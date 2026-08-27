# Specification Quality Checklist: Clinical Records

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-08-26
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

Ten user stories (P1–P10), 94 functional requirements, 18 success criteria, 16 key
entities, 0 clarification markers. Validated in one pass; two defects were found and
corrected before this checklist was completed:

1. **Implementation vocabulary in the edge cases.** "Every enumerated *field*…" was
   corrected to "Every enumerated *detail*…". A field is a storage idea; the
   stakeholder-facing statement is about the vocabulary a detail is chosen from.
2. **Implementation vocabulary in the assumptions.** "maintained as *columns* that can
   go stale" was corrected to "maintained as stored counts that can quietly go stale".

Notes on the items above, so that the ticks are honest rather than decorative:

- **Testable and unambiguous.** Several requirements defer a numeric bound to
  documentation the phase must produce — the maximum length of a note (FR-003), the
  plausible range of each measurement (FR-035), the plausible range of a year of birth
  or death (FR-054), the tie-break used for ordering (FR-007, FR-073, FR-076). Each is
  testable as written, because each requires the bound to exist, to be documented and to
  be enforced. What the number itself should be is a planning decision, not a
  stakeholder decision, and pinning it here would be guessing in the specification.
- **Acceptance criteria per requirement.** Acceptance scenarios are written per user
  story, and every requirement belongs to a story. A minority of requirements — chiefly
  the detail inventories for insurance (FR-044), equipment (FR-048) and vaccinations
  (FR-038) — are not restated as a named Given/When/Then scenario; each is nevertheless
  an individually checkable MUST, and the story it belongs to has scenarios covering the
  behaviour around it.
- **Technology-agnostic success criteria.** SC-014 speaks of browser console errors,
  uncaught page failures and failed resource requests. These are the terms of the
  project's constitutional browser gate and are used identically in the phase 001 and
  phase 002 specifications; they describe what a person observes in a browser, not how
  the application is built, and they are kept for consistency across the six phases.
- **Scope boundary.** Laboratory results, file attachments, sharing, notifications,
  reporting and export are named only as forward references, never specified. Family
  history is specified here deliberately, and the reason is recorded in Assumptions.

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`.
- No items are incomplete.
