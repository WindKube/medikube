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
	OpConditionListPage   = "conditionListPage"
	OpConditionDetailPage = "conditionDetailPage"
)

const conditionListTitle = "Conditions"

// ConditionHandlers is the condition pages' contribution to the route table.
func ConditionHandlers(resolve api.Resolve, patients api.PatientResolve) (httproute.Handlers, error) {
	links, err := newConditionLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &conditionPages{resolve: resolve, patients: patients, links: links, views: ConditionViews{links: links}}

	return httproute.Handlers{
		OpConditionListPage:   web.WithActor(pages.list),
		OpConditionDetailPage: web.WithActor(pages.detail),
	}, nil
}

// ConditionViews is the kind's rendering, as internal/records consumes it.
type ConditionViews struct {
	links conditionLinks
}

var _ recordfamily.Views = ConditionViews{}

func NewConditionViews() (ConditionViews, error) {
	links, err := newConditionLinks()
	if err != nil {
		return ConditionViews{}, err
	}

	return ConditionViews{links: links}, nil
}

func (v ConditionViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v ConditionViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.ConditionList(views.ConditionListProps{
		Conditions: v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Condition, ""),
		NextHref:   nextHref,
	})
}

func (v ConditionViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.ConditionRow(v.view(record))
}

func (v ConditionViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.ConditionDetail(views.ConditionDetailProps{Condition: v.view(record)})
}

func (v ConditionViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	condition := v.view(record)
	fresh := condition.ID == ""

	return views.ConditionForm(views.ConditionFormProps{
		FormID:     ids.RecordForm(kind.Condition, condition.ID),
		New:        fresh,
		OnSubmit:   v.links.submitExpression(condition),
		CancelHref: v.cancelHref(condition),
		Condition:  condition,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
	})
}

func (v ConditionViews) rows(items []recordfamily.Record) []views.ConditionView {
	rendered := make([]views.ConditionView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v ConditionViews) view(record recordfamily.Record) views.ConditionView {
	condition := clinical.Condition{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Condition:
		condition = conditionDetailEntity(record, *body)
	case *api.ConditionSummary:
		condition = conditionSummaryEntity(record, *body)
	case *api.ConditionCreate:
		condition.PatientID = body.Patient
	}

	return views.NewConditionView(condition, v.links.of(condition.ID))
}

func conditionSummaryEntity(record recordfamily.Record, summary api.ConditionSummary) clinical.Condition {
	return clinical.Condition{
		ID:        record.ID,
		Version:   record.Version,
		Diagnosis: summary.Diagnosis,
		Status:    clinical.ConditionStatus(summary.Status),
		Severity:  clinical.Severity(summary.Severity),
		OnsetOn:   readDate(summary.OnsetOn),
		UpdatedAt: readInstant(summary.UpdatedAt),
	}
}

func conditionDetailEntity(record recordfamily.Record, detail api.Condition) clinical.Condition {
	condition := conditionSummaryEntity(record, detail.ConditionSummary)

	condition.PatientID = detail.Patient
	condition.ResolvedOn = readDate(detail.ResolvedOn)
	condition.ICD10Code = detail.ICD10Code
	condition.SNOMEDCode = detail.SNOMEDCode
	condition.PractitionerID = detail.Practitioner
	condition.Notes = detail.Notes
	condition.CreatedAt = readInstant(detail.CreatedAt)

	return condition
}

type conditionPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	links    conditionLinks
	views    ConditionViews
}

func (p *conditionPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Condition.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Condition.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Condition}
	blank.Body = &api.ConditionCreate{Patient: query.PatientID}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, conditionListTitle, sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *conditionPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *conditionPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *conditionPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Condition.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Condition.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Condition); ok {
		patientID = detail.Patient
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.view(found).Diagnosis, sequence{
		context,
		entry.Views.Detail(found),
		entry.Views.Form(found, nil, ""),
	})
}

func (p *conditionPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *conditionPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

type conditionLinks struct {
	listPage        string
	detailPage      string
	settingsPage    string
	patientsPage    string
	medicationsPage string
	record          string
	collection      string
}

func newConditionLinks() (conditionLinks, error) {
	paths, err := routePaths(map[string]string{
		OpConditionListPage:   "",
		OpConditionDetailPage: "",
		OpSettingsPage:        "",
		OpPatientListPage:     "",
		OpMedicationListPage:  "",
		api.OpGetRecord:       "",
		api.OpCreateRecord:    "",
	})
	if err != nil {
		return conditionLinks{}, err
	}

	segment := kind.Condition.Segment()

	return conditionLinks{
		listPage:        paths[OpConditionListPage],
		detailPage:      paths[OpConditionDetailPage],
		settingsPage:    paths[OpSettingsPage],
		patientsPage:    paths[OpPatientListPage],
		medicationsPage: paths[OpMedicationListPage],
		record:          strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:      strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l conditionLinks) of(recordID string) views.ConditionLinks {
	if recordID == "" {
		return views.ConditionLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.ConditionLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Condition, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l conditionLinks) submitExpression(condition views.ConditionView) string {
	if condition.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(condition.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l conditionLinks) cancelHref(condition views.ConditionView) string {
	if condition.Links.Detail != "" {
		return condition.Links.Detail
	}

	return l.listPage
}

func (v ConditionViews) cancelHref(condition views.ConditionView) string {
	return v.links.cancelHref(condition)
}

// nav is FR-050's fixed pair, not a per-kind entry: every signed-in page
// offers the route back to the medication list and to settings, the same
// nav medications.go itself renders.
func (l conditionLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
