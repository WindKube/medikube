// Package timeline is the PocketBase side of US9's cross-kind view
// (internal/service/timeline declares the Reader port; this package answers
// it against search_index — the same collection internal/store/search's
// write side keeps in step, read here with the two extra narrowings the
// timeline needs and the cross-kind list does not: tags and a date range
// (research D-06, contracts/records-clinical.md §2).
package timeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	svctimeline "medikube/internal/service/timeline"
	"medikube/internal/store"
)

// Collection is search_index's name, declared again here rather than
// imported from internal/store/search, the same way every store package
// declares its own copy of a collection it merely reads (internal/store/owner.go).
const Collection = "search_index"

const (
	fieldPatient    = "patient"
	fieldKind       = "kind"
	fieldRecordID   = "record_id"
	fieldOccurredOn = "occurred_on"
	fieldTags       = "tags"
)

// sortOccurredOn is the timeline's one ordering (research D-06): primary date
// descending, absent last, id descending as the tie-break — the same
// SortOccurredOn internal/store/search declares, spelled again here for the
// same reason Collection is.
var sortOccurredOn = domain.SortKey{Field: fieldOccurredOn, Desc: true}

var indexSchema = store.NewSchema(Collection,
	store.Column{Name: fieldPatient},
	store.Column{Name: fieldKind},
	store.Column{Name: fieldRecordID},
	store.Column{Name: fieldOccurredOn},
	store.Column{Name: fieldTags, FilterOnly: true, Value: relationSetValue(fieldTags)},
)

// Repo is timeline.Reader against a real instance.
type Repo struct {
	app     core.App
	cursors *store.CursorCodec
}

var _ svctimeline.Reader = (*Repo)(nil)

func New(app core.App, cursors *store.CursorCodec) (*Repo, error) {
	if app == nil {
		return nil, fmt.Errorf("timeline: the repository is wired with no application")
	}

	if cursors == nil {
		return nil, fmt.Errorf("timeline: the repository is wired with no cursor codec")
	}

	return &Repo{app: app, cursors: cursors}, nil
}

// Page returns one page of a patient's timeline, ordered by sortOccurredOn.
func (r *Repo) Page(
	ctx context.Context, patientID string, kinds []kind.Kind, tags []string, from, to string,
	limit int, cursor string,
) (domain.Page[svctimeline.Ref], error) {
	var empty domain.Page[svctimeline.Ref]

	sortKeys := []domain.SortKey{sortOccurredOn}

	listing := store.Query{
		Conditions: r.narrowing(patientID, kinds, tags, from, to),
		Sort:       sortKeys,
		Limit:      limit,
	}

	if cursor != "" {
		after, err := r.cursors.Decode(scope(patientID), sortKeys, cursor)
		if err != nil {
			return empty, err
		}

		listing.After = after
	}

	built, err := indexSchema.Build(listing)
	if err != nil {
		return empty, err
	}

	size := built.Limit
	built.Limit = size + 1

	var records []*core.Record

	if queryErr := built.Apply(r.app.RecordQuery(Collection)).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("timeline: paging the index: %w", queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]svctimeline.Ref, 0, len(records))

	for _, record := range records {
		ref, refErr := refFromRecord(record)
		if refErr != nil {
			return empty, refErr
		}

		items = append(items, ref)
	}

	var next *string

	if more {
		token, boundaryErr := r.boundary(patientID, records[len(records)-1], sortKeys)
		if boundaryErr != nil {
			return empty, boundaryErr
		}

		next = &token
	}

	return domain.NewPage(items, next), nil
}

// Count answers how many index rows the same narrowing matches, with no
// keyset boundary.
func (r *Repo) Count(ctx context.Context, patientID string, kinds []kind.Kind, tags []string, from, to string) (int, error) {
	built, err := indexSchema.Build(store.Query{Conditions: r.narrowing(patientID, kinds, tags, from, to)})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(Collection).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("timeline: counting the index: %w", rowErr)
	}

	return total, nil
}

func (r *Repo) narrowing(patientID string, kinds []kind.Kind, tags []string, from, to string) []store.Condition {
	conditions := []store.Condition{store.Equal(fieldPatient, patientID)}

	if len(kinds) > 0 {
		values := make([]string, 0, len(kinds))
		for _, k := range kinds {
			values = append(values, k.Enum())
		}

		conditions = append(conditions, store.OneOf(fieldKind, values...))
	}

	if len(tags) > 0 {
		conditions = append(conditions, store.AnyOf(fieldTags, tags...))
	}

	if from != "" {
		conditions = append(conditions, store.GTE(fieldOccurredOn, from))
	}

	if to != "" {
		conditions = append(conditions, store.LTE(fieldOccurredOn, to))
	}

	return conditions
}

func (r *Repo) boundary(patientID string, record *core.Record, sortKeys []domain.SortKey) (string, error) {
	cursor, err := indexSchema.Boundary(record, sortKeys)
	if err != nil {
		return "", err
	}

	return r.cursors.Encode(scope(patientID), cursor)
}

func scope(patientID string) string {
	return Collection + "\x00" + patientID
}

func refFromRecord(record *core.Record) (svctimeline.Ref, error) {
	occurredOn, err := dateFromRecord(record, fieldOccurredOn)
	if err != nil {
		return svctimeline.Ref{}, err
	}

	return svctimeline.Ref{
		Kind:       kind.Kind(record.GetString(fieldKind)),
		RecordID:   record.GetString(fieldRecordID),
		OccurredOn: occurredOn,
	}, nil
}

// dateFromRecord mirrors internal/store/search's own: an unset column holds
// the empty string, which GetDateTime answers as its zero value, and that
// zero has to be recognised before it becomes 1 January of the year 1.
func dateFromRecord(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("timeline: %s is not a calendar date: %w", field, err)
	}

	return date, nil
}

// relationSetValue mirrors internal/store's own: a MaxSelect:0 relation is
// stored as a JSON array of ids, and AnyOf's LIKE match is against that
// encoding.
func relationSetValue(name string) func(record *core.Record) string {
	return func(record *core.Record) string {
		encoded, err := json.Marshal(record.GetStringSlice(name))
		if err != nil {
			return "[]"
		}

		return string(encoded)
	}
}
