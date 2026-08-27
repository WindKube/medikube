# Contract: `GET /api/v1/streams/records` — the Datastar SSE stream

One operation, and the one that carries the most ways to fail silently. Requirements covered:
FR-030, FR-031, FR-032, FR-033, SC-007.

**PocketBase's native realtime is not used**, for three independently verified reasons (research
D-33): under this phase's own lockdown every broadcast is silently skipped because subscription
rules are derived from `nil` `ViewRule`/`ListRule`; PocketBase's event names are not the two
Datastar recognises; and its two-step subscribe handshake is impossible from a Datastar attribute.

## The shape

```
GET /api/v1/streams/records?kind=medications
Accept: text/event-stream
```

**Requires a session.** Anonymous is `401` **before** the stream opens — a 401 delivered as an SSE
frame would be indistinguishable from a working stream that never sends anything.

| Parameter | Values |
|---|---|
| `kind` | comma list of registered path segments. Absent means every registered kind. |

Response headers, all of them set by the mandatory `newStream()` helper:

```
Content-Type: text/event-stream
Cache-Control: no-store
Connection: keep-alive
X-Accel-Buffering: no
```

Registered with `Bind(apis.SkipSuccessActivityLog())` and **exempted from the rate limiter**.

## The frames

Datastar v1 recognises exactly **two** event names. Anything else is silently discarded, so
nothing else is ever sent.

| Event | When | Payload |
|---|---|---|
| `datastar-patch-elements` | a record the subscriber may see was created or changed | the rendered `MedicationRow` component, patched by id with `datastar.WithSelectorID(ids.RecordRow(kind, id))` |
| `datastar-patch-elements` (mode `remove`) | a record the subscriber may see was deleted | a removal of that row's id |
| `datastar-patch-signals` | every 25 seconds | `{"stream_beat": "<RFC3339 UTC>"}` |

The v0.x names — `datastar-merge-fragments`, `datastar-merge-signals`,
`datastar-remove-fragments`, `datastar-remove-signals`, `datastar-execute-script` — **no longer
exist**. Any material using them describes v0.x and must not be followed.

## The rule that makes this safe

**The hub publishes IDs, never record bodies.**

```go
// internal/realtime — imports neither PocketBase nor net/http
type Event struct {
    Kind     kind.Kind
    RecordID string
    OwnerID  string
}
```

A post-commit record hook in `internal/platform/pb` publishes; the per-subscriber handler in
`internal/web/stream` receives, and **for every single event** it:

1. filters on the subscriber's `kind` selection;
2. **re-runs `access.Authorizer.Record(ctx, actor, kind, id, PermView)` for that subscriber**;
3. re-fetches the record;
4. renders `MedicationRow` and patches by deterministic id.

**Step 2 is not an optimisation and must not be hoisted out of the loop.** Fanning out IDs rather
than bodies is what makes per-subscriber authorization possible at all — a hub carrying record
bodies would have to decide at *publish* time who may see them, which is the authorizer's decision
made in the wrong place with the wrong information. Constitution V requires this shape by name and
Principle VII is why.

A subscriber who loses access mid-stream — because the record was deleted, or because their
account was — **stops receiving patches without the stream erroring**. That is the specified
behaviour for the Edge Case "An account is deleted while one of its live views is open
elsewhere": the view stops updating, the staleness detector says so, and the next action lands on
the sign-in page.

**Mandatory test.** Two sessions on two accounts, each streaming; a write on one produces **zero**
frames on the other. This is the test that would catch a hoisted authorization check, and it is
the one phase 002 re-runs patient-scoped and phase 005 re-runs share-scoped.

## Post-commit, or the trail lies

The publisher is bound to `OnRecordAfterCreateSuccess`, `OnRecordAfterUpdateSuccess` and
`OnRecordAfterDeleteSuccess` — **after commit**. A pre-commit hook would render a row for a
transaction that then rolled back, and a live view showing a change that did not happen is worse
than a live view that lags.

`OnRecord*Request` hooks are **never** used: they are bound inside the built-in CRUD handlers the
lockdown disables, so anything placed there is silently dead code. A `forbidigo` pattern enforces
it (research D-14 for the auth-family carve-out).

## The five-minute trap

`apis/serve.go:145-160` constructs the HTTP server as a struct literal with
`WriteTimeout: 5 * time.Minute` and no configuration field. `datastar.NewSSE` sets
`Cache-Control`, `Content-Type` and `Connection` and flushes — it **never touches the write
deadline**. So every long-lived stream dies at exactly five minutes with a write error and the
client reconnect-loops, **and it passes every test shorter than five minutes.**

Two fixes, both applied (research D-34):

1. **Per connection**, in the mandatory `newStream(e)` helper:
   `http.NewResponseController(e.Response).SetWriteDeadline(time.Time{})`. This reaches the real
   `net/http` writer because PocketBase's `router.ResponseWriter` implements
   `Unwrap() http.ResponseWriter`.
2. **Globally**, on the `ServeEvent`: `se.Server.WriteTimeout` is adjusted before the listener
   starts. `core.ServeEvent` exposes `Server *http.Server` as an exported mutable field.

**Every SSE handler goes through `newStream`. There is no second path**, and a lint rule forbids
calling `datastar.NewSSE` outside `internal/web/stream/stream.go`.

**SC-007 is a CI job, not an assertion in a unit test**: a job holds a stream open for **more than
five minutes** and asserts heartbeats keep arriving. It is slow and awkward and it ships anyway,
because without it the fix regresses invisibly the first time somebody refactors the helper
(shared design risk R7).

## Telling the person when it has stopped

**FR-031** requires the person to be told plainly when a live view can no longer be kept current,
"rather than continuing to present data that has quietly stopped changing". The server cannot tell
them — if the stream is dead there is no channel — so the detection is client-side and uses only
**free** Datastar attributes:

- the server patches `$stream_beat` every 25 s;
- the page carries `data-on-interval__duration.10s` comparing `$stream_beat` against the clock and
  setting `$stream_stale` when the gap exceeds 60 s;
- `$stream_stale` reveals a `role="alert"` banner saying the live view has stopped updating, with
  a reload action.

`data-persist`, `data-match-media` and `data-on-raf` are Datastar **Pro** and are not used. The
heartbeat comparison is deterministic, testable without a browser, and cannot go stale the way a
handler bound to an undocumented SSE-error event name would (research D-37).

## Shutdown

`cancelBaseCtx()` fires at the start of PocketBase's terminate sequence and cancels **every**
request context, so the handler selects on `e.Request.Context().Done()` and returns cleanly.
Without that the goroutine leaks until process exit and the client sees a hard reset instead of a
stream close. FR-062's "finish work in flight within a bounded period" depends on it.

## What is deliberately not streamed

- **Not every interaction.** Datastar honours a plain `text/html` response as an element patch, so
  create, edit and delete all use the non-SSE fast path. Streams are reserved for the genuinely
  live list, which keeps the surface exposed to the five-minute trap as small as possible.
- **`datastar.WithCompression` is not used.** PocketBase binds no gzip on application routes
  (verified: `apis.Gzip()` is bound at exactly two sites, both scoped to the `/_` admin tree), and
  a double `Content-Encoding` produces an unreadable stream.
- **No `ConsoleLog`, `ConsoleError`, `Redirect` or any of the inline-script SDK family.** Each
  appends a literal inline `<script>`, each fails under `script-src 'self'`, and each failure logs
  a CSP violation that fails the console gate. A redirect is a plain `303` issued **before** the
  stream opens; a user-visible error goes to `#error-banner`.
