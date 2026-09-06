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
	OpTreatmentListPage   = "treatmentListPage"
	OpTreatmentDetailPage = "treatmentDetailPage"
)

const treatmentListTitleID = "page.treatments.title"

// TreatmentHandlers is the treatment pages' contribution to the route table,
// mirroring medications.go's Handlers end to end (T078).
func TreatmentHandlers(
	resolve api.Resolve, patients api.PatientResolve, courseMedications api.CourseMedicationResolve,
	tags api.TagResolve,
) (httproute.Handlers, error) {
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

	pages := &treatmentPages{
		resolve: resolve, patients: patients, courseMedications: courseMedications, tags: tags,
		links: links, views: TreatmentViews{links: links},
	}

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

func (v TreatmentViews) Title(record recordfamily.Record) string { return v.view(record).Name }

func (v TreatmentViews) detailWithReferenceCount(record recordfamily.Record, referenceCount int) recordfamily.Renderer {
	return views.TreatmentDetail(views.TreatmentDetailProps{Treatment: v.view(record), ReferenceCount: referenceCount})
}

func (v TreatmentViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	treatment := v.view(record)
	fresh := treatment.ID == ""

	formID := ids.RecordForm(kind.Treatment, treatment.ID)

	return views.TreatmentForm(views.TreatmentFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(treatment),
		CancelHref: v.links.cancelHref(treatment),
		Treatment:  treatment,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
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
	resolve           api.Resolve
	patients          api.PatientResolve
	tags              api.TagResolve
	courseMedications api.CourseMedicationResolve
	links             treatmentLinks
	views             TreatmentViews
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

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), treatmentListTitleID), sequence{
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
	web.Localize(e)

	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Treatment.Segment())
	if err != nil {
		return err
	}

	treatmentID := e.Request.PathValue(api.PathID)

	found, err := handler.Get(e.Request.Context(), actor, kind.Treatment.Segment(), treatmentID)
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID, conditionID := "", ""
	if detail, ok := found.Body.(*api.Treatment); ok {
		patientID = detail.Patient
		conditionID = detail.Condition
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	linked, err := p.linkedCondition(e.Request.Context(), handler, actor, conditionID)
	if err != nil {
		return err
	}

	courseMedications, options, err := p.courseMedicationRows(e.Request.Context(), handler, actor, patientID, treatmentID, found.Version)
	if err != nil {
		return err
	}

	sectionID := ids.RecordDetail(kind.Treatment, treatmentID) + "-" + kind.Medication.Collection()

	return p.render(e, actor, p.views.view(found).Name, sequence{
		context,
		p.views.detailWithReferenceCount(found, len(courseMedications)),
		views.LinkedRecords(ids.RecordDetail(kind.Treatment, treatmentID)+"-links", i18n.T(e.Request.Context(), "linked_records.title"), linked),
		views.CourseMedications(sectionID, i18n.T(e.Request.Context(), "course_medications.title"), courseMedications, views.CourseMedicationFormProps{
			ID:         sectionID + "-form",
			UpsertBase: p.links.courseMedicationsBase(treatmentID),
			Etag:       found.Version,
			Options:    options,
		}),
		entry.Views.Form(found, nil, ""),
	})
}

// linkedCondition is the treatment's own `condition` relation rendered as an
// openable link rather than the bare id Entries() shows in the <dl> (FR-059).
// An empty conditionID or one the handler cannot read (deleted, or not this
// actor's) renders no items at all — a broken link is worse than nothing.
func (p *treatmentPages) linkedCondition(
	ctx context.Context, handler *recordfamily.Handler, actor access.Actor, conditionID string,
) ([]views.LinkedRecordItem, error) {
	if conditionID == "" {
		return nil, nil
	}

	found, err := handler.Get(ctx, actor, kind.Condition.Segment(), conditionID)
	if err != nil {
		return nil, nil //nolint:nilerr // a condition this actor can no longer reach renders no link, not a page error
	}

	condition, ok := found.Body.(*api.Condition)
	if !ok || condition.Diagnosis == "" {
		return nil, nil
	}

	return []views.LinkedRecordItem{
		{Kind: string(kind.Condition), Summary: condition.Diagnosis, Href: p.links.conditionHref(conditionID)},
	}, nil
}

// courseMedicationRows resolves FR-060/FR-061's attached medications, each
// with its effective values and their provenance, plus the patient's own
// medications for the attach form's picker.
func (p *treatmentPages) courseMedicationRows(
	ctx context.Context, handler *recordfamily.Handler, actor access.Actor, patientID, treatmentID, treatmentEtag string,
) ([]views.CourseMedicationRow, []views.MedicationLinkOption, error) {
	options, err := patientMedicationOptions(ctx, handler, actor, patientID)
	if err != nil {
		return nil, nil, err
	}

	if p.courseMedications == nil {
		return nil, options, nil
	}

	service, err := p.courseMedications()
	if err != nil {
		return nil, nil, err
	}

	items, err := service.List(ctx, actor, treatmentID)
	if err != nil {
		return nil, nil, err
	}

	rows := make([]views.CourseMedicationRow, 0, len(items))

	for _, item := range items {
		itemHref := p.links.courseMedicationItemHref(treatmentID, item.Medication.ID)

		rows = append(rows, views.CourseMedicationRow{
			MedicationID:   item.Medication.ID,
			MedicationName: item.Medication.Name,
			MedicationHref: p.links.medicationHref(item.Medication.ID),
			RemoveOn:       views.CourseMedicationRemoveExpr(itemHref, treatmentEtag),
			Dosage:         courseMedicationEffective(item.Effective.Dosage),
			Frequency:      courseMedicationEffective(item.Effective.Frequency),
			Duration:       courseMedicationEffective(item.Effective.Duration),
			Timing:         courseMedicationEffective(item.Effective.Timing),
			Prescriber:     courseMedicationEffective(item.Effective.Prescriber),
			Pharmacy:       courseMedicationEffective(item.Effective.Pharmacy),
			StartedOn:      courseMedicationEffective(item.Effective.StartedOn),
			EndedOn:        courseMedicationEffective(item.Effective.EndedOn),
		})
	}

	return rows, options, nil
}

func courseMedicationEffective(effective clinical.Effective) views.CourseMedicationEffectiveView {
	return views.CourseMedicationEffectiveView{
		Value:  effectiveDisplayValue(effective.Value),
		Source: string(effective.Source),
	}
}

// effectiveDisplayValue renders whatever clinical.Effective carried as a
// string: a domain.Date's own String() for a date field, fmt.Sprint for
// everything else (every other effective field is already a string).
func effectiveDisplayValue(value any) string {
	if date, ok := value.(domain.Date); ok {
		return date.String()
	}

	if value == nil {
		return ""
	}

	return fmt.Sprint(value)
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
		NavState{SignedIn: true, Nav: p.links.nav(localizeCtx(e), e.Request.URL.Path), Switcher: switcher}, main)
}

type treatmentLinks struct {
	listPage             string
	medicationsPage      string
	detailPage           string
	conditionDetailPage  string
	medicationDetailPage string
	settingsPage         string
	patientsPage         string
	record               string
	collection           string
	courseMedications    string
}

func newTreatmentLinks() (treatmentLinks, error) {
	paths, err := routePaths(map[string]string{
		OpTreatmentListPage:         "",
		OpTreatmentDetailPage:       "",
		OpConditionDetailPage:       "",
		OpMedicationDetailPage:      "",
		OpSettingsPage:              "",
		OpMedicationListPage:        "",
		OpPatientListPage:           "",
		api.OpGetRecord:             "",
		api.OpCreateRecord:          "",
		api.OpListCourseMedications: "",
	})
	if err != nil {
		return treatmentLinks{}, err
	}

	segment := kind.Treatment.Segment()

	return treatmentLinks{
		listPage:             paths[OpTreatmentListPage],
		medicationsPage:      paths[OpMedicationListPage],
		detailPage:           paths[OpTreatmentDetailPage],
		conditionDetailPage:  paths[OpConditionDetailPage],
		medicationDetailPage: paths[OpMedicationDetailPage],
		settingsPage:         paths[OpSettingsPage],
		patientsPage:         paths[OpPatientListPage],
		record:               strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:           strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
		courseMedications:    paths[api.OpListCourseMedications],
	}, nil
}

// courseMedicationsBase and courseMedicationItemHref build
// contracts/treatment-medications.md's own nested addresses for one
// treatment: the join's list route, not one of the generic six.
func (l treatmentLinks) courseMedicationsBase(treatmentID string) string {
	return strings.ReplaceAll(l.courseMedications, "{"+api.PathID+"}", treatmentID)
}

func (l treatmentLinks) courseMedicationItemHref(treatmentID, medicationID string) string {
	return l.courseMedicationsBase(treatmentID) + "/" + medicationID
}

// conditionHref and medicationHref build another kind's detail page address
// from the id this treatment names — links.templ's Href, so "openable link"
// is a real page rather than a bare id (FR-059).
func (l treatmentLinks) conditionHref(id string) string {
	if id == "" {
		return ""
	}

	return strings.ReplaceAll(l.conditionDetailPage, "{"+api.PathID+"}", id)
}

func (l treatmentLinks) medicationHref(id string) string {
	if id == "" {
		return ""
	}

	return strings.ReplaceAll(l.medicationDetailPage, "{"+api.PathID+"}", id)
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

func (l treatmentLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: i18n.T(ctx, treatmentListTitleID), Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
