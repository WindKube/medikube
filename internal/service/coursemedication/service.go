package coursemedication

import (
	"context"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
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
	if _, err := s.treatments.Get(ctx, treatmentID); err != nil {
		return nil, err
	}

	if err := s.authorize(ctx, actor, kind.Treatment, treatmentID, access.PermView); err != nil {
		return nil, err
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

	if err := entity.Validate(); err != nil {
		return Item{}, false, err
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
// contract: same patient, and Authorizer.Record on both the treatment and
// the medication, every time (FR-057).
func (s *Service) authorizeBothEnds(
	ctx context.Context, actor access.Actor, treatmentID, medicationID string,
) (clinical.Treatment, clinical.Medication, error) {
	treatment, err := s.treatments.Get(ctx, treatmentID)
	if err != nil {
		return clinical.Treatment{}, clinical.Medication{}, err
	}

	if err := s.authorize(ctx, actor, kind.Treatment, treatmentID, access.PermEdit); err != nil {
		return clinical.Treatment{}, clinical.Medication{}, err
	}

	medication, err := s.medications.Get(ctx, medicationID)
	if err != nil {
		return clinical.Treatment{}, clinical.Medication{}, err
	}

	if medication.PatientID != treatment.PatientID {
		return clinical.Treatment{}, clinical.Medication{}, domain.ErrNotFound
	}

	if err := s.authorize(ctx, actor, kind.Medication, medicationID, access.PermEdit); err != nil {
		return clinical.Treatment{}, clinical.Medication{}, err
	}

	return treatment, medication, nil
}

func (s *Service) authorize(ctx context.Context, actor access.Actor, k kind.Kind, id string, need access.Permission) error {
	grant, err := s.authorizer.Record(ctx, actor, k, id, need)
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
