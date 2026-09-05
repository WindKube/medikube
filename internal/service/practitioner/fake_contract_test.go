package practitioner_test

import (
	"fmt"
	"testing"

	"medikube/internal/service/practitioner"
	"medikube/internal/service/practitioner/practitionertest"
)

// TestTheInMemoryRepositoryPassesTheContract is T119's other half: the fake
// the service's own tests run against is held to the same rules as the
// PocketBase repository (internal/store/practitioner runs this same suite).
func TestTheInMemoryRepositoryPassesTheContract(t *testing.T) {
	t.Parallel()

	next := 0

	practitionertest.RunRepositoryContract(t, func(*testing.T) (practitioner.Repository, practitionertest.Accounts) {
		repo := practitionertest.NewRepository()

		return repo, practitionertest.Accounts{
			Owner:    practitionertest.OwnerID,
			Stranger: practitionertest.StrangerID,
			SeedFacility: func(t *testing.T, ownerID string) string {
				t.Helper()

				next++
				id := fmt.Sprintf("mkfakefac%06d", next)
				repo.AllowFacility(ownerID, id)

				return id
			},
		}
	})
}
