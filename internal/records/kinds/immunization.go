// Package kinds registers the clinical kinds whose registration does not
// live beside their service package. It follows internal/service/medication/
// adapter.go's shape exactly; see that file's doc comments for the reasoning
// behind the Codec/Adapter/Wiring split.
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
	"medikube/internal/service/immunization"
)

// Codec is immunization's DTO boundary, declared here for the same reason
// medication.Codec is declared in the service package: this is the only
// place that needs it, and internal/web/api must not appear in the service's
// dependency graph.
type Codec interface {
	Summary(entity clinical.Immunization) any
	Detail(entity clinical.Immunization) any
	Draft(body any) (clinical.Immunization, error)
	Patch(body any) (immunization.Patch, error)
}

// Adapter is records.Service for immunizations.
type Adapter struct {
	service *immunization.Service
	codec   Codec
}

var _ records.Service = (*Adapter)(nil)

func NewAdapter(service *immunization.Service, codec Codec) (*Adapter, error) {
	var absent []string

	if service == nil {
		absent = append(absent, "service")
	}

	if codec == nil {
		absent = append(absent, "codec")
	}

	if len(absent) > 0 {
		return nil, fmt.Errorf("immunization: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, immunization.Query{
		PatientID: query.PatientID,
		Search:    query.Search,
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

func (a *Adapter) detail(entity clinical.Immunization) records.Record {
	return a.record(entity, a.codec.Detail(entity))
}

func (a *Adapter) record(entity clinical.Immunization, body any) records.Record {
	return records.Record{
		ID:        entity.ID,
		Kind:      kind.Immunization,
		PatientID: entity.PatientID,
		Version:   entity.Version,
		Body:      body,
	}
}

// SeedFixtureID is the fixture `medikube seed` builds for this kind.
const SeedFixtureID = "immunization-01"

// StreamFilter is which of this kind's changes may reach a live view before
// the per-subscriber authorization runs. Immunization has no draft state — as
// medication.StreamFilter's doc comment explains, every change is a
// candidate and the only thing refused here is an event naming nothing.
type StreamFilter struct{}

func (StreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// Wiring is everything an immunization registration needs that this package
// does not own, mirroring medication.Wiring.
type Wiring struct {
	Repository immunization.Repository
	Authorizer immunization.Authorizer
	Codec      Codec

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

// Register wires immunizations into the record registry.
func Register(registry *records.Registry, wiring Wiring) error {
	if registry == nil {
		return fmt.Errorf("immunization: there is no registry to register %s into", kind.Immunization)
	}

	service, err := immunization.New(wiring.Repository, wiring.Authorizer)
	if err != nil {
		return err
	}

	adapter, err := NewAdapter(service, wiring.Codec)
	if err != nil {
		return err
	}

	schema := wiring.Schema
	schema.Sorts = immunization.Sorts()
	schema.Filters = map[string]records.FilterSpec{}

	return registry.Register(records.Registration{
		Kind:       kind.Immunization,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindImmunization,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Immunizations",
			Summary: "Every vaccination the person recorded: what it was, when it was given, and by whom.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

func joinWords(words []string) string {
	switch len(words) {
	case 1:
		return words[0]
	case 2:
		return words[0] + " and no " + words[1]
	default:
		return words[0] + ", no " + joinWords(words[1:])
	}
}
