// Package recordstest is the second implementation of every interface
// internal/records declares, and the second registered kind.
//
// It exists because this phase ships one clinical kind and a discriminated
// oneOf with a single branch proves nothing about the mechanism phase 003 bets
// thirteen kinds on. The synthetic kind here is what makes the OpenAPI
// discriminator gate meaningful (research D-08) and what satisfies Principle
// I's two-implementations clause on the day the registry lands (plan.md CT-2).
//
// It is test support and never registered in a production build:
// recordstest_test.go asserts nothing outside a _test.go file imports it, and
// internal/di asserts the registry it builds has no synthetic kinds.
package recordstest

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"sync"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
)

// The synthetic kind's three spellings, declared here for the same reason
// internal/domain/kind declares the real ones in one table: a segment is a URL
// and a collection is a schema name, the enum is singular and the segment is
// plural, and a second file spelling any of them by hand is finding H1.
//
// This is the one place in the repository outside that table where a kind's
// spellings are literals, and it is legitimate because this is that kind's
// declaration — it is deliberately absent from the production vocabulary, so
// there is no table row to read them from.
const (
	// Kind is the enum spelling: singular snake_case, and the value an OpenAPI
	// discriminator mapping is keyed by.
	Kind kind.Kind = "fake_kind"

	// Segment is the plural path spelling: /api/v1/records/fake-kinds.
	Segment = "fake-kinds"

	Collection = "fake_kinds"
)

// OwnerID is the account the fake service attributes its records to and the
// only one it will answer for. Anything else is ErrNotFound, exactly as a real
// kind answers another person's id (FR-033).
const OwnerID = "mkfakeowner0001"

// What the fake views render. Constants rather than assertions on markup: these
// are here so a test can prove the registered views are the ones it registered,
// not so anybody can check HTML.
const (
	RenderedList   = "<fake-list>"
	RenderedRow    = "<fake-row>"
	RenderedDetail = "<fake-detail>"
	RenderedForm   = "<fake-form>"
)

// The synthetic kind's own DTOs. They are real Go types with real json tags,
// because internal/openapi reflects them into the second oneOf branch — a
// map[string]any here would make the gate assert nothing.

// Summary is what the fake kind's list returns.
type Summary struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
}

// Detail is what the fake kind's read returns.
type Detail struct {
	Summary

	Note      string   `json:"note,omitempty"`
	Doses     int      `json:"doses,omitempty"`
	CreatedAt string   `json:"created_at"`
	Tags      []string `json:"tags,omitempty"`
}

// GetTags implements records.Tagged, the same way a real kind's own wire DTO
// does, so a fake registration can prove the search index's tag-lifecycle
// path without a real kind's DTO.
func (d *Detail) GetTags() []string { return d.Tags }

// Create carries no owner and no id, which is how FR-032 is enforced by shape:
// there is no field a caller could re-attribute a record with.
//
// Doses is here and is an int for one reason: it is the only shape of decode
// failure whose message embeds the submitted value, so it is what
// handler_test.go's PHI assertion needs to have something to leak.
type Create struct {
	Name  string `json:"name"`
	Note  string `json:"note,omitempty"`
	Doses int    `json:"doses,omitempty"`
}

// Patch distinguishes an absent field from one that was sent.
type Patch struct {
	Name *string `json:"name,omitempty"`
	Note *string `json:"note,omitempty"`
}

// SortName is the fake kind's one published ordering, and therefore its
// default.
var SortName = domain.SortKey{Field: "name"}

// FilterName is the fake kind's one published query parameter.
const FilterName = "name"

// SeedFixtureID is the fake kind's fixture id, for a registration that needs
// one and has no real seed to point at.
const SeedFixtureID = "fake-kind-01"

type stored struct {
	owner   string
	version int
	detail  Detail
}

// FakeKindService is an in-memory records.Service. It is the second
// implementation the contract suite runs against and the one the generic
// handler's own tests dispatch to.
//
// It enforces ownership the way a real kind must: another account's id is
// answered exactly as an id that never existed, with the same error, so a test
// of the generic layer cannot pass while the layer weakens FR-033.
type FakeKindService struct {
	mu     sync.Mutex
	next   int
	byID   map[string]stored
	kind   kind.Kind
	frozen error
}

func NewFakeKindService() *FakeKindService {
	return &FakeKindService{byID: make(map[string]stored), kind: Kind}
}

// ForKind returns a fake serving some other kind, so a registration for a real
// kind can be assembled without the real service existing yet.
func (s *FakeKindService) ForKind(k kind.Kind) *FakeKindService {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.kind = k

	return s
}

// Fail makes every method return err. It is how a test drives the generic
// handler's error path without inventing a second fake.
func (s *FakeKindService) Fail(err error) *FakeKindService {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.frozen = err

	return s
}

func (s *FakeKindService) List(_ context.Context, actor access.Actor, query records.Query) (domain.Page[records.Record], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.frozen != nil {
		return domain.Page[records.Record]{}, s.frozen
	}

	items := make([]records.Record, 0, len(s.byID))

	for _, id := range slices.Sorted(maps.Keys(s.byID)) {
		row := s.byID[id]
		if row.owner != actor.UserID {
			continue
		}

		if names, filtered := query.Filters[FilterName]; filtered && !slices.Contains(names, row.detail.Name) {
			continue
		}

		items = append(items, s.record(id, row))
	}

	return domain.NewPage(items, nil), nil
}

func (s *FakeKindService) Get(_ context.Context, actor access.Actor, id string) (records.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.owned(actor, id)
	if err != nil {
		return records.Record{}, err
	}

	return s.record(id, row), nil
}

func (s *FakeKindService) Create(_ context.Context, actor access.Actor, body any) (records.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.frozen != nil {
		return records.Record{}, s.frozen
	}

	// The generic handler minted this pointer from the kind's own Schema, so a
	// type it did not mint is a wiring mistake in the registration and not a
	// caller's mistake.
	create, minted := body.(*Create)
	if !minted {
		return records.Record{}, fmt.Errorf("recordstest: create was handed %T and not *recordstest.Create", body)
	}

	var invalid domain.ValidationError
	if create.Name == "" {
		invalid.Add("name", domain.CodeRequired, "a name is required")
	}

	if err := invalid.OrNil(); err != nil {
		return records.Record{}, err
	}

	s.next++
	id := "mkfakerecord" + strconv.Itoa(s.next)

	row := stored{
		owner:   actor.UserID,
		version: 1,
		detail: Detail{
			Summary:   Summary{ID: id, Kind: string(s.kind), Name: create.Name, UpdatedAt: "2026-01-01T00:00:00Z"},
			Note:      create.Note,
			Doses:     create.Doses,
			CreatedAt: "2026-01-01T00:00:00Z",
		},
	}
	s.byID[id] = row

	return s.record(id, row), nil
}

func (s *FakeKindService) Update(_ context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.owned(actor, id)
	if err != nil {
		return records.Record{}, err
	}

	if version != versionOf(row.version) {
		return records.Record{}, domain.ErrVersionMismatch
	}

	patch, minted := body.(*Patch)
	if !minted {
		return records.Record{}, fmt.Errorf("recordstest: update was handed %T and not *recordstest.Patch", body)
	}

	if patch.Name != nil {
		row.detail.Name = *patch.Name
	}

	if patch.Note != nil {
		row.detail.Note = *patch.Note
	}

	row.version++
	s.byID[id] = row

	return s.record(id, row), nil
}

func (s *FakeKindService) Delete(_ context.Context, actor access.Actor, id, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.owned(actor, id)
	if err != nil {
		return err
	}

	if version != versionOf(row.version) {
		return domain.ErrVersionMismatch
	}

	delete(s.byID, id)

	return nil
}

// owned is where FR-033 lives in this fake. The two refusals are the same
// error value with the same text, because a caller able to tell them apart has
// been told which ids exist.
func (s *FakeKindService) owned(actor access.Actor, id string) (stored, error) {
	if s.frozen != nil {
		return stored{}, s.frozen
	}

	row, exists := s.byID[id]
	if !exists || row.owner != actor.UserID {
		return stored{}, domain.ErrNotFound
	}

	return row, nil
}

func (s *FakeKindService) record(id string, row stored) records.Record {
	detail := row.detail

	return records.Record{ID: id, Kind: s.kind, PatientID: row.owner, Version: versionOf(row.version), Body: &detail}
}

func versionOf(n int) string { return "v" + strconv.Itoa(n) }

// Views renders the four constants above.
type Views struct{}

func (Views) List(domain.Page[records.Record]) records.Renderer { return static(RenderedList) }
func (Views) Row(records.Record) records.Renderer               { return static(RenderedRow) }
func (Views) Detail(records.Record) records.Renderer            { return static(RenderedDetail) }

func (Views) Form(records.Record, *domain.ValidationError, string) records.Renderer {
	return static(RenderedForm)
}

type static string

func (s static) Render(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := io.WriteString(w, string(s))

	return err
}

// StreamFilter admits everything unless told otherwise. A test that wants the
// filter to refuse sets Deny; the per-subscriber authorization is a separate
// check and this is not a stand-in for it.
type StreamFilter struct {
	Deny bool
}

func (f StreamFilter) Streams(string, string) bool { return !f.Deny }

// Authorizer grants PermOwn to OwnerID and refuses everybody else with
// ErrNotFound, which is what a patient-scoped refusal is (FR-033).
type Authorizer struct {
	Owner string
}

func (a Authorizer) Patient(_ context.Context, actor access.Actor, _ string, need access.Permission) (access.Grant, error) {
	owner := a.Owner
	if owner == "" {
		owner = OwnerID
	}

	if actor.UserID != owner {
		return access.Grant{}, domain.ErrNotFound
	}

	grant := access.Grant{Level: access.PermOwn}
	if !grant.Allows(need) {
		return access.Grant{}, domain.ErrNotFound
	}

	return grant, nil
}

// Schema is the fake kind's four DTO shapes and its query vocabulary.
func Schema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return &Summary{} },
		NewDetail:  func() any { return &Detail{} },
		NewCreate:  func() any { return &Create{} },
		NewPatch:   func() any { return &Patch{} },
		Sorts:      []domain.SortKey{SortName, {Field: "name", Desc: true}},
		Filters:    map[string]records.FilterSpec{FilterName: {Name: FilterName, Kind: records.FilterFreeform}},
	}
}

// SearchFields reads the fake kind's own DTO, the same shape Views reads.
func SearchFields(body any) (title, text string) {
	detail, ok := body.(*Detail)
	if !ok {
		return "", ""
	}

	return detail.Name, detail.Note
}

// Basis narrows nothing; the fake kind declares no basis-worthy narrowing.
func Basis(any, records.Criteria) []string { return nil }

// Registration is a complete registration for any kind, wired to fakes.
//
// This phase's real kind has no service yet — internal/service/medication is
// T134 and later — so this is what internal/records, internal/openapi and
// internal/web test the registry against in the meantime, and it is what T104
// zeroes one field of at a time to prove each consumer is checked.
func Registration(k kind.Kind, target audit.TargetKind) records.Registration {
	return records.Registration{
		Kind:       k,
		Service:    NewFakeKindService().ForKind(k),
		Schema:     Schema(),
		Views:      Views{},
		Stream:     StreamFilter{},
		Target:     target,
		Authorizer: Authorizer{},
		Inventory: records.Inventory{
			Title:   "Fake records",
			Summary: "A kind wired to fakes, for tests that need a registration and not a database.",
		},
		SearchFields:  SearchFields,
		Basis:         Basis,
		SeedFixtureID: SeedFixtureID,
	}
}

// SyntheticRegistration is the second kind: the one the production vocabulary
// does not declare and the one that gives the discriminated oneOf a second
// branch to be gated against.
func SyntheticRegistration() records.Registration {
	registration := Registration(Kind, audit.TargetKindLabResult)
	registration.Inventory = records.Inventory{
		Title:   "Synthetic records",
		Summary: "The second registered kind, so a one-kind phase can still gate a two-branch discriminator.",
	}

	return registration
}

// RegisterSynthetic adds the second kind to a registry.
//
// The audit target it borrows is lab_result: declared complete in
// internal/domain/audit and written by no phase before 004. Borrowing a
// declared value rather than inventing one keeps the synthetic registration
// subject to the same audit check a real kind faces — a fake that had to be
// waved past a check would only prove the check can be waved past.
func RegisterSynthetic(registry *records.Registry) error {
	return registry.RegisterSynthetic(SyntheticRegistration(), Segment, Collection)
}
