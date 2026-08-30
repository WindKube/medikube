package openapi_test

import (
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/openapi"
)

// T099, VERIFIED FACT 9, shared design risk R1. The record family is six
// operations serving every clinical kind, and the whole 94-operation budget
// rests on a discriminated oneOf being generatable AND gateable. This file is
// the permanent form of the evidence that closed R1.

// componentPrefix is transcribed here rather than imported: a gate that asks
// the generator how it spells a ref is a gate that agrees with itself.
const componentPrefix = "#/components/schemas/"

func TestTheTwoKindDocumentSurvivesAMarshalThenLoadRoundTrip(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	require.Len(t, in.Kinds, 2, "the oneOf gate is meaningless with one branch (research D-08)")

	doc := generate(t, in)

	loaded, raw, err := openapi.RoundTrip(context.Background(), doc)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.NotNil(t, loaded)

	assert.True(t, loaded.IsOpenAPI31OrLater())
}

// The reason RoundTrip exists at all, pinned so nobody replaces it with a plain
// Validate. A document built in memory holds SchemaRefs that carry a Ref and no
// Value, so validating it in place cannot follow them and reports an unresolved
// ref for a document that is in fact correct (plan.md, research D-08). The
// inverse is the dangerous half: a ref carrying BOTH a Ref and a Value passes
// an in-place Validate and fails at load, so the load is where the catch is.
func TestValidatingTheBuiltDocumentInPlaceIsNotTheGate(t *testing.T) {
	t.Parallel()

	doc := generate(t, twoKindInput())

	err := doc.Validate(context.Background())

	require.Error(t, err, "if this ever passes, RoundTrip's marshal-then-load is no longer load-bearing and this file needs rereading")
	assert.Contains(t, err.Error(), "unresolved ref")
}

func TestEveryKindAppearsInTheDiscriminatorMapping(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	loaded := roundTrip(t, generate(t, in))

	require.NotNil(t, loaded.Components)
	schemas := loaded.Components.Schemas

	for _, union := range []string{openapi.RecordSchema, openapi.RecordSummarySchema} {
		t.Run(union, func(t *testing.T) {
			t.Parallel()

			ref, present := schemas[union]
			require.Truef(t, present, "the document has no %s component", union)
			require.NotNil(t, ref.Value)

			record := ref.Value
			require.Len(t, record.OneOf, len(in.Kinds), "one branch per registered kind")

			require.NotNil(t, record.Discriminator, "%s carries no discriminator", union)
			assert.Equal(t, openapi.DiscriminatorProperty, record.Discriminator.PropertyName)

			mapping := record.Discriminator.Mapping
			require.Len(t, mapping, len(in.Kinds))

			for _, k := range in.Kinds {
				// Mapping keys are values of the discriminator PROPERTY — the
				// enum spelling — and never the path segment. Getting this
				// backwards makes every generated client dispatch to the wrong
				// branch, and no check in kin-openapi says a word about it.
				entry, mapped := mapping[k.Enum]
				require.Truef(t, mapped, "%s is not in %s's discriminator mapping", k.Enum, union)
				require.NotEmptyf(t, entry.Ref, "%s maps to an empty ref, which marshals to \"\" without error", k.Enum)

				if k.Enum != k.Segment {
					_, bySegment := mapping[k.Segment]
					assert.Falsef(t, bySegment,
						"%s is mapped by its path segment; the discriminator property carries the enum spelling", k.Segment)
				}

				// Nothing in the library resolves a mapping ref or checks that
				// its target exists: a mapping pointing at a phantom schema
				// marshals, loads and validates cleanly. This is the assertion
				// that catches it.
				name := strings.TrimPrefix(entry.Ref, componentPrefix)
				require.NotEqualf(t, entry.Ref, name, "%s maps to %q, which is not a component ref", k.Enum, entry.Ref)

				branch, exists := schemas[name]
				require.Truef(t, exists, "%s maps to %s, which is not a schema in this document", k.Enum, entry.Ref)
				require.NotNil(t, branch.Value)
			}
		})
	}
}

func TestEachBranchPinsItsOwnDiscriminatorValue(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	loaded := roundTrip(t, generate(t, in))
	schemas := loaded.Components.Schemas

	for _, k := range in.Kinds {
		t.Run(k.Enum, func(t *testing.T) {
			t.Parallel()

			for _, base := range []string{openapi.RecordSchema, openapi.RecordSummarySchema} {
				branch, present := schemas[openapi.BranchSchemaName(base, k.Segment)]
				require.Truef(t, present, "no %s branch for %s", base, k.Segment)
				require.NotNil(t, branch.Value)

				property, carried := branch.Value.Properties[openapi.DiscriminatorProperty]
				require.Truef(t, carried, "%s's %s branch carries no discriminator property", k.Enum, base)
				require.NotNil(t, property.Value)

				assert.Equal(t, []any{k.Enum}, property.Value.Enum,
					"a branch whose discriminator property accepts any string discriminates nothing")
			}
		})
	}
}

// The branch names are built from the path segment, so two kinds cannot collide
// and a kind cannot be silently dropped by overwriting another's component.
func TestEveryKindContributesItsOwnBranchComponents(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	loaded := roundTrip(t, generate(t, in))

	for _, base := range []string{
		openapi.RecordSchema,
		openapi.RecordSummarySchema,
		openapi.RecordCreateSchema,
		openapi.RecordPatchSchema,
	} {
		union, present := loaded.Components.Schemas[base]
		require.Truef(t, present, "the document has no %s component", base)
		require.Len(t, union.Value.OneOf, len(in.Kinds))

		for _, k := range in.Kinds {
			name := openapi.BranchSchemaName(base, k.Segment)
			_, exists := loaded.Components.Schemas[name]
			assert.Truef(t, exists, "%s is missing", name)
		}
	}
}

// The write DTOs carry no `kind` member — contracts/records.md's
// MedicationCreate and MedicationPatch have no such field, because {kind} in
// the path already selects the branch. A discriminator on a union whose
// branches cannot carry the property would be a mapping no client can follow.
func TestTheWriteUnionsCarryNoDiscriminator(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	for _, base := range []string{openapi.RecordCreateSchema, openapi.RecordPatchSchema} {
		union := loaded.Components.Schemas[base]
		require.NotNil(t, union)
		assert.Nilf(t, union.Value.Discriminator,
			"%s discriminates on a property its branches do not carry", base)
	}
}

func TestAKindWhoseReadDTOCarriesNoDiscriminatorPropertyIsRefused(t *testing.T) {
	t.Parallel()

	type noDiscriminator struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	in := twoKindInput()
	broken := fakeKind()
	broken.Detail = noDiscriminator{}
	in.Kinds = []openapi.Kind{realKind(), broken}

	_, err := openapi.Generate(in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), openapi.DiscriminatorProperty)
	assert.Contains(t, err.Error(), fakeKindEnum)
}

func TestTwoKindsCannotShareASpelling(t *testing.T) {
	t.Parallel()

	t.Run("segment", func(t *testing.T) {
		t.Parallel()

		clash := fakeKind()
		clash.Segment = realKind().Segment

		in := twoKindInput()
		in.Kinds = []openapi.Kind{realKind(), clash}

		_, err := openapi.Generate(in)
		require.Error(t, err)
	})

	t.Run("enum", func(t *testing.T) {
		t.Parallel()

		clash := fakeKind()
		clash.Enum = realKind().Enum

		in := twoKindInput()
		in.Kinds = []openapi.Kind{realKind(), clash}

		_, err := openapi.Generate(in)
		require.Error(t, err)
	})
}

// The {kind} path parameter is an enum of the registered path SEGMENTS, which
// is the other half of research D-05's two vocabularies: the segment addresses
// the collection, the enum value discriminates the body.
func TestTheKindPathParameterEnumeratesTheSegments(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	loaded := roundTrip(t, generate(t, in))

	expected := make([]any, 0, len(in.Kinds))
	for _, k := range in.Kinds {
		expected = append(expected, k.Segment)
	}

	checked := 0

	for _, path := range loaded.Paths.Keys() {
		if !strings.Contains(path, "{"+openapi.KindPathParameter+"}") {
			continue
		}

		for method, op := range loaded.Paths.Value(path).Operations() {
			parameter := op.Parameters.GetByInAndName(openapi3.ParameterInPath, openapi.KindPathParameter)
			require.NotNilf(t, parameter, "%s %s templates {%s} but declares no such parameter", method, path, openapi.KindPathParameter)
			require.NotNil(t, parameter.Schema)
			require.NotNil(t, parameter.Schema.Value)

			assert.ElementsMatch(t, expected, parameter.Schema.Value.Enum,
				"%s %s does not enumerate the registered segments", method, path)

			checked++
		}
	}

	require.Positive(t, checked, "no operation templates {%s}; this test asserted nothing", openapi.KindPathParameter)
}
