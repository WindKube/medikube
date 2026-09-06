package symptom

import (
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

const (
	fieldPatient       = "patient"
	fieldName          = "name"
	fieldCategory      = "category"
	fieldSeverity      = "severity"
	fieldOccurredAt    = "occurred_at"
	fieldDurationMin   = "duration_minutes"
	fieldPainScale     = "pain_scale"
	fieldBodySite      = "body_site"
	fieldTriggers      = "triggers"
	fieldReliefMethods = "relief_methods"
	fieldImpact        = "impact"
	fieldResolvedAt    = "resolved_at"
	fieldIsChronic     = "is_chronic"
	fieldStatus        = "status"
	fieldTags          = "tags"
	fieldCreated       = "created"
	fieldUpdated       = "updated"
)

// ErrUnexpectedCollection is a record handed to the wrong mapper.
var ErrUnexpectedCollection = errors.New("store/symptom: the record is not from this collection")

// FromRecord reads a stored row into the entity. It does not validate: a row
// already stored is a fact whatever the current rules say about it.
func FromRecord(record *core.Record) (clinical.Symptom, error) {
	if err := expectCollection(record); err != nil {
		return clinical.Symptom{}, err
	}

	occurredAt, err := recordInstant(record, fieldOccurredAt)
	if err != nil {
		return clinical.Symptom{}, err
	}

	resolvedAt, err := recordInstant(record, fieldResolvedAt)
	if err != nil {
		return clinical.Symptom{}, err
	}

	return clinical.Symptom{
		ID:              record.Id,
		PatientID:       record.GetString(fieldPatient),
		Name:            record.GetString(fieldName),
		Category:        clinical.SymptomCategory(record.GetString(fieldCategory)),
		Severity:        clinical.Severity(record.GetString(fieldSeverity)),
		OccurredAt:      occurredAt,
		DurationMinutes: recordIntPtr(record, fieldDurationMin),
		PainScale:       recordIntPtr(record, fieldPainScale),
		BodySite:        record.GetString(fieldBodySite),
		Triggers:        record.GetStringSlice(fieldTriggers),
		ReliefMethods:   record.GetStringSlice(fieldReliefMethods),
		Impact:          clinical.SymptomImpact(record.GetString(fieldImpact)),
		ResolvedAt:      resolvedAt,
		IsChronic:       record.GetBool(fieldIsChronic),
		Tags:            record.GetStringSlice(fieldTags),
		Status:          clinical.ConditionStatus(record.GetString(fieldStatus)),
		CreatedAt:       record.GetDateTime(fieldCreated).Time().UTC().Truncate(time.Millisecond),
		UpdatedAt:       record.GetDateTime(fieldUpdated).Time().UTC().Truncate(time.Millisecond),
		Version:         store.Version(record),
	}, nil
}

// ToRecord writes the entity's own columns onto the record. It never writes
// episode_count or last_occurred_at — FR-031's aggregate is never stored.
func ToRecord(record *core.Record, s clinical.Symptom) error {
	if err := expectCollection(record); err != nil {
		return err
	}

	record.Set(fieldPatient, s.PatientID)
	record.Set(fieldName, s.Name)
	record.Set(fieldCategory, string(s.Category))
	record.Set(fieldSeverity, string(s.Severity))
	record.Set(fieldOccurredAt, s.OccurredAt.Time())
	setIntPtr(record, fieldDurationMin, s.DurationMinutes)
	setIntPtr(record, fieldPainScale, s.PainScale)
	record.Set(fieldBodySite, s.BodySite)
	record.Set(fieldTriggers, orEmpty(s.Triggers))
	record.Set(fieldReliefMethods, orEmpty(s.ReliefMethods))
	record.Set(fieldImpact, string(s.Impact))

	if s.ResolvedAt.IsZero() {
		record.Set(fieldResolvedAt, time.Time{})
	} else {
		record.Set(fieldResolvedAt, s.ResolvedAt.Time())
	}

	record.Set(fieldIsChronic, s.IsChronic)
	record.Set(fieldTags, s.Tags)
	record.Set(fieldStatus, string(s.Status))

	return nil
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}

// recordIntPtr and setIntPtr treat 0 as "not recorded", the same convention
// PocketBase's own NumberField uses (core: ValidateValue skips Min/Max for an
// exact zero) and the one data-model's other optional numeric columns already
// live with (patients.height_cm, patients.weight_kg): a NumberField column is
// `NOT NULL DEFAULT 0`, so a genuine zero and an unset field are the same row
// once written. A pain rating of exactly zero is the one case this loses; it
// is a documented PocketBase limitation and not a bug this mapper introduces.
func recordIntPtr(record *core.Record, field string) *int {
	n := record.GetInt(field)
	if n == 0 {
		return nil
	}

	return &n
}

func setIntPtr(record *core.Record, field string, value *int) {
	if value == nil {
		record.Set(field, 0)

		return
	}

	record.Set(field, *value)
}

func recordInstant(record *core.Record, field string) (clinical.Instant, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return clinical.Instant{}, nil
	}

	return clinical.NewInstant(stored.Time().UTC()), nil
}

func expectCollection(record *core.Record) error {
	collection := record.Collection()
	if collection == nil || collection.Name != kind.Symptom.Collection() {
		return fmt.Errorf("%w", ErrUnexpectedCollection)
	}

	return nil
}
