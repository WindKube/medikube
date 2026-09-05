package audit

import (
	"time"
	"unicode/utf8"

	"medikube/internal/domain"
)

// The bounds data-model §3 sizes the two text columns at. They are constants
// here and the same number in the migration, because the domain refusing what
// the column would refuse is what keeps an audit write from failing at the
// storage layer, after the thing it records has already happened.
const (
	// MaxTargetID holds the longest identifier any phase composes — a record id
	// is 15, and phase 006's restore safety copy is the long case data-model §3
	// works out. event_test.go asserts that name still fits.
	MaxTargetID = 64

	MaxRequestID = 64

	// MaxPatientID is a bare PocketBase record id and nothing composed, unlike
	// TargetID (constitution: "15-character opaque PocketBase text ids").
	MaxPatientID = 15
)

// Event is one thing that happened. Its defining property is negative: there is
// no field here that a value, a name, a note or a diff could be written into,
// which is what makes FR-038 structural rather than a rule somebody has to
// remember at each call site. Adding a field is therefore a spec decision, and
// event_test.go is where that decision gets stopped.
//
// It is also why the type needs no redacting log marshaller, unlike every other
// domain entity: there is nothing here to redact.
type Event struct {
	// Server clock, never a client-supplied time.
	OccurredAt time.Time

	// Empty for system and superuser actions, and emptied — not cascaded — when
	// the account is deleted, so the account_delete row outlives its actor
	// (research D-22). ActorKind still says what kind of actor it was.
	ActorID string

	ActorKind  ActorKind
	Action     Action
	TargetKind TargetKind

	// An opaque id: never a name, never a path, never a filename. The one
	// exception is a system, backup or export target, where there is no record
	// to point at and this carries the job or archive name the operator already
	// chose (data-model §3).
	TargetID string

	// Required, so a row that correlates to nothing cannot be written. A
	// background run has no HTTP request and still fills this from the run id
	// its own log lines carry.
	RequestID string

	// PatientID is the person a patient-scoped action concerned. Null for a
	// non-patient action — creating a practitioner, an admin session — and
	// unset (not cascaded) when the patient is deleted, so a historical entry
	// survives without pointing at a ghost (phase 002 data-model §5).
	PatientID string
}

// Validate reports every offending column at once. It is the last check before
// a row is written by a hook or by audit.Record; a refusal here is a MediKube
// bug rather than a person's mistake, and it is reported in the same shape as
// one so the caller has one error type to handle.
func (e Event) Validate() error {
	var invalid domain.ValidationError

	if e.OccurredAt.IsZero() {
		invalid.Add("occurred_at", domain.CodeRequired, "an occurrence time is required")
	}

	switch {
	case e.ActorKind == "":
		invalid.Add("actor_kind", domain.CodeRequired, "an actor kind is required")
	case !e.ActorKind.Valid():
		invalid.Add("actor_kind", domain.CodeInvalidValue, "the actor kind is not one of the published kinds")
	}

	switch {
	case e.Action == "":
		invalid.Add("action", domain.CodeRequired, "an action is required")
	case !e.Action.Valid():
		invalid.Add("action", domain.CodeInvalidValue, "the action is not one of the published actions")
	}

	switch {
	case e.TargetKind == "":
		invalid.Add("target_kind", domain.CodeRequired, "a target kind is required")
	case !e.TargetKind.Valid():
		invalid.Add("target_kind", domain.CodeInvalidValue, "the target kind is not one of the published kinds")
	}

	if utf8.RuneCountInString(e.TargetID) > MaxTargetID {
		invalid.Addf("target_id", domain.CodeTooLong, "a target id is at most %d characters", MaxTargetID)
	}

	switch {
	case e.RequestID == "":
		invalid.Add("request_id", domain.CodeRequired, "a request id is required")
	case utf8.RuneCountInString(e.RequestID) > MaxRequestID:
		invalid.Addf("request_id", domain.CodeTooLong, "a request id is at most %d characters", MaxRequestID)
	}

	if utf8.RuneCountInString(e.PatientID) > MaxPatientID {
		invalid.Addf("patient", domain.CodeTooLong, "a patient id is at most %d characters", MaxPatientID)
	}

	return invalid.OrNil()
}
