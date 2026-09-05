package tagtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	dtag "medikube/internal/domain/tag"
	svc "medikube/internal/service/tag"
)

// Accounts is the two owners a contract run needs.
type Accounts struct {
	Owner    string
	Stranger string
}

// NewRepo builds one fresh Repository plus the two accounts to run the
// contract against.
type NewRepo func(t *testing.T) (svc.Repository, Accounts)

// RunRepositoryContract is every implementation's shared proof — the fake
// and internal/store/tag both pass this (Principle III's contract tier).
func RunRepositoryContract(t *testing.T, newRepo NewRepo) {
	t.Helper()

	t.Run("create refuses a case-insensitive duplicate", func(t *testing.T) {
		t.Parallel()

		repo, accounts := newRepo(t)
		ctx := context.Background()

		_, err := repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "cardiology"})
		require.NoError(t, err)

		_, err = repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "Cardiology"})
		require.ErrorIs(t, err, svc.ErrDuplicateName)
	})

	t.Run("the same name is free for another owner", func(t *testing.T) {
		t.Parallel()

		repo, accounts := newRepo(t)
		ctx := context.Background()

		_, err := repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "cardiology"})
		require.NoError(t, err)

		_, err = repo.Create(ctx, dtag.Tag{OwnerID: accounts.Stranger, Name: "cardiology"})
		require.NoError(t, err)
	})

	t.Run("get answers not found for another owner's tag", func(t *testing.T) {
		t.Parallel()

		repo, accounts := newRepo(t)
		ctx := context.Background()

		created, err := repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "cardiology"})
		require.NoError(t, err)

		_, err = repo.Get(ctx, accounts.Stranger, created.ID)
		require.ErrorIs(t, err, domain.ErrNotFound)

		_, err = repo.Get(ctx, accounts.Owner, "does-not-exist")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("update renames without duplicating the row", func(t *testing.T) {
		t.Parallel()

		repo, accounts := newRepo(t)
		ctx := context.Background()

		created, err := repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "cardiology"})
		require.NoError(t, err)

		created.Name = "cardio"
		updated, err := repo.Update(ctx, created)
		require.NoError(t, err)
		assert.Equal(t, "cardio", updated.Name)
		assert.Equal(t, created.ID, updated.ID)

		page, err := repo.List(ctx, accounts.Owner, svc.Query{Limit: 10})
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "cardio", page.Items[0].Name)
	})

	t.Run("delete removes the row", func(t *testing.T) {
		t.Parallel()

		repo, accounts := newRepo(t)
		ctx := context.Background()

		created, err := repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "cardiology"})
		require.NoError(t, err)

		require.NoError(t, repo.Delete(ctx, accounts.Owner, created.ID))

		_, err = repo.Get(ctx, accounts.Owner, created.ID)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("delete of another owner's tag is not found", func(t *testing.T) {
		t.Parallel()

		repo, accounts := newRepo(t)
		ctx := context.Background()

		created, err := repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "cardiology"})
		require.NoError(t, err)

		err = repo.Delete(ctx, accounts.Stranger, created.ID)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("list is scoped to the owner", func(t *testing.T) {
		t.Parallel()

		repo, accounts := newRepo(t)
		ctx := context.Background()

		_, err := repo.Create(ctx, dtag.Tag{OwnerID: accounts.Owner, Name: "cardiology"})
		require.NoError(t, err)
		_, err = repo.Create(ctx, dtag.Tag{OwnerID: accounts.Stranger, Name: "school forms"})
		require.NoError(t, err)

		page, err := repo.List(ctx, accounts.Owner, svc.Query{Limit: 10})
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "cardiology", page.Items[0].Name)
	})
}
