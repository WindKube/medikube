package page

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domaintag "medikube/internal/domain/tag"
	tagsvc "medikube/internal/service/tag"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/components"
	"medikube/internal/web/views/ids"
	viewtags "medikube/internal/web/views/tags"
)

// tagForms implements api.TagForms, mirroring facilityForms' own reasoning:
// the API renders through the same components the /tags page itself builds.
type tagForms struct {
	resolve api.TagResolve
	links   tagLinks
}

// NewTagForms builds the adapter api.TagHandlers renders a Datastar form
// submit through.
func NewTagForms(resolve api.TagResolve) (api.TagForms, error) {
	if resolve == nil {
		return nil, api.ErrNoTags
	}

	links, err := newTagLinks()
	if err != nil {
		return nil, err
	}

	return tagForms{resolve: resolve, links: links}, nil
}

func (f tagForms) view(t domaintag.Tag, usage int) viewtags.TagView {
	return viewtags.NewTagView(t, usage, f.links.of(t.ID))
}

// Invalid re-renders only the one form the submission came from — the
// create form for a rejected create, that tag's own rename form for a
// rejected rename — patched into place by its own id.
func (f tagForms) Invalid(
	_ context.Context, _ access.Actor, submitted domaintag.Tag, isNew bool, invalid *domain.ValidationError,
) (web.Component, error) {
	view := f.view(submitted, 0)
	if isNew {
		view.ID = ""
	}

	return viewtags.Form(viewtags.FormProps{
		FormID:   ids.DirectoryForm(viewtags.Segment, view.ID),
		New:      isNew,
		OnSubmit: f.submitExpression(view, isNew),
		CancelOn: f.cancelOn(view, isNew),
		Tag:      view,
		Errors:   components.NewFieldErrors(invalid),
	}), nil
}

// Created re-renders the whole manager: the new tag needs its own row and
// the create form resets to blank, which is simplest as one fresh list
// rather than two separately-targeted patches.
func (f tagForms) Created(ctx context.Context, actor access.Actor, created domaintag.Tag, usage int) (web.Component, error) {
	return f.manager(ctx, actor)
}

// Updated is Created's twin: a rename or recolour changes what the row
// shows, so the whole list is re-rendered rather than guessing which other
// rows might also need to change.
func (f tagForms) Updated(ctx context.Context, actor access.Actor, updated domaintag.Tag, usage int) (web.Component, error) {
	return f.manager(ctx, actor)
}

func (f tagForms) manager(ctx context.Context, actor access.Actor) (web.Component, error) {
	service, err := f.resolve()
	if err != nil {
		return nil, err
	}

	page, err := service.List(ctx, actor, tagsvc.Query{Limit: web.DefaultLimit})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(page.Items))
	for _, t := range page.Items {
		ids = append(ids, t.ID)
	}

	usage, err := service.Usage(ctx, actor, ids)
	if err != nil {
		return nil, err
	}

	views := make([]viewtags.TagView, 0, len(page.Items))
	for _, t := range page.Items {
		views = append(views, f.view(t, usage[t.ID]))
	}

	return viewtags.Manager(viewtags.ManagerProps{Tags: views, CreateHref: f.links.collection}), nil
}

// submitExpression mirrors facilityLinks' own: a create posts to the
// collection, a rename patches the tag's own record. There is no If-Match to
// carry — a tag is not a clinical record (contracts/tags.md §3).
func (f tagForms) submitExpression(view viewtags.TagView, isNew bool) string {
	if isNew {
		return "@post(" + quote(f.links.collection) + ")"
	}

	return "@patch(" + quote(view.Links.Record) + ")"
}

func (f tagForms) cancelOn(view viewtags.TagView, isNew bool) string {
	if isNew {
		return ""
	}

	return "$" + viewtags.EditSignal(view.ID) + " = false"
}
