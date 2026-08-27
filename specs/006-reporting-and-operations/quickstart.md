# Quickstart: Reporting and Operations (phase 006)

How to run this phase and verify it by hand, end to end. Nine walkthroughs, one per user story, then
the gates in the order CI runs them.

Everything below assumes the repository root `/Users/krzysztof.wiatrzyk/private/monorepo/medikube`.

---

## 0. Preconditions (30 seconds, and skipping them wastes an hour)

```bash
go version                      # must print go1.27.x — PocketBase v0.40.1 does NOT build on 1.26.5
echo "${GOTOOLCHAIN:-unset}"    # must be unset; GOTOOLCHAIN=local fails with a misleading error
node --version                  # build-time only, for the Playwright gate; never in the runtime image
```

Two things that will otherwise bite:

- **`MEDIKUBE_STATE_DIR` must not be inside `MEDIKUBE_DATA_DIR`.** The restore journal lives there
  precisely because a restore replaces `pb_data` wholesale ([research D-23](./research.md#d-23)), and
  the boot validation refuses to start if you nest them.
- **The PDF spike must have passed.** `task spike:pdf` builds a two-page document with
  `AddUTF8FontFromBytes`, `SetHeaderFunc`, `SetFooterFunc`, `AliasNbPages`, `Line`, `Rect`,
  `SetDashPattern` and `RegisterImageOptionsReader` on `CGO_ENABLED=0`. If it fails, the fallback is
  `signintech/gopdf` and only `internal/render/pdf` changes
  ([research D-01](./research.md#d-01)) — do not start the renderer until it is settled.

---

## 1. Build and run

```bash
task gen                        # templ + Tailwind; every other task depends on it
task build
task seed                       # deterministic fixtures, including an empty account and a trail
task seed -- --print-ids        # the ids the walkthroughs below paste
task run
```

Minimum environment for a hand-run:

```bash
export MEDIKUBE_PUBLIC_URL=http://127.0.0.1:8090
export MEDIKUBE_DATA_DIR=./pb_data
export MEDIKUBE_STATE_DIR=./medikube_state      # NOT inside pb_data
export MEDIKUBE_RETENTION_EXPORT_DAYS=7
export MEDIKUBE_BACKUP_WARN_AFTER=168h
export MEDIKUBE_LOG_PRETTY=true
```

To watch retention actually happen without waiting a week, run the walkthroughs with
`MEDIKUBE_RETENTION_EXPORT_DAYS=1` and use `task purge` to run the sweeps on demand
(§9 below).

---

## 2. The seeded cast

| Account | Password | What it is for |
|---|---|---|
| `owner@medikube.local` | `medikube-dev` | The populated account: people, records, documents, 3 saved reports, 5 jobs in 5 states |
| `empty@medikube.local` | `medikube-dev` | Holds **nothing**. Every empty state, and the second Playwright pass |
| `admin@medikube.local` | `medikube-dev` | `role = admin`. The operator surface |
| `admin2@medikube.local` | `medikube-dev` | The second administrator, for the last-administrator refusals |
| `disabled@medikube.local` | `medikube-dev` | `disabled_at` set |
| `mustchange@medikube.local` | `medikube-dev` | `must_change_password = true` |
| PocketBase superuser | printed by `task seed` | The **break-glass** credential. Not the administrative tier |

The seeded person `Amina Zayd` carries 12 readings of one lab component in `mmol/L`, 3 of the **same**
component in `mg/dL`, and 1 reading of a second component — exactly the shape US4's independent test
needs. A second person carries a name, a tag and a document description in Arabic, Hebrew, CJK and
`<script>` text.

---

## 3. Story 1 — walk out with the paper (about 3 minutes)

1. Sign in as `owner@medikube.local`, open **`/reports`**, choose Amina.
2. Confirm the builder shows **every kind with a count**, including kinds at zero, and a total.
3. Tick four kinds, set the range to the last twelve months, add the `rheumatology` tag. Watch the
   resolved count change as you go — it should settle in well under a second.
4. Press **Produce**. You should be told immediately that it is being prepared, with a position if
   anything else is queued. **Navigate away and come back**; the progress is still there.
5. Download the PDF. Check, on the document itself:
   - page 1 identifies Amina, states the production moment, and states the criteria **in words**;
   - there is one section per selected kind, in the documented order;
   - a kind that matched nothing says so explicitly rather than being missing;
   - **every** page carries her identity and `Page N of M`;
   - the second person's records appear nowhere.
6. Now switch to the person whose name is in Arabic and produce a report. Page 1 must carry the
   counted, plain-language note about characters that could not be rendered. Nothing may be silently
   dropped or substituted.
7. Try to produce a report for a person you cannot reach:

   ```bash
   curl -si -X POST localhost:8090/api/v1/exports -H 'Content-Type: application/json' \
     -H "Authorization: Bearer $OWNER_TOKEN" \
     -d '{"kind":"report","patient":"recNOTYOURSxxxxx","criteria":{"kinds":["medication"]}}'
   # 404, byte-identical to a person that does not exist — and an access_denied row appears in /admin/audit
   ```

8. Copy the download URL, sign out, and paste it. Nothing is returned, and the attempt is recorded.

---

## 4. Story 2 — take everything with me (about 4 minutes)

1. As `owner@medikube.local`, open **`/exports`**, request everything with documents included.
2. Confirm the acknowledgement is immediate, the progress moves, and the finished size is reported.
3. **Stop the application**, unzip the archive somewhere else, and read it:

   ```bash
   unzip -l medikube-export-*.zip | head -30
   jq '.format_version, .produced_at, .kinds, .documents_included' manifest.json
   ```

   `manifest.json` must describe **every** other file in the archive. `docs/export-format-v1.md`
   must describe every key you see, and nothing you do not.
4. Prove nothing secret is in it:

   ```bash
   grep -rl 'tokenKey\|MEDIKUBE_\|BEGIN PRIVATE KEY' . || echo "clean"
   ```

5. Restart, request a second export while the first is running, and confirm it is accepted and shows
   its **position** rather than looking stalled.
6. Cancel a running export. Confirm it stops, nothing partial is downloadable, and the cancellation is
   in the trail.
7. Kill the process mid-export and restart it. The job must report itself **failed with a plain
   reason** and be offered for retry — never still "running".

---

## 5. Story 3 — save the question (1 minute)

1. Save the story-1 selection as **"Rheumatology, quarterly"**.
2. Add a new medication to Amina.
3. Reopen the saved report. The count is **one higher**, with nothing edited and no staleness warning.
4. Save a second report under `rheumatology, QUARTERLY` — refused, naming the conflict, nothing
   overwritten.
5. Open the same saved report in two tabs, save in one, then save in the other. The second is refused,
   you are told it changed underneath you, and the current values are shown.
6. Delete the person the saved report names. Reopen it: it says the person is gone, stays editable,
   and refuses to produce. **Documents already produced from it are untouched.**

---

## 6. Story 4 — show how a number moved (1 minute)

1. On `/reports`, open the chart picker for Amina.
2. The component recorded in two units appears **twice**, once per unit, each with its own count.
   The single-reading component appears as **not yet chartable**, with the number it has and the
   number it needs.
3. Pick the `mmol/L` series, produce the report, and confirm the document contains the chart **and a
   table of exactly the points it plots**, with the reference range shown and nothing conveyed by
   colour alone. No `mg/dL` reading appears on that chart, and no value was converted.

---

## 7. Stories 5, 6 and 8 — the operator surface (3 minutes)

Sign in as `admin@medikube.local`.

1. **`/admin`** — every figure has a definition and a computed-at. With MFA and the IP allowlist
   unconfigured, an unmistakable warning names exactly which is missing. The same warning is in the
   boot output — scroll back and check.
2. Take a backup from `/admin/backups`, return to `/admin`, and confirm the **last-backup figure
   moved**. That is the live-versus-refreshed split working.
3. Confirm the retention section lists each window with its value, what it applies to, and **when its
   job last ran and last succeeded**.
4. **`/admin/users`** — disable `owner@medikube.local` while it is signed in elsewhere. Its very next
   request fails, well inside five seconds. Sign in as it with the **wrong** password and then the
   **right** one, and confirm the two answers differ exactly as
   [D-49](./research.md#d-49) specifies and in no other way.
5. Try to demote yourself → refused. Disable `admin2`, then try to demote yourself again → refused,
   naming the last-administrator reason.
6. **`/admin/audit`** — narrow by person, by actor, by action and by date, singly and combined. Page
   through. Confirm no entry shows a name, a value or a file name. Export the narrowing as CSV and
   confirm **exactly one** `audit_export` row appears — and that reading the trail added none.
7. Sign in as `owner@medikube.local` and request `/admin` directly. You get the 404 view, and the
   attempt is in the trail.

---

## 8. Story 7 — the restore, at eleven at night (5 minutes, and worth doing properly)

1. As `admin@medikube.local`, take an archive with the note `before the test`.
2. Add a record afterwards, so there is something to lose.
3. Open the **restore preview**. It must state when the archive was taken, its size, its note, the
   version that produced it, what exists now, and — in words — that everything recorded since will be
   lost.
4. Attempt the restore **without** the phrase → refused, nothing replaced. Attempt it without the
   password → refused.
5. Start an export, then attempt the restore → refused with `job_in_progress`. Cancel the export.
6. Confirm properly. You must receive the **safety copy's reference before anything is replaced**.
7. The instance restarts. Sign back in and confirm:
   - the record you added in step 2 is **gone**;
   - it **is** present in the safety copy;
   - **`/admin/audit` shows the restore, the safety copy and both references** — even though the
     restore replaced the very rows the trail is kept in. This is the single most important assertion
     in the phase ([D-23](./research.md#d-23)).
   - your Sentry DSN and every other `,unset` secret survived the restart
     ([D-24](./research.md#d-24)). Check `/admin` rather than guessing.
8. Upload an archive with no `medikube.json`. The preview says "version unknown" and the restore is
   refused unless you pass `accept_unknown_version`.

---

## 9. Story 8 — retention, without waiting a week

```bash
MEDIKUBE_RETENTION_EXPORT_DAYS=1 task run &
# produce a report and an export, then move the clock or wait, then:
task purge                                  # runs every sweep once, synchronously
```

Confirm: both artifacts are gone from storage, both requests read **expired with the window that
applied**, both offer re-production, and downloading either is a plain "it expired" — not an error.
Then confirm `/admin` shows each window's last run and last success, and that a deliberately failed
sweep appears **exactly once** in the attention list and in the trail, and is retried on the **next
scheduled run** rather than in a loop.

---

## 10. The gates, in the order CI runs them

```bash
task check                                   # fmt, vet, golangci-lint v2, go test -race -count=1 ./...
task lint:isnull                             # no literal IS NULL in any store package
task lint:noconvert                          # no unit-conversion function exists anywhere
task openapi && git diff --exit-code api/openapi.json
task routes                                  # the inventory the browser gate is derived from
task test:e2e                                # the whole-product sweep, both viewports, populated + empty
task test:phileak                            # zero sentinels in logs, metrics, traces, Sentry
task test:netgate                            # zero non-loopback dials with nothing configured
task test:scale                              # every published budget, at the documented volumes
go test -tags slowsse ./internal/web/stream/...   # >11 minutes; merge-to-main and nightly only
task docker:build                            # from the repository root — a bare `docker build .` fails
```

---

## 11. Things that will look like bugs and are not

- **The trail records nothing when you read it.** By design (FR-075). Exporting it records exactly
  one entry. The handbook says so, so an absence of read entries is never mistaken for an absence of
  reads.
- **You read your own person's records and no entry appeared.** Also by design (FR-115). An entry is
  written when the reader is **not** the owner, or when the break-glass credential is used.
- **An administrator cannot download another account's document or archive.** Not a permissions bug:
  FR-013 says so in as many words. An administrator sees counts, never contents.
- **`/admin` shows database bytes with an age of up to 15 minutes.** Deliberate — a walk of the
  document store is not on the request path ([D-16](./research.md#d-16)). The counts beside it are
  live.
- **Arabic renders in isolated forms in the PDF.** A stated, documented limitation
  ([D-04](./research.md#d-04)): the export always carries the exact text, and the document counts and
  states the affected entries on page 1. It is in the handbook as a known limitation, not a defect to
  be discovered by a user.
- **A produced document appears on `/reports`, not `/exports`.** One resource, two pages, one queue
  ([D-53](./research.md#d-53)).
- **There is no `/trash` page.** Deleted documents are recovered on `/documents?deleted=true`, where
  phase 004 put them. `/admin` shows the instance-wide count and bytes and links there
  ([D-14](./research.md#d-14)).
- **The archive download is a form POST.** It has to be: password re-entry, per-request
  authorization, and no credential in a URL ([D-27](./research.md#d-27)).
