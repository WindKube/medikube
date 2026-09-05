package tag_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	dtag "medikube/internal/domain/tag"
	"medikube/internal/service/tag"
	"medikube/internal/service/tag/tagtest"
)

func newService(t *testing.T) (*tag.Service, *tagtest.Repository) {
	t.Helper()

	repository := tagtest.NewRepository()
	authorizer := tagtest.NewAuthorizer(tagtest.OwnerID)
	auditor := tagtest.NewAuditor()

	service, err := tag.New(repository, repository, repository, authorizer, auditor)
	require.NoError(t, err)

	return service, repository
}

func ownerActor() access.Actor { return access.Actor{UserID: tagtest.OwnerID} }

// T152: creating "Cardiology" after "cardiology" is 409 duplicate_name
// (FR-063, US7-2).
func TestCreateRefusesACaseInsensitiveDuplicate(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)
	ctx := t.Context()

	_, err := service.Create(ctx, ownerActor(), dtag.Tag{Name: "cardiology"})
	require.NoError(t, err)

	_, err = service.Create(ctx, ownerActor(), dtag.Tag{Name: "Cardiology"})
	require.ErrorIs(t, err, tag.ErrDuplicateName)
}

// A rename is a single row update that no carrier loses (FR-065): the fake
// Update rewrites one row and the service's List still finds it under the new
// name (the referencing kinds' own AnyOf/AllOf conditions read the same
// relation, so nothing here can silently lose a carrier).
func TestRenameIsOneRowUpdate(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)
	ctx := t.Context()

	created, err := service.Create(ctx, ownerActor(), dtag.Tag{Name: "cardiology"})
	require.NoError(t, err)

	renamed := "cardio"
	updated, err := service.Update(ctx, ownerActor(), created.ID, tag.Patch{Name: &renamed})
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "cardio", updated.Name)

	page, err := service.List(ctx, ownerActor(), tag.Query{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "cardio", page.Items[0].Name)
}

// usage_count is derived across every kind and correct after a carrier is
// deleted (FR-068): the fake counter mirrors what a real deletion would
// leave behind, and Usage answers it directly.
func TestUsageIsDerivedAndCorrectAfterACarrierIsDeleted(t *testing.T) {
	t.Parallel()

	service, repository := newService(t)
	ctx := t.Context()

	created, err := service.Create(ctx, ownerActor(), dtag.Tag{Name: "cardiology"})
	require.NoError(t, err)

	repository.SetUsage(created.ID, 8)

	counts, err := service.Usage(ctx, ownerActor(), []string{created.ID})
	require.NoError(t, err)
	assert.Equal(t, 8, counts[created.ID])

	repository.SetUsage(created.ID, 7)

	counts, err = service.Usage(ctx, ownerActor(), []string{created.ID})
	require.NoError(t, err)
	assert.Equal(t, 7, counts[created.ID])
}

// Another account's tags are neither listed nor addressable (FR-062, US7-5).
func TestAnotherAccountsTagsAreNeitherListedNorAddressable(t *testing.T) {
	t.Parallel()

	service, repository := newService(t)
	ctx := t.Context()

	stranger := access.Actor{UserID: tagtest.StrangerID}

	stray, err := repository.Create(ctx, dtag.Tag{OwnerID: tagtest.StrangerID, Name: "claim pending"})
	require.NoError(t, err)

	_, err = service.Get(ctx, ownerActor(), stray.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = service.Update(ctx, ownerActor(), stray.ID, tag.Patch{})
	require.ErrorIs(t, err, domain.ErrNotFound)

	err = service.Delete(ctx, ownerActor(), stray.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = service.List(ctx, stranger, tag.Query{})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

// Owned is internal/records.TagChecker's shape (FR-064): a foreign tag id is
// refused as not found, identical to one that does not exist.
func TestOwnedRefusesAForeignID(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)
	ctx := t.Context()

	created, err := service.Create(ctx, ownerActor(), dtag.Tag{Name: "cardiology"})
	require.NoError(t, err)

	require.NoError(t, service.Owned(ctx, ownerActor(), []string{created.ID}))

	err = service.Owned(ctx, ownerActor(), []string{"does-not-exist"})
	require.ErrorIs(t, err, domain.ErrNotFound)
}
