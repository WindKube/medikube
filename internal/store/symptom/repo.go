package symptom

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/symptom"
	"medikube/internal/store"
)

// Repo is the symptom repository the service declares, against PocketBase.
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
			kind.Symptom, strings.Join(missing, " and "))
	}

	return &Repo{app: app, cursors: cursors, schema: Schema()}, nil
}

func (r *Repo) List(ctx context.Context, patientID string, query service.Query) (domain.Page[clinical.Symptom], error) {
	var empty domain.Page[clinical.Symptom]

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

	var rows []*core.Record

	if queryErr := built.Apply(r.app.RecordQuery(kind.Symptom.Collection())).
		WithContext(ctx).All(&rows); queryErr != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.Symptom, queryErr)
	}

	more := len(rows) > size
	if more {
		rows = rows[:size]
	}

	aggregates, err := Aggregate(ctx, r.app, patientID)
	if err != nil {
		return empty, err
	}

	items := make([]clinical.Symptom, 0, len(rows))

	for _, row := range rows {
		entity, mapErr := FromRecord(row)
		if mapErr != nil {
			return empty, mapErr
		}

		items = append(items, withAggregate(entity, aggregates))
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

func (r *Repo) narrowing(patientID string, query service.Query) []store.Condition {
	conditions := []store.Condition{store.Equal(ColumnPatient, patientID)}

	if query.Search != "" {
		conditions = append(conditions, store.Contains(ColumnName, query.Search))
	}

	if query.Name != "" {
		conditions = append(conditions, store.Equal(ColumnName, query.Name))
	}

	if len(query.Severities) > 0 {
		conditions = append(conditions, store.OneOf(ColumnSeverity, stringsOf(query.Severities)...))
	}

	if len(query.Statuses) > 0 {
		conditions = append(conditions, store.OneOf(ColumnStatus, stringsOfStatus(query.Statuses)...))
	}

	if query.IsChronic != nil {
		value := "false"
		if *query.IsChronic {
			value = "true"
		}

		conditions = append(conditions, store.Equal(ColumnIsChronic, value))
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

	counting := r.app.RecordQuery(kind.Symptom.Collection()).Select("count(*)")
	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int
	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.Symptom, rowErr)
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

func (r *Repo) Get(ctx context.Context, id string) (clinical.Symptom, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.Symptom{}, err
	}

	entity, err := FromRecord(record)
	if err != nil {
		return clinical.Symptom{}, err
	}

	summary, err := AggregateOne(ctx, r.app, entity.PatientID, entity.Name)
	if err != nil {
		return clinical.Symptom{}, err
	}

	entity.EpisodeCount = summary.EpisodeCount
	entity.LastOccurredAt = summary.LastOccurredAt

	return entity, nil
}

func (r *Repo) Create(ctx context.Context, entity clinical.Symptom) (clinical.Symptom, error) {
	collection, err := r.app.FindCachedCollectionByNameOrId(kind.Symptom.Collection())
	if err != nil {
		return clinical.Symptom{}, fmt.Errorf("reading the %s collection: %w", kind.Symptom, err)
	}

	record := core.NewRecord(collection)
	if mapErr := ToRecord(record, entity); mapErr != nil {
		return clinical.Symptom{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return clinical.Symptom{}, fmt.Errorf("creating a %s: %w", kind.Symptom, saveErr)
	}

	return r.Get(ctx, record.Id)
}

func (r *Repo) Update(ctx context.Context, entity clinical.Symptom, expectedVersion string) (clinical.Symptom, error) {
	var updatedID string

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if store.Version(record) != expectedVersion {
			return fmt.Errorf("%s %s: %w", kind.Symptom, entity.ID, domain.ErrVersionMismatch)
		}

		if mapErr := ToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.Symptom, entity.ID, saveErr)
		}

		updatedID = record.Id

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.Symptom{}, txErr
	}

	return r.Get(ctx, updatedID)
}

func (r *Repo) Delete(ctx context.Context, id, expectedVersion string) error {
	return store.RunInTransaction(r.app, func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, id)
		if err != nil {
			return err
		}

		if store.Version(record) != expectedVersion {
			return fmt.Errorf("%s %s: %w", kind.Symptom, id, domain.ErrVersionMismatch)
		}

		if deleteErr := txApp.DeleteWithContext(ctx, record); deleteErr != nil {
			return fmt.Errorf("deleting %s %s: %w", kind.Symptom, id, deleteErr)
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

	if queryErr := built.Apply(app.RecordQuery(kind.Symptom.Collection())).
		WithContext(ctx).All(&rows); queryErr != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.Symptom, id, queryErr)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.Symptom, id, domain.ErrNotFound)
	}

	return rows[0], nil
}

func withAggregate(entity clinical.Symptom, aggregates map[string]clinical.Symptom) clinical.Symptom {
	if summary, ok := aggregates[strings.ToLower(entity.Name)]; ok {
		entity.EpisodeCount = summary.EpisodeCount
		entity.LastOccurredAt = summary.LastOccurredAt
	}

	return entity
}

func scope(patientID string) string {
	return kind.Symptom.Collection() + "\x00" + patientID
}

func stringsOf(values []clinical.Severity) []string {
	converted := make([]string, 0, len(values))
	for _, v := range values {
		converted = append(converted, string(v))
	}

	return converted
}

func stringsOfStatus(values []clinical.ConditionStatus) []string {
	converted := make([]string, 0, len(values))
	for _, v := range values {
		converted = append(converted, string(v))
	}

	return converted
}
