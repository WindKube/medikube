package seed

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// Three tags account A holds, applied across every registered kind's own
// seeded rows (US7). Phase 003 currently registers seven kinds
// (medication, immunization, injury, insurance, equipment, symptom,
// vitals); T163 asks for at least eight, and there is no eighth kind on
// this branch to carry one — every kind this build has gets at least one
// of these three.
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
		collection string
		recordID   string
		tags       []string
	}{
		{kind.Medication.Collection(), NameOnlyID, []string{TagChronicID, TagFollowUpID}},
		{kind.Immunization.Collection(), ImmunizationSampleID, []string{TagFlaggedID}},
		{kind.Injury.Collection(), InjuryAnkleID, []string{TagFollowUpID}},
		{kind.Insurance.Collection(), InsurancePrimaryID, []string{TagChronicID}},
		{kind.Equipment.Collection(), EquipmentOverdueID, []string{TagFlaggedID, TagChronicID}},
		{kind.Symptom.Collection(), SymptomHeadacheOne, []string{TagFollowUpID}},
		{kind.Vitals.Collection(), VitalsOne, []string{TagChronicID}},
	}

	for _, target := range targets {
		if err := applyTagsTo(app, target.collection, target.recordID, target.tags); err != nil {
			return err
		}
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
