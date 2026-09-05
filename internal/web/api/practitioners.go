package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/directory"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	facilitysvc "medikube/internal/service/facility"
	practitionersvc "medikube/internal/service/practitioner"
	"medikube/internal/web"
)

// The five operation ids of contracts/practitioners.md.
const (
	OpListPractitioners   = "listPractitioners"
	OpCreatePractitioner  = "createPractitioner"
	OpGetPractitioner     = "getPractitioner"
	OpUpdatePractitioner  = "updatePractitioner"
	OpDeletePractitioner  = "deletePractitioner"
	practitionersBasePath = "/api/v1/practitioners"
)

// directoryCacheControl mirrors medication's own (recordCacheControl): a
// directory entry is somebody's clinician or place of care, never cached and
// never stored on disk by an intermediary.
const directoryCacheControl = "private, no-store"

// PractitionerResolve resolves the practitioner service, once, on first use —
// the same reason records.Resolve is a function and not a value: a
// repository needs the cursor codec, and the codec is keyed from a secret the
// migrations have only just created (internal/web/api/records.go's own
// Resolve documents the mechanism this mirrors).
type PractitionerResolve func() (*practitionersvc.Service, error)

// FacilityResolve is PractitionerResolve's twin for the facility service. The
// practitioner handlers hold one too, to render the FacilityRef a
// practitioner's response names its facility with.
type FacilityResolve func() (*facilitysvc.Service, error)

// ErrNoPractitioners is a build whose practitioner service was never resolved.
var ErrNoPractitioners = errors.New("api: the practitioner operations were wired without a way to resolve the service")

// PractitionerForms is PatientForms' twin for the practitioner directory: a
// nil value means "JSON only".
type PractitionerForms interface {
	Invalid(ctx context.Context, actor access.Actor, submitted directory.Practitioner, isNew bool, invalid *domain.ValidationError) (web.Component, error)
	Stale(ctx context.Context, actor access.Actor, current directory.Practitioner) (web.Component, error)
	Created(ctx context.Context, actor access.Actor, created directory.Practitioner) (web.Component, error)
	Updated(ctx context.Context, actor access.Actor, updated directory.Practitioner) (web.Component, error)
}

// PractitionerDeps is what the five practitioner handlers need.
type PractitionerDeps struct {
	Resolve PractitionerResolve
	// Facilities is optional in shape only: a nil value makes every facility
	// reference render as null rather than fail the request.
	Facilities FacilityResolve
	// Forms is optional too: a nil value serves JSON only.
	Forms PractitionerForms
}

func (d PractitionerDeps) validate() error {
	if d.Resolve == nil {
		return ErrNoPractitioners
	}

	return nil
}

type practitionerHandlers struct {
	deps PractitionerDeps
}

// PractitionerHandlers is contracts/practitioners.md's five operations.
func PractitionerHandlers(deps PractitionerDeps) (httproute.Handlers, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	h := &practitionerHandlers{deps: deps}

	return httproute.Handlers{
		OpListPractitioners:  web.WithActor(h.list),
		OpCreatePractitioner: web.WithActor(h.create),
		OpGetPractitioner:    web.WithActor(h.get),
		OpUpdatePractitioner: web.WithActor(h.update),
		OpDeletePractitioner: web.WithActor(h.remove),
	}, nil
}

// list serves both the directory page and the type-ahead behind every
// practitioner picker (FR-039): `?q=` with a short prefix and `?limit=10` is
// the autocomplete call, and there is no separate operation for it.
func (h *practitionerHandlers) list(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	params, err := web.ListQuery(e, practitionersvc.Sorts())
	if err != nil {
		return err
	}

	query := practitionersvc.Query{
		Search: params.Search,
		Sort:   params.Sort,
		Limit:  params.Limit,
		Cursor: params.Cursor,
		Count:  params.Count,
	}

	raw := e.Request.URL.Query()
	query.Specialty = directory.Specialty(raw.Get(practitionersvc.FilterSpecialty))
	query.FacilityID = raw.Get(practitionersvc.FilterFacility)

	page, err := service.List(e.Request.Context(), actor, query)
	if err != nil {
		return web.OwnerScoped(err)
	}

	items := make([]any, 0, len(page.Items))

	for _, p := range page.Items {
		ref, refErr := h.facilityRef(e.Request.Context(), actor, p.FacilityID)
		if refErr != nil {
			return web.OwnerScoped(refErr)
		}

		items = append(items, practitionerSummary(p, ref))
	}

	envelope := domain.NewPage(items, page.NextCursor)
	if page.Total != nil {
		envelope = envelope.WithTotal(*page.Total)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return web.WriteJSON(e, http.StatusOK, envelope)
}

func (h *practitionerHandlers) create(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	var body PractitionerCreate
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	draft := practitionerDraft(body)

	created, err := service.Create(e.Request.Context(), actor, draft)
	if err != nil {
		return h.invalid(e, actor, draft, true, err)
	}

	if wantsFormPatch(e) && h.deps.Forms != nil {
		component, formErr := h.deps.Forms.Created(e.Request.Context(), actor, created)
		if formErr != nil {
			return formErr
		}

		return web.Patch(e, component, web.ByElementID())
	}

	return h.writeDetail(e, actor, service, http.StatusCreated, created, true)
}

// invalid answers a rejected create or update with the form re-rendered from
// what was submitted, patched into place by its own id.
func (h *practitionerHandlers) invalid(
	e *core.RequestEvent, actor access.Actor, submitted directory.Practitioner, isNew bool, err error,
) error {
	var invalid *domain.ValidationError
	if !errors.As(err, &invalid) {
		return web.OwnerScoped(err)
	}

	if !wantsFormPatch(e) || h.deps.Forms == nil {
		return web.OwnerScoped(err)
	}

	component, formErr := h.deps.Forms.Invalid(e.Request.Context(), actor, submitted, isNew, invalid)
	if formErr != nil {
		return formErr
	}

	return web.Patch(e, component, web.ByElementID())
}

func (h *practitionerHandlers) get(e *core.RequestEvent, actor access.Actor) error {
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

func (h *practitionerHandlers) update(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	var body PractitionerPatch
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	updated, err := service.Update(e.Request.Context(), actor, id, version, practitionerPatch(body))
	if err != nil {
		if errors.Is(err, domain.ErrVersionMismatch) {
			return h.stale(e, actor, service, id, err)
		}

		return h.invalidUpdate(e, actor, service, id, err)
	}

	if wantsFormPatch(e) && h.deps.Forms != nil {
		component, formErr := h.deps.Forms.Updated(e.Request.Context(), actor, updated)
		if formErr != nil {
			return formErr
		}

		return web.Patch(e, component, web.ByElementID())
	}

	return h.writeDetail(e, actor, service, http.StatusOK, updated, false)
}

// invalidUpdate re-reads the record rather than overlaying the submitted
// patch onto it: a patch is partial by design and the fields it left
// untouched have no submitted value to show.
func (h *practitionerHandlers) invalidUpdate(
	e *core.RequestEvent, actor access.Actor, service *practitionersvc.Service, id string, err error,
) error {
	var invalid *domain.ValidationError
	if !errors.As(err, &invalid) {
		return web.OwnerScoped(err)
	}

	if !wantsFormPatch(e) || h.deps.Forms == nil {
		return web.OwnerScoped(err)
	}

	current, readErr := service.Get(e.Request.Context(), actor, id)
	if readErr != nil {
		return web.OwnerScoped(readErr)
	}

	return h.invalid(e, actor, current, false, err)
}

func (h *practitionerHandlers) remove(e *core.RequestEvent, actor access.Actor) error {
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

// stale turns a version mismatch into the 412 the contract specifies, carrying
// the current representation, exactly as records.go's recordHandlers.stale
// does.
func (h *practitionerHandlers) stale(
	e *core.RequestEvent, actor access.Actor, service *practitionersvc.Service, id string, err error,
) error {
	if !errors.Is(err, domain.ErrVersionMismatch) {
		return web.OwnerScoped(err)
	}

	current, readErr := service.Get(e.Request.Context(), actor, id)
	if readErr != nil {
		return web.OwnerScoped(readErr)
	}

	if wantsFormPatch(e) && h.deps.Forms != nil {
		component, formErr := h.deps.Forms.Stale(e.Request.Context(), actor, current)
		if formErr != nil {
			return formErr
		}

		web.SetETag(e, current.Version)

		return web.Patch(e, component, web.ByElementID())
	}

	ref, refErr := h.facilityRef(e.Request.Context(), actor, current.FacilityID)
	if refErr != nil {
		return web.OwnerScoped(refErr)
	}

	usage, usageErr := service.Usage(e.Request.Context(), actor, id)
	if usageErr != nil {
		return web.OwnerScoped(usageErr)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)

	return web.WriteVersionMismatch(e, obs.CorrelationID(e.Request.Context()),
		current.Version, practitionerDetail(current, ref, usage))
}

func (h *practitionerHandlers) writeDetail(
	e *core.RequestEvent, actor access.Actor, service *practitionersvc.Service,
	status int, p directory.Practitioner, created bool,
) error {
	ctx := e.Request.Context()

	ref, err := h.facilityRef(ctx, actor, p.FacilityID)
	if err != nil {
		return web.OwnerScoped(err)
	}

	usage, err := service.Usage(ctx, actor, p.ID)
	if err != nil {
		return web.OwnerScoped(err)
	}

	if created {
		e.Response.Header().Set("Location", practitionersBasePath+"/"+p.ID)
	}

	e.Response.Header().Set("Cache-Control", directoryCacheControl)
	web.SetETag(e, p.Version)

	return web.WriteJSON(e, status, practitionerDetail(p, ref, usage))
}

// facilityRef reads the facility a practitioner names, as the same actor: a
// facility a practitioner legitimately points at is always this owner's
// (FR-042 refuses any other at write time), so the read cannot fail on
// ownership grounds. A facility that no longer exists — deleted between the
// two reads — renders as no reference rather than failing the practitioner's
// own response.
func (h *practitionerHandlers) facilityRef(ctx context.Context, actor access.Actor, facilityID string) (*FacilityRef, error) {
	if facilityID == "" || h.deps.Facilities == nil {
		return nil, nil
	}

	facilities, err := h.deps.Facilities()
	if err != nil {
		return nil, err
	}

	found, err := facilities.Get(ctx, actor, facilityID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return facilityRefFor(&found), nil
}
