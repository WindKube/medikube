package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	dtag "medikube/internal/domain/tag"
	"medikube/internal/httproute"
	tagsvc "medikube/internal/service/tag"
	"medikube/internal/web"
)

// The four operation ids of contracts/tags.md. One GET serves list,
// autocomplete and popularity; there is no getTag — no operation names a
// single tag on its own.
const (
	OpListTags   = "listTags"
	OpCreateTag  = "createTag"
	OpUpdateTag  = "updateTag"
	OpDeleteTag  = "deleteTag"
	tagsBasePath = "/api/v1/tags"
)

// ErrNoTags is a build whose tag service was never resolved.
var ErrNoTags = errors.New("api: the tag operations were wired without a way to resolve the service")

// TagResolve resolves the tag service, once, on first use — the same reason
// PractitionerResolve and FacilityResolve are functions and not values.
type TagResolve func() (*tagsvc.Service, error)

// TagForms is the tag manager's Datastar half: the API renders a submit
// through the same components the /tags page itself builds, the same
// reasoning FacilityForms documents. There is no Stale case — a tag has no
// If-Match to go stale against (contracts/tags.md §3).
type TagForms interface {
	Invalid(ctx context.Context, actor access.Actor, submitted dtag.Tag, isNew bool, invalid *domain.ValidationError) (web.Component, error)
	Created(ctx context.Context, actor access.Actor, created dtag.Tag, usage int) (web.Component, error)
	Updated(ctx context.Context, actor access.Actor, updated dtag.Tag, usage int) (web.Component, error)
}

// TagDeps is what the four tag handlers need.
type TagDeps struct {
	Resolve TagResolve
	// Forms is optional: a nil value serves JSON only.
	Forms TagForms
}

func (d TagDeps) validate() error {
	if d.Resolve == nil {
		return ErrNoTags
	}

	return nil
}

type tagHandlers struct {
	deps TagDeps
}

// TagHandlers is contracts/tags.md's four operations.
func TagHandlers(deps TagDeps) (httproute.Handlers, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	h := &tagHandlers{deps: deps}

	return httproute.Handlers{
		OpListTags:  web.WithActor(h.list),
		OpCreateTag: web.WithActor(h.create),
		OpUpdateTag: web.WithActor(h.update),
		OpDeleteTag: web.WithActor(h.remove),
	}, nil
}

// list serves the tag manager, the autocomplete every tag picker types
// against (FR-068) and each tag's derived usage_count: one operation, not
// three.
func (h *tagHandlers) list(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	params, err := web.ListQuery(e, tagsvc.Sorts())
	if err != nil {
		return err
	}

	query := tagsvc.Query{
		Search: params.Search,
		Sort:   params.Sort,
		Limit:  params.Limit,
		Cursor: params.Cursor,
		Count:  params.Count,
	}

	page, err := service.List(e.Request.Context(), actor, query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	ids := make([]string, 0, len(page.Items))
	for _, t := range page.Items {
		ids = append(ids, t.ID)
	}

	usage, err := service.Usage(e.Request.Context(), actor, ids)
	if err != nil {
		return web.OwnerScoped(err)
	}

	items := make([]any, 0, len(page.Items))
	for _, t := range page.Items {
		items = append(items, tagDTO(t, usage[t.ID]))
	}

	envelope := domain.NewPage(items, page.NextCursor)
	if page.Total != nil {
		envelope = envelope.WithTotal(*page.Total)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return web.WriteJSON(e, http.StatusOK, envelope)
}

func (h *tagHandlers) create(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	var body TagCreate
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	draft := tagDraft(body)

	created, err := service.Create(e.Request.Context(), actor, draft)
	if err != nil {
		return h.invalid(e, actor, draft, true, err)
	}

	usage, err := service.Usage(e.Request.Context(), actor, []string{created.ID})
	if err != nil {
		return web.OwnerScoped(err)
	}

	if wantsFormPatch(e) && h.deps.Forms != nil {
		component, formErr := h.deps.Forms.Created(e.Request.Context(), actor, created, usage[created.ID])
		if formErr != nil {
			return formErr
		}

		return web.Patch(e, component, web.ByElementID())
	}

	e.Response.Header().Set("Location", tagsBasePath+"/"+created.ID)
	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return web.WriteJSON(e, http.StatusCreated, tagDTO(created, usage[created.ID]))
}

// invalid answers a rejected create or update with the form re-rendered from
// what was submitted, patched into place by its own id. A duplicate name is
// folded into the same shape as a *domain.ValidationError so the form has
// one refusal path to render, even though the JSON error the same failure
// produces (web.ErrDuplicateName, a 409) is not a ValidationError at all.
func (h *tagHandlers) invalid(
	e *core.RequestEvent, actor access.Actor, submitted dtag.Tag, isNew bool, err error,
) error {
	if !wantsFormPatch(e) || h.deps.Forms == nil {
		return h.refused(err)
	}

	var invalid *domain.ValidationError
	switch {
	case errors.Is(err, tagsvc.ErrDuplicateName):
		invalid = &domain.ValidationError{}
		invalid.Add(tagsvc.FieldName, domain.CodeInvalidValue, web.Message(web.CodeDuplicateName))
	case errors.As(err, &invalid):
	default:
		return web.OwnerScoped(err)
	}

	component, formErr := h.deps.Forms.Invalid(e.Request.Context(), actor, submitted, isNew, invalid)
	if formErr != nil {
		return formErr
	}

	return web.Patch(e, component, web.ByElementID())
}

func (h *tagHandlers) update(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathID)

	var body TagPatch
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	updated, err := service.Update(e.Request.Context(), actor, id, tagsvc.Patch{Name: body.Name, Color: body.Color})
	if err != nil {
		submitted := dtag.Tag{ID: id}
		if body.Name != nil {
			submitted.Name = *body.Name
		}
		if body.Color != nil {
			submitted.Color = *body.Color
		}

		return h.invalid(e, actor, submitted, false, err)
	}

	usage, err := service.Usage(e.Request.Context(), actor, []string{updated.ID})
	if err != nil {
		return web.OwnerScoped(err)
	}

	if wantsFormPatch(e) && h.deps.Forms != nil {
		component, formErr := h.deps.Forms.Updated(e.Request.Context(), actor, updated, usage[updated.ID])
		if formErr != nil {
			return formErr
		}

		return web.Patch(e, component, web.ByElementID())
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return web.WriteJSON(e, http.StatusOK, tagDTO(updated, usage[updated.ID]))
}

func (h *tagHandlers) remove(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathID)

	if err := service.Delete(e.Request.Context(), actor, id); err != nil {
		return web.OwnerScoped(err)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return e.NoContent(http.StatusNoContent)
}

// refused translates tag.ErrDuplicateName into MediKube's envelope; anything
// else is FR-033's owner-scoped answer.
func (h *tagHandlers) refused(err error) error {
	if errors.Is(err, tagsvc.ErrDuplicateName) {
		return web.ErrDuplicateName
	}

	return web.OwnerScoped(err)
}
