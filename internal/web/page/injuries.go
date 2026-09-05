package page

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	recordfamily "medikube/internal/records"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
	views "medikube/internal/web/views/records"
	"medikube/internal/web/views/shell"
)

// The operation ids of injuries' list and detail pages, mirroring
// medications.go's OpMedicationListPage/OpMedicationDetailPage.
const (
	OpInjuryListPage   = "injuryListPage"
	OpInjuryDetailPage = "injuryDetailPage"
)

const injuryListTitle = "Injuries"

// InjuryHandlers is injuries' contribution to the route table. It is named
// with the kind's prefix rather than the bare `Handlers` medications.go
// exports, because that name is already taken in this package — this
// package's later kinds all follow PractitionerHandlers/FacilityHandlers'
// precedent instead.
func InjuryHandlers(resolve api.Resolve, patients api.PatientResolve) (httproute.Handlers, error) {
	links, err := newInjuryLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &injuryPages{resolve: resolve, patients: patients, links: links, views: InjuryViews{links: links}}

	return httproute.Handlers{
		OpInjuryListPage:   web.WithActor(pages.list),
		OpInjuryDetailPage: web.WithActor(pages.detail),
	}, nil
}

// InjuryViews is the kind's rendering, as internal/records consumes it,
// mirroring MedicationViews.
type InjuryViews struct {
	links injuryLinks
}

var _ recordfamily.Views = InjuryViews{}

// NewInjuryViews reads its address templates out of the route inventory, so
// the link on a row is by construction the route the router serves.
func NewInjuryViews() (InjuryViews, error) {
	links, err := newInjuryLinks()
	if err != nil {
		return InjuryViews{}, err
	}

	return InjuryViews{links: links}, nil
}

func (v InjuryViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v InjuryViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.InjuryList(views.InjuryListProps{
		Injuries:   v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Injury, ""),
		NextHref:   nextHref,
	})
}

func (v InjuryViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.InjuryRow(v.view(record))
}

func (v InjuryViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.InjuryDetail(views.InjuryDetailProps{Injury: v.view(record)})
}

func (v InjuryViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	injury := v.view(record)
	fresh := injury.ID == ""

	return views.InjuryForm(views.InjuryFormProps{
		FormID:     ids.RecordForm(kind.Injury, injury.ID),
		New:        fresh,
		OnSubmit:   v.links.submitExpression(injury),
		CancelHref: v.cancelHref(injury),
		Injury:     injury,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
	})
}

func (v InjuryViews) rows(items []recordfamily.Record) []views.InjuryView {
	rendered := make([]views.InjuryView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v InjuryViews) view(record recordfamily.Record) views.InjuryView {
	injury := clinical.Injury{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Injury:
		injury = injuryDetailEntity(record, *body)
	case *api.InjurySummary:
		injury = injurySummaryEntity(record, *body)
	case *api.InjuryCreate:
		injury.PatientID = body.Patient
	}

	return views.NewInjuryView(injury, v.links.of(injury.ID))
}

func injurySummaryEntity(record recordfamily.Record, summary api.InjurySummary) clinical.Injury {
	return clinical.Injury{
		ID:         record.ID,
		Version:    record.Version,
		Name:       summary.Name,
		Type:       clinical.InjuryType(summary.Type),
		Severity:   clinical.Severity(summary.Severity),
		Status:     clinical.ConditionStatus(summary.Status),
		OccurredOn: readDate(summary.OccurredOn),
		UpdatedAt:  readInstant(summary.UpdatedAt),
	}
}

func injuryDetailEntity(record recordfamily.Record, detail api.Injury) clinical.Injury {
	injury := injurySummaryEntity(record, detail.InjurySummary)

	injury.PatientID = detail.Patient
	injury.PractitionerID = detail.Practitioner
	injury.BodyPart = detail.BodyPart
	injury.Laterality = clinical.Laterality(detail.Laterality)
	injury.Mechanism = detail.Mechanism
	injury.RecoveryNotes = detail.RecoveryNotes
	injury.MedicationIDs = detail.Medications
	injury.CreatedAt = readInstant(detail.CreatedAt)

	return injury
}

type injuryPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	links    injuryLinks
	views    InjuryViews
}

// list renders the list page, mirroring medicationPages.list.
func (p *injuryPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Injury.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Injury.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Injury}
	blank.Body = &api.InjuryCreate{Patient: query.PatientID}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, injuryListTitle, sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *injuryPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
	svc, err := p.patients()
	if err != nil {
		return err
	}

	active, err := svc.ResolveActivePatient(e.Request.Context(), actor)
	if err != nil {
		return err
	}

	if active == nil {
		return e.Redirect(http.StatusSeeOther, p.links.patientsPage)
	}

	next := *e.Request.URL
	values := next.Query()
	values.Set(api.ParamPatient, active.ID)
	next.RawQuery = values.Encode()

	return e.Redirect(http.StatusSeeOther, next.RequestURI())
}

func (p *injuryPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
	if patientID == "" {
		return shell.PatientContextHeader(shell.PatientContextProps{}), nil
	}

	svc, err := p.patients()
	if err != nil {
		return nil, err
	}

	found, err := svc.Get(ctx, actor, patientID)
	if err != nil {
		return nil, err
	}

	return shell.PatientContextHeader(shell.PatientContextProps{
		Name: found.FirstName + " " + found.LastName,
		Href: "/patients/" + found.ID,
	}), nil
}

func (p *injuryPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Injury.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Injury.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Injury); ok {
		patientID = detail.Patient
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.view(found).Name, sequence{
		context,
		entry.Views.Detail(found),
		entry.Views.Form(found, nil, ""),
	})
}

func (p *injuryPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *injuryPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

// injuryLinks holds the addresses the pages and the components need,
// mirroring medicationLinks.
type injuryLinks struct {
	listPage       string
	detailPage     string
	medicationsURL string
	settingsPage   string
	patientsPage   string
	record         string
	collection     string
}

func newInjuryLinks() (injuryLinks, error) {
	paths, err := routePaths(map[string]string{
		OpInjuryListPage:     "",
		OpInjuryDetailPage:   "",
		OpMedicationListPage: "",
		OpSettingsPage:       "",
		OpPatientListPage:    "",
		api.OpGetRecord:      "",
		api.OpCreateRecord:   "",
	})
	if err != nil {
		return injuryLinks{}, err
	}

	segment := kind.Injury.Segment()

	return injuryLinks{
		listPage:       paths[OpInjuryListPage],
		detailPage:     paths[OpInjuryDetailPage],
		medicationsURL: paths[OpMedicationListPage],
		settingsPage:   paths[OpSettingsPage],
		patientsPage:   paths[OpPatientListPage],
		record:         strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:     strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l injuryLinks) of(recordID string) views.InjuryLinks {
	if recordID == "" {
		return views.InjuryLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.InjuryLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Injury, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l injuryLinks) submitExpression(injury views.InjuryView) string {
	if injury.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(injury.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l injuryLinks) cancelHref(injury views.InjuryView) string {
	if injury.Links.Detail != "" {
		return injury.Links.Detail
	}

	return l.listPage
}

func (v InjuryViews) cancelHref(injury views.InjuryView) string {
	return v.links.cancelHref(injury)
}

// nav is the primary navigation's contents. FR-050 requires a route back to
// the medication list from every signed-in page, this one included, so it is
// offered here beside injury's own list and settings — mirroring
// practitionerLinks.nav and facilityLinks.nav's precedent for a non-medication
// kind rather than medicationLinks.nav, which is the medication list's own
// entry and has nothing else to link back to.
func (l injuryLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsURL, Current: strings.HasPrefix(current, l.medicationsURL)},
		{Label: injuryListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
