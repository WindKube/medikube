package pb_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
)

// T221a, FR-040's third clause and the tenth of the ten action values
// data-model §3 declares this phase writes.
//
// A PocketBase superuser is the break-glass credential and not a MediKube role:
// it bypasses every collection rule and its only interface is the admin UI. The
// moment one of those sessions BEGINS is therefore an event worth recording in
// its own right. The credential separation and the boot warning are other
// tasks' (T055–T059, T064/T065); this file is only about the row.

func TestASuperuserSessionWritesExactlyOneAdminSessionRow(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeSuperuserSignIn, "", credentials(fixtureSuperuserEmail, fixturePassword))
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	rows := trail.of(audit.ActionAdminSession)
	require.Lenf(t, rows, 1, "%d admin_session rows for one superuser sign-in", len(rows))

	assert.Equal(t, audit.ActorKindSuperuser, rows[0].ActorKind, "the row does not say a superuser did it")
	assert.Equal(t, audit.TargetKindUser, rows[0].TargetKind)
	assert.Equal(t, h.recordID(t, core.CollectionNameSuperusers, fixtureSuperuserEmail), rows[0].TargetID)
	assert.NotEmpty(t, rows[0].RequestID)
	assert.NoError(t, rows[0].Validate())

	// The actor RELATION points at the account collection and a superuser has
	// no row in it, so an id here would be a dangling reference the column
	// itself refuses. actor_kind is what still says who it was — the same
	// property that lets an account_delete row outlive its actor (D-22).
	assert.Empty(t, rows[0].ActorID)

	// The two bindings are tagged by collection and the tags isolate: a
	// superuser's session is not a sign-in to MediKube and is not recorded as
	// one.
	assert.Empty(t, trail.of(audit.ActionLogin),
		"a superuser session was also recorded as an ordinary sign-in")
}

// The other half, and the one that makes the assertion above mean anything: an
// ordinary sign-in writes NO admin_session row. Without it, a binding that
// recorded every authentication as an admin session would pass the test above.
func TestAnOrdinarySignInOpensNoAdminSession(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, fixturePassword))
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	assert.Empty(t, trail.of(audit.ActionAdminSession),
		"an ordinary person's sign-in was recorded as an admin session")
	assert.Len(t, trail.of(audit.ActionLogin), 1)
}

// The renewal trap from the superuser's side: an administrator refreshing a
// token must not write a second admin_session row, or the trail says a
// break-glass credential was used every hour a tab was left open.
func TestASuperuserRenewingATokenOpensNoSecondAdminSession(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeSuperuserSignIn, "", credentials(fixtureSuperuserEmail, fixturePassword))
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	res = h.do(t, http.MethodPost, nativeSuperuserRefres, h.superuserToken(t), "{}")
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	assert.Len(t, trail.of(audit.ActionAdminSession), 1, "a renewal was recorded as a second admin session")
}

// A refused superuser sign-in opens nothing. `admin_session` means a session
// BEGAN, and a row for an attempt that did not would make the trail unreadable
// at exactly the moment somebody is reading it to find out whether the
// break-glass credential was used.
func TestARefusedSuperuserSignInOpensNoAdminSession(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeSuperuserSignIn, "", credentials(fixtureSuperuserEmail, "not-the-password"))
	require.NotEqual(t, http.StatusOK, res.Status)

	assert.Empty(t, trail.of(audit.ActionAdminSession))

	// Nor is it MediKube's login_failed: that row is about an account in the
	// account collection, and a superuser is deliberately not one (FR-040).
	assert.Empty(t, trail.of(audit.ActionLoginFailed))
}
