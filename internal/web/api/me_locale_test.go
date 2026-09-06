package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// T016. contracts/account.md's updateMe against 007-localisation's own
// membership check (FR-001, US1-1): a shipped language is stored, one that is
// not is refused by its code, and a Datastar settings submit is answered in
// the language it just chose.

func TestPatchingTheLocaleToAShippedLanguageIsStoredAndReadBack(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).
		do(http.MethodPatch, meURL, body("locale", quoted("pl")), nil)

	require.Equal(t, http.StatusOK, answer.Status, answer.Body)
	assert.Equal(t, "pl", answer.me(t).Locale)

	stored, err := store.UserFromRecord(instance.stored(t, testsupport.AccountAEmail))
	require.NoError(t, err)
	assert.Equal(t, "pl", stored.Locale)
}

func TestPatchingTheLocaleToALanguageThisInstanceDoesNotShipIsRefused(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).
		do(http.MethodPatch, meURL, body("locale", quoted("xx")), nil)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	envelope := answer.envelope(t)
	require.Len(t, envelope.Error.Fields, 1)
	assert.Equal(t, "locale", envelope.Error.Fields[0].Field)
	assert.Equal(t, domain.CodeInvalidValue, envelope.Error.Fields[0].Code)

	stored, err := store.UserFromRecord(instance.stored(t, testsupport.AccountAEmail))
	require.NoError(t, err)
	assert.NotEqual(t, "xx", stored.Locale, "a refused patch changed the record anyway")
}

// A Datastar settings-form submit answers with the profile form re-rendered
// in the language it just chose, on the SAME response — not the next one,
// which is what proves the response is resolved from the just-updated
// account rather than from the session it started with (D-04).
func TestADatastarSettingsSubmitToPolishAnswersInPolishOnTheSameResponse(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).
		do(http.MethodPatch, meURL, body("locale", quoted("pl")), map[string]string{"Datastar-Request": "true"})

	require.Equal(t, http.StatusOK, answer.Status, answer.Body)
	assert.Contains(t, answer.Header.Get("Content-Type"), "text/html")
	assert.Contains(t, answer.Body, "Język", "the re-rendered form is not in Polish (settings.language.label)")
}
