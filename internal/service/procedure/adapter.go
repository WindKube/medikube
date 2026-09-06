package procedure

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

type Codec interface {
	Summary(p clinical.Procedure) any
	Detail(p clinical.Procedure) any
	Draft(body any) (clinical.Procedure, error)
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
		return nil, fmt.Errorf("procedure: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, Query{
		PatientID: query.PatientID,
		Search:    query.Search,
		Statuses:  statuses(query.Filters[FilterStatus]),
		Scheduled: scheduledFlag(query.Filters[FilterScheduled]),
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

func (a *Adapter) detail(p clinical.Procedure) records.Record {
	return a.record(p, a.codec.Detail(p))
}

func (a *Adapter) record(p clinical.Procedure, body any) records.Record {
	return records.Record{ID: p.ID, Kind: kind.Procedure, PatientID: p.PatientID, Version: p.Version, Body: body}
}

const SeedFixtureID = "procedure-01"

func statuses(values []string) []clinical.OrderStatus {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.OrderStatus, 0, len(values))
	for _, v := range values {
		converted = append(converted, clinical.OrderStatus(v))
	}

	return converted
}

func scheduledFlag(values []string) *bool {
	if len(values) == 0 {
		return nil
	}

	flag := values[0] == "true"

	return &flag
}

// matchOf reads the resolved `?match=` filter value, defaulting to "any" the
// same way records.TagFilters' FilterSpec.Default does.
func matchOf(values []string) string {
	if len(values) == 0 {
		return records.MatchAny
	}

	return values[0]
}

// StreamFilter admits every event that names a record and a patient.
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
		return fmt.Errorf("procedure: there is no registry to register %s into", kind.Procedure)
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
		FilterStatus:    {Name: FilterStatus, Kind: records.FilterEnum, Allowed: orderStatusStrings()},
		FilterScheduled: {Name: FilterScheduled, Kind: records.FilterEnum, Allowed: []string{"true", "false"}},
	}

	for name, spec := range records.TagFilters() {
		schema.Filters[name] = spec
	}

	return registry.Register(records.Registration{
		Kind:       kind.Procedure,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindProcedure,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Procedures",
			Summary: "Every procedure the person recorded: what was done, when, how it turned out and what it was for.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

func orderStatusStrings() []string {
	values := clinical.OrderStatuses()
	out := make([]string, 0, len(values))

	for _, v := range values {
		out = append(out, string(v))
	}

	return out
}

// BasisFor is FR-026's row-level distinction: "scheduled" or "ordered", read
// off the wire DTO's own status member so it agrees with what Views renders.
func BasisFor(status string) []string {
	switch clinical.OrderStatus(status) {
	case clinical.OrderStatusScheduled:
		return []string{"scheduled"}
	case clinical.OrderStatusOrdered:
		return []string{"ordered"}
	default:
		return nil
	}
}
