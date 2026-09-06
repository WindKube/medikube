// Package search is the PocketBase side of the unified search index's write
// path: internal/service/search declares the Repository port, this package
// implements it against the search_index collection (data-model §5.3).
package search

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/service/search"
	"medikube/internal/store"
)

// Collection is search_index's name, declared here rather than imported from
// internal/store/migrations: every store package that owns a collection
// declares its own copy of the name and its field spellings (see
// internal/store/owner.go), so that no store package depends on the
// migrations that merely created what it now reads and writes.
const Collection = "search_index"

const (
	fieldPatient    = "patient"
	fieldKind       = "kind"
	fieldRecordID   = "record_id"
	fieldTitle      = "title"
	fieldBody       = "body"
	fieldOccurredOn = "occurred_on"
	fieldTags       = "tags"
)

// SortOccurredOn is the cross-kind list's one published ordering: most recent
// first. A bare descending column already puts the empty string — an unset
// occurred_on — after every real date, so nulls sort last with no AbsentLast
// flag needed (unlike the ascending case internal/store/filter.go's
// AbsentLast column exists to fix).
var SortOccurredOn = domain.SortKey{Field: fieldOccurredOn, Desc: true}

// indexSchema is search_index's query surface: the two columns a cross-kind
// page narrows by (patient, kind) and the one it orders by. It is declared
// here, beside the field names it reads, for the same reason
// internal/store's own per-collection schemas are declared beside theirs: the
// column's SQL expression and its Go twin are the same knowledge, and
// splitting them across two files is how the two drift apart.
var indexSchema = store.NewSchema(Collection,
	store.Column{Name: fieldPatient},
	store.Column{Name: fieldKind},
	store.Column{Name: fieldRecordID},
	store.Column{Name: fieldOccurredOn},
	// title and body are narrowable and never orderable: US8's term match is
	// the disjunction store.ContainsAny builds over the two, and FilterOnly
	// keeps either out of every ordering and every keyset boundary — the same
	// reason a drug name never travels in a cursor (filter.go's own doc on
	// Column.FilterOnly).
	store.Column{Name: fieldTitle, FilterOnly: true, Searchable: true},
	store.Column{Name: fieldBody, FilterOnly: true, Searchable: true},
	// FilterOnly: `?tags=` narrows (T164-T177 follow-up), but a MaxSelect:0
	// relation's JSON column is never an ordering (research D-05's
	// cursor-disclosure rule) — the same reason every kind's own tags column
	// is FilterOnly.
	store.Column{Name: fieldTags, FilterOnly: true},
)

// Repo is search.Repository — and search.Reader — against a real instance.
type Repo struct {
	app     core.App
	cursors *store.CursorCodec
}

var (
	_ search.Repository = (*Repo)(nil)
	_ search.Reader     = (*Repo)(nil)
	_ search.Searcher   = (*Repo)(nil)
	_ search.Counter    = (*Repo)(nil)
)

// New wires the repository to an instance and to the codec that seals the
// cross-kind list's cursors — the same codec every other paged repository in
// this module shares (internal/store/medication, internal/store/patient).
func New(app core.App, cursors *store.CursorCodec) (*Repo, error) {
	if app == nil {
		return nil, fmt.Errorf("search: the repository is wired with no application")
	}

	if cursors == nil {
		return nil, fmt.Errorf("search: the repository is wired with no cursor codec")
	}

	return &Repo{app: app, cursors: cursors}, nil
}

// Upsert creates the row if none exists for (Kind, RecordID) and replaces it
// otherwise — one row per record, always (uniq_search_record).
func (r *Repo) Upsert(ctx context.Context, row search.Row) error {
	record, err := r.find(ctx, row.Kind, row.RecordID)
	if err != nil {
		return err
	}

	if record == nil {
		collection, collErr := r.app.FindCachedCollectionByNameOrId(Collection)
		if collErr != nil {
			return fmt.Errorf("search: reading %s: %w", Collection, collErr)
		}

		record = core.NewRecord(collection)
	}

	record.Set(fieldPatient, row.PatientID)
	record.Set(fieldKind, row.Kind.Enum())
	record.Set(fieldRecordID, row.RecordID)
	record.Set(fieldTitle, row.Title)
	record.Set(fieldBody, row.Body)
	record.Set(fieldOccurredOn, row.OccurredOn.UTC())
	record.Set(fieldTags, row.TagIDs)

	if err := r.app.SaveWithContext(ctx, record); err != nil {
		return fmt.Errorf("search: indexing a %s row: %w", row.Kind, err)
	}

	return nil
}

// Find reads the indexed row for one record, if any. It exists for a caller
// that reads the index back to prove the write side actually wrote what it
// claims to (internal/web/api's own record-contract proof), not for the
// unified search that is US8's own.
func (r *Repo) Find(ctx context.Context, k kind.Kind, recordID string) (search.Row, bool, error) {
	record, err := r.find(ctx, k, recordID)
	if err != nil {
		return search.Row{}, false, err
	}

	if record == nil {
		return search.Row{}, false, nil
	}

	return search.Row{
		PatientID: record.GetString(fieldPatient),
		Kind:      k,
		RecordID:  record.GetString(fieldRecordID),
		Title:     record.GetString(fieldTitle),
		Body:      record.GetString(fieldBody),
		TagIDs:    record.GetStringSlice(fieldTags),
	}, true, nil
}

// Remove deletes the row for one record, if any.
func (r *Repo) Remove(ctx context.Context, k kind.Kind, recordID string) error {
	record, err := r.find(ctx, k, recordID)
	if err != nil {
		return err
	}

	if record == nil {
		return nil
	}

	if err := r.app.DeleteWithContext(ctx, record); err != nil {
		return fmt.Errorf("search: removing a %s row: %w", k, err)
	}

	return nil
}

// RemoveByPatient deletes every row for a patient, in the same commit as the
// patient's own delete (FR-087, SC-005). The relation also cascades this on
// its own; this exists for the caller that removes a patient without going
// through PocketBase's own delete, and so that the cascade is asserted rather
// than assumed.
func (r *Repo) RemoveByPatient(ctx context.Context, patientID string) error {
	var records []*core.Record

	if err := r.app.RecordQuery(Collection).
		AndWhere(dbx.HashExp{fieldPatient: patientID}).
		WithContext(ctx).
		All(&records); err != nil {
		return fmt.Errorf("search: reading a patient's index rows: %w", err)
	}

	for _, record := range records {
		if err := r.app.DeleteWithContext(ctx, record); err != nil {
			return fmt.Errorf("search: removing a patient's index row: %w", err)
		}
	}

	return nil
}

// Page returns one page of a patient's cross-kind index, ordered by
// SortOccurredOn, and mints the boundary for the next one.
//
// Paged the same way internal/store/medication/repo.go's List pages: one row
// more than the page decides whether there is a next one, and the boundary is
// the last row read rather than an offset, so a concurrent insert cannot make
// this traversal repeat or skip a row (FR-023).
func (r *Repo) Page(
	ctx context.Context, patientID string, kinds []kind.Kind, limit int, cursor string,
) (domain.Page[search.Ref], error) {
	var empty domain.Page[search.Ref]

	sortKeys := []domain.SortKey{SortOccurredOn}

	listing := store.Query{Conditions: r.narrowing(patientID, kinds), Sort: sortKeys, Limit: limit}

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
		return empty, fmt.Errorf("search: paging the index: %w", queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]search.Ref, 0, len(records))

	for _, record := range records {
		ref, refErr := refFromRecord(record)
		if refErr != nil {
			return empty, refErr
		}

		items = append(items, ref)
	}

	var next *string

	if more {
		token, boundaryErr := r.boundaryFor(scope(patientID), records[len(records)-1], sortKeys)
		if boundaryErr != nil {
			return empty, boundaryErr
		}

		next = &token
	}

	return domain.NewPage(items, next), nil
}

// SearchKind is US8's per-kind matcher: one page of one kind's rows whose title
// or body contains term, ordered SortOccurredOn, paged the same keyset way
// Page is. The term is bound through store.ContainsAny — dbx params, never a
// concatenated filter string (contracts/search.md §5) — which is what
// escapes `%`, `_` and the escape character before it ever reaches SQLite.
func (r *Repo) SearchKind(
	ctx context.Context, patientID string, k kind.Kind, term string, tagIDs []string, match string, limit int, cursor string,
) (domain.Page[search.Hit], error) {
	var empty domain.Page[search.Hit]

	sortKeys := []domain.SortKey{SortOccurredOn}

	conditions := []store.Condition{
		store.Equal(fieldPatient, patientID),
		store.Equal(fieldKind, k.Enum()),
		store.ContainsAny(term, fieldTitle, fieldBody),
	}

	// `?tags=`/`?match=` (T164-T177 follow-up), the same AnyOf/AllOf every
	// kind's own list already narrows by (e.g. internal/store/condition).
	if len(tagIDs) > 0 {
		if match == search.MatchAll {
			conditions = append(conditions, store.AllOf(fieldTags, tagIDs...))
		} else {
			conditions = append(conditions, store.AnyOf(fieldTags, tagIDs...))
		}
	}

	listing := store.Query{Conditions: conditions, Sort: sortKeys, Limit: limit}

	scopeKey := searchScope(patientID, k)

	if cursor != "" {
		after, err := r.cursors.Decode(scopeKey, sortKeys, cursor)
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
		return empty, fmt.Errorf("search: matching the index: %w", queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]search.Hit, 0, len(records))

	for _, record := range records {
		hit, hitErr := hitFromRecord(record)
		if hitErr != nil {
			return empty, hitErr
		}

		items = append(items, hit)
	}

	var next *string

	if more {
		token, boundaryErr := r.boundaryFor(scopeKey, records[len(records)-1], sortKeys)
		if boundaryErr != nil {
			return empty, boundaryErr
		}

		next = &token
	}

	return domain.NewPage(items, next), nil
}

// searchScope is the query a per-kind search cursor continues: this index,
// for this patient, for this one kind's group. It carries no term — the
// cursor is authenticated encryption over the sort values and the row id
// (research D-25), the same reasoning scope's own doc gives for the
// cross-kind list, and a different term submitted against an old cursor
// simply lands on a boundary that term may not match, which the keyset
// predicate already answers correctly.
func searchScope(patientID string, k kind.Kind) string {
	return Collection + "\x00search\x00" + patientID + "\x00" + k.Enum()
}

// hitFromRecord reads a stored row into US8's own result shape — title and
// tags included, unlike refFromRecord's Ref, because a search result names
// what it found rather than making the caller re-fetch the record to render
// one line.
func hitFromRecord(record *core.Record) (search.Hit, error) {
	occurredOn, err := dateFromRecord(record, fieldOccurredOn)
	if err != nil {
		return search.Hit{}, err
	}

	return search.Hit{
		Kind:       kind.Kind(record.GetString(fieldKind)),
		RecordID:   record.GetString(fieldRecordID),
		Title:      record.GetString(fieldTitle),
		TagIDs:     record.GetStringSlice(fieldTags),
		OccurredOn: occurredOn,
	}, nil
}

// Count answers how many index rows the same narrowing matches, with no
// keyset boundary — the same number on every page of a traversal.
func (r *Repo) Count(ctx context.Context, patientID string, kinds []kind.Kind) (int, error) {
	built, err := indexSchema.Build(store.Query{Conditions: r.narrowing(patientID, kinds)})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(Collection).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("search: counting the index: %w", rowErr)
	}

	return total, nil
}

// narrowing is the page's terms: this patient, and — when the caller selected
// fewer than every registered kind — this set of kinds.
func (r *Repo) narrowing(patientID string, kinds []kind.Kind) []store.Condition {
	conditions := make([]store.Condition, 0, 2)
	conditions = append(conditions, store.Equal(fieldPatient, patientID))

	if len(kinds) == 0 {
		return conditions
	}

	values := make([]string, 0, len(kinds))
	for _, k := range kinds {
		values = append(values, k.Enum())
	}

	return append(conditions, store.OneOf(fieldKind, values...))
}

// boundaryFor seals the last row of a page into the token the next request
// hands back, under whichever scope key that page's cursor is bound to.
func (r *Repo) boundaryFor(scopeKey string, record *core.Record, sortKeys []domain.SortKey) (string, error) {
	cursor, err := indexSchema.Boundary(record, sortKeys)
	if err != nil {
		return "", err
	}

	return r.cursors.Encode(scopeKey, cursor)
}

// scope is the query a cursor continues: this index, for this patient. Kinds
// are not part of it — the cursor is authenticated encryption over the sort
// values and the row id (research D-25), and a different kind selection
// simply lands on a boundary row that selection may not contain, which the
// keyset predicate already answers correctly.
func scope(patientID string) string {
	return Collection + "\x00" + patientID
}

// refFromRecord reads a stored row into the ref a cross-kind page hydrates
// through the kind's own Service.Get. It carries no title and no body: this
// path never logs or echoes either (data-model §5.3's own content stays
// inside the kind it was indexed from).
func refFromRecord(record *core.Record) (search.Ref, error) {
	occurredOn, err := dateFromRecord(record, fieldOccurredOn)
	if err != nil {
		return search.Ref{}, err
	}

	return search.Ref{
		Kind:       kind.Kind(record.GetString(fieldKind)),
		RecordID:   record.GetString(fieldRecordID),
		OccurredOn: occurredOn,
	}, nil
}

// dateFromRecord reads a calendar date, exactly as internal/store's own
// recordDate does: an unset column holds the empty string, which
// GetDateTime answers as its zero value, and that zero has to be recognised
// before it is converted or "no occurred_on recorded" would become 1 January
// of the year 1.
func dateFromRecord(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("search: %s is not a calendar date: %w", field, err)
	}

	return date, nil
}

func (r *Repo) find(ctx context.Context, k kind.Kind, recordID string) (*core.Record, error) {
	var records []*core.Record

	err := r.app.RecordQuery(Collection).
		AndWhere(dbx.HashExp{fieldKind: k.Enum(), fieldRecordID: recordID}).
		Limit(1).
		WithContext(ctx).
		All(&records)
	if err != nil {
		return nil, fmt.Errorf("search: reading a %s row: %w", k, err)
	}

	if len(records) == 0 {
		return nil, nil
	}

	return records[0], nil
}
