package symptom

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

// Codec is the kind's DTO boundary, declared here so this package never names
// a wire type (Principle II).
type Codec interface {
	Summary(symptom clinical.Symptom) any
	Detail(symptom clinical.Symptom) any
	Draft(body any) (clinical.Symptom, error)
	Patch(body any) (Patch, error)
}

// Adapter is records.Service for symptom episodes.
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
		return nil, fmt.Errorf("symptom: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, Query{
		PatientID:  query.PatientID,
		Search:     query.Search,
		Name:       first(query.Filters[FilterName]),
		Severities: severities(query.Filters[FilterSeverity]),
		Statuses:   statuses(query.Filters[FilterStatus]),
		IsChronic:  isChronic(query.Filters[FilterIsChronic]),
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

func (a *Adapter) detail(s clinical.Symptom) records.Record {
	return a.record(s, a.codec.Detail(s))
}

func (a *Adapter) record(s clinical.Symptom, body any) records.Record {
	return records.Record{ID: s.ID, Kind: kind.Symptom, PatientID: s.PatientID, Version: s.Version, Body: body}
}

// SeedFixtureID is the fixture `medikube seed` builds for this kind.
const SeedFixtureID = "symptom-01"

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

func severities(values []string) []clinical.Severity {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.Severity, 0, len(values))
	for _, v := range values {
		converted = append(converted, clinical.Severity(v))
	}

	return converted
}

func statuses(values []string) []clinical.ConditionStatus {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.ConditionStatus, 0, len(values))
	for _, v := range values {
		converted = append(converted, clinical.ConditionStatus(v))
	}

	return converted
}

func isChronic(values []string) *bool {
	if len(values) == 0 {
		return nil
	}

	v := values[0] == "true"

	return &v
}

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

// Wiring is everything a symptom registration needs that this package does
// not own.
type Wiring struct {
	Repository Repository
	Authorizer Authorizer
	Codec      Codec

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

// Register wires symptoms into the record registry.
func Register(registry *records.Registry, wiring Wiring) error {
	if registry == nil {
		return fmt.Errorf("symptom: there is no registry to register %s into", kind.Symptom)
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
		FilterName:      {Name: FilterName, Kind: records.FilterFreeform},
		FilterSeverity:  {Name: FilterSeverity, Kind: records.FilterEnum, Allowed: severityStrings()},
		FilterStatus:    {Name: FilterStatus, Kind: records.FilterEnum, Allowed: statusStrings()},
		FilterIsChronic: {Name: FilterIsChronic, Kind: records.FilterEnum, Allowed: []string{"true", "false"}},
	}

	for name, spec := range records.TagFilters() {
		schema.Filters[name] = spec
	}

	return registry.Register(records.Registration{
		Kind:       kind.Symptom,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindSymptom,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Symptoms",
			Summary: "Every episode the person recorded: what it was, how severe, when, and how often it happens.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

func severityStrings() []string {
	values := clinical.Severities()
	converted := make([]string, 0, len(values))

	for _, v := range values {
		converted = append(converted, string(v))
	}

	return converted
}

func statusStrings() []string {
	values := clinical.ConditionStatuses()
	converted := make([]string, 0, len(values))

	for _, v := range values {
		converted = append(converted, string(v))
	}

	return converted
}
