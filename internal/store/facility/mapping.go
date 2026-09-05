package facility

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/directory"
	"medikube/internal/store"
)

// recordFromFacility reads one row into the entity. It is the round trip
// facilitytest.RunRepositoryContract's create-then-read case asserts every
// field of.
func recordFromFacility(record *core.Record) directory.Facility {
	return directory.Facility{
		ID:           record.Id,
		OwnerID:      record.GetString(fieldOwner),
		Kind:         directory.FacilityKind(record.GetString(fieldKind)),
		Name:         record.GetString(fieldName),
		Brand:        record.GetString(fieldBrand),
		Street:       record.GetString(fieldStreet),
		City:         record.GetString(fieldCity),
		Region:       record.GetString(fieldRegion),
		PostalCode:   record.GetString(fieldPostalCode),
		Country:      record.GetString(fieldCountry),
		Phone:        record.GetString(fieldPhone),
		Fax:          record.GetString(fieldFax),
		Email:        record.GetString(fieldEmail),
		Website:      record.GetString(fieldWebsite),
		PortalURL:    record.GetString(fieldPortalURL),
		Hours:        record.GetString(fieldHours),
		Open24h:      record.GetBool(fieldOpen24h),
		DriveThrough: record.GetBool(fieldDriveThrough),
		Services:     record.GetString(fieldServices),
		Notes:        record.GetString(fieldNotes),
		CreatedAt:    recordInstant(record, fieldCreated),
		UpdatedAt:    recordInstant(record, fieldUpdated),
		Version:      store.Version(record),
	}
}

// facilityToRecord writes every field but the four PocketBase owns: id,
// created, updated and — derived from updated — the version.
func facilityToRecord(record *core.Record, entity directory.Facility) error {
	if record.Collection().Name != collectionName {
		return fmt.Errorf("facility: expected the %s collection, got %s", collectionName, record.Collection().Name)
	}

	record.Set(fieldOwner, entity.OwnerID)
	record.Set(fieldKind, string(entity.Kind))
	record.Set(fieldName, entity.Name)
	record.Set(fieldBrand, entity.Brand)
	record.Set(fieldStreet, entity.Street)
	record.Set(fieldCity, entity.City)
	record.Set(fieldRegion, entity.Region)
	record.Set(fieldPostalCode, entity.PostalCode)
	record.Set(fieldCountry, entity.Country)
	record.Set(fieldPhone, entity.Phone)
	record.Set(fieldFax, entity.Fax)
	record.Set(fieldEmail, entity.Email)
	record.Set(fieldWebsite, entity.Website)
	record.Set(fieldPortalURL, entity.PortalURL)
	record.Set(fieldHours, entity.Hours)
	record.Set(fieldOpen24h, entity.Open24h)
	record.Set(fieldDriveThrough, entity.DriveThrough)
	record.Set(fieldServices, entity.Services)
	record.Set(fieldNotes, entity.Notes)

	return nil
}

// recordInstant reads a stored instant in UTC, truncated to the millisecond
// precision the column actually holds — matching internal/store's own
// recordInstant, so a create's returned entity compares equal to a re-read of
// the same row.
func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
