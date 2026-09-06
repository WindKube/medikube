package coursemedication

import (
	"context"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
)

type Service struct {
	repository  Repository
	treatments  Treatments
	medications Medications
	authorizer  Authorizer
	auditor     Auditor
}

func New(
	repository Repository, treatments Treatments, medications Medications, authorizer Authorizer, auditor Auditor,
) (*Service, error) {
	if repository == nil || treatments == nil || medications == nil || authorizer == nil || auditor == nil {
		return nil, fmt.Errorf("coursemedication: the service is wired with a nil dependency")
	}

	return &Service{
		repository: repository, treatments: treatments, medications: medications,
		authorizer: authorizer, auditor: auditor,
	}, nil
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
	treatment, medication, err := s.authorizeBothEnds(ctx, actor, treatmentID, medicationID)
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

	action := audit.ActionUpdate
	if created {
		action = audit.ActionCreate
	}

	s.audit(ctx, actor, action, treatmentID, treatment.PatientID)

	return Item{CourseMedication: stored, Medication: medication, Effective: stored.Resolve(medication)}, created, nil
}

// Delete is `DELETE /treatments/{id}/medications/{medicationId}` (FR-058):
// removes the link row only.
func (s *Service) Delete(ctx context.Context, actor access.Actor, treatmentID, medicationID, expectedTreatmentVersion string) error {
	treatment, _, err := s.authorizeBothEnds(ctx, actor, treatmentID, medicationID)
	if err != nil {
		return err
	}

	if err := s.repository.Delete(ctx, treatmentID, medicationID, expectedTreatmentVersion); err != nil {
		return err
	}

	s.audit(ctx, actor, audit.ActionDelete, treatmentID, treatment.PatientID)

	return nil
}

// audit is FR-084's row for an attach, a re-attach or a detach: the
// treatment the course medication belongs to, never the dose or any other
// course-specific field this event carries no column for (FR-085). Its own
// failure is not reported to the caller, the same trade-off
// access.Authorizer.denyPatient makes for a denial: the write already
// committed, and there is nothing left to undo.
func (s *Service) audit(ctx context.Context, actor access.Actor, action audit.Action, treatmentID, patientID string) {
	_ = s.auditor.Record(ctx, audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  audit.ActorKindUser,
		Action:     action,
		TargetKind: audit.TargetKindTreatment,
		TargetID:   treatmentID,
		RequestID:  actor.RequestID,
		PatientID:  patientID,
	})
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
