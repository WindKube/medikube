# Contract: the phrase catalogue

The catalogue is not an HTTP surface, but it is what every screen's rendered text is a contract
test against — this file is what `catalogue_test.go` and `reference_test.go`
(`internal/i18n/`) are written to.

## 1. File naming

`internal/i18n/locales/active.<lang>.toml`, one file per shipped language, `<lang>` a bare
[BCP 47](https://www.rfc-editor.org/rfc/rfc5646) language subtag (`en`, `pl` — never a region:
`pl-PL` is not a filename this phase ever creates, per spec Assumptions). The tag is read from the
**filename**, not from any field inside the file: go-i18n's `i18n/parse.go:parsePath` takes
"everything after the second to last `.`" as the tag, so `active.pl.toml` → tag `pl`, format
`toml` (verified in `research.md` D-02). Renaming the file changes the language it loads as.

`active.en.toml` is the reference; every other file is measured against it (`data-model.md` §4).

## 2. TOML shape

A message with a fixed form (no count) is a table keyed by its id:

```toml
[nav.timeline]
description = "Primary navigation: the timeline link"
other = "Timeline"
```

A message with plural forms adds one key per CLDR form the language's own plural rule declares
(§4 below):

```toml
[kind.allergy]
description = "How many allergy records a list holds, e.g. \"3 allergies\""
one = "{{.PluralCount}} allergy"
other = "{{.PluralCount}} allergies"
```

Every entry MediKube writes includes `description` — go-i18n's bare-string shorthand
(`id = "just the text"`, no table, no description) is valid to the library but is never used in
this codebase's own files, so every id a translator opens carries the context US3 scenario 5
requires. A translation file (every file but the reference) additionally carries `hash`, written
by `goi18n merge` (quickstart §5) to detect when the English source has changed since the
translation was last reviewed — never hand-maintained.

A dotted id (`nav.timeline`) written as a TOML table header (`[nav.timeline]`) is ordinary TOML
nested-table syntax (`nav` containing `timeline`); go-i18n's own parser
(`i18n/parse.go:recGetMessages`/`addChildMessages`) walks nested tables recursively and rejoins
the id with `.` at each level, reconstructing the flat id — confirmed by reading the parser, not
assumed.

## 3. Id rules

- Dotted, lowercase, ASCII. Namespaces and two examples each: `data-model.md` §3.
- **Never the English text.** An id names what the phrase is *for* (`action.save`), not what it
  currently *says* — an English copy edit never requires renaming an id or re-keying a
  translation.
- **Stable.** Once an id ships, it is not renamed; a phrase that stops being used anywhere is
  removed from `active.en.toml` (which then makes every other language's copy of it a build
  failure under invariant (b) until removed there too — this is deliberate, not a bug to route
  around).
- One id, one meaning, reused across every screen that needs exactly that phrase — an id is
  never forked into near-duplicates to avoid a shared dependency (mirrors the shared-vocabulary
  discipline of `003-clinical-records`' `data-model.md` §1).

## 4. Plural-form contract, per shipped language

Each shipped language's plural forms are exactly the set its CLDR rule declares — not more, not
fewer — verified against `internal/plural/rule_gen.go` in the go-i18n v2.6.1 module cache:

| Language | Forms | CLDR rule (as implemented) |
|---|---|---|
| `en` | `one`, `other` | `rule_gen.go:36-44` — `one` iff `i=1 ∧ v=0`; `other` otherwise |
| `pl` | `one`, `few`, `many`, `other` | `rule_gen.go:391-410` — `one` iff `i=1 ∧ v=0`; `few` iff `v=0 ∧ i%10∈[2,4] ∧ i%100∉[12,14]`; `many` iff `v=0 ∧ ((i≠1 ∧ i%10∈[0,1]) ∨ i%10∈[5,9] ∨ i%100∈[12,14])`; `other` otherwise |

(`i` = integer part of the count, `v` = number of visible fraction digits — go-i18n's own
`internal/plural.Operands`, not a MediKube concept.) FR-008 requires at least one phrase in active
use to exercise every one of Polish's four forms; `kind.*` ids (a kind's display name with a
count) are written to guarantee this, since list headers count records across every kind.

Adding a language whose CLDR rule has forms neither `en` nor `pl` uses (e.g. Arabic's `zero`/
`two`) is unaffected by anything in this contract — `catalogue_test.go` checks a file's plural
ids against *that language's own* declared form set, not against `en`'s or `pl`'s.

## 5. Fallback rule (FR-009)

- A phrase **present in English, missing in the chosen language**: renders the English text.
  This is go-i18n's own `Localizer.getMessageTemplate` behaviour (`i18n/localizer.go`), not
  something MediKube adds — verified in `research.md` D-01. Never a blank, never the bare id,
  never an error surfaced to a page.
- A phrase **missing in English**: cannot happen at runtime. `reference_test.go` (invariant c)
  fails the build for any call site naming an id `active.en.toml` does not define, so by the time
  a binary runs, every id any screen asks for already exists in the reference file.

## 6. What is never in the catalogue

- **User data** — record field values, patient names, note text, tag names. These are never
  translated (FR-006) and never pass through `i18n.T`/`i18n.N`; they render as stored, in
  whatever language they were typed in.
- **API identifiers** — `Failure.Code`, `Fields[].Code`, `Fields[].Field`, every vocabulary wire
  value (`enum.<vocab>.<value>` ids exist so a *label* can be shown next to a wire value on a
  page, but the wire value itself, e.g. `"severity":"mild"` in a JSON body, is never looked up in
  the catalogue and never changes with language — FR-012, SC-005).
- **`basis` / `criteria`** and any other machine-readable selection-reason field from an earlier
  phase's search or filter contract.
- **Diagnostic output** — log lines, metric names, trace spans, error reports. These stay in
  English unconditionally (FR-013) and are never sourced from `internal/i18n`; `forbidigo`'s
  existing ban on stray logging calls outside `internal/platform/pb/hooks.go` is untouched by this
  phase and is a second, independent reason a log call site could not reach the catalogue even by
  mistake.

## 7. Build-time invariants (FR-011), exact test names

| Invariant | Test |
|---|---|
| every shipped language has every id `en` has | `internal/i18n/catalogue_test.go` |
| no shipped language has an id `en` lacks | `internal/i18n/catalogue_test.go` |
| every id a `.templ`/`.go` source under `internal/web/**` asks for exists in `en` | `internal/i18n/reference_test.go` |

All three run under `task test:i18n` (`quickstart.md` §1) and are ordinary `go test` — no build
tag, so `task test` alone already runs them; `test:i18n` exists to name the failure rather than to
gate anything new into CI.
