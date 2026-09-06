# Polish catalogue consistency pass (T029)

This is **not** a substitute for review by a native Polish speaker — it's the mechanical,
systematic pass that can be done without one: internal consistency of `active.pl.toml` against
itself and against `active.en.toml`, checked entry by entry across all 730 message ids. It found
and fixed one grammatical error and eight terminology inconsistencies. The remaining open
questions need a native reader's judgment and are listed at the end.

## Fixed: `kind.family_member`'s `other` form was ungrammatical

Every other `kind.*` entry's `other` form (CLDR's fallback, used for non-integer counts) is the
noun's genitive singular. `kind.family_member` had copy-pasted its `many` form instead:

```
one   = "{{.PluralCount}} członek rodziny"
few   = "{{.PluralCount}} członkowie rodziny"
many  = "{{.PluralCount}} członków rodziny"
other = "{{.PluralCount}} członków rodziny"   # wrong: identical to many
```

"członek" declines like "lek" (kind.medication: many="leków", other="leku" — different). Genitive
singular of "członek" is "członka", not "członków". Fixed:

```
other = "{{.PluralCount}} członka rodziny"
```

(The apparent coincidences elsewhere — e.g. `kind.condition`'s `few` and `other` both reading
"schorzenia" — are not bugs: "schorzenie" is a neuter noun whose genitive singular and nominative
plural happen to be the same word. Verified against Polish declension tables before leaving them
alone.)

## Fixed: "record" translated two different ways in the same catalogue

`active.en.toml` uses "record" for the same underlying concept (a patient's saved entry of any
kind) in about a dozen ids. Polish had split this into two words — "rekord" in some places, "wpis"
("entry") in others — with no discernible reason for the split; it reads as two different
translators' vocabulary rather than a deliberate register choice. "rekord" is the dominant term
(`directory.usage_records`, `confirm.delete_consequence`, `patient.delete_consequence`,
`linked_records.title` all already used it), so the "wpis" outliers were brought in line with it:

| id | before | after |
|---|---|---|
| `empty.record_first_one_body` | "Zapisz pierwszy **wpis**, a pojawi się..." | "Zapisz pierwszy **rekord**, a pojawi się..." |
| `tag.usage_count` (one/few/many/other) | "wpis" / "wpisy" / "wpisów" / "wpisu" | "rekord" / "rekordy" / "rekordów" / "rekordu" |
| `tag.delete_consequence` (one/few/many/other) | "...do {{.PluralCount}} **wpisu**...z tego **wpisu**." (and few/many/other variants) | same sentences with "wpis*" → "rekord*" |
| `patient.tile_empty` | "Jeszcze nic — dodaj pierwszy **wpis**" | "Jeszcze nic — dodaj pierwszy **rekord**" |
| `patient.activity_default_record` | "**Wpis**" | "**Rekord**" |
| `confirm.facility_delete_consequence` | "...zachowuje swój **wpis**, z wyczyszczonym..." | "...zachowuje swój **rekord**, z wyczyszczonym..." |
| `confirm.practitioner_delete_consequence` | "...zachowuje swój **wpis**, z wyczyszczonym..." | "...zachowuje swój **rekord**, z wyczyszczonym..." |
| `directory.empty_body` | "Dodaj pierwszy **wpis**, a pojawi się tutaj." | "Dodaj pierwszy **rekord**, a pojawi się tutaj." |
| `search.no_matches_body` | "Żadne **wpisy** nie pasują do tego wyszukiwania." | "Żadne **rekordy** nie pasują do tego wyszukiwania." |
| `patient.nothing_recorded_body` | "Każdy rodzaj **wpisu** dla tej osoby zaczyna się..." | "Każdy rodzaj **rekordu** dla tej osoby zaczyna się..." |

After this pass, `wpis`/`wpisy`/`wpisów`/`wpisu` does not appear anywhere in `active.pl.toml`
(confirmed by grep); "rekord" is now the catalogue's one word for this concept.

`symptom.episodes_recorded`'s "epizod" was left alone — its English source says "episode", not
"record", so that's a different concept translated correctly, not the same inconsistency.

## Checked and found consistent (no change)

- **Register**: every message id uses the informal second-person ("ty") address consistently —
  "Twoje hasło", "Zaloguj się", "twój adres e-mail". No `Pan`/`Pani` forms anywhere, so there is no
  register mixing to fix. Buttons are uniformly the imperative ("Zapisz", "Usuń", "Anuluj",
  "Dodaj", "Edytuj"); confirmation-dialog questions are uniformly the impersonal infinitive
  ("Usunąć na stałe?", "Usunąć ten zabieg?") — these are two different UI elements with two
  different established Polish idioms, not a mixed register.
- **`{{.Kind}}`/`{{.Label}}` substitution**: traced every call site that builds a `kind.<x>` id
  (`internal/web/localize.go`'s `KindLabel`, `internal/web/page/timeline.go`'s `kindNoun`,
  `internal/web/page/accounts.go`'s holdings list) against where the Polish catalogue embeds the
  result. All three call sites use the substituted noun as a standalone label/tag/list item
  (a chart tile heading, a timeline chip, an account-holdings `<li>`), never inside a sentence
  whose surrounding words would require a different case — so the plain nominative/genitive forms
  the `kind.*` entries already carry are correct wherever they land. The two sentences that do
  embed `{{.Kind}}` mid-string (`empty.nothing_recorded`, `overview.medication_summary`) use a
  colon-separated "label: value" construction specifically so the embedded count phrase never has
  to agree in case with the sentence around it — deliberate, not an oversight, though see the open
  question below.
- **Diacritics**: grepped for common ASCII-typo stems (`hasl`, `dlug`, `krotk`, `wlasciw`, `zle`,
  `blad`, `dostep`, `godzin`, `zmian`, `usuwa`, `wygas`, and about a dozen more) — every hit already
  carries its diacritics correctly (hasło/hasła/hasłem all present, none regressed to `haslo`).
  Found nothing to fix.
- **few/many pairs**: checked every multi-form entry (`kind.*`, `tag.usage_count`,
  `tag.delete_consequence`, `directory.usage_practitioners`, `directory.usage_patients`,
  `directory.usage_records`, `symptom.episodes_recorded`, `auth.password_length_unit`,
  `confirm.delete_consequence`, `patient.delete_consequence`) against Polish's CLDR
  one/few/many/other rule (few: n%10 in 2-4 and n%100 not in 12-14; many: everything else plural;
  other: fractional). All use the grammatically correct declension for their noun class once the
  `other`-form bug above was fixed. `kind.equipment`'s `one`/`few` both reading "sprzęt" unchanged
  is correct, not a bug — "sprzęt" is a mass noun with no plural form in Polish.

## For a native Polish reviewer to still check

1. **Masculine-personal virile plural in the "few" (2-4) count forms** —
   `directory.usage_practitioners` ("2 specjaliści"), `directory.usage_patients`
   ("2 pacjenci"), `kind.family_member` ("2 członkowie rodziny"). The nominative virile plural used
   here is grammatically defensible as a standalone count phrase, but Polish also commonly uses the
   genitive-governed form ("2 specjalistów", "2 pacjentów") in this exact UI position (a bare
   number + noun, not a full sentence with a virile-triggering verb). This is a genuine style choice
   between two correct forms, not an error either way — a native speaker should pick one and it
   should probably match whichever convention `kind.*`'s own "few" forms use for virile nouns
   elsewhere, for consistency.
2. **The colon-construction sentences** (`empty.nothing_recorded`: "Nie zapisano jeszcze:
   {{.Kind}}", `empty.nothing_matches_body`: "Żadne z: {{.Kind}} nie pasują...",
   `overview.medication_summary`: "Masz zapisane: {{.Kind}}.") — grammatically safe (sidesteps the
   case-agreement problem entirely) but reads a little telegraphic/label-like rather than as a
   flowing sentence. Worth a native speaker's opinion on whether the colon construction is the
   right tradeoff versus writing each as an ordinary sentence per kind (which the codebase already
   does elsewhere, e.g. `empty.procedure_body`, `empty.symptom_body`).
3. **Idiomatic phrasing and tone generally** — nothing here was tested by ear, only by grammar and
   internal consistency. Sentences like `auth.registration_closed_body` and
   `settings.sessions_body` are long, information-dense explanations; a native speaker should read
   them for whether they sound natural in Polish or read as translated English.
4. **Medical/clinical terminology precision** — vocabulary values (`enum.specialty.*`,
   `enum.medication_route.*`, `enum.injury_type.*`, etc.) were checked for internal consistency
   only, not against how Polish clinicians or patients actually refer to these terms in practice.
   A domain-literate native speaker should confirm terms like "Teleporada" (telehealth),
   "Balkonik" (walker), "Zawroty..." equivalents, and the anatomical/procedural vocabulary generally.
5. **Regional register preference** — this pass assumed the informal "ty" register is the right
   choice for a personal health app; a native reviewer familiar with the target market should
   confirm that's the expected tone rather than the more formal "Pan/Pani" address some Polish
   medical software uses.
