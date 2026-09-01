package settings

import (
	"medikube/internal/domain/identity"
	"medikube/internal/web/views/components"
)

// The landmarks. contracts/pages.md fixes P6's — region[name="Settings"] — as a
// Playwright selector, so changing it is a change to the browser gate; the four
// inside it are this page's own and are named here for the same reason, because
// a test and a template that spelled them separately would drift.
const (
	SettingsLandmark      = "Settings"
	ProfileLandmark       = "Profile"
	PasswordLandmark      = "Change password"
	DangerZoneLandmark    = "Danger zone"
	DeleteAccountLandmark = "Delete account"
)

// The control names, which are the members of contracts/account.md's MePatch,
// ChangePasswordRequest and DeleteAccountRequest.
//
// They are spelled here rather than imported from the service because a view
// that imported a service would be a view that could call one. fields_test.go
// is the mechanical tie: it asserts each of these equals the constant the
// refusal is raised against, so a rename there is a failing test here rather
// than a form reporting an error against a control nobody can see.
const (
	FieldName       = "name"
	FieldUnitSystem = "unit_system"
	FieldLocale     = "locale"
	FieldDateFormat = "date_format"
	FieldTheme      = "theme"

	FieldCurrentPassword = "current_password"
	FieldNewPassword     = identity.FieldNewPassword

	FieldPassword     = identity.FieldPassword
	FieldConfirmation = "confirmation"
)

// The elements addressed by id: the two states of FR-075 and the published
// password rules the new-password control points at.
const (
	// EmailUnconfirmedID is FR-075's "not confirmed, send it again" state.
	// Account C is seeded unconfirmed so this is a state the smoke run walks
	// through rather than a branch somebody asserted once (T222).
	EmailUnconfirmedID = "settings-email-unconfirmed"

	// EmailConfirmedID is the other half. Both exist so a test can assert that
	// exactly one of them is rendered.
	EmailConfirmedID = "settings-email-confirmed"

	// PasswordRulesID is FR-004's rules, published on the change form too: a
	// password chosen here is held to the same rules as one chosen at
	// registration, so it is stated in the same words.
	PasswordRulesID = "settings-password-rules"

	// HoldingsID is what deletion will destroy, named and counted. FR-013
	// requires the consequence to be stated plainly beforehand rather than
	// taken on trust.
	HoldingsID = "settings-delete-holdings"
)

// Option is one choice of a preference control. It carries its own Selected
// rather than the control comparing values, so the template holds no equality.
type Option struct {
	Value    string
	Label    string
	Selected bool
}

// Holding is one row of what an account holds, as the deletion confirmation
// states it.
//
// The label is DATA and not a literal: it arrives as the kind's own published
// path segment, read off the counts the API answered with, because a view that
// spelled a kind's plural would be the fourth spelling of it and the one
// nothing checks (research D-05).
type Holding struct {
	Label string
	Count int
}

// SettingsProps is the whole of contracts/pages.md's P6.
type SettingsProps struct {
	Profile  ProfileProps
	Password PasswordProps
	Danger   DangerZoneProps

	// SignOutOn is the Datastar expression the sign-out control runs. The page
	// builds it, because the address belongs to the route table.
	SignOutOn string
}

// ProfileProps is FR-011's five changeable things, plus the address, which is
// shown and cannot be changed here (contracts/account.md).
type ProfileProps struct {
	FormID   string
	OnSubmit string

	// Email is displayed and is not a control. FR-011 enumerates what a person
	// may change about themselves and the sign-in address is not among them;
	// MePatch has no member for it either, so a control here would be inviting
	// a 422.
	Email string

	// EmailConfirmed is FR-075's third clause: the account holder is shown
	// whether their address is confirmed.
	EmailConfirmed bool

	// ResendOn is the Datastar expression the "send it again" control runs. It
	// carries no address: requestEmailVerification reads no body at all, so
	// there is no shape in which a signed-in caller could aim the message at a
	// stranger.
	ResendOn string

	Name string

	UnitSystems []Option
	Locale      string
	DateFormats []Option
	Themes      []Option

	Errors components.FieldErrors
}

// PasswordProps is FR-009's proof and the replacement.
type PasswordProps struct {
	FormID   string
	OnSubmit string

	// Rules are the published ones, stated here as well as at registration
	// because FR-074 and FR-004 hold a password to one rule set wherever it is
	// chosen.
	Rules identity.PasswordRules

	Errors components.FieldErrors
}

// DangerZoneProps is FR-013's one irreversible operation.
type DangerZoneProps struct {
	FormID   string
	OnSubmit string

	// Phrase is what must be typed, spelled once in the domain so the form that
	// asks for it and the check that compares it cannot differ by a space.
	Phrase string

	// Holdings is what will be destroyed, stated before either credential is
	// asked for.
	Holdings []Holding

	Errors components.FieldErrors
}
