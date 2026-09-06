package encounter

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

// Codec is the kind's DTO boundary. internal/web/api owns the real DTOs; this
// package must not know they exist (mirrors internal/service/medication).
type Codec interface {
	Summary(e clinical.Encounter) any
	Detail(e clinical.Encounter) any
	Draft(body any) (clinical.Encounter, error)
	Patch(body any) (Patch, error)
}

type Adapter struct {
	service *Service
	codec   Codec
}

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
		return nil, fmt.Errorf("encounter: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, Query{
		PatientID:  query.PatientID,
		Search:     query.Search,
		VisitTypes: visitTypes(query.Filters[FilterVisitType]),
		Priorities: priorities(query.Filters[FilterPriority]),
		Tags:       query.Filters[records.FilterTags],
		Match:      matchOf(query.Filters[records.FilterMatch]),
		Sort:       query.Sort,
		Limit:      query.Limit,
		Cursor:     query.Cursor,
		Count:      query.Count,
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

func (a *Adapter) detail(e clinical.Encounter) records.Record {
	return a.record(e, a.codec.Detail(e))
}

func (a *Adapter) record(e clinical.Encounter, body any) records.Record {
	return records.Record{ID: e.ID, Kind: kind.Encounter, PatientID: e.PatientID, Version: e.Version, Body: body}
}

const SeedFixtureID = "encounter-01"

func visitTypes(values []string) []clinical.VisitType {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.VisitType, 0, len(values))
	for _, v := range values {
		converted = append(converted, clinical.VisitType(v))
	}

	return converted
}

func priorities(values []string) []clinical.VisitPriority {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.VisitPriority, 0, len(values))
	for _, v := range values {
		converted = append(converted, clinical.VisitPriority(v))
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

// StreamFilter admits every event that names a record and a patient — an
// encounter has no draft state (mirrors medication.StreamFilter).
type StreamFilter struct{}

func (StreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

type Wiring struct {
	Repository Repository
	Authorizer Authorizer
	Codec      Codec

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

func Register(registry *records.Registry, wiring Wiring) error {
	if registry == nil {
		return fmt.Errorf("encounter: there is no registry to register %s into", kind.Encounter)
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
		FilterVisitType: {Name: FilterVisitType, Kind: records.FilterEnum, Allowed: visitTypeStrings()},
		FilterPriority:  {Name: FilterPriority, Kind: records.FilterEnum, Allowed: priorityStrings()},
	}

	for name, spec := range records.TagFilters() {
		schema.Filters[name] = spec
	}

	return registry.Register(records.Registration{
		Kind:       kind.Encounter,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindEncounter,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Encounters",
			Summary: "Every visit the person recorded: why they went, who they saw, where, what was concluded and what happens next.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

func visitTypeStrings() []string {
	values := clinical.VisitTypes()
	out := make([]string, 0, len(values))

	for _, v := range values {
		out = append(out, string(v))
	}

	return out
}

func priorityStrings() []string {
	values := clinical.VisitPriorities()
	out := make([]string, 0, len(values))

	for _, v := range values {
		out = append(out, string(v))
	}

	return out
}
