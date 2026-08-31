package medication_test

import (
	"testing"

	"medikube/internal/service/medication"
	"medikube/internal/service/medication/medicationtest"
)

// TestTheInMemoryRepositoryPassesTheContract is T139, and it is half of what
// makes the contract suite worth writing: the fake the service's own tests run
// against is held to the same rules as the PocketBase repository, so a test
// that passes here cannot be passing because the fake is more forgiving.
//
// The other half is T140, where internal/store/medication runs this same suite
// against a real instance.
func TestTheInMemoryRepositoryPassesTheContract(t *testing.T) {
	t.Parallel()

	medicationtest.RunRepositoryContract(t, func(*testing.T) (medication.Repository, medicationtest.Accounts) {
		return medicationtest.NewRepository(), medicationtest.Accounts{
			Owner:    medicationtest.OwnerID,
			Stranger: medicationtest.StrangerID,
		}
	})
}
