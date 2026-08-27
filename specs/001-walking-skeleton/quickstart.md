# Quickstart: running and verifying the Walking Skeleton by hand

How a developer brings the very first MediGo instance up from an empty directory and convinces
themselves it actually works — including the claims that are easy to state and hard to believe:
that the auto-generated CRUD API really is gone, that a refusal really is byte-identical to a
not-found, that the live stream really survives past five minutes, and that the render gate really
goes red when the UI breaks.

There is no previous phase. Everything here starts from a checkout.

Everything below runs from `medigo/` unless stated. Every routine action is a Task, per the
monorepo convention: you run tasks, not remembered command lines.

---

## 0. Prerequisites

| Need | Why | Check |
|---|---|---|
| **Go 1.27+** | PocketBase v0.40.1 declares `go 1.27` and imports `encoding/json/v2` in 67 non-test files. **Go 1.26.5 cannot build it** — this was verified by trying. | `go version` |
| `GOTOOLCHAIN` **unset** (or `auto`) | `GOTOOLCHAIN=local` on a 1.26 machine fails with the misleading "built with go1.26 < targeted go1.27". CI must not set it. | `go env GOTOOLCHAIN` |
| Task | the entry point for everything | `task --list` |
| Node + Playwright browsers | **build-time only**; Node never enters the runtime image | `npx playwright --version` |
| Docker | for the image build and the distroless checks | `docker version` |
| `curl` and `jq` | the manual API walkthrough below | |

```bash
task install:tailwind          # pinned TAILWIND_VERSION; the x64 asset, not amd64
task install:golangci-lint     # v2 schema — v1 does not understand this config
npx playwright install --with-deps chromium
```

---

## 1. Build and bring it up

```bash
task gen        # templ generate + tailwind; vet/lint/test/build all depend on this
task build
task migrate    # three migrations: users_medigo_fields, medications, audit_events
task seed       # deterministic demo data — same accounts, same ids, every time
task run
```

`task run` needs the environment. The minimum for a local instance:

```bash
export MEDIGO_ENV=dev
export MEDIGO_DEV=true
export MEDIGO_DATA_DIR=./pb_data
export MEDIGO_HTTP_ADDR=127.0.0.1:8090
export MEDIGO_PUBLIC_URL=http://127.0.0.1:8090
export MEDIGO_LOG_LEVEL=debug
export MEDIGO_LOG_PRETTY=true
export MEDIGO_AUTH_REGISTRATION_OPEN=true      # closed by default; open it to exercise sign-up
export MEDIGO_SESSION_TTL=168h                 # 7 days
```

`MEDIGO_DATA_DIR` is **required**. Leaving it unset makes PocketBase put `pb_data` next to the
binary, which in the distroless image is a read-only layer; the config validator refuses to start
rather than let you discover that in production.

### Three things you should see at boot, and two you should not

**Should see:**

- exactly **one** startup line at info, naming the version, the data directory, the listen address
  and the applied migration count. Nothing else at info until the first request.
- the migration log naming all three migrations in order.
- a **loud warning** about the admin UI, because locally the superuser IP allowlist is empty and
  superuser MFA is off. That warning is expected here and must never be expected in production.

**Should not see:**

- **the process starting at all** if any collection has a non-nil API rule, or `Batch` is enabled,
  or any file field is unprotected. Those are boot assertions and they refuse to serve. If you
  want to watch one work, set `ListRule` to `types.Pointer("")` on `medications` in migration 2
  and try to boot.
- **any log line that is not zerolog JSON.** No `slog` text, no `[pocketbase]` prefixes, no
  `log.Printf`. If you see one, the bridge has a hole and that is a Principle VI failure, not a
  cosmetic one.

Sign in as the seeded account:

```
amara@example.test / medigo-dev-password
```

Account B is `bo@example.test` (one medication), account C is `chen@example.test` (none — the
empty state).

---

## 2. Walk each user story

### US1 — Medications, the vertical slice (P1)

The whole point of the phase: one record kind proving every layer.

1. Open `http://127.0.0.1:8090/medications`. You get `region[name="Medications"]` with amara's
   list — active, completed and stopped, plus one row with every optional field empty.
2. Create one. Name, dose, form, frequency, started-on. It appears without a full page reload.
3. Edit it. Change the dose. Note the response carries a **new `ETag`**.
4. Open the same medication in a second tab, edit in the first, then try to save the stale second
   tab. You get **412** with the current representation in the body — not a silent overwrite, and
   not a bare error you cannot recover from.
5. Delete it, via the **rendered** confirmation (`region[name="Confirm delete"]`) — not a browser
   dialog. It is **hard deleted**: no `deleted_at`, no tombstone, no way to get it back. Confirm
   with `sqlite3 pb_data/data.db 'select count(*) from medications where id = "..."'`.
6. Sign in as `chen@example.test`: `region[name="Medications"]` is present and contains the empty
   state — the region is there, not replaced.

### US2 — Accounts (P2)

1. `/register` with a 7-character password: refused, with the rule stated next to the field.
2. Register properly, sign in, sign out, sign in again. The session cookie is `HttpOnly`,
   `Secure` (in production), `SameSite=Lax`.
3. Change the password in `/settings`. **Every other session for that account stops working** —
   check by keeping a second browser signed in and reloading it.
4. Delete the account. The confirmation phrase is exactly `DELETE MY ACCOUNT`; anything else is
   refused. Then:

```bash
sqlite3 pb_data/data.db "select count(*) from medications where owner = '<deleted id>'"   # 0
sqlite3 pb_data/data.db "select count(*) from audit_events where actor = '<deleted id>'"  # > 0
```

The medications cascade; the audit rows **do not**, and their `actor` becomes a dangling id on
purpose. An audit trail that erases itself when the account goes is not an audit trail.

**Recovery and confirmation (FR-073 … FR-077).** A local instance has no mail configured, and that
is the first thing to verify rather than a reason to skip this:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8090/api/v1/auth/password-reset \
  -H 'content-type: application/json' -d '{"email":"amara@example.test"}'      # 503
```

`503 mail_unconfigured`, **not** a cheerful `202`. That is FR-076: an instance that cannot send
mail must not tell somebody a message is on its way. The boot output has been warning about the
same condition since start-up.

Then point it at a sink — `MailHog`, `mailpit`, anything that speaks SMTP — by filling in SMTP in
the admin UI at `/_/` (it is PocketBase settings state, not a `MEDIGO_` variable; that carve-out is
in the constitution's Technology Constraints), and repeat:

```bash
for addr in amara@example.test nobody@example.test; do
  curl -s -X POST localhost:8090/api/v1/auth/password-reset \
    -H 'content-type: application/json' -d "{\"email\":\"$addr\"}"; echo
done
```

Both answers are `202` and **identical**. Only the first produces a message in the sink. Take the
token out of that message, then:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  localhost:8090/api/v1/auth/password-reset/confirm -H 'content-type: application/json' \
  -d '{"token":"<from the mail>","password":"a-new-one","password_confirm":"a-new-one"}'   # 204
```

Then check three things: the new password signs in; a browser that was signed in before the reset
is now `401`; and re-posting the **same** token is `400 invalid_token` — the same answer an expired
or forged one gets. Finally open `/reset-password/rubbish` and confirm you get an ordinary page
saying the link is no longer usable, at `200`, not an error view: that page *is* the smoke case.

### US3 — Privacy, the one that has to be checked rather than believed (P3)

This is the story the whole architecture exists for, so verify it directly.

```bash
# as bo, request one of amara's medications and something that never existed
curl -s -o /tmp/a.json -w '%{http_code} %{time_total}\n' \
  -b bo.cookies http://127.0.0.1:8090/api/v1/records/medications/<amara-med-id>
curl -s -o /tmp/b.json -w '%{http_code} %{time_total}\n' \
  -b bo.cookies http://127.0.0.1:8090/api/v1/records/medications/zzzzzzzzzzzzzzz

diff /tmp/a.json /tmp/b.json && echo "IDENTICAL"
```

Both are `404`. `diff` must be empty — **byte-identical, not merely similar** — and the two times
must be comparable. A 403 here, or a different body, or a measurably slower refusal, tells a
stranger that a record exists. That is the leak Principle VII exists to prevent.

Then confirm the refusal was *recorded*:

```bash
sqlite3 pb_data/data.db \
  "select action, resource_type, resource_id from audit_events order by created desc limit 3"
```

You should see `access_denied` for the first request and **nothing** for the second — a
not-found is not a denial.

### US4 — The shell (P4)

1. Every page has banner, navigation, main, contentinfo. `Tab` from the top reaches "Skip to
   content" first.
2. Open the browser console and click through all nine pages. **Zero** errors, **zero** warnings,
   **zero** CSP violations. If you see `Refused to execute inline script`, somebody used a
   Datastar inline-script SDK helper (`ConsoleLog`, `Redirect`, …) and that is banned.
3. Resize to 390×844. Nothing overflows horizontally; the navigation collapses; the landmarks are
   still all there.
4. Toggle dark mode in `/settings`, reload: **no flash of the wrong theme**, because the class is
   server-rendered on `<html>`.
5. Disable JavaScript and reload: you get a plain statement that MediGo requires it, inside
   `main` — not a blank page.

### US5 — Operations (P5)

```bash
curl -s localhost:8090/api/v1/healthz | jq          # {"status":"ok","version":...}
curl -s localhost:8090/api/v1/readyz  | jq          # {"status":"ready","checks":{...}}

chmod 000 pb_data/data.db
curl -s -o /dev/null -w '%{http_code}\n' localhost:8090/api/v1/healthz    # 200 — still alive
curl -s localhost:8090/api/v1/readyz | jq                                  # 503, database: error
chmod 644 pb_data/data.db
```

Read the `readyz` failure body carefully: it says `"database": "error"` and **nothing else**. No
path, no DSN, no driver message. Those went to the log stream with the request id.

Now `Ctrl-C` while holding a request open. `readyz` flips to `draining` first, in-flight work
finishes, then the process exits. It does **not** get cut off after one second, which is what
would happen without MediGo's `-10000` terminate handler running ahead of PocketBase's hardcoded
one-second window.

### US6 — The gates (P6)

```bash
task lint          # depguard + forbidigo
task test
task openapi && git diff --exit-code docs/openapi.json
task smoke         # Playwright, both viewports
```

Now break things on purpose, because a gate nobody has watched fail is a gate nobody should trust:

```bash
# 1. import PocketBase from a domain package
sed -i '' 's|^import (|import (\n\t_ "github.com/pocketbase/pocketbase/core"|' internal/domain/medication/medication.go
task lint          # must fail: depguard, Principle II

# 2. call app.Logger() anywhere
task lint          # must fail: forbidigo, Principle VI

# 3. remove a landmark
task smoke         # must FAIL

# 4. add a console.error to one page
task smoke         # must FAIL
```

**If steps 3 and 4 pass, the render gate is decorative.** This is the check most likely to be
skipped and the one whose absence makes every other UI claim in this phase worthless.

---

## 3. Verify the hard-to-believe claims

### The auto-generated CRUD API is actually gone

```bash
# anonymous, and as a signed-in non-superuser
curl -s -o /dev/null -w '%{http_code}\n' \
  localhost:8090/api/collections/medications/records            # 404
curl -s -o /dev/null -w '%{http_code}\n' -b amara.cookies \
  localhost:8090/api/collections/medications/records            # 404
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  localhost:8090/api/batch                                      # 404
```

### …but PocketBase's auth endpoints are still reachable

```bash
curl -s localhost:8090/api/collections/users/auth-methods | jq   # 200, this must work
```

The lockdown is scoped to the `/records` subtree, not to `/api/collections/`. If `auth-methods`
404s, the middleware's prefix match is too greedy and every auth flow in phases 002-006 is broken.

### The stream survives past five minutes

```bash
curl -N -b amara.cookies "localhost:8090/api/v1/streams/records?kind=medications" \
  | ts '[%H:%M:%S]' | head -40
```

Leave it for **six minutes**. Heartbeats (`datastar-patch-signals` carrying `stream_beat`) must
keep arriving. PocketBase hardcodes `WriteTimeout: 5 * time.Minute` in a struct literal with no
config field, so without the two fixes this dies at exactly five minutes — **and passes every
test shorter than that.** CI runs a job longer than five minutes for this reason.

### One subscriber never sees another's records

Two terminals, two accounts, both streaming. Create a medication as amara. Bo's stream must
receive **zero** frames. This is the test that catches an authorization check hoisted out of the
per-event loop.

### Files are never served by PocketBase

```bash
curl -s -o /dev/null -w '%{http_code}\n' \
  localhost:8090/api/files/medications/<id>/<filename>           # 404
```

Every file field is `Protected: true` and files are served only from MediGo's own `/api/v1`
routes, with authorization applied. PocketBase's file-token mechanism is not used and is not
allowed to be — a file token is a bearer credential for patient data with no authorization
checkpoint behind it. (No file fields ship in this phase; the assertion and the route ban do,
because phase 002 adds one.)

---

## 4. The container

```bash
task docker:build
docker run --rm -p 8090:8090 \
  -e MEDIGO_DATA_DIR=/data -e MEDIGO_ENV=dev -v medigo-data:/data medigo:dev
```

Checks worth doing once:

```bash
docker run --rm --entrypoint /medigo medigo:dev healthcheck ; echo $?   # non-zero, nothing running
docker inspect medigo:dev | jq '.[0].Config.User'                       # "65532:65532"
docker run --rm --entrypoint sh medigo:dev -c ls                        # fails: no shell
```

The image is distroless, non-root by numeric uid, `CGO_ENABLED=0`, and declares **no**
`HEALTHCHECK` — that is the house pattern, and `medigo healthcheck` exists precisely because there
is no `curl` or `wget` inside to write one with.

If the build fails with a "file not found" that makes no sense, the cause is almost certainly
`/.dockerignore`: it is a deny-everything allowlist and `!medigo/` must be readmitted. That change
and the `build-image.yaml` matrix entry are part of this phase.

---

## 5. When something is wrong

| Symptom | Almost certainly |
|---|---|
| `built with go1.26 < targeted go1.27` | `GOTOOLCHAIN=local` |
| the process refuses to boot with a lockdown message | a migration left an API rule non-nil — that is the assertion working |
| a `[pocketbase]` or slog-formatted line in the output | one of the two log-bridge mechanisms is missing, or `Logs.MaxDays` is `0` |
| PB logs vanish entirely | `Logs.MaxDays = 0` — at 0 the internal `BeforeAddFunc` short-circuits and mechanism (b) never fires. It must be `1`. |
| the stream dies at exactly 5:00 | `newStream()` was bypassed, or `se.Server.WriteTimeout` was not overridden |
| `data-on-click` does nothing, silently | Datastar v1 uses a colon: `data-on:click` |
| infinite recursion / stack overflow in tests | a `tests.TestApp` was shared across `ApiScenario` cases. Each case gets a fresh one. |
| a hook you bound never fires | it is an `OnRecord*Request` hook — those live inside the CRUD handlers the lockdown disables |
