package api

import (
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/vitals"
	"medikube/internal/web"
)

const (
	MemberVitalsRecordedAt         = "recorded_at"
	MemberVitalsSystolicMmHg       = "systolic_mmhg"
	MemberVitalsDiastolicMmHg      = "diastolic_mmhg"
	MemberVitalsHeartRateBpm       = "heart_rate_bpm"
	MemberVitalsRespiratoryRateBpm = "respiratory_rate_bpm"
	MemberVitalsTemperatureC       = "temperature_c"
	MemberVitalsSpo2Pct            = "spo2_pct"
	MemberVitalsWeightKg           = "weight_kg"
	MemberVitalsHeightCm           = "height_cm"
	MemberVitalsGlucoseMmolL       = "glucose_mmol_l"
	MemberVitalsGlucoseContext     = "glucose_context"
	MemberVitalsHba1cPct           = "hba1c_pct"
	MemberVitalsPainScale          = "pain_scale"
	MemberVitalsDevice             = "device"
	MemberVitalsPractitioner       = "practitioner"
)

var vitalsMembers = []string{
	MemberVitalsRecordedAt, MemberVitalsSystolicMmHg, MemberVitalsDiastolicMmHg,
	MemberVitalsHeartRateBpm, MemberVitalsRespiratoryRateBpm, MemberVitalsTemperatureC,
	MemberVitalsSpo2Pct, MemberVitalsWeightKg, MemberVitalsHeightCm, MemberVitalsGlucoseMmolL,
	MemberVitalsGlucoseContext, MemberVitalsHba1cPct, MemberVitalsPainScale,
	MemberVitalsDevice, MemberVitalsPractitioner,
}

// VitalsSummary carries only the measurements present (contracts/records-
// clinical.md §8: "the measured values present and a derived bmi when both
// height and weight are present"), and every value already in the actor's own
// unit_system.
type VitalsSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	RecordedAt string `json:"recorded_at"`

	SystolicMmHg       *float64 `json:"systolic_mmhg,omitempty"`
	DiastolicMmHg      *float64 `json:"diastolic_mmhg,omitempty"`
	HeartRateBpm       *float64 `json:"heart_rate_bpm,omitempty"`
	RespiratoryRateBpm *float64 `json:"respiratory_rate_bpm,omitempty"`
	TemperatureC       *float64 `json:"temperature_c,omitempty"`
	SpO2Pct            *float64 `json:"spo2_pct,omitempty"`
	WeightKg           *float64 `json:"weight_kg,omitempty"`
	HeightCm           *float64 `json:"height_cm,omitempty"`
	GlucoseMmolL       *float64 `json:"glucose_mmol_l,omitempty"`
	Hba1cPct           *float64 `json:"hba1c_pct,omitempty"`
	PainScale          *float64 `json:"pain_scale,omitempty"`

	// Bmi is derived and rendered only when both height and weight are
	// present (FR-037); it is never a member of Create or Patch.
	Bmi *float64 `json:"bmi,omitempty"`

	UpdatedAt string   `json:"updated_at"`
	Basis     []string `json:"basis"`
}

func (s *VitalsSummary) SetBasis(basis []string) { s.Basis = basis }

type Vitals struct {
	VitalsSummary

	Patient        string   `json:"patient"`
	GlucoseContext string   `json:"glucose_context,omitempty"`
	Device         string   `json:"device,omitempty"`
	Practitioner   string   `json:"practitioner,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	CreatedAt      string   `json:"created_at"`

	// Display names the unit system every measurement above is already
	// expressed in (research D-15) — the same idea patients.md's Display
	// carries, applied to actual values rather than only a formatted string.
	Display Display `json:"display"`
}

// GetTags implements records.Tagged so the search index stays in step with
// this record's tags on write (T164-T177 follow-up).
func (v *Vitals) GetTags() []string { return v.Tags }

type VitalsCreate struct {
	Patient            string   `json:"patient"`
	RecordedAt         string   `json:"recorded_at"`
	SystolicMmHg       *float64 `json:"systolic_mmhg,omitempty"`
	DiastolicMmHg      *float64 `json:"diastolic_mmhg,omitempty"`
	HeartRateBpm       *float64 `json:"heart_rate_bpm,omitempty"`
	RespiratoryRateBpm *float64 `json:"respiratory_rate_bpm,omitempty"`
	TemperatureC       *float64 `json:"temperature_c,omitempty"`
	SpO2Pct            *float64 `json:"spo2_pct,omitempty"`
	WeightKg           *float64 `json:"weight_kg,omitempty"`
	HeightCm           *float64 `json:"height_cm,omitempty"`
	GlucoseMmolL       *float64 `json:"glucose_mmol_l,omitempty"`
	GlucoseContext     string   `json:"glucose_context,omitempty"`
	Hba1cPct           *float64 `json:"hba1c_pct,omitempty"`
	PainScale          *float64 `json:"pain_scale,omitempty"`
	Device             string   `json:"device,omitempty"`
	Practitioner       *string  `json:"practitioner,omitempty"`
	Tags               []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *VitalsCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

type VitalsPatch struct {
	RecordedAt *string `json:"recorded_at,omitempty"`

	SystolicMmHg       web.Optional[float64] `json:"systolic_mmhg,omitzero"`
	DiastolicMmHg      web.Optional[float64] `json:"diastolic_mmhg,omitzero"`
	HeartRateBpm       web.Optional[float64] `json:"heart_rate_bpm,omitzero"`
	RespiratoryRateBpm web.Optional[float64] `json:"respiratory_rate_bpm,omitzero"`
	TemperatureC       web.Optional[float64] `json:"temperature_c,omitzero"`
	SpO2Pct            web.Optional[float64] `json:"spo2_pct,omitzero"`
	WeightKg           web.Optional[float64] `json:"weight_kg,omitzero"`
	HeightCm           web.Optional[float64] `json:"height_cm,omitzero"`
	GlucoseMmolL       web.Optional[float64] `json:"glucose_mmol_l,omitzero"`
	GlucoseContext     *string               `json:"glucose_context,omitempty"`
	Hba1cPct           web.Optional[float64] `json:"hba1c_pct,omitzero"`
	PainScale          web.Optional[float64] `json:"pain_scale,omitzero"`

	Device       *string `json:"device,omitempty"`
	Practitioner *string `json:"practitioner,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *VitalsPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// VitalsCodec is the DTO boundary for measurement sets, and the one place
// FR-037's unit conversion happens (research D-15).
type VitalsCodec struct{}

var _ vitals.Codec = VitalsCodec{}

func VitalsSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(VitalsSummary) },
		NewDetail:  func() any { return new(Vitals) },
		NewCreate:  func() any { return new(VitalsCreate) },
		NewPatch:   func() any { return new(VitalsPatch) },
	}
}

// VitalsSearchFields reads the device — the one free-text field a vitals row
// carries.
func VitalsSearchFields(body any) (title, text string) {
	v, ok := body.(*Vitals)
	if !ok {
		return "", ""
	}

	return v.RecordedAt, v.Device
}

// VitalsBasis narrows nothing: vitals publishes no per-row narrowing beyond
// the date range every kind shares.
func VitalsBasis(any, records.Criteria) []string { return nil }

func (VitalsCodec) Summary(v clinical.Vitals, system identity.UnitSystem) any {
	summary := &VitalsSummary{
		ID:                 v.ID,
		Kind:               kind.Vitals.Enum(),
		RecordedAt:         wireClinicalInstant(v.RecordedAt),
		SystolicMmHg:       v.SystolicMmHg,
		DiastolicMmHg:      v.DiastolicMmHg,
		HeartRateBpm:       v.HeartRateBpm,
		RespiratoryRateBpm: v.RespiratoryRateBpm,
		TemperatureC:       temperatureToDisplay(v.TemperatureC, system),
		SpO2Pct:            v.SpO2Pct,
		WeightKg:           weightToDisplay(v.WeightKg, system),
		HeightCm:           heightToDisplay(v.HeightCm, system),
		GlucoseMmolL:       glucoseToDisplay(v.GlucoseMmolL, system),
		Hba1cPct:           v.Hba1cPct,
		PainScale:          v.PainScale,
		UpdatedAt:          wireInstant(v.UpdatedAt),
	}

	if bmi, ok := v.BMI(); ok {
		summary.Bmi = &bmi
	}

	return summary
}

func (c VitalsCodec) Detail(v clinical.Vitals, system identity.UnitSystem) any {
	summary, ok := c.Summary(v, system).(*VitalsSummary)
	if !ok {
		return &Vitals{}
	}

	return &Vitals{
		VitalsSummary:  *summary,
		Patient:        v.PatientID,
		GlucoseContext: string(v.GlucoseContext),
		Device:         v.Device,
		Practitioner:   v.PractitionerID,
		Tags:           v.Tags,
		CreatedAt:      wireInstant(v.CreatedAt),
		Display:        Display{UnitSystem: string(system)},
	}
}

func (VitalsCodec) Draft(body any, system identity.UnitSystem) (clinical.Vitals, error) {
	create, ok := body.(*VitalsCreate)
	if !ok {
		return clinical.Vitals{}, ErrWrongBodyType
	}

	var invalid domain.ValidationError

	recordedAt := readClinicalInstant(&invalid, MemberVitalsRecordedAt, &create.RecordedAt)

	if err := orderedVitalsRefusal(&invalid); err != nil {
		return clinical.Vitals{}, err
	}

	return clinical.Vitals{
		PatientID:          create.Patient,
		RecordedAt:         recordedAt,
		SystolicMmHg:       create.SystolicMmHg,
		DiastolicMmHg:      create.DiastolicMmHg,
		HeartRateBpm:       create.HeartRateBpm,
		RespiratoryRateBpm: create.RespiratoryRateBpm,
		TemperatureC:       temperatureToSI(create.TemperatureC, system),
		SpO2Pct:            create.SpO2Pct,
		WeightKg:           weightToSI(create.WeightKg, system),
		HeightCm:           heightToSI(create.HeightCm, system),
		GlucoseMmolL:       glucoseToSI(create.GlucoseMmolL, system),
		GlucoseContext:     clinical.GlucoseContext(create.GlucoseContext),
		Hba1cPct:           create.Hba1cPct,
		PainScale:          create.PainScale,
		Device:             create.Device,
		PractitionerID:     deref(create.Practitioner),
		Tags:               create.Tags,
	}, nil
}

func (VitalsCodec) Patch(body any, system identity.UnitSystem) (vitals.Patch, error) {
	incoming, ok := body.(*VitalsPatch)
	if !ok {
		return vitals.Patch{}, ErrWrongBodyType
	}

	var invalid domain.ValidationError

	patch := vitals.Patch{
		RecordedAt:         readOptionalClinicalInstant(&invalid, MemberVitalsRecordedAt, incoming.RecordedAt),
		SystolicMmHg:       readOptionalFloatPtr(incoming.SystolicMmHg, identity.UnitSystemMetric, nil),
		DiastolicMmHg:      readOptionalFloatPtr(incoming.DiastolicMmHg, identity.UnitSystemMetric, nil),
		HeartRateBpm:       readOptionalFloatPtr(incoming.HeartRateBpm, identity.UnitSystemMetric, nil),
		RespiratoryRateBpm: readOptionalFloatPtr(incoming.RespiratoryRateBpm, identity.UnitSystemMetric, nil),
		TemperatureC:       readOptionalFloatPtr(incoming.TemperatureC, system, temperatureToSI),
		SpO2Pct:            readOptionalFloatPtr(incoming.SpO2Pct, identity.UnitSystemMetric, nil),
		WeightKg:           readOptionalFloatPtr(incoming.WeightKg, system, weightToSI),
		HeightCm:           readOptionalFloatPtr(incoming.HeightCm, system, heightToSI),
		GlucoseMmolL:       readOptionalFloatPtr(incoming.GlucoseMmolL, system, glucoseToSI),
		GlucoseContext:     convert[clinical.GlucoseContext](incoming.GlucoseContext),
		Hba1cPct:           readOptionalFloatPtr(incoming.Hba1cPct, identity.UnitSystemMetric, nil),
		PainScale:          readOptionalFloatPtr(incoming.PainScale, identity.UnitSystemMetric, nil),
		Device:             incoming.Device,
		PractitionerID:     incoming.Practitioner,
		Tags:               incoming.Tags,
	}

	if err := orderedVitalsRefusal(&invalid); err != nil {
		return vitals.Patch{}, err
	}

	return patch, nil
}

func orderedVitalsRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(vitalsMembers, left.Field) - slices.Index(vitalsMembers, right.Field)
	})

	return invalid.OrNil()
}

// readOptionalFloatPtr is the PATCH three-state reader (absent/leave alone,
// null/clear, value/set), with an optional unit conversion applied to a
// supplied value on its way to SI storage.
func readOptionalFloatPtr(
	supplied web.Optional[float64], system identity.UnitSystem, toSI func(*float64, identity.UnitSystem) *float64,
) **float64 {
	if !supplied.Present() {
		return nil
	}

	value, given := supplied.Get()
	if !given {
		var cleared *float64

		return &cleared
	}

	if toSI != nil {
		converted := toSI(&value, system)
		if converted != nil {
			value = *converted
		}
	}

	v := value
	pv := &v

	return &pv
}
