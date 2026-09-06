# Implementation Plan: Localisation

**Branch**: `007-localisation` | **Date**: 2026-09-06 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/007-localisation/spec.md`

## Summary

Every application-owned phrase moves out of the screens and into one catalogue file per
language (`internal/i18n/locales/active.<lang>.toml`), read through
`github.com/nicksnyder/go-i18n/v2`. A per-request `Localizer` is resolved once — the account's
stored `locale`, else the browser's `Accept-Language`, else `en` — placed on the request context
by `RenderPage`, and read by templ components through `i18n.T(ctx, id, ...)`. The set of shipped
languages is whatever files are embedded; three build-time tests hold every file to English's
key set and every key asked for in code to the catalogue. Polish ships complete. The account's
`locale` column, domain validation, store mapping and `PATCH /me` DTO already exist from phase
001 — this phase adds the settings control, the membership check and the consumers.

## Technical Context

**Language/Version**: Go 1.27 (unchanged; see [001's plan](../001-walking-skeleton/plan.md#technical-context)).

**Primary Dependencies** (all pinned; **one new direct dependency**, admitted by constitution 1.4.0):

| Module | Version | Used for |
|---|---|---|
| `github.com/nicksnyder/go-i18n/v2` | **v2.6.1** (new) | message bundle, per-request `Localizer`, CLDR plural rules (Polish `one/few/many/other`), TOML catalogue loading |
| `golang.org/x/text` | v0.41.0 (already admitted in 1.3.0; becomes a direct import) | `language.Tag`, `language.NewMatcher` for `Accept-Language` |
| `github.com/BurntSushi/toml` | transitive of go-i18n | catalogue parsing; not imported by MediKube |
| `github.com/pocketbase/pocketbase` | v0.40.1 | unchanged; `e.Auth.GetString("locale")` is the account read |
| `github.com/a-h/templ` | v0.3.1020 | unchanged; components already receive `ctx` |
| everything else | as pinned in 003 | unchanged |

Why go-i18n rather than `golang.org/x/text/message` alone: `x/text/message` needs its
`gotext` generator and a catalogue compiled into Go, so adding a language is a code change;
go-i18n loads plain files at build time with the same plural rules, which is what FR-010
demands. Alternatives rejected in [research.md](research.md) D-01.

**Storage**: none new. `users.locale` (text, `^[a-z]{2}(-[A-Za-z]{2})?$`, required, default `en`)
exists since migration `1756100100_users_profile`. The phrase catalogues are `embed.FS` in the
binary.

**Testing**: the five tiers of constitution Principle III. New build-time gates in
`internal/i18n/catalogue_test.go` (FR-011). Browser gate gains `e2e/locale.spec.ts` (FR-016).

**Target Platform**: unchanged — single static binary, distroless image.

**Project Type**: web service (server-rendered).

**Performance Goals**: resolving a `Localizer` is one header parse and one map lookup per
request; catalogue lookup is a map hit. No measurable change to the SC-004 chart benchmark.

**Constraints**: `internal/domain/**` still imports only the standard library and zerolog, so
the *set of shipped languages* is not knowable in the domain. Membership is checked in the
identity service through an injected `func(string) bool`; the domain keeps its shape check.

**Scale/Scope**: ~22 page titles, ~170 field labels, ~35 enum vocabularies, ~300–450 templ copy
literals across 82 templates (control scaffolding repeats per kind), ~25 web error messages;
two languages; every page in the browser gate.

## Constitution Check

*GATE — evaluated before Phase 0 and re-evaluated after Phase 1 design.*

### I. Simplicity Is A Gate — **PASS**

One package (`internal/i18n`), one context key, one helper `T`. No per-component `Localizer`
prop threading through 82 templates — templ already passes `ctx`, so the localiser rides it.
No translation of user data, no region variants, no date/unit localisation (they have their
own preferences and are out of scope), no runtime language reload, no translation admin UI,
no machine translation. **Explicit YAGNI**: no per-patient language, no per-message
language on realtime patches (they render for the page's owner), no RTL scaffolding.

### II. Interfaces At Every Seam — **PASS**

- `internal/i18n` exposes `Resolve(ctx, accountLocale, acceptLanguage) *Localizer`,
  `With(ctx, l)`, `From(ctx)`, `T(ctx, id, data...)`, `N(ctx, id, count, data...)` and
  `Supported() []Language`. Consumers depend on those six names.
- The identity service takes `SupportedLocale func(string) bool`; the wiring passes
  `i18n.IsSupported`. The domain is untouched beyond nothing.
- Kind display names come from a message id derived from `kind.Kind.Enum()`; no `switch k`.

### III. Tests Are The Specification — **PASS**

Unit: `internal/i18n` (resolution order, fallback, plurals per form, missing-key behaviour).
Build-time: catalogue completeness, surplus, and code→catalogue reference scan
(`catalogue_test.go`, `reference_test.go`). HTTP: `PATCH /me` locale membership; a Polish
account's page carries `lang="pl"` and a Polish title; JSON error `message` follows the
caller, codes do not. Browser: `e2e/locale.spec.ts` at both viewports.

### V. PocketBase Is The Platform — **PASS**

Language lives on the `users` record PocketBase already owns. Email templates: PocketBase's
mail templates are single-language settings; recorded as out of scope (spec Assumptions).

### VI/VII. Observability, Privacy — **PASS**

Logs stay English (FR-013); the language code is not PHI, but it is not logged at INFO either
— nothing about the request changes what is logged.

### VIII. Accessibility — **PASS, strengthened**

`<html lang>` becomes correct for the first time. Every `aria-label` is a catalogue phrase.

### IX. Gates — **PASS, extended**

`task check` gains `test:i18n` (the three catalogue tests are ordinary `go test`, so nothing
new to wire; the task exists so the failure is named). `scripts/check-naming.sh` unchanged.

### Technology Constraints — **requires amendment 1.4.0**

The stack is settled by constitution; `go-i18n` is new. Amendment 1.4.0 (MINOR: admits a
module, weakens nothing) is part of this phase's first pull request, with this plan as its
rationale.

## Project Structure

### Documentation (this feature)

```text
specs/007-localisation/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── README.md
│   └── catalogue.md       # phrase-id conventions, plural form contract, file layout
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/i18n/                      # [new] the only importer of go-i18n
├── i18n.go                          # Bundle from embed.FS; Supported(); Resolve/With/From/T/N
├── locales/
│   ├── active.en.toml               # reference catalogue
│   └── active.pl.toml               # Polish, complete
├── i18n_test.go                     # resolution order, fallback, plural forms
├── catalogue_test.go                # every locale == en key set (FR-011 a, b)
└── reference_test.go                # every i18n.T/N id literal in internal/web exists in en (FR-011 c)

internal/web/page/shell.go           # RenderPage: resolveLocale(e) beside resolveTheme(e); ctx gets the Localizer; DocumentProps.Lang
internal/web/views/shell/layout.templ# <html lang={ props.Lang }>
internal/web/views/**/*.templ        # literals → i18n.T(ctx, "...")
internal/web/views/records/*.go      # Label:/enum-label funcs → message ids resolved at render
internal/web/page/*.go               # title consts → ids; nav labels → ids
internal/web/errors.go               # Message(code) → Message(ctx, code)
internal/web/api/dto_me.go           # unchanged shape; locale membership error surfaces
internal/service/identity/service.go # SupportedLocale func(string) bool; sign-up takes the request's resolved locale
internal/web/page/settings.go        # localeOptions(user.Locale) via optionsOf
internal/web/views/settings/settings.templ  # the language select
internal/architecture/forbidden_deps_test.go # unchanged; depguard: go-i18n only under internal/i18n
e2e/locale.spec.ts                   # FR-016
e2e/fixtures.ts                      # title(...) unchanged (gate stays English)
Taskfile.yaml                        # test:i18n; check depends on it
.specify/memory/constitution.md      # 1.4.0
```

**Structure Decision**: one new package at `internal/i18n`, importable by `internal/web/**`
and `internal/service/identity` only (depguard). `internal/domain` never imports it.

## Phase 0 → Phase 1 design decisions (summary; detail in research.md)

- **D-01** go-i18n v2 over `x/text/message`, `kataras/i18n`, `invopop/ctxi18n`: most used,
  plural rules from CLDR, file-based catalogues, pure Go, three-package dependency footprint.
- **D-02** Catalogue format TOML, one file per language named `active.<lang>.toml` — go-i18n's
  own convention, so its `goi18n merge` tooling works on the files as committed.
- **D-03** Message ids are dotted, lowercase, stable: `nav.timeline`, `page.allergies.title`,
  `field.allergy.allergen`, `enum.severity.mild`, `kind.allergy.one`/`.other`,
  `error.forbidden`, `empty.nothing_recorded`. Ids never contain the English text.
- **D-04** Resolution order: `e.Auth.locale` → `Accept-Language` best match against
  `Supported()` → `en`. Region is stripped (`pl-PL` → `pl`).
- **D-05** The `Localizer` rides `ctx`; `RenderPage` (and `web.Render` for non-page responses)
  is the single place that sets it. Components call `i18n.T(ctx, id)`; a missing `Localizer`
  on `ctx` means English, so unit tests of components need no setup.
- **D-06** Go-side label producers (`Label:` fields, `SeverityLabel` etc.) return **message
  ids**; the template translates at the point of rendering. `describe(control)` in e2e and
  `titleFor` keep working because the English gate reads English.
- **D-07** Shipped languages are derived from the embedded directory listing; there is no
  Go slice to edit (FR-010).
- **D-08** Reference scan: `reference_test.go` parses `internal/web/**/*.templ` and `*.go`
  (excluding `*_templ.go`) for `i18n.T(ctx, "` / `i18n.N(ctx, "` and dynamic-id builders
  registered in one map, and asserts each id exists in `active.en.toml`.
- **D-09** JSON `Failure.Message` uses the same `Localizer`; `Code`, `Fields[].Field`,
  `Fields[].Code`, `basis`, `criteria` and vocabulary values are untouched (FR-012); a test
  diffs an English and a Polish response with `message` removed.
- **D-10** Sign-up carries the request's resolved language into the new account (FR-004); the
  service signature gains a `locale string` argument rather than reading the request.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New direct dependency (go-i18n) | FR-008 plural rules and FR-010 file-per-language | `x/text/message` needs generated Go per language (FR-010 fails); hand-rolled plurals for Polish are exactly the wheel FR-014 forbids |
| Reference scan over source text | FR-011c — a phrase asked for that no language defines must fail the build | templ generates Go, so a compile-time typed key would need a generator of its own; a scan test is 80 lines and names the file:line |
