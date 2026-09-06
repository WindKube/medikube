# Quickstart: Clinical Records (phase 003)

How to run this phase's work locally and verify it by hand, end to end. Everything is a `task`;
nobody memorises command lines.

---

## 0. Prerequisites, and the one that bites

```bash
go version        # must print go1.27.x — NOT 1.26.5
```

**Go 1.27 is mandatory and it is not the monorepo's house standard.** PocketBase v0.40.1's
`go.mod` declares `go 1.27` and 67 non-test files import the 1.27 stdlib package
`encoding/json/v2`. On 1.26.5 the build fails with `go.mod requires go >= 1.27`; if you have
`GOTOOLCHAIN=local` set anywhere it fails with a misleading *"built with go1.26 < targeted
go1.27"*. Unset it.

```bash
cd /Users/krzysztof.wiatrzyk/private/monorepo/medikube
unset GOTOOLCHAIN
task install:tailwind          # pinned standalone binary; note the x64-not-amd64 asset name
task install:golangci-lint     # v2; v1 does not understand Go 1.27
```

**T001 verification (2026-09-05).** `go.mod` declares `go 1.27` with `toolchain go1.27.0` right
below it. `Taskfile.yaml` sets `GOTOOLCHAIN: auto` explicitly in its global `env` block (not
unset, but the safe default a CI runner inheriting a stray `local` from its shell cannot
override); its own comment names why. `.github/workflows/go.yaml` sets up Go via
`go-version-file` and never sets `GOTOOLCHAIN` at all — its comment says so in the same breath it
warns against `local`. No workflow under `.github/workflows/` sets `GOTOOLCHAIN=local`.
Precondition holds; nothing to change.

---

## 1. Generate, build, run

```bash
task gen                       # go tool templ generate + tailwind -> internal/web/static/app.css
task build                     # CGO_ENABLED=0, -trimpath, version stamped from git describe
task run
```

`task gen` uses `go tool templ` (the `tool` directive in `go.mod`), **not** an installed
`templ@latest`. That is deliberate: an installed generator drifts from the linked runtime and emits
code that no longer compiles.

On first run the migrations in `internal/store/migrations/` apply in order (`data-model.md` §8) and
create the sixteen new collections. Watch for the boot assertions:

```
INF boot assertion passed: all collection API rules are nil  collections=22
INF boot assertion passed: every file field is Protected      fields=1
WRN superuser MFA is not configured                            <- expected in dev
WRN superuser IP allowlist is not configured                   <- expected in dev
```

If either boot assertion **fails**, the process exits. That is the intended behaviour: a non-nil
API rule re-opens the record CRUD API as a second undocumented public API, and an unprotected file
field is served to anonymous callers.

---

## 2. Seed a chart worth looking at

```bash
task seed                      # medikube seed — deterministic, idempotent
```

The seed creates the Playwright test user, three patients, and records across every one of the
fourteen kinds — **and deliberately leaves `/family-history`, `/equipment` and `/immunizations`
empty**, because the empty-state path is what the browser gate's landmark assertion needs to
exercise (see `contracts/pages.md` §5). It also creates three tags applied across at least eight
kinds, so the tag-usage counts are non-trivial from the first page load.

Sign in at <http://localhost:8090/login> with the credentials the seed prints.

---

## 3. Verify by hand — one pass per user story

Each block below is the story's Independent Test from `spec.md`, done by hand.

### US1 — what would hurt me in an emergency

1. `/allergies` on an empty patient → an explanation and an obvious create action, **not** a blank
   screen (FR-008).
2. Record two allergies: one `penicillin` / `anaphylaxis` / `life_threatening` / `active`, one
   mild and `resolved`. The first is visibly distinguished on the list and on the chart (FR-018).
3. `/conditions` → create one with `status: resolved` and **no** `resolved_on`. Save is refused and
   the message names the missing field (FR-020).
4. Give it an `onset_on` of `2024-06-01` and a `resolved_on` of `2024-01-01`. Refused, **both
   values reported in one response** (FR-013, FR-004).
5. `/emergency-contacts` → create two, mark the second primary. The first stops being primary and
   the UI **says so** rather than silently applying it (FR-051).
6. Sign in as a second account and request any of those ids directly. The response is
   indistinguishable from the record not existing (FR-082).

### US2 — the care I have received

1. `/encounters` → create with a reason and a date; attach a practitioner and a facility from the
   phase-002 directories. Fill `assessment` and `plan`; confirm neither is presented as a diagnosis
   (FR-023).
2. `/procedures` → one `scheduled` with a **future** date (accepted), one `completed` with a future
   date (refused, FR-025). `?scheduled=true` lists the first and each row states its basis.
3. `/treatments` → create with `started_on` and an earlier `ended_on`. Refused with both values
   (FR-013).
4. Open the condition named on the encounter: the encounter is listed on it, without your having
   recorded the relationship twice (FR-021).

### US3 — how I feel and what the numbers say

1. `/symptoms` → record "dizziness" four times on different dates. Four episodes, no definition
   ever created (FR-030). The list states **4 episodes** and the most recent date (FR-031). Delete
   one; the count is 3 immediately (SC-016).
2. `/vitals` → submit with nothing filled in. Refused (FR-034).
3. Submit a systolic with no diastolic. Refused, the missing number named (FR-036).
4. Submit `temperature 250`. Refused, **with the accepted range 25–45 °C named** (FR-035).
5. Switch `unit_system` to `imperial` in `/settings`, reopen the same reading: the same underlying
   value in °F and lb, and nothing stored has changed (FR-037). BMI is shown and is not a stored
   field.

### US4 — prevention and what happened to me

1. `/immunizations` → one with dose number, lot number, manufacturer, site and route. Then try
   `dose_number: 0` → refused (FR-039).
2. `/injuries` → the type list is fixed, includes `other`, and offers no way to add a value
   (FR-040, US4-3). Record laterality `not_applicable` on a non-paired body part (FR-041).
3. `?unresolved=true` excludes a `resolved` injury (US4-5).

### US5 — cover and equipment

1. `/insurance` → record a policy with `coverage` amounts and a `currency`. Omit the currency with
   an amount present → `422` (FR-044).
2. Mark a second policy primary; the first is displaced and the UI explains it (FR-045).
3. `?expiring_within_days=60` lists one and **each row states why** (FR-046).
4. `/equipment` → one with `service_due_on` in the past, one due in 20 days.
   `?service_due_within_days=30` lists both, one `overdue` and one `due_soon` — **distinguished
   per row** (FR-049).
5. `grep -ri 'the member number you typed' <the log output>` finds nothing (FR-047, US5-5).

### US6 — connect the records that belong together

1. From a condition, link two medications. Open either medication: the condition is there, recorded
   once (FR-055, US6-1).
2. Link a symptom to a medication as **suspected cause**, and another as **treats**. The role is
   shown wherever the link is shown (FR-032, US6-2).
3. Try to link a record of patient A to a record of patient B. Refused, and the refusal discloses
   nothing — compare it byte-for-byte with the response for a random 15-character id (FR-057).
4. Attach a medication to a treatment with a course-specific dose and **no** course prescriber.
   The row shows the course dose marked `course` and the medication's prescriber marked
   `medication` (FR-060, US6-5).
5. Attach the same medication to the same treatment again. It updates; there is still one row
   (FR-061, US6-6).
6. Delete the condition. The medications survive and show no dangling link (FR-058, US6-4).

### US7 — tags

1. `/tags` → create "cardiology". Create "Cardiology" → refused as a duplicate (FR-063).
2. Apply it across five kinds. Rename it; every record follows and none loses it (FR-065).
3. Delete it: the confirmation states **how many records carry it** first; afterwards every one of
   those records still exists without the tag (FR-066).
4. Sign in as a second account: the first account's tags are neither offered nor discoverable
   (FR-062).

### US8 — find anything

1. `/search?patient=…` → a term present in three kinds returns three groups, each paged separately
   and each stating whether more exist (FR-072).
2. Remove `?patient=` → refused, with no fallback to whoever is in view (FR-070).
3. Search a term that exists only in another account's records → nothing, and the response is
   identical to a nonsense term (FR-074).
4. `grep -i '<your search term>' <the log output>` finds **nothing** (FR-075).

### US9 — the current picture

1. `/timeline?patient=…` → records of eight kinds interleaved in date order, each stating its kind,
   its summary and its date (FR-076).
2. Narrow to two kinds and a three-month window: the narrowing is visible and removable (FR-077).
3. Create a record with no primary date: the timeline states its date is unknown and does **not**
   put it at the beginning or end of time (FR-077).
4. Every status view (`/conditions?active=true`, `/medications?active=true`,
   `/procedures?scheduled=true`, `/injuries?unresolved=true`, `/allergies?critical=true`,
   `/equipment?service_due_within_days=30`, `/insurance?expiring_within_days=60`) returns exactly
   what the hand narrowing returns and states its basis (FR-078, FR-079).
5. An empty patient shows a helpful empty state on every one of them, not a row of zeros (FR-080).

### US10 — family history

1. `/family-history` → record three relatives, one deceased, one with two conditions each carrying
   an age at diagnosis (FR-052, FR-053).
2. A death year earlier than the birth year → refused with both reported (FR-054).

---

## 4. Verify the live behaviour (FR-010, SC-017)

Open `/conditions?patient=X` in two browser windows. Create a condition in one; it appears in the
other **within 5 seconds without a refresh**.

Then the trap that passes every short test: leave one window open for **more than five minutes**
and create another condition.

> PocketBase's server has a hardcoded 5-minute `WriteTimeout` that silently kills every long-lived
> SSE stream. It is overridden on the `ServeEvent`'s `*http.Server` by phase 001's `newStream()`
> path. **Every test shorter than five minutes passes whether or not the override is present.**

If the second window stops updating at the five-minute mark, the override has regressed. The
in-repo guard is `internal/web/stream/deadline_test.go`; the genuinely long-running CI job belongs
to phase 006.

---

## 5. Run the gates, in the order CI runs them

```bash
task check                       # gen + vet + lint + test
task test                        # go test -race -count=1 ./...
task openapi                     # regenerate api/openapi.json
git diff --exit-code api/openapi.json     # MUST be clean — an unintended API change is a diff
task routes                      # medikube routes -> the JSON inventory
task test:e2e                    # Playwright, both viewports
```

Four gates fail the build independently:

| Gate | Fails when |
|---|---|
| `internal/openapi/gate_test.go` | a registered `operationId` is missing from `api/openapi.json`, or vice versa |
| `internal/records/registry_completeness_test.go` | a `kind.Kind` value lacks a registry entry, a `oneOf` branch with a `kind` discriminator, two page routes, a default sort, a searchable-field declaration, a seed fixture or two smoke cases |
| `e2e/routes.gate.spec.ts` | a route from `medikube routes` with `Page: true` has no smoke case |
| `golangci-lint run` | `depguard` (a service or domain package imported PocketBase, `net/http` or templ), `forbidigo` (`app.Logger()`, `fmt.Print*`, `log.*`, `OnRecord*Request`, the Datastar inline-script family, a Datastar Pro attribute) |

Occasional / nightly:

```bash
task test:scale                                # 50,000 records; asserts SC-002 and SC-003
go test ./internal/testsupport/phileak/...     # FR-094 / SC-012 — drives every operation,
                                               # then asserts no sentinel reaches logs,
                                               # metrics, spans or Sentry
```

### T214 — measured numbers, `task test:scale` (2026-09-06, arm64 dev container)

Every list page, status view and the timeline share the same keyset-paginated
filter builder (`internal/store/filter.go`); `internal/store/timeline` times
that builder directly at 50,000 rows on one patient, which is the number a
list page or a status view pays too — narrowing to one kind or one status
only ever removes rows from the same query:

| Operation | Budget | Measured |
|---|---|---|
| Paging 50,000 timeline rows (`internal/store/timeline`, SC-011) | < 2s | 70.8ms |
| Counting 50,000 timeline rows | < 2s | 3.1ms |
| Grouped search first page, 50,000 indexed rows, `medication` (SC-003) | < 3s | 353µs |
| ...`allergy` | < 3s | 231µs |
| ...`condition` | < 3s | 221µs |
| ...`encounter` | < 3s | 208µs |
| Renaming one tag carried by 500 records (SC-007) | < 2s | 168µs, `O(1)` — the row itself is never rewritten |

All comfortably inside budget; the dominant cost in every scale test is
building the 50,000-row fixture, not the query under test.

Correctness at scale, asserted by the same suite: `TestSearchKindAt50000Rows`
requires every one of the four seeded kinds to come back non-empty (no kind
starved by the others); `TestPagingAndCountingFiftyThousandRowsStaysUnderTwoSeconds`
requires `Count` to equal exactly 50,000; `TestRenamingATagCarriedBy500RecordsTouchesOnlyTheTagRow`
requires all 500 carriers to show the new name, 0 to lose it, and every
carrier's row version to be untouched. Per-symptom episode counts and
per-kind chart counts are proved correct by `internal/store/symptom`'s
`TestAggregateCountsEpisodesAndTracksTheMostRecentOccurrence` and phase 002's
`internal/store/patient`'s chart-summary tests; neither currently has a
50,000-row variant, so their correctness is proven at integration scale, not
at SC-002/SC-003's scale — the derivation is the same SQL aggregate either
way, but a dedicated scale case for them is future work, not present here.

---

## 6. Things that will waste your afternoon if nobody tells you

- **`ExpectedContent` in `tests.ApiScenario` compares against *re-encoded* JSON.** The harness
  normalises the body through `jsontext` first, so a substring you copied from `curl` output may
  not match. Write expectations against the re-encoded form.
- **Never share a `tests.TestApp` across `ApiScenario` cases.** `bindUIExtensions` re-enters on
  every `OnServe` and the handler chain grows until the stack overflows. One app per scenario.
- **`tests.NewTestApp` clones `internal/testdata/pb_data`.** If you add a migration and forget to
  regenerate and commit that fixture, every integration test runs against the old schema and fails
  in a way that looks like your code is wrong.
- **`OnRecord*Request` hooks never fire.** They are bound inside the built-in CRUD handlers, which
  the Principle V lockdown disables. Anything you put there is silently dead code. Use
  `OnRecordAfter*Success`.
- **`data-on-click` silently does nothing.** The v1 delimiter is a colon: `data-on:click`.
  `data-on-load` is now `data-init`.
- **A slice that marshals as `null` instead of `[]` is a wire-shape change.** Go 1.27's
  `encoding/json/v2` is not fully backward compatible here. Every DTO has a round-trip test for
  exactly this reason.
- **The Tailwind release asset for x86_64 is called `x64`, not `amd64`.** The unmapped URL 404s and
  the failure reads like a network blip.
- **Tailwind must be pointed at the `.templ` sources.** Auto-detection skips them because generated
  files are gitignored, and you get a stylesheet with none of the app's utilities in it.
