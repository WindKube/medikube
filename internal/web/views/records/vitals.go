package records

import (
	viewtags "medikube/internal/web/views/tags"
	"strconv"

	"medikube/internal/domain/clinical"
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
	VitalsFormLabelCreate = "Record measurements"
	VitalsFormLabelEdit   = "Edit measurements"
)

var vitalsFields = []string{
	VitalsFieldRecordedAt, VitalsFieldSystolicMmHg, VitalsFieldDiastolicMmHg,
	VitalsFieldHeartRateBpm, VitalsFieldRespiratoryRateBpm, VitalsFieldTemperatureC,
	VitalsFieldSpo2Pct, VitalsFieldWeightKg, VitalsFieldHeightCm, VitalsFieldGlucoseMmolL,
	VitalsFieldGlucoseContext, VitalsFieldHba1cPct, VitalsFieldPainScale, VitalsFieldDevice,
}

func VitalsFields() []string { return append([]string(nil), vitalsFields...) }

var vitalsFieldLabels = map[string]string{
	VitalsFieldRecordedAt:         "When",
	VitalsFieldSystolicMmHg:       "Systolic (mmHg)",
	VitalsFieldDiastolicMmHg:      "Diastolic (mmHg)",
	VitalsFieldHeartRateBpm:       "Heart rate (bpm)",
	VitalsFieldRespiratoryRateBpm: "Respiratory rate (breaths/min)",
	VitalsFieldTemperatureC:       "Temperature",
	VitalsFieldSpo2Pct:            "Oxygen saturation (%)",
	VitalsFieldWeightKg:           "Weight",
	VitalsFieldHeightCm:           "Height",
	VitalsFieldGlucoseMmolL:       "Glucose",
	VitalsFieldGlucoseContext:     "Taken",
	VitalsFieldHba1cPct:           "HbA1c (%)",
	VitalsFieldPainScale:          "Pain (0-10)",
	VitalsFieldDevice:             "Device",
	VitalsFieldBmi:                "BMI",
}

func VitalsFieldLabel(field string) string {
	if label, known := vitalsFieldLabels[field]; known {
		return label
	}

	return field
}

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
		options = append(options, Option{Value: string(value), Label: GlucoseContextLabel(value), Selected: value == selected})
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

func (p VitalsFormProps) Label() string {
	if p.New {
		return VitalsFormLabelCreate
	}

	return VitalsFormLabelEdit
}

// Summary is the row's one-line rendering of "only the measurements present"
// (T089): each measurement actually taken, and nothing for the rest.
func (m VitalsView) Summary() string {
	parts := make([]string, 0, 8)

	add := func(label, value string) {
		if value != "" {
			parts = append(parts, label+" "+value)
		}
	}

	add("Systolic", m.SystolicMmHg)
	add("Diastolic", m.DiastolicMmHg)
	add("HR", m.HeartRateBpm)
	add("Temp", m.TemperatureC)
	add("SpO2", m.Spo2Pct)
	add("Weight", m.WeightKg)
	add("Height", m.HeightCm)
	add("Glucose", m.GlucoseMmolL)
	add("BMI", m.Bmi)

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

func (p VitalsFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
}

func deleteVitalsExpression(v VitalsView) string {
	return "@delete(" + jsLiteral(v.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
