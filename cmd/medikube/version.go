package main

// version is stamped at link time with `-ldflags -X main.version=...` by
// `task build` and by the Dockerfile. A tree built without the flag — `go build
// ./cmd/medikube` — is honestly labelled `dev` rather than claiming a release it
// is not, and the operator surface prints whatever is here rather than a value
// derived at runtime from a VCS directory the distroless image does not carry.
var version = "dev"
