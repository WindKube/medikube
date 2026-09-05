package equipment

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/equipment"
	"medikube/internal/store"
)

// Repo is the equipment repository the service declares, against PocketBase.
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
			kind.Equipment, strings.Join(missing, " and "))
	}

	return &Repo{app: app, cursors: cursors, schema: Schema()}, nil
}

func (r *Repo) List(ctx context.Context, patientID string, query service.Query) (domain.Page[clinical.Equipment], error) {
	var empty domain.Page[clinical.Equipment]

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = service.Sorts()[:1]
	}

	conditions := r.narrowing(patientID, query)

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

	var records []*core.Record

	if queryErr := built.Apply(r.app.RecordQuery(kind.Equipment.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.Equipment, queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]clinical.Equipment, 0, len(records))

	for _, record := range records {
		entity, mapErr := FromRecord(record)
		if mapErr != nil {
			return empty, mapErr
		}

		items = append(items, entity)
	}

	var next *string

	if more {
		token, cursorErr := r.boundary(patientID, records[len(records)-1], sortKeys)
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

func (r *Repo) narrowing(patientID string, query service.Query) []store.Condition {
	conditions := make([]store.Condition, 0, 4)
	conditions = append(conditions, store.Equal(ColumnPatient, patientID))

	if query.Search != "" {
		conditions = append(conditions, store.Contains(ColumnName, query.Search))
	}

	if len(query.Types) > 0 {
		values := make([]string, 0, len(query.Types))
		for _, t := range query.Types {
			values = append(values, string(t))
		}

		conditions = append(conditions, store.OneOf(ColumnType, values...))
	}

	if len(query.Statuses) > 0 {
		values := make([]string, 0, len(query.Statuses))
		for _, status := range query.Statuses {
			values = append(values, string(status))
		}

		conditions = append(conditions, store.OneOf(ColumnStatus, values...))
	}

	if len(query.Tags) > 0 {
		if query.Match == service.MatchAll {
			conditions = append(conditions, store.AllOf(fieldTags, query.Tags...))
		} else {
			conditions = append(conditions, store.AnyOf(fieldTags, query.Tags...))
		}
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(kind.Equipment.Collection()).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.Equipment, rowErr)
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

func (r *Repo) Get(ctx context.Context, id string) (clinical.Equipment, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.Equipment{}, err
	}

	return FromRecord(record)
}

func (r *Repo) Create(ctx context.Context, entity clinical.Equipment) (clinical.Equipment, error) {
	collection, err := r.collection(r.app)
	if err != nil {
		return clinical.Equipment{}, err
	}

	record := core.NewRecord(collection)

	if mapErr := ToRecord(record, entity); mapErr != nil {
		return clinical.Equipment{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return clinical.Equipment{}, fmt.Errorf("creating a %s: %w", kind.Equipment, saveErr)
	}

	return FromRecord(record)
}

func (r *Repo) Update(
	ctx context.Context,
	entity clinical.Equipment,
	expectedVersion string,
) (clinical.Equipment, error) {
	var updated clinical.Equipment

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if mapErr := ToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.Equipment, entity.ID, saveErr)
		}

		mapped, mapErr := FromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.Equipment{}, txErr
	}

	return updated, nil
}

func (r *Repo) Delete(ctx context.Context, id, expectedVersion string) error {
	return store.RunInTransaction(r.app, func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, id)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, id, expectedVersion); versionErr != nil {
			return versionErr
		}

		if deleteErr := txApp.DeleteWithContext(ctx, record); deleteErr != nil {
			return fmt.Errorf("deleting %s %s: %w", kind.Equipment, id, deleteErr)
		}

		return nil
	})
}

func (r *Repo) byID(ctx context.Context, app core.App, id string) (*core.Record, error) {
	built, err := r.schema.Build(store.Query{
		Conditions: []store.Condition{store.Equal(ColumnID, id)},
		Limit:      1,
	})
	if err != nil {
		return nil, err
	}

	var records []*core.Record

	if queryErr := built.Apply(app.RecordQuery(kind.Equipment.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.Equipment, id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.Equipment, id, domain.ErrNotFound)
	}

	return records[0], nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	collection, err := app.FindCachedCollectionByNameOrId(kind.Equipment.Collection())
	if err != nil {
		return nil, fmt.Errorf("reading the %s collection: %w", kind.Equipment, err)
	}

	return collection, nil
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("%s %s: %w", kind.Equipment, id, domain.ErrVersionMismatch)
}

func scope(patientID string) string {
	return kind.Equipment.Collection() + "\x00" + patientID
}
