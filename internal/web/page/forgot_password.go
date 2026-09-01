package page

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
)

// forgotPassword renders contracts/pages.md's P7.
//
// Public, and public for the same reason /login is: the only person who needs
// it is one who cannot sign in. It is rendered for a signed-in caller too — a
// person who has just remembered they cannot remember their password is not
// helped by a redirect.
//
// The page says nothing about any address, and there is nothing here that
// could: FR-073's acknowledgement is built by a constructor that takes no
// arguments, so what this form submits to answers identically whether or not
// an account exists, and the words above the control say so.
//
// Mailable is read per request rather than at wiring time. An operator can turn
// outgoing mail on in the admin UI without restarting MediKube, and a page that
// had cached the answer at boot would keep refusing on an instance that had
// started working (FR-076).
func (p *accountPages) forgotPassword(e *core.RequestEvent, _ access.Actor) error {
	return p.render(e, forgotPasswordTitle, false, p.links.signedOutNav(e.Request.URL.Path),
		auth.ForgotPassword(auth.ForgotPasswordProps{
			FormID:    ids.ForgotPasswordForm,
			OnSubmit:  p.links.post(p.links.recover),
			LoginHref: p.links.loginPage,
			Mailable:  p.deps.Mail(),
		}))
}
