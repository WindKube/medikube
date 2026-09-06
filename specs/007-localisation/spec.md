# Feature Specification: Localisation

**Feature Branch**: `007-localisation`

**Created**: 2026-09-06

**Status**: Draft

**Input**: User description: "Add multi-language support — first additional language Polish. Ensure it is easy to add other languages; use a popular open-source library for it."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Use MediKube in my own language (Priority: P1)

Amara's mother reads Polish far more comfortably than English. In her account settings she
chooses Polish, and from that moment every screen she opens — the navigation, every heading,
button, field label, placeholder, status word, empty-state explanation, confirmation and
validation message — is in Polish. Her father's records themselves (a diagnosis typed in
English, a note in Polish) are shown exactly as they were entered: the application translates
itself, never the person's data. The choice sticks across sign-ins and devices, because it is
part of her account rather than her browser.

**Why this priority**: A health record nobody in the household can read is not a health record.
This is the whole feature; the other stories refine when the language is chosen and how the
next one is added.

**Independent Test**: Sign in, open settings, choose Polish, save. Visit every page the browser
gate lists and confirm no English application text remains; open a record whose fields were
typed in English and confirm they are unchanged; sign out and back in and confirm Polish is
still in force; switch back to English and confirm the reverse.

**Acceptance Scenarios**:

1. **Given** a signed-in account holder on the settings screen, **When** they choose Polish and
   save, **Then** the settings screen itself re-renders in Polish, the choice is saved on the
   account, and every subsequent page is in Polish.
2. **Given** an account whose language is Polish, **When** any page the browser gate lists is
   opened, **Then** no application-owned text on it is in English — navigation, headings,
   controls, labels, placeholders, empty states, confirmations and the page title included.
3. **Given** an account whose language is Polish and a record whose fields were typed in
   English, **When** the record is opened, **Then** the field values are shown exactly as typed
   while the labels around them are Polish.
4. **Given** an account whose language is Polish, **When** a save is refused, **Then** the
   explanation naming the offending fields is in Polish and names the same fields it would in
   English.
5. **Given** an account whose language is Polish, **When** a list states how many items it
   holds or how many days remain, **Then** the number agrees grammatically in Polish (1 rekord,
   2 rekordy, 5 rekordów), not by appending a suffix.
6. **Given** an account whose language is Polish, **When** the page is inspected by assistive
   technology, **Then** the document declares Polish as its language so it is read with Polish
   pronunciation.
7. **Given** an account whose language is Polish, **When** they sign out and sign in again on a
   different device, **Then** Polish is in force without choosing it again.
8. **Given** a phrase for which no Polish text exists yet, **When** it is shown, **Then** the
   English phrase appears in its place — never a blank, a key, or an error.

---

### User Story 2 - Be greeted in my language before I have an account (Priority: P2)

Someone opens MediKube for the first time from a browser set to Polish. The sign-in and
sign-up screens, and the account-recovery flow, are already in Polish. When they create an
account, Polish becomes the account's language without a further step, and it stays Polish
even if they later sign in from an English browser — because the account, not the browser,
now owns the choice.

**Why this priority**: Everything before sign-in is where a new household decides whether to
trust the application at all. It depends on story 1's translations and adds only the rule for
choosing a language when there is no account to ask.

**Independent Test**: With a browser whose preferred language is Polish and no session, open the
sign-in page and confirm it is in Polish; create an account and confirm the account's language
is Polish; sign in from a browser preferring English and confirm the application stays Polish.

**Acceptance Scenarios**:

1. **Given** no session and a browser preferring Polish, **When** the sign-in, sign-up or
   recovery screen is opened, **Then** it is in Polish.
2. **Given** no session and a browser preferring a language MediKube does not have, **When**
   any screen is opened, **Then** it is in English.
3. **Given** a browser preferring Polish, **When** an account is created, **Then** the account's
   language is Polish and the first signed-in page is in Polish.
4. **Given** an account whose language is Polish, **When** the account holder signs in from a
   browser preferring English, **Then** the application is in Polish.

---

### User Story 3 - Add the next language by adding one file (Priority: P3)

A contributor wants to add German. They copy the English phrase file, translate it, and drop
it beside the others. Nothing else is edited: the settings screen offers German, the
pre-sign-in language choice recognises German browsers, and the build refuses the change if
the German file leaves any phrase untranslated or invents a phrase English does not have —
so a language can never ship half done and a stale phrase can never linger unnoticed.

**Why this priority**: The user asked that further languages be easy to add. This story is what
makes that a property of the codebase rather than a promise, and it is the gate that keeps
Polish complete as English grows.

**Independent Test**: Add a phrase file for a new language with one phrase missing and confirm
the build fails naming the phrase; complete the file and confirm the new language appears in
settings with no other change; remove one English phrase still used on a screen and confirm the
build fails naming the screen.

**Acceptance Scenarios**:

1. **Given** a complete phrase file for a new language, **When** it is added and nothing else
   is changed, **Then** the language is offered in settings and recognised from the browser.
2. **Given** a phrase file missing one phrase English has, **When** the build runs, **Then** it
   fails and names the missing phrase and the language.
3. **Given** a phrase file containing a phrase English does not have, **When** the build runs,
   **Then** it fails and names the surplus phrase.
4. **Given** a screen that asks for a phrase no language defines, **When** the build runs,
   **Then** it fails and names the phrase and where it is asked for.
5. **Given** the English phrase file, **When** it is inspected, **Then** every phrase has a
   stable identifier and a comment or context sufficient for a translator who has never seen
   the screen.

---

### Edge Cases

- A stored language the application no longer ships (a file was removed): the account is
  treated as English until it chooses again, and the settings screen shows English selected.
- A browser preference with a region (`pl-PL`, `en-GB`): the language matches on its language
  part; regions are not distinguished in this phase.
- A phrase with a count of zero or a negative number: the grammatical form is still correct
  and no phrase ever shows a bare identifier.
- Text that is longer in Polish than in English (commonly 20–30%): controls wrap or grow; no
  text is clipped, and the mobile layout the browser gate exercises still passes.
- The JSON API is not a screen: its codes, field names, vocabulary values and selection
  reasons are identifiers and are never translated; only its human-readable `message` follows
  the caller's language.
- Diagnostic output (logs, metrics, traces, error reports) stays in English and never carries
  the chosen language as identifying content beyond the language code itself.
- Realtime patches pushed to an open page are rendered in the language of the account that
  owns the page, not of whoever caused the change.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every account MUST carry a language, chosen from the languages the application
  ships, defaulting to English.
- **FR-002**: The account holder MUST be able to change their language on the settings screen,
  and the change MUST take effect on the very response that saves it.
- **FR-003**: Every page MUST be rendered in the language of the signed-in account; when there
  is no account, in the browser's first preferred language the application ships; otherwise in
  English.
- **FR-004**: An account created during a session whose language was chosen from the browser
  MUST be created with that language.
- **FR-005**: Every piece of application-owned text on every page — navigation, headings,
  control labels, placeholders, accessible names, status and vocabulary words, empty-state
  copy, confirmations, validation explanations and page titles — MUST come from the phrase
  catalogue, never from a literal in a screen.
- **FR-006**: Data entered by people — record fields, names, notes, tag names — MUST be shown
  exactly as entered and MUST never pass through translation.
- **FR-007**: The document MUST declare its language so assistive technology reads it
  correctly.
- **FR-008**: Phrases that include a number MUST use the plural rules of the language, and
  Polish's four forms (one, few, many, other) MUST all be exercised by at least one phrase in
  use.
- **FR-009**: A phrase missing in the chosen language MUST fall back to English; a phrase
  missing in English MUST fail the build, never render.
- **FR-010**: Adding a language MUST require exactly one new file and no change to any other
  file; the settings choice and the browser match MUST derive from the files present.
- **FR-011**: The build MUST fail when any shipped language lacks a phrase English has, has a
  phrase English lacks, or when any screen asks for a phrase no language defines — naming the
  phrase, the language and the asking screen respectively.
- **FR-012**: The JSON API's codes, field names, vocabulary values and selection reasons MUST
  remain untranslated identifiers; only its human-readable `message` MAY follow the caller's
  language, chosen by the same rule as FR-003.
- **FR-013**: Diagnostic output MUST remain in English.
- **FR-014**: The translation mechanism MUST be a widely used open-source library with plural
  support for the languages shipped; MediKube MUST NOT write its own.
- **FR-015**: Polish MUST ship complete in this phase: every phrase English has, translated by
  a person or reviewed by one, not machine output left unchecked.
- **FR-016**: The existing automated browser check MUST continue to pass in English, and MUST
  additionally prove at both viewports that a Polish account sees no English application text
  on a representative page of every screen family and that the document language is Polish.
- **FR-017**: Every acceptance scenario in this specification MUST exist as an automated test,
  and the phase MUST NOT be considered complete until every one passes.

### Key Entities

- **Account language**: the account's chosen language, one of those shipped; already stored on
  the account as `locale` and validated, but with no way to choose it until this phase.
- **Phrase catalogue**: one file per language holding every application-owned phrase under a
  stable identifier, with plural forms where the phrase carries a number. English is the
  reference; every other file is measured against it.
- **Shipped languages**: derived from the catalogue files present, never listed by hand.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With Polish chosen, 100% of the pages the browser gate lists render with zero
  English application-owned phrases at both viewports, and 100% still pass the gate in
  English.
- **SC-002**: Changing language on the settings screen takes effect on that same response and
  survives sign-out, sign-in and a second browser 100% of the time.
- **SC-003**: Adding a complete phrase file for a new language makes it selectable and
  browser-matched with zero other files changed.
- **SC-004**: A phrase file missing or surplus by a single phrase fails the build 100% of the
  time, naming the phrase; a screen asking for an undefined phrase fails the build 100% of the
  time, naming the phrase and the screen.
- **SC-005**: 100% of JSON API codes, field names, vocabulary values and selection reasons are
  byte-identical between an English and a Polish caller.
- **SC-006**: Every acceptance scenario in this specification exists as an automated test and
  passes.

## Assumptions

- Polish is the first and only additional language in this phase; the mechanism is proven by
  the third story, not by shipping a third language.
- Regions within a language (`pl-PL`, `en-GB`) are not distinguished; the account keeps its
  existing language field's shape so a later phase can distinguish them without migration.
- Date and unit presentation already have their own account preferences (`date_format`,
  `unit_system`) and are out of scope here; dates continue to render as they do today.
- Right-to-left scripts are out of scope; the mechanism does not preclude them.
- Email text sent by the platform (verification, recovery) is out of scope unless the
  platform's own template mechanism already supports a per-message language; if it does not,
  emails remain in English and this is recorded.
- The superuser admin interface is the platform's own and is not translated.
- Translating the phrase catalogue is a task for a person; this phase supplies the Polish
  file with the translations reviewed as part of the pull request.
