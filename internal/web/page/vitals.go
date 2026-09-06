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
	OpVitalsListPage   = "measurementsListPage"
	OpVitalsDetailPage = "measurementsDetailPage"
)

const vitalsListTitle = "Measurements"

// VitalsHandlers is the vitals pages' contribution to the route table.
func VitalsHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newVitalsLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &vitalsPages{resolve: resolve, patients: patients, tags: tags, links: links, views: VitalsViews{links: links}}

	return httproute.Handlers{
		OpVitalsListPage:   web.WithActor(pages.list),
		OpVitalsDetailPage: web.WithActor(pages.detail),
	}, nil
}

// VitalsViews is the kind's rendering, as internal/records consumes it.
type VitalsViews struct {
	links vitalsLinks
}

var _ recordfamily.Views = VitalsViews{}

func NewVitalsViews() (VitalsViews, error) {
	links, err := newVitalsLinks()
	if err != nil {
		return VitalsViews{}, err
	}

	return VitalsViews{links: links}, nil
}

func (v VitalsViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v VitalsViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.VitalsList(views.VitalsListProps{
		Vitals:     v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Vitals, ""),
		NextHref:   nextHref,
	})
}

func (v VitalsViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.VitalsRow(v.view(record))
}

func (v VitalsViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.VitalsDetail(views.VitalsDetailProps{Vitals: v.view(record)})
}

func (v VitalsViews) Title(record recordfamily.Record) string {
	if title := v.view(record).RecordedAt; title != "" {
		return title
	}

	return vitalsListTitle
}

func (v VitalsViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	vitals := v.view(record)
	fresh := vitals.ID == ""

	formID := ids.RecordForm(kind.Vitals, vitals.ID)

	return views.VitalsForm(views.VitalsFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(vitals),
		CancelHref: v.links.cancelHref(vitals),
		Vitals:     vitals,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v VitalsViews) rows(items []recordfamily.Record) []views.VitalsView {
	rendered := make([]views.VitalsView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v VitalsViews) view(record recordfamily.Record) views.VitalsView {
	entity := clinical.Vitals{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Vitals:
		entity = vitalsDetailEntity(record, *body)
	case *api.VitalsSummary:
		entity = vitalsSummaryEntity(record, *body)
	case *api.VitalsCreate:
		entity.PatientID = body.Patient
	}

	return views.NewVitalsView(entity, v.links.of(entity.ID))
}

func vitalsSummaryEntity(record recordfamily.Record, summary api.VitalsSummary) clinical.Vitals {
	recordedAt, _ := readClinicalInstant(summary.RecordedAt)

	return clinical.Vitals{
		ID:                 record.ID,
		Version:            record.Version,
		RecordedAt:         recordedAt,
		SystolicMmHg:       summary.SystolicMmHg,
		DiastolicMmHg:      summary.DiastolicMmHg,
		HeartRateBpm:       summary.HeartRateBpm,
		RespiratoryRateBpm: summary.RespiratoryRateBpm,
		TemperatureC:       summary.TemperatureC,
		SpO2Pct:            summary.SpO2Pct,
		WeightKg:           summary.WeightKg,
		HeightCm:           summary.HeightCm,
		GlucoseMmolL:       summary.GlucoseMmolL,
		Hba1cPct:           summary.Hba1cPct,
		PainScale:          summary.PainScale,
		UpdatedAt:          readInstant(summary.UpdatedAt),
	}
}

func vitalsDetailEntity(record recordfamily.Record, detail api.Vitals) clinical.Vitals {
	entity := vitalsSummaryEntity(record, detail.VitalsSummary)

	entity.PatientID = detail.Patient
	entity.GlucoseContext = clinical.GlucoseContext(detail.GlucoseContext)
	entity.Device = detail.Device
	entity.PractitionerID = detail.Practitioner
	entity.CreatedAt = readInstant(detail.CreatedAt)

	return entity
}

type vitalsPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    vitalsLinks
	views    VitalsViews
}

func (p *vitalsPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Vitals.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Vitals.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Vitals}
	blank.Body = &api.VitalsCreate{Patient: query.PatientID}

	if err := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); err != nil {
		return err
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, vitalsListTitle, sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *vitalsPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *vitalsPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *vitalsPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Vitals.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Vitals.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if err := attachTagOptions(e.Request.Context(), actor, p.tags, &found); err != nil {
		return err
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Vitals); ok {
		patientID = detail.Patient
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.Title(found), sequence{
		context,
		entry.Views.Detail(found),
		entry.Views.Form(found, nil, ""),
	})
}

func (p *vitalsPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *vitalsPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

type vitalsLinks struct {
	listPage        string
	detailPage      string
	settingsPage    string
	patientsPage    string
	medicationsPage string
	record          string
	collection      string
}

func newVitalsLinks() (vitalsLinks, error) {
	paths, err := routePaths(map[string]string{
		OpVitalsListPage:     "",
		OpVitalsDetailPage:   "",
		OpSettingsPage:       "",
		OpPatientListPage:    "",
		OpMedicationListPage: "",
		api.OpGetRecord:      "",
		api.OpCreateRecord:   "",
	})
	if err != nil {
		return vitalsLinks{}, err
	}

	segment := kind.Vitals.Segment()

	return vitalsLinks{
		listPage:        paths[OpVitalsListPage],
		detailPage:      paths[OpVitalsDetailPage],
		settingsPage:    paths[OpSettingsPage],
		patientsPage:    paths[OpPatientListPage],
		medicationsPage: paths[OpMedicationListPage],
		record:          strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:      strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l vitalsLinks) of(recordID string) views.VitalsLinks {
	if recordID == "" {
		return views.VitalsLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.VitalsLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Vitals, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l vitalsLinks) submitExpression(v views.VitalsView) string {
	if v.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(v.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l vitalsLinks) cancelHref(v views.VitalsView) string {
	if v.Links.Detail != "" {
		return v.Links.Detail
	}

	return l.listPage
}

func (l vitalsLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: vitalsListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
