# Specification Quality Checklist: Patient Core

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

## Validation Notes

Checked on 2026-08-26 against the written spec, item by item.

- **No implementation details**: verified by reading the spec and by scanning it for the
  vocabulary the phase brief forbids — programming languages, frameworks, libraries, storage
  engines, request methods, resource paths, collection or table names, file paths. Two
  borderline phrases were found on the first pass and rewritten: a reference to upstream's
  specialty "table" became "list", and "common web image formats" became "common photographic
  image formats". The remaining matches for the scan pattern are the ordinary English verb
  "delete" and the noun "account".
- **Non-technical readability**: the spec speaks of people, account holders, charts, directories
  of practitioners and places of care, and the person currently in view. Where a technical
  mechanism is unavoidable — optimistic concurrency, cursor-stable paging, content-sniffed file
  types, per-request authorization — it is expressed as an observable behaviour (FR-011, FR-053,
  FR-008, FR-041) rather than as a mechanism.
- **Testable and unambiguous**: every functional requirement uses MUST and names an observable
  outcome. FR-004, FR-011, FR-022, FR-024, FR-038, FR-042 and FR-051 each describe a refusal
  with a stated consequence, which is what makes them falsifiable.
- **Measurable, technology-agnostic success criteria**: SC-001 through SC-014 are stated in
  minutes, seconds, counts, and percentages of user-visible outcomes. SC-004 states two seconds
  for a 50,000-record chart rather than a response-time budget for a named component. SC-013
  refers to "the project's automated browser check at both the desktop and mobile sizes it
  defines" without naming the tool.
- **Acceptance scenarios**: six prioritised user stories (P1–P6), each with an Independent Test
  line and Given/When/Then scenarios — 8, 6, 6, 6, 6 and 6 respectively, 38 in total. Each story
  is independently demonstrable: P1 alone delivers a private family profile directory.
- **Edge cases**: fourteen are documented, covering empty states (a brand-new account, a person
  with no records, a person with only a name and a birth date), concurrent edits and concurrent
  switching, partial data, permission boundaries and identifier guessing, deletion of a
  referenced directory entry, deletion of the person another window is viewing, very large
  charts and many people, paging while data changes, rejected uploads, and what happens to
  everything owned when an account is closed.
- **Scope bounded**: the Assumptions section separates what phase 001 already delivered, the
  decisions taken here, and what phases 003, 004, 005 and 006 will change. Sharing, the
  remaining record types, laboratory results and attachments are referenced as later phases and
  are not specified.
- **Privacy and authorization**: required by the project's constitution for every specification
  in this application. Covered by FR-041 through FR-047 (who may see the data, what a refused
  attempt looks like, and what is recorded about it), reinforced by the last scenario of user
  stories 1, 2 and 6, and measured by SC-005, SC-008 and SC-009.
- **Tests explicitly required**: FR-054, FR-055 and FR-056, and SC-012 and SC-013, state that
  the phase is not complete until the acceptance scenarios exist as automated tests, until every
  capability touching a person's data has a test proving a non-owner is refused, and until every
  new screen is covered by the automated browser check. This is deliberate: without it a task
  generator treats test tasks as optional.

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`.
  Nothing is marked incomplete.
- No [NEEDS CLARIFICATION] markers were needed. Five points that could have become questions —
  whether practices and pharmacies are one concept, whether specialties are user-extensible,
  whether reference data is per account or global, whether a record can be re-attributed, and
  whether ownership can be transferred — were decided against the shared design contract and
  recorded under Assumptions with the reasoning.
