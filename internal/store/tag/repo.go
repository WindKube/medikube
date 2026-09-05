// Package tag is the tags collection's repository, against PocketBase
// (data-model §5.1, US7).
package tag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	dtag "medikube/internal/domain/tag"
	svc "medikube/internal/service/tag"
	"medikube/internal/store"
)

// collection is the tags collection's name. Not a kind.Kind: a tag is not a
// clinical record, it is what a record's own tags relation points at
// (internal/store/migrations/1756300000_tags.go).
const collection = "tags"

const (
	fieldOwner = "owner"
	fieldName  = "name"
	fieldColor = "color"
	fieldID    = "id"
)

// Repo is the tag repository service.Repository, Ownership and UsageCounter
// declare, against PocketBase.
type Repo struct {
	app     core.App
	cursors *store.CursorCodec
	schema  store.Schema

	// taggables is every registered kind's own collection name, read from
	// the registry rather than hand-maintained (FR-090): usage_count sums a
	// membership query over each of these.
	taggables func() []string
}

var (
	_ svc.Repository   = (*Repo)(nil)
	_ svc.Ownership    = (*Repo)(nil)
	_ svc.UsageCounter = (*Repo)(nil)
)

// New wires the repository to an instance, the cursor codec, and a function
// answering the collections a tag's usage is counted across — a closure over
// the record registry rather than a fixed slice, since the registry is not
// fully populated at the moment the tag repository itself is constructed
// (cmd/medikube/handlers.go wires the tag checker before the kinds that need
// it register).
func New(app core.App, cursors *store.CursorCodec, taggables func() []string) (*Repo, error) {
	var missing []string

	if app == nil {
		missing = append(missing, "application")
	}

	if cursors == nil {
		missing = append(missing, "cursor codec")
	}

	if taggables == nil {
		missing = append(missing, "taggable collections")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("tag: the repository is wired with no %s", strings.Join(missing, " and "))
	}

	return &Repo{app: app, cursors: cursors, schema: schema(), taggables: taggables}, nil
}

func schema() store.Schema {
	return store.NewSchema(collection,
		store.Column{Name: fieldOwner},
		store.Column{
			Name:       fieldName,
			Expr:       "LOWER(" + quoteColumn(fieldName) + ")",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldName))
			},
		},
	)
}

func quoteColumn(name string) string { return "[[" + name + "]]" }

func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

func (r *Repo) List(ctx context.Context, ownerID string, query svc.Query) (domain.Page[dtag.Tag], error) {
	var empty domain.Page[dtag.Tag]

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = svc.Sorts()[:1]
	}

	conditions := []store.Condition{store.Equal(fieldOwner, ownerID)}
	if query.Search != "" {
		conditions = append(conditions, store.Contains(fieldName, query.Search))
	}

	// "usage" is not a stored, orderable column (FR-090): sorting by it is
	// served in-process, over the owner's whole tag set, rather than pushed
	// into schema.Build.
	byUsage := len(sortKeys) > 0 && sortKeys[0].Field == "usage"

	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return empty, err
	}

	var records []*core.Record

	if queryErr := built.Apply(r.app.RecordQuery(collection)).WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing tags: %w", queryErr)
	}

	items := make([]dtag.Tag, 0, len(records))

	for _, record := range records {
		entity, mapErr := fromRecord(record)
		if mapErr != nil {
			return empty, mapErr
		}

		items = append(items, entity)
	}

	if byUsage {
		items, err = r.sortByUsage(ctx, ownerID, items)
		if err != nil {
			return empty, err
		}
	} else {
		sortByName(items, sortKeys[0].Desc)
	}

	total := len(items)

	limit := query.Limit
	if limit <= 0 {
		limit = store.DefaultLimit
	}

	offset := 0

	if query.Cursor != "" {
		decoded, decodeErr := r.cursors.Decode(scope(ownerID), sortKeys, query.Cursor)
		if decodeErr != nil {
			return empty, decodeErr
		}

		offset = offsetOf(decoded)
	}

	end := offset + limit
	if end > len(items) {
		end = len(items)
	}

	var page []dtag.Tag
	if offset < len(items) {
		page = items[offset:end]
	}

	var next *string

	if end < len(items) {
		// Not a keyset boundary: an account's own tag vocabulary is small
		// (US7-1's "three tags" is the typical scale, not the exception), and
		// "usage" is a derived aggregate with no indexed column to seat a
		// keyset cursor on. The offset travels inside the same authenticated,
		// scoped cursor every other kind uses, so it is still opaque and
		// still refuses to decode under a different ordering or owner.
		token, tokenErr := r.cursors.Encode(scope(ownerID), store.Cursor{
			Sort: sortKeys, Values: []string{""}, ID: fmt.Sprintf("%d", end),
		})
		if tokenErr != nil {
			return empty, tokenErr
		}

		next = &token
	}

	result := domain.NewPage(page, next)

	if query.Count {
		result = result.WithTotal(total)
	}

	return result, nil
}

func offsetOf(cursor store.Cursor) int {
	var offset int

	_, _ = fmt.Sscanf(cursor.ID, "%d", &offset)

	return offset
}

func sortByName(items []dtag.Tag, desc bool) {
	sortSlice(items, func(i, j int) bool {
		if desc {
			return asciiLower(items[i].Name) > asciiLower(items[j].Name)
		}

		return asciiLower(items[i].Name) < asciiLower(items[j].Name)
	})
}

func (r *Repo) sortByUsage(ctx context.Context, ownerID string, items []dtag.Tag) ([]dtag.Tag, error) {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}

	counts, err := r.Counts(ctx, ownerID, ids)
	if err != nil {
		return nil, err
	}

	sortSlice(items, func(i, j int) bool {
		if counts[items[i].ID] != counts[items[j].ID] {
			return counts[items[i].ID] > counts[items[j].ID]
		}

		return asciiLower(items[i].Name) < asciiLower(items[j].Name)
	})

	return items, nil
}

func sortSlice(items []dtag.Tag, less func(i, j int) bool) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func (r *Repo) Get(ctx context.Context, ownerID, id string) (dtag.Tag, error) {
	record, err := r.owned(ctx, r.app, ownerID, id)
	if err != nil {
		return dtag.Tag{}, err
	}

	return fromRecord(record)
}

func (r *Repo) Create(ctx context.Context, t dtag.Tag) (dtag.Tag, error) {
	coll, err := r.app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		return dtag.Tag{}, fmt.Errorf("reading the tags collection: %w", err)
	}

	record := core.NewRecord(coll)
	toRecord(record, t)

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return dtag.Tag{}, writeFailure("creating a tag", saveErr)
	}

	return fromRecord(record)
}

func (r *Repo) Update(ctx context.Context, t dtag.Tag) (dtag.Tag, error) {
	var updated dtag.Tag

	write := func(txApp core.App) error {
		record, err := r.owned(ctx, txApp, t.OwnerID, t.ID)
		if err != nil {
			return err
		}

		toRecord(record, t)

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return writeFailure(fmt.Sprintf("updating tag %s", t.ID), saveErr)
		}

		mapped, mapErr := fromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return dtag.Tag{}, txErr
	}

	return updated, nil
}

// Delete is permanent. PocketBase's own relation cleanup removes the tag from
// every referencing record's tags field; this deletes only the tag's own row
// (FR-066, US7-4).
func (r *Repo) Delete(ctx context.Context, ownerID, id string) error {
	return store.RunInTransaction(r.app, func(txApp core.App) error {
		record, err := r.owned(ctx, txApp, ownerID, id)
		if err != nil {
			return err
		}

		if deleteErr := txApp.DeleteWithContext(ctx, record); deleteErr != nil {
			return fmt.Errorf("deleting tag %s: %w", id, deleteErr)
		}

		return nil
	})
}

// Owned answers whether every id in ids is a tag ownerID holds (FR-064).
func (r *Repo) Owned(ctx context.Context, ownerID string, ids []string) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}

	var found []string

	err := r.app.RecordQuery(collection).
		Select(fieldID).
		AndWhere(dbx.HashExp{fieldOwner: ownerID}).
		AndWhere(dbx.In(fieldID, toAny(ids)...)).
		WithContext(ctx).
		Column(&found)
	if err != nil {
		return false, fmt.Errorf("checking tag ownership: %w", err)
	}

	return len(found) == len(unique(ids)), nil
}

// Counts is FR-068 and FR-090: how many rows across every taggable
// collection carry each tag, derived at read time and never stored.
func (r *Repo) Counts(ctx context.Context, ownerID string, ids []string) (map[string]int, error) {
	counts := make(map[string]int, len(ids))
	for _, id := range ids {
		counts[id] = 0
	}

	if len(ids) == 0 {
		return counts, nil
	}

	for _, collectionName := range r.taggables() {
		target, err := r.app.FindCachedCollectionByNameOrId(collectionName)
		if err != nil || target.Fields.GetByName("tags") == nil {
			continue
		}

		countSchema := store.NewSchema(collectionName, store.Column{Name: "tags"})

		for _, id := range ids {
			built, buildErr := countSchema.Build(store.Query{Conditions: []store.Condition{store.AnyOf("tags", id)}})
			if buildErr != nil {
				return nil, buildErr
			}

			var n int

			counting := r.app.RecordQuery(collectionName).Select("count(*)")
			if built.Where != nil {
				counting = counting.AndWhere(built.Where)
			}

			if rowErr := counting.WithContext(ctx).Row(&n); rowErr != nil {
				return nil, fmt.Errorf("counting %s carrying tag %s: %w", collectionName, id, rowErr)
			}

			counts[id] += n
		}
	}

	_ = ownerID // every id here already belongs to ownerID; the caller checked

	return counts, nil
}

func toAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}

	return out
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))

	var out []string

	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}

		seen[v] = struct{}{}

		out = append(out, v)
	}

	return out
}

func (r *Repo) owned(ctx context.Context, app core.App, ownerID, id string) (*core.Record, error) {
	var found struct {
		ID string `db:"id"`
	}

	err := app.RecordQuery(collection).
		Select(fieldID).
		AndWhere(dbx.HashExp{fieldID: id, fieldOwner: ownerID}).
		Limit(1).
		WithContext(ctx).
		One(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("tag %s: %w", id, domain.ErrNotFound)
		}

		return nil, fmt.Errorf("reading tag %s: %w", id, err)
	}

	record, err := app.FindRecordById(collection, found.ID)
	if err != nil {
		return nil, fmt.Errorf("reading tag %s: %w", id, err)
	}

	return record, nil
}

func writeFailure(doing string, err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
		return fmt.Errorf("%s: %w", doing, svc.ErrDuplicateName)
	}

	return fmt.Errorf("%s: %w", doing, err)
}

func scope(ownerID string) string { return collection + "\x00" + ownerID }

func fromRecord(record *core.Record) (dtag.Tag, error) {
	return dtag.Tag{
		ID:        record.Id,
		OwnerID:   record.GetString(fieldOwner),
		Name:      record.GetString(fieldName),
		Color:     record.GetString(fieldColor),
		CreatedAt: record.GetDateTime("created").Time().UTC(),
		UpdatedAt: record.GetDateTime("updated").Time().UTC(),
		Version:   store.Version(record),
	}, nil
}

func toRecord(record *core.Record, t dtag.Tag) {
	record.Set(fieldOwner, t.OwnerID)
	record.Set(fieldName, t.Name)
	record.Set(fieldColor, t.Color)
}
