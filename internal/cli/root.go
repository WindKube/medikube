package cli

import (
	"fmt"
	"io"

	"medikube/internal/httproute"
	"medikube/internal/openapi"
)

// migrate is not one of these: it stays a real PocketBase RootCmd subcommand
// (migratecmd), not a command this package implements.
const (
	commandRoutes      = "routes"
	commandOpenAPI     = "openapi"
	commandHealthcheck = "healthcheck"
	commandSeed        = "seed"
)

// Names is every command Dispatch recognises, in the order `medikube help`
// lists them.
func Names() []string {
	return []string{commandRoutes, commandOpenAPI, commandHealthcheck, commandSeed}
}

// Deps is what MediKube's own commands need. routes, openapi and healthcheck
// need none of the database-touching fields — that's what lets them run with
// no MEDIKUBE_DATA_DIR set (T276).
type Deps struct {
	Stdout io.Writer
	Stderr io.Writer

	Routes  func() []httproute.Route
	OpenAPI func() (openapi.Input, error)

	// HealthcheckAddr is the default dial target when --addr is not given.
	HealthcheckAddr string

	// Seed bootstraps the instance, runs the fixture and releases what it
	// bootstrapped. The refusal rules (FR-060) live in internal/cli/seed.go.
	Seed func(out io.Writer) error
}

// Dispatch recognises MediKube's own command names and runs them, reporting
// handled=false for anything else (including no arguments) so cmd/medikube
// falls through to PocketBase's RootCmd for serve, superuser and migrate.
//
// Every flag is parsed by a FlagSet built for that command alone, never
// registered globally (T282, contracts/cli.md trap 2).
func Dispatch(args []string, deps Deps) (handled bool, err error) {
	if len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case commandRoutes:
		return true, runRoutes(args[1:], deps)
	case commandOpenAPI:
		return true, runOpenAPI(args[1:], deps)
	case commandHealthcheck:
		return true, runHealthcheck(args[1:], deps)
	case commandSeed:
		return true, runSeed(args[1:], deps)
	default:
		return false, nil
	}
}

// Usage is the MediKube half of `medikube help`; cmd/medikube prints
// PocketBase's own RootCmd help beneath it (docs/spec-defects.md D28).
func Usage(w io.Writer) error {
	_, err := fmt.Fprint(w, `MediKube's own commands:
  routes        Print the route inventory (--json for the machine form)
  openapi       Write the OpenAPI document (--out FILE, default stdout)
  healthcheck   Dial readyz and exit 0 on 200, non-zero otherwise (--addr)
  seed          Create the demo accounts and their records

PocketBase's own commands (serve, superuser, migrate) are listed below.
`)

	return err
}
