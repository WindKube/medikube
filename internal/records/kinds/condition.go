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
	"medikube/internal/service/condition"
)

// ConditionSeedFixtureID is the fixture `medikube seed` builds for this kind.
const ConditionSeedFixtureID = "condition-01"

// ConditionCodec is the kind's DTO boundary, declared here rather than in
// internal/service/condition (Principle II).
type ConditionCodec interface {
	Summary(c clinical.Condition) any
	Detail(c clinical.Condition) any
	Draft(body any) (clinical.Condition, error)
	Patch(body any) (condition.Patch, error)
}

type conditionAdapter struct {
	service *condition.Service
	codec   ConditionCodec
}

var _ records.Service = (*conditionAdapter)(nil)

func newConditionAdapter(service *condition.Service, codec ConditionCodec) (*conditionAdapter, error) {
	var absent []string

	if service == nil {
		absent = append(absent, "service")
	}

	if codec == nil {
		absent = append(absent, "codec")
	}

	if len(absent) > 0 {
		return nil, fmt.Errorf("condition: the adapter is wired with no %s", joinWords(absent))
	}

	return &conditionAdapter{service: service, codec: codec}, nil
}

func (a *conditionAdapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, condition.Query{
		PatientID:  query.PatientID,
		Search:     query.Search,
		Statuses:   conditionStatuses(query.Filters[condition.FilterStatus]),
		Severities: severities(query.Filters[condition.FilterSeverity]),
		Active:     boolFilter(query.Filters[condition.FilterActive]),
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

func (a *conditionAdapter) Get(ctx context.Context, actor access.Actor, id string) (records.Record, error) {
	found, err := a.service.Get(ctx, actor, id)
	if err != nil {
		return records.Record{}, err
	}

	return a.detail(found), nil
}

func (a *conditionAdapter) Create(ctx context.Context, actor access.Actor, body any) (records.Record, error) {
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

func (a *conditionAdapter) Update(ctx context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
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

func (a *conditionAdapter) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	return a.service.Delete(ctx, actor, id, version)
}

func (a *conditionAdapter) detail(entity clinical.Condition) records.Record {
	return a.record(entity, a.codec.Detail(entity))
}

func (a *conditionAdapter) record(entity clinical.Condition, body any) records.Record {
	return records.Record{
		ID:        entity.ID,
		Kind:      kind.Condition,
		PatientID: entity.PatientID,
		Version:   entity.Version,
		Body:      body,
	}
}

type conditionStreamFilter struct{}

func (conditionStreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// ConditionWiring is everything a condition registration needs that this
// package does not own.
type ConditionWiring struct {
	Repository condition.Repository
	Authorizer condition.Authorizer
	Codec      ConditionCodec

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

// RegisterCondition wires conditions into the record registry.
func RegisterCondition(registry *records.Registry, wiring ConditionWiring) error {
	if registry == nil {
		return fmt.Errorf("condition: there is no registry to register %s into", kind.Condition)
	}

	service, err := condition.New(wiring.Repository, wiring.Authorizer)
	if err != nil {
		return err
	}

	adapter, err := newConditionAdapter(service, wiring.Codec)
	if err != nil {
		return err
	}

	schema := wiring.Schema
	schema.Sorts = condition.Sorts()
	schema.Filters = map[string]records.FilterSpec{
		condition.FilterStatus: {
			Name:    condition.FilterStatus,
			Kind:    records.FilterEnum,
			Allowed: conditionStatusStrings(),
		},
		condition.FilterSeverity: {
			Name:    condition.FilterSeverity,
			Kind:    records.FilterEnum,
			Allowed: severityStrings(),
		},
		condition.FilterActive: {
			Name:    condition.FilterActive,
			Kind:    records.FilterEnum,
			Allowed: boolStrings(),
		},
	}

	for name, spec := range records.TagFilters() {
		schema.Filters[name] = spec
	}

	return registry.Register(records.Registration{
		Kind:       kind.Condition,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     conditionStreamFilter{},
		Target:     audit.TargetKindCondition,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Conditions",
			Summary: "Every diagnosis the person recorded: what it is, how severe, and whether it has resolved.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: ConditionSeedFixtureID,
	})
}
