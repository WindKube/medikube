package clinical

import "medikube/internal/domain"

const (
	maxInjuryName    = 300
	maxBodyPart      = 100
	maxMechanism     = 500
	maxRecoveryNotes = 2000
)

// Validate reports every offending field at once (FR-027), in data-model
// §4.9's column order.
//
// InjuryType is checked against Valid() the same as every other published
// select — there is no code path anywhere in this package that adds a value
// to injuryTypes, which is FR-040's "fixed vocabulary... cannot be extended"
// made structural rather than a rule this method enforces at the boundary.
func (i Injury) Validate() error {
	var invalid domain.ValidationError

	if name := i.Name; len(name) < 2 {
		invalid.Add("name", domain.CodeRequired, "a name of at least two characters is required")
	} else {
		checkLength(&invalid, "name", "the name", name, maxInjuryName)
	}

	if i.Type != "" && !i.Type.Valid() {
		invalid.Add("type", domain.CodeInvalidValue, "not one of the types MediKube accepts")
	}

	if bodyPart := i.BodyPart; bodyPart == "" {
		invalid.Add("body_part", domain.CodeRequired, "the part of the body is required")
	} else {
		checkLength(&invalid, "body_part", "the part of the body", bodyPart, maxBodyPart)
	}

	if i.Laterality != "" && !i.Laterality.Valid() {
		invalid.Add("laterality", domain.CodeInvalidValue, "not one of the sides MediKube accepts")
	}

	if fieldErr := NotFuture(Ref{Field: "occurred_on", Value: i.OccurredOn}, Today()); fieldErr != nil {
		invalid.Fields = append(invalid.Fields, *fieldErr)
	}

	checkLength(&invalid, "mechanism", "how it happened", i.Mechanism, maxMechanism)

	if i.Severity != "" && !i.Severity.Valid() {
		invalid.Add("severity", domain.CodeInvalidValue, "not one of the severities MediKube accepts")
	}

	if i.Status != "" && !i.Status.Valid() {
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	checkLength(&invalid, "recovery_notes", "the recovery notes", i.RecoveryNotes, maxRecoveryNotes)

	return invalid.OrNil()
}
