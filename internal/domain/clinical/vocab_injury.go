package clinical

import "slices"

// Laterality is which side, including not_applicable for an injury with no
// side (FR-041).
type Laterality string

const (
	LateralityLeft          Laterality = "left"
	LateralityRight         Laterality = "right"
	LateralityBilateral     Laterality = "bilateral"
	LateralityNotApplicable Laterality = "not_applicable"
)

// InjuryType is the fixed vocabulary that replaces upstream's user-extensible
// injury_types collection entirely (FR-040). There is no code path that adds a
// value to it.
type InjuryType string

const (
	InjuryTypeSprain      InjuryType = "sprain"
	InjuryTypeStrain      InjuryType = "strain"
	InjuryTypeFracture    InjuryType = "fracture"
	InjuryTypeDislocation InjuryType = "dislocation"
	InjuryTypeLaceration  InjuryType = "laceration"
	InjuryTypeContusion   InjuryType = "contusion"
	InjuryTypeBurn        InjuryType = "burn"
	InjuryTypeConcussion  InjuryType = "concussion"
	InjuryTypePuncture    InjuryType = "puncture"
	InjuryTypeAbrasion    InjuryType = "abrasion"
	InjuryTypeOther       InjuryType = "other"
)

var (
	lateralities = []Laterality{LateralityLeft, LateralityRight, LateralityBilateral, LateralityNotApplicable}

	injuryTypes = []InjuryType{
		InjuryTypeSprain, InjuryTypeStrain, InjuryTypeFracture, InjuryTypeDislocation,
		InjuryTypeLaceration, InjuryTypeContusion, InjuryTypeBurn, InjuryTypeConcussion,
		InjuryTypePuncture, InjuryTypeAbrasion, InjuryTypeOther,
	}
)

func Lateralities() []Laterality { return slices.Clone(lateralities) }
func InjuryTypes() []InjuryType  { return slices.Clone(injuryTypes) }

func (v Laterality) Valid() bool { return slices.Contains(lateralities, v) }
func (v InjuryType) Valid() bool { return slices.Contains(injuryTypes, v) }
