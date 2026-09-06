package immunization

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

func recordFromImmunization(record *core.Record) (clinical.Immunization, error) {
	administeredOn, err := recordDate(record, fieldAdministered)
	if err != nil {
		return clinical.Immunization{}, err
	}

	expiresOn, err := recordDate(record, fieldExpiresOn)
	if err != nil {
		return clinical.Immunization{}, err
	}

	return clinical.Immunization{
		ID:             record.Id,
		PatientID:      record.GetString(fieldPatient),
		PractitionerID: record.GetString(fieldPractitioner),
		FacilityID:     record.GetString(fieldFacility),
		VaccineName:    record.GetString(fieldVaccineName),
		TradeName:      record.GetString(fieldTradeName),
		AdministeredOn: administeredOn,
		DoseNumber:     doseFromRecord(record),
		LotNumber:      record.GetString(fieldLotNumber),
		Manufacturer:   record.GetString(fieldManufacturer),
		Site:           clinical.ImmunizationSite(record.GetString(fieldSite)),
		Route:          clinical.ImmunizationRoute(record.GetString(fieldRoute)),
		ExpiresOn:      expiresOn,
		Tags:           record.GetStringSlice(fieldTags),
		CreatedAt:      recordInstant(record, fieldCreated),
		UpdatedAt:      recordInstant(record, fieldUpdated),
		Version:        store.Version(record),
	}, nil
}

func immunizationToRecord(record *core.Record, entity clinical.Immunization) error {
	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldPractitioner, entity.PractitionerID)
	record.Set(fieldFacility, entity.FacilityID)
	record.Set(fieldVaccineName, entity.VaccineName)
	record.Set(fieldTradeName, entity.TradeName)
	setDate(record, fieldAdministered, entity.AdministeredOn)

	if entity.DoseNumber != nil {
		record.Set(fieldDoseNumber, *entity.DoseNumber)
	} else {
		record.Set(fieldDoseNumber, 0)
	}

	record.Set(fieldLotNumber, entity.LotNumber)
	record.Set(fieldManufacturer, entity.Manufacturer)
	record.Set(fieldSite, string(entity.Site))
	record.Set(fieldRoute, string(entity.Route))
	setDate(record, fieldExpiresOn, entity.ExpiresOn)
	record.Set(fieldTags, entity.Tags)

	return nil
}

func doseFromRecord(record *core.Record) *int {
	raw := record.GetInt(fieldDoseNumber)
	if raw == 0 {
		return nil
	}

	return &raw
}

// recordDate and setDate mirror internal/store's own (unexported there), the
// same duplication internal/store/facility and internal/store/practitioner
// already accept for a per-kind mapper. A DateField stores a full instant, so
// the zero value must be recognised before Scan converts it — otherwise "no
// date recorded" becomes 1 January of year 1 (research D-27).
func recordDate(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("%s.%s is not a calendar date: %w", kind.Immunization.Collection(), field, err)
	}

	return date, nil
}

func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
