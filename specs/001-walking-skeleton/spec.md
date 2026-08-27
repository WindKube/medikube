# Feature Specification: Walking Skeleton

**Feature Branch**: `001-walking-skeleton`

**Created**: 2026-08-26

**Status**: Draft

**Input**: User description: "Phase 001 of MediKube. One clinical record type works end to end — from stored data through to a rendered page and a passing browser check in continuous integration — establishing every architectural seam the later phases copy. In scope: account sign-up, sign-in, sign-out, password change, self-service password recovery by email, email confirmation, session handling and account deletion; a signed-in person's own profile and preferences; medications with list, view, create, edit and delete; the application shell (navigation, layout, error pages, empty states, light and dark); configuration from the environment; operational logging, measurements, tracing and error reporting; liveness and readiness signals; the operator command surface; the published description of the public interface and the address-inventory gate; the automated browser check; continuous integration wired end to end; and the repository-level registration that lets the deployable image build."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Keep an accurate medication list (Priority: P1)

A person who takes medication needs one trustworthy place to write down what they take, how much, how often, why, since when, and whether they are still taking it — so that they can answer a doctor, a pharmacist or an emergency responder without relying on memory or a shoebox of packaging. They sign in, see everything they take, add a new medication in under a minute, correct a dose that changed, mark one as stopped, and remove one recorded by mistake.

**Why this priority**: This is the reason the product exists, reduced to its smallest honest form. It is the only story in this phase that delivers standalone value to a person, and it is the slice that exercises every layer of the system at once — stored data, business rules, the programmatic interface, the rendered page and the automated checks. If only this story ships, MediKube is already a useful personal medication list.

**Independent Test**: Sign in as the seeded demo account, open the medications page, add a medication with name, dose, frequency and start date, confirm it appears in the list, open its detail view, change its dose, mark it stopped, then delete it and confirm it is gone from the list. Fully testable with no other story implemented beyond the ability to sign in.

**Acceptance Scenarios**:

1. **Given** a signed-in person with no medications recorded, **When** they open their medication list, **Then** they see an explanation that nothing is recorded yet and a clear action to add the first medication, presented inside the same page structure as a populated list.
2. **Given** a signed-in person on their medication list, **When** they add a medication supplying only a name, **Then** the medication is saved, appears at the top of the list, and every field they left blank is simply absent from the detail view rather than shown as an empty value.
3. **Given** a signed-in person adding a medication, **When** they supply a name, kind, dose, frequency, route, reason, start date and notes, **Then** all of those values are stored and shown back to them exactly as entered.
4. **Given** a signed-in person adding a medication, **When** they supply an end date earlier than the start date and leave the name blank, **Then** the save is refused, both problems are reported at once next to the fields they belong to, and everything else they typed is still on the form.
5. **Given** a medication recorded as currently being taken, **When** the person marks it stopped and records why in the notes, **Then** the list shows it as stopped and the detail view shows the recorded reason and the date it was last changed.
6. **Given** a person with a thousand recorded medications, **When** they filter the list by a word in the name and sort by most recently started, **Then** only matching entries are shown, in the requested order, page by page, and moving between pages never repeats or skips an entry.
7. **Given** a person viewing a medication they intend to remove, **When** they choose to delete it, **Then** they must confirm an action that names the medication and warns that it cannot be undone, and only then is it permanently removed from the list.
8. **Given** the same medication list open in two places at once, **When** a medication is added in one, **Then** the other shows it without the person refreshing the page, and a list left open for an hour is still receiving those updates.
9. **Given** one person has the same medication open for editing in two places at once, **When** they save in the second place after having saved in the first, **Then** the second save is refused with a plain explanation that the record changed underneath it, and the current values are shown so they can decide what to do.

---

### User Story 2 - Own and control my account (Priority: P2)

A person needs an account of their own on this instance, needs to be able to get out of it, change its password, recover it when they have forgotten that password, confirm the address it is registered to, set how the application presents information to them, and — because this is their medical history — needs to be able to destroy it completely and be certain nothing is left behind.

**Why this priority**: Story 1 assumes a person is signed in. This story is what makes that true for anyone other than the seeded demo account, and it carries the destructive operation that a medical-records product must get right the first time. It is second rather than first because a single seeded account is enough to demonstrate and test Story 1.

**Independent Test**: From a signed-out browser, create a new account, sign in with it, change the display name and the appearance preference, change the password, confirm the old password no longer works and the new one does, ask for a recovery message and use the link in it to set a password without knowing the old one, then delete the account and confirm the credentials no longer sign in and nothing recorded under them can be reached.

**Acceptance Scenarios**:

1. **Given** an instance where the operator has opened self-registration, **When** a visitor supplies an email address, a display name and a password that meets the published rules, **Then** an account is created and they are signed in.
2. **Given** an instance where the operator has closed self-registration, **When** a visitor opens the sign-up page, **Then** they are told plainly that sign-up is closed on this instance, the page is otherwise a normal page of the application, and any attempt to create an account anyway is refused.
3. **Given** an email address that already has an account, **When** somebody tries to register it again, **Then** the attempt is refused and no second or partially created account exists afterwards.
4. **Given** a registered person at the sign-in page, **When** they supply an unknown email address, and separately when they supply a known address with the wrong password, **Then** they receive the same refusal message in both cases.
5. **Given** a signed-in person, **When** they sign out, **Then** they are returned to the sign-in page and the ended session cannot be used again from any other place it was still open.
6. **Given** a signed-in person, **When** they change their password by supplying the current one, **Then** the change succeeds, they remain signed in where they made the change, and any other session opened before the change stops working.
7. **Given** a signed-in person, **When** they attempt to change their own permission tier or account status through sign-up or through their profile, **Then** the attempt has no effect.
8. **Given** a signed-in person with medications recorded, **When** they delete their account after re-entering their password and typing the confirmation phrase, **Then** the account and every medication recorded under it are permanently removed, and their credentials no longer sign in.
9. **Given** a person who cannot remember their password, **When** they ask for a recovery message for their address, **Then** they are told a message has been sent if that address has an account — in wording identical to the wording used for an address that has none — and a message carrying a single-use, time-limited link is sent only when the account exists.
10. **Given** a valid recovery link, **When** the person opens it and chooses a password that meets the published rules, **Then** the password is replaced without the old one being supplied, every session issued before the change stops working, and they can sign in with the new password.
11. **Given** a recovery or confirmation link that has expired, has already been used, or has been tampered with, **When** the person opens it, **Then** they are shown an ordinary page of the application explaining that the link is no longer usable and offering to send another, rather than an error page or a blank screen.
12. **Given** an instance on which the operator has not configured outgoing mail, **When** a person asks for a recovery or confirmation message, **Then** they are told plainly that this instance cannot send mail and who to ask, the request is not silently accepted, and the operator has been warned about the same condition at every start-up.

---

### User Story 3 - My records are mine alone (Priority: P3)

A person putting their medication history into a self-hosted application needs to know that nobody else using the same instance can read it, that a mistaken or malicious attempt to reach it is refused and leaves a trace, and that the instance's own operational record — the logs an operator reads, the measurements they graph, the error reports they receive — never contains what they take or who they are.

**Why this priority**: Privacy is the governing concern of the whole product, but as a shippable slice it depends on Stories 1 and 2 existing to be worth proving. It is stated as its own story because it must be demonstrated and tested deliberately rather than assumed as a side effect of the others.

**Independent Test**: With two seeded accounts each holding medications, sign in as the first and attempt to read, change and delete the second account's medications by every way the application offers, including addressing them directly by identifier. Every attempt must be refused and indistinguishable from asking for something that does not exist. Then exercise every operation in this phase while collecting the instance's logs, measurements, traces and error reports, and confirm none of them contains a name, an email address, a medication name, a dose or any free text.

**Acceptance Scenarios**:

1. **Given** two signed-up people each with medications, **When** the first asks for one of the second's medications by its exact identifier, **Then** the answer is the same one they would get for an identifier that has never existed — no hint that the record exists.
2. **Given** the same two people, **When** the first attempts to change or delete one of the second's medications, **Then** the attempt is refused, nothing is changed, and the refusal is recorded in the audit trail with who attempted it, what they attempted, which record and when — and with no field values.
3. **Given** a signed-out visitor, **When** they request any page or operation other than the sign-in page, the sign-up page and the liveness and readiness signals, **Then** they are refused and told nothing about whether the thing they asked for exists.
4. **Given** an ordinary signed-in person, **When** they look for a general-purpose way to browse or bulk-extract stored data, **Then** none is reachable — clinical data can be reached only through the operations this phase defines.
5. **Given** a full exercise of every operation in this phase, **When** the operator reads the instance's logs, measurements, traces and error reports, **Then** no personal or clinical content appears anywhere in them, and records are referred to only by opaque identifier.
6. **Given** an instance started without second-factor authentication and address restriction configured for the administrative credential, **When** it starts, **Then** it warns loudly and unmistakably in its operational record, and it continues to warn on every restart until both are configured.
7. **Given** somebody signs in to the administrative surface, **When** that session begins, **Then** an audit entry is recorded for it.
8. **Given** an instance with no outbound destinations configured by the operator, **When** it runs and is used, **Then** it makes no outbound network connection of any kind.

---

### User Story 4 - Find my way around without getting lost (Priority: P4)

A person using MediKube on a laptop at home and on a phone in a waiting room needs the same application in both places: consistent navigation, readable in a dark room, honest about failures, and never a blank white page with a technical message on it.

**Why this priority**: The shell is what makes Stories 1 and 2 feel like an application rather than a form. It is fourth because those stories can be demonstrated with a minimal frame, and because every later phase inherits this shell unchanged — getting it right once is worth more than getting it early.

**Independent Test**: Sign in and visit every page of the application at a desktop width and a phone width, navigating only by keyboard; switch the appearance preference to dark, reload, and confirm it is still dark and did not flash light first; then request an address that does not exist and one belonging to somebody else, and confirm both produce a plain-language page inside the normal application frame with no technical detail.

**Acceptance Scenarios**:

1. **Given** any page of the application, **When** it is displayed, **Then** it presents the same frame — a skip-to-content link, a banner with the primary navigation, one main content region, and a footer — and the current location is obvious in the navigation.
2. **Given** any page of the application, **When** it is displayed at a phone width, **Then** all content and all navigation remain reachable and readable without horizontal scrolling.
3. **Given** a signed-in person, **When** they set their appearance preference to dark and then sign in on a different device, **Then** that device also presents the dark appearance, applied from the first moment the page is drawn.
4. **Given** a person requests an address that does not exist, **When** the page is shown, **Then** it explains in plain language that the page was not found, sits inside the normal application frame, and reveals neither the requested address nor any internal detail.
5. **Given** an unexpected failure while handling a request, **When** the person is shown a page, **Then** it says that something went wrong and offers a way onward, gives no technical detail, and includes a short reference the person can quote to an operator.
6. **Given** a person navigating only by keyboard, **When** they move through any page, **Then** every interactive element can be reached and operated, and the focused element is always visibly indicated.
7. **Given** a browser with scripting disabled, **When** a person opens the application, **Then** they are told plainly that MediKube needs a current browser with scripting enabled, rather than being shown a page that silently does nothing.
8. **Given** any action that changes data, **When** it succeeds or fails, **Then** the person is told which, in a way that is also announced to assistive technology, and never left unsure whether their change was saved.

---

### User Story 5 - Run the instance and know it is healthy (Priority: P5)

An operator self-hosting MediKube for their household needs to start it with settings supplied by the environment, be told immediately and clearly when a setting is missing, watch one log stream rather than four, know from an automated check whether the instance is alive and whether it is ready to serve, load a known demo data set, and see exactly what the application serves.

**Why this priority**: Nothing here is visible to a person using the application, but every later phase depends on it and it cannot be retrofitted cheaply. It is fifth because Stories 1–4 can be demonstrated by a developer running the application by hand.

**Independent Test**: Start the instance against an empty storage location with only the required settings supplied; confirm it creates what it needs and serves. Remove a required setting and confirm it refuses to start with a message naming that setting. Query the liveness and readiness signals with storage available and again with storage unavailable. Run each operator command and confirm it does what it says. Read the log stream and confirm one entry per meaningful occurrence, each carrying a correlation identifier.

**Acceptance Scenarios**:

1. **Given** an operator with the documented settings supplied by the environment and an empty storage location, **When** they start the instance, **Then** it creates everything it needs, brings its stored-data structure up to date, and serves — with no configuration file and no companion service anywhere.
2. **Given** a required setting missing or invalid, **When** the operator starts the instance, **Then** it refuses to start and says exactly which setting is wrong and what is expected of it.
3. **Given** a running instance, **When** the liveness signal is queried, **Then** it answers about the process only and never touches or reveals stored data; **and when** the readiness signal is queried, **Then** it answers whether storage is reachable and its structure is up to date.
4. **Given** a running instance whose storage has become unreachable, **When** both signals are queried, **Then** liveness still reports alive, readiness reports not ready with a non-revealing reason, and the failure is recorded without exposing any credential.
5. **Given** an operator watching the instance, **When** a person signs in and records a medication, **Then** the operator sees one entry per meaningful occurrence in a single machine-readable log stream, each carrying the same correlation identifier for the whole request, and containing no personal or clinical content.
6. **Given** an operator who has configured no tracing or error-reporting destination, **When** the instance runs, **Then** neither is active; **and given** the operator configures them, **Then** both become active and both are scrubbed of personal and clinical content.
7. **Given** an operator, **When** they run the demo-data command twice against a fresh instance, **Then** they get the identical set of accounts and medications both times.
8. **Given** an operator, **When** they ask the application to list every address it serves and to emit the machine-readable description of its public interface, **Then** both are produced from the running application itself rather than from a hand-maintained list.
9. **Given** a running instance under load, **When** the operator asks it to shut down, **Then** it stops accepting new work, finishes what is in flight within a bounded period, closes storage without loss, and exits.

---

### User Story 6 - Every change proves itself before it ships (Priority: P6)

A maintainer of MediKube needs the build to fail when somebody adds a page without a check for it, changes the public interface without meaning to, breaks a page so that the browser complains, or removes an authorization test. The gates established here are copied by every later phase, so they must be real gates and not decorative ones.

**Why this priority**: This story protects all the others in perpetuity, but it can only be built once there are pages, operations and tests to gate. It is last in priority and first in permanence.

**Independent Test**: Deliberately break a page so that the browser reports an error, and confirm the automated browser check fails. Add a new user-facing page without a check for it, and confirm the build fails. Change the public interface without updating the published description, and confirm the build fails. Restore all three and confirm the build passes.

**Acceptance Scenarios**:

1. **Given** the application, **When** the automated browser check runs against a deterministically seeded instance, **Then** every user-facing page — including pages that require signing in — is visited at a desktop and a phone viewport and asserted to load, to present its expected landmark, and to produce zero browser errors, zero uncaught page failures and zero failed resource requests.
2. **Given** a page that has been deliberately broken so the browser reports an error, **When** the check runs, **Then** it fails.
3. **Given** a new user-facing page added without a corresponding entry in the check, **When** the build runs, **Then** it fails — because the pages under check are derived from the application's own inventory of what it serves.
4. **Given** a change to the public interface, **When** the build runs without the published description being regenerated, **Then** it fails, and when it is regenerated the change appears as a reviewable difference.
5. **Given** every acceptance scenario in this specification, **When** the phase is proposed as complete, **Then** each one exists as an automated test and all of them pass.
6. **Given** any operation that touches clinical data, **When** the tests run, **Then** there is a test proving somebody without permission is refused and a test proving the owner succeeds.
7. **Given** any proposed change to the project, **When** it is submitted, **Then** all of these checks run automatically and any failure prevents it from being merged.
8. **Given** a clean checkout of the repository, **When** the shared image pipeline is run for this project, **Then** a deployable image is produced without manual intervention.

---

### Edge Cases

**Nothing there yet**

- A brand-new account opens the medication list: an explanatory empty state with the action to add the first medication, inside the same page structure as a populated list, so the automated browser check does not go falsely red on a page that is legitimately empty.
- A filter or a search term that matches nothing: a distinct "nothing matched that" state offering to clear the filter, not the same message as "you have not recorded anything yet".
- The very last medication is deleted: the list returns to the empty state without an intermediate broken or blank render.

**Two things happening at once**

- One person edits the same medication in two places at once, or on two devices: the later save is refused, told the record changed underneath it, and shown the current values. (Nothing in this phase lets two different people reach the same medication; phase 005 introduces shared access and this same rule is what will make it safe.)
- One place deletes a medication while another has it open: the second is told it no longer exists and returned to the list rather than shown a failure.
- Two visitors register the same email address simultaneously: exactly one account exists afterwards; the other attempt is refused cleanly.
- A person signs out in one place while another place has a list open: the open place's next action lands on the sign-in page, not on a broken page.
- A session expires while a person is halfway through filling in a medication: they are told plainly, sent to sign in, and returned to where they were afterwards; the half-finished entry is not silently discarded without saying so.
- An account is deleted while one of its live views is open elsewhere: that view stops updating, says so, and lands on the sign-in page.

**Partial and awkward data**

- A medication with only a name: renders correctly everywhere, with unfilled fields absent rather than shown as blanks or placeholders.
- Free text at exactly its documented limit is accepted; one character over is refused with a message naming the field and the limit.
- A start date and an end date on the same day (a single-day course) is valid; an end date before the start date is refused.
- A start date in the future (a course beginning next week) is accepted.
- Names in non-Latin scripts, names containing right-to-left text, and names containing characters that look like markup: stored, displayed and searched correctly, and never interpreted as anything other than text.
- A date-only value shows as the same calendar date regardless of the viewer's time zone or device clock.
- A value outside a published set (an unrecognised kind, route or state) is refused rather than stored as free text.

**Recovery links and outgoing mail**

- A recovery message is requested for an address that has no account: the answer is word-for-word the answer given for an address that does have one, and nothing is sent. Anything else is an account-existence oracle on a medical instance.
- A recovery link is opened twice: the first use works, the second is refused with the same explanation as an expired one.
- A recovery link is opened after the person has already changed their password another way: it is refused, because the link was issued against the credential that has since been replaced.
- Recovery is requested repeatedly for one address: the requests are slowed, the answer never changes, and the audit trail carries the refusals without carrying the address.
- Outgoing mail is unconfigured or unreachable: the person is told the instance cannot send mail rather than being told a message is on its way, the failure is recorded once, and the operator has been warned at every start-up since the instance came up.
- A person opens a confirmation link for an address they have since changed: it is refused, and nothing about the account is disclosed.

**Permission boundaries**

- Guessing or enumerating another person's medication identifiers yields the same answer as identifiers that never existed.
- Reaching a signed-in page while signed out lands on the sign-in page and reveals nothing about what was requested.
- An administrative session can see everything by design; every such session leaves an audit entry, and the instance warns at every start until that credential is protected by a second factor and an address restriction.

**Deletion and what depends on it**

- Deleting an account removes every medication recorded under it; this is the only parent-child relationship in this phase, and the removal is proven by looking for the data afterwards, not assumed.
- Nothing in this phase refers to a medication from elsewhere, so a medication can always be deleted. Later phases introduce records that do refer to medications, and each of those phases must state what happens to the reference when the medication is removed.
- Deletion is permanent for records in this phase: no recycle bin, no undo, and the confirmation must say so before the person commits.

**Scale and duration**

- Five thousand medications on one account: the list, its filter, its sort and its paging all remain usable, and no page of the list takes noticeably longer than the first.
- Paging while entries are being added or removed never shows the same entry twice and never skips one.
- A live list left open for an hour is still receiving updates; when updates can no longer be delivered, the person is told rather than left looking at data that has quietly stopped changing.
- Many people signed in at once on one instance never see each other's data, and one person's large list does not degrade another's experience.

**Environment failures**

- Storage unreachable at start-up: readiness reports not ready with a reason that reveals nothing about the storage location or credentials; liveness still reports alive; the failure is logged.
- The stored-data structure has not been brought up to date: readiness reports not ready and names that as the reason.
- The network drops mid-save: the person is told the change was not saved, and their entered values are still on the form.
- Configured tracing or error-reporting destination is unreachable: the application keeps serving; the failure is recorded once, not on every request.

## Requirements *(mandatory)*

### Functional Requirements

#### Accounts, sessions and identity

- **FR-001**: The system MUST allow a visitor to create an account by supplying an email address, a display name and a password, whenever the operator has opened self-registration on that instance.
- **FR-002**: The system MUST allow the operator to open or close self-registration. When closed, the sign-up page MUST render an explanation inside the normal application frame, and every attempt to create an account MUST be refused.
- **FR-003**: The system MUST refuse to create a second account for an email address that already has one, treating addresses that differ only in letter case as the same address, and MUST leave no partially created account behind if any part of account creation fails.
- **FR-004**: The system MUST enforce and publish its password rules — a minimum length of at least 8 characters, and refusal of a password identical to the account's email address or display name.
- **FR-005**: The system MUST allow a registered person to sign in with their email address and password, and MUST answer an unknown address and a wrong password with the same refusal, so that neither can be probed.
- **FR-006**: The system MUST slow or block repeated failed sign-in attempts so that passwords cannot be guessed at speed, and MUST record every failure in the audit trail.
- **FR-007**: The system MUST allow a signed-in person to sign out, after which the ended session MUST NOT be usable again from anywhere it was still open.
- **FR-008**: A session MUST expire after a period the operator configures, after which the person is asked to sign in again and told why.
- **FR-009**: The system MUST allow a signed-in person to change their password only by supplying their current one, and MUST refuse the change when the current password is absent or wrong.
- **FR-010**: After a successful password change, every session issued before the change MUST stop working, while the person who made the change remains signed in where they made it.
- **FR-011**: The system MUST allow a signed-in person to view and change their own display name and their preferences for measurement units, language, date presentation and appearance.
- **FR-012**: The system MUST NOT allow a person to set or change their own permission tier or account status, through sign-up or through editing their profile.
- **FR-013**: The system MUST allow a person to delete their own account, requiring re-entry of their password and an explicitly typed confirmation, and MUST state plainly beforehand that the action cannot be undone.
- **FR-014**: Account deletion MUST permanently remove the account and every medication recorded under it, after which those credentials MUST NOT sign in and none of that person's clinical data MUST remain retrievable anywhere in the system.

#### Recovering an account and confirming its address

*(Added after the first five requirement groups were numbered, so these five carry the next free
numbers rather than renumbering seventy-two requirements that other documents already cite.)*

- **FR-073**: The system MUST allow a person who cannot sign in to request a recovery message for the address on their account, and MUST answer every such request identically whether or not an account exists for that address, so that the request cannot be used to discover who has an account here.
- **FR-074**: A recovery message MUST carry a single-use link that expires after a documented period, and that link MUST allow the person to set a new password meeting the published rules without supplying the old one. Using it MUST end every session issued before the change. A link that has expired, has already been used, or has been altered MUST be refused and explained inside the ordinary application frame, with the offer to request another.
- **FR-075**: The system MUST allow a person to confirm that they control the address their account is registered to, by a link sent to that address, MUST allow that message to be requested again by the signed-in account holder, and MUST show the account holder whether their address is confirmed.
- **FR-076**: When the instance cannot send mail, the system MUST NOT accept a recovery or confirmation request as though it had succeeded: it MUST tell the person plainly that this instance cannot send mail, MUST record the failure once rather than on every attempt, and MUST warn the operator at every start-up while outgoing mail is unconfigured.
- **FR-077**: Recovery and confirmation requests MUST be slowed or blocked when repeated, MUST be recorded in the audit trail by opaque account identifier, and MUST never record the address that was typed, the link, or the secret it carries.

#### Medications

- **FR-015**: The system MUST allow a signed-in person to record a medication with a name, and optionally an alternative name, a kind, a dose, how often it is taken, a route of administration, the reason for taking it, a start date, an end date, its current state, side effects observed, and free-text notes.
- **FR-016**: The system MUST accept for kind, route and state only values from published fixed sets — kind is one of prescription, over-the-counter, supplement or herbal; route is one of a published set of administration routes (by mouth, under the tongue, on the skin, inhaled, injected and the rest); state is one of currently taking, paused, finished, stopped or cancelled — and MUST refuse any other value rather than storing it as free text.
- **FR-017**: The system MUST enforce a documented maximum length on every free-text field of a medication and MUST refuse over-long input with a message naming the field and its limit.
- **FR-018**: The system MUST refuse an end date earlier than its start date and MUST refuse any value that is not a real calendar date; an end date equal to the start date MUST be accepted.
- **FR-019**: Dates that carry no time of day MUST be stored and displayed as calendar dates, showing identically regardless of the viewer's time zone or device clock.
- **FR-020**: The system MUST record for every medication when it was created and when it was last changed, and MUST show the last-changed time on the detail view.
- **FR-021**: The system MUST present a person's medications as a list showing enough of each entry to identify it without opening it — at minimum its name, dose, how often it is taken, its state and its start date.
- **FR-022**: The system MUST allow the list to be ordered by most recently started, by name and by most recently changed, and to be narrowed by state and by a text match against the name.
- **FR-023**: The system MUST page long lists, and paging MUST NOT show the same entry twice nor skip an entry because entries were added or removed while the person was paging.
- **FR-024**: The system MUST provide a detail view containing every value recorded for a medication, omitting fields that were never filled in rather than presenting empty placeholders.
- **FR-025**: The system MUST allow a person to change any recorded field of their own medication, and the change MUST be visible in both the detail view and the list immediately afterwards.
- **FR-026**: The system MUST refuse a save based on a version of a medication that has since changed, explain why in plain language, and present the current values so the person can decide what to do.
- **FR-027**: The system MUST report every validation problem in a single response, each attached to the field it concerns, and MUST preserve what the person typed.
- **FR-028**: The system MUST require an explicit confirmation that names the medication before deleting it, and the deletion MUST be permanent — this phase provides no recycle bin for records and no undo.
- **FR-029**: When a person has no medications recorded, the list MUST present an explanatory empty state offering the action to record the first one, using the same page structure as a populated list.
- **FR-030**: A medication list left open MUST reflect creations, changes and deletions of the entries it shows without the person refreshing the page, and MUST continue doing so for at least one hour of continuous viewing.
- **FR-031**: When a live view can no longer be kept current, the system MUST tell the person plainly rather than continuing to present data that has quietly stopped changing.

#### Privacy, authorization and the audit trail

- **FR-032**: Every read and every change of clinical data MUST be authorized against the signed-in person at the moment of access, and permission MUST NOT be inferred from any identifier supplied by the caller.
- **FR-033**: A request for a medication belonging to somebody else MUST be answered exactly as a request for one that has never existed, so that the existence of a record is never disclosed.
- **FR-034**: An anonymous request for anything other than the sign-in page, the sign-up page, the recovery and confirmation pages and the liveness and readiness signals MUST be refused, and the refusal MUST NOT disclose whether the requested thing exists.
- **FR-035**: The system MUST NOT expose any general-purpose data browsing or bulk-extraction facility to an ordinary signed-in person; clinical data MUST be reachable only through the operations this specification defines.
- **FR-036**: The system MUST record an audit entry for account creation, sign-in, failed sign-in, sign-out, password change — including a password replaced through a recovery link — address confirmation, profile change, account deletion, every medication creation, change and deletion, every refused attempt to reach a record, and every administrative session. Each entry MUST record who acted, what they did, which record it concerned, when it happened and the correlation identifier of the request — and MUST NOT record any field value, name, note, file content or credential.
- **FR-037**: Audit entries MUST NOT be editable or deletable through the application, and MUST be retained for a period the operator configures, after which they are purged automatically.
- **FR-038**: No personal or clinical content MUST appear in the instance's operational record — its logs, measurements, traces or error reports. This includes names, email addresses, medication names, doses, reasons, notes and free text of any kind; records MUST be referred to by opaque identifier only.
- **FR-039**: The system MUST make no outbound network connection that the operator has not explicitly configured: no default telemetry, no externally hosted page content, no update checks.
- **FR-040**: Administrative access MUST use a credential separate from ordinary accounts; the system MUST warn unmistakably at every start-up while second-factor authentication or address restriction for that credential is unconfigured; and every administrative session MUST produce an audit entry.
- **FR-041**: Secrets MUST be supplied by the environment the instance runs in, MUST never appear in the interface, MUST never be written to the operational record, and MUST never be stored in readable form.
- **FR-042**: Every page MUST be delivered with restrictions that prevent it from loading or embedding anything originating outside the instance itself, and that prevent the application from being embedded in another site.

#### The application shell

- **FR-043**: Every page MUST present the same frame: a skip-to-content link, a banner containing the primary navigation, exactly one main content region, a footer, and a region through which changes are announced to assistive technology.
- **FR-044**: Every page MUST be usable at both a desktop and a phone-sized viewport, with all content and navigation reachable without horizontal scrolling.
- **FR-045**: The system MUST offer light, dark and follow-the-device appearance, MUST store the choice on the account so it follows the person to another device, and MUST apply it from the first moment a page is drawn rather than after a visible change.
- **FR-046**: Not-found, not-permitted and unexpected-failure conditions MUST be presented as pages inside the same frame, in plain language, without technical detail, without echoing the requested address, and without any internal error text.
- **FR-047**: Every action that changes data MUST give explicit feedback on success and on failure, announced to assistive technology as well as shown, and MUST never leave the person unsure whether their change was saved.
- **FR-048**: Every interactive element MUST be reachable and operable by keyboard alone, the focused element MUST always be visibly indicated, and every page MUST expose the landmark structure that assistive technology relies on.
- **FR-049**: The interface requires a current browser with scripting enabled; when scripting is unavailable the system MUST say so plainly instead of presenting a page that silently does nothing.
- **FR-050**: Navigation MUST make the person's current location obvious and MUST offer a route back to the medication list and to account settings from every signed-in page.

#### Running the instance

- **FR-051**: All configuration MUST come from the environment the instance is started in, as one documented set of settings with no configuration files and no second mechanism; the instance MUST refuse to start with a message naming the offending setting when a required one is missing or invalid.
- **FR-052**: The instance MUST expose a liveness signal that reflects only whether the process is running and never touches stored data, and a separate readiness signal that reflects whether storage is reachable and its structure is up to date. Neither MUST reveal anything about the data held.
- **FR-053**: The instance MUST emit one machine-readable operational log stream, with one entry per meaningful occurrence, at a level of detail the operator configures.
- **FR-054**: Every request MUST carry a single correlation identifier that appears on every log entry produced while handling it and in any error page or error response the person is shown, so that a report can be tied to the operational record without asking the person for personal detail.
- **FR-055**: Operational measurements MUST be available to the operator on a channel that is not reachable by an ordinary visitor, and their labels MUST be drawn from bounded, published sets — never from an identifier, a name or free text.
- **FR-056**: Tracing and error reporting MUST be inactive unless the operator configures a destination, and when active MUST be scrubbed of personal and clinical content, with request contents excluded.
- **FR-057**: A single occurrence MUST NOT be reported more than once across the log stream, the error-reporting destination and the measurements.
- **FR-058**: The operator MUST have commands to start the service, bring the stored-data structure up to date, load the deterministic demo data set, list every address the application serves, emit the machine-readable description of the public interface, and check the health of a running instance from within its own environment.
- **FR-059**: Changes to the structure of stored data MUST be applied by an explicit, ordered, versioned mechanism, and each step MUST be reversible or MUST carry a written statement of why it is not.
- **FR-060**: The demo data set MUST be deterministic — the same command producing the same accounts and the same medications every time — and MUST include at least two separate accounts holding medications, so that isolation between people can be exercised automatically.
- **FR-061**: The instance MUST run as a single self-contained artefact requiring no companion services, storing everything it holds under one location the operator chooses.
- **FR-062**: The instance MUST shut down cleanly on request: stop accepting new work, finish work in flight within a bounded period, and close storage without loss or corruption.
- **FR-063**: The instance MUST start successfully against an empty storage location by creating everything it needs, and MUST be safe to restart repeatedly without manual intervention.

#### Proving it

- **FR-064**: The application MUST publish a machine-readable description of every operation in its public interface, generated from the application itself, and that description MUST be kept under version control so any change to the interface appears as a reviewable difference.
- **FR-065**: The application's own inventory of the addresses it serves, the published description of its public interface, and the coverage of the automated browser check MUST agree with one another; any disagreement MUST fail the build.
- **FR-066**: Every user-facing page MUST be covered by an automated browser check asserting that the page loads, that its expected landmark is present, and that there are zero browser errors, zero uncaught page failures and zero failed resource requests — at a desktop and a phone viewport, against a deterministically seeded instance, including pages that require signing in.
- **FR-067**: The list of pages under automated browser check MUST be derived from the application's own inventory of what it serves, so that adding a page without adding a check fails the build.
- **FR-068**: Every acceptance scenario in this specification MUST exist as an automated test, and this phase MUST NOT be considered complete until all of them pass.
- **FR-069**: For every operation that touches clinical data there MUST be automated tests proving that a person without permission is refused and that the owner succeeds.
- **FR-070**: All automated checks MUST run automatically on every proposed change, and any failure MUST prevent that change from being merged.
- **FR-071**: The project's deployable image MUST build through the organisation's shared build pipeline from a clean checkout of the repository, and the change that introduces this phase MUST include whatever repository-level registration that build requires.
- **FR-072**: The automated browser check MUST be demonstrated to fail when a page is deliberately broken, before this phase is accepted as complete.

### Key Entities

- **Person (Account)**: Somebody who signs in to this instance. Holds their sign-in identity (an email address), a display name, a password known only to them, their presentation preferences (measurement units, language, date presentation, appearance), a permission tier that only an operator can set, and whether the account is disabled. A person owns everything they record. In this phase a person's records are their own; from phase 002 onward a person owns one or more patients and records hang from the patient rather than from the account.
- **Session**: A person's proven sign-in, valid for a bounded period, endable by the person signing out and invalidated by a password change. A session is what turns an anonymous request into an identified one; it is never the source of permission by itself.
- **Medication**: Something a person takes or has taken. Carries a name and, optionally, an alternative name; what kind of thing it is; how much, how often and by what route it is taken; why it is taken; when it started and when it ended; its current state; side effects observed; and free notes. Belongs to exactly one person, who is the only one who may see or change it. In later phases a medication is additionally linked to a prescriber, a pharmacy, tags, allergies, treatments and attached documents — none of which exist in this phase.
- **Audit Entry**: An immutable record that something happened: who acted, what they did, which record and which person it concerned, when, and the correlation identifier of the request. It never contains the content of any record. It is written by the system, cannot be altered or removed through the application, and is retained for a configured period before being purged. In this phase it is written and readable by an operator through the administrative surface; a later phase adds a reading interface for it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

A criterion marked **[outcome metric]** is observed on a real person rather than built: it maps to
no task by design, and it says so here rather than being left silently unmapped. Every other
criterion below maps either to a task in `tasks.md` or to a phase-exit criterion in `plan.md`, so
an unmapped one that carries no marker is a gap.

- **SC-001** *[outcome metric]*: A person who has never used MediKube can create an account, sign in and record their first medication in under 3 minutes without reading any documentation.
- **SC-002**: A person with 1,000 recorded medications can locate any one of them within 10 seconds using only the list's own ordering and narrowing, and every page of that list is displayed within 2 seconds of being requested.
- **SC-003**: 100% of user-facing pages pass the automated browser check at both a desktop and a phone viewport, with zero browser errors, zero uncaught page failures and zero failed resource requests on every one of them.
- **SC-004**: 100% of the acceptance scenarios in this specification exist as automated tests and pass. This phase is not complete while any one of them is missing or failing.
- **SC-005**: Zero occurrences of personal or clinical content are found in the instance's logs, measurements, traces and error reports after an automated exercise of every operation this phase defines.
- **SC-006**: 100% of attempts by one person to read, change or delete another person's medication are refused, and every refusal is indistinguishable from a request for something that does not exist — verified by automated tests across every way a medication can be addressed.
- **SC-007**: A change made to a medication in one open view appears in another open view of the same list within 5 seconds without any manual refresh, and a view left open for 60 continuous minutes is still receiving updates.
- **SC-008**: An operator who has never run MediKube can go from a clean machine to a reachable, ready instance in under 10 minutes using only the documented settings and one command.
- **SC-009**: Adding a user-facing page without adding it to the automated browser check fails the build 100% of the time — demonstrated by attempting it before this phase is accepted.
- **SC-010**: Deliberately breaking a page so that the browser reports an error causes the automated browser check to fail — demonstrated at least once before this phase is accepted.
- **SC-011**: The published description of the public interface matches the operations the application actually serves at all times; any mismatch fails the build 100% of the time.
- **SC-012**: Deleting an account removes 100% of the medications recorded under it, verified by searching the stored data afterwards and finding none.
- **SC-013**: An operator can determine, in under 30 seconds and using only the liveness and readiness signals, whether an instance is running and whether it is ready to serve — including in the case where storage is unreachable.
- **SC-014**: A person using only a keyboard can complete the whole of recording, editing and deleting a medication, at both viewports, without becoming unable to see where the focus is.
- **SC-015**: A deployable image is produced from a clean checkout of the repository by the shared build pipeline on the first attempt, with no manual step.
- **SC-016**: A person who has forgotten their password can regain access to their own account, on an instance whose operator has configured outgoing mail, without the operator touching the instance — measured end to end in under 5 minutes, and 0% of recovery attempts for an address without an account are distinguishable from attempts for one that has an account.

## Assumptions

**Position in the sequence**

- This is the first phase of MediKube. It assumes no earlier phase and inherits no existing code, data or interface. Nothing here is a migration of an existing system: MediKube is greenfield and does not import data from its predecessor.
- Everything this phase establishes — how records are stored, how permission is decided, how an operation is shaped, how a page is built, how it is tested and gated — is the template that phases 002 through 006 copy. That is why the phase specifies a single clinical record type in full depth rather than several in outline.
- Medications are chosen as that first record type because the shared design contract already uses medications as its worked example of a complete vertical slice. Where that contract's phase table allocates medications to a later phase, this phase's charter governs, and the later phase adds the remaining clinical record types.

**Scope decisions taken here rather than deferred**

- **Self-registration is closed by default.** A self-hosted medical-records instance reachable from the internet must not accept accounts from strangers by default. The operator opens it deliberately. The demo data set used by automated checks configures it open, so the sign-up path is exercised.
- **Passwords require at least 8 characters** and may not equal the email address or display name. This is a floor, not a recommendation; the rule is published to the person at the point of choosing.
- **A session is valid for a period the operator configures, defaulting to 7 days**, after which the person signs in again.
- **Password recovery by email and email confirmation are in this phase, not deferred.** An instance holding somebody's medical history in which a forgotten password can only be recovered by asking whoever runs the machine is broken on the day it ships, and "the operator can reset it" is not an acceptable answer for the person whose own history is locked away. Both flows are recovery of an account the person already proved they own, not new capability, and neither requires anything this phase does not already have.
- **Both flows depend on the operator having configured outgoing mail, and that dependency is stated rather than hidden.** An instance with no mail configured still starts, still signs people in, and still warns its operator at every start-up; what it must never do is tell somebody a recovery message is on its way when nothing was sent.
- **Audit entries are retained for 2 years by default**, configurable by the operator, and purged automatically thereafter.
- **A medication belongs to the signed-in person.** The distinct notion of a patient — a person whose records you keep, who may not be you — arrives in phase 002, which introduces multiple patients per account and switching between them, and which will re-anchor medications from the account holder to a patient. This phase deliberately does not model that, so the seam is introduced once, with real requirements behind it.
- **Records are deleted permanently.** There is no recycle bin and no undo for a medication or an account. Recoverability applies only to attached files, which a later phase introduces.
- **The interface is English-only in this phase.** The language preference is recorded and honoured for date and number presentation, but only English text ships.
- **The instance runs as a single instance.** It is not replicated or horizontally scaled; availability means restarting quickly from a backup.

**Deliberately not in this phase**

- Sign-in through an external identity provider. It depends on the operator having configured a provider, it is a deployment integration rather than a day-one flow, and it belongs with the operator surface that configures it. **Phase 006 owns it**, and this phase leaves external sign-in switched off. Nothing else in this specification depends on it.
- Reminders and notifications for medications. Recording that a medication should prompt somebody is only useful once there is something to deliver the prompt; both arrive together in a later phase.
- A prescriber, a pharmacy and tags on a medication. These are reference data that phase 002 introduces; adding placeholder versions now would be work thrown away.
- Every other clinical record type, patients as entities distinct from the account holder, sharing a record with another person, invitations, attached files, lab results, reporting and export, and the operator dashboard and audit reader. Each belongs to a named later phase and is referenced here only so the reader knows where it went.

**Environment and audience**

- People using MediKube have a current browser with scripting enabled. MediKube does not function without it and says so.
- People self-host MediKube for themselves and their household, on a machine they control, and are willing to supply settings through the environment their instance runs in.
- The administrative surface is the operator's tool for inspecting stored data, taking a backup and, as a last resort, resetting a password for somebody whose address can no longer receive mail — it is not the ordinary recovery path, which this phase gives the person themselves. It ships with the product rather than being rebuilt, and it is treated as a break-glass credential because whoever holds it can read every record on the instance.
- Times are recorded in a single universal time reference and presented in the viewer's local terms; values that are calendar dates carry no time at all and never shift with a time zone.
