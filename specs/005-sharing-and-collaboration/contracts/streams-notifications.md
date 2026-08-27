# Contract: the notice stream and the revocation cut-off (operation 67)

## 67. `GET /api/v1/streams/notifications` — SSE

Datastar Server-Sent Events. Opened by the shell on every authenticated page, once per session.

**Request**: `GET /api/v1/streams/notifications`, session required. No query parameters — the stream
is scoped to the authenticated account and nothing else, because a parameter would be a way to ask
about somebody else.

**Response**: `text/event-stream`, opened through phase 001's **mandatory** `newStream()` helper,
which clears the write deadline (PocketBase hardcodes a 5-minute `WriteTimeout` that silently kills
every long-lived stream and passes every test shorter than five minutes), sets
`X-Accel-Buffering: no` and `Cache-Control: no-store`, and marks the request
`SkipSuccessActivityLog`.

**Event types**: exactly the two Datastar v1 names —`datastar-patch-elements` and
`datastar-patch-signals`. Nothing else is recognised by the client
(VERIFIED-SOURCE-FACTS FACT 4). The inline-script SDK family (`ExecuteScript`, `ConsoleLog`,
`ConsoleError`, `Redirect`, `DispatchCustomEvent`, `ReplaceURL`, `Prefetch`) is **banned**: each
appends a literal inline `<script>`, which the CSP forbids, which logs a console violation, which
fails the Principle VIII gate.

**What is delivered** — a `PatchElements` of `shell/notice.templ` into `#toast`
(`append` mode), plus a `PatchSignals` bumping the unanswered-invitation badge count.

| Notice | Reaches | Says | Requirement |
|---|---|---|---|
| invitation received | the recipient, when the address has an account | sender display name, kind of thing, level | FR-064, US6 scenario 1 |
| invitation answered | the sender | recipient display name, accepted or declined | FR-064, US6 scenario 2 |
| access granted | the grantee | grantor display name, kind, level | FR-064 |
| access changed | the grantee | grantor display name, the new level | FR-064, US6 scenario 3 |
| access ended | the grantee | grantor display name | FR-064, US6 scenario 3 |

**What a notice may never contain** (FR-065, SC-017): a patient's name, a relative's name, a
diagnosis, a medication, a measurement, a document name, or any other clinical value. Like the
invitation preview, this is enforced by **DTO shape** — `api.Notice` has fields for a display name,
an event kind, a resource kind and a level, and no field capable of carrying anything else. A render
test asserts the templ component emits no other text node.

**Delivery rules**

| Rule | Requirement |
|---|---|
| the hub carries **ids and event kinds only**, never bodies | Constitution V |
| the handler **re-resolves the actor's entitlement at the moment of delivery** and drops the event if it no longer holds | FR-066, US6 scenario 5 |
| an account that is not connected simply misses it; nothing is persisted, nothing is retried | FR-067, [D-20](../research.md#d-20) |
| **nothing in this phase's correctness depends on a notice arriving** — every behaviour is also reachable by reloading a page | FR-067 |
| a session left open for **60 continuous minutes** is still receiving notices | SC-017, [D-04](../research.md#d-04) |

**Tests**

- `internal/web/stream/notifications_test.go` — an event addressed to user A is delivered to A's
  stream and **not** to B's; an event for a grant revoked between publish and delivery is dropped
  (FR-066); the payload contains no patient name (a sentinel-value assertion).
- `internal/web/stream/notifications_slow_test.go` (`//go:build slowsse`) — the stream is still
  delivering after **6 minutes**, which is the only way to catch a regression of the `WriteTimeout`
  override.
- `e2e/specs/sharing-live.spec.ts` — two browser contexts, one notice, no refresh, under 5 s.
- Goroutine-leak assertion: closing the request context unsubscribes and returns; the hub's
  subscriber count returns to its pre-test value.

---

## The revocation cut-off on `GET /api/v1/streams/records` (operation 29, amended)

The existing record stream gains one behaviour, and it is the reason the hub grew a second event
shape ([D-21](../research.md#d-21)).

**Before**: the handler subscribes to a patient topic, and on each event re-fetches the record id,
**re-authorises it for that subscriber**, renders and patches.

**After**: it subscribes to the patient topic **and** to its own subscriber's user topic. On an
`AccessChanged` event naming that user and that patient it re-authorises immediately; when
authorisation now fails it:

1. patches `sharing/accessended.templ` into the list's container — a plain, non-clinical panel
   saying access has ended and offering a link back to `/patients`;
2. returns, closing the stream.

**Requirements discharged**: FR-045 and US2 scenario 2 (the screen stops updating **and says why**,
rather than silently freezing or continuing to show content), and SC-005's 5-second budget.

**Belt and braces**: the existing keepalive tick also re-authorises, so a revoke whose hub event was
missed still cuts the stream within one tick. The hub event is the fast path; the tick is the
guarantee.

**What it must not do**: leave the old rows on screen (US2 scenario 2 calls that out by name), or
emit a console message, or redirect from inside the stream (a `303` is issued **before** a stream is
opened, never through the SDK).

**Test**: `internal/web/stream/records_test.go` — with a stream open on a shared patient, revoking
the grant causes exactly one patch and a closed stream, and the patched HTML contains no clinical
content.
