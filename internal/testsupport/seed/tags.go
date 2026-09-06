package seed

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// Three tags account A holds, applied across every registered kind's own
// seeded rows (US7). Phase 003 registers fourteen kinds; T163 asks for at
// least eight to carry one of these three, and every kind this build has
// gets at least one.
const (
	TagChronicID  = "mktagamara00001"
	TagFollowUpID = "mktagamara00002"
	TagFlaggedID  = "mktagamara00003"
)

// InjuryAnkleID is the sprained-ankle row Injuries() seeds for account A's
// self-record — the same id applyInjuries writes, transcribed here so a
// caller outside this package (internal/store's cross-kind tags test) can
// name it rather than repeat the literal.
const InjuryAnkleID = "mkinjamara00001"

const (
	tagFieldOwner = "owner"
	tagFieldName  = "name"
	tagFieldColor = "color"
	fieldTags     = "tags"
)

// applyTags writes account A's tag vocabulary and applies it across one
// seeded row of every registered kind. Called last, from Apply, so every
// row it touches already exists.
func applyTags(app core.App) error {
	if err := applyTagRows(app); err != nil {
		return err
	}

	targets := []struct {
		k        kind.Kind
		recordID string
		tags     []string
	}{
		{kind.Medication, NameOnlyID, []string{TagChronicID, TagFollowUpID}},
		{kind.Immunization, ImmunizationSampleID, []string{TagFlaggedID}},
		{kind.Injury, InjuryAnkleID, []string{TagFollowUpID}},
		{kind.Insurance, InsurancePrimaryID, []string{TagChronicID}},
		{kind.Equipment, EquipmentOverdueID, []string{TagFlaggedID, TagChronicID}},
		{kind.Symptom, SymptomHeadacheOne, []string{TagFollowUpID}},
		{kind.Vitals, VitalsOne, []string{TagChronicID}},
		{kind.Allergy, CriticalAllergyID, []string{TagFlaggedID}},
		{kind.Condition, ResolvedConditionID, []string{TagChronicID}},
		{kind.EmergencyContact, PrimaryContactID, []string{TagFollowUpID}},
		{kind.Encounter, EncounterNameOnlyID, []string{TagChronicID}},
		{kind.Procedure, ProcedureNameOnlyID, []string{TagFlaggedID}},
		{kind.Treatment, TreatmentNameOnlyID, []string{TagFollowUpID}},
		{kind.FamilyMember, FamilyMemberGrandmotherID, []string{TagChronicID}},
	}

	for _, target := range targets {
		if err := applyTagsTo(app, target.k.Collection(), target.recordID, target.tags); err != nil {
			return err
		}

		// search_index.tags is written by the same fourteen kinds' own
		// indexingService in production (T164-T177 follow-up); the seed
		// writes each record directly rather than through the service, so
		// nothing else keeps this row in step with what applyTagsTo just set.
		if err := indexTags(app, target.k, target.recordID, target.tags); err != nil {
			return err
		}
	}

	return nil
}

func indexTags(app core.App, k kind.Kind, recordID string, tags []string) error {
	collection, err := app.FindCollectionByNameOrId(searchIndexCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", searchIndexCollection, err)
	}

	record := core.NewRecord(collection)
	if err := app.RecordQuery(collection).
		AndWhere(dbx.HashExp{"kind": string(k), "record_id": recordID}).One(record); err != nil {
		return fmt.Errorf("finding %s %s's index row to tag it: %w", k, recordID, err)
	}

	record.Set(fieldTags, tags)

	if err := app.Save(record); err != nil {
		return fmt.Errorf("indexing %s %s's tags: %w", k, recordID, err)
	}

	return nil
}

func applyTagRows(app core.App) error {
	collection, err := app.FindCollectionByNameOrId("tags")
	if err != nil {
		return fmt.Errorf("finding tags: %w", err)
	}

	rows := []struct {
		id    string
		name  string
		color string
	}{
		{TagChronicID, "chronic", "#aa3311"},
		{TagFollowUpID, "follow-up", "#1155aa"},
		{TagFlaggedID, "flagged", "#997700"},
	}

	for _, row := range rows {
		record, err := findOrNew(app, collection, row.id)
		if err != nil {
			return err
		}

		record.Set(tagFieldOwner, AccountAID)
		record.Set(tagFieldName, row.name)
		record.Set(tagFieldColor, row.color)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding tag %s: %w", row.id, err)
		}
	}

	return nil
}

func applyTagsTo(app core.App, collectionName, recordID string, tags []string) error {
	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		return fmt.Errorf("finding %s: %w", collectionName, err)
	}

	record, err := app.FindRecordById(collection, recordID)
	if err != nil {
		return fmt.Errorf("finding %s %s to tag it: %w", collectionName, recordID, err)
	}

	record.Set(fieldTags, tags)

	if err := app.Save(record); err != nil {
		return fmt.Errorf("tagging %s %s: %w", collectionName, recordID, err)
	}

	return nil
}
