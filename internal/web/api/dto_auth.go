package api

import (
	"medikube/internal/domain/identity"
)

// MemberPasswordConfirm is the only member of contracts/auth.md's bodies that a
// refusal is ever raised against from this layer. The rest are named by the
// domain and the service — identity.FieldPassword, identity.FieldNewPassword,
// identity.FieldCurrentPassword — and dto_test.go asserts the struct tags here
// agree with them, so a rename in one place cannot leave a form reporting an
// error against a member nobody can see.
const MemberPasswordConfirm = "password_confirm"

// AuthConfig is what GET /api/v1/auth/config publishes.
//
// It says nothing about anybody: not how many accounts exist, not whether a
// given address is registered, not what any of them holds. That is what makes
// it safe to serve to an anonymous caller, and it is the reason the operation
// exists at all — the sign-up form has to state the password rules BEFORE a
// person chooses one, and a rule discovered by violating it is not published
// (FR-004).
type AuthConfig struct {
	RegistrationOpen bool    `json:"registration_open"`
	PasswordRules    PwRules `json:"password_rules"`
}

// PwRules is FR-004's rules on the wire.
//
// Every member is read from identity.PublishedPasswordRules, which is the same
// value ValidatePassword enforces. The two cannot drift: a rule switched off in
// the domain stops being enforced AND stops being published, in one edit.
type PwRules struct {
	MinLength    int  `json:"min_length"`
	MaxLength    int  `json:"max_length"`
	RejectsEmail bool `json:"rejects_email"`
	RejectsName  bool `json:"rejects_name"`
}

// NewAuthConfig renders the instance's answer.
func NewAuthConfig(registrationOpen bool) AuthConfig {
	rules := identity.PublishedPasswordRules()

	return AuthConfig{
		RegistrationOpen: registrationOpen,
		PasswordRules: PwRules{
			MinLength:    rules.MinLength,
			MaxLength:    rules.MaxLength,
			RejectsEmail: rules.RejectsEmail,
			RejectsName:  rules.RejectsName,
		},
	}
}

// RegisterRequest is the whole of what a sign-up may say.
//
// There is no Role, no DisabledAt and no Verified member, and their absence IS
// the enforcement of FR-012 rather than a check somebody could forget: unknown
// members are rejected by the decoder, so a body carrying any of the three is
// 422 `unknown_field` before a handler has an opinion. A runtime check would be
// one edit away from being deleted; a member that does not exist is not.
type RegisterRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// LoginRequest is one sign-in attempt.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Session is the answer to register, login and refresh.
//
// IT CARRIES NO TOKEN. The token is the HttpOnly cookie and nothing else
// (research D-15): a token in this body is a token a script can read, and the
// content security policy grants 'unsafe-eval' for Datastar's expression
// compiler, so an injected expression that could read it would have it.
//
// This is also why MediKube writes this body itself rather than letting
// PocketBase write its own. apis.RecordAuthResponse's native shape is
// {"record": <the whole record>, "token": "<jwt>"} — the token in the body, and
// `role`, `verified` and `disabled_at` beside it.
type Session struct {
	User      Me     `json:"user"`
	ExpiresAt string `json:"expires_at"`
}

// PasswordResetRequest asks for a recovery message for one address.
type PasswordResetRequest struct {
	Email string `json:"email"`
}

// PasswordResetConfirm sets a new password from a recovery link.
type PasswordResetConfirm struct {
	Token           string `json:"token"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"password_confirm"`
}

// EmailVerificationConfirm confirms an address from a confirmation link.
type EmailVerificationConfirm struct {
	Token string `json:"token"`
}

// Acknowledgement is the body of every accepted recovery or confirmation
// request (contracts/auth.md).
//
// It has one member and that member is a constant per operation, which is the
// enumeration defence expressed on the wire: there is nothing here an account's
// existence could vary. The value for a recovery request comes from
// identity.AcknowledgeRecovery, whose constructor takes no arguments, so
// nothing about the address can reach it even by accident (FR-073).
type Acknowledgement struct {
	Status string `json:"status"`
}

// VerificationSent is the status a confirmation request answers with. It is not
// "sent_if_registered": the caller holds the account, so there is nobody to
// hide from and nothing to hedge about.
const VerificationSent = "sent"
