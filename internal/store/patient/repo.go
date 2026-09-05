package patient

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/person"
	service "medikube/internal/service/patient"
	"medikube/internal/store"
)

// Repo is the PocketBase adapter for service.Repository.
//
// Owner-scoped in the predicate, mirroring internal/store/medication: the
// service authorizes above this and this refuses a row that is not the
// owner's anyway.
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
		return nil, fmt.Errorf("patient: the repository is wired with no %s", joinWords(missing))
	}

	return &Repo{app: app, cursors: cursors, schema: store.PatientsSchema()}, nil
}

// scope is the cursor's authenticated binding: this resource and this owner,
// so a cursor issued for one account's patients cannot decode against
// another's (mirrors internal/web.CursorScope).
func scope(ownerID string) string {
	return "patients\x00" + ownerID
}

func (r *Repo) List(ctx context.Context, ownerID string, query service.Query) (domain.Page[person.Patient], error) {
	var empty domain.Page[person.Patient]

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = []domain.SortKey{{Field: "last_name"}, {Field: "first_name"}, {Field: "id"}}
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

	if queryErr := built.Apply(r.app.RecordQuery(store.PatientCollection)).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing patients: %w", queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]person.Patient, 0, len(records))

	for _, record := range records {
		entity, mapErr := store.PatientFromRecord(record)
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
	conditions := make([]store.Condition, 0, 2)
	conditions = append(conditions, store.Equal(store.PatientOwner, ownerID))

	if query.Search != "" {
		conditions = append(conditions, store.ContainsAny(query.Search, "last_name", "first_name"))
	}

	return conditions
}

func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(store.PatientCollection).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting patients: %w", rowErr)
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

func (r *Repo) Get(ctx context.Context, ownerID, id string) (person.Patient, error) {
	record, err := r.owned(ctx, r.app, ownerID, id)
	if err != nil {
		return person.Patient{}, err
	}

	return store.PatientFromRecord(record)
}

func (r *Repo) Create(ctx context.Context, draft person.Patient) (person.Patient, error) {
	collection, err := r.collection(r.app)
	if err != nil {
		return person.Patient{}, err
	}

	record := core.NewRecord(collection)

	if mapErr := store.PatientToRecord(record, draft); mapErr != nil {
		return person.Patient{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		if isUniqueViolation(saveErr) {
			return person.Patient{}, fmt.Errorf("creating a patient: %w", domain.ErrConflict)
		}

		return person.Patient{}, fmt.Errorf("creating a patient: %w", saveErr)
	}

	return store.PatientFromRecord(record)
}

func (r *Repo) Update(ctx context.Context, patient person.Patient, expectedVersion string) (person.Patient, error) {
	var updated person.Patient

	write := func(txApp core.App) error {
		record, err := r.owned(ctx, txApp, patient.OwnerID, patient.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, patient.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if mapErr := store.PatientToRecord(record, patient); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating patient %s: %w", patient.ID, saveErr)
		}

		mapped, mapErr := store.PatientFromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return person.Patient{}, txErr
	}

	return updated, nil
}

func (r *Repo) SelfRecord(ctx context.Context, ownerID string) (person.Patient, error) {
	var records []*core.Record

	err := r.app.RecordQuery(store.PatientCollection).
		AndWhere(dbx.HashExp{store.PatientOwner: ownerID, store.PatientIsSelfRecord: true}).
		Limit(1).
		WithContext(ctx).
		All(&records)
	if err != nil {
		return person.Patient{}, fmt.Errorf("finding the self-record for %s: %w", ownerID, err)
	}

	if len(records) == 0 {
		return person.Patient{}, fmt.Errorf("no self-record for %s: %w", ownerID, domain.ErrNotFound)
	}

	return store.PatientFromRecord(records[0])
}

// PatientOwner is the resolver access.PatientOwners consumes (research D-05):
// an id in, the owning account out, domain.ErrNotFound for a row that is not
// there.
func (r *Repo) PatientOwner(ctx context.Context, patientID string) (string, error) {
	var owned struct {
		Owner string `db:"owner"`
	}

	err := r.app.RecordQuery(store.PatientCollection).
		Select(store.PatientOwner).
		AndWhere(dbx.HashExp{"id": patientID}).
		Limit(1).
		WithContext(ctx).
		One(&owned)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("reading the owner of a patient: %w", domain.ErrNotFound)
		}

		return "", fmt.Errorf("reading the owner of a patient: %w", err)
	}

	return owned.Owner, nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	collection, err := app.FindCachedCollectionByNameOrId(store.PatientCollection)
	if err != nil {
		return nil, fmt.Errorf("finding the patients collection: %w", err)
	}

	return collection, nil
}

func (r *Repo) owned(ctx context.Context, app core.App, ownerID, id string) (*core.Record, error) {
	built, err := r.schema.Build(store.Query{
		Conditions: []store.Condition{
			store.Equal(store.PatientOwner, ownerID),
			store.Equal(store.ColumnID, id),
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	var records []*core.Record

	if queryErr := built.Apply(app.RecordQuery(store.PatientCollection)).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("finding patient %s: %w", id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("patient %s: %w", id, domain.ErrNotFound)
	}

	return records[0], nil
}

// expectVersion refuses a stale write, mirroring
// internal/store/medication's own helper.
func expectVersion(record *core.Record, id, expectedVersion string) error {
	if store.Version(record) != expectedVersion {
		return fmt.Errorf("patient %s: %w", id, domain.ErrVersionMismatch)
	}

	return nil
}

// isUniqueViolation is the partial unique index's own refusal (FR-004),
// caught here as the second, storage-layer line of defence behind the
// service's own SelfRecord check (data-model §3).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
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
