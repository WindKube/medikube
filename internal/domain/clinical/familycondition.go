package clinical

import (
	"unicode/utf8"

	"medikube/internal/domain"
)

// data-model §6.1's bounds (FR-053, FR-054).
const (
	familyConditionNameMax  = 300
	familyConditionCodeMax  = 10
	familyConditionNotesMax = 2000
	familyConditionAgeMin   = 0
	familyConditionAgeMax   = 130
	familyConditionsMax     = 50
)

// FamilyCondition is one condition a relative had, recorded against a
// family_members row (data-model §6.1, FR-053). It is a validated JSON value
// object, not a collection of its own.
type FamilyCondition struct {
	Name         string
	ICD10Code    string
	DiagnosedAge *int
	Severity     Severity
	Status       ConditionStatus
	Notes        string
}

// ValidateFamilyConditions enforces the list bound and validates every entry
// independently, reporting every offence in one *domain.ValidationError
// (FR-004): a submission with three bad entries names all three rather than
// stopping at the first.
func ValidateFamilyConditions(conditions []FamilyCondition) error {
	var invalid domain.ValidationError

	if len(conditions) > familyConditionsMax {
		invalid.Addf("conditions", domain.CodeTooLong, "conditions accepts at most %d entries", familyConditionsMax)

		return invalid.OrNil()
	}

	for _, condition := range conditions {
		condition.validate(&invalid)
	}

	return invalid.OrNil()
}

func (c FamilyCondition) validate(invalid *domain.ValidationError) {
	if c.Name == "" {
		invalid.Add("conditions", domain.CodeRequired, "a condition's name is required")
	} else if utf8.RuneCountInString(c.Name) > familyConditionNameMax {
		invalid.Addf("conditions", domain.CodeTooLong, "a condition's name accepts at most %d characters", familyConditionNameMax)
	}

	if utf8.RuneCountInString(c.ICD10Code) > familyConditionCodeMax {
		invalid.Addf("conditions", domain.CodeTooLong, "a condition's clinical code accepts at most %d characters", familyConditionCodeMax)
	}

	if c.DiagnosedAge != nil && (*c.DiagnosedAge < familyConditionAgeMin || *c.DiagnosedAge > familyConditionAgeMax) {
		invalid.Addf("conditions", domain.CodeOutOfRange,
			"a condition's age at diagnosis is between %d and %d", familyConditionAgeMin, familyConditionAgeMax)
	}

	if c.Severity != "" && !c.Severity.Valid() {
		invalid.Add("conditions", domain.CodeInvalidValue, "not one of the severities MediKube accepts")
	}

	if c.Status != "" && !c.Status.Valid() {
		invalid.Add("conditions", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	if utf8.RuneCountInString(c.Notes) > familyConditionNotesMax {
		invalid.Addf("conditions", domain.CodeTooLong, "a condition's notes accept at most %d characters", familyConditionNotesMax)
	}
}
