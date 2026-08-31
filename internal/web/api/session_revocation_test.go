package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storeidentity "medikube/internal/store/identity"
	"medikube/internal/testsupport"
)

// T200 and FR-010, against a real instance and a real token: after a password
// change, every session issued before it stops working — including the ones
// nothing asked about.
//
// This is the ordinary-request half. The other two halves are already closed
// and this one is deliberately the same mechanism seen from a third side:
//
//   - internal/service/identity's fakes and identitytest's contract suite prove
//     the service reaches SetPassword and that both implementations rotate;
//   - internal/web/stream/revocation_test.go proves an already-open stream ends;
//   - this proves the next ordinary request is refused, through BOTH transports
//     — the API client's bearer token and the browser's cookie.
//
// The transports need separate assertions because they are separate code paths
// as far as the token is concerned: the cookie is translated into the header by
// internal/web's session middleware, and a browser that kept working after a
// password change while curl was correctly refused is exactly the shape of
// outage a header-only suite cannot see.

func TestAPasswordChangeRefusesEverySessionIssuedBeforeIt(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	// One account, two places it is signed in: the browser it is about to
	// change the password in, and the phone it left on the train.
	laptop := testsupport.UserToken(t, caller.app, testsupport.AccountAEmail)
	phone := testsupport.UserToken(t, caller.app, testsupport.AccountAEmail)
	stranger := testsupport.UserToken(t, caller.app, testsupport.AccountBEmail)

	require.Equal(t, http.StatusOK, byCookie(caller, laptop).Status,
		"the session was not working before the change, so nothing below proves it stopped")
	require.Equal(t, http.StatusOK, byHeader(caller, phone).Status)
	require.Equal(t, http.StatusOK, byCookie(caller, stranger).Status)

	record, err := caller.app.FindAuthRecordByEmail(usersCollection, testsupport.AccountAEmail)
	require.NoError(t, err)

	before := record.TokenKey()

	// The password is changed through MediKube's own credential adapter, which
	// is the only path a password change takes: internal/store/identity's
	// SetPassword sets it on the record and saves, and PocketBase
	// re-randomises tokenKey inside that save (core/record_model.go:1448-1453).
	// An adapter that wrote the hash with a raw UPDATE would change the
	// credential and leave every stolen session alive, and no response
	// anywhere would differ.
	authenticator, err := storeidentity.NewAuthenticator(caller.app)
	require.NoError(t, err)
	require.NoError(t, authenticator.SetPassword(t.Context(), testsupport.AccountAID,
		"a-quite-different-"+testsupport.Password))

	record, err = caller.app.FindAuthRecordByEmail(usersCollection, testsupport.AccountAEmail)
	require.NoError(t, err)
	require.NotEqual(t, before, record.TokenKey(),
		"the password change did not rotate the token key, so no session was revoked")

	for _, tc := range []struct {
		name     string
		response response
	}{
		{name: "the browser it was changed in", response: byCookie(caller, laptop)},
		{name: "the phone on the train", response: byHeader(caller, phone)},
		{name: "the phone, if it were a browser", response: byCookie(caller, phone)},
		{name: "the browser, if it were an API client", response: byHeader(caller, laptop)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, http.StatusUnauthorized, tc.response.Status, tc.response.Body)
			assert.Equal(t, "unauthenticated", tc.response.envelope(t).Error.Code)
		})
	}

	// FR-010's second clause: the person who made the change stays signed in
	// where they made it. The token is minted from the SAVED record, which is
	// the ordering contracts/account.md pins — rotate, save, mint, set the
	// cookie — and it is what makes this a revocation rather than a lockout.
	assert.Equal(t, http.StatusOK, byCookie(caller, mustToken(t, record)).Status)

	// Nobody else's session is touched. Rotating the collection's own auth
	// secret would satisfy every assertion above and sign out every account on
	// the instance, which is a denial of service wearing a fix's clothes.
	assert.Equal(t, http.StatusOK, byCookie(caller, stranger).Status,
		"changing one account's password signed another account out")
}

// The other direction, and it needs its own test because the cheapest way to
// pass the one above is to rotate on every save. FR-010 is about the password;
// a person who changes their display name or their theme must stay signed in,
// in every browser they left open.
func TestAProfileChangeLeavesEverySessionAlone(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)
	token := testsupport.UserToken(t, caller.app, testsupport.AccountAEmail)

	require.Equal(t, http.StatusOK, byCookie(caller, token).Status)

	record, err := caller.app.FindAuthRecordByEmail(usersCollection, testsupport.AccountAEmail)
	require.NoError(t, err)

	before := record.TokenKey()

	const renamed = "Amara O."

	record.Set("name", renamed)
	require.NoError(t, caller.app.Save(record))

	stored, err := caller.app.FindRecordById(usersCollection, testsupport.AccountAID)
	require.NoError(t, err)
	require.Equal(t, renamed, stored.GetString("name"),
		"the profile change did not reach the database, so the session below survived a write that never happened")
	require.Equal(t, before, stored.TokenKey())

	assert.Equal(t, http.StatusOK, byCookie(caller, token).Status,
		"changing a display name signed the person out of every browser they had open")
	assert.Equal(t, http.StatusOK, byHeader(caller, token).Status)
}
