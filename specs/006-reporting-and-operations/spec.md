# Feature Specification: Reporting and Operations

**Feature Branch**: `006-reporting-and-operations`

**Created**: 2026-08-27

**Status**: Draft

**Input**: User description: "Phase 006 of MediGo, the last in the sequence. A person can produce something to hand to a doctor or take away with them, and an operator can run, audit, back up and recover the instance. In scope: report building — choosing what a report contains, saving that choice as a reusable template, generating it, and including charts of measured values over time; portable export of an account's complete data in a documented format so nobody is ever trapped; the operator surface — an overview of instance health and usage, a readable trail of who did what to which record and when, an instance-wide view of how much is waiting to be purged, and backup and restore built on the facilities the instance already ships with rather than a second mechanism; and release hardening — browser coverage of every page added by phases 001 to 006, performance budgets, a privacy review of the finished application, and operator documentation."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Walk into a visit with the right paper (Priority: P1)

Ines has twenty minutes with a rheumatologist who has never met her mother. The consultant will ask three questions: what is she taking, what has she been diagnosed with, and what did the last year of blood work show. Ines does not want to scroll a phone in front of a specialist. She wants a document.

She opens the report builder for her mother, and before she chooses anything she is shown how much there is: eleven medications, four active conditions, two allergies, nineteen lab results, six encounters. She ticks the four kinds she wants, sets the range to the last twelve months, narrows to the records tagged for this specialist, and is told exactly how many records that leaves. She asks for it, and because there is a lot of it she does not sit and wait — she is told it is being prepared and she can come back. When it is ready she downloads one document: her mother identified on the first page, the criteria stated in words underneath so the consultant knows what they are and are not looking at, and one section per kind after that. She prints it on the way out.

**Why this priority**: This is the reason somebody maintains these records at all. Five phases of careful data entry pay off at the moment a clinician reads something useful. If only this story ships, the application has delivered its purpose: a person can turn a maintained history into something they can hand over.

**Independent Test**: Sign in as the seeded account, open the report builder for the seeded person, confirm the per-kind counts match what the application holds, select four kinds and a twelve-month range, confirm the count shown before asking matches what comes out, produce the report, download it, and confirm it contains exactly those records, identifies the person and the criteria on its first page, and contains nothing belonging to any other person. Fully testable with nothing else in this phase implemented.

**Acceptance Scenarios**:

1. **Given** a person with nothing recorded at all, **When** the report builder is opened for them, **Then** every kind is shown at zero with an explanation of what a report is and how to start recording, inside the same page structure as a populated builder, and asking for a report is refused with that same explanation rather than producing an empty document.
2. **Given** a person with records of several kinds, **When** the report builder is opened, **Then** each kind is listed with the number of records it holds for that person, together with a total, and those figures match exactly what a report over the same selection would contain.
3. **Given** a report being defined, **When** the account holder selects kinds, a date range, tags and lifecycle values, **Then** the number of records the selection resolves to is shown and updates as the selection changes.
4. **Given** a selection that resolves to no records at all, **When** the account holder asks for the report, **Then** it is refused with an explanation that nothing matched and the selection is left intact so it can be widened.
5. **Given** a valid selection, **When** the account holder asks for the report, **Then** they are told immediately that it is being prepared, are not held waiting, can navigate away, and can see its progress and its completion when they return.
6. **Given** a completed report, **When** it is downloaded, **Then** it is a single self-contained document that opens and prints on any device without the application, its first page identifies the person and states when it was produced and what criteria produced it, and every page carries the person's identity and a page number.
7. **Given** a completed report, **When** its sections are read, **Then** there is one section per selected kind in a documented order, each record shows its own meaningful fields together with its notes and tags, and a kind that matched nothing is stated as having nothing rather than omitted silently.
8. **Given** a report being defined, **When** the account holder chooses not to include the identifying header or the photograph, **Then** neither appears in the produced document and the person is still identifiable by the reference the report carries.
9. **Given** a report definition naming a person the account holder cannot reach, **When** it is submitted by any means, **Then** it is refused in a way indistinguishable from that person not existing, and the attempt is recorded.
10. **Given** two people's records exist under one account, **When** a report is produced, **Then** it concerns exactly one person and contains no record belonging to the other.
11. **Given** a completed report, **When** the documented retention window has passed, **Then** it can no longer be downloaded, is stated as expired, its content is no longer stored, and producing it again is offered.
12. **Given** a completed report, **When** its download address is used by any other account or by nobody signed in, **Then** nothing is returned and the attempt is recorded, whether the address was guessed or copied from somewhere it should not have been.
13. **Given** a report is produced and later downloaded, **When** the activity trail is read, **Then** both appear, recording who, what, which person and when, and neither entry contains a record value, a name of anything recorded, or a file name.
14. **Given** a selection resolving to more records than the documented maximum a single report may contain, **When** the account holder asks for it, **Then** they are told the limit and asked to narrow the selection before anything is produced.
15. **Given** a person whose name, notes or tags contain characters the document's typefaces cannot render faithfully, **When** the document is produced, **Then** the affected entries are counted and the limitation is stated on the first page, and no character is silently dropped or replaced by a different one without that statement.

---

### User Story 2 - Take everything with me (Priority: P2)

Marek has kept his family's records here for three years. He is moving countries, and before he trusts a new arrangement he wants proof that leaving is possible. He asks for everything: every person he owns, every record of every kind, every document he has attached, his tags, his saved reports, his preferences, and the trail of what happened to it all. He is told it will take a while because there are two thousand documents, and he watches it get there. He downloads one archive, opens it on his laptop with nothing of MediGo installed, and finds a manifest that tells him what every file in it is, structured data he can read with a program, tables he can open in a spreadsheet, and his documents under their own names.

**Why this priority**: An application that holds a person's medical history and cannot give it back is a trap, and the project has committed in writing that no user is ever trapped. This ranks second only because a person visits a doctor more often than they emigrate. Delivered alone, it is a complete and honest guarantee of portability.

**Independent Test**: Seed an account with several people, records across every kind available, and attached documents. Ask for a complete export. Confirm it is acknowledged immediately, reports progress, and completes. Download the archive, open it with the application stopped, and confirm the manifest describes every file, that every record and document held for that account is present and readable, that the format version is stated, and that nothing belonging to any other account appears anywhere in it.

**Acceptance Scenarios**:

1. **Given** an account holding nothing at all, **When** a complete export is requested, **Then** it still completes and yields an archive with a manifest and empty sets, so that holding nothing is provable rather than indistinguishable from failure.
2. **Given** an account with substantial data, **When** a complete export is requested, **Then** the request is acknowledged immediately, the account holder is not held waiting, and the work reports its progress and its finished size.
3. **Given** a finished export, **When** the archive is opened outside the application, **Then** it contains a manifest stating the format version, when it was produced, which account, which people, which kinds and how many of each, and the meaning of every other file in the archive.
4. **Given** a finished export, **When** its contents are examined, **Then** every record of every selected kind is present in a machine-readable structured form, tabular files are present for each kind when that form was chosen, and every included document is present under a name that unambiguously identifies which entry it belongs to while preserving its original name.
5. **Given** an export being requested, **When** the account holder narrows it to one person, a subset of kinds and a date range, **Then** the archive contains exactly that and its manifest says so.
6. **Given** an export being requested, **When** the account holder excludes documents, **Then** the archive contains no document content, is correspondingly smaller, and its manifest states that documents were excluded.
7. **Given** an account that can reach a person only through an arrangement made by somebody else, **When** an export is produced, **Then** what the archive contains is decided by what the account could reach at the moment of production, not at the moment of request, and any person dropped in between is named in the manifest as withdrawn.
8. **Given** a finished export, **When** the documented retention window passes, **Then** the archive can no longer be downloaded, its content is no longer stored, the request is shown as expired, and running it again is offered.
9. **Given** an export that is running, **When** the instance is restarted, **Then** the request is reported as failed with a plain reason and offered for retry, and is never left reporting itself as running forever.
10. **Given** an export that is running, **When** a second export is requested, **Then** it is accepted, its position in the queue is shown, and it begins when the first finishes.
11. **Given** an export that is running, **When** the account holder cancels it, **Then** it stops, nothing partial is downloadable, and the cancellation is recorded.
12. **Given** storage that cannot accept the archive, **When** the export runs, **Then** it fails with a reason that names no storage location, nothing partial is left downloadable, and the failure is visible both to the account holder and to the operator.
13. **Given** a finished export, **When** its download address is used by another account or by nobody signed in, **Then** nothing is returned and the attempt is recorded.
14. **Given** an account with a finished export, **When** the account is deleted, **Then** its requests and its archives are destroyed with it and nothing remains downloadable.
15. **Given** an export archive, **When** it is examined for anything it should not contain, **Then** it holds no credential, no secret, no operator setting, and nothing about any person the account cannot reach.

---

### User Story 3 - Save the choice so the next visit takes a minute (Priority: P3)

Ines sees the rheumatologist every three months, and the endocrinologist every six, and each wants a different slice. Rebuilding the same selection four times a year is exactly the sort of chore that makes people stop bothering. She saves the rheumatology selection under a name, with a note about what it is for. Three months later she opens it, and it has quietly picked up the two medications and the four lab results recorded since — because what she saved was the question, not the answer.

**Why this priority**: It converts a one-off capability into a habit, and it fixes the defect that made the predecessor's saved reports worthless: they stored a fixed list of records and silently rotted. It ranks below the two capabilities it accelerates because neither depends on it.

**Independent Test**: Create a saved report with criteria over the seeded person, note which records it resolves to, add a new record that matches those criteria, reopen the saved report and confirm the newly added record is now included in what it resolves to without anything being edited. Then rename it, change its criteria, delete it with confirmation, and confirm a second account can neither see nor open it. Fully testable without producing a single document.

**Acceptance Scenarios**:

1. **Given** an account with no saved reports, **When** the list is opened, **Then** an explanation of what a saved report is and an action to create the first one are shown inside the same page structure as a populated list.
2. **Given** a report definition the account holder is happy with, **When** they save it under a name with an optional description, **Then** it is stored against their account and appears in their list.
3. **Given** a saved report, **When** it is opened, **Then** it shows the criteria it holds and how many records those criteria resolve to right now.
4. **Given** a saved report, **When** a new record matching its criteria is added afterwards and the saved report is opened again, **Then** the newly added record is included in what it resolves to, with nothing edited and no warning about staleness.
5. **Given** a saved report, **When** a record it previously resolved to is deleted, **Then** it simply resolves to one fewer record and does not report an error about a missing record.
6. **Given** a saved report, **When** the account holder edits any part of it — name, description, criteria, charts, presentation settings — **Then** the changes are stored and take effect the next time it is used.
7. **Given** the same saved report open for editing in two places, **When** it is saved in the second place after having been saved in the first, **Then** the second save is refused, the account holder is told it changed underneath them, and the current values are shown.
8. **Given** an account with a saved report of that name already, **When** another is saved under the same name ignoring capitalisation, **Then** it is refused with a message naming the conflict, and nothing is overwritten.
9. **Given** a saved report, **When** the account holder deletes it, **Then** they must confirm an action that names it, and documents already produced from it are unaffected.
10. **Given** a saved report belonging to another account, **When** it is requested by any means, **Then** the answer is indistinguishable from it not existing.
11. **Given** a saved report whose person can no longer be reached, **When** it is opened, **Then** it says so plainly, remains editable so another person can be chosen, and is never silently produced against a different person.
12. **Given** a saved report, **When** it is used to produce a document with a different date range just this once, **Then** the document reflects the override and the saved report is unchanged.
13. **Given** an account with saved reports, **When** a complete export is produced, **Then** the saved reports are in it.

---

### User Story 4 - Show how a number moved (Priority: P4)

Tomas has been on the same blood-pressure treatment for a year and his doctor keeps asking whether it is working. The answer is not in any single reading, it is in the shape of thirty of them. He adds two charts to his report: his systolic pressure over the last year, and his HbA1c over the last two. The application tells him which measured values he actually has enough readings of to be worth charting, and how many readings each has. His HbA1c was recorded in two different units by two different laboratories, so he is asked which one he means, and the chart he gets is honest about it.

**Why this priority**: A chart is what turns a list of numbers into an argument, and it is the single feature most likely to change what a clinician does in the room. It ranks after the report itself because a report without charts is already useful, and a chart without a report has nowhere to live.

**Independent Test**: Seed a person with twelve readings of one measured value in one unit, three of the same value in a different unit, and one reading of another value. Open the chart picker. Confirm it lists only the values with enough readings, states the count and the units for each, and offers the single-reading value as not chartable. Select one, choose a unit and a range, produce the report, and confirm the document contains a chart for it with a table of the same points beside it, and that no reading in the other unit appears on it.

**Acceptance Scenarios**:

1. **Given** a person with no measured values recorded at all, **When** the chart picker is opened, **Then** it explains that charts need repeated readings and how to record them, inside the standard page structure.
2. **Given** a person with readings of several measured values, **When** the chart picker is opened, **Then** it lists each value that has at least the documented minimum number of readings, with how many readings it has, over what span, and in which units they were recorded.
3. **Given** a measured value with fewer readings than the minimum, **When** the picker is opened, **Then** it is shown as not yet chartable with the number it has and the number it needs, rather than being hidden.
4. **Given** a measured value recorded in more than one unit, **When** it is chosen for a chart, **Then** a unit must be chosen, and readings recorded in any other unit do not appear on that chart and are never converted.
5. **Given** a chart selection with its own date range, **When** the range resolves to fewer readings than the minimum, **Then** the account holder is told at the moment they choose it and no empty chart is produced.
6. **Given** a report containing charts, **When** the produced document is read, **Then** every chart is accompanied by a table of the exact values it plots with their dates, so nothing depends on reading the picture.
7. **Given** a chart of a value that was recorded with a reference range, **When** it is drawn, **Then** the range is shown on the chart and stated in the accompanying table, and nothing on the chart depends on colour alone to be understood.
8. **Given** a chart in a saved report, **When** a reading is corrected afterwards and the report is produced again, **Then** the new document reflects the corrected reading.
9. **Given** more charts selected than the documented maximum for one report, **When** the report is asked for, **Then** the limit is stated and the report is not produced until the selection is reduced.
10. **Given** a chart naming a person the account holder cannot reach, **When** it is submitted by any means, **Then** it is refused indistinguishably from that person not existing.
11. **Given** charts of both a vital sign and a laboratory measurement, **When** the report is produced, **Then** both appear, each labelled with what it is, its unit, and the span it covers.
12. **Given** a series with more readings than a chart can legibly draw, **When** it is drawn, **Then** the document states that the series was reduced and by what rule, and the accompanying table states how many readings the reduction left out.

---

### User Story 5 - Know the instance is healthy, and manage who uses it (Priority: P5)

Piotr runs the instance for his extended family on a small machine in a cupboard. He does not want a monitoring stack; he wants one page that tells him the truth. Is it ready? How long has it been up? How much disk are the records and the documents taking? How many accounts, people and records are on it? When did the last backup actually succeed? Is the break-glass credential protected by a second factor and an address restriction, or has he been running exposed since he set it up in a hurry? Is anything stuck? And when his brother-in-law stops using it, he wants to disable that account without deleting three years of his mother's history along with it. He also decides whether the household signs in with passwords alone or through the identity provider the family already uses, because that is a decision about how this instance is deployed rather than a feature a patient asks for.

**Why this priority**: This is what makes a self-hosted instance safe to depend on rather than merely possible to install. It ranks below the person-facing stories because the application delivers no value to a patient without them, but no operator should be asked to run something they cannot see into.

**Independent Test**: Sign in as an administrator on a seeded instance, confirm every figure on the overview has a definition and a value, take a backup and confirm the last-backup figure moves, leave the break-glass protections unconfigured and confirm the warning appears both on the page and at every start-up, disable an account and confirm it can no longer sign in and its existing session ends, promote and demote another account, attempt to demote yourself and confirm refusal, and confirm that an account which is not an administrator gets nothing from any of it.

**Acceptance Scenarios**:

1. **Given** a brand-new instance with one account and nothing recorded, **When** the operator overview is opened, **Then** every figure shows zero with an explanation, inside the standard page structure, and nothing reports an error.
2. **Given** a running instance, **When** the operator overview is opened, **Then** it states whether the instance is ready to serve, how long it has been running, which version it is, how much storage the records take and how much the documents take separately, and how many accounts, people and records of each kind exist — each figure with a stated definition and the moment it was computed.
3. **Given** an instance where a backup has never succeeded, **When** the overview is opened, **Then** it says so as a warning rather than showing a blank or a zero.
4. **Given** an instance whose last successful backup is older than the documented threshold, **When** the overview is opened, **Then** the figure is presented as a warning with the age stated.
5. **Given** an instance where the break-glass credential has no second factor or no address restriction configured, **When** the instance starts and whenever the overview is opened, **Then** an unmistakable warning names exactly which protection is missing and what to do about it, and it keeps appearing until it is fixed.
6. **Given** an instance with failed or abandoned work — an export interrupted by a restart, a scheduled clean-up that did not run, a scheduled backup that failed — **When** the overview is opened, **Then** each is listed with what it was and when, so nothing fails silently.
7. **Given** any figure on the overview, **When** it is inspected, **Then** it is a count, a size, a duration, a version, a state or a moment in time — never a person's name, a date of birth, a diagnosis, a recorded value or a file name.
8. **Given** an account that is not an administrator, **When** it opens any operator view or attempts any operator action by any means, **Then** the answer is indistinguishable from the page not existing, nothing is disclosed, and the attempt is recorded.
9. **Given** an administrator, **When** they open the account list, **Then** they see each account's sign-in identity, display name, tier, whether it is disabled, when it was created, when it last signed in, and how many people it owns — and nothing about what those people's records contain.
10. **Given** an administrator, **When** they disable an account, **Then** that account's sessions end immediately, further sign-in is refused with a plain message telling the person to contact the operator, and the action is recorded.
11. **Given** an administrator, **When** they require an account to change its password, **Then** that account is asked to set a new one at its next sign-in and cannot reach anything else until it has, and the action is recorded.
12. **Given** an administrator, **When** they attempt to remove their own administrative tier or disable their own account, **Then** it is refused with an explanation, so that an instance cannot be locked out of its own administration.
13. **Given** an instance with exactly one enabled administrator, **When** an attempt is made to demote or disable that account, **Then** it is refused with an explanation naming the reason.
14. **Given** any administrative action on an account, **When** the activity trail is read, **Then** it appears marked as administrative, naming who acted, which account, what changed and when, and never a password or any record content.
15. **Given** an administrator on any operator view, **When** they look for a way to read a person's records, **Then** there is none; reaching records requires the break-glass credential, and every such session appears in the trail.
16. **Given** an instance on which the operator has configured an external identity provider, **When** somebody signs in through it, **Then** an account is reached or created according to the operator's setting, that account holds no administrative tier and no disabled state it did not already have, the sign-in is recorded like any other, and — on an instance with no provider configured — no provider is offered anywhere in the interface.

---

### User Story 6 - Find out who touched a record, and when (Priority: P6)

Ines shares her mother's chart with her brother. Two weeks later a medication entry is different from how she remembers it. She does not want a fight, she wants a fact: who changed it and when. She opens the trail, narrows it to her mother and to changes, and reads the answer. Piotr, running the instance, uses the same trail for a different question: has anybody failed to sign in fifty times overnight, and did anyone use the break-glass credential.

**Why this priority**: The trail is written from phase 001 onwards; without a way to read it, it is a promise nobody can check. Accountability is what makes sharing a chart survivable. It ranks here because the data exists and accrues correctly whether or not this view is built.

**Independent Test**: Perform a known series of actions as two different accounts on two different people. Open the trail. Confirm every action appears with who, what, which person and when, and no content of any kind. Narrow by person, by actor, by action, by kind and by date range, singly and combined, and confirm each narrowing returns exactly the matching entries. Page through and confirm no entry is repeated or skipped while new ones arrive. Export the narrowing as a table and confirm it holds the same rows. Confirm the second account sees only entries concerning what it can reach.

**Acceptance Scenarios**:

1. **Given** an instance where nothing has happened yet, **When** the trail is opened, **Then** it explains that nothing has been recorded yet, inside the standard page structure.
2. **Given** entries exist, **When** the trail is opened, **Then** they are shown newest first, each stating when it happened, who acted, what they did, what kind of thing it concerned, which person it concerned, and the opaque reference of the thing itself.
3. **Given** entries exist, **When** the reader narrows by person, by actor, by action, by kind of thing, or by a date range, singly or in combination, **Then** exactly the matching entries are returned and the narrowing in force is stated.
4. **Given** a long trail, **When** the reader pages through it while new entries are being written, **Then** no entry is shown twice and none is skipped.
5. **Given** a narrowing, **When** the reader exports it, **Then** they receive a tabular file containing the same entries with the same fields, and the export itself is recorded as an action.
6. **Given** any entry, **When** it is read, **Then** it contains no name of a person, no diagnosis, no recorded value, no note, no tag name and no file name — and the view never fetches such content to display beside it, including for a thing that still exists.
7. **Given** an entry concerning something that has since been deleted, **When** it is read, **Then** it still shows its opaque reference and its kind, and does not report an error.
8. **Given** any account, **When** it attempts to create, change or delete an entry through any part of the application, **Then** it is refused; entries are written by the system only.
9. **Given** an account that is not an administrator, **When** it reads the trail, **Then** it sees only entries concerning people it can reach and its own account, and entries about anything else are indistinguishable from not existing.
10. **Given** an administrator, **When** they read the trail, **Then** they additionally see sign-in failures, administrative sessions, use of the break-glass credential, backups, restores, scheduled clean-ups and other system actions, with system actions attributed to the system rather than to a person.
11. **Given** a refused attempt to reach something, **When** the trail is read, **Then** the refusal appears, so that probing leaves a trace.
12. **Given** the documented retention window, **When** the trail is opened, **Then** it states the window and the age of the oldest entry it still holds, and entries older than the window have been removed automatically.
13. **Given** an owner who opens their own person's records, **When** the trail is read afterwards, **Then** no entry was written for that reading; **and given** somebody who is not the owner opens the same records through an arrangement made by the owner, or through the break-glass credential, **Then** an entry was written.

---

### User Story 7 - Take a backup, and get back from a disaster (Priority: P7)

Piotr's cupboard machine is one power cut away from being a story he tells at parties. He wants a copy he can hold, a schedule that takes one without being asked, and — the part he has never actually tested anywhere — a restore that he can carry out at eleven at night, tired, without destroying the thing he is trying to save. Before he restores, he wants to be told what he is about to lose. And whatever else happens, he wants a copy of the current state taken automatically before the old one is put back.

**Why this priority**: Availability for a single-instance self-hosted application means restarting quickly from a backup; that is the whole recovery plan. It ranks here because the underlying capability already ships with the instance — what this phase adds is the guard rails that make using it at eleven at night survivable.

**Independent Test**: Take a backup and confirm it is listed with its size, its age and who took it. Add a record afterwards. Open the restore preview and confirm it states when the archive was taken, what exists now, and that everything created since will be lost. Attempt a restore without the confirmation phrase and confirm refusal. Restore properly, confirm a safety copy was taken first and its reference reported, confirm the instance returns on the restored data with the later record gone, and confirm that record is present in the safety copy. Then download an archive, confirm it matches what was taken, and delete one.

**Acceptance Scenarios**:

1. **Given** an instance where no archive has ever been taken, **When** the archives view is opened, **Then** it explains what an archive is and offers to take the first one, inside the standard page structure.
2. **Given** an administrator, **When** they take an archive with an optional note, **Then** it appears in the list with when it was taken, its size, how it was taken, who took it and the note, and the action is recorded.
3. **Given** the operator has configured a schedule, **When** it comes due, **Then** an archive is taken without being asked, no more than the configured number are kept, the oldest beyond that are removed, and a failure to take one is surfaced on the operator overview and in the trail rather than passing silently.
4. **Given** an archive taken on another machine, **When** an administrator uploads it, **Then** it is listed and treated identically to one taken here, and the upload is recorded.
5. **Given** an archive, **When** an administrator asks what restoring it would do, **Then** they are told when it was taken, its size, its note, which version of the application produced it, how much exists on the instance now that would be replaced, and explicitly that everything recorded since it was taken will be lost.
6. **Given** an archive, **When** a restore is attempted without the required confirmation phrase and without re-entering the administrator's password, **Then** it is refused and nothing is replaced.
7. **Given** a confirmed restore, **When** it begins, **Then** a safety copy of the current state is taken first and its reference is reported to the administrator before anything is replaced; **and given** the safety copy cannot be taken, **Then** the restore does not proceed.
8. **Given** a confirmed restore, **When** it runs, **Then** both the records and the stored documents are replaced by those in the archive, the administrator is told the instance will be briefly unavailable, and it returns either fully restored or unchanged — never partly.
9. **Given** an archive that cannot be read, fails its integrity check, or was produced by an incompatible version of the application, **When** a restore is attempted, **Then** it is refused before anything on the instance is touched, with a reason that names no storage location.
10. **Given** a restore in progress, **When** a second restore or a new archive is attempted, **Then** it is refused with an explanation.
11. **Given** a report or an export being produced, **When** a restore is confirmed, **Then** it is refused until that work finishes or is cancelled, with an explanation naming the reason.
12. **Given** an archive, **When** an administrator downloads it, **Then** they must re-enter their password, the download is authorized on every request rather than by possessing its address, it is byte-for-byte what was taken, and it is recorded as the most sensitive action on the instance.
13. **Given** an archive, **When** an administrator deletes it, **Then** they must confirm, only that archive is removed, and the action is recorded.
14. **Given** an account that is not an administrator, **When** it attempts to list, take, upload, preview, download, restore or delete an archive by any means, **Then** the answer is indistinguishable from the capability not existing and the attempt is recorded.
15. **Given** a completed restore, **When** the trail is read afterwards, **Then** the restore, the safety copy and their references appear, even though the restore replaced the very records the trail is kept in.

---

### User Story 8 - Nothing lingers after its window (Priority: P8)

Piotr's cupboard machine has a finite disk and a family that will never stop adding to it. He has been told that produced documents are kept for a week, that deleted attachments wait a month before they are removed for good, and that the activity trail keeps two years. He wants to see that those are real numbers being enforced rather than sentences in a handbook. On one page he sees each window in force, how much is currently waiting to be removed under each, and when each scheduled clean-up last ran. When a clean-up fails at three in the morning because the disk was briefly full, he does not find out three months later: the failure is on the page, in the trail, and the job tries again the next night.

**Why this priority**: A retention window that nothing enforces is not a retention window, and a purge nobody can see is indistinguishable from one that never ran. It ranks last of the capability stories because the enforcement of each window is delivered by the phase that created the thing being kept; what this phase adds is one place where an operator can see that all of it is actually happening, and the windows for the artifacts this phase itself produces.

**Independent Test**: Configure short windows. Produce a report and an export. Move past the window and confirm both are shown as expired, are no longer downloadable, their content is no longer stored, and producing them again is offered. Open the trail and confirm it states the window in force and the age of its oldest entry, and that entries older than the window are gone. Delete an attachment and confirm the operator overview's count and bytes for documents awaiting removal move, and that the overview points at where a document is recovered rather than offering a second place to do it. Make one scheduled clean-up fail and confirm the failure appears on the overview and in the trail exactly once, and that the next run retries it.

**Acceptance Scenarios**:

1. **Given** a produced report or a finished export archive whose retention window has passed, **When** the scheduled clean-up runs, **Then** the content is no longer stored, the request is shown as expired with the window that applied, and producing it again is offered.
2. **Given** an expired produced report or export archive, **When** its download is attempted by the account that requested it, **Then** it is refused with a plain statement that it expired, not an error suggesting something went wrong.
3. **Given** an instance with deleted documents awaiting removal, **When** the operator overview is opened, **Then** it states how many there are across the whole instance and how many bytes they occupy, names the window that applies, and shows neither a file name nor which person any of them concerns.
4. **Given** an operator looking at that figure, **When** they follow it, **Then** they are pointed at the place where a document is recovered by the account that can reach it, and no second recovery surface exists.
5. **Given** the activity trail, **When** it is opened, **Then** it states the retention window in force and the age of its oldest entry, and no entry older than the window remains.
6. **Given** every retention window the instance enforces, **When** the operator overview is opened, **Then** each is listed with its current value, what it applies to, and when the clean-up that enforces it last ran and last succeeded.
7. **Given** a scheduled clean-up that fails, **When** the operator overview and the trail are read, **Then** the failure appears once with what failed and when, is retried on the next scheduled run rather than immediately in a loop, and nothing is left half-removed.
8. **Given** an operator who changes a retention window, **When** the next scheduled clean-up runs, **Then** it enforces the new window, and every view stating that window states the new value.
9. **Given** the clock jumping forward or backward, **When** a clean-up runs, **Then** windows are measured in whole days from the recorded moment, and nothing is removed early and nothing escapes removal.
10. **Given** an account that is not an administrator, **When** it looks for the instance-wide retention figures by any means, **Then** the answer is indistinguishable from them not existing.
11. **Given** a person's account is deleted, **When** the instance is examined afterwards, **Then** that account's produced documents and export archives are gone immediately rather than waiting out their window.

---

### User Story 9 - Prove it is ready to ship (Priority: P9)

The application is finished. Before anybody is asked to keep their family's medical history in it, somebody has to be able to say — and show — that every page renders in a real browser without complaint, that the published description of what it serves matches what it actually serves, that it stays usable with a decade of records in it, that nothing personal has leaked into the operational record, and that an operator can run it from documentation alone.

**Why this priority**: Hardening belongs at the end because there is finally a complete application to harden. It ranks last because it adds no new capability; it establishes that everything before it is true.

**Independent Test**: Run the automated browser check across every page in the application and confirm each passes at both a desktop and a phone viewport, against both a populated account and an account holding nothing. Deliberately break one page and confirm the build goes red. Add a page without coverage and confirm the build goes red. Remove one operation from the published description and confirm the build goes red. Run the exercise that loads an instance to the documented volumes and confirm each budget is met. Run the search of logs, measurements, traces and error reports after exercising every operation and confirm no personal or clinical content is present. Hand the operator handbook to somebody who has never seen the application and have them go from a clean machine to a running, backed-up and restored instance without asking a question.

**Acceptance Scenarios**:

1. **Given** the finished application, **When** the automated browser check runs, **Then** every user-facing page delivered by any phase returns successfully, shows its expected landmarks, and produces zero browser errors, zero uncaught page failures and zero failed resource requests, at both a desktop and a phone viewport.
2. **Given** an account holding nothing at all, **When** the automated browser check runs against it, **Then** every page that can be empty shows its explanation inside its own page landmark and the check passes, so that an empty instance never turns the gate falsely red.
3. **Given** a page deliberately broken so the browser reports an error, **When** the check runs, **Then** it fails, and the failure names the page.
4. **Given** a new page added without coverage, **When** the build runs, **Then** it fails, because the list of pages under test is derived from the application itself rather than maintained by hand.
5. **Given** an operation added, changed or removed without the published description being updated, **When** the build runs, **Then** it fails.
6. **Given** an instance loaded to the documented volumes, **When** the budgeted journeys are exercised, **Then** each completes within its published budget, and a regression beyond a budget fails the build rather than being noticed later.
7. **Given** every operation in the application exercised end to end, **When** the instance's logs, measurements, traces and error reports are searched, **Then** no personal or clinical content of any kind appears in them, including names, dates of birth, notes, values, tag names and file names.
8. **Given** a view that updates live, **When** it is left open for longer than ten minutes, **Then** it is still receiving updates, and this is proven by an automated test that holds one open for longer than that rather than assumed.
9. **Given** an instance with no outbound destinations configured, **When** every operation in the application is exercised, **Then** it makes no outbound network connection of any kind.
10. **Given** every operation that touches a person's data, **When** the test suite is inspected, **Then** each carries tests proving a stranger is refused, an entitled reader succeeds, a reader whose access ended is refused, and — where the operation writes — a read-only reader is refused.
11. **Given** the operator handbook, **When** somebody who has never run the application follows it, **Then** they reach a running instance, take a backup, restore it, and understand every setting, every retention window and every figure on the operator overview, without needing to ask anybody.
12. **Given** the finished application, **When** a privacy review is carried out against the stated privacy rules, **Then** it is written down, every finding is either fixed or recorded with a reason, and the review is repeatable rather than a one-off reading.

---

### Edge Cases

**Nothing there yet**

- A person with no records, an account with no saved reports, an account with no exports, an instance with no archives, and an instance with an empty trail: each view has its own explanation and its own next action, rendered inside the standard page structure, so the automated browser check does not go falsely red on a legitimately empty instance.
- A brand-new instance shows zeros on the operator overview with definitions, never a blank tile, an error, or a figure that cannot be distinguished from a failure to compute it.
- A person who has records but none matching a selection is told the selection matched nothing, which is a different message from having nothing recorded at all.
- A measured value with a single reading is shown as not yet chartable with the number needed, rather than being hidden as though it did not exist.

**Two things happening at once**

- The same saved report is edited in two places: the later save is refused, told it changed underneath, and shown the current values. Criteria are never merged.
- A report is being produced while the saved report it came from is edited: the document reflects the criteria as they were when production started, and says so on its first page.
- A saved report is deleted while a document is being produced from it: the production completes and the document remains downloadable; deleting the definition never destroys what was already produced.
- Two exports are requested at once: both are accepted, they run in order, and the waiting one shows its position rather than appearing stalled.
- An export is running when a restore is confirmed: the restore is refused until the export finishes or is cancelled, because a restore replaces the storage the export is writing into.
- An archive is taken while an export is running: both complete, and the archive either contains the finished artifact or does not, never a half-written one.
- A record is deleted while a report that includes it is being produced: the document contains it or does not, and never a half-written section or a reference to something it does not contain.
- An account is disabled by an administrator while that account is producing an export: the work stops, the artifact is not downloadable, and the account is told why at its next sign-in attempt.
- An administrator's own tier is changed by a second administrator while they are on an operator view: their next action is authorized against the tier they now hold, not the one the page was rendered with.

**Partial and awkward data**

- A saved report whose criteria now match nothing: it opens, reports zero, and stays editable; it is never silently produced as an empty document.
- A saved report naming a person who has been deleted, or whose access has been withdrawn: it says so plainly, refuses to produce, and offers to point at a person the account can still reach.
- A chart whose readings were recorded in more than one unit: a unit is required, the other readings are excluded, the exclusion is stated, and no conversion is ever performed.
- A chart whose readings are all on one day: it is drawn honestly as a small number of points, not stretched across an invented span.
- A record whose optional fields are empty appears in a report with those fields absent rather than shown blank or as zero.
- A record with an unusually long note appears in the document wrapped and complete rather than truncated silently.
- A person's name, a tag or a document name containing non-Latin script, right-to-left text or characters that look like markup: carried exactly in the export, never interpreted as anything other than text anywhere, and — where the produced document's typefaces cannot render a character faithfully — counted and stated on the document's first page rather than silently altered.
- A date-only value appears as the same calendar date in a produced document and in an export regardless of the viewer's time zone or device clock.
- An archive uploaded from a different version of the application: the preview states the version it was taken with, and a restore from an incompatible version is refused with an explanation rather than attempted.
- An activity entry whose actor has since been deleted: the entry survives and shows an opaque reference rather than an error or a blank.

**Permission boundaries**

- Guessing or enumerating the reference of a saved report, an export, a produced document, an archive or an activity entry belonging to somebody else yields the same answer as a reference that never existed.
- The address of a produced document or an archive is not a credential: possessing it grants nothing, and passing it to somebody else gives them nothing.
- An account that is not an administrator reaches no operator view and no operator action by any route, including by addressing them directly, and every attempt is recorded.
- An administrator can see counts about an account but cannot read that account's records through any operator view; reading records requires the break-glass credential, whose every session is recorded.
- An administrator cannot demote or disable themselves, and the last enabled administrator cannot be demoted or disabled by anybody, so an instance cannot lock itself out of its own administration.
- An account required to change its password can reach the password change and nothing else, including a report, an export or an operator view, until it has changed it.
- What a report or an export contains is decided by what the requester can reach at the moment of production, not at the moment the definition was saved: access withdrawn in between removes the data from the result and the removal is stated in the export's manifest.
- The activity trail read by somebody who is not an administrator shows only what concerns people they can reach and their own account; the existence of everything else is not disclosed, not even as a count.
- An account that can reach a person only through an arrangement made by somebody else may produce a report or an export of no more than that arrangement allows it to read, and doing so is itself recorded.

**Deletion and what depends on it**

- Deleting an account destroys its saved reports, its export requests, its produced documents and its export archives, along with everything it owned.
- Deleting a person leaves saved reports that named them reporting plainly that the person is gone, and never silently reassigns them.
- Deleting a produced document's underlying records does not change a document already produced; a produced document is a snapshot and says on its face when it was taken.
- Removing an instance archive removes only that archive; other archives, the safety copies and the trail are untouched.
- Restoring an archive replaces records and stored documents wholesale: anything recorded since it was taken is lost, which is why a safety copy is always taken first and why the preview states it in those words before anything is confirmed.
- A restore destroys the very records the activity trail is kept in, so the account of the restore itself survives the restore and is readable afterwards.
- Purging is permanent: once a retention window closes, nothing in the application can bring back what it removed.
- Activity entries cannot be deleted by anybody through the application; they leave only by ageing past the retention window.

**Very large record sets and long-running work**

- A person with 10,000 records: the report builder's counts, the selection and the resolved figure stay responsive, and the produced document is paginated with page numbers and section headings rather than becoming one unnavigable block.
- A selection resolving to more than the documented maximum for a single report: the account holder is told the limit and asked to narrow before anything is produced.
- A measured value with 500 readings charted over five years: the chart stays legible, and where the series must be reduced for legibility the document states that it was reduced and by what rule.
- A complete export of an account holding 10,000 records and 2,000 documents: it completes, reports progress throughout, never holds the requester waiting on a page, and does so within a bounded amount of memory rather than assembling the whole archive first.
- An activity trail of several million entries: narrowing, paging and exporting stay usable, and the export streams rather than being assembled in one piece.
- An archive of several gigabytes: taking, downloading and restoring it all complete, and the progress of each is visible.
- Paging the trail, the export list, the archive list or the account list while entries are being added never shows the same item twice and never skips one.

**Environment failures**

- Storage fills while a report, an export or an archive is being written: the work fails with a reason that names no storage location, nothing partial is downloadable or restorable, and the failure is visible to both the account holder and the operator.
- The instance is restarted while an export or a report is being produced: the request is reported as failed with a plain reason and offered for retry, never left reporting itself as running.
- The instance is restarted during a restore: it returns either on the archive's data or on the data it had before, never on a mixture, and the trail records what happened.
- A scheduled clean-up, a scheduled backup or the production worker fails: each failure is recorded once, surfaced on the operator overview, and retried on its next scheduled run, and none of them leaves data half-processed.
- The clock moves backwards or jumps forward: retention windows are measured in whole days from the recorded moment, and a jump neither removes anything early nor prevents removal.
- The break-glass credential is used: the session appears in the trail as an administrative session even though it bypasses the application's own permission checks by design.
- The instance is started with a retention window, a limit or a threshold set to a value it cannot honour: it refuses to start with a message naming the setting, rather than starting and quietly ignoring it.

## Requirements *(mandatory)*

### Functional Requirements

#### Defining and producing a report

- **FR-001**: The system MUST allow an account holder to define a report over exactly one person, and MUST refuse a definition naming a person the account holder cannot reach, in a way indistinguishable from that person not existing.
- **FR-002**: A report definition MUST be able to select which record kinds to include, a date range, tags, and lifecycle values, and to state whether the person's identifying header and photograph are included.
- **FR-003**: The system MUST show, before anything is produced, how many records each selected kind contributes and the total, and those figures MUST match exactly what a report over the same selection would contain.
- **FR-004**: The system MUST refuse to produce a report whose selection resolves to no records, explaining that nothing matched and leaving the selection intact.
- **FR-005**: The system MUST acknowledge a request to produce a report immediately without holding the requester waiting, MUST report its progress, and MUST make it available when it is ready even if the requester navigated away.
- **FR-006**: A produced report MUST be a single self-contained document that opens and prints on any device without the application, with numbered pages and the person identified on every page; where a character cannot be rendered faithfully, the document MUST state that on its first page and count the affected entries rather than dropping or silently substituting anything.
- **FR-007**: A produced report MUST state on its first page which person it concerns, the moment it was produced, and a plain-language statement of the criteria that produced it.
- **FR-008**: A produced report MUST contain one section per selected kind in a documented order, each record showing its own meaningful fields together with its notes and tags, and MUST state explicitly where a selected kind matched nothing rather than omitting it silently.
- **FR-009**: A report definition MUST be able to carry presentation settings — at least the ordering and grouping of records within a section — and the produced document MUST honour them.
- **FR-010**: The system MUST refuse to produce a report whose selection exceeds the documented maximum number of records for one document, stating the limit and asking for a narrower selection before producing anything.
- **FR-011**: A produced report MUST contain only records the requester could reach at the moment production began, re-checked at that moment, and MUST NOT contain any record belonging to any other person.
- **FR-012**: A produced report MUST be retained for a documented window, MUST be downloadable by the account that requested it until then, and MUST be purged automatically afterwards, with the request then shown as expired and re-production offered.
- **FR-013**: A produced report MUST be reachable only through a request authorized against the account that requested it; its address MUST NOT act as a credential, and no other account — including an administrator — MUST be able to download it.
- **FR-014**: Producing a report and downloading a report MUST each be recorded in the activity trail with who acted, which person it concerned and when, and MUST NOT record any field value, name, note, tag name or file name.
- **FR-015**: Producing the same definition twice MUST yield two independent documents, each downloadable and each with its own production moment.

#### Charts of measured values within a report

- **FR-016**: The system MUST be able to tell an account holder which measured values recorded for a person — both vital signs and laboratory measurements — have enough readings to be charted, how many readings each has, over what span, and in which units they were recorded.
- **FR-017**: The system MUST publish the minimum number of readings required to chart a value, MUST show values below that minimum as not yet chartable together with how many they have and how many they need, and MUST NOT hide them.
- **FR-018**: When a value has readings in more than one unit, the system MUST require a unit to be chosen for a chart, MUST exclude readings in every other unit from that chart, MUST state that they were excluded, and MUST NOT convert between units under any circumstances.
- **FR-019**: Each chart MUST be able to carry its own date range independent of the report's range, and the system MUST report at the moment of selection when a chart's range resolves to fewer readings than the published minimum.
- **FR-020**: Every chart in a produced document MUST be accompanied by a table of the exact values it plots with their dates and units, so that no information is available only by reading the picture.
- **FR-021**: Every chart MUST label its axes with the unit and the dates, MUST show the reference range where one was recorded with the readings, and MUST NOT convey any meaning by colour alone.
- **FR-022**: Charts MUST be computed from the readings as they stand when the document is produced, so that correcting a reading changes the next document produced.
- **FR-023**: The system MUST publish a maximum number of charts for one report and MUST state that limit rather than silently dropping charts beyond it.
- **FR-024**: A chart naming a person the requester cannot reach MUST be refused indistinguishably from that person not existing; and where a series carries more readings than can be drawn legibly, the document MUST state that the series was reduced and by what rule.

#### Saved reports

- **FR-025**: The system MUST allow an account holder to save a report definition under a name, with an optional description, against their own account.
- **FR-026**: A saved report MUST store the criteria of the selection and MUST NOT store a fixed list of records; it MUST resolve against current data every time it is opened or used.
- **FR-027**: The system MUST show, when a saved report is opened, how many records its criteria resolve to at that moment.
- **FR-028**: The system MUST allow every part of a saved report to be changed — name, description, criteria, chart selections and presentation settings — and MUST allow it to be deleted behind a confirmation that names it.
- **FR-029**: The system MUST refuse a save to a saved report that has been changed elsewhere since it was opened, telling the account holder it changed underneath them and showing the current values.
- **FR-030**: Saved report names MUST be unique within an account ignoring capitalisation, and a duplicate MUST be refused with a message naming the conflict rather than overwriting anything.
- **FR-031**: A saved report MUST be private to the account that owns it; another account's saved report MUST be indistinguishable from one that does not exist, and sharing saved reports with another account is not offered in this phase.
- **FR-032**: A saved report naming a person the account can no longer reach MUST report that plainly, MUST refuse to produce, MUST remain editable so another person can be chosen, and MUST NOT be produced against a different person.
- **FR-033**: The system MUST allow a saved report to be produced with a date range overridden for that production only, leaving the saved definition unchanged.
- **FR-034**: Deleting a saved report MUST NOT affect any document already produced from it.
- **FR-035**: Saved reports MUST be included in the account's portable export.

#### Portable export

- **FR-036**: The system MUST allow an account holder to request an export of everything their account holds, and MUST allow that request to be narrowed to selected people, selected kinds, a date range, and with or without document content.
- **FR-037**: An export MUST be produced as a single archive containing, at minimum, every selected record in a machine-readable structured form, and MUST be able to additionally contain the same records as tabular files that open in a spreadsheet.
- **FR-038**: An export archive MUST contain a manifest stating the format version, the moment of production, which account, which people, which kinds, the count of each, and the meaning of every other file in the archive.
- **FR-039**: The export format MUST be documented and published with the release, MUST be versioned, and MUST be readable without the application or any part of it.
- **FR-040**: Where document content is included, each document MUST appear under a name that unambiguously identifies which entry it belongs to while preserving its original name, and the manifest MUST record the mapping.
- **FR-041**: A complete export MUST include the account's people, records of every kind, document content when chosen, tags, saved reports, preferences, and the activity entries concerning the exported people.
- **FR-042**: Activity entries within an export MUST identify any actor other than the exporting account by opaque reference only, and MUST carry no content, exactly as they carry none inside the application.
- **FR-043**: An export MUST NOT contain anything concerning a person the account cannot reach, anything belonging to another account, any credential, any secret, or any operator setting.
- **FR-044**: The system MUST acknowledge an export request immediately without holding the requester waiting, MUST report progress and the finished size, and MUST allow the requester to leave and return.
- **FR-045**: The system MUST produce at most one export or report at a time on an instance, MUST queue the rest in the order they were requested, and MUST show a waiting request its position.
- **FR-046**: The system MUST allow a queued or running export to be cancelled, after which nothing partial is downloadable and the cancellation is recorded.
- **FR-047**: An export archive MUST be retained for a documented window, MUST be downloadable by the requesting account until then, and MUST be purged automatically afterwards, with the request shown as expired and re-running offered.
- **FR-048**: Downloading an export archive MUST be authorized against the requesting account on every request, MUST NOT be possible by possessing its address alone, and MUST be recorded in the activity trail.
- **FR-049**: An export interrupted by a restart MUST be reported as failed with a plain reason and offered for retry, and MUST NOT be left reporting itself as running.
- **FR-050**: A failure reason for an export or a report MUST be a plain statement carrying no record content, no file name and no storage location.
- **FR-051**: Deleting an account MUST destroy its export requests, its archives and its produced documents immediately.
- **FR-052**: Reading an export archive back into an instance is NOT offered by this phase; restoring an instance from an archive taken by the operator is the supported route back, and the documentation MUST say so plainly.

#### Retention enforced, and visible, across the instance

- **FR-053**: Every retention window the instance enforces MUST have a documented default, MUST be an operator setting, MUST be measured in whole days from the recorded moment, and MUST be stated in the view of the thing it governs.
- **FR-054**: The system MUST enforce every retention window with scheduled work that runs without an operator having to ask, and MUST NOT rely on anything being read or opened for a window to take effect.
- **FR-055**: The operator overview MUST list every retention window in force with its current value, what it applies to, when the work that enforces it last ran, and when it last succeeded.
- **FR-056**: The operator overview MUST state how many deleted documents are awaiting removal across the whole instance and how many bytes they occupy, and MUST NOT show any of their names, their descriptions, or which person they concern.
- **FR-057**: The operator overview MUST point at the place where a deleted document is recovered by an account that can reach it, and the system MUST NOT provide a second surface for listing, restoring or removing deleted documents.
- **FR-058**: A failure of any scheduled clean-up MUST be recorded once, MUST be surfaced on the operator overview with what failed and when, MUST be retried on its next scheduled run rather than immediately, and MUST NOT leave anything half-removed.
- **FR-059**: Changing a retention window MUST take effect on the next scheduled run, and every view stating that window MUST state the new value.
- **FR-060**: Once anything has been removed by an enforced retention window, it MUST NOT be recoverable by any means available within the application.
- **FR-061**: A clock that jumps forward or backward MUST NOT cause anything to be removed before its window has elapsed, and MUST NOT prevent removal once it has.
- **FR-062**: The instance-wide retention figures MUST be reachable only by an administrator, and MUST be indistinguishable from not existing to any other account.
- **FR-063**: Deleting an account MUST remove its produced documents and export archives immediately rather than leaving them to age out of their window.

#### Reading the activity trail

- **FR-064**: The system MUST provide a view of the activity trail showing entries newest first, each stating when it happened, who acted, what they did, what kind of thing it concerned, which person it concerned, and the opaque reference of the thing.
- **FR-065**: The system MUST allow the trail to be narrowed by person, by actor, by action, by kind of thing and by date range, singly and in combination, and MUST state the narrowing in force.
- **FR-066**: The system MUST page the trail without ever repeating or skipping an entry while new entries are being written.
- **FR-067**: The system MUST allow the current narrowing to be exported as a tabular file containing the same entries and the same fields, and MUST record that export as an action.
- **FR-068**: An activity entry MUST NOT contain any content — no person's name, no diagnosis, no recorded value, no note, no tag name, no file name — and the view MUST NOT retrieve such content to display alongside an entry, including where the thing referred to still exists.
- **FR-069**: An entry referring to something since deleted MUST still display with its kind and its opaque reference, and MUST NOT report an error.
- **FR-070**: Activity entries MUST NOT be creatable, editable or deletable by any account through any part of the application; they are written by the system alone and leave only by ageing past the retention window.
- **FR-071**: An account that is not an administrator MUST see only entries concerning people it can reach and its own account; the existence of any other entry MUST NOT be disclosed, not even as a count.
- **FR-072**: An administrator MUST see every entry on the instance, including sign-in failures, administrative sessions, use of the break-glass credential, backups, restores, scheduled clean-ups and other system actions, with system actions attributed to the system rather than to a person.
- **FR-073**: A refused attempt to reach something MUST appear in the trail, so that probing leaves a trace.
- **FR-074**: The trail view MUST state the retention window in force and the age of the oldest entry it still holds.
- **FR-075**: Reading the trail MUST NOT itself create an entry; exporting it MUST. The documentation MUST state this so that an absence of read entries is not mistaken for an absence of reads.

#### The operator overview

- **FR-076**: Every operator view and every operator action MUST be reachable only by an account holding the administrative tier; every other account MUST receive an answer indistinguishable from the view or action not existing, and the attempt MUST be recorded.
- **FR-077**: The operator overview MUST state whether the instance is ready to serve, how long the process has been running, and which version of the application it is.
- **FR-078**: The operator overview MUST state how much storage the records occupy and how much the stored documents occupy, as two separate figures.
- **FR-079**: The operator overview MUST state how many accounts, how many people, how many records of each kind, how many stored documents, how many live sharing arrangements and how many outstanding invitations exist.
- **FR-080**: Every figure on the operator overview MUST carry a stated definition and the moment at which it was computed, and a figure that is not computed at the moment of viewing MUST show how old it is.
- **FR-081**: On an instance where nothing has happened yet, every figure MUST show zero with its explanation, and MUST NOT show a blank, an error, or anything a reader could mistake for a failure to compute it.
- **FR-082**: The operator overview MUST state when a backup last succeeded, its size and its age; MUST present the absence of any successful backup as a warning rather than a blank or a zero; and MUST present an age beyond a documented threshold as a warning stating the age.
- **FR-083**: The system MUST detect whether the break-glass credential is protected by a second factor and by an address restriction, MUST warn unmistakably both when the instance starts and whenever the operator overview is opened when either is missing, MUST name which one is missing and what to do about it, and MUST keep warning until it is fixed.
- **FR-084**: The operator overview MUST state whether outbound email is configured, because features of earlier phases depend on it and silently drop messages without it.
- **FR-085**: The operator overview MUST list failed or abandoned work — work interrupted by a restart, a scheduled clean-up that did not run, a scheduled backup that failed — with what it was and when, so that nothing fails silently.
- **FR-086**: Every figure and every value on any operator view MUST be a count, a size, a duration, a version, a state, a moment in time, or a setting's value — and MUST NOT be a person's name, a date of birth, a diagnosis, a recorded value, a note, a tag name or a file name.
- **FR-087**: The operator overview MUST state the current value of every configurable limit and window this phase introduces, so that an operator never has to read anything but the application to learn one.
- **FR-088**: No operator view MUST offer any route to reading a person's records; reaching a person's records requires the break-glass credential, and every such session MUST appear in the activity trail as an administrative session.

#### Administering accounts

- **FR-089**: An administrator MUST be able to list accounts, seeing for each its sign-in identity, display name, tier, whether it is disabled, when it was created, when it last signed in and how many people it owns — and nothing about what those people's records contain.
- **FR-090**: An administrator MUST be able to disable an account, which MUST end that account's existing sessions immediately rather than at their natural expiry, and MUST be recorded.
- **FR-091**: A disabled account attempting to sign in with correct credentials MUST be refused with a plain message telling the person to contact the operator; an attempt with incorrect credentials MUST answer identically whether or not the account is disabled, so that disabling cannot be detected from outside.
- **FR-092**: An administrator MUST be able to re-enable a disabled account, after which it signs in normally, and the action MUST be recorded.
- **FR-093**: An administrator MUST be able to require an account to set a new password, after which that account MUST be able to reach the password change and nothing else until it has changed it; setting a new password MUST clear the requirement, and both the requirement and its clearing MUST be recorded.
- **FR-094**: An administrator MUST be able to grant and remove the administrative tier on another account, and each change MUST be recorded.
- **FR-095**: The system MUST refuse an attempt by any account to remove its own administrative tier or to disable itself, with an explanation.
- **FR-096**: The system MUST refuse any attempt to demote or disable the last enabled administrative account, by anybody, with an explanation naming the reason, so that an instance cannot lock itself out of its own administration.
- **FR-097**: The administrative tier and the disabled state MUST NOT be settable through registration or through any self-service action; the operator surface MUST be the only way either changes.
- **FR-098**: Every administrative action on an account MUST be recorded and marked as administrative, naming who acted, which account was affected, what changed and when, and MUST NOT record a password or any record content.

#### Sign-in through an external identity provider

- **FR-134**: The system MUST allow a person to sign in through an external identity provider that the operator has configured, and MUST offer no such route in the interface when no provider is configured.
- **FR-135**: A sign-in through a provider MUST NOT be able to set or change an account's tier or disabled state, MUST refuse a disabled account exactly as a password sign-in does, and MUST be recorded in the activity trail in the same way as any other sign-in.
- **FR-136**: The operator MUST be able to see on the operator overview whether external sign-in is available on this instance, alongside the other posture figures, and MUST configure providers through the instance's own administrative surface rather than through a second mechanism this application invents.
- **FR-137**: Where an address arriving from a provider already has an account, the system MUST link the two rather than creating a second account for that address, and MUST NOT allow one person's provider identity to be attached to another person's account.

#### Backup and restore

- **FR-099**: The system MUST list the archives the instance holds, each with when it was taken, its size, how it came to be taken, who took it and the note it carries.
- **FR-100**: An administrator MUST be able to take an archive on demand with an optional note, and the action MUST be recorded.
- **FR-101**: The system MUST be able to take archives on a schedule the operator configures, MUST keep no more than the configured number and remove the oldest beyond it, and MUST surface a failure to take one on the operator overview and in the activity trail rather than letting it pass silently.
- **FR-102**: An administrator MUST be able to upload an archive taken elsewhere, and once accepted it MUST be listed and treated identically to one taken on this instance, with the upload recorded.
- **FR-103**: Before any restore, the system MUST be able to state what restoring an archive would do: when it was taken, its size, its note, which version of the application produced it, how much exists on the instance now that would be replaced, and explicitly that everything recorded since it was taken will be lost.
- **FR-104**: A restore MUST require both a typed confirmation phrase naming the archive and re-entry of the administrator's password, and MUST be refused without either, with nothing replaced.
- **FR-105**: A restore MUST take a safety copy of the current state before anything is replaced and MUST report that copy's reference to the administrator; if the safety copy cannot be taken, the restore MUST NOT proceed.
- **FR-106**: A restore MUST replace both the records and the stored documents with those in the archive, MUST tell the administrator the instance will be briefly unavailable, and MUST leave the instance either fully restored or unchanged — never partly.
- **FR-107**: The system MUST refuse a restore from an archive that cannot be read, that fails its integrity check, or that was produced by an incompatible version of the application, before anything on the instance is touched, with a reason that names no storage location.
- **FR-108**: The system MUST allow only one archive operation to run at a time, refusing a second with an explanation, and MUST refuse a restore while a report or an export is being produced.
- **FR-109**: Downloading an archive MUST require re-entry of the administrator's password, MUST be authorized on every request rather than by possession of its address, MUST deliver byte-for-byte what was taken, and MUST be recorded as the most sensitive action the instance offers.
- **FR-110**: An administrator MUST be able to delete an archive behind a confirmation, removing only that archive and leaving other archives, safety copies and the activity trail untouched, and the deletion MUST be recorded.
- **FR-111**: The account of a restore — the restore itself, the safety copy taken before it and both references — MUST survive the restore and MUST be readable in the activity trail afterwards, even though the restore replaces the records the trail is kept in.
- **FR-112**: The system MUST NOT provide a second backup or restore mechanism alongside the one the instance already ships with; what this phase adds is the preview, the mandatory safety copy, the confirmation, the authorization and the visibility of failures.
- **FR-113**: The instance MUST refuse to start when a retention window, a limit or a threshold is configured to a value it cannot honour, naming the setting, rather than starting and quietly ignoring it.

#### Privacy and accountability

- **FR-114**: An activity entry MUST record the actor, the action, the kind of thing acted on, the opaque reference of that thing, the person it concerned where there is one, and the moment — and MUST NOT record content of any kind.
- **FR-115**: The system MUST record the reading of a person's records or documents only when the reader is not the owner — a recipient of an arrangement the owner made, or a break-glass session. An owner reading their own MUST NOT produce an entry, because recording every self-read produces unusable noise and builds a timeline of when a person read their own most sensitive results, which is itself an exposure.
- **FR-116**: Producing, downloading and cancelling a report or an export, exporting the trail, every administrative action, every archive operation and every refused attempt at any of them MUST be recorded.
- **FR-117**: No personal or clinical content of any kind MUST appear in the instance's logs, measurements, traces or error reports as a result of anything this phase adds, including names, dates of birth, notes, recorded values, tag names and file names.
- **FR-118**: Every failure message, error code and operator-facing reason produced by this phase MUST carry no record content, no file name and no storage location.
- **FR-119**: The instance MUST make no outbound network connection that the operator has not explicitly configured, and MUST make none at all when nothing is configured.
- **FR-120**: A break-glass session MUST appear in the activity trail as an administrative session even though it bypasses the application's own permission checks by design.

#### Scale

- **FR-121**: The system MUST keep the report builder, the exports view, the activity trail, the archive list, the account list and the operator overview correct and responsive, within the times stated in the success criteria, on an instance holding the documented volumes.
- **FR-122**: The system MUST page the activity trail, the export list, the archive list and the account list so that entries are neither repeated nor skipped while new ones are being written.
- **FR-123**: Producing an export, producing a report and exporting the activity trail MUST each stream their output within a bounded amount of memory rather than assembling the whole result before delivering any of it.

#### Verification and release hardening

- **FR-124**: Every acceptance scenario in this specification MUST exist as an automated test, and the phase MUST NOT be considered complete until every one of them passes.
- **FR-125**: Every page this phase adds and every page it changes MUST be covered by the project's automated browser check at both the desktop and mobile sizes it defines, run against both a populated account and an account holding nothing at all; a page added without such coverage MUST fail the build.
- **FR-126**: The automated browser check MUST cover every user-facing page delivered by phases 001 to 006, asserting a successful response, the page's expected landmark, and zero browser errors, zero uncaught page failures and zero failed resource requests; the list of pages under test MUST be derived from the application itself rather than maintained by hand.
- **FR-127**: The published description of the operations the application serves MUST agree with what it actually serves; an operation added, changed or removed without the description being updated MUST fail the build.
- **FR-128**: Every operation this phase adds MUST carry automated tests proving that an account with no access is refused, that an entitled account succeeds, that an account whose access has ended is refused, that an account without the administrative tier is refused from every operator operation, and — where the operation writes — that a read-only reader is refused.
- **FR-129**: A view that updates live MUST be proven still to be receiving updates after more than ten continuous minutes by an automated test that holds one open for longer than that, rather than by assumption.
- **FR-130**: The privacy rules of this specification MUST be verified by an automated exercise of every operation in the application, every scheduled job and every production worker, which then inspects the instance's logs, measurements, traces and error reports and finds no personal or clinical content in any of them, and no opaque identifier used as a measurement label.
- **FR-131**: A performance budget MUST be published for each of the journeys named in the success criteria, MUST be asserted against an instance loaded to the documented volumes, and a regression beyond a budget MUST fail the build rather than being noticed later.
- **FR-132**: A privacy review of the finished application MUST be carried out against the stated privacy rules, MUST be written down with every finding either fixed or recorded with a reason, and MUST be repeatable, so that running it again after a change produces a comparable result.
- **FR-133**: An operator handbook MUST be published that takes somebody who has never run the application from a clean machine to a running, backed-up and restored instance, and MUST document every setting, every retention window, every limit and every figure on the operator overview.

### Key Entities

- **Report definition**: The question a report asks — one person, which kinds of record, over what dates, with which tags and lifecycle values, which charts, how the sections are ordered and grouped, and whether the identifying header and photograph appear. It names records by criteria and never by a fixed list, so it never rots.
- **Saved report**: A report definition kept under a name with an optional description, private to the account that owns it. Resolves against current data every time it is opened or used, so it picks up whatever has been recorded since.
- **Produced report document**: A single self-contained document produced from a definition at a moment in time, downloadable only by the account that asked for it, retained for a documented window and then gone. It is a snapshot and says on its face when it was taken and what criteria produced it.
- **Chart selection**: One measured value, one unit, and an optional date range of its own, chosen for inclusion in a report. Excludes every reading in any other unit and never converts between units.
- **Export request**: An account holder's request for a portable copy of what their account holds, optionally narrowed to some people, some kinds, a date range and with or without document content. Moves from waiting through running to exactly one of finished, failed, cancelled or expired, carries its position in the queue while waiting and its progress while running.
- **Export archive**: The single archive an export request produces, retained for a documented window. Contains structured records, optional tabular files, optional document content and a manifest, and nothing that concerns anybody the account cannot reach.
- **Export manifest**: The archive's map of itself — format version, moment of production, which account and which people, which kinds and how many of each, whether documents were included, where each document sits and which entry it belongs to, which people were dropped because access ended, and the meaning of every other file present. It is what makes the archive readable without the application.
- **Activity entry**: One thing that happened — who acted, what they did, the kind of thing it concerned, that thing's opaque reference, the person it concerned where there is one, and when. Never content. Written only by the system, never editable or deletable, and leaves only by ageing past the retention window.
- **Operator overview figure**: One measurement of the instance — a count, a size, a duration, a version, a state, a moment or a setting's value — carrying its own definition and the moment it was computed. Never a name, a value or anything belonging to a person.
- **Retention window**: A documented number of whole days after which the system removes something without being asked — a produced document, an export archive, a deleted attachment, an activity entry. Has a default, is an operator setting, is stated wherever the thing it governs is shown, and is enforced by scheduled work whose last run and last success are visible.
- **Instance archive**: A complete copy of the instance's records and stored documents at a moment in time, taken on demand or on a schedule or uploaded from elsewhere, carrying when it was taken, its size, how it came about, who took it, its note and the version of the application that produced it.
- **Safety copy**: The instance archive taken automatically immediately before a restore, so that a restore is reversible. Its reference is reported to the administrator before anything is replaced, and a restore that cannot take one does not happen.
- **Restore preview**: The statement of what a restore would do before it is confirmed — when the archive was taken, what it holds, what exists now that would be replaced, and in plain words that everything recorded since will be lost.

## Success Criteria *(mandatory)*

### Measurable Outcomes

A criterion marked **[outcome metric]** is observed on a real person rather than built: it maps to
no task by design, and it says so here rather than being left silently unmapped. Every other
criterion below maps either to a task in `tasks.md` or to a phase-exit criterion in `plan.md`, so
an unmapped one that carries no marker is a gap.

- **SC-001** *[outcome metric]*: An account holder who has used the application before can go from opening the report builder to holding a downloaded document in under 3 minutes for a selection of 200 records, without documentation or assistance.
- **SC-002**: The per-kind counts shown before a report is asked for match the contents of the produced document exactly, in 100% of cases, and 0 records belonging to any other person appear in any produced document.
- **SC-003**: 100% of report and export requests are acknowledged in under 2 seconds without holding the requester on a page, and 100% of them report progress and remain available to that account when it returns.
- **SC-004**: 100% of produced documents open and print on a device with nothing of the application installed, carry the person's identity and a page number on every page, and state their criteria and their production moment on the first page.
- **SC-005**: A complete export of an account holding 10,000 records and 2,000 documents completes within 5 minutes and within 256 MiB of memory, and 100% of its records and documents are present and readable with the application stopped.
- **SC-006**: 100% of export archives contain a manifest that describes every other file in the archive, and 0 archives contain a credential, a secret, an operator setting or anything concerning a person the account cannot reach — verified by searching the archive bytes, not by inspection of the code.
- **SC-007**: A saved report reopened after new matching records were added resolves to the new total in 100% of cases, with 0 saved reports requiring an edit to stay current and 0 reporting an error because a record it previously matched was deleted.
- **SC-008**: 0 charts plot readings recorded in more than one unit on one axis, 0 unit conversions are performed anywhere in the application, and 100% of charts in a produced document are accompanied by a table of the exact values they plot.
- **SC-009**: 100% of attempts to reach a saved report, a produced document, an export archive, an instance archive or an activity entry belonging to somebody else — by guessing a reference, by using a copied address, or while not signed in — are answered indistinguishably from the thing not existing, and 100% of those attempts are recorded.
- **SC-010**: 100% of operator views and operator actions are refused to an account without the administrative tier, with a response indistinguishable from the view not existing, and 100% of those attempts are recorded.
- **SC-011**: Every figure on the operator overview carries a definition and the moment it was computed, 100% of them are counts, sizes, durations, versions, states, moments or setting values, and 0 of them are a name, a date of birth, a recorded value, a note, a tag name or a file name.
- **SC-012**: An instance with the break-glass credential missing a second factor or an address restriction produces an unmistakable warning at 100% of start-ups and on 100% of operator overview views, naming the missing protection, until it is configured.
- **SC-013**: An administrator disabling an account ends that account's existing sessions within 5 seconds and refuses its next sign-in 100% of the time; and 100% of attempts to demote or disable oneself, or to demote or disable the last enabled administrator, are refused.
- **SC-014**: 0 activity entries contain a name, a date of birth, a diagnosis, a recorded value, a note, a tag name or a file name; and reading the trail produces 0 entries while exporting it produces exactly 1.
- **SC-015**: An owner reading their own person's records produces 0 activity entries; a reader who is not the owner reading the same records through an arrangement or through the break-glass credential produces exactly 1 per record opened or document downloaded.
- **SC-016**: The first page of the activity trail over 1,000,000 entries appears within 2 seconds, paging 50 consecutive pages while entries are being written repeats 0 entries and skips 0, and a tabular export of 1,000,000 entries completes within 128 MiB of memory.
- **SC-017**: 100% of restores take a safety copy first and report its reference before anything is replaced, 100% of restores whose safety copy fails do not proceed, and 100% of restores leave the instance either fully restored or unchanged with 0 partial outcomes.
- **SC-018**: 100% of restores are refused without both the typed confirmation phrase and password re-entry, 100% of archive downloads require password re-entry and are authorized per request, and 100% of archive operations appear in the activity trail — including after a restore has replaced the records the trail is kept in.
- **SC-019**: 100% of things past their retention window are removed by scheduled work without an operator asking, 0 of them are recoverable through the application afterwards, and every window in force is stated with its current value, its last run and its last success on the operator overview.
- **SC-020**: 100% of scheduled clean-up failures appear on the operator overview and in the activity trail exactly once and are retried on the next scheduled run, and 0 of them leave anything half-removed.
- **SC-021**: 100% of the user-facing pages delivered by phases 001 to 006 pass the automated browser check at both the desktop and mobile sizes, with zero browser console errors, zero uncaught page failures and zero failed resource requests, against both a populated account and an account holding nothing; and adding a page without adding it to that check fails the build 100% of the time.
- **SC-022**: Across an automated exercise of every operation in the application, every scheduled job and every production worker, 0 names, dates of birth, notes, recorded values, tag names or file names appear in the instance's logs, measurements, traces or error reports; 0 measurement labels are opaque identifiers; and 0 outbound network connections are made when none is configured.
- **SC-023**: On an instance loaded to the documented volumes — 10,000 records, 2,000 documents, 1,000,000 activity entries and 500 readings of one measured value — the report builder appears within 2 seconds, a selection's resolved count updates within 500 milliseconds, the operator overview appears within 1 second, and a 5,000-record report is produced within 2 minutes; a regression beyond any published budget fails the build.
- **SC-024**: A view that updates live is still receiving updates after 10 continuous minutes, proven by an automated test that holds one open longer than that.
- **SC-025**: 100% of the acceptance scenarios in this specification exist as automated tests and pass. This phase — and the product — is not complete while any one of them is missing or failing, and every operation this phase adds additionally carries automated tests for the stranger, the entitled account, the account whose access ended and the account without the administrative tier.
- **SC-026**: Somebody who has never run the application goes from a clean machine to a running instance, takes a backup and restores it using only the operator handbook, with 0 questions asked of anybody, and can state from the handbook alone what every setting, every retention window and every operator figure means.
- **SC-027**: A privacy review of the finished application is written down against the stated privacy rules, with 100% of its findings either fixed with the change named or recorded with a reason, and re-running the same procedure after a change produces a comparable written result.
- **SC-028**: On an instance with no external identity provider configured, 0 routes, buttons or links offering external sign-in are reachable anywhere in the interface, and 0 provider sign-ins can grant an administrative tier or revive a disabled account — both verified by automated tests (FR-134, FR-135).

## Assumptions

### What earlier phases already delivered, and is not re-specified here

- **Phase 001 (Walking Skeleton)** provides the running application, its configuration mechanism and its start-up validation, its diagnostics, the application shell and its navigation landmarks, the automated browser check and the page inventory that check is derived from, and the published description of the operations the application serves. This phase adds pages to that check and operations to that description; it does not restate how either works.
- **Phase 001** provides the activity trail itself: the entries, what they may contain, and the writing of one for every action that mutates or exports a person's data. This phase builds the reader, states what the reader may show, and adds the entries for the actions it introduces. It does not redesign the trail.
- **Phase 001** provides accounts, sign-in, sessions, password handling — including self-service password recovery by email and confirmation of the address on an account — account deletion, and the boot-time warning about break-glass protections. **External sign-in is the one part of authentication phase 001 deliberately left to this phase**, because it is a deployment integration that needs provider configuration, and this phase already owns the operator surface where the instance's own settings are configured. This phase surfaces that warning on the operator overview as well and adds the administrative tier's effect on those accounts.
- **Phase 001** provides live updating of open views and the rule that a view left open for an hour is still receiving updates. This phase owns the automated proof that a view survives longer than ten continuous minutes, because the failure it guards against is silent and passes every shorter test.
- **Phase 001** provides the rule that a save based on a version that has since changed is refused with the current values shown. Saved reports use that rule unchanged.
- **Phase 002 (Patient Core)** provides people, ownership, the person currently in view, and the rule that the person in view is never a basis for permission. Every report, chart and export in this phase is authorized against what the requester can actually reach, never against what is on screen.
- **Phase 003 (Clinical Records)** provides the record kinds, their fields, their tags and their lifecycle values. This phase selects over them and prints them; it adds no record kind and changes no field.
- **Phase 004 (Labs and Attachments)** provides laboratory results with their components, units and reference ranges — which is what makes charting possible — and attached documents together with their recoverable deletion, their thirty-day retention window, restoring, purging early, the scheduled purge and the orphan sweep. **This phase re-specifies none of that.** Deleted documents are recovered where phase 004 put them: as a filter on the document library of the person they belong to. This phase adds only the instance-wide operator figures — how many deleted documents are awaiting removal and how many bytes they occupy — and the visibility of the scheduled work's health.
- **Phase 004** also settled who may remove a document permanently before its window closes: its owner behind a typed confirmation, the holder of the break-glass credential for any, and never a recipient of somebody else's arrangement. That rule stands as written there.
- **Phase 005 (Sharing and Collaboration)** provides arrangements by which one account reaches another's person, their levels, their ending and the rule that access is re-checked on every request. What a report or an export contains is decided by that rule at the moment production begins.
- **Phase 005** also writes the entries recording that somebody who is not the owner opened a record or downloaded a document. This phase is where those entries are finally readable.
- **Backup and restore, file storage, authentication and the break-glass administration surface** are the instance's own facilities. This phase wraps them with a preview, a mandatory safety copy, a confirmation, authorization and the visibility of failures. It does not reimplement any of them.

### Decisions taken here rather than deferred

- **The attachment trash belongs to phase 004, and this phase adds no second surface for it.** An earlier draft of this specification restated soft delete, the retention window, restoring, purging early and the scheduled purge, and put them behind a dedicated recovery page. That contradicted phase 004 on both the authorization rule and the surface, and it would have given the product two places to restore the same document. This phase owns one thing about deleted documents: the instance-wide count and byte total on the operator overview, and a pointer to where recovery already happens.
- **Record-level soft delete does not exist and will not.** Soft delete applies to files only. Records are removed outright when they are deleted, which is why every destructive record action is confirmed in the interface and recorded in the trail. Nothing in this phase introduces a recovery window for a record, and the operator handbook says so plainly rather than leaving a reader to wonder.
- **Reading your own records is not recorded; somebody else reading them is.** An entry is written when the reader is not the owner — through an arrangement the owner made, or through the break-glass credential. Recording every self-read would drown the trail in noise and would build a timeline of exactly when a person read their own most sensitive results, which is itself a disclosure. This is the same asymmetry that makes reading the trail unaudited while exporting it is audited.
- **An activity entry never carries content, and the reader has nowhere to put it.** The reader does not fetch a name to display beside an entry, even for a thing that still exists — because an entry that shows a name proves the thing still exists, which is itself a disclosure, and because "remember to redact" is not a control.
- **A saved report stores the question, not the answer.** The predecessor stored a fixed list of records, so a saved report silently rotted. Criteria resolve against current data every time, which is why a saved report picks up what was recorded since without anybody editing it.
- **Units are never converted, anywhere, for any reason.** A conversion table is a place for a clinical error to live. A measured value recorded in two units is two chartable series, a unit must be chosen, and the exclusion of the others is stated on the document.
- **Values below the charting minimum are shown, not hidden.** A person needs to know that two more readings would make their blood pressure chartable.
- **One report or export is produced at a time on an instance, and the rest queue.** The instance is single-instance by construction and runs on modest hardware; producing several large documents at once is how an operator's cupboard machine becomes unresponsive. A waiting request shows its position so that waiting never looks like a hang.
- **Authorization is re-resolved when production starts, not when it was requested.** A person whose access was withdrawn between the request and the run is absent from the result, and the export's manifest says so rather than leaving a silent gap.
- **A produced document and an export archive are reached only through an authorized request.** Their addresses are not credentials, and no administrator can download another account's document or archive. An administrator sees counts, never contents.
- **Reports and exports share one retention window.** Both need "a documented window" and neither needs a different number from the other. One setting, one sentence in the handbook.
- **Every limit and window is a setting with a published default, shown in the application.** The words "documented" and "published" are only true if an operator can read the value without reading the source, and change it without rebuilding anything.
- **Backup and restore wrap what the instance already ships with.** Writing a second backup mechanism would mean reimplementing the safe copying of records that are being written to while the copy is taken, and the existing one already handles schedules and how many copies are kept. What it does not do is tell anybody when it fails, refuse to overwrite without a preview, or take a safety copy first — and those four things are exactly what this phase adds.
- **A restore always takes a safety copy first, and a restore whose safety copy fails does not happen.** This is the only thing standing between a tired administrator at eleven at night and permanent loss, and it is not optional or skippable for a recent archive.
- **The account of a restore survives the restore.** A restore replaces the records the trail is kept in, so without deliberate care the only record of the most destructive action the instance offers would be destroyed by that action. The restore, its safety copy and both references are readable afterwards.
- **Progress on long-running work is shown by the page asking again, not by a live stream.** A progress bar is not worth a long-lived connection, a new operation and the failure modes that come with one.
- **Job progress and instance figures are split by what they cost.** Cheap counts are computed when the page is opened, so a figure an operator just changed moves. Anything expensive to compute — a walk of the document store, for example — is refreshed on a schedule and shown with the moment it was computed, because an operator dashboard must never be the thing that takes the instance down.
- **External sign-in is configured, not built.** The instance's own administrative surface already configures providers and already performs the whole authorize-and-callback exchange, including linking a provider identity to an existing account. This phase adds one operation and one posture figure over it; it invents no provider registry, no second configuration mechanism and no linking screens of its own. That is the same rule this phase applies to backup and restore.
- **The administrative tier is not the break-glass credential.** An administrator sees counts, health, accounts, archives and the trail. Reading a person's records requires the break-glass credential, which bypasses the application's permission checks by design and whose every session is recorded. Nothing on any operator view is a route around that.
- **An instance refuses to start on a configuration it cannot honour.** Starting with a nonsensical retention window and quietly ignoring it is how a promise about deletion becomes untrue without anybody noticing.
- **The application states what it cannot render rather than hiding it.** Some scripts cannot be rendered faithfully in a produced document. The export always carries the exact text, the produced document counts and states the affected entries on its first page, and the handbook says so — because a document handed to a clinician must never quietly change what a person wrote.
- **The English-only interface** of the earlier phases continues, with the language preference still honoured for date and number presentation.

### What this phase deliberately does not do

- **Scheduled delivery of reports by email, or delivery to anywhere else.** A report is produced when somebody asks for it and is downloaded by them. Sending one on a schedule, to an address, to a clinician or to another system is not part of this phase and is not planned for one.
- **Notification channels.** Chat services, push services, webhooks and digests are not added here. Nothing in this phase depends on a message arriving anywhere.
- **Aggregate behavioural analytics.** The operator overview counts what the instance holds and how it is running. It does not count what people do with it, does not build usage profiles and does not report anything anywhere.
- **Record-level soft delete.** Files only. See the decision above.
- **A second surface for recovering deleted documents.** See the decision above.
- **Reading an export archive back into an instance.** The archive exists so that a person is never trapped; the supported route back into a MediGo instance is a restore from an instance archive, and the documentation says so plainly rather than leaving somebody to discover it.
- **Sharing a saved report with another account.** Saved reports are private to the account that owns them. If they ever need sharing, they become another kind of shareable thing under phase 005's one sharing model, not a pair of flags.
- **Report designs beyond the documented structure.** The section order, the criteria statement and the identifying header are fixed and documented so that two reports of the same selection are comparable. A layout designer, custom templates, letterheads and per-clinician styling are not part of this phase.
- **Running the instance as more than one process.** The instance is single-instance by construction. High availability means restarting quickly from an archive, which is what this phase makes survivable.

### What remains open after this phase, stated so nobody discovers it later

- **This is the last planned phase.** Anything named above as out of scope requires a new phase, not a patch, and the constitution's amendment process where a decision here would have to be revisited.
- **Some scripts cannot be rendered faithfully in a produced document, and one class of them is not contextually shaped at all.** The limitation is stated on the document's first page, the affected entries are counted, the export always carries the exact text, and the operator handbook records it as a known limitation rather than a defect to be discovered by a user.
- **The instance's own upgrade fragility.** Several of the mechanisms this product relies on sit on the behaviour of the embedded platform. The checklist that re-verifies them on every upgrade is maintained from phase 001 onwards, and this phase adds its own touch points to it.
