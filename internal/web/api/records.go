package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/records"
	"medikube/internal/web"
)

// The operation ids of contracts/records.md's six operations. They are the keys
// the route table joins handlers on and the operationIds the Principle IX gate
// reads out of api/openapi.json, so they are constants here and matched against
// the registry by records_test.go rather than spelled twice.
const (
	OpListRecords       = "listRecords"
	OpListRecordsOfKind = "listRecordsOfKind"
	OpCreateRecord      = "createRecord"
	OpGetRecord         = "getRecord"
	OpUpdateRecord      = "updateRecord"
	OpDeleteRecord      = "deleteRecord"
)

// The two path parameters of the record family, spelled as internal/httproute
// declares them.
const (
	PathKind = "kind"
	PathID   = "id"
)

// The cross-kind list's own parameters (contracts/records.md).
const (
	ParamKind = "kind"
	ParamFrom = "from"
	ParamTo   = "to"
)

// ParamPatient is `?patient=`, required on every list over patient-scoped data
// (contracts/medications-rescope.md, FR-015). There is no fallback to the
// caller's own active patient here: every request names its patient.
const ParamPatient = "patient"

// requiredPatient reads `?patient=`, refusing its absence before anything else
// runs.
func requiredPatient(e *core.RequestEvent) (string, error) {
	patientID := e.Request.URL.Query().Get(ParamPatient)
	if patientID == "" {
		return "", web.ErrPatientRequired
	}

	return patientID, nil
}

// recordCacheControl is what a clinical record is served with. `private` keeps
// it out of every shared cache and `no-store` keeps it off disk: the response
// body is somebody's medication list, and a validator-based revalidation policy
// would still permit it to be written down.
const recordCacheControl = "private, no-store"

// Resolve hands the generic record handler to the six operations.
//
// It is a function rather than a value because the kind registry cannot be
// complete at the moment the route table is wired. A kind's repository needs
// the cursor codec, the codec is keyed from a collection secret
// (store.CursorSecret), and that collection does not exist until migrations
// have run — which apis.Serve does inside OnServe, long after the composition
// root has built its handler table. The root therefore resolves once, at boot,
// and hands back what it built; every call after the first is expected to
// return the same handler.
//
// A resolver that fails is an internal error and never the caller's: nothing
// they sent caused it and nothing they change will fix it.
type Resolve func() (*records.Handler, error)

// ErrNoRecords is a build whose record family was never resolved.
var ErrNoRecords = errors.New("api: the record operations were wired without a way to resolve the kind registry")

// Handlers is the record family's contribution to the route table: six
// operations, every registered kind, no route of its own. Phase 003 registers
// eleven more kinds and this function does not change.
func Handlers(resolve Resolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, ErrNoRecords
	}

	one, err := recordPathTemplate()
	if err != nil {
		return nil, err
	}

	handlers := &recordHandlers{resolve: resolve, one: one}

	return httproute.Handlers{
		OpListRecords:       web.WithActor(handlers.list),
		OpListRecordsOfKind: web.WithActor(handlers.listOfKind),
		OpCreateRecord:      web.WithActor(handlers.create),
		OpGetRecord:         web.WithActor(handlers.get),
		OpUpdateRecord:      web.WithActor(handlers.update),
		OpDeleteRecord:      web.WithActor(handlers.remove),
	}, nil
}

type recordHandlers struct {
	resolve Resolve

	// one is `getRecord`'s registered path with its two parameters still in
	// it. The Location header a create answers with is rendered from it, so
	// the address a client is sent to is by construction the address the
	// router serves and cannot drift from it.
	one string
}

// recordPathTemplate recovers getRecord's path from the route inventory.
//
// Reading it back rather than spelling it is what makes Location correct by
// construction: /api/v1 lives in internal/httproute, the plural lives in
// internal/domain/kind, and a create that composed its own address would be a
// third place either could be changed without.
func recordPathTemplate() (string, error) {
	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == OpGetRecord {
			return route.Path, nil
		}
	}

	return "", fmt.Errorf("api: %s is not in the route table, so a create has no address to answer with", OpGetRecord)
}

func (h *recordHandlers) location(segment, id string) string {
	address := strings.ReplaceAll(h.one, "{"+PathKind+"}", segment)

	return strings.ReplaceAll(address, "{"+PathID+"}", id)
}

// list is the cross-kind list. It publishes no ordering and no search of its
// own — the kinds it spans do not share one — so `?sort=` and `?q=` are refused
// here and served by listRecordsOfKind.
func (h *recordHandlers) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := h.resolve()
	if err != nil {
		return err
	}

	patientID, err := requiredPatient(e)
	if err != nil {
		return err
	}

	params, err := web.ListQuery(e, nil)
	if err != nil {
		return err
	}

	selected, err := h.selection(e)
	if err != nil {
		return err
	}

	page, err := handler.List(e.Request.Context(), actor, records.Query{
		Kinds:     selected,
		PatientID: patientID,
		Sort:      params.Sort,
		Limit:     params.Limit,
		Cursor:    params.Cursor,
		Count:     params.Count,
	})
	if err != nil {
		return web.OwnerScoped(err)
	}

	return h.writePage(e, page)
}

// listOfKind is one kind's list, with that kind's own published vocabulary: its
// ordering allowlist, its named filters and the shared search.
func (h *recordHandlers) listOfKind(e *core.RequestEvent, actor access.Actor) error {
	handler, err := h.resolve()
	if err != nil {
		return err
	}

	segment := e.Request.PathValue(PathKind)

	entry, err := handler.Dispatch(segment)
	if err != nil {
		return err
	}

	query, err := KindQuery(e, entry)
	if err != nil {
		return err
	}

	page, err := handler.ListOfKind(e.Request.Context(), actor, segment, query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	return h.writePage(e, page)
}

// KindQuery turns one request into one kind's list query: the shared parameters
// internal/web parses, plus the kind's own declared vocabulary.
//
// It is exported and it is the only construction of a records.Query for a
// single kind, because the JSON list and the rendered page must ask the same
// question. They did not: internal/web/page parsed the parameters and then
// built its own Query out of three of them, so `?q=` and `?status=` were
// checked and silently discarded on /medications while /api/v1/records/
// medications honoured both. A silently ignored narrowing produces a list that
// looks right and is not — the same reason contracts/records.md refuses to
// ignore an unpublished `sort` — and a second spelling is how it came about.
func KindQuery(e *core.RequestEvent, entry records.Entry) (records.Query, error) {
	patientID, err := requiredPatient(e)
	if err != nil {
		return records.Query{}, err
	}

	params, err := web.ListQuery(e, entry.Schema.Sorts)
	if err != nil {
		return records.Query{}, err
	}

	return records.Query{
		PatientID: patientID,
		Search:    params.Search,
		Filters:   filters(e, entry.Schema.Filters),
		Sort:      params.Sort,
		Limit:     params.Limit,
		Cursor:    params.Cursor,
		Count:     params.Count,
	}, nil
}

func (h *recordHandlers) create(e *core.RequestEvent, actor access.Actor) error {
	handler, err := h.resolve()
	if err != nil {
		return err
	}

	segment := e.Request.PathValue(PathKind)

	body, err := readBody(e)
	if err != nil {
		return err
	}

	created, err := handler.Create(e.Request.Context(), actor, segment, body)
	if err != nil {
		return web.OwnerScoped(err)
	}

	e.Response.Header().Set("Location", h.location(segment, created.ID))
	e.Response.Header().Set("Cache-Control", recordCacheControl)
	web.SetETag(e, created.Version)

	return web.WriteJSON(e, http.StatusCreated, created.Body)
}

func (h *recordHandlers) get(e *core.RequestEvent, actor access.Actor) error {
	handler, err := h.resolve()
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		e.Request.PathValue(PathKind), e.Request.PathValue(PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	return h.writeRecord(e, http.StatusOK, found)
}

// update applies the supplied members and nothing else.
//
// If-Match is required rather than honoured: an optional precondition is a
// precondition nobody sends (FR-026). A mismatch answers 412 carrying the
// record as it now stands, so "the current values are shown so they can decide
// what to do" is a property of the response and not a second request the page
// has to remember to make.
func (h *recordHandlers) update(e *core.RequestEvent, actor access.Actor) error {
	handler, err := h.resolve()
	if err != nil {
		return err
	}

	segment, id := e.Request.PathValue(PathKind), e.Request.PathValue(PathID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	body, err := readBody(e)
	if err != nil {
		return err
	}

	updated, err := handler.Update(e.Request.Context(), actor, segment, id, version, body)
	if err != nil {
		return h.stale(e, actor, segment, id, err)
	}

	return h.writeRecord(e, http.StatusOK, updated)
}

func (h *recordHandlers) remove(e *core.RequestEvent, actor access.Actor) error {
	handler, err := h.resolve()
	if err != nil {
		return err
	}

	segment, id := e.Request.PathValue(PathKind), e.Request.PathValue(PathID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	if err := handler.Delete(e.Request.Context(), actor, segment, id, version); err != nil {
		return h.stale(e, actor, segment, id, err)
	}

	e.Response.Header().Set("Cache-Control", recordCacheControl)

	return e.NoContent(http.StatusNoContent)
}

// stale turns a version mismatch into the 412 the contract specifies and hands
// everything else on unchanged.
//
// The re-read is deliberate and is the whole value of the response: the caller
// is holding a version that is no longer current, and the only thing that helps
// is the one that is. A re-read that fails answers as the mismatch it is rather
// than as a 500 — the write did not happen either way, and a 500 would send a
// client into a retry loop over a precondition that will never pass.
func (h *recordHandlers) stale(e *core.RequestEvent, actor access.Actor, segment, id string, err error) error {
	if !errors.Is(err, domain.ErrVersionMismatch) {
		return web.OwnerScoped(err)
	}

	handler, resolveErr := h.resolve()
	if resolveErr != nil {
		return resolveErr
	}

	current, readErr := handler.Get(e.Request.Context(), actor, segment, id)
	if readErr != nil {
		return web.OwnerScoped(readErr)
	}

	e.Response.Header().Set("Cache-Control", recordCacheControl)

	return web.WriteVersionMismatch(e, obs.CorrelationID(e.Request.Context()), current.Version, current.Body)
}

func (h *recordHandlers) writeRecord(e *core.RequestEvent, status int, record records.Record) error {
	e.Response.Header().Set("Cache-Control", recordCacheControl)
	web.SetETag(e, record.Version)

	return web.WriteJSON(e, status, record.Body)
}

// writePage writes the list envelope. The items are the kinds' own DTOs, so the
// envelope is rebuilt over `any` rather than marshalled as a page of
// records.Record — which carries an id, a kind and a version that are not the
// published shape.
func (h *recordHandlers) writePage(e *core.RequestEvent, page domain.Page[records.Record]) error {
	bodies := make([]any, 0, len(page.Items))
	for _, item := range page.Items {
		bodies = append(bodies, item.Body)
	}

	envelope := domain.NewPage(bodies, page.NextCursor)
	if page.Total != nil {
		envelope = envelope.WithTotal(*page.Total)
	}

	e.Response.Header().Set("Cache-Control", recordCacheControl)

	return web.WriteJSON(e, http.StatusOK, envelope)
}

// selection reads the cross-kind list's `kind` parameter.
//
// An unregistered value here is 422 and not 404, and the asymmetry with the
// path is deliberate: a query parameter is reached only by a caller who already
// knows the path exists, so naming the offending parameter discloses nothing it
// did not already have. A path segment is the other case and stays a 404.
//
// `from` and `to` are refused rather than ignored. contracts/records.md
// documents them and records.Query has no member to carry them, so serving them
// would mean answering an unnarrowed list to a caller who asked for a narrowed
// one — the same silent wrongness the contract rules out for `sort`.
func (h *recordHandlers) selection(e *core.RequestEvent) ([]kind.Kind, error) {
	query := e.Request.URL.Query()

	var invalid domain.ValidationError

	for _, name := range []string{ParamFrom, ParamTo} {
		if query.Get(name) != "" {
			invalid.Add(name, domain.CodeInvalidValue,
				"this instance does not narrow the cross-kind list by date yet, and answering an unnarrowed list would be worse than saying so")
		}
	}

	var selected []kind.Kind

	for _, segment := range commaList(query.Get(ParamKind)) {
		k, declared := kind.FromSegment(segment)
		if !declared {
			invalid.Add(ParamKind, domain.CodeInvalidValue, "the kind is not one this instance serves")

			continue
		}

		selected = append(selected, k)
	}

	if err := invalid.OrNil(); err != nil {
		return nil, err
	}

	return selected, nil
}

// filters reads the kind's own named parameters and nothing else. A parameter
// the kind does not publish is not read at all, so PocketBase's filter DSL has
// no member to arrive in.
func filters(e *core.RequestEvent, declared []string) map[string][]string {
	if len(declared) == 0 {
		return nil
	}

	query := e.Request.URL.Query()
	supplied := make(map[string][]string, len(declared))

	for _, name := range declared {
		if values := commaList(query.Get(name)); len(values) > 0 {
			supplied[name] = values
		}
	}

	if len(supplied) == 0 {
		return nil
	}

	return supplied
}

// commaList splits one comma-separated parameter. Whitespace is not trimmed:
// contracts/README.md says so, and trimming would make ` active` a second
// spelling of a published value.
func commaList(raw string) []string {
	if raw == "" {
		return nil
	}

	return strings.Split(raw, ",")
}

// readBody reads the whole request body.
//
// PocketBase wraps every body in a reader that rewinds on EOF
// (tools/router/router.go:136), so a decoder that scans for trailing
// whitespace reads the document twice and every decode fails; the body is
// bounded by PocketBase's own BodyLimit middleware, which is what makes reading
// it whole safe. internal/web/dto.go documents the same trap.
func readBody(e *core.RequestEvent) ([]byte, error) {
	raw, err := io.ReadAll(e.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("api: reading the request body: %w", err)
	}

	return raw, nil
}
