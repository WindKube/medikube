package emergencycontact

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/emergencycontact"
	"medikube/internal/store"
)

// Repo is the emergency contact repository service.Repository declares.
// Create and Update apply FR-045/FR-051's primary displacement inside the
// same transaction as the write itself (research D-16): the previous
// primary is cleared before the new one is saved, or the partial unique
// index would refuse it.
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
		return nil, fmt.Errorf("emergencycontact: the repository is wired with no %s", missing[0])
	}

	return &Repo{app: app, cursors: cursors, schema: contactSchema()}, nil
}

func (r *Repo) List(ctx context.Context, patientID string, query service.Query) (domain.Page[clinical.EmergencyContact], error) {
	var empty domain.Page[clinical.EmergencyContact]

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = service.Sorts()
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

	if queryErr := built.Apply(r.app.RecordQuery(kind.EmergencyContact.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.EmergencyContact, queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]clinical.EmergencyContact, 0, len(records))

	for _, record := range records {
		entity, mapErr := recordFromContact(record)
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
	conditions := make([]store.Condition, 0, 3)
	conditions = append(conditions, store.Equal(fieldPatient, patientID))

	if query.Search != "" {
		conditions = append(conditions, store.Contains(fieldName, query.Search))
	}

	if query.IsActive != nil {
		conditions = append(conditions, store.Equal(fieldIsActive, boolString(*query.IsActive)))
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(kind.EmergencyContact.Collection()).Select("count(*)")
	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.EmergencyContact, rowErr)
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

func (r *Repo) Get(ctx context.Context, id string) (clinical.EmergencyContact, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.EmergencyContact{}, err
	}

	return recordFromContact(record)
}

func (r *Repo) Create(ctx context.Context, entity clinical.EmergencyContact) (clinical.EmergencyContact, error) {
	var created clinical.EmergencyContact

	write := func(txApp core.App) error {
		collection, err := r.collection(txApp)
		if err != nil {
			return err
		}

		record := core.NewRecord(collection)
		if mapErr := contactToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		var displacedID string

		if entity.IsPrimary {
			displacedID, err = r.clearPrimary(ctx, txApp, entity.PatientID, "")
			if err != nil {
				return err
			}
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("creating a %s: %w", kind.EmergencyContact, saveErr)
		}

		mapped, mapErr := recordFromContact(record)
		if mapErr != nil {
			return mapErr
		}

		mapped.DisplacedID = displacedID
		created = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.EmergencyContact{}, txErr
	}

	return created, nil
}

func (r *Repo) Update(ctx context.Context, entity clinical.EmergencyContact, expectedVersion string) (clinical.EmergencyContact, error) {
	var updated clinical.EmergencyContact

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		wasPrimary := record.GetBool(fieldIsPrimary)

		var displacedID string

		if entity.IsPrimary && !wasPrimary {
			displacedID, err = r.clearPrimary(ctx, txApp, entity.PatientID, entity.ID)
			if err != nil {
				return err
			}
		}

		if mapErr := contactToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.EmergencyContact, entity.ID, saveErr)
		}

		mapped, mapErr := recordFromContact(record)
		if mapErr != nil {
			return mapErr
		}

		mapped.DisplacedID = displacedID
		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.EmergencyContact{}, txErr
	}

	return updated, nil
}

// clearPrimary unsets the previously primary contact of the same patient,
// other than excludeID, and reports its id. It must run before the new
// primary is saved: uniq_contacts_primary allows exactly one row with
// is_primary = 1 per patient, and saving the new one first would refuse.
func (r *Repo) clearPrimary(ctx context.Context, app core.App, patientID, excludeID string) (string, error) {
	var records []*core.Record

	where := dbx.HashExp{fieldPatient: patientID, fieldIsPrimary: true}

	query := app.RecordQuery(kind.EmergencyContact.Collection()).AndWhere(where)
	if excludeID != "" {
		query = query.AndWhere(dbx.Not(dbx.HashExp{"id": excludeID}))
	}

	if err := query.Limit(1).WithContext(ctx).All(&records); err != nil {
		return "", fmt.Errorf("emergencycontact: reading the previous primary: %w", err)
	}

	if len(records) == 0 {
		return "", nil
	}

	previous := records[0]
	previous.Set(fieldIsPrimary, false)

	if err := app.SaveWithContext(ctx, previous); err != nil {
		return "", fmt.Errorf("emergencycontact: displacing the previous primary: %w", err)
	}

	return previous.Id, nil
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
			return fmt.Errorf("deleting %s %s: %w", kind.EmergencyContact, id, deleteErr)
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

	if queryErr := built.Apply(app.RecordQuery(kind.EmergencyContact.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.EmergencyContact, id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.EmergencyContact, id, domain.ErrNotFound)
	}

	return records[0], nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	collection, err := app.FindCachedCollectionByNameOrId(kind.EmergencyContact.Collection())
	if err != nil {
		return nil, fmt.Errorf("reading the %s collection: %w", kind.EmergencyContact, err)
	}

	return collection, nil
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("%s %s: %w", kind.EmergencyContact, id, domain.ErrVersionMismatch)
}

func scope(patientID string) string {
	return kind.EmergencyContact.Collection() + "\x00" + patientID
}
