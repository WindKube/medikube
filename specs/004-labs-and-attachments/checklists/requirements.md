# Specification Quality Checklist: Labs and Attachments

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
changed as a result.

**Content Quality.** The specification was searched for the name of every technology this
project has settled on — the language, the embedded framework, the templating and
hypermedia libraries, the stylesheet tool, the logger, the storage engine, the tracing,
metrics and error-reporting libraries, the command-line library, the browser-automation
tool, and the assertion library. None appears. It was searched again for request methods,
address paths, stored-data structure names, file paths, and the vocabulary of the interface
description format. None appears. Two phrasings failed the first pass and were rewritten:
"reporting every offending field in the same response" became "…in the same submission"
(FR-007), and "zero browser console errors" became "zero browser errors" (FR-082), matching
the wording phase 001 established.

Four terms were kept deliberately after consideration. *Landmark* — the accessibility
structure a screen-reader user navigates by, which is what the constitution's browser check
asserts and which a stakeholder can verify with a screen reader; phase 001 kept it for the
same reason. *Collection date* — specimen collection is clinical vocabulary, not a
stored-data structure, and every occurrence in this document is that clinical sense.
*File format names* (PDF, JPEG, PNG, WebP, HEIC, TIFF, GIF, plain text, comma-separated
values) — a stakeholder can name the kinds of file they want to attach and can verify the
rule by trying one; naming them is what makes FR-052 and SC-012 testable. *Bytes and
megabytes* — the unit a size limit is expressed in and the unit in which "identical to what
was uploaded" is checked.

**Requirement Completeness.** No [NEEDS CLARIFICATION] markers were left. Six points where
the phase charter was silent were settled as informed decisions and recorded under
Assumptions rather than asked about: the 30-day default retention window, the 32-megabyte
default size limit and the default accepted-type list, previews for decodable images only
with a type icon for everything else, the absence of any per-account storage quota, the
refusal to convert between units, and the halves rule by which a direction is derived. None
is an architectural fork the locked decisions leave genuinely open; each has one defensible
default, and each is stated so that an operator or a reviewer can disagree with it in one
place rather than hunting for it.

Two scope boundaries were resolved rather than left ambiguous, and both are recorded.
Unified cross-record search is **not** in this phase: the phase charter does not list it,
and where the shared design contract's phase table places it here, the charter governs —
the same precedence phase 001 applied to its own record type. The automatic purge of
deleted documents **is** in this phase even though the shared design contract's phase table
places the purge with the later operations phase, because a retention window that nothing
enforces is not a retention window, and the constitution requires the window to be enforced
by a scheduled purge. Both departures are stated in the Assumptions section rather than
left for a planner to discover.

Every functional requirement was re-read against the question "could a tester fail this?".
Three were tightened as a result during drafting rather than after: FR-031 now publishes the
exact rule by which a direction is computed instead of saying "a documented rule"; FR-019
states which of a derived classification and a recorded status wins; and FR-065 states what
happens when a restore is attempted for a document whose record is gone, which was
previously only implied by an edge case.

**Requirement Coverage.** Every acceptance scenario traces to at least one functional
requirement, and every functional requirement is exercised by at least one acceptance
scenario, one edge case or one success criterion. The privacy and authorization
requirements (FR-072 through FR-080) are covered by acceptance scenarios in stories 1, 2
and 3, by the permission-boundary edge cases, and by SC-005, SC-006 and SC-008. The rule
that this phase is not complete until its acceptance scenarios exist as automated tests is
stated twice on purpose — as FR-083 and as SC-010 — because the task generator treats test
tasks as optional unless the specification demands them.

**Feature Readiness.** The five user stories are independently testable in the order given:
story 1 needs nothing from this phase, story 2 needs only a record of some kind from an
earlier phase, story 3 needs data of the shape story 1 produces but none of its code paths,
story 4 improves the accuracy of story 3 without being required by it, and story 5 depends
only on record kinds earlier phases delivered. Shipping story 1 alone leaves a usable
structured laboratory history; shipping story 2 alone leaves a usable per-record document
store.

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`
- All items pass as of the validation date above; nothing is ticked that is not true
