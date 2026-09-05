package equipment

import (
	"context"
	"fmt"
	"strconv"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
)

// Codec is the kind's DTO boundary, declared here because this is the only
// place that needs it (medication.Codec's own doc comment explains why the
// service hands `any` in both directions).
type Codec interface {
	// Summary's basis is FR-049's per-row overdue/due_soon distinction
	// (ServiceDueBasis), nil when the list was not narrowed by
	// ?service_due_within_days=.
	Summary(entity clinical.Equipment, basis []string) any
	Detail(entity clinical.Equipment) any
	Draft(body any) (clinical.Equipment, error)
	Patch(body any) (Patch, error)
}

// Adapter is records.Service for equipment.
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
		return nil, fmt.Errorf("equipment: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	resolved := Query{
		PatientID:            query.PatientID,
		Search:               query.Search,
		Types:                types(query.Filters[FilterType]),
		Statuses:             statuses(query.Filters[FilterStatus]),
		ServiceDueWithinDays: withinDays(query.Filters[ParamServiceDueWithin]),
		Tags:                 query.Filters[records.FilterTags],
		Match:                matchOf(query.Filters[records.FilterMatch]),
		Sort:                 query.Sort,
		Limit:                query.Limit,
		Cursor:               query.Cursor,
		Count:                query.Count,
	}

	page, err := a.service.List(ctx, actor, resolved)
	if err != nil {
		return domain.Page[records.Record]{}, err
	}

	items := make([]records.Record, 0, len(page.Items))
	for _, item := range page.Items {
		var basis []string
		if resolved.ServiceDueWithinDays != nil {
			basis = ServiceDueBasis(item, *resolved.ServiceDueWithinDays)
		}

		items = append(items, a.record(item, a.codec.Summary(item, basis)))
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

func (a *Adapter) detail(entity clinical.Equipment) records.Record {
	return a.record(entity, a.codec.Detail(entity))
}

func (a *Adapter) record(entity clinical.Equipment, body any) records.Record {
	return records.Record{
		ID:        entity.ID,
		Kind:      kind.Equipment,
		PatientID: entity.PatientID,
		Version:   entity.Version,
		Body:      body,
	}
}

// SeedFixtureID is the fixture `medikube seed` builds for this kind.
const SeedFixtureID = "equipment-01"

func types(values []string) []clinical.EquipmentType {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.EquipmentType, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.EquipmentType(value))
	}

	return converted
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

func withinDays(values []string) *int {
	if len(values) == 0 {
		return nil
	}

	days := DefaultServiceDueWithinDays

	if parsed, err := strconv.Atoi(values[0]); err == nil {
		days = parsed
	}

	return &days
}

// matchOf reads the resolved `?match=` filter value, defaulting to "any" the
// same way records.TagFilters' FilterSpec.Default does.
func matchOf(values []string) string {
	if len(values) == 0 {
		return records.MatchAny
	}

	return values[0]
}

// StreamFilter is which of this kind's changes may reach a live view.
type StreamFilter struct{}

func (StreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// Wiring is everything an equipment registration needs that this package does
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

// Register wires equipment into the record registry.
func Register(registry *records.Registry, wiring Wiring) error {
	if registry == nil {
		return fmt.Errorf("equipment: there is no registry to register %s into", kind.Equipment)
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
		FilterType: {
			Name:    FilterType,
			Kind:    records.FilterEnum,
			Allowed: equipmentTypeStrings(),
		},
		FilterStatus: {
			Name:    FilterStatus,
			Kind:    records.FilterEnum,
			Allowed: therapyStatusStrings(),
		},
		ParamServiceDueWithin: {
			Name:    ParamServiceDueWithin,
			Kind:    records.FilterFreeform,
			Default: strconv.Itoa(DefaultServiceDueWithinDays),
		},
	}

	for name, spec := range records.TagFilters() {
		schema.Filters[name] = spec
	}

	return registry.Register(records.Registration{
		Kind:       kind.Equipment,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindEquipment,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Equipment",
			Summary: "Medical equipment the person depends on: what it is, when it was serviced, and when service is next due.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

func equipmentTypeStrings() []string {
	published := clinical.EquipmentTypes()
	values := make([]string, 0, len(published))

	for _, value := range published {
		values = append(values, string(value))
	}

	return values
}

func therapyStatusStrings() []string {
	statuses := clinical.TherapyStatuses()
	values := make([]string, 0, len(statuses))

	for _, status := range statuses {
		values = append(values, string(status))
	}

	return values
}
