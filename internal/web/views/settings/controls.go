package settings

import (
	"strconv"
	"strings"

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
}

// control assembles one, given the form's id and its refusals. The `extra`
// describes what the control points at besides its own refusal — the published
// rules, or the sentence that says what the phrase must be.
func control(formID string, errs components.FieldErrors, field fieldProps, extra ...string) fieldProps {
	field.ID = ids.Field(formID, field.Name)
	field.ErrorID = ids.FieldError(formID, field.Name)
	field.Invalid = errs.Has(field.Name)
	field.Messages = errs.Messages(field.Name)

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
func (p ProfileProps) Controls() []fieldProps {
	return []fieldProps{
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldName, Label: "Display name", Type: "text",
			Value: p.Name, Autocomplete: "name", Required: true,
		}),
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldUnitSystem, Label: "Measurement units", Options: p.UnitSystems,
		}),
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldLocale, Label: "Language", Type: "text", Value: p.Locale, Required: true,
			Hint: "A two-letter code, optionally with a region, such as en or en-GB.",
		}),
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldDateFormat, Label: "Date presentation", Options: p.DateFormats,
		}),
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldTheme, Label: "Appearance", Options: p.Themes,
		}),
	}
}

// Controls is the password form's two. The new password points at the published
// rules as well as at its own refusal, so the rules are announced with the
// control rather than sitting beside it unread.
func (p PasswordProps) Controls() []fieldProps {
	return []fieldProps{
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldCurrentPassword, Label: "Current password", Type: "password",
			Autocomplete: "current-password", Required: true,
		}),
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldNewPassword, Label: "New password", Type: "password",
			Autocomplete: "new-password", Required: true,
		}, PasswordRulesID),
	}
}

// RuleSentences is FR-004's published rules, in the same words the sign-up form
// states them and derived from the same value the check reads.
func (p PasswordProps) RuleSentences() []string {
	sentences := []string{
		"At least " + strconv.Itoa(p.Rules.MinLength) + " characters, and at most " +
			strconv.Itoa(p.Rules.MaxLength) + ".",
	}

	if p.Rules.RejectsEmail {
		sentences = append(sentences, "Not your email address.")
	}

	if p.Rules.RejectsName {
		sentences = append(sentences, "Not your display name.")
	}

	return sentences
}

// Controls is the deletion form's two proofs (FR-013).
func (p DangerZoneProps) Controls() []fieldProps {
	return []fieldProps{
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldPassword, Label: "Your password", Type: "password",
			Autocomplete: "current-password", Required: true,
		}),
		control(p.FormID, p.Errors, fieldProps{
			Name: FieldConfirmation, Label: "Type " + p.Phrase + " to confirm", Type: "text",
			Autocomplete: "off", Required: true,
			Hint: "Exactly as written, in capitals. Nothing else deletes the account.",
		}),
	}
}
