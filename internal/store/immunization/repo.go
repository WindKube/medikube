package immunization

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/immunization"
	"medikube/internal/store"
)

// Repo is the immunization repository the service declares, against
// PocketBase, mirroring internal/store/medication/repo.go's shape exactly.
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
		return nil, fmt.Errorf("%s: the repository is wired with no %s", kind.Immunization, joinWords(missing))
	}

	return &Repo{app: app, cursors: cursors, schema: immunizationSchema()}, nil
}

func (r *Repo) List(ctx context.Context, patientID string, query service.Query) (domain.Page[clinical.Immunization], error) {
	var empty domain.Page[clinical.Immunization]

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

	if queryErr := built.Apply(r.app.RecordQuery(kind.Immunization.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.Immunization, queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]clinical.Immunization, 0, len(records))

	for _, record := range records {
		entity, mapErr := recordFromImmunization(record)
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
	conditions := make([]store.Condition, 0, 2)
	conditions = append(conditions, store.Equal(fieldPatient, patientID))

	if query.Search != "" {
		conditions = append(conditions, store.ContainsAny(query.Search, fieldVaccineName, fieldTradeName))
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(kind.Immunization.Collection()).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.Immunization, rowErr)
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

func (r *Repo) Get(ctx context.Context, id string) (clinical.Immunization, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.Immunization{}, err
	}

	return recordFromImmunization(record)
}

func (r *Repo) Create(ctx context.Context, entity clinical.Immunization) (clinical.Immunization, error) {
	collection, err := r.collection(r.app)
	if err != nil {
		return clinical.Immunization{}, err
	}

	record := core.NewRecord(collection)

	if mapErr := immunizationToRecord(record, entity); mapErr != nil {
		return clinical.Immunization{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return clinical.Immunization{}, fmt.Errorf("creating a %s: %w", kind.Immunization, saveErr)
	}

	return recordFromImmunization(record)
}

func (r *Repo) Update(ctx context.Context, entity clinical.Immunization, expectedVersion string) (clinical.Immunization, error) {
	var updated clinical.Immunization

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if mapErr := immunizationToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.Immunization, entity.ID, saveErr)
		}

		mapped, mapErr := recordFromImmunization(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.Immunization{}, txErr
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
			return fmt.Errorf("deleting %s %s: %w", kind.Immunization, id, deleteErr)
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

	if queryErr := built.Apply(app.RecordQuery(kind.Immunization.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.Immunization, id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.Immunization, id, domain.ErrNotFound)
	}

	return records[0], nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	collection, err := app.FindCachedCollectionByNameOrId(kind.Immunization.Collection())
	if err != nil {
		return nil, fmt.Errorf("reading the %s collection: %w", kind.Immunization, err)
	}

	return collection, nil
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("%s %s: %w", kind.Immunization, id, domain.ErrVersionMismatch)
}

func scope(patientID string) string {
	return kind.Immunization.Collection() + "\x00" + patientID
}

func joinWords(words []string) string {
	switch len(words) {
	case 1:
		return words[0]
	case 2:
		return words[0] + " and no " + words[1]
	default:
		return words[0] + ", no " + joinWords(words[1:])
	}
}
