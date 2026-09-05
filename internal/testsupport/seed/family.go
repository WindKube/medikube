package seed

import (
	"encoding/json"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// The family member ids. Seeded against account A's parent patient rather
// than the self-record, so /family-history stays empty on the self-record
// for the empty-state smoke case (US10, contracts/pages.md).
const (
	FamilyMemberGrandmotherID = "mkfamamara00001"
	FamilyMemberBrotherID     = "mkfamamara00002"
	FamilyMemberUncleID       = "mkfamamara00003"
)

func familyDiagnosedAge(v int) *int { return &v }

// FamilyMembers is three relatives recorded against account A's parent
// patient (US10's independent test): differing relationships, one deceased,
// and one carrying two conditions each with its own age at diagnosis.
func FamilyMembers() []clinical.FamilyMember {
	grandmotherBirthYear := 1948
	grandmotherDeathYear := 2018

	return []clinical.FamilyMember{
		{
			ID: FamilyMemberGrandmotherID, PatientID: accountAPatientParentID,
			Name: "Adaeze Okonkwo", Relationship: clinical.FamilyRelationshipGrandmother,
			BirthYear: &grandmotherBirthYear, DeathYear: &grandmotherDeathYear, IsDeceased: true,
			Conditions: []clinical.FamilyCondition{
				{
					Name: "Breast cancer", DiagnosedAge: familyDiagnosedAge(62),
					Severity: clinical.SeveritySevere, Status: clinical.ConditionStatusResolved,
				},
				{
					Name: "Hypertension", DiagnosedAge: familyDiagnosedAge(55),
					Severity: clinical.SeverityModerate, Status: clinical.ConditionStatusChronic,
				},
			},
		},
		{
			ID: FamilyMemberBrotherID, PatientID: accountAPatientParentID,
			Name: "Chinedu Okonkwo", Relationship: clinical.FamilyRelationshipBrother,
			Conditions: []clinical.FamilyCondition{
				{Name: "Heart arrhythmia", Severity: clinical.SeverityModerate, Status: clinical.ConditionStatusActive},
			},
		},
		{
			ID: FamilyMemberUncleID, PatientID: accountAPatientParentID,
			Name: "Emeka Okonkwo", Relationship: clinical.FamilyRelationshipUncle,
		},
	}
}

const (
	columnFamilyRelationship = "relationship"
	columnFamilySex          = "sex"
	columnFamilyBirthYear    = "birth_year"
	columnFamilyDeathYear    = "death_year"
	columnFamilyIsDeceased   = "is_deceased"
	columnFamilyConditions   = "conditions"
)

type wireFamilyCondition struct {
	Name         string `json:"name"`
	ICD10Code    string `json:"icd10_code,omitempty"`
	DiagnosedAge *int   `json:"diagnosed_age,omitempty"`
	Severity     string `json:"severity,omitempty"`
	Status       string `json:"status,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

func applyFamilyMembers(app core.App) error {
	name := kind.FamilyMember.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnName, columnFamilyRelationship, columnFamilySex,
		columnFamilyBirthYear, columnFamilyDeathYear, columnFamilyIsDeceased, columnFamilyConditions,
	); err != nil {
		return err
	}

	for _, member := range FamilyMembers() {
		if err := member.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", member.ID, err)
		}

		record, err := findOrNew(app, collection, member.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, member.PatientID)
		record.Set(columnName, member.Name)
		record.Set(columnFamilyRelationship, string(member.Relationship))
		record.Set(columnFamilySex, string(member.Sex))
		record.Set(columnFamilyBirthYear, member.BirthYear)
		record.Set(columnFamilyDeathYear, member.DeathYear)
		record.Set(columnFamilyIsDeceased, member.IsDeceased)

		encoded, err := encodeFamilyConditions(member.Conditions)
		if err != nil {
			return fmt.Errorf("seeding %s: %w", member.ID, err)
		}

		record.Set(columnFamilyConditions, encoded)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", member.ID, err)
		}

		if err := IndexRecord(app, kind.FamilyMember, member.ID, member.PatientID,
			member.Name, "", domain.Date{}); err != nil {
			return err
		}
	}

	return nil
}

func encodeFamilyConditions(conditions []clinical.FamilyCondition) (string, error) {
	wire := make([]wireFamilyCondition, 0, len(conditions))

	for _, condition := range conditions {
		wire = append(wire, wireFamilyCondition{
			Name:         condition.Name,
			ICD10Code:    condition.ICD10Code,
			DiagnosedAge: condition.DiagnosedAge,
			Severity:     string(condition.Severity),
			Status:       string(condition.Status),
			Notes:        condition.Notes,
		})
	}

	encoded, err := json.Marshal(wire)
	if err != nil {
		return "", fmt.Errorf("encoding conditions: %w", err)
	}

	return string(encoded), nil
}
