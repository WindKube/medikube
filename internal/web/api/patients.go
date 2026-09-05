package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/person"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/service/patient"
	"medikube/internal/web"
)

// The operation ids of contracts/patients.md's four CRUD operations.
const (
	OpListPatients  = "listPatients"
	OpCreatePatient = "createPatient"
	OpGetPatient    = "getPatient"
	OpUpdatePatient = "updatePatient"
)

// PathPatientID is the one path parameter every patient and photo route
// carries.
const PathPatientID = "patientId"

// PatientOperations is every operation id contracts/patients.md and
// contracts/patient-photo.md serve, so cmd/medikube's stub inventory knows
// these seven are wired without importing this package's handler
// construction.
func PatientOperations() []string {
	return []string{
		OpListPatients, OpCreatePatient, OpGetPatient, OpUpdatePatient,
		OpPutPatientPhoto, OpGetPatientPhoto, OpDeletePatientPhoto,
	}
}

// patientCacheControl mirrors internal/web/api/records.go's own: a patient's
// record is somebody's medical data and must never sit in a shared cache or on
// disk.
const patientCacheControl = "private, no-store"

// UnitSystemOf resolves the actor's own display preference. It is a function
// rather than a direct dependency on *identity.Service so this file's own
// tests can fake it without building the identity stack.
type UnitSystemOf func(ctx context.Context, actor access.Actor) (identity.UnitSystem, error)

// PatientResolve hands the generic composition root its patient stack, once,
// the same way api.Resolve does for the kind registry (records.go) — the
// repository needs the cursor codec, which is keyed from a secret the
// migrations create, so it cannot be built before they have run.
type PatientResolve func() (*patient.Service, error)

// ErrNoPatients is a build whose patient stack was never resolved.
var ErrNoPatients = &web.Coded{Status: http.StatusInternalServerError, Code: web.CodeInternal}

// SelfRecordFunc provisions the one patient FR-005 guarantees for every
// account: at registration. It is a function, not a direct dependency on
// *patient.Service, for the same reason PatientResolve is one: auth.go's own
// tests fake it without building the patient stack.
type SelfRecordFunc func(ctx context.Context, ownerID, displayName string) (person.Patient, error)

// SelfRecordOf adapts a PatientResolve into a SelfRecordFunc, resolving the
// patient stack lazily on the first registration rather than at boot.
func SelfRecordOf(resolve PatientResolve) SelfRecordFunc {
	return func(ctx context.Context, ownerID, displayName string) (person.Patient, error) {
		service, err := resolve()
		if err != nil {
			return person.Patient{}, err
		}

		return service.CreateSelfRecord(ctx, ownerID, displayName)
	}
}

type patientHandlers struct {
	resolve  PatientResolve
	unitOf   UnitSystemOf
	photoURL func(id string) string
}

// PatientHandlers is contracts/patients.md's four CRUD operations.
func PatientHandlers(resolve PatientResolve, unitOf UnitSystemOf) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, ErrNoPatients
	}

	if unitOf == nil {
		return nil, ErrNoPatients
	}

	h := &patientHandlers{
		resolve: resolve,
		unitOf:  unitOf,
		photoURL: func(id string) string {
			return "/api/v1/patients/" + id + "/photo"
		},
	}

	return httproute.Handlers{
		OpListPatients:  web.WithActor(h.list),
		OpCreatePatient: web.WithActor(h.create),
		OpGetPatient:    web.WithActor(h.get),
		OpUpdatePatient: web.WithActor(h.update),
	}, nil
}

// list answers only the actor's own patients (FR-042). `total` and
// `owned_count` are unconditional, not gated by `?count=true` (FR-010).
func (h *patientHandlers) list(e *core.RequestEvent, actor access.Actor) error {
	svc, err := h.resolve()
	if err != nil {
		return err
	}

	params, err := web.ListQuery(e, web.PatientsSort())
	if err != nil {
		return err
	}

	page, err := svc.List(e.Request.Context(), actor, patient.Query{
		Search: params.Search,
		Sort:   params.Sort,
		Limit:  params.Limit,
		Cursor: params.Cursor,
		Count:  true,
	})
	if err != nil {
		return err
	}

	items := make([]any, 0, len(page.Items))
	for _, p := range page.Items {
		items = append(items, h.summary(p))
	}

	envelope := struct {
		Items      []any   `json:"items"`
		NextCursor *string `json:"next_cursor"`
		Total      int     `json:"total"`
		OwnedCount int     `json:"owned_count"`
	}{
		Items:      items,
		NextCursor: page.NextCursor,
	}

	if page.Total != nil {
		envelope.Total = *page.Total
		envelope.OwnedCount = *page.Total
	}

	e.Response.Header().Set("Cache-Control", patientCacheControl)

	return web.WriteJSON(e, http.StatusOK, envelope)
}

func (h *patientHandlers) create(e *core.RequestEvent, actor access.Actor) error {
	svc, err := h.resolve()
	if err != nil {
		return err
	}

	var body PatientCreate
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	draft, err := body.Draft()
	if err != nil {
		return err
	}

	created, err := svc.Create(e.Request.Context(), actor, draft)
	if err != nil {
		return err
	}

	rendered, err := h.detail(e.Request.Context(), actor, created)
	if err != nil {
		return err
	}

	e.Response.Header().Set("Location", "/api/v1/patients/"+created.ID)
	e.Response.Header().Set("Cache-Control", patientCacheControl)
	web.SetETag(e, created.Version)

	return web.WriteJSON(e, http.StatusCreated, rendered)
}

func (h *patientHandlers) get(e *core.RequestEvent, actor access.Actor) error {
	svc, err := h.resolve()
	if err != nil {
		return err
	}

	found, err := svc.Get(e.Request.Context(), actor, e.Request.PathValue(PathPatientID))
	if err != nil {
		return err
	}

	rendered, err := h.detail(e.Request.Context(), actor, found)
	if err != nil {
		return err
	}

	e.Response.Header().Set("Cache-Control", patientCacheControl)
	web.SetETag(e, found.Version)

	return web.WriteJSON(e, http.StatusOK, rendered)
}

// update requires If-Match, and a stale one answers 412 carrying the current
// representation (FR-011, US1-7).
func (h *patientHandlers) update(e *core.RequestEvent, actor access.Actor) error {
	svc, err := h.resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathPatientID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	var body PatientPatch
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	patch, err := body.ToServicePatch()
	if err != nil {
		return err
	}

	updated, updateErr := svc.Update(e.Request.Context(), actor, id, version, patch)
	if updateErr != nil {
		return h.stale(e, actor, svc, id, updateErr)
	}

	rendered, err := h.detail(e.Request.Context(), actor, updated)
	if err != nil {
		return err
	}

	e.Response.Header().Set("Cache-Control", patientCacheControl)
	web.SetETag(e, updated.Version)

	return web.WriteJSON(e, http.StatusOK, rendered)
}

func (h *patientHandlers) stale(e *core.RequestEvent, actor access.Actor, svc *patient.Service, id string, err error) error {
	if !errors.Is(err, domain.ErrVersionMismatch) {
		return err
	}

	current, readErr := svc.Get(e.Request.Context(), actor, id)
	if readErr != nil {
		return readErr
	}

	rendered, renderErr := h.detail(e.Request.Context(), actor, current)
	if renderErr != nil {
		return renderErr
	}

	e.Response.Header().Set("Cache-Control", patientCacheControl)

	return web.WriteVersionMismatch(e, obs.CorrelationID(e.Request.Context()), current.Version, rendered)
}

func (h *patientHandlers) summary(p person.Patient) any {
	var photoURL *string
	if p.HasPhoto {
		url := h.photoURL(p.ID)
		photoURL = &url
	}

	return NewPatientSummary(p, photoURL)
}

func (h *patientHandlers) detail(ctx context.Context, actor access.Actor, p person.Patient) (Patient, error) {
	system, err := h.unitOf(ctx, actor)
	if err != nil {
		return Patient{}, err
	}

	var photoURL *string
	if p.HasPhoto {
		url := h.photoURL(p.ID)
		photoURL = &url
	}

	// The primary practitioner is not yet resolved to a name/specialty in
	// this build (the practitioner directory is a sibling story); the id is
	// carried on the domain entity and this renders no reference rather than
	// fabricate one.
	return NewPatient(p, photoURL, system, nil), nil
}
