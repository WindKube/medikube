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
	OpEncounterListPage   = "encounterListPage"
	OpEncounterDetailPage = "encounterDetailPage"
)

const encounterListTitle = "Encounters"

// EncounterHandlers is the encounter pages' contribution to the route table,
// mirroring medications.go's Handlers end to end (T078).
func EncounterHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newEncounterLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &encounterPages{resolve: resolve, patients: patients, tags: tags, links: links, views: EncounterViews{links: links}}

	return httproute.Handlers{
		OpEncounterListPage:   web.WithActor(pages.list),
		OpEncounterDetailPage: web.WithActor(pages.detail),
	}, nil
}

// EncounterViews is the kind's rendering, as internal/records consumes it.
type EncounterViews struct {
	links encounterLinks
}

var _ recordfamily.Views = EncounterViews{}

func NewEncounterViews() (EncounterViews, error) {
	links, err := newEncounterLinks()
	if err != nil {
		return EncounterViews{}, err
	}

	return EncounterViews{links: links}, nil
}

func (v EncounterViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v EncounterViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.EncounterList(views.EncounterListProps{
		Encounters: v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Encounter, ""),
		NextHref:   nextHref,
	})
}

func (v EncounterViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.EncounterRow(v.view(record))
}

func (v EncounterViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.EncounterDetail(views.EncounterDetailProps{Encounter: v.view(record)})
}

func (v EncounterViews) Title(record recordfamily.Record) string { return v.view(record).Reason }

func (v EncounterViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	encounter := v.view(record)
	fresh := encounter.ID == ""

	formID := ids.RecordForm(kind.Encounter, encounter.ID)

	return views.EncounterForm(views.EncounterFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(encounter),
		CancelHref: v.links.cancelHref(encounter),
		Encounter:  encounter,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v EncounterViews) rows(items []recordfamily.Record) []views.EncounterView {
	rendered := make([]views.EncounterView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v EncounterViews) view(record recordfamily.Record) views.EncounterView {
	encounter := clinical.Encounter{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.Encounter:
		encounter = encounterDetailEntity(record, *body)
	case *api.EncounterSummary:
		encounter = encounterSummaryEntity(record, *body)
	case *api.EncounterCreate:
		encounter.PatientID = body.Patient
	}

	return views.NewEncounterView(encounter, v.links.of(encounter.ID))
}

func encounterSummaryEntity(record recordfamily.Record, summary api.EncounterSummary) clinical.Encounter {
	return clinical.Encounter{
		ID:         record.ID,
		Version:    record.Version,
		Reason:     summary.Reason,
		OccurredOn: readDate(summary.OccurredOn),
		VisitType:  clinical.VisitType(summary.VisitType),
		Priority:   clinical.VisitPriority(summary.Priority),
		UpdatedAt:  readInstant(summary.UpdatedAt),
	}
}

func encounterDetailEntity(record recordfamily.Record, detail api.Encounter) clinical.Encounter {
	encounter := encounterSummaryEntity(record, detail.EncounterSummary)

	encounter.PatientID = detail.Patient
	encounter.Assessment = detail.Assessment
	encounter.Plan = detail.Plan
	encounter.FollowUp = detail.FollowUp
	encounter.DurationMin = detail.DurationMin
	encounter.PractitionerID = detail.Practitioner
	encounter.FacilityID = detail.Facility
	encounter.Notes = detail.Notes
	encounter.CreatedAt = readInstant(detail.CreatedAt)

	return encounter
}

type encounterPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    encounterLinks
	views    EncounterViews
}

func (p *encounterPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Encounter.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Encounter.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Encounter}
	blank.Body = &api.EncounterCreate{Patient: query.PatientID}

	if err := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); err != nil {
		return err
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, encounterListTitle, sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *encounterPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *encounterPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *encounterPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Encounter.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Encounter.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if err := attachTagOptions(e.Request.Context(), actor, p.tags, &found); err != nil {
		return err
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Encounter); ok {
		patientID = detail.Patient
	}

	context, err := p.patientContext(e.Request.Context(), actor, patientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, p.views.view(found).Reason, sequence{
		context,
		entry.Views.Detail(found),
		entry.Views.Form(found, nil, ""),
	})
}

func (p *encounterPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *encounterPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

type encounterLinks struct {
	listPage        string
	medicationsPage string
	detailPage      string
	settingsPage    string
	patientsPage    string
	record          string
	collection      string
}

func newEncounterLinks() (encounterLinks, error) {
	paths, err := routePaths(map[string]string{
		OpEncounterListPage:   "",
		OpEncounterDetailPage: "",
		OpSettingsPage:        "",
		OpMedicationListPage:  "",
		OpPatientListPage:     "",
		api.OpGetRecord:       "",
		api.OpCreateRecord:    "",
	})
	if err != nil {
		return encounterLinks{}, err
	}

	segment := kind.Encounter.Segment()

	return encounterLinks{
		listPage:        paths[OpEncounterListPage],
		medicationsPage: paths[OpMedicationListPage],
		detailPage:      paths[OpEncounterDetailPage],
		settingsPage:    paths[OpSettingsPage],
		patientsPage:    paths[OpPatientListPage],
		record:          strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:      strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l encounterLinks) of(recordID string) views.EncounterLinks {
	if recordID == "" {
		return views.EncounterLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.EncounterLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Encounter, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l encounterLinks) submitExpression(encounter views.EncounterView) string {
	if encounter.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(encounter.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l encounterLinks) cancelHref(encounter views.EncounterView) string {
	if encounter.Links.Detail != "" {
		return encounter.Links.Detail
	}

	return l.listPage
}

func (l encounterLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: encounterListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
