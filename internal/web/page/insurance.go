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
	OpInsuranceListPage   = "insuranceListPage"
	OpInsuranceDetailPage = "insuranceDetailPage"
)

const insuranceListTitle = "Insurance"

// insuranceListTitleID is a message id (D-06), resolved at render time. The
// raw insuranceListTitle stays as-is: shell.NavLink.Label (out of scope,
// shell package) renders it unresolved.
const insuranceListTitleID = "page.insurance.title"

// InsuranceHandlers is the record pages' contribution to the route table.
func InsuranceHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newInsuranceLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &insurancePages{resolve: resolve, patients: patients, tags: tags, links: links, views: InsuranceViews{links: links}}

	return httproute.Handlers{
		OpInsuranceListPage:   web.WithActor(pages.list),
		OpInsuranceDetailPage: web.WithActor(pages.detail),
	}, nil
}

// InsuranceViews is the kind's rendering, as internal/records consumes it.
type InsuranceViews struct {
	links insuranceLinks
}

var _ recordfamily.Views = InsuranceViews{}

func NewInsuranceViews() (InsuranceViews, error) {
	links, err := newInsuranceLinks()
	if err != nil {
		return InsuranceViews{}, err
	}

	return InsuranceViews{links: links}, nil
}

func (v InsuranceViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v InsuranceViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.InsuranceList(views.InsuranceListProps{
		Policies:   v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Insurance, ""),
		NextHref:   nextHref,
	})
}

func (v InsuranceViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.InsuranceRow(v.view(record))
}

func (v InsuranceViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.InsuranceDetail(views.InsuranceDetailProps{Insurance: v.view(record)})
}

func (v InsuranceViews) Title(record recordfamily.Record) string { return v.view(record).Company }

func (v InsuranceViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	policy := v.view(record)
	fresh := policy.ID == ""

	formID := ids.RecordForm(kind.Insurance, policy.ID)

	return views.InsuranceForm(views.InsuranceFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(policy),
		CancelHref: v.cancelHref(policy),
		Insurance:  policy,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v InsuranceViews) rows(items []recordfamily.Record) []views.InsuranceView {
	rendered := make([]views.InsuranceView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v InsuranceViews) view(record recordfamily.Record) views.InsuranceView {
	entity := clinical.Insurance{ID: record.ID, Version: record.Version}

	var basis []string

	switch body := record.Body.(type) {
	case *api.Insurance:
		entity = insuranceDetailEntity(record, *body)
	case *api.InsuranceSummary:
		entity = insuranceSummaryEntity(record, *body)
		basis = body.Basis
	case *api.InsuranceCreate:
		entity.PatientID = body.Patient
	}

	return views.NewInsuranceView(entity, basis, v.links.of(entity.ID))
}

func insuranceSummaryEntity(record recordfamily.Record, summary api.InsuranceSummary) clinical.Insurance {
	return clinical.Insurance{
		ID:        record.ID,
		Version:   record.Version,
		Company:   summary.Company,
		Type:      clinical.InsuranceType(summary.Type),
		Status:    clinical.InsuranceStatus(summary.Status),
		IsPrimary: summary.IsPrimary,
		ExpiresOn: readDate(summary.ExpiresOn),
		UpdatedAt: readInstant(summary.UpdatedAt),
	}
}

func insuranceDetailEntity(record recordfamily.Record, detail api.Insurance) clinical.Insurance {
	entity := insuranceSummaryEntity(record, detail.InsuranceSummary)

	entity.PatientID = detail.Patient
	entity.PlanName = detail.PlanName
	entity.EmployerGroup = detail.EmployerGroup
	entity.MemberName = detail.MemberName
	entity.MemberID = detail.MemberID
	entity.GroupNumber = detail.GroupNumber
	entity.HolderName = detail.HolderName
	entity.Relationship = clinical.HolderRelationship(detail.Relationship)
	entity.EffectiveOn = readDate(&detail.EffectiveOn)
	entity.Notes = detail.Notes
	entity.CreatedAt = readInstant(detail.CreatedAt)

	return entity
}

type insurancePages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    insuranceLinks
	views    InsuranceViews
}

func (p *insurancePages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Insurance.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Insurance.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Insurance}
	blank.Body = &api.InsuranceCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), insuranceListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *insurancePages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *insurancePages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *insurancePages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Insurance.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Insurance.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Insurance); ok {
		patientID = detail.Patient
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.view(found).Company, sequence{
		context,
		entry.Views.Detail(found),
		entry.Views.Form(found, nil, ""),
	})
}

func (p *insurancePages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *insurancePages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(localizeCtx(e), e.Request.URL.Path), Switcher: switcher}, main)
}

type insuranceLinks struct {
	listPage       string
	detailPage     string
	settingsPage   string
	patientsPage   string
	medicationPage string
	record         string
	collection     string
}

func newInsuranceLinks() (insuranceLinks, error) {
	paths, err := routePaths(map[string]string{
		OpInsuranceListPage:   "",
		OpInsuranceDetailPage: "",
		OpSettingsPage:        "",
		OpPatientListPage:     "",
		OpMedicationListPage:  "",
		api.OpGetRecord:       "",
		api.OpCreateRecord:    "",
	})
	if err != nil {
		return insuranceLinks{}, err
	}

	segment := kind.Insurance.Segment()

	return insuranceLinks{
		listPage:       paths[OpInsuranceListPage],
		detailPage:     paths[OpInsuranceDetailPage],
		settingsPage:   paths[OpSettingsPage],
		patientsPage:   paths[OpPatientListPage],
		medicationPage: paths[OpMedicationListPage],
		record:         strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:     strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l insuranceLinks) of(recordID string) views.InsuranceLinks {
	if recordID == "" {
		return views.InsuranceLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.InsuranceLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Insurance, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l insuranceLinks) submitExpression(policy views.InsuranceView) string {
	if policy.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(policy.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l insuranceLinks) cancelHref(policy views.InsuranceView) string {
	if policy.Links.Detail != "" {
		return policy.Links.Detail
	}

	return l.listPage
}

func (v InsuranceViews) cancelHref(policy views.InsuranceView) string {
	return v.links.cancelHref(policy)
}

// nav offers, beyond this kind's own entry, the medication list and settings
// (FR-050): every signed-in page keeps both one link away, regardless of
// which record kind it is on.
func (l insuranceLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.medicationPage, Current: strings.HasPrefix(current, l.medicationPage)},
		{Label: insuranceListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
