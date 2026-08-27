# Feature Specification: Labs and Attachments

**Feature Branch**: `004-labs-and-attachments`

**Created**: 2026-08-26

**Status**: Draft

**Input**: User description: "Phase 004 of MediKube. Lab results with individual test components that can be trended over time, and documents attached to any record. In scope: lab results with ordering practitioner, dates, status and interpretation; individual test components within a result — name, value, unit, reference range, and whether the value is abnormal; a catalogue of standardized tests so components can be entered consistently and compared; trending a component across results over time, and highlighting out-of-range values; linking lab results to the conditions, encounters, medications, procedures and treatments they relate to; file attachments on any record type — upload, download, view inline, replace, delete, with size and type limits and storage accounting."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Record a blood test and see at a glance what is off (Priority: P1)

Amara's father has quarterly blood work. She gets back a printed panel with two dozen lines on it: haemoglobin, white cells, creatinine, each with a number, a unit, and the laboratory's own normal range printed beside it. She wants that panel in his record, line by line, so that the next time a doctor asks "what was his creatinine in March", she has the number, the unit, the range it was measured against, and whether it was outside it — instead of a photograph of a page she has to squint at.

She records the panel: what test it was, who ordered it, which laboratory, when it was collected and when it was resulted, where it is in its life (ordered, in progress, completed), and the laboratory's overall comment. Then she enters each line as its own component with its name, value, unit and reference range. Every line that falls outside its range is marked, plainly and not by colour alone, both in the panel and in the list of results.

**Why this priority**: This is the structural heart of the phase and the reason it exists separately from the other clinical record types. A lab result that cannot hold its individual lines is just a note with a date on it; the whole of the rest of this phase — trending, catalogue-assisted entry, out-of-range highlighting — is built on the components being real, separate, comparable values. If only this story ships, a carer can already keep a structured, searchable laboratory history that answers the question a clinician actually asks.

**Independent Test**: Sign in as the seeded account, open the lab results for the seeded person, record a panel with a name, an ordering practitioner, a laboratory, collection and result dates, an interpretation, and ten components with values, units and ranges. Confirm the panel appears in the list, that its detail shows every component in the order entered, that the three components deliberately given out-of-range values are marked and counted, and that the seven in range are not. Fully testable with nothing else in this phase implemented.

**Acceptance Scenarios**:

1. **Given** a person with no lab results recorded, **When** their lab result list is opened, **Then** an explanation that nothing is recorded yet is shown together with the action to record the first result, inside the same page structure as a populated list.
2. **Given** a signed-in account holder, **When** they record a lab result supplying only a test name and the person it concerns, **Then** it is saved, appears at the top of the list, and every value they left blank is absent from the detail view rather than shown as a blank or a zero.
3. **Given** a lab result being recorded, **When** the account holder supplies a test name, a test code, a category, a status, an ordering practitioner, a place of care, an ordered date, a collection date, a result date, an overall interpretation and notes, **Then** all of those are stored and shown back exactly as entered.
4. **Given** a lab result being recorded, **When** the collection date is before the ordered date and the status is a value outside the published set, **Then** the save is refused, both problems are reported at once beside the fields they belong to, and everything else entered is still on the form.
5. **Given** a lab result being recorded, **When** ten components are entered with names, values, units and reference ranges, **Then** the result is saved with exactly those ten components, in the order they were entered.
6. **Given** a saved panel of ten components, **When** three of them hold values outside their recorded reference ranges, **Then** those three are marked as out of range wherever their value is shown, the marking is conveyed by something other than colour alone, the other seven are not marked, and the panel reports that three of its components are out of range.
7. **Given** a component whose value is recorded without any reference range, **When** the panel is displayed, **Then** that component is shown as not assessed against a range, and is never presented as normal.
8. **Given** a saved panel, **When** the account holder edits it and submits a component set with one component removed, one changed and one added, **Then** the stored set matches exactly what was submitted: the removed one is gone, the changed one is updated, and the added one is present.
9. **Given** a lab result that carries its own single overall value, **When** the account holder converts it into a panel by adding components, **Then** the conversion is allowed and the result no longer carries its own overall value; and the reverse conversion is allowed too.
10. **Given** a lab result that carries both its own overall value and a set of components in one submission, **When** it is saved, **Then** the save is refused with an explanation that a result holds either one overall value or a set of components, and neither is silently discarded.
11. **Given** the same lab result open for editing in two places, **When** it is saved in the second place after having been saved in the first, **Then** the second save is refused, the account holder is told the record changed underneath them, and the current values are shown.
12. **Given** a lab result the account holder intends to remove, **When** they choose to delete it, **Then** they must confirm an action that names the result and warns that it and its components cannot be recovered, and only then is it removed together with every one of its components.
13. **Given** a lab result belonging to a person owned by a different account, **When** it is requested by any means, **Then** the answer is indistinguishable from the result not existing, nothing about it is revealed, and the attempt is recorded.

---

### User Story 2 - Keep the paperwork with the record it belongs to (Priority: P2)

Tomas keeps a shoebox: discharge letters, the scan of an insurance card, a photograph of a rash he wants to show a dermatologist, and the laboratory's own printed report for every blood test. He wants each of those living on the record it belongs to, not in a folder named "medical" on a laptop. He attaches the laboratory's report to the lab result he just recorded, a photograph to a recorded symptom, and a discharge letter to an encounter. Later he opens the report in the browser without downloading it, downloads the original when the specialist asks for it, replaces one he attached to the wrong record with a corrected scan, and deletes one he attached twice — knowing that if he deletes the wrong one he has a month to get it back.

**Why this priority**: Attachments are the second pillar of this phase and the one that touches every record type in the application, not just laboratory work. They are second rather than first because a document is worth most when it hangs off a structured record, and because the phase's headline value — a comparable laboratory history — comes from story 1. On its own this story already delivers a private, per-record document store with recoverable deletion, which is a whole product for somebody who mostly keeps paperwork.

**Independent Test**: Sign in as the seeded account, attach a document to a record of each kind available, confirm it appears on that record and in the person's document library, open a supported one inline, download one and compare the downloaded bytes with the uploaded bytes, replace one, change a description, delete one and restore it, delete another and confirm it is not listed with its record. Fully testable with story 1 not implemented, using any record kind delivered by an earlier phase.

**Acceptance Scenarios**:

1. **Given** a person with no documents attached to anything, **When** their document library is opened, **Then** an explanation that nothing has been attached yet is shown together with guidance on where to attach the first one, inside the same page structure as a populated library.
2. **Given** a clinical record of any kind, **When** the account holder attaches a document with a description and a category, **Then** it is stored, appears on that record and in the person's document library, and shows its original name, its size, its type, who attached it and when.
3. **Given** a document being attached, **When** the file is larger than the instance's configured limit, **Then** the attachment is refused before anything is stored, the message states the limit, and no partial document exists afterwards.
4. **Given** a document being attached, **When** the file's actual content is not one of the accepted types, **Then** the attachment is refused naming the accepted types — and this holds even when the file's name or the type claimed by the browser says otherwise.
5. **Given** a document being attached, **When** the file has no content at all, **Then** the attachment is refused with an explanation.
6. **Given** an attached document of a type the system can display, **When** the account holder chooses to view it, **Then** it is shown in the browser without being downloaded; **and given** a type the system cannot display, **Then** only downloading is offered.
7. **Given** an attached document, **When** the account holder downloads it, **Then** the retrieved content is byte-for-byte identical to what was uploaded and carries the original name.
8. **Given** an attached document, **When** the account holder replaces it with a corrected version, **Then** the new version takes its place on the same record with the same description and category, and the replaced version is recoverable for the retention window.
9. **Given** an attached document, **When** the account holder changes its description and category, **Then** those change and the original name, size and type do not.
10. **Given** an attached document, **When** the account holder deletes it, **Then** they must confirm, it stops being listed with its record and in the library, and it remains recoverable for the documented retention window.
11. **Given** a document deleted 5 days ago with a retention window of 30 days, **When** the account holder restores it, **Then** it returns to the record it was attached to and is listed again.
12. **Given** a document deleted longer ago than the retention window, **When** it is looked for by any means, **Then** it no longer exists, cannot be restored by anyone through the application, and its content is no longer stored.
13. **Given** a deleted document whose record has since been deleted as well, **When** the account holder tries to restore it, **Then** the restore is refused with an explanation that the record it belonged to no longer exists, and they are offered the chance to download a copy before it is purged.
14. **Given** a clinical record with three documents attached, **When** the record is deleted, **Then** the record is gone permanently and its three documents are in the trash, recoverable for the retention window.
15. **Given** a document attached to a person owned by another account, **When** it is requested — by opening its address directly, by guessing its identifier, or without being signed in at all — **Then** the answer is indistinguishable from it not existing, no name, size, type or preview is revealed, and the attempt is recorded.
16. **Given** any retrieval of a document's content, **When** it succeeds, **Then** an activity entry records who retrieved it, which document, which person it concerned and when — and contains neither the file name, the description, nor any of the content.

---

### User Story 3 - Watch one value move over time (Priority: P3)

Priya was told her thyroid level was "a bit high last year and better now". She has four years of results in MediKube, entered one panel at a time. She wants to pick one value — thyroid stimulating hormone — and see every reading of it, in order, with the range it was measured against, so she can see for herself whether "better" is a trend or one good day. She picks it from a list of every value ever recorded for her, sees its readings plotted in date order with the out-of-range ones marked, and reads the count, the earliest and latest reading, the lowest, the highest, the average, and whether it is rising, falling or steady.

**Why this priority**: A component is only clinically meaningful in comparison with itself. This is the value the phase's structure exists to unlock and it is the thing a spreadsheet does badly. It is third because it requires several results already recorded — it is worthless on an empty account, so it cannot be the phase's first shippable slice, and it depends on nothing that stories 1 and 2 do not already produce.

**Independent Test**: Seed a person with eight lab results spanning two years, each carrying the same three component names, two of them numeric and one categorical, with two of the numeric readings recorded in a different unit from the rest. Open the trend view, confirm every distinct component name is listed with its latest value, unit, status and reading count; select one, confirm the readings appear in date order with out-of-range readings marked, confirm the summary figures, confirm the differing-unit readings are not mixed into the same series and that the view says which unit is shown; select the categorical one and confirm a value history rather than a chart.

**Acceptance Scenarios**:

1. **Given** a person with several lab results, **When** the trend view is opened, **Then** every distinct component name ever recorded for that person is listed once, each with its latest value, its unit, its latest status, how many readings exist and the date of the latest one.
2. **Given** a person with no lab results at all, **When** the trend view is opened, **Then** an explanation that there is nothing to compare yet is shown, with a route to recording a first result, inside the same page structure as a populated view.
3. **Given** a component with twelve numeric readings across twelve results, **When** it is selected, **Then** its readings are shown in date order with their values, units, the reference range each was measured against, and out-of-range readings marked.
4. **Given** a component whose readings were recorded in two different units, **When** it is selected, **Then** the readings are never combined into one series, the account holder is told that readings exist in more than one unit and can choose which to see, and the unit being shown is stated on the view.
5. **Given** readings in different units, **When** any of them are displayed, **Then** no value is converted from one unit into another anywhere in the system.
6. **Given** a component with a single reading, **When** it is selected, **Then** the account holder is told there are not enough readings to compare, and the one reading is still shown.
7. **Given** a numeric component with at least three readings, **When** its summary is shown, **Then** it reports the number of readings, the earliest and latest reading dates, the latest value, the lowest, the highest, the average, how many readings fell within their range, and a direction of rising, falling or steady derived by the published rule.
8. **Given** a numeric component with fewer than three readings, **When** its summary is shown, **Then** no direction is claimed and the view says there are not enough readings to say.
9. **Given** a component whose values are categorical or free text rather than numeric, **When** it is selected, **Then** a history of the recorded values in date order is shown together with how often each distinct value occurred, and no chart or average is offered.
10. **Given** readings whose reference ranges changed between results, **When** the series is shown, **Then** each reading is judged against the range recorded with it, and the view states which range it is drawing as a band.
11. **Given** a trend restricted to a date range, **When** the range is applied, **Then** only readings within it are included, in the summary figures as well as in the series.
12. **Given** a person owned by another account, **When** a trend for them is requested, **Then** the answer is indistinguishable from that person not existing.

---

### User Story 4 - Enter the same test the same way every time (Priority: P4)

Amara types "haemoglobin" one visit and "Hgb" the next, and gets two entries in her trend list that should have been one. She wants the application to recognise a standard test as she types it, offer the standard name, fill in the usual unit and the usual reference range, and record that both entries are the same test — while still letting her enter something the catalogue has never heard of.

**Why this priority**: This is what makes story 3 correct rather than merely present, and it removes most of the typing from story 1. It is fourth because trending works without it — readings group by their own normalised name — and because it is the only story here whose value is measured in accuracy rather than in a capability that did not previously exist.

**Independent Test**: Seed the instance with the standard test catalogue. While entering a component, type three characters of a common test's name and confirm suggestions appear; pick one and confirm the name, unit, category and typical reference range are filled in and remain editable; save, then enter a second component for the same test using a different spelling and again pick the catalogue entry; confirm the trend view lists that test once with two readings. Then enter a component the catalogue does not contain and confirm it saves and trends on its own.

**Acceptance Scenarios**:

1. **Given** an account holder entering a component, **When** they type at least three characters of a test name, **Then** matching catalogue entries are offered, matching on the standard name, on alternative names and on the standard code.
2. **Given** a catalogue lookup that matches nothing, **When** the results are shown, **Then** the account holder is told nothing matched and is still able to enter their own name.
3. **Given** an offered catalogue entry, **When** the account holder picks it, **Then** the component's name, unit, category and typical reference range are filled in, and every one of them can still be changed before saving.
4. **Given** two components recorded under different spellings but matched to the same catalogue entry, **When** the trend view is opened, **Then** they appear as one entry with two readings.
5. **Given** two components recorded under the same name in different letter case and with surrounding spaces, and matched to no catalogue entry, **When** the trend view is opened, **Then** they still appear as one entry with two readings.
6. **Given** a component whose test the catalogue does not contain, **When** it is saved, **Then** it saves without complaint and is trendable in its own right.
7. **Given** any signed-in account holder, **When** they attempt to add to, change or delete a catalogue entry, **Then** the attempt is refused: the catalogue is reference data that ships with the instance.
8. **Given** the catalogue, **When** it is read, **Then** it contains nothing about any person and reading it discloses nothing about anyone.

---

### User Story 5 - Say what a result was about (Priority: P5)

Tomas's kidney function panel exists because of a diagnosed condition, was ordered at a particular consultation, and is being tracked because of the medication he is on. He wants those connections recorded, so that opening the condition shows the results that bear on it, and opening the result shows why it was taken.

**Why this priority**: It turns a pile of results into a chart. It is last because every other story stands entirely without it, because it depends on the other clinical record types existing, and because a wrong link is a smaller loss than a wrong value.

**Independent Test**: Seed a person with a condition, an encounter, a medication, a procedure and a treatment. Record a lab result, link it to one of each, confirm all five appear on the result, confirm the result appears on each of the five, remove one link and confirm both records survive intact, then delete one of the linked records and confirm the lab result is untouched apart from losing that connection.

**Acceptance Scenarios**:

1. **Given** a lab result and a condition belonging to the same person, **When** the account holder links them, **Then** the condition is listed on the lab result and the lab result is listed on the condition.
2. **Given** a lab result, **When** the account holder links it to several conditions, encounters, medications, procedures and treatments at once, **Then** all of those links are recorded and shown.
3. **Given** a lab result, **When** the account holder tries to link it to a record belonging to a different person, **Then** the link is refused and the refusal discloses nothing about that record's existence.
4. **Given** a linked pair, **When** the link is removed, **Then** both records remain unchanged apart from no longer referring to each other.
5. **Given** a condition linked to two lab results, **When** the condition is deleted, **Then** both lab results survive intact and simply no longer refer to it.
6. **Given** a lab result linked to five records, **When** the lab result is deleted, **Then** all five survive intact and no longer refer to it.
7. **Given** any linking or unlinking, **When** it is saved, **Then** it is recorded in the activity trail as a change to the lab result.

---

### Edge Cases

**Nothing there yet**

- A person with no lab results, a person with no attached documents, and a person with results but nothing yet worth comparing: each view has its own explanation and its own next action, all rendered inside the standard page structure so the automated browser check does not go falsely red on a legitimately empty page.
- A lab result saved with no components and no overall value: valid — an ordered test that has not come back yet — and shown as awaiting a result rather than as an error.
- A search of the standard test catalogue that matches nothing shows "nothing matched that", which is a different message from "the catalogue has not been loaded".
- The last document on a record is deleted: the record returns to its no-documents state without an intermediate broken render.

**Two things happening at once**

- The same lab result is edited in two places and both submit a component set: the later save is refused, told the record changed underneath it, and shown the current components. A component set is never merged.
- A document is attached to a record that is deleted in another place while the upload is in flight: the upload fails cleanly, nothing is stored, and no document exists that points at a record that does not.
- A document is deleted in one place while another has it open: the second is told it has been deleted and offered the restore action if the window has not closed.
- A restore and the scheduled purge race for the same document at the end of the retention window: exactly one wins, and if it is the purge, the account holder is told the document is gone rather than shown a broken restore.
- Two uploads of the same file at the same time produce two separate documents; neither is silently discarded as a duplicate.
- A lab result is deleted while its trend is open in another place: the trend stops including its readings on the next update, and the count and summary figures follow.

**Partial and awkward data**

- A component with a value but no unit, or with a reference range expressed as text ("negative", "not detected") instead of two numbers: both accepted, and a range expressed as text means the reading is not automatically judged in or out of range.
- A result reported as being below a detection limit ("<0.01") is recorded as a textual value, not silently converted into a number.
- The same component name appears twice in one panel — a fasting and a random glucose, say: accepted, ordered by the order given, and both shown at that date in the trend.
- A component's unit changes between one laboratory and the next: no conversion, two series, and the trend view says so rather than drawing one misleading line.
- A reference range whose lower bound is above its upper bound is refused; a range with only a lower bound or only an upper bound is accepted and judged on the bound it has.
- A lab result with no dates at all still sorts deterministically and does not disappear from the list.
- Free text at exactly its documented limit is accepted; one character over is refused with a message naming the field and the limit.
- A document whose name contains non-Latin script, right-to-left text or characters that look like markup: stored, listed and downloaded correctly under its original name, and never interpreted as anything other than text.
- A document with an extremely long name is displayed truncated but downloads under its full original name.
- A document whose name says one thing and whose content says another: the content decides what type it is and whether it is accepted.
- A date-only value — collected on, resulted on — shows as the same calendar date regardless of the viewer's time zone or device clock.

**Permission boundaries**

- Guessing or enumerating another person's lab result, component or document identifiers yields the same answer as identifiers that never existed.
- Requesting a document's content, or its preview image, without being signed in yields nothing, whether the address was guessed or copied from somewhere it should not have been.
- A document's address is not a credential: possessing it grants nothing, and sharing it with somebody who cannot already reach that person gives them nothing.
- An administrative session can reach everything by design; every such session leaves an activity entry.
- Everything in this phase is reachable in this phase only by the account that owns the person concerned. Phase 005 widens that to people the owner has shared with, and this phase's rules are written so that widening changes who passes the check and nothing else.

**Deletion and what depends on it**

- Deleting a lab result destroys its components permanently and moves its attached documents to the trash; the confirmation says both before the account holder commits.
- Deleting a clinical record of any kind moves its attached documents to the trash rather than destroying them, so a mistaken deletion does not take the paperwork with it irrecoverably.
- Deleting a record that a lab result refers to removes only the reference; neither record is destroyed by the other's deletion.
- Deleting a person destroys everything recorded for them, including their documents and anything of theirs sitting in the trash, and the confirmation says so.
- Deleting the account destroys the people it owns and everything under them by the same rule.
- Purging is permanent: once the retention window closes, nothing in the application can bring a document back.

**Scale and duration**

- Five thousand lab results for one person: the list, its narrowing, its sorting and its paging stay usable, and no page takes noticeably longer than the first.
- A single panel with a hundred components: it displays as one page without truncating silently.
- A component with five hundred readings: the trend stays responsive; where the series must be capped, the account holder is told it was capped and how to narrow the range.
- Two thousand documents for one person: the library pages, narrows and sorts without degrading.
- A document at the size limit uploaded over a slow connection: progress is visible and the upload is not abandoned by a timeout that the account holder cannot see.
- Paging the library or the result list while documents are being attached never shows the same item twice and never skips one.

**Environment failures**

- Storage is full or a write fails part-way through an upload: the attachment is refused with a message that reveals nothing about where files are kept, and no half-stored document and no document without content exists afterwards.
- A document's content cannot be read back although its entry exists: the account holder is told the content is unavailable, the failure is recorded once for the operator, and the message discloses nothing about storage locations.
- The scheduled purge fails: it is recorded and retried, and documents remain wholly in the trash rather than half-deleted.
- The standard test catalogue has not been loaded on an instance: catalogue-assisted entry says so plainly and manual entry continues to work.

## Requirements *(mandatory)*

### Functional Requirements

#### Lab results

- **FR-001**: The system MUST allow an account holder to record a lab result attributed to exactly one person, and MUST refuse to store one that is not attributed to a person the account holder may reach.
- **FR-002**: A lab result MUST be able to carry a test name, a test code, a category from the published set, a lifecycle status, an ordered date, a collection date, a result date, an ordering practitioner, a place of care, an overall interpretation, and free notes. Only the test name and the person are required.
- **FR-003**: The lifecycle status MUST be drawn from the vocabulary already established for other record kinds — ordered, scheduled, in progress, completed, cancelled — and a value outside it MUST be refused rather than stored as free text. A new result defaults to ordered.
- **FR-004**: The ordering practitioner and the place of care MUST be chosen from the directories that already exist for the account holder, and MUST NOT be free text.
- **FR-005**: A lab result MUST hold either one overall value with its own unit and reference range, or a set of components — never both. A submission carrying both MUST be refused with an explanation, and neither part silently discarded.
- **FR-006**: The system MUST allow a lab result to be converted between the two forms after it was created, in either direction, without recreating it.
- **FR-007**: The system MUST refuse a lab result whose collection date precedes its ordered date, or whose result date precedes its collection date, reporting every offending field in the same submission.
- **FR-008**: The system MUST order a person's lab results by their most recent meaningful date — result date, else collection date, else ordered date, else when the entry was created — so that a result with no dates still has a defined position.
- **FR-009**: The system MUST allow a person's lab results to be narrowed by status, by category, by a date range, and by words occurring in the test name, and MUST page results without ever repeating or skipping an entry while entries are being added or removed.
- **FR-010**: Lab results MUST behave like every other clinical record kind already delivered: the same create, view, edit and delete flow, the same confirmation naming the record before a permanent deletion, the same refusal when the record changed underneath the editor, and the same live updating of an open list without a manual refresh.
- **FR-011**: The system MUST record every creation, change and deletion of a lab result in the activity trail, by identifier and never by content.

#### Test components

- **FR-012**: A component MUST be able to carry a test name, an abbreviation, a value kind, a value, a unit, a reference range expressed either as a lower and upper bound or as text, an out-of-range status, and a position within its result. Only the test name and the value kind are required.
- **FR-013**: The value kind MUST be one of numeric, categorical or textual, defaulting to numeric. A numeric component MUST carry a number and no text value; a categorical or textual component MUST carry text and no number. Any other combination MUST be refused.
- **FR-014**: Components MUST be managed as the complete set belonging to their lab result: saving a result stores exactly the components submitted, creating those that are new, updating those that changed, and deleting those that were omitted.
- **FR-015**: Deleting a lab result MUST delete all of its components permanently, and those readings MUST disappear from every trend.
- **FR-016**: The system MUST accept two components with the same test name within one result, preserving the order given.
- **FR-017**: The system MUST refuse a reference range whose lower bound is greater than its upper bound, and MUST accept a range that has only one of the two bounds.
- **FR-018**: When a component has a numeric value and at least one numeric reference bound, the system MUST classify the reading as below range, within range or above range.
- **FR-019**: The system MUST allow an explicit status — normal, high, low, critical or abnormal — to be recorded against a component, and where one is recorded it MUST be the status shown, so that a laboratory's own judgement is never overridden by arithmetic.
- **FR-020**: A component with no numeric reference bound and no explicit status MUST be presented as not assessed against a range, and MUST NEVER be presented as normal.
- **FR-021**: Every out-of-range reading MUST be visually distinguished wherever its value is shown, and the distinction MUST be conveyed by more than colour alone.
- **FR-022**: A lab result MUST report how many of its components are out of range.
- **FR-023**: The system MUST record a change to a result's component set as a change to that lab result in the activity trail.

#### Comparing a component over time

- **FR-024**: The system MUST be able to list, for one person, every distinct component ever recorded for them, each shown once with its latest value, its unit, its latest status, the number of readings and the date of the latest reading.
- **FR-025**: Readings MUST be grouped by the standard test they were matched to where one was matched, and otherwise by their own test name ignoring letter case and surrounding spaces.
- **FR-026**: The system MUST be able to present the readings of one component in date order, each with its value, its unit, the reference range it was measured against, and whether it was out of range.
- **FR-027**: Readings recorded in different units MUST NEVER be combined into one series. The account holder MUST be told that readings exist in more than one unit, MUST be able to choose which unit to see, and the unit being shown MUST be stated on the view.
- **FR-028**: The system MUST NOT convert a value from one unit into another anywhere.
- **FR-029**: A series MUST be restrictable to a date range, and the summary figures MUST be computed over the same range as the series shown.
- **FR-030**: For a numeric series the system MUST report the number of readings, the earliest and latest reading dates, the latest value, the lowest, the highest, the average, and how many readings fell within their reference range.
- **FR-031**: For a numeric series of at least three readings the system MUST report a direction of rising, falling or steady, derived by this published rule: split the readings chronologically into an older half and a newer half, discarding the middle reading when the count is odd; rising if the newer mean exceeds the older mean by more than five per cent of the older mean, falling if it is below by more than five per cent, steady otherwise. The rule MUST be stated where the direction is shown.
- **FR-032**: For a series of fewer than three readings the system MUST state that there are not enough readings to say, and MUST NOT claim a direction.
- **FR-033**: For a categorical or textual component the system MUST present a history of recorded values in date order together with how often each distinct value occurred, and MUST NOT offer an average, a range or a direction.
- **FR-034**: Where a series must be capped for size, the system MUST tell the account holder that it was capped and how to narrow it, and MUST NOT silently return part of a series as though it were the whole.
- **FR-035**: Each reading MUST be judged against the reference range recorded with it, not against the newest one, and where a range is drawn as a band the view MUST state which reading's range it is.

#### The standard test catalogue

- **FR-036**: The system MUST ship a catalogue of standardized lab tests, each carrying a standard code, a name, a short name, a default unit, a category, alternative names, whether it is commonly used, and typical reference bounds.
- **FR-037**: The catalogue MUST be read-only to every account holder: attempts to add to it, change it or delete from it MUST be refused.
- **FR-038**: The catalogue MUST be searchable by name, by alternative name and by standard code, and MUST be filterable by category and by whether an entry is commonly used.
- **FR-039**: While a lab result or a component is being entered, the system MUST offer matching catalogue entries once at least three characters have been typed, and MUST say that nothing matched rather than showing an empty box.
- **FR-040**: Choosing a catalogue entry MUST fill in the name, the unit, the category and the typical reference range, and every one of those MUST remain editable before saving.
- **FR-041**: Choosing a catalogue entry MUST record which entry was chosen, so that readings entered under different spellings of the same test are compared as one.
- **FR-042**: The system MUST NOT require a catalogue match: a result or a component whose test is not in the catalogue MUST be recordable and comparable in its own right.
- **FR-043**: The catalogue MUST contain nothing about any person, and reading it MUST disclose nothing about anyone.

#### Relating a lab result to other records

- **FR-044**: A lab result MUST be able to refer to any number of conditions, encounters, medications, procedures and treatments belonging to the same person.
- **FR-045**: The system MUST refuse a reference to a record belonging to a different person, and the refusal MUST disclose nothing about whether that record exists.
- **FR-046**: A reference MUST be visible from both ends: the lab result lists what it refers to, and each referred record lists the lab results referring to it.
- **FR-047**: Removing a reference MUST leave both records otherwise unchanged, and deleting either record MUST remove the reference without affecting the other record.
- **FR-048**: A reference MUST carry no content of its own in this phase — no per-reference note, purpose or label.

#### Attached documents

- **FR-049**: An account holder MUST be able to attach one or more documents to a clinical record of any kind that exists in the application, without that record's kind having been anticipated by this phase.
- **FR-050**: An attached document MUST record the content, the original name, the size in bytes, the type determined from the content, the person it concerns, the record it is attached to, who attached it, when, an optional description and an optional category from the published set.
- **FR-051**: The type MUST be determined from the content itself, and MUST take precedence over the file's name and over any type the uploader's browser declares.
- **FR-052**: The system MUST refuse a document whose determined type is not among the accepted types, naming the accepted types in the refusal.
- **FR-053**: The system MUST refuse a document larger than the instance's configured size limit before storing any of it, stating the limit, and MUST leave nothing partially stored.
- **FR-054**: The system MUST refuse a document with no content.
- **FR-055**: The system MUST NOT treat a repeated upload of identical content as a duplicate: each attachment is its own item, distinguished by its description and the time it was attached.
- **FR-056**: An account holder MUST be able to retrieve a document's content exactly as uploaded, byte for byte, under its original name.
- **FR-057**: An account holder MUST be able to view a document of a displayable type without downloading it; for every other type only downloading MUST be offered.
- **FR-058**: The system MUST NEVER present an uploaded document in a way that allows its content to run as part of the application, regardless of which types the operator has accepted.
- **FR-059**: The system MUST prepare small preview images when a document is stored, for the types it can render, so that listings show them without further work; for every other type it MUST show an icon indicating the type instead.
- **FR-060**: A preview MUST be protected exactly as strictly as the document it previews.
- **FR-061**: An account holder MUST be able to replace an attached document with a corrected version, keeping its description, its category and its place on the record; the replaced version MUST become recoverable for the retention window.
- **FR-062**: An account holder MUST be able to change a document's description and category; the original name, size and type MUST NOT be editable.
- **FR-063**: Deleting an attached document MUST require confirmation, MUST remove it from the record's listing and from the library, and MUST keep it recoverable for a retention window the operator configures, defaulting to 30 days.
- **FR-064**: An account holder MUST be able to restore a document within the retention window, returning it to the record it was attached to.
- **FR-065**: The system MUST refuse to restore a document whose record no longer exists, explain why, and allow its content to be retrieved until it is purged.
- **FR-066**: The system MUST permanently and irrecoverably purge deleted documents, content included, once the retention window has passed, without an operator having to ask. It MUST also allow a deleted document to be purged **before** its window closes, and to exactly two parties and no others: the owner of the person the document concerns, for their own documents, and the holder of the break-glass credential, for any document. An owner-initiated early purge MUST be guarded by a typed confirmation that names the file and states that the action cannot be undone. An account that can reach the document only through an arrangement somebody else made MUST NOT be able to purge it and MUST be answered exactly as though no such mode existed — owning your own medical records means being able to destroy them on demand, but a recipient of a share never acquires a destructive power the arrangement did not confer. This is MediKube's only statement of the early-purge rule; phase 006 refers to it rather than restating it.
- **FR-067**: Deleting a clinical record MUST move its attached documents into the trash rather than destroying them, keeping the same retention window.
- **FR-068**: Deleting a person or an account MUST destroy their documents outright, including anything of theirs awaiting purge, and the confirmation MUST say so.
- **FR-069**: An account holder MUST be able to see every document for one person in one library, narrowed by the kind of record they are attached to, by category, by words in the name or description, and by whether they are deleted, and sorted by when they were attached.
- **FR-070**: Each document in the library MUST show which record it belongs to and offer a way to open that record.
- **FR-071**: The system MUST report how many documents are stored for a person and how many bytes they occupy, counting what is awaiting purge separately, and MUST make the instance-wide total available to an operator.

#### Privacy, permission and the activity trail

- **FR-072**: Every lab result, component, comparison and document MUST concern exactly one person, and every read and every change MUST be authorized against the signed-in account holder at the moment it happens — never inferred from which person is currently in view.
- **FR-073**: A request for anything in this phase belonging to a person the account holder cannot reach MUST be answered exactly as though it did not exist, disclosing no name, no size, no type, no count and no preview; and the refused attempt MUST be recorded in the activity trail by identifier, without content.
- **FR-074**: A document's address MUST NOT itself grant access to it: it MUST NOT contain a credential, and possessing it MUST give nothing to anyone who could not already reach that person.
- **FR-075**: Document content and previews MUST NOT be retrievable by an unauthenticated request, by a guessed identifier, or by any means that bypasses the authorization check.
- **FR-076**: A retrieval of a document's content or of its preview, whether for viewing or for downloading, MUST be recorded in the activity trail as a sensitive read when, and only when, the reader is not the owner of the person the document concerns — in this phase, the holder of the break-glass credential reading somebody else's document; from phase 005, a recipient of an arrangement as well. The entry MUST carry who, which document, which person and when — and never the file name, the description or any content. An account holder retrieving a document of their own MUST NOT produce an entry: the trail exists to answer who reached data they do not own, recording every self-read would drown it in noise, and a complete timeline of when a person read their own most sensitive results is itself a disclosure. The rule is stated once for records and documents alike in phase 005's widened-authorization contract, and is the same asymmetry that makes reading the trail unaudited while exporting it is audited.
- **FR-077**: Attaching, replacing, describing, deleting, restoring and purging a document MUST each be recorded in the activity trail.
- **FR-078**: No file name, description, test name, value, unit, reference range, interpretation or note may be written to the instance's logs, measurements, traces or error reports. Anything recorded about them MUST use opaque identifiers only.
- **FR-079**: A refusal shown to an uploader MAY name their own file to them, but MUST NOT put that name into any log, measurement, trace or error report.
- **FR-080**: For every operation this phase introduces there MUST be an automated test proving that the owning account succeeds and that an account with no access is refused indistinguishably from the thing not existing.

#### Views, gates and proof

- **FR-081**: This phase MUST add four user-facing views — a list of a person's lab results, a lab result with its components, a comparison view for one component over time, and a person's document library — each rendered inside the application's standard page structure with its own identifiable landmark and its own explanatory empty state.
- **FR-082**: Every view this phase adds MUST be covered by the automated browser check at both a desktop and a phone viewport, with zero browser errors, zero uncaught page failures and zero failed resource requests.
- **FR-083**: Every acceptance scenario in this specification MUST exist as an automated test. This phase is not complete while any of them is missing or failing.
- **FR-084**: The published description of the public interface MUST cover every operation this phase adds, and the application's own address inventory MUST agree with it; a mismatch MUST fail the build.
- **FR-085**: The system MUST remain usable with 5,000 lab results for one person, a single result of 100 components, a component with 500 readings, and 2,000 documents for one person, and this MUST be proven by automated tests rather than assumed.

### Key Entities

- **Lab result**: One laboratory investigation for one person — what was tested, who ordered it, where it was done, when it was ordered, collected and resulted, where it is in its life, and the laboratory's overall comment. Holds either one overall value with its unit and range, or a set of components. Belongs to exactly one person and is deleted permanently when they or it are deleted. Behaves in every other respect like the clinical record kinds delivered by earlier phases, including notes, tagging, confirmation before deletion and refusal of a stale edit.
- **Test component**: One measured line within a lab result — a name, an abbreviation, a value that is a number, a category or free text, a unit, the reference range it was measured against, whether it was out of range, and its position within the result. Reached only through its result; it has no independent life, no notes and no tags of its own, and it is destroyed with its result.
- **Reference range**: What a value was measured against — a lower bound, an upper bound, or a description in words. Recorded with the reading rather than looked up later, so that a reading from three years ago is still judged against the range that applied when it was taken.
- **Standard test**: A catalogue entry describing a test the wider world recognises — a standard code, a name, a short name, a default unit, a category, alternative names, and typical bounds. Ships with the instance, is the same for everybody, contains nothing about any person, and cannot be changed through the application. Choosing one is what makes two differently-spelled readings comparable.
- **Component series (derived)**: Every reading of one component for one person, in one unit, over a period — with the summary figures and the direction computed from them. Nothing about it is stored separately from the readings it is computed from, so correcting a reading corrects the comparison.
- **Attached document**: A file kept with the record it belongs to — its content, its original name, its size, the type determined from that content, an optional description and category, who attached it and when. Concerns exactly one person and is attached to exactly one record. Unlike every clinical record in MediKube, it is not destroyed the moment it is deleted: it waits out a retention window during which it can be brought back, and is then purged permanently.
- **Document preview**: A small rendering of a document, prepared when the document is stored, shown in listings. Exists only for types the system can render; every other type is represented by an icon. Protected exactly as strictly as the document itself.
- **Related-record reference**: The statement that a lab result bears on a condition, an encounter, a medication, a procedure or a treatment belonging to the same person. Readable from both ends, carrying nothing of its own, and removed — without harming either record — when either record is deleted.
- **Storage usage (derived)**: How many documents are held for a person and how many bytes they take, with what is awaiting purge counted separately, and an instance-wide total for the operator. Computed from the documents themselves.
- **Activity entry**: An existing entity from earlier phases, extended here by the actions this phase introduces — attaching, retrieving, replacing, deleting, restoring and purging a document, and creating, changing and deleting a lab result — and by naming a document as the thing an action concerned. It still records who, what, which person, which identifier and when, and never any content.
- **Clinical record (any kind)**: An existing entity from earlier phases, extended here by being able to carry attached documents. No record kind needs to know anything about documents for this to work, and a record kind added in a future phase inherits it without change.

## Success Criteria *(mandatory)*

### Measurable Outcomes

A criterion marked **[outcome metric]** is observed on a real person rather than built: it maps to
no task by design, and it says so here rather than being left silently unmapped. Every other
criterion below maps either to a task in `tasks.md` or to a phase-exit criterion in `plan.md`, so
an unmapped one that carries no marker is a gap.

- **SC-001** *[outcome metric]*: A person who already knows the application can record a lab panel of ten components, with values, units and ranges, in under 3 minutes.
- **SC-002**: 100% of numeric readings that fall outside their recorded reference range are marked as out of range everywhere their value appears, 0% of readings inside their range are marked, and the marking is perceivable without relying on colour — verified by automated tests and at both viewports.
- **SC-003**: A component with 100 readings across 50 lab results presents its comparison and its summary figures within 2 seconds of being asked for, and readings recorded in two different units are never presented as one series — verified by an automated test that would fail if they were.
- **SC-004**: A document of any accepted type, up to the configured size limit, is retrieved byte-for-byte identical to what was uploaded, verified by comparing the retrieved content with the original in an automated test.
- **SC-005**: 100% of attempts by an account with no access to list, read, retrieve, change or delete a lab result, a component, a comparison or a document are refused, and every refusal is indistinguishable from a request for something that never existed — verified across every way each of them can be addressed.
- **SC-006**: 100% of document retrievals by somebody who is not the owner appear in the activity trail as a sensitive read, 0% of an owner's retrievals of their own documents produce an entry at all, and 0% of activity entries contain a file name, a description, a test name, a value or any other content.
- **SC-007**: A document deleted within the retention window is restored by its owner in under 30 seconds; a document deleted longer ago than the window cannot be recovered by anyone through the application, and its content is no longer stored — both verified by automated tests.
- **SC-008**: Zero occurrences of clinical or identifying content — including file names — are found in the instance's logs, measurements, traces and error reports after an automated exercise of every operation this phase defines.
- **SC-009**: 100% of the user-facing views this phase adds pass the automated browser check at both a desktop and a phone viewport, with zero browser errors, zero uncaught page failures and zero failed resource requests on every one of them.
- **SC-010**: 100% of the acceptance scenarios in this specification exist as automated tests and pass. This phase is not complete while any one of them is missing or failing.
- **SC-011**: A person with 5,000 lab results locates any one of them within 10 seconds using only the list's own ordering and narrowing, and every page of that list is displayed within 2 seconds of being requested.
- **SC-012**: 100% of uploads whose content is not an accepted type, or which exceed the size limit, or which are empty, are refused regardless of the file's name or declared type, and 0% of them leave anything stored.
- **SC-013**: Deleting a lab result removes 100% of its components and moves 100% of its attached documents into the trash; deleting a person removes 100% of their documents including those awaiting purge — both verified by looking for the data afterwards rather than by assumption.
- **SC-014**: Suggestions from the standard test catalogue appear within 1 second of the third character being typed, and 100% of chosen entries fill in the name, unit, category and typical range while leaving all four editable.
- **SC-015**: Two readings of the same standard test entered under different spellings are presented as one comparable series 100% of the time when both were matched to that catalogue entry.
- **SC-016**: The published description of the public interface matches the operations the application actually serves at all times; any mismatch fails the build 100% of the time.

## Assumptions

**Position in the sequence**

- **Phase 001 (Walking Skeleton)** has delivered accounts, sign-in and sessions, the application shell with its navigation, error views and empty-state pattern, one clinical record kind end to end, permanent deletion with confirmation, refusal of a stale edit, live updating of an open list, the activity trail, configuration from the environment, the operator command surface, the published interface description with its address-inventory gate, and the automated browser check. This phase adds record kinds and views to those mechanisms and changes none of them.
- **Phase 002 (Patient Core)** has delivered people as entities distinct from the account holder, the ownership rule by which an account reaches only the people it owns, switching between people, the rule that every clinical record is attributed to exactly one person, and the directories of practitioners and places of care. Lab results and documents are attributed and reached by exactly those rules; this phase introduces no new basis for permission.
- **Phase 003 (Clinical Records)** has delivered the remaining clinical record kinds — among them conditions, encounters, procedures and treatments — which story 5 links lab results to and to which story 2 attaches documents. Where a kind named in story 5 is not present on an instance, the reference to it is simply not offered; nothing in this phase fails because of it.
- **Phase 005 (Sharing and Invitations)** will widen who can reach a person, and with them that person's lab results and documents. Everything specified here is written so that widening changes only which accounts pass the permission check. Two consequences are stated now so that phase is not surprised: a recipient granted viewing access must be able to open and retrieve documents but not attach, replace, delete or purge them; and every retrieval by a recipient is recorded in the activity trail — precisely because it is not an owner's own read, which FR-076 deliberately leaves unrecorded.
- **Phase 006 (Reporting and Ops)** will add the instance-wide operator figures for deleted documents awaiting purge — a count and a byte total on the operator overview, pointing back at this phase's surface rather than offering a second one — the charts that plot the comparisons this phase computes, and export. This phase deliberately does not build any of them. It does, however, deliver the automatic purge itself, because a retention window nothing enforces is not a retention window; the later phase adds visibility over it, not the behaviour.

**Scope decisions taken here rather than deferred**

- **Documents wait 30 days by default before being purged.** The window is an operator setting. It is measured from when the document was deleted, applies equally to documents deleted directly, replaced, and orphaned by their record being deleted, and is stated to the account holder at the moment they confirm a deletion.
- **The default size limit for one document is 32 megabytes**, and the default accepted types are PDF, JPEG, PNG, WebP, HEIC, TIFF, GIF, plain text and comma-separated values. Both are operator settings. Types whose content can run in a browser — markup, scalable vector graphics, scripts — are not accepted by default, and even where an operator adds one it is never presented in a way that lets it run.
- **Previews are prepared only for the image types the system can decode.** Everything else — including PDF, which is the commonest attachment of all — is represented in listings by an icon indicating its type. A preview of the first page of a document is not promised by this phase.
- **There is no per-account or per-person storage quota.** The limits are per document, and storage is reported rather than rationed. A self-hosted instance's disk is the operator's to manage, and the reporting is what lets them manage it.
- **Values are never converted between units.** A reading in one unit and a reading in another are different series, presented as such. Converting them silently is how a laboratory history becomes actively misleading, and no conversion table is trustworthy across every test in the catalogue.
- **A component carries no notes and no tags of its own.** It is reached through its result, and the result's notes and tags cover it. Giving each line of a panel its own tagging would multiply the tagging surface by a hundred for no use anybody has described.
- **A lab result holds either one overall value or a set of components, and can be changed from one to the other.** The predecessor system fixed this shape at creation and could not correct a result that turned out to be a panel; that restriction is not carried over.
- **The direction of a comparison is computed by the published halves rule in FR-031** and requires at least three readings. It is a description of the data, stated with its rule visible, and it is not a clinical judgement.
- **Replacing a document is specified by its effect**, not by a mechanism: the corrected version takes the old one's place and the old one becomes recoverable for the retention window. How that is achieved is a matter for planning.
- **The interface remains English-only in this phase**, consistent with earlier phases; dates and numbers follow the account holder's recorded presentation preferences.
- **Document content is held wherever the operator has configured the instance to hold it.** Nothing in this specification changes with that choice, and every rule about authorization, retention and purging applies identically.

**Deliberately not in this phase**

- **Reading a value out of an uploaded document.** Extracting text or laboratory values from a scanned report is out of scope for MediKube entirely, not merely deferred. Documents are stored and shown; they are never parsed.
- **Synchronising documents with an external document-management system.** Also out of scope for MediKube entirely. The predecessor system carried a whole synchronisation state machine for this; MediKube does not.
- **Charts and printed reports built on these comparisons.** The comparison and its figures are produced here; plotting them into a report belongs to phase 006.
- **A unified search across every record kind.** The charter for this phase does not include it, and where the shared design contract's phase table places it here, the charter governs — exactly as it did for phase 001. This phase delivers narrowing within a person's lab result list and within their document library, each over its own content, and nothing that searches across kinds.
- **A second surface for deleted documents.** Deleted documents are reachable as a filter on a person's document library — `?deleted=true` — and that is the only place in MediKube where one is listed, restored or purged. Phase 006 adds the instance-wide count and byte total to the operator overview and links here; it does not add a page of its own.
- **Versions of a document beyond the one replacement rule.** Replacing keeps the previous version only for the retention window, and only so that a mistaken replacement can be undone. This is not document version history.
- **Recording who at a laboratory performed a test, specimen identifiers, or the accession number the laboratory used.** None of these has been asked for, and each would add a required field to every panel entered by hand.
