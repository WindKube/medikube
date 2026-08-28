package pb_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/platform/pb"
)

// Settings PocketBase persists are written from validated environment at boot
// and never hand-edited in the admin UI (ANALYSIS M4). So the assertion is not
// only "the values are right in memory" but "they are right after the instance
// reads them back", which is what a restart does.
func TestSettingsAfterBootMatchTheMediKubeConfiguration(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	cfg := testConfig(t, app.DataDir())

	superusersBefore, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	require.NoError(t, err)
	superuserTTLBefore := superusersBefore.AuthToken.Duration

	require.NoError(t, pb.ApplySettings(app, cfg))

	// Read them back rather than trusting the in-memory struct: tests.NewTestApp
	// itself writes Logs.MaxDays = 0 in memory (tests/app.go:131), so an
	// assertion that never reloads can pass against a value nothing persisted.
	require.NoError(t, app.ReloadSettings())

	settings := app.Settings()

	t.Run("the batch endpoint stays disabled", func(t *testing.T) {
		// A regression guard, not a change: false is already PocketBase's
		// default. /api/batch calls the record-CRUD handler bodies directly
		// (apis/batch.go:38-88), bypassing the router and therefore the
		// lockdown middleware, so this is the only thing that closes that door
		// from the inside.
		assert.False(t, settings.Batch.Enabled)
	})

	t.Run("the log pipeline stays alive but writes no row and no address", func(t *testing.T) {
		// One, never zero: BeforeAddFunc returns MaxDays > 0, so at zero the
		// record never enters the batch and the _logs interception that turns
		// PocketBase's own failures into zerolog lines never fires (D-29).
		assert.Equal(t, 1, settings.Logs.MaxDays)
		assert.False(t, settings.Logs.LogIP, "an IP address is personal data nothing asked to keep (FR-038)")
		assert.False(t, settings.Logs.LogAuthId)
	})

	t.Run("the rate limiter is on with MediKube's rules", func(t *testing.T) {
		// PocketBase ships RateLimits.Enabled false, which is not what a
		// self-hosted medical instance reachable from the internet wants
		// (FR-006, research D-18).
		assert.True(t, settings.RateLimits.Enabled)
		assert.Equal(t, pb.RateLimitRules(), settings.RateLimits.Rules)

		labels := make([]string, 0, len(settings.RateLimits.Rules))
		for _, rule := range settings.RateLimits.Rules {
			labels = append(labels, rule.Label)
		}

		assert.Contains(t, labels, "POST /api/v1/auth/login")
		assert.Contains(t, labels, "POST /api/v1/auth/register")
	})

	t.Run("the session lifetime is the configured one", func(t *testing.T) {
		// Token TTLs are per auth collection in v0.40.1
		// (core/collection_model_auth_options.go:139-143); there is no token
		// section on core.Settings at all.
		users, err := app.FindCollectionByNameOrId("users")
		require.NoError(t, err)

		assert.EqualValues(t, cfg.Auth.SessionTTL.Seconds(), users.AuthToken.Duration)
	})

	t.Run("the superuser session lifetime is left to PocketBase", func(t *testing.T) {
		// Deliberately not MEDIKUBE_AUTH_SESSION_TTL: a week-long admin session
		// is the opposite of what Principle VII wants for the one credential
		// that bypasses every API rule.
		superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		require.NoError(t, err)

		assert.Equal(t, superuserTTLBefore, superusers.AuthToken.Duration)
		assert.NotEqualValues(t, cfg.Auth.SessionTTL.Seconds(), superusers.AuthToken.Duration)
	})
}

// Applying twice must be a no-op, because the composition root applies settings
// on every boot and a restart is not a configuration change.
func TestApplySettingsIsIdempotent(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	cfg := testConfig(t, app.DataDir())

	require.NoError(t, pb.ApplySettings(app, cfg))

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	firstSecret := users.AuthToken.Secret

	require.NoError(t, pb.ApplySettings(app, cfg))
	require.NoError(t, app.ReloadSettings())

	users, err = app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	assert.Equal(t, 1, app.Settings().Logs.MaxDays)
	assert.EqualValues(t, cfg.Auth.SessionTTL.Seconds(), users.AuthToken.Duration)
	// The auth token secret rotates whenever AuthRule *changes value*
	// (core/collection_model.go:862-866), and every outstanding session dies
	// with it. Re-applying settings must not touch AuthRule.
	assert.Equal(t, firstSecret, users.AuthToken.Secret, "re-applying settings must not log everybody out")
}

// The rules have to satisfy PocketBase's own validator, and its uniqueness
// check rejects any label that is a prefix of another (core/settings_model.go's
// checkUniqueRuleLabel) — which is exactly the shape an /api/v1/auth/... family
// tends to grow into.
func TestRateLimitRulesAreAcceptedByPocketBase(t *testing.T) {
	t.Parallel()

	limits := core.RateLimitsConfig{Enabled: true, Rules: pb.RateLimitRules()}

	require.NotEmpty(t, limits.Rules)
	assert.NoError(t, limits.Validate())
}
