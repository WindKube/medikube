# Specification Quality Checklist: Walking Skeleton

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

## Validation Record

Validated 2026-08-26 against the specification as written. What was checked, and what
changed as a result:

**Content Quality.** The specification was searched for the names of every technology this
project has settled on — the language, the embedded framework, the templating and
hypermedia libraries, the stylesheet tool, the logger, the storage engine, the tracing,
metrics and error-reporting libraries, the command-line library, the browser-automation
tool and the image tooling. None appears. Neither do request methods, address paths,
stored-data structure names, nor file paths. Three phrasings failed the first pass and were
rewritten: "applies its schema" became "brings its stored-data structure up to date"
(FR-052, FR-058, FR-059 and two acceptance scenarios), and "every route by which a
medication can be addressed" became "every way a medication can be addressed" (SC-006).
The remaining uses of the word "route" are the clinical route of administration, which is
domain vocabulary, and one ordinary-English "a route back to the medication list".

Three terms were kept deliberately after consideration: *landmark* (the accessibility
structure a screen-reader user navigates by — it is what the constitution's browser gate
asserts and a stakeholder can verify it with a screen reader), *deployable image* (the
delivery artefact, named without naming any tool that produces it), and *scripting enabled*
(a browser capability a person must know about, named without naming the language).

**Requirement Completeness.** No [NEEDS CLARIFICATION] markers were left. Every point where
the phase charter was silent was settled as an informed decision and recorded under
Assumptions instead: self-registration closed by default, the minimum password length, the
default session lifetime, the default audit retention, English-only text, the decision to
build password recovery by email and email confirmation here rather than defer them, and the
three capabilities held back for later phases (external sign-in in phase 006, reminders, and
reference data on a medication). None of these is an architectural fork; each
has a defensible default, and recording them as assumptions is what the specify skill asks
for in preference to a marker.

All 77 functional requirements use MUST, name a single behaviour, and can be failed by an
observation. Requirements that would otherwise be vague were given a number: the free-text
limits are "documented per-field" and enforced with a message naming the field (FR-017), the
live view holds "at least one hour" (FR-030), passwords are "at least 8 characters"
(FR-004). The sixteen success criteria carry a number, a percentage or a duration, and each
is expressed in something a person or an operator can observe — time to complete a task,
pages that pass a check, occurrences found in an operational record — rather than in an
internal unit.

**Bounded scope.** The Assumptions section names what is deliberately absent and where it
goes: patients distinct from the account holder in phase 002, shared access in phase 005,
and files, labs, reporting and the operator dashboard in later phases. Three forward
references appear in the body itself so a reader is not surprised — under the Medication
entity, in the deletion edge cases, and in the concurrent-edit edge case.

**Tests.** The specification demands its own tests in its own words rather than leaving it
to the task generator: FR-068 requires every acceptance scenario in the document to exist as
an automated test before the phase is complete, FR-069 requires authorization tests on both
sides of every operation touching clinical data, FR-066 and FR-067 require the automated
browser check over every user-facing page derived from the application's own inventory, and
FR-072 requires that check to be proven capable of failing. SC-003, SC-004, SC-009 and
SC-010 restate the same demands as measurable outcomes.

**Privacy and authorization.** Every specification for this product must address who may see
the data, what happens when somebody who may not tries, and what is recorded about the
attempt. User Story 3 is dedicated to it, FR-032 through FR-042 state it as requirements,
SC-005 and SC-006 make it measurable, and the Edge Cases section covers the permission
boundary explicitly.

**Known soft spots, recorded rather than hidden.**

1. SC-001 (a first medication recorded in under 3 minutes), SC-008 (a running instance in
   under 10 minutes) and SC-013 (a health judgement in under 30 seconds) are human-timed
   criteria. They are measurable and they are honest targets, but verifying them needs a
   person with a stopwatch rather than an automated check. They are stated as acceptance
   targets, not as build gates.
2. FR-057 ("a single occurrence MUST NOT be reported more than once") is testable for the
   failure paths this phase actually has, but it is a rule about the whole system's
   behaviour and will need re-checking whenever a new reporting path is added. That is a
   standing obligation, not a defect in this specification.
3. FR-073 through FR-077 (recovery and confirmation by email) are the one requirement group
   whose behaviour depends on something outside the instance — the operator having configured
   outgoing mail. FR-076 is written so that the unconfigured case is itself a specified,
   testable behaviour rather than an untested branch, but the happy path can only be proven
   end to end against a mail sink. That is recorded as a planning obligation.
4. The phase charter and the shared design contract disagree about which record type comes
   first. This is recorded openly in Assumptions rather than papered over: the charter
   governs, medications is the first record type, and the contract's own worked example of a
   complete vertical slice is medications, so the two are reconcilable in substance.

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`.
- All items pass as of 2026-08-26, and were re-checked on 2026-08-27 after password recovery
  by email and email confirmation moved into this phase (cross-artifact finding H7): the five
  new requirements FR-073–FR-077, the four new acceptance scenarios on User Story 2, the new
  edge-case group and SC-016 were read for implementation leakage and for testability. No
  technology name, address path or request method entered the specification with them.
- The soft spots above are recorded for the planning phase to keep in view; none of them
  blocks planning.
