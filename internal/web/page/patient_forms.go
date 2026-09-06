package page

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domainperson "medikube/internal/domain/person"
	"medikube/internal/i18n"
	"medikube/internal/service/patient"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/patients"
)

// staleFormNotice is research D-24's own explanation, rendered inside a form
// re-populated from the server's current values after a stale If-Match —
// shared by the patient, facility and practitioner forms.
func staleFormNotice(ctx context.Context) string {
	return i18n.T(ctx, "form.stale_notice")
}

// patientForms implements api.PatientForms by rendering the same components
// the pages themselves build, so a Datastar form submit and a full page load
// can never disagree about what a patient's form, list or detail looks like.
type patientForms struct {
	deps  PatientDeps
	links patientLinks
}

// NewPatientForms builds the adapter api.PatientHandlers renders a Datastar
// form submit through. It is exported so the composition root can build it
// before wiring the API handlers.
func NewPatientForms(deps PatientDeps) (api.PatientForms, error) {
	if deps.Resolve == nil || deps.UnitOf == nil {
		return nil, ErrNoPatientPages
	}

	links, err := newPatientLinks()
	if err != nil {
		return nil, err
	}

	return patientForms{deps: deps, links: links}, nil
}

func (f patientForms) view(ctx context.Context, actor access.Actor, p domainperson.Patient) (patients.PatientView, error) {
	system, err := f.deps.UnitOf(ctx, actor)
	if err != nil {
		return patients.PatientView{}, err
	}

	var photoURL string
	if p.HasPhoto {
		photoURL = "/api/v1/patients/" + p.ID + "/photo"
	}

	links := patients.PatientLinks{}
	if p.ID != "" {
		links = patients.PatientLinks{Detail: f.links.of(p.ID), Record: "/api/v1/patients/" + p.ID}
	}

	return patients.NewPatientView(ctx, p, photoURL, system, links), nil
}

func (f patientForms) Invalid(
	ctx context.Context, actor access.Actor, submitted domainperson.Patient, isNew bool, invalid *domain.ValidationError,
) (web.Component, error) {
	view, err := f.view(ctx, actor, submitted)
	if err != nil {
		return nil, err
	}

	return patients.PatientForm(patients.PatientFormProps{
		FormID:     ids.PatientForm(view.ID),
		New:        isNew,
		OnSubmit:   f.links.submitExpression(view),
		CancelHref: f.links.cancelHref(view),
		Patient:    view,
		Errors:     patients.NewFieldErrors(invalid),
	}), nil
}

func (f patientForms) Stale(ctx context.Context, actor access.Actor, current domainperson.Patient) (web.Component, error) {
	view, err := f.view(ctx, actor, current)
	if err != nil {
		return nil, err
	}

	return patients.PatientForm(patients.PatientFormProps{
		FormID:     ids.PatientForm(view.ID),
		New:        false,
		OnSubmit:   f.links.submitExpression(view),
		CancelHref: f.links.cancelHref(view),
		Patient:    view,
		Errors:     patients.NewFieldErrors(nil),
		Notice:     staleFormNotice(ctx),
	}), nil
}

func (f patientForms) Created(ctx context.Context, actor access.Actor, created domainperson.Patient) (web.Component, error) {
	svc, err := f.deps.Resolve()
	if err != nil {
		return nil, err
	}

	page, err := svc.List(ctx, actor, patient.Query{Count: true})
	if err != nil {
		return nil, err
	}

	views := make([]patients.PatientView, 0, len(page.Items))

	for _, item := range page.Items {
		view, viewErr := f.view(ctx, actor, item)
		if viewErr != nil {
			return nil, viewErr
		}

		views = append(views, view)
	}

	total := len(views)
	if page.Total != nil {
		total = *page.Total
	}

	blank := patients.PatientView{}

	return sequence{
		patients.PatientList(patients.PatientListProps{
			Patients:   views,
			Total:      total,
			CreateHref: "#" + ids.PatientForm(""),
		}),
		patients.PatientForm(patients.PatientFormProps{
			FormID:     ids.PatientForm(""),
			New:        true,
			OnSubmit:   f.links.submitExpression(blank),
			CancelHref: f.links.cancelHref(blank),
			Patient:    blank,
			Errors:     patients.NewFieldErrors(nil),
		}),
	}, nil
}

func (f patientForms) Updated(ctx context.Context, actor access.Actor, updated domainperson.Patient) (web.Component, error) {
	svc, err := f.deps.Resolve()
	if err != nil {
		return nil, err
	}

	chart, err := svc.Summary(ctx, actor, updated.ID)
	if err != nil {
		return nil, err
	}

	view, err := f.view(ctx, actor, chart.Patient)
	if err != nil {
		return nil, err
	}

	tiles := make([]patients.CountTile, 0, len(chart.Counts))
	for _, entry := range chart.Counts {
		tiles = append(tiles, patients.CountTile{Label: entry.Label, Path: entry.Path, Count: entry.Count})
	}

	events := make([]patients.ActivityEventView, 0, len(chart.RecentActivity))
	for _, event := range chart.RecentActivity {
		events = append(events, patients.ActivityEventView{
			OccurredAt:   event.OccurredAt,
			Action:       string(event.Action),
			TargetKind:   string(event.TargetKind),
			TargetID:     event.TargetID,
			TargetExists: api.TargetExists(ctx, f.deps.Records, actor, event),
		})
	}

	return sequence{
		patients.PatientDetail(patients.PatientDetailProps{
			Patient:      view,
			Tiles:        patients.NewChartTiles(view.ID, tiles),
			Activity:     patients.NewActivityItems(ctx, events, func(string, string) string { return "" }),
			TotalRecords: chart.TotalRecords,
		}),
		patients.PatientForm(patients.PatientFormProps{
			FormID:     ids.PatientForm(view.ID),
			New:        false,
			OnSubmit:   f.links.submitExpression(view),
			CancelHref: f.links.cancelHref(view),
			Patient:    view,
			Errors:     patients.NewFieldErrors(nil),
		}),
	}, nil
}
