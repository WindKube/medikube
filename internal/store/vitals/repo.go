package vitals

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/vitals"
	"medikube/internal/store"
)

// Repo is the vitals repository the service declares, against PocketBase.
type Repo struct {
	app     core.App
	cursors *store.CursorCodec
	schema  store.Schema
}

var _ service.Repository = (*Repo)(nil)

func New(app core.App, cursors *store.CursorCodec) (*Repo, error) {
	var missing []string

	if app == nil {
		missing = append(missing, "application")
	}

	if cursors == nil {
		missing = append(missing, "cursor codec")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("%s: the repository is wired with no %s",
			kind.Vitals, strings.Join(missing, " and "))
	}

	return &Repo{app: app, cursors: cursors, schema: Schema()}, nil
}

func (r *Repo) List(ctx context.Context, patientID string, query service.Query) (domain.Page[clinical.Vitals], error) {
	var empty domain.Page[clinical.Vitals]

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = service.Sorts()[:1]
	}

	conditions := []store.Condition{store.Equal(ColumnPatient, patientID)}
	listing := store.Query{Conditions: conditions, Sort: sortKeys, Limit: query.Limit}

	if query.Cursor != "" {
		after, err := r.cursors.Decode(scope(patientID), sortKeys, query.Cursor)
		if err != nil {
			return empty, err
		}

		listing.After = after
	}

	built, err := r.schema.Build(listing)
	if err != nil {
		return empty, err
	}

	size := built.Limit
	built.Limit = size + 1

	var rows []*core.Record

	if queryErr := built.Apply(r.app.RecordQuery(kind.Vitals.Collection())).
		WithContext(ctx).All(&rows); queryErr != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.Vitals, queryErr)
	}

	more := len(rows) > size
	if more {
		rows = rows[:size]
	}

	items := make([]clinical.Vitals, 0, len(rows))

	for _, row := range rows {
		entity, mapErr := FromRecord(row)
		if mapErr != nil {
			return empty, mapErr
		}

		items = append(items, entity)
	}

	var next *string

	if more {
		token, cursorErr := r.boundary(patientID, rows[len(rows)-1], sortKeys)
		if cursorErr != nil {
			return empty, cursorErr
		}

		next = &token
	}

	page := domain.NewPage(items, next)

	if query.Count {
		total, countErr := r.count(ctx, conditions)
		if countErr != nil {
			return empty, countErr
		}

		page = page.WithTotal(total)
	}

	return page, nil
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(kind.Vitals.Collection()).Select("count(*)")
	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int
	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.Vitals, rowErr)
	}

	return total, nil
}

func (r *Repo) boundary(patientID string, record *core.Record, sortKeys []domain.SortKey) (string, error) {
	cursor, err := r.schema.Boundary(record, sortKeys)
	if err != nil {
		return "", err
	}

	return r.cursors.Encode(scope(patientID), cursor)
}

func (r *Repo) Get(ctx context.Context, id string) (clinical.Vitals, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.Vitals{}, err
	}

	return FromRecord(record)
}

func (r *Repo) Create(ctx context.Context, entity clinical.Vitals) (clinical.Vitals, error) {
	collection, err := r.app.FindCachedCollectionByNameOrId(kind.Vitals.Collection())
	if err != nil {
		return clinical.Vitals{}, fmt.Errorf("reading the %s collection: %w", kind.Vitals, err)
	}

	record := core.NewRecord(collection)
	if mapErr := ToRecord(record, entity); mapErr != nil {
		return clinical.Vitals{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return clinical.Vitals{}, fmt.Errorf("creating a %s: %w", kind.Vitals, saveErr)
	}

	return FromRecord(record)
}

func (r *Repo) Update(ctx context.Context, entity clinical.Vitals, expectedVersion string) (clinical.Vitals, error) {
	var updated clinical.Vitals

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if store.Version(record) != expectedVersion {
			return fmt.Errorf("%s %s: %w", kind.Vitals, entity.ID, domain.ErrVersionMismatch)
		}

		if mapErr := ToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.Vitals, entity.ID, saveErr)
		}

		mapped, mapErr := FromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.Vitals{}, txErr
	}

	return updated, nil
}

func (r *Repo) Delete(ctx context.Context, id, expectedVersion string) error {
	return store.RunInTransaction(r.app, func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, id)
		if err != nil {
			return err
		}

		if store.Version(record) != expectedVersion {
			return fmt.Errorf("%s %s: %w", kind.Vitals, id, domain.ErrVersionMismatch)
		}

		if deleteErr := txApp.DeleteWithContext(ctx, record); deleteErr != nil {
			return fmt.Errorf("deleting %s %s: %w", kind.Vitals, id, deleteErr)
		}

		return nil
	})
}

func (r *Repo) byID(ctx context.Context, app core.App, id string) (*core.Record, error) {
	built, err := r.schema.Build(store.Query{
		Conditions: []store.Condition{store.Equal(store.ColumnID, id)},
		Limit:      1,
	})
	if err != nil {
		return nil, err
	}

	var rows []*core.Record

	if queryErr := built.Apply(app.RecordQuery(kind.Vitals.Collection())).
		WithContext(ctx).All(&rows); queryErr != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.Vitals, id, queryErr)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.Vitals, id, domain.ErrNotFound)
	}

	return rows[0], nil
}

func scope(patientID string) string {
	return kind.Vitals.Collection() + "\x00" + patientID
}
