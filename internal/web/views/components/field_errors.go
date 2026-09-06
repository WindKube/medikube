package components

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/i18n"
)

// FieldErrors is a *domain.ValidationError as a form reads it: every refusal
// for a field at once, because aria-describedby names one element and a second
// message rendered outside it is a message nobody hears.
//
// It lives beside FieldError rather than in one kind's view package because
// three of them render forms now — the record forms, the two auth forms and the
// three settings forms — and a second copy of this type would be a second
// answer to "which refusals does this control carry".
type FieldErrors struct {
	byField map[string][]fieldRefusal
	fields  []string
}

// fieldRefusal is one domain.FieldError as this layer keeps it: the code,
// which is what a translation is keyed by, and the domain's own English
// message, which is what a code without a catalogue entry falls back to
// (T031, US1-4).
type fieldRefusal struct {
	Code    string
	Message string
}

// explanationID is the catalogue entry a refusal's code resolves to. Only the
// codes internal/domain declares itself (constitution-shared, never naming a
// submitted value) are covered; an entity-specific code (end_before_start,
// same_as_email, …) has no generic phrasing that would not repeat what the
// domain's own message already says, so it falls back to that message
// (English) rather than to a bare id.
func (r fieldRefusal) explanation(ctx context.Context) string {
	switch r.Code {
	case domain.CodeRequired, domain.CodeInvalidValue, domain.CodeInvalidDate,
		domain.CodeTooLong, domain.CodeTooShort, domain.CodeOutOfRange, domain.CodeUnknownField:
		return i18n.T(ctx, "error.field."+r.Code)
	default:
		return r.Message
	}
}

// NewFieldErrors takes nil, which is the clean form. The zero value works for
// the same reason.
func NewFieldErrors(invalid *domain.ValidationError) FieldErrors {
	if invalid == nil || invalid.Empty() {
		return FieldErrors{}
	}

	errs := FieldErrors{byField: make(map[string][]fieldRefusal, len(invalid.Fields))}

	for _, refusal := range invalid.Fields {
		if _, seen := errs.byField[refusal.Field]; !seen {
			errs.fields = append(errs.fields, refusal.Field)
		}
		errs.byField[refusal.Field] = append(errs.byField[refusal.Field], fieldRefusal{Code: refusal.Code, Message: refusal.Message})
	}

	return errs
}

func (f FieldErrors) Has(field string) bool { return len(f.byField[field]) > 0 }

// Messages resolves every refusal a field carries in ctx's language: a
// catalogue phrase for the codes internal/domain shares across entities,
// falling back to the domain's own English message for one that carries no
// generic phrasing (US1-4, T031).
func (f FieldErrors) Messages(ctx context.Context, field string) []string {
	refusals := f.byField[field]
	if len(refusals) == 0 {
		return nil
	}

	messages := make([]string, len(refusals))
	for i, refusal := range refusals {
		messages[i] = refusal.explanation(ctx)
	}

	return messages
}

// Fields are the refused fields in the order the rules found them, which is the
// order the form renders and therefore the order the person reads.
func (f FieldErrors) Fields() []string { return append([]string(nil), f.fields...) }
