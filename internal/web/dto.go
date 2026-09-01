package web

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
)

// ContentTypeJSON is what goes in and what comes out (contracts/README.md).
const ContentTypeJSON = "application/json"

// headerContentType is spelled here rather than imported: PocketBase keeps its
// own copy unexported (tools/router/event.go:155).
const headerContentType = "Content-Type"

// Optional carries the three states a PATCH member can be in: absent, an
// explicit null, or a value. The distinction is what lets a PATCH clear one
// field without clearing every field it did not mention.
//
// contracts/records.md spells this `**string` and says the mechanism "marshals
// correctly under encoding/json/v2". It does not: encoding/json zeroes the
// whole pointer chain when it reads a null, so an explicit null and an absent
// member both arrive as a nil outer pointer — under json/v2, under Go 1.27's
// v1 retrofit, and under Go 1.26's real v1. Pre-populating the outer pointer
// does not rescue it either. json_semantics_test.go pins that, so if a future
// toolchain changes it the deviation is reconsidered rather than forgotten.
//
// The zero value is the absent one, so a member that nobody sent needs no
// constructor and no initialisation. Declare it with `omitzero`: without that
// tag an absent member marshals as null and the clear is indistinguishable
// again, this time on the way out.
//
// It is deliberately not comparable with == and carries unexported fields:
// internal/openapi reflects DTOs, and a type whose members were exported would
// be published as an object with a `present` boolean in it.
type Optional[T any] struct {
	// present is whether the member appeared in the document at all.
	present bool
	// value is nil for an explicit null, which is the state present cannot
	// carry on its own.
	value *T
}

// Given is a member the client sent with a value.
func Given[T any](value T) Optional[T] {
	return Optional[T]{present: true, value: &value}
}

// Cleared is a member the client sent as an explicit null — "remove what is
// recorded here", which is a different instruction from saying nothing.
func Cleared[T any]() Optional[T] {
	return Optional[T]{present: true}
}

// Present reports whether the client sent the member at all.
func (o Optional[T]) Present() bool { return o.present }

// Clears reports whether the client asked for the field to be emptied.
func (o Optional[T]) Clears() bool { return o.present && o.value == nil }

// Get returns the value and whether there is one. An absent member and a
// cleared one both answer false, which is why a caller that must tell them
// apart asks Present first.
func (o Optional[T]) Get() (T, bool) {
	if o.value == nil {
		var zero T

		return zero, false
	}

	return *o.value, true
}

// IsZero is what the `omitzero` tag reads. Only the absent state is omitted: an
// explicit null is a value the client sent and it marshals as one.
func (o Optional[T]) IsZero() bool { return !o.present }

// MarshalJSONTo writes the member as the client would have sent it.
func (o Optional[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if o.value == nil {
		return enc.WriteToken(jsontext.Null)
	}

	return json.MarshalEncode(enc, o.value)
}

// UnmarshalJSONFrom reads the null itself and delegates everything else, so the
// caller's options — RejectUnknownMembers in particular — still apply to the
// value inside.
func (o *Optional[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if dec.PeekKind() == 'n' {
		if _, err := dec.ReadToken(); err != nil {
			return err
		}

		o.present, o.value = true, nil

		return nil
	}

	var value T
	if err := json.UnmarshalDecode(dec, &value); err != nil {
		return err
	}

	o.present, o.value = true, &value

	return nil
}

// OpenAPIValue publishes an Optional as what it marshals as: a member of T
// that may also be an explicit null.
//
// It satisfies openapi.SchemaSource structurally, so this package does not
// import internal/openapi and kin-openapi stays out of the serving binary.
// Without it the reflector would describe this struct's Go shape — an object
// with no members, because they are all unexported — and publish a schema that
// refuses every value the type actually writes.
func (Optional[T]) OpenAPIValue() (any, bool) {
	var zero T

	return zero, true
}

// Decode reads a request body into target.
//
// It reads the body rather than decoding from it. PocketBase wraps every
// request body in a reader that rewinds on EOF (tools/router/router.go:136),
// so json.UnmarshalRead's trailing-whitespace scan reads the document a second
// time and every decode fails with "invalid character '{' after top-level
// value". The body is bounded by PocketBase's own BodyLimit middleware
// (apis/base.go:36), which is what makes reading it whole safe.
func Decode(e *core.RequestEvent, target any) error {
	raw, err := io.ReadAll(e.Request.Body)
	if err != nil {
		return fmt.Errorf("web: reading the request body: %w", err)
	}

	return DecodeBytes(raw, target)
}

// DecodeBytes is the typed DTO boundary: unknown members are rejected, which is
// what makes FR-032 a property of the shape rather than a runtime check. No
// write DTO has an `owner` member, so a body carrying one is refused here.
//
// Duplicate member names and case-mismatched names are rejected too, both by
// encoding/json/v2's defaults (research D-28).
func DecodeBytes(raw []byte, target any) error {
	if target == nil {
		// A wiring mistake, not the client's fault, so it is not a
		// ValidationError: it must reach the operator as a 500 and not the
		// person as a rejected form.
		return errors.New("web: decoding into a nil target would accept every body and fill nothing")
	}

	if len(raw) == 0 {
		return bodyRefusal("a JSON object is required")
	}

	if err := json.Unmarshal(raw, target, json.RejectUnknownMembers(true)); err != nil {
		return decodeFailure(err)
	}

	return nil
}

// WriteJSON writes value as the response body.
//
// It is used in place of PocketBase's e.JSON, which applies a `?fields=` picker
// to every 2xx response (tools/router/event.go:191-207). contracts/README.md
// says there is no `?fields=` and that there are no partial responses: a picker
// would answer a documented DTO with an undocumented subset, silently, on a
// query parameter no OpenAPI operation declares.
//
// Deterministic makes a map marshal in sorted key order. Everything MediKube
// returns is a struct today, and struct member order is fixed either way — but
// FR-033's byte-identical refusal and the whole-body assertions that check it
// must not become flaky the first time somebody hands this a map.
func WriteJSON(e *core.RequestEvent, status int, value any) error {
	if e.Written() {
		return fmt.Errorf("web: %d cannot be written: the response has already gone", status)
	}

	e.Response.Header().Set(headerContentType, ContentTypeJSON)
	e.Response.WriteHeader(status)

	// AllowInvalidUTF8 mirrors PocketBase's own writer. A stored value with a
	// broken byte in it would otherwise fail to marshal, and a record nobody
	// can read back is worse than a replacement character in a name.
	return json.MarshalWrite(e.Response, value, json.Deterministic(true), jsontext.AllowInvalidUTF8(true))
}

// decodeFailure translates a decoder error into MediKube's own field errors,
// and deliberately drops everything the decoder said.
//
// Go's own text embeds the submitted value — `cannot unmarshal JSON number
// 99999999999999999999 into Go int`, and an RFC3339 parse failure quotes the
// string in full — and on this application's DTOs the submitted value is
// medical data. The JSON pointer is machine-recoverable and carries only the
// member name (research D-28).
//
// The member name goes through domain.SafeFieldName first, and it has to. For
// an unknown member the name is by definition one MediKube does not publish:
// it is whatever the client sent, unbounded and unfiltered, and it would
// otherwise reach both the response body and the one log stream verbatim.
func decodeFailure(err error) error {
	var invalid domain.ValidationError

	var semantic *json.SemanticError
	if errors.As(err, &semantic) {
		field := domain.SafeFieldName(semantic.JSONPointer.LastToken())
		if field == "" {
			return bodyRefusal("the request body is not a JSON object this operation accepts")
		}

		if errors.Is(semantic.Err, json.ErrUnknownName) {
			invalid.Add(field, domain.CodeUnknownField, "the field is not one this operation accepts")

			return invalid.OrNil()
		}

		invalid.Add(field, domain.CodeInvalidValue, "the value is not the shape this field takes")

		return invalid.OrNil()
	}

	// A duplicate member name is syntactic rather than semantic —
	// encoding/json/v2 refuses it with no option asked for — and its pointer
	// still names the member, which is a field name MediKube published.
	var syntactic *jsontext.SyntacticError
	if errors.As(err, &syntactic) {
		if field := domain.SafeFieldName(syntactic.JSONPointer.LastToken()); field != "" {
			invalid.Add(field, domain.CodeInvalidValue, "the field was sent more than once")

			return invalid.OrNil()
		}
	}

	return bodyRefusal("the request body is not a JSON object this operation accepts")
}

// bodyRefusal is the refusal that names no member, for the bodies that have
// none to name: an empty one, an array, a truncated one.
func bodyRefusal(message string) error {
	var invalid domain.ValidationError
	invalid.Add("body", domain.CodeInvalidValue, message)

	return invalid.OrNil()
}
