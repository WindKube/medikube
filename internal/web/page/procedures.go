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

const (
	OpProcedureListPage   = "procedureListPage"
	OpProcedureDetailPage = "procedureDetailPage"
)

const procedureListTitleID = "page.procedureListPage.title"

// ProcedureHandlers is the procedure pages' contribution to the route table,
// mirroring medications.go's Handlers end to end (T078).
func ProcedureHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newProcedureLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &procedurePages{resolve: resolve, patients: patients, tags: tags, links: links, views: ProcedureViews{links: links}}

	return httproute.Handlers{
		OpProcedureListPage:   web.WithActor(pages.list),
		OpProcedureDetailPage: web.WithActor(pages.detail),
	}, nil
}

// ProcedureViews is the kind's rendering, as internal/records consumes it.
type ProcedureViews struct {
	links procedureLinks
}

var _ recordfamily.Views = ProcedureViews{}

func NewProcedureViews() (ProcedureViews, error) {
	links, err := newProcedureLinks()
	if err != nil {
		return ProcedureViews{}, err
	}

	return ProcedureViews{links: links}, nil
}

func (v ProcedureViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v ProcedureViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.ProcedureList(views.ProcedureListProps{
		Procedures: v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Procedure, ""),
		NextHref:   nextHref,
	})
}

func (v ProcedureViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.ProcedureRow(v.view(record))
}

func (v ProcedureViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.ProcedureDetail(views.ProcedureDetailProps{Procedure: v.view(record)})
}

func (v ProcedureViews) Title(record recordfamily.Record) string { return v.view(record).Name }

func (v ProcedureViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	procedure := v.view(record)
	fresh := procedure.ID == ""

	formID := ids.RecordForm(kind.Procedure, procedure.ID)

	return views.ProcedureForm(views.ProcedureFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(procedure),
		CancelHref: v.links.cancelHref(procedure),
		Procedure:  procedure,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v ProcedureViews) rows(items []recordfamily.Record) []views.ProcedureView {
	rendered := make([]views.ProcedureView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v ProcedureViews) view(record recordfamily.Record) views.ProcedureView {
	procedure := clinical.Procedure{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Procedure:
		procedure = procedureDetailEntity(record, *body)
	case *api.ProcedureSummary:
		procedure = procedureSummaryEntity(record, *body)
	case *api.ProcedureCreate:
		procedure.PatientID = body.Patient
	}

	return views.NewProcedureView(procedure, v.links.of(procedure.ID))
}

func procedureSummaryEntity(record recordfamily.Record, summary api.ProcedureSummary) clinical.Procedure {
	return clinical.Procedure{
		ID:         record.ID,
		Version:    record.Version,
		Name:       summary.Name,
		Status:     clinical.OrderStatus(summary.Status),
		OccurredOn: readDate(summary.OccurredOn),
		Outcome:    clinical.ProcedureOutcome(summary.Outcome),
		UpdatedAt:  readInstant(summary.UpdatedAt),
	}
}

func procedureDetailEntity(record recordfamily.Record, detail api.Procedure) clinical.Procedure {
	procedure := procedureSummaryEntity(record, detail.ProcedureSummary)

	procedure.PatientID = detail.Patient
	procedure.Type = clinical.ProcedureType(detail.Type)
	procedure.Code = detail.Code
	procedure.Description = detail.Description
	procedure.Setting = clinical.ProcedureSetting(detail.Setting)
	procedure.Complications = detail.Complications
	procedure.DurationMin = detail.DurationMin
	procedure.Anesthesia = clinical.Anesthesia(detail.Anesthesia)
	procedure.AnesthesiaNotes = detail.AnesthesiaNotes
	procedure.PractitionerID = detail.Practitioner
	procedure.FacilityID = detail.Facility
	procedure.Notes = detail.Notes
	procedure.CreatedAt = readInstant(detail.CreatedAt)

	return procedure
}

type procedurePages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    procedureLinks
	views    ProcedureViews
}

func (p *procedurePages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Procedure.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Procedure.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Procedure}
	blank.Body = &api.ProcedureCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), procedureListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *procedurePages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *procedurePages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *procedurePages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Procedure.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Procedure.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Procedure); ok {
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

func (p *procedurePages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *procedurePages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(localizeCtx(e), e.Request.URL.Path), Switcher: switcher}, main)
}

type procedureLinks struct {
	listPage        string
	medicationsPage string
	detailPage      string
	settingsPage    string
	patientsPage    string
	record          string
	collection      string
}

func newProcedureLinks() (procedureLinks, error) {
	paths, err := routePaths(map[string]string{
		OpProcedureListPage:   "",
		OpProcedureDetailPage: "",
		OpSettingsPage:        "",
		OpMedicationListPage:  "",
		OpPatientListPage:     "",
		api.OpGetRecord:       "",
		api.OpCreateRecord:    "",
	})
	if err != nil {
		return procedureLinks{}, err
	}

	segment := kind.Procedure.Segment()

	return procedureLinks{
		listPage:        paths[OpProcedureListPage],
		medicationsPage: paths[OpMedicationListPage],
		detailPage:      paths[OpProcedureDetailPage],
		settingsPage:    paths[OpSettingsPage],
		patientsPage:    paths[OpPatientListPage],
		record:          strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:      strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l procedureLinks) of(recordID string) views.ProcedureLinks {
	if recordID == "" {
		return views.ProcedureLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.ProcedureLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Procedure, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l procedureLinks) submitExpression(procedure views.ProcedureView) string {
	if procedure.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(procedure.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l procedureLinks) cancelHref(procedure views.ProcedureView) string {
	if procedure.Links.Detail != "" {
		return procedure.Links.Detail
	}

	return l.listPage
}

func (l procedureLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: i18n.T(ctx, procedureListTitleID), Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
