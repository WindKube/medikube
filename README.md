# MediKube

A self-hosted personal medical records application: **one static Go binary** with an embedded
[PocketBase](https://pocketbase.io). No companion service, no external database, no Node.js at
run time. The container image is 8 MB and runs as a non-root user on a distroless base.

The data MediKube holds is among the most sensitive a person owns, and the architecture is
shaped around that — see [the constitution](.specify/memory/constitution.md).

> **Status: walking skeleton.** Phase 001 is under construction. What is here is the module, the
> toolchain, the gates and the package boundaries — not yet a running application.

## Prerequisites

- **Go 1.27.** Not 1.26. PocketBase v0.40.1 imports the Go 1.27 standard-library package
  `encoding/json/v2` in 67 non-test files, so an older toolchain fails at compile time with
  `go.mod requires go >= 1.27`. This is the first project in the house off the 1.26.5 standard,
  deliberately. Never set `GOTOOLCHAIN=local` — it turns that decision into a misleading error.
- [Task](https://taskfile.dev) for the task runner.
- Docker, only if you are building the image.

## Getting started

```bash
task gen      # templ components + the Tailwind bundle; everything else depends on this
task build    # ./medikube
task test     # the Go suites, the spec-corpus assertions and the naming guard
task ci       # everything CI runs, in CI's order
```

`task --list` shows the rest. Every task runs the same command its CI job runs, so `task ci` is a
faithful dry run rather than an approximation.

## Configuration

Every setting MediKube has is an environment variable read once at boot into one validated struct
(`internal/config`). There is no configuration file, no flag that outranks the environment and no
settings collection an operator edits by hand — FR-051 makes that a single mechanism, and
`internal/config/documented_test.go` fails the build if a field ever appears here that the struct
does not declare, or a field is declared that this table does not name.

`MEDIKUBE_DATA_DIR` and `MEDIKUBE_PUBLIC_URL` are required; everything else has the default shown.
Validation reports **every** problem at once rather than the first, so a restart costs one round
trip and not five.

### Core

| Variable | Default | What it is |
|---|---|---|
| `MEDIKUBE_ENV` | `production` | `production`, `staging` or `development`. Only `production` changes behaviour; the others exist so a typo is refused rather than silently read as non-production. |
| `MEDIKUBE_DEV` | `false` | Development conveniences, including PocketBase's `Automigrate`. Refused when `MEDIKUBE_ENV=production`. |
| `MEDIKUBE_DATA_DIR` | **required** | The one directory everything this instance holds lives under (FR-061). There is deliberately no default: PocketBase's own fallback puts `pb_data` beside the binary, which in the image is a read-only layer. |
| `MEDIKUBE_HTTP_ADDR` | `0.0.0.0:8090` | The listener address. |
| `MEDIKUBE_PUBLIC_URL` | **required** | The absolute URL the instance is reached at. Must be `https` in production unless it is loopback. |
| `MEDIKUBE_DRAIN_DELAY` | `5s` | How long a shutdown keeps accepting before it stops. |
| `MEDIKUBE_DRAIN_MAX` | `25s` | The hard deadline for a shutdown. Must exceed `MEDIKUBE_DRAIN_DELAY`. |
| `MEDIKUBE_ALLOWED_ORIGINS` | empty | Comma-separated CORS origins. Empty means same-origin only. |
| `MEDIKUBE_TRUSTED_PROXIES` | empty | Comma-separated proxy addresses whose forwarding headers are believed. |
| `MEDIKUBE_CURSOR_KEY` | empty | Override for the list cursor's signing key, which is otherwise derived from PocketBase's persisted auth-token secret. May be given as a file path. |

### Logging

| Variable | Default | What it is |
|---|---|---|
| `MEDIKUBE_LOG_LEVEL` | `info` | Any zerolog level. |
| `MEDIKUBE_LOG_PRETTY` | `false` | Human-readable console output. Refused when `MEDIKUBE_ENV=production`. |

### Accounts and retention

| Variable | Default | What it is |
|---|---|---|
| `MEDIKUBE_AUTH_REGISTRATION_OPEN` | `false` | Closed by default: an instance reachable from the internet must not accept accounts from strangers. |
| `MEDIKUBE_AUTH_SESSION_TTL` | `168h` | Session lifetime, seven days (FR-008). |
| `MEDIKUBE_RETENTION_AUDIT_DAYS` | `730` | How long audit events are kept. |

### Sentry

| Variable | Default | What it is |
|---|---|---|
| `MEDIKUBE_SENTRY_DSN` | empty | Empty disables Sentry entirely. May be given as a file path, and is dropped from the process environment after it is read. |
| `MEDIKUBE_SENTRY_ENVIRONMENT` | `production` | The environment tag on reported events. |
| `MEDIKUBE_SENTRY_SAMPLE_RATE` | `1.0` | Within `[0,1]`. |
| `MEDIKUBE_SENTRY_DEBUG` | `false` | Sentry's own transport diagnostics. |

### Metrics

| Variable | Default | What it is |
|---|---|---|
| `MEDIKUBE_METRICS_ENABLED` | `true` | Whether the Prometheus endpoint is served. |
| `MEDIKUBE_METRICS_ADDR` | `127.0.0.1:9090` | Loopback by default: exposing metrics is an explicit act. |
| `MEDIKUBE_METRICS_TOKEN` | empty | Bearer token for the metrics endpoint. Required in production whenever `MEDIKUBE_METRICS_ADDR` is not bound to loopback. May be given as a file path. |

### Tracing

| Variable | Default | What it is |
|---|---|---|
| `MEDIKUBE_OTEL_ENABLED` | `false` | Off unless a collector is actually there. |
| `MEDIKUBE_OTEL_ENDPOINT` | `localhost:4318` | The OTLP/HTTP collector. Required when tracing is enabled. |
| `MEDIKUBE_OTEL_INSECURE` | `true` | Plaintext to the collector, which is normally a sidecar. |
| `MEDIKUBE_OTEL_SAMPLE_RATIO` | `1.0` | Within `[0,1]`. |
| `MEDIKUBE_OTEL_HEADERS` | empty | `key:value,key:value` headers on the OTLP export. May be given as a file path. |
| `MEDIKUBE_OTEL_ENVIRONMENT` | `production` | The `deployment.environment` resource attribute. |

### Uploads

| Variable | Default | What it is |
|---|---|---|
| `MEDIKUBE_FILES_MAX_UPLOAD_BYTES` | `33554432` | 32 MiB. |
| `MEDIKUBE_FILES_ALLOWED_MIME` | `application/pdf,image/png,image/jpeg,image/heic,text/plain` | The accepted upload types. At least one is required. |

Secrets never reach the log stream: the boot line is written by an allowlist
(`internal/config/redact.go`), so a DSN or a token is reported as present or absent and never as a
value (FR-041).

## Layout

| Path | What it is |
|---|---|
| `cmd/medikube/` | the composition root, and the only place permitted to panic |
| `internal/domain/` | entities and rules; imports the standard library and zerolog, nothing else |
| `internal/service/` | use cases, behind interfaces, with hand-written fakes |
| `internal/store/` | the PocketBase-backed repositories and the migrations |
| `internal/platform/pb/` | the embedded PocketBase instance itself |
| `internal/web/` | the HTTP edge: API handlers, server-rendered pages, the SSE stream |
| `specs/` | the specification corpus this code is written against |
| `.specify/memory/constitution.md` | the nine principles, and the locked stack |

Packages that may import `github.com/pocketbase/pocketbase/...` are marked `[PB]` in
[`specs/001-walking-skeleton/plan.md`](specs/001-walking-skeleton/plan.md), and that boundary is
a `depguard` rule rather than a convention — crossing it fails the lint job.

## Contributing

[`CLAUDE.md`](CLAUDE.md) covers the day-to-day: the toolchain trap, the import boundary, where
each tier of test lives, and the gates that will reject a change.
