package settings

import (
	"context"
	"strings"

	"medikube/internal/i18n"
	"medikube/internal/web/views/components"
	"medikube/internal/web/views/ids"
)

// hintSuffix names the element a control's hint renders into. It hangs off the
// control's own id, which ids composed, so it is unique for the same reason.
const hintSuffix = "-hint"

// fieldProps is one control. It is shared by the three forms for the same
// reason the two auth forms share theirs: a control copied per form is three
// places for aria-describedby to go missing.
type fieldProps struct {
	ID    string
	Name  string
	Label string
	Type  string
	Value string
	Hint  string

	Autocomplete string
	Required     bool

	Invalid     bool
	DescribedBy string
	ErrorID     string
	HintID      string
	Messages    []string

	// Options non-empty renders a select rather than an input.
	Options []Option

	// AriaLabel is an explicit accessible name, for a control the catalogue
	// names directly rather than through the adjacent <label> alone.
	AriaLabel string
}

// control assembles one, given the form's id and its refusals. The `extra`
// describes what the control points at besides its own refusal — the published
// rules, or the sentence that says what the phrase must be.
func control(ctx context.Context, formID string, errs components.FieldErrors, field fieldProps, extra ...string) fieldProps {
	field.ID = ids.Field(formID, field.Name)
	field.ErrorID = ids.FieldError(formID, field.Name)
	field.Invalid = errs.Has(field.Name)
	field.Messages = errs.Messages(ctx, field.Name)

	described := make([]string, 0, len(extra)+2)
	if field.Invalid {
		described = append(described, field.ErrorID)
	}

	// The hint is announced WITH the control rather than read as loose text
	// beside it, which for the deletion phrase is the difference between being
	// told what to type and being left to see it.
	if field.Hint != "" {
		field.HintID = field.ID + hintSuffix
		described = append(described, field.HintID)
	}

	described = append(described, extra...)
	field.DescribedBy = strings.Join(described, " ")

	return field
}

// Controls is the profile form's five, in the order FR-011 names them.
func (p ProfileProps) Controls(ctx context.Context) []fieldProps {
	return []fieldProps{
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldName, Label: i18n.T(ctx, "field.display_name"), Type: "text",
			Value: p.Name, Autocomplete: "name", Required: true,
		}),
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldUnitSystem, Label: i18n.T(ctx, "field.settings.measurement_units"), Options: p.UnitSystems,
		}),
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldLocale, Options: p.Locales, Required: true,
			Label:     i18n.T(ctx, "settings.language.label"),
			Hint:      i18n.T(ctx, "settings.language.description"),
			AriaLabel: i18n.T(ctx, "settings.language.label"),
		}),
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldDateFormat, Label: i18n.T(ctx, "field.settings.date_presentation"), Options: p.DateFormats,
		}),
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldTheme, Label: i18n.T(ctx, "field.settings.appearance"), Options: p.Themes,
		}),
	}
}

// Controls is the password form's two. The new password points at the published
// rules as well as at its own refusal, so the rules are announced with the
// control rather than sitting beside it unread.
func (p PasswordProps) Controls(ctx context.Context) []fieldProps {
	return []fieldProps{
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldCurrentPassword, Label: i18n.T(ctx, "field.settings.current_password"), Type: "password",
			Autocomplete: "current-password", Required: true,
		}),
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldNewPassword, Label: i18n.T(ctx, "field.new_password"), Type: "password",
			Autocomplete: "new-password", Required: true,
		}, PasswordRulesID),
	}
}

// RuleSentences is FR-004's published rules, in the same words the sign-up form
// states them and derived from the same value the check reads.
func (p PasswordProps) RuleSentences(ctx context.Context) []string {
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

// Controls is the deletion form's two proofs (FR-013).
func (p DangerZoneProps) Controls(ctx context.Context) []fieldProps {
	return []fieldProps{
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldPassword, Label: i18n.T(ctx, "field.settings.your_password"), Type: "password",
			Autocomplete: "current-password", Required: true,
		}),
		control(ctx, p.FormID, p.Errors, fieldProps{
			Name: FieldConfirmation, Label: i18n.T(ctx, "field.settings.type_phrase_to_confirm", map[string]any{"Phrase": p.Phrase}), Type: "text",
			Autocomplete: "off", Required: true,
			Hint: i18n.T(ctx, "field.settings.confirmation_hint"),
		}),
	}
}
