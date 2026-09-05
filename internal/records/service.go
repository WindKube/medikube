package records

import (
	"context"
	"io"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
)

// Renderer is templ.Component's method set, declared here rather than imported.
//
// internal/service/** implements Service, so an import of
// github.com/a-h/templ in this package would put the rendering library in the
// dependency graph of every service — the thing plan.md's dependency-inversion
// bullet rules out by name. A templ.Component satisfies this with no adapter
// and no conversion, because the method set is identical.
type Renderer interface {
	Render(ctx context.Context, w io.Writer) error
}

// Record is one record as the kind-agnostic layer sees it.
//
// Body is the kind's own typed DTO and is never inspected here: the generic
// handler minted it from the kind's Schema, the kind's service filled it in and
// the edge marshals it. Nothing in between needs to know which type it is,
// which is the whole reason a new kind adds no route.
type Record struct {
	ID   string
	Kind kind.Kind

	// PatientID is the record's own anchor, set by the kind's adapter from the
	// entity it just read or wrote. It is what lets the registry index a
	// record into search_index (patient-scoped, FR-087) without a kind-
	// specific accessor: every registered kind is patient-scoped, so this is
	// never empty for anything a kind's Service returns.
	PatientID string

	// Version is the ETag source — store.Version(record) — carried on the
	// record rather than fetched separately, because If-Match is required on
	// every write and a version read by a second call is a version that can
	// already be stale.
	Version string

	Body any
}

// Criteria is a list's resolved narrowing: what the caller asked for, after
// the registry checked it against the kind's own vocabulary. It is what a
// kind's Basis function reads to say why one row qualifies for a reason
// materially different from another's (research D-05), and US9 echoes it in
// the response envelope so a chip can be removed for what it names.
type Criteria struct {
	Filters map[string][]string
	Search  string
}

// Query is one list request, already parsed at the edge and not yet resolved to
// a kind. It names its parameters: PocketBase's filter DSL never reaches the
// wire, so there is no field here a caller could put an expression in.
type Query struct {
	// Empty means every registered kind. It is only read by the cross-kind
	// list; ListOfKind takes its kind from the path.
	Kinds []kind.Kind

	// PatientID is phase 002's addition (contracts/medications-rescope.md):
	// every list over patient-scoped data requires one, so this package
	// carries it rather than each kind inventing its own query member for the
	// same required parameter.
	PatientID string

	// The case-insensitive substring search (`?q=`).
	Search string

	// Filters are the kind's own named parameters — `status` for medications —
	// checked against the kind's declared vocabulary before the service sees
	// them.
	Filters map[string][]string

	// Sort is the resolved ordering, checked against the kind's allowlist. It
	// is also what the cursor codec binds into its associated data, so the
	// terms have to be the resolved ones and not the raw `?sort=`.
	Sort []domain.SortKey

	Limit  int
	Cursor string
	Count  bool
}

// Service is the whole record surface, kind-agnostic. Five methods, which is
// the interface-segregation cap plan.md sets, and the only thing the generic
// handler needs from a kind.
//
// The two write methods take `any` because the value is one the kind itself
// minted through Schema.NewCreate / Schema.NewPatch: the generic handler
// decoded the request body into it and hands it straight back, so the kind
// receives a pointer to its own type and asserts it without a conversion the
// middle had to know about.
type Service interface {
	List(ctx context.Context, actor access.Actor, query Query) (domain.Page[Record], error)
	Get(ctx context.Context, actor access.Actor, id string) (Record, error)
	Create(ctx context.Context, actor access.Actor, body any) (Record, error)
	Update(ctx context.Context, actor access.Actor, id, version string, body any) (Record, error)
	Delete(ctx context.Context, actor access.Actor, id, version string) error
}

// Views is a kind's rendering, registered with it so that a kind cannot reach a
// page without one. The four are the components contracts/pages.md and
// contracts/streams.md name: the list region, the row the SSE stream patches by
// id, the detail article and the form that is re-rendered from the submitted
// values plus the field errors (FR-027).
type Views interface {
	List(page domain.Page[Record]) Renderer
	Row(record Record) Renderer
	Detail(record Record) Renderer
	// notice is a stale If-Match's explanation, rendered inside the form
	// alongside the record's current values (research D-24); every other
	// caller passes the empty string.
	Form(record Record, invalid *domain.ValidationError, notice string) Renderer
}

// StreamFilter decides which of a kind's changes a subscriber may be told
// about, before the per-subscriber authorization runs.
//
// It is not that authorization and must never be mistaken for it:
// contracts/streams.md requires access.Authorizer.Record to run inside the
// subscriber loop for every single event, and this runs once per event for
// every subscriber. It takes ids because ids are all the hub carries — the
// publisher fans out identifiers and never record bodies, which is what makes
// per-subscriber authorization possible at all.
type StreamFilter interface {
	Streams(recordID, patientID string) bool
}

// Authorizer is the checkpoint a kind's records are reached through. The
// signature is the one contracts/medications-rescope.md and
// contracts/streams.md name — the patient anchor, phase 002 onward — so the
// single implementation in internal/service/access satisfies it directly and
// every kind registers the same instance.
//
// It is a registered facet rather than something the handler looks up, because
// phase 005 anchors sharing on top of the same patient and a checkpoint that
// switched on kind.Kind to decide which anchor to use would be the open/closed
// violation this registry exists to prevent.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}

// Schema is the kind's typed boundary: the four DTO shapes and the query
// vocabulary. internal/openapi reflects the four into the kind's oneOf branch
// and publishes the vocabulary as the operation's parameters, and the generic
// handler decodes request bodies into the same two write types — so the
// documented schema and the decoded type cannot be different types.
//
// It is a struct and not an interface because it has six members and
// plan.md's interface-segregation clause caps an interface at five. Each New
// function returns a new, empty pointer to the kind's own type; returning a
// shared value would let one request's decode be visible in the next.
type Schema struct {
	NewSummary func() any
	NewDetail  func() any
	NewCreate  func() any
	NewPatch   func() any

	// Sorts is the kind's sort allowlist in the order OpenAPI publishes it, the
	// first entry being the default. A `sort` outside it is 422 invalid_value
	// and never silently ignored, because a silently ignored sort produces a
	// list that looks right and is not (contracts/records.md).
	Sorts []domain.SortKey

	// DefaultSort overrides what an absent `?sort=` resolves to, for the kind
	// whose ordering is one fixed compound and not several single-field
	// alternatives (FR-051): Sorts[0] alone is not that ordering, the whole of
	// Sorts is. Left nil, resolveQuery keeps defaulting to Sorts[0].
	DefaultSort []domain.SortKey

	// Filters are the kind's own named query parameters, keyed by name. It may
	// be empty: a kind whose list takes nothing but the shared parameters is a
	// legitimate registration, and requiring a value here would only make
	// somebody invent one. An unknown parameter, or a value outside a
	// FilterSpec's declared vocabulary, is 400 bad_request and never silently
	// ignored (contracts/records-clinical.md §1).
	Filters map[string]FilterSpec
}

// Inventory is what the operator surface prints for a kind: the human name of
// the thing and one line saying what it holds. A kind that is reachable and
// that `medikube routes` cannot name is a kind nobody reviewing the surface
// will notice.
type Inventory struct {
	Title   string
	Summary string
}
