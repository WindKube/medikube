package openapi

import (
	"encoding"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"medikube/internal/domain"
)

// The JSON pointer every component reference in this document starts with.
const componentRef = "#/components/schemas/"

// The named components. They are exported because the gates assert against
// them and because internal/cli writes the document a reviewer diffs.
const (
	// ErrorEnvelopeSchema is the ONE error shape. Every documented failure
	// references it, so api/openapi.json cannot disagree with the mapper in
	// internal/web/errors.go (contracts/README.md).
	ErrorEnvelopeSchema = "ErrorEnvelope"

	// RecordSchema and RecordSummarySchema are the discriminated unions the
	// six-operation record family reads and writes. Each registered kind
	// contributes one branch, so phase 003's eleven kinds add eleven branches
	// and zero operations (research D-08).
	RecordSchema        = "Record"
	RecordSummarySchema = "RecordSummary"

	// The write unions carry no discriminator: contracts/records.md's create
	// and patch DTOs have no kind member, because {kind} in the path has
	// already selected the branch.
	RecordCreateSchema = "RecordCreate"
	RecordPatchSchema  = "RecordPatch"

	// RecordSummaryPageSchema is contracts/README.md's list envelope around the
	// summary union. There is one list shape in the whole API.
	RecordSummaryPageSchema = "RecordSummaryPage"

	// SearchResponseSchema is contracts/search.md §2's grouped envelope — not
	// RecordSummaryPageSchema, because a grouped search has one cursor per
	// kind rather than one for the whole page.
	SearchResponseSchema = "SearchResponse"
)

// DiscriminatorProperty is the member whose value selects a branch of the
// record union. Its values are a kind's ENUM spelling — singular snake_case.
const DiscriminatorProperty = "kind"

// KindPathParameter is the {kind} template in the record family's paths. It
// shares its name with DiscriminatorProperty and carries the OTHER vocabulary:
// a path segment, plural and kebab-case, which is not a mechanical plural of
// the enum value and is not interchangeable with it (research D-05). The two
// coincide by name and never by value; nothing in OpenAPI would notice them
// being swapped, and every generated client would mis-dispatch.
const KindPathParameter = "kind"

// Kind is one record kind's contribution to the document: its two spellings and
// the four DTOs its branches are reflected from.
//
// It is a value rather than an interface because internal/records does not
// exist yet and because the synthetic second kind that makes the discriminator
// gate meaningful has no kind.Kind of its own — it must not be in the
// production kind table. A real kind fills Enum and Segment from Kind.Enum()
// and Kind.Segment(); nothing spells either by hand.
type Kind struct {
	Enum    string
	Segment string

	// Summary is the DTO list endpoints return, Detail the one detail
	// endpoints return; both must carry the discriminator member. Create and
	// Patch are the write DTOs and must not.
	Summary any
	Detail  any
	Create  any
	Patch   any
}

// BranchSchemaName is how a union's branch is named for one kind. The path
// segment is used rather than the enum value because it is already the
// collection's URL identity and is unique by construction.
func BranchSchemaName(union, segment string) string { return union + "_" + segment }

// SchemaSource is implemented by a type whose JSON form is not its Go shape.
//
// The reflector below describes Go structs. A type that marshals itself —
// web.Optional[T] is the one this phase ships — has a wire form the reflector
// cannot see, and describing its Go shape publishes an object with no members:
// a schema that accepts nothing, for a member whose wire form is a string.
// That is worse than the errors this package already raises, because it
// validates, round-trips and looks right.
//
// Implementing it costs the implementing package nothing: the interface is
// satisfied structurally, so internal/web does not import this package and
// kin-openapi stays out of the serving binary.
type SchemaSource interface {
	// OpenAPIValue returns a value of the type actually written, and whether
	// an explicit null is one of the states this type writes. The value is
	// reflected in place of the declaring type.
	OpenAPIValue() (value any, nullable bool)
}

// errOpaqueJSON refuses a type that marshals itself and does not say as what.
var errOpaqueJSON = errors.New(
	"the type marshals itself, so its JSON form is not its Go shape and reflecting it would " +
		"publish an object with no members; implement openapi.SchemaSource to say what it writes")

var (
	schemaSourceType  = reflect.TypeFor[SchemaSource]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	jsonMarshalerType = reflect.TypeFor[interface{ MarshalJSON() ([]byte, error) }]()
	jsonMarshalerToTy = reflect.TypeFor[interface {
		MarshalJSONTo(*jsontext.Encoder) error
	}]()
)

// satisfies reports whether t or *t implements iface. Both are asked because a
// marshaler declared on a pointer receiver still governs how a member of that
// type is written when the surrounding value is addressable, which it is.
func satisfies(t, iface reflect.Type) bool {
	return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
}

// declaredSchema answers for a type the struct walk must not describe. The
// second result reports whether it answered at all.
func declaredSchema(t reflect.Type) (*openapi3.Schema, bool, error) {
	switch {
	case satisfies(t, schemaSourceType):
		source, ok := reflect.New(t).Interface().(SchemaSource)
		if !ok {
			return nil, true, fmt.Errorf("%s implements SchemaSource but a value of it does not", t)
		}

		value, nullable := source.OpenAPIValue()

		schema, err := SchemaOf(value)
		if err != nil {
			return nil, true, fmt.Errorf("%s publishes as %T: %w", t, value, err)
		}

		if nullable {
			widenToNull(schema)
		}

		return schema, true, nil

	// A JSON marshaler writes whatever it likes; nothing here can know what.
	case satisfies(t, jsonMarshalerType) || satisfies(t, jsonMarshalerToTy):
		return nil, true, fmt.Errorf("%s: %w", t, errOpaqueJSON)

	// A text marshaler writes a JSON string. That is a guarantee of the
	// encoding contract rather than a guess, so it needs no declaration.
	case satisfies(t, textMarshalerType):
		return primitive("string"), true, nil
	}

	return nil, false, nil
}

// widenToNull adds the null type once. Both a pointer member and a nullable
// SchemaSource reach it, and a type list carrying "null" twice is not a schema.
func widenToNull(schema *openapi3.Schema) {
	if schema.Type == nil {
		schema.Type = &openapi3.Types{}
	}

	if slices.Contains(*schema.Type, "null") {
		return
	}

	*schema.Type = append(*schema.Type, "null")
}

// SchemaOf reflects one DTO into a schema.
//
// The rule that carries the weight is contracts/records.md's, restated
// mechanically: a member without omitempty is required; a POINTER without
// omitempty is required AND nullable, which is what lets a client tell "not
// recorded" from "not in this response". A pointer WITH omitempty is absent
// when nil and never null, so its type is not widened.
func SchemaOf(value any) (*openapi3.Schema, error) {
	if value == nil {
		return nil, errors.New("openapi: a nil value reflects to a schema that permits everything")
	}

	// A DTO handed over as a pointer describes the same object — records.Schema's
	// constructors return `any`, and whether that holds T or *T is not a
	// statement about the wire format. A pointer to a POINTER still fails
	// below, which is the case that matters.
	return schemaForType(indirect(reflect.TypeOf(value)))
}

var errPointerChain = errors.New(
	"a pointer to a pointer cannot carry absent-versus-explicit-null: encoding/json zeroes the whole " +
		"chain on a null, so both cases decode to a nil outer pointer (contracts/records.md's **string)")

func schemaForType(t reflect.Type) (*openapi3.Schema, error) {
	if schema, answered, err := declaredSchema(t); answered {
		return schema, err
	}

	switch t.Kind() {
	case reflect.String:
		return primitive("string"), nil

	case reflect.Bool:
		return primitive("boolean"), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return primitive("integer"), nil

	case reflect.Float32, reflect.Float64:
		return primitive("number"), nil

	case reflect.Slice, reflect.Array:
		items, err := schemaForType(t.Elem())
		if err != nil {
			return nil, err
		}

		return &openapi3.Schema{Type: &openapi3.Types{"array"}, Items: &openapi3.SchemaRef{Value: items}}, nil

	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("a map keyed by %s has no JSON object form", t.Key())
		}

		values, err := schemaForType(t.Elem())
		if err != nil {
			return nil, err
		}

		return &openapi3.Schema{
			Type:                 &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{Schema: &openapi3.SchemaRef{Value: values}},
		}, nil

	case reflect.Struct:
		return objectSchema(t)

	case reflect.Pointer:
		return nil, errPointerChain

	default:
		return nil, fmt.Errorf("a %s has no JSON schema form", t.Kind())
	}
}

func primitive(name string) *openapi3.Schema {
	return &openapi3.Schema{Type: &openapi3.Types{name}}
}

func objectSchema(t reflect.Type) (*openapi3.Schema, error) {
	schema := &openapi3.Schema{
		Type:       &openapi3.Types{"object"},
		Properties: openapi3.Schemas{},
		// contracts/README.md: unknown members are rejected with 422 and not
		// ignored. A schema that stays silent about that publishes a laxer
		// contract than the decoder enforces.
		AdditionalProperties: openapi3.AdditionalProperties{Has: openapi3.Ptr(false)},
	}

	if err := addMembers(schema, t); err != nil {
		return nil, err
	}

	// Sorted so a reordered struct is not a diff in api/openapi.json.
	sort.Strings(schema.Required)

	return schema, nil
}

func addMembers(schema *openapi3.Schema, t reflect.Type) error {
	for i := range t.NumField() {
		field := t.Field(i)

		name, optional, tagged, skip := jsonMember(field)
		if skip {
			continue
		}

		// encoding/json promotes an embedded struct's exported members into
		// the outer object, including when the embedded type itself is
		// unexported. A json name on the embedded field turns it back into an
		// ordinary member, which is why `tagged` decides.
		if field.Anonymous && !tagged {
			if embedded := indirect(field.Type); embedded.Kind() == reflect.Struct {
				if err := addMembers(schema, embedded); err != nil {
					return err
				}

				continue
			}
		}

		if _, shadowed := schema.Properties[name]; shadowed {
			return fmt.Errorf("member %q is declared twice; the schema cannot say which one wins", name)
		}

		inner, nullable, err := unwrapPointer(field.Type)
		if err != nil {
			return fmt.Errorf("member %q: %w", name, err)
		}

		property, err := schemaForType(inner)
		if err != nil {
			return fmt.Errorf("member %q: %w", name, err)
		}

		if nullable && !optional {
			widenToNull(property)
		}

		schema.Properties[name] = &openapi3.SchemaRef{Value: property}

		if !optional {
			schema.Required = append(schema.Required, name)
		}
	}

	return nil
}

// jsonMember reads one field the way encoding/json does.
func jsonMember(field reflect.StructField) (name string, optional, tagged, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, false, true
	}

	if !field.IsExported() && !field.Anonymous {
		return "", false, false, true
	}

	spelled, options, _ := strings.Cut(tag, ",")

	name = spelled
	tagged = spelled != ""

	if !tagged {
		name = field.Name
	}

	for _, option := range strings.Split(options, ",") {
		// omitzero is the Go 1.24 spelling and omitempty the older one; both
		// mean the member can be absent, which is the only thing a schema can
		// say about either.
		if option == "omitempty" || option == "omitzero" {
			optional = true
		}
	}

	return name, optional, tagged, false
}

func indirect(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}

	return t
}

func unwrapPointer(t reflect.Type) (reflect.Type, bool, error) {
	if t.Kind() != reflect.Pointer {
		return t, false, nil
	}

	if t.Elem().Kind() == reflect.Pointer {
		return nil, false, errPointerChain
	}

	return t.Elem(), true, nil
}

// recordComponents builds the record family's schemas: one branch per kind per
// union, the four unions themselves, and the list envelope.
//
// FACT 9 and shared design risk R1 live here. The branches are referenced and
// never inlined, so the document stays one description of each DTO — and so the
// discriminator mapping has something to point at.
func recordComponents(kinds []Kind) (openapi3.Schemas, error) {
	unions := []struct {
		name          string
		dto           func(Kind) any
		discriminated bool
	}{
		{name: RecordSchema, dto: func(k Kind) any { return k.Detail }, discriminated: true},
		{name: RecordSummarySchema, dto: func(k Kind) any { return k.Summary }, discriminated: true},
		{name: RecordCreateSchema, dto: func(k Kind) any { return k.Create }},
		{name: RecordPatchSchema, dto: func(k Kind) any { return k.Patch }},
	}

	schemas := openapi3.Schemas{}

	for _, union := range unions {
		branches := make(openapi3.SchemaRefs, 0, len(kinds))
		mapping := make(map[string]openapi3.MappingRef, len(kinds))

		for _, k := range kinds {
			branch, err := SchemaOf(union.dto(k))
			if err != nil {
				return nil, fmt.Errorf("%s's %s DTO: %w", k.Enum, union.name, err)
			}

			if union.discriminated {
				property, carried := branch.Properties[DiscriminatorProperty]
				if !carried {
					return nil, fmt.Errorf(
						"%s's %s DTO carries no %q member, so nothing selects its branch of the union",
						k.Enum, union.name, DiscriminatorProperty)
				}

				// A branch whose discriminator member accepts any string
				// discriminates nothing.
				property.Value.Enum = []any{k.Enum}
			}

			name := BranchSchemaName(union.name, k.Segment)
			schemas[name] = &openapi3.SchemaRef{Value: branch}
			branches = append(branches, &openapi3.SchemaRef{Ref: componentRef + name})

			// MappingRef is a defined type over SchemaRef that marshals to a
			// bare string — a struct, never a string (research D-08). Nothing
			// in kin-openapi resolves a mapping ref or checks its target
			// exists, which is why internal/openapi/oneof_test.go does.
			mapping[k.Enum] = openapi3.MappingRef{Ref: componentRef + name}
		}

		schema := &openapi3.Schema{OneOf: branches}
		if union.discriminated {
			schema.Discriminator = &openapi3.Discriminator{PropertyName: DiscriminatorProperty, Mapping: mapping}
		}

		schemas[union.name] = &openapi3.SchemaRef{Value: schema}
	}

	page, err := pageOf(componentRef + RecordSummarySchema)
	if err != nil {
		return nil, err
	}

	schemas[RecordSummaryPageSchema] = &openapi3.SchemaRef{Value: page}

	return schemas, nil
}

// pageItem is the placeholder element domain.Page is reflected with. Every list
// in the API is a list of a component, so the element schema is replaced by a
// reference and never published.
type pageItem struct{}

// pageOf builds the list envelope around one item reference. Its member names
// are reflected out of domain.Page rather than written here, so the published
// envelope cannot drift from the type every list handler returns.
func pageOf(itemRef string) (*openapi3.Schema, error) {
	schema, err := SchemaOf(domain.Page[pageItem]{})
	if err != nil {
		return nil, err
	}

	items := soleArrayMember(schema)
	if items == nil {
		return nil, errors.New("openapi: domain.Page no longer has exactly one array member; the list envelope cannot be built from it")
	}

	items.Items = &openapi3.SchemaRef{Ref: itemRef}

	return schema, nil
}

func soleArrayMember(schema *openapi3.Schema) *openapi3.Schema {
	var found *openapi3.Schema

	for _, property := range schema.Properties {
		if !slices.Contains(property.Value.Type.Slice(), "array") {
			continue
		}

		if found != nil {
			return nil
		}

		found = property.Value
	}

	return found
}
