# House patterns — read from the monorepo itself

Read directly from the working tree on 2026-08-26. This OVERRIDES conventions.md
wherever they disagree, and it names a sibling project that dossier was not told
about.

## THE TEMPLATE PROJECT IS `arc-ui`, NOT appbase OR gmod

`/Users/krzysztof.wiatrzyk/private/monorepo/arc-ui` is a Go + templ + Tailwind +
embedded-SQLite web application. It is structurally the same shape as MediGo and
**already uses datastar-go v1.2.2 in production in this monorepo**. Copy its
layout, Taskfile and Dockerfile. Its full dependency overlap with MediGo:

| dependency | arc-ui version | MediGo |
| --- | --- | --- |
| go | 1.26.5 | same |
| github.com/a-h/templ | v0.3.1020 | same |
| github.com/starfederation/datastar-go | v1.2.2 | same |
| github.com/caarlos0/env/v11 | v11.4.1 | same |
| github.com/rs/zerolog | v1.35.1 | same |
| github.com/getsentry/sentry-go | v0.48.0 | same |
| github.com/samber/lo | v1.53.0 | same |
| github.com/spf13/cobra | v1.7.0 | via PocketBase's RootCmd |
| github.com/stretchr/testify | v1.12.0 | same |
| modernc.org/sqlite | v1.56.0 | v1.57.0, transitively via PocketBase |

**Pin MediGo to these exact versions** where they overlap — it keeps the
monorepo's module graph coherent.

arc-ui uses `gin-gonic/gin`. MediGo does NOT, and this is a deliberate
divergence, not an oversight: PocketBase owns MediGo's router (constitution
Principle V). Do not copy arc-ui's HTTP layer.

arc-ui does NOT use the `replace ../go-modules` directive (gmod and appbase do).
MediGo does not need it either — it shares no code with `go-modules`. **This
matters**: the root `.dockerignore` and `build-image.yaml` both explain that the
repository-root build context exists *because* of that replace directive. MediGo
still builds from the repository root, because that is what the shared workflow
passes, but it has no `go-modules` dependency to resolve.

## go.mod header

```go
module medigo

go 1.26.5
```

Bare module name, matching `arc-ui`, `gmod`, `appbase`, `medikeep-mcp`. Not a
domain-prefixed path.

## The `tool` directive — pin generators in the module graph

arc-ui uses Go 1.24+'s `tool` directive:

```go
tool (
	github.com/a-h/templ/cmd/templ
)
```

and then invokes `go tool templ generate ./internal/web/...`. The reason is
recorded in arc-ui's own comments and is worth repeating in MediGo's plan:
`go install .../templ@latest` lets the GENERATOR drift from the templ RUNTIME
the binary links against, producing generated code that no longer compiles.
`go tool` cannot drift. MediGo MUST use the `tool` directive for templ.

## Package layout to copy

```
cmd/medigo/                 main.go, version stamping
internal/config/            caarlos0/env struct, validated at boot
internal/logging/           zerolog setup
internal/metrics/           prometheus registry + collectors
internal/telemetry/         OTel + Sentry wiring
internal/web/               HTTP layer
internal/web/views/         .templ sources (+ generated *_templ.go)
internal/web/static/        embedded assets incl. Tailwind output app.css
assets/input.css            Tailwind entrypoint (scans ../internal/web/**/*.templ)
```

arc-ui also has `internal/store/` for persistence. MediGo's equivalent is the
PocketBase adapter package, and per constitution Principle II it is the ONLY
package permitted to import `github.com/pocketbase/pocketbase/...`.

## Taskfile conventions (copy these task names)

`default` (task --list), `install:tailwind`, `install:golangci-lint`, `gen`,
`gen:templ`, `gen:css`, `tidy`, `vet`, `lint`, `test`, `check`, `build`, `run`,
`docker:build`, `docker:up`, `docker:down`, `clean`.

Notes worth carrying over verbatim:

- `vet`, `lint`, `test` and `build` all declare `deps: [gen]`, so generated code
  is never stale.
- `test` runs `go test -race -count=1 ./...`.
- `VERSION` comes from `git describe --tags --always --dirty 2>/dev/null || echo dev`
  and is stamped with `-ldflags "-X main.version={{.VERSION}}"`.
- `GO_BUILD_FLAGS: "-trimpath"`, and release builds add `-s -w`.
- `install:golangci-lint` installs **golangci-lint v2** with the comment
  "v1 does not understand Go 1.26". MediGo MUST use v2.
- `docker:build` sets `dir: ..` because the build context is the repository
  root. A bare `docker build .` inside the project directory FAILS.
- `clean` deletes the binary, the generated `app.css`, and all `*_templ.go`.

MediGo adds: `gen:migrations` is not needed (migrations are hand-written Go),
but it DOES need `seed`, `routes`, `openapi` task wrappers around its Cobra
subcommands, and a `test:e2e` task for the Playwright gate.

## Dockerfile pattern — four stages, all traps documented

arc-ui's Dockerfile is the template. Stages:

1. `deps` — `--platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm`, copies
   only `go.mod`/`go.sum` then `go mod download`, so editing source does not
   re-download the module graph. The `tool` directive means this ALSO fetches
   the generators.
2. `generate` — downloads the pinned Tailwind standalone binary, runs
   `go tool templ generate`, then `tailwindcss -i assets/input.css -o
   internal/web/static/app.css --minify`.
3. `build` — `CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath
   -ldflags="-s -w -X main.version=${VERSION}"`, plus
   `install -d -m 0755 -o 65532 -g 65532 /data` to stage the data dir.
4. `runtime` — `gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`,
   `VOLUME ["/data"]`, exec-form `ENTRYPOINT`.

**The traps arc-ui documents, which MediGo will hit identically:**

- The Tailwind release asset for x86_64 is named `x64`, NOT `amd64`. Docker's
  `$BUILDARCH` says `amd64`, so the unmapped URL 404s and the failure reads like
  a network blip. Map it explicitly.
- The Tailwind standalone binary is a Bun build and is **not statically
  linked** — it needs glibc. That is why the generate stage sits on the Debian
  `golang` image and pulls plain `linux-x64`/`linux-arm64`. The `-musl` variants
  are for Alpine builders and fail at exec time, not download time.
- Pin `TAILWIND_VERSION` by ARG. Unpinned "latest" makes the generated CSS
  non-reproducible between two builds of the same commit.
- Every stage before the last is `--platform=$BUILDPLATFORM`. With
  `CGO_ENABLED=0` Go cross-compiles for free, so nothing runs under QEMU.
  Emulating an arm64 builder to run templ and Tailwind costs minutes and OOMs
  regularly, for zero benefit — both generators only emit source.
  **This works for MediGo too: PocketBase's SQLite is `modernc.org/sqlite`,
  pure Go, so `CGO_ENABLED=0` holds.**
- Distroless has no shell and no `mkdir`, so the data directory must be created
  in the build stage and COPYed — COPY of a directory creates it in the target.
- Numeric `USER 65532:65532`, not `USER nonroot`: Kubernetes' `runAsNonRoot`
  admission check must decide before start whether the user is root, and some
  runtimes cannot resolve a name from the image's `/etc/passwd`, rejecting the
  pod with "container has runAsNonRoot and image has non-numeric user".
- No `HEALTHCHECK` in the Dockerfile — no curl, no wget in distroless. arc-ui's
  compose probes with the binary's own `healthcheck` subcommand instead.
  **MediGo should ship a `medigo healthcheck` Cobra subcommand for this.**
- Tailwind must be pointed at the `.templ` SOURCES. Auto-detection skips them
  because generated files are gitignored, and you get a stylesheet with none of
  the app's utilities in it.

MediGo differences from arc-ui's Dockerfile: PocketBase's data directory is
`pb_data` and the runtime env var should follow the `MEDIGO_` prefix
convention (arc-ui uses `ARC_UI_HTTP_ADDR`, `ARC_UI_DB_PATH`).

## MONOREPO INTEGRATION — EXACT CHANGES REQUIRED

Both files below MUST be changed in the same commit that creates MediGo, or the
container build fails with a misleading "file not found".

### 1. `/Users/krzysztof.wiatrzyk/private/monorepo/.dockerignore`

It is a **deny-everything-then-readmit allowlist** (`*` first). A project that
is not readmitted is invisible to the build. Current allowlist:

```
!go-modules/
!technologia/
!appbase/
!gmod/
!arc-ui/
```

Add `!medigo/` to that block. Then, mirroring the arc-ui entries, add:

```
medigo/medigo
medigo/.bin/
medigo/**/*_templ.go
medigo/internal/web/static/app.css
medigo/pb_data/
medigo/**/*.db
medigo/**/*.db-wal
medigo/**/*.db-shm
```

The `*_templ.go`, `app.css` and database exclusions exist because the image
regenerates them in the generate stage — a host-built copy in the context would
be copied over the source tree and shadow what that stage produces. `pb_data/`
is PocketBase's data directory and must never enter a build context: it holds
the live database and uploaded files.

### 2. `/Users/krzysztof.wiatrzyk/private/monorepo/.github/workflows/build-image.yaml`

Add `medigo` to the `workflow_dispatch.inputs.project-name.options` list:

```yaml
        options:
          - technologia
          - appbase
          - gmod
          - arc-ui
          - medigo
```

Nothing else in the workflow needs changing — it is generic, keyed on
`inputs.project-name`, builds `context: .` with
`file: <project>/Dockerfile`, builds amd64 and arm64 on native Blacksmith
runners, pushes by digest, then merges a multi-arch manifest. It publishes to
`ghcr.io/windkube/<project>`.

The workflow's sparse-checkout includes `${{ inputs.project-name }}` and
`go-modules`. MediGo does not use `go-modules`, which is harmless.

## Commit convention

Conventional Commits with a project scope. Real examples from `git log`:

```
docs(specs): record what the claudy decomposition actually did
ci: build catalog images from the repository root
feat(gmod): new project owning the Modules catalog
feat(appbase): own the Applications catalog end to end
fix(technologia): block SSRF in logo uploads
```

MediGo's scope is `medigo`, e.g. `feat(medigo): embed PocketBase and serve the
medication list`.

## Root Taskfile

`/Users/krzysztof.wiatrzyk/private/monorepo/Taskfile.yaml` only holds repo-wide
helpers (`commit`, `workflow:run`, `workflow:list`, `workflow:watch`). Projects
own their own Taskfiles; there is no root aggregation to register with.

## golangci-lint — house baseline plus MediGo's constitution enforcers

Both `arc-ui/.golangci.yml` and `medikeep-mcp/.golangci.yml` use golangci-lint
**v2** schema (`version: "2"`, `linters.default`, `linters.settings`,
`linters.exclusions`, a separate top-level `formatters` block). v1 syntax will
not load, and arc-ui's Taskfile notes v1 does not understand Go 1.26.

House baseline to start from (union of the two siblings): `bodyclose`,
`contextcheck`, `copyloopvar`, `errcheck`, `errorlint`, `gocritic`, `gosec`,
`govet` (with `shadow`), `ineffassign`, `misspell`, `nilerr`, `noctx`,
`prealloc`, `revive`, `staticcheck`, `unconvert`, `unused`, `usestdlibvars`.
Formatters: `gofmt`, `goimports` with `local-prefixes: [medigo]`.

Standard exclusions to copy: relax `bodyclose`/`errcheck`/`gosec`/`noctx` in
`_test\.go`; exclude generated `_templ\.go` from `revive`, `gocritic`, `unused`,
`ineffassign`, `prealloc`, `errcheck`, `gosec`; exclude `internal/web/` from
`contextcheck` (the linter walks into templ's generated components and asks why
they take no context — they cannot, because a templ component is a constructor
returning `templ.Component` and the context arrives later at `Render(ctx, w)`).

### NEW for MediGo — these two linters make the constitution mechanical

Neither sibling has them. Without them, constitution Principles II and VI are
review-vigilance rather than gates, which Principle IX forbids.

**`depguard` enforces Principle II's import boundary** — domain and service
packages must not import PocketBase, net/http, or generated templ packages:

```yaml
    depguard:
      rules:
        domain-and-services-stay-pure:
          list-mode: lax
          files:
            - "**/internal/domain/**"
            - "**/internal/service/**"
          deny:
            - pkg: github.com/pocketbase/pocketbase
              desc: >-
                Constitution Principle II. PocketBase is reached only through a
                repository interface implemented in internal/store. Domain and
                service packages must stay testable without a database.
            - pkg: net/http
              desc: >-
                Constitution Principle II. Services decide; handlers speak HTTP.
                A service that knows about net/http has taken on a second
                responsibility.
            - pkg: github.com/a-h/templ
              desc: >-
                Constitution Principle II. Rendering belongs to internal/web.
        no-forbidden-frameworks:
          list-mode: lax
          files:
            - "$all"
          deny:
            - pkg: github.com/gin-gonic/gin
              desc: Forbidden by the constitution. PocketBase's router is the router.
            - pkg: github.com/danielgtaylor/huma
              desc: Forbidden by the constitution. No second OpenAPI framework.
            - pkg: github.com/spf13/viper
              desc: Forbidden by the constitution. caarlos0/env is the only config mechanism.
            - pkg: github.com/pocketbase/pocketbase/plugins/jsvm
              desc: Forbidden by the constitution. MediGo ships no scripting runtime.
```

**`forbidigo` enforces Principle VI's single log stream** — PocketBase v0.40.1
hardcodes its slog handler, so anything written through `app.Logger()` leaves
MediGo's zerolog stream permanently:

```yaml
    forbidigo:
      analyze-types: true
      forbid:
        - pattern: '\.Logger\(\)$'
          msg: >-
            Constitution Principle VI. PocketBase's app.Logger() writes to a
            handler MediGo cannot replace, so those lines never reach the
            zerolog stream. Inject the request-scoped zerolog logger instead.
        - pattern: '^fmt\.Print.*$'
          msg: Constitution Principle VI. Use the injected zerolog logger.
        - pattern: '^log\.(Print|Fatal|Panic).*$'
          msg: Constitution Principle VI. The standard log package is not MediGo's logger.
```

Add `depguard` and `forbidigo` to `linters.enable`. Exclude both from
`_templ\.go` and from the PocketBase adapter package (which legitimately imports
PocketBase).

A third gate worth adding once the sharing phase lands: `gosec` is already on,
but the OpenAPI/route-inventory consistency check from constitution Principle IX
is a **Go test**, not a linter — model it on `medikeep-mcp`'s
`cmd/gen-tools/coverage_test.go`, which fails the build when a single API
operation is unaccounted for. That is the house precedent for "compliance is a
gate, not a paragraph in a README".
