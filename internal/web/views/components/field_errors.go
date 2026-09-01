package components

import "medikube/internal/domain"

// FieldErrors is a *domain.ValidationError as a form reads it: every refusal
// for a field at once, because aria-describedby names one element and a second
// message rendered outside it is a message nobody hears.
//
// It lives beside FieldError rather than in one kind's view package because
// three of them render forms now — the record forms, the two auth forms and the
// three settings forms — and a second copy of this type would be a second
// answer to "which refusals does this control carry".
type FieldErrors struct {
	byField map[string][]string
	fields  []string
}

// NewFieldErrors takes nil, which is the clean form. The zero value works for
// the same reason.
func NewFieldErrors(invalid *domain.ValidationError) FieldErrors {
	if invalid == nil || invalid.Empty() {
		return FieldErrors{}
	}

	errs := FieldErrors{byField: make(map[string][]string, len(invalid.Fields))}

	for _, refusal := range invalid.Fields {
		if _, seen := errs.byField[refusal.Field]; !seen {
			errs.fields = append(errs.fields, refusal.Field)
		}
		errs.byField[refusal.Field] = append(errs.byField[refusal.Field], refusal.Message)
	}

	return errs
}

func (f FieldErrors) Has(field string) bool { return len(f.byField[field]) > 0 }

func (f FieldErrors) Messages(field string) []string { return f.byField[field] }

// Fields are the refused fields in the order the rules found them, which is the
// order the form renders and therefore the order the person reads.
func (f FieldErrors) Fields() []string { return append([]string(nil), f.fields...) }
