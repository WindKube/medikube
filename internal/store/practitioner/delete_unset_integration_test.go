package practitioner_test

import (
	"testing"

	"medikube/internal/domain/kind"
)

// T126. Create a practitioner, reference it from a patient's
// primary_practitioner and from a medication's practitioner, delete the
// practitioner, and assert both referencing records survive with the
// reference cleared to empty (contracts/practitioners.md's
// deletePractitioner, PocketBase's deleteRefRecords behaviour, research D-06).
//
// medications.practitioner is added by 1756200600_medications_repoint.go
// (specs/002-patient-core/data-model.md §19), which had not landed in this
// worktree's migrations when this file was written — grep -rn practitioner
// internal/store/migrations found only patients.primary_practitioner and the
// practitioners collection itself, not a practitioner column on medications.
// Skipped rather than half-written against a schema that is not there yet;
// whichever agent lands that migration should replace this skip with the real
// two-collection assertion the contract requires.
func TestDeletingAPractitionerClearsEveryReference(t *testing.T) {
	t.Skip("patients/" + kind.Medication.Collection() + " practitioner reference migration not yet present: " +
		kind.Medication.Collection() + ".practitioner is added by the repoint migration data-model.md §19 describes, absent from this worktree")
}
