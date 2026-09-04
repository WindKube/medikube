// Package cli implements MediKube's own commands (routes, openapi,
// healthcheck, seed), dispatched from cmd/medikube's main out of os.Args
// before PocketBase's RootCmd runs (docs/spec-defects.md D28).
//
// It sits on the PocketBase side of the import boundary.
package cli
