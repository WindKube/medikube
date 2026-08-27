# Feature Specification: Sharing and Collaboration

**Feature Branch**: `005-sharing-and-collaboration`

**Created**: 2026-08-26

**Status**: Draft

**Input**: User description: "A user can give another user access to a patient record or to family medical history, with explicit permissions, and can take that access away. In scope: a single unified sharing and permission model covering both patient records and family history — upstream has two parallel systems and this application deliberately unifies them; inviting another user, by email address, to share a patient or family history; the invitation lifecycle — sent, pending, accepted, declined, revoked, expired — and its cleanup; permission levels and exactly what each allows; viewing what has been shared with you and what you have shared; revoking access, and removing your own access to something shared with you; family members and their medical history as the shareable family-history resource; and the authorization consequences throughout the application — every existing query and screen from the earlier phases must now honour shared access as well as ownership."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Let somebody I trust see a person's records (Priority: P1)

Amara's father is in and out of hospital and her brother Kwame does half the driving and most of the phone calls. Today only Amara can see their father's chart, so every question comes back to her. She opens her father's record, invites Kwame by his email address to see it, chooses viewing access and writes a line explaining why. Kwame gets an email, follows the link, signs in, sees who invited him and how many things are being shared, and accepts. Their father now appears in Kwame's list of people, plainly marked as shared with him by Amara, and Kwame can read the whole chart — allergies, conditions, medications, results, documents — exactly as Amara sees it, and nothing else that belongs to Amara.

**Why this priority**: This is the whole point of the phase, reduced to its smallest honest form: one person, one recipient, one level, one acceptance. It is the only story that delivers standalone value on its own, and it exercises every part that everything else in this phase builds on — the invitation, the acceptance, the grant, and the widening of every permission decision in the application from "the owner" to "the owner or somebody they let in". If only this story ships, a family can already share the burden of caring for somebody.

**Independent Test**: With two seeded accounts, sign in as the first, invite the second by email address to view one person's chart, sign in as the second, accept the invitation, and confirm that the person appears in the second account's list of people marked as shared, that every screen of that person's chart opens and shows the same content the owner sees, that no other person or record belonging to the first account is reachable, and that a third account can reach none of it.

**Acceptance Scenarios**:

1. **Given** an account holder viewing a person they own, **When** they invite an email address to view that person and add a note, **Then** the invitation is created as pending, the sender is told it has been sent, and no access exists yet.
2. **Given** a pending invitation, **When** the invited person signs in with the invited address and opens their invitations, **Then** they see who invited them, what kind of thing is being shared, how many items, at what level, when the invitation lapses, and the sender's note — and nothing that identifies the person whose records these are and no clinical content.
3. **Given** a pending invitation, **When** the invited person accepts it, **Then** access begins immediately, the shared person appears in their list of people marked as shared and naming who shared it, and the sender sees the invitation as accepted.
4. **Given** an account holder with viewing access to somebody else's person, **When** they open that person's chart, records of every type, documents, search, timeline and status views, **Then** they see exactly what the owner sees for that person, and no record belonging to any other person the owner owns.
5. **Given** an account holder with viewing access, **When** they open their list of people, **Then** the people they own and the people shared with them are both present, are visibly distinguished, each shared one names who shared it and at what level, and each group is counted.
6. **Given** an account with no access to a person, **When** it tries to reach that person or any of their records by any means, **Then** the answer is indistinguishable from the person not existing, and the attempt is recorded.
7. **Given** an account holder who has shared a person, **When** they open the sharing screen, **Then** they see every account that has access to each of their people, at what level, since when, when it lapses and the note they wrote.
8. **Given** an installation where nothing has been shared, **When** either the sharing screen or the invitations screen is opened, **Then** an explanation of what sharing is for and an obvious way to start is shown, inside the same page structure as a populated screen, not a blank screen or an error.
9. **Given** an account holder with viewing access to somebody else's person, **When** they open one of that person's records or download one of their documents, **Then** an accountability entry is recorded naming who did it, what they did, and which person and which record by opaque identifier, and containing no diagnosis, medication, measurement, note or document name.

---

### User Story 2 - Take access away, and be certain it is gone (Priority: P2)

Amara's arrangement with Kwame changes: he moves abroad, or they fall out, or the temporary carer's month is up. She opens the sharing screen, ends Kwame's access, and it is over — the next thing Kwame does in the application, including a screen he already had open, tells him his access has ended and shows him nothing more. Kwame can equally walk away himself: he removes his own access to their father and it disappears from his list. And where Amara set an end date when she shared, the access simply stops on that date without anybody having to remember.

**Why this priority**: Sharing without reliable revocation is worse than no sharing at all, and revocation is the single behaviour in this phase most likely to leak medical records if it is wrong. It comes second only because there must be something to revoke.

**Independent Test**: Grant a second account access to a person, confirm it can read the chart, then end the access from the owner's side and confirm every way to that person — the list, the chart, a record opened directly, a document, a search, a screen left open — is refused as though the person never existed, without the second account signing out and in. Repeat for the grantee ending their own access, and for a grant given an end date a moment in the future.

**Acceptance Scenarios**:

1. **Given** an active grant, **When** the owner ends it, **Then** the grantee's very next request for anything about that person is refused as though the person did not exist, with no sign-out required and no waiting for any periodic job.
2. **Given** a grantee with the shared person's list of records open and updating live, **When** the owner ends their access, **Then** the open screen stops receiving updates within a few seconds and states plainly that access has ended, rather than silently freezing or continuing to show content.
3. **Given** a grantee who has the shared person selected as the person in view, **When** their access ends, **Then** the selection resolves to nothing and they are returned to their own list of people rather than to an error.
4. **Given** an active grant, **When** the grantee removes their own access, **Then** it ends the same way, the shared person disappears from their list, and the owner sees that the grantee left rather than that the owner revoked.
5. **Given** a grant with an end date, **When** that date passes, **Then** access is refused from that moment on every route, whether or not any cleanup has run, and the grant is shown as lapsed to both sides.
6. **Given** ended access, **When** the owner looks at the person's records, **Then** every record, correction and document made by the former grantee is still there and unchanged — ending access removes reach, never content.
7. **Given** a grantee who was part-way through correcting a record, **When** their access ends before they save, **Then** the save is refused as though the record did not exist and nothing is changed.
8. **Given** an owner who ended somebody's access, **When** they later decide to share again, **Then** it takes a fresh invitation the other person must accept; the old grant is never resurrected.

---

### User Story 3 - Let a carer keep the record up to date (Priority: P3)

Amara's mother manages her own medications badly and Amara does it for her, but she needs her mother to be able to add the readings the nurse takes. She shares her mother's chart with editing access. Her mother can now add records, correct them and remove them exactly as Amara can — but she cannot delete the person, cannot change who else has access, and cannot pass access on to anybody else. Where Amara shared only for viewing, the recipient's attempts to change anything are refused with a plain explanation instead of a silent failure.

**Why this priority**: Editing is what turns shared access from a window into shared care, and it is the level at which a mistake is costly. It comes after revocation because a wrongly granted editor is recoverable and a wrongly retained viewer is not.

**Independent Test**: Grant one account viewing access and another editing access to the same person. Confirm the editor can create, correct and delete records of every type and add and remove documents, that the viewer's every attempt to do so is refused with an explanation, and that neither can delete the person, change anybody's access, or share the person onward.

**Acceptance Scenarios**:

1. **Given** an account with editing access to a person, **When** they add, correct or delete a record of any type or a document, **Then** it succeeds under exactly the same rules that apply to the owner, and the owner sees the change.
2. **Given** an account with viewing access, **When** they attempt to add, correct or delete anything about that person, **Then** the attempt is refused, they are told they have viewing access only, and nothing changes.
3. **Given** an account with editing access, **When** they attempt to delete the person, change the person's name or date of birth, alter anybody's access, or share the person with a third account, **Then** every one of those attempts is refused as belonging to the owner alone.
4. **Given** an owner who shared at viewing level, **When** they raise it to editing, **Then** the change takes effect on the grantee's next action without a new invitation, and both sides see the new level.
5. **Given** an owner who shared at editing level, **When** they lower it to viewing, **Then** the grantee's next attempt to change anything is refused, and nothing they changed before is undone.
6. **Given** an editor and the owner with the same record open, **When** both save, **Then** the second save is refused because the record changed underneath it and the current values are shown, exactly as when one person edits in two places.
7. **Given** an editor who deletes a record, **When** the owner looks at the accountability trail, **Then** the deletion is recorded against the editor's account and not against the owner's.

---

### User Story 4 - Share what runs in the family, and only that (Priority: P4)

Amara's cousin Ada is being investigated for the same heart condition their grandmother had. Amara has recorded their grandmother as a relative on her father's record, with the conditions she had and the ages she was diagnosed. She shares that one relative with Ada, with a note explaining why. Ada sees the grandmother's entry and her conditions and nothing else — not Amara's father, not his medications, not another relative, not one clinical record about a living person. Ada cannot change what she has been shown, and the reason Amara gave stays with it.

**Why this priority**: This is the second kind of shareable thing, and the reason the model has to be general rather than patient-only. It is the narrower and less urgent of the two, and it is the one where over-disclosure would be most surprising, so it is specified after the chart-sharing rules it reuses.

**Independent Test**: Record a relative with conditions against a person, share that one relative with a second account, and confirm the second account can read exactly that relative's entry and its conditions, cannot reach the person the entry is filed against, their records, or any other relative, cannot change anything, and sees the sender's note; then end the share and confirm the entry becomes unreachable.

**Acceptance Scenarios**:

1. **Given** a person with three relatives recorded, **When** the owner shares one of them with another account, **Then** the recipient, on accepting, can read that relative's name, relationship, years, whether they have died and every condition recorded for them.
2. **Given** that same share, **When** the recipient tries to reach the person the relative is filed against, that person's records, or either of the other two relatives, **Then** every attempt is answered as though nothing existed.
3. **Given** a family-history share, **When** anybody attempts to create it at editing level, **Then** it is refused: family history is shared for reading only.
4. **Given** a family-history share with a note, **When** the recipient views the relative, **Then** the sender's note and the sender's display name are shown alongside it for as long as the share lasts.
5. **Given** a shared relative, **When** the owner corrects the entry or adds a condition, **Then** the recipient sees the correction, and the recipient's own attempts to correct it are refused.
6. **Given** an owner sharing four relatives with one person in a single invitation, **When** the recipient accepts, **Then** all four become separately listed grants that can be ended one at a time.
7. **Given** a shared relative, **When** the owner deletes that relative, **Then** every grant of it ends, the recipient no longer sees it, and no other grant of theirs is affected.

---

### User Story 5 - Run the invitations, including to somebody with no account yet (Priority: P5)

Amara invites her father's new physiotherapist, who has never used this installation. He receives an email, follows the link, is told who invited him and how many items are being shared but nothing about whom, creates an account with that address, and the invitation is waiting for him when he signs in. Elsewhere, Amara has three invitations she is waiting on: she cancels the one she sent to the wrong address, sees that her aunt declined with a polite note, and watches one to a dormant address lapse after a week. She can also take back an invitation somebody already accepted, which ends the access it created.

**Why this priority**: The lifecycle around the happy path — cancelling, declining, lapsing, inviting a stranger — is what makes sharing usable by a household rather than a demo. It is separable from stories 1 to 4, each of which needs only the accept path.

**Independent Test**: Send four invitations: one to an address with no account, one that is cancelled before it is answered, one that is declined with a note, and one whose lapse date is set to pass during the test. Confirm the first can be accepted after signing up with that address, that the cancelled one's link stops working, that the declined one shows its note to the sender, that the lapsed one can no longer be answered, and that every one of them appears in the sender's and recipient's lists with the right state.

**Acceptance Scenarios**:

1. **Given** an invitation to an address with no account, **When** the recipient follows the emailed link, **Then** they are shown who invited them, what kind of thing, how many items, at what level, when it lapses and the sender's note, and are offered the chance to create an account with that address.
2. **Given** that recipient, **When** they create an account with the invited address and sign in, **Then** the invitation is waiting for them to accept or decline.
3. **Given** somebody who follows an invitation link while signed in as a different address, **When** they attempt to accept, **Then** it is refused as not addressed to them, and nothing is disclosed about who it was for or what it covered.
4. **Given** a pending invitation, **When** the sender cancels it, **Then** the link stops working immediately, the recipient no longer sees it as answerable, and no access is created.
5. **Given** a pending invitation, **When** the recipient declines with a note, **Then** the sender sees it declined with the note and when, and no access is created.
6. **Given** a pending invitation whose lapse date has passed, **When** either side tries to act on it, **Then** it is refused as lapsed and is shown as lapsed to both.
7. **Given** an invitation the recipient already answered, **When** they try to answer it again, **Then** they are told what the answer was and when, and nothing changes.
8. **Given** an accepted invitation, **When** the sender takes it back, **Then** every grant it created ends in one step and both sides see it as withdrawn.
9. **Given** an invitation covering several people, **When** the recipient accepts, **Then** access to all of them begins together, and if any one of them can no longer be shared then none of them is shared and the recipient is told the items are no longer available.
10. **Given** an address that already has active access to a person, **When** the owner invites the same address to the same person again, **Then** the send is refused and the owner is directed to change the existing access instead.

---

### User Story 6 - Know when something changes without watching for it (Priority: P6)

Kwame does not sit in the application waiting. When Amara shares their father with him, a notice appears where he is, without him refreshing. When he accepts, Amara sees it the same way. When Amara later ends his access, he is told rather than left guessing why a screen went empty. None of these notices says anything about a diagnosis, a medication or a person's name.

**Why this priority**: It makes the collaboration feel like collaboration instead of a mailbox, and it removes the worst confusion in story 2 — a screen that empties with no explanation. It is last because nothing else depends on a notice arriving.

**Independent Test**: With two accounts signed in at once, send an invitation and confirm a notice reaches the recipient without a refresh; accept it and confirm a notice reaches the sender; end the access and confirm a notice reaches the former grantee. Leave a session open for an hour and confirm notices are still arriving. Confirm no notice contains a person's name or any clinical detail.

**Acceptance Scenarios**:

1. **Given** two accounts signed in at once, **When** one invites the other, **Then** the recipient is notified within a few seconds without refreshing, and the notice names the sender and the kind of thing only.
2. **Given** a pending invitation, **When** the recipient accepts or declines it, **Then** the sender is notified the same way.
3. **Given** an active grant, **When** the owner changes its level or ends it, **Then** the grantee is notified and told what changed.
4. **Given** a session left open for an hour without interaction, **When** an invitation arrives, **Then** the notice still reaches it.
5. **Given** an account that lost access between an event and its delivery, **When** notices are delivered, **Then** that account receives nothing about it.
6. **Given** any notice, **When** it is examined, **Then** it contains no diagnosis, no medication, no measurement, no document name and no name of the person whose records are concerned.

---

### Edge Cases

- **The owner deletes a person that is shared.** Every grant of that person ends, it disappears from every grantee's list, a grantee looking at it is answered as though it never existed, and a grantee who had it in view is returned to their own list. The owner is told, before confirming, how many people currently have access.
- **The owner deletes their account.** Their people, their relatives and every grant they gave end together; every former grantee's list loses them; nothing is left reachable through a grant to something that no longer exists.
- **The grantee deletes their account.** Every grant to them ends, and the owner's sharing screen no longer lists them.
- **The grantee's account is disabled.** Access is refused for as long as it is disabled and resumes if it is re-enabled; the grant itself is untouched and the owner is not told the account is disabled.
- **The invited address changes.** An account that changes its address after accepting keeps its grants — grants belong to the account, not to the address. A pending invitation is still tied to the address it was sent to.
- **Two people accept the same invitation at once, or one person accepts twice.** Exactly one set of grants exists afterwards; the second attempt is refused as already answered and creates nothing.
- **The owner revokes at the same moment the grantee accepts.** The outcome is one of the two, never a half-applied state: either the acceptance happens and is immediately revoked, or the acceptance is refused, and both sides see a consistent state and one accountability entry per event.
- **Two editors change the same record at once.** The later save is refused because the record changed underneath it and the current values are shown, exactly as specified for one person editing in two places.
- **An invitation covers something that is deleted before it is answered.** The acceptance fails as a whole, the invitation reaches a terminal state, and the recipient is told the items are no longer available without being told what they were.
- **The sender stops owning the resource between sending and acceptance.** The acceptance is refused; ownership is re-checked at the moment access is created, never trusted from the invitation.
- **An end date in the past.** Refused when set, on both a new invitation and a change to an existing grant.
- **Inviting yourself, or an address that is only a different casing of an existing one.** Refused as sharing with yourself; addresses are matched ignoring case.
- **Inviting an address that has no account, on an installation with no outbound email configured.** The send is refused with a plain explanation, because there is no way to reach that person; where the address does have an account the invitation is delivered inside the application and the sender is told that no email could be sent. The operator is warned about the missing configuration at start-up and on the operator screens.
- **An invitation link that leaks.** It is unguessable, it stops working the moment the invitation is answered, cancelled or lapses, and it can only be acted on while signed in as the address it was sent to — so possession of the link alone grants nothing.
- **Repeated attempts to guess whether an address has an account.** Sending an invitation answers identically whether or not the address is known to the installation.
- **A grantee tries to pass access on.** Refused. There is no delegation at any level.
- **A grantee reaches a record through something else it references.** A record of a shared person that refers to a practitioner or a place of care shows that reference as part of the record, but the owner's directories as a whole remain unreachable to the grantee.
- **A shared person's records carry the owner's labels.** The grantee sees the labels applied to records they may see, may not create, rename or delete labels in the owner's set, and — with editing access — may apply or remove only labels that already exist in it. The grantee's own labels stay their own.
- **Empty everything.** A first-time account with nothing shared in either direction, an owner with no invitations sent, and a person with no relatives all present an explanation and an action, never a blank screen, a row of zeros or an error.
- **Very many grants.** An owner with two hundred active grants across twenty people, and a grantee reaching fifty shared people, page and filter both screens without repeating or skipping an entry.
- **A very long-lived stale link.** An invitation answered months ago cannot be reopened by its link, and a terminal invitation never returns to pending by any route.

## Requirements *(mandatory)*

### Functional Requirements

#### One sharing model, two kinds of shareable thing

- **FR-001**: The system MUST provide exactly one sharing mechanism, covering both a person's whole chart and a single relative's family-history entry, with one way to invite, one list of who has access, one way to end access and one accountability trail. It MUST NOT operate two parallel sharing mechanisms for the two kinds.
- **FR-002**: The system MUST support exactly two access levels — viewing and editing — and every grant MUST state exactly one of them. No third level exists, and no grant may carry unnamed or caller-defined permissions.
- **FR-003**: Viewing access to a person MUST allow reading everything about that person that the owner can read: their profile, every clinical record of every type, laboratory results, documents and their contents, the chart summary, search over that person, the timeline and the status views — and nothing else belonging to the owner.
- **FR-004**: Editing access to a person MUST allow everything viewing allows, plus creating, correcting and deleting that person's clinical records and documents, under exactly the validation, confirmation and concurrency rules that apply to the owner.
- **FR-005**: The system MUST reserve to the owner alone, at every access level: deleting the person; changing the person's identifying profile; granting, changing or ending anyone's access to the person; and transferring ownership.
- **FR-006**: The system MUST NOT allow a grantee to share onward at any level. Access is granted only by the owner of the thing being shared.
- **FR-007**: The system MUST refuse any family-history grant at editing level; a relative's entry is shared for reading only.
- **FR-008**: The system MUST allow a grant to carry an end date, MUST treat its absence as open-ended, and MUST refuse an end date that is not in the future at the moment it is set.
- **FR-009**: The system MUST allow the granting account to attach a note to what it shares, and MUST show that note to the grantee for as long as the grant lasts.
- **FR-010**: The system MUST hold at most one active grant of a given thing to a given account, and MUST NOT create a second one alongside it.
- **FR-011**: The system MUST refuse any attempt to share something with the account that owns it.
- **FR-012**: The system MUST decide the access level from the grant recorded on the installation and MUST ignore any level, permission or ownership claimed by the caller in a request.

#### Inviting

- **FR-013**: The system MUST create access only through an invitation the recipient accepts, and MUST provide no way for one account to obtain access to another's data without an explicit acceptance.
- **FR-014**: The system MUST allow an invitation to be addressed to any email address, whether or not it belongs to an existing account on the installation.
- **FR-015**: The system MUST allow one invitation to cover several things of the same kind, all owned by the sender, and MUST have it accepted or declined as a whole.
- **FR-016**: The system MUST require the sender to own every named thing when the invitation is sent, and MUST re-check that ownership at the moment access is created.
- **FR-017**: The system MUST record on every invitation the kind of thing, the access level, how many items, the sender's optional note and a lapse date, defaulting to seven days and settable between one hour and one year.
- **FR-018**: The system MUST answer a send identically whether or not the invited address belongs to an existing account, so that sending an invitation cannot be used to discover who has an account.
- **FR-019**: The system MUST refuse to send an invitation for a thing already actively shared with that address, and MUST tell the sender to change the existing access instead.
- **FR-020**: The system MUST refuse a second pending invitation for the same thing to the same address, and MUST tell the sender an invitation is already outstanding and can be cancelled.
- **FR-021**: The system MUST refuse an invitation addressed to the sender's own address.
- **FR-022**: The system MUST deliver an invitation by email to the invited address, and additionally inside the application where that address belongs to an existing account. Where outbound email is not configured, the system MUST refuse to invite an address with no account and explain why, MUST still deliver in-application to an address that has an account while telling the sender no email could be sent, and MUST warn the operator that outbound email is unconfigured both at start-up and on the operator screens.
- **FR-023**: Anything shown or sent to a recipient who has not yet accepted MUST identify only the sender's display name, the kind of thing, the number of items, the access level, the lapse date and the sender's own note. It MUST NOT name the person or relative concerned and MUST carry no clinical content of any kind.
- **FR-024**: The system MUST make the invitation link unguessable, MUST store it so that it cannot be read back out of the installation, and MUST make it stop working the moment the invitation is accepted, declined, cancelled, withdrawn or lapsed.
- **FR-025**: The system MUST allow an invitation to be answered only by an account signed in with the invited address, matched ignoring case, and MUST refuse any other account without disclosing who the invitation was for or what it covered.

#### The invitation lifecycle

- **FR-026**: The system MUST move an invitation only along these paths: pending to accepted, declined, cancelled or lapsed; and accepted to withdrawn. Every one of those states except pending is final and MUST NOT return to pending.
- **FR-027**: The system MUST let the recipient accept or decline, optionally with a note, and MUST record which it was, when, and the note.
- **FR-028**: The system MUST create every grant an invitation covers as one step: if any one of them cannot be created, none is created and the invitation moves to a final state with an explanation that the items are no longer available.
- **FR-029**: The system MUST treat an invitation as lapsed from the moment its lapse date passes, whether or not any periodic cleanup has run, and MUST refuse any attempt to answer it.
- **FR-030**: The system MUST let the sender cancel a pending invitation at any time, after which its link stops working and no access is ever created from it.
- **FR-031**: The system MUST let the sender withdraw an invitation the recipient already accepted, ending in one step every grant that invitation created.
- **FR-032**: The system MUST refuse a second answer to an invitation that has already been answered, cancelled, withdrawn or lapsed, and MUST tell the caller the state it is in.
- **FR-033**: The system MUST keep answered, cancelled, withdrawn and lapsed invitations out of the lists of things still to act on, MUST retain them for a documented period for accountability, and MUST then remove them while their accountability entries survive.
- **FR-034**: The system MUST NOT depend on any periodic cleanup for correctness. Lapsing is decided whenever an invitation or a grant is looked at; a periodic job may only tidy what has already lapsed.

#### Seeing and managing access

- **FR-035**: The system MUST show an owner, for each of their people and each relative, every account that has access, at what level, since when, when it lapses, whether it came from an invitation and the note attached to it.
- **FR-036**: The system MUST show a grantee everything shared with them: what kind of thing, who shared it, at what level, when it began, when it lapses and the note.
- **FR-037**: The system MUST let an owner change the level or the lapse date of an existing grant without a new invitation, MUST make the change effective on the grantee's next action, and MUST show the new values to both sides.
- **FR-038**: The system MUST let an owner end any grant of anything they own, at any moment.
- **FR-039**: The system MUST let a grantee end their own access to anything shared with them, at any moment, and MUST distinguish that from an owner's revocation in what the owner sees and in the accountability trail.
- **FR-040**: The system MUST let both lists be narrowed by kind, by state and by the other account, MUST page them, and MUST present a helpful empty state rather than a blank screen wherever there is nothing to show.
- **FR-041**: The system MUST show the invitations an account has sent and the invitations it has received, each with its state, and MUST allow each to be acted on according to the caller's role in it.

#### Ending access

- **FR-042**: The system MUST make ended access effective on the grantee's very next request, without their signing out and in and without waiting for any periodic job.
- **FR-043**: The system MUST evaluate the lapse date every time access is decided, so that a lapsed grant is never honoured.
- **FR-044**: After access ends, the system MUST make the thing unreachable by every route — the list of people, the chart, a record opened directly, a document, a search, the timeline, a status view and any live view — and MUST answer every attempt exactly as though the thing did not exist.
- **FR-045**: The system MUST stop any live view of a thing whose access has ended from receiving further updates, and MUST tell the person their access has ended rather than leaving the screen silently frozen.
- **FR-046**: The system MUST resolve a grantee's selected person to nothing when access to that person ends, and MUST return them to their own list of people rather than to an error.
- **FR-047**: The system MUST NOT delete or alter any record, document or relative when access ends; ending access removes reach only, and everything a former grantee created or corrected remains.
- **FR-048**: The system MUST end every grant of a thing when that thing is deleted, MUST end every grant given and received by an account when that account is deleted, and MUST refuse access under a grant held by a disabled account for as long as it is disabled without destroying the grant.

#### Family history as a shareable thing

- **FR-049**: The system MUST allow a single relative's family-history entry to be shared on its own, exposing that entry and the conditions recorded on it and nothing else — not the person it is filed against, not that person's other relatives, and not one clinical record.
- **FR-050**: The system MUST show a family-history grantee the relative exactly as recorded — name, relationship, sex, years of birth and death, whether they have died, and each condition with its name, code, age at diagnosis, severity, status and notes — together with the sender's display name and note.
- **FR-051**: The system MUST show a family-history grantee every correction the owner makes to the shared entry, and MUST refuse every attempt by the grantee to change it.
- **FR-052**: The system MUST allow several relatives to be shared in one invitation and MUST make each of them a separately listed grant that can be ended on its own.
- **FR-053**: The system MUST end every grant of a relative when that relative is deleted, and MUST leave every other grant held by the same accounts untouched.

#### The consequences everywhere else in the application

- **FR-054**: The system MUST decide every read and every write of a person's data, anywhere in the application, from ownership recorded on the data or from an active grant — never from the person currently in view and never from anything the caller supplies.
- **FR-055**: The system MUST include in an account's list of people both the people it owns and the people shared with it, MUST distinguish the two visibly, MUST name for each shared person who shared it and at what level, and MUST count each group.
- **FR-056**: Every list, chart summary, search, timeline, status view, document and live view delivered in the earlier phases MUST return a shared person's data to an entitled grantee exactly as it does to the owner, and MUST NOT disclose any other person's data in doing so.
- **FR-057**: A search run by a grantee over a shared person MUST return only that person's records, and a search naming a person the caller cannot reach MUST be answered as though that person did not exist.
- **FR-058**: The system MUST refuse every attempt by a viewer to create, correct or delete anything about a shared person, MUST tell them they have viewing access only, and MUST change nothing.
- **FR-059**: The system MUST show a grantee the labels the owner has applied to the records they may see, MUST keep the creation, renaming and deletion of the owner's labels with the owner, MUST allow an editor to apply and remove only labels that already exist in the owner's set, and MUST keep every account's own set of labels private to it.
- **FR-060**: The system MUST show a grantee a practitioner or place of care referred to by a record they may see, as part of that record, while keeping the owner's directories as a whole unreachable to them.
- **FR-061**: The system MUST allow a grantee at viewing level to open and download the documents attached to a shared person's records, and an editor to add, correct and remove them under the same recoverable-deletion rules that apply to the owner.
- **FR-062**: The system MUST disclose nothing about the granting account to a grantee beyond its display name, and nothing about a grantee to the granting account beyond its display name and the address that was invited.
- **FR-063**: The system MUST refuse every request about a shared thing from anyone not signed in, and MUST include nothing about the thing in that refusal.

#### Notices

- **FR-064**: The system MUST tell an account, without it refreshing the screen, when an invitation arrives for it, when an invitation it sent is answered, and when access it holds is granted, changed or ended.
- **FR-065**: A notice MUST carry no clinical content and MUST NOT name the person or relative concerned; it may name only the other account's display name and the kind of event.
- **FR-066**: The system MUST deliver a notice only to the account it concerns, MUST re-check that entitlement at the moment of delivery, and MUST deliver nothing to an account that lost entitlement between the event and the delivery.
- **FR-067**: The system MUST treat notices as a courtesy: nothing in this phase may depend on one being delivered, and a missed notice MUST never change what an account may reach.

#### Privacy, permission and accountability

- **FR-068**: The system MUST record in the accountability trail every invitation sent, accepted, declined, cancelled, withdrawn and lapsed, and every grant created, changed and ended — distinguishing an owner's revocation from a grantee's departure and from a lapse — storing who acted, what they did, which thing and which person by opaque identifier, and when.
- **FR-069**: The system MUST record every refused attempt to reach a thing through a grant that does not exist, has ended or has lapsed.
- **FR-070**: The system MUST record each opening of an individual record and each download of a document by an account that is not the owner of the person concerned, so that an owner can later account for who looked at what.
- **FR-071**: The system MUST NOT store in the accountability trail any name, email address, note text, label, clinical value or document name; things and accounts are referred to by opaque identifier only.
- **FR-072**: The system MUST NOT write an invitation link, an email address, a note, a display name or a person's name into any log, measurement, trace or error report.
- **FR-073**: The system MUST send nothing about sharing outside the installation except the invitation email to the invited address, and only where the operator has configured outbound email.

#### Scale

- **FR-074**: The system MUST keep both sharing screens, both invitation lists and every widened list correct and responsive, within the times stated in the success criteria, for an owner holding two hundred active grants across twenty people and a grantee reaching fifty shared people.
- **FR-075**: The system MUST page both sharing screens and both invitation lists so that entries are neither repeated nor skipped while grants and invitations are being created and ended.

#### Verification

- **FR-076**: Every acceptance scenario in this specification MUST exist as an automated test, and the phase MUST NOT be considered complete until every one of them passes.
- **FR-077**: Every operation anywhere in the application that touches a person's data MUST carry automated tests proving that an account with no access is refused, that an account holding an active grant of the right level succeeds, and that an account whose grant has ended or lapsed is refused — together with tests proving a viewer is refused on every write.
- **FR-078**: The family-history isolation rule MUST be proved by automated tests that attempt, from a family-history grantee's account, to reach the person the entry is filed against, that person's records of every type and their other relatives, and find every attempt answered as though nothing existed.
- **FR-079**: Every screen this phase adds and every screen it changes MUST be covered by the project's automated browser check at both the desktop and mobile sizes it defines, on an installation seeded so that some of those screens have nothing to show, and a screen added without such coverage MUST fail the build.
- **FR-080**: The privacy rules in this section MUST be verified by an automated exercise of every operation this phase defines, which then inspects the installation's diagnostic output and finds no email address, note, display name, person's name or invitation link in it.

### Key Entities

- **Shareable thing**: The two things that can be shared, and the only two — a person's whole chart, and one relative's family-history entry. Everything else in the application is reached through one of them or not at all.
- **Access grant**: One account's live permission over one shareable thing, given by its owner. Records who gave it, who holds it, at what level, from when, until when if ever, the note that came with it, whether and when and by whom it was ended, and which invitation produced it. Its absence is indistinguishable from the thing not existing.
- **Access level**: Viewing or editing. Viewing reads everything about the shared thing; editing also changes the records and documents of a shared person. Neither ever deletes the person, changes who else has access, or passes access on. Family history is viewing only.
- **Invitation**: An offer of access, addressed to an email address rather than to an account, covering one or more things of one kind at one level, carrying a note and a lapse date, and moving from pending to exactly one of accepted, declined, cancelled, withdrawn or lapsed. It is the only way a grant comes into being.
- **Invitation link**: The unguessable credential in the invitation email. It identifies the invitation, is never readable back out of the installation, stops working the moment the invitation leaves the pending state, and by itself grants nothing — it can only be acted on by somebody signed in as the invited address.
- **Sharing notice**: A short, contentless message telling an account that an invitation arrived, that one it sent was answered, or that access it holds changed. Names the other account and the kind of event, never the person concerned and never anything clinical.

## Success Criteria *(mandatory)*

### Measurable Outcomes

A criterion marked **[outcome metric]** is observed on a real person rather than built: it maps to
no task by design, and it says so here rather than being left silently unmapped. Every other
criterion below maps either to a task in `tasks.md` or to a phase-exit criterion in `plan.md`, so
an unmapped one that carries no marker is a gap.

- **SC-001** *[outcome metric]*: An account holder who has used the application before can share a person with somebody by email address, at a chosen level, in under 60 seconds, without documentation or assistance.
- **SC-002** *[outcome metric]*: A recipient can go from the emailed link to reading the shared chart in under 2 minutes, including creating an account if they do not have one. *(An end-to-end human-speed outcome spanning registration, confirmation and the shared chart, with no buildable work of its own beyond FR-013–FR-021's tasks; measured by observation, like SC-001.)*
- **SC-003**: 100% of a shared person's records, documents, search results, timeline entries and status views are readable by an entitled viewer and identical to what the owner sees, and 0 records belonging to any other person are reachable through the grant.
- **SC-004**: 100% of attempts by an account with no grant, an ended grant or a lapsed grant to reach a shared person or relative — by the list, the chart, a record opened directly, a document, a search, the timeline, a status view or a live view — are refused with a response indistinguishable from the thing not existing.
- **SC-005**: Ending access is effective on the former grantee's next request 100% of the time, with no sign-out and no periodic job involved, and any screen they had open stops updating and states that access ended within 5 seconds.
- **SC-006**: 100% of writes attempted by a viewer are refused with an explanation naming their level, and 0 of them change anything.
- **SC-007**: 100% of attempts by any grantee to delete a shared person, change that person's identifying profile, alter anybody's access, or pass access on are refused.
- **SC-008**: For a family-history grantee, 100% of attempts to reach the person the shared entry is filed against, that person's records of every type, or any other relative are refused, and exactly 1 relative's entry is reachable per grant.
- **SC-009**: 100% of invitations to an address with no account can be accepted after that address registers, and 0 of them can be accepted by an account signed in with a different address.
- **SC-010**: 0 of the messages, screens and previews shown to a recipient who has not yet accepted contain the name of the person or relative concerned or any clinical detail.
- **SC-011**: Sending an invitation produces the same observable result for an address that has an account and one that does not, so that 0 information about who holds an account can be obtained by sending invitations.
- **SC-012**: 100% of invitation links stop working the moment the invitation is answered, cancelled, withdrawn or lapses, and 0 invitations in a final state can be returned to pending by any route.
- **SC-013**: 100% of invitations covering several things are accepted all-or-nothing, leaving 0 partial sets of grants when one of the things can no longer be shared.
- **SC-014**: For an owner with 200 active grants across 20 people and a grantee reaching 50 shared people, both sharing screens, both invitation lists and the list of people appear within 2 seconds of being requested, and paging through any of them repeats or skips 0 entries.
- **SC-015**: 100% of invitations sent, answered, cancelled, withdrawn and lapsed, of grants created, changed and ended, of record openings and document downloads by somebody who is not the owner, and of refused attempts, produce an accountability entry; and 0 of those entries contain a name, an email address, a note, a label or a clinical value.
- **SC-016**: Across an automated exercise of every operation this phase defines, 0 email addresses, notes, display names, person names or invitation links appear in the installation's logs, measurements, traces or error reports.
- **SC-017**: A notice of an invitation, an answer or a change of access reaches the account concerned within 5 seconds without a manual refresh, a session left open for 60 continuous minutes still receives them, and 0 notices contain a person's name or any clinical detail.
- **SC-018**: 100% of the acceptance scenarios in this specification exist as automated tests and pass. This phase is not complete while any one of them is missing or failing, and every operation in the application that touches a person's data additionally carries automated tests for the stranger, the entitled grantee and the grantee whose access ended.
- **SC-019**: 100% of the screens this phase adds or changes pass the project's automated browser check at both the desktop and mobile sizes, with zero browser console errors, zero uncaught page failures and zero failed resource requests, on a seeded installation where several of those screens have nothing to show; and adding a screen without adding it to that check fails the build 100% of the time.
- **SC-020**: A person using only a keyboard can send an invitation, answer one, change a level and end access, at both viewports, without losing sight of where the focus is.

## Assumptions

### What earlier phases already delivered, and is not re-specified here

- **Phase 001 (Walking Skeleton)** provides the running application, its configuration, its diagnostics, the accountability trail this phase writes to, the application shell and its navigation landmarks, and the automated browser check together with the screen inventory that check is derived from. This phase adds screens to that check; it does not restate how the check works.
- **Phase 001** provides accounts, sign-up, sign-in, sessions, password handling and account deletion, and the rule that a record belongs to somebody and is refused to everybody else. This phase widens that rule; it does not replace it.
- **Phase 001** provides live updating of open screens and the rule that a screen left open for an hour is still receiving updates. The notices in this phase and the cut-off of a revoked grantee's open screens use that same mechanism.
- **Phase 001** provides the rule that a save based on a version that has since changed is refused with the current values shown. That rule is what makes two people editing one shared chart safe, and it is not restated here.
- **Phase 002 (Patient Core)** provides people, ownership, the person currently in view, the rule that the person in view is never a basis for permission, and the chart summary. This phase adds shared people to the list, the switcher and the summary under exactly those rules.
- **Phase 002** provides the directories of practitioners and places of care, owned per account.
- **Phase 003 (Clinical Records)** provides the twelve further record types, the relationships between them, labels, search, the timeline and the status views — and family history, including a relative's conditions with their ages at diagnosis. This phase shares those records and that family history; it does not add to them or change what they hold.
- **Phase 004 (Labs and Attachments)** provides laboratory results and attached documents, including their recoverable deletion window. This phase makes them reachable through a grant; it does not change how they work.
- **Outbound email** is the installation's, configured by the operator. This phase uses it to deliver invitations and depends on the operator having configured it in order to invite an address with no account.

### Decisions taken here rather than deferred

- **One sharing model, not two.** The system this one reimagines ran patient sharing and family-history sharing as two entirely separate mechanisms, each with its own way to invite, its own way to revoke and its own "shared with me" list. The product distinction between delegating a chart and exchanging a pedigree is real and is kept — as the kind of thing being shared, its level ceiling and its narrower disclosure — but the mechanism is one. Two mechanisms means two places to get authorization wrong.
- **Two levels, not three.** The system this one reimagines documented viewing, editing and a third level it never defined, and deletion was the owner's regardless. A level nobody can describe is a level nobody can test.
- **Access is only ever created by acceptance.** There is no way for an owner to push access onto somebody. Somebody's medical records appearing in your account without your having agreed is not a feature.
- **Ownership is re-checked when access is created, not only when it is offered.** An invitation is an offer, and an offer can go stale.
- **Lapsing is decided when access is decided, not by a sweeper.** The system this one reimagines marked shares inactive with a periodic job, which meant a lapsed share was still honoured until the job ran. Here the correctness never depends on a job having run; a job only tidies.
- **A grantee cannot pass access on.** Delegation multiplies the number of people who can reach a medical record without the owner ever agreeing to any of them, and nothing in this phase's stories needs it.
- **Editing does not extend to who the person is.** An editor keeps the care record — the medications, the readings, the visits, the documents. The person's name, date of birth, sex and photograph are identity rather than care and stay with the owner, as does deleting the person.
- **Nothing shown before acceptance names the person concerned.** A recipient decides on the basis of who invited them, what kind of thing, how many items and the sender's own words. The sender may say whatever they like in the note; the application adds no identifying detail of its own to a message aimed at somebody who has not yet been granted anything.
- **Sending an invitation cannot be used to find out who has an account.** The result is the same either way, because the alternative turns the sharing screen into an account-enumeration tool.
- **An invitation to an already-shared thing is refused rather than silently merged.** The owner is directed to change the existing access, which is a clearer act with a clearer trail.
- **Reads of a shared person's records by somebody who is not the owner are recorded.** An owner who lets somebody into their father's chart is entitled to know afterwards who opened what. Individual record openings and document downloads are recorded; the ordinary paging of a list is not, because that would drown the trail without answering the question anybody actually asks.
- **A grantee sees the owner's labels on the records they can see, but never the owner's label vocabulary.** A label is part of how a record reads; the set of labels an account uses across its whole household is not.
- **Notices are in-application and by invitation email only.** Delivery over chat services, push services or arbitrary webhooks, digests, and per-event delivery preferences are not part of this phase and are not planned for one; nothing here depends on a notice arriving.
- **Answered invitations are kept for a documented period and then removed.** Their accountability entries outlive them, so history is preserved without an ever-growing list of things nobody will ever act on again.
- **The English-only interface** of the earlier phases continues, with the language preference still honoured for date and number presentation.

### What this phase deliberately does not do

- **Real-time collaborative editing.** Two people may hold the same shared chart, and the existing rule that a save based on a stale version is refused is what keeps that safe. Simultaneous editing of one record with merged changes is not part of this phase and is not planned for one.
- **Organisation, practice or clinic accounts.** Sharing is between individual accounts. A shape in which a practice holds access, staff inherit it and an administrator manages the roster is a different model and is not part of this phase.
- **Public or link-only sharing.** There is no way to make anything readable to somebody who is not signed in as the invited address. Possession of a link is never sufficient.
- **Sharing anything other than a person's chart and a relative's family-history entry.** Sharing a single record, a document on its own, or a saved report is not part of this phase.

### What later phases will change about what is specified here

- **Phase 006 (Reporting and Operations)** adds reporting, export and the operator surface. Its audit reader is where an owner will actually read the accountability entries this phase writes, including who opened which of their records; this phase writes them and states what they must contain, and does not build the reader. Whether and how a grantee may export a shared person's data is decided there, under the rule this phase sets: no later feature may deliver to a grantee more than their access level allows, and export by a grantee is itself an accountable act.
- **Phase 006** adds the operator screens on which the warning about unconfigured outbound email, required by this phase, is surfaced.
- **Phase 006** owns the scheduled tidying of lapsed invitations and grants and the removal of answered invitations past their retention period. Nothing in this phase's correctness depends on that tidying running.
- **The operator's break-glass administrative access** can reach every record on the installation by design, is protected by multi-factor authentication and an address allowlist, and every such session is recorded. It is not a grant, it does not appear on anybody's sharing screen, and it is not a route by which one account holder reaches another's records.
