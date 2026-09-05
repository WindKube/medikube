package clinical

import "slices"

// TreatmentSetting is where a course of treatment takes place.
type TreatmentSetting string

const (
	TreatmentSettingInpatient  TreatmentSetting = "inpatient"
	TreatmentSettingOutpatient TreatmentSetting = "outpatient"
	TreatmentSettingHome       TreatmentSetting = "home"
)

var treatmentSettings = []TreatmentSetting{TreatmentSettingInpatient, TreatmentSettingOutpatient, TreatmentSettingHome}

func TreatmentSettings() []TreatmentSetting { return slices.Clone(treatmentSettings) }

func (v TreatmentSetting) Valid() bool { return slices.Contains(treatmentSettings, v) }
