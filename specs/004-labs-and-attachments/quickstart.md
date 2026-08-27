# Quickstart: Labs and Attachments (phase 004)

How a developer runs this phase's work locally and verifies it by hand, end to end. Every command
is run from `/Users/krzysztof.wiatrzyk/private/monorepo/medigo` unless stated otherwise.

---

## 0. Preconditions

```bash
go version          # must print go1.27.x — PocketBase v0.40.1 cannot build on 1.26.5
echo "$GOTOOLCHAIN" # must be EMPTY. GOTOOLCHAIN=local fails with a misleading toolchain error
task --list         # the Taskfile is the entry point for every routine action
```

Phases 001, 002 and 003 must be in place: this phase registers the fifteenth record kind into an
existing registry, attaches documents to record kinds those phases created, and reuses their
authorizer, audit trail, realtime bridge, cursor pagination and error envelope.

One-time tools:

```bash
task install:tailwind          # standalone CLI, pinned by ARG; the x86_64 asset is named x64, not amd64
task install:golangci-lint     # v2; v1 does not understand Go 1.27
npx playwright install --with-deps chromium   # build-time only; never in the runtime image
```

---

## 1. Build and run

```bash
task gen        # go tool templ generate + tailwind; never `templ` from PATH — the tool directive pins it
task build
task run        # http://127.0.0.1:8090
```

`task run` applies the migrations at startup, which for this phase means the four new collections
**and the standardized test catalogue**, because the catalogue ships as a migration rather than as
seed data (research D-11).

Environment for a local run:

```bash
export MEDIGO_PUBLIC_URL=http://127.0.0.1:8090
export MEDIGO_LOG_LEVEL=debug
export MEDIGO_LOG_PRETTY=true
export MEDIGO_FILES_MAX_UPLOAD_BYTES=33554432          # 32 MiB, the default
export MEDIGO_FILES_ALLOWED_MIME=application/pdf,image/jpeg,image/png,image/webp,image/heic,image/heif,image/tiff,image/gif,text/plain
export MEDIGO_RETENTION_TRASH_DAYS=30
export MEDIGO_LABS_MAX_SERIES_POINTS=500               # new in this phase
```

> **A note that will otherwise cost somebody an afternoon.** `text/csv` is **not** in the default
> allowlist and adding it achieves nothing: a CSV file has no byte signature that distinguishes it
> from prose, so it sniffs as `text/plain` and is accepted by that entry. The stored `mime` of a
> CSV is `text/plain`, and that is correct under FR-051 — the content decides (research D-14).

---

## 2. Seed a deterministic instance

```bash
task seed        # medigo seed
```

The seed creates:

- **Patient A ("Amara's father")** — 8 lab results spanning two years, each carrying the same three
  components: two numeric and one categorical, with **two of the numeric readings recorded in a
  different unit** from the rest so the multi-unit path is exercised. One result is a scalar. One
  result has no dates at all. One panel has 10 components of which exactly 3 are out of range. One
  component has a value but no reference range at all. Three documents attached across three
  different record kinds; one of them already in the trash.
- **Patient B** — deliberately **empty**: no lab results, no documents. This is what the Playwright
  landmark assertions exercise on the empty-state path.

Sign in with the seeded account printed by `task seed`.

---

## 3. Verify US1 — record a panel and see what is off

1. Open `/lab-results` with patient B selected. You should see the **empty state** inside
   `region[name="Lab results"]`, with the action to record the first result — not a blank page.
2. Record a result supplying **only** a test name. It saves, appears at the top, and every value
   you left blank is **absent** from the detail view — not shown as a blank or a zero.
3. Edit it and supply a collection date **earlier** than the ordered date, and a status outside the
   published set, in one submission. The save is refused and **both** problems are reported at
   once, each beside its own field, with everything else you typed still on the form.
4. Fix those and add ten components with names, values, units and ranges, three of them
   deliberately outside their ranges. Save.
5. On the detail page confirm: all ten components in the order you entered them; the three
   out-of-range ones marked **by a text marker and a shape, not by colour alone**; the other seven
   unmarked; and the panel reporting that three components are out of range.
6. Add a component with a value and **no reference range**. Confirm it reads "not assessed against
   a range" and is **never** presented as normal.
7. Edit the panel: remove one component, change one, add one. Confirm the stored set is exactly
   what you submitted.
8. Try to give the result an overall value **while** it has components. The save is refused with an
   explanation that a result holds one or the other, and **neither part is discarded** — both are
   still on the form.
9. Remove every component and give it an overall value instead. The conversion is allowed. Convert
   back.
10. Open the same result in two browser tabs. Save in the first, then save in the second. The
    second is refused, tells you the record changed underneath you, and shows the current values.
11. Delete it. The confirmation must name the result and warn that it **and its components** cannot
    be recovered, and that its documents move to the trash.

```bash
# and the refusal that matters most:
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $OTHER_ACCOUNT_TOKEN" \
  http://127.0.0.1:8090/api/v1/records/lab-results/$SOME_ID
# 404 — and byte-identical to a request for an id that never existed
```

---

## 4. Verify US2 — documents

1. On any record's detail page, attach a document with a description and a category. Confirm it
   appears on that record **and** in `/documents`, showing its original name, size, type, who
   attached it and when.
2. Try a file larger than the limit. It is refused **before anything is stored**, the message
   states the limit, and nothing partial exists afterwards:

   ```bash
   head -c 40000000 /dev/urandom > /tmp/too-big.pdf
   # upload it in the UI; then confirm nothing was stored:
   ls -la pb_data/storage/*/ | tail
   ```

3. Rename a PDF to `.png` and upload it. It is accepted **as a PDF** — the content decides. Now
   rename an HTML file to `.pdf` and upload it: refused, naming the accepted types.
4. Upload an empty file: refused with an explanation.
5. View a PDF and a JPEG inline. Confirm they display without downloading. Then confirm a `.tiff`
   offers **only** downloading.
6. Download one and compare bytes:

   ```bash
   sha256sum /tmp/original.pdf
   curl -s -H "Authorization: Bearer $TOKEN" \
     "http://127.0.0.1:8090/api/v1/attachments/$ID?disposition=attachment" | sha256sum
   # identical
   ```

7. Replace one with a corrected version. Confirm the new one is on the record with the same
   description and category, and the old one is in the trash.
8. Change a description and category. Confirm the original name, size and type did **not** change —
   there is no field to change them with.
9. Delete one. Confirm the confirmation states the retention window, that it leaves the record's
   listing and the library, and that it appears under `?deleted=true` with its days remaining.
10. Restore it. It returns to the record it was attached to.
11. Delete a document, then delete the record it was attached to, then try to restore the document.
    Refused with an explanation that the record no longer exists — and the download is still
    offered.
12. Delete a record with three documents attached. The record is gone permanently; all three are in
    the trash.
13. Confirm a document with no preview (a PDF) shows a **type icon**, not a broken image. Open the
    browser console: it must be empty.
14. Confirm the URL is not a credential:

    ```bash
    curl -s -o /dev/null -w '%{http_code}\n' \
      "http://127.0.0.1:8090/api/v1/attachments/$ID"                       # 401 — signed out
    curl -s -o /dev/null -w '%{http_code}\n' \
      -H "Authorization: Bearer $OTHER_ACCOUNT_TOKEN" \
      "http://127.0.0.1:8090/api/v1/attachments/$ID"                       # 404 — not 403
    ```

15. Confirm the right retrievals were audited — and only those — and that **no file name reached
    the trail**:

    ```bash
    task run 2>&1 | grep -i 'riverside-labs-2026'   # must print NOTHING
    # then read the audit trail through the admin UI or the phase-006 reader:
    #   * the owner's own GETs of content or a preview (steps 8-13) leave NO read_sensitive row
    #   * the superuser GET of the same document leaves EXACTLY ONE, with no name and no
    #     description
    # An owner's own reads are deliberately unrecorded: see FR-076 and 005 D-25.
    ```

---

## 5. Verify US3 — trends

1. Open `/labs/trends` for patient A. Every distinct component ever recorded is listed **once**,
   with its latest value, unit, latest status, reading count and latest date.
2. Select the numeric component that has readings in two units. Confirm the page **tells you** more
   than one unit exists, lets you choose, states which unit is being shown, and that **no value has
   been converted** — cross-check two readings against the raw rows.
3. Confirm the readings appear in date order with the range each was measured against, and that
   out-of-range readings are marked by shape and by text.
4. Confirm the summary reports the count, the earliest and latest dates, the latest value, the
   minimum, the maximum, the mean, how many fell in range, and a direction — **with the rule that
   produced it stated on the page**.
5. Select a component with one reading. Confirm the page says there are not enough readings to
   compare and still shows the reading.
6. Select the categorical component. Confirm a value history with per-value counts and **no** chart,
   mean or direction.
7. Apply a date range. Confirm the summary figures change with the series, not independently of it.
8. Delete one of patient A's lab results in another tab. Confirm the open trend stops including its
   readings and that the count and the summary follow.
9. Request another account's patient:

   ```bash
   curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $OTHER_ACCOUNT_TOKEN" \
     "http://127.0.0.1:8090/api/v1/lab-components?patient=$PATIENT_A"    # 404
   ```

---

## 6. Verify US4 — the catalogue

1. Start typing a common test's name in a component row. Nothing happens for the first two
   characters; suggestions appear from the third, within a second.
2. Pick one. Confirm the name, unit, category and typical range are filled in **and remain
   editable**.
3. Save, then add a second component for the same test under a different spelling and pick the same
   catalogue entry. On `/labs/trends`, confirm the test is listed **once with two readings**.
4. Type three characters that match nothing. Confirm the message is "nothing matched that" — which
   is a **different message** from "the catalogue has not been loaded".
5. Enter a component the catalogue does not contain. It saves without complaint and trends on its
   own.
6. Confirm the catalogue is read-only. There is nothing to click, and there is no route either:

   ```bash
   curl -s -o /dev/null -w '%{http_code}\n' -X POST \
     -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"name":"x"}' \
     http://127.0.0.1:8090/api/v1/catalog/lab-tests            # 404/405 — the route does not exist
   ./medigo routes | jq '.[] | select(.path | startswith("/api/v1/catalog")) | .method'
   # must print only "GET"
   ```

7. Confirm reading it discloses nothing: fetch the same query as two different accounts and diff
   the responses. They must be byte-identical.

---

## 7. Verify US5 — relating a result to other records

1. On a lab result, link it to a condition, an encounter, a medication, a procedure and a
   treatment. Confirm all five appear on the result **and** that the result appears on each of the
   five.
2. Try to link it to a record belonging to a different patient. Refused, and the refusal discloses
   nothing about whether that record exists.
3. Remove one link. Both records survive unchanged apart from no longer referring to each other.
4. Delete one of the linked records. The lab result is untouched apart from losing that connection.
5. Delete the lab result. All five linked records survive intact.
6. Confirm every link and unlink appears in the activity trail as a change to the **lab result**.

---

## 8. Run the gates

```bash
task check                # gen, vet, lint, test -race
task test                 # go test -race -count=1 ./...
task lint                 # golangci-lint v2, incl. depguard and forbidigo
task openapi              # regenerate api/openapi.json
git diff --exit-code api/openapi.json     # MUST be clean — an unintended API change is a diff, not a surprise
task routes               # medigo routes | jq
task test:e2e             # Playwright, both viewports
task test:scale           # build-tagged: 5,000 results, a 100-component panel, 500 readings, 2,000 documents
task test:phileak         # build-tagged: no clinical or identifying content in logs, metrics, traces or Sentry
task purge                # runs the trash sweep once, from the CLI
```

Gate-specific expectations for this phase:

| Gate | Must show |
|---|---|
| `task routes` | 9 `api`, 4 `page`, 4 `page_action` entries new in this phase; only `GET` under `/api/v1/catalog` |
| `git diff api/openapi.json` | clean after `task openapi`; nine new `operationId`s, no `page_action` route present |
| `task test:e2e` | 8 smoke cases green at both viewports, zero console errors, zero page errors, **zero failed network requests** |
| `task lint` | zero findings; in particular no import of `internal/domain/clinical/units` from any `lab*` package, and no call to `NewFileToken` or `NewFileFromURL` |
| boot | no warning about an unprotected file field; the app refuses to start if `attachments.file.Protected` is flipped to false |

---

## 9. Regenerating the test fixture

Any migration in this phase changes the schema that `tests.NewTestApp` clones. **Forgetting this
makes every integration test run against the old schema and fail in a way that looks like a code
bug.**

```bash
task fixture:regen        # runs migrations against a clean db + medigo seed, rewrites internal/testdata/pb_data
git status internal/testdata/pb_data     # commit the result
```

Keep the fixture small: `NewTestApp`'s cost is dominated by the directory copy. Binary file
fixtures live in `internal/testdata/files/` and are all tiny; the at-the-limit upload case is
**generated** at test time, never committed.

---

## 10. Troubleshooting

| Symptom | Cause |
|---|---|
| `go.mod requires go >= 1.27` | `GOTOOLCHAIN=local` is set. Unset it. |
| Integration tests fail on a field that exists in the migration | The fixture is stale. `task fixture:regen`. |
| A thumbnail 404s in the browser and the smoke gate goes red | `has_preview` was not written, or the listing guessed it from the MIME type. It is a stored column for exactly this reason. |
| An SSE stream or a slow upload dies at five minutes | PocketBase's hardcoded `WriteTimeout` override was lost or narrowed to stream routes. It is one `*http.Server`, and it bounds uploads too (research D-33). |
| The inline viewer shows a blank frame | The page CSP is missing `frame-src 'self'`, or the attachment response's `frame-ancestors` is `'none'` instead of `'self'`. |
| A PDF opens as a download instead of inline | Its sniffed `mime` is not `application/pdf` — check what was actually stored, not what the file is called. |
| `ApiScenario` tests recurse until the stack overflows | A `tests.TestApp` was shared across scenarios. Never do that (VERIFIED-SOURCE-FACTS FACT 7). |
| A CSV upload is refused | `text/csv` was added to the allowlist instead of `text/plain`. See §1. |
