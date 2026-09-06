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
	"medikube/internal/i18n"
	recordfamily "medikube/internal/records"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
	views "medikube/internal/web/views/records"
	"medikube/internal/web/views/shell"
)

// The operation ids of contracts/pages.md's P4 and P5, spelled as
// internal/httproute declares them, mirroring medications.go's.
const (
	OpImmunizationListPage   = "immunizationListPage"
	OpImmunizationDetailPage = "immunizationDetailPage"
)

const immunizationListTitle = "Vaccinations"

// immunizationListTitleID is a message id (D-06), resolved at render time.
// The raw immunizationListTitle stays as-is: shell.NavLink.Label (out of
// scope, shell package) renders it unresolved.
const immunizationListTitleID = "page.vaccinations.title"

// ImmunizationHandlers is the record pages' contribution to the route table,
// named after the kind rather than bare Handlers: page.Handlers is already
// medication's (the one kind whose page functions predate this naming
// convention), and page.PractitionerHandlers / page.FacilityHandlers are the
// precedent every other kind follows.
func ImmunizationHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newImmunizationLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &immunizationPages{resolve: resolve, patients: patients, tags: tags, links: links, views: ImmunizationViews{links: links}}

	return httproute.Handlers{
		OpImmunizationListPage:   web.WithActor(pages.list),
		OpImmunizationDetailPage: web.WithActor(pages.detail),
	}, nil
}

// ImmunizationViews is the kind's rendering, as internal/records consumes it.
type ImmunizationViews struct {
	links immunizationLinks
}

var _ recordfamily.Views = ImmunizationViews{}

// NewImmunizationViews reads its three address templates out of the route
// inventory, mirroring NewMedicationViews.
func NewImmunizationViews() (ImmunizationViews, error) {
	links, err := newImmunizationLinks()
	if err != nil {
		return ImmunizationViews{}, err
	}

	return ImmunizationViews{links: links}, nil
}

func (v ImmunizationViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v ImmunizationViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.ImmunizationList(views.ImmunizationListProps{
		Immunizations: v.rows(page.Items),
		CreateHref:    "#" + ids.RecordForm(kind.Immunization, ""),
		NextHref:      nextHref,
	})
}

func (v ImmunizationViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.ImmunizationRow(v.view(record))
}

func (v ImmunizationViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.ImmunizationDetail(views.ImmunizationDetailProps{Immunization: v.view(record)})
}

func (v ImmunizationViews) Title(record recordfamily.Record) string {
	return v.view(record).VaccineName
}

func (v ImmunizationViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	immunization := v.view(record)
	fresh := immunization.ID == ""

	formID := ids.RecordForm(kind.Immunization, immunization.ID)

	return views.ImmunizationForm(views.ImmunizationFormProps{
		FormID:       formID,
		New:          fresh,
		OnSubmit:     v.links.submitExpression(immunization),
		CancelHref:   v.cancelHref(immunization),
		Immunization: immunization,
		Errors:       views.NewFieldErrors(invalid),
		Notice:       notice,
		Tags:         tagField(formID, record),
	})
}

func (v ImmunizationViews) rows(items []recordfamily.Record) []views.ImmunizationView {
	rendered := make([]views.ImmunizationView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v ImmunizationViews) view(record recordfamily.Record) views.ImmunizationView {
	immunization := clinical.Immunization{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Immunization:
		immunization = immunizationDetailEntity(record, *body)
	case *api.ImmunizationSummary:
		immunization = immunizationSummaryEntity(record, *body)
	case *api.ImmunizationCreate:
		immunization.PatientID = body.Patient
	}

	return views.NewImmunizationView(immunization, v.links.of(immunization.ID))
}

func immunizationSummaryEntity(record recordfamily.Record, summary api.ImmunizationSummary) clinical.Immunization {
	return clinical.Immunization{
		ID:             record.ID,
		Version:        record.Version,
		VaccineName:    summary.VaccineName,
		AdministeredOn: readDate(summary.AdministeredOn),
		DoseNumber:     summary.DoseNumber,
		UpdatedAt:      readInstant(summary.UpdatedAt),
	}
}

func immunizationDetailEntity(record recordfamily.Record, detail api.Immunization) clinical.Immunization {
	immunization := immunizationSummaryEntity(record, detail.ImmunizationSummary)

	immunization.PatientID = detail.Patient
	immunization.PractitionerID = detail.Practitioner
	immunization.FacilityID = detail.Facility
	immunization.TradeName = detail.TradeName
	immunization.LotNumber = detail.LotNumber
	immunization.Manufacturer = detail.Manufacturer
	immunization.Site = clinical.ImmunizationSite(detail.Site)
	immunization.Route = clinical.ImmunizationRoute(detail.Route)
	immunization.ExpiresOn = readDate(detail.ExpiresOn)
	immunization.CreatedAt = readInstant(detail.CreatedAt)

	return immunization
}

type immunizationPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    immunizationLinks
	views    ImmunizationViews
}

func (p *immunizationPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Immunization.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Immunization.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Immunization}
	blank.Body = &api.ImmunizationCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), immunizationListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *immunizationPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *immunizationPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *immunizationPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Immunization.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Immunization.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Immunization); ok {
		patientID = detail.Patient
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.view(found).VaccineName, sequence{
		context,
		entry.Views.Detail(found),
		entry.Views.Form(found, nil, ""),
	})
}

func (p *immunizationPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *immunizationPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.Context(), e.Request.URL.Path), Switcher: switcher}, main)
}

// immunizationLinks holds the five addresses the pages and the components
// need, mirroring medicationLinks.
type immunizationLinks struct {
	listPage       string
	detailPage     string
	medicationsURL string
	settingsPage   string
	patientsPage   string
	record         string
	collection     string
}

func newImmunizationLinks() (immunizationLinks, error) {
	paths, err := routePaths(map[string]string{
		OpImmunizationListPage:   "",
		OpImmunizationDetailPage: "",
		OpMedicationListPage:     "",
		OpSettingsPage:           "",
		OpPatientListPage:        "",
		api.OpGetRecord:          "",
		api.OpCreateRecord:       "",
	})
	if err != nil {
		return immunizationLinks{}, err
	}

	segment := kind.Immunization.Segment()

	return immunizationLinks{
		listPage:       paths[OpImmunizationListPage],
		detailPage:     paths[OpImmunizationDetailPage],
		medicationsURL: paths[OpMedicationListPage],
		settingsPage:   paths[OpSettingsPage],
		patientsPage:   paths[OpPatientListPage],
		record:         strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:     strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l immunizationLinks) of(recordID string) views.ImmunizationLinks {
	if recordID == "" {
		return views.ImmunizationLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.ImmunizationLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Immunization, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l immunizationLinks) submitExpression(immunization views.ImmunizationView) string {
	if immunization.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(immunization.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l immunizationLinks) cancelHref(immunization views.ImmunizationView) string {
	if immunization.Links.Detail != "" {
		return immunization.Links.Detail
	}

	return l.listPage
}

func (v ImmunizationViews) cancelHref(immunization views.ImmunizationView) string {
	return v.links.cancelHref(immunization)
}

func (l immunizationLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.medicationsURL, Current: strings.HasPrefix(current, l.medicationsURL)},
		{Label: immunizationListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
