package medication

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/medication"
	"medikube/internal/store"
)

// Repo is the medication repository the service declares, against PocketBase.
//
// List is patient-scoped in the predicate. Get, Update and Delete are not: the
// service resolves and authorizes the patient from the row itself before
// touching it (research D-13), so a second, redundant scope here would only
// hide which layer actually decided (contracts/medications-rescope.md).
type Repo struct {
	app     core.App
	cursors *store.CursorCodec
	schema  store.Schema
}

// The seam this package exists to fill. A compile-time assertion rather than a
// runtime one: a method that drifts from the port should not build.
var _ service.Repository = (*Repo)(nil)

// New wires the repository to an instance and to the codec that seals its
// cursors.
//
// The codec is a dependency rather than something derived here, because the key
// material is the operator's or the instance's (store.CursorSecret) and a
// repository that reached for it would be a second place deciding how cursors
// are keyed.
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
			kind.Medication, strings.Join(missing, " and "))
	}

	return &Repo{app: app, cursors: cursors, schema: store.MedicationSchema()}, nil
}

// List returns one page of the patient's medications and mints the boundary
// for the next one.
//
// The boundary is a row, never an offset. An offset is defined against a result
// set that is changing underneath it, so a row inserted above it shifts every
// later page by one and the reader silently never sees one entry — which is
// exactly FR-023's "must not show the same entry twice nor skip an entry".
func (r *Repo) List(
	ctx context.Context,
	patientID string,
	query service.Query,
) (domain.Page[clinical.Medication], error) {
	var empty domain.Page[clinical.Medication]

	// An unstated ordering is the published default and not the database's.
	// Left to the database it would be insertion order, which is stable until
	// the day it is not — and the fake resolves it the same way, or the two
	// implementations would disagree about a query the contract runs.
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

	// One row more than the page. "Is there another page" is then answered by
	// the same query rather than by a count, which would be a second read of a
	// list that can change between the two.
	size := built.Limit
	built.Limit = size + 1

	var records []*core.Record

	if queryErr := built.Apply(r.app.RecordQuery(kind.Medication.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return empty, fmt.Errorf("listing %s: %w", kind.Medication, queryErr)
	}

	more := len(records) > size
	if more {
		records = records[:size]
	}

	items := make([]clinical.Medication, 0, len(records))

	for _, record := range records {
		entity, mapErr := store.MedicationFromRecord(record)
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

// narrowing is the query's terms, patient first.
//
// The patient is a term of its own and stays one. The search is the only term
// that spans more than one column, and internal/store admits that shape only
// over columns declared searchable — so a widened disjunction cannot swallow
// the scope, whatever this function is later edited to say.
func (r *Repo) narrowing(patientID string, query service.Query) []store.Condition {
	conditions := make([]store.Condition, 0, 3)
	conditions = append(conditions, store.Equal(store.MedicationPatient, patientID))

	if query.Search != "" {
		conditions = append(conditions,
			store.ContainsAny(query.Search, store.MedicationName, store.MedicationAlternativeName))
	}

	if len(query.Statuses) > 0 {
		states := make([]string, 0, len(query.Statuses))
		for _, status := range query.Statuses {
			states = append(states, string(status))
		}

		conditions = append(conditions, store.OneOf(store.MedicationStatus, states...))
	}

	return conditions
}

// count answers how many rows the narrowing matches, and deliberately not how
// many are left: the keyset boundary is absent from this query, so `total` is
// the same number on every page of a traversal.
func (r *Repo) count(ctx context.Context, conditions []store.Condition) (int, error) {
	built, err := r.schema.Build(store.Query{Conditions: conditions})
	if err != nil {
		return 0, err
	}

	counting := r.app.RecordQuery(kind.Medication.Collection()).Select("count(*)")

	if built.Where != nil {
		counting = counting.AndWhere(built.Where)
	}

	var total int

	if rowErr := counting.WithContext(ctx).Row(&total); rowErr != nil {
		return 0, fmt.Errorf("counting %s: %w", kind.Medication, rowErr)
	}

	return total, nil
}

// boundary seals the last row of a page into the token the next request hands
// back.
func (r *Repo) boundary(patientID string, record *core.Record, sortKeys []domain.SortKey) (string, error) {
	cursor, err := r.schema.Boundary(record, sortKeys)
	if err != nil {
		return "", err
	}

	return r.cursors.Encode(scope(patientID), cursor)
}

// Get answers domain.ErrNotFound for a row that does not exist. It is not
// scoped by patient: the service reads medication.PatientID off the result and
// authorizes it there (FR-033, research D-13).
func (r *Repo) Get(ctx context.Context, id string) (clinical.Medication, error) {
	record, err := r.byID(ctx, r.app, id)
	if err != nil {
		return clinical.Medication{}, err
	}

	return store.MedicationFromRecord(record)
}

// Create mints the identity, the timestamps and the version. Whatever the
// entity carried in those four is not read: PocketBase owns all of them, and a
// repository that honoured an id from a request would let a caller choose one.
//
// The write goes through SaveWithContext rather than Save, as every write in
// this file does. The difference is what happens when the caller has gone away:
// with the context the statement is cancelled, and without it the row is
// written for a request nobody is waiting for any more.
func (r *Repo) Create(ctx context.Context, entity clinical.Medication) (clinical.Medication, error) {
	collection, err := r.collection(r.app)
	if err != nil {
		return clinical.Medication{}, err
	}

	record := core.NewRecord(collection)

	if mapErr := store.MedicationToRecord(record, entity); mapErr != nil {
		return clinical.Medication{}, mapErr
	}

	if saveErr := r.app.SaveWithContext(ctx, record); saveErr != nil {
		return clinical.Medication{}, fmt.Errorf("creating a %s: %w", kind.Medication, saveErr)
	}

	return store.MedicationFromRecord(record)
}

// Update writes the entity over the row it identifies, only while the stored
// version is still expectedVersion.
//
// The read and the write are one transaction, which is what makes the version
// check a check. Read the version in the service and write here and two callers
// both read the same version, both are told they are current, and the second
// overwrites the first with no error anywhere.
func (r *Repo) Update(
	ctx context.Context,
	entity clinical.Medication,
	expectedVersion string,
) (clinical.Medication, error) {
	var updated clinical.Medication

	write := func(txApp core.App) error {
		record, err := r.byID(ctx, txApp, entity.ID)
		if err != nil {
			return err
		}

		if versionErr := expectVersion(record, entity.ID, expectedVersion); versionErr != nil {
			return versionErr
		}

		if mapErr := store.MedicationToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("updating %s %s: %w", kind.Medication, entity.ID, saveErr)
		}

		mapped, mapErr := store.MedicationFromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		updated = mapped

		return nil
	}

	if txErr := store.RunInTransaction(r.app, write); txErr != nil {
		return clinical.Medication{}, txErr
	}

	return updated, nil
}

// Delete is permanent (FR-028): no tombstone, no deleted_at, nothing left
// behind to be filtered out of a later read.
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
			return fmt.Errorf("deleting %s %s: %w", kind.Medication, id, deleteErr)
		}

		return nil
	})
}

// byID reads one row by id, unscoped: the caller authorizes what it names.
func (r *Repo) byID(ctx context.Context, app core.App, id string) (*core.Record, error) {
	built, err := r.schema.Build(store.Query{
		Conditions: []store.Condition{
			store.Equal(store.ColumnID, id),
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	var records []*core.Record

	if queryErr := built.Apply(app.RecordQuery(kind.Medication.Collection())).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("reading %s %s: %w", kind.Medication, id, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("%s %s: %w", kind.Medication, id, domain.ErrNotFound)
	}

	return records[0], nil
}

func (r *Repo) collection(app core.App) (*core.Collection, error) {
	collection, err := app.FindCachedCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return nil, fmt.Errorf("reading the %s collection: %w", kind.Medication, err)
	}

	return collection, nil
}

// expectVersion is FR-026's If-Match, resolved against the row the transaction
// is holding rather than against one read earlier.
func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("%s %s: %w", kind.Medication, id, domain.ErrVersionMismatch)
}

// scope is the query a cursor continues: this kind, for this patient.
//
// It is authenticated into the token and never transmitted, so a boundary
// discloses nothing about whose list it is and cannot be replayed against
// somebody else's. The separator is a byte no identifier holds, so two
// different scopes cannot concatenate into the same string.
func scope(patientID string) string {
	return kind.Medication.Collection() + "\x00" + patientID
}
