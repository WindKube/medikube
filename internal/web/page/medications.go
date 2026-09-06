package page

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/i18n"
	recordfamily "medikube/internal/records"
	"medikube/internal/store/link"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
	views "medikube/internal/web/views/records"
	"medikube/internal/web/views/shell"
)

// The operation ids of contracts/pages.md's P4 and P5, spelled as
// internal/httproute declares them.
const (
	OpMedicationListPage   = "medicationListPage"
	OpMedicationDetailPage = "medicationDetailPage"
)

// The two page titles of contracts/pages.md, without the product suffix, which
// shell.Title adds. P5's is the record's own name.
const medicationListTitle = "Medications"

// medicationListTitle stays English for shell.NavLink.Label until T020 resolves nav labels.
const medicationListTitleID = "page.medications.title"

// Handlers is the record pages' contribution to the route table.
//
// A page that reached the router without a Landmark or a SmokeURL cannot be
// registered at all — httproute.Handle panics on either — so wiring these two
// is also what takes them off the 501 stub list.
func Handlers(
	resolve api.Resolve, patients api.PatientResolve, references api.ReferencesResolve, tags api.TagResolve,
) (httproute.Handlers, error) {
	links, err := newMedicationLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &medicationPages{
		resolve: resolve, patients: patients, references: references, tags: tags,
		links: links, views: MedicationViews{links: links},
	}

	table := httproute.Handlers{
		OpMedicationListPage:   web.WithActor(pages.list),
		OpMedicationDetailPage: web.WithActor(pages.detail),
	}

	allergies, err := AllergyHandlers(resolve, patients, tags)
	if err != nil {
		return nil, err
	}

	conditions, err := ConditionHandlers(resolve, patients, tags)
	if err != nil {
		return nil, err
	}

	emergencyContacts, err := EmergencyContactHandlers(resolve, patients, tags)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, allergies)
	maps.Copy(table, conditions)
	maps.Copy(table, emergencyContacts)

	return table, nil
}

// MedicationViews is the kind's rendering, as internal/records consumes it.
//
// It lives in the page layer and not beside the components because it is the
// one place that knows both halves: the wire DTO internal/web/api minted, and
// the URLs of the routes this build serves. A component may spell neither — a
// view that built a path would be the fourth spelling of a kind's segment and
// the one nothing checks (research D-05).
type MedicationViews struct {
	links medicationLinks
}

var _ recordfamily.Views = MedicationViews{}

// NewMedicationViews reads its three address templates out of the route
// inventory, so the link on a row is by construction the route the router
// serves.
func NewMedicationViews() (MedicationViews, error) {
	links, err := newMedicationLinks()
	if err != nil {
		return MedicationViews{}, err
	}

	return MedicationViews{links: links}, nil
}

// List renders one page of rows inside contracts/pages.md P4's landmark, with
// the empty state inside that landmark rather than instead of it (FR-029).
//
// It renders no paging link because records.Views is handed a page of records
// and not the request that asked for it. The page handler, which has both,
// calls ListOfPage.
func (v MedicationViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

// ListOfPage is List plus the address of the next page. A pager that is never
// handed a link renders as an empty landmark on every list, and a list of more
// than one page silently stops at its first (FR-023).
func (v MedicationViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.MedicationList(views.MedicationListProps{
		Medications: v.rows(page.Items),
		CreateHref:  "#" + ids.RecordForm(kind.Medication, ""),
		NextHref:    nextHref,
	})
}

// Row is the element contracts/streams.md patches by id.
func (v MedicationViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.MedicationRow(v.view(record))
}

// Detail is contracts/pages.md P5's landmark, with the delete confirmation
// rendered inside it (FR-028).
func (v MedicationViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.MedicationDetail(views.MedicationDetailProps{Medication: v.view(record)})
}

func (v MedicationViews) Title(record recordfamily.Record) string { return v.view(record).Name }

// Form is the create form and the edit form, re-rendered from the submitted
// values plus the field errors and never cleared (FR-027).
func (v MedicationViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	medication := v.view(record)
	fresh := medication.ID == ""

	formID := ids.RecordForm(kind.Medication, medication.ID)

	return views.MedicationForm(views.MedicationFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(medication),
		CancelHref: v.cancelHref(medication),
		Medication: medication,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v MedicationViews) rows(items []recordfamily.Record) []views.MedicationView {
	rendered := make([]views.MedicationView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

// view is the DTO-to-page mapping. It reads the wire shape rather than the
// entity because that is what the generic handler carries: the kind's service
// answered in its own published shape and nothing between here and there is
// allowed to know which type it is.
func (v MedicationViews) view(record recordfamily.Record) views.MedicationView {
	medication := clinical.Medication{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Medication:
		medication = detailEntity(record, *body)
	case *api.MedicationSummary:
		medication = summaryEntity(record, *body)
	case *api.MedicationCreate:
		// The blank create form (FR-025): nothing recorded yet but the
		// patient it will be filed against, fixed at render time.
		medication.PatientID = body.Patient
	}

	return views.NewMedicationView(medication, v.links.of(medication.ID))
}

func summaryEntity(record recordfamily.Record, summary api.MedicationSummary) clinical.Medication {
	return clinical.Medication{
		ID:        record.ID,
		Version:   record.Version,
		Name:      summary.Name,
		Dosage:    summary.Dosage,
		Frequency: summary.Frequency,
		Status:    clinical.TherapyStatus(summary.Status),
		StartedOn: readDate(summary.StartedOn),
		UpdatedAt: readInstant(summary.UpdatedAt),
	}
}

func detailEntity(record recordfamily.Record, detail api.Medication) clinical.Medication {
	medication := summaryEntity(record, detail.MedicationSummary)

	medication.PatientID = detail.Patient
	medication.AlternativeName = detail.AlternativeName
	medication.Type = clinical.MedicationType(detail.Type)
	medication.Route = clinical.MedicationRoute(detail.Route)
	medication.Indication = detail.Indication
	medication.EndedOn = readDate(detail.EndedOn)
	medication.SideEffects = detail.SideEffects
	medication.Notes = detail.Notes
	medication.CreatedAt = readInstant(detail.CreatedAt)

	return medication
}

type medicationPages struct {
	resolve    api.Resolve
	patients   api.PatientResolve
	references api.ReferencesResolve
	tags       api.TagResolve
	links      medicationLinks
	views      MedicationViews
}

// list renders P4. The rows come through the same generic handler the API
// serves, so a page and its JSON cannot disagree about what the account owns.
//
// A bare /medications (no `?patient=`) 303s to the person in view, or to
// /patients when there is none (FR-016, contracts/active-patient.md) — the
// page layer's own resolution, never a fallback inside the API's 400
// `patient_required`.
func (p *medicationPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Medication.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Medication.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Medication}
	blank.Body = &api.MedicationCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), medicationListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

// redirectToActivePatient is FR-016's page-layer fallback: the person in
// view, resolved and auto-selected exactly as getMe resolves it (FR-018), or
// /patients when there is nobody to redirect to.
func (p *medicationPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

// patientContext renders FR-019's naming for the list and the form both:
// which patient's medications this screen is for.
func (p *medicationPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

// nextPageHref is this same list one page further on: the address that was
// asked for, carrying the cursor the store just minted.
//
// There is no companion previous link, and inventing one would be a lie. The
// cursor is a keyset boundary and the store mints exactly one direction of it
// (contracts/records.md), so a page reached by following Next has no recorded
// way back; a control labelled Previous that returned to the first page would
// be wrong on every page after the second.
func nextPageHref(e *core.RequestEvent, listing domain.Page[recordfamily.Record]) string {
	if listing.NextCursor == nil {
		return ""
	}

	next := *e.Request.URL
	query := next.Query()
	query.Set(web.ParamCursor, *listing.NextCursor)
	next.RawQuery = query.Encode()

	return next.RequestURI()
}

// detail renders P5. A record belonging to somebody else is a 404 here for the
// same reason it is one through the API: the existence of an identifier is
// itself a disclosure (FR-033).
func (p *medicationPages) detail(e *core.RequestEvent, actor access.Actor) error {
	web.Localize(e)

	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Medication.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Medication.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Medication); ok {
		patientID = detail.Patient
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	links, total, err := p.backrelatedLinks(e.Request.Context(), actor, handler, found.ID)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.view(found).Name, sequence{
		context,
		views.MedicationDetail(views.MedicationDetailProps{Medication: p.views.view(found), Links: links, ReferenceCount: total}),
		entry.Views.Form(found, nil, ""),
	})
}

// backrelatedLinks is FR-055's other end (medication's own page) plus FR-006's
// reference count: every allergy, condition, injury and symptom naming this
// medication, and every treatment it is attached to via treatment_medications,
// each rendered as an openable, removable link. A reference this actor can no
// longer reach (deleted, or not this actor's) is dropped rather than shown
// broken — the same rule linkedCondition follows.
func (p *medicationPages) backrelatedLinks(
	ctx context.Context, actor access.Actor, handler *recordfamily.Handler, medicationID string,
) (views.RemovableLinksProps, int, error) {
	if p.references == nil {
		return views.RemovableLinksProps{}, 0, nil
	}

	backrel, err := p.references()
	if err != nil {
		return views.RemovableLinksProps{}, 0, err
	}

	refs, err := backrel.Medications(ctx, medicationID)
	if err != nil {
		return views.RemovableLinksProps{}, 0, err
	}

	joins, err := backrel.TreatmentMedicationTreatments(ctx, medicationID)
	if err != nil {
		return views.RemovableLinksProps{}, 0, err
	}

	items := make([]views.RemovableLink, 0, len(refs)+len(joins))
	seen := make(map[string]bool, len(refs))

	for _, ref := range refs {
		key := string(ref.Kind) + ":" + ref.ID
		if seen[key] {
			continue
		}
		seen[key] = true

		item, ok, err := p.resolveBackrelation(ctx, actor, handler, ref, medicationID)
		if err != nil {
			return views.RemovableLinksProps{}, 0, err
		}

		if ok {
			items = append(items, item)
		}
	}

	for _, ref := range joins {
		item, ok, err := p.resolveCourseMedicationBackrelation(ctx, actor, handler, ref.ID, medicationID)
		if err != nil {
			return views.RemovableLinksProps{}, 0, err
		}

		if ok {
			items = append(items, item)
		}
	}

	props := views.RemovableLinksProps{
		ID:    ids.RecordDetail(kind.Medication, medicationID) + "-links",
		Title: i18n.T(ctx, "linked_records.title"),
		Items: items,
	}

	return props, len(refs) + len(joins), nil
}

// resolveBackrelation hydrates one allergy/condition/injury/symptom ref into
// a removable link, reading whichever field(s) it names this medication in
// off the record's own current body so removal can PATCH the exact set minus
// this one id.
func (p *medicationPages) resolveBackrelation(
	ctx context.Context, actor access.Actor, handler *recordfamily.Handler, ref link.Ref, medicationID string,
) (views.RemovableLink, bool, error) {
	found, err := handler.Get(ctx, actor, ref.Kind.Segment(), ref.ID)
	if err != nil {
		return views.RemovableLink{}, false, nil //nolint:nilerr // gone or not this actor's: render nothing, not a page error
	}

	switch body := found.Body.(type) {
	case *api.Allergy:
		return views.RemovableLink{
			Kind: string(ref.Kind), Summary: body.Allergen, Href: p.links.allergyHref(ref.ID),
			RemoveOn: views.MedicationRemoveExpr(p.links.allergyRecordHref(ref.ID), found.Version,
				api.MemberMedications, body.Medications, medicationID),
		}, true, nil

	case *api.Condition:
		return views.RemovableLink{
			Kind: string(ref.Kind), Summary: body.Diagnosis, Href: p.links.conditionHref(ref.ID),
			RemoveOn: views.MedicationRemoveExpr(p.links.conditionRecordHref(ref.ID), found.Version,
				api.MemberMedications, body.Medications, medicationID),
		}, true, nil

	case *api.Injury:
		return views.RemovableLink{
			Kind: string(ref.Kind), Summary: body.Name, Href: p.links.injuryHref(ref.ID),
			RemoveOn: views.MedicationRemoveExpr(p.links.injuryRecordHref(ref.ID), found.Version,
				api.InjuryMemberMedications, body.Medications, medicationID),
		}, true, nil

	case *api.Symptom:
		return views.RemovableLink{
			Kind: string(ref.Kind), Summary: body.Name, Href: p.links.symptomHref(ref.ID),
			RemoveOn: views.SymptomMedicationRemoveExpr(p.links.symptomRecordHref(ref.ID), found.Version,
				api.MemberSymptomTreatedByMedications, api.MemberSymptomCausedByMedications,
				body.TreatedByMedications, body.CausedByMedications, medicationID),
		}, true, nil

	default:
		return views.RemovableLink{}, false, nil
	}
}

// resolveCourseMedicationBackrelation hydrates one treatment_medications join
// into a removable link: the join row is what removal deletes, not a field of
// the treatment itself, so RemoveOn is CourseMedicationRemoveExpr's DELETE
// rather than a PATCH of the treatment's own body.
func (p *medicationPages) resolveCourseMedicationBackrelation(
	ctx context.Context, actor access.Actor, handler *recordfamily.Handler, treatmentID, medicationID string,
) (views.RemovableLink, bool, error) {
	found, err := handler.Get(ctx, actor, kind.Treatment.Segment(), treatmentID)
	if err != nil {
		return views.RemovableLink{}, false, nil //nolint:nilerr // gone or not this actor's: render nothing, not a page error
	}

	detail, ok := found.Body.(*api.Treatment)
	if !ok {
		return views.RemovableLink{}, false, nil
	}

	itemHref := p.links.courseMedicationItemHref(treatmentID, medicationID)

	return views.RemovableLink{
		Kind:     string(kind.Treatment),
		Summary:  detail.Name,
		Href:     p.links.treatmentHref(treatmentID),
		RemoveOn: views.CourseMedicationRemoveExpr(itemHref, found.Version),
	}, true, nil
}

// session refuses a page that needs one to a caller who has none.
//
// It is 403 and not 404: contracts/pages.md's E2 renders the sign-in prompt,
// because the existence of /medications is not information about anybody. The
// router does not do this for pages — httproute.Bind binds apis.RequireAuth
// only to the non-page routes — precisely so the decision stays here, where the
// full shell can be rendered around it.
func (p *medicationPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *medicationPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(localizeCtx(e), e.Request.URL.Path), Switcher: switcher}, main)
}

// pageCacheControl keeps a rendered medication list out of every shared cache
// and off disk, for the same reason the JSON is served that way.
const pageCacheControl = "private, no-store"

// sequence renders several components into one. It exists so that a page whose
// main landmark is a region plus the form that adds to it needs no wrapper
// component and no second templ file — the page composes, the components do
// not know about each other.
type sequence []recordfamily.Renderer

func (s sequence) Render(ctx context.Context, w io.Writer) error {
	for _, component := range s {
		if component == nil {
			continue
		}

		if err := component.Render(ctx, w); err != nil {
			return err
		}
	}

	return nil
}

// medicationLinks holds the five addresses the pages and the components need,
// each recovered from the route table rather than composed here.
type medicationLinks struct {
	listPage     string
	detailPage   string
	settingsPage string
	patientsPage string
	record       string
	collection   string

	// recordTemplate is api.OpGetRecord's own path, {kind} and {id}
	// unsubstituted — FR-055's other end resolves a back-relation's own
	// PATCH target by naming a different kind than this page's own.
	recordTemplate string

	allergyDetailPage   string
	conditionDetailPage string
	injuryDetailPage    string
	symptomDetailPage   string
	treatmentDetailPage string
	courseMedications   string
}

func newMedicationLinks() (medicationLinks, error) {
	paths, err := routePaths(map[string]string{
		OpMedicationListPage:        "",
		OpMedicationDetailPage:      "",
		OpSettingsPage:              "",
		OpPatientListPage:           "",
		OpAllergyDetailPage:         "",
		OpConditionDetailPage:       "",
		OpInjuryDetailPage:          "",
		OpSymptomDetailPage:         "",
		OpTreatmentDetailPage:       "",
		api.OpGetRecord:             "",
		api.OpCreateRecord:          "",
		api.OpListCourseMedications: "",
	})
	if err != nil {
		return medicationLinks{}, err
	}

	segment := kind.Medication.Segment()

	return medicationLinks{
		listPage:            paths[OpMedicationListPage],
		detailPage:          paths[OpMedicationDetailPage],
		settingsPage:        paths[OpSettingsPage],
		patientsPage:        paths[OpPatientListPage],
		record:              strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:          strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
		recordTemplate:      paths[api.OpGetRecord],
		allergyDetailPage:   paths[OpAllergyDetailPage],
		conditionDetailPage: paths[OpConditionDetailPage],
		injuryDetailPage:    paths[OpInjuryDetailPage],
		symptomDetailPage:   paths[OpSymptomDetailPage],
		treatmentDetailPage: paths[OpTreatmentDetailPage],
		courseMedications:   paths[api.OpListCourseMedications],
	}, nil
}

// recordHref is another kind's own generic PATCH/DELETE target — the same
// address that kind's own page builds for itself, computed here because this
// page is the one resolving a back-relation into it, not that kind's own page.
func (l medicationLinks) recordHref(segment, id string) string {
	href := strings.ReplaceAll(l.recordTemplate, "{"+api.PathKind+"}", segment)
	return strings.ReplaceAll(href, "{"+api.PathID+"}", id)
}

func (l medicationLinks) allergyHref(id string) string {
	return strings.ReplaceAll(l.allergyDetailPage, "{"+api.PathID+"}", id)
}

func (l medicationLinks) allergyRecordHref(id string) string {
	return l.recordHref(kind.Allergy.Segment(), id)
}

func (l medicationLinks) conditionHref(id string) string {
	return strings.ReplaceAll(l.conditionDetailPage, "{"+api.PathID+"}", id)
}

func (l medicationLinks) conditionRecordHref(id string) string {
	return l.recordHref(kind.Condition.Segment(), id)
}

func (l medicationLinks) injuryHref(id string) string {
	return strings.ReplaceAll(l.injuryDetailPage, "{"+api.PathID+"}", id)
}

func (l medicationLinks) injuryRecordHref(id string) string {
	return l.recordHref(kind.Injury.Segment(), id)
}

func (l medicationLinks) symptomHref(id string) string {
	return strings.ReplaceAll(l.symptomDetailPage, "{"+api.PathID+"}", id)
}

func (l medicationLinks) symptomRecordHref(id string) string {
	return l.recordHref(kind.Symptom.Segment(), id)
}

func (l medicationLinks) treatmentHref(id string) string {
	return strings.ReplaceAll(l.treatmentDetailPage, "{"+api.PathID+"}", id)
}

func (l medicationLinks) courseMedicationItemHref(treatmentID, medicationID string) string {
	base := strings.ReplaceAll(l.courseMedications, "{"+api.PathID+"}", treatmentID)
	return base + "/" + medicationID
}

// of is one record's three addresses. Edit is the detail page: this phase
// registers no edit page, and the form the detail renders is where a change is
// made, so pointing elsewhere would be a link to a route that does not exist.
func (l medicationLinks) of(recordID string) views.MedicationLinks {
	if recordID == "" {
		return views.MedicationLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.MedicationLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Medication, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

// submitExpression is what the form's submission runs. A create posts to the
// collection; a change patches the record and carries the version the page was
// rendered from as If-Match, because a version fetched again is a version that
// can already be stale (FR-026).
func (l medicationLinks) submitExpression(medication views.MedicationView) string {
	if medication.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(medication.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l medicationLinks) cancelHref(medication views.MedicationView) string {
	if medication.Links.Detail != "" {
		return medication.Links.Detail
	}

	return l.listPage
}

func (v MedicationViews) cancelHref(medication views.MedicationView) string {
	return v.links.cancelHref(medication)
}

// nav is the primary navigation's contents. contracts/pages.md fixes the
// landmark on every page and leaves what is in it to the page.
//
// Settings is here too, and not only on the account surface's own nav
// (accountLinks.signedInNav): FR-050 requires every signed-in page to offer a
// route to the medication list AND to settings, and a person reading a
// medication's detail is one of them.
func (l medicationLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}

// routePaths recovers several registered paths in one walk and reports every
// one it could not find, so a rename in the route table names all of its
// casualties at once.
func routePaths(wanted map[string]string) (map[string]string, error) {
	found := make(map[string]string, len(wanted))

	for _, route := range httproute.Inventory().Routes() {
		if _, needed := wanted[route.OpID]; needed {
			found[route.OpID] = route.Path
		}
	}

	var missing []string

	for opID := range wanted {
		if found[opID] == "" {
			missing = append(missing, opID)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("page: the route table has no %s, so a page has no address to link to", strings.Join(missing, ", "))
	}

	return found, nil
}

func quote(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `\'`) + `'`
}

func readDate(raw *string) domain.Date {
	if raw == nil {
		return domain.Date{}
	}

	// A date this application wrote and cannot read back is a defect, not a
	// refusal to make to the person reading the page: the absent date renders
	// as nothing, which is what an unrecorded one does.
	parsed, err := domain.ParseDate(*raw)
	if err != nil {
		return domain.Date{}
	}

	return parsed
}

func readInstant(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}

	return parsed.UTC()
}
