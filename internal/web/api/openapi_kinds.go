package api

import (
	"medikube/internal/domain/kind"
	"medikube/internal/openapi"
	"medikube/internal/records"
)

// OpenAPIKinds is every kind with a documented DTO set, in the kind table's
// order. A kind absent from the map is simply not documented, so the registry
// completeness test asserts every registered kind is present here.
func OpenAPIKinds() []openapi.Kind {
	schemas := map[kind.Kind]records.Schema{
		kind.Medication:       MedicationSchema(),
		kind.Allergy:          AllergySchema(),
		kind.Condition:        ConditionSchema(),
		kind.EmergencyContact: EmergencyContactSchema(),
		kind.Immunization:     ImmunizationSchema(),
		kind.Injury:           InjurySchema(),
		kind.Insurance:        InsuranceSchema(),
		kind.Equipment:        EquipmentSchema(),
		kind.Symptom:          SymptomSchema(),
		kind.Vitals:           VitalsSchema(),
		kind.Encounter:        EncounterSchema(),
		kind.Procedure:        ProcedureSchema(),
		kind.Treatment:        TreatmentSchema(),
		kind.FamilyMember:     FamilyMemberSchema(),
	}

	kinds := make([]openapi.Kind, 0, len(schemas))
	for _, k := range kind.Kinds() {
		schema, ok := schemas[k]
		if !ok {
			continue
		}

		kinds = append(kinds, openapi.Kind{
			Enum:    k.Enum(),
			Segment: k.Segment(),
			Summary: schema.NewSummary(),
			Detail:  schema.NewDetail(),
			Create:  schema.NewCreate(),
			Patch:   schema.NewPatch(),
		})
	}

	return kinds
}
