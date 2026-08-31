package page

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/service/identity"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
)

// verifyEmail renders contracts/pages.md's P9, and confirms nothing by being
// opened.
//
// The confirmation is a control the person presses. Mail clients and link
// scanners follow the links in a message before anybody reads it, so a page
// that confirmed on GET would routinely spend the token on the person's behalf
// — and a GET that changes state is wrong in every other way as well.
//
// AN ADDRESS THAT IS ALREADY CONFIRMED IS A SPENT LINK, and the page says so.
// PocketBase does not invalidate a verification token when it is redeemed, so
// the link stays resolvable for its full twenty-four hours; the service refuses
// the second use itself, and this page renders the same state rather than
// offering a control that would be refused. It is deliberately the same state
// as an expired link, because telling the two apart tells somebody which links
// once existed.
func (p *accountPages) verifyEmail(e *core.RequestEvent, _ access.Actor) error {
	user, resolved, err := p.resolveLink(e, identity.TokenEmailConfirmation)
	if err != nil {
		return err
	}

	return p.render(e, verifyEmailTitle, false, p.links.signedOutNav(e.Request.URL.Path),
		auth.VerifyEmail(auth.VerifyEmailProps{
			FormID:    ids.ConfirmAddressForm,
			OnConfirm: p.links.post(p.links.confirmEmail),
			LoginHref: p.links.loginPage,
			Token:     e.Request.PathValue(PathToken),
			Usable:    resolved && !user.EmailConfirmed,
			Mailable:  p.deps.Mail(),
		}))
}
