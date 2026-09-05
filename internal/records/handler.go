package records

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/service/search"
)

// IfMatchField is the field name a missing precondition is reported against.
// It is the header's own spelling because that is what the caller has to add,
// and contracts/records.md names it in the 422 body.
const IfMatchField = "If-Match"

// crossKindSort is the cross-kind list's one published ordering: most recent
// first, the same term search.SortOccurredOn names on the index side. A
// `sort` other than the default is refused exactly as an unknown one is on a
// single kind's list (contracts/records.md) — the merged page has one
// ordering, not an allowlist.
var crossKindSort = domain.SortKey{Field: "occurred_on", Desc: true}

// Handler is the ONE generic record handler. It resolves {kind} to a
// registration, decodes into that kind's own typed DTO and calls that kind's
// service — and that is the whole of the kind-specific knowledge in the record
// family. Every later phase adds a kind and no route.
//
// Nothing here switches on kind.Kind. If a `switch k` ever appears below, the
// registry has failed at the one job it exists for.
type Handler struct {
	// dispatch is the table T108 asserts covers every registered kind. It is a
	// snapshot taken at construction rather than a live view of the registry,
	// because everything is registered before anything listens and a table that
	// could change under a request is a table two requests can disagree about.
	dispatch map[string]Entry
	order    []string

	// byKind answers the cross-kind list's `kind` selection. A synthetic kind
	// has no row in the kind table, so its Segment() is empty and a lookup
	// routed through it would silently match nothing — which is how the
	// second registered kind would drop out of the selection unnoticed.
	byKind map[kind.Kind]Entry

	// searchReader pages search_index for a cross-kind list, and is nil
	// wherever the registry was never given one (SetSearchReader) — a
	// registration built for a test that never selects two kinds at once.
	searchReader search.Reader
}

func NewHandler(registry *Registry) *Handler {
	entries := registry.Entries()

	handler := &Handler{
		dispatch:     make(map[string]Entry, len(entries)),
		order:        make([]string, 0, len(entries)),
		byKind:       make(map[kind.Kind]Entry, len(entries)),
		searchReader: registry.searchReader,
	}

	for _, entry := range entries {
		handler.dispatch[entry.Segment] = entry
		handler.byKind[entry.Kind] = entry
		handler.order = append(handler.order, entry.Segment)
	}

	return handler
}

// Segments is the dispatch table's key set in registration order.
func (h *Handler) Segments() []string { return slices.Clone(h.order) }

// Dispatch resolves a {kind} path segment.
//
// An unregistered segment is ErrNotFound and never a validation failure: a 400
// would say "that is not a kind", which tells an anonymous prober which kinds
// exist — the same disclosure FR-033 closes on record ids, arriving one level
// up the path. The match is exact, so a different case or a trailing slash is
// a different path and not a second spelling of this one.
func (h *Handler) Dispatch(segment string) (Entry, error) {
	entry, registered := h.dispatch[segment]
	if !registered {
		return Entry{}, fmt.Errorf("records: no kind is registered at that path: %w", domain.ErrNotFound)
	}

	return entry, nil
}

// List is the cross-kind list. An empty selection means every registered kind.
func (h *Handler) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[Record], error) {
	selected, err := h.selection(query.Kinds)
	if err != nil {
		return domain.Page[Record]{}, err
	}

	switch len(selected) {
	case 0:
		return domain.NewPage([]Record{}, nil), nil
	case 1:
		return h.list(ctx, actor, selected[0], query)
	default:
		return h.crossKindList(ctx, actor, selected, query)
	}
}

// crossKindList pages search_index directly, ordered by occurred_on, and
// hydrates every ref it reads through that ref's own kind's Service.Get —
// which is where FR-033's ownership check already lives, and stays.
//
// A ref whose record is gone by the time it is hydrated (deleted between the
// index read and this call) is skipped rather than failing the page: the
// index is a maintained *index*, not the record, and its being one request
// stale is not the caller's problem.
func (h *Handler) crossKindList(
	ctx context.Context, actor access.Actor, selected []Entry, query Query,
) (domain.Page[Record], error) {
	if h.searchReader == nil {
		return domain.Page[Record]{}, fmt.Errorf(
			"records: the cross-kind list has no search index reader to page: %w", domain.ErrNotFound)
	}

	// The patient scope is authorized once, up front, against any one
	// selected kind's checkpoint: every registered kind anchors on the same
	// patient (registry.go's own doc), and a per-ref hydrate cannot answer
	// "why is the caller not even allowed to list this patient at all" —
	// only "is this one record theirs" — which is the wrong refusal for a
	// stranger naming somebody else's patient.
	if _, err := selected[0].Authorizer.Patient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return domain.Page[Record]{}, err
	}

	if len(query.Sort) > 0 && (len(query.Sort) != 1 || query.Sort[0] != crossKindSort) {
		var invalid domain.ValidationError
		invalid.Add("sort", domain.CodeInvalidValue, "the cross-kind list publishes one ordering and it is not this one")

		return domain.Page[Record]{}, invalid.OrNil()
	}

	if query.Search != "" || len(query.Filters) > 0 {
		return domain.Page[Record]{}, fmt.Errorf(
			"%w: the cross-kind list has no search and no named filters of its own", domain.ErrBadRequest)
	}

	kinds := make([]kind.Kind, 0, len(selected))
	for _, entry := range selected {
		kinds = append(kinds, entry.Kind)
	}

	refs, err := h.searchReader.Page(ctx, query.PatientID, kinds, query.Limit, query.Cursor)
	if err != nil {
		return domain.Page[Record]{}, err
	}

	items := make([]Record, 0, len(refs.Items))

	for _, ref := range refs.Items {
		entry, registered := h.byKind[ref.Kind]
		if !registered {
			continue
		}

		record, getErr := entry.Service.Get(ctx, actor, ref.RecordID)
		if errors.Is(getErr, domain.ErrNotFound) {
			continue
		}

		if getErr != nil {
			return domain.Page[Record]{}, getErr
		}

		items = append(items, record)
	}

	page := domain.NewPage(items, refs.NextCursor)

	if query.Count {
		total, countErr := h.searchReader.Count(ctx, query.PatientID, kinds)
		if countErr != nil {
			return domain.Page[Record]{}, countErr
		}

		page = page.WithTotal(total)
	}

	return page, nil
}

// ListOfKind is one kind's list. The kind comes from the path, so a selection
// in the query is ignored rather than intersected: two places naming the kind
// is one place too many.
func (h *Handler) ListOfKind(ctx context.Context, actor access.Actor, segment string, query Query) (domain.Page[Record], error) {
	entry, err := h.Dispatch(segment)
	if err != nil {
		return domain.Page[Record]{}, err
	}

	return h.list(ctx, actor, entry, query)
}

func (h *Handler) Create(ctx context.Context, actor access.Actor, segment string, body []byte) (Record, error) {
	entry, err := h.Dispatch(segment)
	if err != nil {
		return Record{}, err
	}

	decoded := entry.Schema.NewCreate()
	if err := decode(body, decoded); err != nil {
		return Record{}, err
	}

	return entry.Service.Create(ctx, actor, decoded)
}

// DecodeCreate and DecodePatch decode a request body into the kind's own
// create/patch shape without calling the service. Create and Update discard
// the decoded value on every path that does not end in a save, and a caller
// that must re-render a rejected submission — the Datastar form patch —
// needs what was typed rather than what was saved.
func (h *Handler) DecodeCreate(segment string, body []byte) (Entry, any, error) {
	entry, err := h.Dispatch(segment)
	if err != nil {
		return Entry{}, nil, err
	}

	decoded := entry.Schema.NewCreate()
	if err := decode(body, decoded); err != nil {
		return entry, nil, err
	}

	return entry, decoded, nil
}

func (h *Handler) DecodePatch(segment string, body []byte) (Entry, any, error) {
	entry, err := h.Dispatch(segment)
	if err != nil {
		return Entry{}, nil, err
	}

	decoded := entry.Schema.NewPatch()
	if err := decode(body, decoded); err != nil {
		return entry, nil, err
	}

	return entry, decoded, nil
}

func (h *Handler) Get(ctx context.Context, actor access.Actor, segment, id string) (Record, error) {
	entry, err := h.Dispatch(segment)
	if err != nil {
		return Record{}, err
	}

	return entry.Service.Get(ctx, actor, id)
}

// Update requires If-Match. An optional precondition is a precondition nobody
// sends (FR-026), and requiring it here is what gives every later kind the rule
// without writing it again.
func (h *Handler) Update(ctx context.Context, actor access.Actor, segment, id, version string, body []byte) (Record, error) {
	entry, err := h.Dispatch(segment)
	if err != nil {
		return Record{}, err
	}

	if err := requirePrecondition(version); err != nil {
		return Record{}, err
	}

	decoded := entry.Schema.NewPatch()
	if err := decode(body, decoded); err != nil {
		return Record{}, err
	}

	return entry.Service.Update(ctx, actor, id, version, decoded)
}

// Delete requires If-Match for the same reason Update does: deleting the row
// you last saw is a different act from deleting whatever is there now.
func (h *Handler) Delete(ctx context.Context, actor access.Actor, segment, id, version string) error {
	entry, err := h.Dispatch(segment)
	if err != nil {
		return err
	}

	if err := requirePrecondition(version); err != nil {
		return err
	}

	return entry.Service.Delete(ctx, actor, id, version)
}

func (h *Handler) list(ctx context.Context, actor access.Actor, entry Entry, query Query) (domain.Page[Record], error) {
	resolved, err := resolveQuery(entry, query)
	if err != nil {
		return domain.Page[Record]{}, err
	}

	return entry.Service.List(ctx, actor, resolved)
}

// selection resolves the cross-kind list's `kind` parameter. An unregistered
// value there is a validation failure and not a 404: a query parameter is
// reached only by a caller who already knows the path exists, so naming the
// offending value discloses nothing it did not already have. A path segment is
// the other case and stays a 404.
func (h *Handler) selection(selected []kind.Kind) ([]Entry, error) {
	if len(selected) == 0 {
		entries := make([]Entry, 0, len(h.order))
		for _, segment := range h.order {
			entries = append(entries, h.dispatch[segment])
		}

		return entries, nil
	}

	var (
		entries []Entry
		invalid domain.ValidationError
	)

	for _, k := range selected {
		entry, registered := h.byKind[k]
		if !registered {
			invalid.Add("kind", domain.CodeInvalidValue, "the kind is not one this instance serves")

			continue
		}

		entries = append(entries, entry)
	}

	if err := invalid.OrNil(); err != nil {
		return nil, err
	}

	return entries, nil
}

// resolveQuery applies the kind's declared vocabulary. A sort outside the
// allowlist is 422 invalid_value and never silently ignored, because a
// silently ignored sort produces a list that looks right and is not. A filter
// outside the kind's named parameters, or a value outside its declared
// vocabulary, is 400 bad_request (contracts/records-clinical.md §1) — a
// caller guessing at PocketBase's filter DSL, which never reaches the wire,
// and not a rejected field on a form.
func resolveQuery(entry Entry, query Query) (Query, error) {
	var invalid domain.ValidationError

	if len(query.Sort) == 0 {
		query.Sort = []domain.SortKey{entry.Schema.Sorts[0]}
	}

	for _, term := range query.Sort {
		if !slices.Contains(entry.Schema.Sorts, term) {
			invalid.Add("sort", domain.CodeInvalidValue, "the sort is not one this kind publishes")
		}
	}

	if err := invalid.OrNil(); err != nil {
		return Query{}, err
	}

	resolved, err := checkFilters(entry.Schema.Filters, query.Filters)
	if err != nil {
		return Query{}, err
	}

	query.Filters = resolved
	query.Kinds = []kind.Kind{entry.Kind}

	return query, nil
}

func requirePrecondition(version string) error {
	if version != "" {
		return nil
	}

	var invalid domain.ValidationError
	invalid.Add(IfMatchField, domain.CodeRequired,
		"the version you are replacing is required, so a write cannot silently overwrite a change you have not seen")

	return invalid.OrNil()
}

// decode is the typed DTO boundary. Unknown members are rejected, which is what
// makes FR-032 a property of the shape rather than a runtime check: neither
// write DTO has an `owner` field, so a body carrying one is refused here.
//
// Duplicate member names and case-mismatched names are rejected too, both by
// encoding/json/v2's defaults (research D-28).
func decode(body []byte, target any) error {
	if err := json.Unmarshal(body, target, json.RejectUnknownMembers(true)); err != nil {
		return decodeFailure(err)
	}

	return nil
}

// decodeFailure translates a decoder error into MediKube's own field errors,
// and deliberately drops everything the decoder said.
//
// Go's own text embeds the submitted value — `cannot unmarshal JSON number
// 99999999999999999999 into Go int`, and the RFC3339 parse failure quotes the
// string in full — and on this application's DTOs the submitted value is
// medical data. The JSON pointer is machine-recoverable and carries only the
// member name (research D-28).
//
// The member name goes through domain.SafeFieldName first, and it has to. For
// an unknown member the name is by definition one MediKube does not publish:
// it is whatever the client sent, unbounded and unfiltered, and it would
// otherwise reach both the response body and the one log stream verbatim.
func decodeFailure(err error) error {
	var invalid domain.ValidationError

	var semantic *json.SemanticError
	if errors.As(err, &semantic) {
		field := domain.SafeFieldName(semantic.JSONPointer.LastToken())
		if field == "" {
			field = "body"
		}

		if errors.Is(semantic.Err, json.ErrUnknownName) {
			invalid.Add(field, domain.CodeUnknownField, "the field is not one this operation accepts")

			return invalid.OrNil()
		}

		invalid.Add(field, domain.CodeInvalidValue, "the value is not the shape this field takes")

		return invalid.OrNil()
	}

	// A duplicate member name is syntactic, not semantic — encoding/json/v2
	// refuses it with no option asked for (research D-28) — and its pointer
	// still names the member, which is a field name MediKube published.
	var syntactic *jsontext.SyntacticError
	if errors.As(err, &syntactic) {
		if field := domain.SafeFieldName(syntactic.JSONPointer.LastToken()); field != "" {
			invalid.Add(field, domain.CodeInvalidValue, "the field was sent more than once")

			return invalid.OrNil()
		}
	}

	invalid.Add("body", domain.CodeInvalidValue, "the request body is not a JSON object this operation accepts")

	return invalid.OrNil()
}
