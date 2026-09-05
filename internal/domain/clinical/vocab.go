package clinical

import "slices"

// Severity is the four-rung ladder shared by allergy, condition, injury and
// symptom (data-model §1). It has no catch-all: the ladder is complete and
// ordered, and an "other" would break the ordering FR-018's critical
// derivation depends on.
type Severity string

const (
	SeverityMild            Severity = "mild"
	SeverityModerate        Severity = "moderate"
	SeveritySevere          Severity = "severe"
	SeverityLifeThreatening Severity = "life_threatening"
)

// ConditionStatus is the "is it still going on" ladder shared by allergy,
// condition, injury and symptom. Not a state machine: any value may move to
// any other, because a carer correcting a mis-record must be able to.
type ConditionStatus string

const (
	ConditionStatusActive   ConditionStatus = "active"
	ConditionStatusHealing  ConditionStatus = "healing"
	ConditionStatusInactive ConditionStatus = "inactive"
	ConditionStatusResolved ConditionStatus = "resolved"
	ConditionStatusChronic  ConditionStatus = "chronic"
)

// OrderStatus is the ordered-event ladder used by procedure (and, from phase
// 004, lab_result).
type OrderStatus string

const (
	OrderStatusOrdered    OrderStatus = "ordered"
	OrderStatusScheduled  OrderStatus = "scheduled"
	OrderStatusInProgress OrderStatus = "in_progress"
	OrderStatusCompleted  OrderStatus = "completed"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

var (
	severities = []Severity{
		SeverityMild, SeverityModerate, SeveritySevere, SeverityLifeThreatening,
	}

	conditionStatuses = []ConditionStatus{
		ConditionStatusActive, ConditionStatusHealing, ConditionStatusInactive,
		ConditionStatusResolved, ConditionStatusChronic,
	}

	orderStatuses = []OrderStatus{
		OrderStatusOrdered, OrderStatusScheduled, OrderStatusInProgress,
		OrderStatusCompleted, OrderStatusCancelled,
	}
)

func Severities() []Severity               { return slices.Clone(severities) }
func ConditionStatuses() []ConditionStatus { return slices.Clone(conditionStatuses) }
func OrderStatuses() []OrderStatus         { return slices.Clone(orderStatuses) }

func (s Severity) Valid() bool        { return slices.Contains(severities, s) }
func (s ConditionStatus) Valid() bool { return slices.Contains(conditionStatuses, s) }
func (s OrderStatus) Valid() bool     { return slices.Contains(orderStatuses, s) }

func (s Severity) String() string        { return string(s) }
func (s ConditionStatus) String() string { return string(s) }
func (s OrderStatus) String() string     { return string(s) }
