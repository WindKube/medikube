// Package kinds is where US1-US6 register their kinds into the record
// registry: one file per kind, one Register call each, no hand-written
// routes (contracts/records.md).
package kinds

import (
	"context"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/allergy"
	"medikube/internal/service/link"
)

// AllergySeedFixtureID is the fixture `medikube seed` builds for this kind.
const AllergySeedFixtureID = "allergy-01"

// AllergyCodec is the kind's DTO boundary, declared here rather than in
// internal/service/allergy so that package need never know a wire type
// exists (Principle II).
type AllergyCodec interface {
	Summary(a clinical.Allergy) any
	Detail(a clinical.Allergy) any
	Draft(body any) (clinical.Allergy, error)
	Patch(body any) (allergy.Patch, error)
}

// allergyAdapter is records.Service for allergies.
type allergyAdapter struct {
	service *allergy.Service
	codec   AllergyCodec
}

var _ records.Service = (*allergyAdapter)(nil)

func newAllergyAdapter(service *allergy.Service, codec AllergyCodec) (*allergyAdapter, error) {
	var absent []string

	if service == nil {
		absent = append(absent, "service")
	}

	if codec == nil {
		absent = append(absent, "codec")
	}

	if len(absent) > 0 {
		return nil, fmt.Errorf("allergy: the adapter is wired with no %s", joinWords(absent))
	}

	return &allergyAdapter{service: service, codec: codec}, nil
}

func (a *allergyAdapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, allergy.Query{
		PatientID:  query.PatientID,
		Search:     query.Search,
		Statuses:   conditionStatuses(query.Filters[allergy.FilterStatus]),
		Severities: severities(query.Filters[allergy.FilterSeverity]),
		Critical:   boolFilter(query.Filters[allergy.FilterCritical]),
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

func (a *allergyAdapter) Get(ctx context.Context, actor access.Actor, id string) (records.Record, error) {
	found, err := a.service.Get(ctx, actor, id)
	if err != nil {
		return records.Record{}, err
	}

	return a.detail(found), nil
}

func (a *allergyAdapter) Create(ctx context.Context, actor access.Actor, body any) (records.Record, error) {
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

func (a *allergyAdapter) Update(ctx context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
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

func (a *allergyAdapter) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	return a.service.Delete(ctx, actor, id, version)
}

func (a *allergyAdapter) detail(entity clinical.Allergy) records.Record {
	return a.record(entity, a.codec.Detail(entity))
}

func (a *allergyAdapter) record(entity clinical.Allergy, body any) records.Record {
	return records.Record{
		ID:        entity.ID,
		Kind:      kind.Allergy,
		PatientID: entity.PatientID,
		Version:   entity.Version,
		Body:      body,
	}
}

// allergyStreamFilter admits every change that names a record and a patient;
// per-subscriber authorization runs separately (contracts/streams.md).
type allergyStreamFilter struct{}

func (allergyStreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// AllergyWiring is everything an allergy registration needs that this
// package does not own.
type AllergyWiring struct {
	Repository allergy.Repository
	Authorizer allergy.Authorizer
	Codec      AllergyCodec

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string

	// LinkResolver and LinkAuthorizer are FR-057's validation for the
	// `medications` field. Both nil is accepted — a registration built
	// without them writes the field unvalidated — but supplying only one is
	// refused: a half-wired checkpoint is worse than none.
	LinkResolver   link.Resolver
	LinkAuthorizer link.Authorizer
}

// RegisterAllergy wires allergies into the record registry.
func RegisterAllergy(registry *records.Registry, wiring AllergyWiring) error {
	if registry == nil {
		return fmt.Errorf("allergy: there is no registry to register %s into", kind.Allergy)
	}

	var options []allergy.Option
	if wiring.LinkResolver != nil && wiring.LinkAuthorizer != nil {
		options = append(options, allergy.WithLinks(wiring.LinkResolver, wiring.LinkAuthorizer))
	}

	service, err := allergy.New(wiring.Repository, wiring.Authorizer, options...)
	if err != nil {
		return err
	}

	adapter, err := newAllergyAdapter(service, wiring.Codec)
	if err != nil {
		return err
	}

	schema := wiring.Schema
	schema.Sorts = allergy.Sorts()
	schema.Filters = map[string]records.FilterSpec{
		allergy.FilterStatus: {
			Name:    allergy.FilterStatus,
			Kind:    records.FilterEnum,
			Allowed: conditionStatusStrings(),
		},
		allergy.FilterSeverity: {
			Name:    allergy.FilterSeverity,
			Kind:    records.FilterEnum,
			Allowed: severityStrings(),
		},
		allergy.FilterCritical: {
			Name:    allergy.FilterCritical,
			Kind:    records.FilterEnum,
			Allowed: boolStrings(),
		},
	}

	for name, spec := range records.TagFilters() {
		schema.Filters[name] = spec
	}

	return registry.Register(records.Registration{
		Kind:       kind.Allergy,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     allergyStreamFilter{},
		Target:     audit.TargetKindAllergy,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Allergies",
			Summary: "Every allergy the person recorded: what it is, how severe, and whether it is still active.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: AllergySeedFixtureID,
	})
}
