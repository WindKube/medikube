package tag_test

import (
	"testing"

	"medikube/internal/service/tag"
	"medikube/internal/service/tag/tagtest"
)

// TestTheInMemoryRepositoryPassesTheContract is T153's other half:
// internal/store/tag runs this same suite.
func TestTheInMemoryRepositoryPassesTheContract(t *testing.T) {
	t.Parallel()

	tagtest.RunRepositoryContract(t, func(*testing.T) (tag.Repository, tagtest.Accounts) {
		repo := tagtest.NewRepository()

		return repo, tagtest.Accounts{Owner: tagtest.OwnerID, Stranger: tagtest.StrangerID}
	})
}
