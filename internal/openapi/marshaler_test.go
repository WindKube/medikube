package openapi_test

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/openapi"
	"medikube/internal/web"
)

// A type that marshals itself has a wire form the struct walk cannot see.
// Describing its Go shape publishes `{"type":"object","additionalProperties":
// false}` — a schema that accepts nothing — for a member whose wire form is a
// string. It validates, it round-trips and every generated client rejects the
// server's own responses.
//
// The two types in this file are the ones this phase actually ships:
// domain.Date, which every clinical date is held in, and web.Optional[T],
// which carries a PATCH's absent-versus-null distinction.

// shipped is a DTO built from the real types rather than from fixtures, so
// this file fails when either of them changes how it marshals.
type shipped struct {
	Name      string               `json:"name"`
	StartedOn domain.Date          `json:"started_on"`
	EndedOn   *domain.Date         `json:"ended_on"`
	Notes     web.Optional[string] `json:"notes,omitzero"`
	Doses     web.Optional[int]    `json:"doses,omitzero"`
}

// property is the marshalled schema of one member, which is what a client
// generator reads. Comparing the JSON rather than the struct keeps the
// assertion in the vocabulary of the published document.
func property(t *testing.T, schema any, member string) string {
	t.Helper()

	encoded, err := json.Marshal(schema)
	require.NoError(t, err)

	var document struct {
		Properties map[string]jsontext.Value `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(encoded, &document))

	value, present := document.Properties[member]
	require.True(t, present, "%q is not a member of the published schema", member)

	return string(value)
}

func TestASelfMarshallingMemberIsPublishedAsWhatItWrites(t *testing.T) {
	t.Parallel()

	schema, err := openapi.SchemaOf(&shipped{})
	require.NoError(t, err)

	for _, testCase := range []struct {
		member    string
		published string
		why       string
	}{
		{
			member:    "started_on",
			published: `{"type":"string"}`,
			why:       "a calendar date marshals through MarshalText, so it is a JSON string",
		},
		{
			member:    "ended_on",
			published: `{"type":["string","null"]}`,
			why:       "a pointer without omitempty is required and nullable",
		},
		{
			member:    "notes",
			published: `{"type":["string","null"]}`,
			why:       "an Optional writes the value, or an explicit null, or nothing at all",
		},
		{
			member:    "doses",
			published: `{"type":["integer","null"]}`,
			why:       "the element type travels through the wrapper",
		},
	} {
		t.Run(testCase.member, func(t *testing.T) {
			t.Parallel()

			assert.JSONEq(t, testCase.published, property(t, schema, testCase.member), testCase.why)
		})
	}
}

// The absent state is the tag's business and the null state is the type's, and
// a schema that confused them would publish a member a PATCH cannot omit.
func TestAnOptionalMemberIsOptionalAndNullableAtOnce(t *testing.T) {
	t.Parallel()

	schema, err := openapi.SchemaOf(&shipped{})
	require.NoError(t, err)

	assert.NotContains(t, schema.Required, "notes", "omitzero says the member may be absent")
	assert.Contains(t, schema.Required, "started_on", "a member with no omit option is required")
}

// The published schema and the bytes the type actually writes are the two
// halves that have to agree; asserting only the first is how the empty object
// survived. Every value here is accepted by the schema above.
func TestTheWireFormMatchesTheSchemaThisPackagePublishes(t *testing.T) {
	t.Parallel()

	started, err := domain.ParseDate("2026-01-01")
	require.NoError(t, err)

	for _, testCase := range []struct {
		name  string
		value shipped
		wire  string
	}{
		{
			name:  "a value",
			value: shipped{Name: "n", StartedOn: started, Notes: web.Given("take with food")},
			wire:  `{"name":"n","started_on":"2026-01-01","ended_on":null,"notes":"take with food"}`,
		},
		{
			name:  "an explicit null",
			value: shipped{Name: "n", StartedOn: started, Notes: web.Cleared[string]()},
			wire:  `{"name":"n","started_on":"2026-01-01","ended_on":null,"notes":null}`,
		},
		{
			name:  "absent",
			value: shipped{Name: "n", StartedOn: started},
			wire:  `{"name":"n","started_on":"2026-01-01","ended_on":null}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(testCase.value)
			require.NoError(t, err)
			assert.JSONEq(t, testCase.wire, string(encoded))
		})
	}
}

// opaque marshals itself and says nothing about what it writes. A generator
// cannot describe it, and a schema that guessed would be the failure this
// refusal exists to prevent.
type opaque struct{ hidden string }

func (o opaque) MarshalJSON() ([]byte, error) { return json.Marshal(o.hidden) }

// opaqueV2 is the same hole through encoding/json/v2's own marshaler
// interface, which a type reached for by writing MarshalJSONTo rather than
// MarshalJSON — the spelling web.Optional itself uses.
type opaqueV2 struct{ hidden string }

func (o opaqueV2) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, o.hidden)
}

func TestATypeThatMarshalsItselfAndDeclaresNothingIsRefused(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		dto  any
	}{
		{name: "MarshalJSON", dto: &struct {
			Field opaque `json:"field"`
		}{}},
		{name: "MarshalJSONTo", dto: &struct {
			Field opaqueV2 `json:"field"`
		}{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := openapi.SchemaOf(testCase.dto)
			require.Error(t, err, "the reflector described a type whose JSON form it cannot see")
			assert.Contains(t, err.Error(), "field",
				"the refusal must name the member so the fix has an address")
			assert.Contains(t, err.Error(), "SchemaSource",
				"the refusal must name the seam that resolves it")
		})
	}
}
