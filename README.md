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
