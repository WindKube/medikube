# Phase 0 Research: Localisation

**Feature**: `007-localisation` | **Date**: 2026-09-06

Every decision this phase makes, with its evidence. Facts about
`github.com/nicksnyder/go-i18n/v2@v2.6.1` are verified against the module source, fetched with
`GOFLAGS=-mod=mod go mod download github.com/nicksnyder/go-i18n/v2@v2.6.1` and read from
`$(go env GOMODCACHE)/github.com/nicksnyder/go-i18n/v2@v2.6.1/`. Nothing below is left as
`NEEDS CLARIFICATION`.

---

## D-01 — go-i18n v2.6.1 over `x/text/message`, `kataras/i18n`, `invopop/ctxi18n`

**Decision.** `github.com/nicksnyder/go-i18n/v2` v2.6.1 is the translation mechanism (FR-014).

**Evidence, verified against the module.**

`go.mod` (module cache) declares three dependencies: `github.com/BurntSushi/toml v1.6.0`,
`go.yaml.in/yaml/v3 v3.0.4`, `golang.org/x/text v0.32.0`. Pure Go, no cgo, and `x/text` is
already admitted (constitution 1.3.0) so the marginal dependency footprint is TOML plus YAML —
YAML is unused by MediKube, which loads only the `toml` unmarshaler.

Plural rules come from CLDR, generated code, not hand-written: `internal/plural/rule_gen.go`
carries one `addPluralRules` call per language, sourced from `internal/plural/codegen/plurals.xml`
(CLDR's own plural-rules XML per the codegen `README.md`). Polish's rule
(`internal/plural/rule_gen.go:391-410`) is exactly the CLDR `pl` rule: `one` when
`i=1,v=0`; `few` when `v=0 ∧ i%10∈[2,4] ∧ i%100∉[12,14]`; `many` for the remaining `v=0` cases
(`i≠1 ∧ i%10∈[0,1]`, or `i%10∈[5,9]`, or `i%100∈[12,14]`); `other` otherwise — four forms,
matching FR-008's requirement that Polish's `one/few/many/other` all be exercised.

Catalogue format is plain files loaded at build time (`i18n/bundle.go`: `LoadMessageFile`,
`LoadMessageFileFS` in `i18n/bundlefs.go`, `ParseMessageFileBytes` in `i18n/parse.go`) — not a
generator. `i18n/parse.go:parsePath` derives the language tag from the filename itself
("everything after the second to last `.`, or after the last path separator, but before the
format"): `active.pl.toml` parses to tag `pl`, format `toml`. Adding a language is exactly the
file FR-010 demands; nothing is registered in Go.

`i18n/message.go` defines `Message{ID, Hash, Description, LeftDelim, RightDelim, Zero, One, Two,
Few, Many, Other}` — the six CLDR plural-form fields plus `Description` (D-03's translator
context, spec scenario US3.5) and `Hash` (used only by `goi18n merge`, quickstart §5).

`i18n/localizer.go:Localizer.getMessageTemplate` walks: the best-matched tag's own template, else
— if that tag *is* the bundle's default language — the caller-supplied `DefaultMessage`, else the
default language's template, else the `DefaultMessage`, returning `*MessageNotFoundErr` alongside
whichever fallback string it found. `LocalizeWithTag` (lines ~150-175) then executes the resolved
plural form and, if execution fails, retries the `Other` form. This is FR-009's two fallback
rules in one mechanism: a phrase absent from Polish silently resolves through the default-language
branch (English) with no blank, no key, no error surfaced to the page; a phrase absent from
*English* has no `DefaultMessage` to fall back to and `Localize` returns `*MessageNotFoundErr` —
MediKube treats that error as a build-time-only condition (D-08's reference scan makes it
unreachable at runtime).

**Alternatives considered, same four axes.**

| | plural source | catalogue = file only? | dependency footprint | cgo |
|---|---|---|---|---|
| **go-i18n v2.6.1** | CLDR (generated) | yes — `active.<lang>.toml` | toml + yaml + x/text | no |
| `x/text/message` + gotext | CLDR (`x/text/feature/plural`) | no — `gotext extract`/`gotext generate` compiles a catalogue into a Go source file that must be committed and regenerated per language | already-admitted `x/text` only | no |
| `kataras/i18n` | its own simplified plural switch, not CLDR — no four-form Polish rule shipped | yes, but the whole Kataras web-framework module tree is a transitive pull for one plural helper | large (kataras/iris ecosystem) | no |
| `invopop/ctxi18n` | wraps `x/text/language` matching but plural forms are hand-written per catalogue entry (`One`/`Other` only in the shipped examples), not CLDR-complete for Slavic four-form languages | yes, YAML files | small, but last tagged release predates this evaluation by over a year (low maintenance activity vs. go-i18n's active CHANGELOG) | no |

`x/text/message` is rejected on FR-010 directly: `gotext generate` turns translated strings into
`catalog.NewBuilder()` calls in a generated `.go` file, so adding German is a code change and a
regeneration step, not "one file, nothing else edited." `kataras/i18n` and `invopop/ctxi18n` are
rejected on FR-008: neither ships a generated-from-CLDR Polish `few`/`many` split, which this
phase must exercise, and MediKube is not going to hand-write the CLDR algorithm FR-014 forbids
writing.

---

## D-02 — Catalogue format: TOML, one file per language, `active.<lang>.toml`

**Decision.** `internal/i18n/locales/active.en.toml`, `active.pl.toml`, loaded with
`Bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)` (`github.com/BurntSushi/toml`, already a
transitive dependency of go-i18n) and `Bundle.LoadMessageFileFS` against the package's
`embed.FS`.

**Evidence.** This is go-i18n's own convention, not an invented one: its `goi18n` CLI ships
`example/active.en.toml` / `example/active.es.toml` in exactly this shape, and `goi18n merge`
(`goi18n/marshal.go:writeFile`) *writes* `filepath.Join(outdir, fmt.Sprintf("%s.%s.%s", label,
langTag, format))` — i.e. `active.pl.toml` for label `"active"`, tag `pl`, format `toml` — the
identical name this phase commits. Following the convention means `goi18n merge` (quickstart §5)
runs against the committed files unmodified.

A message with plural forms is a TOML table keyed by its id, e.g.:

```toml
[nav.timeline]
description = "Primary navigation: the timeline link"
other = "Timeline"

[kind.allergy]
description = "How many allergy records a list holds"
one = "{{.PluralCount}} allergy"
other = "{{.PluralCount}} allergies"
```

(`example/active.en.toml` in the module cache; a message with only `other` and no description may
collapse to a bare string, `HelloPerson = "Hello {{.Name}}"`, but this phase always writes the
table form so `description` is never optional — D-03.) A dotted id such as `nav.timeline` written
as a bracketed table header (`[nav.timeline]`) is TOML syntax for nested tables (`nav` containing
`timeline`); `i18n/parse.go:recGetMessages`/`addChildMessages` walks nested maps recursively and
rejoins the id with `.` at each level, producing the flat id `"nav.timeline"` again — so the
natural TOML nested-table syntax already yields D-03's dotted ids with no special-casing.

**Alternatives rejected.** JSON: go-i18n supports it, but TOML supports inline comments next to a
key, which the Description field's own multi-line style benefits from during review; also not
go-i18n's own documented default. YAML: it is one of the three formats `goi18n merge` writes but
pulls in `go.yaml.in/yaml/v3` for a format MediKube has no other use for; TOML is already a
transitive dependency, so registering it adds nothing.

---

## D-03 — Message id scheme: dotted, lowercase, stable, never the English text

**Decision.** Namespaces: `nav.<link>`, `page.<op>.title`, `field.<kind>.<name>`,
`enum.<vocab>.<value>`, `kind.<kind>.one`/`.other` (plural forms of a kind's display name),
`error.<code>`, `empty.<context>`, `action.<verb>`, `confirm.<context>`, `a11y.<context>`. Full
shape and examples in `data-model.md` §3.

**Evidence.** `i18n/message.go`'s `Message.ID` is an opaque string to the library; nothing about
go-i18n prescribes a scheme, so the scheme is MediKube's own choice, constrained only by: ids must
be stable across a phrase's English wording changing (a translator's work on `active.pl.toml`
should survive an English copy edit) and every id must carry a `description` (spec US3 scenario 5:
"a comment or context sufficient for a translator who has never seen the screen") — `Message`'s
`Description` field exists exactly for this and is surfaced to translators by
`goi18n merge`'s `-sourceLanguage` extraction path (`marshal.go:marshalValue`, `m["description"]`).
Ids never contain the English text itself, so an English wording change never requires renaming
the id or re-keying the Polish translation.

---

## D-04 — Resolution order: account `locale` → `Accept-Language` best match → `en`

**Decision.** `Resolve(ctx, accountLocale, acceptLanguage string) *Localizer` tries, in order:
the account's stored `locale` if non-empty and supported; else the best match of
`Accept-Language` against `Supported()`; else `en`. Region is stripped before either comparison
(`pl-PL` → `pl`, edge case in spec.md).

**Evidence.** `i18n.NewLocalizer(bundle, langs ...string)` (`i18n/localizer.go`) already accepts
multiple preference strings and parses each with `language.ParseAcceptLanguage` — it can parse a
raw `Accept-Language` header value directly, so the account locale and the header can both be
passed as candidates in priority order in one `NewLocalizer` call, letting go-i18n's own
`language.NewMatcher`-based `Localizer.getMessageTemplate` do the matching (bundle.go's
`b.matcher`, built by `addTag` on every `AddMessages`/`LoadMessageFileFS` call). Region-stripping:
`language.Tag` parsing itself keeps the region, but `Supported()` only ever contains bare
language tags (`en`, `pl`, from the two-tag file names), so the matcher's own algorithm already
resolves `pl-PL` to `pl` — the base language is the best (only) match in a bundle with no
regional variants registered.

---

## D-05 — The `Localizer` rides `ctx`; `RenderPage` is the one place that sets it

**Decision.** `internal/i18n` exposes `With(ctx, l) context.Context` and
`From(ctx) *Localizer`. `RenderPage` (and `web.Render` for non-page JSON responses) calls
`Resolve` once per request and stores the result with `With`. Templ components and Go helpers
read it with `From`; a missing `Localizer` on `ctx` is treated as English (`From` returns a
localizer over the bundle's default language when the context carries none).

**Evidence.** `internal/web/page/shell.go:resolveTheme` (lines 35-46) is the existing precedent
for exactly this shape: a small per-request resolver function beside `RenderPage`, reading
`e.Auth.GetString(...)`, falling back to a zero-cost default when `e.Auth` is nil. `resolveLocale`
is written the same way, reading `e.Auth.GetString("locale")` instead of `"theme"`. Because templ
components already receive `ctx` (constitution: no new prop threading, Simplicity gate), no
signature of any of the ~82 templates changes — only their bodies gain `i18n.T(ctx, "id")` calls
in place of literals.

---

## D-06 — Go-side label producers return message ids, not strings

**Decision.** `Label:` struct fields and enum-label functions (e.g. `SeverityLabel`) return a
message id string; translation happens at the template, at render time, via `i18n.T(ctx, id)`.

**Evidence.** This keeps every producer of a label ignorant of `ctx` and of the `Localizer` — the
existing `Label:`-field call sites (registry-driven form/control scaffolding) do not carry a
context today, and adding one to every call site would be exactly the "per-component `Localizer`
prop threading" the Simplicity gate (plan.md) rules out. It also means `e2e` `describe(control)`
and `titleFor` (which read English strings) keep working unmodified, because the English gate
renders with no `Localizer` on `ctx` (D-05's default) — the same code path, same ids, same English
output as before this phase touched it.

---

## D-07 — Shipped languages are derived from the embedded directory, never a Go slice

**Decision.** `Supported() []Language` lists whatever `active.*.toml` files are embedded
(`embed.FS` directory read at package init), each entry a bare language tag string plus its
English display name for the settings `<select>`. No hand-maintained list exists anywhere in the
Go source.

**Evidence.** `i18n/bundle.go:Bundle.LanguageTags()` already gives the list of tags a `Bundle`
has loaded; `internal/i18n.Supported()` wraps a call built the same way, sourced from the
directory listing that also drives which files get `LoadMessageFileFS`'d at package init — the
same one read, no second list to fall out of sync. This is FR-010's "the settings choice and the
browser match MUST derive from the files present" satisfied structurally: there is no second
place to edit because there is no second list.

---

## D-08 — Reference scan: every `i18n.T`/`i18n.N` call site must name an id that exists in `en`

**Decision.** `internal/i18n/reference_test.go` parses `internal/web/**/*.templ` and `*.go`
(excluding generated `*_templ.go`) for string-literal first arguments to `i18n.T(ctx, "..."` and
`i18n.N(ctx, "..."`, plus dynamically-built ids registered in one lookup map (for the handful of
call sites that compose an id from a `kind.Kind` or vocabulary value rather than writing it as a
literal), and asserts each resulting id exists in `active.en.toml`.

**Evidence.** go-i18n does not parse Go source itself and has no ahead-of-time id-existence check
of any kind — its own `MessageNotFoundErr` (`i18n/localizer.go`) is a *runtime* return value, and
FR-011's third clause ("a screen asks for a phrase no language defines… the build MUST fail") is a
build-time guarantee no upstream mechanism provides. This is why the scan test is MediKube's own
80-ish lines rather than a knob on the library: a `templ` source compiles to Go before this test
ever runs, so a typed generated-key alternative would need a code generator of its own
(`plan.md`'s Complexity Tracking makes the same call).

---

## D-09 — JSON `Failure.Message` is localised; every other field is not

**Decision.** `internal/web/errors.go`'s `Message(code)` becomes `Message(ctx, code)` and looks up
`error.<code>` through the request's `Localizer`. `Code`, `Fields[].Field`, `Fields[].Code`,
`basis`, `criteria`, and every vocabulary wire value are untouched literals (FR-012). A contract
test requests the same failing operation as an English and a Polish caller and diffs the two
`Failure` bodies with `message` removed — the diff must be empty.

**Evidence.** Nothing here is a go-i18n fact; it is `data-model.md` (this phase) restating
`contracts/README.md`'s existing error envelope from phase 003 with one field's source swapped
from a literal to `i18n.T`.

---

## D-10 — Sign-up carries the request's resolved locale into the new account

**Decision.** The identity service's sign-up method gains a `locale string` parameter; the HTTP
handler passes the value already resolved by `RenderPage`/`Resolve` for that request (D-04/D-05),
not a fresh read of `Accept-Language`.

**Evidence.** FR-004 ("An account created during a session whose language was chosen from the
browser MUST be created with that language") is satisfied by reusing the one resolution the
request already did, rather than the service re-deriving it — avoiding a second place that could
disagree with what the sign-up page itself rendered in. `internal/domain/identity/validate.go`
already validates `u.Locale` against `localePattern` (`^[a-z]{2}(-[A-Za-z]{2})?$`,
`validate.go:26,68-71`) unconditionally; this phase does not touch that shape check. What is new
is the identity service's own membership check — `constraints` in `plan.md` states the domain
must not know the shipped-language set, so the service takes an injected
`SupportedLocale func(string) bool`, wired to `i18n.IsSupported`, and refuses a locale the shape
check would accept but no catalogue file ships.

---

## Confirmed facts outside the module (repository)

- `internal/domain/identity/validate.go:22-26,68-71` — `locale` is already a required `TextField`-
  shaped string, validated by `localePattern`, with no membership check yet (that is what this
  phase adds).
- `internal/store/migrations/1756100100_users_profile.go` — `users.locale` is a required `TextField`
  with `Max: usersLocaleMax` (`10`, comment: "a two-letter language and an optional two-letter
  region"), default value not set at the column level (default `en` is applied by the domain/
  service layer that constructs a new `identity.User`, per spec FR-001). No migration change is
  needed this phase — confirmed by reading the file; `usersFieldLocale` already exists among
  `usersAddedFields`.
- `internal/web/page/shell.go:35-46` — `resolveTheme(e *core.RequestEvent) domainidentity.Theme` is
  the precedent D-05 follows: nil-`e.Auth` returns a safe default, an invalid stored value returns
  the safe default, otherwise the stored value. `resolveLocale` is written identically in shape.
- `internal/httproute/routes.go:512-517` — the sign-in page's `OpID` is `loginPage`, `Path: "/login"`;
  sign-up is `registerPage` at `/register` (`routes.go:519-524`).
- `internal/httproute/routes.go:254,260-264` — `PATCH /api/v1/me` is `OpID: "updateMe"`, the route
  this phase's settings-language change calls.
