package pb

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"
)

// The four conditions, as codes an operator can grep for and a runbook can
// name. They are codes rather than sentences because the sentence is going to
// be rewritten and the grep is not.
const (
	WarnSuperuserIPsUnset    = "superuser_ips_unset"
	WarnSuperuserMFADisabled = "superuser_mfa_disabled"
	WarnSuperuserMFAPartial  = "superuser_mfa_partial"
	WarnSuperuserAuthMethods = "superuser_auth_methods"
)

// AdminWarning is one unmet condition on the administrative credential.
type AdminWarning struct {
	Code   string
	Detail string
	// Fix is what the operator actually has to go and do. A warning that
	// reports a state without naming the action is a warning people learn to
	// scroll past.
	Fix string
}

// AdminWarnings reports every way the administrative credential is weaker than
// Principle VII requires (FR-040).
//
// It takes the two inputs directly because they live in two different places
// and a check that assumed they sat together would get one of them wrong: the
// address allowlist is on global settings (core/settings_model.go:119), while
// MFA is on the superusers auth collection
// (core/collection_model_auth_options.go:348).
//
// This warns rather than refuses. A superuser has to exist before MFA can be
// enabled on the collection, so refusing to start would make a first run
// impossible; the compensating control is the audit entry FR-040 requires for
// every administrative session.
func AdminWarnings(settings *core.Settings, superusers *core.Collection) []AdminWarning {
	var warnings []AdminWarning

	if len(settings.SuperuserIPs) == 0 {
		warnings = append(warnings, AdminWarning{
			Code:   WarnSuperuserIPsUnset,
			Detail: "the superuser address allowlist is empty, so the admin UI accepts a sign-in from anywhere on the internet",
			Fix:    "set the allowlist to the addresses or CIDR ranges administration is done from",
		})
	}

	if superusers == nil {
		return warnings
	}

	switch {
	case !superusers.MFA.Enabled:
		warnings = append(warnings, AdminWarning{
			Code:   WarnSuperuserMFADisabled,
			Detail: "second-factor authentication is off for the credential that bypasses every API rule",
			Fix:    "enable at least two auth methods on the superusers collection, then enable MFA",
		})
	case superusers.MFA.Rule != "":
		// A rule is a partial rollout. It reads as "MFA is on" in the admin UI
		// while some superuser can still sign in with one factor, which is
		// exactly the situation this warning exists to prevent.
		warnings = append(warnings, AdminWarning{
			Code:   WarnSuperuserMFAPartial,
			Detail: "second-factor authentication applies to only some superusers, so at least one can still sign in with a password alone",
			Fix:    "clear the MFA rule so it applies to every superuser",
		})
	}

	// PocketBase refuses to enable MFA on a collection with fewer than two auth
	// methods (validation_mfa_not_enough_auths), so "turn on MFA" is not a
	// single toggle and the warning has to name what is actually blocking it.
	if enabledAuthMethods(superusers) < 2 {
		warnings = append(warnings, AdminWarning{
			Code:   WarnSuperuserAuthMethods,
			Detail: "the superusers collection has fewer than two auth methods enabled, so PocketBase will refuse to enable MFA on it",
			Fix:    "enable a second auth method — one-time codes or OAuth2 — alongside the password",
		})
	}

	return warnings
}

// enabledAuthMethods counts what apis/record_auth_methods.go:83-115 counts.
//
// MFA is not one of them: it is a second factor, not an auth method, and
// PocketBase reports it in its own section of the response. Counting it would
// let an instance satisfy "two methods" with one method and a toggle.
func enabledAuthMethods(collection *core.Collection) int {
	var count int

	if collection.PasswordAuth.Enabled {
		count++
	}

	if collection.OAuth2.Enabled {
		count++
	}

	if collection.OTP.Enabled {
		count++
	}

	return count
}

// LogAdminWarnings writes the warnings to the one log stream, one line per
// condition, and writes nothing at all when there is nothing to say — a block
// that appears on every boot regardless is one an operator learns to skip.
//
// It re-warns on every start by construction: the composition root calls it on
// every start and nothing here remembers a previous one (US3-6).
func LogAdminWarnings(log zerolog.Logger, warnings []AdminWarning) {
	for _, warning := range warnings {
		log.Warn().
			Str("warning", warning.Code).
			Str("fix", warning.Fix).
			Msg(warning.Detail)
	}
}
