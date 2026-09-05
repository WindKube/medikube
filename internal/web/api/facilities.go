package api

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/directory"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	facilitysvc "medikube/internal/service/facility"
	"medikube/internal/web"
)

// The five operation ids of contracts/facilities.md.
const (
	OpListFacilities   = "listFacilities"
	OpCreateFacility   = "createFacility"
	OpGetFacility      = "getFacility"
	OpUpdateFacility   = "updateFacility"
	OpDeleteFacility   = "deleteFacility"
	facilitiesBasePath = "/api/v1/facilities"
)

// ErrNoFacilities is a build whose facility service was never resolved.
var ErrNoFacilities = errors.New("api: the facility operations were wired without a way to resolve the service")

// FacilityDeps is what the five facility handlers need.
type FacilityDeps struct {
	Resolve FacilityResolve
}

func (d FacilityDeps) validate() error {
	if d.Resolve == nil {
		return ErrNoFacilities
	}

	return nil
}

type facilityHandlers struct {
	deps FacilityDeps
}

// FacilityHandlers is contracts/facilities.md's five operations.
func FacilityHandlers(deps FacilityDeps) (httproute.Handlers, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	h := &facilityHandlers{deps: deps}

	return httproute.Handlers{
		OpListFacilities: web.WithActor(h.list),
		OpCreateFacility: web.WithActor(h.create),
		OpGetFacility:    web.WithActor(h.get),
		OpUpdateFacility: web.WithActor(h.update),
		OpDeleteFacility: web.WithActor(h.remove),
	}, nil
}

// list serves the directory page, the kind filter (FR-036) and the
// type-ahead behind every facility and pharmacy picker (FR-039): one
// operation, not six.
func (h *facilityHandlers) list(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	params, err := web.ListQuery(e, facilitysvc.Sorts())
	if err != nil {
		return err
	}

	query := facilitysvc.Query{
		Search: params.Search,
		Kind:   directory.FacilityKind(e.Request.URL.Query().Get(facilitysvc.FilterKind)),
		Limit:  params.Limit,
		Cursor: params.Cursor,
		Count:  params.Count,
	}

	page, err := service.List(e.Request.Context(), actor, query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	items := make([]any, 0, len(page.Items))
	for _, f := range page.Items {
		items = append(items, facilitySummary(f))
	}

	envelope := domain.NewPage(items, page.NextCursor)
	if page.Total != nil {
		envelope = envelope.WithTotal(*page.Total)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return web.WriteJSON(e, http.StatusOK, envelope)
}

func (h *facilityHandlers) create(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	var body FacilityCreate
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	created, err := service.Create(e.Request.Context(), actor, facilityDraft(body))
	if err != nil {
		return web.OwnerScoped(err)
	}

	return h.writeDetail(e, actor, service, http.StatusCreated, created, true)
}

func (h *facilityHandlers) get(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	found, err := service.Get(e.Request.Context(), actor, e.Request.PathValue(PathID))
	if err != nil {
		return web.OwnerScoped(err)
	}

	return h.writeDetail(e, actor, service, http.StatusOK, found, false)
}

func (h *facilityHandlers) update(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	var body FacilityPatch
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	updated, err := service.Update(e.Request.Context(), actor, id, version, facilityPatch(body))
	if err != nil {
		return h.stale(e, actor, service, id, err)
	}

	return h.writeDetail(e, actor, service, http.StatusOK, updated, false)
}

func (h *facilityHandlers) remove(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	if err := service.Delete(e.Request.Context(), actor, id, version); err != nil {
		return h.stale(e, actor, service, id, err)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return e.NoContent(http.StatusNoContent)
}

func (h *facilityHandlers) stale(
	e *core.RequestEvent, actor access.Actor, service *facilitysvc.Service, id string, err error,
) error {
	if !errors.Is(err, domain.ErrVersionMismatch) {
		return web.OwnerScoped(err)
	}

	current, readErr := service.Get(e.Request.Context(), actor, id)
	if readErr != nil {
		return web.OwnerScoped(readErr)
	}

	usage, usageErr := service.Usage(e.Request.Context(), actor, id)
	if usageErr != nil {
		return web.OwnerScoped(usageErr)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return web.WriteVersionMismatch(e, obs.CorrelationID(e.Request.Context()),
		current.Version, facilityDetail(current, usage))
}

func (h *facilityHandlers) writeDetail(
	e *core.RequestEvent, actor access.Actor, service *facilitysvc.Service,
	status int, f directory.Facility, created bool,
) error {
	usage, err := service.Usage(e.Request.Context(), actor, f.ID)
	if err != nil {
		return web.OwnerScoped(err)
	}

	if created {
		e.Response.Header().Set("Location", facilitiesBasePath+"/"+f.ID)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)
	web.SetETag(e, f.Version)

	return web.WriteJSON(e, status, facilityDetail(f, usage))
}
