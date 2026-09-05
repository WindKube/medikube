package records

import (
	"strconv"
	"strings"

	"medikube/internal/domain/clinical"
)

func itoa(v int) string { return strconv.Itoa(v) }

func join(values []string) string { return strings.Join(values, ", ") }

// The wire spellings and refusal names for symptom episodes.
const (
	SymptomFieldName            = "name"
	SymptomFieldCategory        = "category"
	SymptomFieldSeverity        = "severity"
	SymptomFieldOccurredAt      = "occurred_at"
	SymptomFieldDurationMinutes = "duration_minutes"
	SymptomFieldPainScale       = "pain_scale"
	SymptomFieldBodySite        = "body_site"
	SymptomFieldTriggers        = "triggers"
	SymptomFieldReliefMethods   = "relief_methods"
	SymptomFieldImpact          = "impact"
	SymptomFieldResolvedAt      = "resolved_at"
	SymptomFieldIsChronic       = "is_chronic"
	SymptomFieldStatus          = "status"
)

const (
	SymptomFormLabelCreate = "Record a symptom"
	SymptomFormLabelEdit   = "Edit symptom"
)

var symptomFields = []string{
	SymptomFieldName, SymptomFieldCategory, SymptomFieldSeverity, SymptomFieldOccurredAt,
	SymptomFieldDurationMinutes, SymptomFieldPainScale, SymptomFieldBodySite,
	SymptomFieldTriggers, SymptomFieldReliefMethods, SymptomFieldImpact,
	SymptomFieldResolvedAt, SymptomFieldIsChronic, SymptomFieldStatus,
}

func SymptomFields() []string { return append([]string(nil), symptomFields...) }

var symptomFieldLabels = map[string]string{
	SymptomFieldName:            "What it was",
	SymptomFieldCategory:        "Category",
	SymptomFieldSeverity:        "Severity",
	SymptomFieldOccurredAt:      "When",
	SymptomFieldDurationMinutes: "How long (minutes)",
	SymptomFieldPainScale:       "Pain (0-10)",
	SymptomFieldBodySite:        "Where on the body",
	SymptomFieldTriggers:        "Triggers",
	SymptomFieldReliefMethods:   "What helped",
	SymptomFieldImpact:          "Impact",
	SymptomFieldResolvedAt:      "Resolved",
	SymptomFieldIsChronic:       "Ongoing",
	SymptomFieldStatus:          "State",
}

func SymptomFieldLabel(field string) string {
	if label, known := symptomFieldLabels[field]; known {
		return label
	}

	return field
}

var (
	symptomCategoryLabels = map[clinical.SymptomCategory]string{}

	symptomSeverityLabels = map[clinical.Severity]string{
		clinical.SeverityMild:     "Mild",
		clinical.SeverityModerate: "Moderate",
		clinical.SeveritySevere:   "Severe",
	}

	symptomImpactLabels = map[clinical.SymptomImpact]string{
		clinical.SymptomImpactNone:     "None",
		clinical.SymptomImpactMild:     "Mild",
		clinical.SymptomImpactModerate: "Moderate",
		clinical.SymptomImpactSevere:   "Severe",
	}

	symptomStatusLabels = map[clinical.ConditionStatus]string{}
)

func SymptomCategoryLabel(value clinical.SymptomCategory) string {
	return label(string(value), symptomCategoryLabels[value])
}

func SymptomSeverityLabel(value clinical.Severity) string {
	return label(string(value), symptomSeverityLabels[value])
}

func SymptomImpactLabel(value clinical.SymptomImpact) string {
	return label(string(value), symptomImpactLabels[value])
}

func SymptomStatusLabel(value clinical.ConditionStatus) string {
	return label(string(value), symptomStatusLabels[value])
}

func SymptomCategoryOptions(selected clinical.SymptomCategory) []Option {
	published := clinical.SymptomCategories()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: SymptomCategoryLabel(value), Selected: value == selected})
	}

	return options
}

func SymptomSeverityOptions(selected clinical.Severity) []Option {
	published := clinical.Severities()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: SymptomSeverityLabel(value), Selected: value == selected})
	}

	return options
}

func SymptomImpactOptions(selected clinical.SymptomImpact) []Option {
	published := clinical.SymptomImpacts()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: SymptomImpactLabel(value), Selected: value == selected})
	}

	return options
}

func SymptomStatusOptions(selected clinical.ConditionStatus) []Option {
	published := clinical.ConditionStatuses()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: SymptomStatusLabel(value), Selected: value == selected})
	}

	return options
}

// SymptomLinks are the URLs one symptom episode's views address.
type SymptomLinks struct {
	Detail string
	Edit   string
	Record string
}

// SymptomView is one symptom episode as its views render it.
type SymptomView struct {
	ID        string
	PatientID string

	Name            string
	Category        string
	CategoryValue   string
	Severity        string
	SeverityValue   string
	OccurredAt      string
	DurationMinutes string
	PainScale       string
	BodySite        string
	Triggers        []string
	ReliefMethods   []string
	Impact          string
	ImpactValue     string
	ResolvedAt      string
	IsChronic       bool
	Status          string
	StatusValue     string

	EpisodeCount   int
	LastOccurredAt string
	LastChanged    Timestamp

	Version string

	Links SymptomLinks
}

func NewSymptomView(symptom clinical.Symptom, links SymptomLinks) SymptomView {
	durationMinutes := ""
	if symptom.DurationMinutes != nil {
		durationMinutes = itoa(*symptom.DurationMinutes)
	}

	painScale := ""
	if symptom.PainScale != nil {
		painScale = itoa(*symptom.PainScale)
	}

	return SymptomView{
		ID:              symptom.ID,
		PatientID:       symptom.PatientID,
		Name:            symptom.Name,
		Category:        SymptomCategoryLabel(symptom.Category),
		CategoryValue:   string(symptom.Category),
		Severity:        SymptomSeverityLabel(symptom.Severity),
		SeverityValue:   string(symptom.Severity),
		OccurredAt:      symptom.OccurredAt.String(),
		DurationMinutes: durationMinutes,
		PainScale:       painScale,
		BodySite:        symptom.BodySite,
		Triggers:        symptom.Triggers,
		ReliefMethods:   symptom.ReliefMethods,
		Impact:          SymptomImpactLabel(symptom.Impact),
		ImpactValue:     string(symptom.Impact),
		ResolvedAt:      symptom.ResolvedAt.String(),
		IsChronic:       symptom.IsChronic,
		Status:          SymptomStatusLabel(symptom.Status),
		StatusValue:     string(symptom.Status),
		EpisodeCount:    symptom.EpisodeCount,
		LastOccurredAt:  symptom.LastOccurredAt.String(),
		LastChanged:     NewTimestamp(symptom.UpdatedAt),
		Version:         symptom.Version,
		Links:           links,
	}
}

func (m SymptomView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: SymptomFieldCategory, Value: m.Category},
		{Field: SymptomFieldSeverity, Value: m.Severity},
		{Field: SymptomFieldOccurredAt, Value: m.OccurredAt, Datetime: m.OccurredAt},
		{Field: SymptomFieldDurationMinutes, Value: m.DurationMinutes},
		{Field: SymptomFieldPainScale, Value: m.PainScale},
		{Field: SymptomFieldBodySite, Value: m.BodySite},
		{Field: SymptomFieldImpact, Value: m.Impact},
		{Field: SymptomFieldResolvedAt, Value: m.ResolvedAt, Datetime: m.ResolvedAt},
		{Field: SymptomFieldStatus, Value: m.Status},
	}

	entries := make([]DetailEntry, 0, len(candidates)+2)

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = SymptomFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	if len(m.Triggers) > 0 {
		entries = append(entries, DetailEntry{Field: SymptomFieldTriggers, Label: SymptomFieldLabel(SymptomFieldTriggers), Value: join(m.Triggers)})
	}

	if len(m.ReliefMethods) > 0 {
		entries = append(entries, DetailEntry{Field: SymptomFieldReliefMethods, Label: SymptomFieldLabel(SymptomFieldReliefMethods), Value: join(m.ReliefMethods)})
	}

	if m.IsChronic {
		entries = append(entries, DetailEntry{Field: SymptomFieldIsChronic, Label: SymptomFieldLabel(SymptomFieldIsChronic), Value: "Yes"})
	}

	return entries
}

type SymptomListProps struct {
	Symptoms []SymptomView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

type SymptomDetailProps struct {
	Symptom SymptomView
}

type SymptomFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	Symptom SymptomView
	Errors  FieldErrors

	Notice string
}

func (p SymptomFormProps) Label() string {
	if p.New {
		return SymptomFormLabelCreate
	}

	return SymptomFormLabelEdit
}

func (m SymptomView) CategoryOptions() []Option {
	return SymptomCategoryOptions(clinical.SymptomCategory(m.CategoryValue))
}
func (m SymptomView) SeverityOptions() []Option {
	return SymptomSeverityOptions(clinical.Severity(m.SeverityValue))
}
func (m SymptomView) ImpactOptions() []Option {
	return SymptomImpactOptions(clinical.SymptomImpact(m.ImpactValue))
}
func (m SymptomView) StatusOptions() []Option {
	return SymptomStatusOptions(clinical.ConditionStatus(m.StatusValue))
}

func (m SymptomView) Value(field string) string {
	switch field {
	case SymptomFieldName:
		return m.Name
	case SymptomFieldCategory:
		return m.CategoryValue
	case SymptomFieldSeverity:
		return m.SeverityValue
	case SymptomFieldOccurredAt:
		return m.OccurredAt
	case SymptomFieldDurationMinutes:
		return m.DurationMinutes
	case SymptomFieldPainScale:
		return m.PainScale
	case SymptomFieldBodySite:
		return m.BodySite
	case SymptomFieldTriggers:
		return join(m.Triggers)
	case SymptomFieldReliefMethods:
		return join(m.ReliefMethods)
	case SymptomFieldImpact:
		return m.ImpactValue
	case SymptomFieldResolvedAt:
		return m.ResolvedAt
	case SymptomFieldStatus:
		return m.StatusValue
	default:
		return ""
	}
}

func (p SymptomFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
}

func deleteSymptomExpression(symptom SymptomView) string {
	return "@delete(" + jsLiteral(symptom.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
