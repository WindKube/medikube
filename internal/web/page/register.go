package page

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	"medikube/internal/i18n"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
)

// register renders contracts/pages.md's P2, in both of the operator's
// configurations.
//
// FR-002 and defect D15. Closed, it answers 200 and renders an explanation
// inside the ordinary application frame — it is not a 404 and it is not absent
// from the route table. A 404 is what this codebase answers for owner-scoped
// data, where the existence of the thing is itself the disclosure; whether an
// operator has opened self-registration is instance-wide configuration,
// identical for every caller and evident from the page itself. There is no
// oracle here to close, and a page that vanished under a configuration would be
// a page neither the inventory gate nor the browser gate could check.
//
// The password rules come from the domain value the check itself reads, so the
// rules this page publishes are the rules the API enforces (FR-004).
func (p *accountPages) register(e *core.RequestEvent, _ access.Actor) error {
	ctx := localizeCtx(e)
	title := i18n.T(ctx, "auth.create_account")

	return p.render(e, title, false, p.links.signedOutNav(ctx, p.links.registerPage), auth.Register(auth.RegisterProps{
		FormID:    ids.CreateAccountForm,
		OnSubmit:  p.links.post(p.links.register),
		LoginHref: p.links.loginPage,
		Open:      p.deps.Accounts.RegistrationOpen(),
		Rules:     identity.PublishedPasswordRules(),
	}))
}
