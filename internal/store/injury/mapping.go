package injury

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

func recordFromInjury(record *core.Record) (clinical.Injury, error) {
	occurredOn, err := recordDate(record, fieldOccurredOn)
	if err != nil {
		return clinical.Injury{}, err
	}

	return clinical.Injury{
		ID:             record.Id,
		PatientID:      record.GetString(fieldPatient),
		PractitionerID: record.GetString(fieldPractitioner),
		Name:           record.GetString(fieldName),
		Type:           clinical.InjuryType(record.GetString(fieldType)),
		BodyPart:       record.GetString(fieldBodyPart),
		Laterality:     clinical.Laterality(record.GetString(fieldLaterality)),
		OccurredOn:     occurredOn,
		Mechanism:      record.GetString(fieldMechanism),
		Severity:       clinical.Severity(record.GetString(fieldSeverity)),
		Status:         clinical.ConditionStatus(record.GetString(fieldStatus)),
		RecoveryNotes:  record.GetString(fieldRecoveryNotes),
		MedicationIDs:  record.GetStringSlice(fieldMedications),
		CreatedAt:      recordInstant(record, fieldCreated),
		UpdatedAt:      recordInstant(record, fieldUpdated),
		Version:        store.Version(record),
	}, nil
}

func injuryToRecord(record *core.Record, entity clinical.Injury) error {
	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldPractitioner, entity.PractitionerID)
	record.Set(fieldName, entity.Name)
	record.Set(fieldType, string(entity.Type))
	record.Set(fieldBodyPart, entity.BodyPart)
	record.Set(fieldLaterality, string(entity.Laterality))
	setDate(record, fieldOccurredOn, entity.OccurredOn)
	record.Set(fieldMechanism, entity.Mechanism)
	record.Set(fieldSeverity, string(entity.Severity))
	record.Set(fieldStatus, string(entity.Status))
	record.Set(fieldRecoveryNotes, entity.RecoveryNotes)
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
		return domain.Date{}, fmt.Errorf("%s.%s is not a calendar date: %w", kind.Injury.Collection(), field, err)
	}

	return date, nil
}

func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
