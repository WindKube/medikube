package clinical

import (
	"slices"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

const (
	maxCourseMedicationDosage    = 200
	maxCourseMedicationFrequency = 100
	maxCourseMedicationDuration  = 100
	maxCourseMedicationTiming    = 300
)

// CourseMedication is one medication attached to a course of treatment
// (data-model §5.2, FR-060/FR-061). Every field is course-specific and
// optional: an absent one falls back to the medication's own value, which is
// what Effective computes.
type CourseMedication struct {
	ID string

	TreatmentID  string
	MedicationID string

	Dosage    string
	Frequency string
	Duration  string
	Timing    string

	PrescriberID string
	PharmacyID   string

	StartedOn domain.Date
	EndedOn   domain.Date

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

func (c CourseMedication) MarshalZerologObject(e *zerolog.Event) {
	e.Str("treatment_id", c.TreatmentID).Str("medication_id", c.MedicationID)
}

// Validate is FR-060's one rule: an end before a start is refused, the same
// as every other date pair in this package.
func (c CourseMedication) Validate() error {
	var invalid domain.ValidationError

	checkLength(&invalid, "dosage", "the dose", c.Dosage, maxCourseMedicationDosage)
	checkLength(&invalid, "frequency", "how often", c.Frequency, maxCourseMedicationFrequency)
	checkLength(&invalid, "duration", "the duration", c.Duration, maxCourseMedicationDuration)
	checkLength(&invalid, "timing", "the timing", c.Timing, maxCourseMedicationTiming)

	if err := Order(Ref{Field: "started_on", Value: c.StartedOn}, Ref{Field: "ended_on", Value: c.EndedOn}); err != nil {
		invalid.Fields = append(invalid.Fields, *err)
	}

	return invalid.OrNil()
}

// EffectiveSource is FR-060's provenance: which of the two records a rendered
// value came from, or neither.
type EffectiveSource string

const (
	SourceCourse     EffectiveSource = "course"
	SourceMedication EffectiveSource = "medication"
	SourceNone       EffectiveSource = "none"
)

var effectiveSources = []EffectiveSource{SourceCourse, SourceMedication, SourceNone}

// EffectiveSources is the published enum slice (research architecture gate):
// every declared EffectiveSource constant, in declaration order.
func EffectiveSources() []EffectiveSource { return slices.Clone(effectiveSources) }

// Effective is one field's resolved value and where it came from. Value is
// `any` because the fields it resolves are not all strings — the two dates
// are domain.Date — and the contract's whole point is that the source travels
// with the value rather than being re-derived by whoever renders it.
type Effective struct {
	Value  any
	Source EffectiveSource
}

// EffectiveFields is every one of FR-060's eight resolved values, computed
// once by Resolve. Duration and Timing have no fallback: data-model §4.14
// gives the medication itself neither field, so an absent course value there
// is always Source: none.
type EffectiveFields struct {
	Dosage     Effective
	Frequency  Effective
	Duration   Effective
	Timing     Effective
	Prescriber Effective
	Pharmacy   Effective
	StartedOn  Effective
	EndedOn    Effective
}

// Resolve is D-09's COALESCE-with-provenance, run once per link row. It never
// touches SQL and never runs in the browser — this is the single place FR-060
// is decided.
func (c CourseMedication) Resolve(medication Medication) EffectiveFields {
	return EffectiveFields{
		Dosage:     resolveText(c.Dosage, medication.Dosage),
		Frequency:  resolveText(c.Frequency, medication.Frequency),
		Duration:   resolveText(c.Duration, ""),
		Timing:     resolveText(c.Timing, ""),
		Prescriber: resolveID(c.PrescriberID, medication.PractitionerID),
		Pharmacy:   resolveID(c.PharmacyID, medication.PharmacyID),
		StartedOn:  resolveDate(c.StartedOn, medication.StartedOn),
		EndedOn:    resolveDate(c.EndedOn, medication.EndedOn),
	}
}

func resolveText(course, medication string) Effective {
	if course != "" {
		return Effective{Value: course, Source: SourceCourse}
	}

	if medication != "" {
		return Effective{Value: medication, Source: SourceMedication}
	}

	return Effective{Value: nil, Source: SourceNone}
}

func resolveID(course, medication string) Effective {
	return resolveText(course, medication)
}

func resolveDate(course, medication domain.Date) Effective {
	if !course.IsZero() {
		return Effective{Value: course, Source: SourceCourse}
	}

	if !medication.IsZero() {
		return Effective{Value: medication, Source: SourceMedication}
	}

	return Effective{Value: nil, Source: SourceNone}
}
