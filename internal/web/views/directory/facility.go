package directory

import (
	"context"

	"medikube/internal/domain/directory"
	"medikube/internal/i18n"
	"medikube/internal/web/views/records"
)

// FacilitySegment is contracts/pages.md's P5/P6 path segment.
const FacilitySegment = "facilities"

const (
	FieldFacilityKind         = "kind"
	FieldFacilityName         = "name"
	FieldFacilityBrand        = "brand"
	FieldFacilityStreet       = "street"
	FieldFacilityCity         = "city"
	FieldFacilityRegion       = "region"
	FieldFacilityPostalCode   = "postal_code"
	FieldFacilityCountry      = "country"
	FieldFacilityPhone        = "phone"
	FieldFacilityFax          = "fax"
	FieldFacilityEmail        = "email"
	FieldFacilityWebsite      = "website"
	FieldFacilityPortalURL    = "portal_url"
	FieldFacilityHours        = "hours"
	FieldFacilityOpen24h      = "open_24h"
	FieldFacilityDriveThrough = "drive_through"
	FieldFacilityServices     = "services"
	FieldFacilityNotes        = "notes"
)

var facilityFieldLabelIDs = map[string]string{
	FieldFacilityKind:         "field.facility.kind",
	FieldFacilityName:         "field.facility.name",
	FieldFacilityBrand:        "field.facility.brand",
	FieldFacilityStreet:       "field.facility.street",
	FieldFacilityCity:         "field.facility.city",
	FieldFacilityRegion:       "field.facility.region",
	FieldFacilityPostalCode:   "field.facility.postal_code",
	FieldFacilityCountry:      "field.facility.country",
	FieldFacilityPhone:        "field.facility.phone",
	FieldFacilityFax:          "field.facility.fax",
	FieldFacilityEmail:        "field.facility.email",
	FieldFacilityWebsite:      "field.facility.website",
	FieldFacilityPortalURL:    "field.facility.portal_url",
	FieldFacilityHours:        "field.facility.hours",
	FieldFacilityOpen24h:      "field.facility.open_24h",
	FieldFacilityDriveThrough: "field.facility.drive_through",
	FieldFacilityServices:     "field.facility.services",
	FieldFacilityNotes:        "field.facility.notes",
}

func FacilityFieldLabel(ctx context.Context, field string) string {
	if id, known := facilityFieldLabelIDs[field]; known {
		return i18n.T(ctx, id)
	}

	return field
}

var facilityKindLabelIDs = map[directory.FacilityKind]string{
	directory.FacilityKindPractice: "enum.facility_kind.practice",
	directory.FacilityKindPharmacy: "enum.facility_kind.pharmacy",
	directory.FacilityKindHospital: "enum.facility_kind.hospital",
	directory.FacilityKindLab:      "enum.facility_kind.lab",
	directory.FacilityKindImaging:  "enum.facility_kind.imaging",
	directory.FacilityKindOther:    "enum.facility_kind.other",
}

func FacilityKindLabel(ctx context.Context, value directory.FacilityKind) string {
	if value == "" {
		return ""
	}

	if id, known := facilityKindLabelIDs[value]; known {
		return i18n.T(ctx, id)
	}

	return string(value)
}

func FacilityKindOptions(ctx context.Context, selected directory.FacilityKind) []records.Option {
	published := directory.FacilityKinds()
	options := make([]records.Option, 0, len(published))

	for _, value := range published {
		options = append(options, records.Option{
			Value:    string(value),
			Label:    FacilityKindLabel(ctx, value),
			Selected: value == selected,
		})
	}

	return options
}

type FacilityLinks struct {
	Detail string
	Record string
}

type FacilityView struct {
	ID           string
	Kind         string
	KindValue    string
	Name         string
	Brand        string
	Street       string
	City         string
	Region       string
	PostalCode   string
	Country      string
	Phone        string
	Fax          string
	Email        string
	Website      string
	PortalURL    string
	Hours        string
	Open24h      bool
	DriveThrough bool
	Services     string
	Notes        string

	UsagePractitioners int
	UsageRecords       int

	Version string
	Links   FacilityLinks
}

func NewFacilityView(ctx context.Context, f directory.Facility, links FacilityLinks) FacilityView {
	return FacilityView{
		ID:           f.ID,
		Kind:         FacilityKindLabel(ctx, f.Kind),
		KindValue:    string(f.Kind),
		Name:         f.Name,
		Brand:        f.Brand,
		Street:       f.Street,
		City:         f.City,
		Region:       f.Region,
		PostalCode:   f.PostalCode,
		Country:      f.Country,
		Phone:        f.Phone,
		Fax:          f.Fax,
		Email:        f.Email,
		Website:      f.Website,
		PortalURL:    f.PortalURL,
		Hours:        f.Hours,
		Open24h:      f.Open24h,
		DriveThrough: f.DriveThrough,
		Services:     f.Services,
		Notes:        f.Notes,
		Version:      f.Version,
		Links:        links,
	}
}

func (f FacilityView) Value(field string) string {
	switch field {
	case FieldFacilityKind:
		return f.KindValue
	case FieldFacilityName:
		return f.Name
	case FieldFacilityBrand:
		return f.Brand
	case FieldFacilityStreet:
		return f.Street
	case FieldFacilityCity:
		return f.City
	case FieldFacilityRegion:
		return f.Region
	case FieldFacilityPostalCode:
		return f.PostalCode
	case FieldFacilityCountry:
		return f.Country
	case FieldFacilityPhone:
		return f.Phone
	case FieldFacilityFax:
		return f.Fax
	case FieldFacilityEmail:
		return f.Email
	case FieldFacilityWebsite:
		return f.Website
	case FieldFacilityPortalURL:
		return f.PortalURL
	case FieldFacilityHours:
		return f.Hours
	case FieldFacilityServices:
		return f.Services
	case FieldFacilityNotes:
		return f.Notes
	default:
		return ""
	}
}

func (f FacilityView) KindOptions(ctx context.Context) []records.Option {
	return FacilityKindOptions(ctx, directory.FacilityKind(f.KindValue))
}

func (f FacilityView) Entries(ctx context.Context) []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldFacilityKind, Value: f.Kind},
		{Field: FieldFacilityBrand, Value: f.Brand},
		{Field: FieldFacilityStreet, Value: f.Street},
		{Field: FieldFacilityCity, Value: f.City},
		{Field: FieldFacilityRegion, Value: f.Region},
		{Field: FieldFacilityPostalCode, Value: f.PostalCode},
		{Field: FieldFacilityCountry, Value: f.Country},
		{Field: FieldFacilityPhone, Value: f.Phone},
		{Field: FieldFacilityFax, Value: f.Fax},
		{Field: FieldFacilityEmail, Value: f.Email},
		{Field: FieldFacilityWebsite, Value: f.Website},
		{Field: FieldFacilityPortalURL, Value: f.PortalURL},
		{Field: FieldFacilityHours, Value: f.Hours},
		{Field: FieldFacilityServices, Value: f.Services, Multiline: true},
		{Field: FieldFacilityNotes, Value: f.Notes, Multiline: true},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = FacilityFieldLabel(ctx, entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

type FacilityListProps struct {
	Facilities []FacilityView
	CreateHref string
	NextHref   string
}

type FacilityDetailProps struct {
	Facility FacilityView
}

type FacilityFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string
	Facility   FacilityView
	Errors     FieldErrors

	// Notice is set when the form was re-rendered from the server's current
	// values after a stale If-Match.
	Notice string
}

func (f FacilityFormProps) Label(ctx context.Context) string {
	if f.New {
		return i18n.T(ctx, "directory.add_facility")
	}

	return i18n.T(ctx, "directory.edit_facility")
}

func (f FacilityFormProps) SubmitLabel(ctx context.Context) string {
	if f.New {
		return i18n.T(ctx, "directory.add_facility_submit")
	}

	return i18n.T(ctx, "action.save_changes")
}
