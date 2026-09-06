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
	"medikube/internal/domain/person"
	"medikube/internal/httproute"
	recordfamily "medikube/internal/records"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
	views "medikube/internal/web/views/records"
	"medikube/internal/web/views/shell"
)

const (
	OpFamilyMemberListPage   = "familyHistoryListPage"
	OpFamilyMemberDetailPage = "familyHistoryDetailPage"
)

const familyMemberListTitle = "Family history"

// FamilyMemberHandlers is the record pages' contribution to the route table.
func FamilyMemberHandlers(resolve api.Resolve, patients api.PatientResolve) (httproute.Handlers, error) {
	links, err := newFamilyMemberLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &familyMemberPages{resolve: resolve, patients: patients, links: links, views: FamilyMemberViews{links: links}}

	return httproute.Handlers{
		OpFamilyMemberListPage:   web.WithActor(pages.list),
		OpFamilyMemberDetailPage: web.WithActor(pages.detail),
	}, nil
}

// FamilyMemberViews is the kind's rendering, as internal/records consumes it.
type FamilyMemberViews struct {
	links familyMemberLinks
}

var _ recordfamily.Views = FamilyMemberViews{}

func NewFamilyMemberViews() (FamilyMemberViews, error) {
	links, err := newFamilyMemberLinks()
	if err != nil {
		return FamilyMemberViews{}, err
	}

	return FamilyMemberViews{links: links}, nil
}

func (v FamilyMemberViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v FamilyMemberViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.FamilyMemberList(views.FamilyMemberListProps{
		FamilyMembers: v.rows(page.Items),
		CreateHref:    "#" + ids.RecordForm(kind.FamilyMember, ""),
		NextHref:      nextHref,
	})
}

func (v FamilyMemberViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.FamilyMemberRow(v.view(record))
}

func (v FamilyMemberViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.FamilyMemberDetail(views.FamilyMemberDetailProps{FamilyMember: v.view(record)})
}

func (v FamilyMemberViews) Title(record recordfamily.Record) string { return v.view(record).Name }

func (v FamilyMemberViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	item := v.view(record)
	fresh := item.ID == ""

	return views.FamilyMemberForm(views.FamilyMemberFormProps{
		FormID:       ids.RecordForm(kind.FamilyMember, item.ID),
		New:          fresh,
		OnSubmit:     v.links.submitExpression(item),
		CancelHref:   v.cancelHref(item),
		FamilyMember: item,
		Errors:       views.NewFieldErrors(invalid),
		Notice:       notice,
	})
}

func (v FamilyMemberViews) rows(items []recordfamily.Record) []views.FamilyMemberView {
	rendered := make([]views.FamilyMemberView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v FamilyMemberViews) view(record recordfamily.Record) views.FamilyMemberView {
	entity := clinical.FamilyMember{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.FamilyMember:
		entity = familyMemberDetailEntity(record, *body)
	case *api.FamilyMemberSummary:
		entity = familyMemberSummaryEntity(record, *body)
	case *api.FamilyMemberCreate:
		entity.PatientID = body.Patient
	}

	return views.NewFamilyMemberView(entity, v.links.of(entity.ID))
}

func familyMemberSummaryEntity(record recordfamily.Record, summary api.FamilyMemberSummary) clinical.FamilyMember {
	return clinical.FamilyMember{
		ID:           record.ID,
		Version:      record.Version,
		Name:         summary.Name,
		Relationship: clinical.FamilyRelationship(summary.Relationship),
		UpdatedAt:    readInstant(summary.UpdatedAt),
	}
}

func familyMemberDetailEntity(record recordfamily.Record, detail api.FamilyMember) clinical.FamilyMember {
	entity := familyMemberSummaryEntity(record, detail.FamilyMemberSummary)

	entity.PatientID = detail.Patient
	entity.Sex = person.Sex(detail.Sex)
	entity.BirthYear = detail.BirthYear
	entity.DeathYear = detail.DeathYear
	entity.IsDeceased = detail.IsDeceased
	entity.Conditions = familyConditionsOf(detail.Conditions)
	entity.CreatedAt = readInstant(detail.CreatedAt)

	return entity
}

func familyConditionsOf(conditions []api.FamilyCondition) []clinical.FamilyCondition {
	converted := make([]clinical.FamilyCondition, 0, len(conditions))

	for _, condition := range conditions {
		converted = append(converted, clinical.FamilyCondition{
			Name:         condition.Name,
			ICD10Code:    condition.ICD10Code,
			DiagnosedAge: condition.DiagnosedAge,
			Severity:     clinical.Severity(condition.Severity),
			Status:       clinical.ConditionStatus(condition.Status),
			Notes:        condition.Notes,
		})
	}

	return converted
}

type familyMemberPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	links    familyMemberLinks
	views    FamilyMemberViews
}

func (p *familyMemberPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.FamilyMember.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.FamilyMember.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.FamilyMember}
	blank.Body = &api.FamilyMemberCreate{Patient: query.PatientID}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	return p.render(e, actor, familyMemberListTitle, sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *familyMemberPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *familyMemberPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *familyMemberPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.FamilyMember.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.FamilyMember.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	patientID := ""
	if detail, ok := found.Body.(*api.FamilyMember); ok {
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

func (p *familyMemberPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *familyMemberPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

type familyMemberLinks struct {
	listPage        string
	detailPage      string
	settingsPage    string
	patientsPage    string
	medicationsPage string
	record          string
	collection      string
}

func newFamilyMemberLinks() (familyMemberLinks, error) {
	paths, err := routePaths(map[string]string{
		OpFamilyMemberListPage:   "",
		OpFamilyMemberDetailPage: "",
		OpSettingsPage:           "",
		OpPatientListPage:        "",
		OpMedicationListPage:     "",
		api.OpGetRecord:          "",
		api.OpCreateRecord:       "",
	})
	if err != nil {
		return familyMemberLinks{}, err
	}

	segment := kind.FamilyMember.Segment()

	return familyMemberLinks{
		listPage:        paths[OpFamilyMemberListPage],
		detailPage:      paths[OpFamilyMemberDetailPage],
		settingsPage:    paths[OpSettingsPage],
		patientsPage:    paths[OpPatientListPage],
		medicationsPage: paths[OpMedicationListPage],
		record:          strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:      strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l familyMemberLinks) of(recordID string) views.FamilyMemberLinks {
	if recordID == "" {
		return views.FamilyMemberLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.FamilyMemberLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.FamilyMember, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l familyMemberLinks) submitExpression(item views.FamilyMemberView) string {
	if item.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(item.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l familyMemberLinks) cancelHref(item views.FamilyMemberView) string {
	if item.Links.Detail != "" {
		return item.Links.Detail
	}

	return l.listPage
}

func (v FamilyMemberViews) cancelHref(item views.FamilyMemberView) string {
	return v.links.cancelHref(item)
}

func (l familyMemberLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: familyMemberListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
