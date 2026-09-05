package clinical

import (
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Symptom is one episode (FR-029). There is no symptom_definitions header row
// upstream carried: recording the same name again creates a second episode
// (FR-030), and episode_count/last_occurred_at are a correlated aggregate the
// store computes on read (FR-031), never a column here.
type Symptom struct {
	ID        string
	PatientID string

	Name            string
	Category        SymptomCategory
	Severity        Severity
	OccurredAt      Instant
	DurationMinutes *int
	PainScale       *int
	BodySite        string
	Triggers        []string
	ReliefMethods   []string
	Impact          SymptomImpact
	ResolvedAt      Instant
	IsChronic       bool
	Status          ConditionStatus

	// FR-032: what it is attributed to, and the two distinct medication
	// roles — treats it, or is suspected of causing it.
	ConditionIDs           []string
	TreatmentIDs           []string
	TreatedByMedicationIDs []string
	CausedByMedicationIDs  []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string

	// EpisodeCount and LastOccurredAt are FR-031's derived aggregate over
	// (patient, LOWER(name)): never stored, filled in by the store on every
	// read and never touched by Validate or by the mapper's write side.
	EpisodeCount   int
	LastOccurredAt Instant
}

const (
	symptomNameMax     = 200
	symptomBodySiteMax = 120
	symptomListMax     = 20
	symptomListItemMax = 80
)

// Validate reports every offending field at once (FR-027), in data-model
// §4.6's column order.
func (s Symptom) Validate() error {
	var invalid domain.ValidationError

	if s.Name == "" {
		invalid.Add("name", domain.CodeRequired, "a name is required")
	} else if utf8.RuneCountInString(s.Name) > symptomNameMax {
		invalid.Addf("name", domain.CodeTooLong, "the name accepts at most %d characters", symptomNameMax)
	}

	if s.Category != "" && !s.Category.Valid() {
		invalid.Add("category", domain.CodeInvalidValue, "not one of the categories MediKube accepts")
	}

	switch {
	case s.Severity == "":
		invalid.Add("severity", domain.CodeRequired, "a severity is required")
	case !s.Severity.Valid():
		invalid.Add("severity", domain.CodeInvalidValue, "not one of the severities MediKube accepts")
	}

	if s.OccurredAt.IsZero() {
		invalid.Add("occurred_at", domain.CodeRequired, "when it occurred is required")
	}

	if s.DurationMinutes != nil && *s.DurationMinutes < 0 {
		invalid.Add("duration_minutes", domain.CodeOutOfRange, "the duration cannot be negative")
	}

	if s.PainScale != nil && (*s.PainScale < 0 || *s.PainScale > 10) {
		invalid.Add("pain_scale", domain.CodeOutOfRange, "a pain rating is between 0 and 10")
	}

	if utf8.RuneCountInString(s.BodySite) > symptomBodySiteMax {
		invalid.Addf("body_site", domain.CodeTooLong, "the body site accepts at most %d characters", symptomBodySiteMax)
	}

	checkStringList(&invalid, "triggers", s.Triggers)
	checkStringList(&invalid, "relief_methods", s.ReliefMethods)

	if s.Impact != "" && !s.Impact.Valid() {
		invalid.Add("impact", domain.CodeInvalidValue, "not one of the levels MediKube accepts")
	}

	if !s.ResolvedAt.IsZero() && !s.OccurredAt.IsZero() && s.ResolvedAt.Before(s.OccurredAt) {
		invalid.Add("resolved_at", CodeEndBeforeStart, "resolved_at is before occurred_at")
	}

	if s.Status != "" && !s.Status.Valid() {
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	return invalid.OrNil()
}

// checkStringList is FR-029's bound on triggers and relief_methods: at most
// twenty entries, each at most eighty characters.
func checkStringList(invalid *domain.ValidationError, field string, values []string) {
	if len(values) > symptomListMax {
		invalid.Addf(field, domain.CodeTooLong, "%s accepts at most %d entries", field, symptomListMax)

		return
	}

	for _, value := range values {
		if utf8.RuneCountInString(value) > symptomListItemMax {
			invalid.Addf(field, domain.CodeTooLong, "each entry of %s accepts at most %d characters", field, symptomListItemMax)

			return
		}
	}
}

// MarshalZerologObject emits the two identifiers and nothing else — the name,
// the body site and everything else here is PHI (constitution VII).
func (s Symptom) MarshalZerologObject(e *zerolog.Event) {
	e.Str("symptom_id", s.ID).Str("patient_id", s.PatientID)
}
