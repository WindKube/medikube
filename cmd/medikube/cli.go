package main

import (
	"fmt"
	"io"
	"os"

	"medikube/internal/cli"
	"medikube/internal/config"
	"medikube/internal/httproute"
	"medikube/internal/logging"
	"medikube/internal/openapi"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// healthcheckDefaultAddr: 127.0.0.1, not config.Config's 0.0.0.0 default —
// a healthcheck dials itself, and 0.0.0.0 is a bind address, not a dial target.
const healthcheckDefaultAddr = "127.0.0.1:8090"

// dispatchMediKube runs MediKube's own commands out of os.Args before
// PocketBase's RootCmd sees them (docs/spec-defects.md D28). routes and
// openapi need no config.Load, which is what lets `medikube routes` answer
// with no MEDIKUBE_DATA_DIR set and no port bound (T276).
func dispatchMediKube(args []string) (bool, error) {
	deps := cli.Deps{
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		Routes:          func() []httproute.Route { return httproute.Inventory().Routes() },
		OpenAPI:         openAPIInput,
		HealthcheckAddr: healthcheckDefaultAddr,
		Seed:            seedInstance,
	}

	return cli.Dispatch(args, deps)
}

// openAPIDocumentVersion is deliberately not the binary's own `version`
// (stamped from `git describe`, which changes every commit): a committed
// document diffed byte-for-byte (internal/openapi/staleness_test.go) needs a
// version that moves only when the API itself changes.
const openAPIDocumentVersion = "0.1.0"

func openAPIInput() (openapi.Input, error) {
	return openapi.Input{
		Version:        openAPIDocumentVersion,
		Routes:         httproute.Inventory().Routes(),
		Kinds:          api.OpenAPIKinds(),
		Envelope:       web.Envelope{},
		SearchResponse: api.SearchResponse{},
	}, nil
}

// seedInstance bootstraps rather than serves: apis.Serve is what applies
// migrations, so a database the migrations have not reached yet bootstraps
// far enough for cli.Seed's requireSchema check to see that and refuse.
func seedInstance(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cli: reading the MediKube configuration: %w", err)
	}

	log := logging.New(cfg.Log, version)

	app, container, _, err := build(cfg, log)
	if err != nil {
		return fmt.Errorf("cli: assembling MediKube: %w", err)
	}

	defer func() {
		if shutdownErr := container.Shutdown(); shutdownErr != nil {
			log.Error().Err(shutdownErr).Msg("release the MediKube container")
		}
	}()

	if err := app.Bootstrap(); err != nil {
		return fmt.Errorf("cli: opening the MediKube database: %w", err)
	}

	return cli.Seed(app, cfg.Env, out)
}
