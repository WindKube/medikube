# Monorepo house conventions — what MediKube's spec must match

Repo: `/Users/krzysztof.wiatrzyk/private/monorepo`. Evidence gathered from `appbase/`,
`gmod/`, `arc-ui/`, `technologia/`, `medikeep-mcp/`, `go-modules/`, the root
`.dockerignore` / `.github/workflows/`, and `git log`.

---

## 0. Read this first: the closest sibling is `arc-ui`, not `appbase`

The task brief pointed at `appbase` as "the closest sibling". It is not. **`arc-ui` is
the closest sibling by a wide margin** and is the layout MediKube should copy.

`arc-ui/go.mod` (`/Users/krzysztof.wiatrzyk/private/monorepo/arc-ui/go.mod`) already
carries almost exactly MediKube's locked stack:

| MediKube locked decision | arc-ui | appbase |
| --- | --- | --- |
| templ v0.3.1020 | ✅ `github.com/a-h/templ v0.3.1020` | ✅ same version |
| Datastar v1.2.2 | ✅ `github.com/starfederation/datastar-go v1.2.2` | ❌ vendored HTMX 1.9.12 |
| Tailwind (no Node at runtime) | ✅ Tailwind v4 standalone, pinned `v4.1.14` | ❌ hand-written `styles.css` |
| caarlos0/env/v11 | ✅ `v11.4.1` | ✅ `v11.4.1` |
| zerolog | ✅ `v1.35.1` | ✅ `v1.35.1` |
| testify | ✅ `v1.12.0` | ❌ stdlib `testing` only |
| samber/lo | ✅ `v1.53.0` | ❌ |
| Sentry | ✅ `getsentry/sentry-go v0.48.0` | (indirect only) |
| Prometheus | ✅ `client_model` + `common` | ❌ |
| Cobra | ✅ `v1.7.0` | ✅ `v1.10.2` |
| Embedded SQLite, CGO-free | ✅ `modernc.org/sqlite` | ❌ (Baserow API) |
| Server-rendered + SSE live updates | ✅ `internal/web/stream.go` | ❌ |
| Gin / Huma | ⚠️ uses gin (MediKube drops it — PocketBase owns the router) | uses gin + huma |
| Config file | none — env only | none — env only |

**Copy from arc-ui:** package layout, `internal/web` (templ + Datastar + hashed embedded
assets), `internal/config`, `internal/logging`, Taskfile, `.golangci.yml`, the 4-stage
Dockerfile, the go:embed/.gitkeep trap, testify-based tests.

**Copy from medikeep-mcp:** the *compliance-is-a-gate* pattern (`cmd/gen-tools`,
`task api:coverage`, `coverage_test.go`) and its stricter linter settings.

**Copy from appbase/gmod:** the project-prefixed `COPY` paths in the Dockerfile and the
`docker:build` task with `dir: ..`.

**Ignore:** `medi-keep-go/` — a prior MediKeep-in-Go attempt, currently staged for
deletion (`git status` shows 90+ `D medi-keep-go/…` entries). It used Gin + Huma + GORM +
HTMX (all now dropped) and, critically, **was never registered in `.dockerignore` or
`build-image.yaml`** — it built with its own directory as context. MediKube must not repeat
that. Its `.golangci.yml`, however, is byte-identical to arc-ui's/appbase's profile, which
confirms that profile is the house standard.

---

## 1. Canonical Go project layout

```
medikube/
├── .claude/skills/medikube/SKILL.md # optional; every catalog project has one
├── .env.example                     # committed; .env is gitignored
├── .gitignore
├── .golangci.yml
├── Dockerfile
├── README.md
├── Taskfile.yaml
├── compose.yaml
├── go.mod
├── go.sum
├── assets/input.css                 # Tailwind entrypoint (arc-ui pattern)
├── cmd/
│   └── medikube/
│       ├── main.go                  # `var version = "dev"`, cobra root
│       ├── serve.go                 # one file per subcommand (appbase/technologia style)
│       ├── healthcheck.go
│       └── main_test.go
└── internal/
    ├── config/         config.go, config_test.go
    ├── logging/        logging.go, logging_test.go
    ├── api/            server.go + one file per resource group
    ├── web/            *.templ, *_templ.go (generated), assets.go, stream.go, *_test.go
    │   └── static/     .gitkeep (committed!), app.css (generated), datastar.js
    ├── <domain>/       patients/, records/, labs/, sharing/, files/, …
    ├── observability/  sentry.go   (medikeep-mcp name)  — or telemetry/ (arc-ui name)
    └── metrics/        metrics.go, health.go
```

### Rules extracted

**`cmd/<binary>/`** — one directory named exactly after the binary, which is named exactly
after the project directory. Never `cmd/server`, never `cmd/app`.

- `appbase/cmd/appbase/` holds `main.go` + one file per cobra subcommand: `admin.go`,
  `category.go`, `enqueue.go`, `image.go`, `mcp.go`, `records.go`, `worker.go`.
- `arc-ui/cmd/arc-ui/` keeps everything in `main.go` (it has only `serve`, `version`,
  `healthcheck`).
- MediKube has many subcommands via PocketBase's `RootCmd` → follow appbase: one file per
  subcommand.
- `main.go` always declares `// version is stamped at build time with -ldflags -X main.version=…`
  and `var version = "dev"`.

**`internal/<name>/`** — flat, single-word, all-lowercase package names. No `internal/pkg/`,
no `internal/domain/service/impl/`. Deeper nesting appears only where a generator or an API
version forces it:

- `arc-ui/internal/store/ent/schema/` (ent generator)
- `arc-ui/internal/arcapi/v1alpha1/` (K8s API versioning)
- `medikeep-mcp/internal/gen/coverage/` (one sub-concern of the generator)

Recurring package names across every sibling — reuse these exact names:
`config`, `logging`, `api`, `web`, `store`, `metrics`, `observability`/`telemetry`,
`mcpserver`. appbase additionally nests templ files under `internal/web/views/`; arc-ui
keeps them flat in `internal/web/`. **Use arc-ui's flat form** — its `web` package holds
`layout.templ`, `overview.templ`, `detail.templ`, `primitives.templ`, `charts.templ`
alongside `page.go`, `view.go`, `stream.go`, `assets.go`.

**Tests live beside the code**, same package (internal tests — no `_test` package suffix,
no `/test` or `/testdata`-only directory). `internal/store/store_test.go`,
`internal/web/web_test.go`, `internal/config/config_test.go`. Density varies:
`medikeep-mcp` has a `_test.go` for essentially every source file — that is the standard
MediKube should hold itself to, given the constitution's Principle III.

**testify is the assertion library.** arc-ui and medikeep-mcp both use
`require` for fatal preconditions and `assert` for the assertions under test:

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
s, err := Open(t.Context(), path, zerolog.Nop())
require.NoError(t, err, "Open")
```

Note `t.Context()` (Go 1.24+), `t.TempDir()`, `zerolog.Nop()` for silent loggers, and
package-level fixed instants (`var base = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)`)
so assertions are deterministic.

### Is generated code committed?

**Two distinct answers, and the distinction is the convention:**

| Kind | Committed? | Why |
| --- | --- | --- |
| templ output (`*_templ.go`) | **No** — gitignored | Pure build product; `task gen` reproduces it. Gitignored in `arc-ui/.gitignore`, `appbase/.gitignore`, `medi-keep-go/.gitignore`. |
| Tailwind bundle (`internal/web/static/app.css`) | **No** — gitignored | Same. |
| ent client (`internal/store/ent/`) | **Yes** (arc-ui) | Regenerated only on schema change (`task gen:ent`), not on every build. |
| `medikeep-mcp/internal/tools/zz_generated_tools.go` | **Yes**, and diff-checked | It is an *input to a build gate*: `task api:coverage` reads the **compiled** registry, so the file must exist and compile. `task gen:check` runs `git diff --exit-code` on it. |

**Rule for MediKube:** generated code that is a pure build product is gitignored and
regenerated by `task gen`. Generated code that a compliance gate reads back is committed
and guarded by a `gen:check` diff task.

**The go:embed trap — do not skip this.** `arc-ui/internal/web/assets.go`:

```go
//go:embed all:static
var staticFS embed.FS
```

`go:embed` on a directory is a **compile-time error if the directory is empty**. Since
`app.css` is gitignored, a fresh clone would not build. arc-ui commits
`internal/web/static/.gitkeep` to prevent that, and uses the `all:` prefix so embed does
not skip the dot-file. MediKube must commit the same `.gitkeep`.

### Two architectural details worth copying verbatim from arc-ui

1. **Content-hashed embedded assets** (`internal/web/assets.go`). Files read from an
   `embed.FS` report a zero mtime, so `http.ServeContent` emits neither `Last-Modified`
   nor `ETag` and every browser revalidates on every load. arc-ui hashes the content into
   the filename (`app.<sha12>.css`), serves the hashed path `Cache-Control: public,
   max-age=31536000, immutable` and the plain path `max-age=300`.
2. **`WriteTimeout: 0` on the `http.Server`** — any non-zero value is a hard deadline on
   the whole response, which severs an SSE stream mid-page on a timer. Handlers bound
   their own work through the request context. MediKube uses Datastar SSE → this applies
   directly. (Note: PocketBase owns the router, so this becomes a `serve` hook / server
   config concern rather than a hand-built `http.Server`.)

---

## 2. The exact `go.mod` header

```go
module medikube

go 1.26.5
```

**Verified.** Bare module name, no domain prefix, matching the directory name exactly:

```
appbase/      module appbase        go 1.26.5
arc-ui/       module arc-ui         go 1.26.5
gmod/         module gmod           go 1.26.5
go-modules/   module go-modules     go 1.26.5
technologia/  module technologia    go 1.26.5
claudy/       module claudy         go 1.26.5
sre-agent/    module sre-agent      go 1.26.5
medikeep-mcp/ module medikeep-mcp   go 1.26      ← older
tape/         module tape           go 1.26      ← older
```

Every project from the most recent (Aug 2026) batch is on **`go 1.26.5`**. Legacy /
imported projects use domain-prefixed module paths (`github.com/windkube/github-dashboard`,
`github.com/mergestat/mergestat`) — those are foreign, not house style.

**Counter-example to avoid:** the deleted `medi-keep-go/` declared `module medikeep` in a
directory named `medi-keep-go` — a name mismatch. MediKube's directory is `medikube`, so the
module is `medikube`.

**No `replace` directive.** `replace go-modules => ../go-modules` exists only in the three
catalog projects (technologia, appbase, gmod) that import the shared toolkit. `go-modules/`
holds catalog-specific primitives — a GitHub client, Hatchet bootstrap, MCP/agent glue, an
SVG rasteriser, a NocoDB/Baserow/Teable-oriented `safefetch`. **MediKube imports none of it.**
arc-ui and medikeep-mcp have no `replace` either.

**No root `go.work` — ever.** From
`docs/superpowers/specs/2026-08-17-claudy-decomposition-design.md`, Decision 8:
> "No root `go.work`. Go walks up to find `go.work`; a root workspace breaks every unlisted
> module in this monorepo."

**`tool` directive (arc-ui only, and recommended for MediKube):**

```go
tool (
	github.com/a-h/templ/cmd/templ
)
```

This pins the templ *generator* to the same version as the templ *runtime library*, so
`go tool templ generate` can never drift from what the code links against. appbase instead
does `go install …/templ@v0.3.1020` and has to keep two version strings in sync by hand (in
`Taskfile.yaml` and `Dockerfile`). **Use arc-ui's `tool` directive.**

---

## 3. `.golangci.yml` recommendation

### The house profile (identical in `arc-ui`, `appbase`, and the deleted `medi-keep-go`)

`version: "2"`, `default: none`, 13 explicitly enabled linters:

```
bodyclose  contextcheck  errcheck  gocritic  gosec  govet  ineffassign
misspell   prealloc      revive    staticcheck  unconvert  unused
```

Settings: `revive` with `var-naming` + `package-comments` enabled and `unused-parameter`
disabled; `gocritic` with `ifElseChain` disabled; `misspell.ignore-rules` listing the
project name in each casing. Formatters: `gofmt` + `goimports`.

`gmod/` and `technologia/` have **no** `.golangci.yml` at all — they are the write-side
CLI projects and are the laxest, not a model.

### The strictest sibling: `medikeep-mcp`

`medikeep-mcp/.golangci.yml` takes a different and stricter tack — `default: standard`
(errcheck, govet, ineffassign, staticcheck, unused) **plus 10 more**, with tightened
per-linter settings:

```yaml
linters:
  default: standard
  enable: [bodyclose, copyloopvar, errorlint, gocritic, misspell,
           nilerr, noctx, revive, unconvert, usestdlibvars]
  settings:
    errcheck:
      check-type-assertions: true      # ← stricter than any sibling
    govet:
      enable: [shadow]                 # ← stricter than any sibling
    staticcheck:
      checks: [all]                    # ← stricter than any sibling
issues:
  max-issues-per-linter: 0             # ← no truncation of findings
  max-same-issues: 0
formatters:
  settings:
    goimports:
      local-prefixes: [medikeep-mcp]   # ← import grouping
```

It lacks `gosec`, `prealloc` and `contextcheck` (it has no templ and no long-lived
handlers), which the house profile has.

### Recommendation for MediKube: the union

MediKube holds medical records — it should be the strictest project in the repo. Take the
house 13, add medikeep-mcp's 5 extras and all four of its tightened settings:

```yaml
version: "2"

run:
  timeout: 5m
  tests: true

linters:
  default: none
  enable:
    # House profile (arc-ui / appbase)
    - bodyclose
    - contextcheck
    - errcheck
    - gocritic
    - gosec
    - govet
    - ineffassign
    - misspell
    - prealloc
    - revive
    - staticcheck
    - unconvert
    - unused
    # medikeep-mcp's additions
    - copyloopvar
    - errorlint
    - nilerr
    - noctx
    - usestdlibvars

  settings:
    errcheck:
      check-type-assertions: true
    govet:
      enable:
        - shadow
    staticcheck:
      checks:
        - all
    revive:
      rules:
        - name: var-naming
        - name: package-comments
        - name: unused-parameter
          disabled: true
    gocritic:
      disabled-checks:
        - ifElseChain
    misspell:
      ignore-rules:
        - medikube
        - Medikube
        - MEDIKUBE
        - MediKube

  exclusions:
    generated: lax
    rules:
      # contextcheck walks into templ's generated components and asks why they
      # do not take a context. They cannot: a templ component is a constructor
      # returning templ.Component; the context arrives later at Render(ctx, w).
      - path: internal/web/
        linters:
          - contextcheck

      # Tests ignore Close errors on responses they are about to discard and
      # build URLs from a test server's address.
      - path: _test\.go
        linters:
          - bodyclose
          - errcheck
          - gosec
          - noctx

      # templ emits long, deliberately un-idiomatic functions; the fix would be
      # to edit the generator.
      - path: _templ\.go
        linters:
          - revive
          - gocritic
          - unused
          - ineffassign
          - prealloc
          - errcheck
          - gosec

issues:
  max-issues-per-linter: 0
  max-same-issues: 0

formatters:
  enable:
    - gofmt
    - goimports
  settings:
    goimports:
      local-prefixes:
        - medikube
  exclusions:
    generated: lax
```

Every exclusion above is copied from a sibling and each carries its original justification
comment — the house style is that an exclusion without a written reason does not exist.

**golangci-lint v2 is mandatory.** v1 does not understand Go 1.26. Both arc-ui and appbase
carry an `install:golangci-lint` task pinning to `github.com/golangci/golangci-lint/v2`.
`medi-keep-go` additionally documented the toolchain trap:
> `GOTOOLCHAIN=local` forces the build to use the installed Go (1.26.x) rather than the
> older toolchain golangci-lint pins; otherwise the resulting binary refuses to lint this
> 1.26 module ("built with go1.25 < targeted go1.26").

---

## 4. `Taskfile.yaml` conventions

`version: "3"` (quoted; only the root Taskfile uses bare `3`). Per-project Taskfiles are
**fully independent** — the root `Taskfile.yaml`
(`/Users/krzysztof.wiatrzyk/private/monorepo/Taskfile.yaml`) has no `includes:` and no
`dir:` fan-out. It holds four repo-wide tasks only: `commit`, `workflow:run`,
`workflow:list`, `workflow:watch` (all `gh` wrappers). **You `cd` into the project and run
`task <x>` there.** MediKube does not register itself with the root Taskfile.

### Real task names across siblings

| Task | Present in | Notes |
| --- | --- | --- |
| `default` | all | `task --list`; `silent: true` in medikeep-mcp |
| `install:templ` | appbase, medi-keep-go | with a `status:` guard for idempotency |
| `install:tailwind` | arc-ui | downloads the standalone binary into `./.bin` |
| `install:golangci-lint` | arc-ui, appbase, medi-keep-go | `status: [command -v golangci-lint]` |
| `gen` | arc-ui, appbase, medi-keep-go | aggregate, `deps: [gen:templ, gen:css]` |
| `gen:templ` | arc-ui, appbase | arc-ui uses `go tool templ generate ./internal/web/...` |
| `gen:css` | arc-ui | `.bin/tailwindcss -i assets/input.css -o internal/web/static/app.css --minify` |
| `gen:ent` | arc-ui | only after a schema edit |
| `gen:check` | medikeep-mcp | `git diff --exit-code` on the generated file |
| `tidy` | all | `go mod tidy` |
| `vet` | all | `go vet ./...`; `deps: [gen]` where templ is used |
| `lint` | arc-ui, appbase, medikeep-mcp | `golangci-lint run ./...` |
| `test` | all | `go test -race -count=1 ./...` (arc-ui) or `./internal/...` (appbase/gmod/technologia) |
| `test:cover` | medikeep-mcp | `-coverprofile` + `go tool cover -func … \| tail -1` |
| `check` | all | the aggregate. arc-ui: `vet` + `lint` + `test`. Others: `vet` + `test` |
| `build` | all | `deps: [gen]` |
| `run` | arc-ui, appbase, technologia, medikeep-mcp | `deps: [build]`, then `./{{.BIN}} <cmd>` |
| `run:<variant>` | appbase, gmod, technologia, arc-ui | `run:mcp`, `run:worker:ai`, `run:worker:repodata`, `run:proxy` |
| `docker:build` | all | **`dir: ..`** — builds from the repo root |
| `docker:up` / `docker:down` | arc-ui, appbase, gmod, technologia | `docker compose up --build` |
| `docker:run` | medikeep-mcp | hardened `docker run` with `--read-only --cap-drop ALL` |
| `api:coverage` | medikeep-mcp | **the compliance gate** |
| `preview` | arc-ui | renders views to standalone HTML for review |
| `helm:lint` / `helm:template` | arc-ui | |
| `clean` | all | `rm -f {{.BIN}}` + `find . -name '*_templ.go' -delete` |

### Conventions inside the file

- `vars:` — `BIN: medikube` (arc-ui/appbase/gmod/technologia) or `BINARY` + `MAIN`
  (medikeep-mcp). Use `BIN`.
- `GO_BUILD_FLAGS: "-trimpath"` — every project.
- Version stamping (arc-ui + medikeep-mcp):

  ```yaml
  VERSION:
    sh: git describe --tags --always --dirty 2>/dev/null || echo dev
  ```

  with `-ldflags="-s -w -X main.version={{.VERSION}}"`. The comment in arc-ui:
  *"A dirty or tagless tree is honestly labelled `dev` rather than pretending to be a
  release."*
- Pinned tool versions live in `vars:` (`TAILWIND_VERSION: v4.1.14`,
  `TEMPL_VERSION: v0.3.1020`), never inline.
- `deps: [gen]` on `vet`, `lint`, `test`, `build` — generated code must exist first.
- `status:` guards on every `install:*` task so re-running is a no-op.
- **`docker:build` uses `dir: ..`**, with the comment stating why:

  ```yaml
  # The image is built from the repository root, because that is the context CI
  # passes (`context: .`, `file: medikube/Dockerfile`) and the COPY paths in the
  # Dockerfile are project-prefixed to match. A bare `docker build .` here fails.
  docker:build:
    desc: Build the Docker image
    dir: ..
    cmds:
      - docker build -f medikube/Dockerfile --build-arg VERSION={{.VERSION}} -t {{.BIN}}:local .
  ```

- Gate tasks are **deliberately not fingerprinted** with `sources:`/`generates:`.
  medikeep-mcp's comment:
  > *"Deliberately not fingerprinted with sources/generates: `gen:check` is a build gate,
  > and a skipped run would turn a stale registry into a passing CI job."*

### The compliance-gate pattern MediKube must adopt

`medikeep-mcp` proves its tool registry covers all 500 upstream operations, mechanically.
Three files:

- `medikeep-mcp/cmd/gen-tools/main.go` — `-mode=generate` rewrites the registry;
  `-mode=coverage` runs the set arithmetic and exits non-zero on a gap.
- `medikeep-mcp/internal/gen/coverage/coverage.go` — pure set arithmetic on strings:

  ```
  covered    = BUILT ∩ SPEC
  missing    = SPEC − BUILT − EXCLUDED   → fail
  orphaned   = BUILT − SPEC              → fail
  stale_excl = EXCLUDED − SPEC           → fail
  ```

- `medikeep-mcp/cmd/gen-tools/coverage_test.go` — **the gate that guards the gate**. It
  asserts BUILT was read out of the *compiled* registry and not re-derived from the spec,
  using sort order as the discriminator:

  ```go
  if sort.StringsAreSorted(got) {
      t.Fatal("built ids are in operationId order, which is spec order, not registry order")
  }
  ```

  With a comment stating the failure mode being prevented: *"Re-deriving it from the spec
  would make `task api:coverage` a tautology: it would report full coverage against a
  registry that had never been generated at all."*
- `medikeep-mcp/api/exclusions.yaml` — every non-covered operation with a written reason.
  A stale exclusion (one whose operation no longer exists) is itself a failure.

**MediKube's analogue** (constitution Principle IX, *"Compliance Is A Build Gate, Not A README
Paragraph"*): a `task api:coverage` proving every `/api/v1` route in the OpenAPI document
has a hand-written handler with explicit DTOs, and — the higher-value one — that **no
PocketBase auto-CRUD `/api/collections/*` record route is publicly reachable**, read out of
the running app's registered routes rather than asserted in prose. Plus a
`task ui:smoke` wrapping the Playwright console-error gate.

---

## 5. Dockerfile pattern

Copy `arc-ui/Dockerfile` structurally; it is the only sibling that also runs templ +
Tailwind.

```dockerfile
# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.26
ARG TAILWIND_VERSION=v4.1.14
```

- **First line is always `# syntax=docker/dockerfile:1.7`.**
- **`ARG GO_VERSION=1.26`** — the *image tag*, not the go.mod patch version. Every sibling
  uses `1.26` even where go.mod says `1.26.5`.
- **Base: `golang:${GO_VERSION}-bookworm`** (Debian), not Alpine. medikeep-mcp uses
  `golang:1.26-alpine` but it has no Tailwind. **MediKube must use bookworm** — arc-ui's
  comment explains: *"The [Tailwind] binary is a Bun build and is NOT statically linked —
  it needs glibc. … The `-musl` variants exist for Alpine builders only; using one here
  fails at exec time, not at download time."*
- **Stages.** arc-ui uses four; appbase/gmod use two. MediKube should use arc-ui's four:
  1. `deps` — `COPY medikube/go.mod medikube/go.sum ./` + `go mod download`, keyed on
     manifests alone so a source edit does not re-download the module graph.
  2. `generate` — `go tool templ generate ./internal/web/...` + the Tailwind standalone.
  3. `build` — the cross-compile, plus staging any directory the runtime cannot create.
  4. `runtime` — distroless.
- **Cross-compilation, not QEMU.** Every non-final stage is
  `FROM --platform=$BUILDPLATFORM …`, with `ARG TARGETOS` / `ARG TARGETARCH`:

  ```dockerfile
  RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
      go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/medikube ./cmd/medikube
  ```

  arc-ui's reason: *"Emulating an arm64 builder to run templ and Tailwind costs minutes of
  wall clock and OOMs regularly, for zero benefit — both generators only ever emit source."*
  **PocketBase note:** PocketBase v0.40.1 uses `modernc.org/sqlite` (pure Go), so
  `CGO_ENABLED=0` holds and this pattern works unchanged.
- **`ARG VERSION=dev`**, stamped via `-X main.version=${VERSION}`.
- **Build args used:** `GO_VERSION`, `TAILWIND_VERSION`, `VERSION`, `TARGETOS`,
  `TARGETARCH`, `BUILDARCH`.
- **Runtime base:**
  - `gcr.io/distroless/static-debian12:nonroot` — arc-ui, medikeep-mcp. **Use this.**
  - `debian:bookworm-slim` + `ca-certificates curl tzdata` — appbase, gmod, only because
    their compose healthcheck shells out to curl.
- **`USER 65532:65532` numerically**, not `USER nonroot`. arc-ui's comment:
  > *"Kubernetes' runAsNonRoot admission check has to decide before the container starts
  > whether the user is root, and not every runtime can resolve a name out of the image's
  > /etc/passwd to do it — a `USER nonroot` image gets rejected with 'container has
  > runAsNonRoot and image has non-numeric user' on those runtimes."*
- **Writable data directory on distroless.** There is no shell and no `mkdir`, so it must
  be built in the build stage and copied:

  ```dockerfile
  RUN install -d -m 0755 -o 65532 -g 65532 /pb_data
  ...
  COPY --from=build --chown=65532:65532 /pb_data /pb_data
  VOLUME ["/pb_data"]
  ```

  MediKube needs this for PocketBase's data directory.
- **No `HEALTHCHECK` on distroless** — no curl, no wget, no shell. arc-ui instead ships a
  `arc-ui healthcheck` cobra subcommand that probes `/healthz` and exits non-zero, and
  compose calls `test: ["CMD", "/usr/local/bin/arc-ui", "healthcheck"]`. **MediKube should
  add a `medikube healthcheck` subcommand for the same reason.**
- **Exec-form `ENTRYPOINT`/`CMD` only** — no shell to interpret shell form.
- **`ENV` defaults in the image** so the container is runnable with no env file:

  ```dockerfile
  ENV MEDIKUBE_HTTP_ADDR=0.0.0.0:8080 \
      MEDIKUBE_PB_DATA_DIR=/pb_data
  ```

### Static assets

- Tailwind input at `assets/input.css`, output to `internal/web/static/app.css`, embedded
  via `//go:embed all:static`. Nothing is served from disk at runtime; there is no Node in
  the image or the runtime.
- **The Tailwind arch trap, twice** (once in the Taskfile, once in the Dockerfile): the
  release asset for x86_64 is named `x64`, **not** `amd64`. An unmapped `uname -m` /
  `$BUILDARCH` 404s and the failure reads like a network blip:

  ```sh
  arch="${BUILDARCH}"; case "${arch}" in amd64) arch=x64 ;; arm64) arch=arm64 ;; esac
  ```

- **The Tailwind source-scanning trap** (`arc-ui/assets/input.css`): Tailwind v4
  auto-detects sources by walking the project and **deliberately skips anything
  `.gitignore` excludes**. `*_templ.go` is gitignored, so on a clean tree auto-detection
  finds no class names and silently emits a stylesheet with none of the app's utilities —
  the page renders unstyled and nothing errors. Fix with explicit `@source` directives
  pointing at the `.templ` **sources** and at any `.go` file that builds class names:

  ```css
  @import "tailwindcss";
  @source "../internal/web/**/*.templ";
  @source "../internal/web/*.go";
  ```

- **Build context is the repository root.** Non-negotiable — see §6.

---

## 6. THE INTEGRATION CHECKLIST — every monorepo file a new project must touch

This is the section that prevents the misleading "file not found" Docker failure.

### 6.1 `/Users/krzysztof.wiatrzyk/private/monorepo/.dockerignore` — **CRITICAL, allowlist**

This file is an **allowlist**: it denies everything with `*` and re-admits named
directories. A project that is not re-admitted has an **empty build context**, and its
Dockerfile's first `COPY` fails with a "file not found" error that points at the COPY line,
not at this file. Its own header says so:

> *"Docker reads .dockerignore from the build context root and nowhere else … a
> .dockerignore inside a project directory is silently ignored."*
> *"The strategy is deny-everything-then-readmit, not an exclusion list. … An allowlist
> fails closed instead."*

**Change 1 — re-admit the directory.** In the allowlist block, after `!arc-ui/`:

```
*

!go-modules/
!technologia/
!appbase/
!gmod/
!arc-ui/
!medikube/
```

**Change 2 — exclude the bare-named build artifact.** In the "Build outputs" block, after
`arc-ui/.bin/`:

```
# Build outputs, including the bare-named binaries `task build` leaves in place.
technologia/technologia
appbase/appbase
gmod/gmod
arc-ui/arc-ui
arc-ui/.bin/
medikube/medikube
medikube/.bin/
```

**Change 3 — exclude everything MediKube regenerates inside the image, plus its live data.**
Append a new block after the existing `arc-ui/**` block, in the same commented style:

```
# medikube regenerates all of these inside the image — templ output and the Tailwind
# bundle in the generate stage. A host-built copy in the context would be copied
# over the source tree and shadow what the generate stage produces.
medikube/**/*_templ.go
medikube/internal/web/static/app.css

# PocketBase's data directory: the SQLite database, uploaded attachments and
# issued auth tokens. It holds live medical records and must never be transferred
# to a build daemon, captured in build cache, or recorded in provenance.
medikube/pb_data/
medikube/pb_public/
medikube/**/*.db
medikube/**/*.db-wal
medikube/**/*.db-shm

# Spec/planning material and Playwright output: none of it is read during a build.
medikube/.specify/
medikube/specs/
medikube/docs/
medikube/test-results/
medikube/playwright-report/
medikube/node_modules/
```

Already covered by existing global patterns — **do not re-add**: `**/.env`, `**/.env.*`,
`**/.git`, `**/.gitignore`, `**/.claude/`, `**/.golangci.yml`, `**/node_modules/`,
`**/README.md`, `**/compose.yaml`, `**/Taskfile.yaml`, `**/.dockerignore`, `**/.mcp.json`.

> Note the header's warning: exclusions are deliberately specific rather than `**/*.md`,
> because `prompts/*.md` **is** copied into the runtime image for the catalog projects.
> MediKube has no `prompts/` directory, so listing its markdown directories individually (as
> above) is both safe and consistent with the file's style.

**Why this matters, in the maintainer's own words** (commit `e936a32`):
> *"the build context went from ~25MB per project to 506MB, and it included the .env files
> belonging to four unrelated projects in this monorepo. … they were being transferred to
> the build daemon and captured in build cache and provenance, which is not somewhere other
> projects' credentials should be."*

### 6.2 `/Users/krzysztof.wiatrzyk/private/monorepo/.github/workflows/build-image.yaml`

**Exactly one change. Not a matrix entry, not a path filter.**

- **Matrix entry? No.** The `strategy.matrix` is architecture-only (`amd64` / `arm64` on
  Blacksmith runners) and is project-agnostic.
- **Path filter? No.** The workflow has **no** `push:` or `pull_request:` trigger — its
  only trigger is `workflow_dispatch:`. There is nothing to filter.
- **What to add:** one line in the `project-name` choice list.

```yaml
on:
  workflow_dispatch:
    inputs:
      project-name:
        description: Project directory to build (e.g. appbase)
        required: true
        type: choice
        options:
          - technologia
          - appbase
          - gmod
          - arc-ui
          - medikube        # ← the only change
```

Everything downstream is already parameterised on `inputs.project-name`:

- sparse-checkout takes `${{ inputs.project-name }}` **and** `go-modules` (MediKube does not
  need go-modules, but it comes along harmlessly);
- the Dockerfile presence check reads `${{ inputs.project-name }}/${{ inputs.dockerfile }}`;
- the image is `ghcr.io/windkube/medikube`;
- **the build context is `context: .` (the repository root)** with
  `file: ${{ inputs.project-name }}/${{ inputs.dockerfile }}`.

That last point is the hard constraint on MediKube's Dockerfile. The workflow's own comment:

> *"The repository root, not the project directory: each project's go.mod has a `replace
> ../go-modules` directive, and Go cannot resolve a path outside the build context. The
> Dockerfiles COPY with project-prefixed paths to match."*

**MediKube has no `replace` directive, but the workflow passes `context: .` unconditionally
with no per-project override.** So MediKube's Dockerfile **must** use project-prefixed COPY
paths anyway:

```dockerfile
WORKDIR /src/medikube
COPY medikube/go.mod medikube/go.sum ./
RUN go mod download
COPY medikube/ ./
```

A bare `COPY go.mod go.sum ./` (as in `medikeep-mcp/Dockerfile`, which is *not* registered
in this workflow) would fail in CI. This is the single most likely thing to get wrong.

### 6.3 Files inside `medikube/` that the monorepo expects to find

| Path | Required? | Content |
| --- | --- | --- |
| `medikube/go.mod` | yes | `module medikube` / `go 1.26.5` / `tool (github.com/a-h/templ/cmd/templ)` |
| `medikube/Taskfile.yaml` | yes | §4; `docker:build` with `dir: ..` |
| `medikube/.golangci.yml` | yes | §3 |
| `medikube/Dockerfile` | yes | §5; project-prefixed COPY paths |
| `medikube/compose.yaml` | yes | `build: {context: .., dockerfile: medikube/Dockerfile}` |
| `medikube/README.md` | yes | See §6.4 |
| `medikube/.gitignore` | yes | See below |
| `medikube/.env.example` | yes | Committed template; every var, commented, with defaults |
| `medikube/internal/web/static/.gitkeep` | **yes** | Or `go:embed` fails to compile on a fresh clone |
| `medikube/assets/input.css` | yes | Tailwind entrypoint with explicit `@source` directives |
| `medikube/.claude/skills/medikube/SKILL.md` | optional | Every catalog project has one; MediKube is an app, not a catalog |
| `medikube/.mcp.json` | no | Only for projects exposing an MCP server |

`medikube/.gitignore` (modelled on `arc-ui/.gitignore`, whose every entry carries a reason):

```gitignore
# Build artefacts
/medikube
/.bin/

# Generated: templ output and the Tailwind bundle. Both are reproduced by
# `task gen`, so they are not worth reviewing in a diff.
#
# TRAP: internal/web/static is consumed by a `go:embed` of the whole directory,
# and go:embed FAILS AT COMPILE TIME on a directory with no files at all. That is
# why internal/web/static/.gitkeep is committed — without it a fresh clone does
# not build until Tailwind has run once.
*_templ.go
internal/web/static/app.css

# PocketBase data: the SQLite database, uploads and issued auth tokens.
pb_data/
pb_public/
*.db
*.db-wal
*.db-shm

# Local env
.env
.env.*
!.env.example

# Playwright
test-results/
playwright-report/
node_modules/

# Editor / OS
.DS_Store
.idea/
.vscode/
```

Note: `medikube/.dockerignore` is **optional and inert** for the real builds. arc-ui keeps one
anyway, with a header explaining it only applies to a context rooted in that directory and
is kept in sync so the two cannot disagree. Copying that habit is fine; relying on it is not.

### 6.4 Conventions that are not files but are enforced by review

- **README structure** (arc-ui and appbase are the models): one-paragraph what-it-is, a
  table of the views or subcommands, a `## Stack` bullet list naming every library with its
  role, a `## Configuration` **table** with one row per env var (`Variable | Default | What
  it does`) declaring `internal/config/config.go` as "the authority; this table is it in
  prose", `## Local development` with the exact task sequence, and `## Docker` explaining
  the repo-root build context.
- **Config: one flat struct, env only.** `arc-ui/internal/config/config.go`:

  ```go
  // Package config loads and validates the process configuration from the
  // environment. Everything is a single flat struct so the full surface is
  // visible in one place; nested prefixes buy nothing at this size.
  type Config struct {
      HTTPAddr  string `env:"ARC_UI_HTTP_ADDR" envDefault:"0.0.0.0:8080"`
      LogLevel  string `env:"ARC_UI_LOG_LEVEL" envDefault:"info"`
      LogFormat string `env:"ARC_UI_LOG_FORMAT" envDefault:"json"`
      ...
  }
  func Load() (Config, []Warning, error)
  ```

  Loaded with `env.ParseAs[Config]()`, **validated at boot** (a bad `LOG_FORMAT` refuses to
  start), and returning a separate `[]Warning` for non-fatal problems *"instead of logging
  directly so the caller controls where they go"*. MediKube's prefix is `MEDIKUBE_`. Variables
  naming **shared infrastructure** rather than this app's own settings go **unprefixed** —
  arc-ui's `KUBE_API_URL`, appbase's `HATCHET_CLIENT_TOKEN` / `AI_GATEWAY_URL` /
  `GITHUB_APP_ID`.
- **Logging: constructor, not a global.**

  ```go
  // Package logging builds the process logger. The logger is passed explicitly
  // to whatever needs it rather than living in a package-level global.
  func New(level, format string) zerolog.Logger
  ```

  `console` for humans, JSON on **stderr** otherwise, `zerolog.TimeFieldFormat = time.RFC3339`,
  and a `.Str("service", "medikube")` field on every line.
- **Interfaces are declared at the consumer**, narrow, with a comment saying why the seam
  exists. `arc-ui/internal/web/stream.go`:

  ```go
  // EventSource fetches recent Kubernetes events for one runner pod.
  //
  // It is separate from the snapshot because events are the highest-churn object
  // in a cluster and must never be held in an informer cache; …
  type EventSource interface {
      Events(ctx context.Context, r fleet.Runner) ([]fleet.Event, error)
  }
  ```

- **Every non-obvious decision carries a comment stating the failure mode it prevents**,
  not what the code does. This is the single most distinctive house habit — it appears in
  the Dockerfiles, the Taskfiles, `.dockerignore`, `.gitignore`, `compose.yaml`, and the
  Go source. A MediKube spec that omits the reasoning will read as foreign to this repo.
- **No root `go.work`.**
- **`docs/superpowers/specs/<YYYY-MM-DD>-<slug>-design.md`** — design docs live either at
  the repo root (`docs/superpowers/specs/2026-08-17-claudy-decomposition-design.md`) for
  cross-project work, or under the project (`technologia/docs/superpowers/specs/…`,
  `medikeep-mcp/docs/superpowers/{plans,specs}/…`) for project-local work. Header format:
  `# Title`, `Status: approved, in implementation`, `Date: 2026-08-17`, then
  `## Problem` / `## Goal` / `## Decisions` (a numbered table of Decision + Rationale) /
  `## Target topology`. MediKube already has `medikube/.specify/` (speckit), which is a
  different toolchain — worth noting the divergence explicitly in the spec so nobody
  looks for the design doc in the wrong place.

### 6.5 CI beyond image building

There is **no lint/test CI for the Go catalog projects** — `build-image.yaml` is
`workflow_dispatch`-only and runs no tests. The only real CI is `tape-ci.yml`, and its
header states the placement rule:

> *"GitHub only runs workflows found in `.github/workflows` at the **repository** root."*

If MediKube wants CI (it should — the constitution has a build-gate principle), the file goes
at `/Users/krzysztof.wiatrzyk/private/monorepo/.github/workflows/medikube-ci.yml`, following
`tape-ci.yml`'s shape:

```yaml
name: medikube-ci
on:
  push:
    paths: ["medikube/**", ".github/workflows/medikube-ci.yml"]
  pull_request:
    paths: ["medikube/**", ".github/workflows/medikube-ci.yml"]
  workflow_dispatch:
permissions:
  contents: read
concurrency:
  group: medikube-ci-${{ github.ref }}
  cancel-in-progress: true
defaults:
  run:
    working-directory: medikube
jobs:
  ci:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: medikube/go.mod    # ← never a hardcoded version
      - run: go build ./...
      - run: go vet ./...
      - run: go test -race ./internal/...
      - uses: golangci/golangci-lint-action@v8
        with:
          working-directory: medikube
```

`go-version-file: medikube/go.mod` is the house pattern — the Go version is never duplicated
into the workflow.

### 6.6 The complete checklist, condensed

```
[ ] medikube/go.mod                           module medikube / go 1.26.5 / tool templ
[ ] medikube/.gitignore                       §6.3
[ ] medikube/.golangci.yml                    §3
[ ] medikube/Taskfile.yaml                    §4, docker:build with dir: ..
[ ] medikube/Dockerfile                       §5, PROJECT-PREFIXED COPY PATHS
[ ] medikube/compose.yaml                     context: .. / dockerfile: medikube/Dockerfile
[ ] medikube/README.md                        §6.4
[ ] medikube/.env.example                     every MEDIKUBE_* var, commented
[ ] medikube/assets/input.css                 @source pointing at .templ sources
[ ] medikube/internal/web/static/.gitkeep      or go:embed will not compile
[ ] /.dockerignore                            !medikube/ + artifacts + pb_data + specs   ← FAILS BUILD IF MISSED
[ ] /.github/workflows/build-image.yaml       one line: - medikube in the options list
[ ] /.github/workflows/medikube-ci.yml        optional, but must live at the ROOT
[ ] NOT a root go.work                        forbidden by design decision 8
[ ] NOT registered in the root Taskfile       per-project Taskfiles are independent
[ ] delete medi-keep-go/                      already staged; land it with MediKube
```

---

## 7. Commit message convention

**Conventional Commits.** `type(scope): subject` — lowercase subject, imperative mood, no
trailing period, roughly ≤72 chars.

**Types observed:** `feat`, `fix`, `docs`, `ci`.
**Scopes:** the project directory name — `technologia`, `appbase`, `gmod`, `go-modules`,
`tape`, and `specs` for design-doc changes. `ci:` and `docs:` are used bare when the change
is repo-wide.

Real subject lines from `git log --oneline -25`:

```
docs(specs): record what the claudy decomposition actually did
ci: build catalog images from the repository root
feat(gmod): new project owning the Modules catalog
feat(appbase): own the Applications catalog end to end
fix(technologia): block SSRF in logo uploads
feat(technologia): own the Technologies catalog end to end
feat(go-modules): add the shared module for the catalog projects
docs: design for decomposing claudy into technologia, appbase and gmod
docs(tape): close out review backlog with fix commit references
fix(tape): send the translate flag to whisper; redact provider API keys in list responses
fix(tape): drain both hotkey event channels to avoid leaking x/hotkey's internal goroutines
fix(tape): survive corrupt config on launch; stop bundling a CWD fallback in production
fix(tape): make RecorderService.Stop idempotent
ci: register tape-ci workflow at monorepo root (GitHub only runs root .github/workflows)
```

**MediKube's scope is `medikube`:** `feat(medikube): …`, `fix(medikube): …`, `docs(medikube): …`.

**Bodies are substantial prose, and they explain WHY.** They are not changelogs. The
recurring devices:

- A `Things that changed rather than moved, each for a reason:` bullet list, where each
  bullet names the bug class, not the diff.
- **Verified claims with numbers.** `"Verified: exits in 1s."` ·
  `"Verified: context is now 119kB (gmod) to 687kB (appbase), all three images build…"`
- **Operational warnings in caps** where a deploy step is required:
  `"DRAIN THE OLD no-logo-go-modules QUEUE BEFORE DEPLOYING, because tasks sitting on it
  have no worker once the rename lands."`
- **Honest disclosure of leftover risk**, including the author's own:
  `"NOTE: that endpoint's URL embedded a secret and has been in git history since b9d5600 —
  it needs rotating regardless of this change."`

Excerpt from `e936a32` (`ci: build catalog images from the repository root`), the commit
that created the `.dockerignore` MediKube must edit:

> Adds a root .dockerignore, which the root context makes necessary rather than optional.
> Docker reads .dockerignore from the context root and nowhere else, so moving the context
> up silently disabled appbase/.dockerignore and technologia/.dockerignore — those two
> files are deleted here because they had become inert and reading them would suggest
> otherwise.

**Trailers on every commit, in this order:**

```
Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_<id>
```

---

## 8. Gaps and judgement calls for the MediKube spec

Things the house style does not settle, where the spec should decide explicitly:

1. **Router.** Every HTTP sibling uses gin (arc-ui, appbase, technologia). MediKube drops it
   — PocketBase owns the router. So `internal/api` becomes route *registration* against
   `core.App`'s `OnServe` hook rather than a `*gin.Engine` + `*http.Server` it owns. The
   `WriteTimeout: 0` SSE constraint (§1) still applies and must be reached through
   PocketBase's server configuration.
2. **`internal/web` vs `internal/web/views`.** arc-ui is flat, appbase nests. Recommend
   arc-ui's flat layout, but MediKube has ~13 record types + labs + sharing + ops — that is a
   lot of templ files for one package. A `internal/web/views/` split is the defensible
   deviation if the flat package exceeds ~25 files.
3. **PocketBase's own `pb_public/` and migrations directory** have no precedent here.
   `pb_data/` must be gitignored and dockerignored (§6.1, §6.3); if PocketBase Go
   migrations are used they are ordinary committed Go files under `internal/…` and follow
   the normal rules.
4. **`slog` → zerolog bridge.** The MediKube constitution
   (`medikube/.specify/memory/constitution.md`) explicitly records that this is **not
   achievable** in PocketBase v0.40.1 — `core.BaseApp.initLogger` hardcodes its slog handler
   with no injection point — and that PB's log persistence is disabled instead. The task
   brief's stack list says "a slog.Handler bridges PocketBase's internal logs into zerolog",
   which contradicts the constitution. **The spec must reconcile these two, and the
   constitution is the one with the verified evidence.**
5. **Playwright** has no precedent in this monorepo (arc-ui's nearest equivalent is
   `task preview`, which renders views to standalone HTML via a Go test:
   `ARC_UI_PREVIEW_DIR="$PWD/docs/screenshots" go test ./internal/web/ -run TestWritePreview`).
   The deleted `medi-keep-go/scripts/screenshots.mjs` is the only Node-in-a-Go-project
   precedent. MediKube's Playwright gate needs its own `task ui:smoke`, and its `node_modules/`
   and report directories must be excluded in both ignore files (§6.1, §6.3).
