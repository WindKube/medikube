package web

import (
	json "encoding/json/v2"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// D-24. The ETag is derived from `updated` and is never a column of its own, so
// a write that changes a record changes its version with no second mechanism to
// keep in step.
func TestTheETagIsDerivedFromUpdated(t *testing.T) {
	t.Parallel()

	app := testsupport.NewApp(t)

	record, err := app.FindRecordById(kind.Medication.Collection(), testsupport.NameOnlyMedicationID)
	require.NoError(t, err)

	before := ETag(store.Version(record))
	require.NotEmpty(t, before)
	assert.Equal(t, `"`+store.Version(record)+`"`, before, "the ETag is not the version, quoted")

	// RFC 9110's etagc excludes the space PocketBase's own date layout carries,
	// so the raw instant is not a legal entity-tag at all.
	assert.NotContains(t, before, " ")
	assert.NotContains(t, before, record.GetDateTime("updated").String())

	require.NoError(t, app.Save(record))

	after := ETag(store.Version(record))
	assert.NotEqual(t, before, after, "the record was written and its version did not move")
}

func TestAnUnsavedRecordHasNoETagRatherThanOneEveryUnsavedRecordShares(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ETag(""), "an empty version rendered as an entity-tag, which every unsaved record would then share")

	e, recorder := event(t, http.MethodGet, "/x")
	SetETag(e, "")
	assert.Empty(t, recorder.Header().Get(ETagHeader))

	e, recorder = event(t, http.MethodGet, "/x")
	SetETag(e, "abcd1234")
	assert.Equal(t, `"abcd1234"`, recorder.Header().Get(ETagHeader))
}

// contracts/README.md, contracts/records.md and research D-24 all say the same
// thing: If-Match is REQUIRED on PATCH and DELETE, and its absence is 422
// validation_failed with field If-Match and code required.
//
// tasks.md T115 says 428 instead. It is outvoted three to one, it contradicts
// internal/records.Handler, which already answers 422 for the same condition,
// and specs/002-patient-core/contracts/patients.md:161 records the decision
// explicitly: "428 -> not used; a missing If-Match is 422 validation_failed
// with field If-Match, code required — keeps one error taxonomy."
func TestAMissingIfMatchIsAValidationFailureNamingTheHeader(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodPatch, http.MethodDelete, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			e, _ := event(t, method, "/x/1")

			version, err := IfMatch(e)
			require.Error(t, err)
			assert.Empty(t, version)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, IfMatchHeader, invalid.Fields[0].Field,
				"the field names something other than the header the caller has to add")
			assert.Equal(t, domain.CodeRequired, invalid.Fields[0].Code)

			status, code := Classify(err)
			assert.Equal(t, http.StatusUnprocessableEntity, status)
			assert.Equal(t, domain.CodeValidationFailed, code)
		})
	}
}

func TestIfMatchAcceptsExactlyTheTagsAServerIssued(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		header  string
		version string
		code    string
	}{
		{"the tag as issued", `"abcd1234"`, "abcd1234", ""},
		{"surrounding space", `  "abcd1234" `, "abcd1234", ""},
		{"unquoted", `abcd1234`, "", domain.CodeInvalidValue},
		{"weak", `W/"abcd1234"`, "", domain.CodeInvalidValue},
		{"a list", `"abcd1234", "efgh5678"`, "", domain.CodeInvalidValue},
		{"empty", `""`, "", domain.CodeInvalidValue},
		{"a wildcard", `*`, "", domain.CodeInvalidValue},
		{"an unclosed quote", `"abcd1234`, "", domain.CodeInvalidValue},
		{"a tag with a quote in it", `"ab"cd"`, "", domain.CodeInvalidValue},
		{"blank", "   ", "", domain.CodeRequired},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			e, _ := event(t, http.MethodPatch, "/x/1")
			e.Request.Header.Set(IfMatchHeader, one.header)

			version, err := IfMatch(e)

			if one.code == "" {
				require.NoError(t, err)
				assert.Equal(t, one.version, version)

				return
			}

			require.Error(t, err)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, IfMatchHeader, invalid.Fields[0].Field)
			assert.Equal(t, one.code, invalid.Fields[0].Code)
		})
	}
}

// A wildcard means "whatever is there now", which is exactly the overwrite
// FR-026 refuses. RFC 9110 allows it; MediKube does not, because an optional
// precondition is a precondition nobody sends and a wildcard one is a
// precondition that always passes.
func TestAWildcardIfMatchIsRefusedRatherThanTreatedAsAMatch(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodDelete, "/x/1")
	e.Request.Header.Set(IfMatchHeader, "*")

	_, err := IfMatch(e)
	require.Error(t, err, "a wildcard precondition was accepted, so any client can overwrite a change it never saw")
}

// FR-026 and US1-9: the 412 carries the server's current representation, so
// "the current values are shown so they can decide what to do" is a property of
// the response rather than a second request the page has to remember to make.
func TestAStaleIfMatchAnswersWithTheCurrentRepresentation(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodPatch, "/x/1")

	current := map[string]string{"id": "1", "name": "the value on the server"}

	require.NoError(t, WriteVersionMismatch(e, "req-1", "efgh5678", current))

	assert.Equal(t, http.StatusPreconditionFailed, recorder.Code)
	assert.Equal(t, `"efgh5678"`, recorder.Header().Get(ETagHeader),
		"the 412 does not carry the version the caller has to retry with")

	var body struct {
		Error   Failure           `json:"error"`
		Current map[string]string `json:"current"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	assert.Equal(t, CodeVersionMismatch, body.Error.Code)
	assert.Equal(t, "req-1", body.Error.RequestID)
	assert.Equal(t, current, body.Current, "the current values are not in the response")
}

// The same flaky-gate trap as the envelope: the 412 body is compared whole by
// the tests that assert FR-026, and a member order that moves would fail them
// on the ordering rather than on the behaviour.
func TestTheVersionMismatchBodyIsByteStable(t *testing.T) {
	t.Parallel()

	current := map[string]string{"b": "2", "a": "1", "c": "3"}

	var first string

	for i := range 100 {
		e, recorder := event(t, http.MethodPatch, "/x/1")
		require.NoError(t, WriteVersionMismatch(e, "req-1", "efgh5678", current))

		if i == 0 {
			first = recorder.Body.String()

			continue
		}

		require.Equal(t, first, recorder.Body.String(),
			"the 412 body's member order is not stable, so every whole-body assertion on it is flaky")
	}

	assert.Contains(t, first, `{"a":"1","b":"2","c":"3"}`)
}
