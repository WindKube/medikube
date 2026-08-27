# Quickstart: Sharing and Collaboration (phase 005)

How to run this phase's work locally and verify it by hand, end to end. Every command is run from
`/Users/krzysztof.wiatrzyk/private/monorepo/medikube` unless stated.

---

## 0. Preconditions (30 seconds, and skipping them wastes an hour)

```bash
go version          # must print go1.27.x — NOT 1.26.5
echo "$GOTOOLCHAIN"  # must be empty. GOTOOLCHAIN=local fails with a misleading toolchain error
grep -E '^(go|toolchain) ' go.mod
task --list
```

PocketBase v0.40.1 requires Go 1.27: its `go.mod` declares it and 67 non-test files import
`encoding/json/v2` (VERIFIED-SOURCE-FACTS FACT 0). MediKube is deliberately the only project in this
monorepo off the 1.26.5 house standard.

**If you changed a migration or a seed fixture, regenerate the test fixture first**, or every
integration test silently runs against the previous schema:

```bash
task fixture:regen          # migrations + `medikube seed` -> internal/testdata/pb_data
```

---

## 1. Build and run

```bash
task gen                    # templ generate + tailwind
task build
export MEDIKUBE_PUBLIC_URL="http://127.0.0.1:8090"
export MEDIKUBE_LOG_PRETTY=true
./medikube seed --reset     # deterministic accounts, patients, grants and invitations
./medikube serve --http=127.0.0.1:8090
```

The boot log should contain **exactly one** warning about outbound email, and after this phase it
names invitations alongside the features phase 001 already listed:

```
WRN outbound email is not configured; password recovery, address confirmation and invitations
    to addresses without an account will be refused
```

That single line satisfies FR-022's boot-warning clause and phase 001's FR-076 at once — it is
phase 001's warning with one more feature named, **not a second warning** — and it fires because
`Settings().SMTP.Enabled` is false on a fresh instance ([research D-05](./research.md#d-05)). To make it go away, set SMTP in the admin UI at
`/_/` (Settings → Mail) — a local catcher such as MailHog or Mailpit on `localhost:1025` is enough
— restart, and confirm the warning is gone.

The seed prints the pending invitation's **plaintext token**. It is printed by the seeder only, on
a throwaway instance; the server never prints it, never logs it and stores only its SHA-256
([research D-15](./research.md#d-15)).

---

## 2. The seeded cast

| Account (password `medikube-dev-password`) | Holds |
|---|---|
| `owner@medikube.local` | 2 patients, one with 3 relatives, records of every kind, 1 attachment |
| `viewer@medikube.local` | an accepted **view** grant on patient A |
| `editor@medikube.local` | an accepted **edit** grant on patient A |
| `cousin@medikube.local` | an accepted view grant on **one relative** of patient A |
| `stranger@medikube.local` | nothing |
| `empty@medikube.local` | nothing shared in either direction — the empty-state account |
| `left@medikube.local` | a grant they revoked themselves |
| `lapsed@medikube.local` | a grant whose end date has passed |
| `disabled@medikube.local` | an active grant, account disabled |
| `newcomer@medikube.local` | **deliberately not registered** — the invite-a-stranger path |

---

## 3. Walk story 1 by hand — share a chart (about 3 minutes)

1. Sign in as `owner@medikube.local`. Open `/patients`, open patient A, press **Share**.
   The drawer opens **without a navigation** — it is a Datastar signal, not a route.
2. Enter `kwame@example.org`, choose **Viewing**, write a note, leave the lapse date at its default.
   Submit. You should see "invitation sent", and — because that address has no account and SMTP is
   off — a `422` explaining outbound email is not configured. **This is correct**
   ([D-06](./research.md#d-06)). Configure SMTP (step 1) and try again, or use
   `viewer2@medikube.local`, which does have an account.
3. Open `/invitations` → **Sent**. The invitation is `pending`, `resource_count: 1`, with your note.
4. Copy the invitation link from the mail catcher (or the seed output). Open it in a **private
   window**: `/invite/{token}` shows who invited you, that it is a person's chart, one item, at
   viewing level, when it lapses, your note, and a masked hint of the address. **Confirm it shows no
   patient name anywhere on the page** — that is FR-023 and it is the single most important thing on
   this screen.
5. Sign in as the recipient. `/invitations` → **Received** shows it. Accept.
6. Open `/patients`: the shared person appears in a visibly separate group, labelled
   "shared by … (view)", and both groups are counted.
7. Open their chart, every kind list, a record, the documents library, search and the timeline.
   Everything the owner sees, you see.
8. Open `/patients` again and confirm **the owner's other patient is not there**, and that
   `/practitioners` is empty for you even though a practitioner's name is visible on one of the
   shared records — that asymmetry is FR-060 and [D-23](./research.md#d-23).

**What should be impossible from here:** editing anything (the write controls are absent, and a
forced write is `403 forbidden_view_only`), deleting the person, changing the person's name,
sharing it onward, or seeing the sharing screen for somebody else's grant.

---

## 4. Walk story 2 — take it away, and prove it is gone (2 minutes)

1. As the recipient, leave the shared patient's record list **open**.
2. As `owner@medikube.local`, open `/sharing` → **Granted**, find the row, press **End access**.
3. Watch the recipient's open window: within 5 seconds the list is replaced by a plain panel saying
   access has ended, with a link back to `/patients`. **It must not silently freeze and must not
   keep showing content** (FR-045, US2 scenario 2).
4. In the recipient's window, click anything: everything is `404`. **Do not sign out** — the whole
   point is that no sign-out and no scheduled job is involved (FR-042, SC-005).
5. Reload `/patients` as the recipient: the person is gone, the active-patient switcher resolved to
   nothing, and you are on your own list rather than an error (FR-046).
6. As the owner, open the patient's records: **everything the former grantee created is still
   there** (FR-047).
7. Confirm the owner cannot re-grant by pressing anything: sharing again takes a fresh invitation
   the other person must accept (US2 scenario 8).

Repeat with `lapsed@medikube.local` to see the same thing happen with no revoke at all — the end date
is evaluated on every request, never by a sweeper (FR-043, [D-02](./research.md#d-02)).

---

## 5. Walk story 4 — the pedigree, and only the pedigree (1 minute)

Sign in as `cousin@medikube.local`. You can read exactly one relative — name, relationship, years,
whether they have died, and every condition with its code, age at diagnosis, severity, status and
notes — plus the sender's display name and note.

Now try to escape, and fail every time (FR-078, SC-008):

```bash
# with the cousin's session cookie in $C, and the seeded ids from `medikube seed --print-ids`
curl -s -o /dev/null -w '%{http_code}\n' -b "$C" localhost:8090/api/v1/patients/$PATIENT_A       # 404
curl -s -o /dev/null -w '%{http_code}\n' -b "$C" localhost:8090/api/v1/records/medications?patient=$PATIENT_A  # 404
curl -s -o /dev/null -w '%{http_code}\n' -b "$C" localhost:8090/api/v1/records/family-history/$OTHER_RELATIVE   # 404
curl -s -o /dev/null -w '%{http_code}\n' -b "$C" localhost:8090/api/v1/search?patient=$PATIENT_A&q=a            # 404
```

Every one is `404`, byte-identical to a non-existent id.

---

## 6. Walk story 6 — the notice (1 minute)

Open two browsers, signed in as the owner and the recipient. Send an invitation. A notice appears in
the recipient's window within 5 seconds **without a refresh**. Accept it: a notice appears in the
owner's window. Change the level: another notice.

Read one of them carefully: it names the other account's display name and the kind of event, and
**nothing else** — no patient name, no diagnosis, no medication (FR-065, SC-017).

Leave a window open for an hour and send another invitation. It still arrives — that is the
PocketBase 5-minute `WriteTimeout` trap not biting, which is the only reason the
`newStream()` helper exists ([D-04](./research.md#d-04)).

---

## 7. The gates, in the order CI runs them

```bash
task check                          # fmt + vet + golangci-lint v2 + go test -race -count=1 ./...
go test ./internal/store/migrations/...    # the `= ''` index assertion — read D-01 before touching it
go test ./internal/service/access/...      # the authorization matrix + its coverage gate
go test ./internal/web/api/... -run Authz   # the six-actor matrix over every patient-scoped route
task openapi && git diff --exit-code api/openapi.json   # regenerate + diff gate
task routes                          # the route inventory the browser gate is derived from
task test:e2e                        # Playwright, both viewports, zero console errors
task test:phileak                    # no address, note, display name or token in any diagnostic
go test -tags slowsse ./internal/web/stream/...   # ~6 minutes: the >5-minute SSE liveness assertion
go test -tags scale ./internal/testsupport/scale/...  # 200 grants / 20 people / 50 shared patients
```

Any red step blocks merge (Principle IX). Two of them fail in ways worth recognising:

- **`git diff --exit-code api/openapi.json` non-empty** — you added or changed an operation and did
  not commit the regenerated document. Commit it; the diff is the review artefact.
- **`internal/service/access/coverage_test.go` failing** — you added a route that touches patient
  data and did not give it the six-actor matrix. That is the gate doing its job; write the tests.

---

## 8. Things that will look like bugs and are not

| Symptom | Explanation |
|---|---|
| A share query returns nothing at all | You wrote `revoked_at IS NULL`. PocketBase columns are `TEXT DEFAULT '' NOT NULL` — there is no NULL. Use `= ''`. [D-01](./research.md#d-01) |
| A share query returns **revoked** grants | The same mistake, inverted (`IS NULL OR …`). This one is a disclosure bug. Read D-01 before writing another predicate |
| Inviting an unknown address is refused on a fresh instance | SMTP is unconfigured. It is the documented behaviour, warned at boot. [D-05](./research.md#d-05), [D-06](./research.md#d-06) |
| The invitation email never arrives and no error is raised | Same cause on a machine that *has* `sendmail`: PocketBase falls back to exec'ing it. Configure SMTP |
| A lapsed grant still shows on `/sharing` | Correct. It is *shown* as lapsed to both sides (FR-029); it is never *honoured*. The tidy pass renames the invitation later |
| No `share_expire` audit row the instant a grant lapses | Correct. A lapse is the absence of an event; the row is written by the tidy pass. [D-19](./research.md#d-19) |
| An SSE stream dies at exactly five minutes | The `newStream()` helper was bypassed. Every stream must go through it |
| A grantee can see a practitioner's name but `/practitioners` is empty for them | Correct, and asserted by a test. FR-060, [D-23](./research.md#d-23) |
| `tests.TestApp` recursion / stack overflow | A `TestApp` was shared across `ApiScenario` cases. Never do that (VERIFIED-SOURCE-FACTS FACT 7) |
