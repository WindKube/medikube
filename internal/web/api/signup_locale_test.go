package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T034 (FR-004, US2-3, US2-4). A sign-up carries the request's resolved
// language into the new account, and the account, not the browser, owns it
// from then on.
func TestSignUpCarriesTheRequestsLanguageIntoTheNewAccount(t *testing.T) {
	t.Parallel()

	document := body(
		"email", quoted(newAccountEmail),
		"name", quoted(newAccountName),
		"password", quoted(newAccountPassword),
	)

	t.Run("Accept-Language: pl creates the account with locale=pl", func(t *testing.T) {
		t.Parallel()

		instance := openRig(t)

		answer := instance.anonymous().do(http.MethodPost, registerURL, document, map[string]string{"Accept-Language": "pl"})
		require.Equal(t, http.StatusCreated, answer.Status, answer.Body)
		assert.Equal(t, "pl", answer.session(t).User.Locale)

		cookie := answer.sessionCookie(t)
		require.NotNil(t, cookie)

		firstPage := instance.anonymous().cookieGet("/", cookie.Value)
		require.Equal(t, http.StatusOK, firstPage.Status, firstPage.Body)
		assert.Contains(t, firstPage.Body, `lang="pl"`)

		t.Run("signing in from Accept-Language: en stays Polish", func(t *testing.T) {
			loginDocument := body("email", quoted(newAccountEmail), "password", quoted(newAccountPassword))

			login := instance.anonymous().do(http.MethodPost, loginURL, loginDocument, map[string]string{"Accept-Language": "en"})
			require.Equal(t, http.StatusOK, login.Status, login.Body)
			assert.Equal(t, "pl", login.session(t).User.Locale)

			englishBrowserCookie := login.sessionCookie(t)
			require.NotNil(t, englishBrowserCookie)

			page := instance.anonymous().cookieGet("/", englishBrowserCookie.Value)
			require.Equal(t, http.StatusOK, page.Status, page.Body)
			assert.Contains(t, page.Body, `lang="pl"`,
				"the account's own language must win over the browser that signed in")
		})
	})

	t.Run("no Accept-Language creates the account with the default locale", func(t *testing.T) {
		t.Parallel()

		instance := openRig(t)

		answer := instance.anonymous().post(registerURL, document)
		require.Equal(t, http.StatusCreated, answer.Status, answer.Body)
		assert.Equal(t, "en", answer.session(t).User.Locale)
	})
}
