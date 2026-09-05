package facility_test

import (
	"testing"

	"medikube/internal/service/facility"
	"medikube/internal/service/facility/facilitytest"
)

// TestTheInMemoryRepositoryPassesTheContract is T120's other half: the fake
// the service's own tests run against is held to the same rules as the
// PocketBase repository (internal/store/facility runs this same suite against
// a real instance), so a test that passes here cannot be passing because the
// fake is more forgiving.
func TestTheInMemoryRepositoryPassesTheContract(t *testing.T) {
	t.Parallel()

	facilitytest.RunRepositoryContract(t, func(*testing.T) (facility.Repository, facilitytest.Accounts) {
		return facilitytest.NewRepository(), facilitytest.Accounts{
			Owner:    facilitytest.OwnerID,
			Stranger: facilitytest.StrangerID,
		}
	})
}
