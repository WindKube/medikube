package clinical

import "slices"

// EquipmentType is the kind of medical equipment, with a catch-all.
type EquipmentType string

const (
	EquipmentTypeCPAP         EquipmentType = "cpap"
	EquipmentTypeNebulizer    EquipmentType = "nebulizer"
	EquipmentTypeWheelchair   EquipmentType = "wheelchair"
	EquipmentTypeWalker       EquipmentType = "walker"
	EquipmentTypeGlucoseMeter EquipmentType = "glucose_meter"
	EquipmentTypeBPMonitor    EquipmentType = "bp_monitor"
	EquipmentTypeOximeter     EquipmentType = "oximeter"
	EquipmentTypeOxygen       EquipmentType = "oxygen"
	EquipmentTypeHearingAid   EquipmentType = "hearing_aid"
	EquipmentTypeProsthetic   EquipmentType = "prosthetic"
	EquipmentTypeOrthotic     EquipmentType = "orthotic"
	EquipmentTypeOther        EquipmentType = "other"
)

var equipmentTypes = []EquipmentType{
	EquipmentTypeCPAP, EquipmentTypeNebulizer, EquipmentTypeWheelchair, EquipmentTypeWalker,
	EquipmentTypeGlucoseMeter, EquipmentTypeBPMonitor, EquipmentTypeOximeter, EquipmentTypeOxygen,
	EquipmentTypeHearingAid, EquipmentTypeProsthetic, EquipmentTypeOrthotic, EquipmentTypeOther,
}

func EquipmentTypes() []EquipmentType { return slices.Clone(equipmentTypes) }

func (v EquipmentType) Valid() bool { return slices.Contains(equipmentTypes, v) }
