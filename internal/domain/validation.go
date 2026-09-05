package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The machine codes shared across entities. A code is what a client switches on
// and what a translation is keyed by, so it is a constant here rather than a
// string literal at each call site; the entity-specific ones (end_before_start,
// same_as_email) stay with the rule that raises them.
const (
	CodeValidationFailed = "validation_failed"
	CodeRequired         = "required"
	CodeInvalidValue     = "invalid_value"
	CodeInvalidDate      = "invalid_date"
	CodeTooLong          = "too_long"
	CodeTooShort         = "too_short"
	CodeOutOfRange       = "out_of_range"
	CodeUnknownField     = "unknown_field"
)

// ValidationMessage is constant on purpose. The per-field messages carry the
// detail; this one is read by a person who has not opened the form yet, and a
// message assembled from the submission is a PHI leak waiting for a log line.
const ValidationMessage = "one or more fields were rejected"

// FieldError is one refusal, attached to the field it concerns. The message
// names the field and its limit — never the value the person typed, which is
// medical data on almost every field MediKube has (constitution VII).
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError is every refusal a Validate() found, not the first one.
// FR-027 requires a form to show all of its problems at once, and a type that
// holds a single field makes that unreachable however the handler is written.
//
// The zero value accumulates, so a Validate() constructs nothing until it has
// something to complain about:
//
//	var v domain.ValidationError
//	if m.Name == "" {
//		v.Add("name", domain.CodeRequired, "a name is required")
//	}
//	return v.OrNil()
type ValidationError struct {
	// In the order the rules found them, which is the order the form renders.
	Fields []FieldError
}

func (e *ValidationError) Add(field, code, message string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Code: code, Message: message})
}

// Addf composes a message from a limit or a vocabulary — never from the
// submitted value.
func (e *ValidationError) Addf(field, code, format string, args ...any) {
	e.Add(field, code, fmt.Sprintf(format, args...))
}

func (e *ValidationError) Empty() bool { return len(e.Fields) == 0 }

// OrNil is the return statement of every Validate(). It returns an untyped nil
// rather than a nil *ValidationError in an error interface, because the second
// is non-nil to every caller and would refuse a valid record.
func (e *ValidationError) OrNil() error {
	if e.Empty() {
		return nil
	}
	return e
}

// Error carries the fields and their codes and none of the authored messages:
// this string is what reaches the log and Sentry, and a rule that interpolated
// a value into its message would otherwise have leaked it there.
func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, field := range e.Fields {
		parts = append(parts, field.Field+" ("+field.Code+")")
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// MarshalJSON emits the error envelope of contracts/README.md without
// request_id, which is minted at the HTTP edge and is the edge's to add along
// with the outer {"error": …} object.
//
// Fields marshals as [] and never null: the contract says so, and Go 1.27's
// encoding/json/v2 retrofit is not backward compatible on exactly that point
// (research D-28).
func (e *ValidationError) MarshalJSON() ([]byte, error) {
	fields := e.Fields
	if fields == nil {
		fields = []FieldError{}
	}
	return json.Marshal(struct {
		Code    string       `json:"code"`
		Message string       `json:"message"`
		Fields  []FieldError `json:"fields"`
	}{Code: CodeValidationFailed, Message: ValidationMessage, Fields: fields})
}

// MaxFieldNameLen bounds the field name in a refusal.
//
// MediKube's own longest published member name is under twenty characters, and
// a client's typo of one is about as long. Sixty-four is generous for the case
// this has to stay useful for and short enough that the case it exists for —
// a member name of a megabyte — costs one log line the size of a tweet.
const MaxFieldNameLen = 64

// SafeFieldName is what a member name a client sent must pass through before
// it reaches a refusal.
//
// For json.ErrUnknownName the token is BY DEFINITION a name MediKube does not
// publish: it is attacker-controlled free text, and it goes into the response
// body and into the one log stream. Unbounded, that is a log-volume vector and
// a general-purpose echo channel in an application that holds medical records.
//
// The policy is truncate-then-restrict, in that order so that the work is
// bounded by the limit rather than by what was sent:
//
//   - Everything past MaxFieldNameLen bytes is dropped.
//   - Every byte outside [A-Za-z0-9_-] becomes '_'. That is the whole
//     vocabulary MediKube's own DTO members are spelled in, so a legitimate
//     name survives byte for byte and a typo of one still names the field the
//     client meant. A control character, a newline, a quote and a UTF-8
//     continuation byte all do not — including the split rune truncation can
//     leave behind, so the result is always plain ASCII whatever the encoder
//     downstream does with it.
//
// Substitution rather than rejection, because "the field is not one this
// operation accepts" naming `na_me` still tells a developer where to look,
// and naming nothing does not.
//
// The empty string is returned unchanged: a pointer with no member to name is
// a different refusal, and each caller has its own.
func SafeFieldName(name string) string {
	if name == "" {
		return ""
	}

	if len(name) > MaxFieldNameLen {
		name = name[:MaxFieldNameLen]
	}

	safe := []byte(name)

	for i, b := range safe {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '_', b == '-':
		default:
			safe[i] = '_'
		}
	}

	return string(safe)
}
