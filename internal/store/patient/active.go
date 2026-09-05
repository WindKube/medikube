package patient

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	service "medikube/internal/service/patient"
	"medikube/internal/store"
)

// ActivePatientRepo is the PocketBase adapter for service.ActivePatientStore:
// the users.active_patient column, read and written apart from the profile
// (contracts/active-patient.md).
type ActivePatientRepo struct {
	app core.App
}

var _ service.ActivePatientStore = (*ActivePatientRepo)(nil)

func NewActivePatientRepo(app core.App) (*ActivePatientRepo, error) {
	if app == nil {
		return nil, errors.New("patient: the active-patient repository is wired with no application")
	}

	return &ActivePatientRepo{app: app}, nil
}

// ActivePatient answers the pointer, or "" when it is unset.
func (r *ActivePatientRepo) ActivePatient(ctx context.Context, userID string) (string, error) {
	record, err := r.user(ctx, userID)
	if err != nil {
		return "", err
	}

	return store.UserActivePatientID(record), nil
}

// SetActivePatient writes the pointer. patientID "" clears it.
func (r *ActivePatientRepo) SetActivePatient(ctx context.Context, userID, patientID string) error {
	record, err := r.user(ctx, userID)
	if err != nil {
		return err
	}

	store.SetUserActivePatientID(record, patientID)

	if err := r.app.SaveWithContext(ctx, record); err != nil {
		return fmt.Errorf("patient: writing the active-patient pointer: %w", err)
	}

	return nil
}

func (r *ActivePatientRepo) user(ctx context.Context, userID string) (*core.Record, error) {
	record, err := r.app.FindRecordById(store.AccountCollection, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("patient: reading the account for the active-patient pointer: %w", domain.ErrNotFound)
		}

		return nil, fmt.Errorf("patient: reading the account for the active-patient pointer: %w", err)
	}

	return record, nil
}
