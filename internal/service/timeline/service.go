package timeline

import (
	"context"
	"errors"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
)

// Entry is one timeline row: its kind, its identifying summary (the same
// title search.Row indexes, research D-11) and its primary date — nil when
// the record has none, rendered as `occurred_on: null` and grouped under
// "Date not recorded" rather than placed at either extreme (FR-076, FR-077).
type Entry struct {
	ID         string
	Kind       kind.Kind
	Title      string
	OccurredOn *string
}

// Query is one timeline request. Kinds empty means every registered kind.
type Query struct {
	PatientID string
	Kinds     []kind.Kind
	Tags      []string
	From, To  string
	Limit     int
	Cursor    string
	Count     bool
}

// Service is US9's timeline: the registry supplies each kind's authorizer,
// its per-record hydration and the title search.Row already indexed;
// Reader supplies the ordering (research D-06).
type Service struct {
	registry *records.Registry
	reader   Reader
}

func New(registry *records.Registry, reader Reader) (*Service, error) {
	if registry == nil {
		return nil, errors.New("timeline: the service is wired with no registry")
	}

	if reader == nil {
		return nil, errors.New("timeline: the service is wired with no reader")
	}

	return &Service{registry: registry, reader: reader}, nil
}

// List answers one page of a patient's timeline, authorized once against any
// one selected kind's checkpoint — every registered kind anchors on the same
// patient (internal/records/registry.go's own doc) — and hydrated per row
// through that row's own kind, which is where FR-081's ownership check lives.
// A ref whose record is gone by the time it is hydrated is skipped rather than
// failing the page, mirroring internal/records' own cross-kind list.
func (s *Service) List(ctx context.Context, actor access.Actor, query Query) (domain.Page[Entry], error) {
	entries, err := s.resolve(query.Kinds)
	if err != nil {
		return domain.Page[Entry]{}, err
	}

	if len(entries) == 0 {
		return domain.NewPage([]Entry{}, nil), nil
	}

	if _, authErr := entries[0].Authorizer.Patient(ctx, actor, query.PatientID, access.PermView); authErr != nil {
		return domain.Page[Entry]{}, authErr
	}

	kinds := make([]kind.Kind, 0, len(entries))
	for _, entry := range entries {
		kinds = append(kinds, entry.Kind)
	}

	refs, err := s.reader.Page(ctx, query.PatientID, kinds, query.Tags, query.From, query.To, query.Limit, query.Cursor)
	if err != nil {
		return domain.Page[Entry]{}, err
	}

	items := make([]Entry, 0, len(refs.Items))

	for _, ref := range refs.Items {
		entry, found := s.registry.FromKind(ref.Kind)
		if !found {
			continue
		}

		record, getErr := entry.Service.Get(ctx, actor, ref.RecordID)
		if errors.Is(getErr, domain.ErrNotFound) {
			continue
		}

		if getErr != nil {
			return domain.Page[Entry]{}, getErr
		}

		title, _ := entry.SearchFields(record.Body)

		items = append(items, Entry{
			ID:         record.ID,
			Kind:       ref.Kind,
			Title:      title,
			OccurredOn: dateOrNil(ref.OccurredOn),
		})
	}

	page := domain.NewPage(items, refs.NextCursor)

	if query.Count {
		total, countErr := s.reader.Count(ctx, query.PatientID, kinds, query.Tags, query.From, query.To)
		if countErr != nil {
			return domain.Page[Entry]{}, countErr
		}

		page = page.WithTotal(total)
	}

	return page, nil
}

// resolve turns the caller's kind selection into registered entries. An empty
// selection is every registered kind; an unregistered value in a non-empty
// one is refused rather than silently dropped (mirrors
// internal/records.Handler.selection).
func (s *Service) resolve(selected []kind.Kind) ([]records.Entry, error) {
	if len(selected) == 0 {
		return s.registry.Entries(), nil
	}

	var (
		entries []records.Entry
		invalid domain.ValidationError
	)

	for _, k := range selected {
		entry, found := s.registry.FromKind(k)
		if !found {
			invalid.Add("kind", domain.CodeInvalidValue, "the kind is not one this instance serves")

			continue
		}

		entries = append(entries, entry)
	}

	if err := invalid.OrNil(); err != nil {
		return nil, err
	}

	return entries, nil
}

func dateOrNil(d domain.Date) *string {
	if d.IsZero() {
		return nil
	}

	value := d.String()

	return &value
}
