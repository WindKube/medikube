package clinical

import (
	"strings"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Condition is a diagnosis with an onset, a status and an optional resolution
// (data-model §4.2, US1).
type Condition struct {
	ID        string
	PatientID string

	Diagnosis  string
	Status     ConditionStatus
	Severity   Severity
	OnsetOn    domain.Date
	ResolvedOn domain.Date
	ICD10Code  string
	SNOMEDCode string

	PractitionerID string
	MedicationIDs  []string

	Notes string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

func (c Condition) MarshalZerologObject(e *zerolog.Event) {
	e.Str("condition_id", c.ID).Str("patient_id", c.PatientID)
}

const (
	maxDiagnosis  = 500
	maxICD10Code  = 10
	maxSNOMEDCode = 20
)

// Validate is FR-019: diagnosis and status required; status = resolved
// requires resolved_on (FR-020), which must be no earlier than onset_on and no
// later than today — and both bounds are checked so a submission violating
// both reports both fields (FR-004, FR-013).
func (c Condition) Validate() error {
	var invalid domain.ValidationError

	if diagnosis := strings.TrimSpace(c.Diagnosis); diagnosis == "" {
		invalid.Add("diagnosis", domain.CodeRequired, "a diagnosis is required")
	} else {
		checkLength(&invalid, "diagnosis", "the diagnosis", diagnosis, maxDiagnosis)
	}

	switch {
	case c.Status == "":
		invalid.Add("status", domain.CodeRequired, "a state is required")
	case !c.Status.Valid():
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	if c.Severity != "" && !c.Severity.Valid() {
		invalid.Add("severity", domain.CodeInvalidValue, "not one of the severities MediKube accepts")
	}

	checkLength(&invalid, "icd10_code", "the ICD-10 code", c.ICD10Code, maxICD10Code)
	checkLength(&invalid, "snomed_code", "the SNOMED code", c.SNOMEDCode, maxSNOMEDCode)
	checkLength(&invalid, "notes", "the notes", c.Notes, maxNotes)

	if err := RequiredWhen(c.Status == ConditionStatusResolved,
		Ref{Field: "resolved_on", Value: c.ResolvedOn}); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}

	if err := Order(Ref{Field: "onset_on", Value: c.OnsetOn}, Ref{Field: "resolved_on", Value: c.ResolvedOn}); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}

	if err := NotFuture(Ref{Field: "resolved_on", Value: c.ResolvedOn}, Today()); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}

	return invalid.OrNil()
}
