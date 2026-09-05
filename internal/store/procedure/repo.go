// Package procedure is the procedure repository against PocketBase, mirroring
// internal/store/medication.
package procedure

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/procedure"
	"medikube/internal/store"
)

type Repo struct {
	app     core.App
	cursors *store.CursorCodec
	schema  store.Schema
}

var _ service.Repository = (*Repo)(nil)

func New(app core.App, cursors *store.CursorCodec) (*Repo, error) {
	if app == nil || cursors == nil {
		return nil, fmt.Errorf("%s: the repository is wired with no application or cursor codec", kind.Procedure)
	}

	return &Repo{app: app, cursors: cursors, schema: store.ProcedureSchema()}, nil
}

func (r *Repo) List(ctx context.Context, patientID string, query service.Query) (domain.Page[clinical.Procedure], error) {
	var empty domain.Page[clinical.Procedure]

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

	if err := built.Apply(r.app.RecordQuery(kind.Procedure.Collection())).WithContext(ctx).All(&records); err != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.Procedure, err)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]clinical.Procedure, 0, len(records))

	for _, record := range records {
		entity, err := store.ProcedureFromRecord(record)
		if err != nil {
			return empty, err
		}

		items = append(items, entity)
	}

	var next *string

	if more {
		token, err := r.boundary(patientID, records[len(records)-1], sortKeys)
		if err != nil {
			return empty, err
		}

		next = &token
	}

	page := domain.NewPage(items, next)

	if query.Count {
		total, err := r.count(ctx, conditions)
		if err != nil {
			return empty, err
		}

		page = page.WithTotal(total)
	}

	return page, nil
}

func (r *Repo) narrowing(patientID string, query service.Query) []store.Condition {
	conditions := make([]store.Condition, 0, 3)
	conditions = append(conditions, store.Equal(store.ProcedurePatient, patientID))

	statuses := query.Statuses

	if query.Scheduled != nil {
		scheduled := []clinical.OrderStatus{clinical.OrderStatusOrdered, clinical.OrderStatusScheduled}

		if *query.Scheduled {
			statuses = scheduled
		} else if len(statuses) == 0 {
			for _, s := range clinical.OrderStatuses() {
				if s != clinical.OrderStatusOrdered && s != clinical.OrderStatusScheduled {
					statuses = append(statuses, s)
				}
			}
		}
	}

	if len(statuses) > 0 {
		values := make([]string, 0, len(statuses))
		for _, v := range statuses {
			values = append(values, string(v))
		}

		conditions = append(conditions, store.OneOf(store.ProcedureStatus, values...))
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(kind.Procedure.Collection()).Select("count(*)")
	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int
	if err := counting.WithContext(ctx).Row(&total); err != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.Procedure, err)
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

func (r *Repo) Get(ctx context.Context, id string) (clinical.Procedure, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.Procedure{}, err
	}

	return store.ProcedureFromRecord(record)
}

func (r *Repo) Create(ctx context.Context, entity clinical.Procedure) (clinical.Procedure, error) {
	collection, err := r.app.FindCachedCollectionByNameOrId(kind.Procedure.Collection())
	if err != nil {
		return clinical.Procedure{}, fmt.Errorf("reading the %s collection: %w", kind.Procedure, err)
	}

	record := core.NewRecord(collection)
	if err := store.ProcedureToRecord(record, entity); err != nil {
		return clinical.Procedure{}, err
	}

	if err := r.app.SaveWithContext(ctx, record); err != nil {
		return clinical.Procedure{}, fmt.Errorf("creating a %s: %w", kind.Procedure, err)
	}

	return store.ProcedureFromRecord(record)
}

func (r *Repo) Update(ctx context.Context, entity clinical.Procedure, expectedVersion string) (clinical.Procedure, error) {
	var updated clinical.Procedure

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if mapErr := store.ProcedureToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.Procedure, entity.ID, saveErr)
		}

		mapped, err := store.ProcedureFromRecord(record)
		if err != nil {
			return err
		}

		updated = mapped

		return nil
	}

	if err := store.RunInTransaction(r.app, write); err != nil {
		return clinical.Procedure{}, err
	}

	return updated, nil
}

func (r *Repo) Delete(ctx context.Context, id, expectedVersion string) error {
	return store.RunInTransaction(r.app, func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, id)
		if err != nil {
			return err
		}

		if err := expectVersion(record, id, expectedVersion); err != nil {
			return err
		}

		if err := txApp.DeleteWithContext(ctx, record); err != nil {
			return fmt.Errorf("deleting %s %s: %w", kind.Procedure, id, err)
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

	var records []*core.Record

	if err := built.Apply(app.RecordQuery(kind.Procedure.Collection())).WithContext(ctx).All(&records); err != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.Procedure, id, err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.Procedure, id, domain.ErrNotFound)
	}

	return records[0], nil
}

func scope(patientID string) string {
	return kind.Procedure.Collection() + "\x00" + patientID
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("%s %s: %w", kind.Procedure, id, domain.ErrVersionMismatch)
}
