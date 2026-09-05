package insurance

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

// Codec is the kind's DTO boundary. Detail additionally takes the write's
// displacement, so the response can carry `"displaced": {...}` (FR-045,
// research D-16) without records.Record growing a member every other kind
// carries as an always-empty field.
type Codec interface {
	Summary(entity clinical.Insurance) any
	Detail(entity clinical.Insurance, displaced *Displaced) any
	Draft(body any) (clinical.Insurance, error)
	Patch(body any) (Patch, error)
}

// Adapter is records.Service for insurance.
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
		return nil, fmt.Errorf("insurance: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, Query{
		PatientID:          query.PatientID,
		Search:             query.Search,
		Types:              types(query.Filters[FilterType]),
		Statuses:           statuses(query.Filters[FilterStatus]),
		IsPrimary:          boolFilter(query.Filters[FilterIsPrimary]),
		ExpiringWithinDays: withinDays(query.Filters[ParamExpiringWithin]),
		Sort:               query.Sort,
		Limit:              query.Limit,
		Cursor:             query.Cursor,
		Count:              query.Count,
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

	return a.record(found, a.codec.Detail(found, nil)), nil
}

func (a *Adapter) Create(ctx context.Context, actor access.Actor, body any) (records.Record, error) {
	draft, err := a.codec.Draft(body)
	if err != nil {
		return records.Record{}, err
	}

	result, err := a.service.Create(ctx, actor, draft)
	if err != nil {
		return records.Record{}, err
	}

	return a.record(result.Insurance, a.codec.Detail(result.Insurance, result.Displaced)), nil
}

func (a *Adapter) Update(ctx context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
	patch, err := a.codec.Patch(body)
	if err != nil {
		return records.Record{}, err
	}

	result, err := a.service.Update(ctx, actor, id, version, patch)
	if err != nil {
		return records.Record{}, err
	}

	return a.record(result.Insurance, a.codec.Detail(result.Insurance, result.Displaced)), nil
}

func (a *Adapter) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	return a.service.Delete(ctx, actor, id, version)
}

func (a *Adapter) record(entity clinical.Insurance, body any) records.Record {
	return records.Record{
		ID:        entity.ID,
		Kind:      kind.Insurance,
		PatientID: entity.PatientID,
		Version:   entity.Version,
		Body:      body,
	}
}

// SeedFixtureID is the fixture `medikube seed` builds for this kind.
const SeedFixtureID = "insurance-01"

func types(values []string) []clinical.InsuranceType {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.InsuranceType, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.InsuranceType(value))
	}

	return converted
}

func statuses(values []string) []clinical.InsuranceStatus {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.InsuranceStatus, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.InsuranceStatus(value))
	}

	return converted
}

func boolFilter(values []string) *bool {
	if len(values) == 0 {
		return nil
	}

	parsed, err := strconv.ParseBool(values[0])
	if err != nil {
		return nil
	}

	return &parsed
}

func withinDays(values []string) *int {
	if len(values) == 0 {
		return nil
	}

	days := DefaultExpiringWithinDays

	if parsed, err := strconv.Atoi(values[0]); err == nil {
		days = parsed
	}

	return &days
}

// StreamFilter is which of this kind's changes may reach a live view.
type StreamFilter struct{}

func (StreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// Wiring is everything an insurance registration needs that this package does
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

// Register wires insurance into the record registry.
func Register(registry *records.Registry, wiring Wiring) error {
	if registry == nil {
		return fmt.Errorf("insurance: there is no registry to register %s into", kind.Insurance)
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
			Allowed: insuranceTypeStrings(),
		},
		FilterStatus: {
			Name:    FilterStatus,
			Kind:    records.FilterEnum,
			Allowed: insuranceStatusStrings(),
		},
		FilterIsPrimary: {
			Name:    FilterIsPrimary,
			Kind:    records.FilterEnum,
			Allowed: []string{"true", "false"},
		},
		ParamExpiringWithin: {
			Name:    ParamExpiringWithin,
			Kind:    records.FilterFreeform,
			Default: strconv.Itoa(DefaultExpiringWithinDays),
		},
	}

	return registry.Register(records.Registration{
		Kind:       kind.Insurance,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindInsurance,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Insurance",
			Summary: "The insurance policies the person holds: the insurer, the cover, and which policy is primary.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

func insuranceTypeStrings() []string {
	published := clinical.InsuranceTypes()
	values := make([]string, 0, len(published))

	for _, value := range published {
		values = append(values, string(value))
	}

	return values
}

func insuranceStatusStrings() []string {
	published := clinical.InsuranceStatuses()
	values := make([]string, 0, len(published))

	for _, value := range published {
		values = append(values, string(value))
	}

	return values
}
