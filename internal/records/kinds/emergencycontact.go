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
	"medikube/internal/service/emergencycontact"
)

// EmergencyContactSeedFixtureID is the fixture `medikube seed` builds for
// this kind.
const EmergencyContactSeedFixtureID = "emergency-contact-01"

// EmergencyContactCodec is the kind's DTO boundary, declared here rather
// than in internal/service/emergencycontact (Principle II).
type EmergencyContactCodec interface {
	Summary(c clinical.EmergencyContact) any
	Detail(c clinical.EmergencyContact) any
	Draft(body any) (clinical.EmergencyContact, error)
	Patch(body any) (emergencycontact.Patch, error)
}

type emergencyContactAdapter struct {
	service *emergencycontact.Service
	codec   EmergencyContactCodec
}

var _ records.Service = (*emergencyContactAdapter)(nil)

func newEmergencyContactAdapter(service *emergencycontact.Service, codec EmergencyContactCodec) (*emergencyContactAdapter, error) {
	var absent []string

	if service == nil {
		absent = append(absent, "service")
	}

	if codec == nil {
		absent = append(absent, "codec")
	}

	if len(absent) > 0 {
		return nil, fmt.Errorf("emergencycontact: the adapter is wired with no %s", joinWords(absent))
	}

	return &emergencyContactAdapter{service: service, codec: codec}, nil
}

func (a *emergencyContactAdapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, emergencycontact.Query{
		PatientID: query.PatientID,
		Search:    query.Search,
		IsActive:  boolFilter(query.Filters[emergencycontact.FilterIsActive]),
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

func (a *emergencyContactAdapter) Get(ctx context.Context, actor access.Actor, id string) (records.Record, error) {
	found, err := a.service.Get(ctx, actor, id)
	if err != nil {
		return records.Record{}, err
	}

	return a.detail(found), nil
}

func (a *emergencyContactAdapter) Create(ctx context.Context, actor access.Actor, body any) (records.Record, error) {
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

func (a *emergencyContactAdapter) Update(ctx context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
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

func (a *emergencyContactAdapter) Delete(ctx context.Context, actor access.Actor, id, version string) error {
	return a.service.Delete(ctx, actor, id, version)
}

func (a *emergencyContactAdapter) detail(entity clinical.EmergencyContact) records.Record {
	return a.record(entity, a.codec.Detail(entity))
}

func (a *emergencyContactAdapter) record(entity clinical.EmergencyContact, body any) records.Record {
	return records.Record{
		ID:        entity.ID,
		Kind:      kind.EmergencyContact,
		PatientID: entity.PatientID,
		Version:   entity.Version,
		Body:      body,
	}
}

type emergencyContactStreamFilter struct{}

func (emergencyContactStreamFilter) Streams(recordID, patientID string) bool {
	return recordID != "" && patientID != ""
}

// EmergencyContactWiring is everything an emergency-contact registration
// needs that this package does not own.
type EmergencyContactWiring struct {
	Repository emergencycontact.Repository
	Authorizer emergencycontact.Authorizer
	Codec      EmergencyContactCodec

	Schema records.Schema
	Views  records.Views

	SearchFields func(any) (title, body string)
	Basis        func(any, records.Criteria) []string
}

// RegisterEmergencyContact wires emergency contacts into the record
// registry.
func RegisterEmergencyContact(registry *records.Registry, wiring EmergencyContactWiring) error {
	if registry == nil {
		return fmt.Errorf("emergencycontact: there is no registry to register %s into", kind.EmergencyContact)
	}

	service, err := emergencycontact.New(wiring.Repository, wiring.Authorizer)
	if err != nil {
		return err
	}

	adapter, err := newEmergencyContactAdapter(service, wiring.Codec)
	if err != nil {
		return err
	}

	schema := wiring.Schema
	schema.Sorts = emergencycontact.Sorts()
	schema.DefaultSort = emergencycontact.Sorts()
	schema.Filters = map[string]records.FilterSpec{
		emergencycontact.FilterIsActive: {
			Name:    emergencycontact.FilterIsActive,
			Kind:    records.FilterEnum,
			Allowed: boolStrings(),
		},
	}

	return registry.Register(records.Registration{
		Kind:       kind.EmergencyContact,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     emergencyContactStreamFilter{},
		Target:     audit.TargetKindEmergencyContact,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Emergency contacts",
			Summary: "Who to reach for the person, in the order to call them.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: EmergencyContactSeedFixtureID,
	})
}
