package openapi_test

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/openapi"
)

// The synthetic second kind, research D-08's "two-kind fixture". A oneOf with a
// single branch proves nothing about the mechanism phase 003 bets thirteen
// kinds on, so every gate in this package runs against the real kind plus this
// one. It is declared here rather than in internal/records/recordstest (T109)
// only because that package does not exist yet; when it does, these two
// constants and fakeKind become one call into it and nothing else changes.
const (
	fakeKindEnum    = "fake_kind"
	fakeKindSegment = "fake-kinds"
)

// The document version the fixtures generate under. It is an input rather than
// a constant of the package because api/openapi.json carries the build stamp
// health.md's `healthz` reports.
const fixtureVersion = "0.0.0-fixture"

// The DTO fixtures below mirror contracts/records.md's medication DTOs. They
// stand in for internal/web/api's real types, which land with T112-T113; this
// package reflects whatever it is handed, so the reconciliation is a change of
// argument and not a change of code.
//
// One deliberate divergence: contracts/records.md spells the two patch dates
// `**string`, and that mechanism does not work — an explicit null and an absent
// member both decode to a nil outer pointer under Go 1.27's encoding/json.
// The reflector refuses `**T` for that reason, so the fixture uses `*string`
// and the divergence is reported rather than hidden.

type medicationSummaryFixture struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Name      string  `json:"name"`
	Dosage    string  `json:"dosage,omitempty"`
	Frequency string  `json:"frequency,omitempty"`
	Status    string  `json:"status"`
	StartedOn *string `json:"started_on"`
	UpdatedAt string  `json:"updated_at"`
}

type medicationDetailFixture struct {
	medicationSummaryFixture

	AlternativeName string  `json:"alternative_name,omitempty"`
	Type            string  `json:"type,omitempty"`
	Route           string  `json:"route,omitempty"`
	Indication      string  `json:"indication,omitempty"`
	EndedOn         *string `json:"ended_on"`
	SideEffects     string  `json:"side_effects,omitempty"`
	Notes           string  `json:"notes,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

type medicationCreateFixture struct {
	Name            string  `json:"name"`
	AlternativeName string  `json:"alternative_name,omitempty"`
	Type            string  `json:"type,omitempty"`
	Dosage          string  `json:"dosage,omitempty"`
	Frequency       string  `json:"frequency,omitempty"`
	Route           string  `json:"route,omitempty"`
	Indication      string  `json:"indication,omitempty"`
	StartedOn       *string `json:"started_on,omitempty"`
	EndedOn         *string `json:"ended_on,omitempty"`
	Status          string  `json:"status,omitempty"`
	SideEffects     string  `json:"side_effects,omitempty"`
	Notes           string  `json:"notes,omitempty"`
}

type medicationPatchFixture struct {
	Name        *string `json:"name,omitempty"`
	Dosage      *string `json:"dosage,omitempty"`
	StartedOn   *string `json:"started_on,omitempty"`
	EndedOn     *string `json:"ended_on,omitempty"`
	Status      *string `json:"status,omitempty"`
	SideEffects *string `json:"side_effects,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

// The synthetic kind's DTOs are deliberately not shaped like the real one: a
// fixture that mirrors the thing it is meant to discriminate from proves
// nothing.
type fakeSummaryFixture struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	UpdatedAt string `json:"updated_at"`
}

type fakeDetailFixture struct {
	fakeSummaryFixture

	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
}

type fakeCreateFixture struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

type fakePatchFixture struct {
	Label  *string `json:"label,omitempty"`
	Detail *string `json:"detail,omitempty"`
}

// contracts/README.md, "The error envelope, on every non-2xx". This stands in
// for the struct internal/web/errors.go writes (T111); envelope_test.go asserts
// the schema it produces against the contract, so wiring the real type in and
// getting it wrong is a red test rather than a silent contract change.
type errorEnvelopeFixture struct {
	Error errorBodyFixture `json:"error"`
}

type errorBodyFixture struct {
	Code      string              `json:"code"`
	Message   string              `json:"message"`
	RequestID string              `json:"request_id"`
	Fields    []fieldErrorFixture `json:"fields,omitempty"`
}

type fieldErrorFixture struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// realKind is the one kind this build ships, read back through the kind table
// so neither spelling is written down twice (research D-05).
func realKind() openapi.Kind {
	return openapi.Kind{
		Enum:    kind.Medication.Enum(),
		Segment: kind.Medication.Segment(),
		Summary: medicationSummaryFixture{},
		Detail:  medicationDetailFixture{},
		Create:  medicationCreateFixture{},
		Patch:   medicationPatchFixture{},
	}
}

func fakeKind() openapi.Kind {
	return openapi.Kind{
		Enum:    fakeKindEnum,
		Segment: fakeKindSegment,
		Summary: fakeSummaryFixture{},
		Detail:  fakeDetailFixture{},
		Create:  fakeCreateFixture{},
		Patch:   fakePatchFixture{},
	}
}

// searchResponseFixture mirrors api.SearchResponse's shape (contracts/search.md
// §2), the same way errorEnvelopeFixture mirrors web.Envelope's.
type searchResponseFixture struct {
	Groups      []searchGroupFixture  `json:"groups"`
	Criteria    searchCriteriaFixture `json:"criteria"`
	EmptyReason *string               `json:"empty_reason"`
}

type searchGroupFixture struct {
	Kind       string              `json:"kind"`
	Items      []searchItemFixture `json:"items"`
	NextCursor *string             `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
}

type searchItemFixture struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Snippet    *string  `json:"snippet"`
	OccurredOn *string  `json:"occurred_on"`
	Tags       []string `json:"tags"`
}

type searchCriteriaFixture struct {
	QPresent bool     `json:"q_present"`
	Kinds    []string `json:"kinds"`
	Tags     []string `json:"tags"`
	Match    string   `json:"match"`
}

// twoKindInput is the fixture every gate in this package runs against.
func twoKindInput() openapi.Input {
	return openapi.Input{
		Version:        fixtureVersion,
		Routes:         httproute.Inventory().Routes(),
		Kinds:          []openapi.Kind{realKind(), fakeKind()},
		Envelope:       errorEnvelopeFixture{},
		SearchResponse: searchResponseFixture{},
	}
}

func generate(t *testing.T, in openapi.Input) *openapi3.T {
	t.Helper()

	doc, err := openapi.Generate(in)
	require.NoError(t, err)
	require.NotNil(t, doc)

	return doc
}

// roundTrip is the assertion FACT 9 and risk R1 turn on. A document built in
// memory holds SchemaRefs with a Ref and no Value, so an in-place Validate
// cannot follow them; and a ref whose target is missing from Components is
// caught by the LOADER, not by the reloaded document's Validate. Both errors
// are therefore asserted, and dropping either one produces a gate that catches
// nothing.
func roundTrip(t *testing.T, doc *openapi3.T) *openapi3.T {
	t.Helper()

	loaded, _, err := openapi.RoundTrip(context.Background(), doc)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	return loaded
}

// documentedRoutes is the expected side of the both-directions gate, computed
// from the registry rather than from the generator.
func documentedRoutes(t *testing.T) []httproute.Route {
	t.Helper()

	var documented []httproute.Route

	for _, route := range httproute.Inventory().Routes() {
		switch route.Kind {
		case httproute.KindAPI, httproute.KindStream, httproute.KindExternal:
			documented = append(documented, route)
		case httproute.KindPage, httproute.KindAsset:
		}
	}

	require.NotEmpty(t, documented)

	return documented
}
