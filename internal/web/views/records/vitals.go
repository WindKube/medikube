package records

import (
	"context"
	"strconv"

	"medikube/internal/domain/clinical"
	"medikube/internal/i18n"
	viewtags "medikube/internal/web/views/tags"
)

const (
	VitalsFieldRecordedAt         = "recorded_at"
	VitalsFieldSystolicMmHg       = "systolic_mmhg"
	VitalsFieldDiastolicMmHg      = "diastolic_mmhg"
	VitalsFieldHeartRateBpm       = "heart_rate_bpm"
	VitalsFieldRespiratoryRateBpm = "respiratory_rate_bpm"
	VitalsFieldTemperatureC       = "temperature_c"
	VitalsFieldSpo2Pct            = "spo2_pct"
	VitalsFieldWeightKg           = "weight_kg"
	VitalsFieldHeightCm           = "height_cm"
	VitalsFieldGlucoseMmolL       = "glucose_mmol_l"
	VitalsFieldGlucoseContext     = "glucose_context"
	VitalsFieldHba1cPct           = "hba1c_pct"
	VitalsFieldPainScale          = "pain_scale"
	VitalsFieldDevice             = "device"
	VitalsFieldBmi                = "bmi"
)

const (
	VitalsFormLabelCreate = "action.record_vitals"
	VitalsFormLabelEdit   = "a11y.edit_vitals_form"
)

var vitalsFields = []string{
	VitalsFieldRecordedAt, VitalsFieldSystolicMmHg, VitalsFieldDiastolicMmHg,
	VitalsFieldHeartRateBpm, VitalsFieldRespiratoryRateBpm, VitalsFieldTemperatureC,
	VitalsFieldSpo2Pct, VitalsFieldWeightKg, VitalsFieldHeightCm, VitalsFieldGlucoseMmolL,
	VitalsFieldGlucoseContext, VitalsFieldHba1cPct, VitalsFieldPainScale, VitalsFieldDevice,
}

func VitalsFields() []string { return append([]string(nil), vitalsFields...) }

// vitalsFieldLabels maps each of vitals' own fields to its message id
// (D-06); the templ that prints a label resolves it with i18n.T at render.
var vitalsFieldLabels = map[string]string{
	VitalsFieldRecordedAt:         "field.vitals.recorded_at",
	VitalsFieldSystolicMmHg:       "field.vitals.systolic_mmhg",
	VitalsFieldDiastolicMmHg:      "field.vitals.diastolic_mmhg",
	VitalsFieldHeartRateBpm:       "field.vitals.heart_rate_bpm",
	VitalsFieldRespiratoryRateBpm: "field.vitals.respiratory_rate_bpm",
	VitalsFieldTemperatureC:       "field.vitals.temperature_c",
	VitalsFieldSpo2Pct:            "field.vitals.spo2_pct",
	VitalsFieldWeightKg:           "field.vitals.weight_kg",
	VitalsFieldHeightCm:           "field.vitals.height_cm",
	VitalsFieldGlucoseMmolL:       "field.vitals.glucose_mmol_l",
	VitalsFieldGlucoseContext:     "field.vitals.glucose_context",
	VitalsFieldHba1cPct:           "field.vitals.hba1c_pct",
	VitalsFieldPainScale:          "field.vitals.pain_scale",
	VitalsFieldDevice:             "field.vitals.device",
	VitalsFieldBmi:                "field.vitals.bmi",
}

// VitalsFieldLabel returns the field's message id; the caller resolves it
// with i18n.T at the point it is printed.
func VitalsFieldLabel(field string) string {
	if id, known := vitalsFieldLabels[field]; known {
		return id
	}

	return field
}

// glucoseContextLabels stays plain English: it feeds View.GlucoseContext, a
// DetailEntry.Value printed by the shared, cross-slice detailValue templ
// that does not (yet) call i18n.T (see GlucoseContextOptions below for the
// id-producing twin used by the form select, which this codebase does
// control).
var glucoseContextLabels = map[clinical.GlucoseContext]string{
	clinical.GlucoseContextFasting:    "Fasting",
	clinical.GlucoseContextBeforeMeal: "Before a meal",
	clinical.GlucoseContextAfterMeal:  "After a meal",
	clinical.GlucoseContextRandom:     "Random",
}

func GlucoseContextLabel(value clinical.GlucoseContext) string {
	return label(string(value), glucoseContextLabels[value])
}

func GlucoseContextOptions(selected clinical.GlucoseContext) []Option {
	published := clinical.GlucoseContexts()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: "enum.glucose_context." + string(value), Selected: value == selected})
	}

	return options
}

// VitalsLinks are the URLs one measurement set's views address.
type VitalsLinks struct {
	Detail string
	Edit   string
	Record string
}

// VitalsView is one measurement set as its views render it. Every numeric
// field arrives already expressed in the actor's own unit system (research
// D-15) — this layer formats, it does not convert.
type VitalsView struct {
	ID        string
	PatientID string

	RecordedAt      string
	RecordedAtInput string

	SystolicMmHg        string
	DiastolicMmHg       string
	HeartRateBpm        string
	RespiratoryRateBpm  string
	TemperatureC        string
	Spo2Pct             string
	WeightKg            string
	HeightCm            string
	GlucoseMmolL        string
	GlucoseContext      string
	GlucoseContextValue string
	Hba1cPct            string
	PainScale           string
	Bmi                 string

	Device       string
	Practitioner string

	Created     Timestamp
	LastChanged Timestamp

	Version string

	Links VitalsLinks
}

func formatFloatPtr(v *float64) string {
	if v == nil {
		return ""
	}

	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func NewVitalsView(v clinical.Vitals, links VitalsLinks) VitalsView {
	bmi := ""
	if computed, ok := v.BMI(); ok {
		bmi = strconv.FormatFloat(computed, 'f', 1, 64)
	}

	return VitalsView{
		ID:                  v.ID,
		PatientID:           v.PatientID,
		RecordedAt:          v.RecordedAt.String(),
		RecordedAtInput:     v.RecordedAt.Input(),
		SystolicMmHg:        formatFloatPtr(v.SystolicMmHg),
		DiastolicMmHg:       formatFloatPtr(v.DiastolicMmHg),
		HeartRateBpm:        formatFloatPtr(v.HeartRateBpm),
		RespiratoryRateBpm:  formatFloatPtr(v.RespiratoryRateBpm),
		TemperatureC:        formatFloatPtr(v.TemperatureC),
		Spo2Pct:             formatFloatPtr(v.SpO2Pct),
		WeightKg:            formatFloatPtr(v.WeightKg),
		HeightCm:            formatFloatPtr(v.HeightCm),
		GlucoseMmolL:        formatFloatPtr(v.GlucoseMmolL),
		GlucoseContext:      GlucoseContextLabel(v.GlucoseContext),
		GlucoseContextValue: string(v.GlucoseContext),
		Hba1cPct:            formatFloatPtr(v.Hba1cPct),
		PainScale:           formatFloatPtr(v.PainScale),
		Bmi:                 bmi,
		Device:              v.Device,
		Practitioner:        v.PractitionerID,
		Created:             NewTimestamp(v.CreatedAt),
		LastChanged:         NewTimestamp(v.UpdatedAt),
		Version:             v.Version,
		Links:               links,
	}
}

// Entries is FR-024's "only the measurements present" made a property of the
// mapping: a measurement never taken produces no row.
func (m VitalsView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: VitalsFieldSystolicMmHg, Value: m.SystolicMmHg},
		{Field: VitalsFieldDiastolicMmHg, Value: m.DiastolicMmHg},
		{Field: VitalsFieldHeartRateBpm, Value: m.HeartRateBpm},
		{Field: VitalsFieldRespiratoryRateBpm, Value: m.RespiratoryRateBpm},
		{Field: VitalsFieldTemperatureC, Value: m.TemperatureC},
		{Field: VitalsFieldSpo2Pct, Value: m.Spo2Pct},
		{Field: VitalsFieldWeightKg, Value: m.WeightKg},
		{Field: VitalsFieldHeightCm, Value: m.HeightCm},
		{Field: VitalsFieldBmi, Value: m.Bmi},
		{Field: VitalsFieldGlucoseMmolL, Value: m.GlucoseMmolL},
		{Field: VitalsFieldGlucoseContext, Value: m.GlucoseContext},
		{Field: VitalsFieldHba1cPct, Value: m.Hba1cPct},
		{Field: VitalsFieldPainScale, Value: m.PainScale},
		{Field: VitalsFieldDevice, Value: m.Device},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = VitalsFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

type VitalsListProps struct {
	Vitals []VitalsView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

type VitalsDetailProps struct {
	Vitals VitalsView
}

type VitalsFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	Vitals VitalsView
	Errors FieldErrors

	Notice string

	Tags viewtags.FieldProps
}

func (p VitalsFormProps) Label(ctx context.Context) string {
	if p.New {
		return i18n.T(ctx, VitalsFormLabelCreate)
	}

	return i18n.T(ctx, VitalsFormLabelEdit)
}

// Summary is the row's one-line rendering of "only the measurements present"
// (T089): each measurement actually taken, and nothing for the rest.
func (m VitalsView) Summary(ctx context.Context) string {
	parts := make([]string, 0, 8)

	add := func(id, value string) {
		if value != "" {
			parts = append(parts, i18n.T(ctx, id)+" "+value)
		}
	}

	add("field.vitals.systolic_short", m.SystolicMmHg)
	add("field.vitals.diastolic_short", m.DiastolicMmHg)
	add("field.vitals.heart_rate_short", m.HeartRateBpm)
	add("field.vitals.temperature_short", m.TemperatureC)
	add("field.vitals.spo2_short", m.Spo2Pct)
	add("field.vitals.weight_short", m.WeightKg)
	add("field.vitals.height_short", m.HeightCm)
	add("field.vitals.glucose_short", m.GlucoseMmolL)
	add("field.vitals.bmi_short", m.Bmi)

	return join(parts)
}

func (m VitalsView) GlucoseContextOptions() []Option {
	return GlucoseContextOptions(clinical.GlucoseContext(m.GlucoseContextValue))
}

func (m VitalsView) Value(field string) string {
	switch field {
	case VitalsFieldRecordedAt:
		return m.RecordedAtInput
	case VitalsFieldSystolicMmHg:
		return m.SystolicMmHg
	case VitalsFieldDiastolicMmHg:
		return m.DiastolicMmHg
	case VitalsFieldHeartRateBpm:
		return m.HeartRateBpm
	case VitalsFieldRespiratoryRateBpm:
		return m.RespiratoryRateBpm
	case VitalsFieldTemperatureC:
		return m.TemperatureC
	case VitalsFieldSpo2Pct:
		return m.Spo2Pct
	case VitalsFieldWeightKg:
		return m.WeightKg
	case VitalsFieldHeightCm:
		return m.HeightCm
	case VitalsFieldGlucoseMmolL:
		return m.GlucoseMmolL
	case VitalsFieldGlucoseContext:
		return m.GlucoseContextValue
	case VitalsFieldHba1cPct:
		return m.Hba1cPct
	case VitalsFieldPainScale:
		return m.PainScale
	case VitalsFieldDevice:
		return m.Device
	default:
		return ""
	}
}

func (p VitalsFormProps) SubmitLabel(ctx context.Context) string {
	if p.New {
		return i18n.T(ctx, "action.record_it")
	}

	return i18n.T(ctx, "action.save_changes")
}

func deleteVitalsExpression(v VitalsView) string {
	return "@delete(" + jsLiteral(v.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
