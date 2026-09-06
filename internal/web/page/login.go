package page

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/i18n"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
)

// The query the sign-in page is reached with when a session ran out.
//
// FR-008 requires the person to be told WHY they are being asked to sign in
// again, and nothing in the request layer can tell an expired token from a
// caller who never had one — PocketBase's loadAuthToken swallows both into an
// anonymous request. So the reason travels as a parameter on the redirect, and
// this is the one value the page recognises. Anything else is an ordinary
// visit: a stale link is not a mistake worth a 400.
const (
	ParamReason   = "reason"
	ReasonExpired = "expired"
)

// login renders contracts/pages.md's P1.
//
// The page is public and stays public for a signed-in caller: signing in as
// somebody else from a browser that already holds a session is a thing people
// do, and a redirect away from here would make it impossible without first
// finding the sign-out control on a page they cannot reach.
func (p *accountPages) login(e *core.RequestEvent, _ access.Actor) error {
	ctx := localizeCtx(e)
	title := i18n.T(ctx, "action.sign_in")

	return p.render(e, title, false, p.links.signedOutNav(ctx, p.links.loginPage), auth.Login(auth.LoginProps{
		FormID:       ids.SignInForm,
		OnSubmit:     p.links.post(p.links.login),
		RegisterHref: p.links.registerPage,
		ForgotHref:   p.links.forgotPage,

		// FR-008's explanation, and only when it happened: a page that claimed
		// a session had expired on every first visit would be telling everybody
		// about a session they never had.
		SessionExpired: queryIs(e, ParamReason, ReasonExpired),
	}))
}
