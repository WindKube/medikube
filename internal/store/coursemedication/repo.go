package coursemedication

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	service "medikube/internal/service/coursemedication"
	"medikube/internal/store"
)

// linkSchema is treatment_medications' throwaway query surface: both relation
// columns, neither orderable, just enough to find the one row a (treatment,
// medication) pair names without PocketBase's filter DSL.
func linkSchema() store.Schema {
	return store.NewSchema(Collection,
		store.Column{Name: fieldTreatment, FilterOnly: true},
		store.Column{Name: fieldMedication, FilterOnly: true},
	)
}

// findLink is the unique-index lookup Upsert and Delete both need: the one
// treatment_medications row for this pair, or nil if there is none.
func findLink(ctx context.Context, app core.App, treatmentID, medicationID string) (*core.Record, error) {
	built, err := linkSchema().Build(store.Query{
		Conditions: []store.Condition{
			store.Equal(fieldTreatment, treatmentID),
			store.Equal(fieldMedication, medicationID),
		},
		Limit: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("store/coursemedication: building the link query: %w", err)
	}

	var records []*core.Record
	if err := built.Apply(app.RecordQuery(Collection)).WithContext(ctx).All(&records); err != nil {
		return nil, fmt.Errorf("store/coursemedication: finding the link: %w", err)
	}

	if len(records) == 0 {
		return nil, nil
	}

	return records[0], nil
}

type Repo struct {
	app core.App
}

var _ service.Repository = (*Repo)(nil)

func New(app core.App) (*Repo, error) {
	if app == nil {
		return nil, fmt.Errorf("store/coursemedication: the repository is wired with no application")
	}

	return &Repo{app: app}, nil
}

// treatmentMedicationSort is contracts/treatment-medications.md §1's
// published ordering: PocketBase's dot notation resolves the related
// medication's own `name` column.
const treatmentMedicationSort = "medication.name,id"

func (r *Repo) List(_ context.Context, treatmentID string) ([]clinical.CourseMedication, error) {
	records, err := r.app.FindRecordsByFilter(
		Collection, fieldTreatment+" = {:treatment}", treatmentMedicationSort, 0, 0,
		dbx.Params{"treatment": treatmentID},
	)
	if err != nil {
		return nil, fmt.Errorf("store/coursemedication: listing %s: %w", treatmentID, err)
	}

	items := make([]clinical.CourseMedication, 0, len(records))

	for _, record := range records {
		mapped, err := FromRecord(record)
		if err != nil {
			return nil, err
		}

		items = append(items, mapped)
	}

	return items, nil
}

// Upsert is FR-061 inside a transaction, guarded by the unique index on
// (treatment, medication): a row already there is updated, never duplicated.
func (r *Repo) Upsert(
	ctx context.Context, entity clinical.CourseMedication, expectedTreatmentVersion string,
) (clinical.CourseMedication, bool, error) {
	var (
		result  clinical.CourseMedication
		created bool
	)

	write := func(txApp core.App) error {
		treatmentRecord, err := txApp.FindRecordById(kind.Treatment.Collection(), entity.TreatmentID)
		if err != nil {
			return fmt.Errorf("%w: finding treatment %s: %w", domain.ErrNotFound, entity.TreatmentID, err)
		}

		if err := expectVersion(treatmentRecord, entity.TreatmentID, expectedTreatmentVersion); err != nil {
			return err
		}

		collection, err := txApp.FindCollectionByNameOrId(Collection)
		if err != nil {
			return fmt.Errorf("store/coursemedication: finding %s: %w", Collection, err)
		}

		record, findErr := findLink(ctx, txApp, entity.TreatmentID, entity.MedicationID)
		if findErr != nil {
			return findErr
		}

		if record == nil {
			record = core.NewRecord(collection)
			created = true
		}

		if mapErr := ToRecord(record, entity); mapErr != nil {
			return mapErr
		}

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return fmt.Errorf("store/coursemedication: saving the link: %w", saveErr)
		}

		mapped, mapErr := FromRecord(record)
		if mapErr != nil {
			return mapErr
		}

		result = mapped

		return nil
	}

	if err := store.RunInTransaction(r.app, write); err != nil {
		return clinical.CourseMedication{}, false, err
	}

	return result, created, nil
}

func (r *Repo) Delete(ctx context.Context, treatmentID, medicationID, expectedTreatmentVersion string) error {
	write := func(txApp core.App) error {
		treatmentRecord, err := txApp.FindRecordById(kind.Treatment.Collection(), treatmentID)
		if err != nil {
			return fmt.Errorf("%w: finding treatment %s: %w", domain.ErrNotFound, treatmentID, err)
		}

		if err := expectVersion(treatmentRecord, treatmentID, expectedTreatmentVersion); err != nil {
			return err
		}

		record, err := findLink(ctx, txApp, treatmentID, medicationID)
		if err != nil {
			return err
		}

		if record == nil {
			return fmt.Errorf("%w: no link between %s and %s", domain.ErrNotFound, treatmentID, medicationID)
		}

		if delErr := txApp.DeleteWithContext(ctx, record); delErr != nil {
			return fmt.Errorf("store/coursemedication: deleting the link: %w", delErr)
		}

		return nil
	}

	return store.RunInTransaction(r.app, write)
}

func expectVersion(record *core.Record, id, expected string) error {
	if store.Version(record) == expected {
		return nil
	}

	return fmt.Errorf("treatment %s: %w", id, domain.ErrVersionMismatch)
}
