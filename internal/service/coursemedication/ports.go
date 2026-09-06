// Package coursemedication is FR-060/FR-061's course-medication attachment:
// GET/PUT/DELETE /api/v1/treatments/{id}/medications[/{medicationId}]. It is
// not one of internal/records' registered kinds — it is the relationship
// between a treatment and a medication, not a record kind of its own.
package coursemedication

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

// Repository is the treatment_medications storage seam.
type Repository interface {
	List(ctx context.Context, treatmentID string) ([]clinical.CourseMedication, error)

	// Upsert is FR-061's idempotent attach: the same (treatment, medication)
	// pair upserts to one row. created is true only the first time. It runs
	// inside its own transaction, guarded by the unique index, and checks
	// expectedTreatmentVersion against the treatment row it reads there.
	Upsert(
		ctx context.Context, entity clinical.CourseMedication, expectedTreatmentVersion string,
	) (result clinical.CourseMedication, created bool, err error)

	// Delete removes the link row only; the treatment and the medication are
	// untouched (FR-058).
	Delete(ctx context.Context, treatmentID, medicationID, expectedTreatmentVersion string) error
}

// Treatments and Medications are read-only seams onto the two record kinds
// this package attaches: reused rather than re-declared, because
// internal/store/treatment.Repo and internal/store/medication's own
// repository already satisfy this shape.
type Treatments interface {
	Get(ctx context.Context, id string) (clinical.Treatment, error)
}

type Medications interface {
	Get(ctx context.Context, id string) (clinical.Medication, error)
}

// Authorizer is the per-patient checkpoint. Every mutation authorizes both
// the treatment's patient and the medication's patient (data-model §7.4,
// "actor may edit both") — both are patient-scoped kinds with no flat
// "owner" column of their own (phase 002 D-13), so the anchor is the patient,
// the same way treatment.Service and medication.Service authorize themselves.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}

// Patch is one upsert's body: every field optional, and an absent one means
// "fall back to the medication's own value" (FR-060).
type Patch struct {
	Dosage    *string
	Frequency *string
	Duration  *string
	Timing    *string

	PrescriberID *string
	PharmacyID   *string

	StartedOn *domain.Date
	EndedOn   *domain.Date
}

// Item is one resolved link row: the stored course-specific fields, the
// medication it attaches, and every effective value with its provenance
// (FR-060).
type Item struct {
	CourseMedication clinical.CourseMedication
	Medication       clinical.Medication
	Effective        clinical.EffectiveFields
}
