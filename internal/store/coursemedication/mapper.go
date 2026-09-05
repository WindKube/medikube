// Package coursemedication is the treatment_medications repository against
// PocketBase.
package coursemedication

import (
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// Collection and its field names mirror internal/store/migrations' own
// unexported constants: duplicated rather than imported, the same way every
// store package already names its own field constants independently. Its
// value is derived from the medications collection's own name rather than
// spelled whole (research D-05).
var Collection = "treatment_" + kind.Medication.Collection()

const (
	fieldTreatment  = "treatment"
	fieldMedication = "medication"
	fieldDosage     = "dosage"
	fieldFrequency  = "frequency"
	fieldDuration   = "duration"
	fieldTiming     = "timing"
	fieldPrescriber = "prescriber"
	fieldPharmacy   = "pharmacy"
	fieldStartedOn  = "started_on"
	fieldEndedOn    = "ended_on"
	fieldCreated    = "created"
	fieldUpdated    = "updated"
)

// ErrUnexpectedCollection is a record handed to the wrong mapper.
var ErrUnexpectedCollection = errors.New("store/coursemedication: the record is not from this collection")

func FromRecord(record *core.Record) (clinical.CourseMedication, error) {
	if err := expectCollection(record); err != nil {
		return clinical.CourseMedication{}, err
	}

	startedOn, err := recordDate(record, fieldStartedOn)
	if err != nil {
		return clinical.CourseMedication{}, err
	}

	endedOn, err := recordDate(record, fieldEndedOn)
	if err != nil {
		return clinical.CourseMedication{}, err
	}

	return clinical.CourseMedication{
		ID:           record.Id,
		TreatmentID:  record.GetString(fieldTreatment),
		MedicationID: record.GetString(fieldMedication),
		Dosage:       record.GetString(fieldDosage),
		Frequency:    record.GetString(fieldFrequency),
		Duration:     record.GetString(fieldDuration),
		Timing:       record.GetString(fieldTiming),
		PrescriberID: record.GetString(fieldPrescriber),
		PharmacyID:   record.GetString(fieldPharmacy),
		StartedOn:    startedOn,
		EndedOn:      endedOn,
		CreatedAt:    record.GetDateTime(fieldCreated).Time().UTC().Truncate(time.Millisecond),
		UpdatedAt:    record.GetDateTime(fieldUpdated).Time().UTC().Truncate(time.Millisecond),
		Version:      store.Version(record),
	}, nil
}

func ToRecord(record *core.Record, c clinical.CourseMedication) error {
	if err := expectCollection(record); err != nil {
		return err
	}

	record.Set(fieldTreatment, c.TreatmentID)
	record.Set(fieldMedication, c.MedicationID)
	record.Set(fieldDosage, c.Dosage)
	record.Set(fieldFrequency, c.Frequency)
	record.Set(fieldDuration, c.Duration)
	record.Set(fieldTiming, c.Timing)
	record.Set(fieldPrescriber, c.PrescriberID)
	record.Set(fieldPharmacy, c.PharmacyID)
	setDate(record, fieldStartedOn, c.StartedOn)
	setDate(record, fieldEndedOn, c.EndedOn)

	return nil
}

func recordDate(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("%s.%s is not a calendar date: %w", Collection, field, err)
	}

	return date, nil
}

func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

func expectCollection(record *core.Record) error {
	collection := record.Collection()
	if collection == nil || collection.Name != Collection {
		return fmt.Errorf("%w", ErrUnexpectedCollection)
	}

	return nil
}
