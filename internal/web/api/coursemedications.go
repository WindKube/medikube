package api

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	coursemedicationsvc "medikube/internal/service/coursemedication"
	"medikube/internal/web"
)

// The three operation ids of contracts/treatment-medications.md.
const (
	OpListCourseMedications  = "listCourseMedications"
	OpUpsertCourseMedication = "upsertCourseMedication"
	OpDeleteCourseMedication = "deleteCourseMedication"
)

// PathMedicationID is the join route's second path parameter, alongside
// records.go's own PathID for the treatment.
const PathMedicationID = "medicationId"

// courseMedicationCacheControl mirrors every other clinical response
// (recordCacheControl): a course medication is somebody's dosage, never
// cached and never stored on disk by an intermediary.
const courseMedicationCacheControl = "private, no-store"

// CourseMedicationResolve resolves the service once, on first use — the same
// reason records.Resolve is a function (records.go documents the mechanism).
type CourseMedicationResolve func() (*coursemedicationsvc.Service, error)

// ErrNoCourseMedications is a build whose service was never resolved.
var ErrNoCourseMedications = errors.New("api: the course-medication operations were wired without a way to resolve the service")

// CourseMedicationDeps is what the three handlers need. Records is the
// record family's own resolver, reused here only to answer a stale If-Match
// with the treatment's current representation — the same 412 shape
// updateRecord/deleteRecord already answer with, references.go's
// `references` field included: a stale If-Match on a course medication still
// re-reads the treatment, and that representation must carry the same field
// getRecord would put on it.
type CourseMedicationDeps struct {
	Resolve    CourseMedicationResolve
	Records    Resolve
	References ReferencesResolve
}

func (d CourseMedicationDeps) validate() error {
	if d.Resolve == nil || d.Records == nil {
		return ErrNoCourseMedications
	}

	return nil
}

type courseMedicationHandlers struct {
	deps CourseMedicationDeps
}

// CourseMedicationHandlers is contracts/treatment-medications.md's three
// operations.
func CourseMedicationHandlers(deps CourseMedicationDeps) (httproute.Handlers, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	h := &courseMedicationHandlers{deps: deps}

	return httproute.Handlers{
		OpListCourseMedications:  web.WithActor(h.list),
		OpUpsertCourseMedication: web.WithActor(h.upsert),
		OpDeleteCourseMedication: web.WithActor(h.remove),
	}, nil
}

// CourseMedicationEffective is FR-060's `{value, source}` pair.
type CourseMedicationEffective struct {
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// CourseMedicationItem is one `items` entry of `listCourseMedications` and
// the whole body of `upsertCourseMedication`'s response
// (contracts/treatment-medications.md §1-2).
type CourseMedicationItem struct {
	Medication *MedicationSummary `json:"medication"`

	EffectiveDosage     CourseMedicationEffective `json:"effective_dosage"`
	EffectiveFrequency  CourseMedicationEffective `json:"effective_frequency"`
	EffectiveDuration   CourseMedicationEffective `json:"effective_duration"`
	EffectiveTiming     CourseMedicationEffective `json:"effective_timing"`
	EffectivePrescriber CourseMedicationEffective `json:"effective_prescriber"`
	EffectivePharmacy   CourseMedicationEffective `json:"effective_pharmacy"`
	EffectiveStartedOn  CourseMedicationEffective `json:"effective_started_on"`
	EffectiveEndedOn    CourseMedicationEffective `json:"effective_ended_on"`

	UpdatedAt string `json:"updated_at"`
}

// CourseMedicationPut is the upsert body. Every field is optional: an absent
// one falls back to the medication's own value (FR-060).
type CourseMedicationPut struct {
	Dosage     *string `json:"dosage,omitempty"`
	Frequency  *string `json:"frequency,omitempty"`
	Duration   *string `json:"duration,omitempty"`
	Timing     *string `json:"timing,omitempty"`
	Prescriber *string `json:"prescriber,omitempty"`
	Pharmacy   *string `json:"pharmacy,omitempty"`
	StartedOn  *string `json:"started_on,omitempty"`
	EndedOn    *string `json:"ended_on,omitempty"`
}

// list is `GET /records/treatments/{id}/medications`.
func (h *courseMedicationHandlers) list(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	treatmentID := e.Request.PathValue(PathID)

	items, err := service.List(e.Request.Context(), actor, treatmentID)
	if err != nil {
		return web.OwnerScoped(err)
	}

	body := make([]any, 0, len(items))
	for _, item := range items {
		body = append(body, courseMedicationItem(item))
	}

	envelope := domain.NewPage(body, (*string)(nil))

	e.Response.Header().Set("Cache-Control", courseMedicationCacheControl)

	return web.WriteJSON(e, http.StatusOK, envelope)
}

// upsert is `PUT /records/treatments/{id}/medications/{medicationId}`
// (FR-061): idempotent attach, `200` on update and `201` on create.
func (h *courseMedicationHandlers) upsert(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	treatmentID, medicationID := e.Request.PathValue(PathID), e.Request.PathValue(PathMedicationID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	var body CourseMedicationPut
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	var invalid domain.ValidationError

	patch := coursemedicationsvc.Patch{
		Dosage: body.Dosage, Frequency: body.Frequency, Duration: body.Duration, Timing: body.Timing,
		PrescriberID: body.Prescriber, PharmacyID: body.Pharmacy,
		StartedOn: readPatchDate(&invalid, "started_on", body.StartedOn),
		EndedOn:   readPatchDate(&invalid, "ended_on", body.EndedOn),
	}

	if invalidErr := invalid.OrNil(); invalidErr != nil {
		return invalidErr
	}

	item, created, err := service.Upsert(e.Request.Context(), actor, treatmentID, medicationID, patch, version)
	if err != nil {
		return h.failure(e, actor, treatmentID, err)
	}

	e.Response.Header().Set("Cache-Control", courseMedicationCacheControl)

	if created {
		e.Response.Header().Set("Location", e.Request.URL.Path)

		return web.WriteJSON(e, http.StatusCreated, courseMedicationItem(item))
	}

	return web.WriteJSON(e, http.StatusOK, courseMedicationItem(item))
}

// remove is `DELETE /records/treatments/{id}/medications/{medicationId}`
// (FR-058): removes the link row only, leaving both ends intact.
func (h *courseMedicationHandlers) remove(e *core.RequestEvent, actor access.Actor) error {
	service, err := h.deps.Resolve()
	if err != nil {
		return err
	}

	treatmentID, medicationID := e.Request.PathValue(PathID), e.Request.PathValue(PathMedicationID)

	version, err := web.IfMatch(e)
	if err != nil {
		return err
	}

	if err := service.Delete(e.Request.Context(), actor, treatmentID, medicationID, version); err != nil {
		return h.failure(e, actor, treatmentID, err)
	}

	e.Response.Header().Set("Cache-Control", courseMedicationCacheControl)

	return e.NoContent(http.StatusNoContent)
}

// failure turns a stale If-Match into the 412 the contract specifies,
// carrying the treatment's current representation — the same shape
// updateRecord/deleteRecord answer a stale precondition with — and hands
// everything else on unchanged.
func (h *courseMedicationHandlers) failure(e *core.RequestEvent, actor access.Actor, treatmentID string, err error) error {
	if !errors.Is(err, domain.ErrVersionMismatch) {
		return web.OwnerScoped(err)
	}

	handler, resolveErr := h.deps.Records()
	if resolveErr != nil {
		return resolveErr
	}

	current, readErr := handler.Get(e.Request.Context(), actor, kind.Treatment.Collection(), treatmentID)
	if readErr != nil {
		return web.OwnerScoped(readErr)
	}

	if refErr := attachReferences(e.Request.Context(), h.deps.References, current.Kind, current.ID, current.Body); refErr != nil {
		return refErr
	}

	e.Response.Header().Set("Cache-Control", courseMedicationCacheControl)

	return web.WriteVersionMismatch(e, obs.CorrelationID(e.Request.Context()), current.Version, current.Body)
}

func courseMedicationItem(item coursemedicationsvc.Item) CourseMedicationItem {
	summary, _ := MedicationCodec{}.Summary(item.Medication).(*MedicationSummary)

	return CourseMedicationItem{
		Medication:          summary,
		EffectiveDosage:     wireEffective(item.Effective.Dosage),
		EffectiveFrequency:  wireEffective(item.Effective.Frequency),
		EffectiveDuration:   wireEffective(item.Effective.Duration),
		EffectiveTiming:     wireEffective(item.Effective.Timing),
		EffectivePrescriber: wireEffective(item.Effective.Prescriber),
		EffectivePharmacy:   wireEffective(item.Effective.Pharmacy),
		EffectiveStartedOn:  wireEffective(item.Effective.StartedOn),
		EffectiveEndedOn:    wireEffective(item.Effective.EndedOn),
		UpdatedAt:           wireInstant(item.CourseMedication.UpdatedAt),
	}
}

// wireEffective renders one `{value, source}` pair. Value is `any` on the
// domain side because Resolve's eight fields are not all strings — the two
// dates are domain.Date — so this is the one place that decides how each
// underlying Go type is written on the wire.
func wireEffective(effective clinical.Effective) CourseMedicationEffective {
	return CourseMedicationEffective{Value: effectiveValue(effective.Value), Source: string(effective.Source)}
}

func effectiveValue(value any) any {
	if date, ok := value.(domain.Date); ok {
		return wireDate(date)
	}

	return value
}

// readPatchDate parses an optional wire date for a PUT body: absent stays
// absent (fall back to the medication), and a submitted one is parsed the
// same way every other date on the wire is.
func readPatchDate(invalid *domain.ValidationError, member string, raw *string) *domain.Date {
	if raw == nil {
		return nil
	}

	parsed := readDate(invalid, member, raw)

	return &parsed
}
