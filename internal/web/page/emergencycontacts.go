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
	OpEmergencyContactListPage   = "emergencyContactListPage"
	OpEmergencyContactDetailPage = "emergencyContactDetailPage"
)

// emergencyContactListTitleID is a message id (D-06), resolved at render time.
const emergencyContactListTitleID = "page.emergency_contacts.title"

// EmergencyContactHandlers is the emergency-contact pages' contribution to
// the route table.
func EmergencyContactHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newEmergencyContactLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &emergencyContactPages{resolve: resolve, patients: patients, tags: tags, links: links, views: EmergencyContactViews{links: links}}

	return httproute.Handlers{
		OpEmergencyContactListPage:   web.WithActor(pages.list),
		OpEmergencyContactDetailPage: web.WithActor(pages.detail),
	}, nil
}

// EmergencyContactViews is the kind's rendering, as internal/records consumes
// it.
type EmergencyContactViews struct {
	links emergencyContactLinks
}

var _ recordfamily.Views = EmergencyContactViews{}

func NewEmergencyContactViews() (EmergencyContactViews, error) {
	links, err := newEmergencyContactLinks()
	if err != nil {
		return EmergencyContactViews{}, err
	}

	return EmergencyContactViews{links: links}, nil
}

func (v EmergencyContactViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v EmergencyContactViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.EmergencyContactList(views.EmergencyContactListProps{
		Contacts:   v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.EmergencyContact, ""),
		NextHref:   nextHref,
	})
}

func (v EmergencyContactViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.EmergencyContactRow(v.view(record))
}

func (v EmergencyContactViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.EmergencyContactDetail(views.EmergencyContactDetailProps{Contact: v.view(record)})
}

func (v EmergencyContactViews) Title(record recordfamily.Record) string { return v.view(record).Name }

func (v EmergencyContactViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	contact := v.view(record)
	fresh := contact.ID == ""

	formID := ids.RecordForm(kind.EmergencyContact, contact.ID)

	return views.EmergencyContactForm(views.EmergencyContactFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(contact),
		CancelHref: v.cancelHref(contact),
		Contact:    contact,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v EmergencyContactViews) rows(items []recordfamily.Record) []views.EmergencyContactView {
	rendered := make([]views.EmergencyContactView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v EmergencyContactViews) view(record recordfamily.Record) views.EmergencyContactView {
	contact := clinical.EmergencyContact{ID: record.ID, Version: record.Version}

	switch body := record.Body.(type) {
	case *api.EmergencyContact:
		contact = emergencyContactDetailEntity(record, *body)
	case *api.EmergencyContactSummary:
		contact = emergencyContactSummaryEntity(record, *body)
	case *api.EmergencyContactCreate:
		contact.PatientID = body.Patient
		contact.IsActive = true
	}

	return views.NewEmergencyContactView(contact, v.links.of(contact.ID))
}

func emergencyContactSummaryEntity(record recordfamily.Record, summary api.EmergencyContactSummary) clinical.EmergencyContact {
	return clinical.EmergencyContact{
		ID:           record.ID,
		Version:      record.Version,
		Name:         summary.Name,
		Relationship: clinical.ContactRelationship(summary.Relationship),
		Phone:        summary.Phone,
		IsPrimary:    summary.IsPrimary,
		IsActive:     summary.IsActive,
		UpdatedAt:    readInstant(summary.UpdatedAt),
	}
}

func emergencyContactDetailEntity(record recordfamily.Record, detail api.EmergencyContact) clinical.EmergencyContact {
	contact := emergencyContactSummaryEntity(record, detail.EmergencyContactSummary)

	contact.PatientID = detail.Patient
	contact.PhoneAlt = detail.PhoneAlt
	contact.Email = detail.Email
	contact.Address = detail.Address
	contact.Notes = detail.Notes
	contact.CreatedAt = readInstant(detail.CreatedAt)

	if detail.Displaced != nil {
		contact.DisplacedID = detail.Displaced.ID
	}

	return contact
}

type emergencyContactPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    emergencyContactLinks
	views    EmergencyContactViews
}

func (p *emergencyContactPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.EmergencyContact.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.EmergencyContact.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.EmergencyContact}
	blank.Body = &api.EmergencyContactCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), emergencyContactListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *emergencyContactPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *emergencyContactPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *emergencyContactPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.EmergencyContact.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.EmergencyContact.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID := ""
	if detail, ok := found.Body.(*api.EmergencyContact); ok {
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

func (p *emergencyContactPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *emergencyContactPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(localizeCtx(e), e.Request.URL.Path), Switcher: switcher}, main)
}

type emergencyContactLinks struct {
	listPage        string
	detailPage      string
	settingsPage    string
	patientsPage    string
	medicationsPage string
	record          string
	collection      string
}

func newEmergencyContactLinks() (emergencyContactLinks, error) {
	paths, err := routePaths(map[string]string{
		OpEmergencyContactListPage:   "",
		OpEmergencyContactDetailPage: "",
		OpSettingsPage:               "",
		OpPatientListPage:            "",
		OpMedicationListPage:         "",
		api.OpGetRecord:              "",
		api.OpCreateRecord:           "",
	})
	if err != nil {
		return emergencyContactLinks{}, err
	}

	segment := kind.EmergencyContact.Segment()

	return emergencyContactLinks{
		listPage:        paths[OpEmergencyContactListPage],
		detailPage:      paths[OpEmergencyContactDetailPage],
		settingsPage:    paths[OpSettingsPage],
		patientsPage:    paths[OpPatientListPage],
		medicationsPage: paths[OpMedicationListPage],
		record:          strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:      strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l emergencyContactLinks) of(recordID string) views.EmergencyContactLinks {
	if recordID == "" {
		return views.EmergencyContactLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.EmergencyContactLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.EmergencyContact, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l emergencyContactLinks) submitExpression(contact views.EmergencyContactView) string {
	if contact.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(contact.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l emergencyContactLinks) cancelHref(contact views.EmergencyContactView) string {
	if contact.Links.Detail != "" {
		return contact.Links.Detail
	}

	return l.listPage
}

func (v EmergencyContactViews) cancelHref(contact views.EmergencyContactView) string {
	return v.links.cancelHref(contact)
}

// nav is FR-050's fixed pair, not a per-kind entry: every signed-in page
// offers the route back to the medication list and to settings, the same
// nav medications.go itself renders.
func (l emergencyContactLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "nav.medications"), Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: i18n.T(ctx, "nav.settings"), Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
