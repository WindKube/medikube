package clinical

import (
	"fmt"
	"time"

	"medikube/internal/domain"
)

// Ref names one field for a rule's refusal — the field an entity's Validate()
// should attach the *FieldError to, which is not always the field the rule
// reads (research D-04: "ended_on >= started_on" reports on ended_on).
type Ref struct {
	Field string
	Value Date
}

// Order refuses when later is set, earlier is set, and later is before
// earlier. Equality is accepted throughout this phase (a single-day course, a
// same-day resolution) — no caller asks for strict ordering.
func Order(earlier, later Ref) *domain.FieldError {
	if earlier.Value.IsZero() || later.Value.IsZero() {
		return nil
	}

	if later.Value.Before(earlier.Value) {
		return &domain.FieldError{
			Field:   later.Field,
			Code:    CodeEndBeforeStart,
			Message: fmt.Sprintf("%s is before %s", later.Field, earlier.Field),
		}
	}

	return nil
}

// CodeNotFuture is the refusal a date after today raises.
const CodeNotFuture = "not_future"

// NotFuture refuses a set date later than the given reference "today". An
// absent date passes: whether a field is required is a different rule.
func NotFuture(ref Ref, today Date) *domain.FieldError {
	if ref.Value.IsZero() || !ref.Value.After(today) {
		return nil
	}

	return &domain.FieldError{
		Field:   ref.Field,
		Code:    CodeNotFuture,
		Message: fmt.Sprintf("%s cannot be in the future", ref.Field),
	}
}

// RequiredWhen refuses an absent date when cond holds — condition.resolved_on
// required when status = resolved (FR-020), and its siblings.
func RequiredWhen(cond bool, ref Ref) *domain.FieldError {
	if !cond || !ref.Value.IsZero() {
		return nil
	}

	return &domain.FieldError{
		Field:   ref.Field,
		Code:    domain.CodeRequired,
		Message: fmt.Sprintf("%s is required", ref.Field),
	}
}

// Today is UTC midnight of the current instant — the reference NotFuture
// compares against everywhere in this phase, so "not future" means the same
// thing for every kind regardless of the caller's own zone.
func Today() Date {
	now := time.Now().UTC()
	d, _ := domain.NewDate(now.Year(), now.Month(), now.Day())

	return d
}
