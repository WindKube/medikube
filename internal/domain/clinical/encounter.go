package clinical

import (
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Encounter is one visit (FR-022, data-model §4.3).
type Encounter struct {
	ID        string
	PatientID string

	Reason      string
	OccurredOn  domain.Date
	VisitType   VisitType
	Priority    VisitPriority
	Assessment  string
	Plan        string
	FollowUp    string
	DurationMin int

	PractitionerID string
	FacilityID     string
	ConditionID    string

	Notes string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

// MarshalZerologObject emits the two identifiers and nothing else (FR-038's
// pattern, applied here): reason, assessment, plan and follow-up are all PHI.
func (e Encounter) MarshalZerologObject(ev *zerolog.Event) {
	ev.Str("encounter_id", e.ID).Str("patient_id", e.PatientID)
}

const (
	maxEncounterReason   = 300
	maxEncounterFreeText = 5000
	maxEncounterFollowUp = 2000
)

// Validate reports every offending field at once, in data-model §4.3's column
// order (FR-022's "assessment and plan are never mapped to or from a
// diagnosed condition" is satisfied by shape: ConditionID is a plain
// reference, never read from or written into Assessment or Plan).
func (e Encounter) Validate() error {
	var invalid domain.ValidationError

	requireText(&invalid, "reason", "the reason for the visit", e.Reason, 1, maxEncounterReason)

	if e.OccurredOn.IsZero() {
		invalid.Add("occurred_on", domain.CodeRequired, "the date it happened is required")
	}

	if e.VisitType != "" && !e.VisitType.Valid() {
		invalid.Add("visit_type", domain.CodeInvalidValue, "not one of the visit types MediKube accepts")
	}

	if e.Priority != "" && !e.Priority.Valid() {
		invalid.Add("priority", domain.CodeInvalidValue, "not one of the priorities MediKube accepts")
	}

	checkLength(&invalid, "assessment", "the assessment", e.Assessment, maxEncounterFreeText)
	checkLength(&invalid, "plan", "the plan", e.Plan, maxEncounterFreeText)
	checkLength(&invalid, "follow_up", "the follow-up", e.FollowUp, maxEncounterFollowUp)

	if e.DurationMin < 0 {
		invalid.Add("duration_minutes", domain.CodeOutOfRange, "duration cannot be negative")
	}

	checkLength(&invalid, "notes", "the notes", e.Notes, maxNotes)

	return invalid.OrNil()
}
