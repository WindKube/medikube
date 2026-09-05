package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/platform/pb"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/apitest"
)

// The account surface's own harness. It sits beside harness_test.go's `caller`
// rather than inside it: the record family needs an instance and a token, and
// the account surface needs the INSTANCE ITSELF — the identity service to drive
// directly, the authenticator's comparison counter to read, the mail sink to
// look in, and the operator switches to turn.

// rig is one wired instance plus the handler it serves, so several callers can
// be made onto the same stored data.
type rig struct {
	t        *testing.T
	instance *apitest.Instance
	handler  http.Handler
}

func newRig(t *testing.T, options ...apitest.Option) *rig {
	t.Helper()

	instance := apitest.New(t, options...)

	return &rig{t: t, instance: instance, handler: testsupport.NewEdgeHandler(t, instance.App)}
}

// openRig is an instance whose operator has opened self-registration (FR-002).
func openRig(t *testing.T) *rig {
	t.Helper()

	return newRig(t, apitest.WithRegistrationOpen(true))
}

// as is a caller signed in as one seeded account, by bearer token.
func (r *rig) as(email string) *caller {
	r.t.Helper()

	return &caller{
		t:       r.t,
		app:     r.instance.App,
		handler: r.handler,
		token:   testsupport.UserToken(r.t, r.instance.App, email),
	}
}

// anonymous carries no credential at all.
func (r *rig) anonymous() *caller {
	return &caller{t: r.t, app: r.instance.App, handler: r.handler}
}

// superuser is the break-glass credential. Every account operation must refuse
// it: it is not a MediKube role, it has no row in the account collection, and
// MediKube's own routes are not a second, unaudited way into somebody's
// account (data-model §1, FR-040).
func (r *rig) superuser() *caller {
	r.t.Helper()

	return &caller{
		t:       r.t,
		app:     r.instance.App,
		handler: r.handler,
		token:   testsupport.AuthToken(r.t, r.instance.App, core.CollectionNameSuperusers, testsupport.SuperuserEmail),
	}
}

// withMail turns outgoing mail on. tests.TestApp swaps in a mailer that always
// succeeds (tests/app.go binds OnMailerSend), so this switches the SETTING the
// handlers gate on rather than the transport — which is the gate FR-076
// specifies, and the only one an instance can answer before it tries to send.
func (r *rig) withMail(enabled bool) *rig {
	r.t.Helper()

	settings := r.instance.App.Settings()
	settings.SMTP.Enabled = enabled
	require.NoError(r.t, r.instance.App.Save(settings))

	return r
}

// withRateLimits applies MediKube's own settings, which is what turns
// PocketBase's rate limiter on: the committed fixture has RateLimits.Enabled
// false, so a rate-limit assertion made without this passes VACUOUSLY.
func (r *rig) withRateLimits() *rig {
	r.t.Helper()

	require.NoError(r.t, pb.ApplySettings(r.instance.App, config.Config{
		Auth: config.AuthConfig{SessionTTL: apitest.SessionTTL},
	}))

	require.True(r.t, r.instance.App.Settings().RateLimits.Enabled,
		"the limiter is off, so every assertion below would pass without limiting anything")

	return r
}

// disable takes an account out of service the way an operator would: through
// the mapper, so no test spells a column name PocketBase might rename.
func (r *rig) disable(email string) {
	r.t.Helper()

	record, err := r.instance.App.FindAuthRecordByEmail(store.AccountCollection, email)
	require.NoError(r.t, err)

	user, err := store.UserFromRecord(record)
	require.NoError(r.t, err)

	user.DisabledAt = time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(r.t, store.UserToRecord(record, user))
	require.NoError(r.t, r.instance.App.SaveNoValidate(record))
}

// stored reads one account back out of the database, so an assertion about what
// a request changed is never made against the response that claimed to change
// it.
func (r *rig) stored(t *testing.T, email string) *core.Record {
	t.Helper()

	record, err := r.instance.App.FindAuthRecordByEmail(store.AccountCollection, email)
	require.NoError(t, err)

	return record
}

func (r *rig) accountExists(t *testing.T, email string) bool {
	t.Helper()

	_, err := r.instance.App.FindAuthRecordByEmail(store.AccountCollection, email)

	return err == nil
}

// The addresses under test, composed from the route table rather than spelled,
// so an assertion cannot outlive the path it is made against.
const (
	authConfigURL    = "/api/v1/auth/config"
	registerURL      = "/api/v1/auth/register"
	loginURL         = "/api/v1/auth/login"
	refreshURL       = "/api/v1/auth/refresh"
	logoutURL        = "/api/v1/auth/logout"
	passwordResetURL = "/api/v1/auth/password-reset"
	confirmResetURL  = "/api/v1/auth/password-reset/confirm"
	verifyEmailURL   = "/api/v1/auth/verify-email"
	confirmVerifyURL = "/api/v1/auth/verify-email/confirm"

	meURL       = "/api/v1/me"
	passwordURL = "/api/v1/me/password"
)

func (c *caller) put(url, body string) response { return c.do(http.MethodPut, url, body, nil) }

// cookieDo presents a session the way a browser does and in the only way a
// browser can: the cookie, and no Authorization header anywhere, because a
// plain navigation cannot set one. harness_test.go's byCookie does the same for
// the record list; this one takes the address as a parameter.
func (c *caller) cookieDo(method, url, requestBody, token string) response {
	return c.anonymous().do(method, url, requestBody,
		map[string]string{"Cookie": web.SessionCookieName + "=" + token})
}

func (c *caller) cookieGet(url, token string) response {
	return c.cookieDo(http.MethodGet, url, "", token)
}

func (c *caller) deleteWithBody(url, body string) response {
	return c.do(http.MethodDelete, url, body, nil)
}

// sessionCookie is the Set-Cookie the response issued, or nil.
func (r response) sessionCookie(t *testing.T) *http.Cookie {
	t.Helper()

	for _, cookie := range (&http.Response{Header: r.Header}).Cookies() {
		if cookie.Name == web.SessionCookieName {
			return cookie
		}
	}

	return nil
}

// meDTO mirrors api.Me as a client sees it. It is declared here rather than
// imported for the same reason medicationDTO is: a change to the published
// shape has to be made twice, which is what makes it a contract instead of
// whatever the struct currently is.
type meDTO struct {
	ID             string         `json:"id"`
	Email          string         `json:"email"`
	EmailConfirmed bool           `json:"email_confirmed"`
	Name           string         `json:"name"`
	Role           string         `json:"role"`
	UnitSystem     string         `json:"unit_system"`
	Locale         string         `json:"locale"`
	DateFormat     string         `json:"date_format"`
	Theme          string         `json:"theme"`
	CreatedAt      string         `json:"created_at"`
	Counts         map[string]int `json:"counts"`
	ActivePatient  *struct {
		ID string `json:"id"`
	} `json:"active_patient"`
	Patients struct {
		OwnedCount int `json:"owned_count"`
	} `json:"patients"`
}

type sessionDTO struct {
	User      meDTO  `json:"user"`
	ExpiresAt string `json:"expires_at"`
}

type authConfigDTO struct {
	RegistrationOpen bool `json:"registration_open"`
	PasswordRules    struct {
		MinLength    int  `json:"min_length"`
		MaxLength    int  `json:"max_length"`
		RejectsEmail bool `json:"rejects_email"`
		RejectsName  bool `json:"rejects_name"`
	} `json:"password_rules"`
}

type acknowledgementDTO struct {
	Status string `json:"status"`
}

func (r response) session(t *testing.T) sessionDTO {
	t.Helper()

	var session sessionDTO
	r.decode(t, &session)

	return session
}

func (r response) me(t *testing.T) meDTO {
	t.Helper()

	var me meDTO
	r.decode(t, &me)

	return me
}
