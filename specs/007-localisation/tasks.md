# Tasks: Localisation

**Input**: Design documents from `/specs/007-localisation/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/catalogue.md

**Tests**: mandatory (constitution Principle III; spec FR-017). Every test task is written first
and fails before the implementation task that makes it pass. Every task cites the requirement
or scenario it satisfies.

**Organization**: by user story. US1 is the bulk of the phase and is split by screen family so
the extraction can run in parallel on disjoint files.

## Format: `[ID] [P?] [Story] Description (citations)`

- **[P]**: parallelisable — different files, no dependency on an unfinished task
- **[Story]**: US1, US2, US3
- **[EDIT]**: modifies an existing file; everything else is new

---

## Phase 1: Setup

- [ ] T001 Amend `.specify/memory/constitution.md` to 1.4.0: SYNC IMPACT REPORT entry, `go-i18n/v2` admitted under Cross-cutting confined to `internal/i18n`, version line (plan Constitution Check; FR-014)
- [x] T002 `go get github.com/nicksnyder/go-i18n/v2@v2.6.1` and `golang.org/x/text@v0.41.0` as direct requires; `go mod tidy`; confirm `internal/architecture/forbidden_deps_test.go` still passes and no cgo entered (FR-014)
- [x] T003 [EDIT] `.golangci.yml`: depguard rule — `github.com/nicksnyder/go-i18n/**` allowed only under `internal/i18n`; `golang.org/x/text/**` allowed under `internal/i18n` only; add a failing-import test case in `internal/architecture/` proving the rule fires (plan Structure Decision)
- [x] T004 [P] [EDIT] `Taskfile.yaml`: `test:i18n` runs `go test -count=1 ./internal/i18n/...`; `check` depends on it (plan IX)

---

## Phase 2: Foundational — the package, the seam, the gates

**Purpose**: everything a story needs to translate a single phrase end to end.

### Tests first

- [x] T005 [P] Failing unit tests `internal/i18n/i18n_test.go`: `Resolve` order (account locale wins over `Accept-Language`; `Accept-Language: pl-PL,en;q=0.8` → `pl`; unknown → `en`; region stripped; unsupported stored locale → `en`), `T` on a missing id in `pl` falls back to the English phrase, `T` with no `Localizer` on ctx is English, `N` returns each Polish form for counts 1, 2, 5, 22, 0 (FR-003, FR-008, FR-009, US1-8, US2-2; Edge Cases)
- [x] T006 [P] Failing build-time test `internal/i18n/catalogue_test.go`: for every `locales/active.*.toml`, the id set equals `active.en.toml`'s — a missing id fails naming `(language, id)`, a surplus id fails naming `(language, id)`; every plural message in `pl` defines `one`, `few`, `many`, `other` (FR-011a, FR-011b, US3-2, US3-3, SC-004)
- [x] T007 [P] Failing build-time test `internal/i18n/reference_test.go`: scans `internal/web/**/*.templ` and `internal/web/**/*.go` (not `*_templ.go`) for `i18n.T(ctx, "` / `i18n.N(ctx, "` literals plus every id producer registered in `i18n.KnownDynamicIDs()`, and asserts each exists in `active.en.toml`, failing with `file:line: id` (FR-011c, US3-4, SC-004)
- [x] T008 [P] Failing test `internal/i18n/supported_test.go`: `Supported()` is derived from the embedded directory — a temporary `active.xx.toml` fixture (via an `fs.FS` injection point) makes `xx` appear with no other change (FR-010, US3-1, SC-003)

### Implementation

- [x] T009 `internal/i18n/i18n.go`: `//go:embed locales/*.toml`; `Bundle` built once at package init with `language.English` default and TOML unmarshal registered; `Supported() []Language{Tag, Name}` from filenames, `Name` read from each file's `language.name` id; `IsSupported(string) bool`; `Resolve(accountLocale, acceptLanguage string) *Localizer`; `With(ctx, *Localizer) context.Context`; `From(ctx) *Localizer` (nil → English); `T(ctx, id string, data ...map[string]any) string`; `N(ctx, id string, count int, data ...map[string]any) string`; `KnownDynamicIDs() []string` (D-04, D-05, D-07)
- [x] T010 `internal/i18n/locales/active.en.toml` skeleton: `language.name = "English"` and the ids Phase 2 itself needs (`nav.*`, `error.*`, `empty.*`, `action.*`, `confirm.*`); `active.pl.toml` with the same ids in Polish (`language.name = "Polski"`) (D-03; contracts/catalogue.md)
- [x] T011 [EDIT] `internal/web/page/shell.go`: `resolveLocale(e)` beside `resolveTheme(e)` (`e.Auth.GetString("locale")`, then `e.Request.Header.Get("Accept-Language")`); `RenderPage` sets `e.Request = e.Request.WithContext(i18n.With(ctx, l))` before rendering and passes `Lang: l.Tag.String()` into `DocumentProps` (FR-003, FR-007)
- [x] T012 [P] [EDIT] `internal/web/views/shell/props.go` + `layout.templ`: `Lang string`; `<html lang={ props.Lang }>` (FR-007, US1-6)
- [x] T013 [P] [EDIT] `internal/web/render.go` (or wherever `web.Render`/`web.Patch` build the ctx for non-page responses): the same `Localizer` is on ctx for Datastar patches and JSON, resolved by the same rule (Edge Cases: realtime patches; FR-012)
- [x] T014 [EDIT] `internal/web/errors.go`: `Message(ctx, code)` reads `i18n.T(ctx, "error."+code)`; `Failure.Code`, `Fields[].Field`, `Fields[].Code` untouched; HTTP test `internal/web/api/errors_locale_test.go` diffs an `en` and a `pl` 403 with `message` removed → byte-identical, and asserts the two `message`s differ (FR-012, D-09, SC-005)
- [x] T015 [EDIT] `internal/web/page/shell_test.go`: a Polish account's page carries `lang="pl"`, an anonymous `Accept-Language: pl` page carries `lang="pl"`, an English one `lang="en"` (US1-6, US2-1, US2-2)

**Checkpoint**: one phrase (`nav.timeline`) renders in Polish for a Polish account; the three gates pass on a two-file catalogue.

---

## Phase 3: User Story 1 — Use MediKube in my own language (P1) 🎯 MVP

**Goal**: every application-owned phrase on every page comes from the catalogue; Polish complete.

**Independent Test**: spec US1.

### The settings control

- [x] T016 [P] [US1] Failing HTTP tests `internal/web/api/me_locale_test.go`: `PATCH /api/v1/me {"locale":"pl"}` → 200 and the stored value; `{"locale":"xx"}` → 422 with field `locale`, code `invalid_value`; a Datastar settings submit answers the re-rendered settings form **in Polish** on the same response (FR-001, FR-002, US1-1, SC-002)
- [x] T017 [US1] [EDIT] `internal/service/identity/service.go`: `Config.SupportedLocale func(string) bool`; `Update` refuses a locale the predicate rejects with `domain.ValidationError{locale: invalid_value}`; `internal/web/api/wiring.go` passes `i18n.IsSupported` into `serviceidentity.Config`; unit test with a fake predicate (FR-001)
- [x] T018 [US1] [EDIT] `internal/web/page/settings.go`: `localeOptions(user.Locale)` via `optionsOf` over `i18n.Supported()`, labelled by each language's own `language.name`; `internal/web/views/settings/settings.templ`: the `<select name="locale">` beside theme with an `aria-label` from the catalogue (FR-002, US1-1)
- [x] T019 [US1] `e2e/settings-locale.spec.ts`: choose Polski, save, assert the settings page is Polish on the same load, reload → still Polish, sign out/in → Polish, switch back → English; both viewports (US1-1, US1-7, SC-002)

### Extraction — every family in parallel, disjoint files

Each task: replace every literal phrase in its files with `i18n.T(ctx, "<id>")` / `i18n.N(...)`, add the ids to `active.en.toml` with a `description`, and add the Polish to `active.pl.toml`. Go-side `Label:` values and enum-label functions return ids (D-06). Existing render tests keep passing because the default locale is English. Kind display names come from `kind.<enum>.one|other`. No `data-*` attribute, class, route, signal name, enum wire value or `basis` string is ever touched.

- [x] T020 [P] [US1] Shell, navigation, switcher, empty states, confirm dialog: `internal/web/views/shell/*.templ`, `shared/*.templ`, `components/*.templ`, `page/shell.go` nav labels (FR-005)
- [x] T021 [P] [US1] Sign-in, sign-up, recovery, verification, sessions, settings: `internal/web/views/auth/**`, `views/settings/**`, `views/errors/**`, `page/accounts.go`, `page/settings.go` titles and option labels (`themeOptions`, unit/date-format options) (FR-005)
- [x] T022 [P] [US1] Patients and directories: `views/patients/**`, `views/directory/**`, `page/patients.go`, `page/practitioners.go`, `page/facilities.go` (FR-005)
- [x] T023 [P] [US1] Records A–E: `views/records/{allergy,condition,emergencycontact,encounter,equipment}.templ` + `.go` labels and enum-label functions; `page/{allergies,conditions,emergencycontacts,encounters,equipment}.go` titles (FR-005)
- [x] T024 [P] [US1] Records F–M: `views/records/{familymember,immunization,injury,insurance,medication}.templ` + `.go`; matching `page/*.go` titles (FR-005)
- [x] T025 [P] [US1] Records P–V and links: `views/records/{procedure,symptom,treatment,vitals,links,coursemedications}.templ` + `.go`; matching `page/*.go` titles (FR-005)
- [x] T026 [P] [US1] Tags, search, timeline, dashboard/chart: `views/tags/**`, `views/search/**`, `views/timeline/**`, `views/overview/**`, `page/tags.go`, `page/search.go`, `page/timeline.go` — replace `strings.ReplaceAll(kind.Segment(), "-", " ")` with `i18n.T(ctx, "kind."+k.Enum()+".one")`, registered in `KnownDynamicIDs` (FR-005, D-06) — `views/overview/**` left untouched, out of this branch's assigned scope; see this task's own final report
- [x] T027 [P] [US1] Counts and durations: every "N records", "N days", "expires in N days" phrase becomes `i18n.N` with `one/few/many/other` in `pl`; a unit test lists the ids and renders 1, 2, 5, 22 (FR-008, US1-5)
- [x] T028 [US1] After T020–T027: `task test:i18n` passes with zero surplus/missing ids; grep gate `scripts/check-i18n-literals.sh` (added to `task lint:i18n`, run by `task lint`) fails on any `>[A-Z][a-z]+[ a-z]*<` or `placeholder="[A-Z]` / `aria-label="[A-Z]` literal in `internal/web/views/**/*.templ`, allow-listed only for user-data slots — prove it fails on one reintroduced literal (FR-005, US1-2)
- [ ] T029 [US1] Review pass of `active.pl.toml` by a Polish speaker recorded in the PR (declension of kind names in context, formal/informal register consistent — use the impersonal/formal register throughout, imperatives for actions: "Zapisz", "Usuń") (FR-015) — consistency pass done; native review pending; see `specs/007-localisation/polish-review.md`
- [x] T030 [US1] HTTP test `internal/web/page/locale_render_test.go`: for every registered page route's smoke URL, a Polish account's HTML contains none of the English page titles and none of the English nav labels; user data in the seed (diagnoses, names) is present verbatim (US1-2, US1-3, SC-001)
- [x] T031 [US1] HTTP test: a refused save for a Polish account carries Polish field explanations for the same field set as the English refusal (US1-4)
- [x] T032 [US1] `e2e/locale.spec.ts`: for one representative page per family (settings, patients list, a record list, a record detail, tags, search, timeline) as a Polish account: `html[lang="pl"]`, title in Polish, zero console errors, zero English nav labels; both viewports (FR-016, US1-2, US1-6, SC-001)

**Checkpoint**: US1 complete; gate green in English; Polish account sees no English.

---

## Phase 4: User Story 2 — Before I have an account (P2)

- [x] T033 [P] [US2] Failing HTTP tests `internal/web/page/anonymous_locale_test.go`: `/login`, `/register`, recovery with `Accept-Language: pl` → Polish title and `lang="pl"`; `Accept-Language: de` → English; `Accept-Language: pl-PL;q=0.9,en;q=0.8` → Polish (US2-1, US2-2; Edge Cases: region)
- [x] T034 [US2] Failing test `internal/web/api/signup_locale_test.go`: sign-up with `Accept-Language: pl` creates the account with `locale = "pl"` and the first page after sign-in is Polish; sign-in from `Accept-Language: en` for that account stays Polish (FR-004, US2-3, US2-4)
- [x] T035 [US2] [EDIT] `internal/service/identity/service.go` `Registration` gains `Locale string`, applied by `Register(ctx, actor, registration)`; the sign-up handler fills it from `i18n.From(ctx).Tag.String()`; `identity.DefaultLocale` remains the fallback (FR-004, D-10)
- [x] T036 [US2] `e2e/locale.spec.ts` [EDIT]: a browser context with `locale: 'pl-PL'` sees the Polish sign-in page; creates an account; the first signed-in page is Polish; both viewports (US2-1, US2-3)

---

## Phase 5: User Story 3 — Add the next language by adding one file (P3)

- [x] T037 [P] [US3] `internal/i18n/add_language_test.go`: with an injected `fs.FS` holding `active.en.toml` + a complete `active.de.toml`, `Supported()` lists `de`, `Resolve("", "de")` yields German, `settings.localeOptions` offers it — and nothing else in the repository changed (`git diff --stat` in the PR description) (FR-010, US3-1, SC-003)
- [x] T038 [P] [US3] Negative fixtures for T006/T007 under `internal/i18n/testdata/`: one file missing an id, one with a surplus id, one templ snippet asking for an undefined id; the tests run against fixtures and assert the exact failure text (US3-2, US3-3, US3-4, SC-004)
- [x] T039 [P] [US3] `active.en.toml` lint in `catalogue_test.go`: every message has a non-empty `description`; ids match `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`; no id contains its English text (US3-5, D-03)
- [x] T040 [US3] `specs/007-localisation/quickstart.md` §"Add a language" verified by actually doing it for `de` with three phrases in a scratch branch and deleting it; the commands in the doc are the ones run (US3-1)

---

## Phase 6: Polish & cross-cutting

- [ ] T041 [P] `internal/testsupport/phileak/exercise.go` [EDIT]: drive one Polish-account page and one `PATCH /me` locale change; assertion unchanged (FR-013; constitution VII)
- [ ] T042 [P] `docs/pocketbase-upgrade-checklist.md` and `CLAUDE.md` [EDIT]: "Localisation (phase 007)" — no literal English in templ, `i18n.T(ctx, ...)` only, ids never contain English, `task test:i18n`, how to add a language, what is never translated (FR-005, FR-012)
- [ ] T043 [P] `e2e/routes.gate.spec.ts` unchanged and green in English; `e2e/fixtures.ts` `title()` unchanged; confirm no e2e spec asserts a Polish string except `locale*.spec.ts` (FR-016)
- [ ] T044 `api/openapi.json` regenerated only if `Failure.message` description changed ("in the caller's language"); `task openapi:check` (FR-012)
- [ ] T045 Traceability: every FR-001..FR-017 and SC-001..SC-006 and every US scenario cited on a task above; scenario list: US1-1..8, US2-1..4, US3-1..5 (FR-017, SC-006)

---

## Dependencies

- Phase 1 → Phase 2 → US1 settings (T016–T019) and extraction (T020–T027, parallel) → T028–T032.
- US2 depends on Phase 2 and T021 (auth screens extracted).
- US3 depends on Phase 2 only; T037–T039 can run alongside US1.
- Phase 6 last.

## Pull requests (gh stack, on top of phase 003's #45)

1. `feat/007-spec` — this directory + constitution 1.4.0 (T001)
2. `feat/007-foundation` — T002–T015
3. `feat/007-us1-settings` — T016–T019
4. `feat/007-us1-extract-a` — T020, T021, T022
5. `feat/007-us1-extract-b` — T023, T024, T025
6. `feat/007-us1-extract-c` — T026, T027
7. `feat/007-us1-close` — T028–T032
8. `feat/007-us2` — T033–T036
9. `feat/007-us3` — T037–T040
10. `feat/007-polish` — T041–T045
