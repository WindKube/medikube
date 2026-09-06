package clinical

import (
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Treatment is one course of treatment (FR-027, data-model §4.5). Encounters
// and Equipment are FR-028's two multi-relations and are wired here since both
// targets are this story's own kinds.
type Treatment struct {
	ID        string
	PatientID string

	Name            string
	Type            string
	Setting         TreatmentSetting
	Description     string
	StartedOn       domain.Date
	EndedOn         domain.Date
	Frequency       string
	Dosage          string
	ExpectedOutcome string
	Status          TherapyStatus

	PractitionerID string
	FacilityID     string
	ConditionID    string

	// Encounters and Equipment are FR-028's multi-relations: a set of ids, not
	// an order. LinkSet is what compares two sets for FR-056's idempotent-add;
	// the store is what turns this into a replace-set write.
	Encounters []string
	Equipment  []string

	Notes string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

func (t Treatment) MarshalZerologObject(ev *zerolog.Event) {
	ev.Str("treatment_id", t.ID).Str("patient_id", t.PatientID)
}

const maxTreatmentExpectedOutcome = 300

// Validate is FR-013 applied to a course of treatment: ended_on before
// started_on is refused with both values named.
func (t Treatment) Validate() error {
	var invalid domain.ValidationError

	requireText(&invalid, "name", "the name", t.Name, 2, 300)
	checkLength(&invalid, "type", "the type", t.Type, 120)

	if t.Setting != "" && !t.Setting.Valid() {
		invalid.Add("setting", domain.CodeInvalidValue, "not one of the settings MediKube accepts")
	}

	checkLength(&invalid, "description", "the description", t.Description, 5000)

	if err := Order(Ref{Field: "started_on", Value: t.StartedOn}, Ref{Field: "ended_on", Value: t.EndedOn}); err != nil {
		invalid.Fields = append(invalid.Fields, *err)
	}

	checkLength(&invalid, "frequency", "how often", t.Frequency, 100)
	checkLength(&invalid, "dosage", "the dose", t.Dosage, 200)
	checkLength(&invalid, "expected_outcome", "the expected outcome", t.ExpectedOutcome, maxTreatmentExpectedOutcome)

	if t.Status != "" && !t.Status.Valid() {
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	checkLength(&invalid, "notes", "the notes", t.Notes, maxNotes)

	return invalid.OrNil()
}
