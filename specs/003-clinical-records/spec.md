# Feature Specification: Clinical Records

**Feature Branch**: `003-clinical-records`

**Created**: 2026-08-26

**Status**: Draft

**Input**: User description: "The full clinical picture: the remaining twelve record types, the relationships between them, and the ability to find anything. In scope: allergies, conditions, encounters, procedures, treatments, symptoms with their occurrences, vitals, immunizations, injuries with injury types, insurance, medical equipment, and emergency contacts — each with list, view, create, edit, delete, and the fields and enumerations the domain requires. The relationships between records: a condition treated by a medication, a symptom linked to a condition, a procedure arising from an injury. Tagging across record types. Unified search across a patient's whole record. Timeline and status views such as active conditions and current medications."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Record what would hurt me in an emergency (Priority: P1)

Amara's father is taken to hospital confused and unable to answer questions. On her phone she opens his chart and reads out, in this order: he is allergic to penicillin and the reaction is anaphylaxis; he lives with atrial fibrillation and type 2 diabetes, both active; his brother is the second person to call after her. She recorded all of that months ago — each allergy with what it does to him and how badly, each condition with when it started and whether it is still going on, each contact with a relationship and a number — and she can correct any of it from the same screens.

**Why this priority**: These three are what a clinician asks for first and what a carer is least able to recall under stress. They are also the smallest complete slice of this phase: on their own they turn the application from "a list of medications" into something worth opening in an emergency, and they establish the shape every remaining record type repeats.

**Independent Test**: Sign in, choose a person, record two allergies of differing severity, two conditions with differing status, and two emergency contacts one of which is marked primary. Confirm each appears in its own list and on the person's chart, that the severe allergy is visibly distinguished from the mild one, that each can be corrected and deleted with confirmation, and that a second account can reach none of them.

**Acceptance Scenarios**:

1. **Given** a person in view with nothing recorded, **When** the account holder opens the allergies screen, **Then** they are shown an explanation of what allergies are for and an obvious way to record the first one, not a blank screen or an error.
2. **Given** an account holder recording an allergy, **When** they supply what the person reacts to and how severe the reaction is, **Then** the allergy is saved against that person, appears at the top of the list, and can be opened, corrected and deleted.
3. **Given** an allergy recorded as life-threatening and still active, **When** the person's chart is opened, **Then** that allergy is presented distinctly from allergies that are mild or no longer active.
4. **Given** an account holder recording a condition, **When** they mark it resolved without supplying a date it resolved, **Then** the save is refused with an explanation naming the missing detail.
5. **Given** a condition with an onset date, **When** the account holder supplies a resolution date earlier than the onset date, **Then** the save is refused and both offending values are reported together.
6. **Given** two emergency contacts for one person, **When** the account holder marks the second as primary, **Then** the first is no longer primary and the change is explained rather than silently applied.
7. **Given** a record of any of these three types, **When** an account that does not own the person requests it by any means, **Then** the response is indistinguishable from the record not existing.

---

### User Story 2 - Record the care I have received (Priority: P2)

Amara keeps a running account of her father's care: the encounter last Tuesday and what was said, the cardioversion he had in March and how it went, the anticoagulation therapy he has been on since. Each visit records why he went, who he saw, where, what they concluded and what happens next. Each procedure records what was done, when, how it turned out and what it was for. Each course of treatment records what it is, when it started, whether it is still running and what it is meant to achieve.

**Why this priority**: This is the narrative of care — the part a person cannot reconstruct from memory a year later and the part a new clinician asks for. It depends on nothing in story 1 and delivers value on its own, but it is second because a mistake here is inconvenient where a mistake in story 1 is dangerous.

**Independent Test**: For one person, record an encounter with a practitioner and a place of care from the existing directories, a procedure that is scheduled for a future date, a procedure that has already been completed, and a course of treatment that is still running. Confirm each lists, opens, corrects and deletes; confirm the scheduled procedure appears in a scheduled view and the completed one does not.

**Acceptance Scenarios**:

1. **Given** an account holder recording an encounter, **When** they supply the reason for it and the date it happened, **Then** it is saved, and the practitioner and place of care they chose from their directories are shown on it.
2. **Given** an encounter, **When** the account holder records what was concluded and what happens next, **Then** both are kept separately from the reason for the visit and neither is confused with a diagnosed condition.
3. **Given** a procedure marked as scheduled, **When** the account holder gives it a date in the future, **Then** it is accepted; **And** when they mark a procedure completed with a date in the future, the save is refused.
4. **Given** a course of treatment with a start date, **When** the account holder records an end date before it, **Then** the save is refused with both values reported.
5. **Given** an encounter that names a condition it concerned, **When** that condition is opened, **Then** the encounter is listed on it, without the account holder having recorded the relationship twice.

---

### User Story 3 - Track how I actually feel, and what the numbers say (Priority: P3)

Between encounters, Amara records episodes as they happen — her father's dizziness on Tuesday morning, seven out of ten, lasting an hour, after standing up — and the blood pressure and weight she measures at home. When the cardiologist asks how often the dizziness happens and what his pressure has been doing, the answer is in the application rather than in her memory.

**Why this priority**: Episodes and measurements are the only clinical data the person themselves produces, and the only data that is worthless if it is not captured at the moment it occurs. It is third because it is additive: the chart is already useful without it.

**Independent Test**: Record four episodes of the same symptom on different dates with differing severity, and six sets of measurements over two months. Confirm the symptom screen shows how many times that symptom has been recorded and when it was last recorded, without a separate "symptom definition" ever having been created; confirm an out-of-range measurement is refused with the acceptable range named.

**Acceptance Scenarios**:

1. **Given** an account holder recording a symptom episode, **When** they supply what it was, how severe it was and when it occurred, **Then** an episode is saved; **And** recording the same symptom again a week later creates a second episode rather than editing the first.
2. **Given** several episodes of the same symptom for one person, **When** the symptom list is opened, **Then** it states for that symptom how many episodes have been recorded and the date of the most recent one.
3. **Given** an account holder recording measurements, **When** they submit a set with no measurement filled in at all, **Then** the save is refused, because a reading of nothing is not a reading.
4. **Given** an account holder recording a blood pressure, **When** they supply only one of the two numbers, **Then** the save is refused and the missing number is named.
5. **Given** an account holder recording a body temperature of 250, **When** they submit it, **Then** the save is refused with an explanation naming the range the application accepts.
6. **Given** measurements recorded by an account holder who prefers imperial units, **When** another member of the household views the same measurements with metric preferred, **Then** both see the same underlying reading expressed in their own units, and neither view has altered what was recorded.

---

### User Story 4 - Record prevention and the things that happened to me (Priority: P4)

Amara's son needs a vaccination record for school and broke his wrist falling off a scooter. She records each vaccination with what it was, when it was given, by whom and where, along with the batch number from the card. She records the injury with what it was, which part of the body and which side, how it happened, how bad it was and whether it has healed.

**Why this priority**: Vaccination history is the record most often demanded by somebody outside the household, and an injury history is what makes a later complaint about the same joint make sense. Both are self-contained and neither blocks anything else.

**Independent Test**: Record three vaccinations including one with a dose number and a batch number, and two injuries with differing type, side and status. Confirm each lists, opens, corrects and deletes; confirm the injury type comes from a fixed vocabulary that includes a catch-all and cannot be extended by ordinary use.

**Acceptance Scenarios**:

1. **Given** an account holder recording a vaccination, **When** they supply what was given and when, **Then** it is saved, and the batch number, dose number, manufacturer, site and route they optionally supplied are kept with it.
2. **Given** a vaccination given as part of a series, **When** the account holder records a dose number of zero or a negative number, **Then** the save is refused.
3. **Given** an account holder recording an injury, **When** they choose the injury type, **Then** they choose from a fixed vocabulary including a catch-all value, and they cannot add a new type in passing.
4. **Given** an injury to a paired part of the body, **When** the account holder records which side, **Then** the side is shown wherever the injury is shown; **And** where the side does not apply, they may say so rather than being forced to guess.
5. **Given** an injury recorded as healed, **When** the injuries list is filtered to those still unresolved, **Then** it does not appear.

---

### User Story 5 - Record the practical layer: cover and equipment (Priority: P5)

Amara keeps her father's insurance policy details where she can reach them from a hospital corridor: who the insurer is, the member number, what it covers and what it costs, and when it expires. She also keeps the equipment he depends on — the machine he sleeps with, the meter he tests with — with model and serial numbers, when it was prescribed and when it is next due for a service.

**Why this priority**: Neither is clinical, and both are what actually holds up admission or a repair. They are last of the record types because everything above them is medical and these are administrative.

**Independent Test**: Record two policies for one person, one marked primary, one expiring next month; and two pieces of equipment, one overdue for service. Confirm the second policy marked primary displaces the first, that policies expiring soon can be listed, and that equipment due or overdue for service can be listed.

**Acceptance Scenarios**:

1. **Given** an account holder recording a policy, **When** they supply the kind of cover, the insurer, the member's name and number and the date cover began, **Then** it is saved; **And** the amounts they recorded for deductible, out-of-pocket maximum, co-payments and coinsurance are kept with a stated currency.
2. **Given** a policy already marked primary for a person, **When** the account holder marks a second policy primary, **Then** only one remains primary and the change is explained.
3. **Given** a policy that expires within the next sixty days, **When** the account holder asks for cover that is expiring, **Then** it is listed and the list states why each entry qualifies.
4. **Given** a piece of equipment with a service due date in the past, **When** the account holder asks for equipment needing service, **Then** it is listed alongside equipment due within the next thirty days, and each row states which of the two it is.
5. **Given** a member number recorded on a policy, **When** any diagnostic output of the installation is inspected, **Then** the member number appears nowhere in it.

---

### User Story 6 - Connect the records that belong together (Priority: P6)

Amara's father's dizziness is caused by one of his medications, his anticoagulation was started because of the atrial fibrillation, and the cardioversion happened because of the same condition. She records those connections once, from whichever record she happens to be looking at, and from then on opening any one of them shows the others. On the treatment, she records that the anticoagulant is taken at a different dose from the one on the medication itself — and the application shows the course dose while still making clear where the other value came from.

**Why this priority**: Connections are what turn thirteen separate lists into one clinical picture, and the reason to link a symptom to the medication suspected of causing it is patient safety. It is sixth because every record type it connects must exist first.

**Independent Test**: Link a condition to two medications, a symptom to a condition and to a medication in a "suspected cause" role, a procedure to an injury, and a medication to a course of treatment with its own dose and prescriber. Open each record from both ends and confirm the connection is visible from both. Delete one linked record and confirm the other survives with the connection gone.

**Acceptance Scenarios**:

1. **Given** a condition and two medications for the same person, **When** the account holder links them from the condition, **Then** opening either medication shows the condition, without a second connection having been recorded.
2. **Given** a symptom and a medication, **When** the account holder links them, **Then** they state whether the medication treats the symptom or is suspected of causing it, and that distinction is shown wherever the link is shown.
3. **Given** a record of one person and a record of another person, **When** the account holder attempts to link them, **Then** the link is refused and no information about the other person's record is disclosed.
4. **Given** a condition linked to a medication, **When** the condition is deleted, **Then** the medication survives untouched and no longer shows a link to anything.
5. **Given** a medication attached to a course of treatment, **When** the account holder records a course-specific dose but no course-specific prescriber, **Then** the course shows its own dose and the medication's own prescriber, and states which values came from the medication.
6. **Given** a medication already attached to a course of treatment, **When** the account holder attaches the same medication to the same course again, **Then** the existing attachment is updated rather than duplicated.

---

### User Story 7 - Organise records my own way (Priority: P7)

Amara tags records across every type — "cardiology", "school forms", "insurance claim pending" — and then pulls up everything carrying a tag regardless of what kind of record it is. She renames a tag once and every record follows. She deletes one and no record is lost.

**Why this priority**: A tag that works on one record type is a category; a tag that works on all of them is how a household actually organises care. It is seventh because it is only useful once there are many kinds of record to organise.

**Independent Test**: Create three tags, apply them across at least five different record types for one person, rename one tag, filter each list by it, then delete a tag and confirm every record it was on still exists without it.

**Acceptance Scenarios**:

1. **Given** an account holder recording any type of record, **When** they apply tags, **Then** any number of tags may be applied, and the same tag may be applied to records of different types.
2. **Given** an existing tag, **When** the account holder creates another with the same name in different letter case, **Then** it is refused as a duplicate.
3. **Given** a tag applied to many records, **When** the account holder renames it, **Then** every record shows the new name, and no record loses the tag.
4. **Given** a tag applied to many records, **When** the account holder deletes it, **Then** they are first told how many records carry it, and afterwards every one of those records still exists with the tag removed.
5. **Given** tags belonging to one account, **When** another account records anything, **Then** the first account's tags are neither offered nor discoverable.

---

### User Story 8 - Find anything in the whole record (Priority: P8)

Amara half-remembers something about "warfarin" in her father's chart but not whether it was a medication, a note on an encounter or a symptom. She types it once and gets back everything of his that mentions it, grouped by what kind of record it is, most recent first, and she can narrow it by kind, by tag and by date.

**Why this priority**: With thirteen record types, the cost of not finding something rises sharply; search is what keeps the application usable as the chart grows. It is eighth because it searches what the earlier stories create.

**Independent Test**: For a person with records of at least six different types, search a term that occurs in three of them and confirm all three groups come back, grouped and paged separately; search a term that occurs in another person's records only and confirm nothing comes back; narrow one search by kind, by tag and by date range and confirm each narrowing is reflected.

**Acceptance Scenarios**:

1. **Given** a person with records of several types, **When** the account holder searches for a term, **Then** matching records of every type are returned together, grouped by type, each group stating whether more results exist.
2. **Given** a search with no matches, **When** the results are shown, **Then** the "nothing matched that" state is distinct from the "nothing has been recorded yet" state and offers to clear the narrowing.
3. **Given** a search, **When** the account holder does not say which person they are searching, **Then** the search is refused rather than silently answered for whoever happens to be in view.
4. **Given** a term that appears in another account's records, **When** it is searched, **Then** nothing is returned and nothing discloses that a match exists elsewhere.
5. **Given** any search, **When** the installation's diagnostic output is inspected afterwards, **Then** the search term appears nowhere in it.

---

### User Story 9 - See the current picture without reading everything (Priority: P9)

Before an encounter Amara wants four answers: what is active, what is being taken, what is coming up and what needs attention. One screen answers them — active conditions, current medications and treatments, scheduled procedures, unresolved injuries, equipment due for service, cover about to lapse — and a single chronological timeline shows everything that has happened to her father in the last year regardless of type.

**Why this priority**: This is what the phase is for; it is last because it is a view over everything above it and delivers nothing without them.

**Independent Test**: For a person with a mix of active and inactive records across at least eight types, confirm each status view returns exactly the records that qualify and states why each qualifies, and confirm the timeline interleaves records of different types in date order and can be narrowed by type and date range.

**Acceptance Scenarios**:

1. **Given** a person with both active and resolved conditions, **When** the active view is opened, **Then** only active conditions appear and the view states the basis on which they were selected.
2. **Given** records of eight different types with dates spread over two years, **When** the timeline is opened, **Then** they appear interleaved in date order, each entry stating its type, its identifying summary and its date.
3. **Given** the timeline narrowed to two types and a three-month window, **When** it is shown, **Then** only records of those types within that window appear, and the narrowing is visible and removable.
4. **Given** a person with nothing recorded, **When** any status view or the timeline is opened, **Then** a helpful empty state appears rather than a blank screen, a row of zeros or an error.
5. **Given** a record the account holder is not entitled to see, **When** any status view or the timeline is built, **Then** that record is absent from it and its absence discloses nothing.

---

### User Story 10 - Record what runs in the family (Priority: P10)

Amara records that her father's mother had breast cancer diagnosed at 62 and that his brother has the same heart rhythm problem, so that a clinician asking about family history gets an answer rather than a shrug.

**Why this priority**: Family history is real clinical value but it is the least urgent thing in this phase, it is nobody's daily use, and its main pay-off — sharing a pedigree with a relative — arrives in a later phase.

**Independent Test**: For one person, record three relatives with differing relationships, one deceased, and give one of them two conditions with an age at diagnosis. Confirm they list, open, correct and delete, and that a second account can reach none of them.

**Acceptance Scenarios**:

1. **Given** an account holder recording a relative, **When** they supply a name and a relationship chosen from the offered vocabulary, **Then** the relative is saved against the person whose family it is.
2. **Given** a relative, **When** the account holder records conditions that relative had, **Then** each condition keeps its own name, age at diagnosis, severity, status and notes.
3. **Given** a relative marked deceased, **When** a year of death earlier than the year of birth is recorded, **Then** the save is refused with both values reported.
4. **Given** a relative recorded for one person, **When** a second account requests them, **Then** the response is indistinguishable from the relative not existing.

---

### Edge Cases

- **A record with nothing but the bare minimum.** Every type has a small number of required details and a large number of optional ones. A record carrying only the required ones must list, open, correct, delete, be searched, be tagged and appear on the timeline exactly like a fully populated one, and every screen must show absent details as absent rather than as blanks, dashes or zeros.
- **A record whose date is unknown.** For every type whose primary date is optional, a record without one must still be reachable and must sort predictably; the timeline must state that its date is unknown rather than placing it at the beginning or end of time.
- **Two people editing the same record.** A save based on a version that has since changed is refused, the account holder is told what happened and shown the current values; nothing is silently overwritten. The same rule applies to attaching and detaching a link.
- **Two people adding to the same list at once.** Paging through a list while records are being added or removed must neither repeat nor skip an entry.
- **Deleting a record that other records point at.** The linked records survive, keep every other link they had, and the removed link disappears from both sides. The account holder is told, before confirming, how many other records refer to the one being deleted.
- **Deleting a directory entry that records point at.** Deleting a practitioner or a place of care never deletes a clinical record; the reference is cleared and the record remains, as established in the previous phase.
- **Deleting the person.** Deleting a person destroys every record of every type in this phase, their links, their tags' application to them and their family history, leaving nothing attributed to a person who no longer exists.
- **Deleting a tag that is in use.** The account holder is told how many records carry it. On confirmation, the tag disappears from every record and no record is lost.
- **A vocabulary value that no longer fits.** Every enumerated detail offers a catch-all where the domain genuinely admits one, so no clinical fact is unrecordable; where no catch-all is appropriate the vocabulary is documented and complete.
- **Contradictory dates.** An end before a start, a resolution before an onset, a death before a birth, a completed event in the future: each is refused, and every offending value in one submission is reported together rather than one at a time.
- **Impossible measurements.** Every numeric measurement has a documented acceptable range and a value outside it is refused with the range stated. A blood pressure with only one of its two numbers is refused. A set of measurements with no measurement in it is refused.
- **The same symptom, over and over.** Recording the same symptom on twenty occasions produces twenty episodes, never one record edited twenty times, and the application derives how often and how recently rather than asking the account holder to maintain a count.
- **Records that arrive faster than a person can read them.** A list left open while records are added elsewhere updates without a manual refresh, and a list left open for an hour is still updating.
- **A very large chart.** A person with fifty thousand records across every type must still list, search, filter, page and see a timeline within the times stated in the success criteria, and the counts on their chart must remain correct.
- **A search term that matches everything.** A term matching thousands of records returns a first page promptly, states that more exist per group, and never attempts to present them all at once.
- **A search term that matches nothing.** Distinguished from an empty chart, and offering to clear the narrowing rather than repeating the same message.
- **Text that looks like markup, or is not in Latin script.** Stored, displayed, searched and tagged correctly, and never interpreted as anything other than text.
- **An attempt to link across people.** Refused, with a response that discloses nothing about the other person's record — including whether it exists.
- **An attempt to reach any record of any type from an account that does not own the person.** Refused identically to a request for something that does not exist, whether the record is addressed directly, through a link on another record, through a tag, through search, through the timeline or through a status view.
- **A signed-out request.** Refused, with nothing about any person in the refusal.

## Requirements *(mandatory)*

### Functional Requirements

#### The shape every record type shares

- **FR-001**: The system MUST allow an account holder to list, open, create, correct and delete records of each of the twelve types this phase adds — allergies, conditions, encounters, procedures, courses of treatment, symptom episodes, sets of measurements, vaccinations, injuries, insurance policies, medical equipment and emergency contacts — and of family history, for any person that account can reach.
- **FR-002**: The system MUST file every record of every type against exactly one person, MUST make it impossible to create one filed against nobody, and MUST refuse any attempt to re-file an existing record against a different person.
- **FR-003**: The system MUST offer free-text notes on every record type, up to a documented length, and MUST treat those notes as clinical content everywhere they are stored, displayed, exported or diagnosed.
- **FR-004**: The system MUST report every invalid detail of a submission together, rather than rejecting one at a time, for every type.
- **FR-005**: The system MUST refuse a correction that is based on a version of the record which has since changed, MUST say so plainly, and MUST show the current values.
- **FR-006**: The system MUST require an explicit confirmation before deleting any record, MUST state in that confirmation what will be destroyed and how many other records refer to it, and MUST make the deletion permanent with no recovery path offered.
- **FR-007**: The system MUST order every list by that type's own principal date, most recent first, with a documented tie-break so that ordering is identical on every request, and MUST page every list in a way that neither repeats nor skips an entry while records are added or removed.
- **FR-008**: The system MUST present a helpful empty state where a person has no records of a type, and MUST present a distinct state where records exist but none match the narrowing in force.
- **FR-009**: The system MUST give every record type its own list screen and its own detail screen, and MUST allow a record to be created from either that type's list or the person's chart, stating in both cases which person it will be filed against.
- **FR-010**: The system MUST keep an open list of any type up to date as records are added, changed and deleted elsewhere, without the viewer refreshing, and MUST continue doing so for a list left open for at least an hour.
- **FR-011**: The system MUST record a clinical date that has no meaningful time of day as a calendar date that never shifts with the viewer's time zone, and MUST record anything that happened at an instant in a single universal reference and present it in the viewer's local terms.
- **FR-012**: The system MUST draw every enumerated detail from a fixed, documented vocabulary that includes a catch-all value wherever the domain admits one, MUST NOT allow those vocabularies to be extended by ordinary use, and MUST use one vocabulary for one idea across every type that expresses that idea — one severity ladder, one ladder for whether something is still going on, one ladder for a course of therapy, one ladder for an ordered event.
- **FR-013**: The system MUST refuse a record whose end date precedes its start date, whose resolution date precedes its onset date, or whose completion is dated in the future, for every type that carries such a pair.
- **FR-014**: The system MUST show, on any record that names a practitioner or a place of care, the entry from the account holder's own directory, and MUST allow such an entry to be added at the moment it is needed without losing the record being written.
- **FR-015**: The system MUST make every record of every type in this phase countable in the per-kind counts on a person's chart and includable in that person's recent-activity summary, without either being re-specified per type.

#### Allergies

- **FR-016**: The system MUST record for an allergy what the person reacts to, what the reaction is, how severe it is, whether it is still current, and when it was first noticed — requiring what they react to and how severe it is, and treating the rest as optional.
- **FR-017**: The system MUST allow one allergy to refer to several medications, because a reaction to a class of drug concerns every medication in that class.
- **FR-018**: The system MUST distinguish, wherever allergies are presented, those that are severe or life-threatening and still current from those that are not, so that a reader scanning under pressure cannot miss one.

#### Conditions

- **FR-019**: The system MUST record for a condition the diagnosis, whether it is active, healing, inactive, resolved or long-term, how severe it is, when it began, when it resolved, the clinical codes a clinician may have given it, and the practitioner concerned — requiring the diagnosis and whether it is still going on.
- **FR-020**: The system MUST require a resolution date on a condition marked resolved, and MUST refuse a resolution date earlier than the onset date or later than today.
- **FR-021**: The system MUST show, on a condition, the records that concern it — the medications it is treated with, the encounters about it, the procedures arising from it, the courses of treatment for it, the symptoms attributed to it and the injuries that led to it — without the account holder having recorded any of those relationships twice.

#### Encounters and visits

- **FR-022**: The system MUST record for an encounter why the person attended and when, and optionally what kind of visit it was, how urgent it was, what was concluded, what was planned, what follow-up was arranged, how long it took, the practitioner, the place of care and the condition it concerned — requiring the reason and the date.
- **FR-023**: The system MUST keep what was concluded at an encounter separate from the person's recorded conditions, so that a clinician's impression on the day is never mistaken for a diagnosis the person lives with.

#### Procedures

- **FR-024**: The system MUST record for a procedure what was done, when, and whether it is ordered, scheduled, under way, completed or cancelled, and optionally its kind, its code, a description, how it turned out, the setting, complications, how long it took, the anaesthesia used and notes on it, the practitioner, the place of care and the condition it arose from — requiring what was done, the date and the status.
- **FR-025**: The system MUST accept a future date on a procedure that is scheduled and MUST refuse a future date on one recorded as completed.
- **FR-026**: The system MUST allow procedures that have not yet happened to be listed on their own, and MUST state in that list the basis on which each was selected.

#### Courses of treatment

- **FR-027**: The system MUST record for a course of treatment what it is, its kind, where it is delivered, a description, when it started and ended, how often and at what dose it is given, what it is expected to achieve, whether it is running, on hold, completed, stopped or cancelled, the practitioner, the place of care and the condition it addresses — requiring what it is.
- **FR-028**: The system MUST allow a course of treatment to refer to the encounters at which it was reviewed, the equipment it uses and the medications it involves.

#### Symptom episodes

- **FR-029**: The system MUST record a symptom as an individual episode — what it was, when it occurred, how severe it was, and optionally its category, how long it lasted, a pain rating from nought to ten, where in the body it was, what triggered it, what relieved it, how much it interfered with daily life, when it resolved, whether it is long-standing and its current state — requiring what it was, when it occurred and how severe it was.
- **FR-030**: The system MUST create a new episode each time the same symptom is recorded again, and MUST NOT require the account holder to define a symptom before recording an episode of it.
- **FR-031**: The system MUST derive and present, for each distinct symptom of a person, how many episodes have been recorded and the date of the most recent one, and MUST keep both correct as episodes are added and deleted without either being stored as a maintained count.
- **FR-032**: The system MUST allow a symptom episode to refer to the conditions it is attributed to, the courses of treatment addressing it, and medications in either of two distinct roles — a medication that treats it and a medication suspected of causing it — and MUST show which role applies wherever the reference is shown.

#### Measurements

- **FR-033**: The system MUST record a set of measurements taken at a stated time, which may include blood pressure, heart rate, breathing rate, temperature, oxygen saturation, weight, height, blood glucose with the circumstances of the reading, long-term glucose control, a pain rating and the device used, together with the practitioner where one took them.
- **FR-034**: The system MUST refuse a set of measurements in which no measurement at all has been supplied.
- **FR-035**: The system MUST refuse any measurement outside a documented plausible range for that measurement, and MUST name the accepted range in the refusal.
- **FR-036**: The system MUST require both numbers of a blood pressure whenever either is supplied, and MUST refuse a reading in which the lower number is not below the upper one.
- **FR-037**: The system MUST accept and present height, weight, temperature and glucose in the account holder's preferred unit system while recording each in one canonical form, and MUST derive body mass index when it is shown rather than storing it.

#### Vaccinations

- **FR-038**: The system MUST record for a vaccination what was given and when, and optionally the trade name, the dose number in a series, the batch number, the manufacturer, the site and route of administration, the expiry date, the practitioner and the place of care — requiring what was given and when.
- **FR-039**: The system MUST refuse a dose number that is not a positive whole number.

#### Injuries

- **FR-040**: The system MUST record for an injury what it was, which part of the body it affected, and optionally its type chosen from a fixed vocabulary, which side of the body, when it happened, how it happened, how severe it was, whether it has healed and notes on recovery, together with the practitioner concerned — requiring what it was and the part of the body.
- **FR-041**: The system MUST offer, for the side of the body, values covering left, right, both and not applicable, so that an injury to a part that has no side is recordable without a false choice.
- **FR-042**: The system MUST allow an injury to refer to the conditions that followed it, the medications given for it, the procedures it led to and the courses of treatment it required.

#### Insurance

- **FR-043**: The system MUST record for an insurance policy the kind of cover, the insurer, the member's name, the member number and the date cover began as required details, and optionally the plan name, the employer group, the group number, the policy holder's name and the member's relationship to them, the date cover ends, whether the policy is active, inactive, expired or pending, and whether it is the primary policy.
- **FR-044**: The system MUST record with a policy the amounts that matter at the point of care — deductible, out-of-pocket maximum, co-payments for primary, specialist and emergency care, and coinsurance — each with a stated currency, and the insurer's contact details including a claims telephone number and a portal address.
- **FR-045**: The system MUST allow at most one policy per person to be marked primary, and MUST explain the change when marking a second displaces the first.
- **FR-046**: The system MUST allow policies whose cover ends within a stated number of days to be listed, defaulting to sixty days, and MUST state in that list why each policy qualifies.
- **FR-047**: The system MUST treat a member number, a group number and a policy holder's name as identifying content subject to every privacy rule in this specification.

#### Medical equipment

- **FR-048**: The system MUST record for a piece of equipment what it is and its type chosen from a fixed vocabulary as required details, and optionally the manufacturer, model and serial number, when it was prescribed, when it was last serviced, when its next service is due, instructions for use, whether it is in use, on hold, retired or cancelled, the supplier and the prescribing practitioner.
- **FR-049**: The system MUST allow equipment whose service is overdue or falls due within the next thirty days to be listed, and MUST state for each row which of the two it is.

#### Emergency contacts

- **FR-050**: The system MUST record for an emergency contact a name, a relationship to the person and a telephone number as required details, and optionally a second number, an email address, an address, whether they are the primary contact and whether the contact is still current.
- **FR-051**: The system MUST allow at most one contact per person to be marked primary, MUST explain the change when marking a second displaces the first, and MUST present current contacts before contacts no longer current, primary first.

#### Family history

- **FR-052**: The system MUST record for a relative a name and a relationship chosen from a fixed vocabulary as required details, and optionally their sex, year of birth, year of death and whether they have died, filed against the person whose family it is.
- **FR-053**: The system MUST record against a relative any number of conditions, each with its own name, clinical code, age at diagnosis, severity, status and notes.
- **FR-054**: The system MUST refuse a year of death earlier than a year of birth and MUST refuse either year outside a documented plausible range.

#### Relationships between records

- **FR-055**: The system MUST allow the relationships this specification names to be created and removed from either of the two records involved, and MUST present the relationship on both without the account holder recording it twice.
- **FR-056**: The system MUST allow one record to be related to several records of the same type where the domain allows it, and MUST refuse a duplicate relationship between the same two records.
- **FR-057**: The system MUST refuse any relationship between records filed against different people, and MUST disclose nothing about the other record in that refusal.
- **FR-058**: The system MUST leave the other record intact when a related record is deleted, removing only the relationship, and MUST never present a relationship to a record that no longer exists.
- **FR-059**: The system MUST show, for every related record listed on a detail screen, what type it is and enough of its identifying detail to be recognised, and MUST allow the reader to open it directly.
- **FR-060**: The system MUST allow a medication attached to a course of treatment to carry course-specific dose, frequency, duration, timing, prescriber, dispensing place and start and end dates, MUST fall back to the medication's own values for anything the course does not state, and MUST make clear on screen which values came from the course and which from the medication.
- **FR-061**: The system MUST hold at most one attachment of a given medication to a given course of treatment, and MUST update the existing attachment when the same pair is attached again rather than creating a second.

#### Tags

- **FR-062**: The system MUST allow an account holder to keep their own set of tags, each with a name and a colour, private to that account.
- **FR-063**: The system MUST refuse a second tag with the same name, ignoring letter case, within one account.
- **FR-064**: The system MUST allow any number of tags to be applied to any record of any type in this phase, and to medications recorded earlier.
- **FR-065**: The system MUST apply a rename of a tag to every record carrying it, in one action, losing the tag from none of them.
- **FR-066**: The system MUST state, before a tag is deleted, how many records carry it, and MUST remove it from every one of them on deletion while destroying no record.
- **FR-067**: The system MUST allow any list, the search and the timeline to be narrowed by tag, matching records carrying any of the chosen tags or records carrying all of them, at the account holder's choice.
- **FR-068**: The system MUST offer matching tags as the account holder types, and MUST report how many records carry each tag.

#### Finding records

- **FR-069**: The system MUST offer one search across all of a named person's records of every type, matching the details of each type that carry identifying meaning, together with notes.
- **FR-070**: The system MUST refuse a search that does not name the person being searched, and MUST NOT fall back to the person currently in view.
- **FR-071**: The system MUST allow a search to be narrowed by record type, by tag, by date range and by the state of the record, and MUST show the narrowing in force and allow it to be cleared.
- **FR-072**: The system MUST group search results by record type, page each group independently, and state for each group whether more results exist.
- **FR-073**: The system MUST order results within each group by date, most recent first, with a documented tie-break, and MUST NOT claim to order them by how well they match.
- **FR-074**: The system MUST return only records the requesting account is entitled to see, MUST return nothing for a term that matches only another account's records, and MUST disclose nothing about the existence of such a match.
- **FR-075**: The system MUST NOT write a search term into any log, measurement, trace or error report.

#### The timeline and the current picture

- **FR-076**: The system MUST present one chronological view of a named person's records across every type, each entry stating its type, its identifying summary and its date, ordered by date with a documented tie-break.
- **FR-077**: The system MUST allow the timeline to be narrowed by record type, by date range and by tag, and MUST state what an entry without a date is rather than placing it arbitrarily at either extreme.
- **FR-078**: The system MUST present views of what is currently true for a person — conditions that are active, medications and courses of treatment currently taken, procedures scheduled, injuries not yet healed, allergies that are current, equipment due or overdue for service and cover about to lapse — and MUST state in each view the basis on which a record was selected.
- **FR-079**: The system MUST express every such view as a narrowing of that type's own list, so that any of them can equally be reached by narrowing the list by hand and no view returns rows the equivalent narrowing would not.
- **FR-080**: The system MUST present a helpful empty state, not a blank screen, a row of zeros or an error, wherever a person has nothing to show in a status view or the timeline.

#### Privacy, permission and accountability

- **FR-081**: The system MUST authorize every read and every write of any record in this phase against the authenticated account and the person the record is filed against, deciding from the ownership recorded on the data itself and never from any value supplied by the caller, and never from the person currently in view.
- **FR-082**: The system MUST answer any request for a record the account may not reach exactly as though it did not exist — whether the record is reached directly, through a relationship on another record, through a tag, through search, through the timeline or through a status view.
- **FR-083**: The system MUST refuse every request for person-specific information from anyone not signed in, and MUST include no patient information in that refusal.
- **FR-084**: The system MUST record in the activity trail every creation, correction and deletion of every record type in this phase, every relationship created or removed, every tag applied or removed, and every refused attempt to reach a record — storing who acted, what they did, which person and which record by opaque identifier, and when.
- **FR-085**: The system MUST NOT store in the activity trail any detail of what a record contains — no diagnosis, no medication name, no measurement, no member number, no note, no tag name and no search term.
- **FR-086**: The system MUST NOT write any clinical or identifying content into logs, measurements, traces or error reports; records and people MUST be referred to there by opaque identifier only.
- **FR-087**: The system MUST destroy every record of every type in this phase, and every relationship and tag application among them, when the person they are filed against is deleted, leaving nothing attributed to a person who no longer exists.
- **FR-088**: The system MUST send nothing about a person outside the installation except where the operator has explicitly configured it.

#### Scale

- **FR-089**: The system MUST keep every list, search, status view and timeline in this phase correct and responsive, within the times stated in the success criteria, for a person holding fifty thousand records spread across every type.
- **FR-090**: The system MUST keep the per-symptom episode counts, the per-kind counts on a person's chart and the tag usage counts correct at that scale, without any of them being maintained by hand.

#### Verification

- **FR-091**: Every acceptance scenario in this specification MUST exist as an automated test, and the phase MUST NOT be considered complete until every one of them passes.
- **FR-092**: Every record type in this phase MUST carry automated tests proving that an account which does not own the person is refused on reading, listing, creating, correcting, deleting, relating, tagging and searching it, in addition to tests proving the owning account succeeds.
- **FR-093**: Every user-facing screen this phase adds MUST be covered by the project's automated browser check at both the desktop and mobile sizes it defines, on an installation seeded so that some of those screens are empty, and a screen added without such coverage MUST fail the build.
- **FR-094**: The privacy rules in this section MUST be verified by an automated exercise of every operation this phase defines, which then inspects the installation's diagnostic output and finds no clinical or identifying content in it.

### Key Entities

- **Allergy**: Something a person reacts badly to, with what the reaction is, how severe, whether it is still current and when it was first noticed. May concern several medications.
- **Condition**: Something a person has been diagnosed with, with its status, severity, onset and resolution, and the clinical codes a clinician gave it. The hub of the clinical picture: medications, encounters, procedures, treatments, symptoms and injuries all point at it.
- **Encounter**: An occasion on which a person saw a clinician, with why they went, what kind of visit it was, what was concluded, what was planned and what follows. Points at a practitioner, a place of care and a condition.
- **Procedure**: Something done to a person on a date, with its status from ordered through completed, how it turned out, where and under what anaesthesia, and the condition or injury it arose from.
- **Course of treatment**: An ongoing therapy with a start, an end, a frequency, a dose, an intended outcome and a status. Refers to the encounters that reviewed it, the equipment it uses and the medications it involves.
- **Course medication**: The attachment of one medication to one course of treatment, carrying the dose, frequency, duration, timing, prescriber, dispensing place and dates that apply to that course specifically, and falling back to the medication's own values where it says nothing. At most one per medication per course.
- **Symptom episode**: One occurrence of a symptom — what, when, how severe, how long, where, what triggered it, what relieved it and how much it interfered. Repeated occurrences are repeated episodes; how often and how recently are derived from them, never maintained.
- **Measurement set**: Readings taken at one time — blood pressure, heart rate, breathing rate, temperature, oxygen saturation, weight, height, glucose, pain — each within a documented plausible range, recorded canonically and presented in the reader's preferred units.
- **Vaccination**: Something given to a person on a date to prevent disease, with dose number, batch, manufacturer, site and route.
- **Injury**: Harm to a part of a person's body, with its type from a fixed vocabulary, which side, how it happened, how severe and whether it has healed. Points at the conditions, medications, procedures and treatments that followed.
- **Insurance policy**: Cover a person holds, with the insurer, member details, dates of cover, status, whether it is primary, the amounts that apply at the point of care and the insurer's contacts. Its member and group numbers are identifying content.
- **Medical equipment**: A device a person depends on, with make, model, serial number, when it was prescribed, when it was serviced, when service is next due, and who supplies it.
- **Emergency contact**: Somebody to call about a person, with a relationship, numbers, an address, whether they are primary and whether the contact is still current.
- **Relative (family history)**: A member of a person's family, with a relationship, years of birth and death, and the conditions they had, each with an age at diagnosis. Recorded for its clinical bearing on the person.
- **Tag**: A word an account holder applies to records of any type to group them their own way. Owned by the account, unique within it ignoring letter case, carrying a colour. Renaming it changes it everywhere; deleting it destroys no record.
- **Relationship**: A stated connection between two records of the same person — a condition treated by a medication, a symptom attributed to a condition, a procedure arising from an injury. Visible and editable from both ends, carrying no content of its own except where a course of treatment attaches a medication, and carrying a role where a medication is linked to a symptom.

## Success Criteria *(mandatory)*

### Measurable Outcomes

A criterion marked **[outcome metric]** is observed on a real person rather than built: it maps to
no task by design, and it says so here rather than being left silently unmapped. Every other
criterion below maps either to a task in `tasks.md` or to a phase-exit criterion in `plan.md`, so
an unmapped one that carries no marker is a gap.

- **SC-001** *[outcome metric]*: An account holder who has used the application before can record a first record of any of the twelve types in under 90 seconds each, without documentation or assistance.
- **SC-002**: For a person holding 50,000 records spread across every type, any list page, any status view and the cross-type timeline appear within 2 seconds of being requested.
- **SC-003**: For that same person, a search across their whole record returns its first page of grouped results within 3 seconds, and 100% of the record types that contain a match are represented in those groups.
- **SC-004**: 100% of attempts by one account to reach another account's record of any type — directly, through a relationship, through a tag, through search, through the timeline or through a status view — are refused with a response indistinguishable from the record not existing, and 0 of those refusals disclose that a match exists.
- **SC-005**: Deleting a person removes 100% of their records of every type in this phase together with their relationships and tag applications, and leaves 0 records attributed to a person who no longer exists.
- **SC-006**: Deleting a record that is related to others leaves 100% of those others intact and 0 relationships pointing at something that no longer exists.
- **SC-007**: Renaming a tag carried by 500 records across at least eight types is reflected on 100% of them in one action, and 0 of them lose the tag; deleting that tag removes it from 100% of them and destroys 0 records.
- **SC-008**: 100% of measurements submitted outside the documented plausible range are refused with the accepted range stated, and 0 of them are stored.
- **SC-009**: 100% of corrections based on a version that has since changed are refused with the current values shown, and 0 changes are silently overwritten.
- **SC-010**: A carer opening a person's chart can answer what they are allergic to, what conditions they live with, what they are currently taking and who to call, in under 30 seconds and without leaving the chart.
- **SC-011**: 100% of creations, corrections, deletions, relationship changes, tag changes and refused access attempts across every type produce an activity-trail entry, and 0 of those entries contain a diagnosis, a measurement, a member number, a note, a tag name or a search term.
- **SC-012**: Across an automated exercise of every operation this phase defines, 0 clinical values, names, member numbers, tags or search terms appear in the installation's logs, measurements, traces or error reports.
- **SC-013**: 100% of the acceptance scenarios in this specification exist as automated tests and pass. This phase is not complete while any one of them is missing or failing, and every record type additionally carries automated tests proving a non-owning account is refused on every operation.
- **SC-014**: 100% of the user-facing screens this phase adds pass the project's automated browser check at both the desktop and mobile sizes, with zero browser console errors, zero uncaught page failures and zero failed resource requests, on a seeded installation where several of those screens have nothing to show.
- **SC-015**: Adding a record type or a screen without adding it to the automated browser check fails the build 100% of the time.
- **SC-016**: Recording the same symptom on 20 occasions produces 20 episodes, and the derived count and most recent date are correct on 100% of readings, including immediately after an episode is deleted.
- **SC-017**: A change made to any record type in one open view appears in another open view of the same list within 5 seconds without a manual refresh, and a view left open for 60 continuous minutes is still receiving updates.
- **SC-018**: A person using only a keyboard can complete recording, correcting, relating, tagging and deleting a record of every type, at both viewports, without losing sight of where the focus is.

## Assumptions

### What earlier phases already delivered, and is not re-specified here

- **Phase 001 (Walking Skeleton)** provides the running application, its configuration, its diagnostics, its activity trail, the application shell and its navigation landmarks, and the automated browser check together with the route inventory that check is derived from. This phase adds screens to that check; it does not restate how the check works.
- **Phase 001** provides accounts, sign-in and sessions, and the rule that a record belongs to somebody and is refused to everybody else. It also delivered medications as the first clinical record type, in full depth, as the pattern every record type in this phase repeats. Medications are not re-specified here; they are related to, tagged and searched by this phase like every other type.
- **Phase 001** provides live updating of record lists. Every list this phase adds joins that mechanism rather than inventing its own.
- **Phase 002 (Patient Core)** provides people, ownership, and the person currently in view, together with the rule that the person in view is never a basis for permission and that every view of a person's records names its person explicitly. Every record type in this phase is filed against a person under exactly those rules.
- **Phase 002** provides the chart summary with its per-kind counts and recent activity; this phase feeds those counters with twelve more types rather than building a second set.
- **Phase 002** provides the directories of practitioners and places of care, and the rule that deleting a directory entry clears the reference and preserves the record. Every reference to a prescriber, a clinician, a hospital, a laboratory, a pharmacy or a supplier in this phase is a reference into those directories.
- **Phase 002** provides the account holder's preferred unit system and the rule that measurements are recorded canonically and converted for display. This phase's measurements follow it.

### Decisions taken here rather than deferred

- **A symptom is an episode, not a definition with occurrences underneath it.** Upstream modelled a symptom as a header record with child occurrences, which is the only two-level model in the application and which obliges the account holder to define something before recording it. Recording an episode directly is what a person actually does, and "how often" and "how recently" are derived from the episodes rather than maintained as stored counts that can quietly go stale.
- **Injury types are a fixed vocabulary, not a list users extend.** This follows the decision already taken for medical specialties in phase 002, for the same reason: a reference list that can only be appended to is not a reference model. The vocabulary includes a catch-all so no injury is unrecordable.
- **A vaccination records what was given by name.** A standardised vaccine library — which would let the application state which diseases a person is covered against — is deliberately not part of this phase; nothing in this phase's requirements needs it, and adding it here would be work done ahead of a requirement.
- **One vocabulary per idea, across every type.** One severity ladder serves allergies, conditions, injuries and symptoms; one ladder serves anything that is still going on or has resolved; one serves a course of therapy; one serves an ordered event from ordered to cancelled. Four ladders instead of a dozen near-duplicates is what makes a cross-type timeline and a cross-type status view possible at all.
- **Relationships between records carry no content, with one exception.** Attaching a medication to a course of treatment carries the dose and prescribing detail that apply to that course specifically, because that genuinely differs from the medication's own. Every other relationship is a plain statement that two records concern each other, and a relationship between a medication and a symptom additionally states whether the medication treats the symptom or is suspected of causing it, because patient safety turns on the difference.
- **A relationship is one thing, editable from both ends.** Recording it from the condition and recording it from the medication are the same act, and the application never asks for it twice or shows two versions of it.
- **Records may only be related within one person.** A relationship spanning two people has no clinical meaning here and would create a route by which one person's chart discloses another's.
- **Tags belong to the account, not to a person.** An account holder organises their whole household's care with one set of tags; a shared installation never discloses one household's tags to another.
- **Search orders by date, not by how well a term matches.** The application does not claim relevance ranking it cannot demonstrate; results are grouped by type, most recent first, with a documented tie-break, which is predictable and testable.
- **Search must name its person.** There is no search across every person an account can reach. Each search answers a question about one person, which keeps the permission decision to a single subject and keeps results comprehensible.
- **Status views are narrowings, not separate reports.** "Active conditions" is the condition list narrowed by status, reachable equally by narrowing it by hand. Upstream shipped fifteen bespoke views that returned an ordinary record and left the reader unable to see why it had been selected; here every view states its basis.
- **Records are deleted permanently.** There is no recycle bin for a clinical record. Only attached files are recoverable, for a documented window, and files arrive in phase 004.
- **Family history is specified in this phase**, because the shared design contract allocates it here and because a pedigree has to exist before it can be shared. Its sharing is phase 005's. This phase's charter enumerates twelve record types and does not name family history; it is included at the lowest priority so that no phase is left assuming another one built it.
- **The interface is English-only in this phase**, as in the phases before it; the language preference continues to be honoured for date and number presentation.

### What later phases will change about what is specified here

- **Phase 004** adds laboratory results with their components, and file attachments. Laboratory results become another record type filed against a person under exactly the rules in this specification, and they take part in the relationships this phase defines: an encounter refers to the results ordered at it, and a course of treatment to the results that monitor it. Those two relationships are specified here as belonging to the record types in this phase, and are completed when laboratory results exist. Attachments will hang from records of every type in this phase; nothing here changes to accommodate them.
- **Phase 004** extends the search this phase introduces to cover laboratory results and the documents attached to records. Its grouping, its narrowing, its ordering and its permission rules are the ones specified here; the later phase adds types to it rather than replacing it.
- **Phase 005** adds sharing a person's chart with another account, and sharing family history with a relative. Everywhere this specification says the owning account, that widens to the owning account plus accounts granted access, at a stated level. Every rule here is written so that widening is additive: permission will still never be inferred from the person in view or from anything the caller supplies, a reader granted only viewing access will still not be able to correct or delete, deletion will remain the owner's, relationships will still be refused across people, and a shared reader's search, timeline and status views will still return only what they are entitled to see.
- **Phase 005** adds notifications. Nothing in this phase depends on them, and the reminder details recorded against a medication in phase 001 remain unacted upon until they exist.
- **Phase 006** adds reporting, export and the operator surface. Its summaries and trends read the counts and the status narrowings this phase defines rather than computing their own; its export carries every record type in this phase in a documented portable format; and its audit reader reads the activity entries this phase writes.
- **The operator's break-glass administrative access** can reach every record on the installation by design, is protected by multi-factor authentication and an address allowlist, and every such session is recorded. It is not a route by which one account holder reaches another's records.
