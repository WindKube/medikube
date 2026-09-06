package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/person"
)

// data-model §4.13's column bounds (belt and suspenders: clinical.FamilyMember
// and clinical.FamilyCondition are the authority, this is what stops a row
// entering through any other door).
const (
	familyNameMax          = 100
	familyYearMin          = 1850
	familyYearMax          = 2200
	familyConditionsMaxLen = 50 * 400 // dbx JSONField has no per-entry bound; enforced in the domain
)

const (
	familyFieldPatient      = "patient"
	familyFieldName         = "name"
	familyFieldRelationship = "relationship"
	familyFieldSex          = "sex"
	familyFieldBirthYear    = "birth_year"
	familyFieldDeathYear    = "death_year"
	familyFieldIsDeceased   = "is_deceased"
	familyFieldConditions   = "conditions"
)

func init() {
	register(familyMembersUp, familyMembersDown)
}

func familyMembersUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	collection := core.NewBaseCollection(kind.FamilyMember.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          familyFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     familyFieldName,
		Required: true,
		Min:      1,
		Max:      familyNameMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      familyFieldRelationship,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(clinical.FamilyRelationships()),
	})
	collection.Fields.Add(&core.SelectField{
		Name:      familyFieldSex,
		MaxSelect: 1,
		Values:    enumValues(person.Sexes()),
	})
	collection.Fields.Add(&core.NumberField{
		Name: familyFieldBirthYear,
		Min:  ptr(float64(familyYearMin)),
		Max:  ptr(float64(familyYearMax)),
	})
	collection.Fields.Add(&core.NumberField{
		Name: familyFieldDeathYear,
		Min:  ptr(float64(familyYearMin)),
		Max:  ptr(float64(familyYearMax)),
	})
	collection.Fields.Add(&core.BoolField{Name: familyFieldIsDeceased})
	collection.Fields.Add(&core.JSONField{Name: familyFieldConditions, MaxSize: familyConditionsMaxLen})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.FamilyMember.Collection()
	collection.AddIndex("idx_family_patient", false,
		familyFieldPatient+", "+familyFieldRelationship+", LOWER("+familyFieldName+"), id", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func familyMembersDown(app core.App) error {
	return deleteCollection(app, kind.FamilyMember.Collection())
}
