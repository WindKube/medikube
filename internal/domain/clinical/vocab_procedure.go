package clinical

import "slices"

// ProcedureSetting is where a procedure took place.
type ProcedureSetting string

const (
	ProcedureSettingOutpatient ProcedureSetting = "outpatient"
	ProcedureSettingInpatient  ProcedureSetting = "inpatient"
	ProcedureSettingOffice     ProcedureSetting = "office"
)

// ProcedureType is FR-024's kind of procedure, with a catch-all.
type ProcedureType string

const (
	ProcedureTypeSurgical    ProcedureType = "surgical"
	ProcedureTypeDiagnostic  ProcedureType = "diagnostic"
	ProcedureTypeTherapeutic ProcedureType = "therapeutic"
	ProcedureTypePreventive  ProcedureType = "preventive"
	ProcedureTypeOther       ProcedureType = "other"
)

// ProcedureOutcome is how it went.
type ProcedureOutcome string

const (
	ProcedureOutcomeSuccessful    ProcedureOutcome = "successful"
	ProcedureOutcomePartial       ProcedureOutcome = "partial"
	ProcedureOutcomeUnsuccessful  ProcedureOutcome = "unsuccessful"
	ProcedureOutcomeComplications ProcedureOutcome = "complications"
)

// Anesthesia is what was used, if anything.
type Anesthesia string

const (
	AnesthesiaNone     Anesthesia = "none"
	AnesthesiaLocal    Anesthesia = "local"
	AnesthesiaRegional Anesthesia = "regional"
	AnesthesiaSedation Anesthesia = "sedation"
	AnesthesiaGeneral  Anesthesia = "general"
)

var (
	procedureSettings = []ProcedureSetting{ProcedureSettingOutpatient, ProcedureSettingInpatient, ProcedureSettingOffice}

	procedureTypes = []ProcedureType{
		ProcedureTypeSurgical, ProcedureTypeDiagnostic, ProcedureTypeTherapeutic,
		ProcedureTypePreventive, ProcedureTypeOther,
	}

	procedureOutcomes = []ProcedureOutcome{
		ProcedureOutcomeSuccessful, ProcedureOutcomePartial,
		ProcedureOutcomeUnsuccessful, ProcedureOutcomeComplications,
	}

	anesthesias = []Anesthesia{
		AnesthesiaNone, AnesthesiaLocal, AnesthesiaRegional, AnesthesiaSedation, AnesthesiaGeneral,
	}
)

func ProcedureSettings() []ProcedureSetting { return slices.Clone(procedureSettings) }
func ProcedureTypes() []ProcedureType       { return slices.Clone(procedureTypes) }
func ProcedureOutcomes() []ProcedureOutcome { return slices.Clone(procedureOutcomes) }
func Anesthesias() []Anesthesia             { return slices.Clone(anesthesias) }

func (v ProcedureSetting) Valid() bool { return slices.Contains(procedureSettings, v) }
func (v ProcedureType) Valid() bool    { return slices.Contains(procedureTypes, v) }
func (v ProcedureOutcome) Valid() bool { return slices.Contains(procedureOutcomes, v) }
func (v Anesthesia) Valid() bool       { return slices.Contains(anesthesias, v) }
