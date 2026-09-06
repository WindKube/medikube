package page

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domaindirectory "medikube/internal/domain/directory"
	practitionersvc "medikube/internal/service/practitioner"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
)

// practitionerForms implements api.PractitionerForms, mirroring
// patientForms' own reasoning: the API renders through the same components
// the pages themselves build.
type practitionerForms struct {
	resolve    api.PractitionerResolve
	facilities api.FacilityResolve
	links      practitionerLinks
}

// NewPractitionerForms builds the adapter api.PractitionerHandlers renders a
// Datastar form submit through.
func NewPractitionerForms(resolve api.PractitionerResolve, facilities api.FacilityResolve) (api.PractitionerForms, error) {
	if resolve == nil {
		return nil, api.ErrNoPractitioners
	}

	links, err := newPractitionerLinks()
	if err != nil {
		return nil, err
	}

	return practitionerForms{resolve: resolve, facilities: facilities, links: links}, nil
}

func (f practitionerForms) facilityName(ctx context.Context, actor access.Actor, facilityID string) string {
	if facilityID == "" || f.facilities == nil {
		return ""
	}

	facilities, err := f.facilities()
	if err != nil {
		return ""
	}

	found, err := facilities.Get(ctx, actor, facilityID)
	if err != nil {
		return ""
	}

	return found.Name
}

// listQuery mirrors practitionerPages.list's own query: the directory's
// default ordering, one page.
func (f practitionerForms) listQuery() practitionersvc.Query {
	return practitionersvc.Query{Sort: practitionersvc.Sorts()[:1], Limit: web.DefaultLimit}
}

func (f practitionerForms) view(ctx context.Context, actor access.Actor, p domaindirectory.Practitioner) directory.PractitionerView {
	return directory.NewPractitionerView(ctx, p, f.facilityName(ctx, actor, p.FacilityID), f.links.of(p.ID))
}

func (f practitionerForms) Invalid(
	ctx context.Context, actor access.Actor, submitted domaindirectory.Practitioner, isNew bool, invalid *domain.ValidationError,
) (web.Component, error) {
	view := f.view(ctx, actor, submitted)

	return directory.PractitionerForm(directory.PractitionerFormProps{
		FormID:       ids.DirectoryForm(directory.PractitionerSegment, view.ID),
		New:          isNew,
		OnSubmit:     f.links.submitExpression(view),
		CancelHref:   f.links.cancelHref(view),
		Practitioner: view,
		Errors:       directory.NewFieldErrors(invalid),
	}), nil
}

func (f practitionerForms) Stale(ctx context.Context, actor access.Actor, current domaindirectory.Practitioner) (web.Component, error) {
	view := f.view(ctx, actor, current)

	return directory.PractitionerForm(directory.PractitionerFormProps{
		FormID:       ids.DirectoryForm(directory.PractitionerSegment, view.ID),
		New:          false,
		OnSubmit:     f.links.submitExpression(view),
		CancelHref:   f.links.cancelHref(view),
		Practitioner: view,
		Errors:       directory.NewFieldErrors(nil),
		Notice:       staleFormNotice(ctx),
	}), nil
}

func (f practitionerForms) Created(ctx context.Context, actor access.Actor, created domaindirectory.Practitioner) (web.Component, error) {
	service, err := f.resolve()
	if err != nil {
		return nil, err
	}

	page, err := service.List(ctx, actor, f.listQuery())
	if err != nil {
		return nil, err
	}

	views := make([]directory.PractitionerView, 0, len(page.Items))
	for _, item := range page.Items {
		views = append(views, f.view(ctx, actor, item))
	}

	blank := directory.PractitionerView{}

	return sequence{
		directory.PractitionerList(directory.PractitionerListProps{
			Practitioners: views,
			CreateHref:    f.links.listPage + "#" + ids.DirectoryForm(directory.PractitionerSegment, ""),
		}),
		directory.PractitionerForm(directory.PractitionerFormProps{
			FormID:       ids.DirectoryForm(directory.PractitionerSegment, ""),
			New:          true,
			OnSubmit:     f.links.submitExpression(blank),
			CancelHref:   f.links.cancelHref(blank),
			Practitioner: blank,
			Errors:       directory.NewFieldErrors(nil),
		}),
	}, nil
}

func (f practitionerForms) Updated(ctx context.Context, actor access.Actor, updated domaindirectory.Practitioner) (web.Component, error) {
	service, err := f.resolve()
	if err != nil {
		return nil, err
	}

	usage, err := service.Usage(ctx, actor, updated.ID)
	if err != nil {
		return nil, err
	}

	view := f.view(ctx, actor, updated)
	view.UsagePatients = usage.Patients
	view.UsageRecords = usage.Records

	return sequence{
		directory.PractitionerDetail(directory.PractitionerDetailProps{Practitioner: view}),
		directory.PractitionerForm(directory.PractitionerFormProps{
			FormID:       ids.DirectoryForm(directory.PractitionerSegment, view.ID),
			New:          false,
			OnSubmit:     f.links.submitExpression(view),
			CancelHref:   f.links.cancelHref(view),
			Practitioner: view,
			Errors:       directory.NewFieldErrors(nil),
		}),
	}, nil
}
