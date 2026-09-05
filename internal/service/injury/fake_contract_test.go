package injury_test

import (
	"testing"

	"medikube/internal/service/injury"
	"medikube/internal/service/injury/injurytest"
)

func TestTheInMemoryRepositoryPassesTheContract(t *testing.T) {
	t.Parallel()

	injurytest.RunRepositoryContract(t, func(*testing.T) (injury.Repository, injurytest.Accounts) {
		return injurytest.NewRepository(), injurytest.Accounts{
			Patient:         injurytest.PatientID,
			StrangerPatient: injurytest.StrangerPatientID,
		}
	})
}
