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
	OpSymptomListPage   = "symptomListPage"
	OpSymptomDetailPage = "symptomDetailPage"
)

const symptomListTitleID = "page.symptomListPage.title"

// SymptomHandlers is the symptom pages' contribution to the route table.
func SymptomHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newSymptomLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &symptomPages{resolve: resolve, patients: patients, tags: tags, links: links, views: SymptomViews{links: links}}

	return httproute.Handlers{
		OpSymptomListPage:   web.WithActor(pages.list),
		OpSymptomDetailPage: web.WithActor(pages.detail),
	}, nil
}

// SymptomViews is the kind's rendering, as internal/records consumes it.
type SymptomViews struct {
	links symptomLinks
}

var _ recordfamily.Views = SymptomViews{}

func NewSymptomViews() (SymptomViews, error) {
	links, err := newSymptomLinks()
	if err != nil {
		return SymptomViews{}, err
	}

	return SymptomViews{links: links}, nil
}

func (v SymptomViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v SymptomViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.SymptomList(views.SymptomListProps{
		Symptoms:   v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Symptom, ""),
		NextHref:   nextHref,
	})
}

func (v SymptomViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.SymptomRow(v.view(record))
}

func (v SymptomViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.SymptomDetail(views.SymptomDetailProps{Symptom: v.view(record)})
}

func (v SymptomViews) Title(record recordfamily.Record) string { return v.view(record).Name }

func (v SymptomViews) detailWithMedications(record recordfamily.Record, medications views.MedicationLinksEditorProps) recordfamily.Renderer {
	return views.SymptomDetail(views.SymptomDetailProps{Symptom: v.view(record), Medications: medications})
}

func (v SymptomViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	symptom := v.view(record)
	fresh := symptom.ID == ""

	formID := ids.RecordForm(kind.Symptom, symptom.ID)

	return views.SymptomForm(views.SymptomFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(symptom),
		CancelHref: v.links.cancelHref(symptom),
		Symptom:    symptom,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v SymptomViews) rows(items []recordfamily.Record) []views.SymptomView {
	rendered := make([]views.SymptomView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v SymptomViews) view(record recordfamily.Record) views.SymptomView {
	symptom := clinical.Symptom{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Symptom:
		symptom = symptomDetailEntity(record, *body)
	case *api.SymptomSummary:
		symptom = symptomSummaryEntity(record, *body)
	case *api.SymptomCreate:
		symptom.PatientID = body.Patient
	}

	return views.NewSymptomView(symptom, v.links.of(symptom.ID))
}

func symptomSummaryEntity(record recordfamily.Record, summary api.SymptomSummary) clinical.Symptom {
	occurredAt, _ := readClinicalInstant(summary.OccurredAt)
	lastOccurredAt, _ := readClinicalInstant(summary.LastOccurredAt)

	return clinical.Symptom{
		ID:             record.ID,
		Version:        record.Version,
		Name:           summary.Name,
		Severity:       clinical.Severity(summary.Severity),
		OccurredAt:     occurredAt,
		EpisodeCount:   summary.EpisodeCount,
		LastOccurredAt: lastOccurredAt,
		Status:         clinical.ConditionStatus(summary.Status),
		UpdatedAt:      readInstant(summary.UpdatedAt),
	}
}

func symptomDetailEntity(record recordfamily.Record, detail api.Symptom) clinical.Symptom {
	symptom := symptomSummaryEntity(record, detail.SymptomSummary)

	resolvedAt, _ := readClinicalInstantPtr(detail.ResolvedAt)

	symptom.PatientID = detail.Patient
	symptom.Category = clinical.SymptomCategory(detail.Category)
	symptom.DurationMinutes = detail.DurationMinutes
	symptom.PainScale = detail.PainScale
	symptom.BodySite = detail.BodySite
	symptom.Triggers = detail.Triggers
	symptom.ReliefMethods = detail.ReliefMethods
	symptom.Impact = clinical.SymptomImpact(detail.Impact)
	symptom.ResolvedAt = resolvedAt
	symptom.IsChronic = detail.IsChronic

	return symptom
}

func readClinicalInstant(raw string) (clinical.Instant, error) {
	if raw == "" {
		return clinical.Instant{}, nil
	}

	var parsed clinical.Instant
	if err := parsed.UnmarshalText([]byte(raw)); err != nil {
		return clinical.Instant{}, err
	}

	return parsed, nil
}

func readClinicalInstantPtr(raw *string) (clinical.Instant, error) {
	if raw == nil {
		return clinical.Instant{}, nil
	}

	return readClinicalInstant(*raw)
}

type symptomPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    symptomLinks
	views    SymptomViews
}

func (p *symptomPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Symptom.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Symptom.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Symptom}
	blank.Body = &api.SymptomCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), symptomListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *symptomPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *symptomPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *symptomPages) detail(e *core.RequestEvent, actor access.Actor) error {
	web.Localize(e)

	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Symptom.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Symptom.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID := ""
	var treatedBy, causedBy []string
	if detail, ok := found.Body.(*api.Symptom); ok {
		patientID, treatedBy, causedBy = detail.Patient, detail.TreatedByMedications, detail.CausedByMedications
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	editor, err := p.medicationsEditor(e.Request.Context(), handler, actor, patientID, found, treatedBy, causedBy)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.view(found).Name, sequence{
		context,
		p.views.detailWithMedications(found, editor),
		entry.Views.Form(found, nil, ""),
	})
}

// medicationsEditor builds FR-055's editor for a symptom's two distinct
// medication roles (FR-032): a medication may treat it, cause it, or both,
// independently.
func (p *symptomPages) medicationsEditor(
	ctx context.Context, handler *recordfamily.Handler, actor access.Actor, patientID string,
	found recordfamily.Record, treatedBy, causedBy []string,
) (views.MedicationLinksEditorProps, error) {
	options, err := patientMedicationOptions(ctx, handler, actor, patientID)
	if err != nil {
		return views.MedicationLinksEditorProps{}, err
	}

	treatedRole := medicationLinkRole(api.MemberSymptomTreatedByMedications, i18n.T(ctx, "field.symptom.treated_by_role"), treatedBy, options, p.links.medicationHref)
	causedRole := medicationLinkRole(api.MemberSymptomCausedByMedications, i18n.T(ctx, "field.symptom.caused_by_role"), causedBy, options, p.links.medicationHref)

	return views.MedicationLinksEditorProps{
		ID:         ids.RecordDetail(kind.Symptom, found.ID) + "-" + kind.Medication.Collection(),
		Title:      i18n.T(ctx, "medication_links.title"),
		RecordHref: p.links.of(found.ID).Record,
		Options:    options,
		Roles:      []views.MedicationLinkRole{treatedRole, causedRole},
	}, nil
}

func (p *symptomPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *symptomPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

type symptomLinks struct {
	listPage             string
	detailPage           string
	settingsPage         string
	patientsPage         string
	medicationsPage      string
	medicationDetailPage string
	record               string
	collection           string
}

func newSymptomLinks() (symptomLinks, error) {
	paths, err := routePaths(map[string]string{
		OpSymptomListPage:      "",
		OpSymptomDetailPage:    "",
		OpSettingsPage:         "",
		OpPatientListPage:      "",
		OpMedicationListPage:   "",
		OpMedicationDetailPage: "",
		api.OpGetRecord:        "",
		api.OpCreateRecord:     "",
	})
	if err != nil {
		return symptomLinks{}, err
	}

	segment := kind.Symptom.Segment()

	return symptomLinks{
		listPage:             paths[OpSymptomListPage],
		detailPage:           paths[OpSymptomDetailPage],
		settingsPage:         paths[OpSettingsPage],
		patientsPage:         paths[OpPatientListPage],
		medicationsPage:      paths[OpMedicationListPage],
		medicationDetailPage: paths[OpMedicationDetailPage],
		record:               strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:           strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l symptomLinks) medicationHref(id string) string {
	if id == "" {
		return ""
	}

	return strings.ReplaceAll(l.medicationDetailPage, "{"+api.PathID+"}", id)
}

func (l symptomLinks) of(recordID string) views.SymptomLinks {
	if recordID == "" {
		return views.SymptomLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.SymptomLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Symptom, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l symptomLinks) submitExpression(symptom views.SymptomView) string {
	if symptom.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(symptom.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l symptomLinks) cancelHref(symptom views.SymptomView) string {
	if symptom.Links.Detail != "" {
		return symptom.Links.Detail
	}

	return l.listPage
}

func (l symptomLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		// Nav labels are not yet resolved through i18n.T by shell/nav.templ
		// (T020); left in English here until that seam lands (US1 cross-slice
		// note in this task's report).
		{Label: "Symptoms", Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
