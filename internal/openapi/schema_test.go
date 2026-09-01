package openapi_test

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/openapi"
)

// T100. The reflector is what keeps api/openapi.json a function of the DTOs
// rather than a second description of them, so its rules are asserted directly
// and not only through a generated document.
//
// The rule that carries the most weight is the one contracts/records.md states
// in prose: a member without `omitempty` is required, and a POINTER without
// `omitempty` is required AND nullable — which is exactly how a client tells
// "not recorded" from "not in this response".

func TestReflectingADTO(t *testing.T) {
	t.Parallel()

	type nested struct {
		Deep string `json:"deep"`
	}

	type embedded struct {
		Promoted string `json:"promoted"`
	}

	type dto struct {
		embedded

		Required   string            `json:"required"`
		Optional   string            `json:"optional,omitempty"`
		Zeroable   string            `json:"zeroable,omitzero"`
		Nullable   *string           `json:"nullable"`
		Absentable *string           `json:"absentable,omitempty"`
		Flag       bool              `json:"flag"`
		Count      int               `json:"count"`
		Ratio      float64           `json:"ratio"`
		List       []string          `json:"list"`
		Lookup     map[string]int    `json:"lookup"`
		Object     nested            `json:"object"`
		Ignored    string            `json:"-"`
		unexported string            //nolint:unused // the point of the case
		Untagged   map[string]string `json:""`
	}

	schema, err := openapi.SchemaOf(dto{})
	require.NoError(t, err)

	assert.Equal(t, &openapi3.Types{"object"}, schema.Type)
	require.NotNil(t, schema.AdditionalProperties.Has)
	assert.False(t, *schema.AdditionalProperties.Has,
		"contracts/README.md rejects unknown members rather than ignoring them, and the schema should say so")

	t.Run("members", func(t *testing.T) {
		t.Parallel()

		for _, name := range []string{
			"promoted", "required", "optional", "zeroable", "nullable", "absentable",
			"flag", "count", "ratio", "list", "lookup", "object", "Untagged",
		} {
			_, present := schema.Properties[name]
			assert.Truef(t, present, "%s is missing", name)
		}

		for _, name := range []string{"Ignored", "-", "unexported", "embedded"} {
			_, present := schema.Properties[name]
			assert.Falsef(t, present, "%s should not be a member", name)
		}
	})

	t.Run("required", func(t *testing.T) {
		t.Parallel()

		assert.ElementsMatch(t,
			[]string{"promoted", "required", "nullable", "flag", "count", "ratio", "list", "lookup", "object", "Untagged"},
			schema.Required)
	})

	t.Run("types", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			member string
			want   *openapi3.Types
		}{
			{member: "required", want: &openapi3.Types{"string"}},
			{member: "optional", want: &openapi3.Types{"string"}},
			// A pointer with omitempty is absent when nil, never null, so
			// widening its type would document a value it cannot carry.
			{member: "absentable", want: &openapi3.Types{"string"}},
			{member: "nullable", want: &openapi3.Types{"string", "null"}},
			{member: "flag", want: &openapi3.Types{"boolean"}},
			{member: "count", want: &openapi3.Types{"integer"}},
			{member: "ratio", want: &openapi3.Types{"number"}},
			{member: "list", want: &openapi3.Types{"array"}},
			{member: "lookup", want: &openapi3.Types{"object"}},
			{member: "object", want: &openapi3.Types{"object"}},
		}

		for _, tc := range cases {
			t.Run(tc.member, func(t *testing.T) {
				t.Parallel()

				property := schema.Properties[tc.member]
				require.NotNil(t, property)
				require.NotNil(t, property.Value)
				assert.Equal(t, tc.want, property.Value.Type)
			})
		}
	})

	t.Run("collections carry their element", func(t *testing.T) {
		t.Parallel()

		list := schema.Properties["list"].Value
		require.NotNil(t, list.Items)
		assert.Equal(t, &openapi3.Types{"string"}, list.Items.Value.Type)

		lookup := schema.Properties["lookup"].Value
		require.NotNil(t, lookup.AdditionalProperties.Schema)
		assert.Equal(t, &openapi3.Types{"integer"}, lookup.AdditionalProperties.Schema.Value.Type)

		object := schema.Properties["object"].Value
		_, deep := object.Properties["deep"]
		assert.True(t, deep)
	})
}

// A DTO supplied as a pointer is the same DTO. internal/records hands its four
// schemas over as `func() any`, and whether an implementation returns T or *T
// says nothing about the wire format.
func TestReflectingAPointerToADTO(t *testing.T) {
	t.Parallel()

	type dto struct {
		Name string `json:"name"`
	}

	fromValue, err := openapi.SchemaOf(dto{})
	require.NoError(t, err)

	fromPointer, err := openapi.SchemaOf(&dto{})
	require.NoError(t, err)

	assert.Equal(t, fromValue, fromPointer)
}

func TestTheReflectorRefusesWhatItCannotDescribe(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		about string
	}{
		{
			name: "nothing at all",
			// A nil DTO reflects to an empty schema that accepts everything,
			// which is the most permissive contract there is and the least
			// likely to be noticed.
			value: nil,
			about: "",
		},
		{
			name: "pointer to pointer",
			value: struct {
				Doomed **string `json:"doomed,omitempty"`
			}{},
			// contracts/records.md spells the two patch dates **string to carry
			// absent-versus-null. It does not work: encoding/json zeroes the
			// whole chain on an explicit null, so both cases arrive as a nil
			// outer pointer. Describing it in the document would publish a
			// distinction the decoder cannot make.
			about: "doomed",
		},
		{
			name: "a channel",
			value: struct {
				Ch chan int `json:"ch"`
			}{},
			about: "ch",
		},
		{
			name: "a function",
			value: struct {
				Fn func() `json:"fn"`
			}{},
			about: "fn",
		},
		{
			name: "an interface",
			value: struct {
				Any any `json:"any"`
			}{},
			about: "any",
		},
		{
			name: "a map that is not keyed by a string",
			value: struct {
				M map[int]string `json:"m"`
			}{},
			about: "m",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := openapi.SchemaOf(tc.value)

			require.Error(t, err)

			if tc.about != "" {
				assert.Contains(t, err.Error(), tc.about)
			}
		})
	}
}

// The list envelope is contracts/README.md's, and its member names come from
// domain.Page rather than from this package, so the published shape cannot
// drift from the type every list handler returns.
func TestTheListEnvelopeIsTheOneEveryListReturns(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	page, present := loaded.Components.Schemas[openapi.RecordSummaryPageSchema]
	require.True(t, present)
	require.NotNil(t, page.Value)

	items, carried := page.Value.Properties["items"]
	require.True(t, carried)
	require.NotNil(t, items.Value)
	assert.Equal(t, &openapi3.Types{"array"}, items.Value.Type)

	require.NotNil(t, items.Value.Items)
	assert.Equal(t, componentPrefix+openapi.RecordSummarySchema, items.Value.Items.Ref,
		"a list of records that inlines its item schema is a second copy of the union")

	cursor, paged := page.Value.Properties["next_cursor"]
	require.True(t, paged)
	assert.Equal(t, &openapi3.Types{"string", "null"}, cursor.Value.Type,
		"nil is the last page and the member is present either way")
	assert.Contains(t, page.Value.Required, "next_cursor")

	_, counted := page.Value.Properties["total"]
	require.True(t, counted)
	assert.NotContains(t, page.Value.Required, "total", "total appears only when ?count=true was passed")
}
