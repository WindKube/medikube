package domain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FR-027: a form shows every problem at once. A validation type that stops at
// the first offending field makes that impossible no matter how the handler is
// written, so this is a property of the type and not of its callers.
func TestValidationErrorCarriesEveryOffendingField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		add   func(v *ValidationError)
		want  []FieldError
		isErr bool
	}{
		{
			name: "nothing added is not an error",
			add:  func(*ValidationError) {},
			want: nil,
		},
		{
			name: "one field",
			add: func(v *ValidationError) {
				v.Add("name", CodeRequired, "a name is required")
			},
			want:  []FieldError{{Field: "name", Code: CodeRequired, Message: "a name is required"}},
			isErr: true,
		},
		{
			name: "four simultaneous faults are four entries, in the order they were found",
			add: func(v *ValidationError) {
				v.Add("name", CodeRequired, "a name is required")
				v.Add("dosage", CodeTooLong, "at most 200 characters")
				v.Add("status", CodeInvalidValue, "not one of the accepted values")
				v.Add("ended_on", "end_before_start", "the end date is before the start date")
			},
			want: []FieldError{
				{Field: "name", Code: CodeRequired, Message: "a name is required"},
				{Field: "dosage", Code: CodeTooLong, Message: "at most 200 characters"},
				{Field: "status", Code: CodeInvalidValue, Message: "not one of the accepted values"},
				{Field: "ended_on", Code: "end_before_start", Message: "the end date is before the start date"},
			},
			isErr: true,
		},
		{
			name: "one field can fail twice",
			add: func(v *ValidationError) {
				v.Add("ended_on", CodeInvalidDate, "not a real calendar date")
				v.Add("ended_on", "end_before_start", "the end date is before the start date")
			},
			want: []FieldError{
				{Field: "ended_on", Code: CodeInvalidDate, Message: "not a real calendar date"},
				{Field: "ended_on", Code: "end_before_start", Message: "the end date is before the start date"},
			},
			isErr: true,
		},
		{
			name: "Addf composes the limit into the message",
			add: func(v *ValidationError) {
				v.Addf("notes", CodeTooLong, "at most %d characters", 5000)
			},
			want:  []FieldError{{Field: "notes", Code: CodeTooLong, Message: "at most 5000 characters"}},
			isErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// The zero value accumulates: a Validate() method should not have to
			// construct anything before it knows whether it has a complaint.
			var v ValidationError
			test.add(&v)

			assert.Equal(t, test.want, v.Fields)
			assert.Equal(t, !test.isErr, v.Empty())

			err := v.OrNil()
			if !test.isErr {
				// Literal nil, not a nil *ValidationError in an error interface:
				// the second is non-nil to every caller and turns a valid record
				// into a 422.
				require.NoError(t, err)
				assert.Nil(t, err)
				return
			}
			require.Error(t, err)

			var target *ValidationError
			require.True(t, errors.As(fmt.Errorf("service: %w", err), &target),
				"a wrapped *ValidationError must still be reachable with errors.As")
			assert.Equal(t, test.want, target.Fields)
		})
	}
}

// Constitution VII. The message a rule authors names the field and its limit;
// the value the person typed is never in it. Error() is what reaches the log,
// so it carries the field and the code and nothing a rule's author could have
// interpolated.
func TestValidationErrorTextCarriesNoAuthoredMessage(t *testing.T) {
	t.Parallel()

	var v ValidationError
	v.Add("name", CodeTooLong, "Amoxicillin 500mg twice daily")
	v.Add("dosage", CodeInvalidValue, "17 units")

	text := v.OrNil().Error()

	assert.Contains(t, text, "name")
	assert.Contains(t, text, CodeTooLong)
	assert.Contains(t, text, "dosage")
	assert.Contains(t, text, CodeInvalidValue)
	assert.NotContains(t, text, "Amoxicillin", "the authored message reached the error text")
	assert.NotContains(t, text, "17 units", "the authored message reached the error text")
}

func TestValidationCodesAreTheDocumentedSpellings(t *testing.T) {
	t.Parallel()

	// data-model.md's validation tables and contracts/README.md name these
	// exactly. They are the machine codes a client switches on, so a typo here
	// is a silent contract break rather than a compile error.
	assert.Equal(t, "validation_failed", CodeValidationFailed)
	assert.Equal(t, "required", CodeRequired)
	assert.Equal(t, "invalid_value", CodeInvalidValue)
	assert.Equal(t, "invalid_date", CodeInvalidDate)
	assert.Equal(t, "too_long", CodeTooLong)
	assert.Equal(t, "too_short", CodeTooShort)
	assert.Equal(t, "unknown_field", CodeUnknownField)
}
