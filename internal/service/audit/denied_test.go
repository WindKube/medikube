package audit

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainaccess "medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	serviceaccess "medikube/internal/service/access"
	"medikube/internal/service/medication"
	"medikube/internal/service/medication/medicationtest"
)

// Research D-20, composed. The two halves of the distinction are each asserted
// where they are decided — the checkpoint grants an identifier that is not
// there (access/authorizer_test.go), and the service writes no row when the
// repository misses (medication/service_test.go, against a fake checkpoint) —
// and neither of those tests would notice if the real checkpoint started
// refusing the miss instead. This is the assembled path, and what it watches is
// the trail: the real checkpoint, the real writer, and a store that holds the
// rows.
//
// The distinction has to survive here because the trail is the only place it
// survives at all. The caller is told the same thing either way, deliberately
// (FR-033), so if a genuine miss also wrote a row there would be nothing left
// anywhere that could tell an attempt from a typo — and an operator reading a
// thousand access_denied rows would have no way to know whether anybody had
// tried anything.

// The account doing the reaching, and the handle its rows correlate to. Its own
// identifiers, not a seeded fixture's: nothing here reaches a database.
const (
	callerID     = "mkdenycaller001"
	callerHandle = "0123456789abcdef0123456789abcdef"
)

// Identifiers no row was ever minted for. The fake repository mints
// `mkfakemedNNNNNN`, so nothing here can collide with a record that exists.
var neverExisted = []string{"mkguessed000001", "mkmistyped00001", "mkprobed0000001"}

// owners is the checkpoint's owner lookup, and the only thing in this file that
// knows whether a record is there at all. A miss is domain.ErrNotFound and not
// a refusal, which is the contract internal/store is held to.
type owners struct{ byID map[string]string }

func (o owners) Owner(_ context.Context, _ kind.Kind, recordID string) (string, error) {
	owner, exists := o.byID[recordID]
	if !exists {
		return "", fmt.Errorf("audit: no such record: %w", domain.ErrNotFound)
	}

	return owner, nil
}

func caller() domainaccess.Actor {
	return domainaccess.Actor{UserID: callerID, RequestID: callerHandle}
}

type refusals struct {
	service *medication.Service
	store   *trail

	// theirs are records that exist and belong to somebody else. They exist:
	// the row is in the repository and the checkpoint's lookup resolves it, so
	// the refusal below is a refusal about a record that is there, which is the
	// only kind of refusal there is.
	theirs []string
}

func newRefusals(t *testing.T) refusals {
	t.Helper()

	repository := medicationtest.NewRepository()
	resolve := owners{byID: map[string]string{}}

	names := []string{"Amoxicillin", "Metformin"}
	theirs := make([]string, 0, len(names))

	for _, name := range names {
		stored, err := repository.Create(t.Context(), clinical.Medication{
			OwnerID: medicationtest.StrangerID,
			Name:    name,
			Status:  clinical.TherapyStatusActive,
		})
		require.NoError(t, err)

		resolve.byID[stored.ID] = medicationtest.StrangerID
		theirs = append(theirs, stored.ID)
	}

	require.NotContains(t, theirs, neverExisted[0],
		"the repository minted an identifier this file also uses for a record that never existed")

	checkpoint, err := serviceaccess.New(resolve)
	require.NoError(t, err)

	store := newTrail()

	writer, err := New(store)
	require.NoError(t, err)

	service, err := medication.New(repository, checkpoint, writer)
	require.NoError(t, err)

	return refusals{service: service, store: store, theirs: theirs}
}

// reaches is one record-addressed call. The list and the create name no record,
// so neither has a miss to be told apart from a refusal.
var reaches = []struct {
	name string
	call func(t *testing.T, service *medication.Service, id string) error
}{
	{
		name: "get",
		call: func(t *testing.T, service *medication.Service, id string) error {
			_, err := service.Get(t.Context(), caller(), id)

			return err
		},
	},
	{
		name: "update",
		call: func(t *testing.T, service *medication.Service, id string) error {
			_, err := service.Update(t.Context(), caller(), id, "", medication.Patch{})

			return err
		},
	},
	{
		name: "delete",
		call: func(t *testing.T, service *medication.Service, id string) error {
			return service.Delete(t.Context(), caller(), id, "")
		},
	},
}

func TestARefusalReachesTheTrailAndAGenuineMissReachesNothing(t *testing.T) {
	t.Parallel()

	require.Len(t, reaches, 3, "the table has drifted from the record-addressed methods")

	for _, reach := range reaches {
		t.Run(reach.name, func(t *testing.T) {
			t.Parallel()

			refused := newRefusals(t)
			refusal := reach.call(t, refused.service, refused.theirs[0])

			missed := newRefusals(t)
			miss := reach.call(t, missed.service, neverExisted[0])

			// The premise. The caller is told the same thing either way, so
			// there is nothing in the answer for an operator to read and the
			// trail is the only record of which of the two happened.
			require.ErrorIs(t, refusal, domain.ErrNotFound,
				"a record that is somebody else's was answered as something other than a miss")
			require.ErrorIs(t, miss, domain.ErrNotFound)

			rows := refused.store.Rows()
			require.Len(t, rows, 1, "one refusal reached the trail %d times", len(rows))

			row := rows[0]
			assert.Equal(t, domainaudit.ActionAccessDenied, row.Action)
			assert.Equal(t, domainaudit.ActorKindUser, row.ActorKind)
			assert.Equal(t, domainaudit.TargetKindMedication, row.TargetKind)
			assert.Equal(t, callerID, row.ActorID)
			assert.Equal(t, refused.theirs[0], row.TargetID,
				"the row does not name the record that was reached for")
			assert.Equal(t, callerHandle, row.RequestID,
				"the row correlates to something other than the request that produced it")
			assert.NoError(t, row.Validate(), "the trail took a row the store would refuse")

			assert.Empty(t, missed.store.Rows(),
				"a record that never existed was recorded as an access denial, so every typo is now an attempt")
		})
	}
}

// The operational payoff, and the reason the count matters rather than the
// presence: the number of access_denied rows is the number of attempts somebody
// actually made. A probe that sprays identifiers leaves nothing behind, so a
// trail that is full is full of real reaches — and a trail that fills up is a
// signal rather than a caller with a stale bookmark.
func TestTheTrailHoldsTheAttemptsAndNotTheGuesses(t *testing.T) {
	t.Parallel()

	h := newRefusals(t)

	// Interleaved, because the two are one caller's single sequence of
	// requests: the trail is not sorting them afterwards, it is only ever
	// offered the ones the checkpoint refused.
	reached := []string{neverExisted[0], h.theirs[0], neverExisted[1], h.theirs[1], neverExisted[2]}

	for _, id := range reached {
		_, err := h.service.Get(t.Context(), caller(), id)
		require.ErrorIs(t, err, domain.ErrNotFound)
	}

	recorded := make([]string, 0, len(h.store.Rows()))
	for _, row := range h.store.Rows() {
		assert.Equal(t, domainaudit.ActionAccessDenied, row.Action)
		recorded = append(recorded, row.TargetID)
	}

	assert.Equal(t, h.theirs, recorded,
		"the trail records a different set of identifiers than the ones that were somebody else's")
}
