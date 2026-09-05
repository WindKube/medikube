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
	"medikube/internal/service/injury"
)

// InjuryCodec is injury's DTO boundary, declared here for the same reason
// Codec is declared beside it for immunization: this is the only place that
// needs it, and internal/web/api must not appear in the service's dependency
// graph. Named with the kind's prefix because this package already declares
// Codec/Adapter/Wiring/Register for immunization, and Go allows only one of
// each per package.
type InjuryCodec interface {
	Summary(entity clinical.Injury) any
	Detail(entity clinical.Injury) any
	Draft(body any) (clinical.Injury, error)
	Patch(body any) (injury.Patch, error)
}

// InjuryAdapter is records.Service for injuries.
type InjuryAdapter struct {
	service *injury.Service
	codec   InjuryCodec
}

var _ records.Service = (*InjuryAdapter)(nil)

func NewInjuryAdapter(service *injury.Service, codec InjuryCodec) (*InjuryAdapter, error) {
	var absent []string

	if service == nil {
		absent = append(absent, "service")
	}

	if codec == nil {
		absent = append(absent, "codec")
	}

	if len(absent) > 0 {
		return nil, fmt.Errorf("injury: the adapter is wired with no %s", joinWords(absent))
	}

	return &InjuryAdapter{service: service, codec: codec}, nil
}

func (a *InjuryAdapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, injury.Query{
		PatientID:    query.PatientID,
		Search:       query.Search,
		Statuses:     injuryStatuses(query.Filters[injury.FilterStatus]),
		Severities:   injurySeverities(query.Filters[injury.FilterSeverity]),
		Types:        injuryTypes(query.Filters[injury.FilterType]),
		Lateralities: injuryLateralities(query.Filters[injury.FilterLaterality]),
		Unresolved:   isUnresolved(query.Filters[injury.FilterUnresolved]),
		Sort:         query.Sort,
		Limit:        query.Limit,
		Cursor:       query.Cursor,
		Count:        query.Count,
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

func (a *InjuryAdapter) Get(ctx context.Context, actor access.Actor, id string) (records.Record, error) {
	found, err := a.service.Get(ctx, actor, id)
	if err != nil {
		return records.Record{}, err
	}

	return a.detail(found), nil
}

func (a *InjuryAdapter) Create(ctx context.Context, actor access.Actor, body any) (records.Record, error) {
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

func (a *InjuryAdapter) Update(ctx context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
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

func (a *InjuryAdapter) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	return a.service.Delete(ctx, actor, id, version)
}

func (a *InjuryAdapter) detail(entity clinical.Injury) records.Record {
	return a.record(entity, a.codec.Detail(entity))
}

func (a *InjuryAdapter) record(entity clinical.Injury, body any) records.Record {
	return records.Record{
		ID:        entity.ID,
		Kind:      kind.Injury,
		PatientID: entity.PatientID,
		Version:   entity.Version,
		Body:      body,
	}
}

// InjurySeedFixtureID is the fixture `medikube seed` builds for this kind.
const InjurySeedFixtureID = "injury-01"

// InjuryStreamFilter is which of this kind's changes may reach a live view
// before the per-subscriber authorization runs. Injury has no draft state —
// as StreamFilter's doc comment explains for immunization, every change is a
// candidate and the only thing refused here is an event naming nothing.
type InjuryStreamFilter struct{}

func (InjuryStreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// InjuryWiring is everything an injury registration needs that this package
// does not own, mirroring Wiring.
type InjuryWiring struct {
	Repository injury.Repository
	Authorizer injury.Authorizer
	Codec      InjuryCodec

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

// RegisterInjury wires injuries into the record registry.
func RegisterInjury(registry *records.Registry, wiring InjuryWiring) error {
	if registry == nil {
		return fmt.Errorf("injury: there is no registry to register %s into", kind.Injury)
	}

	service, err := injury.New(wiring.Repository, wiring.Authorizer)
	if err != nil {
		return err
	}

	adapter, err := NewInjuryAdapter(service, wiring.Codec)
	if err != nil {
		return err
	}

	schema := wiring.Schema
	schema.Sorts = injury.Sorts()
	schema.Filters = map[string]records.FilterSpec{
		injury.FilterStatus: {
			Name:    injury.FilterStatus,
			Kind:    records.FilterEnum,
			Allowed: conditionStatusStrings(),
		},
		injury.FilterSeverity: {
			Name:    injury.FilterSeverity,
			Kind:    records.FilterEnum,
			Allowed: severityStrings(),
		},
		injury.FilterType: {
			Name:    injury.FilterType,
			Kind:    records.FilterEnum,
			Allowed: injuryTypeStrings(),
		},
		injury.FilterLaterality: {
			Name:    injury.FilterLaterality,
			Kind:    records.FilterEnum,
			Allowed: lateralityStrings(),
		},
		// FilterUnresolved has no boolean FilterKind to be: records.FilterSpec
		// only carries FilterEnum and FilterFreeform (internal/records/filters.go).
		// It is modelled as a closed vocabulary of one accepted spelling —
		// `?unresolved=true` — the same way a query parameter that is either
		// present-and-true or absent is published elsewhere in this codebase;
		// isUnresolved below is the only place that reads it.
		injury.FilterUnresolved: {
			Name:    injury.FilterUnresolved,
			Kind:    records.FilterEnum,
			Allowed: []string{"true"},
		},
	}

	return registry.Register(records.Registration{
		Kind:       kind.Injury,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     InjuryStreamFilter{},
		Target:     audit.TargetKindInjury,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Injuries",
			Summary: "Every injury the person recorded: what happened, how bad it was, and whether it has healed.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: InjurySeedFixtureID,
	})
}

// injuryStatuses, injurySeverities, injuryTypes and injuryLateralities convert
// the `?status=`, `?severity=`, `?type=` and `?laterality=` values without
// judging them. An unpublished value is refused by the service against its
// own vocabulary, so that the refusal is raised once, in the layer that
// publishes the list.
func injuryStatuses(values []string) []clinical.ConditionStatus {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.ConditionStatus, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.ConditionStatus(value))
	}

	return converted
}

func injurySeverities(values []string) []clinical.Severity {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.Severity, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.Severity(value))
	}

	return converted
}

func injuryTypes(values []string) []clinical.InjuryType {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.InjuryType, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.InjuryType(value))
	}

	return converted
}

func injuryLateralities(values []string) []clinical.Laterality {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.Laterality, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.Laterality(value))
	}

	return converted
}

// isUnresolved reads `?unresolved=` after checkFilters has already refused
// anything but "true": the service's own Query.Unresolved is a bare bool, so
// this is where a supplied value becomes one.
func isUnresolved(values []string) bool {
	for _, value := range values {
		if value == "true" {
			return true
		}
	}

	return false
}

func conditionStatusStrings() []string {
	statuses := clinical.ConditionStatuses()
	values := make([]string, 0, len(statuses))

	for _, status := range statuses {
		values = append(values, string(status))
	}

	return values
}

func severityStrings() []string {
	severities := clinical.Severities()
	values := make([]string, 0, len(severities))

	for _, severity := range severities {
		values = append(values, string(severity))
	}

	return values
}

func injuryTypeStrings() []string {
	types := clinical.InjuryTypes()
	values := make([]string, 0, len(types))

	for _, t := range types {
		values = append(values, string(t))
	}

	return values
}

func lateralityStrings() []string {
	lateralities := clinical.Lateralities()
	values := make([]string, 0, len(lateralities))

	for _, l := range lateralities {
		values = append(values, string(l))
	}

	return values
}
