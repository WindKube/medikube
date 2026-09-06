package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// data-model §4.10's column bounds.
const (
	insuranceCompanyMax    = 200
	insurancePlanNameMax   = 200
	insuranceEmployerMax   = 200
	insuranceMemberNameMax = 200
	insuranceMemberIDMax   = 80
	insuranceGroupNumMax   = 80
	insuranceHolderNameMax = 200
	insuranceNotesMax      = 5000
)

const (
	insuranceFieldPatient       = "patient"
	insuranceFieldType          = "type"
	insuranceFieldCompany       = "company"
	insuranceFieldPlanName      = "plan_name"
	insuranceFieldEmployerGroup = "employer_group"
	insuranceFieldMemberName    = "member_name"
	insuranceFieldMemberID      = "member_id"
	insuranceFieldGroupNumber   = "group_number"
	insuranceFieldHolderName    = "holder_name"
	insuranceFieldRelationship  = "relationship_to_holder"
	insuranceFieldEffectiveOn   = "effective_on"
	insuranceFieldExpiresOn     = "expires_on"
	insuranceFieldStatus        = "status"
	insuranceFieldIsPrimary     = "is_primary"
	insuranceFieldCoverage      = "coverage"
	insuranceFieldContact       = "contact"
	insuranceFieldNotes         = "notes"
	insuranceFieldTags          = "tags"
)

func init() {
	register(insurancesUp, insurancesDown)
}

func insurancesUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection := core.NewBaseCollection(kind.Insurance.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          insuranceFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      insuranceFieldType,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(clinical.InsuranceTypes()),
	})
	collection.Fields.Add(&core.TextField{
		Name:     insuranceFieldCompany,
		Required: true,
		Min:      1,
		Max:      insuranceCompanyMax,
	})
	collection.Fields.Add(&core.TextField{Name: insuranceFieldPlanName, Max: insurancePlanNameMax})
	collection.Fields.Add(&core.TextField{Name: insuranceFieldEmployerGroup, Max: insuranceEmployerMax})
	collection.Fields.Add(&core.TextField{
		Name:     insuranceFieldMemberName,
		Required: true,
		Min:      1,
		Max:      insuranceMemberNameMax,
	})
	collection.Fields.Add(&core.TextField{
		Name:     insuranceFieldMemberID,
		Required: true,
		Min:      1,
		Max:      insuranceMemberIDMax,
	})
	collection.Fields.Add(&core.TextField{Name: insuranceFieldGroupNumber, Max: insuranceGroupNumMax})
	collection.Fields.Add(&core.TextField{Name: insuranceFieldHolderName, Max: insuranceHolderNameMax})
	collection.Fields.Add(&core.SelectField{
		Name:      insuranceFieldRelationship,
		MaxSelect: 1,
		Values:    enumValues(clinical.HolderRelationships()),
	})
	collection.Fields.Add(&core.DateField{Name: insuranceFieldEffectiveOn, Required: true})
	collection.Fields.Add(&core.DateField{Name: insuranceFieldExpiresOn})
	collection.Fields.Add(&core.SelectField{
		Name:      insuranceFieldStatus,
		MaxSelect: 1,
		Values:    enumValues(clinical.InsuranceStatuses()),
	})
	collection.Fields.Add(&core.BoolField{Name: insuranceFieldIsPrimary})
	collection.Fields.Add(&core.JSONField{Name: insuranceFieldCoverage})
	collection.Fields.Add(&core.JSONField{Name: insuranceFieldContact})
	collection.Fields.Add(&core.TextField{Name: insuranceFieldNotes, Max: insuranceNotesMax})
	collection.Fields.Add(&core.RelationField{
		Name:         insuranceFieldTags,
		MaxSelect:    unlimitedTags,
		CollectionId: tags.Id,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Insurance.Collection()
	collection.AddIndex("idx_"+name+"_patient_eff", false,
		insuranceFieldPatient+", "+insuranceFieldEffectiveOn+", id", "")
	collection.AddIndex("idx_"+name+"_patient_expires", false,
		insuranceFieldPatient+", "+insuranceFieldExpiresOn, "")

	// The partial unique index is FR-045's structural guarantee: at most one
	// primary policy per patient, enforced by the database and not only by
	// the service's displacement logic — a second write path could not create
	// a second primary even if it forgot to displace the first.
	collection.AddIndex("uniq_"+name+"_primary", true,
		insuranceFieldPatient, insuranceFieldIsPrimary+" = 1")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func insurancesDown(app core.App) error {
	return deleteCollection(app, kind.Insurance.Collection())
}
