# Feature Specification: Patient Core

**Feature Branch**: `002-patient-core`

**Created**: 2026-08-26

**Status**: Draft

**Input**: User description: "A user can maintain records for more than one person — themselves, a child, a parent — switching between them, with every clinical record correctly attributed and isolated. In scope: the person's profile including photo; the ownership model in which one account owns several people, one of whom is the account holder; selecting and switching the person currently in view, and carrying that selection across the application; re-attributing the medications delivered in phase 001 from the account to a person; the person's chart summary with statistics and recent activity; the supporting directories of practitioners, practices, pharmacies and medical specialties; and the rule that an account reaches only the people it owns."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Keep a profile for each person I care for (Priority: P1)

Amara looks after her own health, her seven-year-old son's, and her father's since his stroke. Today the application knows only about her. She adds a profile for her son and one for her father, each with their own name, date of birth, sex, blood type, height and weight, home address and a photograph so she can tell them apart at a glance. Her own profile is already there, marked as hers, without her having to create it. She can correct any of these details later, and nobody else using the same installation can see that these three people exist.

**Why this priority**: Nothing else in this phase — switching, attribution, the chart summary — has a subject until people exist and belong to somebody. On its own it already delivers value: a private, structured place to keep the identifying and baseline health details of a family, which is what a carer reaches for in an emergency room.

**Independent Test**: Sign in as a new account, confirm a profile for the account holder already exists and is marked as theirs, add two more people with full and partial details, correct one of them, and confirm from a second account that none of the three is visible or even discoverable.

**Acceptance Scenarios**:

1. **Given** a signed-in account holder with no additional people recorded, **When** they open the list of people, **Then** exactly one profile is shown, it is identified as the account holder's own, and the option to add another person is offered.
2. **Given** a signed-in account holder, **When** they add a person with a first name, last name and date of birth, **Then** the person is saved, appears in their list of people, and is owned by that account.
3. **Given** a person being added, **When** the date of birth is in the future or more than 150 years ago, **Then** the person is not saved and the account holder is told which field is wrong and why, together with every other invalid field in the same submission.
4. **Given** an account holder who already has a profile marked as their own, **When** they try to mark a second person as their own, **Then** the attempt is refused with an explanation, and the existing marking is unchanged.
5. **Given** a person with a photograph, **When** the account holder replaces it, **Then** the new photograph is shown everywhere that person appears and the previous one is no longer retrievable.
6. **Given** a person whose only recorded details are a name and a date of birth, **When** their profile is displayed, **Then** the missing details are shown as absent rather than as blanks, zeros or errors.
7. **Given** an account holder who opened a person's profile for editing, **When** the same profile has been changed elsewhere in the meantime and they save, **Then** the save is refused, they are shown the current values, and no change is silently overwritten.
8. **Given** a person owned by another account, **When** an account holder tries to open that person by any means, **Then** the response is indistinguishable from the person not existing, no name, date of birth or photograph is revealed, and the attempt is recorded.

---

### User Story 2 - Every clinical record belongs to one person, and only that person (Priority: P2)

Amara's own medications were recorded before her family existed in the application. After this phase they sit under her own profile, exactly where she expects them. When she records her father's blood pressure tablet, it is filed against her father and appears nowhere in her son's or her own list. When she looks at a list of medications, she is never in doubt whose list it is.

**Why this priority**: This is the phase's reason for existing. Attribution changes the shape of every list and every permission check in the application, and it must be settled before eleven more record types are written against it. It is second only because it needs people to attribute to.

**Independent Test**: With medications recorded before the change, confirm every one now appears under the recording account holder's own profile and under no other. Then record a medication against a second person and confirm the two lists never mix, and that a medication cannot be created without naming the person it belongs to.

**Acceptance Scenarios**:

1. **Given** medications recorded before this capability existed, **When** the change is applied, **Then** every one of them is attributed to the recording account holder's own profile, none is lost, duplicated, or left without a person.
2. **Given** an account holder viewing a person's medications, **When** that person has three medications and another owned person has two, **Then** exactly three are listed and none of the other person's appears.
3. **Given** an account holder recording a new medication, **When** no person is identified, **Then** the record is not created and the account holder is asked which person it is for.
4. **Given** a medication belonging to one person, **When** an attempt is made to re-file it under a different person, **Then** the attempt is refused and the medication remains attributed to the original person.
5. **Given** a medication belonging to a person owned by another account, **When** an account holder tries to open, change or delete it, **Then** the response is indistinguishable from the record not existing, and the attempt is recorded.
6. **Given** an account holder viewing any list of a person's records, **When** the screen is displayed, **Then** the person whose records are shown is named on that screen.

---

### User Story 3 - Switch who I am looking at, and have the application follow (Priority: P3)

Amara finishes reviewing her father's medications, chooses her son from the person selector in the top bar, and the application is now about her son — the same way a household streaming account switches profiles. The choice sticks: when she signs in tomorrow on her phone, she is still looking at her son until she changes it.

**Why this priority**: Switching is what makes several people usable rather than merely stored. It sits below attribution because attribution is what makes switching mean anything, and because a person can be chosen explicitly on every screen even before a persistent selection exists.

**Independent Test**: Select a person, navigate across several screens and confirm each shows that person, sign out and back in and confirm the selection survived, then delete the selected person and confirm the application lands on the list of people rather than showing an error or somebody else's data.

**Acceptance Scenarios**:

1. **Given** an account holder with three people, **When** they choose one from the person selector, **Then** every screen that shows person-specific information now shows that person's, and the selector shows their name and photograph.
2. **Given** a chosen person, **When** the account holder signs out and signs in again, possibly on another device, **Then** the same person is still chosen.
3. **Given** a chosen person, **When** that person is deleted, **Then** the selection resolves to nobody and the account holder is taken to their list of people with an explanation, rather than to an error screen or another person's data.
4. **Given** an account holder whose only person is their own profile, **When** they sign in for the first time, **Then** that profile is already chosen and no selection step is required.
5. **Given** any person-specific request, **When** it is handled, **Then** permission is decided from the person that the request itself identifies and never from the current selection, so that changing the selection can never grant access to anything.
6. **Given** an account holder starting a new record while one person is chosen, **When** they change the chosen person in another window before saving, **Then** the record is filed against the person named on the form they submitted, and that person was visible on the form throughout.

---

### User Story 4 - See a person's chart at a glance (Priority: P4)

Before her father's visit, Amara opens his chart. The top of the screen carries his name, age, sex, blood type and current height and weight in the units she prefers, and who his primary practitioner is. Below it she sees how many records of each kind he has, and the last handful of things that changed — a medication added on Tuesday, a detail corrected on Friday — so she can pick up where she left off.

**Why this priority**: It is the payoff screen for the three stories above and the natural landing place after switching, but everything on it is a view over data those stories already produce.

**Independent Test**: Open the chart of a person with records and confirm the demographic header, the per-kind counts and the recent-change list all match the underlying data; then open the chart of a person with no records at all and confirm a helpful empty state rather than zeros, blanks or an error.

**Acceptance Scenarios**:

1. **Given** a person with recorded medications, **When** their chart summary is displayed, **Then** the number of medications shown equals the number of medications attributed to that person and to no other.
2. **Given** a person with no records of any kind, **When** their chart is displayed, **Then** the screen explains that there is nothing recorded yet and offers the first thing to add, and does not appear broken or empty.
3. **Given** an account holder whose preferred units are imperial, **When** they view a person whose height and weight were entered in metric, **Then** the values are shown in imperial and the recorded measurement itself is unchanged.
4. **Given** a person born today, **When** their chart is displayed, **Then** their age is shown meaningfully rather than as zero.
5. **Given** recent changes to a person's records, **When** the recent-activity list is displayed, **Then** each entry states what kind of record changed, what happened to it and when, and entries for records that have since been deleted carry no identifying detail about them.
6. **Given** a person whose records number in the tens of thousands, **When** their chart summary is displayed, **Then** it appears within the time stated in the success criteria and the counts are correct.

---

### User Story 5 - Keep a directory of the clinicians and places involved in care (Priority: P5)

Amara records her father's cardiologist once, along with the practice she works from and the pharmacy that fills his prescriptions. From then on she picks them from a list when recording anything, instead of retyping names and phone numbers. The directory is hers: another account on the same installation has its own.

**Why this priority**: It removes repeated typing and makes later phases' records consistent, but every record type can be captured without it. It is genuinely optional to the phase's core value, which is why it is last.

**Independent Test**: Add a practitioner, a practice and a pharmacy, attach the practitioner to a person as their primary practitioner and to a medication as its prescriber, then delete the practitioner and confirm the medication survives with its prescriber cleared and the account holder warned first.

**Acceptance Scenarios**:

1. **Given** an account holder recording a practitioner, **When** they supply a name and choose a specialty from the offered vocabulary, **Then** the practitioner is saved to their own directory and is offered the next time a practitioner is needed.
2. **Given** an account holder recording a place of care, **When** they choose what kind of place it is — a practice, a pharmacy, a hospital, a laboratory, an imaging centre or something else — **Then** it is saved in one directory and can be filtered by that kind.
3. **Given** a pharmacy chain with two branches, **When** the account holder records the second branch, **Then** it is a second entry with its own address and hours, and both are offered.
4. **Given** a practitioner already in the directory, **When** the account holder tries to add another with the same name and the same specialty, **Then** the duplicate is refused with an explanation.
5. **Given** a practitioner referenced by a person's profile and by several records, **When** the account holder deletes them, **Then** they are warned how many records reference the practitioner, and on confirming, every one of those records survives with the reference cleared.
6. **Given** an account holder searching the directory, **When** they type part of a name, **Then** matching entries from their own directory appear, and entries belonging to other accounts never do.

---

### User Story 6 - Remove a person permanently and deliberately (Priority: P6)

Amara's father dies. Some months later she decides to remove his records from her installation. She is told exactly what will be destroyed, in words, and has to confirm deliberately. Afterwards nothing of his remains — no medications, no photograph, no half-deleted leftovers — and the removal itself is recorded.

**Why this priority**: It is the most destructive operation in the phase and must be specified deliberately, but no user needs it on day one.

**Independent Test**: Create a person with records and a photograph, delete them through the confirmation flow, and confirm that the person, their records and their photograph are all gone, that no record remains pointing at a person who no longer exists, and that the deletion is in the activity trail.

**Acceptance Scenarios**:

1. **Given** a person with records, **When** the account holder asks to delete them, **Then** they are shown the person's name and how many records will be destroyed and must confirm explicitly before anything is removed.
2. **Given** a confirmed deletion, **When** it completes, **Then** the person, every record attributed to them and their photograph are permanently gone, and nothing is recoverable through the application.
3. **Given** a completed deletion, **When** the data is inspected afterwards, **Then** no record remains attributed to the deleted person.
4. **Given** an account holder's own profile, **When** they try to delete it while their account exists, **Then** the deletion is refused and they are told that removing it means closing their account.
5. **Given** a deletion, **When** it completes, **Then** the activity trail contains who did it, what was deleted and when, and contains no names, values or other content from the deleted records.
6. **Given** a person owned by another account, **When** an account holder attempts to delete them, **Then** the attempt fails as though the person did not exist, nothing is deleted, and the attempt is recorded.

---

### Edge Cases

- **A person with almost no details.** Only a name and a date of birth are required. Every screen must render such a person without blank-looking fields, placeholder zeros, or errors, and must not present an absent blood type as an unknown-but-recorded value.
- **Two people with the same name.** Twins, or a father and son sharing a name, are allowed. Wherever people are listed or chosen, the date of birth disambiguates them so the wrong chart is never selected by accident.
- **Simultaneous edits.** Two windows open on the same profile: the second save is refused, the current values are shown, and nothing is silently overwritten. The same rule applies to the records attributed to a person.
- **Simultaneous switching.** Two windows with different people chosen: each screen names the person whose data it is showing, and each request is answered for the person it identifies, so a stale window shows stale data but never the wrong person's data under the right person's name.
- **Deleting the person currently in view somewhere else.** The other window's next action reports that the person no longer exists and returns the account holder to their list of people.
- **Deleting a directory entry that is referenced.** Practitioners and places of care are referenced by profiles and by records. Deleting one never destroys a clinical record; it clears the reference, and the account holder is told how many records are affected before confirming.
- **Photographs that are not photographs.** A file that is not an accepted image type, or an image over the size limit, is refused with a clear explanation and nothing is stored. What the file claims to be is never trusted over what it is.
- **Retrieving a photograph without permission.** A photograph is reachable only by someone entitled to see the person. There is no link that carries its own credential and works for anybody holding it.
- **An empty installation.** A brand-new account has exactly one person and no records. Every screen this phase adds must render correctly in that state.
- **A very large chart.** One person with tens of thousands of records: lists page predictably, counts stay correct, and the chart summary stays within its stated time.
- **A carer with many people.** Twenty-five people in one account: the selector remains usable and a specific person can be found quickly.
- **Paging while data changes.** A person or directory entry added while the account holder is paging through a list must not cause an entry to be shown twice or skipped.
- **Closing an account.** When an account is closed (phase 001), every person it owns, all of their records and all of their photographs go with it, and nothing is left attributed to an account that no longer exists.
- **Attempted access by identifier guessing.** Requesting a person or record by an identifier belonging to another account discloses nothing beyond a uniform not-found response, and is recorded.

## Requirements *(mandatory)*

### Functional Requirements

#### People and ownership

- **FR-001**: The system MUST allow an authenticated account holder to record any number of people, capturing for each a first name, a last name and a date of birth as required details, and optionally sex, blood type, height, weight, home address, relationship to the account holder, and a primary practitioner.
- **FR-002**: The system MUST record exactly one owning account for every person, established when the person is created, and MUST NOT allow that ownership to be changed through any edit offered in this phase.
- **FR-003**: The system MUST reject a date of birth in the future or more than 150 years in the past, and MUST report every invalid field in a submission together rather than one at a time.
- **FR-004**: The system MUST allow at most one person per account to be marked as the account holder themselves, and MUST refuse any attempt to create or mark a second.
- **FR-005**: The system MUST ensure every account has a person representing the account holder: one MUST be created automatically when an account is created, and one MUST be created for every account that already exists when this capability is introduced, so that no account is ever left with nobody to record against.
- **FR-006**: The system MUST derive a person's age from their date of birth whenever it is displayed and MUST NOT store it, so that no chart can display an age that has gone stale.
- **FR-007**: The system MUST accept and display height and weight in the account holder's chosen unit system while recording the measurement in one canonical form, so that changing the display preference never alters what was recorded.
- **FR-008**: The system MUST allow one photograph per person, replaceable and removable, accepting only common photographic image formats up to a documented size limit, determining the file's true type from its content rather than its name or its stated type, and refusing anything else with a clear explanation and without storing it.
- **FR-009**: The system MUST make a person's photograph available in at least one reduced size suitable for lists and the person selector, prepared at the time it is stored rather than on demand.
- **FR-010**: The system MUST list the people an account can reach, identify which one represents the account holder, order the list predictably, and state how many there are.
- **FR-011**: The system MUST refuse a save to a person's profile that is based on a version of that profile which has since changed, MUST tell the account holder what happened, and MUST show them the current values.
- **FR-012**: The system MUST allow a person's identifying and baseline details to be corrected at any time by the owning account, and MUST record that the change happened.

#### Choosing the person in view

- **FR-013**: The system MUST allow an account holder to choose one of the people they can reach as the person currently in view, and MUST remember that choice for their account across screens, sign-outs and devices until they change it.
- **FR-014**: The system MUST present the person currently in view, by name and photograph, on every screen available to a signed-in account holder, and MUST allow switching from any of them.
- **FR-015**: The system MUST NOT use the person currently in view to decide whether anything may be read or written. Every read and every write MUST be authorized against the person that the request itself identifies, and against the authenticated account.
- **FR-016**: The system MUST require that any view of a person's records identifies its person explicitly, and MUST NOT silently fall back to the person currently in view when none is identified.
- **FR-017**: The system MUST resolve the choice to nobody when the chosen person can no longer be reached, MUST take the account holder to their list of people with an explanation, and MUST NOT show another person's data in place of the missing one.
- **FR-018**: The system MUST choose a person automatically when an account holder can reach exactly one, so that a new account never faces an empty application.
- **FR-019**: The system MUST name, on every screen that shows person-specific information, the person whose information is shown.
- **FR-020**: The system MUST authorize the target person before recording a change to the person in view, and MUST record the change in the activity trail.

#### Attributing clinical records

- **FR-021**: The system MUST attribute every clinical record to exactly one person, and MUST make it impossible to create a clinical record that is attributed to nobody.
- **FR-022**: The system MUST attribute every medication recorded before this phase to the person representing its recording account holder, losing none, duplicating none, and leaving none unattributed.
- **FR-023**: The system MUST return, for any list of a person's records, only records attributed to that person.
- **FR-024**: The system MUST fix the person a record is attributed to at the moment the record is created, and MUST refuse any attempt to re-attribute an existing record to a different person.
- **FR-025**: The system MUST show, on any form that creates a record, which person the record will be filed against, MUST default it to the person currently in view, MUST allow it to be changed before saving to any person the account holder owns, and MUST file the record against the person shown on the submitted form regardless of any subsequent change to the person in view.
- **FR-026**: The system MUST remove every clinical record attributed to a person when that person is deleted, leaving no record attributed to a person who no longer exists.

#### The person's chart summary

- **FR-027**: The system MUST present, for a person, a header carrying their name, derived age, sex, blood type, current height and weight in the account holder's units, relationship to the account holder, and primary practitioner, showing absent details as absent.
- **FR-028**: The system MUST present, for a person, a count of their records of each kind, counting only records that person owns and that the viewer is entitled to see.
- **FR-029**: The system MUST present, for a person, their most recent record changes, stating for each what kind of record changed, what happened to it and when, and MUST NOT include any identifying detail of a record that has since been deleted.
- **FR-030**: The system MUST present a helpful empty state, rather than a blank screen, a list of zeros, or an error, wherever a person has nothing recorded yet.
- **FR-031**: The system MUST keep the chart summary correct and available for a person holding a very large number of records, within the time stated in the success criteria.

#### Practitioners, places of care and specialties

- **FR-032**: The system MUST allow an account holder to keep a directory of practitioners, recording for each a name as a required detail and optionally a specialty, an associated place of care, a telephone number, an email address, a website and notes.
- **FR-033**: The system MUST offer a fixed, documented vocabulary of medical specialties which includes a catch-all value, so that no clinician is unrecordable, and MUST NOT allow the vocabulary to be extended by ordinary use.
- **FR-034**: The system MUST allow an account holder to keep a directory of places of care in one list, each classified as a practice, a pharmacy, a hospital, a laboratory, an imaging centre or other, recording name, brand, address, telephone, fax, email, website, patient portal, opening hours, whether it is open around the clock, whether it has a drive-through, the services it offers and notes.
- **FR-035**: The system MUST treat a second location of the same organisation as a separate entry in that directory rather than as a variation of an existing one.
- **FR-036**: The system MUST allow both directories to be searched by name and filtered — practitioners by specialty and by place of care, places of care by kind.
- **FR-037**: The system MUST keep both directories private to the account that created them, and MUST NOT disclose an entry belonging to one account to another account.
- **FR-038**: The system MUST refuse a second practitioner with the same name and the same specialty within one account, and MUST allow places of care to repeat by name because branches of a chain legitimately share one.
- **FR-039**: The system MUST allow a practitioner or a place of care to be chosen wherever one is needed, offering matches from the account holder's own directory as they type, and MUST allow a new entry to be added at that moment without losing the record being written.
- **FR-040**: The system MUST warn an account holder, before they delete a directory entry, how many people and records reference it, MUST preserve every one of those on deletion, and MUST clear only the reference.

#### Privacy, permission and accountability

- **FR-041**: The system MUST authorize every read and every write of a person's profile, photograph or records against the authenticated account, deciding from the ownership recorded on the data itself and never from any value supplied by the caller.
- **FR-042**: The system MUST answer a request for a person, a photograph or a record the account holder may not reach exactly as though it did not exist, disclosing nothing about whether it exists or whom it belongs to.
- **FR-043**: The system MUST refuse any request for person-specific information from someone who is not signed in, and MUST include no patient information in that refusal.
- **FR-044**: The system MUST make a photograph reachable only through a request authorized for the person it belongs to, and MUST NOT provide any link that carries its own credential and grants access to whoever holds it.
- **FR-045**: The system MUST record in the activity trail every creation, change and deletion of a person, every change of photograph, every change to the person in view, and every refused attempt to reach a person or record, storing who acted, what action, which person and record by opaque identifier, and when — and never any name, value, note, file name or other content.
- **FR-046**: The system MUST NOT write names, dates of birth, addresses, photographs, file names, or any clinical detail into operational diagnostics of any kind; people and records MUST be referred to there by opaque identifier only.
- **FR-047**: The system MUST NOT send any information about a person outside the installation except where the operator has explicitly configured it to.

#### Deleting a person

- **FR-048**: The system MUST require an explicit, deliberate confirmation before deleting a person, naming the person and stating how many records will be destroyed.
- **FR-049**: The system MUST make deletion of a person permanent and complete — the person, their records and their photograph — with no recovery path offered in the application.
- **FR-050**: The system MUST allow only the owning account to delete a person.
- **FR-051**: The system MUST refuse deletion of the person representing the account holder while that account exists, and MUST explain that closing the account is what removes it.
- **FR-052**: The system MUST resolve the person in view to nobody when the person in view is the one deleted.

#### Lists and scale

- **FR-053**: The system MUST page every list it presents in this phase in a way that neither repeats nor skips an entry when entries are added or removed while the account holder is paging.

#### Verification

- **FR-054**: Every acceptance scenario in this specification MUST exist as an automated test, and the phase MUST NOT be considered complete until those tests pass.
- **FR-055**: Every capability that touches a person's data MUST have an automated test proving that an account that does not own the person is refused, in addition to a test proving the owner succeeds.
- **FR-056**: Every user-facing screen this phase adds MUST be covered by the project's automated browser check at both the desktop and mobile sizes it defines, and a new screen added without such coverage MUST fail the build.

### Key Entities

- **Person (patient)**: An individual whose health is being recorded — the account holder, a child, a parent. Carries identifying details (names, date of birth, sex), baseline health details (blood type, height, weight), a home address, an optional photograph, a relationship to the account holder, and an optional primary practitioner. Belongs to exactly one owning account. Exactly one person per account is marked as representing the account holder themselves.
- **Account holder (user)**: An existing entity from phase 001, extended here by which person they are currently viewing and by the unit system and display preferences applied when a person's details are shown. Owns zero or more people, always including one representing themselves.
- **Ownership**: The relationship that makes a person reachable by exactly one account. It is the sole basis on which access to that person and everything attributed to them is decided in this phase. Phase 005 widens reachability without changing ownership.
- **Person photograph**: A single image belonging to a person, held with reduced sizes for lists and the person selector, reachable only through an authorized request for that person.
- **Person in view**: The account holder's current choice of which person the application is about. A convenience that shapes what is displayed and what a new record defaults to. It is never a basis for permission, and it resolves to nobody when the chosen person becomes unreachable.
- **Record attribution**: The link that ties every clinical record to exactly one person. Set when the record is created, never changed afterwards, and the anchor on which every permission decision about that record rests.
- **Chart summary**: A derived view of one person — their demographic header, the number of records of each kind, and their most recent record changes. Nothing in it is stored separately from the data it summarises.
- **Practitioner**: A clinician an account holder wants to record and reuse — name, specialty, associated place of care and contact details. Private to the account that recorded them. Referenced by people, as a primary practitioner, and by clinical records.
- **Place of care**: A practice, pharmacy, hospital, laboratory, imaging centre or other location — one concept distinguished by kind, with address, contact details, hours and services. Private to the account that recorded it. A second branch is a second entry.
- **Medical specialty**: A fixed vocabulary of clinical specialties, including a catch-all, from which a practitioner's specialty is chosen. Chosen from, not added to.
- **Activity entry**: An existing entity from phase 001, recording who did what to which person or record and when, by opaque identifier and never by content. Extended here to carry the person a recorded action concerned, which is what makes a per-person recent-activity view possible.

## Success Criteria *(mandatory)*

### Measurable Outcomes

A criterion marked **[outcome metric]** is observed on a real person rather than built: it maps to
no task by design, and it says so here rather than being left silently unmapped. Every other
criterion below maps either to a task in `tasks.md` or to a phase-exit criterion in `plan.md`, so
an unmapped one that carries no marker is a gap.

- **SC-001** *[outcome metric]*: A signed-in account holder can add a second person and record a medication against them in under two minutes, without documentation or assistance.
- **SC-002**: An account holder can switch from any screen to any other person they can reach in no more than two interactions, and the newly chosen person's information is on screen within one second.
- **SC-003**: 100% of screens showing person-specific information name that person on screen.
- **SC-004**: For a person holding 50,000 records, the chart summary — header, per-kind counts and recent changes — appears within two seconds.
- **SC-005**: 100% of attempts to reach a person, photograph or record belonging to another account are refused with a response that is indistinguishable from the subject not existing; 0 attempts disclose a name, a date of birth, a photograph, or the fact that the subject exists.
- **SC-006**: After the transition, 100% of medications recorded before this phase appear under their recording account holder's own profile and under no other, and 0 clinical records exist without a person.
- **SC-007**: Across the phase's automated test run, 0 lists of one person's records contain a record belonging to another person.
- **SC-008**: Across the phase's automated test run, 0 names, dates of birth, addresses, file names or clinical values appear in operational diagnostics of any kind.
- **SC-009**: 100% of person creations, changes, deletions, photograph changes, changes to the person in view, and refused access attempts produce an activity-trail entry, and 0 of those entries contain patient content.
- **SC-010**: Deleting a person removes 100% of their records and their photograph, and leaves 0 records attributed to a person who no longer exists.
- **SC-011**: An account holder managing 25 people can locate and choose a named one within ten seconds.
- **SC-012**: Every acceptance scenario in this specification exists as an automated test and passes; the phase is not complete while any of them is missing or failing. Every capability touching a person's data additionally carries an automated test proving a non-owning account is refused.
- **SC-013**: Every user-facing screen this phase adds passes the project's automated browser check at both the desktop and mobile sizes it defines, with zero console errors, zero uncaught page errors and zero failed requests, on a freshly seeded installation where several of those screens are empty.
- **SC-014**: 100% of the people, practitioners and places of care an account holder created remain reachable after a sign-out and sign-in, and 0 belong to or are visible from any other account.

## Assumptions

### What phase 001 already delivered, and is not re-specified here

- **Foundation (phase 001)** provides the running application, its configuration, its diagnostics, its activity trail, and its automated browser check together with the route inventory that check is derived from. This specification adds screens to that check; it does not restate how the check works.
- **Foundation (phase 001)** provides accounts and authentication: registration, sign-in, sign-out, password change, self-service password recovery by email, confirmation of the address on an account, sessions, and the account holder's own profile and preferences. It does **not** provide sign-in through an external identity provider; that arrives in phase 006 with the operator surface that configures providers, and nothing in this phase depends on it. This phase adds the person-in-view choice and the unit-system preference to what an account holder carries, and adds the automatic creation of a person representing the account holder to registration.
- **Foundation (phase 001)** provides the application shell — its navigation and landmarks — into which the person selector is placed.
- **Foundation (phase 001)** delivered medications as the first clinical record type, attributed to the recording account holder. This phase re-attributes them to people. Any account that recorded medications before this phase keeps every one of them, on the person representing that account holder.
- **Foundation (phase 001)** provides live updating of record lists. Lists this phase makes person-scoped continue to update live, for the person they identify and for nobody else.

### Decisions taken here rather than deferred

- **Practices and pharmacies are one concept, distinguished by kind.** Upstream modelled them as two entities with two different treatments of the same six address ideas. They are one directory of places of care here, which also absorbs hospitals, laboratories and imaging centres, and gives one list, one form and one search instead of six.
- **Medical specialties are a fixed vocabulary, not a user-extensible list.** Upstream made specialty a required reference to a list that could only be appended to and never read back, corrected or pruned, which is not a reference model. A documented vocabulary with a catch-all value is chosen instead, so that a practitioner's specialty is always recordable and always comparable.
- **Practitioners and places of care belong to an account, not to a person and not to the installation.** They are the account holder's own address book. A shared installation therefore does not leak one household's clinicians to another.
- **A record's person is fixed at creation.** Correcting a mis-filed record means deleting it and recording it again. Allowing re-attribution would mean authorizing two people for one write for a case no requirement asks for, and the simpler rule is preferred until one does.
- **Ownership of a person cannot be transferred to another account in this phase.** No requirement in this or any planned phase asks for it; sharing (phase 005) covers the case where somebody else needs access.
- **Measurements are recorded in one canonical unit system and converted for display.** Upstream stored imperial and converted for export, which makes the recorded value depend on the recorder's preference.
- **There is no limit on how many people one account may own**, beyond what the success criteria require to remain usable.
- **Deleting a person is permanent.** Only deleted files are recoverable in MediKube, for a documented window, and files arrive in phase 004. Records are destroyed on confirmation and the destruction is recorded.
- **A person's age, and any measurement shown in a converted unit, are computed when displayed** and never stored, so no chart can show a value that has quietly gone stale.
- **The application requires a modern browser with scripting enabled.** This is structural to how the interface is built and is stated so it is not discovered later.

### What later phases will change about what is specified here

- **Phase 003** adds the remaining clinical record types. Every rule in this specification about attribution, isolation, permission, deletion and the chart summary applies to them unchanged, and adding them requires no change to what is specified here. That is precisely why this phase precedes them.
- **Phase 004** adds laboratory results and file attachments. Attachments are attributed to a person by the same rule as every other record, and deleting a person will remove theirs.
- **Phase 005** adds sharing a person's chart with another account. It widens "the people an account can reach" from "the people it owns" to "the people it owns, plus the people shared with it", and introduces a permission level below ownership. Every requirement here that says "owns" is written so that widening is additive: the person in view will still never be a basis for permission, a person shared at a viewing level will still not be deletable by the recipient, deletion will remain the owner's alone, and a chart summary will still count only what the viewer is entitled to see.
- **Phase 006** adds reporting, export and the operator surface. Its reports build on the same per-kind counters this phase introduces, rather than computing their own, and its audit view reads the activity entries this phase writes.
- **The operator's break-glass administrative access**, specified in phase 001 and hardened by the project's constitution, can reach every person on the installation by design. That is a deliberate operator-level capability, is protected by multi-factor authentication and an address allowlist, and every such session is recorded. It is not a route by which one account holder reaches another's people.
