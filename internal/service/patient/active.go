package patient

import (
	"context"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/person"
)

// SetActivePatient is contracts/active-patient.md's setActivePatient: the
// target is authorized before the pointer is written (FR-020), so a stale or
// forged id never reaches the column, and the change is audited as
// switch_patient (FR-045). A nil or empty patientID clears the pointer, which
// is never authorized against anything — there is nothing to own.
func (s *Service) SetActivePatient(ctx context.Context, actor access.Actor, patientID *string) (*person.Patient, error) {
	ctx, end := s.span(ctx, "service.patient.SetActivePatient", nil)

	active, outcome, err := s.setActivePatient(ctx, actor, patientID)
	s.metricsOrNoop().PatientSwitch(outcome)
	end(err)

	return active, err
}

// setActivePatient returns the switch's outcome alongside its result so the
// caller can report medikube_patients_switch_total{outcome} for every path,
// including the two that answer with a nil error: clearing the pointer and
// choosing one.
func (s *Service) setActivePatient(ctx context.Context, actor access.Actor, patientID *string) (*person.Patient, string, error) {
	if !actor.Authenticated() {
		return nil, "unauthenticated", domain.ErrUnauthenticated
	}

	if patientID == nil || *patientID == "" {
		if err := s.pointer.SetActivePatient(ctx, actor.UserID, ""); err != nil {
			return nil, "error", err
		}

		return nil, "cleared", nil
	}

	if _, err := s.authorizer.Patient(ctx, actor, *patientID, access.PermView); err != nil {
		return nil, "not_found", err
	}

	chosen, err := s.repository.Get(ctx, actor.UserID, *patientID)
	if err != nil {
		return nil, "not_found", err
	}

	if err := s.pointer.SetActivePatient(ctx, actor.UserID, chosen.ID); err != nil {
		return nil, "error", err
	}

	if err := s.auditor.Record(ctx, switchEvent(actor, chosen.ID)); err != nil {
		return nil, "error", err
	}

	return &chosen, "ok", nil
}

// ResolveActivePatient answers the pointer as contracts/active-patient.md's
// getMe resolves it: null when unset or unreachable (FR-017), and — FR-018 —
// auto-selected and persisted as a side effect when exactly one patient is
// reachable, so a new account never faces an empty application.
func (s *Service) ResolveActivePatient(ctx context.Context, actor access.Actor) (*person.Patient, error) {
	if !actor.Authenticated() {
		return nil, domain.ErrUnauthenticated
	}

	pointer, err := s.pointer.ActivePatient(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}

	if pointer != "" {
		if patient, ok := s.reachable(ctx, actor, pointer); ok {
			return &patient, nil
		}
	}

	page, err := s.repository.List(ctx, actor.UserID, Query{Limit: 1, Count: true})
	if err != nil {
		return nil, err
	}

	if page.Total == nil || *page.Total != 1 || len(page.Items) != 1 {
		return nil, nil
	}

	chosen := page.Items[0]

	if err := s.pointer.SetActivePatient(ctx, actor.UserID, chosen.ID); err != nil {
		return nil, err
	}

	return &chosen, nil
}

// reachable answers whether the actor may still view the pointer, and the
// row when they can. A pointer that fails authorization or no longer exists
// resolves to false rather than an error: FR-017 is a null, never a refusal.
func (s *Service) reachable(ctx context.Context, actor access.Actor, patientID string) (person.Patient, bool) {
	if _, err := s.authorizer.Patient(ctx, actor, patientID, access.PermView); err != nil {
		return person.Patient{}, false
	}

	patient, err := s.repository.Get(ctx, actor.UserID, patientID)
	if err != nil {
		return person.Patient{}, false
	}

	return patient, true
}

func switchEvent(actor access.Actor, patientID string) audit.Event {
	event := audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionSwitchPatient,
		TargetKind: audit.TargetKindPatient,
		TargetID:   patientID,
		PatientID:  patientID,
		RequestID:  actor.RequestID,
	}

	if actor.IsSuperuser {
		event.ActorKind = audit.ActorKindSuperuser
	}

	return event
}
