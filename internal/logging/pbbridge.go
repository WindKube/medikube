package logging

import (
	"log/slog"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"
)

// pbSource marks the lines PocketBase wrote. One stream to read, but a reader
// still has to be able to tell whose failure it is looking at.
const pbSource = "pocketbase"

// loggedApp is the decorator. Embedding the core.App interface promotes every
// other method — including the unexported ones no external type could
// implement — so the wrapper stays complete across a PocketBase upgrade that
// adds methods.
type loggedApp struct {
	core.App

	logger *slog.Logger
}

func (a *loggedApp) Logger() *slog.Logger { return a.logger }

// BridgeApp is mechanism 1 of the log bridge (CT-1): it reassigns the exported
// embedded core.App field on pocketbase.PocketBase to a decorator whose
// Logger() resolves into zerolog.
//
// There is no supported injection point. core.BaseApp.initLogger hardcodes its
// batch handler and app.logger has no setter, so decorating the one exported
// seam is the only way in. It works because PocketBase assigns event.App per
// event rather than capturing a logger at construction, so Logger() resolves
// through the decorator dynamically at call time (research D-29).
//
// This covers everything reached through the core.App value MediKube handed to
// PocketBase — the request path above all. It does NOT cover transaction-scoped
// logging, because createTxApp shallow-copies a *BaseApp and keeps the internal
// logger; BridgeLogs covers that. Both, or there is a hole.
//
// Call it before Bootstrap: the decorator is a field assignment and costs
// nothing, and the lines PocketBase writes on its way up are worth having.
func BridgeApp(pb *pocketbase.PocketBase, base zerolog.Logger) {
	pb.App = &loggedApp{
		App:    pb.App,
		logger: slog.New(zerolog.NewSlogHandler(base.With().Str("src", pbSource).Logger())),
	}
}
