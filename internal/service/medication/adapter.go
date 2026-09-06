package medication

import (
	"context"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
)

// Codec is the kind's DTO boundary, declared here because this is the only
// place that needs it.
//
// internal/web/api owns the four medication DTOs; this package must not know
// they exist — a service that named a wire type would be the layering violation
// Principle II's dependency-inversion clause rules out, and depguard would only
// catch the templ half of it. So the adapter hands `any` in both directions and
// the edge supplies the one implementation that knows what those values are.
//
// The two decoding methods are handed a value the kind itself minted through
// records.Schema.NewCreate / NewPatch, so a type they did not mint is a wiring
// mistake in the registration and never a caller's mistake.
type Codec interface {
	Summary(medication clinical.Medication) any
	Detail(medication clinical.Medication) any
	Draft(body any) (clinical.Medication, error)
	Patch(body any) (Patch, error)
}

// Adapter is records.Service for medications: the whole of what the generic
// record handler needs from this kind.
//
// It decides nothing. Authorization, validation and ordering are the service's,
// the wire shapes are the codec's, and what is left is the translation between
// a kind-agnostic call and a typed one. That is the point — this file is what
// phases 002 through 006 copy for each of their kinds, so anything clever in
// here would be copied thirteen more times.
type Adapter struct {
	service *Service
	codec   Codec
}

// Compile-time proof that the adapter is what the registry will take. A
// mismatch here is a wiring failure at boot; this makes it a build failure.
var _ records.Service = (*Adapter)(nil)

func NewAdapter(service *Service, codec Codec) (*Adapter, error) {
	var absent []string

	if service == nil {
		absent = append(absent, "service")
	}

	if codec == nil {
		absent = append(absent, "codec")
	}

	if len(absent) > 0 {
		return nil, fmt.Errorf("medication: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, Query{
		PatientID: query.PatientID,
		Search:    query.Search,
		Statuses:  statusesOf(query.Filters),
		Tags:      query.Filters[records.FilterTags],
		Match:     matchOf(query.Filters[records.FilterMatch]),
		Sort:      query.Sort,
		Limit:     query.Limit,
		Cursor:    query.Cursor,
		Count:     query.Count,
	})
	if err != nil {
		return domain.Page[records.Record]{}, err
	}

	items := make([]records.Record, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, a.record(item, a.codec.Summary(item)))
	}

	converted := domain.NewPage(items, page.NextCursor)
	if page.Total != nil {
		converted = converted.WithTotal(*page.Total)
	}

	return converted, nil
}

func (a *Adapter) Get(ctx context.Context, actor access.Actor, id string) (records.Record, error) {
	found, err := a.service.Get(ctx, actor, id)
	if err != nil {
		return records.Record{}, err
	}

	return a.detail(found), nil
}

func (a *Adapter) Create(ctx context.Context, actor access.Actor, body any) (records.Record, error) {
	draft, err := a.codec.Draft(body)
	if err != nil {
		return records.Record{}, err
	}

	created, err := a.service.Create(ctx, actor, draft)
	if err != nil {
		return records.Record{}, err
	}

	return a.detail(created), nil
}

func (a *Adapter) Update(ctx context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
	patch, err := a.codec.Patch(body)
	if err != nil {
		return records.Record{}, err
	}

	updated, err := a.service.Update(ctx, actor, id, version, patch)
	if err != nil {
		return records.Record{}, err
	}

	return a.detail(updated), nil
}

func (a *Adapter) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	return a.service.Delete(ctx, actor, id, version)
}

func (a *Adapter) detail(medication clinical.Medication) records.Record {
	return a.record(medication, a.codec.Detail(medication))
}

func (a *Adapter) record(medication clinical.Medication, body any) records.Record {
	return records.Record{
		ID:        medication.ID,
		Kind:      kind.Medication,
		PatientID: medication.PatientID,
		Version:   medication.Version,
		Body:      body,
	}
}

// SeedFixtureID is the fixture `medikube seed` builds for this kind.
const SeedFixtureID = "medication-01"

// statuses converts the `?status=` values without judging them. An unpublished
// value is refused by the service against its own vocabulary, so that the
// refusal is raised once, in the layer that publishes the list.
func statusesOf(filters map[string][]string) []clinical.TherapyStatus {
	active := filters[FilterActive]
	if len(active) == 0 {
		return statuses(filters[FilterStatus])
	}

	if active[0] == "true" {
		return []clinical.TherapyStatus{clinical.TherapyStatusActive}
	}

	inactive := make([]clinical.TherapyStatus, 0, len(clinical.TherapyStatuses()))
	for _, status := range clinical.TherapyStatuses() {
		if status != clinical.TherapyStatusActive {
			inactive = append(inactive, status)
		}
	}

	return inactive
}

func statuses(values []string) []clinical.TherapyStatus {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.TherapyStatus, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.TherapyStatus(value))
	}

	return converted
}

// matchOf reads the resolved `?match=` filter value, defaulting to "any" the
// same way records.TagFilters' FilterSpec.Default does.
func matchOf(values []string) string {
	if len(values) == 0 {
		return records.MatchAny
	}

	return values[0]
}

// StreamFilter is which of this kind's changes may reach a live view before the
// per-subscriber authorization runs.
//
// Every medication change is a candidate — there is no draft state and nothing
// a person records for themselves that another of their own views should not
// see — so the only thing to refuse here is an event that names nothing. That
// is a publisher fault, and admitting it would have the stream handler
// re-fetching an empty id once per subscriber.
//
// It is NOT authorization. contracts/streams.md requires the authorizer to run
// inside the subscriber loop for every single event, and this runs once per
// event for every subscriber.
type StreamFilter struct{}

func (StreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// Wiring is everything a medication registration needs that this package does
// not own. The kind, the audit target, the query vocabulary, the stream filter
// and the operator's inventory row are this package's and are not asked for.
//
// It takes the three ports rather than an assembled Service on purpose: the
// registry needs the authorization checkpoint too, and a Wiring that carried
// both a built service and a checkpoint could carry two different ones — a
// service authorizing against one and a live stream against another, which is
// a hole that boots cleanly and passes every test that only exercises one path.
type Wiring struct {
	Repository Repository
	Authorizer Authorizer
	Codec      Codec

	// Schema carries the four DTO constructors and nothing else: Sorts and
	// Filters are this kind's published vocabulary and are filled in by
	// Register, so the ordering the service enforces and the ordering OpenAPI
	// documents cannot be two lists.
	Schema records.Schema

	Views records.Views

	// SearchFields and Basis read the wire DTO Views already renders — the
	// same value Record.Body carries — and are internal/web/api's, not this
	// package's: this package does not know the DTO type exists (research
	// D-11 and this file's own doc comment on Codec).
	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

// Register wires medications into the record registry.
//
// One call, seven consumers, no route. This is the extension point phases 002
// through 006 use for every kind they add, and the registry refuses a
// registration that leaves any consumer unwired — so the way to find out what a
// new kind owes is to write this function for it and read the refusal.
func Register(registry *records.Registry, wiring Wiring) error {
	if registry == nil {
		return fmt.Errorf("medication: there is no registry to register %s into", kind.Medication)
	}

	service, err := New(wiring.Repository, wiring.Authorizer)
	if err != nil {
		return err
	}

	adapter, err := NewAdapter(service, wiring.Codec)
	if err != nil {
		return err
	}

	schema := wiring.Schema
	schema.Sorts = Sorts()
	schema.Filters = map[string]records.FilterSpec{
		FilterStatus: {
			Name:    FilterStatus,
			Kind:    records.FilterEnum,
			Allowed: therapyStatusStrings(),
		},
		FilterActive: {
			Name:    FilterActive,
			Kind:    records.FilterEnum,
			Allowed: []string{"true", "false"},
		},
	}

	for name, spec := range records.TagFilters() {
		schema.Filters[name] = spec
	}

	return registry.Register(records.Registration{
		Kind:       kind.Medication,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindMedication,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Medications",
			Summary: "Every course of therapy the person recorded: what it is, how much, how often, and whether it is still being taken.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

// therapyStatusStrings is the vocabulary FilterStatus is checked against,
// converted once so the registry's generic filter check never imports
// clinical.
func therapyStatusStrings() []string {
	statuses := clinical.TherapyStatuses()
	values := make([]string, 0, len(statuses))

	for _, status := range statuses {
		values = append(values, string(status))
	}

	return values
}
