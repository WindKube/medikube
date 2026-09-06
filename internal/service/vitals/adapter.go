package vitals

import (
	"context"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
)

// Codec is the kind's DTO boundary. Presentation-edge unit conversion
// (research D-15) happens here and only here: this type is implemented by
// internal/web/api, never by this package. Every method takes the actor's own
// unit_system, resolved by the adapter, so the conversion is a property of who
// is looking rather than of what was stored.
type Codec interface {
	Summary(vitals clinical.Vitals, system identity.UnitSystem) any
	Detail(vitals clinical.Vitals, system identity.UnitSystem) any
	Draft(body any, system identity.UnitSystem) (clinical.Vitals, error)
	Patch(body any, system identity.UnitSystem) (Patch, error)
}

// UnitSystemOf resolves the actor's own display preference (research D-15).
// It mirrors internal/web/api.UnitSystemOf's shape rather than importing it:
// internal/service must not depend on internal/web.
type UnitSystemOf func(ctx context.Context, actor access.Actor) (identity.UnitSystem, error)

// Adapter is records.Service for measurement sets.
type Adapter struct {
	service *Service
	codec   Codec
	unitOf  UnitSystemOf
}

var _ records.Service = (*Adapter)(nil)

func NewAdapter(service *Service, codec Codec, unitOf UnitSystemOf) (*Adapter, error) {
	var absent []string

	if service == nil {
		absent = append(absent, "service")
	}

	if codec == nil {
		absent = append(absent, "codec")
	}

	if unitOf == nil {
		absent = append(absent, "unit system resolver")
	}

	if len(absent) > 0 {
		return nil, fmt.Errorf("measurements: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec, unitOf: unitOf}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, Query{
		PatientID: query.PatientID,
		Search:    query.Search,
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

	system, err := a.unitOf(ctx, actor)
	if err != nil {
		return domain.Page[records.Record]{}, err
	}

	items := make([]records.Record, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, a.record(item, a.codec.Summary(item, system)))
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

	return a.detail(ctx, actor, found)
}

func (a *Adapter) Create(ctx context.Context, actor access.Actor, body any) (records.Record, error) {
	system, err := a.unitOf(ctx, actor)
	if err != nil {
		return records.Record{}, err
	}

	draft, err := a.codec.Draft(body, system)
	if err != nil {
		return records.Record{}, err
	}

	created, err := a.service.Create(ctx, actor, draft)
	if err != nil {
		return records.Record{}, err
	}

	return a.record(created, a.codec.Detail(created, system)), nil
}

func (a *Adapter) Update(ctx context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
	system, err := a.unitOf(ctx, actor)
	if err != nil {
		return records.Record{}, err
	}

	patch, err := a.codec.Patch(body, system)
	if err != nil {
		return records.Record{}, err
	}

	updated, err := a.service.Update(ctx, actor, id, version, patch)
	if err != nil {
		return records.Record{}, err
	}

	return a.record(updated, a.codec.Detail(updated, system)), nil
}

func (a *Adapter) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	return a.service.Delete(ctx, actor, id, version)
}

func (a *Adapter) detail(ctx context.Context, actor access.Actor, v clinical.Vitals) (records.Record, error) {
	system, err := a.unitOf(ctx, actor)
	if err != nil {
		return records.Record{}, err
	}

	return a.record(v, a.codec.Detail(v, system)), nil
}

func (a *Adapter) record(v clinical.Vitals, body any) records.Record {
	return records.Record{ID: v.ID, Kind: kind.Vitals, PatientID: v.PatientID, Version: v.Version, Body: body}
}

// SeedFixtureID is the fixture `medikube seed` builds for this kind.
const SeedFixtureID = "measurements-01"

// matchOf reads the resolved `?match=` filter value, defaulting to "any" the
// same way records.TagFilters' FilterSpec.Default does.
func matchOf(values []string) string {
	if len(values) == 0 {
		return records.MatchAny
	}

	return values[0]
}

// StreamFilter admits every change that names a record and a patient.
type StreamFilter struct{}

func (StreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// Wiring is everything a vitals registration needs that this package does
// not own.
type Wiring struct {
	Repository   Repository
	Authorizer   Authorizer
	Codec        Codec
	UnitSystemOf UnitSystemOf

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

// Register wires vitals into the record registry.
func Register(registry *records.Registry, wiring Wiring) error {
	if registry == nil {
		return fmt.Errorf("measurements: there is no registry to register %s into", kind.Vitals)
	}

	service, err := New(wiring.Repository, wiring.Authorizer)
	if err != nil {
		return err
	}

	adapter, err := NewAdapter(service, wiring.Codec, wiring.UnitSystemOf)
	if err != nil {
		return err
	}

	schema := wiring.Schema
	schema.Sorts = Sorts()
	schema.Filters = records.TagFilters()

	return registry.Register(records.Registration{
		Kind:       kind.Vitals,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindVitals,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Measurements",
			Summary: "Every set of measurements the person recorded at home: blood pressure, weight, glucose and the rest.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}
