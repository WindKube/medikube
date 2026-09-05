package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/domain/person"
	"medikube/internal/httproute"
	"medikube/internal/service/patient"
	"medikube/internal/web"
	"medikube/internal/web/views/shell"
)

// OpSetActivePatient is contracts/active-patient.md's one write.
const OpSetActivePatient = "setActivePatient"

// ActivePatientBody is the request PUT /api/v1/me/active-patient carries. A
// missing member and an explicit null both clear the pointer (there is
// nothing else "absent" could mean for a whole-value replace), so this is a
// plain pointer rather than web.Optional.
type ActivePatientBody struct {
	Patient *string `json:"patient"`
}

// ActivePatientResponse is the 200 body: the newly active patient, or null.
type ActivePatientResponse struct {
	ActivePatient *PatientSummary `json:"active_patient"`
}

type activePatientHandlers struct {
	resolve PatientResolve
}

// ActivePatientHandlers is contracts/active-patient.md's one operation,
// registered separately from AccountHandlers because it needs the patient
// stack and not only the identity one.
func ActivePatientHandlers(resolve PatientResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, ErrNoPatients
	}

	h := &activePatientHandlers{resolve: resolve}

	return httproute.Handlers{
		OpSetActivePatient: web.WithActor(h.set),
	}, nil
}

// set authorizes the target before writing the pointer (FR-020) and audits
// the change as switch_patient (FR-045). A Datastar @put asking for
// text/html gets the re-rendered switcher back as a plain element patch
// (research D-33); everything else gets the JSON envelope.
func (h *activePatientHandlers) set(e *core.RequestEvent, actor access.Actor) error {
	svc, err := h.resolve()
	if err != nil {
		return err
	}

	var body ActivePatientBody
	if decodeErr := web.Decode(e, &body); decodeErr != nil {
		return decodeErr
	}

	active, err := svc.SetActivePatient(e.Request.Context(), actor, body.Patient)
	if err != nil {
		return err
	}

	var summary *PatientSummary
	if active != nil {
		rendered := NewPatientSummary(*active, nil)
		summary = &rendered
	}

	if wantsHTML(e) {
		props, propsErr := h.switcherProps(e.Request.Context(), actor, svc, active)
		if propsErr != nil {
			return propsErr
		}

		return web.Patch(e, shell.PatientSwitcher(props), web.ByElementID())
	}

	return web.WriteJSON(e, http.StatusOK, ActivePatientResponse{ActivePatient: summary})
}

// switcherProps rebuilds the whole control, not only the newly active
// option: the response replaces the element outright (web.ByElementID), so a
// partial render would drop every other patient the account holder could
// still switch to.
func (h *activePatientHandlers) switcherProps(
	ctx context.Context,
	actor access.Actor,
	svc *patient.Service,
	active *person.Patient,
) (shell.PatientSwitcherProps, error) {
	listing, err := svc.List(ctx, actor, patient.Query{Limit: switcherPropsLimit})
	if err != nil {
		return shell.PatientSwitcherProps{}, err
	}

	var activeID string
	if active != nil {
		activeID = active.ID
	}

	options := make([]shell.PatientOption, 0, len(listing.Items))
	for _, p := range listing.Items {
		var photoURL string
		if p.HasPhoto {
			photoURL = PatientPhotoURL(p.ID)
		}

		options = append(options, shell.PatientOption{
			ID:        p.ID,
			Name:      p.FirstName + " " + p.LastName,
			BirthDate: p.BirthDate.String(),
			PhotoURL:  photoURL,
			Active:    p.ID == activeID,
		})
	}

	href, err := routePath(OpSetActivePatient)
	if err != nil {
		return shell.PatientSwitcherProps{}, err
	}

	return shell.PatientSwitcherProps{Options: options, Href: href}, nil
}

// switcherPropsLimit mirrors internal/web/page's own switcherLimit: generous
// rather than exact, so an account with more patients than a small page size
// does not silently lose the ability to switch to the rest.
const switcherPropsLimit = 100

// wantsHTML is the Datastar @put's own negotiation: the runtime's non-SSE
// path is a plain fetch with Accept: text/html, never the
// Datastar-Request header IsDatastarRequest keys on for a JSON body — the
// switcher's morph and the JSON API share one operation and need to be told
// apart by what the caller said it can read.
func wantsHTML(e *core.RequestEvent) bool {
	return strings.Contains(e.Request.Header.Get("Accept"), "text/html")
}
