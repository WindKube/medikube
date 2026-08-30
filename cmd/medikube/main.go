// Command medikube is the MediKube server: one static binary with an embedded
// PocketBase.
//
// This file is the composition root. Everything is constructed here and handed
// down, and this is the only place in MediKube permitted to panic — a
// dependency that cannot be built at startup is programmer error, and there is
// nobody left to return an error to.
//
// It sits on the PocketBase side of the import boundary.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/cmd"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"

	"medikube/internal/config"
	"medikube/internal/di"
	"medikube/internal/httproute"
	"medikube/internal/logging"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/web"

	// MediKube's migrations register themselves from their own init, and
	// core.AppMigrations is what apis.Serve runs. Without this import the list
	// is empty and the binary boots against PocketBase's stock schema: no
	// medications collection, no audit trail, and a users collection still
	// carrying the owner rules that hand its records to PocketBase's own API.
	// The boot assertion catches that and refuses to serve — which is how this
	// import came to be missing exactly once.
	_ "medikube/internal/store/migrations"
)

// bootGateHookID names the OnServe handler that refuses to serve a misassembled
// instance, so binding it twice replaces rather than appends.
const bootGateHookID = "medikubeBootGate"

// bootGatePriority runs the assertions and the settings write before
// pb.BindServe installs the middleware and the routes. Everything it needs
// exists by then: apis.Serve applies every migration before it triggers OnServe
// (apis/serve.go:66), so MediKube's own collections are there to be checked.
const bootGatePriority = pb.ServeHookPriority - 1

func main() {
	if err := run(); err != nil {
		// The failure has already been said out loud, exactly once, through
		// the one stream (FR-057). All that is left is the exit code.
		os.Exit(1)
	}
}

// versionRequested reports whether the invocation is only asking what this
// binary is.
//
// Asking a binary its version must not require configuring it. Everything below
// needs a valid MEDIKUBE_PUBLIC_URL and a data directory it can open; a release
// pipeline identifying an image it has just built has neither, and should not
// have to invent them to get an answer.
func versionRequested(args []string) bool {
	if len(args) == 0 {
		return true
	}

	switch args[0] {
	case "version", "--version", "-v":
		return true
	default:
		return false
	}
}

// run loads the configuration, assembles the instance and hands control to
// PocketBase's command surface.
func run() error {
	if versionRequested(os.Args[1:]) {
		_, err := fmt.Fprintf(os.Stdout, "medikube %s\n", version)

		return err
	}

	cfg, err := config.Load()
	if err != nil {
		// There is no configured logger yet, and the reason there is none is
		// the thing being reported. This one line goes out at the defaults
		// rather than not at all.
		boot := logging.NewTo(os.Stdout, config.LogConfig{Level: zerolog.LevelInfoValue}, version)
		boot.Error().Err(err).Msg("read the MediKube configuration")

		return err
	}

	log := logging.New(cfg.Log, version)

	app, container, err := build(cfg, log)
	if err != nil {
		log.Error().Err(err).Msg("assemble MediKube")

		return err
	}

	defer func() {
		if shutdownErr := container.Shutdown(); shutdownErr != nil {
			log.Error().Err(shutdownErr).Msg("release the MediKube container")
		}
	}()

	if err := registerCommands(app, cfg); err != nil {
		log.Error().Err(err).Msg("build the MediKube command surface")

		return err
	}

	if err := app.Execute(); err != nil {
		log.Error().Err(err).Msg("run MediKube")

		return err
	}

	return nil
}

// build assembles the instance: the container, the embedded PocketBase, both
// halves of the log bridge, the route table and the middleware order.
//
// It returns before anything is bootstrapped or served, which is what lets the
// test drive the same assembly against an empty data directory and a listener
// of its own.
func build(cfg config.Config, log zerolog.Logger) (*pocketbase.PocketBase, *di.Container, error) {
	container, err := di.New(di.Deps{Config: cfg, Logger: log})
	if err != nil {
		return nil, nil, fmt.Errorf("build the MediKube container: %w", err)
	}

	app := pb.New(cfg, pb.Options{
		// Database instrumentation attaches here and is US3 (T247). Nil is a
		// valid build rather than a nil dereference at bootstrap.
		DBConnect: nil,
	})

	// Both halves of the log bridge, and before Bootstrap: the decorator
	// covers everything reached through the core.App value, the _logs
	// interception covers the transaction-scoped app that decorator cannot
	// reach, and the lines PocketBase writes on its way up are worth having.
	logging.BridgeApp(app, log)
	logging.BridgeLogs(app, log)

	registry, err := httproute.New(operations())
	if err != nil {
		return nil, nil, shutdownAfter(container, fmt.Errorf("wire the MediKube routes: %w", err))
	}

	pb.BindServe(app, pb.ServeOptions{
		// Left at zero deliberately. Any positive value is a silent cap on
		// every Server-Sent Events stream, and it fails by killing the
		// connection mid-write while every test shorter than the cap passes
		// (research D-34).
		WriteTimeout: 0,
		Middlewares: []*hook.Handler[*core.RequestEvent]{
			// -1050: outside everything, so one request is one line whatever
			// the chain does to it.
			obs.RequestLogger(log),
			// -1031: outside PocketBase's panic recovery, which is what makes
			// a recovered panic answer in MediKube's envelope. The error view
			// is nil until internal/web/page exists; an API-only build is a
			// build.
			web.Errors(nil),
			// -1019: immediately after PocketBase's loadAuthToken, which is
			// what populates e.Auth.
			web.Actors(),
		},
		// The security headers first, because their binder removes
		// PocketBase's own and a route bound before that would be served
		// under the wrong policy for the length of one boot.
		Routes: binders{web.SecurityBinder{}, registry},
		// Outside PocketBase's router altogether, which is the only place a
		// CORS preflight and a ServeMux path-normalising redirect are visible
		// at all. Without it every OPTIONS response carries no security
		// headers and every `//` or `/../` redirect leaves no log line.
		Outermost: web.Outermost(log),
	})

	bindBootGate(app, cfg, log)

	return app, container, nil
}

// binders composes the several things that bind to the serve event into the
// one seam pb.ServeOptions has. Order is preserved and the first failure stops
// the boot.
type binders []pb.RouteBinder

func (b binders) Bind(se *core.ServeEvent) error {
	for _, binder := range b {
		if err := binder.Bind(se); err != nil {
			return err
		}
	}

	return nil
}

// bindBootGate installs the checks that decide whether this instance is
// allowed to serve at all.
//
// They run inside OnServe rather than before it because they need a schema:
// apis.Serve applies every migration before triggering the hook, and an
// assertion that passed because the collections did not exist yet would be
// worse than no assertion.
//
// The order is contracts/cli.md's: assert, then write the settings. Asserting
// after the write would let an instance whose batch endpoint somebody enabled
// in the admin UI be silently repaired on every boot instead of refusing —
// and the refusal is the point.
func bindBootGate(app core.App, cfg config.Config, log zerolog.Logger) {
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Id:       bootGateHookID,
		Priority: bootGatePriority,
		Func: func(se *core.ServeEvent) error {
			if err := pb.AssertLockedDown(se.App); err != nil {
				return fmt.Errorf("MediKube refuses to serve: %w", err)
			}

			if err := pb.ApplySettings(se.App, cfg); err != nil {
				return fmt.Errorf("MediKube refuses to serve: %w", err)
			}

			superusers, err := se.App.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers)
			if err != nil {
				return fmt.Errorf("read the %s collection: %w", core.CollectionNameSuperusers, err)
			}

			pb.LogAdminWarnings(log, pb.AdminWarnings(se.App.Settings(), superusers))

			// The one startup line (FR-053). T292 adds the applied migration
			// count to it; everything else it names is here.
			log.Info().
				Str("version", version).
				Str("data_dir", cfg.DataDir).
				Str("addr", listenAddr(se)).
				Msg("medikube started")

			return se.Next()
		},
	})
}

// registerCommands puts MediKube's command surface on PocketBase's root
// command: its own serve command with MediKube's validated configuration as
// the defaults, and its superuser command.
//
// Execute rather than Start: Start registers PocketBase's serve command with
// PocketBase's defaults, and MEDIKUBE_HTTP_ADDR and MEDIKUBE_ALLOWED_ORIGINS
// would then be validated configuration that does nothing. The superuser
// command is registered because the first-run installer is deliberately
// disabled (pb.BindServe nils InstallerFunc), so it is the only way a first
// superuser comes into existence.
//
// The values are written into the flags rather than into os.Args: PocketBase
// pre-parses os.Args inside NewWithConfig, before Cobra runs, and silently
// swallows what it does not recognise (contracts/cli.md). An explicit --http
// or --origins on the command line is parsed afterwards and still wins.
//
// Nothing here names a cobra type, and that is deliberate rather than
// incidental: plan.md's dependency table pins spf13/cobra as "Transitive, via
// PocketBase's RootCmd. Never a direct require", because the version is
// pocketbase v0.40.1's own go.mod requirement and moves when PocketBase moves.
// Naming *cobra.Command in a signature is what would promote it, and would
// make the version MediKube's to choose.
func registerCommands(app *pocketbase.PocketBase, cfg config.Config) error {
	// showStartBanner is false: the banner is PocketBase's own stdout write and
	// it would be the one line in the process that is not JSON (Principle VI).
	serve := cmd.NewServeCommand(app, false)

	flags := serve.PersistentFlags()

	if err := flags.Set("http", cfg.HTTPAddr); err != nil {
		return fmt.Errorf("default the listen address to %q: %w", cfg.HTTPAddr, err)
	}

	if len(cfg.AllowedOrigins) > 0 {
		joined := strings.Join(cfg.AllowedOrigins, ",")
		if err := flags.Set("origins", joined); err != nil {
			return fmt.Errorf("default the allowed origins to %q: %w", joined, err)
		}
	}

	app.RootCmd.AddCommand(serve)
	app.RootCmd.AddCommand(cmd.NewSuperuserCommand(app))

	return nil
}

// listenAddr is what the server was told to bind, or the empty string when
// there is no server — which is every tests.ApiScenario, because it builds the
// serve event by hand and sets only App and Router.
func listenAddr(se *core.ServeEvent) string {
	if se.Server == nil {
		return ""
	}

	return se.Server.Addr
}

// shutdownAfter releases the container when assembly fails past the point of
// building it, so a boot failure does not also leak whatever it had already
// constructed.
func shutdownAfter(container *di.Container, err error) error {
	if shutdownErr := container.Shutdown(); shutdownErr != nil {
		return fmt.Errorf("%w (and releasing the container failed too: %w)", err, shutdownErr)
	}

	return err
}
