package page

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/httproute"
	facilitysvc "medikube/internal/service/facility"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/shell"
)

// The operation ids of contracts/pages.md's P5 and P6.
const (
	OpFacilityListPage   = "facilityListPage"
	OpFacilityDetailPage = "facilityDetailPage"
)

const facilityListTitle = "Places of care"

// FacilityHandlers is P5 and P6's contribution to the route table.
func FacilityHandlers(resolve api.FacilityResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, api.ErrNoFacilities
	}

	links, err := newFacilityLinks()
	if err != nil {
		return nil, err
	}

	pages := &facilityPages{resolve: resolve, links: links}

	return httproute.Handlers{
		OpFacilityListPage:   web.WithActor(pages.list),
		OpFacilityDetailPage: web.WithActor(pages.detail),
	}, nil
}

type facilityPages struct {
	resolve api.FacilityResolve
	links   facilityLinks
}

func (p *facilityPages) list(e *core.RequestEvent, actor access.Actor) error {
	if !actor.Authenticated() {
		return fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	service, err := p.resolve()
	if err != nil {
		return err
	}

	page, err := service.List(e.Request.Context(), actor, facilitysvc.Query{
		Limit: web.DefaultLimit,
	})
	if err != nil {
		return web.OwnerScoped(err)
	}

	views := make([]directory.FacilityView, 0, len(page.Items))

	for _, item := range page.Items {
		views = append(views, directory.NewFacilityView(item, p.links.of(item.ID)))
	}

	blank := directory.FacilityView{}

	main := sequence{
		directory.FacilityList(directory.FacilityListProps{
			Facilities: views,
			CreateHref: p.links.listPage + "#" + ids.DirectoryForm(directory.FacilitySegment, ""),
		}),
		directory.FacilityForm(directory.FacilityFormProps{
			FormID:     ids.DirectoryForm(directory.FacilitySegment, ""),
			New:        true,
			OnSubmit:   p.links.submitExpression(blank),
			CancelHref: p.links.cancelHref(blank),
			Facility:   blank,
			Errors:     directory.NewFieldErrors(nil),
		}),
	}

	return RenderPage(e, http.StatusOK, facilityListTitle,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path)}, main)
}

func (p *facilityPages) detail(e *core.RequestEvent, actor access.Actor) error {
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

	view := directory.NewFacilityView(found, p.links.of(found.ID))
	view.UsagePractitioners = usage.Practitioners
	view.UsageRecords = usage.Records

	main := sequence{
		directory.FacilityDetail(directory.FacilityDetailProps{Facility: view}),
		directory.FacilityForm(directory.FacilityFormProps{
			FormID:     ids.DirectoryForm(directory.FacilitySegment, view.ID),
			New:        false,
			OnSubmit:   p.links.submitExpression(view),
			CancelHref: p.links.cancelHref(view),
			Facility:   view,
			Errors:     directory.NewFieldErrors(nil),
		}),
	}

	return RenderPage(e, http.StatusOK, view.Name,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path)}, main)
}

type facilityLinks struct {
	listPage         string
	detailPage       string
	practitionersURL string
	medicationsURL   string
	settingsPage     string
	record           string
	collection       string
}

func newFacilityLinks() (facilityLinks, error) {
	paths, err := routePaths(map[string]string{
		OpFacilityListPage:     "",
		OpFacilityDetailPage:   "",
		OpPractitionerListPage: "",
		OpMedicationListPage:   "",
		OpSettingsPage:         "",
		api.OpGetFacility:      "",
		api.OpCreateFacility:   "",
	})
	if err != nil {
		return facilityLinks{}, err
	}

	return facilityLinks{
		listPage:         paths[OpFacilityListPage],
		detailPage:       paths[OpFacilityDetailPage],
		practitionersURL: paths[OpPractitionerListPage],
		medicationsURL:   paths[OpMedicationListPage],
		settingsPage:     paths[OpSettingsPage],
		record:           paths[api.OpGetFacility],
		collection:       paths[api.OpCreateFacility],
	}, nil
}

func (l facilityLinks) of(id string) directory.FacilityLinks {
	if id == "" {
		return directory.FacilityLinks{}
	}

	return directory.FacilityLinks{
		Detail: strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", id),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", id),
	}
}

// submitExpression and cancelHref mirror medicationLinks' own (medications.go):
// a create posts to the collection, a change patches the record carrying
// $_etag as If-Match, and cancelling returns to the detail page it came from
// or the list when there is none yet.
func (l facilityLinks) submitExpression(view directory.FacilityView) string {
	if view.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(view.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l facilityLinks) cancelHref(view directory.FacilityView) string {
	if view.Links.Detail != "" {
		return view.Links.Detail
	}

	return l.listPage
}

func (l facilityLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsURL, Current: strings.HasPrefix(current, l.medicationsURL)},
		{Label: facilityListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: practitionerListTitle, Href: l.practitionersURL, Current: strings.HasPrefix(current, l.practitionersURL)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
