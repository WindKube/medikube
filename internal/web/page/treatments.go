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

const (
	OpTreatmentListPage   = "treatmentListPage"
	OpTreatmentDetailPage = "treatmentDetailPage"
)

const treatmentListTitle = "Treatments"

// TreatmentHandlers is the treatment pages' contribution to the route table,
// mirroring medications.go's Handlers end to end (T078).
func TreatmentHandlers(resolve api.Resolve, patients api.PatientResolve) (httproute.Handlers, error) {
	links, err := newTreatmentLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &treatmentPages{resolve: resolve, patients: patients, links: links, views: TreatmentViews{links: links}}

	return httproute.Handlers{
		OpTreatmentListPage:   web.WithActor(pages.list),
		OpTreatmentDetailPage: web.WithActor(pages.detail),
	}, nil
}

// TreatmentViews is the kind's rendering, as internal/records consumes it.
type TreatmentViews struct {
	links treatmentLinks
}

var _ recordfamily.Views = TreatmentViews{}

func NewTreatmentViews() (TreatmentViews, error) {
	links, err := newTreatmentLinks()
	if err != nil {
		return TreatmentViews{}, err
	}

	return TreatmentViews{links: links}, nil
}

func (v TreatmentViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v TreatmentViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.TreatmentList(views.TreatmentListProps{
		Treatments: v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Treatment, ""),
		NextHref:   nextHref,
	})
}

func (v TreatmentViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.TreatmentRow(v.view(record))
}

func (v TreatmentViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.TreatmentDetail(views.TreatmentDetailProps{Treatment: v.view(record)})
}

func (v TreatmentViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	treatment := v.view(record)
	fresh := treatment.ID == ""

	return views.TreatmentForm(views.TreatmentFormProps{
		FormID:     ids.RecordForm(kind.Treatment, treatment.ID),
		New:        fresh,
		OnSubmit:   v.links.submitExpression(treatment),
		CancelHref: v.links.cancelHref(treatment),
		Treatment:  treatment,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
	})
}

func (v TreatmentViews) rows(items []recordfamily.Record) []views.TreatmentView {
	rendered := make([]views.TreatmentView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v TreatmentViews) view(record recordfamily.Record) views.TreatmentView {
	treatment := clinical.Treatment{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Treatment:
		treatment = treatmentDetailEntity(record, *body)
	case *api.TreatmentSummary:
		treatment = treatmentSummaryEntity(record, *body)
	case *api.TreatmentCreate:
		treatment.PatientID = body.Patient
	}

	return views.NewTreatmentView(treatment, v.links.of(treatment.ID))
}

func treatmentSummaryEntity(record recordfamily.Record, summary api.TreatmentSummary) clinical.Treatment {
	return clinical.Treatment{
		ID:        record.ID,
		Version:   record.Version,
		Name:      summary.Name,
		Status:    clinical.TherapyStatus(summary.Status),
		StartedOn: readDate(summary.StartedOn),
		UpdatedAt: readInstant(summary.UpdatedAt),
	}
}

func treatmentDetailEntity(record recordfamily.Record, detail api.Treatment) clinical.Treatment {
	treatment := treatmentSummaryEntity(record, detail.TreatmentSummary)

	treatment.PatientID = detail.Patient
	treatment.Type = detail.Type
	treatment.Setting = clinical.TreatmentSetting(detail.Setting)
	treatment.Description = detail.Description
	treatment.EndedOn = readDate(detail.EndedOn)
	treatment.Frequency = detail.Frequency
	treatment.Dosage = detail.Dosage
	treatment.ExpectedOutcome = detail.ExpectedOutcome
	treatment.PractitionerID = detail.Practitioner
	treatment.FacilityID = detail.Facility
	treatment.Encounters = detail.Encounters
	treatment.Equipment = detail.Equipment
	treatment.Notes = detail.Notes
	treatment.CreatedAt = readInstant(detail.CreatedAt)

	return treatment
}

type treatmentPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	links    treatmentLinks
	views    TreatmentViews
}

func (p *treatmentPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Treatment.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Treatment.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Treatment}
	blank.Body = &api.TreatmentCreate{Patient: query.PatientID}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, treatmentListTitle, sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *treatmentPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *treatmentPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *treatmentPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Treatment.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Treatment.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Treatment); ok {
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

func (p *treatmentPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *treatmentPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

type treatmentLinks struct {
	listPage        string
	medicationsPage string
	detailPage      string
	settingsPage    string
	patientsPage    string
	record          string
	collection      string
}

func newTreatmentLinks() (treatmentLinks, error) {
	paths, err := routePaths(map[string]string{
		OpTreatmentListPage:   "",
		OpTreatmentDetailPage: "",
		OpSettingsPage:        "",
		OpMedicationListPage:  "",
		OpPatientListPage:     "",
		api.OpGetRecord:       "",
		api.OpCreateRecord:    "",
	})
	if err != nil {
		return treatmentLinks{}, err
	}

	segment := kind.Treatment.Segment()

	return treatmentLinks{
		listPage:        paths[OpTreatmentListPage],
		medicationsPage: paths[OpMedicationListPage],
		detailPage:      paths[OpTreatmentDetailPage],
		settingsPage:    paths[OpSettingsPage],
		patientsPage:    paths[OpPatientListPage],
		record:          strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:      strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l treatmentLinks) of(recordID string) views.TreatmentLinks {
	if recordID == "" {
		return views.TreatmentLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.TreatmentLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Treatment, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l treatmentLinks) submitExpression(treatment views.TreatmentView) string {
	if treatment.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(treatment.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l treatmentLinks) cancelHref(treatment views.TreatmentView) string {
	if treatment.Links.Detail != "" {
		return treatment.Links.Detail
	}

	return l.listPage
}

func (l treatmentLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: treatmentListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
