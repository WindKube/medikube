package access_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainaccess "medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
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

// superuserHoldingTheOwnersAccount is the leg that makes the superuser refusal
// EXPLICIT rather than accidental (T224).
//
// The refusal of a superuser whose id is some other string is indistinguishable
// from the refusal of any stranger: delete `!actor.IsSuperuser` from reachable
// and the owner comparison refuses them anyway, and every assertion above still
// passes. This actor is a superuser session carrying the owner's own account
// id, so the only thing that can refuse it is the flag itself.
func superuserHoldingTheOwnersAccount() domainaccess.Actor {
	return domainaccess.Actor{UserID: ownerID, IsSuperuser: true, RequestID: requestID}
}

// ladder is access.Permissions(), guarded.
//
// Every table below ranges over it, so a ladder that came back empty would run
// no subtest at all and pass by asserting nothing — true by absence rather than
// true by design. The floor is the three rungs internal/domain/access publishes;
// a phase that adds one widens this and a phase that loses one fails here.
func ladder(t *testing.T) []domainaccess.Permission {
	t.Helper()

	permissions := domainaccess.Permissions()
	require.GreaterOrEqualf(t, len(permissions), 3,
		"access.Permissions() published %d rungs: every table here would range over nothing", len(permissions))

	return permissions
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
			// The same refusal made explicit. See
			// superuserHoldingTheOwnersAccount: the case above is answered by
			// the owner comparison whether or not the flag is read at all.
			name:  "a PocketBase superuser is refused on the owner's own account id",
			actor: superuserHoldingTheOwnersAccount(),
			kind:  kind.Medication,
		},
		{
			name:  "a kind this build does not declare is refused",
			actor: actor(ownerID),
			kind:  undeclared,
		},
	}

	for _, testCase := range cases {
		for _, need := range ladder(t) {
			t.Run(testCase.name+"/"+need.String(), func(t *testing.T) {
				t.Parallel()

				grant, err := checkpoint(t, owners()).
					Record(t.Context(), testCase.actor, testCase.kind, recordID, need)

				require.NoError(t, err)
				assert.Equal(t, testCase.want, grant.Level)

				// Read at every rung and not only at the lowest. A refusal is
				// a refusal of `view` and of `own` alike, and the owner's
				// grant has to answer the whole ladder or a delete would be
				// refused to the person who owns the record.
				assert.Equalf(t, testCase.want != 0, grant.Allows(need),
					"the grant answers %s wrongly", need)
			})
		}
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
			name:  "a PocketBase superuser carrying an account id reaches nothing either",
			actor: superuserHoldingTheOwnersAccount(),
			kind:  kind.Medication,
		},
		{
			name:  "an undeclared kind is refused",
			actor: actor(ownerID),
			kind:  undeclared,
		},
	}

	for _, testCase := range cases {
		for _, need := range ladder(t) {
			t.Run(testCase.name+"/"+need.String(), func(t *testing.T) {
				t.Parallel()

				resolve := owners()

				grant, err := checkpoint(t, resolve).Kind(t.Context(), testCase.actor, testCase.kind, need)

				require.NoError(t, err)
				assert.Equal(t, testCase.want, grant.Level)
				assert.Equalf(t, testCase.want != 0, grant.Allows(need),
					"the grant answers %s wrongly", need)
				assert.Zero(t, resolve.calls, "the kind checkpoint names no record and must resolve no owner")
			})
		}
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

// T239. The checkpoint has two doors and no third one.
//
// "One checkpoint" is not a property of any single decision below — every one
// of them could be right while a third method decided something on its own
// terms. This is the claim stated where it can be broken: a new exported method
// on the Authorizer is a new place authorization happens, and it fails here
// rather than in review.
//
// Kind and Record are the two because a list and a create name no record. The
// day a third is genuinely needed, this line is where the case for it is made.
func TestTheCheckpointHasThreeDoorsAndNoFourthOne(t *testing.T) {
	t.Parallel()

	checkpointType := reflect.TypeOf(checkpoint(t, owners()))

	doors := make([]string, 0, checkpointType.NumMethod())
	for index := range checkpointType.NumMethod() {
		doors = append(doors, checkpointType.Method(index).Name)
	}

	// Patient joined in phase 002: a person is not a kind.Kind, so it is a
	// third door rather than a third argument to Record — and it is still the
	// whole list. A caller reaches one of these three and nothing else decides.
	assert.Equal(t, []string{"Kind", "Patient", "Record"}, doors,
		"the authorization checkpoint has grown a door: every service reaches these three and nothing else decides")
}

// T239. In THIS phase the ladder has one rung, and that is deliberate.
//
// Both methods ignore the level they were asked for — the parameters are `_` —
// because the owner is the only actor there is: they hold everything over their
// own records and nobody else holds anything. So the answer is the same at
// every rung, and this is the assertion that says so out loud rather than
// leaving it as an unexplained `_`.
//
// It is written to FAIL when phase 005's shares make the rungs differ. That is
// the point: the rungs starting to matter is a change to what this checkpoint
// resolves, and it should not be possible to make it quietly.
func TestThisPhasesLadderHasOneRung(t *testing.T) {
	t.Parallel()

	answered := map[domainaccess.Permission]domainaccess.Permission{}

	for _, need := range ladder(t) {
		record, err := checkpoint(t, owners()).Record(t.Context(), actor(ownerID), kind.Medication, recordID, need)
		require.NoError(t, err)

		reach, err := checkpoint(t, owners()).Kind(t.Context(), actor(ownerID), kind.Medication, need)
		require.NoError(t, err)

		assert.Equalf(t, record.Level, reach.Level,
			"the two doors answered %s differently, so which one a caller went through is now an authorization decision", need)

		answered[need] = record.Level
	}

	require.Len(t, answered, len(ladder(t)), "the ladder has a repeated rung, so this ranged over fewer than it looks")

	for need, level := range answered {
		assert.Equalf(t, domainaccess.PermOwn, level,
			"the owner was answered %s for %s: this phase's ladder has one rung and the owner holds it", level, need)
	}
}

// T033/T034. Patient anchors on the person rather than on a kind, and it is
// tested separately from Kind and Record for exactly that reason: it answers
// with a typed error on every refusal, rather than a Grant of zero value, and
// every refusal it produces writes exactly one audit row.

const patientID = "mkptamara00001"

type fakePatientOwners struct {
	owners map[string]string
	fail   error
}

func (f *fakePatientOwners) PatientOwner(_ context.Context, id string) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}

	owner, found := f.owners[id]
	if !found {
		return "", domain.ErrNotFound
	}

	return owner, nil
}

func patientOwners() *fakePatientOwners {
	return &fakePatientOwners{owners: map[string]string{patientID: ownerID}}
}

type fakeAuditor struct {
	rows []domainaudit.Event
}

func (f *fakeAuditor) Record(_ context.Context, event domainaudit.Event) error {
	f.rows = append(f.rows, event)

	return nil
}

func patientCheckpoint(t *testing.T, owners access.PatientOwners, auditor access.Auditor) *access.Authorizer {
	t.Helper()

	authorizer, err := access.New(&fakeOwners{owners: map[string]string{}}, access.WithPatients(owners, auditor))
	require.NoError(t, err)

	return authorizer
}

func TestPatientGrantsTheOwnerAndNobodyElse(t *testing.T) {
	t.Parallel()

	auditor := &fakeAuditor{}
	grant, err := patientCheckpoint(t, patientOwners(), auditor).
		Patient(t.Context(), actor(ownerID), patientID, domainaccess.PermOwn)

	require.NoError(t, err)
	assert.Equal(t, domainaccess.PermOwn, grant.Level)
	assert.Empty(t, auditor.rows, "the owner's own reach must write no refusal row")
}

func TestPatientRefusesAStrangerAsANotFoundAndAuditsIt(t *testing.T) {
	t.Parallel()

	auditor := &fakeAuditor{}
	grant, err := patientCheckpoint(t, patientOwners(), auditor).
		Patient(t.Context(), actor(strangerID), patientID, domainaccess.PermOwn)

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.NotErrorIs(t, err, domain.ErrForbidden,
		"FR-042: a patient's existence is itself PHI, so a stranger is refused exactly as a miss")
	assert.Zero(t, grant.Level)

	require.Len(t, auditor.rows, 1)
	row := auditor.rows[0]
	assert.Equal(t, domainaudit.ActionAccessDenied, row.Action)
	assert.Equal(t, domainaudit.TargetKindPatient, row.TargetKind)
	assert.Equal(t, patientID, row.TargetID)
	assert.Equal(t, patientID, row.PatientID)
	assert.Equal(t, strangerID, row.ActorID)
	assert.Equal(t, requestID, row.RequestID)
}

func TestPatientRefusesAnIdentifierThatIsNotThereTheSameWayAndAuditsIt(t *testing.T) {
	t.Parallel()

	auditor := &fakeAuditor{}
	grant, err := patientCheckpoint(t, patientOwners(), auditor).
		Patient(t.Context(), actor(ownerID), "mkptnosuchrow01", domainaccess.PermOwn)

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Zero(t, grant.Level)
	require.Len(t, auditor.rows, 1)
	assert.Equal(t, "mkptnosuchrow01", auditor.rows[0].TargetID)
}

func TestPatientRefusesAnAnonymousCallerAsUnauthenticatedAndAuditsIt(t *testing.T) {
	t.Parallel()

	auditor := &fakeAuditor{}
	grant, err := patientCheckpoint(t, patientOwners(), auditor).
		Patient(t.Context(), domainaccess.Anonymous(requestID), patientID, domainaccess.PermOwn)

	require.ErrorIs(t, err, domain.ErrUnauthenticated)
	assert.Zero(t, grant.Level)
	require.Len(t, auditor.rows, 1)
	assert.Equal(t, patientID, auditor.rows[0].TargetID)
}

func TestPatientRefusesASuperuserAsANotFoundAndAuditsIt(t *testing.T) {
	t.Parallel()

	auditor := &fakeAuditor{}
	grant, err := patientCheckpoint(t, patientOwners(), auditor).
		Patient(t.Context(), superuserHoldingTheOwnersAccount(), patientID, domainaccess.PermOwn)

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Zero(t, grant.Level)
	assert.Len(t, auditor.rows, 1)
}

func TestPatientReportsAnOwnerLookupThatCouldNotAnswerAndDoesNotAuditIt(t *testing.T) {
	t.Parallel()

	broken := errors.New("the owner lookup could not answer")
	owners := patientOwners()
	owners.fail = broken

	auditor := &fakeAuditor{}
	grant, err := patientCheckpoint(t, owners, auditor).
		Patient(t.Context(), actor(strangerID), patientID, domainaccess.PermOwn)

	require.ErrorIs(t, err, broken)
	assert.Zero(t, grant.Level)
	assert.Empty(t, auditor.rows, "a failure to answer is not a refusal to audit")
}

func TestPatientRefusesConstructionWithNoAnchorWired(t *testing.T) {
	t.Parallel()

	grant, err := checkpoint(t, owners()).Patient(t.Context(), actor(ownerID), patientID, domainaccess.PermOwn)

	require.Error(t, err)
	assert.Zero(t, grant.Level)
}
