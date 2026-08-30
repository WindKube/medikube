package di

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/rs/zerolog"
	"github.com/samber/do/v2"

	"medikube/internal/config"
	"medikube/internal/realtime"
	"medikube/internal/records"
)

// Deps is everything the container cannot build for itself: the validated
// configuration and the one logger, both of which exist before there is a
// container to put them in.
//
// It is a struct rather than a variadic option list because every member is
// required. A container built without a logger would be a container whose
// failures are invisible.
type Deps struct {
	Config config.Config
	Logger zerolog.Logger
}

// Container holds MediKube's own services and the injector that built them.
//
// The accessors return values rather than (value, error) pairs, and that is a
// property New establishes rather than an omission: New resolves every
// provider before returning, so by the time anybody holds a Container the
// graph is built and an accessor that could still fail would be a lie.
type Container struct {
	injector *do.RootScope

	config  config.Config
	logger  zerolog.Logger
	hub     *realtime.Hub
	records *records.Registry
}

// New builds the container and resolves every service in it.
//
// Resolution is eager on purpose. samber/do registers providers blindly and
// builds lazily, so a missing dependency, a constructor that fails and a cycle
// all surface at the first *use* — which for a route handler means somebody's
// first request, in production, at whatever hour that happens to be. Resolving
// here turns each of those into a boot failure and a test failure (T130).
//
// It is called once, from the composition root. Nothing else in MediKube may
// hold an injector: a package that resolves its own dependencies has hidden
// them.
func New(deps Deps) (*Container, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	// samber/do defaults Logf to a no-op. Silent is not the same as absent:
	// the injector's own account of what it built is a log line like any
	// other, and Principle VI admits exactly one stream. Debug, so it is off
	// in production and available by turning MEDIKUBE_LOG_LEVEL down.
	//
	// do calls this from several goroutines at once — it shuts services down
	// in parallel — and zerolog leaves serialisation to the destination. The
	// process logger writes to os.Stdout, where one write is one write(2); a
	// caller handing this a buffer must give it a lock of its own.
	injector := do.NewWithOpts(&do.InjectorOpts{
		Logf: func(format string, args ...any) {
			deps.Logger.Debug().Msgf(format, args...)
		},
	}, register(deps))

	container := &Container{injector: injector}

	// Every root, resolved by hand. There is no resolve-everything helper in
	// do/v2 v2.1.0 — invokeAnyByName is unexported and HealthCheckNamed
	// answers for an un-invoked service without instantiating it — so this
	// list is the mechanism, and assertNothingIsOrphaned below is what stops
	// it drifting from the providers.
	var err error

	if container.config, err = do.Invoke[config.Config](injector); err != nil {
		return nil, resolutionFailure(err)
	}

	if container.logger, err = do.Invoke[zerolog.Logger](injector); err != nil {
		return nil, resolutionFailure(err)
	}

	if container.hub, err = do.Invoke[*realtime.Hub](injector); err != nil {
		return nil, resolutionFailure(err)
	}

	if container.records, err = do.Invoke[*records.Registry](injector); err != nil {
		return nil, resolutionFailure(err)
	}

	if err := assertNothingIsOrphaned(injector); err != nil {
		return nil, err
	}

	return container, nil
}

func (c *Container) Config() config.Config { return c.config }

func (c *Container) Logger() zerolog.Logger { return c.logger }

// Hub is the realtime fan-out. The post-commit hook publishes to it and the
// stream handlers subscribe.
func (c *Container) Hub() *realtime.Hub { return c.hub }

// Records is the kind registry every clinical kind registers itself into.
func (c *Container) Records() *records.Registry { return c.records }

// Shutdown releases every service that holds something, in reverse dependency
// order, and reports what failed rather than the first failure.
//
// It is idempotent: the process may reach it from a signal handler and from
// the composition root's own defer.
func (c *Container) Shutdown() error {
	report := c.injector.Shutdown()
	if report == nil || report.Succeed {
		return nil
	}

	// Dereferenced: ShutdownReport's Error method has a value receiver, and
	// wrapping the pointer would put a type in the chain that errors.Is cannot
	// match against the value a caller would compare with.
	return fmt.Errorf("shut the MediKube container down: %w", *report)
}

// Provided and Invoked are what the wiring gate compares. They are exported
// for that test and for `medikube` diagnostics; nothing in the application
// reads them.
func (c *Container) Provided() []string { return serviceNames(c.injector.ListProvidedServices()) }

func (c *Container) Invoked() []string { return serviceNames(c.injector.ListInvokedServices()) }

func (d Deps) validate() error {
	if d.Config.DataDir == "" {
		// Not a re-run of config.Validate — that has already happened, and
		// this is the one field whose absence means the caller built a Config
		// by hand rather than loading one.
		return errors.New("di: the container was given a configuration that never came from config.Load")
	}

	return nil
}

// assertNothingIsOrphaned is the boot half of T130.
//
// A provider nothing resolves is dead wiring: it compiles, it lints, and the
// service it describes is never built. The failure then surfaces wherever
// somebody assumed it had been, which is a nil dereference on a request rather
// than a line here naming the service.
func assertNothingIsOrphaned(injector *do.RootScope) error {
	invoked := serviceNames(injector.ListInvokedServices())

	var orphans []string

	for _, provided := range serviceNames(injector.ListProvidedServices()) {
		if !slices.Contains(invoked, provided) {
			orphans = append(orphans, provided)
		}
	}

	if len(orphans) > 0 {
		return fmt.Errorf(
			"di: %s provided and never resolved: add it to New's resolution list, or delete the provider",
			strings.Join(orphans, ", "))
	}

	return nil
}

func resolutionFailure(err error) error {
	return fmt.Errorf("di: resolve the MediKube container: %w", err)
}

func serviceNames(services []do.ServiceDescription) []string {
	names := make([]string, 0, len(services))
	for _, service := range services {
		names = append(names, service.Service)
	}

	// Sorted, because ranging the injector's own map is not: an error message
	// that reorders itself between runs is one nobody can diff.
	slices.Sort(names)

	return names
}
