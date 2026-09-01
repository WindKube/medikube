package openapi_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/openapi"
)

// T103. There is ONE error envelope and every documented failure points at it.
// An inline copy is how a published contract and internal/web/errors.go part
// company: the copy is still valid OpenAPI, still round-trips, and is wrong
// from the first day somebody edits one of the two.

const envelopeRef = componentPrefix + openapi.ErrorEnvelopeSchema

// The documented non-2xx responses that deliberately do NOT carry the error
// envelope, transcribed from the contract that makes each an exception. Every
// entry must still be present in the document, so a stale exception is as loud
// as a missing one.
//
// readyz: health.md gives 503 the readiness payload
// `{"status":"not_ready","checks":{...}}` — a fixed, closed vocabulary with no
// message field, precisely so a driver error cannot reach the wire (FR-052).
var nonEnvelopeResponses = map[string][]int{
	"readyz": {503},
}

func TestTheErrorEnvelopeSchemaMatchesTheContract(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	require.NotNil(t, loaded.Components)

	envelope, present := loaded.Components.Schemas[openapi.ErrorEnvelopeSchema]
	require.True(t, present)
	require.NotNil(t, envelope.Value)

	body, wrapped := envelope.Value.Properties["error"]
	require.True(t, wrapped, "the envelope is {\"error\": {...}}, not a bare object")
	require.NotNil(t, body.Value)
	assert.Equal(t, []string{"error"}, envelope.Value.Required)

	for _, member := range []string{"code", "message", "request_id"} {
		_, carried := body.Value.Properties[member]
		assert.Truef(t, carried, "the envelope carries no %s", member)
		assert.Containsf(t, body.Value.Required, member, "%s is on every error, including 500 (FR-054)", member)
	}

	// fields[] is present only for validation_failed, so it is optional — but
	// its shape is contract.
	fields, carried := body.Value.Properties["fields"]
	require.True(t, carried)
	require.NotNil(t, fields.Value)
	assert.NotContains(t, body.Value.Required, "fields")

	require.NotNil(t, fields.Value.Items)
	require.NotNil(t, fields.Value.Items.Value)

	for _, member := range []string{"field", "code", "message"} {
		_, carried := fields.Value.Items.Value.Properties[member]
		assert.Truef(t, carried, "a field error carries no %s", member)
	}
}

func TestEveryDocumentedErrorResponseReferencesTheOneEnvelope(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	seenExceptions := make(map[string][]int)
	checked := 0

	for opID, op := range operationsByID(t, loaded) {
		require.NotNil(t, op.op.Responses)

		for _, code := range op.op.Responses.Keys() {
			status, err := strconv.Atoi(code)
			require.NoErrorf(t, err, "%s documents the non-numeric response %q", opID, code)

			if status < 300 {
				continue
			}

			response := op.op.Responses.Value(code)
			require.NotNil(t, response.Value)
			require.NotNilf(t, response.Value.Description, "%s's %s carries no description", opID, code)

			if slices.Contains(nonEnvelopeResponses[opID], status) {
				seenExceptions[opID] = append(seenExceptions[opID], status)

				media := response.Value.Content.Get("application/json")
				if media != nil && media.Schema != nil {
					assert.NotEqualf(t, envelopeRef, media.Schema.Ref,
						"%s's %s is listed as an envelope exception but carries the envelope", opID, code)
				}

				continue
			}

			media := response.Value.Content.Get("application/json")
			require.NotNilf(t, media, "%s's %s documents no JSON body", opID, code)
			require.NotNilf(t, media.Schema, "%s's %s documents no schema", opID, code)

			assert.Equalf(t, envelopeRef, media.Schema.Ref,
				"%s's %s does not reference the shared error envelope", opID, code)

			checked++
		}
	}

	require.Positive(t, checked, "no error response was checked; this test asserted nothing")

	for opID, statuses := range nonEnvelopeResponses {
		assert.ElementsMatchf(t, statuses, seenExceptions[opID],
			"%s's envelope exceptions do not match what the document holds", opID)
	}
}

// The failure mode the ref check exists to catch, from the other side: an
// inline object that happens to have the envelope's shape passes every
// structural check kin-openapi makes.
func TestNoSchemaOtherThanTheEnvelopeLooksLikeTheEnvelope(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	for name, schema := range loaded.Components.Schemas {
		if name == openapi.ErrorEnvelopeSchema || schema.Value == nil {
			continue
		}

		_, copied := schema.Value.Properties["error"]
		assert.Falsef(t, copied, "%s carries an `error` member; there is one envelope and it is %s",
			name, openapi.ErrorEnvelopeSchema)
	}
}

// Every operation MediKube itself serves can fail, and contracts/README.md's
// last row is "anything else -> 500 internal_error". The documented externals
// are excluded: PocketBase serves them and answers with its own error shape,
// which MediKube does not define and must not claim to.
func TestEveryMediKubeOperationDocumentsTheFiveHundred(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))
	documented := operationsByID(t, loaded)

	for _, route := range documentedRoutes(t) {
		if route.Kind == httproute.KindExternal {
			continue
		}

		op := documented[route.OpID]
		require.NotNil(t, op.op)

		assert.NotNilf(t, op.op.Responses.Value("500"), "%s documents no 500", route.OpID)

		if route.Auth != httproute.AuthPublic {
			assert.NotNilf(t, op.op.Responses.Value("401"),
				"%s requires a session but documents no 401 (FR-034)", route.OpID)
		}
	}
}

func TestGenerateRefusesADocumentWithNoErrorEnvelope(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	in.Envelope = nil

	_, err := openapi.Generate(in)
	require.Error(t, err)
}
