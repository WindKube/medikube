package vitals

import "medikube/internal/domain/clinical"

// Patch is a change to a measurement set: every field optional, each numeric
// field a double pointer so absent/clear/set-to-value are distinguishable —
// the same shape MedicationPatch's two dates use.
type Patch struct {
	RecordedAt *clinical.Instant

	SystolicMmHg       **float64
	DiastolicMmHg      **float64
	HeartRateBpm       **float64
	RespiratoryRateBpm **float64
	TemperatureC       **float64
	SpO2Pct            **float64
	WeightKg           **float64
	HeightCm           **float64
	GlucoseMmolL       **float64
	GlucoseContext     *clinical.GlucoseContext
	Hba1cPct           **float64
	PainScale          **float64

	Device         *string
	PractitionerID *string

	// Tags is data-model §0.8's universal field, replace-set (FR-064,
	// FR-065): nil leaves the applied tags alone, non-nil (including empty)
	// replaces the whole set.
	Tags *[]string
}

func (p Patch) applyTo(v clinical.Vitals) clinical.Vitals {
	assign(&v.RecordedAt, p.RecordedAt)
	assign(&v.SystolicMmHg, p.SystolicMmHg)
	assign(&v.DiastolicMmHg, p.DiastolicMmHg)
	assign(&v.HeartRateBpm, p.HeartRateBpm)
	assign(&v.RespiratoryRateBpm, p.RespiratoryRateBpm)
	assign(&v.TemperatureC, p.TemperatureC)
	assign(&v.SpO2Pct, p.SpO2Pct)
	assign(&v.WeightKg, p.WeightKg)
	assign(&v.HeightCm, p.HeightCm)
	assign(&v.GlucoseMmolL, p.GlucoseMmolL)
	assign(&v.GlucoseContext, p.GlucoseContext)
	assign(&v.Hba1cPct, p.Hba1cPct)
	assign(&v.PainScale, p.PainScale)
	assign(&v.Device, p.Device)
	assign(&v.PractitionerID, p.PractitionerID)

	if p.Tags != nil {
		v.Tags = *p.Tags
	}

	return v
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}
