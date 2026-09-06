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
	OpAllergyListPage   = "allergyListPage"
	OpAllergyDetailPage = "allergyDetailPage"
)

// allergyListTitleID is a message id (D-06), resolved at render time.
const allergyListTitleID = "page.allergies.title"

// AllergyHandlers is the allergy pages' contribution to the route table.
func AllergyHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newAllergyLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &allergyPages{resolve: resolve, patients: patients, tags: tags, links: links, views: AllergyViews{links: links}}

	return httproute.Handlers{
		OpAllergyListPage:   web.WithActor(pages.list),
		OpAllergyDetailPage: web.WithActor(pages.detail),
	}, nil
}

// AllergyViews is the kind's rendering, as internal/records consumes it.
type AllergyViews struct {
	links allergyLinks
}

var _ recordfamily.Views = AllergyViews{}

func NewAllergyViews() (AllergyViews, error) {
	links, err := newAllergyLinks()
	if err != nil {
		return AllergyViews{}, err
	}

	return AllergyViews{links: links}, nil
}

func (v AllergyViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v AllergyViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.AllergyList(views.AllergyListProps{
		Allergies:  v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Allergy, ""),
		NextHref:   nextHref,
	})
}

func (v AllergyViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.AllergyRow(v.view(record))
}

func (v AllergyViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.AllergyDetail(views.AllergyDetailProps{Allergy: v.view(record)})
}

func (v AllergyViews) Title(record recordfamily.Record) string { return v.view(record).Allergen }

func (v AllergyViews) detailWithMedications(record recordfamily.Record, medications views.MedicationLinksEditorProps) recordfamily.Renderer {
	return views.AllergyDetail(views.AllergyDetailProps{Allergy: v.view(record), Medications: medications})
}

func (v AllergyViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	allergy := v.view(record)
	fresh := allergy.ID == ""

	formID := ids.RecordForm(kind.Allergy, allergy.ID)

	return views.AllergyForm(views.AllergyFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(allergy),
		CancelHref: v.cancelHref(allergy),
		Allergy:    allergy,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v AllergyViews) rows(items []recordfamily.Record) []views.AllergyView {
	rendered := make([]views.AllergyView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v AllergyViews) view(record recordfamily.Record) views.AllergyView {
	allergy := clinical.Allergy{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Allergy:
		allergy = allergyDetailEntity(record, *body)
	case *api.AllergySummary:
		allergy = allergySummaryEntity(record, *body)
	case *api.AllergyCreate:
		allergy.PatientID = body.Patient
	}

	return views.NewAllergyView(allergy, v.links.of(allergy.ID))
}

func allergySummaryEntity(record recordfamily.Record, summary api.AllergySummary) clinical.Allergy {
	return clinical.Allergy{
		ID:        record.ID,
		Version:   record.Version,
		Allergen:  summary.Allergen,
		Severity:  clinical.Severity(summary.Severity),
		Status:    clinical.ConditionStatus(summary.Status),
		OnsetOn:   readDate(summary.OnsetOn),
		UpdatedAt: readInstant(summary.UpdatedAt),
	}
}

func allergyDetailEntity(record recordfamily.Record, detail api.Allergy) clinical.Allergy {
	allergy := allergySummaryEntity(record, detail.AllergySummary)

	allergy.PatientID = detail.Patient
	allergy.Reaction = detail.Reaction
	allergy.Notes = detail.Notes
	allergy.CreatedAt = readInstant(detail.CreatedAt)

	return allergy
}

type allergyPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    allergyLinks
	views    AllergyViews
}

func (p *allergyPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Allergy.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Allergy.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Allergy}
	blank.Body = &api.AllergyCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), allergyListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *allergyPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *allergyPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *allergyPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Allergy.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Allergy.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	patientID, medications := "", []string(nil)
	if detail, ok := found.Body.(*api.Allergy); ok {
		patientID, medications = detail.Patient, detail.Medications
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	editor, err := p.medicationsEditor(e.Request.Context(), handler, actor, patientID, found, medications)
	if err != nil {
		return err
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	return p.render(e, actor, p.views.view(found).Allergen, sequence{
		context,
		p.views.detailWithMedications(found, editor),
		entry.Views.Form(found, nil, ""),
	})
}

// medicationsEditor builds FR-055's editor: the patient's own medications as
// the picker, and this allergy's own `medications` field as the one role it
// writes.
func (p *allergyPages) medicationsEditor(
	ctx context.Context, handler *recordfamily.Handler, actor access.Actor, patientID string,
	found recordfamily.Record, medicationIDs []string,
) (views.MedicationLinksEditorProps, error) {
	options, err := patientMedicationOptions(ctx, handler, actor, patientID)
	if err != nil {
		return views.MedicationLinksEditorProps{}, err
	}

	role := medicationLinkRole(api.MemberMedications, "", medicationIDs, options, p.links.medicationHref)

	return views.MedicationLinksEditorProps{
		ID:         ids.RecordDetail(kind.Allergy, found.ID) + "-" + kind.Medication.Collection(),
		Title:      i18n.T(ctx, "nav.medications"),
		RecordHref: p.links.of(found.ID).Record,
		Options:    options,
		Roles:      []views.MedicationLinkRole{role},
	}, nil
}

func (p *allergyPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *allergyPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.Context(), e.Request.URL.Path), Switcher: switcher}, main)
}

type allergyLinks struct {
	listPage             string
	detailPage           string
	settingsPage         string
	patientsPage         string
	medicationsPage      string
	medicationDetailPage string
	record               string
	collection           string
}

func newAllergyLinks() (allergyLinks, error) {
	paths, err := routePaths(map[string]string{
		OpAllergyListPage:      "",
		OpAllergyDetailPage:    "",
		OpSettingsPage:         "",
		OpPatientListPage:      "",
		OpMedicationListPage:   "",
		OpMedicationDetailPage: "",
		api.OpGetRecord:        "",
		api.OpCreateRecord:     "",
	})
	if err != nil {
		return allergyLinks{}, err
	}

	segment := kind.Allergy.Segment()

	return allergyLinks{
		listPage:             paths[OpAllergyListPage],
		detailPage:           paths[OpAllergyDetailPage],
		settingsPage:         paths[OpSettingsPage],
		patientsPage:         paths[OpPatientListPage],
		medicationsPage:      paths[OpMedicationListPage],
		medicationDetailPage: paths[OpMedicationDetailPage],
		record:               strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:           strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l allergyLinks) medicationHref(id string) string {
	if id == "" {
		return ""
	}

	return strings.ReplaceAll(l.medicationDetailPage, "{"+api.PathID+"}", id)
}

func (l allergyLinks) of(recordID string) views.AllergyLinks {
	if recordID == "" {
		return views.AllergyLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.AllergyLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Allergy, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l allergyLinks) submitExpression(allergy views.AllergyView) string {
	if allergy.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(allergy.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l allergyLinks) cancelHref(allergy views.AllergyView) string {
	if allergy.Links.Detail != "" {
		return allergy.Links.Detail
	}

	return l.listPage
}

func (v AllergyViews) cancelHref(allergy views.AllergyView) string {
	return v.links.cancelHref(allergy)
}

// nav is FR-050's fixed pair, not a per-kind entry: every signed-in page
// offers the route back to the medication list and to settings, the same
// nav medications.go itself renders.
func (l allergyLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
