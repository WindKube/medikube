package facility

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
	service "medikube/internal/service/facility"
	"medikube/internal/store"
)

// Repo is the facility repository the service declares, against PocketBase.
//
// It is owner-scoped in the predicate rather than in a check afterwards. The
// service authorizes above this via Owner, and this refuses a row that is not
// the owner's anyway — two independent refusals, because either one of them is
// one edit away from not being there.
type Repo struct {
	app     core.App
	cursors *store.CursorCodec
	schema  store.Schema
}

var _ service.Repository = (*Repo)(nil)

// New wires the repository to an instance and to the codec that seals its
// cursors.
func New(app core.App, cursors *store.CursorCodec) (*Repo, error) {
	var missing []string

	if app == nil {
		missing = append(missing, "application")
	}

	if cursors == nil {
		missing = append(missing, "cursor codec")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("facility: the repository is wired with no %s", joinWords(missing))
	}

	return &Repo{app: app, cursors: cursors, schema: facilitySchema()}, nil
}

// List returns one page of the owner's facilities, ordered kind then name,
// with the identity as the tiebreaker, and mints the boundary for the next
// page.
func (r *Repo) List(ctx context.Context, ownerID string, query service.Query) (domain.Page[directory.Facility], error) {
	var empty domain.Page[directory.Facility]

	sortKeys := service.Sorts()

	conditions := r.narrowing(ownerID, query)

	listing := store.Query{Conditions: conditions, Sort: sortKeys, Limit: query.Limit}

	if query.Cursor != "" {
		after, err := r.cursors.Decode(scope(ownerID), sortKeys, query.Cursor)
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

	if queryErr := built.Apply(r.app.RecordQuery(collectionName)).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing facilities: %w", queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]directory.Facility, 0, len(records))
	for _, record := range records {
		items = append(items, recordFromFacility(record))
	}

	var next *string

	if more {
		token, cursorErr := r.boundary(ownerID, records[len(records)-1], sortKeys)
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

// narrowing is the query's terms, owner first. The search is the one term
// that spans more than one column (FR-036: name and brand), and the kind is a
// term of its own.
func (r *Repo) narrowing(ownerID string, query service.Query) []store.Condition {
	conditions := make([]store.Condition, 0, 3)
	conditions = append(conditions, store.Equal(fieldOwner, ownerID))

	if query.Search != "" {
		conditions = append(conditions, store.ContainsAny(query.Search, fieldName, fieldBrand))
	}

	if query.Kind != "" {
		conditions = append(conditions, store.Equal(fieldKind, string(query.Kind)))
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(collectionName).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting facilities: %w", rowErr)
	}

	return total, nil
}

func (r *Repo) boundary(ownerID string, record *core.Record, sortKeys []domain.SortKey) (string, error) {
	cursor, err := r.schema.Boundary(record, sortKeys)
	if err != nil {
		return "", err
	}

	return r.cursors.Encode(scope(ownerID), cursor)
}

func (r *Repo) Get(ctx context.Context, ownerID, id string) (directory.Facility, error) {
	record, err := r.owned(ctx, r.app, ownerID, id)
	if err != nil {
		return directory.Facility{}, err
	}

	return recordFromFacility(record), nil
}

// Create mints the identity, the timestamps and the version. Whatever the
// entity carried in those four is not read.
func (r *Repo) Create(ctx context.Context, entity directory.Facility) (directory.Facility, error) {
	collection, err := r.collection(r.app)
	if err != nil {
		return directory.Facility{}, err
	}

	record := core.NewRecord(collection)

	if mapErr := facilityToRecord(record, entity); mapErr != nil {
		return directory.Facility{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return directory.Facility{}, fmt.Errorf("creating a facility: %w", saveErr)
	}

	return recordFromFacility(record), nil
}

// Update writes the entity over the row it identifies, within its owner, and
// only while the stored version is still expectedVersion. The read and the
// write are one transaction, which is what makes the version check a check.
func (r *Repo) Update(ctx context.Context, entity directory.Facility, expectedVersion string) (directory.Facility, error) {
	var updated directory.Facility

	write := func(txApp core.App) error {
		record, err := r.owned(ctx, txApp, entity.OwnerID, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if mapErr := facilityToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating facility %s: %w", entity.ID, saveErr)
		}

		updated = recordFromFacility(record)

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return directory.Facility{}, txErr
	}

	return updated, nil
}

// Delete is permanent. References from practitioners.facility and
// medications.pharmacy are unset first, in the same transaction, rather than
// cascaded (research D-06): the practitioner and the medication both survive
// their facility being deleted.
func (r *Repo) Delete(ctx context.Context, ownerID, id, expectedVersion string) error {
	return store.RunInTransaction(r.app, func(txApp core.App) error {
		record, err := r.owned(ctx, txApp, ownerID, id)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, id, expectedVersion); versionErr != nil {
			return versionErr
		}

		if err := r.unsetReferences(ctx, txApp, id); err != nil {
			return err
		}

		if deleteErr := txApp.DeleteWithContext(ctx, record); deleteErr != nil {
			return fmt.Errorf("deleting facility %s: %w", id, deleteErr)
		}

		return nil
	})
}

// unsetReferences clears practitioners.facility and medications.pharmacy for
// every row pointing at id. It is a best-effort no-op for medications.pharmacy
// where that column does not exist yet — another agent's migration adds it —
// so this repository does not block on a schema it does not own.
func (r *Repo) unsetReferences(ctx context.Context, txApp core.App, id string) error {
	if err := unsetRelation(ctx, txApp, practitionersCollection, practitionerFieldFacility, id); err != nil {
		return err
	}

	if hasColumn(txApp, medicationsCollection, medicationFieldPharmacy) {
		if err := unsetRelation(ctx, txApp, medicationsCollection, medicationFieldPharmacy, id); err != nil {
			return err
		}
	}

	return nil
}

func unsetRelation(ctx context.Context, app core.App, collection, field, id string) error {
	var records []*core.Record

	if err := app.RecordQuery(collection).
		AndWhere(dbx.HashExp{field: id}).
		WithContext(ctx).All(&records); err != nil {
		return fmt.Errorf("finding %s referencing facility %s: %w", collection, id, err)
	}

	for _, record := range records {
		record.Set(field, "")

		if err := app.SaveWithContext(ctx, record); err != nil {
			return fmt.Errorf("unsetting %s.%s on %s: %w", collection, field, record.Id, err)
		}
	}

	return nil
}

func hasColumn(app core.App, collection, field string) bool {
	found, err := app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		return false
	}

	return found.Fields.GetByName(field) != nil
}

// Owner answers who owns id, or domain.ErrNotFound if it does not exist —
// without regard to which account is asking. The service uses this to tell a
// row that never existed apart from one that belongs to somebody else, which
// is what decides whether the attempt gets audited.
func (r *Repo) Owner(ctx context.Context, id string) (string, error) {
	var records []*core.Record

	if err := r.app.RecordQuery(collectionName).
		AndWhere(dbx.HashExp{"id": id}).
		WithContext(ctx).All(&records); err != nil {
		return "", fmt.Errorf("reading facility %s: %w", id, err)
	}

	if len(records) == 0 {
		return "", fmt.Errorf("facility %s: %w", id, domain.ErrNotFound)
	}

	return records[0].GetString(fieldOwner), nil
}

// Usage counts what points at id: practitioners whose facility this is, and
// medications whose pharmacy this is (0 while that column does not exist
// yet).
func (r *Repo) Usage(ctx context.Context, ownerID, id string) (service.Usage, error) {
	if _, err := r.owned(ctx, r.app, ownerID, id); err != nil {
		return service.Usage{}, err
	}

	practitioners, err := r.countReferences(ctx, practitionersCollection, practitionerFieldFacility, id)
	if err != nil {
		return service.Usage{}, err
	}

	var records int

	if hasColumn(r.app, medicationsCollection, medicationFieldPharmacy) {
		records, err = r.countReferences(ctx, medicationsCollection, medicationFieldPharmacy, id)
		if err != nil {
			return service.Usage{}, err
		}
	}

	return service.Usage{Practitioners: practitioners, Records: records}, nil
}

func (r *Repo) countReferences(ctx context.Context, collection, field, id string) (int, error) {
	var total int

	if err := r.app.RecordQuery(collection).
		Select("count(*)").
		AndWhere(dbx.HashExp{field: id}).
		WithContext(ctx).Row(&total); err != nil {
		return 0, fmt.Errorf("counting %s referencing facility %s: %w", collection, id, err)
	}

	return total, nil
}

// owned reads one row within its owner. A row that is not this owner's is
// answered exactly as one that never existed (FR-037).
func (r *Repo) owned(ctx context.Context, app core.App, ownerID, id string) (*core.Record, error) {
	built, err := r.schema.Build(store.Query{
		Conditions: []store.Condition{
			store.Equal(fieldOwner, ownerID),
			store.Equal("id", id),
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	var records []*core.Record

	if queryErr := built.Apply(app.RecordQuery(collectionName)).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("reading facility %s: %w", id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("facility %s: %w", id, domain.ErrNotFound)
	}

	return records[0], nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	collection, err := app.FindCachedCollectionByNameOrId(collectionName)
	if err != nil {
		return nil, fmt.Errorf("reading the facilities collection: %w", err)
	}

	return collection, nil
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("facility %s: %w", id, domain.ErrVersionMismatch)
}

// scope is the query a cursor continues: this collection, for this account.
func scope(ownerID string) string {
	return collectionName + "\x00" + ownerID
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
