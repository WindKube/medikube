package records

import "medikube/internal/domain/kind"

// Field names US2's three kinds add that medication.go does not already
// declare. Several columns are shared verbatim with medication (name, type,
// status, started_on, ended_on, frequency, dosage, notes) and FieldLabel
// below reads all of them from one map, so those are not redeclared here.
const (
	FieldReason          = "reason"
	FieldOccurredOn      = "occurred_on"
	FieldVisitType       = "visit_type"
	FieldPriority        = "priority"
	FieldAssessment      = "assessment"
	FieldPlan            = "plan"
	FieldFollowUp        = "follow_up"
	FieldDurationMin     = "duration_minutes"
	FieldCode            = "code"
	FieldDescription     = "description"
	FieldOutcome         = "outcome"
	FieldSetting         = "setting"
	FieldComplications   = "complications"
	FieldAnesthesia      = "anesthesia"
	FieldAnesthesiaNote  = "anesthesia_notes"
	FieldExpectedOutcome = "expected_outcome"
	FieldPractitioner    = "practitioner"
	FieldFacility        = "facility"
	FieldCondition       = "condition"
)

// Named after the collections they relate to, per data-model §4.5 (FR-028),
// so declared from the kind table rather than spelled a second time here.
var (
	FieldEncounters = kind.Encounter.Collection()
	FieldEquipment  = kind.Equipment.Collection()
)

// init extends the shared label table medication.go declares (fieldLabels),
// one entry per new field. Fields that share a name and a meaning with
// medication's own (status, notes, name, type, started_on, ended_on,
// frequency, dosage) keep the label medication.go already gave that name.
// Values are message ids (D-06), resolved at render time.
func init() {
	for field, label := range map[string]string{
		FieldReason:          "field.reason",
		FieldOccurredOn:      "field.occurred_on",
		FieldVisitType:       "field.visit_type",
		FieldPriority:        "field.priority",
		FieldAssessment:      "field.assessment",
		FieldPlan:            "field.plan",
		FieldFollowUp:        "field.follow_up",
		FieldDurationMin:     "field.duration_minutes",
		FieldCode:            "field.code",
		FieldDescription:     "field.description",
		FieldOutcome:         "field.outcome",
		FieldSetting:         "field.setting",
		FieldComplications:   "field.complications",
		FieldAnesthesia:      "field.anesthesia",
		FieldAnesthesiaNote:  "field.anesthesia_notes",
		FieldExpectedOutcome: "field.expected_outcome",
		FieldPractitioner:    "field.related_practitioner",
		FieldFacility:        "field.place_of_care",
		FieldCondition:       "field.related_condition",
		FieldEncounters:      "field.related_encounters",
		FieldEquipment:       "field.related_equipment",
	} {
		fieldLabels[field] = label
	}
}
