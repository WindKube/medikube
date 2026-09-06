package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T014, D-09, SC-005. Message translates; everything else in the envelope
// does not. A closed instance's registration refusal (403 registration_closed)
// is a plain instance-wide 403 with no owner-scoped ambiguity, so it is a
// clean fixture for the comparison.
func TestErrorMessageIsTranslatedAndNothingElseInTheEnvelopeIs(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	document := body(
		"email", quoted(newAccountEmail),
		"name", quoted(newAccountName),
		"password", quoted(newAccountPassword),
	)

	en := rig.anonymous().do(http.MethodPost, registerURL, document, map[string]string{"Accept-Language": "en"})
	pl := rig.anonymous().do(http.MethodPost, registerURL, document, map[string]string{"Accept-Language": "pl"})

	require.Equal(t, http.StatusForbidden, en.Status)
	require.Equal(t, http.StatusForbidden, pl.Status)

	var enBody, plBody envelopeDTO
	en.decode(t, &enBody)
	pl.decode(t, &plBody)

	assert.Equal(t, "registration_closed", enBody.Error.Code)
	assert.Equal(t, "registration_closed", plBody.Error.Code)
	assert.NotEqual(t, enBody.Error.Message, plBody.Error.Message,
		"the two languages produced the same message text")

	assert.Equal(t, withoutMessage(t, en.rawBody), withoutMessage(t, pl.rawBody),
		"code, request_id and fields must be byte-identical once message is removed")
}

// withoutMessage decodes the envelope, blanks Error.Message and re-encodes,
// so the comparison is over everything the caller's language must not
// change.
func withoutMessage(t *testing.T, raw []byte) string {
	t.Helper()

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	errorField, ok := decoded["error"].(map[string]any)
	require.True(t, ok, "the envelope has no error member: %s", raw)

	delete(errorField, "message")
	delete(errorField, "request_id")

	reencoded, err := json.Marshal(decoded)
	require.NoError(t, err)

	return string(reencoded)
}
