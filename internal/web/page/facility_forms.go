package page

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domaindirectory "medikube/internal/domain/directory"
	facilitysvc "medikube/internal/service/facility"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
)

// facilityForms implements api.FacilityForms, mirroring patientForms' own
// reasoning: the API renders through the same components the pages
// themselves build.
type facilityForms struct {
	resolve api.FacilityResolve
	links   facilityLinks
}

// NewFacilityForms builds the adapter api.FacilityHandlers renders a
// Datastar form submit through.
func NewFacilityForms(resolve api.FacilityResolve) (api.FacilityForms, error) {
	if resolve == nil {
		return nil, api.ErrNoFacilities
	}

	links, err := newFacilityLinks()
	if err != nil {
		return nil, err
	}

	return facilityForms{resolve: resolve, links: links}, nil
}

func (f facilityForms) view(ctx context.Context, p domaindirectory.Facility) directory.FacilityView {
	return directory.NewFacilityView(ctx, p, f.links.of(p.ID))
}

func (f facilityForms) Invalid(
	ctx context.Context, _ access.Actor, submitted domaindirectory.Facility, isNew bool, invalid *domain.ValidationError,
) (web.Component, error) {
	view := f.view(ctx, submitted)

	return directory.FacilityForm(directory.FacilityFormProps{
		FormID:     ids.DirectoryForm(directory.FacilitySegment, view.ID),
		New:        isNew,
		OnSubmit:   f.links.submitExpression(view),
		CancelHref: f.links.cancelHref(view),
		Facility:   view,
		Errors:     directory.NewFieldErrors(invalid),
	}), nil
}

func (f facilityForms) Stale(ctx context.Context, _ access.Actor, current domaindirectory.Facility) (web.Component, error) {
	view := f.view(ctx, current)

	return directory.FacilityForm(directory.FacilityFormProps{
		FormID:     ids.DirectoryForm(directory.FacilitySegment, view.ID),
		New:        false,
		OnSubmit:   f.links.submitExpression(view),
		CancelHref: f.links.cancelHref(view),
		Facility:   view,
		Errors:     directory.NewFieldErrors(nil),
		Notice:     staleFormNotice(ctx),
	}), nil
}

func (f facilityForms) Created(ctx context.Context, actor access.Actor, created domaindirectory.Facility) (web.Component, error) {
	service, err := f.resolve()
	if err != nil {
		return nil, err
	}

	page, err := service.List(ctx, actor, f.listQuery())
	if err != nil {
		return nil, err
	}

	views := make([]directory.FacilityView, 0, len(page.Items))
	for _, item := range page.Items {
		views = append(views, f.view(ctx, item))
	}

	blank := directory.FacilityView{}

	return sequence{
		directory.FacilityList(directory.FacilityListProps{
			Facilities: views,
			CreateHref: f.links.listPage + "#" + ids.DirectoryForm(directory.FacilitySegment, ""),
		}),
		directory.FacilityForm(directory.FacilityFormProps{
			FormID:     ids.DirectoryForm(directory.FacilitySegment, ""),
			New:        true,
			OnSubmit:   f.links.submitExpression(blank),
			CancelHref: f.links.cancelHref(blank),
			Facility:   blank,
			Errors:     directory.NewFieldErrors(nil),
		}),
	}, nil
}

func (f facilityForms) Updated(ctx context.Context, actor access.Actor, updated domaindirectory.Facility) (web.Component, error) {
	service, err := f.resolve()
	if err != nil {
		return nil, err
	}

	usage, err := service.Usage(ctx, actor, updated.ID)
	if err != nil {
		return nil, err
	}

	view := f.view(ctx, updated)
	view.UsagePractitioners = usage.Practitioners
	view.UsageRecords = usage.Records

	return sequence{
		directory.FacilityDetail(directory.FacilityDetailProps{Facility: view}),
		directory.FacilityForm(directory.FacilityFormProps{
			FormID:     ids.DirectoryForm(directory.FacilitySegment, view.ID),
			New:        false,
			OnSubmit:   f.links.submitExpression(view),
			CancelHref: f.links.cancelHref(view),
			Facility:   view,
			Errors:     directory.NewFieldErrors(nil),
		}),
	}, nil
}

// listQuery mirrors facilityPages.list's own query: the directory's default
// ordering, one page.
func (f facilityForms) listQuery() facilitysvc.Query {
	return facilitysvc.Query{Limit: web.DefaultLimit}
}
