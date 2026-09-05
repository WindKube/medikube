package condition

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/store"
)

func recordFromCondition(record *core.Record) (clinical.Condition, error) {
	if record.Collection().Name != collectionName {
		return clinical.Condition{}, fmt.Errorf("condition: expected the %s collection, got %s", collectionName, record.Collection().Name)
	}

	onsetOn, err := recordDate(record, fieldOnsetOn)
	if err != nil {
		return clinical.Condition{}, err
	}

	resolvedOn, err := recordDate(record, fieldResolvedOn)
	if err != nil {
		return clinical.Condition{}, err
	}

	return clinical.Condition{
		ID:             record.Id,
		PatientID:      record.GetString(fieldPatient),
		Diagnosis:      record.GetString(fieldDiagnosis),
		Status:         clinical.ConditionStatus(record.GetString(fieldStatus)),
		Severity:       clinical.Severity(record.GetString(fieldSeverity)),
		OnsetOn:        onsetOn,
		ResolvedOn:     resolvedOn,
		ICD10Code:      record.GetString(fieldICD10Code),
		SNOMEDCode:     record.GetString(fieldSNOMEDCode),
		PractitionerID: record.GetString(fieldPractitioner),
		Notes:          record.GetString(fieldNotes),
		Tags:           record.GetStringSlice(fieldTags),
		MedicationIDs:  record.GetStringSlice(fieldMedications),
		CreatedAt:      recordInstant(record, fieldCreated),
		UpdatedAt:      recordInstant(record, fieldUpdated),
		Version:        store.Version(record),
	}, nil
}

func conditionToRecord(record *core.Record, entity clinical.Condition) error {
	if record.Collection().Name != collectionName {
		return fmt.Errorf("condition: expected the %s collection, got %s", collectionName, record.Collection().Name)
	}

	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldDiagnosis, entity.Diagnosis)
	record.Set(fieldStatus, string(entity.Status))
	record.Set(fieldSeverity, string(entity.Severity))
	setDate(record, fieldOnsetOn, entity.OnsetOn)
	setDate(record, fieldResolvedOn, entity.ResolvedOn)
	record.Set(fieldICD10Code, entity.ICD10Code)
	record.Set(fieldSNOMEDCode, entity.SNOMEDCode)
	record.Set(fieldPractitioner, entity.PractitionerID)
	record.Set(fieldNotes, entity.Notes)
	record.Set(fieldTags, entity.Tags)
	record.Set(fieldMedications, entity.MedicationIDs)

	return nil
}

func recordDate(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("condition: %s is not a calendar date: %w", field, err)
	}

	return date, nil
}

func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
