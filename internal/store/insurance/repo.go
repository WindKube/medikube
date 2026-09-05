package insurance

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/insurance"
	"medikube/internal/store"
)

// Repo is the insurance repository the service declares, against PocketBase.
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
			kind.Insurance, strings.Join(missing, " and "))
	}

	return &Repo{app: app, cursors: cursors, schema: Schema()}, nil
}

func (r *Repo) List(ctx context.Context, patientID string, query service.Query) (domain.Page[clinical.Insurance], error) {
	var empty domain.Page[clinical.Insurance]

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

	if queryErr := built.Apply(r.app.RecordQuery(kind.Insurance.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.Insurance, queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]clinical.Insurance, 0, len(records))

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
	conditions := make([]store.Condition, 0, 5)
	conditions = append(conditions, store.Equal(ColumnPatient, patientID))

	if query.Search != "" {
		conditions = append(conditions, store.Contains(ColumnCompany, query.Search))
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

	if query.IsPrimary != nil {
		value := "false"
		if *query.IsPrimary {
			value = "true"
		}

		conditions = append(conditions, store.Equal(ColumnIsPrimary, value))
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(kind.Insurance.Collection()).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.Insurance, rowErr)
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

func (r *Repo) Get(ctx context.Context, id string) (clinical.Insurance, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.Insurance{}, err
	}

	return FromRecord(record)
}

func (r *Repo) Create(ctx context.Context, entity clinical.Insurance) (clinical.Insurance, *service.Displaced, error) {
	var created clinical.Insurance

	var displaced *service.Displaced

	write := func(txApp core.App) error {
		collection, err := r.collection(txApp)
		if err != nil {
			return err
		}

		record := core.NewRecord(collection)

		if mapErr := ToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if entity.IsPrimary {
			found, dispErr := r.displacePrimary(ctx, txApp, entity.PatientID, "")
			if dispErr != nil {
				return dispErr
			}

			displaced = found
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("creating a %s: %w", kind.Insurance, saveErr)
		}

		mapped, mapErr := FromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		created = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.Insurance{}, nil, txErr
	}

	return created, displaced, nil
}

func (r *Repo) Update(
	ctx context.Context,
	entity clinical.Insurance,
	expectedVersion string,
) (clinical.Insurance, *service.Displaced, error) {
	var updated clinical.Insurance

	var displaced *service.Displaced

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if entity.IsPrimary {
			found, dispErr := r.displacePrimary(ctx, txApp, entity.PatientID, entity.ID)
			if dispErr != nil {
				return dispErr
			}

			displaced = found
		}

		if mapErr := ToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.Insurance, entity.ID, saveErr)
		}

		mapped, mapErr := FromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.Insurance{}, nil, txErr
	}

	return updated, displaced, nil
}

// displacePrimary unsets is_primary on every other policy of this patient,
// inside the caller's transaction, and reports the one it displaced (FR-045).
// PocketBase's partial unique index (uniq_insurances_primary) is what makes
// this safe under a concurrent write: two displacements racing each other
// still leave at most one primary row, whichever commits second failing the
// index rather than the invariant silently breaking.
func (r *Repo) displacePrimary(ctx context.Context, app core.App, patientID, excludeID string) (*service.Displaced, error) {
	built, err := r.schema.Build(store.Query{
		Conditions: []store.Condition{
			store.Equal(ColumnPatient, patientID),
			store.Equal(ColumnIsPrimary, "true"),
		},
		Limit: 10,
	})
	if err != nil {
		return nil, err
	}

	var records []*core.Record

	if queryErr := built.Apply(app.RecordQuery(kind.Insurance.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("finding the current primary %s: %w", kind.Insurance, queryErr)
	}

	var displaced *service.Displaced

	for _, record := range records {
		if record.Id == excludeID {
			continue
		}

		record.Set(fieldIsPrimary, false)

		if saveErr := app.SaveWithContext(ctx, record); saveErr != nil {
			return nil, fmt.Errorf("displacing the previous primary %s: %w", kind.Insurance, saveErr)
		}

		displaced = &service.Displaced{ID: record.Id}
	}

	return displaced, nil
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
			return fmt.Errorf("deleting %s %s: %w", kind.Insurance, id, deleteErr)
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

	if queryErr := built.Apply(app.RecordQuery(kind.Insurance.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.Insurance, id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.Insurance, id, domain.ErrNotFound)
	}

	return records[0], nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	collection, err := app.FindCachedCollectionByNameOrId(kind.Insurance.Collection())
	if err != nil {
		return nil, fmt.Errorf("reading the %s collection: %w", kind.Insurance, err)
	}

	return collection, nil
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("%s %s: %w", kind.Insurance, id, domain.ErrVersionMismatch)
}

func scope(patientID string) string {
	return kind.Insurance.Collection() + "\x00" + patientID
}
