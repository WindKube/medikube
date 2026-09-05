package treatment

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
	Summary(t clinical.Treatment) any
	Detail(t clinical.Treatment) any
	Draft(body any) (clinical.Treatment, error)
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
		return nil, fmt.Errorf("treatment: the adapter is wired with no %s", joinWords(absent))
	}

	return &Adapter{service: service, codec: codec}, nil
}

func (a *Adapter) List(ctx context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	page, err := a.service.List(ctx, actor, Query{
		PatientID: query.PatientID,
		Search:    query.Search,
		Statuses:  statuses(query.Filters[FilterStatus]),
		Ongoing:   ongoingFlag(query.Filters[FilterOngoing]),
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

func (a *Adapter) detail(t clinical.Treatment) records.Record {
	return a.record(t, a.codec.Detail(t))
}

func (a *Adapter) record(t clinical.Treatment, body any) records.Record {
	return records.Record{ID: t.ID, Kind: kind.Treatment, PatientID: t.PatientID, Version: t.Version, Body: body}
}

const SeedFixtureID = "treatment-01"

func statuses(values []string) []clinical.TherapyStatus {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.TherapyStatus, 0, len(values))
	for _, v := range values {
		converted = append(converted, clinical.TherapyStatus(v))
	}

	return converted
}

func ongoingFlag(values []string) *bool {
	if len(values) == 0 {
		return nil
	}

	flag := values[0] == "true"

	return &flag
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
		return fmt.Errorf("treatment: there is no registry to register %s into", kind.Treatment)
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
		FilterStatus:  {Name: FilterStatus, Kind: records.FilterEnum, Allowed: therapyStatusStrings()},
		FilterOngoing: {Name: FilterOngoing, Kind: records.FilterEnum, Allowed: []string{"true", "false"}},
	}

	return registry.Register(records.Registration{
		Kind:       kind.Treatment,
		Service:    adapter,
		Schema:     schema,
		Views:      wiring.Views,
		Stream:     StreamFilter{},
		Target:     audit.TargetKindTreatment,
		Authorizer: wiring.Authorizer,
		Inventory: records.Inventory{
			Title:   "Treatments",
			Summary: "Every course of treatment the person recorded: what it is, when it started, whether it is still running and what it is meant to achieve.",
		},
		SearchFields:  wiring.SearchFields,
		Basis:         wiring.Basis,
		SeedFixtureID: SeedFixtureID,
	})
}

func therapyStatusStrings() []string {
	values := clinical.TherapyStatuses()
	out := make([]string, 0, len(values))

	for _, v := range values {
		out = append(out, string(v))
	}

	return out
}

// OngoingSet is the two statuses `?ongoing=true` narrows to.
func OngoingSet() []clinical.TherapyStatus {
	return append([]clinical.TherapyStatus(nil), ongoingStatuses...)
}
