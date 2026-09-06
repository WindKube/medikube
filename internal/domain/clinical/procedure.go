package clinical

import (
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Procedure is one procedure (FR-024, data-model §4.4).
type Procedure struct {
	ID        string
	PatientID string

	Name            string
	Type            ProcedureType
	Code            string
	Description     string
	OccurredOn      domain.Date
	Status          OrderStatus
	Outcome         ProcedureOutcome
	Setting         ProcedureSetting
	Complications   string
	DurationMin     int
	Anesthesia      Anesthesia
	AnesthesiaNotes string

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

func (p Procedure) MarshalZerologObject(ev *zerolog.Event) {
	ev.Str("procedure_id", p.ID).Str("patient_id", p.PatientID)
}

// Scheduled is FR-026's basis: a procedure whose status is one of the two
// ordered-and-not-yet-done states.
func (p Procedure) Scheduled() bool {
	return p.Status == OrderStatusOrdered || p.Status == OrderStatusScheduled
}

const (
	maxProcedureCode            = 50
	maxProcedureDescription     = 5000
	maxProcedureComplications   = 500
	maxProcedureAnesthesiaNotes = 2000
)

// Validate is FR-025: a future occurred_on is accepted for ordered/scheduled
// and refused for completed (in_progress and cancelled are neither refused
// nor specially permitted — data-model states the rule only for the two
// endpoints of the ladder, so anything else follows the general "no rule"
// default).
func (p Procedure) Validate() error {
	var invalid domain.ValidationError

	requireText(&invalid, "name", "the name", p.Name, 2, 300)

	if p.OccurredOn.IsZero() {
		invalid.Add("occurred_on", domain.CodeRequired, "the date it happened is required")
	}

	switch {
	case p.Status == "":
		invalid.Add("status", domain.CodeRequired, "a status is required")
	case !p.Status.Valid():
		invalid.Add("status", domain.CodeInvalidValue, "not one of the statuses MediKube accepts")
	case p.Status == OrderStatusCompleted:
		if err := NotFuture(Ref{Field: "occurred_on", Value: p.OccurredOn}, Today()); err != nil {
			invalid.Fields = append(invalid.Fields, *err)
		}
	}

	if p.Type != "" && !p.Type.Valid() {
		invalid.Add("type", domain.CodeInvalidValue, "not one of the kinds MediKube accepts")
	}

	checkLength(&invalid, "code", "the code", p.Code, maxProcedureCode)
	checkLength(&invalid, "description", "the description", p.Description, maxProcedureDescription)

	if p.Outcome != "" && !p.Outcome.Valid() {
		invalid.Add("outcome", domain.CodeInvalidValue, "not one of the outcomes MediKube accepts")
	}

	if p.Setting != "" && !p.Setting.Valid() {
		invalid.Add("setting", domain.CodeInvalidValue, "not one of the settings MediKube accepts")
	}

	checkLength(&invalid, "complications", "the complications", p.Complications, maxProcedureComplications)

	if p.DurationMin < 0 {
		invalid.Add("duration_minutes", domain.CodeOutOfRange, "duration cannot be negative")
	}

	if p.Anesthesia != "" && !p.Anesthesia.Valid() {
		invalid.Add("anesthesia", domain.CodeInvalidValue, "not one of the kinds of anesthesia MediKube accepts")
	}

	checkLength(&invalid, "anesthesia_notes", "the anesthesia notes", p.AnesthesiaNotes, maxProcedureAnesthesiaNotes)
	checkLength(&invalid, "notes", "the notes", p.Notes, maxNotes)

	return invalid.OrNil()
}
