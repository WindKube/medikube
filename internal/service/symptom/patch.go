package symptom

import (
	"medikube/internal/domain/clinical"
)

// Patch is a change to a symptom episode: every field optional.
type Patch struct {
	Name            *string
	Category        *clinical.SymptomCategory
	Severity        *clinical.Severity
	OccurredAt      *clinical.Instant
	DurationMinutes **int
	PainScale       **int
	BodySite        *string
	Triggers        *[]string
	ReliefMethods   *[]string
	Impact          *clinical.SymptomImpact
	ResolvedAt      *clinical.Instant
	IsChronic       *bool
	Status          *clinical.ConditionStatus

	// Tags is data-model §0.8's universal field, replace-set (FR-064,
	// FR-065): nil leaves the applied tags alone, non-nil (including empty)
	// replaces the whole set.
	Tags *[]string

	// TreatedByMedicationIDs and CausedByMedicationIDs are FR-032's two
	// distinct medication roles: replace-set, nil leaves the set alone.
	TreatedByMedicationIDs *[]string
	CausedByMedicationIDs  *[]string
}

func (p Patch) applyTo(s clinical.Symptom) clinical.Symptom {
	assign(&s.Name, p.Name)
	assign(&s.Category, p.Category)
	assign(&s.Severity, p.Severity)
	assign(&s.OccurredAt, p.OccurredAt)
	assign(&s.DurationMinutes, p.DurationMinutes)
	assign(&s.PainScale, p.PainScale)
	assign(&s.BodySite, p.BodySite)
	assign(&s.Triggers, p.Triggers)
	assign(&s.ReliefMethods, p.ReliefMethods)
	assign(&s.Impact, p.Impact)
	assign(&s.ResolvedAt, p.ResolvedAt)
	assign(&s.IsChronic, p.IsChronic)
	assign(&s.Status, p.Status)

	if p.Tags != nil {
		s.Tags = *p.Tags
	}

	if p.TreatedByMedicationIDs != nil {
		s.TreatedByMedicationIDs = *p.TreatedByMedicationIDs
	}

	if p.CausedByMedicationIDs != nil {
		s.CausedByMedicationIDs = *p.CausedByMedicationIDs
	}

	return s
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}
