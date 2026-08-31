package access_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainaccess "medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/service/access"
)

// MediKube's one authorization checkpoint, tested where it decides.
//
// It shipped with no test file at all, and the reason that was invisible is the
// reason this file is separate from every other suite: internal/store's owner
// predicate refuses the same stranger a second time, independently, so the
// checkpoint can be deleted outright and every HTTP test — the ownership matrix
// included — stays green. Two layers, one guard between them, is one layer.
//
// Nothing below touches a database, an HTTP request or the repository. The
// owner lookup is a hand-written fake, so what fails here can only be the
// decision itself.

const (
	ownerID    = "mkacctamara0001"
	strangerID = "mkacctboris0001"
	recordID   = "mkmedamara00001"
	requestID  = "01K3Q8Z0000000000000000000"

	// undeclared is a kind no build serves. A checkpoint that answered for one
	// would be deciding about records it cannot name.
	undeclared kind.Kind = "not_a_kind"
)

// fakeOwners is the consumer-declared Owners port, by hand.
//
// It answers from a map and it distinguishes the two failures the checkpoint
// treats differently: a miss, which is a grant, and a lookup that could not be
// made, which is not.
type fakeOwners struct {
	owners map[string]string
	fail   error

	calls int
}

func (f *fakeOwners) Owner(_ context.Context, _ kind.Kind, id string) (string, error) {
	f.calls++

	if f.fail != nil {
		return "", f.fail
	}

	owner, found := f.owners[id]
	if !found {
		return "", domain.ErrNotFound
	}

	return owner, nil
}

func owners() *fakeOwners {
	return &fakeOwners{owners: map[string]string{recordID: ownerID}}
}

func actor(userID string) domainaccess.Actor {
	return domainaccess.Actor{UserID: userID, Role: identity.RoleUser, RequestID: requestID}
}

func superuser() domainaccess.Actor {
	return domainaccess.Actor{UserID: "mksuperadmin001", IsSuperuser: true, RequestID: requestID}
}

func checkpoint(t *testing.T, resolve access.Owners) *access.Authorizer {
	t.Helper()

	authorizer, err := access.New(resolve)
	require.NoError(t, err)

	return authorizer
}

// The central decision, for one addressed record. The owner leg and the
// stranger leg are the same call with one field changed, which is what makes
// the comparison the thing under test rather than the plumbing around it.
func TestTheCheckpointGrantsTheOwnerAndNobodyElse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		actor domainaccess.Actor
		kind  kind.Kind
		want  domainaccess.Permission
	}{
		{
			name:  "the owner holds everything over their own record",
			actor: actor(ownerID),
			kind:  kind.Medication,
			want:  domainaccess.PermOwn,
		},
		{
			// The one that matters. Nothing else in the repository fails when
			// this comparison is deleted.
			name:  "a signed-in stranger holds nothing over somebody else's record",
			actor: actor(strangerID),
			kind:  kind.Medication,
		},
		{
			name:  "an unauthenticated caller holds nothing",
			actor: domainaccess.Anonymous(requestID),
			kind:  kind.Medication,
		},
		{
			// FR-040 and data-model §1: the break-glass credential reads data
			// through the audited admin UI, and MediKube's own routes are not
			// a second unaudited way in.
			name:  "a PocketBase superuser is not a MediKube role and holds nothing",
			actor: superuser(),
			kind:  kind.Medication,
		},
		{
			name:  "a kind this build does not declare is refused",
			actor: actor(ownerID),
			kind:  undeclared,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			grant, err := checkpoint(t, owners()).
				Record(t.Context(), testCase.actor, testCase.kind, recordID, domainaccess.PermView)

			require.NoError(t, err)
			assert.Equal(t, testCase.want, grant.Level)

			if testCase.want == 0 {
				assert.False(t, grant.Allows(domainaccess.PermView),
					"a refusal that satisfies the lowest rung of the ladder is not a refusal")
			}
		})
	}
}

// The kind checkpoint answers the calls that name no record — a list, a create
// — and the same ladder applies to it.
func TestTheKindCheckpointAnswersTheCallsThatNameNoRecord(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		actor domainaccess.Actor
		kind  kind.Kind
		want  domainaccess.Permission
	}{
		{
			name:  "a signed-in person reaches their own records of a declared kind",
			actor: actor(ownerID),
			kind:  kind.Medication,
			want:  domainaccess.PermOwn,
		},
		{
			name:  "an unauthenticated caller reaches nothing",
			actor: domainaccess.Anonymous(requestID),
			kind:  kind.Medication,
		},
		{
			name:  "a PocketBase superuser reaches nothing",
			actor: superuser(),
			kind:  kind.Medication,
		},
		{
			name:  "an undeclared kind is refused",
			actor: actor(ownerID),
			kind:  undeclared,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			resolve := owners()

			grant, err := checkpoint(t, resolve).Kind(t.Context(), testCase.actor, testCase.kind, domainaccess.PermView)

			require.NoError(t, err)
			assert.Equal(t, testCase.want, grant.Level)
			assert.Zero(t, resolve.calls, "the kind checkpoint names no record and must resolve no owner")
		})
	}
}

// Research D-20. A record that is not there is a miss for the repository to
// report, not a refusal for the checkpoint to write into the audit trail —
// otherwise every mistyped identifier becomes an attempt nobody made, and a
// genuine miss becomes indistinguishable from a denial in the one place that
// can still tell them apart.
func TestAnIdentifierThatIsNotThereIsGrantedRatherThanRefused(t *testing.T) {
	t.Parallel()

	grant, err := checkpoint(t, owners()).
		Record(t.Context(), actor(ownerID), kind.Medication, "mkmednosuchrow1", domainaccess.PermView)

	require.NoError(t, err)
	assert.Equal(t, domainaccess.PermOwn, grant.Level)
}

// The defensive branch, and it is only reachable because internal/store now
// distinguishes "no such row" from "I could not find out". While every failure
// of the owner lookup arrived as domain.ErrNotFound this branch was dead code
// and the case above answered for it — which composed into a full grant on a
// stranger's record for the duration of any database failure.
func TestAnOwnerLookupThatCouldNotAnswerIsReportedAndNotGranted(t *testing.T) {
	t.Parallel()

	broken := errors.New("the owner lookup could not answer")

	resolve := owners()
	resolve.fail = broken

	grant, err := checkpoint(t, resolve).
		Record(t.Context(), actor(strangerID), kind.Medication, recordID, domainaccess.PermView)

	require.ErrorIs(t, err, broken, "a failed owner lookup was not reported as a failure")
	assert.Zero(t, grant.Level, "a checkpoint that could not answer granted something")
	assert.False(t, grant.Allows(domainaccess.PermView))
	assert.NotErrorIs(t, err, domain.ErrNotFound,
		"the failure was reported as a miss, which the caller above answers with a full grant")
}

// A checkpoint with no way to resolve an owner would grant or refuse everything
// alike, so it is a construction failure rather than a runtime one.
func TestACheckpointWiredWithoutAnOwnerLookupIsRefused(t *testing.T) {
	t.Parallel()

	authorizer, err := access.New(nil)

	require.Error(t, err)
	assert.Nil(t, authorizer)
}
