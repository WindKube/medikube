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
