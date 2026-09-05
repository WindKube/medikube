package clinical

import "slices"

// VisitType is what kind of visit an encounter was, with a catch-all.
type VisitType string

const (
	VisitTypeOffice     VisitType = "office"
	VisitTypeTelehealth VisitType = "telehealth"
	VisitTypeUrgentCare VisitType = "urgent_care"
	VisitTypeEmergency  VisitType = "emergency"
	VisitTypeInpatient  VisitType = "inpatient"
	VisitTypeFollowUp   VisitType = "follow_up"
	VisitTypeAnnual     VisitType = "annual"
	VisitTypeOther      VisitType = "other"
)

// VisitPriority is how urgent the encounter was.
type VisitPriority string

const (
	VisitPriorityRoutine   VisitPriority = "routine"
	VisitPriorityUrgent    VisitPriority = "urgent"
	VisitPriorityEmergency VisitPriority = "emergency"
)

var (
	visitTypes = []VisitType{
		VisitTypeOffice, VisitTypeTelehealth, VisitTypeUrgentCare, VisitTypeEmergency,
		VisitTypeInpatient, VisitTypeFollowUp, VisitTypeAnnual, VisitTypeOther,
	}

	visitPriorities = []VisitPriority{VisitPriorityRoutine, VisitPriorityUrgent, VisitPriorityEmergency}
)

func VisitTypes() []VisitType          { return slices.Clone(visitTypes) }
func VisitPriorities() []VisitPriority { return slices.Clone(visitPriorities) }

func (v VisitType) Valid() bool     { return slices.Contains(visitTypes, v) }
func (v VisitPriority) Valid() bool { return slices.Contains(visitPriorities, v) }
