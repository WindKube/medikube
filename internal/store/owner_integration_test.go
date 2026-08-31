package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainaccess "medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/service/access"
	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// The owner lookup is the one read MediKube's authorization checkpoint makes,
// and the checkpoint answers domain.ErrNotFound with a FULL GRANT (research
// D-20). Those two facts are each correct and they compose into the failure
// this file exists to prevent: while every failure of this read was reported as
// ErrNotFound, a cancelled query or a locked database granted a stranger
// everything over a record they do not own.
//
// So the assertions below are as much about which error as about whether one.

func newOwners(t *testing.T) (*store.Owners, string) {
	t.Helper()

	app := testsupport.NewApp(t)

	owners, err := store.NewOwners(app)
	require.NoError(t, err)

	return owners, testsupport.NameOnlyMedicationID
}

func TestTheOwnerLookupAnswersTheAccountARecordBelongsTo(t *testing.T) {
	t.Parallel()

	owners, seeded := newOwners(t)

	owner, err := owners.Owner(t.Context(), kind.Medication, seeded)

	require.NoError(t, err)
	assert.Equal(t, testsupport.AccountAID, owner)
}

// A miss and only a miss is domain.ErrNotFound.
func TestOnlyAnEmptyResultIsReportedAsAMiss(t *testing.T) {
	t.Parallel()

	owners, _ := newOwners(t)

	cases := []struct {
		name   string
		kind   kind.Kind
		record string
	}{
		{
			name:   "an identifier that has never existed",
			kind:   kind.Medication,
			record: "mkmednosuchrow1",
		},
		{
			name:   "a kind this build does not declare",
			kind:   "not_a_kind",
			record: testsupport.NameOnlyMedicationID,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			owner, err := owners.Owner(t.Context(), testCase.kind, testCase.record)

			require.ErrorIs(t, err, domain.ErrNotFound)
			assert.Empty(t, owner)
		})
	}
}

// The failure that used to be a grant.
//
// A cancelled context stands in for every way the read can fail — a lock held
// past the timeout, a closed connection, a shutdown mid-request. None of them
// is "that record does not exist", and reporting them as one is what turned an
// outage into an authorization bypass.
func TestAReadThatCouldNotBeMadeIsNotReportedAsAMiss(t *testing.T) {
	t.Parallel()

	owners, seeded := newOwners(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	owner, err := owners.Owner(ctx, kind.Medication, seeded)

	require.Error(t, err)
	assert.Empty(t, owner)
	assert.NotErrorIs(t, err, domain.ErrNotFound,
		"a failed read was reported as a miss, and the checkpoint above answers a miss with a full grant")
	assert.ErrorIs(t, err, context.Canceled, "the failure was reported as something other than itself")
}

// The composition, wired the way cmd/medikube wires it: the real checkpoint
// over the real owner lookup, with no repository predicate anywhere in the
// path.
//
// This is the layer's own guard. internal/store/medication's owner predicate
// refuses the same stranger a second time and independently, which is why the
// checkpoint could be deleted outright with the whole suite staying green — so
// the checkpoint needs a test that reaches it and nothing else, and this is
// that test against a real database rather than a fake.
func TestTheCheckpointOverTheRealOwnerLookupRefusesAStrangerAndFailsClosed(t *testing.T) {
	t.Parallel()

	owners, seeded := newOwners(t)

	checkpoint, err := access.New(owners)
	require.NoError(t, err)

	stranger := domainaccess.Actor{UserID: testsupport.AccountBID, RequestID: "test-request"}
	owner := domainaccess.Actor{UserID: testsupport.AccountAID, RequestID: "test-request"}

	t.Run("the owner is granted", func(t *testing.T) {
		t.Parallel()

		grant, err := checkpoint.Record(t.Context(), owner, kind.Medication, seeded, domainaccess.PermView)

		require.NoError(t, err)
		assert.Equal(t, domainaccess.PermOwn, grant.Level)
	})

	t.Run("a stranger is refused", func(t *testing.T) {
		t.Parallel()

		grant, err := checkpoint.Record(t.Context(), stranger, kind.Medication, seeded, domainaccess.PermView)

		require.NoError(t, err)
		assert.Zero(t, grant.Level, "the checkpoint granted another account's record")
	})

	// The measured bypass: a cancelled owner lookup answered {Level:own} for
	// account B on account A's record, because the failure arrived as
	// ErrNotFound and research D-20 grants for a record that is not there.
	t.Run("a lookup that could not answer refuses and reports", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		grant, err := checkpoint.Record(ctx, stranger, kind.Medication, seeded, domainaccess.PermView)

		require.Error(t, err, "a checkpoint that could not read the owner answered anyway")
		assert.Zero(t, grant.Level, "a failed owner lookup granted another account's record")
		assert.False(t, grant.Allows(domainaccess.PermView))
	})
}
