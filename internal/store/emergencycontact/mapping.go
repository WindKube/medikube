package emergencycontact

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/store"
)

func recordFromContact(record *core.Record) (clinical.EmergencyContact, error) {
	if record.Collection().Name != collectionName {
		return clinical.EmergencyContact{}, fmt.Errorf("emergencycontact: expected the %s collection, got %s", collectionName, record.Collection().Name)
	}

	return clinical.EmergencyContact{
		ID:           record.Id,
		PatientID:    record.GetString(fieldPatient),
		Name:         record.GetString(fieldName),
		Relationship: clinical.ContactRelationship(record.GetString(fieldRelationship)),
		Phone:        record.GetString(fieldPhone),
		PhoneAlt:     record.GetString(fieldPhoneAlt),
		Email:        record.GetString(fieldEmail),
		Address:      record.GetString(fieldAddress),
		IsPrimary:    record.GetBool(fieldIsPrimary),
		IsActive:     record.GetBool(fieldIsActive),
		Notes:        record.GetString(fieldNotes),
		Tags:         record.GetStringSlice(fieldTags),
		CreatedAt:    recordInstant(record, fieldCreated),
		UpdatedAt:    recordInstant(record, fieldUpdated),
		Version:      store.Version(record),
	}, nil
}

func contactToRecord(record *core.Record, entity clinical.EmergencyContact) error {
	if record.Collection().Name != collectionName {
		return fmt.Errorf("emergencycontact: expected the %s collection, got %s", collectionName, record.Collection().Name)
	}

	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldName, entity.Name)
	record.Set(fieldRelationship, string(entity.Relationship))
	record.Set(fieldPhone, entity.Phone)
	record.Set(fieldPhoneAlt, entity.PhoneAlt)
	record.Set(fieldEmail, entity.Email)
	record.Set(fieldAddress, entity.Address)
	record.Set(fieldIsPrimary, entity.IsPrimary)
	record.Set(fieldIsActive, entity.IsActive)
	record.Set(fieldNotes, entity.Notes)
	record.Set(fieldTags, entity.Tags)

	return nil
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
