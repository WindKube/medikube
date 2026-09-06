package coursemedication

import (
	"context"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

type Service struct {
	repository  Repository
	treatments  Treatments
	medications Medications
	authorizer  Authorizer
}

func New(repository Repository, treatments Treatments, medications Medications, authorizer Authorizer) (*Service, error) {
	if repository == nil || treatments == nil || medications == nil || authorizer == nil {
		return nil, fmt.Errorf("coursemedication: the service is wired with a nil dependency")
	}

	return &Service{repository: repository, treatments: treatments, medications: medications, authorizer: authorizer}, nil
}

// List is `GET /treatments/{id}/medications`: every medication attached to
// this course, each resolved against its own medication (FR-060).
func (s *Service) List(ctx context.Context, actor access.Actor, treatmentID string) ([]Item, error) {
	treatment, err := s.treatments.Get(ctx, treatmentID)
	if err != nil {
		return nil, err
	}

	if authErr := s.authorizePatient(ctx, actor, treatment.PatientID, access.PermView); authErr != nil {
		return nil, authErr
	}

	rows, err := s.repository.List(ctx, treatmentID)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(rows))

	for _, row := range rows {
		medication, err := s.medications.Get(ctx, row.MedicationID)
		if err != nil {
			return nil, err
		}

		items = append(items, Item{CourseMedication: row, Medication: medication, Effective: row.Resolve(medication)})
	}

	return items, nil
}

// Upsert is `PUT /treatments/{id}/medications/{medicationId}` (FR-061): the
// same pair attached twice updates the one row, and created reports which.
func (s *Service) Upsert(
	ctx context.Context, actor access.Actor, treatmentID, medicationID string, patch Patch, expectedTreatmentVersion string,
) (Item, bool, error) {
	_, medication, err := s.authorizeBothEnds(ctx, actor, treatmentID, medicationID)
	if err != nil {
		return Item{}, false, err
	}

	entity := clinical.CourseMedication{
		TreatmentID: treatmentID, MedicationID: medicationID,
		Dosage: derefString(patch.Dosage), Frequency: derefString(patch.Frequency),
		Duration: derefString(patch.Duration), Timing: derefString(patch.Timing),
		PrescriberID: derefString(patch.PrescriberID), PharmacyID: derefString(patch.PharmacyID),
		StartedOn: derefDate(patch.StartedOn), EndedOn: derefDate(patch.EndedOn),
	}

	if validateErr := entity.Validate(); validateErr != nil {
		return Item{}, false, validateErr
	}

	stored, created, err := s.repository.Upsert(ctx, entity, expectedTreatmentVersion)
	if err != nil {
		return Item{}, false, err
	}

	return Item{CourseMedication: stored, Medication: medication, Effective: stored.Resolve(medication)}, created, nil
}

// Delete is `DELETE /treatments/{id}/medications/{medicationId}` (FR-058):
// removes the link row only.
func (s *Service) Delete(ctx context.Context, actor access.Actor, treatmentID, medicationID, expectedTreatmentVersion string) error {
	if _, _, err := s.authorizeBothEnds(ctx, actor, treatmentID, medicationID); err != nil {
		return err
	}

	return s.repository.Delete(ctx, treatmentID, medicationID, expectedTreatmentVersion)
}

// authorizeBothEnds is data-model §7.4's invariant applied to this one
// contract: same patient, and Authorizer.Patient on both the treatment's
// patient and the medication's, every time (FR-057).
func (s *Service) authorizeBothEnds(
	ctx context.Context, actor access.Actor, treatmentID, medicationID string,
) (clinical.Treatment, clinical.Medication, error) {
	treatment, err := s.treatments.Get(ctx, treatmentID)
	if err != nil {
		return clinical.Treatment{}, clinical.Medication{}, err
	}

	if authErr := s.authorizePatient(ctx, actor, treatment.PatientID, access.PermEdit); authErr != nil {
		return clinical.Treatment{}, clinical.Medication{}, authErr
	}

	medication, err := s.medications.Get(ctx, medicationID)
	if err != nil {
		return clinical.Treatment{}, clinical.Medication{}, err
	}

	if medication.PatientID != treatment.PatientID {
		return clinical.Treatment{}, clinical.Medication{}, domain.ErrNotFound
	}

	if err := s.authorizePatient(ctx, actor, medication.PatientID, access.PermEdit); err != nil {
		return clinical.Treatment{}, clinical.Medication{}, err
	}

	return treatment, medication, nil
}

func (s *Service) authorizePatient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) error {
	grant, err := s.authorizer.Patient(ctx, actor, patientID, need)
	if err != nil {
		return err
	}

	if !grant.Allows(need) {
		return domain.ErrNotFound
	}

	return nil
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}

func derefDate(v *domain.Date) domain.Date {
	if v == nil {
		return domain.Date{}
	}

	return *v
}
