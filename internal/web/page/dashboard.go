package page

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/i18n"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/overview"
)

// OpOverviewPage is contracts/pages.md P3's operation id.
const OpOverviewPage = "overviewPage"

var ErrNoOverviewPage = errors.New("page: the overview page was wired without a record counter")

// OverviewDeps is the same counter contracts/account.md's getMe answers with.
type OverviewDeps struct {
	Counts api.Counter
}

// OverviewPage is the route table's contribution for P3.
func OverviewPage(deps OverviewDeps) (httproute.Handlers, error) {
	if deps.Counts == nil {
		return nil, ErrNoOverviewPage
	}

	links, err := newMedicationLinks()
	if err != nil {
		return nil, err
	}

	account, err := newAccountLinks()
	if err != nil {
		return nil, err
	}

	page := &overviewPage{deps: deps, medications: links, account: account}

	return httproute.Handlers{
		OpOverviewPage: web.WithActor(page.serve),
	}, nil
}

type overviewPage struct {
	deps        OverviewDeps
	medications medicationLinks
	account     accountLinks
}

// serve renders P3. It is a session page like the record pages: a person
// with no session reaching "/" gets the 403 sign-in prompt, not a preview of
// what an account can hold.
func (p *overviewPage) serve(e *core.RequestEvent, actor access.Actor) error {
	if err := requireSession(actor); err != nil {
		return err
	}

	counts, err := p.deps.Counts(e.Request.Context(), actor)
	if err != nil {
		return err
	}

	ctx := localizeCtx(e)

	// "" as the current path matches none of the nav's own hrefs (all of
	// which are absolute paths), so nothing is marked aria-current: the
	// overview page has no entry of its own in the primary navigation.
	nav := p.account.signedInNav(ctx, "")

	main := overview.Overview(overview.Props{
		MedicationCount:  counts[kind.Medication.Segment()],
		MedicationsLabel: kind.Medication.Segment(),
		MedicationsHref:  p.medications.listPage,
		SettingsHref:     p.account.settingsPage,
	})

	return RenderPage(e, http.StatusOK, i18n.T(ctx, "nav.overview"), NavState{SignedIn: true, Nav: nav}, main)
}
