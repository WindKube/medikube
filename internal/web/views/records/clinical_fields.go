package records

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
	FieldEncounters      = "encounters"
	FieldEquipment       = "equipment"
)

// init extends the shared label table medication.go declares (fieldLabels),
// one entry per new field. Fields that share a name and a meaning with
// medication's own (status, notes, name, type, started_on, ended_on,
// frequency, dosage) keep the label medication.go already gave that name.
func init() {
	for field, label := range map[string]string{
		FieldReason:          "Reason for the visit",
		FieldOccurredOn:      "Date",
		FieldVisitType:       "Visit type",
		FieldPriority:        "Priority",
		FieldAssessment:      "Assessment",
		FieldPlan:            "Plan",
		FieldFollowUp:        "Follow-up",
		FieldDurationMin:     "Duration (minutes)",
		FieldCode:            "Code",
		FieldDescription:     "Description",
		FieldOutcome:         "Outcome",
		FieldSetting:         "Setting",
		FieldComplications:   "Complications",
		FieldAnesthesia:      "Anesthesia",
		FieldAnesthesiaNote:  "Anesthesia notes",
		FieldExpectedOutcome: "Expected outcome",
		FieldPractitioner:    "Practitioner",
		FieldFacility:        "Place of care",
		FieldEncounters:      "Related encounters",
		FieldEquipment:       "Related equipment",
	} {
		fieldLabels[field] = label
	}
}
