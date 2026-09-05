package page

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	domainperson "medikube/internal/domain/person"
	"medikube/internal/httproute"
	"medikube/internal/service/patient"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/patients"
	"medikube/internal/web/views/shell"
)

// The operation ids of contracts/pages.md P1 and P2.
const (
	OpPatientListPage   = "patientListPage"
	OpPatientDetailPage = "patientDetailPage"
)

const patientListTitle = "People"

// PatientPageOperations is contracts/pages.md P1 and P2, so cmd/medikube's
// stub inventory knows these two are wired.
func PatientPageOperations() []string {
	return []string{OpPatientListPage, OpPatientDetailPage}
}

// ErrNoPatientPages is a build whose patient pages were wired without a way
// to resolve the patient stack.
var ErrNoPatientPages = errors.New("page: the patient pages were wired without a way to resolve the patient service")

// PatientDeps is what the two patient pages need. Records resolves the kind
// registry's generic handler, used only to answer whether an activity
// entry's target still exists (api.TargetExists) — the same seam
// getPatientChart itself reaches through, so the two never disagree.
type PatientDeps struct {
	Resolve api.PatientResolve
	UnitOf  api.UnitSystemOf
	Records api.Resolve
}

// PatientPages is the route table's contribution for P1 and P2.
func PatientPages(deps PatientDeps) (httproute.Handlers, error) {
	if deps.Resolve == nil || deps.UnitOf == nil {
		return nil, ErrNoPatientPages
	}

	links, err := newPatientLinks()
	if err != nil {
		return nil, err
	}

	p := &patientPages{deps: deps, links: links}

	return httproute.Handlers{
		OpPatientListPage:   web.WithActor(p.list),
		OpPatientDetailPage: web.WithActor(p.detail),
	}, nil
}

type patientPages struct {
	deps  PatientDeps
	links patientLinks
}

// list renders P1: every patient the actor owns (FR-010).
func (p *patientPages) list(e *core.RequestEvent, actor access.Actor) error {
	if err := requireSession(actor); err != nil {
		return err
	}

	svc, err := p.deps.Resolve()
	if err != nil {
		return err
	}

	page, err := svc.List(e.Request.Context(), actor, patient.Query{Count: true})
	if err != nil {
		return err
	}

	system, err := p.deps.UnitOf(e.Request.Context(), actor)
	if err != nil {
		return err
	}

	views := make([]patients.PatientView, 0, len(page.Items))
	for _, item := range page.Items {
		views = append(views, p.view(item, system))
	}

	total := len(views)
	if page.Total != nil {
		total = *page.Total
	}

	blank := patients.PatientView{}

	main := sequence{
		patients.PatientList(patients.PatientListProps{
			Patients:   views,
			Total:      total,
			CreateHref: "#" + ids.PatientForm(""),
			Notice:     noticeFor(e.Request.URL.Query().Get("notice")),
		}),
		patients.PatientForm(patients.PatientFormProps{
			FormID:     ids.PatientForm(""),
			New:        true,
			OnSubmit:   p.links.submitExpression(blank),
			CancelHref: p.links.cancelHref(blank),
			Patient:    blank,
			Errors:     patients.NewFieldErrors(nil),
		}),
	}

	return RenderPage(e, http.StatusOK, patientListTitle, p.nav(e, actor), main)
}

// noticeFor is FR-017/US3-3's explanation for a stale window: a tab left
// open on a person later deleted lands here rather than on a bare 404, and
// this is what it reads.
func noticeFor(notice string) string {
	if notice == "gone" {
		return "That person's page is no longer available."
	}

	return ""
}

// detail renders P2: a patient belonging to somebody else is a 404 here for
// the same reason it is one through the API (FR-042). A patient that no
// longer exists — including one this very tab just deleted, or one deleted
// from another tab while this one sat open (US3-3's stale window) — lands
// on the list with an explanation instead of a bare 404 (FR-017, US6's
// post-delete redirect).
func (p *patientPages) detail(e *core.RequestEvent, actor access.Actor) error {
	if err := requireSession(actor); err != nil {
		return err
	}

	svc, err := p.deps.Resolve()
	if err != nil {
		return err
	}

	chart, err := svc.Summary(e.Request.Context(), actor, e.Request.PathValue(api.PathPatientID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return e.Redirect(http.StatusSeeOther, p.links.listPage+"?notice=gone")
		}

		return err
	}

	system, err := p.deps.UnitOf(e.Request.Context(), actor)
	if err != nil {
		return err
	}

	view := p.view(chart.Patient, system)

	tiles := make([]patients.CountTile, 0, len(chart.Counts))
	for _, entry := range chart.Counts {
		tiles = append(tiles, patients.CountTile{Label: entry.Label, Path: entry.Path, Count: entry.Count})
	}

	events := make([]patients.ActivityEventView, 0, len(chart.RecentActivity))
	for _, event := range chart.RecentActivity {
		events = append(events, patients.ActivityEventView{
			OccurredAt:   event.OccurredAt,
			Action:       string(event.Action),
			TargetKind:   string(event.TargetKind),
			TargetID:     event.TargetID,
			TargetExists: api.TargetExists(e.Request.Context(), p.deps.Records, actor, event),
		})
	}

	main := sequence{
		patients.PatientDetail(patients.PatientDetailProps{
			Patient:      view,
			Tiles:        patients.NewChartTiles(view.ID, tiles),
			Activity:     patients.NewActivityItems(events, func(string, string) string { return "" }),
			TotalRecords: chart.TotalRecords,
		}),
		patients.PatientForm(patients.PatientFormProps{
			FormID:     ids.PatientForm(view.ID),
			New:        false,
			OnSubmit:   p.links.submitExpression(view),
			CancelHref: p.links.cancelHref(view),
			Patient:    view,
			Errors:     patients.NewFieldErrors(nil),
		}),
	}

	return RenderPage(e, http.StatusOK, view.FullName(), p.nav(e, actor), main)
}

// nav builds the primary navigation plus FR-014's switcher, threaded through
// every page in this package the same way.
func (p *patientPages) nav(e *core.RequestEvent, actor access.Actor) NavState {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.deps.Resolve)
	if err != nil {
		// The switcher is presentation on top of a page that has already
		// resolved and rendered fine without it; it degrades to an empty
		// control rather than failing a page that has nothing else wrong.
		switcher = shell.PatientSwitcherProps{}
	}

	return NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}
}

func (p *patientPages) view(found domainperson.Patient, system identity.UnitSystem) patients.PatientView {
	var photoURL string
	if found.HasPhoto {
		photoURL = "/api/v1/patients/" + found.ID + "/photo"
	}

	return patients.NewPatientView(found, photoURL, system, patients.PatientLinks{
		Detail: p.links.of(found.ID),
		Record: "/api/v1/patients/" + found.ID,
	})
}

type patientLinks struct {
	listPage        string
	detailPage      string
	settingsPage    string
	medicationsPage string
	collection      string
}

func newPatientLinks() (patientLinks, error) {
	paths, err := routePaths(map[string]string{
		OpPatientListPage:    "",
		OpPatientDetailPage:  "",
		OpSettingsPage:       "",
		OpMedicationListPage: "",
		api.OpCreatePatient:  "",
	})
	if err != nil {
		return patientLinks{}, err
	}

	return patientLinks{
		listPage:        paths[OpPatientListPage],
		detailPage:      paths[OpPatientDetailPage],
		settingsPage:    paths[OpSettingsPage],
		medicationsPage: paths[OpMedicationListPage],
		collection:      paths[api.OpCreatePatient],
	}, nil
}

func (l patientLinks) of(id string) string {
	return strings.ReplaceAll(l.detailPage, "{"+api.PathPatientID+"}", id)
}

// submitExpression mirrors medicationLinks' own (medications.go): a create
// posts to the collection, a change patches the record and carries the ETag
// the detail page rendered into $etag as If-Match.
func (l patientLinks) submitExpression(view patients.PatientView) string {
	if view.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(view.Links.Record) + ", {headers: {'If-Match': $etag}})"
}

func (l patientLinks) cancelHref(view patients.PatientView) string {
	if view.Links.Detail != "" {
		return view.Links.Detail
	}

	return l.listPage
}

// nav mirrors medicationLinks.nav's own reasoning: FR-050 requires every
// signed-in page, this surface included, to offer a route back to the
// medication list and to settings.
func (l patientLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: patientListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
