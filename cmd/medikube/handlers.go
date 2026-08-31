package main

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/realtime"
	"medikube/internal/records"
	accessservice "medikube/internal/service/access"
	auditservice "medikube/internal/service/audit"
	"medikube/internal/service/medication"
	"medikube/internal/store"
	auditstore "medikube/internal/store/audit"
	medicationstore "medikube/internal/store/medication"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/page"
	"medikube/internal/web/stream"
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
func operations(resolve api.Resolve, hub *realtime.Hub) (httproute.Handlers, error) {
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
	served, err := wired(resolve, hub)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, served)

	return table, nil
}

// wired is where each group's handlers arrive as they land. The record family,
// the two record pages and the Datastar stream are in; internal/web/api's auth
// and account halves are US2's.
func wired(resolve api.Resolve, hub *realtime.Hub) (httproute.Handlers, error) {
	table := make(httproute.Handlers)

	records, err := api.Handlers(resolve)
	if err != nil {
		return nil, err
	}

	pages, err := page.Handlers(resolve)
	if err != nil {
		return nil, err
	}

	streams, err := stream.Handlers(stream.Deps{Resolve: resolve, Hub: hub})
	if err != nil {
		return nil, err
	}

	maps.Copy(table, records)
	maps.Copy(table, pages)
	maps.Copy(table, streams)

	return table, nil
}

// recordFamily resolves the kind registry, once, on first use.
//
// It cannot be resolved when the route table is wired, and that is a property
// of the instance rather than an inconvenience: a kind's repository needs the
// cursor codec, the codec is keyed from the auth collection's persisted secret
// (store.CursorSecret), and that collection does not exist until the
// migrations have run — which apis.Serve does inside OnServe, after this table
// has been built and bound.
//
// The boot gate calls it before the instance serves anything, so a
// registration that cannot be built is a boot failure and not a 500 on
// somebody's first request.
func recordFamily(app core.App, registry *records.Registry, hub *realtime.Hub) api.Resolve {
	return sync.OnceValues(func() (*records.Handler, error) {
		if err := registerKinds(app, registry, hub); err != nil {
			return nil, err
		}

		return records.NewHandler(registry), nil
	})
}

// registerKinds is the extension point phases 002 through 006 add a kind to.
// One call per kind, seven consumers wired by it, and no route.
func registerKinds(app core.App, registry *records.Registry, hub *realtime.Hub) error {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return err
	}

	cursors, err := store.NewCursorCodec(secret)
	if err != nil {
		return err
	}

	owners, err := store.NewOwners(app)
	if err != nil {
		return err
	}

	authorizer, err := accessservice.New(owners)
	if err != nil {
		return err
	}

	trail, err := auditstore.New(app)
	if err != nil {
		return err
	}

	auditor, err := auditservice.New(trail)
	if err != nil {
		return err
	}

	views, err := page.NewMedicationViews()
	if err != nil {
		return err
	}

	repository, err := medicationstore.New(app, cursors)
	if err != nil {
		return err
	}

	if err := medication.Register(registry, medication.Wiring{
		Repository: repository,
		Authorizer: authorizer,
		Auditor:    auditor,
		Codec:      api.MedicationCodec{},
		Schema:     api.MedicationSchema(),
		Views:      views,
	}); err != nil {
		return err
	}

	// FR-036's three rows, written by the post-commit hooks and by no handler
	// (research D-21). Bound after the kinds are registered, so it audits
	// exactly what this build serves and nothing else.
	if err := pb.BindRecordAudit(app, pb.RecordAudit{
		Trail:   auditor,
		Kinds:   registry.Kinds(),
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); err != nil {
		return err
	}

	// contracts/streams.md's publisher, bound to the same three post-commit
	// hooks and to the same kinds: a live view of a kind this build does not
	// serve is a live view of nothing.
	return pb.BindRecordStream(app, pb.RecordStream{Hub: hub, Kinds: registry.Kinds()})
}

// unimplemented lists, sorted, the operations still answered by the stub.
//
// It resolves nothing: the handler table's shape is decided by which groups
// have landed, not by whether an instance could build one, so the resolver
// handed in here is one that is never called.
func unimplemented() []string {
	implemented, err := wired(func() (*records.Handler, error) { return nil, nil }, realtime.New())
	if err != nil {
		panic("medikube: the handler groups cannot be assembled: " + err.Error())
	}

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
