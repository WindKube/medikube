package pb

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/config"
)

// usersCollection is PocketBase's own auth collection, which MediKube amends
// rather than replaces.
const usersCollection = "users"

// LogRetentionDays is one, and never zero.
//
// PocketBase's log batcher checks Logs.MaxDays > 0 before a record enters the
// batch, so at zero the _logs interception that turns PocketBase's own backup,
// mailer, cron and OAuth2 failures into zerolog lines never fires — and in a
// production build printLog does not run either, so those failures would go
// nowhere at all. One keeps the pipeline alive; the interception guarantees no
// row is ever written, so there is still exactly one log store (research D-29).
const LogRetentionDays = 1

// RateLimitRules is MediKube's rate limit configuration.
//
// PocketBase ships RateLimits.Enabled false, which is not what a self-hosted
// medical instance reachable from the internet wants (FR-006, research D-18).
//
// These are constants rather than environment variables because MEDIKUBE_ has
// no knobs for them: the values are a security posture, not a deployment
// choice. The labels are exact method+path matches on MediKube's own auth
// routes; PocketBase's matcher only treats a label as a prefix when it ends in
// a slash (core/settings_model.go's FindRateLimitRule).
//
// The stream route is not exempted here — it cannot be. PocketBase has no
// per-rule exclusion, so the exemption is an Unbind of the limiter on that
// route's group at registration time, which belongs to the route table.
func RateLimitRules() []core.RateLimitRule {
	return []core.RateLimitRule{
		// FR-006: repeated failed sign-ins are slowed. Guests only — a
		// signed-in person re-authenticating is not the attack.
		{
			Label:       "POST /api/v1/auth/login",
			Audience:    core.RateLimitRuleAudienceGuest,
			Duration:    60,
			MaxRequests: 10,
		},
		// Registration is closed by default (research D-18); when an operator
		// opens it, this is what stops it becoming an account farm.
		{
			Label:       "POST /api/v1/auth/register",
			Audience:    core.RateLimitRuleAudienceGuest,
			Duration:    3600,
			MaxRequests: 5,
		},
		// Password recovery sends mail to an address the caller supplies, so
		// an unlimited one is a mail relay pointed at strangers.
		{
			Label:       "POST /api/v1/auth/password-reset",
			Audience:    core.RateLimitRuleAudienceGuest,
			Duration:    3600,
			MaxRequests: 5,
		},
		// The floor under everything else, at PocketBase's own default rate.
		// Dropping it would leave every route but three unlimited.
		{
			Label:       "/api/",
			Duration:    10,
			MaxRequests: 300,
		},
	}
}

// ApplySettings writes the settings PocketBase persists, from MediKube's
// validated configuration.
//
// PocketBase's Settings() store is carved out of the "one configuration
// mechanism" rule by the constitution: it is part of the platform, not a second
// config system MediKube chose. What keeps that honest is that MediKube writes
// it at boot from the environment and nobody edits it in the admin UI
// (ANALYSIS M4) — so this runs on every start, and re-running it is a no-op.
func ApplySettings(app core.App, cfg config.Config) error {
	settings := app.Settings()

	// Half of the batch lockdown. /api/batch calls the record-CRUD handler
	// bodies directly rather than through the router, so the middleware cannot
	// see those sub-requests; this is what closes that door from the inside.
	settings.Batch.Enabled = false

	settings.Logs.MaxDays = LogRetentionDays
	// An IP address is personal data about the actor that no requirement asks
	// to keep, in an application whose governing principle is that privacy is
	// structural (FR-038, research D-19).
	settings.Logs.LogIP = false
	settings.Logs.LogAuthId = false

	settings.RateLimits.Enabled = cfg.RateLimits
	settings.RateLimits.Rules = RateLimitRules()

	if err := app.Save(settings); err != nil {
		return fmt.Errorf("write the PocketBase settings: %w", err)
	}

	return applySessionTTL(app, cfg.Auth.SessionTTL)
}

// applySessionTTL writes MEDIKUBE_AUTH_SESSION_TTL onto the users collection.
//
// Token lifetimes are per auth collection in v0.40.1
// (core/collection_model_auth_options.go:139-143); there is no token section on
// core.Settings at all, so this cannot be part of the settings write above.
//
// The superusers collection is deliberately left at PocketBase's own default: a
// week-long session for the one credential that bypasses every API rule is the
// opposite of what Principle VII wants.
func applySessionTTL(app core.App, ttl time.Duration) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("find the %s collection: %w", usersCollection, err)
	}

	seconds := int64(ttl.Seconds())
	if users.AuthToken.Duration == seconds {
		return nil
	}

	users.AuthToken.Duration = seconds

	// Saving the collection is safe here only because AuthRule is untouched:
	// PocketBase re-randomises the auth token secret whenever AuthRule changes
	// value (core/collection_model.go:862-866), and every outstanding session
	// dies with it. A boot that logged everybody out would be a fine way to
	// discover that on a Monday.
	if err := app.Save(users); err != nil {
		return fmt.Errorf("write the %s session lifetime: %w", usersCollection, err)
	}

	return nil
}
