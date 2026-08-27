# Specification Quality Checklist: Sharing and Collaboration

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

Six user stories (P1–P6), 80 functional requirements, 20 success criteria, 6 key
entities, 0 clarification markers. Validated in one pass; four defects were found and
corrected before this checklist was completed:

1. **Implementation vocabulary describing the system being reimagined.** "ran patient
   sharing and family-history sharing as two tables, two routers, two invite paths" was
   corrected to name two separate *mechanisms*, each with its own way to invite and
   revoke. Tables and routers are storage and code ideas; the stakeholder-facing point is
   that there were two of everything.
2. **Implementation vocabulary in a success criterion.** "no cache to expire" was removed
   from SC-005. Whether anything is cached is a design question; the stakeholder-checkable
   statement is that ending access needs no sign-out and no periodic job.
3. **"Every route to that person"** in a story's Independent Test was corrected to "every
   way to that person", because *route* also names a technical artefact this
   specification must not mention.
4. **A requirement with no scenario.** FR-070 requires that a non-owner's opening of a
   record and downloading of a document be recorded. Nothing demonstrated it, so an
   acceptance scenario was added to User Story 1 stating what must be recorded and what
   must not appear in the entry.

Notes on the items above, so that the ticks are honest rather than decorative:

- **Testable and unambiguous.** Three requirements defer a value to documentation this
  phase must produce: the retention period for answered invitations (FR-033), the maximum
  length of a note (implicit in FR-009, inherited from the note rules of earlier phases),
  and the bounds of the settable lapse window (FR-017 states them — one hour to one year —
  and only the default of seven days is a choice recorded here). Each is testable as
  written, because each requires the value to exist, to be documented and to be enforced.
- **Acceptance criteria per requirement.** Acceptance scenarios are written per user
  story, and every requirement belongs to a story. The scale requirements (FR-074,
  FR-075) and the verification requirements (FR-076 to FR-080) are not restated as named
  Given/When/Then scenarios; they are individually checkable MUSTs, they are each
  measured by a named success criterion (SC-014, SC-018, SC-019), and the scale case is
  additionally written out under Edge Cases.
- **Technology-agnostic success criteria.** SC-019 speaks of browser console errors,
  uncaught page failures and failed resource requests. These are the terms of the
  project's constitutional browser gate and are used identically in the phase 001, 002 and
  003 specifications; they describe what a person observes in a browser, not how the
  application is built, and are kept for consistency across the six phases. SC-005 and
  SC-017 use "within 5 seconds" and "60 continuous minutes", which a stakeholder can time
  with a clock.
- **Independence of the stories.** Story 1 stands alone and is a shippable slice: one
  person shares one chart for reading and the recipient can read it. Stories 2 to 6 each
  add a separable capability, and each states an Independent Test. Stories 2, 3 and 6 need
  a grant to exist before they can be exercised, which is what Story 1 produces; that is a
  test precondition, not a dependency on the rest of Story 1's surface.
- **Scope boundary.** Real-time collaborative editing, organisation or practice accounts,
  public or link-only sharing, and sharing anything narrower than a chart or a relative's
  entry are named under "What this phase deliberately does not do" and are never
  specified. Reporting, export, the audit reader and the operator screens are named only
  as forward references into phase 006.
- **Privacy.** Every user story carries at least one scenario in which somebody is refused,
  and the requirements state who may see what, what a refusal must look like, what is
  recorded about access, and — for the pre-acceptance surface — what must never be
  disclosed at all. This is the phase most able to leak medical records, and the
  specification is written on that assumption.

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`.
- No items are incomplete.
