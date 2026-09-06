package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web/views/ids"
)

// setCallerLocale stores locale on the account the caller is signed in as, so
// every subsequent request through it resolves messages in that language
// (resolveLocale's first source, ahead of Accept-Language).
func setCallerLocale(t *testing.T, c *caller, email, locale string) {
	t.Helper()

	record, err := c.app.FindAuthRecordByEmail("users", email)
	require.NoError(t, err)

	record.Set("locale", locale)
	require.NoError(t, c.app.Save(record))
}

// T031 (US1-4): a save the domain refuses — a condition marked resolved with
// no resolved_on — answers 422 whose fields[] is byte-identical whatever the
// account's language, because domain.FieldError.Message is never translated
// (errors.go's Fields = invalid.Fields, verbatim); only the envelope's own
// message and, on a Datastar re-render, the per-field explanation shown next to
// the control change with the account's locale.
func TestARefusedConditionSaveCarriesPolishExplanationsForTheSameFields(t *testing.T) {
	t.Parallel()

	conditionsURL := "/api/v1/records/" + kind.Condition.Segment()
	body := `{"patient":"` + testsupport.AccountAPatientSelfID + `","diagnosis":"Refused without a resolution date","status":"resolved"}`

	english := newCaller(t)

	englishAnswer := english.post(conditionsURL, body)
	require.Equal(t, http.StatusUnprocessableEntity, englishAnswer.Status, englishAnswer.Body)

	var englishEnvelope struct {
		Error struct {
			Message string              `json:"message"`
			Fields  []domain.FieldError `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(englishAnswer.rawBody, &englishEnvelope))
	require.NotEmpty(t, englishEnvelope.Error.Fields, "the refused save must name at least one field")

	polish := english.as(testsupport.AccountAEmail)
	setCallerLocale(t, polish, testsupport.AccountAEmail, "pl")

	polishAnswer := polish.post(conditionsURL, body)
	require.Equal(t, http.StatusUnprocessableEntity, polishAnswer.Status, polishAnswer.Body)

	var polishEnvelope struct {
		Error struct {
			Message string              `json:"message"`
			Fields  []domain.FieldError `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(polishAnswer.rawBody, &polishEnvelope))

	assert.Equal(t, englishEnvelope.Error.Fields, polishEnvelope.Error.Fields,
		"the same submission must refuse the same fields whatever the account's language (FR-033)")
	assert.NotEqual(t, englishEnvelope.Error.Message, polishEnvelope.Error.Message,
		"the envelope's own message must be shown in the account's language")
	assert.Equal(t, "co najmniej jedno pole zostało odrzucone", polishEnvelope.Error.Message)

	t.Run("the Datastar form re-render carries the Polish per-field explanation", func(t *testing.T) {
		t.Parallel()

		datastar := map[string]string{"Datastar-Request": "true"}

		answer := polish.do(http.MethodPost, conditionsURL, body, datastar)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, ids.RecordForm(kind.Condition, ""))
		assert.Contains(t, answer.Body, "To pole jest wymagane.",
			"the resolved_on control's refusal must be explained in Polish")
		assert.NotContains(t, answer.Body, "This is required.")
	})
}
