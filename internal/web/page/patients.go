package page

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	domainperson "medikube/internal/domain/person"
	"medikube/internal/httproute"
	"medikube/internal/service/patient"
	"medikube/internal/web"
	"medikube/internal/web/api"
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

// PatientDeps is what the two patient pages need.
type PatientDeps struct {
	Resolve api.PatientResolve
	UnitOf  api.UnitSystemOf
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

	main := patients.PatientList(patients.PatientListProps{
		Patients: views,
		Total:    total,
	})

	return RenderPage(e, http.StatusOK, patientListTitle, NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path)}, main)
}

// detail renders P2: a patient belonging to somebody else is a 404 here for
// the same reason it is one through the API (FR-042).
func (p *patientPages) detail(e *core.RequestEvent, actor access.Actor) error {
	if err := requireSession(actor); err != nil {
		return err
	}

	svc, err := p.deps.Resolve()
	if err != nil {
		return err
	}

	found, err := svc.Get(e.Request.Context(), actor, e.Request.PathValue(api.PathPatientID))
	if err != nil {
		return err
	}

	system, err := p.deps.UnitOf(e.Request.Context(), actor)
	if err != nil {
		return err
	}

	view := p.view(found, system)

	main := patients.PatientDetail(patients.PatientDetailProps{Patient: view})

	return RenderPage(e, http.StatusOK, view.FullName(), NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path)}, main)
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
}

func newPatientLinks() (patientLinks, error) {
	paths, err := routePaths(map[string]string{
		OpPatientListPage:    "",
		OpPatientDetailPage:  "",
		OpSettingsPage:       "",
		OpMedicationListPage: "",
	})
	if err != nil {
		return patientLinks{}, err
	}

	return patientLinks{
		listPage:        paths[OpPatientListPage],
		detailPage:      paths[OpPatientDetailPage],
		settingsPage:    paths[OpSettingsPage],
		medicationsPage: paths[OpMedicationListPage],
	}, nil
}

func (l patientLinks) of(id string) string {
	return strings.ReplaceAll(l.detailPage, "{"+api.PathPatientID+"}", id)
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
