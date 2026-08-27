# Contract: `/api/v1/healthz` and `/api/v1/readyz`

Two operations, both public, both deliberately uninformative about the data the instance holds.
Requirements covered: FR-052, FR-058, FR-062, SC-013.

They are **distinct routes with distinct meanings**, which constitution Principle VI requires and
which is the difference between a database blip and a restart loop. Both are excluded from the
activity logger, the metrics middleware and the tracing middleware, so probe traffic does not
dominate the dashboards.

**Paths.** `/api/v1/healthz` and `/api/v1/readyz`, per SHARED-DESIGN §2.3. (The observability
dossier proposes `/api/v1/health/live` and `/api/v1/health/ready`; the contract is the binding
document. Stated here so nobody has to diff them.)

**PocketBase's own `GET /api/health` is left alone and is not MediGo's.** It returns 200
unconditionally and leaks `canBackup`, `realIP` and `possibleProxyHeader` to superusers. It is a
liveness probe and nothing more, and MediGo does not build on it.

---

## `healthz` — `GET /api/v1/healthz`

**Public. Touches no database. Touches no filesystem. Answers about the process and nothing else.**

**200**
```json
{ "status": "ok", "version": "1.2.3", "started_at": "2026-08-27T09:14:02Z" }
```

The only failure mode is not answering at all, which is the correct signal for "this process is
deadlocked or gone".

**FR-052**: "reflects only whether the process is running and never touches stored data". A
liveness probe that checks the database gets the container killed and restarted into the same
outage — the opposite of helpful — and, in a medical application, a probe that touches the
database is a probe that could be made to reveal something about it.

`version` is the build stamp from `-ldflags -X main.version`. `GET /api/v1/version` is **not** a
separate route; this payload is where the version lives (Principle I).

---

## `readyz` — `GET /api/v1/readyz`

**Public.** Answers whether the instance can serve.

**200**
```json
{ "status": "ready",
  "checks": { "database": "ok", "migrations": "ok", "storage": "ok" } }
```

**503**
```json
{ "status": "not_ready",
  "checks": { "database": "error", "migrations": "ok", "storage": "ok" } }
```

**503 while draining**
```json
{ "status": "draining", "checks": {} }
```

**The check vocabulary is fixed and closed**: check names are `database`, `migrations`, `storage`;
values are `ok` and `error`. **There is no message field, ever.** FR-052 requires that neither
signal reveal anything about the data held, and the Edge Case is explicit: "readiness reports not
ready with a reason that reveals nothing about the storage location or credentials". A driver
error string can carry a file path, a DSN or a credential, so it goes to zerolog with the
request id and never to the response body.

| Check | What it does |
|---|---|
| `database` | a real `SELECT 1` through `ConcurrentDB()` with a **2-second context deadline** — not a `Ping`, which on SQLite proves nothing |
| `migrations` | the applied migration set equals the registered set |
| `storage` | `app.NewFilesystem()` opens and closes cleanly |

**SC-013**: an operator can tell, in under 30 seconds and using only these two signals, whether an
instance is running and whether it is ready — **including when storage is unreachable**, where
liveness still reports alive and readiness reports not-ready. That case is a named test: the test
app's database file is made unreadable, `healthz` still returns 200, `readyz` returns 503 with
`database: error`, and the captured log stream contains the underlying error **and** no path or
credential in the response.

**The migration check answers a specific Edge Case**: "The stored-data structure has not been
brought up to date: readiness reports not ready and names that as the reason" — `migrations:
error` is that reason, and it is a check name rather than a sentence.

---

## Draining, and PocketBase's one-second shutdown window

`readyz` respects a drain flag, and that flag is what makes FR-062 achievable at all.

`apis/serve.go:171` binds PocketBase's HTTP shutdown at priority `-9999` with
`context.WithTimeout(context.Background(), 1*time.Second)` — **one second, hardcoded, not exposed
on `ServeConfig`**. Any request still running after one second has its connection cut
mid-response.

MediGo binds its own `OnTerminate` handler at priority **`-10000`**, so it runs **before**
PocketBase's:

1. flip readiness to false — `readyz` starts answering `503 draining`, so a load balancer or an
   orchestrator stops routing new work;
2. sleep `MEDIGO_DRAIN_DELAY` (default 5 s) so that stops being noticed;
3. wait up to `MEDIGO_DRAIN_MAX` (default 25 s) on MediGo's own in-flight counter;
4. `e.Next()`.

By the time PocketBase's one-second window opens there is nothing in flight, so its value no
longer matters. This is the standard fail-readiness-then-drain pattern and FR-062 describes it
almost word for word: "stop accepting new work, finish work in flight within a bounded period,
and close storage without loss or corruption".

`MEDIGO_DRAIN_MAX` must exceed `MEDIGO_DRAIN_DELAY`, and the config validator refuses to start
otherwise.

**Note**: `TerminateEvent.IsRestart` exists — on a restart PocketBase waits an extra 3 seconds for
`execve`, so terminate does not always mean exit and the drain handler must not assume it does.

---

## `medigo healthcheck`

The distroless runtime image has no shell, no `curl` and no `wget`, so a Dockerfile `HEALTHCHECK`
is impossible. FR-058 asks for a command that checks the health of a running instance "from within
its own environment", and this is it: `medigo healthcheck` dials
`http://127.0.0.1:{port}/api/v1/readyz`, exits `0` on `200` and non-zero otherwise, and prints
nothing on success. It is what a compose file or an orchestrator probes with. See
[cli.md](./cli.md).
