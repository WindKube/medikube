package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainaccess "medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/service/access"
	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// The patient-owner lookup is the one read MediKube's authorization checkpoint
// makes for a patient-anchored kind (research D-13), and the checkpoint
// answers domain.ErrNotFound with a FULL GRANT (research D-20). Those two facts
// are each correct and they compose into the failure this file exists to
// prevent: while every failure of this read was reported as ErrNotFound, a
// cancelled query or a locked database granted a stranger everything over a
// patient they do not own.
//
// So the assertions below are as much about which error as about whether one.
//
// This file tested store.Owners against kind.Medication before research D-13:
// medication moved off the owner anchor entirely, and Medication is the only
// kind.Kind this build declares, so that mechanism has no live target left to
// exercise it against. store.PatientOwners is what every registered kind
// authorizes through now, and it is the same shape.

func newPatientOwners(t *testing.T) (*store.PatientOwners, string) {
	t.Helper()

	app := testsupport.NewApp(t)

	owners, err := store.NewPatientOwners(app)
	require.NoError(t, err)

	return owners, testsupport.AccountAPatientSelfID
}

// noopAuditor discards every row. The checkpoint composition test below
// exercises Patient's grant/refuse decision, not what it writes.
type noopAuditor struct{}

func (noopAuditor) Record(context.Context, domainaudit.Event) error { return nil }

func TestThePatientOwnerLookupAnswersTheAccountAPatientBelongsTo(t *testing.T) {
	t.Parallel()

	owners, seeded := newPatientOwners(t)

	owner, err := owners.PatientOwner(t.Context(), seeded)

	require.NoError(t, err)
	assert.Equal(t, testsupport.AccountAID, owner)
}

// A miss and only a miss is domain.ErrNotFound.
func TestOnlyAnEmptyResultIsReportedAsAMiss(t *testing.T) {
	t.Parallel()

	owners, _ := newPatientOwners(t)

	owner, err := owners.PatientOwner(t.Context(), "mkptnosuchrow01")

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, owner)
}

// The failure that used to be a grant.
//
// A cancelled context stands in for every way the read can fail — a lock held
// past the timeout, a closed connection, a shutdown mid-request. None of them
// is "that patient does not exist", and reporting them as one is what turned an
// outage into an authorization bypass.
func TestAReadThatCouldNotBeMadeIsNotReportedAsAMiss(t *testing.T) {
	t.Parallel()

	owners, seeded := newPatientOwners(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	owner, err := owners.PatientOwner(ctx, seeded)

	require.Error(t, err)
	assert.Empty(t, owner)
	assert.NotErrorIs(t, err, domain.ErrNotFound,
		"a failed read was reported as a miss, and the checkpoint above answers a miss with a full grant")
	assert.ErrorIs(t, err, context.Canceled, "the failure was reported as something other than itself")
}

// The composition, wired the way cmd/medikube wires it: the real checkpoint
// over the real patient-owner lookup, with no repository predicate anywhere in
// the path.
//
// This is the layer's own guard. internal/store/medication's own predicate
// refuses the same stranger a second time and independently, which is why the
// checkpoint could be deleted outright with the whole suite staying green — so
// the checkpoint needs a test that reaches it and nothing else, and this is
// that test against a real database rather than a fake.
func TestTheCheckpointOverTheRealPatientOwnerLookupRefusesAStrangerAndFailsClosed(t *testing.T) {
	t.Parallel()

	app := testsupport.NewApp(t)

	owners, err := store.NewOwners(app)
	require.NoError(t, err)

	patientOwners, seeded := newPatientOwners(t)

	checkpoint, err := access.New(owners, access.WithPatients(patientOwners, noopAuditor{}))
	require.NoError(t, err)

	stranger := domainaccess.Actor{UserID: testsupport.AccountBID, RequestID: "test-request"}
	owner := domainaccess.Actor{UserID: testsupport.AccountAID, RequestID: "test-request"}

	t.Run("the owner is granted", func(t *testing.T) {
		t.Parallel()

		grant, err := checkpoint.Patient(t.Context(), owner, seeded, domainaccess.PermView)

		require.NoError(t, err)
		assert.Equal(t, domainaccess.PermOwn, grant.Level)
	})

	t.Run("a stranger is refused", func(t *testing.T) {
		t.Parallel()

		grant, err := checkpoint.Patient(t.Context(), stranger, seeded, domainaccess.PermView)

		require.ErrorIs(t, err, domain.ErrNotFound,
			"FR-042: a patient's existence is itself PHI, so a stranger is refused exactly as a miss")
		assert.Zero(t, grant.Level, "the checkpoint granted another account's patient")
	})

	// The measured bypass: a cancelled owner lookup answered {Level:own} for
	// account B on account A's patient, because the failure arrived as
	// ErrNotFound and research D-20 grants for a record that is not there.
	t.Run("a lookup that could not answer refuses and reports", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		grant, err := checkpoint.Patient(ctx, stranger, seeded, domainaccess.PermView)

		require.Error(t, err, "a checkpoint that could not read the owner answered anyway")
		assert.Zero(t, grant.Level, "a failed owner lookup granted another account's patient")
		assert.False(t, grant.Allows(domainaccess.PermView))
	})
}
