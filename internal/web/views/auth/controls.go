package auth

import (
	"context"
	"strings"

	"medikube/internal/i18n"
	"medikube/internal/web/views/ids"
)

// fieldErrorID and fieldID are ids' spellings, reached through one function
// each so the templates below hold no id composition of their own.
func fieldID(formID, field string) string { return ids.Field(formID, field) }

func fieldErrorID(formID, field string) string { return ids.FieldError(formID, field) }

func join(described []string) string { return strings.Join(described, " ") }

// DescribedBy is LoginProps' half of the shared helper: the sign-in controls
// are described by their refusal and by nothing else.
func (p LoginProps) DescribedBy(field string) string {
	return describedBy(p.Errors, p.FormID, field)
}

// DescribedBy on the sign-up form adds the published rules to the password
// control, which is what makes FR-004's sentence part of the control rather
// than prose near it.
func (p RegisterProps) DescribedBy(field string) string {
	if field == FieldPassword {
		return describedBy(p.Errors, p.FormID, field, PasswordRulesID)
	}

	return describedBy(p.Errors, p.FormID, field)
}

// FieldID and ErrorID are what the templates call, so neither spells an id.
func (p LoginProps) FieldID(field string) string { return fieldID(p.FormID, field) }

func (p LoginProps) ErrorID(field string) string { return fieldErrorID(p.FormID, field) }

func (p RegisterProps) FieldID(field string) string { return fieldID(p.FormID, field) }

func (p RegisterProps) ErrorID(field string) string { return fieldErrorID(p.FormID, field) }

// fieldProps is one control, assembled by the props type that owns it so the
// two forms share a control rather than sharing a copy of one.
//
// It is a struct and not eight parameters because the template renders every
// member and a positional call with two adjacent strings is one transposition
// away from labelling a password "Email address".
type fieldProps struct {
	ID    string
	Name  string
	Label string
	Type  string
	Value string

	// Autocomplete is what tells a password manager which of the two password
	// controls in this application it is looking at. Absent, browsers offer the
	// current password where a new one is being chosen.
	Autocomplete string

	Required bool

	Invalid     bool
	DescribedBy string
	ErrorID     string
	Messages    []string
}

func (p LoginProps) field(ctx context.Context, field, label, inputType, autocomplete, value string) fieldProps {
	return fieldProps{
		ID:           p.FieldID(field),
		Name:         field,
		Label:        label,
		Type:         inputType,
		Value:        value,
		Autocomplete: autocomplete,
		Required:     true,
		Invalid:      p.Errors.Has(field),
		DescribedBy:  p.DescribedBy(field),
		ErrorID:      p.ErrorID(field),
		Messages:     p.Errors.Messages(ctx, field),
	}
}

func (p RegisterProps) field(ctx context.Context, field, label, inputType, autocomplete, value string) fieldProps {
	return fieldProps{
		ID:           p.FieldID(field),
		Name:         field,
		Label:        label,
		Type:         inputType,
		Value:        value,
		Autocomplete: autocomplete,
		Required:     true,
		Invalid:      p.Errors.Has(field),
		DescribedBy:  p.DescribedBy(field),
		ErrorID:      p.ErrorID(field),
		Messages:     p.Errors.Messages(ctx, field),
	}
}

// RuleSentences is FR-004's published rules in the words the form states them.
//
// Every sentence is derived from the PasswordRules value the check itself
// reads, so a rule switched off in the domain stops being enforced and stops
// being stated in one edit, and the number below the control is the number that
// refuses the password.
func (p RegisterProps) RuleSentences(ctx context.Context) []string {
	sentences := []string{
		i18n.T(ctx, "auth.password_rule_length", map[string]any{
			"Min": i18n.N(ctx, "auth.password_length_unit", p.Rules.MinLength),
			"Max": i18n.N(ctx, "auth.password_length_unit", p.Rules.MaxLength),
		}),
	}

	if p.Rules.RejectsEmail {
		sentences = append(sentences, i18n.T(ctx, "auth.password_rule_not_email"))
	}

	if p.Rules.RejectsName {
		sentences = append(sentences, i18n.T(ctx, "auth.password_rule_not_name"))
	}

	return sentences
}

// DescribedBy on the recovery form is LoginProps' shape: the address control is
// described by its refusal and by nothing else.
func (p ForgotPasswordProps) DescribedBy(field string) string {
	return describedBy(p.Errors, p.FormID, field)
}

// DescribedBy on the new-password form adds the published rules to the first
// password control, exactly as the sign-up form's does — the rules are
// announced with the control rather than sitting beside it unread. The second
// typing points only at its own refusal, because the rules it is held to are
// the first one's.
func (p ResetPasswordProps) DescribedBy(field string) string {
	if field == FieldPassword {
		return describedBy(p.Errors, p.FormID, field, NewPasswordRulesID)
	}

	return describedBy(p.Errors, p.FormID, field)
}

func (p ForgotPasswordProps) FieldID(field string) string { return fieldID(p.FormID, field) }

func (p ForgotPasswordProps) ErrorID(field string) string { return fieldErrorID(p.FormID, field) }

func (p ResetPasswordProps) FieldID(field string) string { return fieldID(p.FormID, field) }

func (p ResetPasswordProps) ErrorID(field string) string { return fieldErrorID(p.FormID, field) }

// FieldID on P9 names the one control the region holds. There is no ErrorID
// beside it because there is nothing to refuse: the region carries no value a
// person typed.
func (p VerifyEmailProps) FieldID(field string) string { return fieldID(p.FormID, field) }

func (p ForgotPasswordProps) field(ctx context.Context, field, label, inputType, autocomplete, value string) fieldProps {
	return fieldProps{
		ID:           p.FieldID(field),
		Name:         field,
		Label:        label,
		Type:         inputType,
		Value:        value,
		Autocomplete: autocomplete,
		Required:     true,
		Invalid:      p.Errors.Has(field),
		DescribedBy:  p.DescribedBy(field),
		ErrorID:      p.ErrorID(field),
		Messages:     p.Errors.Messages(ctx, field),
	}
}

func (p ResetPasswordProps) field(ctx context.Context, field, label, inputType, autocomplete, value string) fieldProps {
	return fieldProps{
		ID:           p.FieldID(field),
		Name:         field,
		Label:        label,
		Type:         inputType,
		Value:        value,
		Autocomplete: autocomplete,
		Required:     true,
		Invalid:      p.Errors.Has(field),
		DescribedBy:  p.DescribedBy(field),
		ErrorID:      p.ErrorID(field),
		Messages:     p.Errors.Messages(ctx, field),
	}
}

// RuleSentences is FR-004's published rules in the words the recovery form
// states them, which are the sign-up form's words from the sign-up form's
// value: one rule set, stated wherever a password is chosen (FR-074).
func (p ResetPasswordProps) RuleSentences(ctx context.Context) []string {
	return RegisterProps{Rules: p.Rules}.RuleSentences(ctx)
}

// deadLinkOffer is what FR-074's explanation can offer the person holding a
// link that no longer works.
//
// It carries the offer rather than the page deciding inside the template,
// because the two pages offer different things: a recovery link is asked for
// again from the recovery page, and a confirmation is asked for again by the
// signed-in account holder from their settings. Neither is an offer at all on
// an instance that cannot send mail, which is what Mailable decides.
type deadLinkOffer struct {
	Mailable bool
	Href     string
	Label    string
}

func (p ResetPasswordProps) deadLink(ctx context.Context) deadLinkOffer {
	return deadLinkOffer{Mailable: p.Mailable, Href: p.ForgotHref, Label: i18n.T(ctx, "auth.ask_for_new_link")}
}

func (p VerifyEmailProps) deadLink(ctx context.Context) deadLinkOffer {
	return deadLinkOffer{
		Mailable: p.Mailable,
		Href:     p.LoginHref,
		Label:    i18n.T(ctx, "auth.ask_for_new_confirmation"),
	}
}
