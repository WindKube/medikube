package page

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/i18n"
	"medikube/internal/service/identity"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
)

// resetPassword renders contracts/pages.md's P8, in both of the states a link
// can be in.
//
// 200 EITHER WAY, and the dead-link state inside the landmark (FR-074,
// contracts/pages.md). A 4xx here would be an error view, which is a page
// without this landmark, which is a page the browser gate cannot check — and
// the gate's own smoke URL is a deliberately dead token, because a real one
// lives thirty minutes and any committed fixture is expired before CI reaches
// it.
//
// The link is resolved before the form is rendered rather than after it is
// submitted. Redeeming spends nothing — a reset token is signed with the
// account's own key and only the password write moves that key — so the page
// can ask the question the submission would ask, and answer it while there is
// still something useful to offer.
func (p *accountPages) resetPassword(e *core.RequestEvent, _ access.Actor) error {
	_, usable, err := p.resolveLink(e, identity.TokenPasswordReset)
	if err != nil {
		return err
	}

	ctx := localizeCtx(e)
	title := i18n.T(ctx, "auth.choose_new_password")

	return p.render(e, title, false, p.links.signedOutNav(ctx, e.Request.URL.Path),
		auth.ResetPassword(auth.ResetPasswordProps{
			FormID:     ids.NewPasswordForm,
			OnSubmit:   p.links.post(p.links.setPassword),
			ForgotHref: p.links.forgotPage,
			Token:      e.Request.PathValue(PathToken),
			Usable:     usable,
			Mailable:   p.deps.Mail(),

			// The rules are published before the person chooses, from the
			// domain value the check itself reads: a password chosen through a
			// recovery link is held to exactly the rules registration states
			// (FR-004, FR-074).
			Rules: domainidentity.PublishedPasswordRules(),
		}))
}
