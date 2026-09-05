package page

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/httproute"
	practitionersvc "medikube/internal/service/practitioner"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/shell"
)

// The operation ids of contracts/pages.md's P3 and P4.
const (
	OpPractitionerListPage   = "practitionerListPage"
	OpPractitionerDetailPage = "practitionerDetailPage"
)

const practitionerListTitle = "Practitioners"

// PractitionerHandlers is P3 and P4's contribution to the route table.
func PractitionerHandlers(resolve api.PractitionerResolve, facilities api.FacilityResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, api.ErrNoPractitioners
	}

	links, err := newPractitionerLinks()
	if err != nil {
		return nil, err
	}

	pages := &practitionerPages{resolve: resolve, facilities: facilities, links: links}

	return httproute.Handlers{
		OpPractitionerListPage:   web.WithActor(pages.list),
		OpPractitionerDetailPage: web.WithActor(pages.detail),
	}, nil
}

type practitionerPages struct {
	resolve    api.PractitionerResolve
	facilities api.FacilityResolve
	links      practitionerLinks
}

func (p *practitionerPages) list(e *core.RequestEvent, actor access.Actor) error {
	if !actor.Authenticated() {
		return fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	service, err := p.resolve()
	if err != nil {
		return err
	}

	page, err := service.List(e.Request.Context(), actor, practitionersvc.Query{
		Sort:  practitionersvc.Sorts()[:1],
		Limit: web.DefaultLimit,
	})
	if err != nil {
		return web.OwnerScoped(err)
	}

	views := make([]directory.PractitionerView, 0, len(page.Items))

	for _, item := range page.Items {
		views = append(views, directory.NewPractitionerView(item, p.facilityName(e, actor, item.FacilityID), p.links.of(item.ID)))
	}

	return RenderPage(e, http.StatusOK, practitionerListTitle,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path)},
		directory.PractitionerList(directory.PractitionerListProps{
			Practitioners: views,
			CreateHref:    p.links.listPage + "#" + ids.DirectoryForm(directory.PractitionerSegment, ""),
		}))
}

func (p *practitionerPages) detail(e *core.RequestEvent, actor access.Actor) error {
	if !actor.Authenticated() {
		return fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	service, err := p.resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(api.PathID)

	found, err := service.Get(e.Request.Context(), actor, id)
	if err != nil {
		return web.OwnerScoped(err)
	}

	usage, err := service.Usage(e.Request.Context(), actor, id)
	if err != nil {
		return web.OwnerScoped(err)
	}

	view := directory.NewPractitionerView(found, p.facilityName(e, actor, found.FacilityID), p.links.of(found.ID))
	view.UsagePatients = usage.Patients
	view.UsageRecords = usage.Records

	return RenderPage(e, http.StatusOK, view.Name,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path)},
		directory.PractitionerDetail(directory.PractitionerDetailProps{Practitioner: view}))
}

func (p *practitionerPages) facilityName(e *core.RequestEvent, actor access.Actor, facilityID string) string {
	if facilityID == "" || p.facilities == nil {
		return ""
	}

	facilities, err := p.facilities()
	if err != nil {
		return ""
	}

	found, err := facilities.Get(e.Request.Context(), actor, facilityID)
	if err != nil {
		return ""
	}

	return found.Name
}

type practitionerLinks struct {
	listPage       string
	detailPage     string
	facilitiesURL  string
	medicationsURL string
	settingsPage   string
	record         string
	collection     string
}

func newPractitionerLinks() (practitionerLinks, error) {
	paths, err := routePaths(map[string]string{
		OpPractitionerListPage:   "",
		OpPractitionerDetailPage: "",
		OpFacilityListPage:       "",
		OpMedicationListPage:     "",
		OpSettingsPage:           "",
		api.OpGetPractitioner:    "",
		api.OpCreatePractitioner: "",
	})
	if err != nil {
		return practitionerLinks{}, err
	}

	return practitionerLinks{
		listPage:       paths[OpPractitionerListPage],
		detailPage:     paths[OpPractitionerDetailPage],
		facilitiesURL:  paths[OpFacilityListPage],
		medicationsURL: paths[OpMedicationListPage],
		settingsPage:   paths[OpSettingsPage],
		record:         paths[api.OpGetPractitioner],
		collection:     paths[api.OpCreatePractitioner],
	}, nil
}

func (l practitionerLinks) of(id string) directory.PractitionerLinks {
	if id == "" {
		return directory.PractitionerLinks{}
	}

	return directory.PractitionerLinks{
		Detail: strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", id),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", id),
	}
}

func (l practitionerLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsURL, Current: strings.HasPrefix(current, l.medicationsURL)},
		{Label: practitionerListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: facilityListTitle, Href: l.facilitiesURL, Current: strings.HasPrefix(current, l.facilitiesURL)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
