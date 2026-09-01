# syntax=docker/dockerfile:1
#
# Four stages: dependencies, generation, build, runtime. The image is the single
# self-contained artefact — no companion service, no sidecar, nothing fetched at
# run time (FR-061). That is also why there is no HEALTHCHECK: readiness is
# answered over HTTP at /readyz by the process itself.
#
# NOTE ON PATHS: specs/001-walking-skeleton/tasks.md T010 requires every COPY to be
# project-prefixed (`COPY medikube/go.mod ./`) because the shared monorepo workflow
# passes the repository root as the build context. That was correct while this
# project lived inside the monorepo; it is now the standalone WindKube/medikube
# repository and its own root is the context, so the prefix would break every COPY.
# Paths here are relative to this repository.

# Go 1.27 is required, not preferred: PocketBase v0.40.1 imports the 1.27 stdlib
# package encoding/json/v2 in 67 non-test files (VERIFIED-SOURCE-FACTS FACT 0).
ARG GO_VERSION=1.27.0
ARG TAILWIND_VERSION=v4.3.3

# --- deps ---------------------------------------------------------------------
# Its own stage so the module download is cached against go.mod/go.sum alone and
# survives every source edit.
FROM golang:${GO_VERSION}-bookworm AS deps
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# --- generate -----------------------------------------------------------------
# templ components and the Tailwind bundle. Both outputs are gitignored, so they
# are produced here rather than trusted from the build context — a host-built copy
# would shadow what this stage emits.
FROM deps AS generate
ARG TAILWIND_VERSION
ARG TARGETARCH

COPY . .

# The Tailwind standalone release names the 64-bit Intel asset `x64`, not `amd64`.
# Passing TARGETARCH through unmapped 404s, and the failure reads as a network
# problem rather than a naming one.
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) tw_arch=x64 ;; \
      arm64) tw_arch=arm64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -sSfL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${tw_arch}"; \
    chmod +x /usr/local/bin/tailwindcss

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go tool templ generate

RUN tailwindcss -i ./assets/input.css -o ./internal/web/static/app.css --minify

# --- build --------------------------------------------------------------------
FROM generate AS build
ARG VERSION=dev
ARG TARGETARCH

# CGO_ENABLED=0 is what makes the distroless base viable at all: SQLite comes from
# modernc.org/sqlite, which is pure Go, so the binary is static.
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOARCH="${TARGETARCH}" go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/medikube ./cmd/medikube

# Every directory the runtime needs is created HERE, with its ownership already
# correct. Distroless has no shell and no mkdir, so there is no way to make one in
# the final stage — and a missing data dir surfaces at boot as a permissions error.
RUN mkdir -p /out/data && chown -R 65532:65532 /out/data

# --- runtime ------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

COPY --from=build --chown=65532:65532 /out/medikube /medikube
COPY --from=build --chown=65532:65532 /out/data /data

# nonroot in the distroless image, stated numerically so it survives a base change.
USER 65532:65532

ENV MEDIKUBE_DATA_DIR=/data/pb_data
VOLUME ["/data"]
EXPOSE 8090

ENTRYPOINT ["/medikube"]
CMD ["serve", "--http=0.0.0.0:8090"]
