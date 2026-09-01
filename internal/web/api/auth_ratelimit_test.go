package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/platform/pb"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

// T201, FR-006. Repeated failed sign-ins are blocked.
//
// TWO TRAPS THIS FILE IS BUILT AROUND.
//
// The first: the committed fixture has RateLimits.Enabled FALSE, and
// testsupport.NewApp does not apply MediKube's settings. A rate-limit test that
// forgot to apply them would drive a hundred requests through an unlimited
// route and pass — the exact "test that cannot fail" shape. Every case below
// therefore asserts the limiter is on as a PRECONDITION, with require, before
// it asserts anything about limiting.
//
// The second: MediKube's lockdown middleware sits at priority -1009 and
// PocketBase's limiter at -1000, so anything the lockdown answers 404 to
// short-circuits BEFORE any counter increments. A rule written against a route
// the lockdown swallows would never fire. /api/v1/auth/login is not one of
// those, and the burst below reaching a 429 at all is the proof.

// theBurst is comfortably past the ten attempts a minute the login rule allows,
// and short enough that the whole file stays fast.
const theBurst = 14

func TestRepeatedFailedSignInsAreBlocked(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withRateLimits()

	refused, limited := 0, 0

	var last response

	for range theBurst {
		answer := instance.anonymous().post(loginURL, body(
			"email", quoted(testsupport.AccountAEmail),
			"password", quoted("not-the-password"),
		))

		switch answer.Status {
		case http.StatusUnauthorized:
			refused++
		case http.StatusTooManyRequests:
			limited++
			last = answer
		default:
			t.Fatalf("a sign-in attempt answered %d: %s", answer.Status, answer.Body)
		}
	}

	assert.Positive(t, limited, "%d attempts in one burst and none was limited", theBurst)
	assert.Positive(t, refused, "the very first attempt was limited, so nothing was ever tried")

	// The limited answer is the SAME SHAPE as a wrong password: MediKube's
	// envelope, with a code a client can branch on. PocketBase's own limiter
	// answers {"data":{},"message":"Too Many Requests.","status":429}, which no
	// client of this API is written against.
	require.NotZero(t, last.Status)
	assert.Equal(t, web.CodeRateLimited, last.envelope(t).Error.Code)
	assert.Equal(t, web.Message(web.CodeRateLimited), last.envelope(t).Error.Message)
	assert.NotEmpty(t, last.envelope(t).Error.RequestID)
	assert.NotContains(t, last.Body, testsupport.AccountAEmail)
}

// A burst of correct passwords is limited too, and that is worth stating rather
// than discovering: PocketBase's limiter counts REQUESTS and has no
// failure-only counter, so ten successful sign-ins a minute from one office
// address are throttled the same way ten failures are. FR-006 asks for failures
// to be slowed; this is the cost of the mechanism that does it.
func TestTheLimiterCountsAttemptsRatherThanFailures(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withRateLimits()

	limited := 0

	for range theBurst {
		if instance.anonymous().post(loginURL, body(
			"email", quoted(testsupport.AccountAEmail),
			"password", quoted(testsupport.Password),
		)).Status == http.StatusTooManyRequests {
			limited++
		}
	}

	assert.Positive(t, limited,
		"successful sign-ins are not counted, so a burst of correct guesses walks past the limit")
}

// The limiter is not on by accident and not off by default. Without
// ApplySettings the fixture leaves it disabled, and every assertion above would
// pass against an unlimited instance.
func TestWithoutMediKubesSettingsThereIsNoLimiterAtAll(t *testing.T) {
	t.Parallel()

	unapplied := newRig(t)
	require.False(t, unapplied.instance.App.Settings().RateLimits.Enabled,
		"the fixture now enables the limiter, so withRateLimits is no longer what turns it on")

	for range theBurst {
		answer := unapplied.anonymous().post(loginURL, body(
			"email", quoted(testsupport.AccountAEmail),
			"password", quoted("not-the-password"),
		))

		require.Equal(t, http.StatusUnauthorized, answer.Status,
			"an instance with no settings applied limited something, so the tests above prove nothing")
	}
}

// The rules name routes MediKube actually serves.
//
// PocketBase matches a rule by the literal string `METHOD /path`, so a rule
// whose label drifted from the route it guards is a rule that silently guards
// nothing — no error, no log line, and a passing build. There is no way to see
// that from either side alone, which is why it is asserted from here.
func TestEveryRateLimitRuleNamesARouteThisInstanceServes(t *testing.T) {
	t.Parallel()

	served := map[string]struct{}{}
	for _, route := range httproute.Inventory().Routes() {
		served[route.Method+" "+route.Path] = struct{}{}
	}

	labelled := 0

	for _, rule := range pb.RateLimitRules() {
		if !strings.Contains(rule.Label, " ") {
			// A prefix rule such as `/api/`, which is the floor under
			// everything and names no single route.
			continue
		}

		labelled++

		_, exists := served[rule.Label]
		assert.Truef(t, exists,
			"the rate limit rule %q guards no route this instance serves, so it guards nothing", rule.Label)
	}

	assert.Equal(t, 3, labelled,
		"FR-006's three guarded routes are sign-in, registration and recovery")
}

// The three guarded routes are guarded for guests, which is a deliberate
// decision with a visible cost: an attacker holding ANY valid token falls
// through to the `/api/` floor of 300 requests per 10 seconds. Stating it here
// is what makes it a decision rather than an oversight somebody discovers.
func TestTheGuardedRoutesAreGuardedForCallersWithNoSession(t *testing.T) {
	t.Parallel()

	for _, rule := range pb.RateLimitRules() {
		if !strings.Contains(rule.Label, " ") {
			assert.Empty(t, rule.Audience, "the floor under everything applies to everybody")

			continue
		}

		assert.Equalf(t, core.RateLimitRuleAudienceGuest, rule.Audience,
			"%s is limited for somebody other than a guest", rule.Label)
		assert.Positivef(t, rule.MaxRequests, "%s allows no requests at all", rule.Label)
		assert.Positivef(t, rule.Duration, "%s has no window, so its allowance never resets", rule.Label)
	}
}

// The recovery request is limited too: it sends mail to an address the caller
// supplies, so an unlimited one is a mail relay pointed at strangers
// (contracts/auth.md, T223c's last row).
func TestRepeatedRecoveryRequestsAreLimited(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true).withRateLimits()

	accepted, limited := 0, 0

	for range theBurst {
		answer := instance.anonymous().post(passwordResetURL, body("email", quoted(testsupport.AccountAEmail)))

		switch answer.Status {
		case http.StatusAccepted:
			accepted++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("a recovery request answered %d: %s", answer.Status, answer.Body)
		}
	}

	assert.Positive(t, accepted)
	assert.Positive(t, limited, "%d recovery requests in one burst and none was limited", theBurst)
}
