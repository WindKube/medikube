package main

import (
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/httproute"
	"medikube/internal/web"
)

// operations is MediKube's handler table: the handlers this build has, and a
// 501 for every route whose handler is a later task.
//
// The stub set is derived from the route table rather than listed, so a route
// added to internal/httproute/routes.go cannot leave the composition root
// unable to boot, and a handler wired under a name that is not a route is
// still refused by httproute.New. What is *not* derived is the inventory of
// what is still missing: that lives in cmd/medikube/main_test.go, where a
// finished handler left behind a stub is a failing test and a one-line diff.
func operations() httproute.Handlers {
	table := make(httproute.Handlers)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			// PocketBase's own. Binding one would shadow the real handler
			// with MediKube's, which is what httproute.Handle refuses.
			continue
		}

		table[route.OpID] = notImplemented(route.OpID)
	}

	// The real ones win. This is the only line each later group touches.
	maps.Copy(table, wired())

	return table
}

// wired is where each group's handlers arrive as they land:
//
//	maps.Copy(table, api.Handlers(deps))
//	maps.Copy(table, page.Handlers(deps))
//	maps.Copy(table, stream.Handlers(deps))
//
// It is empty in this phase because internal/web/api, internal/web/page and
// internal/web/stream are US1 and US2. An empty table is an honest state and a
// boot that refused to start because of it would be a phase that cannot be
// checkpointed.
func wired() httproute.Handlers {
	return httproute.Handlers{}
}

// unimplemented lists, sorted, the operations still answered by the stub.
func unimplemented() []string {
	implemented := wired()

	var pending []string

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			continue
		}

		if _, done := implemented[route.OpID]; done {
			continue
		}

		pending = append(pending, route.OpID)
	}

	slices.Sort(pending)

	return pending
}

// errNotImplemented is a 501 in MediKube's own taxonomy.
//
// contracts/README.md's table has no code for it, and inventing one would put
// a value in the published vocabulary that no operation documents. It is
// internal_error instead, which is what every other 5xx MediKube did not
// choose resolves to: the status says what happened and the code says whose
// fault it is.
var errNotImplemented = &web.Coded{Status: http.StatusNotImplemented, Code: web.CodeInternal}

// notImplemented answers a documented operation whose handler does not exist.
//
// The operation id is in the error and therefore in the log line, and it is
// not in the response: the client is told the same thing every internal
// failure tells it, because which handlers are missing is not a client's
// business.
func notImplemented(opID string) httproute.Handler {
	return func(*core.RequestEvent) error {
		return fmt.Errorf("%s has no handler in this build: %w", opID, errNotImplemented)
	}
}
