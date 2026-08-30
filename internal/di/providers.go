package di

import (
	"github.com/rs/zerolog"
	"github.com/samber/do/v2"

	"medikube/internal/config"
	"medikube/internal/realtime"
	"medikube/internal/records"
)

// register declares every service the container knows how to build.
//
// It is one function rather than one per subsystem so that the whole graph is
// readable in one screen, and so that "what does MediKube consist of" has a
// single answer. Every provider added here must also be resolved in New: the
// orphan check refuses a container where the two have parted company.
//
// Nothing here names a PocketBase type, and that is a boundary rather than a
// style: internal/di is not on `.golangci.yml`'s depguard exemption list, so a
// provider with a core.App in its signature does not compile past lint.
// Anything PocketBase-shaped is constructed by the composition root, which is
// [PB], and handed in.
func register(deps Deps) func(do.Injector) {
	return func(injector do.Injector) {
		// The two values that exist before the container does. They are
		// provided rather than closed over so that a service can declare a
		// dependency on the configuration instead of being handed a copy of
		// whichever fields its author remembered.
		do.ProvideValue(injector, deps.Config)
		do.ProvideValue(injector, deps.Logger)

		do.Provide(injector, provideHub)
		do.Provide(injector, provideRecordRegistry)
	}
}

// provideHub builds the realtime fan-out. *realtime.Hub implements
// do.Shutdowner, so the container closes every open stream on the way down and
// no handler is left waiting on a process that has gone.
func provideHub(do.Injector) (*realtime.Hub, error) {
	return realtime.New(), nil
}

// provideRecordRegistry builds the empty kind registry.
//
// It is empty in this phase because no kind has a service yet
// (internal/service/medication is US1). A kind registers itself into this
// value from the composition root — never from an init(), which is what keeps
// two containers independent and lets a test hold a registry the rest of the
// process cannot see.
func provideRecordRegistry(do.Injector) (*records.Registry, error) {
	return records.NewRegistry(), nil
}

// Compile-time proof that the config and logger providers name the types the
// resolution list in New invokes. A mismatch there is a runtime
// ErrServiceNotFound, and this turns it into a build failure.
var (
	_ = do.ProvideValue[config.Config]
	_ = do.ProvideValue[zerolog.Logger]
)
