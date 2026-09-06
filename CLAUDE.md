# Working in this repository

The rules live in [`.specify/memory/constitution.md`](.specify/memory/constitution.md) and the
design in [`specs/`](specs/). This file does not restate either — it covers what you need that
they do not say.

## The toolchain will bite you first

`go` on your PATH is probably 1.26. This module needs **1.27** and will refuse to build otherwise:

```
go: go.mod requires go >= 1.27 (running go 1.26.5; GOTOOLCHAIN=local)
```

`go.mod`'s `toolchain go1.27.0` line handles this by itself as long as nothing sets
`GOTOOLCHAIN=local`. Nothing in this repo does, and CI must not either.

`internal/platform/pb/build_smoke_test.go` exists to make this the *first* thing that fails
rather than the tenth. If it fails to compile, check your Go version before reading any other
error.

`golangci-lint` has the same problem from the other side: releases before 2.13.2 are built with
go1.26 and refuse this module outright, and their vendored staticcheck panics building IR for
1.27 source. `task lint:go` builds the linter from source with this module's own toolchain. Do
not replace it with `golangci-lint-action`.

## `task gen` before anything

`templ` compiles `.templ` sources to `*_templ.go`, and Tailwind compiles `assets/input.css` to
`internal/web/static/app.css`. Both outputs are gitignored and both are inputs to the build, so
every other task depends on `gen`.

The Tailwind glob in `assets/input.css` points at `.templ` **sources**, never the generated
`.go`. A glob that matches nothing produces an empty stylesheet and exits 0 — no error, just an
unstyled application.

## The `[PB]` boundary

`specs/001-walking-skeleton/plan.md` marks packages `[PB]`. Only those may import
`github.com/pocketbase/pocketbase/...`, and `depguard` enforces it. In practice:

- `internal/domain/**` imports the standard library and zerolog. Nothing else. Ever.
- `internal/service/**` talks to interfaces it declares itself, never to PocketBase.
- `internal/store/**`, `internal/platform/pb/**`, `internal/web/**` are on the PocketBase side.

`forbidigo` separately bans `app.Logger()`, `slog.`, `log.Print` and the
`OnRecord*Request` hook family outside `internal/platform/pb/hooks.go`. One log stream, one
place that binds record hooks.

## Tests

Five tiers, all mandatory (constitution Principle III), all with `testify` — `require` for
preconditions, `assert` for independent assertions, table-driven `t.Run` subtests as the default
shape:

| Tier | Where | Against |
|---|---|---|
| unit | `internal/service/**/*_test.go` | hand-written fakes; no DB, no HTTP, no filesystem |
| integration | `internal/store/**/*_integration_test.go` | `tests.NewTestApp`, a real cloned instance |
| contract | `internal/service/**/<pkg>test/contract.go` | every implementation, including the fake |
| HTTP | `internal/web/**/*_test.go` | `tests.ApiScenario` — status, DTO shape, authz boundary |
| browser | `e2e/` | Playwright, build/CI only |

`task test:phileak` is build-tagged and therefore invisible to `task test`: it boots an instance
and drives every endpoint against sentinel data. Extend the exercise, never the assertion.

## Gates that will reject your change

- `scripts/check-naming.sh` fails on the old project name in any case. It is MediKube.
- `internal/architecture/forbidden_deps_test.go` fails on Gin, Huma, Viper, `samber/mo`,
  `samber/ro`, `samber/slog-zerolog`, React, HTMX, jsvm or any cgo dependency.
- `scripts/check-spec-structure.sh` asserts the spec corpus still has its shape.
- Dependency versions are pinned in `specs/001-walking-skeleton/plan.md`'s Technical Context.
  Changing one is an amendment, not a bump.

## Patients (phase 002)

An account owns several patients (a self-record plus dependants). Three rules, all enforced at
the data-access layer rather than by convention:

- Patient scope is explicit. Every record and every call names the patient it is for — nothing
  infers a "current patient" server-side.
- `users.active_patient` (the switcher's pointer) is never an authorization input. It picks what a
  screen shows; access is decided from ownership alone.
- Records are hard deleted, no `deleted_at`, ever (constitution VII). Files are the only thing
  this codebase treats as soft-deletable, and a patient's photo does not use that path — replacing
  or losing one removes the old file and its thumbnails immediately.

## Before upgrading PocketBase

Read [`docs/pocketbase-upgrade-checklist.md`](docs/pocketbase-upgrade-checklist.md). Four places
reach past a public API, and three of the four fail *silently*. Also re-run
`internal/records/registry_completeness_test.go` and the per-collection lockdown scenarios in
`internal/platform/pb/lockdown*_test.go` — neither reaches past an API, but both assert
PocketBase-observable behaviour an upgrade can move without touching a line of MediKube's own
code (risk R8).

## Clinical records (phase 003)

A relation field's `MaxSelect` treats `0` and `1` identically — both mean single-valued.
"Any number" (a `tags` relation) is a named constant like `unlimitedTags`, never a literal `0`.

The CSP ships `style-src 'self'` with no `'unsafe-inline'`, so a `style="..."` attribute is
silently dropped by the browser rather than rendered — express layout in Tailwind classes, not
inline styles.

Every `datetime-local` control renders through `clinical.Instant.Input()`, never a hand-formatted
string: it is the one place that knows the format `<input type="datetime-local">` requires and
round-trips through it.

## Localisation (phase 007)

No literal English application text in a `.templ` — `scripts/check-i18n-literals.sh`
(`task lint:i18n`) greps text nodes, `placeholder=`/`aria-label=`/`title=`/`alt=` and `<option>`
labels for one and fails the build; a genuine exception goes in
`scripts/i18n-literals.allow`, one line per hit. Every phrase comes from
`i18n.T(ctx, "id")` / `i18n.N(ctx, "id", count)` only — never a Go string literal rendered
straight into markup.

Ids are dotted, lowercase, stable (`nav.timeline`, `field.allergy.allergen`) and never contain
their own English text — `TestCatalogueLintDetectsAnIDThatIsItsOwnText` in
`internal/i18n/catalogue_test.go` is the lint. Every entry in `active.en.toml` carries a
`description`; a translation file never does.

`task test:i18n` runs the three build-time gates (FR-011): a shipped language missing an id
`en` has, one with a surplus id, and a source under `internal/web/**` asking for an id no
language defines — each names the id and file:line.

Add a language: copy `internal/i18n/locales/active.en.toml` to `active.<lang>.toml`, translate
every value, keep every id and plural form the language's own CLDR rule needs, run
`task test:i18n` until it passes. Nothing else changes — see `quickstart.md` §5.

Never translated: API codes, field names, vocabulary wire values, `basis`/`criteria`, logs,
and anything a person typed (record fields, names, notes, tag names) — those render exactly as
entered. Polish register is informal (*Twoja sesja*, *Twoje hasło* — *ty*, not *Pan/Pani*);
match it in any new phrase.
