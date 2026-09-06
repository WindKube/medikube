# Traceability: Localisation

Hand-authored (T045), modelled on `specs/003-clinical-records/traceability.md`'s shape.
`scripts/traceability.go` and `scripts/gen-traceability.sh` are each hardcoded to one earlier
phase's paths, same as `internal/architecture/traceability_test.go` is to `001`'s — every phase
so far has its own generator or its own hand-kept file; this one is hand-kept. Regenerate by
re-reading `tasks.md`'s citations after any FR/SC/task renumbering.

## Functional requirements

| Requirement | Tasks | Test |
|---|---|---|
| FR-001 | T016, T017, T018 | `internal/web/api/me_locale_test.go` |
| FR-002 | T016, T018 | `internal/web/api/me_locale_test.go` |
| FR-003 | T005, T011 | `internal/i18n/i18n_test.go`, `internal/web/page/shell_test.go` |
| FR-004 | T034, T035 | `internal/web/api/signup_locale_test.go` |
| FR-005 | T020–T028 | `scripts/check-i18n-literals.sh` (`task lint:i18n`) |
| FR-006 | T030 | `internal/web/page/locale_render_test.go` |
| FR-007 | T011, T012 | `internal/web/page/shell_test.go` |
| FR-008 | T005, T027 | `internal/i18n/i18n_test.go` |
| FR-009 | T005 | `internal/i18n/i18n_test.go` |
| FR-010 | T008, T037 | `internal/i18n/supported_test.go`, `internal/i18n/add_language_test.go` |
| FR-011 | T006 (a, b), T007 (c) | `internal/i18n/catalogue_test.go`, `internal/i18n/reference_test.go` |
| FR-012 | T013, T014, T042, T044 | `internal/web/api/errors_locale_test.go` |
| FR-013 | T041 | `internal/testsupport/phileak/phileak_test.go` (assertion unchanged; exercise extended) |
| FR-014 | T001, T002 | `internal/architecture/forbidden_deps_test.go` |
| FR-015 | T029 | `specs/007-localisation/polish-review.md` (manual review record, not automated) |
| FR-016 | T032, T043 | `e2e/locale.spec.ts`, `e2e/routes.gate.spec.ts` |
| FR-017 | T045 | this document |

## Success criteria

| Criterion | Tasks | Test |
|---|---|---|
| SC-001 | T030, T032 | `internal/web/page/locale_render_test.go`, `e2e/locale.spec.ts` |
| SC-002 | T016, T019 | `internal/web/api/me_locale_test.go`, `e2e/settings-locale.spec.ts` |
| SC-003 | T008, T037 | `internal/i18n/supported_test.go`, `internal/i18n/add_language_test.go` |
| SC-004 | T006, T007, T038 | `internal/i18n/catalogue_test.go`, `internal/i18n/reference_test.go`, `internal/i18n/testdata/*` |
| SC-005 | T014 | `internal/web/api/errors_locale_test.go` |
| SC-006 | T045 | this document (every scenario row below names a test) |

## Acceptance scenarios

| Scenario | Tasks | Test |
|---|---|---|
| US1-1 | T016, T018, T019 | `internal/web/api/me_locale_test.go`, `e2e/settings-locale.spec.ts` |
| US1-2 | T028, T030, T032 | `scripts/check-i18n-literals.sh`, `internal/web/page/locale_render_test.go`, `e2e/locale.spec.ts` |
| US1-3 | T030 | `internal/web/page/locale_render_test.go` |
| US1-4 | T031 | HTTP test named "a refused save..." in T031 (no filename given in tasks.md) |
| US1-5 | T027 | unit test named "lists the ids and renders 1, 2, 5, 22" in T027 (no filename given in tasks.md) |
| US1-6 | T012, T015, T032 | `internal/web/page/shell_test.go`, `e2e/locale.spec.ts` |
| US1-7 | T019 | `e2e/settings-locale.spec.ts` |
| US1-8 | T005 | `internal/i18n/i18n_test.go` |
| US2-1 | T015, T033, T036 | `internal/web/page/anonymous_locale_test.go`, `e2e/locale.spec.ts` |
| US2-2 | T005, T015, T033 | `internal/web/page/anonymous_locale_test.go` |
| US2-3 | T034, T036 | `internal/web/api/signup_locale_test.go`, `e2e/locale.spec.ts` |
| US2-4 | T034 | `internal/web/api/signup_locale_test.go` |
| US3-1 | T008, T037, T040 | `internal/i18n/supported_test.go`, `internal/i18n/add_language_test.go`, `quickstart.md` §5 (manually run, T040) |
| US3-2 | T006, T038 | `internal/i18n/catalogue_test.go`, `internal/i18n/testdata/*` |
| US3-3 | T006, T038 | `internal/i18n/catalogue_test.go`, `internal/i18n/testdata/*` |
| US3-4 | T007, T038 | `internal/i18n/reference_test.go`, `internal/i18n/testdata/*` |
| US3-5 | T039 | `internal/i18n/catalogue_test.go` |

## Gaps

- US1-4 and US1-5 name their test only by description in `tasks.md`, not by file — both tests
  exist (T031, T027 are ticked done) but tasks.md itself does not name the file, unlike every
  other row here. Not a missing test, a missing citation in `tasks.md`'s own prose; left as
  found rather than rewritten, since T045 fixes citation *gaps* (a requirement or scenario with
  no citing task), not prose style.
- US3-1's coverage includes a manual step (`quickstart.md` §5, T040): adding a language and
  proving nothing else changed was done once by hand in a scratch branch, not asserted by an
  automated test on every run. `internal/i18n/add_language_test.go` (T037) is the automated half
  of the same scenario.
- FR-015 (Polish reviewed by a person) has no automated test by nature — `polish-review.md`
  is the record T029 asks for.
