package web

import (
	json "encoding/json/v2"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

// patchFixture is contracts/records.md's patch DTO in miniature: two plain
// optional members and two that must be able to carry an explicit null,
// because clearing a recorded date is a thing a person does and "omitted" must
// not mean the same as "cleared".
type patchFixture struct {
	Name      Optional[string] `json:"name,omitzero"`
	Notes     Optional[string] `json:"notes,omitzero"`
	StartedOn Optional[string] `json:"started_on,omitzero"`
	EndedOn   Optional[string] `json:"ended_on,omitzero"`
}

// state renders one member as the three-way distinction the contract requires,
// so a case can assert it as a single string.
func state(o Optional[string]) string {
	switch {
	case !o.Present():
		return "absent"
	case o.Clears():
		return "cleared"
	}

	value, _ := o.Get()

	return "value:" + value
}

func TestDecodingTellsAbsentFromExplicitNullFromAValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		body      string
		nameState string
		started   string
		ended     string
	}{
		{"an empty object touches nothing", `{}`, "absent", "absent", "absent"},
		{"a value is a value", `{"name":"a"}`, "value:a", "absent", "absent"},
		{"an explicit null clears", `{"started_on":null}`, "absent", "cleared", "absent"},
		{
			"clearing one date leaves the other alone",
			`{"started_on":null}`,
			"absent", "cleared", "absent",
		},
		{
			"clearing both is two nulls and not one",
			`{"started_on":null,"ended_on":null}`,
			"absent", "cleared", "cleared",
		},
		{"a value beside a null", `{"name":"a","ended_on":null}`, "value:a", "absent", "cleared"},
		{"an empty string is a value, not a clear", `{"started_on":""}`, "absent", "value:", "absent"},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			var patch patchFixture
			require.NoError(t, DecodeBytes([]byte(one.body), &patch))

			assert.Equal(t, one.nameState, state(patch.Name), "name")
			assert.Equal(t, one.started, state(patch.StartedOn), "started_on")
			assert.Equal(t, one.ended, state(patch.EndedOn), "ended_on")
			assert.Equal(t, "absent", state(patch.Notes),
				"a member nobody sent came back present, so a PATCH would clear what it never mentioned")
		})
	}
}

// The three states round-trip to the bytes they came from. A patch that
// marshals differently from the way it decoded is a patch the OpenAPI document
// describes incorrectly, and phase 002 reuses this type.
func TestTheThreeStatesRoundTripByteForByte(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`{}`,
		`{"name":"a"}`,
		`{"started_on":null}`,
		`{"name":"a","notes":"n","started_on":"2026-01-01","ended_on":null}`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			var patch patchFixture
			require.NoError(t, DecodeBytes([]byte(body), &patch))

			out, err := json.Marshal(&patch)
			require.NoError(t, err)
			assert.JSONEq(t, body, string(out))
		})
	}
}

func TestTheConstructorsProduceTheStatesTheDecoderProduces(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "absent", state(Optional[string]{}))
	assert.Equal(t, "cleared", state(Cleared[string]()))
	assert.Equal(t, "value:x", state(Given("x")))

	value, ok := Given("x").Get()
	assert.Equal(t, "x", value)
	assert.True(t, ok)

	_, ok = Cleared[string]().Get()
	assert.False(t, ok, "a cleared member handed back a value")

	assert.True(t, Optional[string]{}.IsZero(), "an absent member is not omitted on the way out")
	assert.False(t, Cleared[string]().IsZero(), "an explicit null would be omitted, which loses the clear")
}

// FR-032 is a property of the shape. Neither write DTO has an owner member, so
// a body carrying one is refused by the decoder before any business code runs —
// and the refusal names the member, because a client has to be able to fix it.
func TestAnUnknownMemberIsRefusedAndNamed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		body  string
		field string
		code  string
	}{
		{"a re-attribution attempt", `{"owner":"mkacctboris0001"}`, "owner", domain.CodeUnknownField},
		{"a server-owned member", `{"id":"x"}`, "id", domain.CodeUnknownField},
		{"a near miss on the name", `{"Name":"a"}`, "Name", domain.CodeUnknownField},
		{"a value of the wrong shape", `{"name":5}`, "name", domain.CodeInvalidValue},
		{"the same member twice", `{"name":"a","name":"b"}`, "name", domain.CodeInvalidValue},
		{"not an object at all", `[]`, "body", domain.CodeInvalidValue},
		{"not JSON at all", `{`, "body", domain.CodeInvalidValue},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			var patch patchFixture
			err := DecodeBytes([]byte(one.body), &patch)
			require.Error(t, err)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid, "the decoder's error did not become a MediKube validation failure")
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, one.field, invalid.Fields[0].Field)
			assert.Equal(t, one.code, invalid.Fields[0].Code)

			// The message is MediKube's, always. Everything the decoder said is
			// dropped, and this is what makes that true for every branch rather
			// than for the one the PHI case happens to reach.
			assert.Containsf(t, decodeMessages(), invalid.Fields[0].Message,
				"the refusal repeats what the decoder said, and the decoder quotes the value it choked on")

			status, code := Classify(err)
			assert.Equal(t, http.StatusUnprocessableEntity, status)
			assert.Equal(t, domain.CodeValidationFailed, code)
		})
	}
}

// decodeMessages is MediKube's whole decode vocabulary. A message outside it
// came from the decoder, and the decoder quotes what it was given.
func decodeMessages() []string {
	return []string{
		"the field is not one this operation accepts",
		"the value is not the shape this field takes",
		"the field was sent more than once",
		"a JSON object is required",
		"the request body is not a JSON object this operation accepts",
	}
}

// countedFixture has the member shape whose decoder error is measured to embed
// the submitted value: an integer, whose overflow message quotes the number in
// full.
type countedFixture struct {
	Doses int              `json:"doses"`
	Notes Optional[string] `json:"notes,omitzero"`
}

// research D-28. Go's own decoder text embeds the value it choked on — "cannot
// unmarshal JSON number 99999999999999999999 into Go int", and an RFC3339 parse
// failure quotes the string in full — and on this application's DTOs the value
// is medical data. Nothing the decoder said reaches the response or the log.
func TestADecodeFailureRepeatsNothingTheClientSent(t *testing.T) {
	t.Parallel()

	secret := "Amoxicillin 500mg twice daily"

	cases := map[string]struct {
		body   string
		target any
		leaked string
	}{
		"an unknown member":          {`{"dose_text":"` + secret + `"}`, new(patchFixture), secret},
		"a value of the wrong shape": {`{"notes":{"drug":"` + secret + `"}}`, new(countedFixture), secret},
		"a number the decoder quotes back in full": {
			`{"doses":99999999999999999999}`, new(countedFixture), "99999999999999999999",
		},
	}

	for name, one := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := DecodeBytes([]byte(one.body), one.target)
			require.Error(t, err)

			assert.NotContains(t, err.Error(), one.leaked,
				"the submitted value is in the error, and the error is logged")

			body, marshalErr := json.Marshal(NewEnvelope(err, "req"))
			require.NoError(t, marshalErr)
			assert.NotContains(t, string(body), one.leaked)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			assert.Contains(t, decodeMessages(), invalid.Fields[0].Message)
		})
	}
}

// The rereadable-body trap. PocketBase wraps every request body in a reader
// that rewinds on EOF, so json.UnmarshalRead sees the document twice and every
// decode fails with "invalid character '{' after top-level value". Decode must
// not use it.
func TestDecodeReadsARequestBodyThatRewinds(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodPatch, "/x", `{"name":"a"}`)
	e.Request.Body = rereadable(`{"name":"a"}`)

	var patch patchFixture
	require.NoError(t, Decode(e, &patch))
	assert.Equal(t, "value:a", state(patch.Name))
}

func TestAnEmptyRequestBodyIsARefusalAndNotAnEmptyPatch(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodPatch, "/x", "")

	var patch patchFixture
	err := Decode(e, &patch)
	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "body", invalid.Fields[0].Field)
}

// contracts/README.md: there is no ?fields=. PocketBase's own e.JSON applies a
// field picker to every 2xx when the query carries one, which would answer a
// documented DTO with a partial one — silently, and outside OpenAPI.
func TestWriteJSONIgnoresAFieldsQueryParameter(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodGet, "/x?fields=id")

	require.NoError(t, WriteJSON(e, http.StatusOK, map[string]string{"id": "1", "name": "a"}))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"name":"a"`,
		"a ?fields= query produced a partial response, which no OpenAPI operation describes")
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
}

// The contract's list envelope: items is [] and never null, and next_cursor is
// present either way.
func TestAnEmptyPageMarshalsAsAnEmptyArray(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodGet, "/x")

	require.NoError(t, WriteJSON(e, http.StatusOK, domain.Page[string]{}))

	assert.Equal(t, `{"items":[],"next_cursor":null}`, recorder.Body.String())
}

func TestWriteJSONRefusesToWriteTwice(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodGet, "/x")

	require.NoError(t, WriteJSON(e, http.StatusOK, map[string]string{"a": "b"}))
	require.Error(t, WriteJSON(e, http.StatusOK, map[string]string{"a": "b"}),
		"a second write corrupts the response and logs outside the one stream")
}

func TestDecodeBytesRefusesANilTarget(t *testing.T) {
	t.Parallel()

	require.Error(t, DecodeBytes([]byte(`{}`), nil))
	assert.False(t, errors.As(DecodeBytes([]byte(`{}`), nil), new(*domain.ValidationError)),
		"a wiring mistake was reported to the client as their fault")
}
