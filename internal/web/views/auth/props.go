package auth

import (
	"medikube/internal/domain/identity"
	"medikube/internal/web/views/components"
)

// The control names, which are the members of contracts/auth.md's
// LoginRequest, RegisterRequest, PasswordResetRequest, PasswordResetConfirm and
// EmailVerificationConfirm.
//
// One spelling reaches the control's `name`, its aria-describedby pair and the
// refusal lookup, so a control cannot be named one thing and refused under
// another. FieldPassword is the domain's own, because the rules that refuse a
// password report themselves against it. fields_test.go is the mechanical tie
// to the wire: it reads each member's json tag off the DTO, so a form that
// posted a member nobody decodes is a failing test rather than a 422 nobody
// sees until a person is standing in front of it.
const (
	FieldEmail    = "email"
	FieldName     = "name"
	FieldPassword = identity.FieldPassword

	// FieldToken is the link's own token, carried back with the new password
	// or the confirmation. It is a hidden control and not a member of the
	// address bar alone: the submission is a POST, and the token has to be in
	// its body.
	FieldToken = "token"

	// FieldPasswordConfirm is the second typing of a new password. The two are
	// compared at the edge before the token is spent, so a typo leaves the link
	// usable for the next attempt.
	FieldPasswordConfirm = "password_confirm"
)

// The five landmarks contracts/pages.md fixes for P1, P2, P7, P8 and P9. They
// are Playwright selectors, so they are constants and a change to any of them
// is a change to the browser gate.
const (
	SignInLandmark        = "Sign in"
	CreateAccountLandmark = "Create account"
	ResetPasswordLandmark = "Reset password"
	//nolint:gosec // an accessible name the browser gate resolves, not a credential
	NewPasswordLandmark       = "Choose a new password"
	EmailConfirmationLandmark = "Email confirmation"
)

// LoginProps is contracts/pages.md's P1.
type LoginProps struct {
	// FormID names the controls and their refusals. ids.SignInForm is the one
	// value this ever takes in the application; it is a member so a test can
	// render two forms into one tree without their ids colliding.
	FormID string

	// OnSubmit is the Datastar expression the submission runs. The page builds
	// it, because the address belongs to the route table and no view may spell
	// a path.
	OnSubmit string

	RegisterHref string
	ForgotHref   string

	// Email is what was typed, rendered back into the control. A form that
	// clears itself on a refusal makes a person retype the address they got
	// right (FR-027).
	Email string

	// SessionExpired is FR-008's other half: a person whose session ran out is
	// told why rather than being dropped on an ordinary sign-in page.
	SessionExpired bool

	Errors components.FieldErrors
}

// RegisterProps is contracts/pages.md's P2.
type RegisterProps struct {
	FormID string

	OnSubmit  string
	LoginHref string

	// Open is the operator's switch (FR-002). Closed, the landmark renders an
	// explanation instead of the controls — it does not disappear and the page
	// is not a 404, because whether self-registration is open is instance-wide
	// configuration, identical for every caller (defect D15).
	Open bool

	// Rules are published BEFORE the person chooses a password rather than
	// reported after they fail one (FR-004). They arrive as the domain's own
	// value, so the sentence below the control and the check that refuses the
	// password cannot state different numbers.
	Rules identity.PasswordRules

	Email string
	Name  string

	Errors components.FieldErrors
}

// ForgotPasswordProps is contracts/pages.md's P7.
type ForgotPasswordProps struct {
	FormID string

	OnSubmit  string
	LoginHref string

	// Mailable is FR-076's switch, read from the instance's own settings. False,
	// the landmark renders the explanation in place of the control — the
	// registration switch's shape and for the registration switch's reason: an
	// instance that cannot send anything must not collect an address as though
	// it had, and a form it was always going to refuse is a form it should not
	// offer.
	Mailable bool

	// Email is what was typed, rendered back into the control (FR-027).
	Email string

	Errors components.FieldErrors
}

// ResetPasswordProps is contracts/pages.md's P8.
type ResetPasswordProps struct {
	FormID string

	OnSubmit   string
	ForgotHref string

	// Token is the link's own, submitted back with the new password. It is
	// rendered into a hidden control rather than read from the address bar by a
	// script, because the page already has it and a script that read it would
	// be a second place it lives.
	Token string

	// Usable is whether the link still resolves, asked BEFORE the form is
	// rendered. False, the landmark carries FR-074's explanation and the offer
	// to ask for another, and no control at all: a form that collected a new
	// password against a dead link would refuse it after the person had chosen
	// one.
	Usable bool

	// Mailable decides what the dead-link state can honestly offer. Asking for
	// another link is only an offer on an instance that can send one.
	Mailable bool

	// Rules are published BEFORE the person chooses (FR-004), from the domain
	// value the check itself reads — the same rules registration states, because
	// a password chosen here is held to them too (FR-074).
	Rules identity.PasswordRules

	Errors components.FieldErrors
}

// VerifyEmailProps is contracts/pages.md's P9.
//
// A region and not a form, and one control inside it rather than a confirmation
// performed by the GET: opening a link must not change anything, or every
// scanner that follows links in mail would spend the token before the person
// read the message.
type VerifyEmailProps struct {
	FormID string

	OnConfirm string
	LoginHref string

	Token string

	// Usable is whether the link still resolves AND has not already been used.
	// An address that is already confirmed is a spent link — PocketBase does not
	// invalidate a confirmation token when it is redeemed, so the service
	// refuses the second use itself, and this page shows the same state rather
	// than offering a control that would be refused.
	Usable bool

	Mailable bool
}

// The elements a test and the browser gate address by id: the two states that
// are the whole point of their page, and the published rules a control points
// at with aria-describedby.
const (
	// SessionExpiredID is FR-008's explanation, rendered only when the person
	// actually arrived from an expired session.
	SessionExpiredID = "sign-in-session-expired"

	// RegistrationClosedID is FR-002's explanation, rendered inside the
	// landmark in place of the controls.
	RegistrationClosedID = "create-account-closed"

	// PasswordRulesID is FR-004's published rules. The password control names
	// it in aria-describedby, so the rules are announced with the control
	// rather than sitting beside it unread.
	PasswordRulesID = "create-account-password-rules"

	// NewPasswordRulesID is the same rules on the recovery form, from the same
	// domain value. A password chosen through a recovery link is held to the
	// rules registration publishes (FR-004, FR-074), so it is told them in the
	// same words and before it is chosen.
	NewPasswordRulesID = "new-password-rules"

	// LinkDeadID is FR-074's refusal, rendered INSIDE the landmark of the page
	// the link opened rather than as an error view of its own: a person whose
	// link expired is on the right page and needs the offer to ask for another,
	// not a 404. Expired, already used and altered are one state here for the
	// same reason they are one refusal at the API — telling them apart tells
	// somebody which links once existed.
	LinkDeadID = "recovery-link-dead"

	// MailUnconfiguredID is FR-076 on the page. An instance with no outgoing
	// mail says so plainly, inside the landmark, in place of a control that
	// would have asked for an address it could do nothing with.
	MailUnconfiguredID = "recovery-mail-unconfigured"
)

// DescribedBy is the value of one control's aria-describedby.
//
// It is a SPACE-SEPARATED LIST because the password control is described by two
// things at once — the rules it must meet and the refusal it just collected —
// and a control that dropped one of them would announce either the rules or the
// reason, never both.
func describedBy(errs components.FieldErrors, formID, field string, extra ...string) string {
	described := make([]string, 0, len(extra)+1)

	if errs.Has(field) {
		described = append(described, fieldErrorID(formID, field))
	}

	described = append(described, extra...)

	return join(described)
}
