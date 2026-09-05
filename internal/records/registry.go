package records

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/service/search"
)

// The seven consumers of a kind, named. T104 asserts that one Register call
// wires every one of them and that a registration missing any one is refused,
// and it drives both halves from Consumers() — so a consumer added here with no
// check beside it fails that test rather than shipping unchecked.
const (
	// ConsumerRoutes is /api/v1/records/{kind} and the two pages: the path
	// segment and the collection, both read from the kind table and never
	// spelled in a registration (research D-05).
	ConsumerRoutes = "routes"
	// ConsumerSchema is the kind's oneOf branch and its query vocabulary.
	ConsumerSchema = "openapi schema"
	// ConsumerViews is the list, row, detail and form components.
	ConsumerViews = "page views"
	// ConsumerStream is which of the kind's changes reach a live view.
	ConsumerStream = "stream filter"
	// ConsumerAudit is the audit.TargetKind the post-commit hooks write.
	ConsumerAudit = "audit target"
	// ConsumerAuthorizer is the checkpoint the kind's records are reached
	// through.
	ConsumerAuthorizer = "authorizer"
	// ConsumerInventory is the row the operator surface prints.
	ConsumerInventory = "CLI inventory"
)

var consumers = []string{
	ConsumerRoutes,
	ConsumerSchema,
	ConsumerViews,
	ConsumerStream,
	ConsumerAudit,
	ConsumerAuthorizer,
	ConsumerInventory,
}

// Consumers is the published list, cloned so a caller that sorted it could not
// reorder the refusal message for everybody.
func Consumers() []string { return slices.Clone(consumers) }

var (
	// ErrKindUndeclared is a kind that internal/domain/kind does not declare.
	// It has no path segment and no collection to derive, so there is no route
	// to register and nothing FromSegment could answer.
	ErrKindUndeclared = errors.New("records: the kind is not declared in internal/domain/kind")

	// ErrKindDeclared is the mirror: a declared kind offered to the synthetic
	// door, which would give it a second path segment beside the one the kind
	// table declares — exactly the drift D-05 exists to stop.
	ErrKindDeclared = errors.New("records: the kind is declared in internal/domain/kind and takes its spelling from there")

	ErrAlreadyRegistered = errors.New("records: the kind is already registered")
	ErrSegmentTaken      = errors.New("records: the path segment is already registered to another kind")
	ErrCollectionTaken   = errors.New("records: the collection is already registered to another kind")

	// ErrAuditTargetMismatch is a registration whose audit target is not the
	// kind's own. The two vocabularies are declared complete in two different
	// files and this is the one place both are known.
	ErrAuditTargetMismatch = errors.New("records: the audit target is not this kind's")
)

// IncompleteError names every consumer a registration left unwired, not the
// first one. Reporting them one at a time costs one boot per consumer to fix,
// which is FR-027's argument applied to the operator rather than to the person
// filling in a form.
type IncompleteError struct {
	Kind kind.Kind
	// In the order Consumers() declares them.
	Missing []string
}

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("records: %s wires no %s", e.Kind, strings.Join(e.Missing, ", no "))
}

// Registration is everything one kind supplies. Every field is required:
// a kind that reached the router but not the audit trail is reachable and
// untraceable, and a kind that reached the authorizer but not the stream is a
// live view that silently stops updating. Both look like a working boot, which
// is why they are refused at registration instead.
type Registration struct {
	// Kind is the whole of the routes consumer. Its segment and its collection
	// come from the kind table, so a registration cannot give a kind a second
	// spelling.
	Kind kind.Kind

	// Service is what the generic handler dispatches to. It is not one of the
	// seven consumers — it is the thing they are consumers of — and it is just
	// as mandatory.
	Service Service

	Schema     Schema
	Views      Views
	Stream     StreamFilter
	Target     audit.TargetKind
	Authorizer Authorizer
	Inventory  Inventory

	// SearchFields extracts the unified search index's two columns from the
	// record body the kind's own Service just returned (research D-11): the
	// same value Views reads, so the indexed text and the rendered text
	// cannot drift into two different ideas of "the record".
	SearchFields func(any) (title, body string)

	// Basis states, per row, why a row narrowed by Criteria qualifies, for
	// narrowings where rows qualify for materially different reasons —
	// "overdue" against "due soon", "scheduled" against "ordered" (research
	// D-05). A kind with no such narrowing still declares it; it returns nil.
	Basis func(any, Criteria) []string

	// SeedFixtureID names the fixture `medikube seed` and the shared contract
	// suites build for this kind (internal/records/recordstest).
	SeedFixtureID string
}

// Entry is a registration with its spellings resolved. It is what all four
// consumers read, and it is handed out by value.
type Entry struct {
	Registration

	Segment    string
	Collection string

	// Synthetic marks a kind the production vocabulary does not declare. There
	// is exactly one — recordstest's — and it exists so the discriminated
	// oneOf can be gated with two branches in a phase that ships one kind
	// (plan.md CT-2, research D-08). internal/di asserts a production registry
	// has none.
	Synthetic bool
}

// SchemaName is the kind's component name in the OpenAPI document. It is built
// from the path segment — plural — because that is the spelling research D-08's
// worked example uses and the one the generated clients name their types after.
func (e Entry) SchemaName() string { return "Record_" + e.Segment }

// DiscriminatorValue is the value of the body's `kind` member, which is the
// enum spelling — singular — and not the path segment. The two are different
// strings on purpose and Discriminator.Mapping is keyed by this one; keying it
// by the segment mis-dispatches every generated client and no OpenAPI validator
// will say so (research D-08, and kin-openapi never resolves a mapping ref).
func (e Entry) DiscriminatorValue() string { return e.Kind.Enum() }

// InventoryRow is one line of the operator surface's kind listing.
type InventoryRow struct {
	Kind    kind.Kind
	Segment string
	Title   string
	Summary string
}

// Registry is the extension point: one Register call wires every consumer of a
// kind, and phases 002 through 006 add kinds through it and add no routes.
//
// It is not safe for concurrent registration and does not need to be —
// everything is registered once, from internal/di, before anything listens —
// and it is a value constructed there rather than a package global, so a test
// builds its own and two of them cannot see each other's kinds.
type Registry struct {
	entries []Entry

	byKind       map[kind.Kind]int
	bySegment    map[string]int
	byCollection map[string]int

	// indexer is what every kind registered from here on writes and removes
	// search_index rows through. It is set once, before any kind registers
	// (SetIndexer), and is nil in every test that has no search_index to
	// write to — a registry with no indexer simply does not index, rather
	// than requiring one for every fixture that only needs a registration.
	indexer *search.Indexer

	// searchReader is the cross-kind list's read side: Handler.List pages it
	// directly once more than one kind is selected, because no per-kind
	// keyset cursor can continue a page merged from more than one kind's
	// table. Nil is a legitimate value the same way indexer's is — a
	// registry with no reader simply has no cross-kind list to serve, and
	// every unit-level test that registers two kinds without one never
	// exercises that path.
	searchReader search.Reader

	// tagChecker is FR-064's ownership check, wired into every kind
	// registered from this point on (SetTagChecker). Nil is legitimate the
	// same way indexer's is: a registry with no checker simply does not
	// validate tag ownership, which is every fixture that has no tags
	// collection to check against.
	tagChecker TagChecker
}

// SetIndexer wires the search index's write side into every kind registered
// from this point on. It is a setter and not a NewRegistry parameter because
// the registry is built once, empty, from internal/di (which is not [PB] and
// so cannot construct a search.Indexer), and wired here by the composition
// root before the first kind registers.
func (r *Registry) SetIndexer(indexer *search.Indexer) { r.indexer = indexer }

// SetSearchReader wires the search index's read side into the registry, for
// Handler.List's cross-kind page. Like SetIndexer it is a setter and not a
// NewRegistry parameter, and for the same reason: the registry is built once,
// empty, from internal/di, before the search.Reader it would need exists.
func (r *Registry) SetSearchReader(reader search.Reader) { r.searchReader = reader }

func NewRegistry() *Registry {
	return &Registry{
		byKind:       make(map[kind.Kind]int),
		bySegment:    make(map[string]int),
		byCollection: make(map[string]int),
	}
}

// Register wires a kind declared in internal/domain/kind. The segment and the
// collection are read from that table and are never supplied here, so the three
// spellings of a kind cannot drift apart.
func (r *Registry) Register(registration Registration) error {
	if !registration.Kind.Valid() {
		return fmt.Errorf("%w: %q wires no %s", ErrKindUndeclared, registration.Kind, ConsumerRoutes)
	}

	return r.add(registration, registration.Kind.Segment(), registration.Kind.Collection(), false)
}

// RegisterSynthetic wires a kind the production vocabulary does not declare,
// taking its two spellings explicitly because there is no table row to read
// them from.
//
// It exists for one caller: internal/records/recordstest, whose second kind is
// what makes the discriminated oneOf gate meaningful in a phase that ships one
// real kind and what gives Service its second implementation (plan.md CT-2).
// Everything a real registration is checked for is checked here too — a door
// that skipped the checks would only prove the checks can be skipped.
func (r *Registry) RegisterSynthetic(registration Registration, segment, collection string) error {
	if registration.Kind.Valid() {
		return fmt.Errorf("%w: %s", ErrKindDeclared, registration.Kind)
	}

	if registration.Kind == "" || segment == "" || collection == "" {
		return fmt.Errorf("%w: %q wires no %s: a synthetic kind supplies its own enum, segment and collection",
			ErrKindUndeclared, registration.Kind, ConsumerRoutes)
	}

	return r.add(registration, segment, collection, true)
}

func (r *Registry) add(registration Registration, segment, collection string, synthetic bool) error {
	if err := r.checkFree(registration.Kind, segment, collection); err != nil {
		return err
	}

	if err := validate(registration, synthetic); err != nil {
		return err
	}

	if r.indexer != nil {
		registration.Service = &indexingService{
			Service:      registration.Service,
			indexer:      r.indexer,
			kind:         registration.Kind,
			searchFields: registration.SearchFields,
		}
	}

	if r.tagChecker != nil {
		registration.Service = &tagCheckingService{
			Service: registration.Service,
			checker: r.tagChecker,
		}
	}

	entry := Entry{
		Registration: registration,
		Segment:      segment,
		Collection:   collection,
		Synthetic:    synthetic,
	}

	r.byKind[entry.Kind] = len(r.entries)
	r.bySegment[segment] = len(r.entries)
	r.byCollection[collection] = len(r.entries)
	r.entries = append(r.entries, entry)

	return nil
}

// checkFree runs before anything is written, so a refusal leaves the registry
// exactly as it was. A registry that reports a duplicate and keeps the second
// service has done the damage and told you it did not.
func (r *Registry) checkFree(k kind.Kind, segment, collection string) error {
	if _, taken := r.byKind[k]; taken {
		return fmt.Errorf("%w: %s", ErrAlreadyRegistered, k)
	}

	if index, taken := r.bySegment[segment]; taken {
		return fmt.Errorf("%w: %s and %s both want /%s", ErrSegmentTaken, k, r.entries[index].Kind, segment)
	}

	if index, taken := r.byCollection[collection]; taken {
		return fmt.Errorf("%w: %s and %s both want %s", ErrCollectionTaken, k, r.entries[index].Kind, collection)
	}

	return nil
}

// validate is a hand-written check per consumer rather than a loop over
// Consumers(), so that a consumer added to the published list without a check
// here is caught by T104 instead of passing silently.
func validate(registration Registration, synthetic bool) error {
	if registration.Service == nil {
		return fmt.Errorf("records: %s wires no service, so its routes would panic on their first request", registration.Kind)
	}

	incomplete := IncompleteError{Kind: registration.Kind}

	if registration.Schema.NewSummary == nil || registration.Schema.NewDetail == nil ||
		registration.Schema.NewCreate == nil || registration.Schema.NewPatch == nil ||
		len(registration.Schema.Sorts) == 0 {
		incomplete.Missing = append(incomplete.Missing, ConsumerSchema)
	}

	if registration.Views == nil {
		incomplete.Missing = append(incomplete.Missing, ConsumerViews)
	}

	if registration.Stream == nil {
		incomplete.Missing = append(incomplete.Missing, ConsumerStream)
	}

	if registration.Target == "" || !registration.Target.Valid() {
		incomplete.Missing = append(incomplete.Missing, ConsumerAudit)
	}

	if registration.Authorizer == nil {
		incomplete.Missing = append(incomplete.Missing, ConsumerAuthorizer)
	}

	if registration.Inventory.Title == "" || registration.Inventory.Summary == "" {
		incomplete.Missing = append(incomplete.Missing, ConsumerInventory)
	}

	// These three are not among the seven published Consumers(): they are not
	// consumed through the router, and a consumer added to that list with no
	// wiring assertion beside it is what T104 catches. They are required all
	// the same — a kind with no SearchFields is invisible to US8's search and
	// a kind with no Basis silently drops US9's "why does this row match" —
	// and a registration missing one is refused here, at boot, the same way.
	if registration.SearchFields == nil {
		incomplete.Missing = append(incomplete.Missing, "search fields")
	}

	if registration.Basis == nil {
		incomplete.Missing = append(incomplete.Missing, "basis")
	}

	if registration.SeedFixtureID == "" {
		incomplete.Missing = append(incomplete.Missing, "seed fixture id")
	}

	if len(incomplete.Missing) > 0 {
		return &incomplete
	}

	// A declared kind's audit target is derivable, and is declared anyway so
	// that the two complete-by-declaration vocabularies are cross-checked
	// somewhere. A synthetic kind has no row in either, so all that can be
	// asked of it is that its target is one audit publishes.
	if !synthetic && registration.Target != audit.TargetKind(registration.Kind.Enum()) {
		return fmt.Errorf("%w: %s wires %s %q and its own is %q",
			ErrAuditTargetMismatch, registration.Kind, ConsumerAudit,
			registration.Target, registration.Kind.Enum())
	}

	return nil
}

// Entries returns the inventory in registration order, as a copy: the router,
// the OpenAPI generator, the operator surface and the stream all read it and
// none of them may edit it for the others.
func (r *Registry) Entries() []Entry { return slices.Clone(r.entries) }

// Kinds returns the registered kinds in registration order. That order is the
// order the {kind} enum, the discriminator mapping and `medikube routes` all
// print, so it is a slice and never a map range: a map would make the generated
// document differ between two runs and the committed-diff gate fail at random.
func (r *Registry) Kinds() []kind.Kind {
	registered := make([]kind.Kind, 0, len(r.entries))
	for _, entry := range r.entries {
		registered = append(registered, entry.Kind)
	}

	return registered
}

// Segments is the {kind} path enum, in registration order.
func (r *Registry) Segments() []string {
	segments := make([]string, 0, len(r.entries))
	for _, entry := range r.entries {
		segments = append(segments, entry.Segment)
	}

	return segments
}

// SyntheticKinds is what internal/di asserts is empty in a production build.
func (r *Registry) SyntheticKinds() []kind.Kind {
	var synthetic []kind.Kind

	for _, entry := range r.entries {
		if entry.Synthetic {
			synthetic = append(synthetic, entry.Kind)
		}
	}

	return synthetic
}

// InventoryRows is the operator surface's listing, in registration order.
func (r *Registry) InventoryRows() []InventoryRow {
	rows := make([]InventoryRow, 0, len(r.entries))
	for _, entry := range r.entries {
		rows = append(rows, InventoryRow{
			Kind:    entry.Kind,
			Segment: entry.Segment,
			Title:   entry.Inventory.Title,
			Summary: entry.Inventory.Summary,
		})
	}

	return rows
}

func (r *Registry) FromKind(k kind.Kind) (Entry, bool) {
	index, registered := r.byKind[k]
	if !registered {
		return Entry{}, false
	}

	return r.entries[index], true
}

// FromSegment is exact. Accepting a second spelling — a different case, a
// trailing slash — would create a second URL for one kind: two cache keys, two
// audit spellings and a canonical form that is nobody's.
func (r *Registry) FromSegment(segment string) (Entry, bool) {
	index, registered := r.bySegment[segment]
	if !registered {
		return Entry{}, false
	}

	return r.entries[index], true
}

func (r *Registry) FromCollection(collection string) (Entry, bool) {
	index, registered := r.byCollection[collection]
	if !registered {
		return Entry{}, false
	}

	return r.entries[index], true
}

// CountByKind dispatches one count over every registered kind's collection and
// assembles the result keyed by kind.
//
// This is the extension point patient.Service.Summary consumes (research
// D-22): count is the one indexed `COUNT(*) WHERE patient = ?` a store adapter
// supplies, and nothing here switches on kind to decide which collection to
// ask. Registering a thirteenth kind changes zero lines in this function.
func (r *Registry) CountByKind(
	ctx context.Context,
	count func(ctx context.Context, collection string) (int, error),
) (map[kind.Kind]int, error) {
	result := make(map[kind.Kind]int, len(r.entries))

	for _, entry := range r.entries {
		n, err := count(ctx, entry.Collection)
		if err != nil {
			return nil, fmt.Errorf("records: counting %s: %w", entry.Collection, err)
		}

		result[entry.Kind] = n
	}

	return result, nil
}

// indexingService decorates a kind's Service with the search index's write
// side (research D-11): every create and update re-derives the row's title
// and body from what the service just returned and upserts it, and every
// delete removes it. This is the whole of "wire it into registration" —
// nothing above this file knows the index exists.
//
// A failed index write is reported and not swallowed, the same trade-off
// internal/platform/pb/hooks_records.go makes for the audit trail: the
// underlying write already committed, so there is nothing to undo, and what
// an unindexed write must not be is silent.
type indexingService struct {
	Service

	indexer      *search.Indexer
	kind         kind.Kind
	searchFields func(any) (title, body string)
}

func (s *indexingService) Create(ctx context.Context, actor access.Actor, body any) (Record, error) {
	record, err := s.Service.Create(ctx, actor, body)
	if err != nil {
		return record, err
	}

	if err := s.index(ctx, record); err != nil {
		return record, err
	}

	return record, nil
}

func (s *indexingService) Update(ctx context.Context, actor access.Actor, id, version string, body any) (Record, error) {
	record, err := s.Service.Update(ctx, actor, id, version, body)
	if err != nil {
		return record, err
	}

	if err := s.index(ctx, record); err != nil {
		return record, err
	}

	return record, nil
}

func (s *indexingService) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	if err := s.Service.Delete(ctx, actor, id, version); err != nil {
		return err
	}

	if err := s.indexer.Delete(ctx, s.kind, id); err != nil {
		return fmt.Errorf("records: removing the %s search index row: %w", s.kind, err)
	}

	return nil
}

func (s *indexingService) index(ctx context.Context, record Record) error {
	title, body := s.searchFields(record.Body)

	if err := s.indexer.Create(ctx, search.Row{
		PatientID: record.PatientID,
		Kind:      s.kind,
		RecordID:  record.ID,
		Title:     title,
		Body:      body,
	}); err != nil {
		return fmt.Errorf("records: indexing the %s search row: %w", s.kind, err)
	}

	return nil
}
