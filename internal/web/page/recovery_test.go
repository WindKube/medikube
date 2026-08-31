package page_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web/page"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
)

// T223n. The three recovery pages through the whole edge.
//
// The state each of them renders is a question about a real link and a real
// instance, so it is asked of a real one: the tokens below are minted by
// PocketBase from the seeded accounts, and the dead ones are dead for the
// reason a person's would be — the key underneath them moved, or the address
// was already confirmed.

// withMail turns outgoing mail on for this instance. tests.TestApp swaps in a
// mailer that always succeeds, so this switches the SETTING the pages read
// rather than the transport, which is the one an instance can answer before it
// tries to send (FR-076).
func (b *browser) withMail(enabled bool) *browser {
	b.t.Helper()

	settings := b.app.Settings()
	settings.SMTP.Enabled = enabled
	require.NoError(b.t, b.app.Save(settings))

	return b
}

func account(t *testing.T, b *browser, email string) *core.Record {
	t.Helper()

	record, err := b.app.FindAuthRecordByEmail(store.AccountCollection, email)
	require.NoError(t, err)

	return record
}

func recoveryLink(t *testing.T, b *browser, email string) string {
	t.Helper()

	token, err := account(t, b, email).NewPasswordResetToken()
	require.NoError(t, err)

	return "/reset-password/" + token
}

func confirmationLink(t *testing.T, b *browser, email string) string {
	t.Helper()

	token, err := account(t, b, email).NewVerificationToken()
	require.NoError(t, err)

	return "/verify-email/" + token
}

// FR-076 from the page's side, and the counterpart of the sign-up page's
// closed-registration test: without the mailable control, every assertion here
// would pass on a page that explained itself whatever the instance could do.
func TestTheRecoveryPageAsksForAnAddressOnlyWhenTheInstanceCanSendOne(t *testing.T) {
	t.Parallel()

	path := accountRoutes(t)[page.OpForgotPasswordPage].SmokeURL
	control := attribute("id", ids.Field(ids.ForgotPasswordForm, auth.FieldEmail))

	t.Run("no outgoing mail, which is the default every deployment gets", func(t *testing.T) {
		t.Parallel()

		status, _, body := newBrowser(t).anonymous().get(path)

		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, attribute("id", auth.MailUnconfiguredID))
		assert.NotContains(t, body, control,
			"an instance with no mail asked for an address it could do nothing with")

		assertLandmark(t, body, accountRoutes(t)[page.OpForgotPasswordPage].Landmark)
	})

	t.Run("outgoing mail configured", func(t *testing.T) {
		t.Parallel()

		rig := newBrowser(t).withMail(true)

		status, _, body := rig.anonymous().get(path)

		require.Equal(t, http.StatusOK, status, body)
		assert.NotContains(t, body, attribute("id", auth.MailUnconfiguredID))
		assert.Contains(t, body, control)
	})
}

// contracts/pages.md's deliberately invalid smoke tokens. Both pages answer
// 200 with the dead-link state inside their own landmark — a 4xx here would be
// an error view, which is a page without this landmark, which is a page the
// browser gate cannot check (FR-074).
func TestTheSmokeTokensAnswerTwoHundredWithTheDeadLinkStateInsideTheLandmark(t *testing.T) {
	t.Parallel()

	routes := accountRoutes(t)
	rig := newBrowser(t).anonymous()

	for _, opID := range []string{page.OpResetPasswordPage, page.OpVerifyEmailPage} {
		t.Run(opID, func(t *testing.T) {
			route := routes[opID]

			status, _, body := rig.get(route.SmokeURL)

			require.Equal(t, http.StatusOK, status,
				"the smoke URL the browser gate opens does not answer: %s", body)
			assert.Contains(t, body, attribute("id", auth.LinkDeadID))
			assertLandmark(t, body, route.Landmark)
		})
	}
}

// The page reads the LINK, not the shape of the token. A live recovery link
// offers the form; the same link after the key underneath it has moved — which
// is what setting a password does — offers the explanation instead.
func TestALiveRecoveryLinkOffersTheFormAndASpentOneExplainsItself(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t).withMail(true)
	link := recoveryLink(t, rig, testsupport.AccountAEmail)
	control := attribute("id", ids.Field(ids.NewPasswordForm, auth.FieldPassword))

	status, _, live := rig.anonymous().get(link)

	require.Equal(t, http.StatusOK, status, live)
	assert.Contains(t, live, control)
	assert.Contains(t, live, attribute("id", auth.NewPasswordRulesID),
		"the rules are published before the person chooses (FR-004, FR-074)")
	assert.NotContains(t, live, attribute("id", auth.LinkDeadID))

	// Spent, exactly as using it would spend it: a reset token is signed with
	// the account's own key, and the password write moves that key.
	record := account(t, rig, testsupport.AccountAEmail)
	record.RefreshTokenKey()
	require.NoError(t, rig.app.Save(record))

	status, _, spent := rig.anonymous().get(link)

	require.Equal(t, http.StatusOK, status, spent)
	assert.Contains(t, spent, attribute("id", auth.LinkDeadID))
	assert.NotContains(t, spent, control,
		"a spent link still offered a password control it was always going to refuse")
}

// FR-075, and the second-use rule the service enforces. Account C is seeded
// with an unconfirmed address, so the working state is one the fixture walks
// through; account A's is confirmed already, which makes its link a spent one
// even though it still resolves.
func TestALiveConfirmationLinkOffersTheControlAndAConfirmedAddressExplainsItself(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t).withMail(true)
	control := attribute("id", ids.Field(ids.ConfirmAddressForm, auth.FieldToken))

	status, _, live := rig.anonymous().get(confirmationLink(t, rig, testsupport.AccountCEmail))

	require.Equal(t, http.StatusOK, status, live)
	assert.Contains(t, live, control)
	assert.NotContains(t, live, attribute("id", auth.LinkDeadID))

	status, _, confirmed := rig.anonymous().get(confirmationLink(t, rig, testsupport.AccountAEmail))

	require.Equal(t, http.StatusOK, status, confirmed)
	assert.Contains(t, confirmed, attribute("id", auth.LinkDeadID),
		"a link for an address that is already confirmed is a spent link (FR-075)")
	assert.NotContains(t, confirmed, control)
}

// The three pages are reachable with no session at all, which is the whole
// point of them: the person who needs recovery is the person who cannot sign
// in, and the person following a confirmation link may be on another device.
func TestTheRecoveryPagesAreReachableWithNoSession(t *testing.T) {
	t.Parallel()

	routes := accountRoutes(t)
	rig := newBrowser(t).anonymous()

	for _, opID := range []string{page.OpForgotPasswordPage, page.OpResetPasswordPage, page.OpVerifyEmailPage} {
		t.Run(opID, func(t *testing.T) {
			status, _, body := rig.get(routes[opID].SmokeURL)

			require.Equal(t, http.StatusOK, status, body)
		})
	}
}
