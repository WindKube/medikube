# Quickstart: running and verifying Patient Core by hand

How a developer brings this phase up locally and convinces themselves, end to end, that it works —
including the parts that are easy to believe and hard to check, like "the refusal really is
indistinguishable from a not-found" and "the migration really did move every medication".

Everything below runs from `medikube/` unless stated. Every routine action is a Task, per the
monorepo convention: you run tasks, not remembered command lines.

---

## 0. Prerequisites

| Need | Why | Check |
|---|---|---|
| **Go 1.27+** | PocketBase v0.40.1 declares `go 1.27` and imports `encoding/json/v2` in 67 files. 1.26.5 cannot build it. | `go version` |
| `GOTOOLCHAIN` **unset** (or `auto`) | `GOTOOLCHAIN=local` on a 1.26 machine fails with a misleading "built with go1.26 < targeted go1.27" | `go env GOTOOLCHAIN` |
| Task | the entry point for everything | `task --list` |
| Node + Playwright browsers | build-time only; never in the runtime image | `npx playwright --version` |
| `curl` and `jq` | the manual API walkthrough below | |

```bash
task install:tailwind
task install:golangci-lint     # v2 — v1 does not understand Go 1.27
npx playwright install --with-deps chromium
```

---

## 1. Build and bring it up

```bash
task gen        # templ generate + tailwind; vet/lint/test/build all depend on this
task build
task seed       # deterministic demo data — same accounts, same ids, every time
task run
```

`task run` needs the environment. The minimum for a local instance:

```bash
export MEDIKUBE_ENV=dev
export MEDIKUBE_DEV=true
export MEDIKUBE_DATA_DIR=./pb_data
export MEDIKUBE_HTTP_ADDR=127.0.0.1:8090
export MEDIKUBE_PUBLIC_URL=http://127.0.0.1:8090
export MEDIKUBE_LOG_LEVEL=debug
export MEDIKUBE_LOG_PRETTY=true
export MEDIKUBE_AUTH_REGISTRATION_OPEN=true    # the seed opens it so the sign-up path is exercised
export MEDIKUBE_FILES_PHOTO_MAX_BYTES=15728640 # 15 MiB; PocketBase's own default is 5 MiB
```

**Two things you should see at boot, and one you should not.**

- The migration log names all six of this phase's migrations, in order:
  `facilities → practitioners → patients → users_active_patient → audit_events_patient →
  medications_repoint`. They share one transaction, so it is all six or none.
- A loud warning if superuser MFA or the IP allowlist is unconfigured. That is expected locally and
  is not expected in production.
- **You should not see the process start at all** if any file field is unprotected or any
  collection has a non-nil API rule. The boot assertions refuse to start. If you want to see the
  gate work, flip `Protected: false` on `patients.photo` in the migration and watch it refuse.

Sign in as the seeded account:

```
amara@example.test / medikube-dev-password
```

---

## 2. The five-minute manual walkthrough

Do this in a browser. It follows the six user stories in order, and each step names the
requirement it proves.

### US1 — profiles exist and belong to somebody

1. Open `/patients`. **Exactly one** profile is listed and it is marked as yours (FR-005, FR-010,
   US1-1). You never created it — registration did.
2. Add a person: first name, last name, date of birth. It appears in the list (FR-001, US1-2).
3. Add another, but set the date of birth to tomorrow **and** leave the last name blank. Both
   errors come back **in the same response**, each attached to its field (FR-003, US1-3). Fix them
   and save.
4. Open a profile and upload a photograph. It appears in the list and in the switcher at a small
   size — that thumbnail was generated at upload, not on demand (FR-009).
5. Replace the photograph. The old one is gone from disk, thumbnails included:

   ```bash
   find ./pb_data/storage -path '*thumbs_*' | sort     # exactly two files per patient with a photo
   ```

6. Open a profile that has only a name and a date of birth. Missing details read as **absent** —
   not "0", not "unknown", not a blank box (FR-030, US1-6).

### US2 — every record belongs to one person

7. Open `/medications`. Your pre-existing medications are all there, under your own profile
   (FR-022, SC-006).
8. Record a medication for the second person. Switch back. It is **not** in the first person's
   list (FR-023, US2-2).
9. Try to record a medication with no person selected — the form will not let you, and the API
   refuses with "which person is this for?" (FR-021, US2-3).

### US3 — switching, and switching never granting anything

10. Choose a person in the top-bar switcher. Every person-scoped screen is now about them, and each
    one **names them on screen** (FR-014, FR-019, SC-003).
11. Sign out. Sign back in. The same person is still chosen (FR-013, US3-2).
12. Delete the chosen person (see US6 below). You land on `/patients` with an explanation — not an
    error, and not somebody else's data (FR-017, US3-3).

### US4 — the chart

13. Open a person's chart. Header: name, derived age, sex, blood type, height and weight **in your
    preferred units**, relationship, primary practitioner (FR-027).
14. Go to `/settings`, switch units to imperial, come back. The displayed values changed; the
    recorded values did not (FR-007, US4-3) — verify with `curl` in §3.
15. Open the chart of a person with nothing recorded. A helpful empty state, not a list of zeros
    (FR-030, US4-2).

### US5 — the directory

16. Add a practitioner with a specialty from the offered list. Add a practice and a pharmacy at
    `/facilities` (FR-032, FR-034).
17. Add a **second branch** of the same pharmacy chain, same name, different address. Both are
    stored and both are offered (FR-035, US5-3).
18. Try to add a second practitioner with the same name **and** the same specialty. Refused, with
    an explanation (FR-038, US5-4).
19. Set the practitioner as someone's primary practitioner and as a medication's prescriber. Then
    delete the practitioner: you are told how many things reference them, and after confirming,
    **both records still exist with the reference cleared** (FR-040, US5-5).

### US6 — removing a person

20. Delete a person who has records. You are shown their name and how many records will be
    destroyed, and you must confirm deliberately (FR-048, US6-1).
21. Afterwards: no medications, no photograph, nothing recoverable (FR-049, US6-2).
22. Try to delete **your own** profile. Refused, with the explanation that closing the account is
    what removes it (FR-051, US6-4).

---

## 3. The parts you cannot check by looking

### The refusal really is indistinguishable

This is SC-005, and eyeballing a 404 page proves nothing. Sign in as the **second** seeded account
and address the first account's ids directly.

```bash
A=$(jq -r .accountA.patientId internal/testsupport/fixtures.json)
TOKEN_B=$(curl -s -XPOST localhost:8090/api/v1/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"bo@example.test","password":"medikube-dev-password"}' | jq -r .token)

curl -s -o /tmp/other.json -w '%{http_code}\n' localhost:8090/api/v1/patients/$A \
  -H "Authorization: Bearer $TOKEN_B"
curl -s -o /tmp/ghost.json -w '%{http_code}\n' localhost:8090/api/v1/patients/pat_doesnotexist1 \
  -H "Authorization: Bearer $TOKEN_B"

# both 404, and the bodies differ only by request_id:
diff <(jq 'del(.error.request_id)' /tmp/other.json) <(jq 'del(.error.request_id)' /tmp/ghost.json)
```

Repeat for `/photo` and for a medication id. **No name, no date of birth, no photograph, and no
hint that the subject exists.**

### The person in view really is not an authorization input

FR-015, and the one that matters most. Point your pointer at a patient you own, then ask for
another account's:

```bash
curl -s -XPUT localhost:8090/api/v1/me/active-patient \
  -H "Authorization: Bearer $TOKEN_B" -H 'content-type: application/json' \
  -d "{\"patient\":\"$(jq -r .accountB.patientId internal/testsupport/fixtures.json)\"}"

curl -s -o /dev/null -w '%{http_code}\n' \
  "localhost:8090/api/v1/records/medications?patient=$A" -H "Authorization: Bearer $TOKEN_B"
# 404 — changing the selection granted nothing
```

And the other half — a list with no patient at all:

```bash
curl -s -w '\n%{http_code}\n' localhost:8090/api/v1/records/medications \
  -H "Authorization: Bearer $TOKEN_B"
# 400 patient_required — never a silent fallback to the pointer
```

### The photo has no self-authorizing link

```bash
curl -s -o /dev/null -w '%{http_code}\n' "localhost:8090/api/v1/patients/$A/photo"   # 401
curl -s "localhost:8090/api/v1/patients/$A/photo?size=100x100t" -H "Authorization: Bearer $TOKEN_B" \
  -o /dev/null -w '%{http_code}\n'                                                   # 404
./medikube routes | jq -r '.[].path' | grep -c '^/api/files'                         # 0
```

There is no `?token=` parameter anywhere in the inventory, and there never will be.

### Units convert, values do not

```bash
curl -s localhost:8090/api/v1/patients/$MY -H "Authorization: Bearer $TOKEN" \
  | jq '{height_cm, weight_kg, display}'
# switch the preference, then re-run: height_cm and weight_kg are byte-identical; only display changed
```

### The migration moved everything

Against a database created **before** this phase (keep one around, or check out the phase-001 tag,
run `task seed`, then check out this branch and restart):

```bash
sqlite3 ./pb_data/data.db "
  SELECT 'unattributed', COUNT(*) FROM medications WHERE patient IS NULL OR patient='';
  SELECT 'not-on-a-self-record', COUNT(*) FROM medications m
    JOIN patients p ON p.id = m.patient WHERE p.is_self_record = 0;
  SELECT 'accounts-without-a-self-record', COUNT(*) FROM users u
    WHERE NOT EXISTS (SELECT 1 FROM patients p WHERE p.owner = u.id AND p.is_self_record = 1);
"
# every one must be 0  (FR-005, FR-022, SC-006)
```

### Nothing personal reached the diagnostics

SC-008. Exercise the whole surface, then grep what came out:

```bash
task run 2>&1 | tee /tmp/medikube.log &
task test:e2e
grep -Ei 'amara|okonkwo|1987-|Ibuprofen|\.jpg|[0-9]+ [A-Z][a-z]+ Street' /tmp/medikube.log
# no output. Not one name, date of birth, address, medication name or file name.
curl -s localhost:9090/metrics | grep -E 'patient_id|user_id|record_id'   # no output
```

---

## 4. The automated gates

```bash
task check          # gen + vet + lint + test -race
task test:e2e       # Playwright, both viewports, derived route list
task openapi        # regenerate api/openapi.json
git diff --exit-code api/openapi.json    # must be clean, or the diff is reviewed and intentional
./medikube routes | jq 'length'          # the inventory the smoke gate is built from
```

**Prove the gate can go red before you trust it green** (constitution VIII, open risk R11). Break
one page on purpose — add a `<script>throw new Error('x')</script>` to
`internal/web/views/patients/list.templ`, or delete the `region[name="Patients"]` landmark — then
run `task test:e2e` and watch it fail. Put it back.

**Prove a page without a test cannot ship.** Register a page route in `internal/httproute/routes.go`
without a landmark and rebuild: the route registry refuses it at boot, and if you get past that,
`smokeTargets()` throws during Playwright's collection phase. Either way the build is red, which is
the whole point of deriving the list from the binary (FR-056, SC-013).

---

## 5. When something is wrong

| Symptom | Almost always |
|---|---|
| `go.mod requires go >= 1.27` | `GOTOOLCHAIN=local` is set, or you are on the 1.26.5 house toolchain |
| The process refuses to start naming a collection | a boot assertion: a non-nil API rule, an unprotected file field, or a `CascadeDelete` flag that does not match the declared matrix |
| Deleting a patient returns 500 | `medications.patient` lost `CascadeDelete: true`. PocketBase **fails** the delete when an emptied relation is `Required` (`core/record_model.go:1619`) |
| Deleting a patient deleted the **account** | `users.active_patient` was flipped to `CascadeDelete: true`. There is a boot assertion for exactly this |
| A photo 404s for its owner | the field lost `Protected: true` and something is routing through PocketBase's file handler, or the thumb size is not in the field's `Thumbs` list |
| Thumbnails accumulate after replacing a photo | they were written outside `thumbs_<filename>/`, so PocketBase's `DeletePrefix` cleanup cannot find them |
| Two identically named practitioners with no specialty both saved | `specialty` is storing `NULL` instead of `''`; SQLite treats NULLs as distinct in a unique index |
| The SSE stream dies silently after five minutes | the `WriteTimeout` override on the `ServeEvent`'s `*http.Server` was lost. It passes every test shorter than five minutes |
| A Datastar attribute does nothing at all | you wrote `data-on-click`. v1 uses a colon: `data-on:click`. `data-on-load` is now `data-init` |
| The smoke gate is green on a page you know is broken | it is not in `medikube routes`, so it was never under test |
