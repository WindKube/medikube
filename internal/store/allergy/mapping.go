package allergy

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/store"
)

func recordFromAllergy(record *core.Record) (clinical.Allergy, error) {
	if record.Collection().Name != collectionName {
		return clinical.Allergy{}, fmt.Errorf("allergy: expected the %s collection, got %s", collectionName, record.Collection().Name)
	}

	onsetOn, err := recordDate(record, fieldOnsetOn)
	if err != nil {
		return clinical.Allergy{}, err
	}

	return clinical.Allergy{
		ID:        record.Id,
		PatientID: record.GetString(fieldPatient),
		Allergen:  record.GetString(fieldAllergen),
		Reaction:  record.GetString(fieldReaction),
		Severity:  clinical.Severity(record.GetString(fieldSeverity)),
		Status:    clinical.ConditionStatus(record.GetString(fieldStatus)),
		OnsetOn:   onsetOn,
		Notes:     record.GetString(fieldNotes),
		Tags:      record.GetStringSlice(fieldTags),
		CreatedAt: recordInstant(record, fieldCreated),
		UpdatedAt: recordInstant(record, fieldUpdated),
		Version:   store.Version(record),
	}, nil
}

func allergyToRecord(record *core.Record, entity clinical.Allergy) error {
	if record.Collection().Name != collectionName {
		return fmt.Errorf("allergy: expected the %s collection, got %s", collectionName, record.Collection().Name)
	}

	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldAllergen, entity.Allergen)
	record.Set(fieldReaction, entity.Reaction)
	record.Set(fieldSeverity, string(entity.Severity))
	record.Set(fieldStatus, string(entity.Status))
	setDate(record, fieldOnsetOn, entity.OnsetOn)
	record.Set(fieldNotes, entity.Notes)
	record.Set(fieldTags, entity.Tags)

	return nil
}

// recordDate and setDate mirror internal/store's own unexported helpers
// (mapping.go): each store package that owns a collection carries its own
// copy so no store package depends on another's collection.
func recordDate(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("allergy: %s is not a calendar date: %w", field, err)
	}

	return date, nil
}

func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
