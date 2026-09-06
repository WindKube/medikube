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
	OpEquipmentListPage   = "equipmentListPage"
	OpEquipmentDetailPage = "equipmentDetailPage"
)

const equipmentListTitle = "Equipment"

// equipmentListTitleID is a message id (D-06), resolved at render time. The
// raw equipmentListTitle stays as-is: shell.NavLink.Label (out of scope,
// shell package) renders it unresolved.
const equipmentListTitleID = "page.equipment.title"

// EquipmentHandlers is the record pages' contribution to the route table.
func EquipmentHandlers(resolve api.Resolve, patients api.PatientResolve, tags api.TagResolve) (httproute.Handlers, error) {
	links, err := newEquipmentLinks()
	if err != nil {
		return nil, err
	}

	if resolve == nil {
		return nil, api.ErrNoRecords
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	pages := &equipmentPages{resolve: resolve, patients: patients, tags: tags, links: links, views: EquipmentViews{links: links}}

	return httproute.Handlers{
		OpEquipmentListPage:   web.WithActor(pages.list),
		OpEquipmentDetailPage: web.WithActor(pages.detail),
	}, nil
}

// EquipmentViews is the kind's rendering, as internal/records consumes it.
type EquipmentViews struct {
	links equipmentLinks
}

var _ recordfamily.Views = EquipmentViews{}

func NewEquipmentViews() (EquipmentViews, error) {
	links, err := newEquipmentLinks()
	if err != nil {
		return EquipmentViews{}, err
	}

	return EquipmentViews{links: links}, nil
}

func (v EquipmentViews) List(page domain.Page[recordfamily.Record]) recordfamily.Renderer {
	return v.ListOfPage(page, "")
}

func (v EquipmentViews) ListOfPage(page domain.Page[recordfamily.Record], nextHref string) recordfamily.Renderer {
	return views.EquipmentList(views.EquipmentListProps{
		Equipment:  v.rows(page.Items),
		CreateHref: "#" + ids.RecordForm(kind.Equipment, ""),
		NextHref:   nextHref,
	})
}

func (v EquipmentViews) Row(record recordfamily.Record) recordfamily.Renderer {
	return views.EquipmentRow(v.view(record))
}

func (v EquipmentViews) Detail(record recordfamily.Record) recordfamily.Renderer {
	return views.EquipmentDetail(views.EquipmentDetailProps{Equipment: v.view(record)})
}

func (v EquipmentViews) Title(record recordfamily.Record) string { return v.view(record).Name }

func (v EquipmentViews) Form(record recordfamily.Record, invalid *domain.ValidationError, notice string) recordfamily.Renderer {
	item := v.view(record)
	fresh := item.ID == ""

	formID := ids.RecordForm(kind.Equipment, item.ID)

	return views.EquipmentForm(views.EquipmentFormProps{
		FormID:     formID,
		New:        fresh,
		OnSubmit:   v.links.submitExpression(item),
		CancelHref: v.cancelHref(item),
		Equipment:  item,
		Errors:     views.NewFieldErrors(invalid),
		Notice:     notice,
		Tags:       tagField(formID, record),
	})
}

func (v EquipmentViews) rows(items []recordfamily.Record) []views.EquipmentView {
	rendered := make([]views.EquipmentView, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, v.view(item))
	}

	return rendered
}

func (v EquipmentViews) view(record recordfamily.Record) views.EquipmentView {
	entity := clinical.Equipment{ID: record.ID, Version: record.Version}

	var basis []string

	switch body := record.Body.(type) {
	case *api.Equipment:
		entity = equipmentDetailEntity(record, *body)
	case *api.EquipmentSummary:
		entity = equipmentSummaryEntity(record, *body)
		basis = body.Basis
	case *api.EquipmentCreate:
		entity.PatientID = body.Patient
	}

	return views.NewEquipmentView(entity, basis, v.links.of(entity.ID))
}

func equipmentSummaryEntity(record recordfamily.Record, summary api.EquipmentSummary) clinical.Equipment {
	return clinical.Equipment{
		ID:           record.ID,
		Version:      record.Version,
		Name:         summary.Name,
		Type:         clinical.EquipmentType(summary.Type),
		Status:       clinical.TherapyStatus(summary.Status),
		ServiceDueOn: readDate(summary.ServiceDueOn),
		UpdatedAt:    readInstant(summary.UpdatedAt),
	}
}

func equipmentDetailEntity(record recordfamily.Record, detail api.Equipment) clinical.Equipment {
	entity := equipmentSummaryEntity(record, detail.EquipmentSummary)

	entity.PatientID = detail.Patient
	entity.Manufacturer = detail.Manufacturer
	entity.Model = detail.Model
	entity.Serial = detail.Serial
	entity.PrescribedOn = readDate(detail.PrescribedOn)
	entity.ServicedOn = readDate(detail.ServicedOn)
	entity.Instructions = detail.Instructions
	entity.SupplierID = detail.Supplier
	entity.PractitionerID = detail.Practitioner
	entity.Notes = detail.Notes
	entity.CreatedAt = readInstant(detail.CreatedAt)

	return entity
}

type equipmentPages struct {
	resolve  api.Resolve
	patients api.PatientResolve
	tags     api.TagResolve
	links    equipmentLinks
	views    EquipmentViews
}

func (p *equipmentPages) list(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	if e.Request.URL.Query().Get(api.ParamPatient) == "" {
		return p.redirectToActivePatient(e, actor)
	}

	entry, err := handler.Dispatch(kind.Equipment.Segment())
	if err != nil {
		return err
	}

	query, err := api.KindQuery(e, entry)
	if err != nil {
		return err
	}

	listing, err := handler.ListOfKind(e.Request.Context(), actor, kind.Equipment.Segment(), query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	blank := recordfamily.Record{Kind: kind.Equipment}
	blank.Body = &api.EquipmentCreate{Patient: query.PatientID}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &blank); tagErr != nil {
		return tagErr
	}

	context, err := p.patientContext(e.Request.Context(), actor, query.PatientID)
	if err != nil {
		return err
	}

	web.Localize(e)

	return p.render(e, actor, i18n.T(e.Request.Context(), equipmentListTitleID), sequence{
		context,
		p.views.ListOfPage(listing, nextPageHref(e, listing)),
		entry.Views.Form(blank, nil, ""),
	})
}

func (p *equipmentPages) redirectToActivePatient(e *core.RequestEvent, actor access.Actor) error {
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

func (p *equipmentPages) patientContext(ctx context.Context, actor access.Actor, patientID string) (recordfamily.Renderer, error) {
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

func (p *equipmentPages) detail(e *core.RequestEvent, actor access.Actor) error {
	handler, err := p.session(actor)
	if err != nil {
		return err
	}

	entry, err := handler.Dispatch(kind.Equipment.Segment())
	if err != nil {
		return err
	}

	found, err := handler.Get(e.Request.Context(), actor,
		kind.Equipment.Segment(), e.Request.PathValue(api.PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	if tagErr := attachTagOptions(e.Request.Context(), actor, p.tags, &found); tagErr != nil {
		return tagErr
	}

	patientID := ""
	if detail, ok := found.Body.(*api.Equipment); ok {
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

func (p *equipmentPages) session(actor access.Actor) (*recordfamily.Handler, error) {
	if !actor.Authenticated() {
		return nil, fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return p.resolve()
}

func (p *equipmentPages) render(e *core.RequestEvent, actor access.Actor, title string, main sequence) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, title,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, main)
}

type equipmentLinks struct {
	listPage       string
	detailPage     string
	settingsPage   string
	patientsPage   string
	medicationPage string
	record         string
	collection     string
}

func newEquipmentLinks() (equipmentLinks, error) {
	paths, err := routePaths(map[string]string{
		OpEquipmentListPage:   "",
		OpEquipmentDetailPage: "",
		OpSettingsPage:        "",
		OpPatientListPage:     "",
		OpMedicationListPage:  "",
		api.OpGetRecord:       "",
		api.OpCreateRecord:    "",
	})
	if err != nil {
		return equipmentLinks{}, err
	}

	segment := kind.Equipment.Segment()

	return equipmentLinks{
		listPage:       paths[OpEquipmentListPage],
		detailPage:     paths[OpEquipmentDetailPage],
		settingsPage:   paths[OpSettingsPage],
		patientsPage:   paths[OpPatientListPage],
		medicationPage: paths[OpMedicationListPage],
		record:         strings.ReplaceAll(paths[api.OpGetRecord], "{"+api.PathKind+"}", segment),
		collection:     strings.ReplaceAll(paths[api.OpCreateRecord], "{"+api.PathKind+"}", segment),
	}, nil
}

func (l equipmentLinks) of(recordID string) views.EquipmentLinks {
	if recordID == "" {
		return views.EquipmentLinks{}
	}

	detail := strings.ReplaceAll(l.detailPage, "{"+api.PathID+"}", recordID)

	return views.EquipmentLinks{
		Detail: detail,
		Edit:   detail + "#" + ids.RecordForm(kind.Equipment, recordID),
		Record: strings.ReplaceAll(l.record, "{"+api.PathID+"}", recordID),
	}
}

func (l equipmentLinks) submitExpression(item views.EquipmentView) string {
	if item.ID == "" {
		return "@post(" + quote(l.collection) + ")"
	}

	return "@patch(" + quote(item.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

func (l equipmentLinks) cancelHref(item views.EquipmentView) string {
	if item.Links.Detail != "" {
		return item.Links.Detail
	}

	return l.listPage
}

func (v EquipmentViews) cancelHref(item views.EquipmentView) string {
	return v.links.cancelHref(item)
}

// nav offers, beyond this kind's own entry, the medication list and settings
// (FR-050): every signed-in page keeps both one link away, regardless of
// which record kind it is on.
func (l equipmentLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationPage, Current: strings.HasPrefix(current, l.medicationPage)},
		{Label: equipmentListTitle, Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
