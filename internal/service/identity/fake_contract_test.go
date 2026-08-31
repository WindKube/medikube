package identity_test

import (
	"testing"

	"medikube/internal/service/identity/identitytest"
)

// TestTheInMemoryImplementationPassesTheContract is half of what makes T189
// worth writing: the fakes the service's own tests run against are held to the
// same rules as the PocketBase implementation, so a test that passes above them
// cannot be passing because the fake was more forgiving.
//
// The other half is T191, where internal/store/identity runs this same suite
// against a real instance — including the case-insensitive uniqueness that only
// idx_users_email_lower can actually decide.
func TestTheInMemoryImplementationPassesTheContract(t *testing.T) {
	t.Parallel()

	identitytest.RunRepositoryContract(t, func(*testing.T) identitytest.Implementation {
		repository := identitytest.NewRepository()
		authenticator := identitytest.NewAuthenticator(repository)

		return identitytest.Implementation{
			Repository:    repository,
			Authenticator: authenticator,
			Token:         authenticator.Token,
		}
	})
}
