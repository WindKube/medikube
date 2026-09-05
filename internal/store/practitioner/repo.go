package practitioner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/practitioner"
	"medikube/internal/store"
)

// collection is the practitioners collection's name. It is not a kind.Kind:
// the directory is not a clinical record kind, it is what a record's
// practitioner field points at (internal/store/migrations/1756200200_practitioners.go).
const collection = "practitioners"

// facilitiesCollection is queried directly by id and owner, never through
// internal/store/facility — that package is another kind's repository and
// depending on it would make two directory kinds depend on each other for no
// reason beyond a shared table name.
const facilitiesCollection = "facilities"

const (
	fieldOwner     = "owner"
	fieldName      = "name"
	fieldSpecialty = "specialty"
	fieldFacility  = "facility"
	fieldPhone     = "phone"
	fieldEmail     = "email"
	fieldWebsite   = "website"
	fieldNotes     = "notes"
	fieldCreated   = "created"
	fieldUpdated   = "updated"
	fieldID        = "id"
)

// Repo is the practitioner repository service.Repository declares, against
// PocketBase.
//
// It is owner-scoped in the predicate rather than in a check afterwards,
// exactly as internal/store/medication is: the service authorizes above this
// and this refuses a row that is not the owner's anyway.
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
		return nil, fmt.Errorf("practitioner: the repository is wired with no %s", strings.Join(missing, " and "))
	}

	return &Repo{app: app, cursors: cursors, schema: schema()}, nil
}

// schema is the directory's query surface: the owner (narrowable), the name
// (searched and ordered, folded through LOWER() to match
// idx_practitioners_owner_name_specialty), the specialty and the facility
// (both narrowable, neither orderable — the contract publishes one ordering).
func schema() store.Schema {
	return store.NewSchema(collection,
		store.Column{Name: fieldOwner},
		store.Column{
			Name: fieldName,
			Expr: "LOWER(" + quoteColumn(fieldName) + ")",
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldName))
			},
			Searchable: true,
		},
		store.Column{Name: fieldSpecialty},
		store.Column{Name: fieldFacility},
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

// List returns one page of the owner's directory.
func (r *Repo) List(ctx context.Context, ownerID string, query service.Query) (domain.Page[directory.Practitioner], error) {
	var empty domain.Page[directory.Practitioner]

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = service.Sorts()
	}

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

	if queryErr := built.Apply(r.app.RecordQuery(collection)).WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing practitioners: %w", queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]directory.Practitioner, 0, len(records))

	for _, record := range records {
		entity, mapErr := fromRecord(record)
		if mapErr != nil {
			return empty, mapErr
		}

		items = append(items, entity)
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

func (r *Repo) narrowing(ownerID string, query service.Query) []store.Condition {
	conditions := make([]store.Condition, 0, 4)
	conditions = append(conditions, store.Equal(fieldOwner, ownerID))

	if query.Search != "" {
		conditions = append(conditions, store.Contains(fieldName, query.Search))
	}

	if query.Specialty != "" {
		conditions = append(conditions, store.Equal(fieldSpecialty, string(query.Specialty)))
	}

	if query.FacilityID != "" {
		conditions = append(conditions, store.Equal(fieldFacility, query.FacilityID))
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(collection).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting practitioners: %w", rowErr)
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

func (r *Repo) Get(ctx context.Context, ownerID, id string) (directory.Practitioner, error) {
	record, err := r.owned(ctx, r.app, ownerID, id)
	if err != nil {
		return directory.Practitioner{}, err
	}

	return fromRecord(record)
}

// Owner answers the account a row belongs to, or domain.ErrNotFound for a row
// that does not exist. It is the one lookup the service makes purely to tell
// a cross-owner access attempt apart from a genuine miss for auditing — the
// CRUD methods below independently scope by owner regardless of what this
// answers.
func (r *Repo) Owner(ctx context.Context, id string) (string, error) {
	var owned struct {
		Owner string `db:"owner"`
	}

	err := r.app.RecordQuery(collection).
		Select(fieldOwner).
		AndWhere(dbx.HashExp{fieldID: id}).
		Limit(1).
		WithContext(ctx).
		One(&owned)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("reading the owner of practitioner %s: %w", id, domain.ErrNotFound)
		}

		return "", fmt.Errorf("reading the owner of practitioner %s: %w", id, err)
	}

	return owned.Owner, nil
}

// Usage is FR-040's two indexed COUNT(*)s: patients naming this practitioner
// as their primary one, and medications naming them as the prescriber.
//
// The medications side is guarded rather than assumed: at the time this
// package was written, patients.primary_practitioner existed but
// medications.practitioner (added by 1756200600_medications_repoint.go) did
// not yet in every worktree. Guarding the column's absence keeps Usage
// answering zero for that half instead of failing the whole call.
func (r *Repo) Usage(ctx context.Context, ownerID, id string) (service.Usage, error) {
	if _, err := r.owned(ctx, r.app, ownerID, id); err != nil {
		return service.Usage{}, err
	}

	patients, err := r.countReferencing(ctx, "patients", "primary_practitioner", id)
	if err != nil {
		return service.Usage{}, err
	}

	records, err := r.countReferencing(ctx, kind.Medication.Collection(), "practitioner", id)
	if err != nil {
		return service.Usage{}, err
	}

	return service.Usage{Patients: patients, Records: records}, nil
}

// countReferencing is one indexed COUNT(*) against another collection's
// relation column. A collection or column that does not exist yet answers
// zero rather than an error: research on this phase's migrations found
// medications.practitioner landing in a later migration than this package's
// own, and a usage count that panicked on a schema not yet fully rolled out
// would be worse than an undercount that self-corrects the moment it lands.
func (r *Repo) countReferencing(ctx context.Context, table, column, id string) (int, error) {
	target, err := r.app.FindCollectionByNameOrId(table)
	if err != nil {
		return 0, nil //nolint:nilerr // the collection is not present yet; see the doc comment above
	}

	if target.Fields.GetByName(column) == nil {
		return 0, nil
	}

	var total int

	rowErr := r.app.RecordQuery(table).
		Select("count(*)").
		AndWhere(dbx.HashExp{column: id}).
		WithContext(ctx).
		Row(&total)
	if rowErr != nil {
		return 0, fmt.Errorf("counting %s referencing practitioner %s: %w", table, id, rowErr)
	}

	return total, nil
}

// Create mints the identity, the timestamps and the version. A facility named
// on the draft that is not this owner's is refused as domain.ErrNotFound
// (FR-042); a duplicate (owner, LOWER(name), specialty) is refused as
// domain.ErrConflict (FR-038).
func (r *Repo) Create(ctx context.Context, entity directory.Practitioner) (directory.Practitioner, error) {
	if err := r.checkFacility(ctx, entity.OwnerID, entity.FacilityID); err != nil {
		return directory.Practitioner{}, err
	}

	coll, err := r.collection(r.app)
	if err != nil {
		return directory.Practitioner{}, err
	}

	record := core.NewRecord(coll)

	if mapErr := toRecord(record, entity); mapErr != nil {
		return directory.Practitioner{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return directory.Practitioner{}, writeFailure("creating a practitioner", saveErr)
	}

	return fromRecord(record)
}

// Update writes the entity over the row it identifies, within its owner, and
// only while the stored version is still expectedVersion.
func (r *Repo) Update(ctx context.Context, entity directory.Practitioner, expectedVersion string) (directory.Practitioner, error) {
	if err := r.checkFacility(ctx, entity.OwnerID, entity.FacilityID); err != nil {
		return directory.Practitioner{}, err
	}

	var updated directory.Practitioner

	write := func(txApp core.App) error {
		record, err := r.owned(ctx, txApp, entity.OwnerID, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if mapErr := toRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return writeFailure(fmt.Sprintf("updating practitioner %s", entity.ID), saveErr)
		}

		mapped, mapErr := fromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return directory.Practitioner{}, txErr
	}

	return updated, nil
}

// Delete is permanent. Every referencing record survives with the reference
// cleared — PocketBase's own deleteRefRecords behaviour for a non-cascading,
// non-required relation (research D-06) — which is why this does nothing
// beyond deleting the row itself.
func (r *Repo) Delete(ctx context.Context, ownerID, id, expectedVersion string) error {
	return store.RunInTransaction(r.app, func(txApp core.App) error {
		record, err := r.owned(ctx, txApp, ownerID, id)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, id, expectedVersion); versionErr != nil {
			return versionErr
		}

		if deleteErr := txApp.DeleteWithContext(ctx, record); deleteErr != nil {
			return fmt.Errorf("deleting practitioner %s: %w", id, deleteErr)
		}

		return nil
	})
}

// checkFacility is FR-042: a facility named on the draft has to belong to the
// same owner, checked with a plain query against the facilities collection —
// not internal/store/facility, which is another kind's repository package.
func (r *Repo) checkFacility(ctx context.Context, ownerID, facilityID string) error {
	if facilityID == "" {
		return nil
	}

	var found struct {
		ID string `db:"id"`
	}

	err := r.app.RecordQuery(facilitiesCollection).
		Select(fieldID).
		AndWhere(dbx.HashExp{fieldID: facilityID, fieldOwner: ownerID}).
		Limit(1).
		WithContext(ctx).
		One(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("facility %s: %w", facilityID, domain.ErrNotFound)
		}

		return fmt.Errorf("checking facility %s: %w", facilityID, err)
	}

	return nil
}

func (r *Repo) owned(ctx context.Context, app core.App, ownerID, id string) (*core.Record, error) {
	built, err := r.schema.Build(store.Query{
		Conditions: []store.Condition{
			store.Equal(fieldOwner, ownerID),
			store.Equal(fieldID, id),
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	var records []*core.Record

	if queryErr := built.Apply(app.RecordQuery(collection)).WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("reading practitioner %s: %w", id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("practitioner %s: %w", id, domain.ErrNotFound)
	}

	return records[0], nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	coll, err := app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		return nil, fmt.Errorf("reading the practitioners collection: %w", err)
	}

	return coll, nil
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("practitioner %s: %w", id, domain.ErrVersionMismatch)
}

// writeFailure maps a unique-index violation onto domain.ErrConflict, the same
// convention internal/store/identity's writeFailure follows: the index is the
// enforcement (idx_practitioners_owner_name_specialty) and this is only the
// translation, or FR-038's second create reaches the client as a 500.
func writeFailure(doing string, err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
		return fmt.Errorf("%s: this owner already has a practitioner under that name and specialty: %w",
			doing, domain.ErrConflict)
	}

	return fmt.Errorf("%s: %w", doing, err)
}

func scope(ownerID string) string {
	return collection + "\x00" + ownerID
}

var (
	// ErrUnexpectedCollection mirrors internal/store's own: a record handed to
	// the wrong mapper, where every getter would otherwise answer a zero value
	// and the result would look like an empty entity.
	ErrUnexpectedCollection = errors.New("practitioner: the record is not from the practitioners collection")
)

func fromRecord(record *core.Record) (directory.Practitioner, error) {
	if err := expectCollection(record); err != nil {
		return directory.Practitioner{}, err
	}

	return directory.Practitioner{
		ID:         record.Id,
		OwnerID:    record.GetString(fieldOwner),
		Name:       record.GetString(fieldName),
		Specialty:  directory.Specialty(record.GetString(fieldSpecialty)),
		FacilityID: record.GetString(fieldFacility),
		Phone:      record.GetString(fieldPhone),
		Email:      record.GetString(fieldEmail),
		Website:    record.GetString(fieldWebsite),
		Notes:      record.GetString(fieldNotes),
		CreatedAt:  recordInstant(record, fieldCreated),
		UpdatedAt:  recordInstant(record, fieldUpdated),
		Version:    store.Version(record),
	}, nil
}

func toRecord(record *core.Record, entity directory.Practitioner) error {
	if err := expectCollection(record); err != nil {
		return err
	}

	record.Set(fieldOwner, entity.OwnerID)
	record.Set(fieldName, entity.Name)
	record.Set(fieldSpecialty, string(entity.Specialty))
	record.Set(fieldFacility, entity.FacilityID)
	record.Set(fieldPhone, entity.Phone)
	record.Set(fieldEmail, entity.Email)
	record.Set(fieldWebsite, entity.Website)
	record.Set(fieldNotes, entity.Notes)

	return nil
}

// recordInstant reads a stored instant in UTC, truncated to the millisecond
// precision the column actually holds — otherwise a record just saved carries
// the full-precision time.Time it was stamped with in memory, and a re-read
// of the same row a moment later answers with a different instant (mirrors
// internal/store's own recordInstant).
func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}

func expectCollection(record *core.Record) error {
	coll := record.Collection()
	if coll == nil || coll.Name != collection {
		return fmt.Errorf("%w", ErrUnexpectedCollection)
	}

	return nil
}
