package pb_test

import (
	"bytes"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/platform/pb"
)

// safeSuperusers is the configuration the warning is trying to get an operator
// to reach: an address allowlist, MFA on for everybody rather than for some,
// and enough auth methods that PocketBase will let MFA be on at all.
func safeSuperusers() *core.Collection {
	c := core.NewAuthCollection(core.CollectionNameSuperusers)
	c.PasswordAuth.Enabled = true
	c.OTP.Enabled = true
	c.MFA.Enabled = true
	c.MFA.Rule = ""

	return c
}

func safeSettings() *core.Settings {
	s := &core.Settings{}
	s.SuperuserIPs = []string{"10.0.0.0/8"}

	return s
}

func warningCodes(warnings []pb.AdminWarning) []string {
	codes := make([]string, 0, len(warnings))
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}

	return codes
}

// A superuser bypasses every API rule by design, so the admin credential is the
// one place where the lockdown does not apply. FR-040 requires the instance to
// say so, loudly, at every start, until it is fixed.
//
// Each condition is exercised on its own off a baseline that produces no
// warning, because a warning that fires for three reasons at once tells an
// operator nothing about which one they just fixed.
func TestEachWeakAdminAccessConditionWarnsIndependently(t *testing.T) {
	t.Parallel()

	t.Run("a properly configured instance is silent", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, pb.AdminWarnings(safeSettings(), safeSuperusers()))
	})

	t.Run("the address allowlist is empty", func(t *testing.T) {
		t.Parallel()

		// SuperuserIPs is on global settings (core/settings_model.go:119),
		// not on the collection — the two halves of this warning live in two
		// different places, which is exactly what a naive check gets wrong.
		settings := &core.Settings{}

		assert.Equal(t, []string{pb.WarnSuperuserIPsUnset}, warningCodes(pb.AdminWarnings(settings, safeSuperusers())))
	})

	t.Run("second-factor authentication is off", func(t *testing.T) {
		t.Parallel()

		superusers := safeSuperusers()
		superusers.MFA.Enabled = false

		assert.Equal(t, []string{pb.WarnSuperuserMFADisabled}, warningCodes(pb.AdminWarnings(safeSettings(), superusers)))
	})

	t.Run("second-factor authentication is on for only some superusers", func(t *testing.T) {
		t.Parallel()

		// A non-empty MFA.Rule is a partial rollout that reads as "on" in the
		// admin UI while some superuser can still sign in with one factor.
		// That is the situation the warning exists to prevent, so it warns.
		superusers := safeSuperusers()
		superusers.MFA.Rule = "created > '2026-01-01'"

		assert.Equal(t, []string{pb.WarnSuperuserMFAPartial}, warningCodes(pb.AdminWarnings(safeSettings(), superusers)))
	})

	t.Run("fewer than two auth methods are enabled", func(t *testing.T) {
		t.Parallel()

		// PocketBase refuses to enable MFA on a collection with fewer than two
		// auth methods (validation_mfa_not_enough_auths), so "turn on MFA" is
		// not a single toggle and the warning has to say what actually blocks
		// it.
		superusers := safeSuperusers()
		superusers.OTP.Enabled = false

		assert.Equal(t, []string{pb.WarnSuperuserAuthMethods}, warningCodes(pb.AdminWarnings(safeSettings(), superusers)))
	})

	t.Run("MFA counts three methods and not the second factor itself", func(t *testing.T) {
		t.Parallel()

		// MFA is a second factor, not an auth method: apis/record_auth_methods.go
		// reports it in its own section. Counting it would let an instance
		// satisfy "two methods" with one method and a toggle.
		superusers := safeSuperusers()
		superusers.OTP.Enabled = false
		superusers.OAuth2.Enabled = true

		assert.Empty(t, pb.AdminWarnings(safeSettings(), superusers))
	})
}

// An untouched instance is not a safe one, and that is the point: three of the
// four conditions hold out of the box, so the very first boot warns.
func TestAnUntouchedInstanceWarns(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	superusers, err := app.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers)
	require.NoError(t, err)

	codes := warningCodes(pb.AdminWarnings(app.Settings(), superusers))

	assert.Contains(t, codes, pb.WarnSuperuserIPsUnset)
	assert.Contains(t, codes, pb.WarnSuperuserMFADisabled)
	assert.Contains(t, codes, pb.WarnSuperuserAuthMethods)
}

// "Warns loudly and unmistakably … and continues to warn on every restart"
// (US3-6). One line per condition, at warn level, each carrying its code so an
// operator can grep for the one they are fixing.
func TestTheWarningIsWrittenToTheOneLogStream(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := zerolog.New(&buf)

	warnings := pb.AdminWarnings(&core.Settings{}, core.NewAuthCollection(core.CollectionNameSuperusers))
	require.NotEmpty(t, warnings)

	pb.LogAdminWarnings(log, warnings)

	written := buf.String()
	for _, w := range warnings {
		assert.Contains(t, written, w.Code)
		assert.Contains(t, written, `"level":"warn"`)
	}
}

// Nothing to say means nothing written: a warning block that appears on every
// boot regardless is one an operator learns to skip.
func TestNothingIsWrittenWhenThereIsNothingToWarnAbout(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	pb.LogAdminWarnings(zerolog.New(&buf), pb.AdminWarnings(safeSettings(), safeSuperusers()))

	assert.Empty(t, buf.String())
}
