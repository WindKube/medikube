package immunization_test

import (
	"testing"

	"medikube/internal/service/immunization"
	"medikube/internal/service/immunization/immunizationtest"
)

func TestTheInMemoryRepositoryPassesTheContract(t *testing.T) {
	t.Parallel()

	immunizationtest.RunRepositoryContract(t, func(*testing.T) (immunization.Repository, immunizationtest.Accounts) {
		return immunizationtest.NewRepository(), immunizationtest.Accounts{
			Patient:         immunizationtest.PatientID,
			StrangerPatient: immunizationtest.StrangerPatientID,
		}
	})
}
