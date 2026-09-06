# Phase 1 Data Model: Localisation

**Feature**: `007-localisation` | **Date**: 2026-09-06

No new collection, no migration. This phase's "data model" is the phrase catalogue: its entry
shape, its id namespaces, and the invariants the build enforces over it. Consistent with
`research.md`; deviations are listed in `plan.md`.

---

## 0. The existing field this phase turns on

`users.locale` — unchanged, added by migration `1756100100_users_profile`
(`internal/store/migrations/1756100100_users_profile.go`, field `usersFieldLocale = "locale"`):

| Property | Value |
|---|---|
| Type | `core.TextField` |
| Required | `true` |
| Max | `10` (`usersLocaleMax`; fits `pt-BR`, five characters, with headroom) |
| Shape constraint | `internal/domain/identity/validate.go:26`, `localePattern = ^[a-z]{2}(-[A-Za-z]{2})?$` |
| Default | no column default; a new `identity.User` is `en` unless D-10's sign-up path supplies another shipped language |

Before this phase: the field exists, is validated, is stored, is read by nothing and written by
nothing except account creation. This phase adds: a membership check (shipped languages only,
research D-10), a settings control that writes it via the existing `PATCH /api/v1/me`, and every
consumer that reads it to choose a `Localizer` (research D-04, D-05). **The field's type,
constraint and default are unchanged; no migration is added or altered.**

---

## 1. Shipped languages — derived, not declared

A "shipped language" is a filename: `internal/i18n/locales/active.<lang>.toml`, embedded via
`embed.FS`. `Supported()` lists exactly the tags obtained by reading that directory at package
init (research D-07). Adding `active.de.toml` and nothing else makes `de` shipped everywhere at
once: the settings `<select>`, the `Accept-Language` matcher, and the sign-up membership check
(research D-10) all read the same list.

This phase ships exactly two files: `active.en.toml` (the reference) and `active.pl.toml`
(complete, FR-015).

---

## 2. Phrase catalogue entry shape

One TOML table per message id (research D-02). Every entry MediKube writes carries a
`description`; go-i18n's own bare-string shorthand (`id = "text"`, no plural forms, no
description) is never used in this codebase's own catalogue, so a translator opening
`active.pl.toml` never has to guess intent from the English string alone (US3 acceptance
scenario 5).

**A phrase with no count** (`nav.timeline`):

```toml
[nav.timeline]
description = "Primary navigation: the timeline link"
other = "Timeline"
```

**A phrase with a count, all plural forms English uses** (`kind.allergy` — a kind's display name,
singular/plural; `en` needs only `one`/`other`, `pl` additionally needs `few`/`many`):

```toml
# active.en.toml
[kind.allergy]
description = "How many allergy records a list holds, e.g. \"3 allergies\""
one = "{{.PluralCount}} allergy"
other = "{{.PluralCount}} allergies"

# active.pl.toml
[kind.allergy]
description = "How many allergy records a list holds, e.g. \"3 allergies\""
hash = "sha1-…"
one = "{{.PluralCount}} alergia"
few = "{{.PluralCount}} alergie"
many = "{{.PluralCount}} alergii"
other = "{{.PluralCount}} alergii"
```

(`hash` is written by `goi18n merge`, quickstart §5, to detect a translation that has gone stale
against a changed English source; it is not hand-maintained.)

**A phrase with a named template variable** (`empty.no_records_for`):

```toml
[empty.no_records_for]
description = "Shown under a kind's list when the patient has none of that kind yet"
other = "No {{.Name}} recorded yet"
```

Two fields go through `i18n.T`/`i18n.N`'s `TemplateData`: `{{.Count}}` is never written by hand —
`i18n.N` passes `PluralCount` as `{{.PluralCount}}` automatically (research D-01, go-i18n's own
`LocalizeConfig.PluralCount`) — and `{{.Name}}` (or any other named variable) is supplied by the
call site's `i18n.T(ctx, id, i18n.Data{"Name": kindDisplayName})`-shaped data argument. Both
`{{.PluralCount}}` and any `{{.Name}}` are Go template syntax executed by go-i18n's own
`template.TextParser` (`i18n/localizer.go`'s `getTemplateParser`) — nothing about them is
MediKube-specific parsing.

---

## 3. Message id namespaces

Dotted, lowercase, stable, never containing the English wording (research D-03). Ten namespaces:

| Namespace | Shape | Example 1 | Example 2 |
|---|---|---|---|
| `nav.` | `nav.<link>` | `nav.timeline` | `nav.settings` |
| `page.<op>.title` | `page.<opID>.title` | `page.overviewPage.title` | `page.loginPage.title` |
| `field.<kind>.<name>` | `field.<kind>.<field name>` | `field.allergy.allergen` | `field.condition.onset_on` |
| `enum.<vocab>.<value>` | `enum.<vocabulary>.<wire value>` | `enum.severity.mild` | `enum.condition_status.chronic` |
| `kind.<kind>.one`/`.other`(/`.few`/`.many`) | plural forms of a kind's display name | `kind.allergy.one` | `kind.allergy.other` |
| `error.<code>` | `error.<Failure.Code wire value>` | `error.validation_failed` | `error.not_found` |
| `empty.` | `empty.<context>` | `empty.no_records_for` | `empty.no_search_results` |
| `action.` | `action.<verb>` | `action.save` | `action.delete` |
| `confirm.` | `confirm.<context>` | `confirm.delete_record` | `confirm.discard_changes` |
| `a11y.` | `a11y.<context>` | `a11y.close_dialog` | `a11y.required_field` |

Note on `kind.<kind>.*`: this is the one namespace whose id embeds a `kind.Kind` enum value
(`kind.allergy`, `kind.procedure`, …) rather than a fixed vocabulary word, because the display
name of a kind is itself a phrase with a plural form (spec US1 scenario 5: "1 rekord, 2 rekordy,
5 rekordów"). It is produced from `kind.Kind.Enum()` (plan.md's Interfaces section: "no `switch
k`"), never a hand-written `switch` over thirteen-plus kind values.

---

## 4. Invariants (build-time; FR-011)

| # | Invariant | Test | Failure names |
|---|---|---|---|
| a | Every shipped language file has every id `active.en.toml` has | `internal/i18n/catalogue_test.go` | the missing id, the language |
| b | No shipped language file has an id `active.en.toml` lacks | `internal/i18n/catalogue_test.go` | the surplus id, the language |
| c | Every `i18n.T`/`i18n.N` call site in `internal/web/**` names an id that exists in `active.en.toml` | `internal/i18n/reference_test.go` | the id, the file and line asking for it |

`en` is the reference for (a) and (b): it is the only file no other file is measured against, and
every other shipped file is measured against it symmetrically (both directions, so a stale phrase
removed from English but left in Polish is caught the same way a missing translation is).

Plural-form completeness is a fourth, implicit invariant, folded into (a): a language's plural
rule (research D-01, `internal/plural/rule_gen.go`) declares which forms it uses
(`newPluralFormSet(One, Few, Many, Other)` for `pl`; `(One, Other)` for `en`), and `catalogue_test.go`
checks that every plural-carrying id in that language's file has an entry for each form the
language's own rule requires — not more, not fewer.
