# Specification Quality Checklist: Reporting and Operations

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-08-27
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

Nine user stories (P1–P9), 137 functional requirements, 28 success criteria, 13 key
entities, 0 clarification markers. This specification was repaired rather than written
from nothing: an earlier draft stopped mid-sentence at FR-075 with no Success Criteria,
no Key Entities, no Assumptions and no phase-dependency notes. The user stories and the
first 75 requirements were kept, and six defects were corrected before this checklist
was completed:

1. **A whole capability specified twice, in two phases, with two different answers.**
   The draft re-specified the attachment trash — soft delete, the thirty-day window,
   restore, permanent removal, the scheduled purge and the failure handling — as FR-053
   to FR-063, and gave it a dedicated recovery page and its own user story at P8. Phase
   004 already specifies every one of those behaviours, and it puts recovery on the
   document library as a filter, not on a page of its own. The two specifications also
   disagreed about who may remove a document permanently. All eleven requirements and
   the story were removed. What this phase actually owns — the instance-wide count and
   byte total of documents awaiting removal, and the health of the scheduled work that
   removes them — is now stated in FR-053 to FR-063 and FR-056/FR-057 in particular,
   which point at phase 004's surface and forbid a second one.
2. **A user story replaced rather than deleted.** Removing the trash story would have
   left the phase with no story covering the retention windows this phase does own —
   produced documents, export archives and the activity trail. User Story 8 is now
   "Nothing lingers after its window", which is operator-facing, genuinely owned here,
   and independently testable by moving a clock rather than by re-testing phase 004.
3. **A requirement that contradicted a settled limitation.** The draft's edge case
   claimed non-Latin script is "rendered correctly in the produced document". It is not:
   the product cannot render every script faithfully in a produced document, and the
   settled answer is to count and state the affected entries on the first page while the
   export always carries the exact text. FR-006 and the edge case now say that. A
   specification that promises something the product will not do is worse than one that
   states the limitation.
4. **The read-audit asymmetry was implied but never stated.** The draft's FR-075 said
   reading the trail writes no entry while exporting it does, but nothing said the same
   about reading a person's records. FR-115 now states it in full — an entry is written
   only when the reader is not the owner — together with why, because the alternative
   builds a timeline of when a person read their own most sensitive results.
5. **Four whole requirement areas were missing.** The draft ended at the trail. The
   operator overview, account administration, backup and restore, privacy and
   accountability, scale, and verification and release hardening were all covered by
   user stories with no requirements behind them. FR-076 to FR-133 close that.
6. **Nothing in the draft asked for tests.** Neither the requirements nor any success
   criterion said the phase is incomplete without them, which would have caused the task
   generator to emit no test tasks at all. FR-124 to FR-133 and SC-021, SC-024 and
   SC-025 now say it explicitly, for this phase and for the finished product.

Notes on the items above, so that the ticks are honest rather than decorative:

- **No implementation details.** The specification names no language, library, protocol,
  verb, address, file format or storage mechanism. A produced report is "a single
  self-contained document that opens and prints on any device without the application";
  an export is "a single archive" containing "a machine-readable structured form" and
  "tabular files that open in a spreadsheet". "Backup", "archive", "restore" and
  "break-glass credential" are used as the operator-facing words they are, matching the
  phase 001 specification, which uses "backup" and "break-glass credential" identically.
  Two occurrences of a platform-specific word for the break-glass account, and one
  reference to a "live database", were found in the Assumptions during validation and
  rewritten.
- **Testable and unambiguous.** Fourteen requirements defer a number to documentation
  this phase must publish: the maximum records in one report (FR-010), the minimum
  readings to chart (FR-017), the maximum charts (FR-023), the retention windows for
  produced documents (FR-012), export archives (FR-047), the activity trail (FR-074) and
  deleted documents (FR-053), and the backup staleness threshold (FR-082). Each is
  testable as written, because each requires the value to exist, to have a published
  default, to be an operator setting, to be shown in the application (FR-087) and to be
  enforced (FR-054). The volumes and budgets that would otherwise be vague are pinned to
  numbers in SC-005, SC-016 and SC-023.
- **Acceptance criteria per requirement.** Acceptance scenarios are written per user
  story, and every requirement belongs to a story. The privacy (FR-114 to FR-120), scale
  (FR-121 to FR-123) and verification (FR-124 to FR-133) requirements are not restated as
  named Given/When/Then scenarios; each is an individually checkable MUST, each is
  measured by a named success criterion (SC-014, SC-015, SC-022, SC-016, SC-021, SC-023,
  SC-024, SC-025, SC-026, SC-027, SC-028), and the scale cases are additionally written out under
  Edge Cases.
- **Technology-agnostic success criteria.** SC-021 speaks of browser console errors,
  uncaught page failures and failed resource requests, and SC-005 and SC-016 state memory
  ceilings. The browser terms are the terms of the project's browser gate and are used
  identically in the phase 001 to 005 specifications; they describe what a person observes
  in a browser. The memory ceilings are what make "streams rather than assembling"
  checkable rather than aspirational, and a stakeholder can read one off a process
  monitor without knowing how the product is built.
- **Independence of the stories.** Story 1 stands alone and is a shippable slice: a
  person produces a document and hands it to a clinician. Stories 2, 5, 6, 7 and 8 are
  each independently shippable and independently testable. Stories 3 and 4 accelerate and
  enrich story 1 and need a report definition to exist, which is a test precondition
  rather than a dependency on story 1's whole surface. Story 9 establishes that
  everything before it is true and is, by construction, last.
- **External sign-in (FR-134 to FR-137), added 2026-08-27.** Cross-artifact finding H7 found
  that sign-in through an external identity provider was allocated to no phase at all: phase
  001 recorded it as deferred and no later phase claimed it. It lands here because it is a
  deployment integration that needs provider configuration, and this phase already owns the
  operator surface. The four requirements were checked for implementation leakage like the
  rest: they name no protocol, no provider, no endpoint and no library, and SC-028 makes the
  unconfigured case measurable rather than assumed.
- **Scope boundary.** Scheduled delivery of reports, notification channels, aggregate
  behavioural analytics, record-level soft delete, a second recovery surface for deleted
  documents, importing an export archive back into an instance, sharing a saved report,
  bespoke report design, and running as more than one process are each named under "What
  this phase deliberately does not do" with the reason. Because this is the final phase,
  a further section states what remains permanently open, including the script-rendering
  limitation, so that nobody discovers it after release.
- **Carried over from earlier phases and not re-specified.** The Assumptions list what
  phases 001 to 005 already deliver — the trail's entries, the browser gate and its page
  inventory, people and ownership, record kinds, laboratory components and units, the
  attachment trash in full, and sharing arrangements and their re-checking — and say in
  each case what this phase adds rather than restating the behaviour.
- **Constitutional load-bearing points.** This phase owns the audit trail's read surface,
  so the privacy principle is at its most exposed here: FR-068, FR-114 and FR-115 state
  that an entry records actor, action, target reference and time and never content, and
  that the reader does not fetch content to display beside an entry. FR-086 forbids
  content on any operator view, FR-088 and the "administrative tier is not the break-glass
  credential" decision keep the operator surface off the path to a person's records, and
  FR-119 and SC-022 hold the no-unconfigured-outbound-connection line for the finished
  product.
